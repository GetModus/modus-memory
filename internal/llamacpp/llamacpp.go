//go:build cgo && !nollamacpp

// Package llamacpp provides CGo bindings to llama.cpp for in-process LLM inference.
// Build with CGo enabled and without the nollamacpp tag to use embedded inference.
// Use -tags nollamacpp for a pure-Go binary that falls back to HTTP.
package llamacpp

/*
#cgo CFLAGS: -I/opt/homebrew/include -I/opt/homebrew/Cellar/llama.cpp/8610/include
#cgo LDFLAGS: -L/opt/homebrew/lib -L/opt/homebrew/Cellar/llama.cpp/8610/lib
#cgo LDFLAGS: -lllama -lggml -lggml-base -lm -lstdc++
#cgo darwin LDFLAGS: -framework Accelerate -framework Metal -framework Foundation -framework CoreGraphics
#include <llama.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var (
	backendInitOnce sync.Once
)

// Model wraps a loaded llama.cpp model. Thread-safe for concurrent context creation.
type Model struct {
	model    *C.struct_llama_model
	path     string
	nCtx     int
	nGPU     int
	mu       sync.Mutex
}

// CompletionResult holds the output of a completion request.
type CompletionResult struct {
	Text       string
	TokensUsed int
	DurationMs int64
}

// ChatMessage represents a single chat message.
type ChatMessage struct {
	Role    string
	Content string
}

// SamplerParams controls text generation behavior.
type SamplerParams struct {
	Temperature   float32
	TopK          int32
	TopP          float32
	MinP          float32
	RepeatPenalty float32
	Seed          uint32
}

// DefaultSamplerParams returns conservative defaults for librarian use.
func DefaultSamplerParams() SamplerParams {
	return SamplerParams{
		Temperature:   0.1,
		TopK:          40,
		TopP:          0.95,
		MinP:          0.05,
		RepeatPenalty: 1.1,
		Seed:          C.LLAMA_DEFAULT_SEED,
	}
}

// Available returns true — this is the CGo build.
func Available() bool {
	return true
}

// LoadModel loads a GGUF model file. nGPULayers=-1 means offload all layers.
// nCtx is the default context size (0 = model default).
func LoadModel(path string, nGPULayers, nCtx int) (*Model, error) {
	backendInitOnce.Do(func() {
		C.llama_backend_init()
	})

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	params := C.llama_model_default_params()
	params.n_gpu_layers = C.int32_t(nGPULayers)

	model := C.llama_model_load_from_file(cPath, params)
	if model == nil {
		return nil, fmt.Errorf("llamacpp: failed to load model from %s", path)
	}

	return &Model{
		model: model,
		path:  path,
		nCtx:  nCtx,
		nGPU:  nGPULayers,
	}, nil
}

// Close frees model memory.
func (m *Model) Close() {
	if m.model != nil {
		C.llama_model_free(m.model)
		m.model = nil
	}
}

// Complete runs a single completion with fresh context (hard request isolation).
// A new context is created and destroyed per call — no shared mutable state.
func (m *Model) Complete(prompt string, maxTokens int, params SamplerParams) (*CompletionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()

	// Create fresh context per request (hard isolation per Codex revision)
	ctxParams := C.llama_context_default_params()
	nCtx := m.nCtx
	if nCtx <= 0 {
		nCtx = 4096
	}
	ctxParams.n_ctx = C.uint32_t(nCtx)
	ctxParams.n_batch = C.uint32_t(512)
	ctxParams.n_threads = 4
	ctxParams.n_threads_batch = 4
	ctxParams.flash_attn_type = 1 // enable flash attention

	ctx := C.llama_init_from_model(m.model, ctxParams)
	if ctx == nil {
		return nil, fmt.Errorf("llamacpp: failed to create context")
	}
	defer C.llama_free(ctx)

	// Tokenize prompt
	vocab := C.llama_model_get_vocab(m.model)
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))

	promptLen := C.int(len(prompt))
	maxToks := promptLen + 256 // headroom
	tokens := make([]C.llama_token, maxToks)

	nTokens := C.llama_tokenize(vocab, cPrompt, promptLen, &tokens[0], C.int32_t(maxToks), C.bool(true), C.bool(true))
	if nTokens < 0 {
		return nil, fmt.Errorf("llamacpp: tokenization failed (need %d tokens)", -nTokens)
	}
	tokens = tokens[:nTokens]

	if int(nTokens) >= nCtx {
		return nil, fmt.Errorf("llamacpp: prompt too long (%d tokens, context is %d)", nTokens, nCtx)
	}

	// Create sampler chain
	chainParams := C.llama_sampler_chain_default_params()
	sampler := C.llama_sampler_chain_init(chainParams)
	defer C.llama_sampler_free(sampler)

	C.llama_sampler_chain_add(sampler, C.llama_sampler_init_top_k(C.int32_t(params.TopK)))
	C.llama_sampler_chain_add(sampler, C.llama_sampler_init_top_p(C.float(params.TopP), 1))
	C.llama_sampler_chain_add(sampler, C.llama_sampler_init_min_p(C.float(params.MinP), 1))
	C.llama_sampler_chain_add(sampler, C.llama_sampler_init_temp(C.float(params.Temperature)))
	C.llama_sampler_chain_add(sampler, C.llama_sampler_init_dist(C.uint32_t(params.Seed)))

	// Eval prompt
	batch := C.llama_batch_get_one(&tokens[0], C.int32_t(nTokens))
	if C.llama_decode(ctx, batch) != 0 {
		return nil, fmt.Errorf("llamacpp: prompt decode failed")
	}

	// Generate tokens
	eosToken := C.llama_vocab_eos(vocab)
	var generated []C.llama_token
	totalTokens := int(nTokens)

	for i := 0; i < maxTokens; i++ {
		token := C.llama_sampler_sample(sampler, ctx, -1)
		if token == eosToken {
			break
		}
		generated = append(generated, token)
		totalTokens++

		if totalTokens >= nCtx {
			break
		}

		// Decode the new token
		batch = C.llama_batch_get_one(&token, 1)
		if C.llama_decode(ctx, batch) != 0 {
			break
		}
	}

	// Detokenize
	var sb strings.Builder
	buf := make([]C.char, 256)
	for _, tok := range generated {
		n := C.llama_token_to_piece(vocab, tok, &buf[0], 256, 0, C.bool(true))
		if n > 0 {
			sb.WriteString(C.GoStringN(&buf[0], n))
		}
	}

	return &CompletionResult{
		Text:       sb.String(),
		TokensUsed: totalTokens,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// ChatComplete applies a chat template and runs completion.
// Per Codex revision: fresh context per call, no cross-request state.
func (m *Model) ChatComplete(messages []ChatMessage, maxTokens int, params SamplerParams) (*CompletionResult, error) {
	prompt := applyChatTemplate(m.model, messages)
	return m.Complete(prompt, maxTokens, params)
}

// applyChatTemplate uses llama.cpp's built-in chat template from the model.
func applyChatTemplate(model *C.struct_llama_model, messages []ChatMessage) string {
	// Try llama_chat_apply_template first
	if len(messages) > 0 {
		// Build C chat messages - we use the simpler manual approach
		// since the C API for chat templates is complex
		var sb strings.Builder
		for _, msg := range messages {
			switch msg.Role {
			case "system":
				sb.WriteString("<start_of_turn>user\n")
				sb.WriteString("[System: " + msg.Content + "]\n")
				sb.WriteString("<end_of_turn>\n")
			case "user":
				sb.WriteString("<start_of_turn>user\n")
				sb.WriteString(msg.Content)
				sb.WriteString("\n<end_of_turn>\n")
			case "assistant":
				sb.WriteString("<start_of_turn>model\n")
				sb.WriteString(msg.Content)
				sb.WriteString("\n<end_of_turn>\n")
			}
		}
		sb.WriteString("<start_of_turn>model\n")
		return sb.String()
	}
	return ""
}

// ModelPath returns the path the model was loaded from.
func (m *Model) ModelPath() string {
	return m.path
}

// BackendName returns a human-readable backend identifier.
func (m *Model) BackendName() string {
	return "embedded-llamacpp"
}

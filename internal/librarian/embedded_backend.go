package librarian

import (
	"fmt"
	"strings"

	"github.com/GetModus/modus-memory/internal/llamacpp"
)

// EmbeddedBackend runs inference in-process via llama.cpp CGo bindings.
// Per Codex revision: fresh context per completion, no shared mutable context.
type EmbeddedBackend struct {
	model *llamacpp.Model
}

// NewEmbeddedBackend loads a GGUF model for in-process inference.
// nGPULayers=-1 offloads all layers to Metal. nCtx=0 uses model default.
func NewEmbeddedBackend(modelPath string, nGPULayers, nCtx int) (*EmbeddedBackend, error) {
	if !llamacpp.Available() {
		return nil, fmt.Errorf("embedded backend: llamacpp not available (built with nollamacpp?)")
	}

	model, err := llamacpp.LoadModel(modelPath, nGPULayers, nCtx)
	if err != nil {
		return nil, fmt.Errorf("embedded backend: %w", err)
	}

	return &EmbeddedBackend{model: model}, nil
}

func (e *EmbeddedBackend) Available() bool {
	return e.model != nil
}

func (e *EmbeddedBackend) Complete(system, user string, maxTokens int, temperature float64) (string, error) {
	if e.model == nil {
		return "", fmt.Errorf("embedded backend: model not loaded")
	}

	params := llamacpp.DefaultSamplerParams()
	params.Temperature = float32(temperature)

	messages := []llamacpp.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}

	result, err := e.model.ChatComplete(messages, maxTokens, params)
	if err != nil {
		return "", fmt.Errorf("embedded backend: %w", err)
	}

	return strings.TrimSpace(result.Text), nil
}

func (e *EmbeddedBackend) Identity() string {
	if e.model != nil {
		return "embedded:" + e.model.BackendName()
	}
	return "embedded:unloaded"
}

func (e *EmbeddedBackend) Close() error {
	if e.model != nil {
		e.model.Close()
		e.model = nil
	}
	return nil
}

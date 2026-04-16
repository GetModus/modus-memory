//go:build !cgo || nollamacpp

// Stub implementation for pure-Go builds without llama.cpp.
// All functions return ErrNotAvailable. Use -tags nollamacpp or disable CGo.
package llamacpp

import "errors"

// ErrNotAvailable is returned when llamacpp is not compiled in.
var ErrNotAvailable = errors.New("llamacpp: not available (built without CGo or with nollamacpp tag)")

// Model is a stub — no-op in pure-Go builds.
type Model struct{}

// CompletionResult holds completion output.
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

// SamplerParams controls text generation.
type SamplerParams struct {
	Temperature   float32
	TopK          int32
	TopP          float32
	MinP          float32
	RepeatPenalty float32
	Seed          uint32
}

// DefaultSamplerParams returns defaults (stub).
func DefaultSamplerParams() SamplerParams {
	return SamplerParams{Temperature: 0.1, TopK: 40, TopP: 0.95, MinP: 0.05, RepeatPenalty: 1.1}
}

// Available returns false — this is the stub build.
func Available() bool { return false }

// LoadModel always returns ErrNotAvailable.
func LoadModel(path string, nGPULayers, nCtx int) (*Model, error) {
	return nil, ErrNotAvailable
}

// Close is a no-op.
func (m *Model) Close() {}

// Complete always returns ErrNotAvailable.
func (m *Model) Complete(prompt string, maxTokens int, params SamplerParams) (*CompletionResult, error) {
	return nil, ErrNotAvailable
}

// ChatComplete always returns ErrNotAvailable.
func (m *Model) ChatComplete(messages []ChatMessage, maxTokens int, params SamplerParams) (*CompletionResult, error) {
	return nil, ErrNotAvailable
}

// ModelPath returns empty string.
func (m *Model) ModelPath() string { return "" }

// BackendName returns "disabled".
func (m *Model) BackendName() string { return "disabled" }

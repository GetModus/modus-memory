package librarian

// Backend defines the interface for LLM inference backends.
// Implementations must be safe for concurrent use.
type Backend interface {
	// Available reports whether this backend can serve requests.
	Available() bool

	// Complete sends a system+user prompt and returns the response text.
	Complete(system, user string, maxTokens int, temperature float64) (string, error)

	// Identity returns a human-readable backend name for audit trails.
	Identity() string

	// Close releases backend resources.
	Close() error
}

// defaultBackend is the active backend. Set via SetBackend.
// If nil, falls back to legacy HTTP behavior.
var defaultBackend Backend

// SetBackend sets the active backend for all librarian calls.
func SetBackend(b Backend) {
	defaultBackend = b
}

// GetBackend returns the current active backend, or nil.
func GetBackend() Backend {
	return defaultBackend
}

// BackendIdentity returns the current backend's identity string.
func BackendIdentity() string {
	if defaultBackend != nil {
		return defaultBackend.Identity()
	}
	return "none"
}

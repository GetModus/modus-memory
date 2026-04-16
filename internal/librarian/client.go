// Package librarian provides a local-first context shaping layer for AI
// systems. It can sit between raw text and downstream tools or models to
// expand weak queries, rerank results, summarize context, extract structured
// facts, and produce briefings.
package librarian

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	// Endpoint is the default HTTP backend address.
	Endpoint = "http://127.0.0.1:8090"

	// EndpointEnv is the neutral public environment variable for the HTTP backend.
	EndpointEnv = "LIBRARIAN_ENDPOINT"

	// ModusEndpointEnv is the compatibility alias used by existing MODUS flows.
	ModusEndpointEnv = "MODUS_LIBRARIAN_URL"
)

// ResolveEndpoint returns the effective HTTP endpoint for the Librarian.
// Precedence:
// 1. MODUS_LIBRARIAN_URL (compatibility alias)
// 2. LIBRARIAN_ENDPOINT
// 3. Endpoint constant default
func ResolveEndpoint() string {
	if v := strings.TrimSpace(os.Getenv(ModusEndpointEnv)); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(EndpointEnv)); v != "" {
		return v
	}
	return Endpoint
}

// Available checks whether any backend is reachable.
func Available() bool {
	if defaultBackend != nil {
		return defaultBackend.Available()
	}
	// Compatibility fallback: try HTTP if no explicit backend has been set.
	return NewHTTPBackend(ResolveEndpoint()).Available()
}

// Call sends a system+user prompt to the librarian and returns the text response.
func Call(system, user string, maxTokens int) string {
	return CallWithTemp(system, user, maxTokens, 0.1)
}

// CallWithTemp sends a prompt with a custom temperature.
func CallWithTemp(system, user string, maxTokens int, temperature float64) string {
	backend := defaultBackend
	if backend == nil {
		// Compatibility fallback: direct HTTP if no explicit backend has been set.
		backend = NewHTTPBackend(ResolveEndpoint())
	}

	result, err := backend.Complete(system, user, maxTokens, temperature)
	if err != nil {
		log.Printf("librarian: call failed: %v", err)
		return ""
	}
	return result
}

// StripFences removes markdown code fences and LLM stop tokens from output.
func StripFences(text string) string {
	for _, stop := range []string{"<|user|>", "<|endoftext|>", "<|im_end|>", "<|assistant|>"} {
		if idx := strings.Index(text, stop); idx > 0 {
			text = text[:idx]
		}
	}
	clean := strings.TrimSpace(text)
	if strings.HasPrefix(clean, "```") {
		lines := strings.SplitN(clean, "\n", 2)
		if len(lines) > 1 {
			clean = lines[1]
		}
	}
	if strings.HasSuffix(clean, "```") {
		clean = clean[:len(clean)-3]
	}
	return strings.TrimSpace(clean)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// ParseJSON attempts to unmarshal a librarian response as JSON.
// It strips fences and stop tokens first.
func ParseJSON(text string, v interface{}) error {
	cleaned := StripFences(text)
	if err := json.Unmarshal([]byte(cleaned), v); err != nil {
		return fmt.Errorf("parse librarian JSON: %w (raw: %s)", err, truncate(cleaned, 100))
	}
	return nil
}

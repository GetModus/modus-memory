package librarian

import (
	"fmt"
	"os"
	"testing"
)

// mockBackend implements Backend for testing.
type mockBackend struct {
	available bool
	response  string
	err       error
	identity  string
	calls     int
}

func (m *mockBackend) Available() bool { return m.available }
func (m *mockBackend) Complete(system, user string, maxTokens int, temperature float64) (string, error) {
	m.calls++
	return m.response, m.err
}
func (m *mockBackend) Identity() string { return m.identity }
func (m *mockBackend) Close() error     { return nil }

func TestSetBackendRouting(t *testing.T) {
	// Save and restore original
	orig := defaultBackend
	defer func() { defaultBackend = orig }()

	mock := &mockBackend{
		available: true,
		response:  "mock response",
		identity:  "mock-backend",
	}
	SetBackend(mock)

	if !Available() {
		t.Error("expected Available() = true with mock backend")
	}

	result := Call("system", "user", 100)
	if result != "mock response" {
		t.Errorf("Call = %q, want %q", result, "mock response")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}

	if BackendIdentity() != "mock-backend" {
		t.Errorf("identity = %q, want mock-backend", BackendIdentity())
	}
}

func TestCallWithTempRouting(t *testing.T) {
	orig := defaultBackend
	defer func() { defaultBackend = orig }()

	mock := &mockBackend{
		available: true,
		response:  "temp response",
		identity:  "mock",
	}
	SetBackend(mock)

	result := CallWithTemp("sys", "usr", 50, 0.5)
	if result != "temp response" {
		t.Errorf("CallWithTemp = %q, want %q", result, "temp response")
	}
}

func TestBackendError(t *testing.T) {
	orig := defaultBackend
	defer func() { defaultBackend = orig }()

	mock := &mockBackend{
		available: true,
		err:       fmt.Errorf("backend down"),
		identity:  "mock",
	}
	SetBackend(mock)

	result := Call("sys", "usr", 100)
	if result != "" {
		t.Errorf("expected empty result on error, got %q", result)
	}
}

func TestNoBackendIdentity(t *testing.T) {
	orig := defaultBackend
	defer func() { defaultBackend = orig }()

	defaultBackend = nil
	if BackendIdentity() != "none" {
		t.Errorf("identity = %q, want none", BackendIdentity())
	}
}

func TestHTTPBackendIdentity(t *testing.T) {
	h := NewHTTPBackend("http://localhost:8090")
	if h.Identity() != "http:http://localhost:8090" {
		t.Errorf("identity = %q", h.Identity())
	}
}

func TestResolveEndpoint(t *testing.T) {
	origModus := os.Getenv(ModusEndpointEnv)
	origPlain := os.Getenv(EndpointEnv)
	defer func() {
		_ = os.Setenv(ModusEndpointEnv, origModus)
		_ = os.Setenv(EndpointEnv, origPlain)
	}()

	_ = os.Unsetenv(ModusEndpointEnv)
	_ = os.Unsetenv(EndpointEnv)
	if got := ResolveEndpoint(); got != Endpoint {
		t.Fatalf("ResolveEndpoint default = %q, want %q", got, Endpoint)
	}

	_ = os.Setenv(EndpointEnv, "http://127.0.0.1:9000")
	if got := ResolveEndpoint(); got != "http://127.0.0.1:9000" {
		t.Fatalf("ResolveEndpoint LIBRARIAN_ENDPOINT = %q", got)
	}

	_ = os.Setenv(ModusEndpointEnv, "http://127.0.0.1:9100")
	if got := ResolveEndpoint(); got != "http://127.0.0.1:9100" {
		t.Fatalf("ResolveEndpoint MODUS_LIBRARIAN_URL = %q", got)
	}

	_ = os.Setenv(ModusEndpointEnv, "   ")
	if got := ResolveEndpoint(); got != "http://127.0.0.1:9000" {
		t.Fatalf("ResolveEndpoint ignores blank MODUS override = %q", got)
	}
}

func TestStripFences(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"```json\n{\"key\": \"val\"}\n```", `{"key": "val"}`},
		{"plain text", "plain text"},
		{"some text<|endoftext|>trailing", "some text"},
	}
	for _, tc := range tests {
		got := StripFences(tc.input)
		if got != tc.want {
			t.Errorf("StripFences(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

package librarian

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPBackend wraps a local OpenAI-compatible HTTP inference endpoint such as
// an mlx-lm server.
type HTTPBackend struct {
	endpoint string
	client   *http.Client
}

// NewHTTPBackend creates an HTTP backend targeting the given endpoint.
func NewHTTPBackend(endpoint string) *HTTPBackend {
	return &HTTPBackend{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (h *HTTPBackend) Available() bool {
	check := &http.Client{Timeout: 2 * time.Second}
	resp, err := check.Get(h.endpoint + "/health")
	if err != nil {
		resp, err = check.Get(h.endpoint + "/v1/models")
		if err != nil {
			return false
		}
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (h *HTTPBackend) Complete(system, user string, maxTokens int, temperature float64) (string, error) {
	reqBody := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"max_tokens":           maxTokens,
		"temperature":          temperature,
		"chat_template_kwargs": map[string]interface{}{"enable_thinking": false},
	}

	jsonBody, _ := json.Marshal(reqBody)
	resp, err := h.client.Post(
		h.endpoint+"/v1/chat/completions",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return "", fmt.Errorf("http backend: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		return "", fmt.Errorf("http backend: parse failed (status %d)", resp.StatusCode)
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (h *HTTPBackend) Identity() string {
	return "http:" + h.endpoint
}

func (h *HTTPBackend) Close() error {
	return nil
}

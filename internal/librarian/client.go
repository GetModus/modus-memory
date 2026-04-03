// Package librarian provides the gateway between MODUS and the local LLM
// (Gemma 4 on llama-server). All vault search, triage, extraction, and
// enrichment routes through here. Cloud models never touch raw DB.
package librarian

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Endpoint is the librarian's address. Gemma 4 Q4_K_M via llama-server.
const Endpoint = "http://127.0.0.1:8090"

// Available checks whether the librarian is reachable.
func Available() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(Endpoint + "/health")
	if err != nil {
		// llama-server uses /health; fall back to /v1/models for MLX compat
		resp, err = client.Get(Endpoint + "/v1/models")
		if err != nil {
			return false
		}
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// Call sends a system+user prompt to the librarian and returns the text response.
func Call(system, user string, maxTokens int) string {
	return CallWithTemp(system, user, maxTokens, 0.1)
}

// CallWithTemp sends a prompt with a custom temperature.
func CallWithTemp(system, user string, maxTokens int, temperature float64) string {
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
	client := &http.Client{Timeout: 120 * time.Second}

	resp, err := client.Post(
		Endpoint+"/v1/chat/completions",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		log.Printf("librarian: call failed: %v", err)
		return ""
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
		log.Printf("librarian: parse failed: %v (body: %s)", err, truncate(string(body), 200))
		return ""
	}

	return strings.TrimSpace(result.Choices[0].Message.Content)
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

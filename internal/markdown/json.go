package markdown

import (
	"encoding/json"
	"strings"
)

// ExtractJSON finds and parses the first JSON object from text that may
// contain reasoning, markdown fencing, or other wrapper text.
// This mirrors the Python SAGE _parse_json with brace-matching.
func ExtractJSON(text string) (map[string]interface{}, error) {
	text = strings.TrimSpace(text)

	// Strip markdown code blocks
	if strings.Contains(text, "```") {
		parts := strings.Split(text, "```")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "json") {
				p = strings.TrimSpace(p[4:])
			}
			if strings.HasPrefix(p, "{") {
				text = p
				break
			}
		}
	}

	// Find first { via brace matching
	start := strings.Index(text, "{")
	if start < 0 {
		return nil, json.Unmarshal([]byte("{}"), new(interface{}))
	}

	depth := 0
	inString := false
	escape := false
	end := -1

	for i := start; i < len(text); i++ {
		c := text[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}

	var candidate string
	if end < 0 {
		candidate = text[start:] + "}"
	} else {
		candidate = text[start : end+1]
	}

	// Try parsing as-is first
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(candidate), &result); err == nil {
		return result, nil
	}

	// Escape literal newlines/tabs and retry
	cleaned := strings.ReplaceAll(candidate, "\n", "\\n")
	cleaned = strings.ReplaceAll(cleaned, "\t", "\\t")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\\r")

	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ExtractScore extracts a float "score" from a model response.
func ExtractScore(text string) float64 {
	data, err := ExtractJSON(text)
	if err != nil {
		return 0.5
	}
	score, ok := data["score"].(float64)
	if !ok {
		return 0.5
	}
	if score > 1.0 {
		score = (score - 1) / 9.0
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

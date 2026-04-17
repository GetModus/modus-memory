package app

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type khojConversation struct {
	Title   string `json:"title"`
	Agent   string `json:"agent"`
	Created string `json:"created_at"`
	Updated string `json:"updated_at"`
	ChatLog struct {
		Chat []khojMessage `json:"chat"`
	} `json:"conversation_log"`
	FileFilters []string `json:"file_filters"`
}

type khojMessage struct {
	By      string      `json:"by"`
	Message interface{} `json:"message"`
	Created string      `json:"created"`
	Intent  *struct {
		Type     string   `json:"type"`
		Query    string   `json:"query"`
		Inferred []string `json:"inferred-queries"`
	} `json:"intent,omitempty"`
	Context []struct {
		Compiled string `json:"compiled"`
		File     string `json:"file"`
	} `json:"context,omitempty"`
}

func runImportKhoj(exportPath, vaultDir string) {
	data, err := readKhojExport(exportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading export: %v\n", err)
		os.Exit(1)
	}

	var conversations []khojConversation
	if err := json.Unmarshal(data, &conversations); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing conversations: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d conversations in Khoj export\n", len(conversations))

	convDir := filepath.Join(vaultDir, "brain", "khoj")
	factsDir := filepath.Join(vaultDir, "memory", "facts")
	_ = os.MkdirAll(convDir, 0o755)
	_ = os.MkdirAll(factsDir, 0o755)

	convCount := 0
	factCount := 0
	seenContexts := make(map[string]bool)

	for _, conv := range conversations {
		if len(conv.ChatLog.Chat) == 0 {
			continue
		}

		slug := slugify(conv.Title)
		if slug == "" {
			slug = fmt.Sprintf("conversation-%d", convCount+1)
		}

		created := parseKhojTime(conv.Created)
		filename := fmt.Sprintf("%s-%s.md", created.Format("2006-01-02"), slug)
		path := filepath.Join(convDir, filename)

		if _, err := os.Stat(path); err == nil {
			continue
		}

		fm := map[string]interface{}{
			"title":   conv.Title,
			"source":  "khoj",
			"kind":    "conversation",
			"created": created.Format(time.RFC3339),
			"agent":   conv.Agent,
		}

		tags := collectTags(conv)
		if len(tags) > 0 {
			fm["tags"] = tags
		}

		var body strings.Builder
		for _, msg := range conv.ChatLog.Chat {
			text := messageText(msg.Message)
			if text == "" {
				continue
			}

			if msg.By == "user" {
				body.WriteString("**User:** ")
			} else {
				body.WriteString("**Khoj:** ")
			}
			body.WriteString(text)
			body.WriteString("\n\n")
		}

		if err := writeMarkdown(path, fm, body.String()); err != nil {
			fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", filename, err)
			continue
		}
		convCount++

		for _, msg := range conv.ChatLog.Chat {
			for _, ctx := range msg.Context {
				if ctx.Compiled == "" || len(ctx.Compiled) < 50 {
					continue
				}

				key := ctx.Compiled
				if len(key) > 200 {
					key = key[:200]
				}
				if seenContexts[key] {
					continue
				}
				seenContexts[key] = true

				subject := ctx.File
				if subject == "" {
					subject = extractSubject(ctx.Compiled)
				}

				factSlug := slugify(subject)
				if factSlug == "" {
					factSlug = fmt.Sprintf("khoj-ctx-%d", factCount+1)
				}
				factPath := filepath.Join(factsDir, fmt.Sprintf("khoj-%s.md", factSlug))

				if _, err := os.Stat(factPath); err == nil {
					continue
				}

				factFM := map[string]interface{}{
					"subject":    subject,
					"predicate":  "context-from-khoj",
					"source":     "khoj-import",
					"importance": "medium",
					"confidence": 0.7,
					"created":    created.Format(time.RFC3339),
				}

				content := ctx.Compiled
				if len(content) > 2000 {
					content = content[:2000] + "\n\n[truncated]"
				}

				if err := writeMarkdown(factPath, factFM, content); err != nil {
					continue
				}
				factCount++
			}
		}
	}

	fmt.Printf("Imported: %d conversations → brain/khoj/\n", convCount)
	fmt.Printf("Extracted: %d context facts → memory/facts/\n", factCount)
	fmt.Printf("Skipped: %d conversations (already imported or empty)\n", len(conversations)-convCount)
}

func readKhojExport(path string) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(path), ".zip") {
		return readFromZip(path)
	}
	return os.ReadFile(path)
}

func readFromZip(path string) ([]byte, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.Contains(f.Name, "conversations") && strings.HasSuffix(f.Name, ".json") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s in zip: %w", f.Name, err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}

	return nil, fmt.Errorf("no conversations.json found in ZIP")
}

func parseKhojTime(s string) time.Time {
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Now()
}

func messageText(msg interface{}) string {
	switch v := msg.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func collectTags(conv khojConversation) []string {
	seen := make(map[string]bool)
	var tags []string
	for _, msg := range conv.ChatLog.Chat {
		if msg.Intent != nil && msg.Intent.Type != "" {
			t := strings.ToLower(msg.Intent.Type)
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	return tags
}

func extractSubject(text string) string {
	line := strings.SplitN(text, "\n", 2)[0]
	line = strings.TrimSpace(line)
	if len(line) > 80 {
		line = line[:77] + "..."
	}
	if line == "" {
		return "khoj-context"
	}
	return line
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func writeMarkdown(path string, fm map[string]interface{}, body string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "---")
	for k, v := range fm {
		switch val := v.(type) {
		case []string:
			fmt.Fprintf(f, "%s:\n", k)
			for _, item := range val {
				fmt.Fprintf(f, "  - %s\n", item)
			}
		default:
			fmt.Fprintf(f, "%s: %v\n", k, val)
		}
	}
	fmt.Fprintln(f, "---")
	fmt.Fprintln(f)
	fmt.Fprint(f, body)

	return nil
}

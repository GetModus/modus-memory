package main

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

// khojConversation represents a single exported Khoj conversation.
type khojConversation struct {
	Title    string `json:"title"`
	Agent    string `json:"agent"`
	Created  string `json:"created_at"` // "2024-01-15 10:30:45"
	Updated  string `json:"updated_at"`
	ChatLog  struct {
		Chat []khojMessage `json:"chat"`
	} `json:"conversation_log"`
	FileFilters []string `json:"file_filters"`
}

// khojMessage represents a single message in a Khoj conversation.
type khojMessage struct {
	By      string      `json:"by"`      // "user" or "khoj"
	Message interface{} `json:"message"` // string or list of dicts
	Created string      `json:"created"` // ISO timestamp
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

// runImportKhoj converts a Khoj export (ZIP or JSON) into vault markdown files.
//
// Each conversation becomes a document in brain/khoj/.
// Unique context references become memory facts in memory/facts/.
// User messages are extracted as potential facts when they contain assertions.
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

	// Ensure output directories
	convDir := filepath.Join(vaultDir, "brain", "khoj")
	factsDir := filepath.Join(vaultDir, "memory", "facts")
	os.MkdirAll(convDir, 0755)
	os.MkdirAll(factsDir, 0755)

	convCount := 0
	factCount := 0
	seenContexts := make(map[string]bool)

	for _, conv := range conversations {
		if len(conv.ChatLog.Chat) == 0 {
			continue
		}

		// Convert conversation to markdown
		slug := slugify(conv.Title)
		if slug == "" {
			slug = fmt.Sprintf("conversation-%d", convCount+1)
		}

		created := parseKhojTime(conv.Created)
		filename := fmt.Sprintf("%s-%s.md", created.Format("2006-01-02"), slug)
		path := filepath.Join(convDir, filename)

		// Skip if already imported
		if _, err := os.Stat(path); err == nil {
			continue
		}

		// Build frontmatter
		fm := map[string]interface{}{
			"title":   conv.Title,
			"source":  "khoj",
			"kind":    "conversation",
			"created": created.Format(time.RFC3339),
			"agent":   conv.Agent,
		}

		// Collect tags from intents
		tags := collectTags(conv)
		if len(tags) > 0 {
			fm["tags"] = tags
		}

		// Build body
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

		// Extract unique context references as memory facts
		for _, msg := range conv.ChatLog.Chat {
			for _, ctx := range msg.Context {
				if ctx.Compiled == "" || len(ctx.Compiled) < 50 {
					continue
				}

				// Deduplicate by first 200 chars
				key := ctx.Compiled
				if len(key) > 200 {
					key = key[:200]
				}
				if seenContexts[key] {
					continue
				}
				seenContexts[key] = true

				// Create a memory fact from the context
				subject := ctx.File
				if subject == "" {
					subject = extractSubject(ctx.Compiled)
				}

				factSlug := slugify(subject)
				if factSlug == "" {
					factSlug = fmt.Sprintf("khoj-ctx-%d", factCount+1)
				}
				factPath := filepath.Join(factsDir, fmt.Sprintf("khoj-%s.md", factSlug))

				// Skip if already exists
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

// readKhojExport reads a Khoj export from either a ZIP or raw JSON file.
func readKhojExport(path string) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(path), ".zip") {
		return readFromZip(path)
	}
	return os.ReadFile(path)
}

// readFromZip extracts conversations.json from a Khoj ZIP export.
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

// parseKhojTime parses Khoj's timestamp format.
func parseKhojTime(s string) time.Time {
	// Try Khoj format: "2024-01-15 10:30:45"
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	// Try ISO format
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// Try date only
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Now()
}

// messageText extracts text from a Khoj message (handles string or list-of-dicts).
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

// collectTags extracts unique intent types from a conversation.
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

// extractSubject pulls a subject from context text (first sentence or line).
func extractSubject(text string) string {
	// Take first line, cap at 80 chars
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

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// writeMarkdown writes a markdown file with YAML frontmatter.
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

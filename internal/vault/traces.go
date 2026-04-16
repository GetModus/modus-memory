package vault

import (
	"fmt"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// StoreTrace writes a task execution trace to memory/traces/.
// Traces are provenance-rich procedural memory — they record what was tried,
// what worked, and what failed, with full attribution.
func (v *Vault) StoreTrace(task, outcome string, steps []string, durationSec float64, toolsUsed []string, createdBy, model string) (string, error) {
	if task == "" || outcome == "" {
		return "", fmt.Errorf("task and outcome are required")
	}
	if createdBy == "" {
		createdBy = "unknown"
	}

	now := time.Now()
	slug := slugify(task)
	if len(slug) > 60 {
		slug = slug[:60]
	}
	datePrefix := now.Format("2006-01-02")
	fileName := fmt.Sprintf("%s-%s", datePrefix, slug)

	relPath := fmt.Sprintf("memory/traces/%s.md", fileName)
	path := v.Path("memory", "traces", fileName+".md")

	// Handle duplicates
	for i := 2; fileExists(path); i++ {
		fileName2 := fmt.Sprintf("%s-%d", fileName, i)
		relPath = fmt.Sprintf("memory/traces/%s.md", fileName2)
		path = v.Path("memory", "traces", fileName2+".md")
	}

	fm := map[string]interface{}{
		"task":         task,
		"outcome":      outcome,
		"duration_sec": durationSec,
		"tools_used":   toolsUsed,
		"created":      now.Format(time.RFC3339),
		"created_by":   createdBy,
		"model":        model,
		"confidence":   0.8,
		"importance":   "medium",
		"memory_type":  "procedural",
	}

	// Build body from steps
	var body strings.Builder
	if len(steps) > 0 {
		body.WriteString("## Steps\n\n")
		for i, step := range steps {
			body.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
	}
	if len(toolsUsed) > 0 {
		body.WriteString("\n## Tools Used\n\n")
		for _, tool := range toolsUsed {
			body.WriteString(fmt.Sprintf("- %s\n", tool))
		}
	}

	if err := markdown.Write(path, fm, body.String()); err != nil {
		return "", err
	}
	return relPath, nil
}

// SearchTraces searches memory/traces/ for traces matching the query.
// Uses the index if available, falls back to substring matching.
func (v *Vault) SearchTraces(query string, limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 5
	}

	dir := v.Path("memory", "traces")
	docs, err := markdown.ScanDir(dir)
	if err != nil {
		return nil, nil
	}

	if query == "" {
		if len(docs) > limit {
			docs = docs[:limit]
		}
		return docs, nil
	}

	queryLower := strings.ToLower(query)
	var matches []*markdown.Document
	for _, doc := range docs {
		if len(matches) >= limit {
			break
		}
		task := strings.ToLower(doc.Get("task"))
		body := strings.ToLower(doc.Body)
		if strings.Contains(task, queryLower) || strings.Contains(body, queryLower) {
			matches = append(matches, doc)
		}
	}
	return matches, nil
}

// ListTraces returns recent traces, most recent first.
func (v *Vault) ListTraces(limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 10
	}
	dir := v.Path("memory", "traces")
	docs, err := markdown.ScanDir(dir)
	if err != nil {
		return nil, nil
	}
	if len(docs) > limit {
		docs = docs[:limit]
	}
	return docs, nil
}

// FormatTraceHints formats matching traces as search-time annotations
// for inclusion in memory_search results.
func (v *Vault) FormatTraceHints(query string) string {
	traces, err := v.SearchTraces(query, 3)
	if err != nil || len(traces) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("**Related traces:**\n")
	for _, doc := range traces {
		task := doc.Get("task")
		outcome := doc.Get("outcome")
		duration := doc.Get("duration_sec")
		sb.WriteString(fmt.Sprintf("- **%s** → %s", task, outcome))
		if duration != "" {
			sb.WriteString(fmt.Sprintf(" (%ss)", duration))
		}
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
	return sb.String()
}

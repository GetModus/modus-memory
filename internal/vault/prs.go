package vault

import (
	"fmt"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// OpenPR creates a new PR (evolution proposal) in vault/atlas/prs/.
// Returns the relative path of the created PR file.
func (v *Vault) OpenPR(title, openedBy, targetType, targetID string,
	reasoning string, confidence float64, linkedBeliefIDs []string) (string, error) {

	slug := slugify(title)
	if len(slug) > 80 {
		slug = slug[:80]
	}

	relPath := fmt.Sprintf("atlas/prs/%s.md", slug)

	// Ensure unique filename
	for i := 2; fileExists(v.Path(relPath)); i++ {
		relPath = fmt.Sprintf("atlas/prs/%s-%d.md", slug, i)
	}

	now := time.Now().Format(time.RFC3339)

	fm := map[string]interface{}{
		"title":              title,
		"opened_by":          openedBy,
		"status":             "open",
		"opened_at":          now,
		"target_type":        targetType,
		"target_id":          targetID,
		"confidence":         confidence,
		"linked_belief_ids":  linkedBeliefIDs,
	}

	body := fmt.Sprintf("# %s\n\n## Reasoning\n\n%s\n", title, reasoning)

	if err := v.Write(relPath, fm, body); err != nil {
		return "", err
	}

	return relPath, nil
}

// MergePR marks a PR as merged and reinforces all linked beliefs.
// Only the operator should call this — MODUS never closes its own PRs.
func (v *Vault) MergePR(relPath, closedBy string) error {
	doc, err := v.Read(relPath)
	if err != nil {
		return fmt.Errorf("read PR: %w", err)
	}

	status := doc.Get("status")
	if status != "open" {
		return fmt.Errorf("PR is %s, not open — cannot merge", status)
	}

	doc.Set("status", "merged")
	doc.Set("closed_at", time.Now().Format(time.RFC3339))
	doc.Set("closed_by", closedBy)

	if err := doc.Save(); err != nil {
		return fmt.Errorf("save PR: %w", err)
	}

	// Reinforce linked beliefs
	linkedIDs := doc.Get("linked_belief_ids")
	if linkedIDs != "" {
		for _, id := range parseStringList(linkedIDs) {
			v.ReinforceBelief(id, "pr-merge")
		}
	}

	return nil
}

// RejectPR marks a PR as rejected and weakens all linked beliefs.
func (v *Vault) RejectPR(relPath, closedBy, reason string) error {
	doc, err := v.Read(relPath)
	if err != nil {
		return fmt.Errorf("read PR: %w", err)
	}

	status := doc.Get("status")
	if status != "open" {
		return fmt.Errorf("PR is %s, not open — cannot reject", status)
	}

	doc.Set("status", "rejected")
	doc.Set("closed_at", time.Now().Format(time.RFC3339))
	doc.Set("closed_by", closedBy)
	doc.Set("rejection_reason", reason)

	if err := doc.Save(); err != nil {
		return fmt.Errorf("save PR: %w", err)
	}

	// Weaken linked beliefs
	linkedIDs := doc.Get("linked_belief_ids")
	if linkedIDs != "" {
		for _, id := range parseStringList(linkedIDs) {
			v.WeakenBelief(id)
		}
	}

	return nil
}

// ListPRs returns all PRs, optionally filtered by status.
func (v *Vault) ListPRs(status string) ([]*markdown.Document, error) {
	if status != "" {
		return v.List("atlas/prs", Filter{Field: "status", Value: status})
	}
	return v.List("atlas/prs")
}

// GetPR reads a single PR by slug or path.
func (v *Vault) GetPR(slug string) (*markdown.Document, error) {
	relPath := fmt.Sprintf("atlas/prs/%s.md", slug)
	return v.Read(relPath)
}

// parseStringList handles both Go string slices and YAML-formatted string lists.
func parseStringList(s string) []string {
	// Handle YAML list format: [a, b, c]
	s = trimBrackets(s)
	if s == "" {
		return nil
	}
	parts := splitAndTrim(s, ",")
	return parts
}

func trimBrackets(s string) string {
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-1]
	}
	return s
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimQuotes(trimSpace(part))
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s, sep string) []string {
	var parts []string
	for {
		i := indexOf(s, sep)
		if i < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:i])
		s = s[i+len(sep):]
	}
	return parts
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func trimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

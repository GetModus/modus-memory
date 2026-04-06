// Package learnings implements MODUS-level operational memory.
//
// Unlike per-agent lessons (vault/experience/lessons/), learnings live at
// vault/brain/learnings/ and are accessible to every cartridge — Claude,
// Gemma, Kimi, or whatever model is active. The console learns; cartridges
// execute from what the console knows.
//
// Schema: each learning is a .md file with YAML frontmatter.
// Domains partition learnings so callers only load what's relevant.
package learnings

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// Domain categorizes a learning for efficient filtering.
const (
	DomainSearch       = "search"
	DomainTriage       = "triage"
	DomainCode         = "code"
	DomainIngestion    = "ingestion"
	DomainArchitecture = "architecture"
	DomainOperations   = "operations"
	DomainGeneral      = "general"
)

// Type describes how the learning was acquired.
const (
	TypeMistake    = "mistake"    // something went wrong, don't repeat it
	TypePattern    = "pattern"    // something worked, repeat it
	TypeDecision   = "decision"   // an architectural or product choice and why
	TypeCorrection = "correction" // the General corrected us
)

// Severity controls injection priority.
const (
	SeverityCritical = "critical" // always inject
	SeverityHigh     = "high"    // inject when domain matches
	SeverityMedium   = "medium"  // inject when relevant
	SeverityLow      = "low"     // inject only when asked
)

// Learning is a single operational memory entry.
type Learning struct {
	Slug        string  // filename without .md
	Domain      string  // DomainSearch, DomainTriage, etc.
	Type        string  // TypeMistake, TypePattern, etc.
	Severity    string  // SeverityCritical, SeverityHigh, etc.
	Source      string  // "agent-reflection", "general-correction", "dialectic"
	LearnedFrom string  // which model or "general"
	Created     string  // 2006-01-02
	Reinforced  int     // how many times this was confirmed
	Tags        string  // comma-separated
	Summary     string  // one-line summary (from frontmatter)
	Body        string  // full content: what happened, learning, apply when
	Confidence  float64 // 0.0-1.0
}

// Dir returns the vault path for learnings.
func Dir(vaultDir string) string {
	return filepath.Join(vaultDir, "brain", "learnings")
}

// Record writes a new learning to the vault.
func Record(vaultDir string, l Learning) error {
	dir := Dir(vaultDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create learnings dir: %w", err)
	}

	if l.Slug == "" {
		l.Slug = slugify(l.Summary)
	}
	if l.Created == "" {
		l.Created = time.Now().Format("2006-01-02")
	}
	if l.Confidence == 0 {
		l.Confidence = 0.7
	}

	path := filepath.Join(dir, l.Slug+".md")

	// Don't overwrite — reinforce instead
	if _, err := os.Stat(path); err == nil {
		return Reinforce(vaultDir, l.Slug)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("summary: %q\n", l.Summary))
	sb.WriteString(fmt.Sprintf("domain: %s\n", l.Domain))
	sb.WriteString(fmt.Sprintf("type: %s\n", l.Type))
	sb.WriteString(fmt.Sprintf("severity: %s\n", l.Severity))
	sb.WriteString(fmt.Sprintf("source: %s\n", l.Source))
	sb.WriteString(fmt.Sprintf("learned_from: %s\n", l.LearnedFrom))
	sb.WriteString(fmt.Sprintf("confidence: %.1f\n", l.Confidence))
	sb.WriteString(fmt.Sprintf("reinforced: %d\n", l.Reinforced))
	sb.WriteString(fmt.Sprintf("created: %s\n", l.Created))
	if l.Tags != "" {
		sb.WriteString(fmt.Sprintf("tags: [%s]\n", l.Tags))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(l.Body)
	sb.WriteString("\n")

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// LoadAll reads every learning from the vault.
func LoadAll(vaultDir string) []Learning {
	return loadFromDir(Dir(vaultDir))
}

// LoadByDomain reads learnings filtered to a specific domain.
// Also includes "general" domain learnings and all "critical" severity.
func LoadByDomain(vaultDir, domain string, limit int) []Learning {
	all := LoadAll(vaultDir)

	var filtered []Learning
	for _, l := range all {
		if l.Domain == domain || l.Domain == DomainGeneral || l.Severity == SeverityCritical {
			filtered = append(filtered, l)
		}
	}

	// Sort: critical first, then by reinforcement count (most reinforced = most proven)
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Severity == SeverityCritical && filtered[j].Severity != SeverityCritical {
			return true
		}
		if filtered[i].Severity != SeverityCritical && filtered[j].Severity == SeverityCritical {
			return false
		}
		return filtered[i].Reinforced > filtered[j].Reinforced
	})

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

// LoadForPrompt returns learnings formatted for injection into any model's
// system prompt. This is the key function — every cartridge calls this.
func LoadForPrompt(vaultDir, domain string, limit int) string {
	learnings := LoadByDomain(vaultDir, domain, limit)
	return FormatForPrompt(learnings)
}

// FormatForPrompt renders learnings as a system prompt section.
func FormatForPrompt(learnings []Learning) string {
	if len(learnings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## MODUS Operational Learnings\n\n")
	sb.WriteString("These are verified patterns from past operations. Follow them.\n\n")

	for _, l := range learnings {
		icon := "—"
		switch l.Type {
		case TypeMistake:
			icon = "AVOID"
		case TypePattern:
			icon = "DO"
		case TypeDecision:
			icon = "DECIDED"
		case TypeCorrection:
			icon = "CORRECTED"
		}

		severity := ""
		if l.Severity == SeverityCritical {
			severity = " [CRITICAL]"
		}

		reinforced := ""
		if l.Reinforced > 0 {
			reinforced = fmt.Sprintf(" (confirmed %dx)", l.Reinforced)
		}

		sb.WriteString(fmt.Sprintf("- **[%s]%s** %s%s\n", icon, severity, l.Summary, reinforced))

		// Include body for critical items or when there are few learnings
		if l.Severity == SeverityCritical || len(learnings) <= 5 {
			// Extract just the "Learning" section if structured
			learning := extractSection(l.Body, "Learning")
			if learning == "" {
				learning = extractSection(l.Body, "What we learned")
			}
			if learning == "" && len(l.Body) < 200 {
				learning = l.Body
			}
			if learning != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(learning)))
			}
		}
	}

	return sb.String()
}

// safeLearningPath resolves a slug to a path within the learnings directory,
// rejecting traversal attempts.
func safeLearningPath(vaultDir, slug string) (string, error) {
	dir := Dir(vaultDir)
	abs, err := filepath.Abs(filepath.Join(dir, slug+".md"))
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid learnings root: %w", err)
	}
	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) && abs != root {
		return "", fmt.Errorf("path traversal denied: %s", slug)
	}
	return abs, nil
}

// Reinforce bumps the reinforcement count for a learning.
func Reinforce(vaultDir, slug string) error {
	path, err := safeLearningPath(vaultDir, slug)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", slug, err)
	}

	content := string(data)

	// Find and increment the reinforced count
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "reinforced:") {
			var count int
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "reinforced:")), "%d", &count)
			lines[i] = fmt.Sprintf("reinforced: %d", count+1)
			break
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// Deprecate marks a learning as outdated by setting confidence to 0.
func Deprecate(vaultDir, slug string) error {
	path, err := safeLearningPath(vaultDir, slug)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", slug, err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "confidence:") {
			lines[i] = "confidence: 0.0"
			break
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// Search finds learnings matching a query string (simple substring match).
// For librarian-ranked search, use SearchWithLibrarian.
func Search(vaultDir, query string, limit int) []Learning {
	all := LoadAll(vaultDir)
	query = strings.ToLower(query)

	var matches []Learning
	for _, l := range all {
		searchable := strings.ToLower(l.Summary + " " + l.Body + " " + l.Tags + " " + l.Domain)
		if strings.Contains(searchable, query) {
			matches = append(matches, l)
		}
	}

	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// PromoteFromLesson converts an agent-level lesson to a MODUS-level learning.
// Called when a lesson proves universal (reinforced across multiple roles).
func PromoteFromLesson(vaultDir string, summary, detail, sourceRole string, confidence float64) error {
	return Record(vaultDir, Learning{
		Summary:     summary,
		Domain:      DomainGeneral,
		Type:        TypePattern,
		Severity:    SeverityMedium,
		Source:      "agent-reflection",
		LearnedFrom: sourceRole,
		Confidence:  confidence,
		Body: fmt.Sprintf("## What happened\n\nPromoted from agent lesson (role: %s).\n\n## Learning\n\n%s\n\n## Apply when\n\nAny agent encounters a similar situation.\n",
			sourceRole, detail),
	})
}

// RecordCorrection records a correction from the General.
// These are always high severity — the General corrected us directly.
func RecordCorrection(vaultDir, summary, detail string) error {
	return Record(vaultDir, Learning{
		Summary:     summary,
		Domain:      DomainGeneral,
		Type:        TypeCorrection,
		Severity:    SeverityHigh,
		Source:      "general-correction",
		LearnedFrom: "general",
		Confidence:  0.95,
		Body: fmt.Sprintf("## What happened\n\nThe General corrected our behavior.\n\n## Learning\n\n%s\n\n## Apply when\n\nAlways.\n",
			detail),
	})
}

// --- internal helpers ---

func loadFromDir(dir string) []Learning {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var learnings []Learning
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		doc, err := markdown.Parse(path)
		if err != nil {
			continue
		}

		l := Learning{
			Slug:        strings.TrimSuffix(entry.Name(), ".md"),
			Summary:     doc.Get("summary"),
			Domain:      doc.Get("domain"),
			Type:        doc.Get("type"),
			Severity:    doc.Get("severity"),
			Source:      doc.Get("source"),
			LearnedFrom: doc.Get("learned_from"),
			Created:     doc.Get("created"),
			Tags:        doc.Get("tags"),
			Body:        doc.Body,
		}

		fmt.Sscanf(doc.Get("confidence"), "%f", &l.Confidence)
		fmt.Sscanf(doc.Get("reinforced"), "%d", &l.Reinforced)

		// Skip deprecated learnings
		if l.Confidence <= 0.0 {
			continue
		}

		learnings = append(learnings, l)
	}

	return learnings
}

func extractSection(body, heading string) string {
	lower := strings.ToLower(body)
	target := strings.ToLower("## " + heading)
	idx := strings.Index(lower, target)
	if idx < 0 {
		return ""
	}

	start := idx + len(target)
	// Find the next heading or end of string
	nextHeading := strings.Index(lower[start:], "\n## ")
	if nextHeading >= 0 {
		return strings.TrimSpace(body[start : start+nextHeading])
	}
	return strings.TrimSpace(body[start:])
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == ' ' {
			return r
		}
		return -1
	}, s)
	s = strings.Join(strings.Fields(s), "-")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

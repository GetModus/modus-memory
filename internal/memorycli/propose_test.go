package memorycli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

func setupVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"memory/facts", "memory/episodes", "memory/maintenance", "atlas/entities"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	return dir
}

func writeDoc(t *testing.T, path string, fm map[string]interface{}, body string) {
	t.Helper()
	if err := markdown.Write(path, fm, body); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestProposeHotWritesArtifact(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "operator-priority.md"), map[string]interface{}{
		"subject":            "Operator priority",
		"predicate":          "is",
		"memory_temperature": "warm",
		"importance":         "high",
		"created_at":         time.Now().Format(time.RFC3339),
	}, "Keep the General's thread clear.")

	result, err := ProposeHot(vaultDir, []string{
		"--fact-path", "memory/facts/operator-priority.md",
		"--temperature", "hot",
		"--reason", "This fact belongs in automatic admission.",
	})
	if err != nil {
		t.Fatalf("ProposeHot failed: %v", err)
	}
	if result.ArtifactPath == "" {
		t.Fatal("expected artifact path")
	}
	if !strings.Contains(result.Message, result.ArtifactPath) {
		t.Fatalf("message = %q, want artifact path included", result.Message)
	}

	doc, err := markdown.Parse(filepath.Join(vaultDir, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if doc.Get("type") != "candidate_hot_memory_transition" {
		t.Fatalf("type = %q, want candidate_hot_memory_transition", doc.Get("type"))
	}
	if doc.Get("fact_path") != "memory/facts/operator-priority.md" {
		t.Fatalf("fact_path = %q, want memory/facts/operator-priority.md", doc.Get("fact_path"))
	}
}

func TestProposeStructuralWritesArtifact(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "modus-core.md"), map[string]interface{}{
		"subject":    "MODUS",
		"predicate":  "operates",
		"created_at": time.Now().Format(time.RFC3339),
	}, "MODUS operates as a governed memory system.")
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "modus-related.md"), map[string]interface{}{
		"subject":    "MODUS",
		"predicate":  "requires",
		"created_at": time.Now().Format(time.RFC3339),
	}, "MODUS requires durable structural memory.")
	writeDoc(t, filepath.Join(vaultDir, "memory", "episodes", "evt-structural.md"), map[string]interface{}{
		"type":       "memory_episode",
		"event_id":   "evt-structural",
		"created_at": time.Now().Format(time.RFC3339),
	}, "Structural linking was discussed.")
	writeDoc(t, filepath.Join(vaultDir, "atlas", "entities", "modus.md"), map[string]interface{}{
		"name": "MODUS",
	}, "# MODUS\n")

	result, err := ProposeStructural(vaultDir, []string{
		"--fact-path", "memory/facts/modus-core.md",
		"--related-fact", "memory/facts/modus-related.md",
		"--related-episode", "memory/episodes/evt-structural.md",
		"--related-entity", "MODUS",
		"--related-mission", "Memory Sovereignty",
		"--reason", "Link this fact to its adjacent evidence.",
	})
	if err != nil {
		t.Fatalf("ProposeStructural failed: %v", err)
	}
	if result.ArtifactPath == "" {
		t.Fatal("expected artifact path")
	}

	doc, err := markdown.Parse(filepath.Join(vaultDir, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if doc.Get("type") != "candidate_structural_link_transition" {
		t.Fatalf("type = %q, want candidate_structural_link_transition", doc.Get("type"))
	}
	if refs := doc.Frontmatter["proposed_related_fact_paths"]; refs == nil {
		t.Fatal("expected proposed_related_fact_paths")
	}
	if refs := doc.Frontmatter["proposed_related_episode_paths"]; refs == nil {
		t.Fatal("expected proposed_related_episode_paths")
	}
	if refs := doc.Frontmatter["proposed_related_entity_refs"]; refs == nil {
		t.Fatal("expected proposed_related_entity_refs")
	}
	if refs := doc.Frontmatter["proposed_related_mission_refs"]; refs == nil {
		t.Fatal("expected proposed_related_mission_refs")
	}
}

func TestProposeStructuralRequiresLinks(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "modus-core.md"), map[string]interface{}{
		"subject":    "MODUS",
		"predicate":  "operates",
		"created_at": time.Now().Format(time.RFC3339),
	}, "MODUS operates as a governed memory system.")

	_, err := ProposeStructural(vaultDir, []string{
		"--fact-path", "memory/facts/modus-core.md",
		"--reason", "Link this fact to its adjacent evidence.",
	})
	if err == nil {
		t.Fatal("expected error when no links are provided")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestProposeTemporalWritesArtifact(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "old-lane.md"), map[string]interface{}{
		"subject":         "Scout lane",
		"predicate":       "uses",
		"value":           "qwen",
		"temporal_status": "active",
		"created_at":      time.Now().Format(time.RFC3339),
	}, "Scout used the old lane.")
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "new-lane.md"), map[string]interface{}{
		"subject":    "Scout lane",
		"predicate":  "uses",
		"value":      "gemini",
		"created_at": time.Now().Format(time.RFC3339),
	}, "Scout uses the new lane.")

	result, err := ProposeTemporal(vaultDir, []string{
		"--fact-path", "memory/facts/old-lane.md",
		"--status", "superseded",
		"--superseded-by", "memory/facts/new-lane.md",
		"--reason", "The newer staffing fact supersedes this lane.",
	})
	if err != nil {
		t.Fatalf("ProposeTemporal failed: %v", err)
	}
	if result.ArtifactPath == "" {
		t.Fatal("expected artifact path")
	}

	doc, err := markdown.Parse(filepath.Join(vaultDir, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if doc.Get("type") != "candidate_fact_temporal_transition" {
		t.Fatalf("type = %q, want candidate_fact_temporal_transition", doc.Get("type"))
	}
	if doc.Get("proposed_temporal_status") != "superseded" {
		t.Fatalf("proposed_temporal_status = %q, want superseded", doc.Get("proposed_temporal_status"))
	}
	if doc.Get("superseded_by_path") != "memory/facts/new-lane.md" {
		t.Fatalf("superseded_by_path = %q, want memory/facts/new-lane.md", doc.Get("superseded_by_path"))
	}
}

func TestProposeTemporalRequiresSupersededByForSuperseded(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "old-lane.md"), map[string]interface{}{
		"subject":         "Scout lane",
		"predicate":       "uses",
		"value":           "qwen",
		"temporal_status": "active",
		"created_at":      time.Now().Format(time.RFC3339),
	}, "Scout used the old lane.")

	_, err := ProposeTemporal(vaultDir, []string{
		"--fact-path", "memory/facts/old-lane.md",
		"--status", "superseded",
		"--reason", "The newer staffing fact supersedes this lane.",
	})
	if err == nil {
		t.Fatal("expected error when superseded-by is omitted")
	}
	if !strings.Contains(err.Error(), "superseded_by_path is required") {
		t.Fatalf("err = %v, want superseded_by_path error", err)
	}
}

func TestProposeElderWritesArtifact(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "facts", "founding-covenant.md"), map[string]interface{}{
		"subject":    "Founding covenant",
		"predicate":  "guides",
		"value":      "memory sovereignty",
		"importance": "critical",
		"created_at": time.Now().Add(-40 * 24 * time.Hour).Format(time.RFC3339),
	}, "The founding covenant guides the memory system.")

	result, err := ProposeElder(vaultDir, []string{
		"--fact-path", "memory/facts/founding-covenant.md",
		"--protection-class", "elder",
		"--reason", "This fact should remain protected against ordinary recency bias.",
	})
	if err != nil {
		t.Fatalf("ProposeElder failed: %v", err)
	}
	if result.ArtifactPath == "" {
		t.Fatal("expected artifact path")
	}

	doc, err := markdown.Parse(filepath.Join(vaultDir, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if doc.Get("type") != "candidate_elder_memory_transition" {
		t.Fatalf("type = %q, want candidate_elder_memory_transition", doc.Get("type"))
	}
	if doc.Get("proposed_protection_class") != "elder" {
		t.Fatalf("proposed_protection_class = %q, want elder", doc.Get("proposed_protection_class"))
	}
}

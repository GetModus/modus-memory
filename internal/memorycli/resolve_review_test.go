package memorycli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/GetModus/modus-memory/internal/markdown"
)

func TestResolveReviewRejectsFilteredArtifacts(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "structural-backfill.md"), map[string]interface{}{
		"type":         "candidate_structural_link_transition",
		"status":       "pending",
		"created":      "2026-04-15T18:12:50Z",
		"fact_path":    "memory/facts/modus-core.md",
		"subject":      "MODUS",
		"predicate":    "operates",
		"review_class": "backfill",
	}, "Pending structural backfill.")
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "manual-hot.md"), map[string]interface{}{
		"type":         "candidate_hot_memory_transition",
		"status":       "pending",
		"created":      "2026-04-15T18:13:50Z",
		"fact_path":    "memory/facts/mission.md",
		"subject":      "Mission",
		"predicate":    "operator_context",
		"review_class": "manual",
	}, "Pending manual hot review.")

	result, err := ResolveReview(vaultDir, []string{
		"--status", "pending",
		"--type", "candidate_structural_link_transition",
		"--review-class", "backfill",
		"--set-status", "rejected",
		"--reason", "Reject stale backfill review debt before pretesting.",
	})
	if err != nil {
		t.Fatalf("ResolveReview failed: %v", err)
	}
	if result.Summary.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Summary.Updated)
	}

	rejected, err := markdown.Parse(filepath.Join(vaultDir, "memory", "maintenance", "structural-backfill.md"))
	if err != nil {
		t.Fatalf("parse rejected artifact: %v", err)
	}
	if rejected.Get("status") != "rejected" {
		t.Fatalf("status = %q, want rejected", rejected.Get("status"))
	}
	if rejected.Get("review_resolution_reason") == "" {
		t.Fatal("expected review_resolution_reason on rejected artifact")
	}

	untouched, err := markdown.Parse(filepath.Join(vaultDir, "memory", "maintenance", "manual-hot.md"))
	if err != nil {
		t.Fatalf("parse untouched artifact: %v", err)
	}
	if untouched.Get("status") != "pending" {
		t.Fatalf("untouched status = %q, want pending", untouched.Get("status"))
	}
}

func TestResolveReviewJSONIncludesSummary(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "manual-hot.md"), map[string]interface{}{
		"type":         "candidate_hot_memory_transition",
		"status":       "pending",
		"created":      "2026-04-15T18:13:50Z",
		"fact_path":    "memory/facts/mission.md",
		"subject":      "Mission",
		"predicate":    "operator_context",
		"review_class": "manual",
	}, "Pending manual hot review.")

	result, err := ResolveReview(vaultDir, []string{
		"--status", "pending",
		"--fact-path", "memory/facts/mission.md",
		"--set-status", "approved",
		"--reason", "Commission initial hot tier.",
		"--json",
	})
	if err != nil {
		t.Fatalf("ResolveReview failed: %v", err)
	}
	if !result.JSON {
		t.Fatal("expected json mode")
	}
	if result.Summary.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Summary.Updated)
	}

	data, err := MarshalResolveReviewJSON(result.Summary)
	if err != nil {
		t.Fatalf("MarshalResolveReviewJSON failed: %v", err)
	}
	var decoded ResolveReviewSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	if decoded.SetStatus != "approved" {
		t.Fatalf("set_status = %q, want approved", decoded.SetStatus)
	}
	if decoded.Updated != 1 {
		t.Fatalf("decoded updated = %d, want 1", decoded.Updated)
	}
}

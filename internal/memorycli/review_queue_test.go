package memorycli

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewQueueDefaultsToPending(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "pending-hot.md"), map[string]interface{}{
		"type":         "candidate_hot_memory_transition",
		"status":       "pending",
		"created":      "2026-04-15T13:00:00Z",
		"fact_path":    "memory/facts/operator-priority.md",
		"subject":      "Operator priority",
		"predicate":    "is",
		"review_class": "manual",
	}, "Pending hot review.")
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "approved-elder.md"), map[string]interface{}{
		"type":         "candidate_elder_memory_transition",
		"status":       "approved",
		"created":      "2026-04-15T13:05:00Z",
		"fact_path":    "memory/facts/founding-covenant.md",
		"subject":      "Founding covenant",
		"predicate":    "guides",
		"review_class": "manual",
	}, "Approved elder review.")

	result, err := ReviewQueue(vaultDir, nil)
	if err != nil {
		t.Fatalf("ReviewQueue failed: %v", err)
	}
	if result.JSON {
		t.Fatal("expected text mode by default")
	}
	if result.Summary.Total != 1 {
		t.Fatalf("total = %d, want 1", result.Summary.Total)
	}
	if len(result.Summary.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Summary.Items))
	}
	if result.Summary.Items[0].Type != "candidate_hot_memory_transition" {
		t.Fatalf("type = %q, want candidate_hot_memory_transition", result.Summary.Items[0].Type)
	}
}

func TestReviewQueueAllJSONIncludesCounts(t *testing.T) {
	vaultDir := setupVault(t)
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "pending-temporal.md"), map[string]interface{}{
		"type":         "candidate_fact_temporal_transition",
		"status":       "pending",
		"created":      time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"fact_path":    "memory/facts/old-lane.md",
		"subject":      "Scout lane",
		"predicate":    "uses",
		"review_class": "manual",
	}, "Pending temporal review.")
	writeDoc(t, filepath.Join(vaultDir, "memory", "maintenance", "approved-elder.md"), map[string]interface{}{
		"type":         "candidate_elder_memory_transition",
		"status":       "approved",
		"created":      time.Now().UTC().Format(time.RFC3339),
		"fact_path":    "memory/facts/founding-covenant.md",
		"subject":      "Founding covenant",
		"predicate":    "guides",
		"review_class": "manual",
	}, "Approved elder review.")

	result, err := ReviewQueue(vaultDir, []string{"--status", "all", "--json"})
	if err != nil {
		t.Fatalf("ReviewQueue failed: %v", err)
	}
	if !result.JSON {
		t.Fatal("expected json mode")
	}
	if result.Summary.Total != 2 {
		t.Fatalf("total = %d, want 2", result.Summary.Total)
	}
	if got := result.Summary.CountsByStatus["pending"]; got != 1 {
		t.Fatalf("pending count = %d, want 1", got)
	}
	if got := result.Summary.CountsByStatus["approved"]; got != 1 {
		t.Fatalf("approved count = %d, want 1", got)
	}
	data, err := MarshalReviewQueueJSON(result.Summary)
	if err != nil {
		t.Fatalf("MarshalReviewQueueJSON failed: %v", err)
	}
	var decoded ReviewQueueSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	if decoded.Total != 2 {
		t.Fatalf("decoded total = %d, want 2", decoded.Total)
	}
}

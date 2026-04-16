package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/markdown"
)

func TestStoreCorrection(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "corrections"), 0755)
	v := New(dir, nil)

	relPath, err := v.StoreCorrection("javascript", "typescript", "User prefers TS for all new projects", "user")
	if err != nil {
		t.Fatalf("StoreCorrection failed: %v", err)
	}
	if relPath == "" {
		t.Fatal("expected non-empty relPath")
	}

	// Verify file exists
	fullPath := filepath.Join(dir, relPath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatalf("correction file not created at %s", fullPath)
	}

	// Read it back and verify frontmatter
	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read correction failed: %v", err)
	}
	if doc.Get("original") != "javascript" {
		t.Errorf("original = %q, want %q", doc.Get("original"), "javascript")
	}
	if doc.Get("corrected") != "typescript" {
		t.Errorf("corrected = %q, want %q", doc.Get("corrected"), "typescript")
	}
	if doc.Get("created_by") != "user" {
		t.Errorf("created_by = %q, want %q", doc.Get("created_by"), "user")
	}
	if doc.Get("scope") != "search" {
		t.Errorf("scope = %q, want %q", doc.Get("scope"), "search")
	}

	raw, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read correction file: %v", err)
	}
	if !strings.Contains(string(raw), "producing_signature:") {
		t.Fatal("correction file missing producing_signature")
	}

	ledgerData, err := os.ReadFile(filepath.Join(dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(ledgerData)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "correction_creation" {
		t.Fatalf("action_class = %v, want correction_creation", rec["action_class"])
	}
}

func TestFindCorrections(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "corrections"), 0755)
	v := New(dir, nil)

	// Store a correction
	_, err := v.StoreCorrection("react hooks", "react server components", "New project direction", "user")
	if err != nil {
		t.Fatalf("StoreCorrection failed: %v", err)
	}

	// Query that matches the original
	matches, err := v.FindCorrections("react hooks")
	if err != nil {
		t.Fatalf("FindCorrections failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	// Query that doesn't match
	matches, err = v.FindCorrections("python flask")
	if err != nil {
		t.Fatalf("FindCorrections failed: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFormatCorrectionHints(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "corrections"), 0755)
	v := New(dir, nil)

	// No corrections — should return empty
	hints := v.FormatCorrectionHints("anything")
	if hints != "" {
		t.Errorf("expected empty hints for no corrections, got %q", hints)
	}

	// Add a correction
	_, _ = v.StoreCorrection("javascript", "typescript", "User always means TS", "user")

	// Now should get hints
	hints = v.FormatCorrectionHints("javascript")
	if hints == "" {
		t.Error("expected non-empty hints after storing correction")
	}

	doc, err := v.Read("memory/corrections/javascript.md")
	if err != nil {
		t.Fatalf("Read correction failed: %v", err)
	}
	if doc.GetFloat("apply_count") != 1 {
		t.Fatalf("apply_count = %v, want 1", doc.GetFloat("apply_count"))
	}
	if doc.Get("last_applied") == "" {
		t.Fatal("last_applied should be set after hint emission")
	}

	ledgerData, err := os.ReadFile(filepath.Join(dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(ledgerData)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "correction_application" {
		t.Fatalf("action_class = %v, want correction_application", rec["action_class"])
	}
}

func TestStoreCorrectionDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "corrections"), 0755)
	v := New(dir, nil)

	path1, _ := v.StoreCorrection("test item", "test correction 1", "", "user")
	path2, _ := v.StoreCorrection("test item", "test correction 2", "", "user")

	if path1 == path2 {
		t.Errorf("duplicate paths: %s and %s should differ", path1, path2)
	}
}

func TestStoreCorrectionFlagsAffectedDocsAndWritesPropagationArtifact(t *testing.T) {
	dir := t.TempDir()
	for _, subdir := range []string{
		filepath.Join(dir, "memory", "corrections"),
		filepath.Join(dir, "memory", "maintenance"),
	} {
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", subdir, err)
		}
	}
	v := New(dir, nil)

	factPath, err := v.StoreFact("Frontend stack", "uses", "javascript", 0.9, "high")
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}
	recall, err := v.RecallFacts(RecallRequest{
		Query:              "javascript",
		Limit:              3,
		Harness:            "test",
		Adapter:            "test_adapter",
		Mode:               "manual_search",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "corrections_test",
		StaffingContext:    "unit_test",
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	artifactPath := "memory/maintenance/bootstrap-javascript.md"
	if err := markdown.Write(filepath.Join(dir, artifactPath), map[string]interface{}{
		"type":        "candidate_bootstrap_fact",
		"status":      "pending",
		"subject":     "Frontend stack",
		"predicate":   "uses",
		"value":       "javascript",
		"source_refs": []string{factPath},
	}, "Frontend stack uses javascript."); err != nil {
		t.Fatalf("write maintenance artifact: %v", err)
	}

	correctionPath, err := v.StoreCorrection("javascript", "typescript", "User prefers TS", "user")
	if err != nil {
		t.Fatalf("StoreCorrection: %v", err)
	}

	factDoc, err := v.Read(factPath)
	if err != nil {
		t.Fatalf("Read fact: %v", err)
	}
	if factDoc.Get("correction_review_status") != "pending" {
		t.Fatalf("fact correction_review_status = %q, want pending", factDoc.Get("correction_review_status"))
	}
	if factDoc.Get("stale_due_to_correction") != "true" {
		t.Fatalf("fact stale_due_to_correction = %q, want true", factDoc.Get("stale_due_to_correction"))
	}
	if refs := strings.Join(stringSliceFrontmatter(factDoc.Frontmatter["correction_refs"]), " "); !strings.Contains(refs, correctionPath) {
		t.Fatalf("fact correction_refs missing %q: %v", correctionPath, factDoc.Frontmatter["correction_refs"])
	}

	recallDoc, err := v.Read(recall.ReceiptPath)
	if err != nil {
		t.Fatalf("Read recall receipt: %v", err)
	}
	if recallDoc.Get("correction_review_status") != "pending" {
		t.Fatalf("recall correction_review_status = %q, want pending", recallDoc.Get("correction_review_status"))
	}

	maintenanceDoc, err := v.Read(artifactPath)
	if err != nil {
		t.Fatalf("Read maintenance artifact: %v", err)
	}
	if maintenanceDoc.Get("correction_review_status") != "pending" {
		t.Fatalf("artifact correction_review_status = %q, want pending", maintenanceDoc.Get("correction_review_status"))
	}

	docs, err := markdown.ScanDir(filepath.Join(dir, "memory", "maintenance"))
	if err != nil {
		t.Fatalf("scan maintenance: %v", err)
	}
	foundImpact := false
	for _, doc := range docs {
		if doc.Get("type") != "candidate_correction_propagation" {
			continue
		}
		foundImpact = true
		if doc.Get("correction_path") != correctionPath {
			t.Fatalf("correction_path = %q, want %q", doc.Get("correction_path"), correctionPath)
		}
		if got := int(doc.GetFloat("affected_total")); got < 3 {
			t.Fatalf("affected_total = %d, want at least 3", got)
		}
	}
	if !foundImpact {
		t.Fatal("expected correction propagation artifact")
	}
}

func TestSearchFactsHighlightsCorrectionReviewPending(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "memory", "corrections"), 0o755); err != nil {
		t.Fatalf("mkdir corrections: %v", err)
	}
	v := New(dir, nil)

	if _, err := v.StoreFact("Frontend stack", "uses", "javascript", 0.9, "high"); err != nil {
		t.Fatalf("StoreFact: %v", err)
	}
	if _, err := v.StoreCorrection("javascript", "typescript", "User prefers TS", "user"); err != nil {
		t.Fatalf("StoreCorrection: %v", err)
	}

	lines, err := v.SearchFacts("javascript", 3)
	if err != nil {
		t.Fatalf("SearchFacts: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected at least one search result")
	}
	if !strings.Contains(lines[0], "correction review pending") {
		t.Fatalf("expected correction review warning in result: %s", lines[0])
	}
}

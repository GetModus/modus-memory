package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

func TestAnalyzeFSRS(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "facts"), 0755)
	v := New(dir, nil)

	// Create some synthetic facts
	facts := []struct {
		name       string
		importance string
		stability  float64
		confidence float64
		accessCnt  int
	}{
		{"fact-a", "high", 1200.0, 0.995, 1}, // over-retained
		{"fact-b", "high", 1500.0, 0.998, 0}, // over-retained
		{"fact-c", "high", 1100.0, 0.993, 2}, // over-retained
		{"fact-d", "high", 50.0, 0.5, 5},     // normal
		{"fact-e", "high", 80.0, 0.7, 3},     // normal
		{"fact-f", "medium", 30.0, 0.08, 0},  // over-forgotten (floor=0.1)
		{"fact-g", "medium", 25.0, 0.05, 0},  // over-forgotten
		{"fact-h", "medium", 60.0, 0.6, 2},   // normal
		{"fact-i", "medium", 45.0, 0.4, 1},   // normal
		{"fact-j", "low", 10.0, 0.3, 3},      // normal
	}

	for _, f := range facts {
		fm := map[string]interface{}{
			"type":         "fact",
			"importance":   f.importance,
			"stability":    f.stability,
			"confidence":   f.confidence,
			"access_count": f.accessCnt,
			"created":      time.Now().Format(time.RFC3339),
		}
		path := filepath.Join(dir, "memory", "facts", f.name+".md")
		if err := markdown.Write(path, fm, "Test fact: "+f.name); err != nil {
			t.Fatalf("failed to write %s: %v", f.name, err)
		}
	}

	report, err := v.AnalyzeFSRS()
	if err != nil {
		t.Fatalf("AnalyzeFSRS failed: %v", err)
	}

	if report.TotalFacts != 10 {
		t.Errorf("TotalFacts = %d, want 10", report.TotalFacts)
	}

	// High tier: 5 facts, 3 over-retained
	high := report.ByImportance["high"]
	if high.Count != 5 {
		t.Errorf("high.Count = %d, want 5", high.Count)
	}
	if high.OverRetained != 3 {
		t.Errorf("high.OverRetained = %d, want 3", high.OverRetained)
	}

	// Medium tier: 4 facts, 2 over-forgotten
	med := report.ByImportance["medium"]
	if med.Count != 4 {
		t.Errorf("medium.Count = %d, want 4", med.Count)
	}
	if med.OverForgotten != 2 {
		t.Errorf("medium.OverForgotten = %d, want 2", med.OverForgotten)
	}

	// High should have a proposal (60% over-retained > 20% threshold)
	if _, ok := report.Proposals["high"]; !ok {
		t.Error("expected proposal for 'high' tier")
	}

	// Medium should have a proposal (50% over-forgotten > 20% threshold)
	if _, ok := report.Proposals["medium"]; !ok {
		t.Error("expected proposal for 'medium' tier")
	}
}

func TestFormatTuneReport(t *testing.T) {
	report := &FSRSTuneReport{
		TotalFacts:  50,
		GeneratedAt: time.Now(),
		ByImportance: map[string]*importanceBucket{
			"critical": {Count: 5},
			"high":     {Count: 20, OverRetained: 5, AvgStability: 300.0, AvgConfidence: 0.85},
			"medium":   {Count: 20, AvgStability: 45.0, AvgConfidence: 0.60},
			"low":      {Count: 5, AvgStability: 10.0, AvgConfidence: 0.30},
		},
		Proposals: map[string]fsrsProposal{
			"high": {CurrentStability: 180.0, ProposedStability: 153.0, Reason: "25% over-retained"},
		},
	}

	output := FormatTuneReport(report)
	if !strings.Contains(output, "FSRS Tuning Analysis") {
		t.Error("missing report header")
	}
	if !strings.Contains(output, "| high |") {
		t.Error("missing high tier row")
	}
	if !strings.Contains(output, "153.0") {
		t.Error("missing proposal value")
	}
	if !strings.Contains(output, "Proposals") {
		t.Error("missing proposals section")
	}
}

func TestSaveTuneReport(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "fsrs-tuning"), 0755)
	v := New(dir, nil)

	report := &FSRSTuneReport{
		TotalFacts:  10,
		GeneratedAt: time.Now(),
		ByImportance: map[string]*importanceBucket{
			"critical": {},
			"high":     {Count: 5},
			"medium":   {Count: 3},
			"low":      {Count: 2},
		},
		Proposals: map[string]fsrsProposal{
			"high": {CurrentStability: 180, ProposedStability: 153, Reason: "test"},
		},
	}

	relPath, err := v.SaveTuneReport(report)
	if err != nil {
		t.Fatalf("SaveTuneReport failed: %v", err)
	}
	if relPath == "" {
		t.Fatal("expected non-empty relPath")
	}

	// Verify file exists
	fullPath := filepath.Join(dir, relPath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatalf("report file not created at %s", fullPath)
	}

	// Read back and verify frontmatter
	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read report failed: %v", err)
	}
	if doc.Get("type") != "fsrs-tuning-report" {
		t.Errorf("type = %q, want fsrs-tuning-report", doc.Get("type"))
	}
	if doc.Get("status") != "proposed" {
		t.Errorf("status = %q, want proposed", doc.Get("status"))
	}
}

func TestApplyTuneReport(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory"), 0755)
	v := New(dir, nil)

	// Save original values
	origHigh := fsrsConfigs["high"].InitialStability
	origMed := fsrsConfigs["medium"].InitialStability

	report := &FSRSTuneReport{
		GeneratedAt: time.Now(),
		Proposals: map[string]fsrsProposal{
			"high": {CurrentStability: origHigh, ProposedStability: 153.0, Reason: "test"},
		},
	}

	err := v.ApplyTuneReport(report)
	if err != nil {
		t.Fatalf("ApplyTuneReport failed: %v", err)
	}

	// Check in-memory config was updated
	if fsrsConfigs["high"].InitialStability != 153.0 {
		t.Errorf("high InitialStability = %.1f, want 153.0", fsrsConfigs["high"].InitialStability)
	}

	// Check config file was written
	configPath := filepath.Join(dir, "memory", "fsrs-config.md")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("fsrs-config.md not created")
	}

	// Restore original values
	cfg := fsrsConfigs["high"]
	cfg.InitialStability = origHigh
	fsrsConfigs["high"] = cfg

	// Verify medium was not changed
	if fsrsConfigs["medium"].InitialStability != origMed {
		t.Errorf("medium should not have changed")
	}

	data, err := os.ReadFile(filepath.Join(dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "policy_tuning_apply" {
		t.Fatalf("action_class = %v, want policy_tuning_apply", rec["action_class"])
	}
}

func TestLoadTunedFSRS(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory"), 0755)
	v := New(dir, nil)

	// Save original
	origHigh := fsrsConfigs["high"].InitialStability

	// Write a config file
	fm := map[string]interface{}{
		"type":    "fsrs-config",
		"version": "test",
	}
	body := `# Active FSRS Configuration

## high
- InitialStability: 200.0
- InitialDifficulty: 0.3
- Floor: 0.3

## medium
- InitialStability: 80.0
- InitialDifficulty: 0.5
- Floor: 0.1

## low
- InitialStability: 20.0
- InitialDifficulty: 0.7
- Floor: 0.05
`
	configPath := filepath.Join(dir, "memory", "fsrs-config.md")
	if err := markdown.Write(configPath, fm, body); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	err := v.LoadTunedFSRS()
	if err != nil {
		t.Fatalf("LoadTunedFSRS failed: %v", err)
	}

	if fsrsConfigs["high"].InitialStability != 200.0 {
		t.Errorf("high InitialStability = %.1f, want 200.0", fsrsConfigs["high"].InitialStability)
	}
	if fsrsConfigs["medium"].InitialStability != 80.0 {
		t.Errorf("medium InitialStability = %.1f, want 80.0", fsrsConfigs["medium"].InitialStability)
	}

	// Restore
	cfg := fsrsConfigs["high"]
	cfg.InitialStability = origHigh
	fsrsConfigs["high"] = cfg
	cfg = fsrsConfigs["medium"]
	cfg.InitialStability = 60
	fsrsConfigs["medium"] = cfg
}

func TestApplyTuneReportNoProposals(t *testing.T) {
	dir := t.TempDir()
	v := New(dir, nil)

	report := &FSRSTuneReport{
		Proposals: map[string]fsrsProposal{},
	}

	err := v.ApplyTuneReport(report)
	if err == nil {
		t.Error("expected error for empty proposals")
	}
}

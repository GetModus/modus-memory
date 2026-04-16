package trainer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/vault"
)

func setupVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{
		"memory/facts", "memory/corrections", "memory/traces",
		"memory/maintenance", "memory/training-runs", "brain",
	} {
		os.MkdirAll(filepath.Join(dir, sub), 0755)
	}
	return vault.New(dir, nil), dir
}

func writeMD(t *testing.T, path string, fm map[string]interface{}, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := markdown.Write(path, fm, body); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// --- Signal mining tests ---

func TestMineCorrections(t *testing.T) {
	v, dir := setupVault(t)

	writeMD(t, filepath.Join(dir, "memory", "corrections", "js-to-ts.md"),
		map[string]interface{}{
			"original":  "javascript",
			"corrected": "typescript",
			"context":   "user prefers TS",
		}, "")

	pairs, err := MineCorrections(v)
	if err != nil {
		t.Fatalf("MineCorrections: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	if pairs[0].Source != "correction" {
		t.Errorf("source = %q, want correction", pairs[0].Source)
	}
	// Should have 3 messages: system, user, assistant
	if len(pairs[0].Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(pairs[0].Messages))
	}
}

func TestMineTraces(t *testing.T) {
	v, dir := setupVault(t)

	// Successful trace
	writeMD(t, filepath.Join(dir, "memory", "traces", "2026-04-07-deploy.md"),
		map[string]interface{}{
			"task": "deploy frontend", "outcome": "success",
		}, "Built the Docker image, pushed to registry, deployed via kubectl.")

	// Failed trace — should be excluded
	writeMD(t, filepath.Join(dir, "memory", "traces", "2026-04-07-debug.md"),
		map[string]interface{}{
			"task": "fix auth bug", "outcome": "failure",
		}, "Could not reproduce.")

	pairs, err := MineTraces(v)
	if err != nil {
		t.Fatalf("MineTraces: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (only success), got %d", len(pairs))
	}
	if pairs[0].Source != "trace" {
		t.Errorf("source = %q, want trace", pairs[0].Source)
	}
}

func TestMineFacts(t *testing.T) {
	v, dir := setupVault(t)

	// High quality fact
	writeMD(t, filepath.Join(dir, "memory", "facts", "go-version.md"),
		map[string]interface{}{
			"subject": "Go", "predicate": "latest version",
			"confidence": 0.95, "access_count": 5,
		}, "1.24")

	// Low quality fact — should be excluded
	writeMD(t, filepath.Join(dir, "memory", "facts", "unknown.md"),
		map[string]interface{}{
			"subject": "Unknown", "predicate": "something",
			"confidence": 0.3, "access_count": 0,
		}, "not sure")

	pairs, err := MineFacts(v)
	if err != nil {
		t.Fatalf("MineFacts: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (only high-quality), got %d", len(pairs))
	}
}

func TestMineMaintenanceOutcomes(t *testing.T) {
	v, dir := setupVault(t)

	// Approved outcome
	writeMD(t, filepath.Join(dir, "memory", "maintenance", "approved-merge.md"),
		map[string]interface{}{
			"type": "candidate_merge", "status": "approved",
			"stronger_subj": "Go",
		}, "Merged Go facts successfully.")

	// Pending outcome — should be excluded
	writeMD(t, filepath.Join(dir, "memory", "maintenance", "pending-merge.md"),
		map[string]interface{}{
			"type": "candidate_merge", "status": "pending",
			"stronger_subj": "Python",
		}, "Proposed merge.")

	pairs, err := MineMaintenanceOutcomes(v)
	if err != nil {
		t.Fatalf("MineMaintenanceOutcomes: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (only approved), got %d", len(pairs))
	}
}

func TestGenerateBatch(t *testing.T) {
	v, dir := setupVault(t)

	// Add some data
	writeMD(t, filepath.Join(dir, "memory", "corrections", "fix.md"),
		map[string]interface{}{"original": "react", "corrected": "react server components"},
		"")
	writeMD(t, filepath.Join(dir, "memory", "traces", "2026-04-07-build.md"),
		map[string]interface{}{"task": "build app", "outcome": "success"},
		"Ran build command, deployed.")

	batch, err := GenerateBatch(v)
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(batch.SFT) < 2 {
		t.Errorf("expected at least 2 SFT pairs, got %d", len(batch.SFT))
	}
}

// --- Writer tests ---

func TestWriteBatchAndRead(t *testing.T) {
	dir := t.TempDir()

	batch := &TrainingBatch{
		SFT: []SFTPair{
			{
				Messages: []ChatMessage{
					{Role: "system", Content: "test"},
					{Role: "user", Content: "hello"},
					{Role: "assistant", Content: "world"},
				},
				Source: "test",
			},
		},
		DPO: []DPOTriple{
			{Prompt: "q", Chosen: "good", Rejected: "bad", Source: "test"},
		},
	}

	if err := WriteBatch(batch, dir); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	// Verify SFT file
	sftPath := filepath.Join(dir, "sft_librarian.jsonl")
	data, err := os.ReadFile(sftPath)
	if err != nil {
		t.Fatalf("read SFT: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Error("SFT file should contain user message")
	}

	// Verify it's valid JSONL
	var row map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &row); err != nil {
		t.Fatalf("SFT not valid JSON: %v", err)
	}
	if _, ok := row["messages"]; !ok {
		t.Error("SFT row should have 'messages' key")
	}

	// Verify DPO file
	dpoPath := filepath.Join(dir, "dpo_librarian.jsonl")
	dpoData, err := os.ReadFile(dpoPath)
	if err != nil {
		t.Fatalf("read DPO: %v", err)
	}
	if !strings.Contains(string(dpoData), "good") {
		t.Error("DPO file should contain chosen response")
	}
}

func TestConsolidate(t *testing.T) {
	dir := t.TempDir()

	// Write some SFT pairs
	batch := &TrainingBatch{
		SFT: make([]SFTPair, 20),
	}
	for i := range batch.SFT {
		batch.SFT[i] = SFTPair{
			Messages: []ChatMessage{
				{Role: "system", Content: "sys"},
				{Role: "user", Content: strings.Repeat("q", i+1)}, // unique
				{Role: "assistant", Content: "a"},
			},
		}
	}

	if err := WriteBatch(batch, dir); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	train, valid, err := Consolidate(dir, dir)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	if train+valid != 20 {
		t.Errorf("expected 20 total, got %d train + %d valid = %d", train, valid, train+valid)
	}
	if valid == 0 {
		t.Error("expected at least 1 validation example")
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(dir, "mlx", "train.jsonl")); os.IsNotExist(err) {
		t.Error("mlx/train.jsonl not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "mlx", "valid.jsonl")); os.IsNotExist(err) {
		t.Error("mlx/valid.jsonl not created")
	}
}

func TestMinPairsReached(t *testing.T) {
	dir := t.TempDir()

	// Not enough
	batch := &TrainingBatch{
		SFT: make([]SFTPair, 10),
	}
	for i := range batch.SFT {
		batch.SFT[i] = SFTPair{
			Messages: []ChatMessage{{Role: "user", Content: strings.Repeat("x", i+1)}},
		}
	}
	WriteBatch(batch, dir)
	if MinPairsReached(dir) {
		t.Error("should not reach minimum with 10 pairs")
	}

	// Enough
	dir2 := t.TempDir()
	batch2 := &TrainingBatch{
		SFT: make([]SFTPair, 60),
	}
	for i := range batch2.SFT {
		batch2.SFT[i] = SFTPair{
			Messages: []ChatMessage{{Role: "user", Content: strings.Repeat("y", i+1)}},
		}
	}
	WriteBatch(batch2, dir2)
	if !MinPairsReached(dir2) {
		t.Error("should reach minimum with 60 pairs")
	}
}

// --- Training log tests ---

func TestLogAndListTrainRuns(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	run := &TrainRun{
		Timestamp:   "2026-04-07-120000",
		ModelBase:   "/path/to/model.gguf",
		AdapterPath: "/path/to/adapters",
		SFTPairs:    100,
		DPOPairs:    20,
		TrainLoss:   0.45,
		ValLoss:     0.52,
		DurationSec: 300.0,
	}

	if err := LogTrainRun(dir, run); err != nil {
		t.Fatalf("LogTrainRun: %v", err)
	}

	runs, err := ListTrainRuns(dir)
	if err != nil {
		t.Fatalf("ListTrainRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].SFTPairs != 100 {
		t.Errorf("SFTPairs = %d, want 100", runs[0].SFTPairs)
	}
}

func TestBestUnpromotedRun(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	// Two runs — one promoted, one not
	LogTrainRun(dir, &TrainRun{
		Timestamp: "2026-04-07-100000", ValLoss: 0.6, Promoted: true,
	})
	LogTrainRun(dir, &TrainRun{
		Timestamp: "2026-04-07-110000", ValLoss: 0.4, Promoted: false,
	})

	best := BestUnpromotedRun(dir)
	if best == nil {
		t.Fatal("expected a best unpromoted run")
	}
	if best.ValLoss != 0.4 {
		t.Errorf("ValLoss = %.1f, want 0.4", best.ValLoss)
	}
}

func TestParseLastLoss(t *testing.T) {
	output := `
Step 50: Train Loss: 1.234
Step 100: Train Loss: 0.890
Step 100: Val Loss: 1.100
Step 200: Train Loss: 0.456
Step 200: Val Loss: 0.678
`
	trainLoss := parseLastLoss(output, "Train")
	if trainLoss != 0.456 {
		t.Errorf("trainLoss = %.3f, want 0.456", trainLoss)
	}
	valLoss := parseLastLoss(output, "Val")
	if valLoss != 0.678 {
		t.Errorf("valLoss = %.3f, want 0.678", valLoss)
	}
}

func TestCountPairs(t *testing.T) {
	dir := t.TempDir()
	batch := &TrainingBatch{
		SFT: []SFTPair{
			{Messages: []ChatMessage{{Role: "user", Content: "a"}}},
			{Messages: []ChatMessage{{Role: "user", Content: "b"}}},
		},
		DPO: []DPOTriple{
			{Prompt: "p", Chosen: "c", Rejected: "r"},
		},
	}
	WriteBatch(batch, dir)

	sft, dpo := CountPairs(dir)
	if sft != 2 {
		t.Errorf("sft = %d, want 2", sft)
	}
	if dpo != 1 {
		t.Errorf("dpo = %d, want 1", dpo)
	}
}

// --- Promotion gate tests ---

func TestPromotionCheckFirstPromotion(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	// No prior promoted runs — first promotion should pass
	candidate := &TrainRun{Timestamp: "2026-04-07-100000", ValLoss: 0.5}
	pass, reason := PromotionCheck(dir, candidate)
	if !pass {
		t.Errorf("first promotion should pass, got blocked: %s", reason)
	}
	if !strings.Contains(reason, "first promotion") {
		t.Errorf("reason should mention first promotion, got: %s", reason)
	}
}

func TestPromotionCheckImprovement(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	// Log a promoted baseline
	LogTrainRun(dir, &TrainRun{
		Timestamp: "2026-04-07-100000", ValLoss: 0.6, Promoted: true,
		PromotedAt: "2026-04-07T10:00:00Z",
	})

	// Candidate with better val_loss — should pass
	candidate := &TrainRun{Timestamp: "2026-04-07-110000", ValLoss: 0.4}
	pass, reason := PromotionCheck(dir, candidate)
	if !pass {
		t.Errorf("improved candidate should pass, got blocked: %s", reason)
	}
	if !strings.Contains(reason, "improved") {
		t.Errorf("reason should mention improvement, got: %s", reason)
	}
}

func TestPromotionCheckRegression(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	// Log a promoted baseline
	LogTrainRun(dir, &TrainRun{
		Timestamp: "2026-04-07-100000", ValLoss: 0.4, Promoted: true,
		PromotedAt: "2026-04-07T10:00:00Z",
	})

	// Candidate with WORSE val_loss — should be blocked
	candidate := &TrainRun{Timestamp: "2026-04-07-110000", ValLoss: 0.6}
	pass, reason := PromotionCheck(dir, candidate)
	if pass {
		t.Errorf("regressed candidate should be blocked, but passed: %s", reason)
	}
	if !strings.Contains(reason, "blocked") {
		t.Errorf("reason should mention blocked, got: %s", reason)
	}
}

func TestPromotionCheckNoValLoss(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	candidate := &TrainRun{Timestamp: "2026-04-07-100000", ValLoss: 0}
	pass, _ := PromotionCheck(dir, candidate)
	if pass {
		t.Error("candidate with no val_loss should be blocked")
	}
}

func TestPromoteRunBlocksRegression(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	// Baseline
	LogTrainRun(dir, &TrainRun{
		Timestamp: "2026-04-07-100000", ValLoss: 0.3, Promoted: true,
		PromotedAt: "2026-04-07T10:00:00Z",
	})

	// Worse candidate
	candidate := &TrainRun{Timestamp: "2026-04-07-110000", ValLoss: 0.5}
	err := PromoteRun(dir, candidate, "test")
	if err == nil {
		t.Error("PromoteRun should return error for regression")
	}
	if candidate.Promoted {
		t.Error("candidate should not be marked promoted after blocked promotion")
	}
}

// --- Gap 3: MineSearchSignals (DPO) had zero direct test coverage. ---
// Creates a reinforced fact and a decayed fact with the same subject.
// Verifies MineSearchSignals produces a DPO triple with reinforced as chosen.
func TestMineSearchSignals(t *testing.T) {
	v, dir := setupVault(t)

	// Reinforced fact: high confidence, high access, high stability
	writeMD(t, filepath.Join(dir, "memory", "facts", "go-reinforced.md"),
		map[string]interface{}{
			"subject": "Go", "predicate": "designed at",
			"confidence": 0.95, "access_count": 10, "stability": 500,
		}, "Google")

	// Decayed fact: low confidence, zero access — same subject
	writeMD(t, filepath.Join(dir, "memory", "facts", "go-decayed.md"),
		map[string]interface{}{
			"subject": "Go", "predicate": "version",
			"confidence": 0.1, "access_count": 0, "stability": 5,
		}, "1.18")

	triples, err := MineSearchSignals(v)
	if err != nil {
		t.Fatalf("MineSearchSignals: %v", err)
	}
	if len(triples) != 1 {
		t.Fatalf("expected 1 DPO triple, got %d", len(triples))
	}

	triple := triples[0]
	if triple.Chosen != "Google" {
		t.Errorf("chosen = %q, want 'Google' (the reinforced fact)", triple.Chosen)
	}
	if triple.Rejected != "1.18" {
		t.Errorf("rejected = %q, want '1.18' (the decayed fact)", triple.Rejected)
	}
	if triple.Source != "search-signal" {
		t.Errorf("source = %q, want 'search-signal'", triple.Source)
	}
}

// Verifies that MineSearchSignals produces nothing when facts don't share a subject.
func TestMineSearchSignalsNoOverlap(t *testing.T) {
	v, dir := setupVault(t)

	// Reinforced: subject "Go"
	writeMD(t, filepath.Join(dir, "memory", "facts", "go-high.md"),
		map[string]interface{}{
			"subject": "Go", "predicate": "is",
			"confidence": 0.95, "access_count": 5, "stability": 200,
		}, "compiled language")

	// Decayed: different subject "Python"
	writeMD(t, filepath.Join(dir, "memory", "facts", "python-low.md"),
		map[string]interface{}{
			"subject": "Python", "predicate": "version",
			"confidence": 0.1, "access_count": 0, "stability": 1,
		}, "2.7")

	triples, err := MineSearchSignals(v)
	if err != nil {
		t.Fatalf("MineSearchSignals: %v", err)
	}
	if len(triples) != 0 {
		t.Errorf("expected 0 DPO triples (no subject overlap), got %d", len(triples))
	}
}

// --- Gap 4: PromoteRun success path untested. ---
// Verifies that a passing candidate gets Promoted=true, PromotedAt set,
// and PromotionNote contains the gate reason.
func TestPromoteRunSuccess(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "training-runs"), 0755)

	// Log a promoted baseline
	LogTrainRun(dir, &TrainRun{
		Timestamp: "2026-04-07-100000", ValLoss: 0.6, Promoted: true,
		PromotedAt: "2026-04-07T10:00:00Z",
	})

	// Better candidate
	candidate := &TrainRun{Timestamp: "2026-04-07-110000", ValLoss: 0.4}
	err := PromoteRun(dir, candidate, "manual promotion test")
	if err != nil {
		t.Fatalf("PromoteRun should succeed for improved candidate: %v", err)
	}
	if !candidate.Promoted {
		t.Error("candidate.Promoted should be true after successful promotion")
	}
	if candidate.PromotedAt == "" {
		t.Error("candidate.PromotedAt should be set")
	}
	if !strings.Contains(candidate.PromotionNote, "improved") {
		t.Errorf("PromotionNote should mention improvement, got: %s", candidate.PromotionNote)
	}
	if !strings.Contains(candidate.PromotionNote, "manual promotion test") {
		t.Errorf("PromotionNote should contain caller's note, got: %s", candidate.PromotionNote)
	}

	// Verify it was persisted
	runs, _ := ListTrainRuns(dir)
	var found bool
	for _, r := range runs {
		if r.Timestamp == "2026-04-07-110000" && r.Promoted {
			found = true
		}
	}
	if !found {
		t.Error("promoted run should be persisted to disk")
	}
}

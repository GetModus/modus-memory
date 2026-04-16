package memorykit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/ledger"
)

func TestKernelEvaluateWritesReport(t *testing.T) {
	k := testKernel(t)

	result, err := k.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.ReportPath != "state/memory/evaluations/latest.json" {
		t.Fatalf("ReportPath = %q", result.ReportPath)
	}
	if result.MarkdownPath != "state/memory/evaluations/latest.md" {
		t.Fatalf("MarkdownPath = %q", result.MarkdownPath)
	}
	if result.Report.TotalCases != 6 {
		t.Fatalf("TotalCases = %d, want 6", result.Report.TotalCases)
	}
	if result.Report.PassedCases != result.Report.TotalCases {
		t.Fatalf("PassedCases = %d, want %d; cases=%+v", result.Report.PassedCases, result.Report.TotalCases, result.Report.Cases)
	}
	if result.Report.OverallScore < 0.99 {
		t.Fatalf("OverallScore = %.2f, want near-perfect", result.Report.OverallScore)
	}

	for _, id := range []string{
		"interference_recall_precision",
		"elder_retention",
		"replay_promotion_accuracy",
		"hot_tier_stale_detection",
		"secure_state_tamper_detection",
		"secure_state_rollback_detection",
	} {
		caseResult := findEvaluationCase(result.Report.Cases, id)
		if caseResult == nil {
			t.Fatalf("missing case %q", id)
		}
		if !caseResult.Passed {
			t.Fatalf("case %q failed: %+v", id, *caseResult)
		}
	}

	data, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.ReportPath))
	if err != nil {
		t.Fatalf("read report json: %v", err)
	}
	var persisted EvaluationReport
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("parse report json: %v", err)
	}
	if persisted.TotalCases != result.Report.TotalCases {
		t.Fatalf("persisted TotalCases = %d, want %d", persisted.TotalCases, result.Report.TotalCases)
	}
	if persisted.PassedCases != result.Report.PassedCases {
		t.Fatalf("persisted PassedCases = %d, want %d", persisted.PassedCases, result.Report.PassedCases)
	}

	md, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.MarkdownPath))
	if err != nil {
		t.Fatalf("read report markdown: %v", err)
	}
	if !strings.Contains(string(md), "# Memory Evaluation Report") {
		t.Fatalf("expected markdown report heading, got:\n%s", string(md))
	}

	ops, err := os.ReadFile(filepath.Join(k.Vault.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	if !strings.Contains(string(ops), ledger.ActionMemoryEvaluation) {
		t.Fatalf("expected %q in operations ledger", ledger.ActionMemoryEvaluation)
	}
}

func findEvaluationCase(cases []EvaluationCaseResult, id string) *EvaluationCaseResult {
	for i := range cases {
		if cases[i].ID == id {
			return &cases[i]
		}
	}
	return nil
}

package memorykit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/vault"
)

func TestKernelRunTrialsWritesReport(t *testing.T) {
	k := testKernel(t)

	factPath, err := k.StoreFact("Memory System", "current_capabilities", "5-channel hybrid search", 0.95, "high", vault.FactWriteAuthority{
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "trial_test",
		StaffingContext:     "test",
		AuthorityScope:      ledger.ScopeOperatorMemoryStore,
		TargetDomain:        "memory/facts",
		ObservedAt:          "2026-04-15T12:00:00Z",
		ValidFrom:           "2026-04-15T12:00:00Z",
		RelatedFactPaths:    []string{"memory/facts/founding-law.md"},
		RelatedEpisodePaths: []string{"memory/episodes/evt-memory-capability.md"},
		RelatedEntityRefs:   []string{"MODUS Memory"},
		RelatedMissionRefs:  []string{"Memory Sovereignty"},
		AllowApproval:       true,
	})
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}

	casePath := filepath.Join(k.Vault.Dir, "state", "memory", "trials", "cases", "memory-system.md")
	if err := markdown.Write(casePath, map[string]interface{}{
		"type":                        "memory_trial_case",
		"status":                      "active",
		"title":                       "Memory system capabilities",
		"query":                       "memory system current capabilities",
		"limit":                       3,
		"expect_top_path":             factPath,
		"expect_verification_status":  vault.VerificationStatusSourceMissing,
		"expect_top_temporal_status":  "active",
		"expect_linked_fact_paths":    []string{"memory/facts/founding-law.md"},
		"expect_linked_episode_paths": []string{"memory/episodes/evt-memory-capability.md"},
		"expect_linked_entity_refs":   []string{"MODUS Memory"},
		"expect_linked_mission_refs":  []string{"Memory Sovereignty"},
		"verification_mode":           vault.VerificationModeCritical,
	}, "The real memory estate should recall the existing capability fact and admit that it lacks canonical citation."); err != nil {
		t.Fatalf("write trial case: %v", err)
	}

	result, err := k.RunTrials()
	if err != nil {
		t.Fatalf("RunTrials: %v", err)
	}
	if result.ReportPath != "state/memory/trials/latest.json" {
		t.Fatalf("ReportPath = %q", result.ReportPath)
	}
	if result.MarkdownPath != "state/memory/trials/latest.md" {
		t.Fatalf("MarkdownPath = %q", result.MarkdownPath)
	}
	if result.Report.TotalCases != 1 {
		t.Fatalf("TotalCases = %d, want 1", result.Report.TotalCases)
	}
	if result.Report.PassedCases != 1 {
		t.Fatalf("PassedCases = %d, want 1", result.Report.PassedCases)
	}
	if result.Report.OverallScore < 0.99 {
		t.Fatalf("OverallScore = %.2f, want near-perfect", result.Report.OverallScore)
	}
	if len(result.Report.Cases) != 1 {
		t.Fatalf("report cases = %d, want 1", len(result.Report.Cases))
	}
	if result.Report.Cases[0].TopVerificationStatus != vault.VerificationStatusSourceMissing {
		t.Fatalf("TopVerificationStatus = %q, want %q", result.Report.Cases[0].TopVerificationStatus, vault.VerificationStatusSourceMissing)
	}
	if result.Report.Cases[0].TopTemporalStatus != "active" {
		t.Fatalf("TopTemporalStatus = %q, want active", result.Report.Cases[0].TopTemporalStatus)
	}
	if len(result.Report.Cases[0].LinkedEntityRefs) != 1 || result.Report.Cases[0].LinkedEntityRefs[0] != "MODUS Memory" {
		t.Fatalf("LinkedEntityRefs = %#v", result.Report.Cases[0].LinkedEntityRefs)
	}

	data, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.ReportPath))
	if err != nil {
		t.Fatalf("read report json: %v", err)
	}
	var persisted TrialReport
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("parse report json: %v", err)
	}
	if persisted.TotalCases != 1 || persisted.PassedCases != 1 {
		t.Fatalf("persisted summary = %+v", persisted)
	}

	md, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.MarkdownPath))
	if err != nil {
		t.Fatalf("read report markdown: %v", err)
	}
	if !strings.Contains(string(md), "# Memory Trial Report") {
		t.Fatalf("expected markdown report heading, got:\n%s", string(md))
	}

	ops, err := os.ReadFile(filepath.Join(k.Vault.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	if !strings.Contains(string(ops), ledger.ActionMemoryTrialRun) {
		t.Fatalf("expected %q in operations ledger", ledger.ActionMemoryTrialRun)
	}
}

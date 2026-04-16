package memorykit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/vault"
)

func TestKernelRunReadinessWritesReport(t *testing.T) {
	k := testKernel(t)
	if _, err := k.StoreFact("Founding lesson", "requires", "governed memory", 0.95, "critical", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "readiness_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		MemoryTemperature:  "hot",
		RelatedMissionRefs: []string{"Memory Sovereignty"},
		AllowApproval:      true,
	}); err != nil {
		t.Fatalf("StoreFact: %v", err)
	}

	result, err := k.RunReadiness()
	if err != nil {
		t.Fatalf("RunReadiness: %v", err)
	}
	if result.ReportPath != "state/memory/readiness/latest.json" {
		t.Fatalf("ReportPath = %q", result.ReportPath)
	}
	if result.MarkdownPath != "state/memory/readiness/latest.md" {
		t.Fatalf("MarkdownPath = %q", result.MarkdownPath)
	}
	if result.Report.SecureStatePath != "state/memory/latest.json" {
		t.Fatalf("SecureStatePath = %q", result.Report.SecureStatePath)
	}
	if !result.Report.SecureStateVerified {
		t.Fatalf("expected secure-state verification success, got %+v", result.Report)
	}
	if result.Report.Shelves["facts"].Count != 1 {
		t.Fatalf("facts count = %d, want 1", result.Report.Shelves["facts"].Count)
	}
	if result.Report.Shelves["facts"].HotCount != 1 {
		t.Fatalf("hot facts = %d, want 1", result.Report.Shelves["facts"].HotCount)
	}
	if result.Report.Shelves["facts"].StructuredCount != 1 {
		t.Fatalf("structured facts = %d, want 1", result.Report.Shelves["facts"].StructuredCount)
	}
	if result.Report.Shelves["episodes"].Count != 0 {
		t.Fatalf("episodes count = %d, want 0 in test fixture", result.Report.Shelves["episodes"].Count)
	}
	if len(result.Report.Issues) == 0 {
		t.Fatal("expected readiness issues for missing episodes and reports")
	}

	data, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.ReportPath))
	if err != nil {
		t.Fatalf("read readiness json: %v", err)
	}
	var persisted ReadinessReport
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("parse readiness json: %v", err)
	}
	if persisted.Status == "" {
		t.Fatal("expected persisted readiness status")
	}

	md, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.MarkdownPath))
	if err != nil {
		t.Fatalf("read readiness markdown: %v", err)
	}
	if !strings.Contains(string(md), "# Memory Readiness Report") {
		t.Fatalf("expected readiness heading, got:\n%s", string(md))
	}
	if !strings.Contains(string(md), "Hot facts: `1`") {
		t.Fatalf("expected hot fact count in readiness markdown, got:\n%s", string(md))
	}
	if !strings.Contains(string(md), "Structured facts: `1`") {
		t.Fatalf("expected structured fact count in readiness markdown, got:\n%s", string(md))
	}

	if _, err := os.Stat(filepath.Join(k.Vault.Dir, "state", "memory", "latest.json")); err != nil {
		t.Fatalf("expected secure-state manifest, err=%v", err)
	}
}

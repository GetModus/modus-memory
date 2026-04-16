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

func TestKernelAuditPortabilityWritesReport(t *testing.T) {
	k := testKernel(t)
	cacheDir := t.TempDir()

	writeExternal := func(name, body string) {
		path := filepath.Join(cacheDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir external %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write external %s: %v", name, err)
		}
	}

	writeExternal("continuity_session_journal.md", "# Claude continuity\n")
	writeExternal("project_homefront.md", "# HomeFront\n")
	writeExternal("feedback_testing.md", "# Test all apps\n")
	writeExternal("MEMORY.md", "# Root project memory\n")

	if err := markdown.Write(filepath.Join(k.Vault.Dir, "sessions", "journal.md"), map[string]interface{}{
		"title": "Session Journal",
	}, "canonical continuity"); err != nil {
		t.Fatalf("write sessions/journal.md: %v", err)
	}
	if err := markdown.Write(filepath.Join(k.Vault.Dir, "knowledge", "project_homefront.md"), map[string]interface{}{
		"title": "HomeFront",
	}, "sovereign project note"); err != nil {
		t.Fatalf("write project_homefront counterpart: %v", err)
	}
	if _, err := k.StoreFact("Feedback", "requires", "test all apps before completion", 0.95, "high", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "portability_test",
		StaffingContext:    "unit",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		SourceRef:          "feedback_testing.md",
		AllowApproval:      true,
	}); err != nil {
		t.Fatalf("StoreFact with citation: %v", err)
	}

	result, err := k.AuditPortability(cacheDir)
	if err != nil {
		t.Fatalf("AuditPortability: %v", err)
	}
	if result.ReportPath != "state/memory/portability/latest.json" {
		t.Fatalf("ReportPath = %q", result.ReportPath)
	}
	if result.MarkdownPath != "state/memory/portability/latest.md" {
		t.Fatalf("MarkdownPath = %q", result.MarkdownPath)
	}
	if !result.Report.CachePresent {
		t.Fatal("expected cache_present=true")
	}
	if result.Report.TotalFiles != 4 {
		t.Fatalf("TotalFiles = %d, want 4", result.Report.TotalFiles)
	}
	if result.Report.CoveredFiles != 3 {
		t.Fatalf("CoveredFiles = %d, want 3", result.Report.CoveredFiles)
	}
	if result.Report.ExternalOnly != 1 {
		t.Fatalf("ExternalOnly = %d, want 1", result.Report.ExternalOnly)
	}

	continuity := findPortabilityEntry(result.Report.Entries, "continuity_session_journal.md")
	if continuity == nil || continuity.CoverageClass != portabilityCoverageExplicitEquivalent {
		t.Fatalf("continuity entry = %+v, want explicit equivalent", continuity)
	}
	project := findPortabilityEntry(result.Report.Entries, "project_homefront.md")
	if project == nil || project.CoverageClass != portabilityCoverageExactCounterpart {
		t.Fatalf("project entry = %+v, want exact counterpart", project)
	}
	feedback := findPortabilityEntry(result.Report.Entries, "feedback_testing.md")
	if feedback == nil || feedback.CoverageClass != portabilityCoverageCited {
		t.Fatalf("feedback entry = %+v, want cited coverage", feedback)
	}
	memory := findPortabilityEntry(result.Report.Entries, "MEMORY.md")
	if memory == nil || memory.CoverageClass != portabilityCoverageExternalOnly {
		t.Fatalf("MEMORY entry = %+v, want external-only", memory)
	}

	data, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.ReportPath))
	if err != nil {
		t.Fatalf("read portability json: %v", err)
	}
	var persisted PortabilityAuditReport
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("parse portability json: %v", err)
	}
	if persisted.ExternalOnly != 1 {
		t.Fatalf("persisted ExternalOnly = %d, want 1", persisted.ExternalOnly)
	}

	md, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.MarkdownPath))
	if err != nil {
		t.Fatalf("read portability markdown: %v", err)
	}
	if !strings.Contains(string(md), "# Memory Portability Audit") {
		t.Fatalf("expected portability report heading, got:\n%s", string(md))
	}

	ops, err := os.ReadFile(filepath.Join(k.Vault.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	if !strings.Contains(string(ops), ledger.ActionMemoryPortabilityAudit) {
		t.Fatalf("expected %q in operations ledger", ledger.ActionMemoryPortabilityAudit)
	}
}

func TestKernelBuildPortabilityQueueWritesReport(t *testing.T) {
	k := testKernel(t)
	cacheDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(cacheDir, "MEMORY.md"), []byte("# Root project memory\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	result, err := k.BuildPortabilityQueue(cacheDir)
	if err != nil {
		t.Fatalf("BuildPortabilityQueue: %v", err)
	}
	if result.ReportPath != "state/memory/portability/queue.json" {
		t.Fatalf("ReportPath = %q, want queue.json", result.ReportPath)
	}
	if result.MarkdownPath != "state/memory/portability/queue.md" {
		t.Fatalf("MarkdownPath = %q, want queue.md", result.MarkdownPath)
	}
	if result.Report.TotalItems != 1 {
		t.Fatalf("TotalItems = %d, want 1", result.Report.TotalItems)
	}
	if result.Report.PriorityCounts["critical"] != 1 {
		t.Fatalf("critical queue count = %d, want 1", result.Report.PriorityCounts["critical"])
	}
	if len(result.Report.Entries) != 1 {
		t.Fatalf("queue entries = %d, want 1", len(result.Report.Entries))
	}
	entry := result.Report.Entries[0]
	if entry.CachePath != "MEMORY.md" {
		t.Fatalf("queue cache path = %q, want MEMORY.md", entry.CachePath)
	}
	if entry.Priority != "critical" {
		t.Fatalf("queue priority = %q, want critical", entry.Priority)
	}
	if entry.ProposedAction != "supersede_root_carrier_memory" {
		t.Fatalf("queue action = %q", entry.ProposedAction)
	}

	md, err := os.ReadFile(filepath.Join(k.Vault.Dir, result.MarkdownPath))
	if err != nil {
		t.Fatalf("read portability queue markdown: %v", err)
	}
	if !strings.Contains(string(md), "# Memory Portability Migration Queue") {
		t.Fatalf("expected portability queue heading, got:\n%s", string(md))
	}

	ops, err := os.ReadFile(filepath.Join(k.Vault.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	if !strings.Contains(string(ops), ledger.ActionMemoryPortabilityQueue) {
		t.Fatalf("expected %q in operations ledger", ledger.ActionMemoryPortabilityQueue)
	}
}

func TestKernelAuditPortabilityCreditsRootMemorySupersessionNote(t *testing.T) {
	k := testKernel(t)
	cacheDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(cacheDir, "MEMORY.md"), []byte("# Root project memory\n"), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	if err := markdown.Write(filepath.Join(k.Vault.Dir, "state", "memory", "portability", "MEMORY.md"), map[string]interface{}{
		"title": "Carrier MEMORY Supersession Note",
	}, "sovereign supersession note"); err != nil {
		t.Fatalf("write sovereign MEMORY note: %v", err)
	}

	result, err := k.AuditPortability(cacheDir)
	if err != nil {
		t.Fatalf("AuditPortability: %v", err)
	}
	entry := findPortabilityEntry(result.Report.Entries, "MEMORY.md")
	if entry == nil {
		t.Fatal("missing MEMORY.md entry")
	}
	if entry.CoverageClass != portabilityCoverageExplicitEquivalent {
		t.Fatalf("MEMORY coverage = %q, want explicit equivalent", entry.CoverageClass)
	}
	if len(entry.CounterpartPaths) != 1 || entry.CounterpartPaths[0] != "state/memory/portability/MEMORY.md" {
		t.Fatalf("MEMORY counterpart paths = %+v", entry.CounterpartPaths)
	}
}

func TestKernelArchivePortabilityResidueCopiesExternalOnlyFilesIntoVault(t *testing.T) {
	k := testKernel(t)
	cacheDir := t.TempDir()

	srcPath := filepath.Join(cacheDir, "reference_ios_version.md")
	if err := os.WriteFile(srcPath, []byte("# iOS Version\n"), 0o644); err != nil {
		t.Fatalf("write reference_ios_version.md: %v", err)
	}

	result, err := k.ArchivePortabilityResidue(cacheDir)
	if err != nil {
		t.Fatalf("ArchivePortabilityResidue: %v", err)
	}
	if result.Report.ArchivedCount != 1 {
		t.Fatalf("ArchivedCount = %d, want 1", result.Report.ArchivedCount)
	}
	expectedRel := "brain/claude-memory-archive/reference_ios_version.md"
	if len(result.Report.ArchivedPaths) != 1 || result.Report.ArchivedPaths[0] != expectedRel {
		t.Fatalf("ArchivedPaths = %+v, want %s", result.Report.ArchivedPaths, expectedRel)
	}
	archived, err := os.ReadFile(filepath.Join(k.Vault.Dir, expectedRel))
	if err != nil {
		t.Fatalf("read archived file: %v", err)
	}
	if !strings.Contains(string(archived), "carrier_source_ref") {
		t.Fatalf("expected archived file to carry provenance, got:\n%s", string(archived))
	}

	audit, err := k.AuditPortability(cacheDir)
	if err != nil {
		t.Fatalf("AuditPortability after archive: %v", err)
	}
	entry := findPortabilityEntry(audit.Report.Entries, "reference_ios_version.md")
	if entry == nil {
		t.Fatal("missing reference_ios_version.md after archive")
	}
	if entry.CoverageClass != portabilityCoverageExactCounterpart {
		t.Fatalf("coverage after archive = %q, want exact counterpart", entry.CoverageClass)
	}
}

func findPortabilityEntry(entries []PortabilityAuditEntry, cachePath string) *PortabilityAuditEntry {
	for i := range entries {
		if entries[i].CachePath == cachePath {
			return &entries[i]
		}
	}
	return nil
}

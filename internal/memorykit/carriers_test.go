package memorykit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GetModus/modus-memory/internal/vault"
)

func TestAuditCarriersClassifiesReadyMissingAndDormant(t *testing.T) {
	root := t.TempDir()
	vaultDir := filepath.Join(root, "vault")
	if err := os.MkdirAll(vaultDir, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	for _, name := range []string{"modus-codex", "modus-openclaw"} {
		path := filepath.Join(root, "scripts", name)
		if err := os.WriteFile(path, []byte("#!/bin/zsh\n"), 0o755); err != nil {
			t.Fatalf("write wrapper %s: %v", name, err)
		}
	}

	origLookPath := carrierLookPath
	carrierLookPath = func(file string) (string, error) {
		switch file {
		case "codex":
			return "/usr/local/bin/codex", nil
		case "openclaw":
			return "/usr/local/bin/openclaw", nil
		default:
			return "", os.ErrNotExist
		}
	}
	defer func() { carrierLookPath = origLookPath }()

	k := New(vault.New(vaultDir, nil))
	result, err := k.AuditCarriers()
	if err != nil {
		t.Fatalf("AuditCarriers: %v", err)
	}
	if result.Report.ReadyCount != 2 {
		t.Fatalf("ReadyCount = %d, want 2", result.Report.ReadyCount)
	}
	if result.Report.DormantCount != 1 {
		t.Fatalf("DormantCount = %d, want 1", result.Report.DormantCount)
	}
	if result.Report.MissingCount != len(result.Report.Entries)-3 {
		t.Fatalf("MissingCount = %d, want %d", result.Report.MissingCount, len(result.Report.Entries)-3)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, result.ReportPath)); err != nil {
		t.Fatalf("report path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, result.MarkdownPath)); err != nil {
		t.Fatalf("markdown path missing: %v", err)
	}

	var codexEntry, openclawEntry, claudeEntry *CarrierAuditEntry
	for idx := range result.Report.Entries {
		entry := &result.Report.Entries[idx]
		switch entry.Carrier {
		case "codex":
			codexEntry = entry
		case "openclaw":
			openclawEntry = entry
		case "claude":
			claudeEntry = entry
		}
	}
	if codexEntry == nil || openclawEntry == nil || claudeEntry == nil {
		t.Fatal("expected codex, openclaw, and claude entries")
	}
	if codexEntry.Status != "ready" || codexEntry.RecommendedLane != "wrapper_attachment" {
		t.Fatalf("codex entry = %#v", codexEntry)
	}
	if !openclawEntry.RequiresTarget {
		t.Fatal("expected openclaw to require a target")
	}
	if claudeEntry.Status != "dormant" {
		t.Fatalf("claude status = %q, want dormant", claudeEntry.Status)
	}
}

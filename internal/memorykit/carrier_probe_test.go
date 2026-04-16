package memorykit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeCarriersWritesReportAndContinuesOnFailure(t *testing.T) {
	k := testKernel(t)
	seedAttachmentFact(t, k, "General", "flagship", "brass lantern", "hot")

	codexRunner := &stubAttachmentRunner{
		result: attachmentCarrierResult{
			Text:     "nominal",
			ThreadID: "thread-1",
			Model:    "gpt-5.4",
		},
	}
	qwenRunner := &stubAttachmentRunner{err: errors.New("qwen unavailable")}

	origResolver := attachmentRunnerResolver
	attachmentRunnerResolver = func(carrier string) attachmentCarrierRunner {
		switch normalizeAttachmentCarrier(carrier) {
		case "codex":
			return codexRunner
		case "qwen":
			return qwenRunner
		default:
			return nil
		}
	}
	defer func() { attachmentRunnerResolver = origResolver }()

	result, err := k.ProbeCarriers(context.Background(), CarrierProbeOptions{
		Carriers:    []string{"codex", "qwen"},
		Prompt:      "Reply with exactly: nominal.",
		RecallLimit: 3,
	})
	if err != nil {
		t.Fatalf("ProbeCarriers: %v", err)
	}
	if result.Report.SuccessfulCount != 1 || result.Report.FailedCount != 1 {
		t.Fatalf("success/fail = %d/%d, want 1/1", result.Report.SuccessfulCount, result.Report.FailedCount)
	}
	if _, err := os.Stat(filepath.Join(k.Vault.Dir, result.ReportPath)); err != nil {
		t.Fatalf("report path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(k.Vault.Dir, result.MarkdownPath)); err != nil {
		t.Fatalf("markdown path missing: %v", err)
	}
	if codexRunner.gotPrompt == "" {
		t.Fatal("expected codex runner prompt to be populated")
	}
	if qwenRunner.gotPrompt == "" {
		t.Fatal("expected qwen runner prompt to be populated")
	}

	var codexEntry, qwenEntry *CarrierProbeEntry
	for idx := range result.Report.Entries {
		entry := &result.Report.Entries[idx]
		switch entry.Carrier {
		case "codex":
			codexEntry = entry
		case "qwen":
			qwenEntry = entry
		}
	}
	if codexEntry == nil || qwenEntry == nil {
		t.Fatal("expected codex and qwen entries")
	}
	if codexEntry.Status != "passed" || codexEntry.TracePath == "" || codexEntry.RecallReceiptPath == "" {
		t.Fatalf("codex entry = %#v", codexEntry)
	}
	if qwenEntry.Status != "failed" || qwenEntry.Error == "" || qwenEntry.TracePath == "" {
		t.Fatalf("qwen entry = %#v", qwenEntry)
	}
}

func TestNormalizeProbeCarriersDeduplicatesAliases(t *testing.T) {
	got := normalizeProbeCarriers([]string{"codex,codex-app", "openclaw", "openclaw-cli"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (%v)", len(got), got)
	}
	if got[0] != "codex" || got[1] != "openclaw" {
		t.Fatalf("got = %#v, want [codex openclaw]", got)
	}
}

package memorykit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/vault"
)

type stubAttachmentRunner struct {
	gotPrompt string
	gotOpts   attachmentCarrierOptions
	result    attachmentCarrierResult
	err       error
}

func (s *stubAttachmentRunner) Run(_ context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	s.gotPrompt = opts.Prompt
	s.gotOpts = opts
	if s.err != nil {
		return attachmentCarrierResult{}, s.err
	}
	return s.result, nil
}

func seedAttachmentFact(t *testing.T, k *Kernel, subject, predicate, value, temperature string, auth ...vault.FactWriteAuthority) string {
	t.Helper()
	writeAuth := vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "attachment_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		MemoryTemperature:  temperature,
		AllowApproval:      true,
	}
	if len(auth) > 0 {
		writeAuth = auth[0]
		writeAuth.MemoryTemperature = temperature
		if strings.TrimSpace(writeAuth.ProducingOffice) == "" {
			writeAuth.ProducingOffice = "memory_governance"
		}
		if strings.TrimSpace(writeAuth.ProducingSubsystem) == "" {
			writeAuth.ProducingSubsystem = "attachment_test"
		}
		if strings.TrimSpace(writeAuth.StaffingContext) == "" {
			writeAuth.StaffingContext = "operator_test"
		}
		if strings.TrimSpace(writeAuth.AuthorityScope) == "" {
			writeAuth.AuthorityScope = ledger.ScopeOperatorMemoryStore
		}
		if strings.TrimSpace(writeAuth.TargetDomain) == "" {
			writeAuth.TargetDomain = "memory/facts"
		}
		writeAuth.AllowApproval = true
	}
	path, err := k.StoreFact(subject, predicate, value, 0.95, "high", vault.FactWriteAuthority{
		ProducingOffice:       writeAuth.ProducingOffice,
		ProducingSubsystem:    writeAuth.ProducingSubsystem,
		StaffingContext:       writeAuth.StaffingContext,
		AuthorityScope:        writeAuth.AuthorityScope,
		TargetDomain:          writeAuth.TargetDomain,
		Source:                writeAuth.Source,
		SourceRef:             writeAuth.SourceRef,
		SourceRefs:            writeAuth.SourceRefs,
		ProofRef:              writeAuth.ProofRef,
		PromotionStatus:       writeAuth.PromotionStatus,
		MemoryTemperature:     writeAuth.MemoryTemperature,
		AllowApproval:         writeAuth.AllowApproval,
		SourceEventID:         writeAuth.SourceEventID,
		LineageID:             writeAuth.LineageID,
		CueTerms:              writeAuth.CueTerms,
		Mission:               writeAuth.Mission,
		WorkItemID:            writeAuth.WorkItemID,
		Environment:           writeAuth.Environment,
		MemoryProtectionClass: writeAuth.MemoryProtectionClass,
		MemorySecurityClass:   writeAuth.MemorySecurityClass,
		ObservedAt:            writeAuth.ObservedAt,
		ValidFrom:             writeAuth.ValidFrom,
		ValidTo:               writeAuth.ValidTo,
		TemporalStatus:        writeAuth.TemporalStatus,
		SupersedesPaths:       writeAuth.SupersedesPaths,
		RelatedFactPaths:      writeAuth.RelatedFactPaths,
		RelatedEpisodePaths:   writeAuth.RelatedEpisodePaths,
		RelatedEntityRefs:     writeAuth.RelatedEntityRefs,
		RelatedMissionRefs:    writeAuth.RelatedMissionRefs,
	})
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}
	return path
}

func TestRunAttachedCarrierUsesHotRecallAndStoresReceipts(t *testing.T) {
	k := testKernel(t)
	v := k.Vault
	seedAttachmentFact(t, k, "General", "flagship", "brass lantern", "hot", vault.FactWriteAuthority{
		RelatedFactPaths:    []string{"memory/facts/founding-law.md"},
		RelatedEpisodePaths: []string{"memory/episodes/evt-commissioning.md"},
		RelatedEntityRefs:   []string{"General", "MODUS"},
		RelatedMissionRefs:  []string{"Memory Sovereignty"},
	})
	seedAttachmentFact(t, k, "General", "prefers", "warm archive facts stay on demand", "warm")

	runner := &stubAttachmentRunner{
		result: attachmentCarrierResult{
			Text:     "Attached reply.",
			ThreadID: "thread-123",
			Model:    "gpt-5.4",
		},
	}

	result, err := k.runAttachedCarrier(context.Background(), AttachmentRunOptions{
		Carrier:      "codex",
		Prompt:       "What is the flagship codename?",
		Model:        "gpt-5.4",
		RecallLimit:  4,
		StoreEpisode: true,
		Subject:      "Flagship recall",
		WorkItemID:   "work-flagship",
	}, runner)
	if err != nil {
		t.Fatalf("runAttachedCarrier: %v", err)
	}
	if !result.MemoryApplied {
		t.Fatal("expected hot memory to be applied")
	}
	if !strings.Contains(runner.gotPrompt, "brass lantern") {
		t.Fatalf("attached prompt missing hot fact:\n%s", runner.gotPrompt)
	}
	if !strings.Contains(runner.gotPrompt, "linked entities: General, MODUS") {
		t.Fatalf("attached prompt missing linked entities:\n%s", runner.gotPrompt)
	}
	if !strings.Contains(runner.gotPrompt, "linked missions: Memory Sovereignty") {
		t.Fatalf("attached prompt missing linked missions:\n%s", runner.gotPrompt)
	}
	if strings.Contains(runner.gotPrompt, "warm archive facts") {
		t.Fatalf("attached prompt included warm fact:\n%s", runner.gotPrompt)
	}
	if !strings.HasPrefix(result.RecallReceiptPath, "memory/recalls/") {
		t.Fatalf("RecallReceiptPath = %q", result.RecallReceiptPath)
	}
	if !strings.HasPrefix(result.TracePath, "memory/traces/") {
		t.Fatalf("TracePath = %q", result.TracePath)
	}
	if !strings.HasPrefix(result.EpisodePath, "memory/episodes/") {
		t.Fatalf("EpisodePath = %q", result.EpisodePath)
	}

	receiptPath := filepath.Join(v.Dir, filepath.FromSlash(result.RecallReceiptPath))
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("Read recall receipt: %v", err)
	}
	receiptText := string(receiptBytes)
	if !strings.Contains(receiptText, "harness: "+attachmentHarnessName) {
		t.Fatalf("receipt missing harness:\n%s", receiptText)
	}
	if !strings.Contains(receiptText, "adapter: carrier_codex") {
		t.Fatalf("receipt missing adapter:\n%s", receiptText)
	}
	if !strings.Contains(receiptText, "mode: "+attachmentRecallMode) {
		t.Fatalf("receipt missing mode:\n%s", receiptText)
	}
	if !strings.Contains(receiptText, "work_item_id: work-flagship") {
		t.Fatalf("receipt missing work_item_id:\n%s", receiptText)
	}

	episode, err := v.Read(result.EpisodePath)
	if err != nil {
		t.Fatalf("Read episode: %v", err)
	}
	if episode.Get("source_ref") != result.RecallReceiptPath {
		t.Fatalf("source_ref = %q, want %q", episode.Get("source_ref"), result.RecallReceiptPath)
	}
	if episode.Get("work_item_id") != "work-flagship" {
		t.Fatalf("episode work_item_id = %q", episode.Get("work_item_id"))
	}
	if raw, ok := episode.Frontmatter["related_entity_refs"].([]interface{}); !ok || len(raw) != 2 {
		t.Fatalf("related_entity_refs = %#v", episode.Frontmatter["related_entity_refs"])
	}
	if raw, ok := episode.Frontmatter["related_mission_refs"].([]interface{}); !ok || len(raw) != 1 {
		t.Fatalf("related_mission_refs = %#v", episode.Frontmatter["related_mission_refs"])
	}
	if !strings.Contains(episode.Body, "Attached reply.") {
		t.Fatalf("episode body missing carrier output:\n%s", episode.Body)
	}
}

func TestRunAttachedCarrierStoresFailureTraceOnCarrierError(t *testing.T) {
	k := testKernel(t)
	v := k.Vault
	seedAttachmentFact(t, k, "General", "flagship", "brass lantern", "hot")
	runner := &stubAttachmentRunner{err: errors.New("codex unavailable")}

	result, err := k.runAttachedCarrier(context.Background(), AttachmentRunOptions{
		Carrier:      "codex",
		Prompt:       "What is the flagship codename?",
		StoreEpisode: false,
	}, runner)
	if err == nil {
		t.Fatal("expected carrier error")
	}
	if !strings.Contains(err.Error(), "codex unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result.TracePath, "memory/traces/") {
		t.Fatalf("TracePath = %q", result.TracePath)
	}
	trace, err := v.Read(result.TracePath)
	if err != nil {
		t.Fatalf("Read trace: %v", err)
	}
	if trace.Get("outcome") != "failure" {
		t.Fatalf("trace outcome = %q, want failure", trace.Get("outcome"))
	}
	if !strings.Contains(trace.Body, "codex unavailable") {
		t.Fatalf("trace body missing carrier error:\n%s", trace.Body)
	}
}

func TestComposeAttachmentPromptIncludesReceiptWhenNoHotMemoryMatches(t *testing.T) {
	prompt := composeAttachmentPrompt("Summarize the current task.", RecallResult{
		ReceiptPath:       "memory/recalls/2026-04-15/recall-1.md",
		LinkedEntityRefs:  []string{"General"},
		LinkedMissionRefs: []string{"Memory Sovereignty"},
	})
	if !strings.Contains(prompt, "Recall receipt: memory/recalls/2026-04-15/recall-1.md") {
		t.Fatalf("prompt missing receipt:\n%s", prompt)
	}
	if !strings.Contains(prompt, "No hot memory matched this request.") {
		t.Fatalf("prompt missing empty recall note:\n%s", prompt)
	}
}

func TestNormalizeAttachmentCarrierAliases(t *testing.T) {
	cases := map[string]string{
		"":            "codex",
		"codex":       "codex",
		"codex-cli":   "codex",
		"codex-app":   "codex",
		"claude":      "claude",
		"claude-code": "claude",
		"qwen-cli":    "qwen",
		"gemini-cli":  "gemini",
		"ollama-cli":  "ollama",
		"hermes":      "hermes",
		"openclaw":    "openclaw",
		"opencode":    "opencode",
	}
	for input, want := range cases {
		if got := normalizeAttachmentCarrier(input); got != want {
			t.Fatalf("normalizeAttachmentCarrier(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseQwenAttachmentOutputPrefersResultRecord(t *testing.T) {
	stdout := `{"type":"assistant","message":{"content":[{"type":"text","text":"intermediate"}]}}
{"type":"result","result":"final reply"}`
	got, err := parseQwenAttachmentOutput(stdout)
	if err != nil {
		t.Fatalf("parseQwenAttachmentOutput: %v", err)
	}
	if got != "final reply" {
		t.Fatalf("result = %q, want final reply", got)
	}
}

func TestParseOpenClawAttachmentOutputExtractsNestedReply(t *testing.T) {
	stdout := `{"ok":true,"result":{"reply":{"text":"openclaw nominal"}}}`
	got, err := parseOpenClawAttachmentOutput(stdout)
	if err != nil {
		t.Fatalf("parseOpenClawAttachmentOutput: %v", err)
	}
	if got != "openclaw nominal" {
		t.Fatalf("result = %q, want openclaw nominal", got)
	}
}

func TestParseOpenClawAttachmentOutputExtractsPayloadText(t *testing.T) {
	stdout := `{"payloads":[{"text":"openclaw direct nominal.","mediaUrl":null}],"meta":{"agentMeta":{"model":"gpt-5.4"}}}`
	got, err := parseOpenClawAttachmentOutput(stdout)
	if err != nil {
		t.Fatalf("parseOpenClawAttachmentOutput: %v", err)
	}
	if got != "openclaw direct nominal." {
		t.Fatalf("result = %q, want openclaw direct nominal.", got)
	}
}

func TestParseClaudeAttachmentOutputExtractsJSONResult(t *testing.T) {
	stdout := `{"type":"result","subtype":"success","is_error":false,"result":"claude attachment nominal."}`
	got, isError, model, err := parseClaudeAttachmentOutput(stdout)
	if err != nil {
		t.Fatalf("parseClaudeAttachmentOutput: %v", err)
	}
	if isError {
		t.Fatal("expected non-error result")
	}
	if model != "" {
		t.Fatalf("model = %q, want empty", model)
	}
	if got != "claude attachment nominal." {
		t.Fatalf("result = %q, want claude attachment nominal.", got)
	}
}

func TestParseOpenCodeAttachmentOutputExtractsMessagePart(t *testing.T) {
	stdout := `{"type":"message.part.updated","model":{"id":"ollama/gemma3"},"message":{"parts":[{"type":"text","text":"opencode attachment nominal."}]}}`
	got, isError, model, err := parseOpenCodeAttachmentOutput(stdout)
	if err != nil {
		t.Fatalf("parseOpenCodeAttachmentOutput: %v", err)
	}
	if isError {
		t.Fatal("expected non-error result")
	}
	if model != "ollama/gemma3" {
		t.Fatalf("model = %q, want ollama/gemma3", model)
	}
	if got != "opencode attachment nominal." {
		t.Fatalf("result = %q, want opencode attachment nominal.", got)
	}
}

func TestParseOpenCodeAttachmentOutputExtractsFinalResponseFromTextEvent(t *testing.T) {
	stdout := `{"type":"text","part":{"text":"THOUGHT: some internal narration\n\nRESPONSE: opencode attachment nominal."}}`
	got, isError, model, err := parseOpenCodeAttachmentOutput(stdout)
	if err != nil {
		t.Fatalf("parseOpenCodeAttachmentOutput: %v", err)
	}
	if isError {
		t.Fatal("expected non-error result")
	}
	if model != "" {
		t.Fatalf("model = %q, want empty", model)
	}
	if got != "opencode attachment nominal." {
		t.Fatalf("result = %q, want opencode attachment nominal.", got)
	}
}

func TestCleanHermesAttachmentOutputStripsBannerAndSessionID(t *testing.T) {
	stdout := "╭─ ⚕ Hermes ─────────────────╮\nhermes attachment nominal.\n\nsession_id: abc123\n"
	got := cleanHermesAttachmentOutput(stdout)
	if got != "hermes attachment nominal." {
		t.Fatalf("result = %q, want hermes attachment nominal.", got)
	}
}

func TestCleanGeminiAttachmentOutputStripsMCPPrefix(t *testing.T) {
	stdout := "MCP issues detected. Run /mcp list for status.gemini attachment nominal."
	got := cleanGeminiAttachmentOutput(stdout)
	if got != "gemini attachment nominal." {
		t.Fatalf("result = %q, want gemini attachment nominal.", got)
	}
}

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

func testKernel(t *testing.T) *Kernel {
	t.Helper()
	return New(vault.New(t.TempDir(), nil))
}

func TestKernelStoreEpisodeAndFact(t *testing.T) {
	k := testKernel(t)

	episodePath, eventID, err := k.StoreEpisode("The General set memory as the primary build priority.", vault.EpisodeWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     "operator_memory_store",
		TargetDomain:       "memory/episodes",
		EventKind:          "decision",
		Subject:            "Memory Program",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreEpisode: %v", err)
	}
	if !strings.HasPrefix(episodePath, "memory/episodes/") {
		t.Fatalf("episodePath = %q, want memory/episodes path", episodePath)
	}
	factPath, err := k.StoreFact("MODUS Memory", "priority", "primary", 0.95, "critical", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     "operator_memory_store",
		TargetDomain:       "memory/facts",
		SourceEventID:      eventID,
		LineageID:          eventID,
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}
	doc, err := k.Vault.Read(factPath)
	if err != nil {
		t.Fatalf("Read fact: %v", err)
	}
	if doc.Get("source_event_id") != eventID {
		t.Fatalf("source_event_id = %q, want %q", doc.Get("source_event_id"), eventID)
	}
	if doc.Get("memory_security_class") != "canonical" {
		t.Fatalf("memory_security_class = %q, want canonical", doc.Get("memory_security_class"))
	}
}

func TestKernelHotContextUsesHotTier(t *testing.T) {
	k := testKernel(t)
	v := k.Vault
	if _, err := v.StoreFact("General", "prefers", "concise briefings", 0.9, "high"); err != nil {
		t.Fatalf("StoreFact warm: %v", err)
	}
	if _, err := v.StoreFactGoverned("General", "flagship", "brass lantern", 0.95, "high", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     "operator_memory_store",
		TargetDomain:       "memory/facts",
		MemoryTemperature:  "hot",
		AllowApproval:      true,
	}); err != nil {
		t.Fatalf("StoreFactGoverned hot: %v", err)
	}

	lines, err := k.HotContext("flagship brass lantern", 5)
	if err != nil {
		t.Fatalf("HotContext: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("HotContext returned %d results, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "brass lantern") {
		t.Fatalf("HotContext result missing expected content: %s", lines[0])
	}

	files, err := filepath.Glob(filepath.Join(v.Dir, "memory", "facts", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("expected fact files, err=%v count=%d", err, len(files))
	}
}

func TestKernelRecallWritesReceipt(t *testing.T) {
	k := testKernel(t)
	v := k.Vault
	factPath, err := v.StoreFactGoverned("General", "flagship", "brass lantern", 0.95, "high", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     "operator_memory_store",
		TargetDomain:       "memory/facts",
		SourceEventID:      "evt-flagship",
		LineageID:          "evt-flagship",
		CueTerms:           []string{"flagship", "codename"},
		MemoryTemperature:  "hot",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}

	recall, err := k.Recall(RecallRequest{
		Query:              "flagship brass lantern",
		Limit:              3,
		Options:            vault.FactSearchOptions{MemoryTemperature: "hot"},
		Harness:            "test_harness",
		Adapter:            "test_adapter",
		Mode:               "manual_search",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recall.Lines) != 1 {
		t.Fatalf("Recall returned %d lines, want 1", len(recall.Lines))
	}
	if !strings.HasPrefix(recall.ReceiptPath, "memory/recalls/") {
		t.Fatalf("ReceiptPath = %q, want memory/recalls path", recall.ReceiptPath)
	}
	receipt, err := v.Read(recall.ReceiptPath)
	if err != nil {
		t.Fatalf("Read recall receipt: %v", err)
	}
	if receipt.Get("query") != "flagship brass lantern" {
		t.Fatalf("query = %q", receipt.Get("query"))
	}
	if receipt.Get("harness") != "test_harness" {
		t.Fatalf("harness = %q", receipt.Get("harness"))
	}
	if receipt.Get("adapter") != "test_adapter" {
		t.Fatalf("adapter = %q", receipt.Get("adapter"))
	}
	if receipt.Get("mode") != "manual_search" {
		t.Fatalf("mode = %q", receipt.Get("mode"))
	}
	if receipt.Get("memory_security_class") != "operational" {
		t.Fatalf("memory_security_class = %q, want operational", receipt.Get("memory_security_class"))
	}
	if !strings.Contains(receipt.Body, factPath) {
		t.Fatalf("receipt body missing fact path %q:\n%s", factPath, receipt.Body)
	}

	fact, err := v.Read(factPath)
	if err != nil {
		t.Fatalf("Read recalled fact: %v", err)
	}
	if fact.Get("last_accessed") == "" {
		t.Fatal("expected recall to reinforce fact with last_accessed")
	}
}

func TestKernelBlocksSealedPlaintextWrites(t *testing.T) {
	k := testKernel(t)

	if _, err := k.StoreFact("Sealed note", "contains", "plain secret", 0.9, "high", vault.FactWriteAuthority{
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "kernel_test",
		StaffingContext:     "operator_test",
		AuthorityScope:      ledger.ScopeOperatorMemoryStore,
		TargetDomain:        "memory/facts",
		MemorySecurityClass: "sealed",
		AllowApproval:       true,
	}); err == nil {
		t.Fatal("expected sealed write to be blocked")
	}
}

func TestKernelSecureStateManifestWriteAndVerify(t *testing.T) {
	k := testKernel(t)
	if _, err := k.StoreFact("Founding lesson", "requires", "governed memory", 0.95, "critical", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
	}); err != nil {
		t.Fatalf("StoreFact: %v", err)
	}

	written, err := k.WriteSecureStateManifest()
	if err != nil {
		t.Fatalf("WriteSecureStateManifest: %v", err)
	}
	if written.Manifest.RootHash == "" {
		t.Fatal("expected manifest root hash")
	}

	data, err := os.ReadFile(filepath.Join(k.Vault.Dir, written.ManifestPath))
	if err != nil {
		t.Fatalf("read manifest json: %v", err)
	}
	var manifest SecureStateManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest json: %v", err)
	}
	if manifest.ClassCounts["canonical"] == 0 {
		t.Fatalf("expected canonical class count, got %+v", manifest.ClassCounts)
	}

	verified, err := k.VerifySecureStateManifest()
	if err != nil {
		t.Fatalf("VerifySecureStateManifest: %v", err)
	}
	if !verified.Verified {
		t.Fatalf("expected verification success, got %+v", verified)
	}
}

func TestKernelSecureStateManifestDetectsDrift(t *testing.T) {
	k := testKernel(t)
	factPath, err := k.StoreFact("Founding lesson", "requires", "governed memory", 0.95, "critical", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "kernel_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}
	if _, err := k.WriteSecureStateManifest(); err != nil {
		t.Fatalf("WriteSecureStateManifest: %v", err)
	}

	doc, err := k.Vault.Read(factPath)
	if err != nil {
		t.Fatalf("Read fact: %v", err)
	}
	doc.Body = "mutated memory payload"
	if err := doc.Save(); err != nil {
		t.Fatalf("mutate fact: %v", err)
	}

	verified, err := k.VerifySecureStateManifest()
	if err != nil {
		t.Fatalf("VerifySecureStateManifest: %v", err)
	}
	if verified.Verified {
		t.Fatalf("expected verification failure after drift, got %+v", verified)
	}
	if len(verified.DriftPaths) == 0 {
		t.Fatal("expected drift paths after payload mutation")
	}
}

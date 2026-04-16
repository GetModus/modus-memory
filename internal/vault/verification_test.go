package vault

import (
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/ledger"
)

func TestRecallFactsCriticalVerificationMarksVerifiedFromEpisode(t *testing.T) {
	v := testVault(t)

	_, eventID, err := v.StoreEpisodeGoverned("General flagship codename brass lantern.", EpisodeWriteAuthority{
		ProducingOffice:    "librarian",
		ProducingSubsystem: "verification_test",
		StaffingContext:    "test",
		AuthorityScope:     ledger.ScopeRuntimeMemoryStore,
		TargetDomain:       "memory/episodes",
		EventKind:          "observation",
		Subject:            "General flagship",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreEpisodeGoverned: %v", err)
	}

	factPath, err := v.StoreFactGoverned("General flagship", "codename", "brass lantern", 0.95, "critical", FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "verification_test",
		StaffingContext:    "test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		SourceEventID:      eventID,
		LineageID:          eventID,
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}

	recall, err := v.RecallFacts(RecallRequest{
		Query: "general flagship brass lantern",
		Limit: 3,
		Options: FactSearchOptions{
			VerificationMode: "critical",
		},
		Harness:            "test",
		Adapter:            "test",
		Mode:               "manual_search",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "verification_test",
		StaffingContext:    "test",
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	if len(recall.Verification) != 1 {
		t.Fatalf("verification count = %d, want 1", len(recall.Verification))
	}
	if recall.Verification[0].Status != VerificationStatusVerified {
		t.Fatalf("verification status = %q, want %q", recall.Verification[0].Status, VerificationStatusVerified)
	}
	if len(recall.Verification[0].VerifiedSourceRefs) == 0 {
		t.Fatal("expected verified source refs")
	}
	if len(recall.ResultPaths) != 1 || recall.ResultPaths[0] != factPath {
		t.Fatalf("result paths = %v, want [%s]", recall.ResultPaths, factPath)
	}
	if !strings.Contains(recall.Lines[0], "verification verified") {
		t.Fatalf("line missing verification annotation: %q", recall.Lines[0])
	}

	receipt, err := v.Read(recall.ReceiptPath)
	if err != nil {
		t.Fatalf("Read receipt: %v", err)
	}
	if receipt.Get("verification_mode") != VerificationModeCritical {
		t.Fatalf("verification_mode = %q, want %q", receipt.Get("verification_mode"), VerificationModeCritical)
	}
	if raw := receipt.Frontmatter["verification_verified_paths"]; raw == nil {
		t.Fatal("expected verification_verified_paths on receipt")
	}
}

func TestRecallFactsCriticalVerificationMarksMismatch(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "brain/notes/source.md", map[string]interface{}{
		"title": "flagship note",
	}, "General flagship codename quiet harbor.")
	seedFile(t, v, "memory/facts/flagship.md", map[string]interface{}{
		"subject":            "General flagship",
		"predicate":          "codename",
		"confidence":         0.91,
		"importance":         "high",
		"memory_temperature": "hot",
		"source_ref":         "brain/notes/source.md",
		"source_lineage":     []string{"brain/notes/source.md"},
	}, "brass lantern")

	recall, err := v.RecallFacts(RecallRequest{
		Query: "general flagship brass lantern",
		Limit: 3,
		Options: FactSearchOptions{
			VerificationMode: "critical",
		},
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	if len(recall.Verification) != 1 {
		t.Fatalf("verification count = %d, want 1", len(recall.Verification))
	}
	if recall.Verification[0].Status != VerificationStatusMismatch {
		t.Fatalf("verification status = %q, want %q", recall.Verification[0].Status, VerificationStatusMismatch)
	}
	if !strings.Contains(recall.Lines[0], "verification mismatch") {
		t.Fatalf("line missing mismatch annotation: %q", recall.Lines[0])
	}
}

func TestRecallFactsCriticalVerificationMarksReviewRequired(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "brain/notes/source.md", map[string]interface{}{
		"title": "flagship note",
	}, "General flagship codename brass lantern.")
	seedFile(t, v, "memory/facts/flagship.md", map[string]interface{}{
		"subject":                  "General flagship",
		"predicate":                "codename",
		"confidence":               0.91,
		"importance":               "critical",
		"source_ref":               "brain/notes/source.md",
		"correction_review_status": "pending",
		"stale_due_to_correction":  true,
	}, "brass lantern")

	recall, err := v.RecallFacts(RecallRequest{
		Query: "general flagship brass lantern",
		Limit: 3,
		Options: FactSearchOptions{
			VerificationMode: "critical",
		},
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	if len(recall.Verification) != 1 {
		t.Fatalf("verification count = %d, want 1", len(recall.Verification))
	}
	if recall.Verification[0].Status != VerificationStatusReviewRequired {
		t.Fatalf("verification status = %q, want %q", recall.Verification[0].Status, VerificationStatusReviewRequired)
	}
	if !strings.Contains(recall.Lines[0], "verification review required") {
		t.Fatalf("line missing review annotation: %q", recall.Lines[0])
	}
}

func TestRecallFactsCriticalVerificationMarksSourceMissing(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/flagship.md", map[string]interface{}{
		"subject":    "General flagship",
		"predicate":  "codename",
		"confidence": 0.91,
		"importance": "critical",
		"source_ref": "brain/notes/missing.md",
	}, "brass lantern")

	recall, err := v.RecallFacts(RecallRequest{
		Query: "general flagship brass lantern",
		Limit: 3,
		Options: FactSearchOptions{
			VerificationMode: "critical",
		},
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	if len(recall.Verification) != 1 {
		t.Fatalf("verification count = %d, want 1", len(recall.Verification))
	}
	if recall.Verification[0].Status != VerificationStatusSourceMissing {
		t.Fatalf("verification status = %q, want %q", recall.Verification[0].Status, VerificationStatusSourceMissing)
	}
	if !strings.Contains(recall.Lines[0], "verification source missing") {
		t.Fatalf("line missing source-missing annotation: %q", recall.Lines[0])
	}
}

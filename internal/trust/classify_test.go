package trust

import "testing"

func TestCanonicalMemoryMutationWithPromotionPathRequiresApproval(t *testing.T) {
	resp := Classify(Request{
		ProducingOffice:   "main_brain",
		ActionClass:       ActionCanonicalMemoryMutation,
		TargetDomain:      "memory",
		TouchedState:      []StateClass{StateKnowledge},
		CurrentTrustStage: 2,
		HasPromotionPath:  true,
	})

	if resp.Decision != DecisionApprovalRequired {
		t.Fatalf("decision = %q, want %q", resp.Decision, DecisionApprovalRequired)
	}
	if !resp.RequiredPromotionPath {
		t.Fatal("expected promotion path requirement")
	}
}

func TestCandidateCorrectionCreationAllowedForLibrarian(t *testing.T) {
	resp := Classify(Request{
		ProducingOffice:   "librarian",
		ActionClass:       ActionCandidateGeneration,
		TargetDomain:      "memory_correction",
		TouchedState:      []StateClass{StateReflective},
		CurrentTrustStage: 2,
	})

	if resp.Decision != DecisionAllowedWithProof {
		t.Fatalf("decision = %q, want %q", resp.Decision, DecisionAllowedWithProof)
	}
	if !resp.RequiredProof || !resp.RequiredSignature {
		t.Fatal("expected proof and signature requirements")
	}
}

func TestNonMissionOfficeCannotDirectlyMutateMissionState(t *testing.T) {
	resp := Classify(Request{
		ProducingOffice:   "scout",
		ActionClass:       ActionMissionStateMutation,
		TargetDomain:      "missions",
		TouchedState:      []StateClass{StateKnowledge},
		CurrentTrustStage: 3,
	})

	if resp.Decision != DecisionDeniedOfficeBoundary {
		t.Fatalf("decision = %q, want %q", resp.Decision, DecisionDeniedOfficeBoundary)
	}
}

func TestSessionLineageMutationRequiresProposal(t *testing.T) {
	resp := Classify(Request{
		ProducingOffice:   "session_lineage",
		ActionClass:       ActionSessionLineageMutation,
		TargetDomain:      "sessions",
		TouchedState:      []StateClass{StateReflective},
		CurrentTrustStage: 3,
		HasPromotionPath:  true,
	})

	if resp.Decision != DecisionProposalRequired {
		t.Fatalf("decision = %q, want %q", resp.Decision, DecisionProposalRequired)
	}
}

func TestDestructiveMutationRequiresApproval(t *testing.T) {
	resp := Classify(Request{
		ProducingOffice:   "librarian",
		ActionClass:       ActionDestructiveMutation,
		TargetDomain:      "memory",
		TouchedState:      []StateClass{StateKnowledge},
		CurrentTrustStage: 3,
		IsDestructive:     true,
	})

	if resp.Decision != DecisionApprovalRequired {
		t.Fatalf("decision = %q, want %q", resp.Decision, DecisionApprovalRequired)
	}
}

func TestUnknownActionClassReturnsUnknownClassification(t *testing.T) {
	resp := Classify(Request{
		ProducingOffice:   "main_brain",
		ActionClass:       ActionClass("mystery"),
		CurrentTrustStage: 1,
	})

	if resp.Decision != DecisionUnknownClassification {
		t.Fatalf("decision = %q, want %q", resp.Decision, DecisionUnknownClassification)
	}
}

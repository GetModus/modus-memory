package trust

import "fmt"

// Classify returns the first constitutional trust decision for a consequential action.
// Version 1 intentionally encodes only the highest-signal rules.
func Classify(req Request) Response {
	if req.ActionClass == "" {
		return unknown("missing action class")
	}
	if normalizeOffice(req.ProducingOffice) == "" {
		return unknown("missing producing office")
	}

	switch req.ActionClass {
	case ActionReadOnlyInspection:
		return Response{
			Decision:          DecisionAllowed,
			Reason:            "read-only inspection within declared office scope",
			RequiredSignature: false,
			RequiredProof:     false,
		}

	case ActionDerivedMirrorGeneration:
		return Response{
			Decision:          DecisionAllowedWithProof,
			Reason:            "derived mirrors are lawful with freshness, proof, and signatures",
			RequiredProof:     true,
			RequiredSignature: true,
		}

	case ActionCandidateGeneration, ActionPRCreation:
		if !candidateOwners[normalizeOffice(req.ProducingOffice)] {
			return Response{
				Decision:          DecisionDeniedOfficeBoundary,
				Reason:            fmt.Sprintf("office %q is not authorized to emit candidate outputs in version 1", req.ProducingOffice),
				RequiredSignature: true,
			}
		}
		return Response{
			Decision:          DecisionAllowedWithProof,
			Reason:            "candidate and PR creation are lawful when signed and proof-bearing",
			RequiredProof:     true,
			RequiredSignature: true,
		}

	case ActionCanonicalMemoryMutation:
		if req.HasPromotionPath {
			return Response{
				Decision:              DecisionApprovalRequired,
				Reason:                "canonical memory mutation always requires explicit approval or promotion in version 1",
				RequiredProof:         true,
				RequiredSignature:     true,
				RequiredPromotionPath: true,
			}
		}
		return Response{
			Decision:                DecisionProposalRequired,
			Reason:                  "canonical memory mutation must be transformed into a signed proposal or PR",
			RequiredProof:           true,
			RequiredSignature:       true,
			RequiredPromotionPath:   true,
			SuggestedTransformation: string(ActionPRCreation),
		}

	case ActionMissionStateMutation:
		if !missionOwners[normalizeOffice(req.ProducingOffice)] {
			return Response{
				Decision:          DecisionDeniedOfficeBoundary,
				Reason:            fmt.Sprintf("office %q does not own direct mission mutation", req.ProducingOffice),
				RequiredProof:     true,
				RequiredSignature: true,
			}
		}
		if req.CurrentTrustStage < 3 {
			return Response{
				Decision:                DecisionApprovalRequired,
				Reason:                  "mission mutation requires explicit approval below act trust scope",
				RequiredProof:           true,
				RequiredSignature:       true,
				SuggestedTransformation: string(ActionPRCreation),
			}
		}
		return Response{
			Decision:          DecisionAllowedWithProof,
			Reason:            "mission mutation is lawful for owning offices with act scope and proof",
			RequiredProof:     true,
			RequiredSignature: true,
		}

	case ActionSessionLineageMutation:
		return Response{
			Decision:                DecisionProposalRequired,
			Reason:                  "session lineage mutation requires explicit lineage-aware review",
			RequiredProof:           true,
			RequiredSignature:       true,
			RequiredPromotionPath:   true,
			SuggestedTransformation: string(ActionPRCreation),
		}

	case ActionDestructiveMutation:
		return Response{
			Decision:                DecisionApprovalRequired,
			Reason:                  "destructive archival or deletion always requires strongest explicit approval",
			RequiredProof:           true,
			RequiredSignature:       true,
			RequiredPromotionPath:   true,
			SuggestedTransformation: string(ActionPRCreation),
		}

	case ActionOperationalMutation, ActionRouteOrStaffingChange, ActionPolicyTuningChange, ActionCodeOrHarnessMutation:
		return Response{
			Decision:          DecisionApprovalRequired,
			Reason:            "version 1 treats this consequential mutation class as approval-gated until domain rules are added",
			RequiredProof:     true,
			RequiredSignature: true,
		}

	default:
		return unknown(fmt.Sprintf("unsupported action class %q", req.ActionClass))
	}
}

func unknown(reason string) Response {
	return Response{
		Decision:          DecisionUnknownClassification,
		Reason:            reason,
		RequiredSignature: true,
	}
}

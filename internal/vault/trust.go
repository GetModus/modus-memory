package vault

import (
	"fmt"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trust"
)

const trustPath = "atlas/trust.md"

// GetTrustStage reads the current trust stage from vault/atlas/trust.md.
// Returns the stage (1-3), the full frontmatter config, and any error.
func (v *Vault) GetTrustStage() (int, map[string]interface{}, error) {
	doc, err := v.Read(trustPath)
	if err != nil {
		// Default to stage 1 (Inform) if no trust file exists
		return 1, map[string]interface{}{"stage": 1}, nil
	}

	stage := int(doc.GetFloat("stage"))
	if stage < 1 || stage > 3 {
		stage = 1
	}

	return stage, doc.Frontmatter, nil
}

// SetTrustStage updates the trust stage. Only the operator should call this —
// MODUS never self-promotes. Appends a transition record to the history in the body.
func (v *Vault) SetTrustStage(stage int, updatedBy, reason string) error {
	if stage < 1 || stage > 3 {
		return fmt.Errorf("trust stage must be 1, 2, or 3 (got %d)", stage)
	}
	decision, currentStage, err := trust.ClassifyAtCurrentStage(v.Dir, trust.Request{
		ProducingOffice:    "trust_office",
		ProducingSubsystem: "atlas_trust",
		ActionClass:        trust.ActionRouteOrStaffingChange,
		TargetDomain:       "atlas/trust",
		TouchedState:       []trust.StateClass{trust.StateConstitutional, trust.StateOperational},
		RequestedAuthority: ledger.ScopeOperatorTrustStageChange,
	})
	if err != nil {
		return err
	}
	if !trust.Permits(decision, true) {
		return fmt.Errorf("trust stage change blocked by trust gate: %s", decision.Reason)
	}

	// Read existing or create default
	doc, err := v.Read(trustPath)
	var oldStage int
	var body string

	if err != nil {
		oldStage = 1
		body = "# Trust Configuration\n"
	} else {
		oldStage = int(doc.GetFloat("stage"))
		body = doc.Body
	}

	// Build transition entry
	now := time.Now().Format("2006-01-02 15:04")
	stageNames := map[int]string{1: "Inform", 2: "Recommend", 3: "Act"}
	entry := fmt.Sprintf("- %s | stage %d (%s) → %d (%s) | %s",
		now, oldStage, stageNames[oldStage], stage, stageNames[stage], updatedBy)
	if reason != "" {
		entry += " | " + reason
	}

	// Ensure history section exists, append entry
	if !strings.Contains(body, "## History") {
		body = strings.TrimRight(body, "\n") + "\n\n## History\n"
	}
	body = strings.TrimRight(body, "\n") + "\n" + entry + "\n"

	// Update the stage description line
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "Stage ") {
			lines[i] = fmt.Sprintf("Stage %d: %s", stage, stageNames[stage])
			break
		}
	}
	body = strings.Join(lines, "\n")

	// Write back
	fm := map[string]interface{}{
		"type":       "trust",
		"stage":      stage,
		"updated":    time.Now().Format(time.RFC3339),
		"updated_by": updatedBy,
		"producing_signature": signature.Signature{
			ProducingOffice:    "trust_office",
			ProducingSubsystem: "atlas_trust",
			StaffingContext:    updatedBy,
			AuthorityScope:     ledger.ScopeOperatorTrustStageChange,
			ArtifactState:      "canonical",
			PromotionStatus:    "approved",
			ProofRef:           fmt.Sprintf("trust-stage-%d", stage),
		}.EnsureTimestamp(),
	}
	if err := v.Write(trustPath, fm, body); err != nil {
		return err
	}
	return ledger.Append(v.Dir, ledger.Record{
		Office:         "trust_office",
		Subsystem:      "atlas_trust",
		AuthorityScope: ledger.ScopeOperatorTrustStageChange,
		ActionClass:    ledger.ActionTrustStageTransition,
		TargetDomain:   "atlas/trust",
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"trust_stage_updated"},
		ProofRefs:      []string{trustPath},
		Signature: signature.Signature{
			ProducingOffice:    "trust_office",
			ProducingSubsystem: "atlas_trust",
			StaffingContext:    updatedBy,
			AuthorityScope:     ledger.ScopeOperatorTrustStageChange,
			ArtifactState:      "evidentiary",
			SourceRefs:         []string{trustPath},
			PromotionStatus:    "approved",
		},
		Metadata: map[string]interface{}{
			"classifier_stage": currentStage,
			"old_stage":        oldStage,
			"new_stage":        stage,
			"updated_by":       updatedBy,
			"reason":           reason,
			"trust_decision":   string(decision.Decision),
		},
	})
}

// TrustStageLabel returns a human-readable label for a trust stage.
func TrustStageLabel(stage int) string {
	switch stage {
	case 1:
		return "Inform (Stage 1) — report only, no autonomous actions"
	case 2:
		return "Recommend (Stage 2) — propose actions for approval"
	case 3:
		return "Act (Stage 3) — execute and log autonomously"
	default:
		return fmt.Sprintf("Unknown (Stage %d)", stage)
	}
}

package trust

import (
	"fmt"
	"path/filepath"

	"github.com/GetModus/modus-memory/internal/markdown"
)

const trustConfigPath = "atlas/trust.md"

// CurrentStage reads the canonical trust stage from the vault.
// Missing or malformed configuration falls back to stage 1.
func CurrentStage(vaultDir string) (int, error) {
	doc, err := markdown.Parse(filepath.Join(vaultDir, trustConfigPath))
	if err != nil {
		return 1, nil
	}

	stage := int(doc.GetFloat("stage"))
	if stage < 1 || stage > 3 {
		return 1, nil
	}
	return stage, nil
}

// ClassifyAtCurrentStage injects the vault's current trust stage into the request.
func ClassifyAtCurrentStage(vaultDir string, req Request) (Response, int, error) {
	stage, err := CurrentStage(vaultDir)
	if err != nil {
		return Response{}, 0, fmt.Errorf("trust stage: %w", err)
	}
	req.CurrentTrustStage = stage
	return Classify(req), stage, nil
}

// Permits reports whether a decision is executable on the current surface.
// Explicit approval surfaces may proceed when the classifier requires approval.
func Permits(resp Response, allowApproval bool) bool {
	switch resp.Decision {
	case DecisionAllowed, DecisionAllowedWithProof:
		return true
	case DecisionApprovalRequired:
		return allowApproval
	default:
		return false
	}
}

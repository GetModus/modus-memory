package signature

import (
	"fmt"
	"strings"
	"time"
)

// Signature is the minimum accountability payload for consequential artifacts.
type Signature struct {
	ProducingOffice    string   `json:"producing_office" yaml:"producing_office"`
	ProducingSubsystem string   `json:"producing_subsystem,omitempty" yaml:"producing_subsystem,omitempty"`
	StaffingContext    string   `json:"staffing_context,omitempty" yaml:"staffing_context,omitempty"`
	AuthorityScope     string   `json:"authority_scope,omitempty" yaml:"authority_scope,omitempty"`
	ArtifactState      string   `json:"artifact_state" yaml:"artifact_state"`
	GeneratedAt        string   `json:"generated_at" yaml:"generated_at"`
	SourceRefs         []string `json:"source_refs,omitempty" yaml:"source_refs,omitempty"`
	PromotionStatus    string   `json:"promotion_status,omitempty" yaml:"promotion_status,omitempty"`
	ProofRef           string   `json:"proof_ref,omitempty" yaml:"proof_ref,omitempty"`
}

// EnsureTimestamp fills GeneratedAt when absent.
func (s Signature) EnsureTimestamp() Signature {
	if s.GeneratedAt == "" {
		s.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return s
}

// Validate ensures the minimum accountability fields are present.
func (s Signature) Validate() error {
	if strings.TrimSpace(s.ProducingOffice) == "" {
		return fmt.Errorf("signature missing producing_office")
	}
	if strings.TrimSpace(s.ArtifactState) == "" {
		return fmt.Errorf("signature missing artifact_state")
	}
	if strings.TrimSpace(s.GeneratedAt) == "" {
		return fmt.Errorf("signature missing generated_at")
	}
	return nil
}

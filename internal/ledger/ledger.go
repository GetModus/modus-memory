package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/GetModus/modus-memory/internal/signature"
)

// Record is the first operation-ledger schema for consequential actions.
type Record struct {
	OperationID        string                 `json:"operation_id"`
	Timestamp          string                 `json:"timestamp"`
	Office             string                 `json:"office"`
	Subsystem          string                 `json:"subsystem,omitempty"`
	AuthorityScope     string                 `json:"authority_scope,omitempty"`
	ActionClass        string                 `json:"action_class"`
	TargetDomain       string                 `json:"target_domain,omitempty"`
	ResultStatus       string                 `json:"result_status"`
	Decision           string                 `json:"decision,omitempty"`
	SuggestedTransform string                 `json:"suggested_transformation,omitempty"`
	SideEffects        []string               `json:"side_effects,omitempty"`
	ProofRefs          []string               `json:"proof_refs,omitempty"`
	Signature          signature.Signature    `json:"signature"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// Append writes a ledger record to an append-only JSONL file rooted under the vault.
func Append(vaultDir string, r Record) error {
	if r.Timestamp == "" {
		r.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if r.OperationID == "" {
		r.OperationID = "op-" + time.Now().UTC().Format("20060102T150405.000000000Z07:00")
	}
	if r.Signature.ProducingOffice == "" {
		r.Signature.ProducingOffice = r.Office
	}
	if r.Signature.ProducingSubsystem == "" {
		r.Signature.ProducingSubsystem = r.Subsystem
	}
	if r.Signature.AuthorityScope == "" {
		r.Signature.AuthorityScope = r.AuthorityScope
	}
	if r.Signature.ArtifactState == "" {
		r.Signature.ArtifactState = "evidentiary"
	}
	r.Signature = r.Signature.EnsureTimestamp()
	if r.Signature.ProofRef == "" {
		r.Signature.ProofRef = r.OperationID
	}
	if err := r.Signature.Validate(); err != nil {
		return err
	}

	path := filepath.Join(vaultDir, "state", "operations", "operations.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

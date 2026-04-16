package maintain

import (
	"fmt"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

// FactTemporalTransitionCandidate is an explicit review artifact describing a
// proposed temporal-status or supersession change for a canonical fact.
type FactTemporalTransitionCandidate struct {
	FactPath               string
	Subject                string
	Predicate              string
	CurrentTemporalStatus  string
	ProposedTemporalStatus string
	Reason                 string
	ReviewClass            string
	ObservedAt             string
	ValidFrom              string
	ValidTo                string
	SupersededByPath       string
	SourceRefs             []string
	ProducingOffice        string
	ProducingSubsystem     string
	StaffingContext        string
	AuthorityScope         string
	ProofRef               string
}

func normalizeTemporalStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "superseded":
		return "superseded"
	case "expired":
		return "expired"
	default:
		return "active"
	}
}

func normalizeTimeOrBlank(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.Format(time.RFC3339)
		}
		return ""
	}
	return t.Format(time.RFC3339)
}

func CreateFactTemporalTransitionCandidate(v *vault.Vault, candidate FactTemporalTransitionCandidate) (string, error) {
	factPath := strings.TrimSpace(candidate.FactPath)
	if factPath == "" {
		return "", fmt.Errorf("fact_path is required")
	}
	doc, err := v.Read(factPath)
	if err != nil {
		return "", fmt.Errorf("read fact for temporal transition candidate: %w", err)
	}

	current := normalizeTemporalStatus(firstNonEmpty(candidate.CurrentTemporalStatus, doc.Get("temporal_status")))
	target := normalizeTemporalStatus(candidate.ProposedTemporalStatus)
	if target == "" {
		target = "active"
	}

	subject := firstNonEmpty(strings.TrimSpace(candidate.Subject), doc.Get("subject"))
	predicate := firstNonEmpty(strings.TrimSpace(candidate.Predicate), doc.Get("predicate"))
	office := firstNonEmpty(strings.TrimSpace(candidate.ProducingOffice), "memory_governance")
	subsystem := firstNonEmpty(strings.TrimSpace(candidate.ProducingSubsystem), "temporal_review")
	authorityScope := firstNonEmpty(strings.TrimSpace(candidate.AuthorityScope), ledger.ScopeCandidateFactTemporalReview)
	reviewClass := firstNonEmpty(strings.TrimSpace(candidate.ReviewClass), "manual")
	reason := strings.TrimSpace(candidate.Reason)
	if reason == "" {
		reason = fmt.Sprintf("Proposed temporal status change from %s to %s.", current, target)
	}

	supersededByPath := strings.TrimSpace(candidate.SupersededByPath)
	if supersededByPath != "" {
		if _, err := v.Read(supersededByPath); err != nil {
			return "", fmt.Errorf("read superseded_by fact %s: %w", supersededByPath, err)
		}
	}
	if target == "superseded" && supersededByPath == "" {
		return "", fmt.Errorf("superseded_by_path is required when proposed_temporal_status is superseded")
	}

	observedAt := normalizeTimeOrBlank(candidate.ObservedAt)
	validFrom := normalizeTimeOrBlank(candidate.ValidFrom)
	validTo := normalizeTimeOrBlank(candidate.ValidTo)
	if target == "superseded" && validTo == "" {
		if supersededByPath != "" {
			if newer, err := v.Read(supersededByPath); err == nil {
				validTo = firstNonEmpty(
					normalizeTimeOrBlank(newer.Get("valid_from")),
					normalizeTimeOrBlank(newer.Get("observed_at")),
					normalizeTimeOrBlank(newer.Get("created_at")),
				)
			}
		}
		if validTo == "" {
			validTo = time.Now().Format(time.RFC3339)
		}
	}

	if current == target && observedAt == "" && validFrom == "" && validTo == "" && supersededByPath == "" {
		return "", fmt.Errorf("fact %s already has temporal status %s", factPath, target)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-temporal-transition-%s-%s", timestamp, slugify(subject), slugify(predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")
	relPath := filepathToVaultPath(v.Dir, path)

	sourceRefs := uniqueStrings(append([]string{factPath}, append(candidate.SourceRefs, supersededByPath)...))
	fm := map[string]interface{}{
		"type":                     "candidate_fact_temporal_transition",
		"status":                   "pending",
		"created":                  time.Now().Format(time.RFC3339),
		"fact_path":                factPath,
		"subject":                  subject,
		"predicate":                predicate,
		"current_temporal_status":  current,
		"proposed_temporal_status": target,
		"reason":                   reason,
		"review_class":             reviewClass,
		"producing_signature": signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "temporal-transition-candidate:"+factPath),
		}.EnsureTimestamp(),
	}
	if observedAt != "" {
		fm["observed_at"] = observedAt
	}
	if validFrom != "" {
		fm["valid_from"] = validFrom
	}
	if validTo != "" {
		fm["valid_to"] = validTo
	}
	if supersededByPath != "" {
		fm["superseded_by_path"] = supersededByPath
	}

	var body strings.Builder
	body.WriteString("# Fact Temporal Transition Candidate\n\n")
	body.WriteString(fmt.Sprintf("Fact: `%s`\n\n", factPath))
	body.WriteString(fmt.Sprintf("Temporal status: `%s` -> `%s`\n\n", current, target))
	if supersededByPath != "" {
		body.WriteString(fmt.Sprintf("Superseded by: `%s`\n\n", supersededByPath))
	}
	if observedAt != "" || validFrom != "" || validTo != "" {
		body.WriteString("Temporal fields:\n\n")
		if observedAt != "" {
			body.WriteString(fmt.Sprintf("- observed_at: `%s`\n", observedAt))
		}
		if validFrom != "" {
			body.WriteString(fmt.Sprintf("- valid_from: `%s`\n", validFrom))
		}
		if validTo != "" {
			body.WriteString(fmt.Sprintf("- valid_to: `%s`\n", validTo))
		}
		body.WriteString("\n")
	}
	body.WriteString(fmt.Sprintf("Reason: %s\n\n", reason))
	body.WriteString(fmt.Sprintf("Review class: `%s`\n\n", reviewClass))
	body.WriteString("This artifact never mutates the fact directly. To apply: set `status: approved` and run `memory_maintain` with `mode: apply`.\n")

	if err := markdown.Write(path, fm, body.String()); err != nil {
		return "", err
	}

	if err := ledger.Append(v.Dir, ledger.Record{
		Office:         office,
		Subsystem:      subsystem,
		AuthorityScope: authorityScope,
		ActionClass:    ledger.ActionReviewCandidateGeneration,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"fact_temporal_transition_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "temporal-transition-candidate:"+factPath),
		},
		Metadata: map[string]interface{}{
			"fact_path":                factPath,
			"current_temporal_status":  current,
			"proposed_temporal_status": target,
			"superseded_by_path":       supersededByPath,
			"review_class":             reviewClass,
		},
	}); err != nil {
		return "", err
	}
	return relPath, nil
}

func WriteFactTemporalTransitionCandidate(v *vault.Vault, candidate FactTemporalTransitionCandidate) error {
	_, err := CreateFactTemporalTransitionCandidate(v, candidate)
	return err
}

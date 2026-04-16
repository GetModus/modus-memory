package maintain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

// Contradict detects facts with the same subject and predicate but different values.
// Writes candidate_contradiction artifacts to memory/maintenance/ for review.
// Per Codex revision: contradictions remain inspectable until explicitly resolved.
// Never stores resolution as memory_learn correction.
func Contradict(v *vault.Vault) (int, []string, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, nil, err
	}

	// Key: "subject|predicate" → list of docs
	type spKey struct {
		subject   string
		predicate string
	}
	groups := make(map[spKey][]*markdown.Document)

	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		subj := strings.ToLower(strings.TrimSpace(doc.Get("subject")))
		pred := strings.ToLower(strings.TrimSpace(doc.Get("predicate")))
		if subj == "" || pred == "" {
			continue
		}
		key := spKey{subj, pred}
		groups[key] = append(groups[key], doc)
	}

	candidates := 0
	var actions []string

	for key, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Compare values — if any differ, it's a contradiction
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				bodyI := strings.TrimSpace(group[i].Body)
				bodyJ := strings.TrimSpace(group[j].Body)

				// Same body = duplicate (handled by consolidate), not contradiction
				if strings.EqualFold(bodyI, bodyJ) {
					continue
				}

				candidates++

				confI := group[i].GetFloat("confidence")
				confJ := group[j].GetFloat("confidence")

				// Propose keeping the higher-confidence or more recent one
				proposed, competing := group[i], group[j]
				reason := "higher confidence"
				if confJ > confI {
					proposed, competing = group[j], group[i]
				} else if confI == confJ {
					// Same confidence — prefer more recently created
					reason = "same confidence, no strong preference"
				}

				action := fmt.Sprintf("candidate_contradiction: %s/%s — two values (conf %.2f vs %.2f, %s)",
					key.subject, key.predicate, confI, confJ, reason)
				actions = append(actions, action)

				if err := writeContradictionCandidate(v, proposed, competing, key.subject, key.predicate, reason); err != nil {
					actions = append(actions, fmt.Sprintf("ERROR writing contradiction candidate: %v", err))
				}
			}
		}
	}

	if candidates > 0 {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "contradiction_review",
			AuthorityScope: ledger.ScopeCandidateContradictionGen,
			ActionClass:    ledger.ActionReviewCandidateGeneration,
			TargetDomain:   "memory/maintenance",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionAllowedWithProof,
			SideEffects:    []string{"contradiction_candidates_written"},
			ProofRefs:      []string{"memory/maintenance"},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "contradiction_review",
				AuthorityScope:     ledger.ScopeCandidateContradictionGen,
				ArtifactState:      "evidentiary",
				SourceRefs:         []string{"memory/maintenance"},
				PromotionStatus:    "advisory",
				ProofRef:           "contradiction-candidates",
			},
			Metadata: map[string]interface{}{
				"candidate_count": candidates,
			},
		})
	}

	return candidates, actions, nil
}

func writeContradictionCandidate(v *vault.Vault, proposed, competing *markdown.Document, subject, predicate, reason string) error {
	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-contradiction-%s-%s", timestamp, slugify(subject), slugify(predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")

	// Store explicit file paths so apply can target the right document
	proposedPath, _ := filepath.Rel(v.Dir, proposed.Path)
	competingPath, _ := filepath.Rel(v.Dir, competing.Path)

	fm := map[string]interface{}{
		"type":           "candidate_contradiction",
		"status":         "pending",
		"created":        time.Now().Format(time.RFC3339),
		"subject":        subject,
		"predicate":      predicate,
		"proposed_path":  proposedPath,
		"competing_path": competingPath,
		"proposed_conf":  proposed.GetFloat("confidence"),
		"competing_conf": competing.GetFloat("confidence"),
		"reason":         reason,
		"method":         "heuristic-exact-match",
		"producing_signature": signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "contradiction_review",
			AuthorityScope:     ledger.ScopeCandidateContradictionGen,
			ArtifactState:      "candidate",
			SourceRefs:         []string{proposedPath, competingPath},
			PromotionStatus:    "advisory",
			ProofRef:           "candidate-contradiction:" + slug,
		}.EnsureTimestamp(),
	}

	var body strings.Builder
	body.WriteString("# Contradiction Detected\n\n")
	body.WriteString(fmt.Sprintf("Subject: %s\nPredicate: %s\n\n", subject, predicate))

	body.WriteString("## Proposed Winner\n\n")
	body.WriteString(fmt.Sprintf("Confidence: %.2f\n\n", proposed.GetFloat("confidence")))
	body.WriteString(strings.TrimSpace(proposed.Body))

	body.WriteString("\n\n## Competing Fact\n\n")
	body.WriteString(fmt.Sprintf("Confidence: %.2f\n\n", competing.GetFloat("confidence")))
	body.WriteString(strings.TrimSpace(competing.Body))

	body.WriteString("\n\n## Resolution\n\n")
	body.WriteString(fmt.Sprintf("Proposed: keep the first (reason: %s). Archive the competing fact.\n\n", reason))
	body.WriteString("To resolve: set `status: resolved` and `winner: proposed|competing` in frontmatter.\n")
	body.WriteString("This contradiction remains inspectable until explicitly resolved.\n")

	return markdown.Write(path, fm, body.String())
}

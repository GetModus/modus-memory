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

// Consolidate detects near-duplicate facts by subject and Jaccard similarity.
// Writes candidate_merge artifacts to memory/maintenance/ for review.
// Returns the number of candidates found and a list of actions taken.
func Consolidate(v *vault.Vault) (int, []string, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, nil, err
	}

	// Group by subject (case-insensitive)
	groups := make(map[string][]*markdown.Document)
	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		subj := strings.ToLower(doc.Get("subject"))
		if subj == "" {
			continue
		}
		groups[subj] = append(groups[subj], doc)
	}

	candidates := 0
	var actions []string

	for subj, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Compare all pairs within the subject group
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				sim := jaccardSimilarity(group[i].Body, group[j].Body)
				if sim < 0.5 {
					continue
				}

				// Found a merge candidate
				candidates++

				// Determine which fact is stronger
				confI := group[i].GetFloat("confidence")
				confJ := group[j].GetFloat("confidence")
				stronger, weaker := group[i], group[j]
				if confJ > confI {
					stronger, weaker = group[j], group[i]
				}

				action := fmt.Sprintf("candidate_merge: %q ↔ %q (subject=%s, similarity=%.2f)",
					factLabel(stronger), factLabel(weaker), subj, sim)
				actions = append(actions, action)

				// Write review artifact
				if err := writeMergeCandidate(v, stronger, weaker, sim); err != nil {
					actions = append(actions, fmt.Sprintf("ERROR writing merge candidate: %v", err))
				}
			}
		}
	}

	if candidates > 0 {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "consolidation_review",
			AuthorityScope: ledger.ScopeCandidateMergeGen,
			ActionClass:    ledger.ActionReviewCandidateGeneration,
			TargetDomain:   "memory/maintenance",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionAllowedWithProof,
			SideEffects:    []string{"merge_candidates_written"},
			ProofRefs:      []string{"memory/maintenance"},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "consolidation_review",
				AuthorityScope:     ledger.ScopeCandidateMergeGen,
				ArtifactState:      "evidentiary",
				SourceRefs:         []string{"memory/maintenance"},
				PromotionStatus:    "advisory",
				ProofRef:           "merge-candidates",
			},
			Metadata: map[string]interface{}{
				"candidate_count": candidates,
			},
		})
	}

	return candidates, actions, nil
}

// jaccardSimilarity computes Jaccard index over tokenized words.
func jaccardSimilarity(a, b string) float64 {
	tokensA := tokenize(a)
	tokensB := tokenize(b)
	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}

	setA := make(map[string]bool, len(tokensA))
	for _, t := range tokensA {
		setA[t] = true
	}
	setB := make(map[string]bool, len(tokensB))
	for _, t := range tokensB {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// tokenize splits text into lowercase words, filtering short tokens and punctuation.
func tokenize(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var out []string
	for _, w := range words {
		// Strip leading/trailing punctuation
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#*-_/\\")
		if len(w) >= 2 {
			out = append(out, w)
		}
	}
	return out
}

func factLabel(doc *markdown.Document) string {
	subj := doc.Get("subject")
	pred := doc.Get("predicate")
	if pred != "" {
		return subj + " " + pred
	}
	return subj
}

func writeMergeCandidate(v *vault.Vault, stronger, weaker *markdown.Document, similarity float64) error {
	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-merge-%s", timestamp, slugify(stronger.Get("subject")))
	path := v.Path("memory", "maintenance", slug+".md")

	// Store explicit file paths so apply can target the right documents.
	// Same pattern as contradict.go — avoids fragile confidence-band matching.
	strongerPath, _ := filepath.Rel(v.Dir, stronger.Path)
	weakerPath, _ := filepath.Rel(v.Dir, weaker.Path)

	fm := map[string]interface{}{
		"type":          "candidate_merge",
		"status":        "pending",
		"created":       time.Now().Format(time.RFC3339),
		"similarity":    similarity,
		"stronger_subj": stronger.Get("subject"),
		"stronger_pred": stronger.Get("predicate"),
		"stronger_conf": stronger.GetFloat("confidence"),
		"stronger_path": strongerPath,
		"weaker_subj":   weaker.Get("subject"),
		"weaker_pred":   weaker.Get("predicate"),
		"weaker_conf":   weaker.GetFloat("confidence"),
		"weaker_path":   weakerPath,
		"method":        "heuristic-jaccard",
		"producing_signature": signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "consolidation_review",
			AuthorityScope:     ledger.ScopeCandidateMergeGen,
			ArtifactState:      "candidate",
			SourceRefs:         []string{strongerPath, weakerPath},
			PromotionStatus:    "advisory",
			ProofRef:           "candidate-merge:" + slug,
		}.EnsureTimestamp(),
	}

	var body strings.Builder
	body.WriteString("# Merge Candidate\n\n")
	body.WriteString(fmt.Sprintf("Similarity: %.2f (Jaccard)\n\n", similarity))
	body.WriteString("## Stronger Fact\n\n")
	body.WriteString(fmt.Sprintf("**%s** (confidence: %.2f)\n\n", factLabel(stronger), stronger.GetFloat("confidence")))
	body.WriteString(strings.TrimSpace(stronger.Body))
	body.WriteString("\n\n## Weaker Fact\n\n")
	body.WriteString(fmt.Sprintf("**%s** (confidence: %.2f)\n\n", factLabel(weaker), weaker.GetFloat("confidence")))
	body.WriteString(strings.TrimSpace(weaker.Body))
	body.WriteString("\n\n## Proposed Action\n\n")
	body.WriteString("Merge weaker fact into stronger. Keep stronger's body, archive weaker.\n")
	body.WriteString("To apply: set `status: approved` in this file's frontmatter, then call `memory_maintain` with `mode: apply`.\n")

	return markdown.Write(path, fm, body.String())
}

func slugify(s string) string {
	s = strings.ToLower(s)
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
}

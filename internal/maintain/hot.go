package maintain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

// HotMemoryTransitionCandidate is an explicit review artifact describing a
// proposed change to a fact's hot/warm admission status.
type HotMemoryTransitionCandidate struct {
	FactPath            string
	Subject             string
	Predicate           string
	CurrentTemperature  string
	ProposedTemperature string
	Reason              string
	ReviewClass         string
	SourceRefs          []string
	ProducingOffice     string
	ProducingSubsystem  string
	StaffingContext     string
	AuthorityScope      string
	ProofRef            string
}

type hotFactRecord struct {
	Path        string
	Subject     string
	Predicate   string
	Importance  string
	Confidence  float64
	LastTouched time.Time
}

func ReviewHotTier(v *vault.Vault) (int, []string, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, nil, err
	}

	pending := existingHotTransitionKeys(v)
	var hotFacts []hotFactRecord
	candidates := 0
	var actions []string

	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(doc.Get("memory_temperature")), "hot") {
			continue
		}
		relPath := filepathToVaultPath(v.Dir, doc.Path)
		hotFacts = append(hotFacts, hotFactRecord{
			Path:        relPath,
			Subject:     doc.Get("subject"),
			Predicate:   doc.Get("predicate"),
			Importance:  doc.Get("importance"),
			Confidence:  doc.GetFloat("confidence"),
			LastTouched: hotFactLastTouched(doc),
		})
	}

	sort.SliceStable(hotFacts, func(i, j int) bool {
		pi := hotFactPriority(hotFacts[i])
		pj := hotFactPriority(hotFacts[j])
		if pi == pj {
			if hotFacts[i].LastTouched.Equal(hotFacts[j].LastTouched) {
				return hotFacts[i].Path < hotFacts[j].Path
			}
			return hotFacts[i].LastTouched.After(hotFacts[j].LastTouched)
		}
		return pi > pj
	})

	now := time.Now()
	for idx, fact := range hotFacts {
		if stale := now.Sub(fact.LastTouched); stale > time.Duration(vault.HotMemoryStaleReviewDays)*24*time.Hour && !strings.EqualFold(fact.Importance, "critical") {
			key := hotTransitionKey(fact.Path, "warm", "stale")
			if !pending[key] {
				reason := fmt.Sprintf("Hot fact has gone %d days without review or access; propose downgrade to warm for explicit review.", int(stale.Hours()/24))
				err := WriteHotMemoryTransitionCandidate(v, HotMemoryTransitionCandidate{
					FactPath:            fact.Path,
					Subject:             fact.Subject,
					Predicate:           fact.Predicate,
					CurrentTemperature:  "hot",
					ProposedTemperature: "warm",
					Reason:              reason,
					ReviewClass:         "stale",
					SourceRefs:          []string{fact.Path},
					ProducingOffice:     "memory_governance",
					ProducingSubsystem:  "hot_tier_review",
					StaffingContext:     "stale_review",
					AuthorityScope:      ledger.ScopeCandidateHotMemoryReview,
					ProofRef:            "hot-review:stale:" + fact.Path,
				})
				if err != nil {
					actions = append(actions, fmt.Sprintf("ERROR creating stale hot review for %s: %v", fact.Path, err))
				} else {
					candidates++
					actions = append(actions, fmt.Sprintf("hot_stale_review: %s (last touched %s)", fact.Path, fact.LastTouched.Format(time.RFC3339)))
					pending[key] = true
				}
			}
		}

		if idx >= vault.HotMemoryAdmissionCap && !strings.EqualFold(fact.Importance, "critical") {
			key := hotTransitionKey(fact.Path, "warm", "overflow")
			if !pending[key] {
				reason := fmt.Sprintf("Hot tier is capped at %d facts. This fact ranked %d and should be reviewed for downgrade to warm.", vault.HotMemoryAdmissionCap, idx+1)
				err := WriteHotMemoryTransitionCandidate(v, HotMemoryTransitionCandidate{
					FactPath:            fact.Path,
					Subject:             fact.Subject,
					Predicate:           fact.Predicate,
					CurrentTemperature:  "hot",
					ProposedTemperature: "warm",
					Reason:              reason,
					ReviewClass:         "overflow",
					SourceRefs:          []string{fact.Path},
					ProducingOffice:     "memory_governance",
					ProducingSubsystem:  "hot_tier_review",
					StaffingContext:     fmt.Sprintf("rank_%d", idx+1),
					AuthorityScope:      ledger.ScopeCandidateHotMemoryReview,
					ProofRef:            "hot-review:overflow:" + fact.Path,
				})
				if err != nil {
					actions = append(actions, fmt.Sprintf("ERROR creating overflow hot review for %s: %v", fact.Path, err))
				} else {
					candidates++
					actions = append(actions, fmt.Sprintf("hot_overflow_review: %s (rank %d beyond cap %d)", fact.Path, idx+1, vault.HotMemoryAdmissionCap))
					pending[key] = true
				}
			}
		}
	}

	return candidates, actions, nil
}

func CreateHotMemoryTransitionCandidate(v *vault.Vault, candidate HotMemoryTransitionCandidate) (string, error) {
	factPath := strings.TrimSpace(candidate.FactPath)
	if factPath == "" {
		return "", fmt.Errorf("fact_path is required")
	}
	doc, err := v.Read(factPath)
	if err != nil {
		return "", fmt.Errorf("read fact for hot transition candidate: %w", err)
	}

	current := strings.TrimSpace(candidate.CurrentTemperature)
	if current == "" {
		current = doc.Get("memory_temperature")
	}
	current = normalizeTemperature(current)

	target := normalizeTemperature(candidate.ProposedTemperature)
	if current == target {
		return "", fmt.Errorf("fact %s is already %s", factPath, target)
	}

	subject := strings.TrimSpace(candidate.Subject)
	if subject == "" {
		subject = doc.Get("subject")
	}
	predicate := strings.TrimSpace(candidate.Predicate)
	if predicate == "" {
		predicate = doc.Get("predicate")
	}
	office := firstNonEmpty(strings.TrimSpace(candidate.ProducingOffice), "memory_governance")
	subsystem := firstNonEmpty(strings.TrimSpace(candidate.ProducingSubsystem), "hot_tier_review")
	authorityScope := firstNonEmpty(strings.TrimSpace(candidate.AuthorityScope), ledger.ScopeCandidateHotMemoryReview)
	reviewClass := firstNonEmpty(strings.TrimSpace(candidate.ReviewClass), "manual")
	reason := strings.TrimSpace(candidate.Reason)
	if reason == "" {
		reason = fmt.Sprintf("Proposed memory temperature change from %s to %s.", current, target)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-hot-memory-%s-%s", timestamp, slugify(subject), slugify(predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")
	relPath := filepathToVaultPath(v.Dir, path)

	sourceRefs := append([]string{factPath}, candidate.SourceRefs...)
	sourceRefs = uniqueStrings(sourceRefs)

	fm := map[string]interface{}{
		"type":                 "candidate_hot_memory_transition",
		"status":               "pending",
		"created":              time.Now().Format(time.RFC3339),
		"fact_path":            factPath,
		"subject":              subject,
		"predicate":            predicate,
		"current_temperature":  current,
		"proposed_temperature": target,
		"reason":               reason,
		"review_class":         reviewClass,
		"hot_cap":              vault.HotMemoryAdmissionCap,
		"stale_review_days":    vault.HotMemoryStaleReviewDays,
		"producing_signature": signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "hot-memory-candidate:"+factPath),
		}.EnsureTimestamp(),
	}

	var body strings.Builder
	body.WriteString("# Hot Memory Transition Candidate\n\n")
	body.WriteString(fmt.Sprintf("Fact: `%s`\n\n", factPath))
	body.WriteString(fmt.Sprintf("Transition: `%s` -> `%s`\n\n", current, target))
	body.WriteString(fmt.Sprintf("Reason: %s\n\n", reason))
	body.WriteString(fmt.Sprintf("Review class: `%s`\n\n", reviewClass))
	body.WriteString(fmt.Sprintf("Policy: hot tier capped at `%d` facts; stale review after `%d` days.\n\n", vault.HotMemoryAdmissionCap, vault.HotMemoryStaleReviewDays))
	body.WriteString("To apply: set `status: approved` in frontmatter, then call `memory_maintain` with `mode: apply`.\n")
	body.WriteString("To reject: set `status: rejected`.\n")

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
		SideEffects:    []string{"hot_memory_transition_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "hot-memory-candidate:"+factPath),
		},
		Metadata: map[string]interface{}{
			"fact_path":            factPath,
			"current_temperature":  current,
			"proposed_temperature": target,
			"review_class":         reviewClass,
		},
	}); err != nil {
		return "", err
	}
	return relPath, nil
}

func WriteHotMemoryTransitionCandidate(v *vault.Vault, candidate HotMemoryTransitionCandidate) error {
	_, err := CreateHotMemoryTransitionCandidate(v, candidate)
	return err
}

func hotFactPriority(f hotFactRecord) float64 {
	score := 0.0
	switch strings.ToLower(strings.TrimSpace(f.Importance)) {
	case "critical":
		score += 4
	case "high":
		score += 3
	case "medium":
		score += 2
	default:
		score += 1
	}
	score += f.Confidence
	score += freshnessScore(f.LastTouched)
	return score
}

func freshnessScore(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	age := time.Since(t)
	switch {
	case age <= 7*24*time.Hour:
		return 1.0
	case age <= 30*24*time.Hour:
		return 0.6
	case age <= 90*24*time.Hour:
		return 0.3
	default:
		return 0.0
	}
}

func hotFactLastTouched(doc *markdown.Document) time.Time {
	for _, key := range []string{"last_accessed", "created_at", "created"} {
		if value := strings.TrimSpace(doc.Get(key)); value != "" {
			if t, err := parseReviewTime(value); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func parseReviewTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp: %s", s)
}

func existingHotTransitionKeys(v *vault.Vault) map[string]bool {
	keys := make(map[string]bool)
	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return keys
	}
	for _, doc := range docs {
		if doc.Get("type") != "candidate_hot_memory_transition" {
			continue
		}
		status := strings.TrimSpace(doc.Get("status"))
		if status == "rejected" || status == "applied" {
			continue
		}
		key := hotTransitionKey(doc.Get("fact_path"), doc.Get("proposed_temperature"), doc.Get("review_class"))
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

func hotTransitionKey(factPath, proposedTemperature, reviewClass string) string {
	factPath = strings.TrimSpace(factPath)
	if factPath == "" {
		return ""
	}
	return factPath + "|" + normalizeTemperature(proposedTemperature) + "|" + strings.TrimSpace(reviewClass)
}

func normalizeTemperature(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hot":
		return "hot"
	default:
		return "warm"
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

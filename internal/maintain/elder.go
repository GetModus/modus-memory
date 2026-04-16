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

// ElderMemoryTransitionCandidate is an explicit review artifact describing a
// proposed change to a fact's elder-memory protection class.
type ElderMemoryTransitionCandidate struct {
	FactPath                string
	Subject                 string
	Predicate               string
	CurrentProtectionClass  string
	ProposedProtectionClass string
	Reason                  string
	ReviewClass             string
	SourceRefs              []string
	ProducingOffice         string
	ProducingSubsystem      string
	StaffingContext         string
	AuthorityScope          string
	ProofRef                string
}

type elderFactRecord struct {
	Path        string
	Subject     string
	Predicate   string
	Importance  string
	Confidence  float64
	LastTouched time.Time
	CreatedAt   time.Time
	Doc         *markdown.Document
}

func ReviewElderTier(v *vault.Vault) (int, []string, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, nil, err
	}

	pendingTransitions := existingElderTransitionKeys(v)
	pendingAnomalies := existingElderAnomalyKeys(v)
	var elderFacts []elderFactRecord
	var promotable []elderFactRecord
	candidates := 0
	var actions []string

	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		record := elderFactRecord{
			Path:        filepathToVaultPath(v.Dir, doc.Path),
			Subject:     doc.Get("subject"),
			Predicate:   doc.Get("predicate"),
			Importance:  doc.Get("importance"),
			Confidence:  doc.GetFloat("confidence"),
			LastTouched: hotFactLastTouched(doc),
			CreatedAt:   elderFactCreatedAt(doc),
			Doc:         doc,
		}
		if isElderProtectionClass(doc.Get("memory_protection_class")) {
			elderFacts = append(elderFacts, record)
		} else if elderPromotionEligible(record) {
			promotable = append(promotable, record)
		}
	}

	sort.SliceStable(elderFacts, func(i, j int) bool {
		pi := elderFactPriority(elderFacts[i])
		pj := elderFactPriority(elderFacts[j])
		if pi == pj {
			if elderFacts[i].CreatedAt.Equal(elderFacts[j].CreatedAt) {
				return elderFacts[i].Path < elderFacts[j].Path
			}
			return elderFacts[i].CreatedAt.Before(elderFacts[j].CreatedAt)
		}
		return pi > pj
	})
	sort.SliceStable(promotable, func(i, j int) bool {
		pi := elderFactPriority(promotable[i])
		pj := elderFactPriority(promotable[j])
		if pi == pj {
			if promotable[i].CreatedAt.Equal(promotable[j].CreatedAt) {
				return promotable[i].Path < promotable[j].Path
			}
			return promotable[i].CreatedAt.Before(promotable[j].CreatedAt)
		}
		return pi > pj
	})

	now := time.Now()
	for idx, fact := range elderFacts {
		if idx >= vault.ElderMemoryCap && !strings.EqualFold(strings.TrimSpace(fact.Importance), "critical") {
			key := elderTransitionKey(fact.Path, "ordinary", "overflow")
			if !pendingTransitions[key] {
				reason := fmt.Sprintf("Elder tier is capped at %d facts. This fact ranked %d and should be reviewed for return to ordinary protection.", vault.ElderMemoryCap, idx+1)
				if err := WriteElderMemoryTransitionCandidate(v, ElderMemoryTransitionCandidate{
					FactPath:                fact.Path,
					Subject:                 fact.Subject,
					Predicate:               fact.Predicate,
					CurrentProtectionClass:  "elder",
					ProposedProtectionClass: "ordinary",
					Reason:                  reason,
					ReviewClass:             "overflow",
					SourceRefs:              []string{fact.Path},
					ProducingOffice:         "memory_governance",
					ProducingSubsystem:      "elder_memory_review",
					StaffingContext:         fmt.Sprintf("rank_%d", idx+1),
					AuthorityScope:          ledger.ScopeCandidateElderMemoryReview,
					ProofRef:                "elder-review:overflow:" + fact.Path,
				}); err != nil {
					actions = append(actions, fmt.Sprintf("ERROR creating elder overflow review for %s: %v", fact.Path, err))
				} else {
					candidates++
					actions = append(actions, fmt.Sprintf("elder_overflow_review: %s (rank %d beyond cap %d)", fact.Path, idx+1, vault.ElderMemoryCap))
					pendingTransitions[key] = true
				}
			}
		}

		if stale := now.Sub(fact.LastTouched); !fact.LastTouched.IsZero() && stale > time.Duration(vault.ElderMemoryStaleReviewDays)*24*time.Hour {
			key := elderAnomalyKey(fact.Path, "stale")
			if !pendingAnomalies[key] {
				reason := fmt.Sprintf("Protected elder memory has gone %d days without recall or review. Inspect whether the protection remains justified before any demotion is considered.", int(stale.Hours()/24))
				if err := WriteElderMemoryAnomalyCandidate(v, fact.Path, "stale", reason, []string{fact.Path}); err != nil {
					actions = append(actions, fmt.Sprintf("ERROR creating elder stale anomaly for %s: %v", fact.Path, err))
				} else {
					candidates++
					actions = append(actions, fmt.Sprintf("elder_stale_anomaly: %s (last touched %s)", fact.Path, fact.LastTouched.Format(time.RFC3339)))
					pendingAnomalies[key] = true
				}
			}
		}
	}

	availableSlots := vault.ElderMemoryCap - len(elderFacts)
	if availableSlots > 3 {
		availableSlots = 3
	}
	for _, fact := range promotable {
		if availableSlots <= 0 {
			break
		}
		key := elderTransitionKey(fact.Path, "elder", "promotion")
		if pendingTransitions[key] {
			continue
		}
		reason := "Rare, high-consequence memory has aged beyond ordinary recency and retains strong provenance; propose elder protection so it is not buried by freshness bias or automatic decay."
		if err := WriteElderMemoryTransitionCandidate(v, ElderMemoryTransitionCandidate{
			FactPath:                fact.Path,
			Subject:                 fact.Subject,
			Predicate:               fact.Predicate,
			CurrentProtectionClass:  "ordinary",
			ProposedProtectionClass: "elder",
			Reason:                  reason,
			ReviewClass:             "promotion",
			SourceRefs:              elderPromotionSourceRefs(fact.Doc, fact.Path),
			ProducingOffice:         "memory_governance",
			ProducingSubsystem:      "elder_memory_review",
			StaffingContext:         "promotion_scan",
			AuthorityScope:          ledger.ScopeCandidateElderMemoryReview,
			ProofRef:                "elder-review:promotion:" + fact.Path,
		}); err != nil {
			actions = append(actions, fmt.Sprintf("ERROR creating elder promotion review for %s: %v", fact.Path, err))
		} else {
			candidates++
			availableSlots--
			actions = append(actions, fmt.Sprintf("elder_promotion_review: %s", fact.Path))
			pendingTransitions[key] = true
		}
	}

	maintenanceDocs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return candidates, actions, nil
	}
	for _, doc := range maintenanceDocs {
		if doc.Get("type") != "candidate_contradiction" {
			continue
		}
		status := strings.TrimSpace(doc.Get("status"))
		if status == "rejected" || status == "applied" {
			continue
		}
		artifactPath := filepathToVaultPath(v.Dir, doc.Path)
		for _, factPath := range []string{strings.TrimSpace(doc.Get("proposed_path")), strings.TrimSpace(doc.Get("competing_path"))} {
			if factPath == "" {
				continue
			}
			fact, err := v.Read(factPath)
			if err != nil || !isElderProtectionClass(fact.Get("memory_protection_class")) {
				continue
			}
			key := elderAnomalyKey(factPath, "contradiction")
			if pendingAnomalies[key] {
				continue
			}
			reason := fmt.Sprintf("Protected elder memory is implicated in contradiction review artifact `%s`. Resolve the conflict explicitly before trusting either branch.", artifactPath)
			if err := WriteElderMemoryAnomalyCandidate(v, factPath, "contradiction", reason, []string{factPath, artifactPath}); err != nil {
				actions = append(actions, fmt.Sprintf("ERROR creating elder contradiction anomaly for %s: %v", factPath, err))
			} else {
				candidates++
				actions = append(actions, fmt.Sprintf("elder_contradiction_anomaly: %s", factPath))
				pendingAnomalies[key] = true
			}
		}
	}

	return candidates, actions, nil
}

func CreateElderMemoryTransitionCandidate(v *vault.Vault, candidate ElderMemoryTransitionCandidate) (string, error) {
	factPath := strings.TrimSpace(candidate.FactPath)
	if factPath == "" {
		return "", fmt.Errorf("fact_path is required")
	}
	doc, err := v.Read(factPath)
	if err != nil {
		return "", fmt.Errorf("read fact for elder transition candidate: %w", err)
	}

	current := strings.TrimSpace(candidate.CurrentProtectionClass)
	if current == "" {
		current = doc.Get("memory_protection_class")
	}
	current = normalizeProtectionClass(current)

	target := normalizeProtectionClass(candidate.ProposedProtectionClass)
	if current == target {
		return "", fmt.Errorf("fact %s is already %s", factPath, target)
	}

	subject := firstNonEmpty(strings.TrimSpace(candidate.Subject), doc.Get("subject"))
	predicate := firstNonEmpty(strings.TrimSpace(candidate.Predicate), doc.Get("predicate"))
	office := firstNonEmpty(strings.TrimSpace(candidate.ProducingOffice), "memory_governance")
	subsystem := firstNonEmpty(strings.TrimSpace(candidate.ProducingSubsystem), "elder_memory_review")
	authorityScope := firstNonEmpty(strings.TrimSpace(candidate.AuthorityScope), ledger.ScopeCandidateElderMemoryReview)
	reviewClass := firstNonEmpty(strings.TrimSpace(candidate.ReviewClass), "manual")
	reason := strings.TrimSpace(candidate.Reason)
	if reason == "" {
		reason = fmt.Sprintf("Proposed protection-class change from %s to %s.", current, target)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-elder-memory-%s-%s", timestamp, slugify(subject), slugify(predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")
	relPath := filepathToVaultPath(v.Dir, path)

	sourceRefs := append([]string{factPath}, candidate.SourceRefs...)
	sourceRefs = uniqueStrings(sourceRefs)

	fm := map[string]interface{}{
		"type":                      "candidate_elder_memory_transition",
		"status":                    "pending",
		"created":                   time.Now().Format(time.RFC3339),
		"fact_path":                 factPath,
		"subject":                   subject,
		"predicate":                 predicate,
		"current_protection_class":  current,
		"proposed_protection_class": target,
		"reason":                    reason,
		"review_class":              reviewClass,
		"elder_cap":                 vault.ElderMemoryCap,
		"elder_stale_review_days":   vault.ElderMemoryStaleReviewDays,
		"producing_signature": signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "elder-memory-candidate:"+factPath),
		}.EnsureTimestamp(),
	}

	var body strings.Builder
	body.WriteString("# Elder Memory Transition Candidate\n\n")
	body.WriteString(fmt.Sprintf("Fact: `%s`\n\n", factPath))
	body.WriteString(fmt.Sprintf("Transition: `%s` -> `%s`\n\n", current, target))
	body.WriteString(fmt.Sprintf("Reason: %s\n\n", reason))
	body.WriteString(fmt.Sprintf("Review class: `%s`\n\n", reviewClass))
	body.WriteString(fmt.Sprintf("Policy: elder tier capped at `%d` facts; stale elder review after `%d` days.\n\n", vault.ElderMemoryCap, vault.ElderMemoryStaleReviewDays))
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
		SideEffects:    []string{"elder_memory_transition_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "elder-memory-candidate:"+factPath),
		},
		Metadata: map[string]interface{}{
			"fact_path":                 factPath,
			"current_protection_class":  current,
			"proposed_protection_class": target,
			"review_class":              reviewClass,
		},
	}); err != nil {
		return "", err
	}
	return relPath, nil
}

func WriteElderMemoryTransitionCandidate(v *vault.Vault, candidate ElderMemoryTransitionCandidate) error {
	_, err := CreateElderMemoryTransitionCandidate(v, candidate)
	return err
}

func WriteElderMemoryAnomalyCandidate(v *vault.Vault, factPath, anomalyClass, reason string, sourceRefs []string) error {
	factPath = strings.TrimSpace(factPath)
	if factPath == "" {
		return fmt.Errorf("fact_path is required")
	}
	doc, err := v.Read(factPath)
	if err != nil {
		return fmt.Errorf("read fact for elder anomaly candidate: %w", err)
	}

	subject := firstNonEmpty(doc.Get("subject"), "fact")
	predicate := firstNonEmpty(doc.Get("predicate"), "state")
	anomalyClass = firstNonEmpty(strings.TrimSpace(anomalyClass), "manual")
	reason = firstNonEmpty(strings.TrimSpace(reason), "Protected elder memory requires explicit review.")

	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-elder-anomaly-%s-%s", timestamp, slugify(subject), slugify(predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")
	relPath := filepathToVaultPath(v.Dir, path)

	sourceRefs = uniqueStrings(append([]string{factPath}, sourceRefs...))
	fm := map[string]interface{}{
		"type":             "candidate_elder_memory_anomaly",
		"status":           "pending",
		"created":          time.Now().Format(time.RFC3339),
		"fact_path":        factPath,
		"subject":          subject,
		"predicate":        predicate,
		"anomaly_class":    anomalyClass,
		"reason":           reason,
		"protection_class": "elder",
		"producing_signature": signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "elder_memory_review",
			StaffingContext:    anomalyClass,
			AuthorityScope:     ledger.ScopeCandidateElderMemoryReview,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           "elder-memory-anomaly:" + factPath + ":" + anomalyClass,
		}.EnsureTimestamp(),
	}

	var body strings.Builder
	body.WriteString("# Elder Memory Anomaly Review\n\n")
	body.WriteString(fmt.Sprintf("Fact: `%s`\n\n", factPath))
	body.WriteString(fmt.Sprintf("Anomaly class: `%s`\n\n", anomalyClass))
	body.WriteString(fmt.Sprintf("Reason: %s\n\n", reason))
	body.WriteString("This artifact does not authorize mutation. It exists so protected elder memory is reviewed explicitly rather than quietly decayed, archived, or contradicted.\n")

	if err := markdown.Write(path, fm, body.String()); err != nil {
		return err
	}
	return ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "elder_memory_review",
		AuthorityScope: ledger.ScopeCandidateElderMemoryReview,
		ActionClass:    ledger.ActionReviewCandidateGeneration,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"elder_memory_anomaly_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "elder_memory_review",
			StaffingContext:    anomalyClass,
			AuthorityScope:     ledger.ScopeCandidateElderMemoryReview,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           "elder-memory-anomaly:" + factPath + ":" + anomalyClass,
		},
		Metadata: map[string]interface{}{
			"fact_path":        factPath,
			"anomaly_class":    anomalyClass,
			"protection_class": "elder",
		},
	})
}

func existingElderTransitionKeys(v *vault.Vault) map[string]bool {
	keys := make(map[string]bool)
	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return keys
	}
	for _, doc := range docs {
		if doc.Get("type") != "candidate_elder_memory_transition" {
			continue
		}
		status := strings.TrimSpace(doc.Get("status"))
		if status == "rejected" || status == "applied" {
			continue
		}
		key := elderTransitionKey(doc.Get("fact_path"), doc.Get("proposed_protection_class"), doc.Get("review_class"))
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

func existingElderAnomalyKeys(v *vault.Vault) map[string]bool {
	keys := make(map[string]bool)
	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return keys
	}
	for _, doc := range docs {
		if doc.Get("type") != "candidate_elder_memory_anomaly" {
			continue
		}
		status := strings.TrimSpace(doc.Get("status"))
		if status == "rejected" || status == "applied" {
			continue
		}
		key := elderAnomalyKey(doc.Get("fact_path"), doc.Get("anomaly_class"))
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

func elderTransitionKey(factPath, proposedClass, reviewClass string) string {
	factPath = strings.TrimSpace(factPath)
	if factPath == "" {
		return ""
	}
	return factPath + "|" + normalizeProtectionClass(proposedClass) + "|" + strings.TrimSpace(reviewClass)
}

func elderAnomalyKey(factPath, anomalyClass string) string {
	factPath = strings.TrimSpace(factPath)
	anomalyClass = strings.TrimSpace(anomalyClass)
	if factPath == "" || anomalyClass == "" {
		return ""
	}
	return factPath + "|" + anomalyClass
}

func normalizeProtectionClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "elder":
		return "elder"
	default:
		return "ordinary"
	}
}

func isElderProtectionClass(value string) bool {
	return normalizeProtectionClass(value) == "elder"
}

func elderFactPriority(f elderFactRecord) float64 {
	score := 0.0
	switch strings.ToLower(strings.TrimSpace(f.Importance)) {
	case "critical":
		score += 5
	case "high":
		score += 3
	case "medium":
		score += 2
	default:
		score += 1
	}
	score += f.Confidence
	score += factProvenanceCompleteness(f.Doc)
	if !f.CreatedAt.IsZero() {
		ageDays := time.Since(f.CreatedAt).Hours() / 24
		if ageDays >= 365 {
			score += 1.5
		} else if ageDays >= 180 {
			score += 1.0
		} else if ageDays >= 90 {
			score += 0.5
		}
	}
	return score
}

func elderFactCreatedAt(doc *markdown.Document) time.Time {
	for _, key := range []string{"created_at", "created"} {
		if value := strings.TrimSpace(doc.Get(key)); value != "" {
			if t, err := parseReviewTime(value); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func elderPromotionEligible(record elderFactRecord) bool {
	importance := strings.ToLower(strings.TrimSpace(record.Importance))
	if importance != "critical" && importance != "high" {
		return false
	}
	if record.Confidence < 0.8 {
		return false
	}
	if factProvenanceCompleteness(record.Doc) < 2 {
		return false
	}
	if !hasElderPromotionLineage(record.Doc) {
		return false
	}
	if record.CreatedAt.IsZero() {
		return false
	}
	ageDays := time.Since(record.CreatedAt).Hours() / 24
	if importance == "critical" {
		return ageDays >= 30
	}
	return ageDays >= 90
}

func hasElderPromotionLineage(doc *markdown.Document) bool {
	for _, key := range []string{"mission", "work_item_id", "lineage_id", "source_event_id", "source_ref"} {
		if strings.TrimSpace(doc.Get(key)) != "" {
			return true
		}
	}
	return false
}

func factProvenanceCompleteness(doc *markdown.Document) float64 {
	score := 0.0
	for _, key := range []string{"source", "source_ref", "captured_by_office", "created_at"} {
		if strings.TrimSpace(doc.Get(key)) != "" {
			score += 1
		}
	}
	if strings.TrimSpace(doc.Get("lineage_id")) != "" || strings.TrimSpace(doc.Get("source_event_id")) != "" {
		score += 1
	}
	return score
}

func elderPromotionSourceRefs(doc *markdown.Document, factPath string) []string {
	var refs []string
	refs = append(refs, factPath)
	if sourceRef := strings.TrimSpace(doc.Get("source_ref")); sourceRef != "" {
		refs = append(refs, sourceRef)
	}
	if source := strings.TrimSpace(doc.Get("source")); source != "" {
		refs = append(refs, source)
	}
	return uniqueStrings(refs)
}

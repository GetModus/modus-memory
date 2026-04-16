package maintain

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

const structuralLinkProposalCap = 8

// StructuralLinkTransitionCandidate is an explicit review artifact describing a
// proposed additive structural-link backfill for a canonical fact.
type StructuralLinkTransitionCandidate struct {
	FactPath                    string
	Subject                     string
	Predicate                   string
	ProposedRelatedFactPaths    []string
	ProposedRelatedEpisodePaths []string
	ProposedRelatedEntityRefs   []string
	ProposedRelatedMissionRefs  []string
	Reason                      string
	ReviewClass                 string
	SourceRefs                  []string
	ProducingOffice             string
	ProducingSubsystem          string
	StaffingContext             string
	AuthorityScope              string
	ProofRef                    string
}

type structuralFactRecord struct {
	Path          string
	Subject       string
	Predicate     string
	Mission       string
	WorkItemID    string
	SourceEventID string
	LineageID     string
	Doc           *markdown.Document
}

type structuralEpisodeRecord struct {
	Path          string
	Subject       string
	Mission       string
	WorkItemID    string
	SourceEventID string
	LineageID     string
}

type entityRefRecord struct {
	Name string
	Path string
}

func ReviewStructuralLinks(v *vault.Vault) (int, []string, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, nil, err
	}

	episodes, err := markdown.ScanDir(v.Path("memory", "episodes"))
	if err != nil {
		episodes = nil
	}

	entityIndex := loadEntityRefIndex(v)
	pending := existingStructuralTransitionFactPaths(v)

	var facts []structuralFactRecord
	factsBySourceEvent := make(map[string][]string)
	factsByLineage := make(map[string][]string)
	factsByWorkItem := make(map[string][]string)
	factsByMissionSubject := make(map[string][]string)

	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		record := structuralFactRecord{
			Path:          filepathToVaultPath(v.Dir, doc.Path),
			Subject:       strings.TrimSpace(doc.Get("subject")),
			Predicate:     strings.TrimSpace(doc.Get("predicate")),
			Mission:       strings.TrimSpace(doc.Get("mission")),
			WorkItemID:    strings.TrimSpace(doc.Get("work_item_id")),
			SourceEventID: strings.TrimSpace(doc.Get("source_event_id")),
			LineageID:     strings.TrimSpace(doc.Get("lineage_id")),
			Doc:           doc,
		}
		facts = append(facts, record)
		addStructuralIndex(factsBySourceEvent, record.SourceEventID, record.Path)
		addStructuralIndex(factsByLineage, record.LineageID, record.Path)
		addStructuralIndex(factsByWorkItem, record.WorkItemID, record.Path)
		addStructuralIndex(factsByMissionSubject, structuralMissionSubjectKey(record.Mission, record.Subject), record.Path)
	}

	episodesBySourceEvent := make(map[string][]string)
	episodesByLineage := make(map[string][]string)
	episodesByWorkItem := make(map[string][]string)
	episodesByMissionSubject := make(map[string][]string)
	for _, doc := range episodes {
		if doc.Get("archived") == "true" {
			continue
		}
		record := structuralEpisodeRecord{
			Path:          filepathToVaultPath(v.Dir, doc.Path),
			Subject:       strings.TrimSpace(doc.Get("subject")),
			Mission:       strings.TrimSpace(doc.Get("mission")),
			WorkItemID:    strings.TrimSpace(doc.Get("work_item_id")),
			SourceEventID: strings.TrimSpace(doc.Get("source_event_id")),
			LineageID:     strings.TrimSpace(doc.Get("lineage_id")),
		}
		addStructuralIndex(episodesBySourceEvent, record.SourceEventID, record.Path)
		addStructuralIndex(episodesByLineage, record.LineageID, record.Path)
		addStructuralIndex(episodesByWorkItem, record.WorkItemID, record.Path)
		addStructuralIndex(episodesByMissionSubject, structuralMissionSubjectKey(record.Mission, record.Subject), record.Path)
	}

	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].Subject == facts[j].Subject {
			return facts[i].Path < facts[j].Path
		}
		return facts[i].Subject < facts[j].Subject
	})

	candidates := 0
	var actions []string
	for _, fact := range facts {
		if pending[fact.Path] {
			continue
		}

		existingFactLinks := stringSliceField(fact.Doc.Frontmatter["related_fact_paths"])
		existingEpisodeLinks := stringSliceField(fact.Doc.Frontmatter["related_episode_paths"])
		existingEntityLinks := stringSliceField(fact.Doc.Frontmatter["related_entity_refs"])
		existingMissionLinks := stringSliceField(fact.Doc.Frontmatter["related_mission_refs"])

		var (
			proposedFactPaths    []string
			proposedEpisodePaths []string
			proposedEntityRefs   []string
			proposedMissionRefs  []string
			sourceRefs           []string
			reasons              []string
		)
		sourceRefs = append(sourceRefs, fact.Path)

		if fact.Mission != "" && !containsNormalized(existingMissionLinks, fact.Mission) {
			proposedMissionRefs = append(proposedMissionRefs, fact.Mission)
			reasons = append(reasons, "commissioned mission metadata already present on the fact")
		}

		if entity, ok := entityIndex[normalizeStructuralValue(fact.Subject)]; ok && !containsNormalized(existingEntityLinks, entity.Name) {
			proposedEntityRefs = append(proposedEntityRefs, entity.Name)
			sourceRefs = append(sourceRefs, entity.Path)
			reasons = append(reasons, "exact subject/entity match in atlas")
		}

		if additions := collectStructuralPeers(factsBySourceEvent, fact.SourceEventID, fact.Path); len(additions) > 0 {
			proposedFactPaths = append(proposedFactPaths, additions...)
			sourceRefs = append(sourceRefs, additions...)
			reasons = append(reasons, "shared source_event_id")
		}
		if additions := collectStructuralPeers(episodesBySourceEvent, fact.SourceEventID, ""); len(additions) > 0 {
			proposedEpisodePaths = append(proposedEpisodePaths, additions...)
			sourceRefs = append(sourceRefs, additions...)
			reasons = append(reasons, "episode trace with matching source_event_id")
		}

		if additions := collectStructuralPeers(factsByLineage, fact.LineageID, fact.Path); len(additions) > 0 {
			proposedFactPaths = append(proposedFactPaths, additions...)
			sourceRefs = append(sourceRefs, additions...)
			reasons = append(reasons, "shared lineage_id")
		}
		if additions := collectStructuralPeers(episodesByLineage, fact.LineageID, ""); len(additions) > 0 {
			proposedEpisodePaths = append(proposedEpisodePaths, additions...)
			sourceRefs = append(sourceRefs, additions...)
			reasons = append(reasons, "episode trace with matching lineage_id")
		}

		if additions := collectStructuralPeers(factsByWorkItem, fact.WorkItemID, fact.Path); len(additions) > 0 {
			proposedFactPaths = append(proposedFactPaths, additions...)
			sourceRefs = append(sourceRefs, additions...)
			reasons = append(reasons, "shared work_item_id")
		}
		if additions := collectStructuralPeers(episodesByWorkItem, fact.WorkItemID, ""); len(additions) > 0 {
			proposedEpisodePaths = append(proposedEpisodePaths, additions...)
			sourceRefs = append(sourceRefs, additions...)
			reasons = append(reasons, "episode trace with matching work_item_id")
		}

		if key := structuralMissionSubjectKey(fact.Mission, fact.Subject); key != "" {
			if additions := collectStructuralPeers(factsByMissionSubject, key, fact.Path); len(additions) > 0 {
				proposedFactPaths = append(proposedFactPaths, additions...)
				sourceRefs = append(sourceRefs, additions...)
				reasons = append(reasons, "shared mission and subject")
			}
			if additions := collectStructuralPeers(episodesByMissionSubject, key, ""); len(additions) > 0 {
				proposedEpisodePaths = append(proposedEpisodePaths, additions...)
				sourceRefs = append(sourceRefs, additions...)
				reasons = append(reasons, "episode trace with matching mission and subject")
			}
		}

		proposedFactPaths = limitStrings(filterStructuralAdditions(uniqueStrings(proposedFactPaths), append(existingFactLinks, fact.Path)), structuralLinkProposalCap)
		proposedEpisodePaths = limitStrings(filterStructuralAdditions(uniqueStrings(proposedEpisodePaths), existingEpisodeLinks), structuralLinkProposalCap)
		proposedEntityRefs = limitStrings(filterStructuralAdditions(uniqueStrings(proposedEntityRefs), existingEntityLinks), structuralLinkProposalCap)
		proposedMissionRefs = limitStrings(filterStructuralAdditions(uniqueStrings(proposedMissionRefs), existingMissionLinks), structuralLinkProposalCap)
		sourceRefs = uniqueStrings(sourceRefs)
		reasons = uniqueStrings(reasons)

		if len(proposedFactPaths) == 0 && len(proposedEpisodePaths) == 0 {
			continue
		}
		if len(proposedFactPaths) == 0 && len(proposedEpisodePaths) == 0 && len(proposedEntityRefs) == 0 && len(proposedMissionRefs) == 0 {
			continue
		}

		reason := "Backfill explicit structural links from canonical runtime evidence."
		if len(reasons) > 0 {
			reason = fmt.Sprintf("Backfill explicit structural links from %s. This artifact only adds links already supported by canonical runtime evidence.", strings.Join(reasons, ", "))
		}

		if err := WriteStructuralLinkTransitionCandidate(v, StructuralLinkTransitionCandidate{
			FactPath:                    fact.Path,
			Subject:                     fact.Subject,
			Predicate:                   fact.Predicate,
			ProposedRelatedFactPaths:    proposedFactPaths,
			ProposedRelatedEpisodePaths: proposedEpisodePaths,
			ProposedRelatedEntityRefs:   proposedEntityRefs,
			ProposedRelatedMissionRefs:  proposedMissionRefs,
			Reason:                      reason,
			ReviewClass:                 "backfill",
			SourceRefs:                  sourceRefs,
			ProducingOffice:             "memory_governance",
			ProducingSubsystem:          "structural_link_review",
			StaffingContext:             "hard_evidence_backfill",
			AuthorityScope:              ledger.ScopeCandidateStructuralLinkReview,
			ProofRef:                    "structural-link-candidate:" + fact.Path,
		}); err != nil {
			actions = append(actions, fmt.Sprintf("ERROR creating structural-link review for %s: %v", fact.Path, err))
			continue
		}

		candidates++
		pending[fact.Path] = true
		actions = append(actions, fmt.Sprintf("structural_link_review: %s (+%d facts, +%d episodes, +%d entities, +%d missions)", fact.Path, len(proposedFactPaths), len(proposedEpisodePaths), len(proposedEntityRefs), len(proposedMissionRefs)))
	}

	return candidates, actions, nil
}

func CreateStructuralLinkTransitionCandidate(v *vault.Vault, candidate StructuralLinkTransitionCandidate) (string, error) {
	factPath := strings.TrimSpace(candidate.FactPath)
	if factPath == "" {
		return "", fmt.Errorf("fact_path is required")
	}
	doc, err := v.Read(factPath)
	if err != nil {
		return "", fmt.Errorf("read fact for structural transition candidate: %w", err)
	}

	existingFactLinks := stringSliceField(doc.Frontmatter["related_fact_paths"])
	existingEpisodeLinks := stringSliceField(doc.Frontmatter["related_episode_paths"])
	existingEntityLinks := stringSliceField(doc.Frontmatter["related_entity_refs"])
	existingMissionLinks := stringSliceField(doc.Frontmatter["related_mission_refs"])

	proposedFactPaths := limitStrings(filterStructuralAdditions(uniqueStrings(candidate.ProposedRelatedFactPaths), append(existingFactLinks, factPath)), structuralLinkProposalCap)
	proposedEpisodePaths := limitStrings(filterStructuralAdditions(uniqueStrings(candidate.ProposedRelatedEpisodePaths), existingEpisodeLinks), structuralLinkProposalCap)
	proposedEntityRefs := limitStrings(filterStructuralAdditions(uniqueStrings(candidate.ProposedRelatedEntityRefs), existingEntityLinks), structuralLinkProposalCap)
	proposedMissionRefs := limitStrings(filterStructuralAdditions(uniqueStrings(candidate.ProposedRelatedMissionRefs), existingMissionLinks), structuralLinkProposalCap)

	if len(proposedFactPaths) == 0 && len(proposedEpisodePaths) == 0 && len(proposedEntityRefs) == 0 && len(proposedMissionRefs) == 0 {
		return "", fmt.Errorf("no new structural links proposed for %s", factPath)
	}
	for _, proposedFactPath := range proposedFactPaths {
		if _, err := v.Read(proposedFactPath); err != nil {
			return "", fmt.Errorf("read related fact %s: %w", proposedFactPath, err)
		}
	}
	for _, proposedEpisodePath := range proposedEpisodePaths {
		if _, err := v.Read(proposedEpisodePath); err != nil {
			return "", fmt.Errorf("read related episode %s: %w", proposedEpisodePath, err)
		}
	}
	if len(proposedEntityRefs) > 0 {
		entityIndex := loadEntityRefIndex(v)
		for _, proposedEntityRef := range proposedEntityRefs {
			if _, ok := entityIndex[normalizeStructuralValue(proposedEntityRef)]; !ok {
				return "", fmt.Errorf("unknown related entity ref %s", proposedEntityRef)
			}
		}
	}

	subject := firstNonEmpty(strings.TrimSpace(candidate.Subject), doc.Get("subject"))
	predicate := firstNonEmpty(strings.TrimSpace(candidate.Predicate), doc.Get("predicate"))
	office := firstNonEmpty(strings.TrimSpace(candidate.ProducingOffice), "memory_governance")
	subsystem := firstNonEmpty(strings.TrimSpace(candidate.ProducingSubsystem), "structural_link_review")
	authorityScope := firstNonEmpty(strings.TrimSpace(candidate.AuthorityScope), ledger.ScopeCandidateStructuralLinkReview)
	reviewClass := firstNonEmpty(strings.TrimSpace(candidate.ReviewClass), "manual")
	reason := strings.TrimSpace(candidate.Reason)
	if reason == "" {
		reason = "Proposed structural link backfill from canonical runtime evidence."
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-structural-links-%s-%s", timestamp, slugify(subject), slugify(predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")
	relPath := filepathToVaultPath(v.Dir, path)

	sourceRefs := uniqueStrings(append([]string{factPath}, candidate.SourceRefs...))

	fm := map[string]interface{}{
		"type":         "candidate_structural_link_transition",
		"status":       "pending",
		"created":      time.Now().Format(time.RFC3339),
		"fact_path":    factPath,
		"subject":      subject,
		"predicate":    predicate,
		"reason":       reason,
		"review_class": reviewClass,
		"proposal_cap": structuralLinkProposalCap,
		"producing_signature": signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "structural-link-candidate:"+factPath),
		}.EnsureTimestamp(),
	}
	if len(sourceRefs) > 0 {
		fm["source_refs"] = sourceRefs
	}
	if len(proposedFactPaths) > 0 {
		fm["proposed_related_fact_paths"] = proposedFactPaths
	}
	if len(proposedEpisodePaths) > 0 {
		fm["proposed_related_episode_paths"] = proposedEpisodePaths
	}
	if len(proposedEntityRefs) > 0 {
		fm["proposed_related_entity_refs"] = proposedEntityRefs
	}
	if len(proposedMissionRefs) > 0 {
		fm["proposed_related_mission_refs"] = proposedMissionRefs
	}

	var body strings.Builder
	body.WriteString("# Structural Link Transition Candidate\n\n")
	body.WriteString(fmt.Sprintf("Fact: `%s`\n\n", factPath))
	body.WriteString(fmt.Sprintf("Reason: %s\n\n", reason))
	body.WriteString(fmt.Sprintf("Review class: `%s`\n\n", reviewClass))
	body.WriteString(fmt.Sprintf("Proposal cap: `%d` links per class.\n\n", structuralLinkProposalCap))
	if len(proposedFactPaths) > 0 {
		body.WriteString("Proposed related facts:\n\n")
		for _, value := range proposedFactPaths {
			body.WriteString(fmt.Sprintf("- `%s`\n", value))
		}
		body.WriteString("\n")
	}
	if len(proposedEpisodePaths) > 0 {
		body.WriteString("Proposed related episodes:\n\n")
		for _, value := range proposedEpisodePaths {
			body.WriteString(fmt.Sprintf("- `%s`\n", value))
		}
		body.WriteString("\n")
	}
	if len(proposedEntityRefs) > 0 {
		body.WriteString("Proposed related entities:\n\n")
		for _, value := range proposedEntityRefs {
			body.WriteString(fmt.Sprintf("- `%s`\n", value))
		}
		body.WriteString("\n")
	}
	if len(proposedMissionRefs) > 0 {
		body.WriteString("Proposed related missions:\n\n")
		for _, value := range proposedMissionRefs {
			body.WriteString(fmt.Sprintf("- `%s`\n", value))
		}
		body.WriteString("\n")
	}
	body.WriteString("This artifact only adds explicit structural links. It never deletes existing links or rewrites memory history. To apply: set `status: approved` and run `memory_maintain` with `mode: apply`.\n")

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
		SideEffects:    []string{"structural_link_transition_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     authorityScope,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           firstNonEmpty(candidate.ProofRef, "structural-link-candidate:"+factPath),
		},
		Metadata: map[string]interface{}{
			"fact_path":                      factPath,
			"review_class":                   reviewClass,
			"proposed_related_fact_count":    len(proposedFactPaths),
			"proposed_related_episode_count": len(proposedEpisodePaths),
			"proposed_related_entity_count":  len(proposedEntityRefs),
			"proposed_related_mission_count": len(proposedMissionRefs),
		},
	}); err != nil {
		return "", err
	}
	return relPath, nil
}

func WriteStructuralLinkTransitionCandidate(v *vault.Vault, candidate StructuralLinkTransitionCandidate) error {
	_, err := CreateStructuralLinkTransitionCandidate(v, candidate)
	return err
}

func loadEntityRefIndex(v *vault.Vault) map[string]entityRefRecord {
	index := make(map[string]entityRefRecord)
	docs, err := markdown.ScanDir(v.Path("atlas", "entities"))
	if err != nil {
		return index
	}
	for _, doc := range docs {
		name := firstNonEmpty(strings.TrimSpace(doc.Get("name")), strings.TrimSuffix(filepath.Base(doc.Path), filepath.Ext(doc.Path)))
		key := normalizeStructuralValue(name)
		if key == "" {
			continue
		}
		index[key] = entityRefRecord{
			Name: name,
			Path: filepathToVaultPath(v.Dir, doc.Path),
		}
	}
	return index
}

func existingStructuralTransitionFactPaths(v *vault.Vault) map[string]bool {
	keys := make(map[string]bool)
	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return keys
	}
	for _, doc := range docs {
		if doc.Get("type") != "candidate_structural_link_transition" {
			continue
		}
		status := strings.TrimSpace(doc.Get("status"))
		if status == "rejected" || status == "applied" {
			continue
		}
		if len(stringSliceField(doc.Frontmatter["proposed_related_fact_paths"])) == 0 && len(stringSliceField(doc.Frontmatter["proposed_related_episode_paths"])) == 0 {
			continue
		}
		factPath := strings.TrimSpace(doc.Get("fact_path"))
		if factPath != "" {
			keys[factPath] = true
		}
	}
	return keys
}

func addStructuralIndex(index map[string][]string, key, value string) {
	key = normalizeStructuralValue(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	index[key] = append(index[key], value)
}

func collectStructuralPeers(index map[string][]string, key, exclude string) []string {
	key = normalizeStructuralValue(key)
	if key == "" {
		return nil
	}
	var out []string
	for _, value := range index[key] {
		value = strings.TrimSpace(value)
		if value == "" || value == exclude {
			continue
		}
		out = append(out, value)
	}
	return uniqueStrings(out)
}

func structuralMissionSubjectKey(mission, subject string) string {
	mission = normalizeStructuralValue(mission)
	subject = normalizeStructuralValue(subject)
	if mission == "" || subject == "" {
		return ""
	}
	return mission + "|" + subject
}

func normalizeStructuralValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func containsNormalized(values []string, target string) bool {
	target = normalizeStructuralValue(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if normalizeStructuralValue(value) == target {
			return true
		}
	}
	return false
}

func filterStructuralAdditions(values, existing []string) []string {
	if len(values) == 0 {
		return nil
	}
	blocked := make(map[string]bool, len(existing))
	for _, value := range existing {
		key := normalizeStructuralValue(value)
		if key != "" {
			blocked[key] = true
		}
	}
	var out []string
	for _, value := range values {
		key := normalizeStructuralValue(value)
		if key == "" || blocked[key] {
			continue
		}
		blocked[key] = true
		out = append(out, strings.TrimSpace(value))
	}
	sort.Strings(out)
	return out
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

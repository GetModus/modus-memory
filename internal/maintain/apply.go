package maintain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trust"
	"github.com/GetModus/modus-memory/internal/vault"
)

// ApplyResult holds the outcome of applying approved maintenance artifacts.
type ApplyResult struct {
	MergesApplied                int
	ContradictionsResolved       int
	BootstrapPromoted            int
	ReplayPromoted               int
	HotTransitionsApplied        int
	StructuralTransitionsApplied int
	TemporalTransitionsApplied   int
	ElderTransitionsApplied      int
	Actions                      []string
}

// ApplyApproved scans memory/maintenance/ for artifacts with status "approved" or
// "resolved", executes the proposed actions, and marks them as "applied".
// Only acts on explicitly approved artifacts — never on "pending" ones.
func ApplyApproved(v *vault.Vault) (*ApplyResult, error) {
	decision, stage, err := trust.ClassifyAtCurrentStage(v.Dir, trust.Request{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "maintenance_apply",
		ActionClass:        trust.ActionOperationalMutation,
		TargetDomain:       "memory/maintenance",
		TouchedState:       []trust.StateClass{trust.StateKnowledge, trust.StateEvidentiary},
		RequestedAuthority: ledger.ScopeApprovedMaintenanceApply,
	})
	if err != nil {
		return nil, err
	}
	if !trust.Permits(decision, true) {
		return nil, fmt.Errorf("maintenance apply blocked by trust gate: %s", decision.Reason)
	}

	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return &ApplyResult{}, nil
	}

	result := &ApplyResult{}

	for _, doc := range docs {
		status := doc.Get("status")
		if status != "approved" && status != "resolved" {
			continue
		}

		docType := doc.Get("type")
		switch docType {
		case "candidate_merge":
			if err := applyMerge(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying merge %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.MergesApplied++
			result.Actions = append(result.Actions, fmt.Sprintf("Merged: %s (archived weaker fact)", doc.Get("weaker_subj")))

		case "candidate_contradiction":
			if err := applyContradiction(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying contradiction %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.ContradictionsResolved++
			result.Actions = append(result.Actions, fmt.Sprintf("Resolved contradiction: %s/%s", doc.Get("subject"), doc.Get("predicate")))

		case "candidate_bootstrap_fact":
			if err := applyBootstrap(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying bootstrap %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.BootstrapPromoted++
			result.Actions = append(result.Actions, fmt.Sprintf("Promoted fact: %s %s", doc.Get("subject"), doc.Get("predicate")))

		case "candidate_replay_fact":
			if err := applyBootstrap(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying replay promotion %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.ReplayPromoted++
			result.Actions = append(result.Actions, fmt.Sprintf("Promoted replay fact: %s %s", doc.Get("subject"), doc.Get("predicate")))

		case "candidate_hot_memory_transition":
			if err := applyHotMemoryTransition(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying hot transition %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.HotTransitionsApplied++
			result.Actions = append(result.Actions, fmt.Sprintf("Hot memory transition applied: %s -> %s (%s)", doc.Get("fact_path"), doc.Get("proposed_temperature"), doc.Get("review_class")))

		case "candidate_structural_link_transition":
			if err := applyStructuralLinkTransition(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying structural transition %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.StructuralTransitionsApplied++
			result.Actions = append(result.Actions, fmt.Sprintf("Structural link transition applied: %s (+%s facts, +%s episodes, +%s entities, +%s missions)", doc.Get("fact_path"), countTransitionField(doc.Frontmatter["proposed_related_fact_paths"]), countTransitionField(doc.Frontmatter["proposed_related_episode_paths"]), countTransitionField(doc.Frontmatter["proposed_related_entity_refs"]), countTransitionField(doc.Frontmatter["proposed_related_mission_refs"])))

		case "candidate_fact_temporal_transition":
			if err := applyFactTemporalTransition(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying temporal transition %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.TemporalTransitionsApplied++
			result.Actions = append(result.Actions, fmt.Sprintf("Temporal transition applied: %s -> %s (%s)", doc.Get("fact_path"), doc.Get("proposed_temporal_status"), doc.Get("review_class")))

		case "candidate_elder_memory_transition":
			if err := applyElderMemoryTransition(v, doc); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("ERROR applying elder transition %s: %v", filepath.Base(doc.Path), err))
				continue
			}
			result.ElderTransitionsApplied++
			result.Actions = append(result.Actions, fmt.Sprintf("Elder memory transition applied: %s -> %s (%s)", doc.Get("fact_path"), doc.Get("proposed_protection_class"), doc.Get("review_class")))
		}

		// Mark as applied
		doc.Set("status", "applied")
		doc.Set("applied_at", time.Now().Format(time.RFC3339))
		doc.Save()

		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "maintenance_apply",
			AuthorityScope: ledger.ScopeApprovedMaintenanceApply,
			ActionClass:    ledger.ActionMaintenanceApply,
			TargetDomain:   doc.Path,
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionApproved,
			SideEffects:    []string{"maintenance_artifact_applied"},
			ProofRefs:      []string{doc.Path},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "maintenance_apply",
				StaffingContext:    docType,
				AuthorityScope:     ledger.ScopeApprovedMaintenanceApply,
				ArtifactState:      "evidentiary",
				SourceRefs:         []string{doc.Path},
				PromotionStatus:    "approved",
				ProofRef:           "maintenance-apply:" + filepath.Base(doc.Path),
			},
			Metadata: map[string]interface{}{
				"classifier_stage": stage,
				"type":             docType,
				"status":           "applied",
				"trust_decision":   string(decision.Decision),
			},
		})
	}

	return result, nil
}

// applyMerge archives the weaker fact.
// Uses explicit weaker_path stored in artifact to identify the exact document.
// Falls back to subject+predicate+confidence matching for pre-path artifacts.
func applyMerge(v *vault.Vault, doc *markdown.Document) error {
	weakerPath := doc.Get("weaker_path")
	if weakerPath != "" {
		absPath := filepath.Join(v.Dir, weakerPath)
		weaker, err := markdown.Parse(absPath)
		if err != nil {
			return fmt.Errorf("cannot read weaker fact at %s: %w", weakerPath, err)
		}
		if weaker.Get("archived") == "true" {
			return nil // already archived
		}
		weaker.Set("archived", "true")
		weaker.Set("archived_at", time.Now().Format(time.RFC3339))
		weaker.Set("archived_reason", "merged by maintenance")
		return weaker.Save()
	}

	// Legacy fallback for artifacts without explicit paths
	return applyMergeBySubject(v, doc)
}

// applyMergeBySubject is the legacy fallback for merge artifacts that lack
// explicit file paths. Matches by subject+predicate+confidence band.
func applyMergeBySubject(v *vault.Vault, doc *markdown.Document) error {
	weakerSubj := doc.Get("weaker_subj")
	weakerPred := doc.Get("weaker_pred")
	if weakerSubj == "" {
		return fmt.Errorf("weaker_subj not set and no explicit path available")
	}

	facts, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return err
	}

	for _, fact := range facts {
		if strings.EqualFold(fact.Get("subject"), weakerSubj) &&
			(weakerPred == "" || strings.EqualFold(fact.Get("predicate"), weakerPred)) &&
			fact.Get("archived") != "true" {
			if fact.GetFloat("confidence") <= doc.GetFloat("weaker_conf")+0.01 {
				fact.Set("archived", "true")
				fact.Set("archived_at", time.Now().Format(time.RFC3339))
				fact.Set("archived_reason", "merged by maintenance")
				return fact.Save()
			}
		}
	}

	return fmt.Errorf("weaker fact not found: %s %s", weakerSubj, weakerPred)
}

// applyContradiction archives the losing fact based on the winner field.
// Uses explicit file paths (proposed_path/competing_path) stored in the artifact
// to identify the exact documents — avoids fragile confidence-band matching.
func applyContradiction(v *vault.Vault, doc *markdown.Document) error {
	winner := doc.Get("winner") // "proposed" or "competing"

	// Default to proposed winner if winner field not explicitly set
	if winner == "" {
		winner = "proposed"
	}

	// Determine which path to archive
	var archivePath string
	if winner == "proposed" {
		archivePath = doc.Get("competing_path")
	} else {
		archivePath = doc.Get("proposed_path")
	}

	if archivePath == "" {
		// Fallback for artifacts created before path tracking was added
		return applyContradictionBySubject(v, doc, winner)
	}

	// Resolve relative path against vault root
	absPath := filepath.Join(v.Dir, archivePath)
	loser, err := markdown.Parse(absPath)
	if err != nil {
		return fmt.Errorf("cannot read losing fact at %s: %w", archivePath, err)
	}

	if loser.Get("archived") == "true" {
		return nil // already archived
	}

	loser.Set("archived", "true")
	loser.Set("archived_at", time.Now().Format(time.RFC3339))
	loser.Set("archived_reason", "contradiction resolved by maintenance")
	return loser.Save()
}

// applyContradictionBySubject is the legacy fallback for contradiction artifacts
// that lack explicit file paths. Matches by subject+predicate+confidence band.
func applyContradictionBySubject(v *vault.Vault, doc *markdown.Document, winner string) error {
	subject := doc.Get("subject")
	predicate := doc.Get("predicate")
	if subject == "" || predicate == "" {
		return fmt.Errorf("subject/predicate not set and no explicit paths available")
	}

	var archiveConf float64
	if winner == "proposed" {
		archiveConf = doc.GetFloat("competing_conf")
	} else {
		archiveConf = doc.GetFloat("proposed_conf")
	}

	facts, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return err
	}

	for _, fact := range facts {
		if strings.EqualFold(fact.Get("subject"), subject) &&
			strings.EqualFold(fact.Get("predicate"), predicate) &&
			fact.Get("archived") != "true" {
			factConf := fact.GetFloat("confidence")
			if factConf >= archiveConf-0.01 && factConf <= archiveConf+0.01 {
				fact.Set("archived", "true")
				fact.Set("archived_at", time.Now().Format(time.RFC3339))
				fact.Set("archived_reason", "contradiction resolved by maintenance")
				return fact.Save()
			}
		}
	}

	return fmt.Errorf("losing fact not found: %s %s (conf ~%.2f)", subject, predicate, archiveConf)
}

// applyBootstrap creates a new fact from an approved bootstrap candidate.
func applyBootstrap(v *vault.Vault, doc *markdown.Document) error {
	subject := doc.Get("subject")
	predicate := doc.Get("predicate")
	value := doc.Get("value")
	if subject == "" || predicate == "" || value == "" {
		return fmt.Errorf("subject/predicate/value not set")
	}

	confidence := doc.GetFloat("confidence")
	if confidence <= 0 {
		confidence = 0.5
	}
	importance := doc.Get("importance")
	if importance == "" {
		importance = "medium"
	}

	_, err := v.StoreFactGoverned(subject, predicate, value, confidence, importance, vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "maintenance_apply",
		StaffingContext:    doc.Get("type"),
		AuthorityScope:     ledger.ScopeApprovedMaintenanceApply,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
		Source:             firstNonEmpty(doc.Get("source"), filepathToVaultPath(v.Dir, doc.Path)),
		SourceRef:          firstNonEmpty(doc.Get("source_ref"), doc.Get("source")),
		SourceRefs:         uniqueStrings(append(stringSliceField(doc.Frontmatter["source_refs"]), filepathToVaultPath(v.Dir, doc.Path))),
		SourceEventID:      doc.Get("source_event_id"),
		LineageID:          doc.Get("lineage_id"),
		Mission:            doc.Get("mission"),
		WorkItemID:         firstNonEmpty(doc.Get("work_item_id"), doc.Get("route_work_item_id")),
		Environment:        firstNonEmpty(doc.Get("environment"), doc.Get("route_environment")),
		CueTerms:           stringSliceField(doc.Frontmatter["cue_terms"]),
		ProofRef:           "maintenance-bootstrap:" + filepath.Base(doc.Path),
		PromotionStatus:    "approved",
	})
	return err
}

func applyHotMemoryTransition(v *vault.Vault, doc *markdown.Document) error {
	factPath := strings.TrimSpace(doc.Get("fact_path"))
	if factPath == "" {
		return fmt.Errorf("fact_path not set")
	}

	fact, err := v.Read(factPath)
	if err != nil {
		return fmt.Errorf("read fact %s: %w", factPath, err)
	}

	current := normalizeTemperature(fact.Get("memory_temperature"))
	target := normalizeTemperature(doc.Get("proposed_temperature"))
	if current == target {
		return nil
	}

	now := time.Now().Format(time.RFC3339)
	historyEntry := fmt.Sprintf("%s %s->%s via %s (%s)", now, current, target, filepathToVaultPath(v.Dir, doc.Path), strings.TrimSpace(doc.Get("reason")))
	history := uniqueStrings(append(stringSliceField(fact.Frontmatter["memory_temperature_history"]), historyEntry))

	fact.Set("memory_temperature_previous", current)
	fact.Set("memory_temperature", target)
	fact.Set("memory_temperature_reviewed_at", now)
	fact.Set("memory_temperature_reviewed_by", "memory_governance")
	fact.Set("memory_temperature_review_reason", doc.Get("reason"))
	fact.Set("memory_temperature_review_class", doc.Get("review_class"))
	fact.Set("memory_temperature_review_artifact", filepathToVaultPath(v.Dir, doc.Path))
	fact.Set("memory_temperature_history", history)
	if err := fact.Save(); err != nil {
		return err
	}

	return ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "maintenance_apply",
		AuthorityScope: ledger.ScopeApprovedMaintenanceApply,
		ActionClass:    ledger.ActionMemoryTemperatureTransition,
		TargetDomain:   factPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"memory_temperature_changed"},
		ProofRefs:      []string{factPath, filepathToVaultPath(v.Dir, doc.Path)},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "maintenance_apply",
			StaffingContext:    doc.Get("review_class"),
			AuthorityScope:     ledger.ScopeApprovedMaintenanceApply,
			ArtifactState:      "canonical",
			SourceRefs:         []string{factPath, filepathToVaultPath(v.Dir, doc.Path)},
			PromotionStatus:    "approved",
			ProofRef:           "memory-temperature-transition:" + filepath.Base(doc.Path),
		},
		Metadata: map[string]interface{}{
			"fact_path":            factPath,
			"current_temperature":  current,
			"proposed_temperature": target,
			"review_class":         doc.Get("review_class"),
		},
	})
}

func applyStructuralLinkTransition(v *vault.Vault, doc *markdown.Document) error {
	factPath := strings.TrimSpace(doc.Get("fact_path"))
	if factPath == "" {
		return fmt.Errorf("fact_path not set")
	}

	fact, err := v.Read(factPath)
	if err != nil {
		return fmt.Errorf("read fact %s: %w", factPath, err)
	}

	currentFactLinks := stringSliceField(fact.Frontmatter["related_fact_paths"])
	currentEpisodeLinks := stringSliceField(fact.Frontmatter["related_episode_paths"])
	currentEntityLinks := stringSliceField(fact.Frontmatter["related_entity_refs"])
	currentMissionLinks := stringSliceField(fact.Frontmatter["related_mission_refs"])

	proposedFactLinks := stringSliceField(doc.Frontmatter["proposed_related_fact_paths"])
	proposedEpisodeLinks := stringSliceField(doc.Frontmatter["proposed_related_episode_paths"])
	proposedEntityLinks := stringSliceField(doc.Frontmatter["proposed_related_entity_refs"])
	proposedMissionLinks := stringSliceField(doc.Frontmatter["proposed_related_mission_refs"])

	mergedFactLinks := uniqueStrings(append(currentFactLinks, proposedFactLinks...))
	mergedEpisodeLinks := uniqueStrings(append(currentEpisodeLinks, proposedEpisodeLinks...))
	mergedEntityLinks := uniqueStrings(append(currentEntityLinks, proposedEntityLinks...))
	mergedMissionLinks := uniqueStrings(append(currentMissionLinks, proposedMissionLinks...))

	now := time.Now().Format(time.RFC3339)
	artifactPath := filepathToVaultPath(v.Dir, doc.Path)
	historyEntry := fmt.Sprintf("%s +facts:%d +episodes:%d +entities:%d +missions:%d via %s (%s)", now, len(mergedFactLinks)-len(currentFactLinks), len(mergedEpisodeLinks)-len(currentEpisodeLinks), len(mergedEntityLinks)-len(currentEntityLinks), len(mergedMissionLinks)-len(currentMissionLinks), artifactPath, strings.TrimSpace(doc.Get("reason")))
	history := uniqueStrings(append(stringSliceField(fact.Frontmatter["structural_link_history"]), historyEntry))

	if len(mergedFactLinks) > 0 {
		fact.Set("related_fact_paths", mergedFactLinks)
	}
	if len(mergedEpisodeLinks) > 0 {
		fact.Set("related_episode_paths", mergedEpisodeLinks)
	}
	if len(mergedEntityLinks) > 0 {
		fact.Set("related_entity_refs", mergedEntityLinks)
	}
	if len(mergedMissionLinks) > 0 {
		fact.Set("related_mission_refs", mergedMissionLinks)
	}
	fact.Set("structural_link_reviewed_at", now)
	fact.Set("structural_link_reviewed_by", "memory_governance")
	fact.Set("structural_link_review_reason", doc.Get("reason"))
	fact.Set("structural_link_review_class", doc.Get("review_class"))
	fact.Set("structural_link_review_artifact", artifactPath)
	fact.Set("structural_link_history", history)
	if err := fact.Save(); err != nil {
		return err
	}

	proofRefs := uniqueStrings(append([]string{factPath, artifactPath}, stringSliceField(doc.Frontmatter["source_refs"])...))
	return ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "maintenance_apply",
		AuthorityScope: ledger.ScopeApprovedMaintenanceApply,
		ActionClass:    ledger.ActionMemoryStructuralLinkTransition,
		TargetDomain:   factPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"memory_structural_links_changed"},
		ProofRefs:      proofRefs,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "maintenance_apply",
			StaffingContext:    doc.Get("review_class"),
			AuthorityScope:     ledger.ScopeApprovedMaintenanceApply,
			ArtifactState:      "canonical",
			SourceRefs:         proofRefs,
			PromotionStatus:    "approved",
			ProofRef:           "memory-structural-link-transition:" + filepath.Base(doc.Path),
		},
		Metadata: map[string]interface{}{
			"fact_path":             factPath,
			"related_fact_count":    len(mergedFactLinks),
			"related_episode_count": len(mergedEpisodeLinks),
			"related_entity_count":  len(mergedEntityLinks),
			"related_mission_count": len(mergedMissionLinks),
			"review_class":          doc.Get("review_class"),
		},
	})
}

func applyFactTemporalTransition(v *vault.Vault, doc *markdown.Document) error {
	factPath := strings.TrimSpace(doc.Get("fact_path"))
	if factPath == "" {
		return fmt.Errorf("fact_path not set")
	}

	fact, err := v.Read(factPath)
	if err != nil {
		return fmt.Errorf("read fact %s: %w", factPath, err)
	}

	current := normalizeTemporalStatus(firstNonEmpty(fact.Get("temporal_status"), "active"))
	target := normalizeTemporalStatus(firstNonEmpty(doc.Get("proposed_temporal_status"), "active"))
	artifactPath := filepathToVaultPath(v.Dir, doc.Path)
	now := time.Now().Format(time.RFC3339)

	historyEntry := fmt.Sprintf("%s %s->%s via %s (%s)", now, current, target, artifactPath, strings.TrimSpace(doc.Get("reason")))
	history := uniqueStrings(append(stringSliceField(fact.Frontmatter["temporal_history"]), historyEntry))

	fact.Set("temporal_status_previous", current)
	fact.Set("temporal_status", target)
	fact.Set("temporal_reviewed_at", now)
	fact.Set("temporal_reviewed_by", "memory_governance")
	fact.Set("temporal_review_reason", doc.Get("reason"))
	fact.Set("temporal_review_class", doc.Get("review_class"))
	fact.Set("temporal_review_artifact", artifactPath)
	fact.Set("temporal_history", history)

	if observedAt := normalizeTimeOrBlank(doc.Get("observed_at")); observedAt != "" {
		fact.Set("observed_at", observedAt)
	}
	if validFrom := normalizeTimeOrBlank(doc.Get("valid_from")); validFrom != "" {
		fact.Set("valid_from", validFrom)
	}
	if validTo := normalizeTimeOrBlank(doc.Get("valid_to")); validTo != "" {
		fact.Set("valid_to", validTo)
	}

	supersededByPath := strings.TrimSpace(doc.Get("superseded_by_path"))
	if target == "superseded" && supersededByPath != "" {
		fact.Set("superseded_by", supersededByPath)
		if strings.TrimSpace(fact.Get("valid_to")) == "" {
			fact.Set("valid_to", firstNonEmpty(
				normalizeTimeOrBlank(doc.Get("valid_to")),
				normalizeTimeOrBlank(doc.Get("valid_from")),
				now,
			))
		}
		if newer, err := v.Read(supersededByPath); err == nil {
			supersedes := uniqueStrings(append(stringSliceField(newer.Frontmatter["supersedes_paths"]), factPath))
			newer.Set("supersedes_paths", supersedes)
			if err := newer.Save(); err != nil {
				return err
			}
		}
	}

	if err := fact.Save(); err != nil {
		return err
	}

	return ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "maintenance_apply",
		AuthorityScope: ledger.ScopeApprovedMaintenanceApply,
		ActionClass:    ledger.ActionMemoryTemporalTransition,
		TargetDomain:   factPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"memory_temporal_state_changed"},
		ProofRefs:      []string{factPath, artifactPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "maintenance_apply",
			StaffingContext:    doc.Get("review_class"),
			AuthorityScope:     ledger.ScopeApprovedMaintenanceApply,
			ArtifactState:      "canonical",
			SourceRefs:         []string{factPath, artifactPath},
			PromotionStatus:    "approved",
			ProofRef:           "memory-temporal-transition:" + filepath.Base(doc.Path),
		},
		Metadata: map[string]interface{}{
			"fact_path":                factPath,
			"current_temporal_status":  current,
			"proposed_temporal_status": target,
			"superseded_by_path":       supersededByPath,
			"review_class":             doc.Get("review_class"),
		},
	})
}

func applyElderMemoryTransition(v *vault.Vault, doc *markdown.Document) error {
	factPath := strings.TrimSpace(doc.Get("fact_path"))
	if factPath == "" {
		return fmt.Errorf("fact_path not set")
	}

	fact, err := v.Read(factPath)
	if err != nil {
		return fmt.Errorf("read fact %s: %w", factPath, err)
	}

	current := normalizeProtectionClass(fact.Get("memory_protection_class"))
	target := normalizeProtectionClass(doc.Get("proposed_protection_class"))
	if current == target {
		return nil
	}

	now := time.Now().Format(time.RFC3339)
	historyEntry := fmt.Sprintf("%s %s->%s via %s (%s)", now, current, target, filepathToVaultPath(v.Dir, doc.Path), strings.TrimSpace(doc.Get("reason")))
	history := uniqueStrings(append(stringSliceField(fact.Frontmatter["memory_protection_history"]), historyEntry))

	fact.Set("memory_protection_class_previous", current)
	fact.Set("memory_protection_class", target)
	fact.Set("memory_protection_reviewed_at", now)
	fact.Set("memory_protection_reviewed_by", "memory_governance")
	fact.Set("memory_protection_review_reason", doc.Get("reason"))
	fact.Set("memory_protection_review_class", doc.Get("review_class"))
	fact.Set("memory_protection_review_artifact", filepathToVaultPath(v.Dir, doc.Path))
	fact.Set("memory_protection_history", history)
	if err := fact.Save(); err != nil {
		return err
	}

	return ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "maintenance_apply",
		AuthorityScope: ledger.ScopeApprovedMaintenanceApply,
		ActionClass:    ledger.ActionMemoryProtectionTransition,
		TargetDomain:   factPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"memory_protection_changed"},
		ProofRefs:      []string{factPath, filepathToVaultPath(v.Dir, doc.Path)},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "maintenance_apply",
			StaffingContext:    doc.Get("review_class"),
			AuthorityScope:     ledger.ScopeApprovedMaintenanceApply,
			ArtifactState:      "canonical",
			SourceRefs:         []string{factPath, filepathToVaultPath(v.Dir, doc.Path)},
			PromotionStatus:    "approved",
			ProofRef:           "elder-memory-apply:" + filepath.Base(doc.Path),
		},
		Metadata: map[string]interface{}{
			"fact_path":                 factPath,
			"current_protection_class":  current,
			"proposed_protection_class": target,
			"review_class":              doc.Get("review_class"),
		},
	})
}

// FormatApplyResult renders an apply result as human-readable text.
func FormatApplyResult(r *ApplyResult) string {
	total := r.MergesApplied + r.ContradictionsResolved + r.BootstrapPromoted + r.ReplayPromoted + r.HotTransitionsApplied + r.StructuralTransitionsApplied + r.TemporalTransitionsApplied + r.ElderTransitionsApplied
	if total == 0 {
		return "No approved maintenance artifacts found. Set `status: approved` on review artifacts first."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Applied %d maintenance artifacts:\n", total))
	sb.WriteString(fmt.Sprintf("- Merges applied: %d\n", r.MergesApplied))
	sb.WriteString(fmt.Sprintf("- Contradictions resolved: %d\n", r.ContradictionsResolved))
	sb.WriteString(fmt.Sprintf("- Bootstrap facts promoted: %d\n", r.BootstrapPromoted))
	sb.WriteString(fmt.Sprintf("- Replay facts promoted: %d\n", r.ReplayPromoted))
	sb.WriteString(fmt.Sprintf("- Hot memory transitions applied: %d\n", r.HotTransitionsApplied))
	sb.WriteString(fmt.Sprintf("- Structural link transitions applied: %d\n", r.StructuralTransitionsApplied))
	sb.WriteString(fmt.Sprintf("- Temporal transitions applied: %d\n", r.TemporalTransitionsApplied))
	sb.WriteString(fmt.Sprintf("- Elder memory transitions applied: %d\n", r.ElderTransitionsApplied))

	if len(r.Actions) > 0 {
		sb.WriteString("\nDetails:\n")
		for _, a := range r.Actions {
			sb.WriteString(fmt.Sprintf("  - %s\n", a))
		}
	}
	return sb.String()
}

func countTransitionField(value interface{}) string {
	return fmt.Sprintf("%d", len(stringSliceField(value)))
}

func stringSliceField(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return append([]string{}, v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

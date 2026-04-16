package vault

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/signature"
)

// RecallRequest describes a harness-level retrieval attempt. Unlike plain
// search, a recall request is durable proof that memory was consulted under a
// specific mode, through a specific adapter, with a bounded result set.
type RecallRequest struct {
	Query              string
	Limit              int
	Options            FactSearchOptions
	Harness            string
	Adapter            string
	Mode               string
	ProducingOffice    string
	ProducingSubsystem string
	StaffingContext    string
	WorkItemID         string
}

// RecallResult is the durable result of a recall operation, including the
// human-usable lines returned to the caller and the receipt artifact path.
type RecallResult struct {
	RecallID           string
	ReceiptPath        string
	Lines              []string
	ResultPaths        []string
	Verification       []FactVerificationResult
	LinkedFactPaths    []string
	LinkedEpisodePaths []string
	LinkedEntityRefs   []string
	LinkedMissionRefs  []string
}

func newRecallID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return fmt.Sprintf("recall-%d", time.Now().UTC().UnixNano())
}

func uniqueSorted(values []string) []string {
	values = dedupeNonEmpty(values...)
	if len(values) <= 1 {
		return values
	}
	sorted := append([]string(nil), values...)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

func uniqueOrdered(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
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

func excludeRefs(values []string, excluded []string) []string {
	if len(values) == 0 {
		return nil
	}
	blocked := make(map[string]bool, len(excluded))
	for _, value := range excluded {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		blocked[value] = true
	}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || blocked[value] {
			continue
		}
		out = append(out, value)
	}
	return uniqueSorted(out)
}

func (v *Vault) RecallFacts(req RecallRequest) (RecallResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return RecallResult{}, fmt.Errorf("empty recall query")
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	plan := v.buildRecallRoutePlan(query, req.Options)

	hits, err := v.rankedFactHits(query, req.Limit, req.Options)
	if err != nil {
		return RecallResult{}, err
	}

	var (
		lines                    []string
		resultPaths              []string
		linkedFactPaths          []string
		linkedEpisodePaths       []string
		linkedEntityRefs         []string
		linkedMissionRefs        []string
		sourceEventIDs           []string
		lineageIDs               []string
		cueTerms                 []string
		securityClass            = "operational"
		verificationMode         = normalizeVerificationMode(req.Options.VerificationMode)
		verificationResults      []FactVerificationResult
		verifiedPaths            []string
		reviewRequiredPaths      []string
		mismatchPaths            []string
		unverifiedPaths          []string
		sourceMissingPaths       []string
		verificationReviewedRefs []string
	)
	for _, hit := range hits {
		line := formatFactSearchHit(hit)
		if verificationMode != "" {
			verification := v.verifyFactHit(hit.RelPath, hit.Doc, verificationMode)
			verificationResults = append(verificationResults, verification)
			verificationReviewedRefs = append(verificationReviewedRefs, verification.ReviewedSourceRefs...)
			line = appendVerificationAnnotation(line, verification)
			switch verification.Status {
			case VerificationStatusVerified:
				verifiedPaths = append(verifiedPaths, hit.RelPath)
			case VerificationStatusReviewRequired:
				reviewRequiredPaths = append(reviewRequiredPaths, hit.RelPath)
			case VerificationStatusMismatch:
				mismatchPaths = append(mismatchPaths, hit.RelPath)
			case VerificationStatusUnverified:
				unverifiedPaths = append(unverifiedPaths, hit.RelPath)
			case VerificationStatusSourceMissing:
				sourceMissingPaths = append(sourceMissingPaths, hit.RelPath)
			}
		}
		lines = append(lines, line)
		resultPaths = append(resultPaths, hit.RelPath)
		linkedFactPaths = append(linkedFactPaths, docRelatedFactPaths(hit.Doc)...)
		linkedEpisodePaths = append(linkedEpisodePaths, docRelatedEpisodePaths(hit.Doc)...)
		linkedEntityRefs = append(linkedEntityRefs, docRelatedEntityRefs(hit.Doc)...)
		linkedMissionRefs = append(linkedMissionRefs, docRelatedMissionRefs(hit.Doc)...)
		sourceEventIDs = append(sourceEventIDs, hit.Doc.Get("source_event_id"))
		lineageIDs = append(lineageIDs, hit.Doc.Get("lineage_id"))
		cueTerms = append(cueTerms, stringSliceFrontmatter(hit.Doc.Frontmatter["cue_terms"])...)
		if memorySecurityRank(effectiveFactSecurityClass(hit.Doc)) > memorySecurityRank(securityClass) {
			securityClass = effectiveFactSecurityClass(hit.Doc)
		}
		_ = v.ReinforceFact(hit.RelPath)
	}

	recallID := newRecallID()
	now := time.Now().UTC()
	relPath := filepath.ToSlash(filepath.Join("memory", "recalls", now.Format("2006-01-02"), recallID+".md"))
	resultPaths = uniqueOrdered(resultPaths)
	linkedFactPaths = excludeRefs(linkedFactPaths, resultPaths)
	linkedEpisodePaths = uniqueSorted(linkedEpisodePaths)
	linkedEntityRefs = uniqueSorted(linkedEntityRefs)
	linkedMissionRefs = uniqueSorted(linkedMissionRefs)
	sourceEventIDs = uniqueSorted(sourceEventIDs)
	lineageIDs = uniqueSorted(lineageIDs)
	cueTerms = uniqueSorted(normalizeCueTerms(cueTerms))

	fm := map[string]interface{}{
		"type":                  "memory_recall_receipt",
		"recall_id":             recallID,
		"created":               now.Format(time.RFC3339),
		"created_at":            now.Format(time.RFC3339),
		"query":                 query,
		"result_count":          len(lines),
		"requested_limit":       req.Limit,
		"harness":               firstNonEmpty(strings.TrimSpace(req.Harness), "memorykit"),
		"adapter":               firstNonEmpty(strings.TrimSpace(req.Adapter), "kernel"),
		"mode":                  firstNonEmpty(strings.TrimSpace(req.Mode), "manual_search"),
		"selected_paths":        resultPaths,
		"memory_security_class": normalizeMemorySecurityClass(securityClass),
	}
	if temperature := strings.TrimSpace(req.Options.MemoryTemperature); temperature != "" {
		fm["memory_temperature_filter"] = normalizeMemoryTemperature(temperature)
	}
	if verificationMode != "" {
		var verificationEntries []map[string]interface{}
		for _, verification := range verificationResults {
			verificationEntries = append(verificationEntries, verificationFrontmatterMap(verification))
		}
		fm["verification_mode"] = verificationMode
		fm["verification_results"] = verificationEntries
		if len(verifiedPaths) > 0 {
			fm["verification_verified_paths"] = uniqueSorted(verifiedPaths)
		}
		if len(reviewRequiredPaths) > 0 {
			fm["verification_review_required_paths"] = uniqueSorted(reviewRequiredPaths)
		}
		if len(mismatchPaths) > 0 {
			fm["verification_mismatch_paths"] = uniqueSorted(mismatchPaths)
		}
		if len(unverifiedPaths) > 0 {
			fm["verification_unverified_paths"] = uniqueSorted(unverifiedPaths)
		}
		if len(sourceMissingPaths) > 0 {
			fm["verification_source_missing_paths"] = uniqueSorted(sourceMissingPaths)
		}
	}
	if len(sourceEventIDs) > 0 {
		fm["source_event_ids"] = sourceEventIDs
	}
	if len(lineageIDs) > 0 {
		fm["lineage_ids"] = lineageIDs
	}
	if len(cueTerms) > 0 {
		fm["cue_terms"] = cueTerms
	}
	if len(linkedFactPaths) > 0 {
		fm["linked_fact_paths"] = linkedFactPaths
	}
	if len(linkedEpisodePaths) > 0 {
		fm["linked_episode_paths"] = linkedEpisodePaths
	}
	if len(linkedEntityRefs) > 0 {
		fm["linked_entity_refs"] = linkedEntityRefs
	}
	if len(linkedMissionRefs) > 0 {
		fm["linked_mission_refs"] = linkedMissionRefs
	}
	if len(plan.Subjects) > 0 {
		fm["route_subjects"] = plan.Subjects
	}
	if len(plan.Missions) > 0 {
		fm["route_missions"] = plan.Missions
	}
	if plan.Office != "" {
		fm["route_office"] = plan.Office
	}
	if plan.WorkItemID != "" {
		fm["route_work_item_id"] = plan.WorkItemID
	}
	if plan.LineageID != "" {
		fm["route_lineage_id"] = plan.LineageID
	}
	if plan.Environment != "" {
		fm["route_environment"] = plan.Environment
	}
	if len(plan.CueTerms) > 0 {
		fm["route_cue_terms"] = plan.CueTerms
	}
	if plan.TimeBand != "" {
		fm["route_time_band"] = plan.TimeBand
	}
	if strings.TrimSpace(req.WorkItemID) != "" {
		fm["work_item_id"] = strings.TrimSpace(req.WorkItemID)
	}

	sourceRefs := append([]string{relPath}, resultPaths...)
	fm["producing_signature"] = signature.Signature{
		ProducingOffice:    firstNonEmpty(strings.TrimSpace(req.ProducingOffice), "librarian"),
		ProducingSubsystem: firstNonEmpty(strings.TrimSpace(req.ProducingSubsystem), "memory_recall"),
		StaffingContext:    strings.TrimSpace(req.StaffingContext),
		AuthorityScope:     ledger.ScopeRuntimeMemorySearch,
		ArtifactState:      "derived",
		SourceRefs:         uniqueSorted(sourceRefs),
		PromotionStatus:    "observed",
		ProofRef:           "memory-recall:" + recallID,
	}.EnsureTimestamp()

	var body strings.Builder
	body.WriteString("# Memory Recall Receipt\n\n")
	body.WriteString(fmt.Sprintf("Query: `%s`\n\n", query))
	body.WriteString(fmt.Sprintf("Harness: `%s`\n", fm["harness"]))
	body.WriteString(fmt.Sprintf("Adapter: `%s`\n", fm["adapter"]))
	body.WriteString(fmt.Sprintf("Mode: `%s`\n", fm["mode"]))
	if len(plan.Subjects) > 0 || len(plan.Missions) > 0 || plan.Office != "" || plan.WorkItemID != "" || plan.LineageID != "" || plan.Environment != "" || len(plan.CueTerms) > 0 || plan.TimeBand != "" {
		body.WriteString("Route plan:\n")
		if len(plan.Subjects) > 0 {
			body.WriteString(fmt.Sprintf("- subjects: `%s`\n", strings.Join(plan.Subjects, "`, `")))
		}
		if len(plan.Missions) > 0 {
			body.WriteString(fmt.Sprintf("- missions: `%s`\n", strings.Join(plan.Missions, "`, `")))
		}
		if plan.Office != "" {
			body.WriteString(fmt.Sprintf("- office: `%s`\n", plan.Office))
		}
		if plan.WorkItemID != "" {
			body.WriteString(fmt.Sprintf("- work item: `%s`\n", plan.WorkItemID))
		}
		if plan.LineageID != "" {
			body.WriteString(fmt.Sprintf("- lineage: `%s`\n", plan.LineageID))
		}
		if plan.Environment != "" {
			body.WriteString(fmt.Sprintf("- environment: `%s`\n", plan.Environment))
		}
		if len(plan.CueTerms) > 0 {
			body.WriteString(fmt.Sprintf("- cue terms: `%s`\n", strings.Join(plan.CueTerms, "`, `")))
		}
		if plan.TimeBand != "" {
			body.WriteString(fmt.Sprintf("- time band: `%s`\n", plan.TimeBand))
		}
	}
	if strings.TrimSpace(req.Options.MemoryTemperature) != "" {
		body.WriteString(fmt.Sprintf("Temperature filter: `%s`\n", normalizeMemoryTemperature(req.Options.MemoryTemperature)))
	}
	body.WriteString(fmt.Sprintf("Result count: `%d`\n\n", len(lines)))
	if len(resultPaths) == 0 {
		body.WriteString("No memory facts matched this recall.\n")
	} else {
		body.WriteString("## Selected Facts\n\n")
		for i, hit := range hits {
			body.WriteString(fmt.Sprintf("- `%s`\n", hit.RelPath))
			line := strings.TrimSpace(formatFactSearchHit(hit))
			if verificationMode != "" && i < len(verificationResults) {
				line = strings.TrimSpace(appendVerificationAnnotation(line, verificationResults[i]))
			}
			body.WriteString(fmt.Sprintf("  %s\n", line))
		}
		if verificationMode != "" {
			body.WriteString("\n## Verification\n\n")
			for _, verification := range verificationResults {
				body.WriteString(fmt.Sprintf("- `%s` → `%s`\n", verification.FactPath, verification.Status))
				if strings.TrimSpace(verification.Note) != "" {
					body.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(verification.Note)))
				}
				if len(verification.ReviewedSourceRefs) > 0 {
					body.WriteString(fmt.Sprintf("  reviewed sources: `%s`\n", strings.Join(verification.ReviewedSourceRefs, "`, `")))
				}
			}
		}
		if len(linkedFactPaths) > 0 || len(linkedEpisodePaths) > 0 || len(linkedEntityRefs) > 0 || len(linkedMissionRefs) > 0 {
			body.WriteString("\n## Structural Links\n\n")
			if len(linkedFactPaths) > 0 {
				body.WriteString(fmt.Sprintf("- linked facts: `%s`\n", strings.Join(linkedFactPaths, "`, `")))
			}
			if len(linkedEpisodePaths) > 0 {
				body.WriteString(fmt.Sprintf("- linked episodes: `%s`\n", strings.Join(linkedEpisodePaths, "`, `")))
			}
			if len(linkedEntityRefs) > 0 {
				body.WriteString(fmt.Sprintf("- linked entities: `%s`\n", strings.Join(linkedEntityRefs, "`, `")))
			}
			if len(linkedMissionRefs) > 0 {
				body.WriteString(fmt.Sprintf("- linked missions: `%s`\n", strings.Join(linkedMissionRefs, "`, `")))
			}
		}
	}

	if err := v.Write(relPath, fm, body.String()); err != nil {
		return RecallResult{}, err
	}

	_ = ledger.Append(v.Dir, ledger.Record{
		Office:         firstNonEmpty(strings.TrimSpace(req.ProducingOffice), "librarian"),
		Subsystem:      firstNonEmpty(strings.TrimSpace(req.ProducingSubsystem), "memory_recall"),
		AuthorityScope: ledger.ScopeRuntimeMemorySearch,
		ActionClass:    ledger.ActionMemoryRecall,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultCompleted,
		Decision:       ledger.DecisionAllowedWithProof,
		ProofRefs:      uniqueSorted(sourceRefs),
		Signature: signature.Signature{
			ProducingOffice:    firstNonEmpty(strings.TrimSpace(req.ProducingOffice), "librarian"),
			ProducingSubsystem: firstNonEmpty(strings.TrimSpace(req.ProducingSubsystem), "memory_recall"),
			StaffingContext:    strings.TrimSpace(req.StaffingContext),
			AuthorityScope:     ledger.ScopeRuntimeMemorySearch,
			ArtifactState:      "derived",
			SourceRefs:         uniqueSorted(sourceRefs),
			PromotionStatus:    "observed",
			ProofRef:           "memory-recall:" + recallID,
		}.EnsureTimestamp(),
		Metadata: map[string]interface{}{
			"recall_id":                    recallID,
			"query":                        query,
			"harness":                      fm["harness"],
			"adapter":                      fm["adapter"],
			"mode":                         fm["mode"],
			"result_count":                 len(lines),
			"memory_temperature_filter":    fm["memory_temperature_filter"],
			"route_subjects":               plan.Subjects,
			"route_missions":               plan.Missions,
			"route_office":                 plan.Office,
			"route_work_item_id":           plan.WorkItemID,
			"route_lineage_id":             plan.LineageID,
			"route_environment":            plan.Environment,
			"route_cue_terms":              plan.CueTerms,
			"route_time_band":              plan.TimeBand,
			"work_item_id":                 strings.TrimSpace(req.WorkItemID),
			"verification_mode":            verificationMode,
			"verification_verified":        len(verifiedPaths),
			"verification_reviewed":        len(uniqueSorted(verificationReviewedRefs)),
			"verification_review_required": len(reviewRequiredPaths),
			"verification_mismatch":        len(mismatchPaths),
			"verification_unverified":      len(unverifiedPaths),
			"verification_source_missing":  len(sourceMissingPaths),
			"linked_fact_count":            len(linkedFactPaths),
			"linked_episode_count":         len(linkedEpisodePaths),
			"linked_entity_count":          len(linkedEntityRefs),
			"linked_mission_count":         len(linkedMissionRefs),
		},
	})

	if verificationMode != "" {
		verificationProofRefs := uniqueSorted(append([]string{relPath}, append(resultPaths, verificationReviewedRefs...)...))
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         firstNonEmpty(strings.TrimSpace(req.ProducingOffice), "librarian"),
			Subsystem:      "memory_recall_verification",
			AuthorityScope: ledger.ScopeRuntimeMemoryVerification,
			ActionClass:    ledger.ActionMemoryVerification,
			TargetDomain:   relPath,
			ResultStatus:   ledger.ResultCompleted,
			Decision:       ledger.DecisionAllowedWithProof,
			ProofRefs:      verificationProofRefs,
			Signature: signature.Signature{
				ProducingOffice:    firstNonEmpty(strings.TrimSpace(req.ProducingOffice), "librarian"),
				ProducingSubsystem: "memory_recall_verification",
				StaffingContext:    strings.TrimSpace(req.StaffingContext),
				AuthorityScope:     ledger.ScopeRuntimeMemoryVerification,
				ArtifactState:      "derived",
				SourceRefs:         verificationProofRefs,
				PromotionStatus:    "observed",
				ProofRef:           "memory-verify:" + recallID,
			}.EnsureTimestamp(),
			Metadata: map[string]interface{}{
				"recall_id":             recallID,
				"verification_mode":     verificationMode,
				"verified_count":        len(verifiedPaths),
				"review_required_count": len(reviewRequiredPaths),
				"mismatch_count":        len(mismatchPaths),
				"unverified_count":      len(unverifiedPaths),
				"source_missing_count":  len(sourceMissingPaths),
				"reviewed_source_count": len(uniqueSorted(verificationReviewedRefs)),
				"work_item_id":          strings.TrimSpace(req.WorkItemID),
			},
		})
	}

	return RecallResult{
		RecallID:           recallID,
		ReceiptPath:        relPath,
		Lines:              lines,
		ResultPaths:        resultPaths,
		Verification:       verificationResults,
		LinkedFactPaths:    linkedFactPaths,
		LinkedEpisodePaths: linkedEpisodePaths,
		LinkedEntityRefs:   linkedEntityRefs,
		LinkedMissionRefs:  linkedMissionRefs,
	}, nil
}

func stringSliceFrontmatter(value interface{}) []string {
	switch raw := value.(type) {
	case []string:
		return dedupeNonEmpty(raw...)
	case []interface{}:
		var out []string
		for _, item := range raw {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return dedupeNonEmpty(out...)
	default:
		return nil
	}
}

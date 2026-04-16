package memorykit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/maintain"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

type EvaluationCaseResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Passed      bool     `json:"passed"`
	Score       float64  `json:"score"`
	Evidence    []string `json:"evidence,omitempty"`
}

type EvaluationReport struct {
	Version      int                    `json:"version"`
	Suite        string                 `json:"suite"`
	GeneratedAt  string                 `json:"generated_at"`
	TotalCases   int                    `json:"total_cases"`
	PassedCases  int                    `json:"passed_cases"`
	OverallScore float64                `json:"overall_score"`
	Cases        []EvaluationCaseResult `json:"cases"`
	Signature    signature.Signature    `json:"signature"`
}

type EvaluationReportResult struct {
	ReportPath   string
	MarkdownPath string
	Report       EvaluationReport
}

type evaluationCase struct {
	ID          string
	Name        string
	Description string
	Run         func() (EvaluationCaseResult, error)
}

func (k *Kernel) Evaluate() (EvaluationReportResult, error) {
	cases := []evaluationCase{
		{
			ID:          "interference_recall_precision",
			Name:        "Interference Recall Precision",
			Description: "Route selectors should pull the correct fact to the top even when similarly worded memories compete.",
			Run:         runInterferenceRecallCase,
		},
		{
			ID:          "elder_retention",
			Name:        "Elder Retention",
			Description: "Rare, old, high-consequence memory should survive decay and archival pressure and remain retrievable.",
			Run:         runElderRetentionCase,
		},
		{
			ID:          "replay_promotion_accuracy",
			Name:        "Replay Promotion Accuracy",
			Description: "Repeated episodic evidence plus recall traces should yield one correct replay candidate and a provenance-bearing promoted fact.",
			Run:         runReplayPromotionCase,
		},
		{
			ID:          "hot_tier_stale_detection",
			Name:        "Hot-Tier Stale Detection",
			Description: "Stale hot facts should become explicit review candidates rather than silently cooling or persisting forever.",
			Run:         runHotTierStaleDetectionCase,
		},
		{
			ID:          "secure_state_tamper_detection",
			Name:        "Secure-State Tamper Detection",
			Description: "Manifest verification should detect post-manifest drift in the sovereign memory estate.",
			Run:         runSecureStateTamperCase,
		},
		{
			ID:          "secure_state_rollback_detection",
			Name:        "Secure-State Rollback Detection",
			Description: "Manifest verification should suspect rollback when the latest manifest is older than the newest manifest root recorded in the ledger.",
			Run:         runSecureStateRollbackCase,
		},
	}

	results := make([]EvaluationCaseResult, 0, len(cases))
	passed := 0
	scoreTotal := 0.0
	for _, c := range cases {
		result, err := c.Run()
		if err != nil {
			result = EvaluationCaseResult{
				ID:          c.ID,
				Name:        c.Name,
				Description: c.Description,
				Passed:      false,
				Score:       0,
				Evidence:    []string{fmt.Sprintf("evaluation error: %v", err)},
			}
		}
		if result.ID == "" {
			result.ID = c.ID
		}
		if result.Name == "" {
			result.Name = c.Name
		}
		if result.Description == "" {
			result.Description = c.Description
		}
		if result.Passed {
			passed++
		}
		scoreTotal += clampEvalScore(result.Score)
		results = append(results, result)
	}

	overall := 0.0
	if len(results) > 0 {
		overall = scoreTotal / float64(len(results))
	}
	report := EvaluationReport{
		Version:      1,
		Suite:        "grade_s_phase8",
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		TotalCases:   len(results),
		PassedCases:  passed,
		OverallScore: overall,
		Cases:        results,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_evaluation",
			StaffingContext:    "phase8_synthetic_fixture",
			AuthorityScope:     ledger.ScopeRuntimeMemoryEvaluation,
			ArtifactState:      "derived",
			SourceRefs:         []string{"state/memory/evaluations/latest.json", "state/memory/evaluations/latest.md"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-evaluation:grade-s-phase8",
		}.EnsureTimestamp(),
	}

	jsonRelPath := filepath.ToSlash(filepath.Join("state", "memory", "evaluations", "latest.json"))
	mdRelPath := filepath.ToSlash(filepath.Join("state", "memory", "evaluations", "latest.md"))
	jsonAbsPath := filepath.Join(k.Vault.Dir, jsonRelPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, mdRelPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return EvaluationReportResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return EvaluationReportResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return EvaluationReportResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_evaluation_report",
		"suite":               report.Suite,
		"generated_at":        report.GeneratedAt,
		"overall_score":       report.OverallScore,
		"passed_cases":        report.PassedCases,
		"total_cases":         report.TotalCases,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Evaluation Report\n\n")
	body.WriteString(fmt.Sprintf("Suite: `%s`\n\n", report.Suite))
	body.WriteString(fmt.Sprintf("Overall score: `%.2f`\n\n", report.OverallScore))
	body.WriteString(fmt.Sprintf("Passed cases: `%d/%d`\n\n", report.PassedCases, report.TotalCases))
	body.WriteString("This report is a synthetic fixture evaluation over the live memory kernel, retrieval, replay maintenance, hot-tier governance, and secure-state verification code paths. It does not grade provider quality or rhetorical charm.\n\n")
	body.WriteString("## Cases\n\n")
	for _, result := range report.Cases {
		status := "fail"
		if result.Passed {
			status = "pass"
		}
		body.WriteString(fmt.Sprintf("### %s\n\n", result.Name))
		body.WriteString(fmt.Sprintf("Status: `%s`\n\n", status))
		body.WriteString(fmt.Sprintf("Score: `%.2f`\n\n", clampEvalScore(result.Score)))
		body.WriteString(result.Description)
		body.WriteString("\n\n")
		if len(result.Evidence) > 0 {
			body.WriteString("Evidence:\n")
			for _, line := range result.Evidence {
				body.WriteString(fmt.Sprintf("- %s\n", line))
			}
			body.WriteString("\n")
		}
	}
	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return EvaluationReportResult{}, err
	}

	status := ledger.ResultCompleted
	if report.PassedCases != report.TotalCases {
		status = ledger.ResultFailed
	}
	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_evaluation",
		AuthorityScope: ledger.ScopeRuntimeMemoryEvaluation,
		ActionClass:    ledger.ActionMemoryEvaluation,
		TargetDomain:   jsonRelPath,
		ResultStatus:   status,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_evaluation_written"},
		ProofRefs:      []string{jsonRelPath, mdRelPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_evaluation",
			StaffingContext:    "phase8_synthetic_fixture",
			AuthorityScope:     ledger.ScopeRuntimeMemoryEvaluation,
			ArtifactState:      "derived",
			SourceRefs:         []string{jsonRelPath, mdRelPath},
			PromotionStatus:    "observed",
			ProofRef:           "memory-evaluation:grade-s-phase8",
		}.EnsureTimestamp(),
		Metadata: map[string]interface{}{
			"suite":         report.Suite,
			"overall_score": report.OverallScore,
			"passed_cases":  report.PassedCases,
			"total_cases":   report.TotalCases,
			"case_ids":      evaluationCaseIDs(report.Cases),
		},
	})

	return EvaluationReportResult{
		ReportPath:   jsonRelPath,
		MarkdownPath: mdRelPath,
		Report:       report,
	}, nil
}

func clampEvalScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func evaluationCaseIDs(cases []EvaluationCaseResult) []string {
	ids := make([]string, 0, len(cases))
	for _, result := range cases {
		ids = append(ids, result.ID)
	}
	return ids
}

func newEvaluationFixture() (*Kernel, func(), error) {
	dir, err := os.MkdirTemp("", "modus-memory-eval-*")
	if err != nil {
		return nil, nil, err
	}
	return New(vault.New(dir, nil)), func() {
		_ = os.RemoveAll(dir)
	}, nil
}

func evalFactAuthority(office string) vault.FactWriteAuthority {
	return vault.FactWriteAuthority{
		ProducingOffice:    office,
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_fixture",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
	}
}

func evalEpisodeAuthority(office string) vault.EpisodeWriteAuthority {
	return vault.EpisodeWriteAuthority{
		ProducingOffice:    office,
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_fixture",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/episodes",
		AllowApproval:      true,
	}
}

func runInterferenceRecallCase() (EvaluationCaseResult, error) {
	k, cleanup, err := newEvaluationFixture()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	defer cleanup()

	targetMission, err := k.StoreFact("Bridge status", "finding", "Memory route degraded", 0.92, "high", vault.FactWriteAuthority{
		ProducingOffice:    "librarian",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_interference",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		Mission:            "Memory Sovereignty",
		WorkItemID:         "wk-memory",
		Environment:        "lab",
		CueTerms:           []string{"bridge", "memory"},
		AllowApproval:      true,
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	targetHomeFront, err := k.StoreFact("Bridge status", "finding", "HomeFront route degraded", 0.92, "high", vault.FactWriteAuthority{
		ProducingOffice:    "scout",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_interference",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		Mission:            "HomeFront Solo",
		WorkItemID:         "wk-homefront",
		Environment:        "field",
		CueTerms:           []string{"bridge", "homefront"},
		AllowApproval:      true,
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	targetInspector, err := k.StoreFact("Bridge status", "finding", "WRAITH route delayed", 0.92, "high", vault.FactWriteAuthority{
		ProducingOffice:    "inspector",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_interference",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		Mission:            "WRAITH",
		WorkItemID:         "wk-wraith",
		Environment:        "ops",
		CueTerms:           []string{"bridge", "wraith"},
		AllowApproval:      true,
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}

	queries := []struct {
		Label string
		Opts  vault.FactSearchOptions
		Want  string
	}{
		{
			Label: "mission selector",
			Opts:  vault.FactSearchOptions{RouteMission: "Memory Sovereignty"},
			Want:  targetMission,
		},
		{
			Label: "work item selector",
			Opts:  vault.FactSearchOptions{WorkItemID: "wk-homefront"},
			Want:  targetHomeFront,
		},
		{
			Label: "office selector",
			Opts:  vault.FactSearchOptions{CapturedByOffice: "inspector"},
			Want:  targetInspector,
		},
	}

	correct := 0
	evidence := make([]string, 0, len(queries))
	for _, query := range queries {
		recall, err := k.Recall(RecallRequest{
			Query:              "bridge status finding",
			Limit:              1,
			Options:            query.Opts,
			Harness:            "memorykit_eval",
			Adapter:            "kernel",
			Mode:               "evaluation_interference",
			ProducingOffice:    "librarian",
			ProducingSubsystem: "memory_evaluation",
			StaffingContext:    "phase8_interference",
		})
		if err != nil {
			evidence = append(evidence, fmt.Sprintf("%s: recall error %v", query.Label, err))
			continue
		}
		got := ""
		if len(recall.ResultPaths) > 0 {
			got = recall.ResultPaths[0]
		}
		if got == query.Want {
			correct++
		}
		evidence = append(evidence, fmt.Sprintf("%s: got `%s` want `%s`", query.Label, got, query.Want))
	}

	score := float64(correct) / float64(len(queries))
	return EvaluationCaseResult{
		ID:          "interference_recall_precision",
		Name:        "Interference Recall Precision",
		Description: "Hierarchical route selectors should disambiguate near-identical memories by mission, work item, and office.",
		Passed:      correct == len(queries),
		Score:       score,
		Evidence:    evidence,
	}, nil
}

func runElderRetentionCase() (EvaluationCaseResult, error) {
	k, cleanup, err := newEvaluationFixture()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	defer cleanup()

	elderPath, err := k.StoreFact("Founding law", "requires", "provenance before cleanup", 0.72, "high", vault.FactWriteAuthority{
		ProducingOffice:       "memory_governance",
		ProducingSubsystem:    "memory_evaluation",
		StaffingContext:       "phase8_elder",
		AuthorityScope:        ledger.ScopeOperatorMemoryStore,
		TargetDomain:          "memory/facts",
		MemoryProtectionClass: "elder",
		AllowApproval:         true,
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	if _, err := k.StoreFact("Founding law", "requires", "cleanup before polish", 0.76, "medium", vault.FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_elder",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
	}); err != nil {
		return EvaluationCaseResult{}, err
	}

	elderDoc, err := k.Vault.Read(elderPath)
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	old := time.Now().AddDate(-2, 0, 0).UTC().Format(time.RFC3339)
	elderDoc.Set("created_at", old)
	elderDoc.Set("created", old)
	elderDoc.Set("last_accessed", old)
	elderDoc.Set("confidence", 0.08)
	if err := elderDoc.Save(); err != nil {
		return EvaluationCaseResult{}, err
	}

	if _, err := k.Vault.DecayFacts(); err != nil {
		return EvaluationCaseResult{}, err
	}
	if _, err := k.Vault.ArchiveStaleFacts(0.1); err != nil {
		return EvaluationCaseResult{}, err
	}

	elderDoc, err = k.Vault.Read(elderPath)
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	survived := elderDoc.Get("archived") != "true"

	recall, err := k.Recall(RecallRequest{
		Query:              "founding law provenance",
		Limit:              1,
		Harness:            "memorykit_eval",
		Adapter:            "kernel",
		Mode:               "evaluation_elder_retention",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_elder",
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	recalled := len(recall.ResultPaths) > 0 && recall.ResultPaths[0] == elderPath

	score := 0.0
	if survived {
		score += 0.5
	}
	if recalled {
		score += 0.5
	}
	return EvaluationCaseResult{
		ID:          "elder_retention",
		Name:        "Elder Retention",
		Description: "Elder-protected memory should survive decay and archival pressure and still come back when queried.",
		Passed:      survived && recalled,
		Score:       score,
		Evidence: []string{
			fmt.Sprintf("elder fact path `%s` archived=%t", elderPath, !survived),
			fmt.Sprintf("top recall path `%s`", firstResultPath(recall.ResultPaths)),
		},
	}, nil
}

func runReplayPromotionCase() (EvaluationCaseResult, error) {
	k, cleanup, err := newEvaluationFixture()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	defer cleanup()

	_, _, err = k.StoreEpisode("MODUS uses Go for the agent framework.", vault.EpisodeWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_replay",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/episodes",
		EventID:            "evt-test",
		LineageID:          "lin-test",
		EventKind:          "observation",
		Subject:            "MODUS",
		Mission:            "Memory Sovereignty",
		WorkItemID:         "work-memory",
		Environment:        "operator-shell",
		CueTerms:           []string{"modus", "go", "agent"},
		AllowApproval:      true,
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}

	recallDate := time.Now().Format("2006-01-02")
	for idx := 1; idx <= 2; idx++ {
		relPath := filepath.ToSlash(filepath.Join("memory", "recalls", recallDate, fmt.Sprintf("recall-%d.md", idx)))
		if err := k.Vault.Write(relPath, map[string]interface{}{
			"type":               "memory_recall_receipt",
			"source_event_ids":   []string{"evt-test"},
			"lineage_ids":        []string{"lin-test"},
			"route_missions":     []string{"Memory Sovereignty"},
			"route_work_item_id": "work-memory",
			"route_environment":  "operator-shell",
			"route_cue_terms":    []string{"modus", "go"},
		}, "Replay evaluation recall."); err != nil {
			return EvaluationCaseResult{}, err
		}
	}

	candidates, _, err := maintain.Replay(k.Vault)
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	maintenanceDocs, err := markdown.ScanDir(k.Vault.Path("memory", "maintenance"))
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	var candidate *markdown.Document
	for _, doc := range maintenanceDocs {
		if doc.Get("type") == "candidate_replay_fact" {
			candidate = doc
			break
		}
	}
	if candidate == nil {
		return EvaluationCaseResult{
			ID:          "replay_promotion_accuracy",
			Name:        "Replay Promotion Accuracy",
			Description: "Repeated episodic evidence and recall traces should yield one correct replay candidate and promoted fact.",
			Passed:      false,
			Score:       0,
			Evidence:    []string{"no replay candidate was written"},
		}, nil
	}

	candidate.Set("status", "approved")
	if err := candidate.Save(); err != nil {
		return EvaluationCaseResult{}, err
	}
	applyResult, err := maintain.ApplyApproved(k.Vault)
	if err != nil {
		return EvaluationCaseResult{}, err
	}

	facts, err := markdown.ScanDir(k.Vault.Path("memory", "facts"))
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	var promoted *markdown.Document
	for _, doc := range facts {
		if strings.EqualFold(doc.Get("subject"), "MODUS") && strings.EqualFold(doc.Get("predicate"), "uses") {
			promoted = doc
			break
		}
	}

	candidateOK := candidates == 1 &&
		candidate.Get("subject") == "MODUS" &&
		candidate.Get("predicate") == "uses" &&
		normalizeEvalText(candidate.Get("value")) == normalizeEvalText("Go for the agent framework")
	promotionOK := promoted != nil &&
		promoted.Get("source_event_id") == "evt-test" &&
		promoted.Get("lineage_id") == "lin-test" &&
		promoted.Get("mission") == "Memory Sovereignty" &&
		applyResult.ReplayPromoted == 1

	score := 0.0
	if candidateOK {
		score += 0.5
	}
	if promotionOK {
		score += 0.5
	}
	return EvaluationCaseResult{
		ID:          "replay_promotion_accuracy",
		Name:        "Replay Promotion Accuracy",
		Description: "Replay should emit one lawful promotion candidate and preserve provenance when that candidate is approved and applied.",
		Passed:      candidateOK && promotionOK,
		Score:       score,
		Evidence: []string{
			fmt.Sprintf("replay candidates=%d apply_replay_promoted=%d", candidates, applyResult.ReplayPromoted),
			fmt.Sprintf("candidate method `%s`", candidate.Get("method")),
			fmt.Sprintf("promoted fact lineage `%s` source_event `%s`", getDocValue(promoted, "lineage_id"), getDocValue(promoted, "source_event_id")),
		},
	}, nil
}

func runHotTierStaleDetectionCase() (EvaluationCaseResult, error) {
	k, cleanup, err := newEvaluationFixture()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	defer cleanup()

	factPath, err := k.StoreFact("Operator thread", "needs", "review debt pruning", 0.82, "high", vault.FactWriteAuthority{
		ProducingOffice:    "librarian",
		ProducingSubsystem: "memory_evaluation",
		StaffingContext:    "phase8_hot_review",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		MemoryTemperature:  "hot",
		AllowApproval:      true,
	})
	if err != nil {
		return EvaluationCaseResult{}, err
	}

	doc, err := k.Vault.Read(factPath)
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	staleAt := time.Now().AddDate(0, 0, -(vault.HotMemoryStaleReviewDays + 7)).UTC().Format(time.RFC3339)
	doc.Set("created_at", staleAt)
	doc.Set("created", staleAt)
	doc.Set("last_accessed", staleAt)
	if err := doc.Save(); err != nil {
		return EvaluationCaseResult{}, err
	}

	count, _, err := maintain.ReviewHotTier(k.Vault)
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	maintenanceDocs, err := markdown.ScanDir(k.Vault.Path("memory", "maintenance"))
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	found := false
	for _, artifact := range maintenanceDocs {
		if artifact.Get("type") != "candidate_hot_memory_transition" {
			continue
		}
		if artifact.Get("fact_path") == factPath && artifact.Get("review_class") == "stale" && artifact.Get("proposed_temperature") == "warm" {
			found = true
			break
		}
	}

	score := 0.0
	if found {
		score = 1
	}
	return EvaluationCaseResult{
		ID:          "hot_tier_stale_detection",
		Name:        "Hot-Tier Stale Detection",
		Description: "Stale hot memory should surface as an explicit downgrade review artifact rather than being altered silently.",
		Passed:      found,
		Score:       score,
		Evidence: []string{
			fmt.Sprintf("review candidates=%d", count),
			fmt.Sprintf("stale transition found for `%s`=%t", factPath, found),
		},
	}, nil
}

func runSecureStateTamperCase() (EvaluationCaseResult, error) {
	k, cleanup, err := newEvaluationFixture()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	defer cleanup()

	factPath, err := k.StoreFact("Founding lesson", "requires", "governed memory", 0.95, "critical", evalFactAuthority("memory_governance"))
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	manifest, err := k.WriteSecureStateManifest()
	if err != nil {
		return EvaluationCaseResult{}, err
	}

	doc, err := k.Vault.Read(factPath)
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	doc.Body = "mutated memory payload"
	if err := doc.Save(); err != nil {
		return EvaluationCaseResult{}, err
	}

	verified, err := k.VerifySecureStateManifest()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	passed := !verified.Verified && containsString(verified.DriftPaths, factPath)
	score := 0.0
	if passed {
		score = 1
	}
	return EvaluationCaseResult{
		ID:          "secure_state_tamper_detection",
		Name:        "Secure-State Tamper Detection",
		Description: "Manifest verification should detect post-manifest drift in covered memory files.",
		Passed:      passed,
		Score:       score,
		Evidence: []string{
			fmt.Sprintf("manifest root `%s`", manifest.Manifest.RootHash),
			fmt.Sprintf("verified=%t rollback_suspected=%t drift_paths=%d", verified.Verified, verified.RollbackSuspected, len(verified.DriftPaths)),
		},
	}, nil
}

func runSecureStateRollbackCase() (EvaluationCaseResult, error) {
	k, cleanup, err := newEvaluationFixture()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	defer cleanup()

	if _, err := k.StoreFact("Founding lesson", "requires", "governed memory", 0.95, "critical", evalFactAuthority("memory_governance")); err != nil {
		return EvaluationCaseResult{}, err
	}
	firstManifest, err := k.WriteSecureStateManifest()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	firstBytes, err := os.ReadFile(k.Vault.Path("state", "memory", "latest.json"))
	if err != nil {
		return EvaluationCaseResult{}, err
	}

	if _, err := k.StoreFact("Route law", "requires", "ledger healing", 0.88, "high", evalFactAuthority("memory_governance")); err != nil {
		return EvaluationCaseResult{}, err
	}
	secondManifest, err := k.WriteSecureStateManifest()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	if secondManifest.Manifest.RootHash == firstManifest.Manifest.RootHash {
		return EvaluationCaseResult{}, fmt.Errorf("rollback fixture did not advance manifest root")
	}
	if err := os.WriteFile(k.Vault.Path("state", "memory", "latest.json"), firstBytes, 0o644); err != nil {
		return EvaluationCaseResult{}, err
	}

	verified, err := k.VerifySecureStateManifest()
	if err != nil {
		return EvaluationCaseResult{}, err
	}
	passed := !verified.Verified && verified.RollbackSuspected && verified.LedgerRootHash == secondManifest.Manifest.RootHash
	score := 0.0
	if passed {
		score = 1
	}
	return EvaluationCaseResult{
		ID:          "secure_state_rollback_detection",
		Name:        "Secure-State Rollback Detection",
		Description: "Verification should suspect rollback when the manifest on disk is older than the newest manifest root already recorded in the ledger.",
		Passed:      passed,
		Score:       score,
		Evidence: []string{
			fmt.Sprintf("expected root `%s` current `%s` ledger `%s`", verified.ExpectedRootHash, verified.CurrentRootHash, verified.LedgerRootHash),
			fmt.Sprintf("rollback_suspected=%t", verified.RollbackSuspected),
		},
	}, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func firstResultPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func getDocValue(doc *markdown.Document, key string) string {
	if doc == nil {
		return ""
	}
	return doc.Get(key)
}

func normalizeEvalText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, "!")
	value = strings.TrimSuffix(value, "?")
	return strings.TrimSpace(value)
}

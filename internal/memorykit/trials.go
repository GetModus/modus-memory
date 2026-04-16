package memorykit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

const (
	memoryTrialCasesRoot = "state/memory/trials/cases"
	memoryTrialSuite     = "grade_s_live_trials"
)

type TrialCaseResult struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	CasePath              string   `json:"case_path"`
	Query                 string   `json:"query"`
	Passed                bool     `json:"passed"`
	Score                 float64  `json:"score"`
	ReceiptPath           string   `json:"receipt_path,omitempty"`
	ResultCount           int      `json:"result_count"`
	TopPath               string   `json:"top_path,omitempty"`
	TopVerificationStatus string   `json:"top_verification_status,omitempty"`
	TopTemporalStatus     string   `json:"top_temporal_status,omitempty"`
	LinkedFactPaths       []string `json:"linked_fact_paths,omitempty"`
	LinkedEpisodePaths    []string `json:"linked_episode_paths,omitempty"`
	LinkedEntityRefs      []string `json:"linked_entity_refs,omitempty"`
	LinkedMissionRefs     []string `json:"linked_mission_refs,omitempty"`
	Evidence              []string `json:"evidence,omitempty"`
}

type TrialReport struct {
	Version      int                 `json:"version"`
	Suite        string              `json:"suite"`
	GeneratedAt  string              `json:"generated_at"`
	TotalCases   int                 `json:"total_cases"`
	PassedCases  int                 `json:"passed_cases"`
	OverallScore float64             `json:"overall_score"`
	Cases        []TrialCaseResult   `json:"cases"`
	Signature    signature.Signature `json:"signature"`
}

type TrialReportResult struct {
	ReportPath   string
	MarkdownPath string
	Report       TrialReport
}

type memoryTrialCase struct {
	ID                       string
	Name                     string
	Description              string
	CasePath                 string
	Query                    string
	Limit                    int
	Options                  vault.FactSearchOptions
	ExpectTopPath            string
	ExpectContainsPaths      []string
	ExpectMinResults         int
	ExpectVerificationStatus string
	ExpectTopTemporalStatus  string
	ExpectLineContains       []string
	ExpectLinkedFactPaths    []string
	ExpectLinkedEpisodePaths []string
	ExpectLinkedEntityRefs   []string
	ExpectLinkedMissionRefs  []string
}

func (k *Kernel) RunTrials() (TrialReportResult, error) {
	cases, err := loadMemoryTrialCases(k.Vault)
	if err != nil {
		return TrialReportResult{}, err
	}
	if len(cases) == 0 {
		return TrialReportResult{}, fmt.Errorf("no active memory trial cases under %s", memoryTrialCasesRoot)
	}

	results := make([]TrialCaseResult, 0, len(cases))
	passed := 0
	scoreTotal := 0.0
	for _, tc := range cases {
		result := k.runMemoryTrialCase(tc)
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
	report := TrialReport{
		Version:      1,
		Suite:        memoryTrialSuite,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		TotalCases:   len(results),
		PassedCases:  passed,
		OverallScore: overall,
		Cases:        results,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_trials",
			StaffingContext:    "live_sovereign_vault",
			AuthorityScope:     ledger.ScopeRuntimeMemoryTrials,
			ArtifactState:      "derived",
			SourceRefs:         []string{"state/memory/trials/latest.json", "state/memory/trials/latest.md", memoryTrialCasesRoot},
			PromotionStatus:    "observed",
			ProofRef:           "memory-trials:" + memoryTrialSuite,
		}.EnsureTimestamp(),
	}

	jsonRelPath := filepath.ToSlash(filepath.Join("state", "memory", "trials", "latest.json"))
	mdRelPath := filepath.ToSlash(filepath.Join("state", "memory", "trials", "latest.md"))
	jsonAbsPath := filepath.Join(k.Vault.Dir, jsonRelPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, mdRelPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return TrialReportResult{}, err
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return TrialReportResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return TrialReportResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_trial_report",
		"suite":               report.Suite,
		"generated_at":        report.GeneratedAt,
		"overall_score":       report.OverallScore,
		"passed_cases":        report.PassedCases,
		"total_cases":         report.TotalCases,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Trial Report\n\n")
	body.WriteString(fmt.Sprintf("Suite: `%s`\n\n", report.Suite))
	body.WriteString(fmt.Sprintf("Overall score: `%.2f`\n\n", report.OverallScore))
	body.WriteString(fmt.Sprintf("Passed cases: `%d/%d`\n\n", report.PassedCases, report.TotalCases))
	body.WriteString("This report grades the live sovereign vault against authored trial cases. Unlike the synthetic evaluator, it does not seed a nursery corpus. It measures what the present estate can actually recover, and what it must still admit is under-sourced.\n\n")
	body.WriteString("## Cases\n\n")
	for _, result := range report.Cases {
		status := "fail"
		if result.Passed {
			status = "pass"
		}
		body.WriteString(fmt.Sprintf("### %s\n\n", result.Name))
		body.WriteString(fmt.Sprintf("Status: `%s`\n\n", status))
		body.WriteString(fmt.Sprintf("Score: `%.2f`\n\n", clampEvalScore(result.Score)))
		body.WriteString(fmt.Sprintf("Case: `%s`\n\n", result.CasePath))
		body.WriteString(fmt.Sprintf("Query: `%s`\n\n", result.Query))
		if result.ReceiptPath != "" {
			body.WriteString(fmt.Sprintf("Receipt: `%s`\n\n", result.ReceiptPath))
		}
		if result.TopPath != "" {
			body.WriteString(fmt.Sprintf("Top path: `%s`\n\n", result.TopPath))
		}
		if result.TopVerificationStatus != "" {
			body.WriteString(fmt.Sprintf("Top verification: `%s`\n\n", result.TopVerificationStatus))
		}
		if result.TopTemporalStatus != "" {
			body.WriteString(fmt.Sprintf("Top temporal status: `%s`\n\n", result.TopTemporalStatus))
		}
		body.WriteString(result.Description)
		body.WriteString("\n\n")
		if len(result.LinkedFactPaths) > 0 || len(result.LinkedEpisodePaths) > 0 || len(result.LinkedEntityRefs) > 0 || len(result.LinkedMissionRefs) > 0 {
			body.WriteString("Linked structure:\n")
			if len(result.LinkedFactPaths) > 0 {
				body.WriteString(fmt.Sprintf("- linked facts: `%s`\n", strings.Join(result.LinkedFactPaths, "`, `")))
			}
			if len(result.LinkedEpisodePaths) > 0 {
				body.WriteString(fmt.Sprintf("- linked episodes: `%s`\n", strings.Join(result.LinkedEpisodePaths, "`, `")))
			}
			if len(result.LinkedEntityRefs) > 0 {
				body.WriteString(fmt.Sprintf("- linked entities: `%s`\n", strings.Join(result.LinkedEntityRefs, "`, `")))
			}
			if len(result.LinkedMissionRefs) > 0 {
				body.WriteString(fmt.Sprintf("- linked missions: `%s`\n", strings.Join(result.LinkedMissionRefs, "`, `")))
			}
			body.WriteString("\n")
		}
		if len(result.Evidence) > 0 {
			body.WriteString("Evidence:\n")
			for _, line := range result.Evidence {
				body.WriteString(fmt.Sprintf("- %s\n", line))
			}
			body.WriteString("\n")
		}
	}
	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return TrialReportResult{}, err
	}

	status := ledger.ResultCompleted
	if report.PassedCases != report.TotalCases {
		status = ledger.ResultFailed
	}
	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_trials",
		AuthorityScope: ledger.ScopeRuntimeMemoryTrials,
		ActionClass:    ledger.ActionMemoryTrialRun,
		TargetDomain:   jsonRelPath,
		ResultStatus:   status,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_trial_report_written"},
		ProofRefs:      []string{jsonRelPath, mdRelPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_trials",
			StaffingContext:    "live_sovereign_vault",
			AuthorityScope:     ledger.ScopeRuntimeMemoryTrials,
			ArtifactState:      "derived",
			SourceRefs:         []string{jsonRelPath, mdRelPath, memoryTrialCasesRoot},
			PromotionStatus:    "observed",
			ProofRef:           "memory-trials:" + memoryTrialSuite,
		}.EnsureTimestamp(),
		Metadata: map[string]interface{}{
			"suite":         report.Suite,
			"overall_score": report.OverallScore,
			"passed_cases":  report.PassedCases,
			"total_cases":   report.TotalCases,
			"case_ids":      trialCaseIDs(report.Cases),
		},
	})

	return TrialReportResult{
		ReportPath:   jsonRelPath,
		MarkdownPath: mdRelPath,
		Report:       report,
	}, nil
}

func loadMemoryTrialCases(v *vault.Vault) ([]memoryTrialCase, error) {
	docs, err := markdown.ScanDir(v.Path("state", "memory", "trials", "cases"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })

	var cases []memoryTrialCase
	for _, doc := range docs {
		if strings.TrimSpace(doc.Get("type")) != "memory_trial_case" {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(doc.Get("status")))
		if status != "" && status != "active" {
			continue
		}
		tc, err := parseMemoryTrialCase(v, doc)
		if err != nil {
			return nil, fmt.Errorf("parse trial case %s: %w", doc.Path, err)
		}
		cases = append(cases, tc)
	}
	return cases, nil
}

func parseMemoryTrialCase(v *vault.Vault, doc *markdown.Document) (memoryTrialCase, error) {
	rel, err := filepath.Rel(v.Dir, doc.Path)
	if err != nil {
		rel = doc.Path
	}
	query := strings.TrimSpace(doc.Get("query"))
	if query == "" {
		return memoryTrialCase{}, fmt.Errorf("missing query")
	}
	tc := memoryTrialCase{
		ID:                       firstNonEmptyString(strings.TrimSpace(doc.Get("trial_id")), strings.TrimSuffix(filepath.Base(doc.Path), filepath.Ext(doc.Path))),
		Name:                     firstNonEmptyString(strings.TrimSpace(doc.Get("title")), strings.TrimSpace(doc.Get("name")), strings.TrimSuffix(filepath.Base(doc.Path), filepath.Ext(doc.Path))),
		Description:              strings.TrimSpace(firstNonEmptyString(doc.Get("summary"), doc.Body)),
		CasePath:                 filepath.ToSlash(rel),
		Query:                    query,
		Limit:                    docInt(doc, "limit", 3),
		ExpectTopPath:            normalizeTrialPath(doc.Get("expect_top_path")),
		ExpectContainsPaths:      normalizeTrialPaths(docStringSlice(doc, "expect_contains_paths")),
		ExpectMinResults:         docInt(doc, "expect_min_results", 0),
		ExpectVerificationStatus: strings.TrimSpace(doc.Get("expect_verification_status")),
		ExpectTopTemporalStatus:  strings.TrimSpace(doc.Get("expect_top_temporal_status")),
		ExpectLineContains:       dedupeStrings(docStringSlice(doc, "expect_line_contains")),
		ExpectLinkedFactPaths:    normalizeTrialPaths(docStringSlice(doc, "expect_linked_fact_paths")),
		ExpectLinkedEpisodePaths: normalizeTrialPaths(docStringSlice(doc, "expect_linked_episode_paths")),
		ExpectLinkedEntityRefs:   dedupeStrings(docStringSlice(doc, "expect_linked_entity_refs")),
		ExpectLinkedMissionRefs:  dedupeStrings(docStringSlice(doc, "expect_linked_mission_refs")),
		Options: vault.FactSearchOptions{
			MemoryTemperature: strings.TrimSpace(doc.Get("memory_temperature")),
			VerificationMode:  strings.TrimSpace(doc.Get("verification_mode")),
			RouteSubject:      strings.TrimSpace(doc.Get("route_subject")),
			RouteMission:      strings.TrimSpace(doc.Get("route_mission")),
			CapturedByOffice:  strings.TrimSpace(doc.Get("captured_by_office")),
			CueTerms:          dedupeStrings(docStringSlice(doc, "cue_terms")),
			TimeBand:          strings.TrimSpace(doc.Get("time_band")),
			WorkItemID:        strings.TrimSpace(doc.Get("work_item_id")),
			LineageID:         strings.TrimSpace(doc.Get("lineage_id")),
			Environment:       strings.TrimSpace(doc.Get("environment")),
		},
	}
	if tc.Limit <= 0 {
		tc.Limit = 3
	}
	if tc.Description == "" {
		tc.Description = "Live sovereign-vault memory trial."
	}
	if tc.ExpectTopPath == "" && len(tc.ExpectContainsPaths) == 0 && tc.ExpectMinResults <= 0 && tc.ExpectVerificationStatus == "" && tc.ExpectTopTemporalStatus == "" && len(tc.ExpectLineContains) == 0 && len(tc.ExpectLinkedFactPaths) == 0 && len(tc.ExpectLinkedEpisodePaths) == 0 && len(tc.ExpectLinkedEntityRefs) == 0 && len(tc.ExpectLinkedMissionRefs) == 0 {
		return memoryTrialCase{}, fmt.Errorf("no expectations declared")
	}
	return tc, nil
}

func (k *Kernel) runMemoryTrialCase(tc memoryTrialCase) TrialCaseResult {
	recall, err := k.Recall(RecallRequest{
		Query:              tc.Query,
		Limit:              tc.Limit,
		Options:            tc.Options,
		Harness:            "memory_trials",
		Adapter:            "kernel",
		Mode:               "trial_case",
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "memory_trials",
		StaffingContext:    "live_sovereign_vault",
	})
	result := TrialCaseResult{
		ID:          tc.ID,
		Name:        tc.Name,
		Description: tc.Description,
		CasePath:    tc.CasePath,
		Query:       tc.Query,
	}
	if err != nil {
		result.Evidence = []string{fmt.Sprintf("recall error: %v", err)}
		return result
	}

	result.ReceiptPath = recall.ReceiptPath
	result.ResultCount = len(recall.ResultPaths)
	result.LinkedFactPaths = append([]string(nil), recall.LinkedFactPaths...)
	result.LinkedEpisodePaths = append([]string(nil), recall.LinkedEpisodePaths...)
	result.LinkedEntityRefs = append([]string(nil), recall.LinkedEntityRefs...)
	result.LinkedMissionRefs = append([]string(nil), recall.LinkedMissionRefs...)
	if len(recall.ResultPaths) > 0 {
		result.TopPath = recall.ResultPaths[0]
		if doc, readErr := k.Vault.Read(result.TopPath); readErr == nil {
			result.TopTemporalStatus = effectiveTrialTemporalStatus(doc)
		}
	}
	if len(recall.Verification) > 0 {
		result.TopVerificationStatus = recall.Verification[0].Status
	}

	totalExpectations := 0
	metExpectations := 0
	var evidence []string

	if tc.ExpectTopPath != "" {
		totalExpectations++
		if normalizeTrialPath(result.TopPath) == tc.ExpectTopPath {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("top path matched `%s`", tc.ExpectTopPath))
		} else {
			evidence = append(evidence, fmt.Sprintf("top path got `%s` want `%s`", result.TopPath, tc.ExpectTopPath))
		}
	}
	if len(tc.ExpectContainsPaths) > 0 {
		totalExpectations++
		missing := missingTrialPaths(recall.ResultPaths, tc.ExpectContainsPaths)
		if len(missing) == 0 {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("contained expected paths `%s`", strings.Join(tc.ExpectContainsPaths, "`, `")))
		} else {
			evidence = append(evidence, fmt.Sprintf("missing expected paths `%s`", strings.Join(missing, "`, `")))
		}
	}
	if tc.ExpectMinResults > 0 {
		totalExpectations++
		if len(recall.ResultPaths) >= tc.ExpectMinResults {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("result count %d met minimum %d", len(recall.ResultPaths), tc.ExpectMinResults))
		} else {
			evidence = append(evidence, fmt.Sprintf("result count %d below minimum %d", len(recall.ResultPaths), tc.ExpectMinResults))
		}
	}
	if tc.ExpectVerificationStatus != "" {
		totalExpectations++
		if strings.EqualFold(result.TopVerificationStatus, tc.ExpectVerificationStatus) {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("top verification matched `%s`", tc.ExpectVerificationStatus))
		} else {
			evidence = append(evidence, fmt.Sprintf("top verification got `%s` want `%s`", result.TopVerificationStatus, tc.ExpectVerificationStatus))
		}
	}
	if tc.ExpectTopTemporalStatus != "" {
		totalExpectations++
		if strings.EqualFold(result.TopTemporalStatus, tc.ExpectTopTemporalStatus) {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("top temporal status matched `%s`", tc.ExpectTopTemporalStatus))
		} else {
			evidence = append(evidence, fmt.Sprintf("top temporal status got `%s` want `%s`", result.TopTemporalStatus, tc.ExpectTopTemporalStatus))
		}
	}
	if len(tc.ExpectLineContains) > 0 {
		totalExpectations++
		var missing []string
		for _, needle := range tc.ExpectLineContains {
			if !trialLinesContain(recall.Lines, needle) {
				missing = append(missing, needle)
			}
		}
		if len(missing) == 0 {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("returned lines contained `%s`", strings.Join(tc.ExpectLineContains, "`, `")))
		} else {
			evidence = append(evidence, fmt.Sprintf("returned lines missing `%s`", strings.Join(missing, "`, `")))
		}
	}
	if len(tc.ExpectLinkedFactPaths) > 0 {
		totalExpectations++
		missing := missingTrialPaths(recall.LinkedFactPaths, tc.ExpectLinkedFactPaths)
		if len(missing) == 0 {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("linked facts contained `%s`", strings.Join(tc.ExpectLinkedFactPaths, "`, `")))
		} else {
			evidence = append(evidence, fmt.Sprintf("linked facts missing `%s`", strings.Join(missing, "`, `")))
		}
	}
	if len(tc.ExpectLinkedEpisodePaths) > 0 {
		totalExpectations++
		missing := missingTrialPaths(recall.LinkedEpisodePaths, tc.ExpectLinkedEpisodePaths)
		if len(missing) == 0 {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("linked episodes contained `%s`", strings.Join(tc.ExpectLinkedEpisodePaths, "`, `")))
		} else {
			evidence = append(evidence, fmt.Sprintf("linked episodes missing `%s`", strings.Join(missing, "`, `")))
		}
	}
	if len(tc.ExpectLinkedEntityRefs) > 0 {
		totalExpectations++
		missing := missingTrialValues(recall.LinkedEntityRefs, tc.ExpectLinkedEntityRefs)
		if len(missing) == 0 {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("linked entities contained `%s`", strings.Join(tc.ExpectLinkedEntityRefs, "`, `")))
		} else {
			evidence = append(evidence, fmt.Sprintf("linked entities missing `%s`", strings.Join(missing, "`, `")))
		}
	}
	if len(tc.ExpectLinkedMissionRefs) > 0 {
		totalExpectations++
		missing := missingTrialValues(recall.LinkedMissionRefs, tc.ExpectLinkedMissionRefs)
		if len(missing) == 0 {
			metExpectations++
			evidence = append(evidence, fmt.Sprintf("linked missions contained `%s`", strings.Join(tc.ExpectLinkedMissionRefs, "`, `")))
		} else {
			evidence = append(evidence, fmt.Sprintf("linked missions missing `%s`", strings.Join(missing, "`, `")))
		}
	}

	result.Passed = totalExpectations > 0 && metExpectations == totalExpectations
	if totalExpectations > 0 {
		result.Score = float64(metExpectations) / float64(totalExpectations)
	}
	result.Evidence = evidence
	return result
}

func docInt(doc *markdown.Document, key string, fallback int) int {
	if doc == nil || doc.Frontmatter == nil {
		return fallback
	}
	raw, ok := doc.Frontmatter[key]
	if !ok {
		return fallback
	}
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return fallback
	}
}

func docStringSlice(doc *markdown.Document, key string) []string {
	if doc == nil || doc.Frontmatter == nil {
		return nil
	}
	raw, ok := doc.Frontmatter[key]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return dedupeStrings(value)
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil
		}
		return []string{text}
	default:
		return nil
	}
}

func normalizeTrialPath(path string) string {
	return filepath.ToSlash(strings.TrimSpace(path))
}

func normalizeTrialPaths(paths []string) []string {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = normalizeTrialPath(path)
		if path != "" {
			normalized = append(normalized, path)
		}
	}
	return dedupeStrings(normalized)
}

func missingTrialPaths(got, expected []string) []string {
	seen := make(map[string]bool, len(got))
	for _, path := range got {
		seen[normalizeTrialPath(path)] = true
	}
	var missing []string
	for _, path := range expected {
		if !seen[normalizeTrialPath(path)] {
			missing = append(missing, path)
		}
	}
	return missing
}

func trialLinesContain(lines []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return true
	}
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			return true
		}
	}
	return false
}

func effectiveTrialTemporalStatus(doc *markdown.Document) string {
	status := strings.ToLower(strings.TrimSpace(doc.Get("temporal_status")))
	if status == "" {
		status = "active"
	}
	if status == "active" {
		validTo := strings.TrimSpace(doc.Get("valid_to"))
		if validTo != "" {
			if t, err := time.Parse(time.RFC3339, validTo); err == nil && !t.After(time.Now()) {
				return "expired"
			}
		}
	}
	return status
}

func normalizeTrialValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func missingTrialValues(got, expected []string) []string {
	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[normalizeTrialValue(value)] = true
	}
	var missing []string
	for _, value := range expected {
		if !seen[normalizeTrialValue(value)] {
			missing = append(missing, value)
		}
	}
	return missing
}

func trialCaseIDs(cases []TrialCaseResult) []string {
	ids := make([]string, 0, len(cases))
	for _, result := range cases {
		ids = append(ids, result.ID)
	}
	return ids
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
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

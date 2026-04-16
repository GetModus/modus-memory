package memorykit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
)

type ReadinessShelf struct {
	Count               int `json:"count"`
	HotCount            int `json:"hot_count,omitempty"`
	WarmCount           int `json:"warm_count,omitempty"`
	StructuredCount     int `json:"structured_count,omitempty"`
	ElderProtectedCount int `json:"elder_protected_count,omitempty"`
	SupersededCount     int `json:"superseded_count,omitempty"`
	ExpiredCount        int `json:"expired_count,omitempty"`
}

type ReadinessReport struct {
	Version             int                       `json:"version"`
	GeneratedAt         string                    `json:"generated_at"`
	Status              string                    `json:"status"`
	Issues              []string                  `json:"issues"`
	Shelves             map[string]ReadinessShelf `json:"shelves"`
	PendingMaintenance  int                       `json:"pending_maintenance"`
	TrialScore          float64                   `json:"trial_score"`
	TrialPassedCases    int                       `json:"trial_passed_cases"`
	TrialTotalCases     int                       `json:"trial_total_cases"`
	EvaluationScore     float64                   `json:"evaluation_score"`
	EvaluationPassed    int                       `json:"evaluation_passed_cases"`
	EvaluationTotal     int                       `json:"evaluation_total_cases"`
	PortabilityScore    float64                   `json:"portability_score"`
	PortabilityExternal int                       `json:"portability_external_only"`
	SecureStatePath     string                    `json:"secure_state_path"`
	SecureStateVerified bool                      `json:"secure_state_verified"`
	SecureStateDrift    int                       `json:"secure_state_drift_count"`
	SecureStateRollback bool                      `json:"secure_state_rollback_suspected"`
	Signature           signature.Signature       `json:"signature"`
}

type ReadinessReportResult struct {
	ReportPath   string
	MarkdownPath string
	Report       ReadinessReport
}

func (k *Kernel) RunReadiness() (ReadinessReportResult, error) {
	manifest, err := k.WriteSecureStateManifest()
	if err != nil {
		return ReadinessReportResult{}, fmt.Errorf("write secure-state manifest: %w", err)
	}
	verification, err := k.VerifySecureStateManifest()
	if err != nil {
		return ReadinessReportResult{}, fmt.Errorf("verify secure-state manifest: %w", err)
	}

	factShelf, err := analyzeReadinessShelf(k.Vault.Dir, "memory/facts")
	if err != nil {
		return ReadinessReportResult{}, err
	}
	episodeShelf, err := analyzeReadinessShelf(k.Vault.Dir, "memory/episodes")
	if err != nil {
		return ReadinessReportResult{}, err
	}
	factsCount := factShelf.Count
	episodesCount := episodeShelf.Count
	recallsCount, err := countMarkdownDocs(k.Vault.Dir, "memory/recalls")
	if err != nil {
		return ReadinessReportResult{}, err
	}
	maintenanceCount, err := countMarkdownDocs(k.Vault.Dir, "memory/maintenance")
	if err != nil {
		return ReadinessReportResult{}, err
	}
	pendingMaintenance, err := countPendingMaintenance(k.Vault.Dir)
	if err != nil {
		return ReadinessReportResult{}, err
	}

	trials, _ := readTrialReport(filepath.Join(k.Vault.Dir, "state", "memory", "trials", "latest.json"))
	evaluation, _ := readEvaluationReport(filepath.Join(k.Vault.Dir, "state", "memory", "evaluations", "latest.json"))
	portability, _ := readPortabilityAudit(filepath.Join(k.Vault.Dir, "state", "memory", "portability", "latest.json"))

	shelves := map[string]ReadinessShelf{
		"facts":       factShelf,
		"episodes":    episodeShelf,
		"recalls":     {Count: recallsCount},
		"maintenance": {Count: maintenanceCount},
	}

	var issues []string
	if factsCount == 0 {
		issues = append(issues, "no durable facts present in sovereign memory")
	}
	if episodesCount == 0 {
		issues = append(issues, "no live episodic memory objects present under memory/episodes")
	}
	if recallsCount == 0 {
		issues = append(issues, "no live recall receipts present under memory/recalls")
	}
	if factShelf.HotCount == 0 {
		issues = append(issues, "no hot facts are presently commissioned for automatic admission")
	}
	if factShelf.StructuredCount == 0 {
		issues = append(issues, "no structurally linked facts are present in sovereign memory")
	}
	if pendingMaintenance > 0 {
		issues = append(issues, fmt.Sprintf("%d pending maintenance artifacts remain under memory/maintenance", pendingMaintenance))
	}
	if trials == nil {
		issues = append(issues, "no live memory trial report present")
	} else if trials.PassedCases != trials.TotalCases {
		issues = append(issues, fmt.Sprintf("live memory trials are not clean: %d/%d passed", trials.PassedCases, trials.TotalCases))
	}
	if evaluation == nil {
		issues = append(issues, "no synthetic memory evaluation report present")
	} else if evaluation.PassedCases != evaluation.TotalCases {
		issues = append(issues, fmt.Sprintf("synthetic memory evaluation is not clean: %d/%d passed", evaluation.PassedCases, evaluation.TotalCases))
	}
	if portability == nil {
		issues = append(issues, "no memory portability audit present")
	} else if portability.ExternalOnly > 0 {
		issues = append(issues, fmt.Sprintf("memory portability still has %d external-only residue files", portability.ExternalOnly))
	}
	if !verification.Verified {
		issues = append(issues, fmt.Sprintf("secure-state verification failed with %d drift paths", len(verification.DriftPaths)))
	}
	if verification.RollbackSuspected {
		issues = append(issues, "secure-state verification suspects rollback")
	}

	status := "ready_for_pretesting"
	if len(issues) > 0 {
		status = "attention_required"
	}

	report := ReadinessReport{
		Version:             1,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
		Status:              status,
		Issues:              issues,
		Shelves:             shelves,
		PendingMaintenance:  pendingMaintenance,
		SecureStatePath:     manifest.ManifestPath,
		SecureStateVerified: verification.Verified,
		SecureStateDrift:    len(verification.DriftPaths),
		SecureStateRollback: verification.RollbackSuspected,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_readiness",
			StaffingContext:    "pretesting_readiness",
			AuthorityScope:     ledger.ScopeRuntimeMemoryReadiness,
			ArtifactState:      "derived",
			SourceRefs:         []string{"state/memory/readiness/latest.json", "state/memory/readiness/latest.md"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-readiness:" + time.Now().UTC().Format("20060102T150405Z"),
		}.EnsureTimestamp(),
	}
	if trials != nil {
		report.TrialScore = trials.OverallScore
		report.TrialPassedCases = trials.PassedCases
		report.TrialTotalCases = trials.TotalCases
	}
	if evaluation != nil {
		report.EvaluationScore = evaluation.OverallScore
		report.EvaluationPassed = evaluation.PassedCases
		report.EvaluationTotal = evaluation.TotalCases
	}
	if portability != nil {
		report.PortabilityScore = portability.CoverageScore
		report.PortabilityExternal = portability.ExternalOnly
	}

	reportPath := filepath.ToSlash(filepath.Join("state", "memory", "readiness", "latest.json"))
	markdownPath := filepath.ToSlash(filepath.Join("state", "memory", "readiness", "latest.md"))
	jsonAbsPath := filepath.Join(k.Vault.Dir, reportPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, markdownPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return ReadinessReportResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ReadinessReportResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return ReadinessReportResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                  "memory_readiness_report",
		"generated_at":          report.GeneratedAt,
		"status":                report.Status,
		"issues":                report.Issues,
		"shelves":               report.Shelves,
		"pending_maintenance":   report.PendingMaintenance,
		"trial_score":           report.TrialScore,
		"trial_passed_cases":    report.TrialPassedCases,
		"trial_total_cases":     report.TrialTotalCases,
		"evaluation_score":      report.EvaluationScore,
		"evaluation_passed":     report.EvaluationPassed,
		"evaluation_total":      report.EvaluationTotal,
		"portability_score":     report.PortabilityScore,
		"portability_external":  report.PortabilityExternal,
		"secure_state_path":     report.SecureStatePath,
		"secure_state_verified": report.SecureStateVerified,
		"producing_signature":   report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Readiness Report\n\n")
	body.WriteString(fmt.Sprintf("Status: `%s`\n\n", report.Status))
	body.WriteString(fmt.Sprintf("Facts: `%d`\n\n", factsCount))
	body.WriteString(fmt.Sprintf("Hot facts: `%d`\n\n", factShelf.HotCount))
	body.WriteString(fmt.Sprintf("Warm facts: `%d`\n\n", factShelf.WarmCount))
	body.WriteString(fmt.Sprintf("Structured facts: `%d`\n\n", factShelf.StructuredCount))
	body.WriteString(fmt.Sprintf("Superseded facts: `%d`\n\n", factShelf.SupersededCount))
	body.WriteString(fmt.Sprintf("Expired facts: `%d`\n\n", factShelf.ExpiredCount))
	body.WriteString(fmt.Sprintf("Elder-protected facts: `%d`\n\n", factShelf.ElderProtectedCount))
	body.WriteString(fmt.Sprintf("Episodes: `%d`\n\n", episodesCount))
	body.WriteString(fmt.Sprintf("Structured episodes: `%d`\n\n", episodeShelf.StructuredCount))
	body.WriteString(fmt.Sprintf("Recall receipts: `%d`\n\n", recallsCount))
	body.WriteString(fmt.Sprintf("Maintenance artifacts: `%d`\n\n", maintenanceCount))
	body.WriteString(fmt.Sprintf("Pending maintenance artifacts: `%d`\n\n", pendingMaintenance))
	body.WriteString(fmt.Sprintf("Live trial score: `%.2f` (`%d/%d`)\n\n", report.TrialScore, report.TrialPassedCases, report.TrialTotalCases))
	body.WriteString(fmt.Sprintf("Synthetic evaluation score: `%.2f` (`%d/%d`)\n\n", report.EvaluationScore, report.EvaluationPassed, report.EvaluationTotal))
	body.WriteString(fmt.Sprintf("Portability score: `%.2f` with external residue `%d`\n\n", report.PortabilityScore, report.PortabilityExternal))
	body.WriteString(fmt.Sprintf("Secure-state verified: `%t`\n\n", report.SecureStateVerified))
	body.WriteString(fmt.Sprintf("Secure-state manifest: `%s`\n\n", report.SecureStatePath))
	if len(report.Issues) == 0 {
		body.WriteString("No readiness issues were detected.\n")
	} else {
		body.WriteString("## Issues\n\n")
		for _, issue := range report.Issues {
			body.WriteString(fmt.Sprintf("- %s\n", issue))
		}
	}
	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return ReadinessReportResult{}, err
	}

	resultStatus := ledger.ResultCompleted
	if len(report.Issues) > 0 {
		resultStatus = ledger.ResultFailed
	}
	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_readiness",
		AuthorityScope: ledger.ScopeRuntimeMemoryReadiness,
		ActionClass:    ledger.ActionMemoryReadinessAudit,
		TargetDomain:   reportPath,
		ResultStatus:   resultStatus,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_readiness_report_written"},
		ProofRefs:      []string{reportPath, markdownPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_readiness",
			StaffingContext:    "pretesting_readiness",
			AuthorityScope:     ledger.ScopeRuntimeMemoryReadiness,
			ArtifactState:      "derived",
			SourceRefs:         []string{reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           report.Signature.ProofRef,
		}.EnsureTimestamp(),
		Metadata: map[string]interface{}{
			"status":                report.Status,
			"issue_count":           len(report.Issues),
			"facts":                 factsCount,
			"episodes":              episodesCount,
			"recalls":               recallsCount,
			"maintenance":           maintenanceCount,
			"pending_maintenance":   pendingMaintenance,
			"trial_score":           report.TrialScore,
			"evaluation_score":      report.EvaluationScore,
			"portability_score":     report.PortabilityScore,
			"secure_state_verified": report.SecureStateVerified,
		},
	})

	return ReadinessReportResult{
		ReportPath:   reportPath,
		MarkdownPath: markdownPath,
		Report:       report,
	}, nil
}

func countMarkdownDocs(vaultDir string, rel string) (int, error) {
	docs, err := markdown.ScanDir(filepath.Join(vaultDir, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return len(docs), nil
}

func countPendingMaintenance(vaultDir string) (int, error) {
	docs, err := markdown.ScanDir(filepath.Join(vaultDir, "memory", "maintenance"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, doc := range docs {
		if strings.EqualFold(strings.TrimSpace(doc.Get("status")), "pending") {
			count++
		}
	}
	return count, nil
}

func analyzeReadinessShelf(vaultDir string, rel string) (ReadinessShelf, error) {
	docs, err := markdown.ScanDir(filepath.Join(vaultDir, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return ReadinessShelf{}, nil
		}
		return ReadinessShelf{}, err
	}
	shelf := ReadinessShelf{Count: len(docs)}
	for _, doc := range docs {
		switch strings.ToLower(strings.TrimSpace(doc.Get("memory_temperature"))) {
		case "hot":
			shelf.HotCount++
		case "warm":
			shelf.WarmCount++
		}
		if strings.EqualFold(strings.TrimSpace(doc.Get("memory_protection_class")), "elder") {
			shelf.ElderProtectedCount++
		}
		if readinessDocHasStructure(doc) {
			shelf.StructuredCount++
		}
		switch readinessTemporalStatus(doc) {
		case "superseded":
			shelf.SupersededCount++
		case "expired":
			shelf.ExpiredCount++
		}
	}
	return shelf, nil
}

func readinessDocHasStructure(doc *markdown.Document) bool {
	for _, key := range []string{"related_fact_paths", "related_episode_paths", "related_entity_refs", "related_mission_refs"} {
		if len(docStringSlice(doc, key)) > 0 {
			return true
		}
	}
	return false
}

func readinessTemporalStatus(doc *markdown.Document) string {
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

func readTrialReport(path string) (*TrialReport, error) {
	var report TrialReport
	if err := readJSONReport(path, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func readEvaluationReport(path string) (*EvaluationReport, error) {
	var report EvaluationReport
	if err := readJSONReport(path, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func readPortabilityAudit(path string) (*PortabilityAuditReport, error) {
	var report PortabilityAuditReport
	if err := readJSONReport(path, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func readJSONReport(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

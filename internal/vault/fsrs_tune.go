package vault

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trust"
)

// FSRSTuneReport holds the analysis of current FSRS behavior.
type FSRSTuneReport struct {
	TotalFacts   int
	ByImportance map[string]*importanceBucket
	GeneratedAt  time.Time
	Proposals    map[string]fsrsProposal
}

type importanceBucket struct {
	Count         int
	OverRetained  int // stability > 1000, confidence > 0.99, access_count < 3
	OverForgotten int // hit floor without any reinforcement (access_count == 0)
	AvgStability  float64
	AvgConfidence float64
}

type fsrsProposal struct {
	CurrentStability  float64
	ProposedStability float64
	Reason            string
}

// AnalyzeFSRS scans all facts and computes retention statistics per importance tier.
// Returns a report with over-retained and over-forgotten counts plus proposed adjustments.
// This is advisory only — it does not modify any configuration.
func (v *Vault) AnalyzeFSRS() (*FSRSTuneReport, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return nil, err
	}

	report := &FSRSTuneReport{
		GeneratedAt:  time.Now(),
		ByImportance: make(map[string]*importanceBucket),
		Proposals:    make(map[string]fsrsProposal),
	}

	for _, importance := range []string{"critical", "high", "medium", "low"} {
		report.ByImportance[importance] = &importanceBucket{}
	}

	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		report.TotalFacts++

		importance := doc.Get("importance")
		if importance == "" {
			importance = "medium"
		}
		bucket, ok := report.ByImportance[importance]
		if !ok {
			bucket = &importanceBucket{}
			report.ByImportance[importance] = bucket
		}
		bucket.Count++

		stability := doc.GetFloat("stability")
		confidence := doc.GetFloat("confidence")
		accessCount := int(doc.GetFloat("access_count"))

		bucket.AvgStability += stability
		bucket.AvgConfidence += confidence

		// Over-retained: very high stability, near-max confidence, rarely accessed
		if stability > 1000 && confidence > 0.99 && accessCount < 3 {
			bucket.OverRetained++
		}

		// Over-forgotten: hit floor without ever being recalled
		cfg, cfgOk := fsrsConfigs[importance]
		if cfgOk && confidence > 0 && confidence <= cfg.Floor && accessCount == 0 {
			bucket.OverForgotten++
		}
	}

	// Compute averages and generate proposals
	for importance, bucket := range report.ByImportance {
		if bucket.Count == 0 {
			continue
		}
		bucket.AvgStability /= float64(bucket.Count)
		bucket.AvgConfidence /= float64(bucket.Count)

		cfg, ok := fsrsConfigs[importance]
		if !ok || importance == "critical" {
			continue // never tune critical
		}

		overRetainedPct := float64(bucket.OverRetained) / float64(bucket.Count)
		overForgottenPct := float64(bucket.OverForgotten) / float64(bucket.Count)

		if overRetainedPct > 0.20 {
			proposed := cfg.InitialStability * 0.85
			report.Proposals[importance] = fsrsProposal{
				CurrentStability:  cfg.InitialStability,
				ProposedStability: math.Round(proposed*10) / 10,
				Reason:            fmt.Sprintf("%.0f%% of %s facts are over-retained (stability>1000, conf>0.99, access<3). Reducing InitialStability by 15%%.", overRetainedPct*100, importance),
			}
		} else if overForgottenPct > 0.20 {
			proposed := cfg.InitialStability * 1.15
			// Clamp to 50% increase max
			maxProposed := cfg.InitialStability * 1.5
			if proposed > maxProposed {
				proposed = maxProposed
			}
			report.Proposals[importance] = fsrsProposal{
				CurrentStability:  cfg.InitialStability,
				ProposedStability: math.Round(proposed*10) / 10,
				Reason:            fmt.Sprintf("%.0f%% of %s facts are over-forgotten (hit floor, never recalled). Increasing InitialStability by 15%%.", overForgottenPct*100, importance),
			}
		}
	}

	return report, nil
}

// FormatTuneReport formats the analysis as a human-readable report.
func FormatTuneReport(report *FSRSTuneReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# FSRS Tuning Analysis\n\nGenerated: %s\nTotal active facts: %d\n\n",
		report.GeneratedAt.Format(time.RFC3339), report.TotalFacts))

	sb.WriteString("## Retention by Importance\n\n")
	sb.WriteString("| Tier | Count | Over-Retained | Over-Forgotten | Avg Stability | Avg Confidence |\n")
	sb.WriteString("|------|-------|---------------|----------------|---------------|----------------|\n")
	for _, imp := range []string{"critical", "high", "medium", "low"} {
		b := report.ByImportance[imp]
		if b == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.1f | %.3f |\n",
			imp, b.Count, b.OverRetained, b.OverForgotten, b.AvgStability, b.AvgConfidence))
	}

	if len(report.Proposals) == 0 {
		sb.WriteString("\n## Proposals\n\nNo adjustments recommended. Current FSRS parameters appear well-calibrated.\n")
	} else {
		sb.WriteString("\n## Proposals\n\n")
		for imp, p := range report.Proposals {
			sb.WriteString(fmt.Sprintf("**%s:** InitialStability %.1f → %.1f\n  %s\n\n", imp, p.CurrentStability, p.ProposedStability, p.Reason))
		}
		sb.WriteString("These are proposals only. Run `memory_tune` with `apply: true` to promote a candidate config.\n")
	}

	return sb.String()
}

// SaveTuneReport writes a versioned tuning report to memory/fsrs-tuning/.
// Reports are never overwritten — each analysis gets its own timestamped file.
func (v *Vault) SaveTuneReport(report *FSRSTuneReport) (string, error) {
	timestamp := report.GeneratedAt.Format("2006-01-02-150405")
	relPath := fmt.Sprintf("memory/fsrs-tuning/%s-report.md", timestamp)
	path := v.Path("memory", "fsrs-tuning", timestamp+"-report.md")

	fm := map[string]interface{}{
		"type":        "fsrs-tuning-report",
		"generated":   report.GeneratedAt.Format(time.RFC3339),
		"total_facts": report.TotalFacts,
		"status":      "proposed",
	}

	// Add proposal summary to frontmatter
	if len(report.Proposals) > 0 {
		var proposalKeys []string
		for k := range report.Proposals {
			proposalKeys = append(proposalKeys, k)
		}
		fm["proposals_for"] = proposalKeys
	}

	body := FormatTuneReport(report)

	if err := markdown.Write(path, fm, body); err != nil {
		return "", err
	}
	return relPath, nil
}

// ApplyTuneReport promotes a tuning report's proposals into the active FSRS config.
// Writes the new config to memory/fsrs-config.md with a reference to the source report.
// This is the explicit apply step — it must be called separately from analysis.
func (v *Vault) ApplyTuneReport(report *FSRSTuneReport) error {
	if len(report.Proposals) == 0 {
		return fmt.Errorf("no proposals to apply")
	}
	decision, stage, err := trust.ClassifyAtCurrentStage(v.Dir, trust.Request{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "fsrs_tuning",
		ActionClass:        trust.ActionPolicyTuningChange,
		TargetDomain:       "memory/fsrs-config.md",
		TouchedState:       []trust.StateClass{trust.StateOperational, trust.StateKnowledge},
		RequestedAuthority: ledger.ScopeApprovedTuningApplication,
	})
	if err != nil {
		return err
	}
	if !trust.Permits(decision, true) {
		return fmt.Errorf("FSRS tuning apply blocked by trust gate: %s", decision.Reason)
	}

	for importance, proposal := range report.Proposals {
		cfg, ok := fsrsConfigs[importance]
		if !ok {
			continue
		}
		cfg.InitialStability = proposal.ProposedStability
		fsrsConfigs[importance] = cfg
	}

	// Persist to vault
	fm := map[string]interface{}{
		"type":       "fsrs-config",
		"applied_at": time.Now().Format(time.RFC3339),
		"version":    report.GeneratedAt.Format("2006-01-02-150405"),
	}

	var body strings.Builder
	body.WriteString("# Active FSRS Configuration\n\n")
	body.WriteString("Applied from tuning report. Edit this file to manually override.\n\n")
	for _, imp := range []string{"high", "medium", "low"} {
		cfg := fsrsConfigs[imp]
		body.WriteString(fmt.Sprintf("## %s\n- InitialStability: %.1f\n- InitialDifficulty: %.1f\n- Floor: %.2f\n\n",
			imp, cfg.InitialStability, cfg.InitialDifficulty, cfg.Floor))
	}

	if err := markdown.Write(v.Path("memory", "fsrs-config.md"), fm, body.String()); err != nil {
		return err
	}
	return ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "fsrs_tuning",
		AuthorityScope: ledger.ScopeApprovedTuningApplication,
		ActionClass:    ledger.ActionPolicyTuningApply,
		TargetDomain:   "memory/fsrs-config.md",
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"fsrs_config_updated"},
		ProofRefs:      []string{"memory/fsrs-config.md"},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "fsrs_tuning",
			StaffingContext:    report.GeneratedAt.Format(time.RFC3339),
			AuthorityScope:     ledger.ScopeApprovedTuningApplication,
			ArtifactState:      "canonical",
			SourceRefs:         []string{"memory/fsrs-config.md"},
			PromotionStatus:    "approved",
			ProofRef:           "fsrs-tune-apply:" + report.GeneratedAt.Format("20060102T150405"),
		},
		Metadata: map[string]interface{}{
			"classifier_stage": stage,
			"proposal_count":   len(report.Proposals),
			"version":          fm["version"],
			"trust_decision":   string(decision.Decision),
		},
	})
}

// LoadTunedFSRS reads memory/fsrs-config.md and overrides the default FSRS parameters.
// Called at startup. If the file doesn't exist, defaults are used.
func (v *Vault) LoadTunedFSRS() error {
	path := v.Path("memory", "fsrs-config.md")
	if !fileExists(path) {
		return nil
	}

	doc, err := markdown.Parse(path)
	if err != nil {
		return nil // non-fatal — use defaults
	}

	// Parse each importance tier from the body
	// Format: "## high\n- InitialStability: 180.0\n..."
	for _, imp := range []string{"high", "medium", "low"} {
		cfg, ok := fsrsConfigs[imp]
		if !ok {
			continue
		}

		section := extractSection(doc.Body, imp)
		if section == "" {
			continue
		}

		if val := extractFloat(section, "InitialStability"); val > 0 {
			cfg.InitialStability = val
		}
		if val := extractFloat(section, "InitialDifficulty"); val >= 0 {
			cfg.InitialDifficulty = val
		}
		if val := extractFloat(section, "Floor"); val >= 0 {
			cfg.Floor = val
		}
		fsrsConfigs[imp] = cfg
	}

	return nil
}

// extractSection finds content under a "## name" heading.
func extractSection(body, name string) string {
	marker := "## " + name
	idx := strings.Index(strings.ToLower(body), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(marker):]
	// Find next section or end
	if nextIdx := strings.Index(rest[1:], "\n## "); nextIdx >= 0 {
		rest = rest[:nextIdx+1]
	}
	return rest
}

// extractFloat finds "- Key: 123.4" in a section and returns the float.
func extractFloat(section, key string) float64 {
	marker := key + ":"
	idx := strings.Index(section, marker)
	if idx < 0 {
		return -1
	}
	rest := strings.TrimSpace(section[idx+len(marker):])
	// Read until newline
	if nlIdx := strings.IndexByte(rest, '\n'); nlIdx >= 0 {
		rest = rest[:nlIdx]
	}
	rest = strings.TrimSpace(rest)
	var val float64
	fmt.Sscanf(rest, "%f", &val)
	return val
}

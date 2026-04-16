// Package maintain provides vault health maintenance: consolidation, contradiction
// detection, and bootstrap fact extraction. All outputs are review artifacts stored
// under memory/maintenance/ — nothing is silently rewritten.
package maintain

import (
	"fmt"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/vault"
)

// Mode selects which maintenance pass to run.
type Mode string

const (
	ModeConsolidate Mode = "consolidate"
	ModeContradict  Mode = "contradict"
	ModeBootstrap   Mode = "bootstrap"
	ModeReplay      Mode = "replay"
	ModeStructural  Mode = "structural"
	ModeHot         Mode = "hot"
	ModeElder       Mode = "elder"
	ModeAll         Mode = "all"
	ModeApply       Mode = "apply"
)

// Report holds the result of a maintenance run.
type Report struct {
	Mode               Mode
	Consolidated       int
	Contradicted       int
	Bootstrapped       int
	Replayed           int
	StructuralReviewed int
	HotReviewed        int
	ElderReviewed      int
	StructuralApplied  int
	TemporalApplied    int
	ElderApplied       int
	Actions            []string
	Duration           time.Duration
	GeneratedAt        time.Time
}

// Run dispatches maintenance by mode. useLLM is accepted for future Phase 1
// integration but is currently unused — all passes use heuristic mode.
func Run(v *vault.Vault, mode Mode, useLLM bool) (*Report, error) {
	start := time.Now()
	report := &Report{
		Mode:        mode,
		GeneratedAt: start,
	}

	switch mode {
	case ModeConsolidate:
		n, actions, err := Consolidate(v)
		if err != nil {
			return nil, fmt.Errorf("consolidate: %w", err)
		}
		report.Consolidated = n
		report.Actions = actions

	case ModeContradict:
		n, actions, err := Contradict(v)
		if err != nil {
			return nil, fmt.Errorf("contradict: %w", err)
		}
		report.Contradicted = n
		report.Actions = actions

	case ModeBootstrap:
		n, actions, err := Bootstrap(v)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: %w", err)
		}
		report.Bootstrapped = n
		report.Actions = actions

	case ModeReplay:
		n, actions, err := Replay(v)
		if err != nil {
			return nil, fmt.Errorf("replay: %w", err)
		}
		report.Replayed = n
		report.Actions = actions

	case ModeStructural:
		n, actions, err := ReviewStructuralLinks(v)
		if err != nil {
			return nil, fmt.Errorf("structural review: %w", err)
		}
		report.StructuralReviewed = n
		report.Actions = actions

	case ModeHot:
		n, actions, err := ReviewHotTier(v)
		if err != nil {
			return nil, fmt.Errorf("hot review: %w", err)
		}
		report.HotReviewed = n
		report.Actions = actions

	case ModeElder:
		n, actions, err := ReviewElderTier(v)
		if err != nil {
			return nil, fmt.Errorf("elder review: %w", err)
		}
		report.ElderReviewed = n
		report.Actions = actions

	case ModeApply:
		result, err := ApplyApproved(v)
		if err != nil {
			return nil, fmt.Errorf("apply: %w", err)
		}
		report.Consolidated = result.MergesApplied
		report.Contradicted = result.ContradictionsResolved
		report.Bootstrapped = result.BootstrapPromoted
		report.Replayed = result.ReplayPromoted
		report.StructuralApplied = result.StructuralTransitionsApplied
		report.TemporalApplied = result.TemporalTransitionsApplied
		report.ElderApplied = result.ElderTransitionsApplied
		report.Actions = result.Actions

	case ModeAll:
		n1, a1, err := Consolidate(v)
		if err != nil {
			return nil, fmt.Errorf("consolidate: %w", err)
		}
		report.Consolidated = n1
		report.Actions = append(report.Actions, a1...)

		n2, a2, err := Contradict(v)
		if err != nil {
			return nil, fmt.Errorf("contradict: %w", err)
		}
		report.Contradicted = n2
		report.Actions = append(report.Actions, a2...)

		n3, a3, err := Bootstrap(v)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: %w", err)
		}
		report.Bootstrapped = n3
		report.Actions = append(report.Actions, a3...)

		n4, a4, err := Replay(v)
		if err != nil {
			return nil, fmt.Errorf("replay: %w", err)
		}
		report.Replayed = n4
		report.Actions = append(report.Actions, a4...)

		n5, a5, err := ReviewStructuralLinks(v)
		if err != nil {
			return nil, fmt.Errorf("structural review: %w", err)
		}
		report.StructuralReviewed = n5
		report.Actions = append(report.Actions, a5...)

		n6, a6, err := ReviewHotTier(v)
		if err != nil {
			return nil, fmt.Errorf("hot review: %w", err)
		}
		report.HotReviewed = n6
		report.Actions = append(report.Actions, a6...)

		n7, a7, err := ReviewElderTier(v)
		if err != nil {
			return nil, fmt.Errorf("elder review: %w", err)
		}
		report.ElderReviewed = n7
		report.Actions = append(report.Actions, a7...)

	default:
		return nil, fmt.Errorf("unknown maintenance mode: %s", mode)
	}

	report.Duration = time.Since(start)
	return report, nil
}

// FormatReport renders a maintenance report as markdown.
func FormatReport(r *Report) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Maintenance Report\n\nMode: %s\nGenerated: %s\nDuration: %s\n\n",
		r.Mode, r.GeneratedAt.Format(time.RFC3339), r.Duration.Round(time.Millisecond)))

	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- Consolidation candidates: %d\n", r.Consolidated))
	sb.WriteString(fmt.Sprintf("- Contradiction candidates: %d\n", r.Contradicted))
	sb.WriteString(fmt.Sprintf("- Bootstrap candidates: %d\n", r.Bootstrapped))
	sb.WriteString(fmt.Sprintf("- Replay promotion candidates: %d\n", r.Replayed))
	sb.WriteString(fmt.Sprintf("- Structural link review candidates: %d\n", r.StructuralReviewed))
	sb.WriteString(fmt.Sprintf("- Hot-tier review candidates: %d\n", r.HotReviewed))
	sb.WriteString(fmt.Sprintf("- Elder-memory review candidates: %d\n", r.ElderReviewed))
	sb.WriteString(fmt.Sprintf("- Structural link transitions applied: %d\n", r.StructuralApplied))
	sb.WriteString(fmt.Sprintf("- Temporal transitions applied: %d\n", r.TemporalApplied))
	sb.WriteString(fmt.Sprintf("- Elder-memory transitions applied: %d\n", r.ElderApplied))

	if len(r.Actions) > 0 {
		sb.WriteString("\n## Actions\n\n")
		for _, a := range r.Actions {
			sb.WriteString(fmt.Sprintf("- %s\n", a))
		}
	} else {
		sb.WriteString("\nNo maintenance actions needed.\n")
	}

	sb.WriteString("\nAll outputs are review artifacts under `memory/maintenance/`. Nothing was rewritten.\n")
	return sb.String()
}

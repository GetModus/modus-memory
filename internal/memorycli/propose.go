package memorycli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/maintain"
	"github.com/GetModus/modus-memory/internal/vault"
)

const (
	ProposeHotUsage        = "propose-hot --fact-path <memory/facts/...> --temperature <hot|warm> --reason \"...\""
	ProposeStructuralUsage = "propose-structural --fact-path <memory/facts/...> [--related-fact <path>] [--related-episode <path>] [--related-entity <name>] [--related-mission <name>] --reason \"...\""
	ProposeTemporalUsage   = "propose-temporal --fact-path <memory/facts/...> --status <active|superseded|expired> --reason \"...\" [--observed-at <RFC3339>] [--valid-from <RFC3339>] [--valid-to <RFC3339>] [--superseded-by <memory/facts/...>]"
	ProposeElderUsage      = "propose-elder --fact-path <memory/facts/...> --protection-class <elder|ordinary> --reason \"...\""
)

type ProposalResult struct {
	ArtifactPath string
	Message      string
}

func ProposeHot(vaultDir string, args []string) (ProposalResult, error) {
	fs := flag.NewFlagSet("propose-hot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	factPath := fs.String("fact-path", "", "relative fact path under memory/facts")
	temperature := fs.String("temperature", "hot", "proposed memory temperature: hot or warm")
	reason := fs.String("reason", "", "why this fact should change admission tier")
	reviewClass := fs.String("review-class", "manual", "review class recorded on the artifact")

	if err := fs.Parse(args); err != nil {
		return ProposalResult{}, err
	}
	if strings.TrimSpace(*factPath) == "" || strings.TrimSpace(*reason) == "" {
		return ProposalResult{}, fmt.Errorf("usage: %s", ProposeHotUsage)
	}

	artifactPath, err := maintain.CreateHotMemoryTransitionCandidate(vault.New(vaultDir, nil), maintain.HotMemoryTransitionCandidate{
		FactPath:            strings.TrimSpace(*factPath),
		ProposedTemperature: strings.TrimSpace(*temperature),
		Reason:              strings.TrimSpace(*reason),
		ReviewClass:         strings.TrimSpace(*reviewClass),
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "memory_cli_hot_proposal",
		StaffingContext:     "operator_cli",
		AuthorityScope:      ledger.ScopeCandidateHotMemoryReview,
		ProofRef:            "cli-hot-memory-transition:" + strings.TrimSpace(*factPath),
	})
	if err != nil {
		return ProposalResult{}, err
	}

	return ProposalResult{
		ArtifactPath: artifactPath,
		Message:      fmt.Sprintf("Hot-memory proposal written at %s. Review the artifact, set status: approved when appropriate, then run memory maintenance apply.", artifactPath),
	}, nil
}

func ProposeStructural(vaultDir string, args []string) (ProposalResult, error) {
	fs := flag.NewFlagSet("propose-structural", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	factPath := fs.String("fact-path", "", "relative fact path under memory/facts")
	reason := fs.String("reason", "", "why these structural links should be added")
	reviewClass := fs.String("review-class", "manual", "review class recorded on the artifact")

	var relatedFacts stringListFlag
	var relatedEpisodes stringListFlag
	var relatedEntities stringListFlag
	var relatedMissions stringListFlag
	fs.Var(&relatedFacts, "related-fact", "related fact path; may be repeated or comma-separated")
	fs.Var(&relatedEpisodes, "related-episode", "related episode path; may be repeated or comma-separated")
	fs.Var(&relatedEntities, "related-entity", "related atlas entity ref; may be repeated or comma-separated")
	fs.Var(&relatedMissions, "related-mission", "related mission ref; may be repeated or comma-separated")

	if err := fs.Parse(args); err != nil {
		return ProposalResult{}, err
	}
	if strings.TrimSpace(*factPath) == "" || strings.TrimSpace(*reason) == "" {
		return ProposalResult{}, fmt.Errorf("usage: %s", ProposeStructuralUsage)
	}
	if len(relatedFacts) == 0 && len(relatedEpisodes) == 0 && len(relatedEntities) == 0 && len(relatedMissions) == 0 {
		return ProposalResult{}, fmt.Errorf("usage: %s", ProposeStructuralUsage)
	}

	artifactPath, err := maintain.CreateStructuralLinkTransitionCandidate(vault.New(vaultDir, nil), maintain.StructuralLinkTransitionCandidate{
		FactPath:                    strings.TrimSpace(*factPath),
		ProposedRelatedFactPaths:    relatedFacts.Values(),
		ProposedRelatedEpisodePaths: relatedEpisodes.Values(),
		ProposedRelatedEntityRefs:   relatedEntities.Values(),
		ProposedRelatedMissionRefs:  relatedMissions.Values(),
		Reason:                      strings.TrimSpace(*reason),
		ReviewClass:                 strings.TrimSpace(*reviewClass),
		ProducingOffice:             "memory_governance",
		ProducingSubsystem:          "memory_cli_structural_proposal",
		StaffingContext:             "operator_cli",
		AuthorityScope:              ledger.ScopeCandidateStructuralLinkReview,
		ProofRef:                    "cli-structural-link-transition:" + strings.TrimSpace(*factPath),
	})
	if err != nil {
		return ProposalResult{}, err
	}

	return ProposalResult{
		ArtifactPath: artifactPath,
		Message:      fmt.Sprintf("Structural-link proposal written at %s. Review the artifact, set status: approved when appropriate, then run memory maintenance apply.", artifactPath),
	}, nil
}

func ProposeTemporal(vaultDir string, args []string) (ProposalResult, error) {
	fs := flag.NewFlagSet("propose-temporal", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	factPath := fs.String("fact-path", "", "relative fact path under memory/facts")
	status := fs.String("status", "", "proposed temporal status: active, superseded, or expired")
	reason := fs.String("reason", "", "why this fact should change temporal posture")
	observedAt := fs.String("observed-at", "", "optional RFC3339 observed_at timestamp")
	validFrom := fs.String("valid-from", "", "optional RFC3339 valid_from timestamp")
	validTo := fs.String("valid-to", "", "optional RFC3339 valid_to timestamp")
	supersededBy := fs.String("superseded-by", "", "relative newer fact path when superseding")
	reviewClass := fs.String("review-class", "manual", "review class recorded on the artifact")

	if err := fs.Parse(args); err != nil {
		return ProposalResult{}, err
	}
	if strings.TrimSpace(*factPath) == "" || strings.TrimSpace(*status) == "" || strings.TrimSpace(*reason) == "" {
		return ProposalResult{}, fmt.Errorf("usage: %s", ProposeTemporalUsage)
	}

	artifactPath, err := maintain.CreateFactTemporalTransitionCandidate(vault.New(vaultDir, nil), maintain.FactTemporalTransitionCandidate{
		FactPath:               strings.TrimSpace(*factPath),
		ProposedTemporalStatus: strings.TrimSpace(*status),
		Reason:                 strings.TrimSpace(*reason),
		ObservedAt:             strings.TrimSpace(*observedAt),
		ValidFrom:              strings.TrimSpace(*validFrom),
		ValidTo:                strings.TrimSpace(*validTo),
		SupersededByPath:       strings.TrimSpace(*supersededBy),
		ReviewClass:            strings.TrimSpace(*reviewClass),
		ProducingOffice:        "memory_governance",
		ProducingSubsystem:     "memory_cli_temporal_proposal",
		StaffingContext:        "operator_cli",
		AuthorityScope:         ledger.ScopeCandidateFactTemporalReview,
		ProofRef:               "cli-temporal-transition:" + strings.TrimSpace(*factPath),
	})
	if err != nil {
		return ProposalResult{}, err
	}

	return ProposalResult{
		ArtifactPath: artifactPath,
		Message:      fmt.Sprintf("Temporal-transition proposal written at %s. Review the artifact, set status: approved when appropriate, then run memory maintenance apply.", artifactPath),
	}, nil
}

func ProposeElder(vaultDir string, args []string) (ProposalResult, error) {
	fs := flag.NewFlagSet("propose-elder", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	factPath := fs.String("fact-path", "", "relative fact path under memory/facts")
	protectionClass := fs.String("protection-class", "", "proposed protection class: elder or ordinary")
	reason := fs.String("reason", "", "why this fact should change elder protection")
	reviewClass := fs.String("review-class", "manual", "review class recorded on the artifact")

	if err := fs.Parse(args); err != nil {
		return ProposalResult{}, err
	}
	if strings.TrimSpace(*factPath) == "" || strings.TrimSpace(*protectionClass) == "" || strings.TrimSpace(*reason) == "" {
		return ProposalResult{}, fmt.Errorf("usage: %s", ProposeElderUsage)
	}

	artifactPath, err := maintain.CreateElderMemoryTransitionCandidate(vault.New(vaultDir, nil), maintain.ElderMemoryTransitionCandidate{
		FactPath:                strings.TrimSpace(*factPath),
		ProposedProtectionClass: strings.TrimSpace(*protectionClass),
		Reason:                  strings.TrimSpace(*reason),
		ReviewClass:             strings.TrimSpace(*reviewClass),
		ProducingOffice:         "memory_governance",
		ProducingSubsystem:      "memory_cli_elder_proposal",
		StaffingContext:         "operator_cli",
		AuthorityScope:          ledger.ScopeCandidateElderMemoryReview,
		ProofRef:                "cli-elder-memory-transition:" + strings.TrimSpace(*factPath),
	})
	if err != nil {
		return ProposalResult{}, err
	}

	return ProposalResult{
		ArtifactPath: artifactPath,
		Message:      fmt.Sprintf("Elder-memory proposal written at %s. Review the artifact, set status: approved when appropriate, then run memory maintenance apply.", artifactPath),
	}, nil
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		*f = append(*f, item)
	}
	return nil
}

func (f *stringListFlag) Values() []string {
	seen := make(map[string]bool, len(*f))
	var out []string
	for _, value := range *f {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

package maintain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/vault"
)

type replayObservation struct {
	Subject        string
	Predicate      string
	Value          string
	SourceRefs     []string
	EpisodePaths   []string
	RecallPaths    []string
	SourceEventIDs []string
	LineageIDs     []string
	CueTerms       []string
	Missions       []string
	WorkItemIDs    []string
	Environments   []string
}

// Replay scans episodes and recall receipts for repeated evidence that should
// become an explicit semantic promotion candidate. It never mutates canonical
// facts directly; it emits review artifacts under memory/maintenance/.
func Replay(v *vault.Vault) (int, []string, error) {
	episodes, err := markdown.ScanDir(v.Path("memory", "episodes"))
	if err != nil {
		return 0, nil, err
	}

	existingFacts := make(map[string]bool)
	facts, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err == nil {
		for _, doc := range facts {
			if doc.Get("archived") == "true" {
				continue
			}
			key := replayCandidateKey(doc.Get("subject"), doc.Get("predicate"), strings.TrimSpace(doc.Body))
			if key != "" {
				existingFacts[key] = true
			}
		}
	}

	pending := pendingReplayKeys(v)
	observations := make(map[string]*replayObservation)

	for _, doc := range episodes {
		if doc.Get("archived") == "true" {
			continue
		}
		episodePath := filepathToVaultPath(v.Dir, doc.Path)
		sourceEventID := strings.TrimSpace(doc.Get("event_id"))
		lineageID := strings.TrimSpace(doc.Get("lineage_id"))
		mission := strings.TrimSpace(doc.Get("mission"))
		workItemID := strings.TrimSpace(doc.Get("work_item_id"))
		environment := strings.TrimSpace(doc.Get("environment"))
		cueTerms := stringSliceField(doc.Frontmatter["cue_terms"])

		for _, fact := range extractFactCandidates(doc.Body) {
			key := replayCandidateKey(fact.subject, fact.predicate, fact.value)
			if key == "" {
				continue
			}
			obs := observations[key]
			if obs == nil {
				obs = &replayObservation{
					Subject:   fact.subject,
					Predicate: fact.predicate,
					Value:     fact.value,
				}
				observations[key] = obs
			}
			obs.SourceRefs = append(obs.SourceRefs, episodePath)
			obs.EpisodePaths = append(obs.EpisodePaths, episodePath)
			obs.SourceEventIDs = append(obs.SourceEventIDs, sourceEventID)
			obs.LineageIDs = append(obs.LineageIDs, lineageID)
			obs.CueTerms = append(obs.CueTerms, cueTerms...)
			obs.Missions = append(obs.Missions, mission)
			obs.WorkItemIDs = append(obs.WorkItemIDs, workItemID)
			obs.Environments = append(obs.Environments, environment)
		}
	}

	recalls, err := markdown.ScanDir(v.Path("memory", "recalls"))
	if err == nil {
		for _, doc := range recalls {
			recallPath := filepathToVaultPath(v.Dir, doc.Path)
			sourceEventIDs := stringSliceField(doc.Frontmatter["source_event_ids"])
			lineageIDs := stringSliceField(doc.Frontmatter["lineage_ids"])
			cueTerms := append(stringSliceField(doc.Frontmatter["cue_terms"]), stringSliceField(doc.Frontmatter["route_cue_terms"])...)
			missions := stringSliceField(doc.Frontmatter["route_missions"])
			workItemID := firstNonEmpty(doc.Get("work_item_id"), doc.Get("route_work_item_id"))
			environment := firstNonEmpty(doc.Get("route_environment"), doc.Get("environment"))

			for _, obs := range observations {
				if !intersects(obs.SourceEventIDs, sourceEventIDs) && !intersects(obs.LineageIDs, lineageIDs) {
					continue
				}
				obs.SourceRefs = append(obs.SourceRefs, recallPath)
				obs.RecallPaths = append(obs.RecallPaths, recallPath)
				obs.CueTerms = append(obs.CueTerms, cueTerms...)
				obs.Missions = append(obs.Missions, missions...)
				obs.WorkItemIDs = append(obs.WorkItemIDs, workItemID)
				obs.Environments = append(obs.Environments, environment)
			}
		}
	}

	keys := make([]string, 0, len(observations))
	for key := range observations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	candidates := 0
	var actions []string
	for _, key := range keys {
		obs := observations[key]
		obs.SourceRefs = uniqueStrings(obs.SourceRefs)
		obs.EpisodePaths = uniqueStrings(obs.EpisodePaths)
		obs.RecallPaths = uniqueStrings(obs.RecallPaths)
		obs.SourceEventIDs = uniqueStrings(obs.SourceEventIDs)
		obs.LineageIDs = uniqueStrings(obs.LineageIDs)
		obs.CueTerms = uniqueStrings(obs.CueTerms)
		obs.Missions = uniqueStrings(obs.Missions)
		obs.WorkItemIDs = uniqueStrings(obs.WorkItemIDs)
		obs.Environments = uniqueStrings(obs.Environments)

		episodeCount := len(obs.EpisodePaths)
		recallCount := len(obs.RecallPaths)
		if !qualifiesReplayPromotion(episodeCount, recallCount) {
			continue
		}
		if existingFacts[key] || pending[key] {
			continue
		}

		err := WriteBootstrapCandidate(v, BootstrapCandidate{
			ArtifactType:       "candidate_replay_fact",
			Subject:            obs.Subject,
			Predicate:          obs.Predicate,
			Value:              obs.Value,
			SourcePath:         firstNonEmpty(obs.EpisodePaths...),
			SourceRefs:         obs.SourceRefs,
			Confidence:         replayConfidence(episodeCount, recallCount),
			Importance:         replayImportance(episodeCount, recallCount),
			Method:             "replay-consensus",
			SourceEventID:      singleConsensus(obs.SourceEventIDs),
			LineageID:          singleConsensus(obs.LineageIDs),
			CueTerms:           obs.CueTerms,
			Mission:            singleConsensus(obs.Missions),
			WorkItemID:         singleConsensus(obs.WorkItemIDs),
			Environment:        singleConsensus(obs.Environments),
			EvidenceEpisodes:   episodeCount,
			EvidenceRecalls:    recallCount,
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "replay_review",
			StaffingContext:    fmt.Sprintf("episodes_%d_recalls_%d", episodeCount, recallCount),
			ProofRef:           "replay-candidate:" + key,
		})
		if err != nil {
			actions = append(actions, fmt.Sprintf("ERROR writing replay candidate for %s %s: %v", obs.Subject, obs.Predicate, err))
			continue
		}
		candidates++
		actions = append(actions, fmt.Sprintf("candidate_replay_fact: %s %s → %s (%d episodes, %d recalls)", obs.Subject, obs.Predicate, truncate(obs.Value, 60), episodeCount, recallCount))
		pending[key] = true
	}

	return candidates, actions, nil
}

func replayCandidateKey(subject, predicate, value string) string {
	subject = strings.ToLower(strings.TrimSpace(subject))
	predicate = strings.ToLower(strings.TrimSpace(predicate))
	value = strings.ToLower(strings.TrimSpace(value))
	if subject == "" || predicate == "" || value == "" {
		return ""
	}
	return subject + "|" + predicate + "|" + value
}

func pendingReplayKeys(v *vault.Vault) map[string]bool {
	keys := make(map[string]bool)
	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return keys
	}
	for _, doc := range docs {
		if doc.Get("type") != "candidate_replay_fact" {
			continue
		}
		status := strings.TrimSpace(doc.Get("status"))
		if status == "rejected" || status == "applied" {
			continue
		}
		key := replayCandidateKey(doc.Get("subject"), doc.Get("predicate"), doc.Get("value"))
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

func qualifiesReplayPromotion(episodeCount, recallCount int) bool {
	return episodeCount >= 2 || (episodeCount >= 1 && recallCount >= 2)
}

func replayConfidence(episodeCount, recallCount int) float64 {
	confidence := 0.55 + float64(episodeCount)*0.12 + float64(recallCount)*0.08
	if confidence > 0.92 {
		return 0.92
	}
	return confidence
}

func replayImportance(episodeCount, recallCount int) string {
	if episodeCount >= 3 || recallCount >= 3 {
		return "high"
	}
	return "medium"
}

func singleConsensus(values []string) string {
	unique := uniqueStrings(values)
	if len(unique) == 1 {
		return unique[0]
	}
	return ""
}

func intersects(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	index := make(map[string]bool, len(left))
	for _, value := range left {
		value = strings.TrimSpace(value)
		if value != "" {
			index[value] = true
		}
	}
	for _, value := range right {
		value = strings.TrimSpace(value)
		if value != "" && index[value] {
			return true
		}
	}
	return false
}

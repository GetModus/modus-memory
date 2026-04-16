package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trust"
)

// EpisodeWriteAuthority describes the authority and provenance behind an
// episodic memory write. Episodes preserve raw or near-raw traces and are the
// barcode-like identity surface behind later semantic memory.
type EpisodeWriteAuthority struct {
	ProducingOffice     string
	ProducingSubsystem  string
	StaffingContext     string
	AuthorityScope      string
	TargetDomain        string
	Source              string
	SourceRef           string
	SourceRefs          []string
	ProofRef            string
	PromotionStatus     string
	EventID             string
	LineageID           string
	EventKind           string
	Subject             string
	CueTerms            []string
	AllowApproval       bool
	Mission             string
	WorkItemID          string
	Environment         string
	MemorySecurityClass string
	RelatedFactPaths    []string
	RelatedEpisodePaths []string
	RelatedEntityRefs   []string
	RelatedMissionRefs  []string
}

func deriveEpisodeSecurityClass(auth EpisodeWriteAuthority) string {
	if strings.TrimSpace(auth.MemorySecurityClass) != "" {
		return normalizeMemorySecurityClass(auth.MemorySecurityClass)
	}
	if strings.TrimSpace(auth.Mission) != "" || strings.TrimSpace(auth.WorkItemID) != "" {
		return "canonical"
	}
	switch strings.ToLower(strings.TrimSpace(auth.EventKind)) {
	case "decision", "correction":
		return "canonical"
	default:
		return "operational"
	}
}

func normalizeCueTerms(values []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func newEpisodeID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return fmt.Sprintf("evt-%d", time.Now().UTC().UnixNano())
}

func contentHash(body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
	return hex.EncodeToString(sum[:])
}

// StoreEpisodeGoverned writes a first-class episodic memory object. Episodes are
// append-only raw or near-raw traces that later semantic memory can cite by
// source_event_id and lineage_id.
func (v *Vault) StoreEpisodeGoverned(body string, auth EpisodeWriteAuthority) (string, string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", fmt.Errorf("empty episode body")
	}

	decision, stage, err := trust.ClassifyAtCurrentStage(v.Dir, trust.Request{
		ProducingOffice:    auth.ProducingOffice,
		ProducingSubsystem: auth.ProducingSubsystem,
		ActionClass:        trust.ActionCanonicalMemoryMutation,
		TargetDomain:       firstNonEmpty(auth.TargetDomain, "memory/episodes"),
		TouchedState:       []trust.StateClass{trust.StateKnowledge},
		RequestedAuthority: auth.AuthorityScope,
		HasPromotionPath:   auth.AllowApproval,
	})
	if err != nil {
		return "", "", err
	}
	if !trust.Permits(decision, auth.AllowApproval) {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:             auth.ProducingOffice,
			Subsystem:          auth.ProducingSubsystem,
			AuthorityScope:     auth.AuthorityScope,
			ActionClass:        string(trust.ActionCanonicalMemoryMutation),
			TargetDomain:       firstNonEmpty(auth.TargetDomain, "memory/episodes"),
			ResultStatus:       ledger.ResultBlocked,
			Decision:           string(decision.Decision),
			SuggestedTransform: decision.SuggestedTransformation,
			Metadata: map[string]interface{}{
				"subject":          auth.Subject,
				"event_kind":       auth.EventKind,
				"classifier_stage": stage,
				"reason":           decision.Reason,
			},
		})
		return "", "", fmt.Errorf("memory episode write blocked by trust gate: %s", decision.Reason)
	}

	eventID := strings.TrimSpace(auth.EventID)
	if eventID == "" {
		eventID = newEpisodeID()
	}
	lineageID := strings.TrimSpace(auth.LineageID)
	if lineageID == "" {
		lineageID = eventID
	}
	eventKind := firstNonEmpty(strings.TrimSpace(auth.EventKind), "observation")
	subject := strings.TrimSpace(auth.Subject)
	now := time.Now().UTC().Format(time.RFC3339)
	hash := contentHash(body)

	relPath := filepath.ToSlash(filepath.Join("memory", "episodes", eventID+".md"))
	path := v.Path("memory", "episodes", eventID+".md")
	sourceRefs := dedupeNonEmpty(auth.SourceRefs...)
	if auth.SourceRef != "" {
		sourceRefs = dedupeNonEmpty(append(sourceRefs, auth.SourceRef)...)
	}
	sourceRefs = dedupeNonEmpty(append(sourceRefs, relPath)...)

	fm := map[string]interface{}{
		"type":                  "memory_episode",
		"event_id":              eventID,
		"lineage_id":            lineageID,
		"event_kind":            eventKind,
		"content_hash":          hash,
		"created":               now,
		"created_at":            now,
		"memory_type":           "episodic",
		"memory_security_class": deriveEpisodeSecurityClass(auth),
		"promotion_status":      firstNonEmpty(strings.TrimSpace(auth.PromotionStatus), "observed"),
	}
	if subject != "" {
		fm["subject"] = subject
	}
	if auth.Source != "" {
		fm["source"] = auth.Source
	}
	if auth.SourceRef != "" {
		fm["source_ref"] = auth.SourceRef
	}
	if len(sourceRefs) > 0 {
		fm["source_lineage"] = sourceRefs
	}
	if auth.ProducingOffice != "" {
		fm["captured_by_office"] = auth.ProducingOffice
	}
	if auth.ProducingSubsystem != "" {
		fm["captured_by_subsystem"] = auth.ProducingSubsystem
	}
	if auth.StaffingContext != "" {
		fm["staffing_context"] = auth.StaffingContext
	}
	if mission := strings.TrimSpace(auth.Mission); mission != "" {
		fm["mission"] = mission
	}
	if workItemID := strings.TrimSpace(auth.WorkItemID); workItemID != "" {
		fm["work_item_id"] = workItemID
	}
	if environment := strings.TrimSpace(auth.Environment); environment != "" {
		fm["environment"] = environment
	}
	if cueTerms := normalizeCueTerms(auth.CueTerms); len(cueTerms) > 0 {
		fm["cue_terms"] = cueTerms
	}
	if relatedFactPaths := dedupeNonEmpty(auth.RelatedFactPaths...); len(relatedFactPaths) > 0 {
		fm["related_fact_paths"] = relatedFactPaths
	}
	if relatedEpisodePaths := dedupeNonEmpty(auth.RelatedEpisodePaths...); len(relatedEpisodePaths) > 0 {
		fm["related_episode_paths"] = relatedEpisodePaths
	}
	if relatedEntityRefs := dedupeNonEmpty(auth.RelatedEntityRefs...); len(relatedEntityRefs) > 0 {
		fm["related_entity_refs"] = relatedEntityRefs
	}
	if relatedMissionRefs := dedupeNonEmpty(auth.RelatedMissionRefs...); len(relatedMissionRefs) > 0 {
		fm["related_mission_refs"] = relatedMissionRefs
	}

	if err := markdown.Write(path, fm, body); err != nil {
		return "", "", err
	}

	_ = ledger.Append(v.Dir, ledger.Record{
		Office:         firstNonEmpty(auth.ProducingOffice, "librarian"),
		Subsystem:      firstNonEmpty(auth.ProducingSubsystem, "memory_episode_store"),
		AuthorityScope: firstNonEmpty(auth.AuthorityScope, ledger.ScopeRuntimeMemoryStore),
		ActionClass:    ledger.ActionMemoryEpisodeCreation,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       string(decision.Decision),
		SideEffects:    []string{"memory_episode_created"},
		ProofRefs:      sourceRefs,
		Signature: signature.Signature{
			ProducingOffice:    firstNonEmpty(auth.ProducingOffice, "librarian"),
			ProducingSubsystem: firstNonEmpty(auth.ProducingSubsystem, "memory_episode_store"),
			StaffingContext:    auth.StaffingContext,
			AuthorityScope:     firstNonEmpty(auth.AuthorityScope, ledger.ScopeRuntimeMemoryStore),
			ArtifactState:      "canonical",
			SourceRefs:         sourceRefs,
			PromotionStatus:    firstNonEmpty(strings.TrimSpace(auth.PromotionStatus), "observed"),
			ProofRef:           firstNonEmpty(auth.ProofRef, "memory-episode:"+eventID),
		},
		Metadata: map[string]interface{}{
			"event_id":     eventID,
			"lineage_id":   lineageID,
			"event_kind":   eventKind,
			"content_hash": hash,
			"subject":      subject,
		},
	})

	return relPath, eventID, nil
}

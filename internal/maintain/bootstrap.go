package maintain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

// Bootstrap scans vault prose (brain/) for extractable facts using heuristic
// regex patterns. Writes candidate_bootstrap_fact artifacts to memory/maintenance/.
// Per Codex revision: scans inside vault only by default. Never ingests content
// outside the declared trust boundary unless the operator requests it.
func Bootstrap(v *vault.Vault) (int, []string, error) {
	// Scan brain/ directory for prose documents
	brainDocs, err := markdown.ScanDir(v.Path("brain"))
	if err != nil {
		// brain/ may not exist — that's fine
		brainDocs = nil
	}

	// Also scan any README files at vault root
	rootDocs, err := markdown.ScanDir(v.Dir)
	if err != nil {
		rootDocs = nil
	}

	// Filter root docs to only README-like files
	var readmes []*markdown.Document
	for _, doc := range rootDocs {
		lower := strings.ToLower(doc.Path)
		if strings.Contains(lower, "readme") || strings.Contains(lower, "about") {
			readmes = append(readmes, doc)
		}
	}

	allDocs := append(brainDocs, readmes...)

	// Load existing facts to deduplicate
	existingFacts, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		existingFacts = nil
	}
	existingSubjects := make(map[string]bool)
	for _, doc := range existingFacts {
		subj := strings.ToLower(doc.Get("subject"))
		pred := strings.ToLower(doc.Get("predicate"))
		if subj != "" {
			existingSubjects[subj+"|"+pred] = true
		}
	}

	candidates := 0
	var actions []string

	for _, doc := range allDocs {
		extracted := extractFactCandidates(doc.Body)
		for _, fact := range extracted {
			// Dedup: skip if subject+predicate already exists
			key := strings.ToLower(fact.subject) + "|" + strings.ToLower(fact.predicate)
			if existingSubjects[key] {
				continue
			}
			existingSubjects[key] = true // prevent duplicates within this run

			candidates++
			action := fmt.Sprintf("candidate_bootstrap_fact: %s %s → %s (from %s)",
				fact.subject, fact.predicate, truncate(fact.value, 60), doc.Path)
			actions = append(actions, action)

			if err := WriteBootstrapCandidate(v, BootstrapCandidate{
				Subject:            fact.subject,
				Predicate:          fact.predicate,
				Value:              fact.value,
				SourcePath:         doc.Path,
				Confidence:         0.5,
				Importance:         "medium",
				Method:             "heuristic-regex",
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "maintenance_bootstrap",
				StaffingContext:    "heuristic_bootstrap",
				ProofRef:           "bootstrap-candidate:" + doc.Path,
			}); err != nil {
				actions = append(actions, fmt.Sprintf("ERROR writing bootstrap candidate: %v", err))
			}
		}
	}

	return candidates, actions, nil
}

type extractedFact struct {
	subject   string
	predicate string
	value     string
}

// BootstrapCandidate is a review-only fact proposal emitted by autonomous or
// heuristic discovery machinery. It never becomes canonical without approval.
type BootstrapCandidate struct {
	ArtifactType       string
	Subject            string
	Predicate          string
	Value              string
	SourcePath         string
	SourceRefs         []string
	Confidence         float64
	Importance         string
	Method             string
	SourceEventID      string
	LineageID          string
	CueTerms           []string
	Mission            string
	WorkItemID         string
	Environment        string
	EvidenceEpisodes   int
	EvidenceRecalls    int
	ProducingOffice    string
	ProducingSubsystem string
	StaffingContext    string
	ProofRef           string
}

// extractFactCandidates uses heuristic regex to find fact-like statements in prose.
var (
	// "X uses Y", "X is built with Y", "X runs on Y"
	techStackRE = regexp.MustCompile(`(?i)\b([A-Z][a-zA-Z0-9_-]+)\s+(?:uses?|is built (?:with|on|using)|runs? on|powered by|written in|built (?:with|in|on))\s+([A-Za-z0-9_./ -]+)`)
	// "X is a Y", "X is the Y"
	definitionRE = regexp.MustCompile(`(?i)\b([A-Z][a-zA-Z0-9_-]+)\s+is\s+(?:a|an|the)\s+([A-Za-z0-9_ -]{3,50})`)
	// "version X.Y.Z" or "vX.Y.Z"
	versionRE = regexp.MustCompile(`(?i)\b([A-Z][a-zA-Z0-9_-]+)\s+v?(\d+\.\d+(?:\.\d+)?)\b`)
)

func extractFactCandidates(body string) []extractedFact {
	var facts []extractedFact
	seen := make(map[string]bool)

	for _, match := range techStackRE.FindAllStringSubmatch(body, 20) {
		subj := strings.TrimSpace(match[1])
		value := strings.TrimSpace(match[2])
		key := strings.ToLower(subj + "|uses|" + value)
		if seen[key] || len(value) < 2 {
			continue
		}
		seen[key] = true
		facts = append(facts, extractedFact{subject: subj, predicate: "uses", value: value})
	}

	for _, match := range definitionRE.FindAllStringSubmatch(body, 20) {
		subj := strings.TrimSpace(match[1])
		value := strings.TrimSpace(match[2])
		key := strings.ToLower(subj + "|is|" + value)
		if seen[key] || len(value) < 3 {
			continue
		}
		seen[key] = true
		facts = append(facts, extractedFact{subject: subj, predicate: "is", value: value})
	}

	for _, match := range versionRE.FindAllStringSubmatch(body, 10) {
		subj := strings.TrimSpace(match[1])
		version := strings.TrimSpace(match[2])
		key := strings.ToLower(subj + "|version")
		if seen[key] {
			continue
		}
		seen[key] = true
		facts = append(facts, extractedFact{subject: subj, predicate: "version", value: version})
	}

	return facts
}

// WriteBootstrapCandidate emits a signed bootstrap review artifact.
func WriteBootstrapCandidate(v *vault.Vault, candidate BootstrapCandidate) error {
	timestamp := time.Now().Format("2006-01-02-150405")
	slug := fmt.Sprintf("%s-bootstrap-%s-%s", timestamp, slugify(candidate.Subject), slugify(candidate.Predicate))
	if len(slug) > 120 {
		slug = slug[:120]
	}
	path := v.Path("memory", "maintenance", slug+".md")
	relPath := filepathToVaultPath(v.Dir, path)

	confidence := candidate.Confidence
	if confidence <= 0 {
		confidence = 0.5
	}
	importance := strings.TrimSpace(candidate.Importance)
	if importance == "" {
		importance = "medium"
	}
	method := strings.TrimSpace(candidate.Method)
	if method == "" {
		method = "bootstrap-candidate"
	}
	artifactType := strings.TrimSpace(candidate.ArtifactType)
	if artifactType == "" {
		artifactType = "candidate_bootstrap_fact"
	}
	office := strings.TrimSpace(candidate.ProducingOffice)
	if office == "" {
		office = "memory_governance"
	}
	subsystem := strings.TrimSpace(candidate.ProducingSubsystem)
	if subsystem == "" {
		subsystem = "maintenance_bootstrap"
	}
	sourceRefs := uniqueStrings(append([]string{candidate.SourcePath}, candidate.SourceRefs...))

	fm := map[string]interface{}{
		"type":       artifactType,
		"status":     "pending",
		"created":    time.Now().Format(time.RFC3339),
		"subject":    candidate.Subject,
		"predicate":  candidate.Predicate,
		"value":      candidate.Value,
		"source":     candidate.SourcePath,
		"confidence": confidence,
		"importance": importance,
		"method":     method,
		"producing_signature": signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     ledger.ScopeCandidateBootstrapGen,
			ArtifactState:      "candidate",
			SourceRefs:         sourceRefs,
			PromotionStatus:    "pending",
			ProofRef:           candidate.ProofRef,
		}.EnsureTimestamp(),
	}
	if len(sourceRefs) > 0 {
		fm["source_refs"] = sourceRefs
	}
	if candidate.SourceEventID != "" {
		fm["source_event_id"] = candidate.SourceEventID
	}
	if candidate.LineageID != "" {
		fm["lineage_id"] = candidate.LineageID
	}
	if cueTerms := uniqueStrings(candidate.CueTerms); len(cueTerms) > 0 {
		fm["cue_terms"] = cueTerms
	}
	if candidate.Mission != "" {
		fm["mission"] = candidate.Mission
	}
	if candidate.WorkItemID != "" {
		fm["work_item_id"] = candidate.WorkItemID
	}
	if candidate.Environment != "" {
		fm["environment"] = candidate.Environment
	}
	if candidate.EvidenceEpisodes > 0 {
		fm["evidence_episode_count"] = candidate.EvidenceEpisodes
	}
	if candidate.EvidenceRecalls > 0 {
		fm["evidence_recall_count"] = candidate.EvidenceRecalls
	}

	var body strings.Builder
	if artifactType == "candidate_replay_fact" {
		body.WriteString("# Replay Promotion Candidate\n\n")
	} else {
		body.WriteString("# Bootstrap Fact Candidate\n\n")
	}
	body.WriteString(fmt.Sprintf("**%s** %s → %s\n\n", candidate.Subject, candidate.Predicate, candidate.Value))
	body.WriteString(fmt.Sprintf("Extracted from: `%s`\n", candidate.SourcePath))
	body.WriteString(fmt.Sprintf("Method: %s\n\n", method))
	if candidate.EvidenceEpisodes > 0 || candidate.EvidenceRecalls > 0 {
		body.WriteString(fmt.Sprintf("Replay evidence: `%d` episodes, `%d` recall receipts.\n\n", candidate.EvidenceEpisodes, candidate.EvidenceRecalls))
	}
	body.WriteString("To promote: set `status: approved` in frontmatter, then call `memory_maintain` with `mode: apply`.\n")
	body.WriteString("To discard: set `status: rejected`.\n")

	if err := markdown.Write(path, fm, body.String()); err != nil {
		return err
	}
	return ledger.Append(v.Dir, ledger.Record{
		Office:         office,
		Subsystem:      subsystem,
		AuthorityScope: ledger.ScopeCandidateBootstrapGen,
		ActionClass:    ledger.ActionReviewCandidateGeneration,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"bootstrap_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    office,
			ProducingSubsystem: subsystem,
			StaffingContext:    candidate.StaffingContext,
			AuthorityScope:     ledger.ScopeCandidateBootstrapGen,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           candidate.ProofRef,
		},
		Metadata: map[string]interface{}{
			"type":                   artifactType,
			"subject":                candidate.Subject,
			"predicate":              candidate.Predicate,
			"importance":             importance,
			"confidence":             confidence,
			"method":                 method,
			"evidence_episode_count": candidate.EvidenceEpisodes,
			"evidence_recall_count":  candidate.EvidenceRecalls,
		},
	})
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func filepathToVaultPath(vaultDir, absPath string) string {
	rel := strings.TrimPrefix(absPath, vaultDir)
	return strings.TrimPrefix(rel, "/")
}

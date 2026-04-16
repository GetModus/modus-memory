package vault

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
)

// StoreCorrection writes a correction pattern to memory/corrections/.
// Corrections are search-time annotations — they never rewrite canonical facts.
// The original phrasing is matched against future queries to surface the corrected version.
func (v *Vault) StoreCorrection(original, corrected, context, createdBy string) (string, error) {
	if original == "" || corrected == "" {
		return "", fmt.Errorf("original and corrected are required")
	}
	if createdBy == "" {
		createdBy = "unknown"
	}

	slug := slugify(original)
	if len(slug) > 80 {
		slug = slug[:80]
	}

	relPath := fmt.Sprintf("memory/corrections/%s.md", slug)
	path := v.Path("memory", "corrections", slug+".md")

	// Handle duplicates
	for i := 2; fileExists(path); i++ {
		slug2 := fmt.Sprintf("%s-%d", slug, i)
		relPath = fmt.Sprintf("memory/corrections/%s.md", slug2)
		path = v.Path("memory", "corrections", slug2+".md")
	}

	fm := map[string]interface{}{
		"original":    original,
		"corrected":   corrected,
		"context":     context,
		"created":     time.Now().Format(time.RFC3339),
		"created_by":  createdBy,
		"confidence":  1.0,
		"apply_count": 0,
		"scope":       "search",
		"producing_signature": signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "corrections",
			StaffingContext:    createdBy,
			AuthorityScope:     ledger.ScopeSearchCorrectionCreation,
			ArtifactState:      "derived",
			SourceRefs:         []string{relPath},
			PromotionStatus:    "approved",
			ProofRef:           "correction:" + slug,
		}.EnsureTimestamp(),
	}

	body := fmt.Sprintf("When \"%s\" is encountered, surface \"%s\" instead.\n\nContext: %s", original, corrected, context)

	if err := markdown.Write(path, fm, body); err != nil {
		return "", err
	}

	impact, err := v.propagateCorrection(relPath, original, corrected, context, createdBy)
	if err != nil {
		return "", err
	}
	_ = ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "corrections",
		AuthorityScope: ledger.ScopeSearchCorrectionCreation,
		ActionClass:    ledger.ActionCorrectionCreation,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"correction_stored"},
		ProofRefs:      []string{relPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "corrections",
			StaffingContext:    createdBy,
			AuthorityScope:     ledger.ScopeSearchCorrectionCreation,
			ArtifactState:      "evidentiary",
			SourceRefs:         []string{relPath},
			PromotionStatus:    "approved",
			ProofRef:           "correction-store:" + slug,
		},
		Metadata: map[string]interface{}{
			"original":                original,
			"corrected":               corrected,
			"scope":                   "search",
			"affected_fact_count":     len(impact.FactPaths),
			"affected_recall_count":   len(impact.RecallPaths),
			"affected_artifact_count": len(impact.ArtifactPaths),
			"impact_artifact_path":    impact.ArtifactPath,
		},
	})
	return relPath, nil
}

type correctionImpact struct {
	ArtifactPath  string
	FactPaths     []string
	RecallPaths   []string
	ArtifactPaths []string
}

func (v *Vault) propagateCorrection(correctionPath, original, corrected, context, createdBy string) (correctionImpact, error) {
	original = strings.TrimSpace(original)
	if original == "" {
		return correctionImpact{}, nil
	}
	factPaths, err := v.flagCorrectionAffectedDocs("memory/facts", original, correctionPath, nil)
	if err != nil {
		return correctionImpact{}, err
	}

	factIndex := make(map[string]bool, len(factPaths))
	for _, path := range factPaths {
		factIndex[path] = true
	}
	recallPaths, err := v.flagCorrectionAffectedDocs("memory/recalls", original, correctionPath, func(doc *markdown.Document, relPath string) bool {
		if documentContainsNeedle(doc, original) {
			return true
		}
		for _, selected := range stringSliceFrontmatter(doc.Frontmatter["selected_paths"]) {
			if factIndex[selected] {
				return true
			}
		}
		return false
	})
	if err != nil {
		return correctionImpact{}, err
	}
	artifactPaths, err := v.flagCorrectionAffectedDocs("memory/maintenance", original, correctionPath, func(doc *markdown.Document, relPath string) bool {
		if doc.Get("type") == "candidate_correction_propagation" {
			return false
		}
		if documentContainsNeedle(doc, original) {
			return true
		}
		for _, ref := range append(stringSliceFrontmatter(doc.Frontmatter["source_refs"]), doc.Get("fact_path"), doc.Get("proposed_path"), doc.Get("competing_path"), doc.Get("weaker_path")) {
			if factIndex[strings.TrimSpace(ref)] {
				return true
			}
		}
		return false
	})
	if err != nil {
		return correctionImpact{}, err
	}

	artifactPath, err := v.writeCorrectionImpactArtifact(correctionPath, original, corrected, context, createdBy, factPaths, recallPaths, artifactPaths)
	if err != nil {
		return correctionImpact{}, err
	}
	return correctionImpact{
		ArtifactPath:  artifactPath,
		FactPaths:     factPaths,
		RecallPaths:   recallPaths,
		ArtifactPaths: artifactPaths,
	}, nil
}

func (v *Vault) flagCorrectionAffectedDocs(subdir, original, correctionPath string, matchFn func(*markdown.Document, string) bool) ([]string, error) {
	docs, err := markdown.ScanDir(v.Path(strings.Split(subdir, "/")...))
	if err != nil {
		if fileNotFoundPath(err) {
			return nil, nil
		}
		return nil, err
	}
	var affected []string
	for _, doc := range docs {
		relPath, err := filepath.Rel(v.Dir, doc.Path)
		if err != nil {
			return nil, err
		}
		relPath = filepath.ToSlash(relPath)
		if relPath == correctionPath {
			continue
		}
		matched := false
		if matchFn != nil {
			matched = matchFn(doc, relPath)
		} else {
			matched = documentContainsNeedle(doc, original)
		}
		if !matched {
			continue
		}
		flagDocForCorrectionReview(doc, correctionPath)
		if err := doc.Save(); err != nil {
			return nil, err
		}
		affected = append(affected, relPath)
	}
	return uniqueStringList(affected), nil
}

func flagDocForCorrectionReview(doc *markdown.Document, correctionPath string) {
	now := time.Now().Format(time.RFC3339)
	refs := uniqueStringList(append(stringSliceFrontmatter(doc.Frontmatter["correction_refs"]), correctionPath))
	doc.Set("correction_review_status", "pending")
	doc.Set("stale_due_to_correction", true)
	doc.Set("last_correction_flagged_at", now)
	doc.Set("correction_refs", refs)
}

func (v *Vault) writeCorrectionImpactArtifact(correctionPath, original, corrected, context, createdBy string, factPaths, recallPaths, artifactPaths []string) (string, error) {
	timestamp := time.Now().Format("2006-01-02-150405")
	slug := slugify(original)
	if len(slug) > 60 {
		slug = slug[:60]
	}
	relPath := fmt.Sprintf("memory/maintenance/%s-correction-impact-%s.md", timestamp, slug)
	path := v.Path("memory", "maintenance", filepath.Base(relPath))

	sourceRefs := uniqueStringList(append([]string{correctionPath}, append(append([]string{}, factPaths...), append(recallPaths, artifactPaths...)...)...))
	fm := map[string]interface{}{
		"type":                    "candidate_correction_propagation",
		"status":                  "pending",
		"created":                 time.Now().Format(time.RFC3339),
		"correction_path":         correctionPath,
		"original":                original,
		"corrected":               corrected,
		"context":                 context,
		"created_by":              createdBy,
		"affected_fact_paths":     factPaths,
		"affected_recall_paths":   recallPaths,
		"affected_artifact_paths": artifactPaths,
		"affected_total":          len(factPaths) + len(recallPaths) + len(artifactPaths),
		"producing_signature": signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "correction_propagation",
			StaffingContext:    createdBy,
			AuthorityScope:     ledger.ScopeCandidateCorrectionReview,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           "correction-propagation:" + correctionPath,
		}.EnsureTimestamp(),
	}

	var body strings.Builder
	body.WriteString("# Correction Propagation Review\n\n")
	body.WriteString(fmt.Sprintf("Correction: `%s` -> `%s`\n\n", original, corrected))
	if strings.TrimSpace(context) != "" {
		body.WriteString(fmt.Sprintf("Context: %s\n\n", context))
	}
	body.WriteString("This artifact marks memory objects whose current content or references may still reflect the corrected statement. The memory objects were flagged non-destructively with `correction_review_status: pending`.\n\n")
	body.WriteString(fmt.Sprintf("Affected facts: `%d`\n", len(factPaths)))
	body.WriteString(fmt.Sprintf("Affected recall receipts: `%d`\n", len(recallPaths)))
	body.WriteString(fmt.Sprintf("Affected maintenance artifacts: `%d`\n\n", len(artifactPaths)))
	if len(factPaths) > 0 {
		body.WriteString("## Affected Facts\n\n")
		for _, item := range factPaths {
			body.WriteString(fmt.Sprintf("- `%s`\n", item))
		}
		body.WriteString("\n")
	}
	if len(recallPaths) > 0 {
		body.WriteString("## Affected Recall Receipts\n\n")
		for _, item := range recallPaths {
			body.WriteString(fmt.Sprintf("- `%s`\n", item))
		}
		body.WriteString("\n")
	}
	if len(artifactPaths) > 0 {
		body.WriteString("## Affected Maintenance Artifacts\n\n")
		for _, item := range artifactPaths {
			body.WriteString(fmt.Sprintf("- `%s`\n", item))
		}
		body.WriteString("\n")
	}
	body.WriteString("Next action: inspect the flagged items, re-derive or correct where needed, and clear the review status only through an explicit follow-up path.\n")

	if err := markdown.Write(path, fm, body.String()); err != nil {
		return "", err
	}
	if err := ledger.Append(v.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "correction_propagation",
		AuthorityScope: ledger.ScopeCandidateCorrectionReview,
		ActionClass:    ledger.ActionReviewCandidateGeneration,
		TargetDomain:   relPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"correction_propagation_candidate_created"},
		ProofRefs:      append(sourceRefs, relPath),
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "correction_propagation",
			StaffingContext:    createdBy,
			AuthorityScope:     ledger.ScopeCandidateCorrectionReview,
			ArtifactState:      "candidate",
			SourceRefs:         append(sourceRefs, relPath),
			PromotionStatus:    "pending",
			ProofRef:           "correction-propagation:" + correctionPath,
		}.EnsureTimestamp(),
		Metadata: map[string]interface{}{
			"correction_path":         correctionPath,
			"affected_fact_count":     len(factPaths),
			"affected_recall_count":   len(recallPaths),
			"affected_artifact_count": len(artifactPaths),
		},
	}); err != nil {
		return "", err
	}
	return relPath, nil
}

func documentContainsNeedle(doc *markdown.Document, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	for _, value := range documentTextValues(doc) {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func documentTextValues(doc *markdown.Document) []string {
	values := []string{doc.Body}
	for _, value := range doc.Frontmatter {
		values = append(values, flattenTextValues(value)...)
	}
	return values
}

func flattenTextValues(value interface{}) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, flattenTextValues(item)...)
		}
		return out
	case []interface{}:
		var out []string
		for _, item := range typed {
			out = append(out, flattenTextValues(item)...)
		}
		return out
	case map[string]interface{}:
		var out []string
		for _, item := range typed {
			out = append(out, flattenTextValues(item)...)
		}
		return out
	default:
		if value == nil {
			return nil
		}
		return []string{fmt.Sprint(value)}
	}
}

func uniqueStringList(values []string) []string {
	seen := make(map[string]bool)
	var out []string
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

func fileNotFoundPath(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no such file or directory")
}

// FindCorrections scans memory/corrections/ for corrections whose original
// field matches the query. Returns matching correction documents.
func (v *Vault) FindCorrections(query string) ([]*markdown.Document, error) {
	dir := v.Path("memory", "corrections")
	docs, err := markdown.ScanDir(dir)
	if err != nil {
		// Directory may not exist yet
		return nil, nil
	}

	queryLower := strings.ToLower(query)
	var matches []*markdown.Document
	for _, doc := range docs {
		original := strings.ToLower(doc.Get("original"))
		if original == "" {
			continue
		}
		// Match if query contains the original phrasing or vice versa
		if strings.Contains(queryLower, original) || strings.Contains(original, queryLower) {
			matches = append(matches, doc)
		}
	}
	return matches, nil
}

// FormatCorrectionHints formats matching corrections as search-time annotations.
// These are prepended to search results as hints, not silent rewrites.
func (v *Vault) FormatCorrectionHints(query string) string {
	corrections, err := v.FindCorrections(query)
	if err != nil || len(corrections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("**Correction notes:**\n")
	applied := 0
	for _, doc := range corrections {
		original := doc.Get("original")
		corrected := doc.Get("corrected")
		sb.WriteString(fmt.Sprintf("- \"%s\" → \"%s\"", original, corrected))
		if ctx := doc.Get("context"); ctx != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", ctx))
		}
		sb.WriteByte('\n')

		// Increment apply_count
		ac := doc.GetFloat("apply_count")
		doc.Set("apply_count", int(ac)+1)
		doc.Set("last_applied", time.Now().Format(time.RFC3339))
		if err := doc.Save(); err == nil {
			applied++
		}
	}
	sb.WriteByte('\n')

	if applied > 0 {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "corrections",
			AuthorityScope: ledger.ScopeSearchCorrectionApplication,
			ActionClass:    ledger.ActionCorrectionApplication,
			TargetDomain:   "memory/corrections",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionAllowedWithProof,
			SideEffects:    []string{"correction_hints_emitted"},
			ProofRefs:      []string{"memory/corrections"},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "corrections",
				StaffingContext:    query,
				AuthorityScope:     ledger.ScopeSearchCorrectionApplication,
				ArtifactState:      "reflective",
				SourceRefs:         []string{"memory/corrections"},
				PromotionStatus:    "advisory",
				ProofRef:           "correction-apply",
			},
			Metadata: map[string]interface{}{
				"query":         query,
				"applied_count": applied,
			},
		})
	}
	return sb.String()
}

// ListCorrections returns all correction documents, most recent first.
func (v *Vault) ListCorrections(limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 20
	}
	dir := v.Path("memory", "corrections")
	docs, err := markdown.ScanDir(dir)
	if err != nil {
		return nil, nil
	}
	if len(docs) > limit {
		docs = docs[:limit]
	}
	return docs, nil
}

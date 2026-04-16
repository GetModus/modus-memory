package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/GetModus/modus-memory/internal/markdown"
)

const (
	VerificationModeCritical = "critical"

	VerificationStatusVerified       = "verified"
	VerificationStatusReviewRequired = "review_required"
	VerificationStatusMismatch       = "mismatch"
	VerificationStatusUnverified     = "unverified"
	VerificationStatusSourceMissing  = "source_missing"
)

type FactVerificationResult struct {
	FactPath           string
	Status             string
	Note               string
	SourceRefs         []string
	ReviewedSourceRefs []string
	VerifiedSourceRefs []string
}

func normalizeVerificationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "verify", "verified", "high_stakes", "high-stakes":
		return VerificationModeCritical
	default:
		return ""
	}
}

func verificationSourceRefsForFact(doc *markdown.Document) []string {
	var refs []string
	if eventID := strings.TrimSpace(doc.Get("source_event_id")); eventID != "" {
		refs = append(refs, filepath.ToSlash(filepath.Join("memory", "episodes", eventID+".md")))
	}
	if sourceRef := normalizeVerificationSourceRef(doc.Get("source_ref")); sourceRef != "" {
		refs = append(refs, sourceRef)
	}
	for _, ref := range stringSliceFrontmatter(doc.Frontmatter["source_lineage"]) {
		if normalized := normalizeVerificationSourceRef(ref); normalized != "" {
			refs = append(refs, normalized)
		}
	}
	return uniqueStringList(refs)
}

func normalizeVerificationSourceRef(ref string) string {
	ref = filepath.ToSlash(strings.TrimSpace(ref))
	if ref == "" {
		return ""
	}
	return strings.TrimPrefix(ref, "vault/")
}

func (v *Vault) readVerificationSourceText(ref string) (string, string, error) {
	ref = normalizeVerificationSourceRef(ref)
	if ref == "" {
		return "", "", os.ErrNotExist
	}
	abs := ref
	if filepath.IsAbs(abs) {
		rel, err := filepath.Rel(v.Dir, abs)
		if err == nil {
			ref = filepath.ToSlash(rel)
		}
	} else {
		abs = filepath.Join(v.Dir, filepath.FromSlash(ref))
	}

	if strings.HasSuffix(strings.ToLower(abs), ".md") {
		doc, err := markdown.Parse(abs)
		if err != nil {
			return ref, "", err
		}
		return ref, strings.Join(documentTextValues(doc), "\n"), nil
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return ref, "", err
	}
	return ref, string(data), nil
}

func normalizeVerificationText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func sourceTextSupportsFact(sourceText, subject, value string, cueTerms []string) (bool, bool) {
	sourceText = normalizeVerificationText(sourceText)
	subject = normalizeVerificationText(subject)
	value = normalizeVerificationText(value)
	if sourceText == "" {
		return false, false
	}

	contextMatch := false
	if subject != "" && strings.Contains(sourceText, subject) {
		contextMatch = true
	}
	for _, cue := range cueTerms {
		cue = normalizeVerificationText(cue)
		if cue != "" && strings.Contains(sourceText, cue) {
			contextMatch = true
			break
		}
	}
	if value == "" {
		return false, contextMatch
	}
	if strings.Contains(sourceText, value) {
		return true, true
	}
	return false, contextMatch
}

func (v *Vault) verifyFactHit(relPath string, doc *markdown.Document, mode string) FactVerificationResult {
	result := FactVerificationResult{
		FactPath:   relPath,
		SourceRefs: verificationSourceRefsForFact(doc),
	}
	if normalizeVerificationMode(mode) == "" {
		return result
	}
	if strings.EqualFold(strings.TrimSpace(doc.Get("correction_review_status")), "pending") ||
		strings.EqualFold(strings.TrimSpace(doc.Get("stale_due_to_correction")), "true") {
		result.Status = VerificationStatusReviewRequired
		result.Note = "correction review pending"
		return result
	}
	if len(result.SourceRefs) == 0 {
		result.Status = VerificationStatusSourceMissing
		result.Note = "no canonical source reference available"
		return result
	}

	subject := doc.Get("subject")
	value := strings.TrimSpace(doc.Body)
	cueTerms := docCueTerms(doc)
	sawContext := false
	for _, ref := range result.SourceRefs {
		resolvedRef, sourceText, err := v.readVerificationSourceText(ref)
		if err != nil {
			continue
		}
		result.ReviewedSourceRefs = append(result.ReviewedSourceRefs, resolvedRef)
		verified, context := sourceTextSupportsFact(sourceText, subject, value, cueTerms)
		if context {
			sawContext = true
		}
		if verified {
			result.VerifiedSourceRefs = append(result.VerifiedSourceRefs, resolvedRef)
		}
	}

	result.ReviewedSourceRefs = uniqueStringList(result.ReviewedSourceRefs)
	result.VerifiedSourceRefs = uniqueStringList(result.VerifiedSourceRefs)
	if len(result.VerifiedSourceRefs) > 0 {
		result.Status = VerificationStatusVerified
		result.Note = fmt.Sprintf("supported by %s", strings.Join(result.VerifiedSourceRefs, ", "))
		return result
	}
	if len(result.ReviewedSourceRefs) == 0 {
		result.Status = VerificationStatusSourceMissing
		result.Note = "cited source could not be reopened"
		return result
	}
	if sawContext {
		result.Status = VerificationStatusMismatch
		result.Note = "source context reopened but stored claim text was not directly supported"
		return result
	}
	result.Status = VerificationStatusUnverified
	result.Note = "source reopened but no direct textual support was found"
	return result
}

func formatVerificationAnnotation(result FactVerificationResult) string {
	switch result.Status {
	case VerificationStatusVerified:
		return "verification verified"
	case VerificationStatusReviewRequired:
		return "verification review required"
	case VerificationStatusMismatch:
		return "verification mismatch"
	case VerificationStatusUnverified:
		return "verification unverified"
	case VerificationStatusSourceMissing:
		return "verification source missing"
	default:
		return ""
	}
}

func appendVerificationAnnotation(line string, result FactVerificationResult) string {
	annotation := formatVerificationAnnotation(result)
	if annotation == "" {
		return line
	}
	return fmt.Sprintf("%s [%s]", line, annotation)
}

func verificationFrontmatterMap(result FactVerificationResult) map[string]interface{} {
	fm := map[string]interface{}{
		"fact_path":   result.FactPath,
		"status":      result.Status,
		"source_refs": result.SourceRefs,
	}
	if len(result.ReviewedSourceRefs) > 0 {
		fm["reviewed_source_refs"] = result.ReviewedSourceRefs
	}
	if len(result.VerifiedSourceRefs) > 0 {
		fm["verified_source_refs"] = result.VerifiedSourceRefs
	}
	if strings.TrimSpace(result.Note) != "" {
		fm["note"] = strings.TrimSpace(result.Note)
	}
	return fm
}

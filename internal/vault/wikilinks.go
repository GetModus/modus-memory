package vault

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GetModus/modus-memory/internal/markdown"
)

type WikiLinkIssue struct {
	SourcePath string
	Raw        string
	Target     string
	Reason     string
}

type WikiLinkRewrite struct {
	SourcePath string
	Before     string
	After      string
}

type WikiLinkAudit struct {
	Documents   int
	Links       int
	Rewrites    []WikiLinkRewrite
	Issues      []WikiLinkIssue
	UpdatedDocs int
}

func (v *Vault) AuditWikiLinks(write bool) (*WikiLinkAudit, error) {
	docs, err := markdown.ScanDir(v.Dir)
	if err != nil {
		return nil, err
	}

	audit := &WikiLinkAudit{Documents: len(docs)}
	for _, doc := range docs {
		rel, err := filepath.Rel(v.Dir, doc.Path)
		if err != nil {
			rel = doc.Path
		}

		body, rewrites, issues := v.reconcileDocumentWikiLinks(doc.Body, filepath.ToSlash(rel))
		audit.Links += len(rewrites) + len(issues)
		audit.Rewrites = append(audit.Rewrites, rewrites...)
		audit.Issues = append(audit.Issues, issues...)

		if write && body != doc.Body {
			doc.Body = body
			if err := doc.Save(); err != nil {
				return nil, fmt.Errorf("save %s: %w", rel, err)
			}
			audit.UpdatedDocs++
		}
	}

	return audit, nil
}

func (v *Vault) reconcileDocumentWikiLinks(body, sourcePath string) (string, []WikiLinkRewrite, []WikiLinkIssue) {
	var rewrites []WikiLinkRewrite
	var issues []WikiLinkIssue
	var out strings.Builder
	last := 0

	for _, span := range findWikiLinkSpans(body) {
		out.WriteString(body[last:span.open])

		rawInner := body[span.open+2 : span.close]
		rawTarget, label, anchor := parseWikiLinkParts(rawInner)
		resolved := v.ResolveWikiLink(rawTarget)
		if resolved == "" && rawTarget != "" {
			issues = append(issues, WikiLinkIssue{
				SourcePath: sourcePath,
				Raw:        strings.TrimSpace(rawInner),
				Target:     rawTarget,
				Reason:     "unresolved",
			})
			out.WriteString(body[span.open : span.close+2])
			last = span.close + 2
			continue
		}

		rewritten := rewriteWikiLink(rawInner, rawTarget, resolved, label, anchor)
		if rewritten != strings.TrimSpace(rawInner) {
			rewrites = append(rewrites, WikiLinkRewrite{
				SourcePath: sourcePath,
				Before:     strings.TrimSpace(rawInner),
				After:      rewritten,
			})
		}
		out.WriteString("[[")
		out.WriteString(rewritten)
		out.WriteString("]]")
		last = span.close + 2
	}
	if len(body) > last {
		out.WriteString(body[last:])
	}

	return out.String(), rewrites, issues
}

func parseWikiLinkParts(raw string) (target, label, anchor string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", ""
	}

	parts := strings.SplitN(trimmed, "|", 2)
	target = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		label = strings.TrimSpace(parts[1])
	}
	if idx := strings.Index(target, "#"); idx >= 0 {
		anchor = strings.TrimSpace(target[idx+1:])
		target = strings.TrimSpace(target[:idx])
	}
	return target, label, anchor
}

func rewriteWikiLink(rawInner, rawTarget, resolved, label, anchor string) string {
	trimmed := strings.TrimSpace(rawInner)
	if resolved == "" || !shouldCanonicalizeWikiLink(rawTarget) {
		return trimmed
	}

	target := strings.TrimSuffix(filepath.ToSlash(resolved), ".md")
	if anchor != "" {
		target += "#" + anchor
	}
	if label != "" {
		target += "|" + label
	}
	return target
}

func shouldCanonicalizeWikiLink(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	return filepath.IsAbs(target) ||
		strings.HasPrefix(target, "vault/") ||
		strings.Contains(target, "/") ||
		strings.HasSuffix(target, ".md")
}

type wikiLinkSpan struct {
	open  int
	close int
}

func findWikiLinkSpans(body string) []wikiLinkSpan {
	var spans []wikiLinkSpan
	inInlineCode := false
	inFence := false
	fenceMarker := ""
	lineStart := true

	for i := 0; i < len(body); {
		if lineStart && !inInlineCode {
			if strings.HasPrefix(body[i:], "```") {
				if inFence && fenceMarker == "```" {
					inFence = false
					fenceMarker = ""
				} else if !inFence {
					inFence = true
					fenceMarker = "```"
				}
				i += 3
				lineStart = false
				continue
			}
			if strings.HasPrefix(body[i:], "~~~") {
				if inFence && fenceMarker == "~~~" {
					inFence = false
					fenceMarker = ""
				} else if !inFence {
					inFence = true
					fenceMarker = "~~~"
				}
				i += 3
				lineStart = false
				continue
			}
		}

		if body[i] == '\n' {
			lineStart = true
			i++
			continue
		}

		if inFence {
			lineStart = false
			i++
			continue
		}

		if body[i] == '`' {
			inInlineCode = !inInlineCode
			lineStart = false
			i++
			continue
		}

		if !inInlineCode && strings.HasPrefix(body[i:], "[[") {
			close := strings.Index(body[i+2:], "]]")
			if close < 0 {
				break
			}
			spans = append(spans, wikiLinkSpan{
				open:  i,
				close: i + 2 + close,
			})
			i += close + 4
			lineStart = false
			continue
		}

		lineStart = false
		i++
	}

	return spans
}

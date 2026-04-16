package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// ListEntities returns all entity documents from atlas/entities/.
func (v *Vault) ListEntities() ([]*markdown.Document, error) {
	return markdown.ScanDir(v.Path("atlas", "entities"))
}

// GetEntity finds an entity by name or slug.
func (v *Vault) GetEntity(name string) (*markdown.Document, error) {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Try exact match
	path := v.Path("atlas", "entities", slug+".md")
	if fileExists(path) {
		return markdown.Parse(path)
	}

	// Search by name in frontmatter
	docs, _ := markdown.ScanDir(v.Path("atlas", "entities"))
	for _, doc := range docs {
		if strings.EqualFold(doc.Get("name"), name) {
			return doc, nil
		}
	}

	return nil, fmt.Errorf("entity %q not found", name)
}

// ListBeliefs returns beliefs, optionally filtered by subject.
func (v *Vault) ListBeliefs(subject string, limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 20
	}

	docs, err := markdown.ScanDir(v.Path("atlas", "beliefs"))
	if err != nil {
		return nil, err
	}

	var result []*markdown.Document
	for _, doc := range docs {
		if len(result) >= limit {
			break
		}
		if subject != "" && !strings.EqualFold(doc.Get("subject"), subject) {
			continue
		}
		result = append(result, doc)
	}
	return result, nil
}

// ResolveWikiLink finds the .md file matching a [[wiki-link]].
func (v *Vault) ResolveWikiLink(link string) string {
	link = normalizeWikiLink(link)
	if link == "" {
		return ""
	}

	if resolved := v.resolveWikiLinkPath(link); resolved != "" {
		return resolved
	}

	prefixes := map[string]string{
		"belief-":  "atlas/beliefs/",
		"entity-":  "atlas/entities/",
		"mission-": "missions/active/",
	}

	for prefix, dir := range prefixes {
		if strings.HasPrefix(link, prefix) {
			slug := strings.TrimPrefix(link, prefix)
			path := v.Path(dir, slug+".md")
			if fileExists(path) {
				return filepath.Join(dir, slug+".md")
			}
			path = v.Path(dir, link+".md")
			if fileExists(path) {
				return filepath.Join(dir, link+".md")
			}
		}
	}

	// Search by entity title first.
	if doc, err := v.GetEntity(link); err == nil {
		rel, _ := filepath.Rel(v.Dir, doc.Path)
		return filepath.ToSlash(rel)
	}

	// Fallback: walk and try title/name/base matches.
	var found string
	filepath.Walk(v.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		doc, _ := markdown.Parse(path)
		title := strings.TrimSpace(doc.Get("title"))
		name := strings.TrimSpace(doc.Get("name"))
		baseSlug := slugify(base)
		linkSlug := slugify(link)
		switch {
		case strings.EqualFold(base, link):
		case baseSlug != "" && baseSlug == linkSlug:
		case strings.EqualFold(title, link):
		case title != "" && slugify(title) == linkSlug:
		case strings.EqualFold(name, link):
		case name != "" && slugify(name) == linkSlug:
		case strings.Contains(strings.ToLower(base), strings.ToLower(link)):
		default:
			return nil
		}
		rel, _ := filepath.Rel(v.Dir, path)
		found = filepath.ToSlash(rel)
		return filepath.SkipAll
	})

	return found
}

func (v *Vault) resolveWikiLinkPath(link string) string {
	pathLike := link

	// Accept absolute filesystem paths anywhere under the vault root.
	if filepath.IsAbs(pathLike) {
		if !strings.HasPrefix(filepath.Clean(pathLike), filepath.Clean(v.Dir)+string(os.PathSeparator)) &&
			filepath.Clean(pathLike) != filepath.Clean(v.Dir) {
			return ""
		}
		return resolveVaultPath(v.Dir, pathLike)
	}

	// Accept both vault-root-relative paths and "vault/..."-prefixed paths.
	if strings.HasPrefix(pathLike, "vault/") {
		pathLike = strings.TrimPrefix(pathLike, "vault/")
	}

	return resolveVaultPath(v.Dir, v.Path(pathLike))
}

func resolveVaultPath(vaultRoot, candidate string) string {
	cleaned := filepath.Clean(candidate)
	if fileExists(cleaned) {
		rel, err := filepath.Rel(vaultRoot, cleaned)
		if err == nil {
			return filepath.ToSlash(rel)
		}
	}

	if !strings.HasSuffix(cleaned, ".md") {
		withExt := cleaned + ".md"
		if fileExists(withExt) {
			rel, err := filepath.Rel(vaultRoot, withExt)
			if err == nil {
				return filepath.ToSlash(rel)
			}
		}
	}

	return ""
}

func normalizeWikiLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if pipe := strings.Index(link, "|"); pipe >= 0 {
		link = strings.TrimSpace(link[:pipe])
	}
	if anchor := strings.Index(link, "#"); anchor >= 0 {
		link = strings.TrimSpace(link[:anchor])
	}
	return link
}

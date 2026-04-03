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

	// Fallback: walk
	var found string
	filepath.Walk(v.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		if base == link || strings.Contains(base, link) {
			rel, _ := filepath.Rel(v.Dir, path)
			found = rel
			return filepath.SkipAll
		}
		return nil
	})

	return found
}

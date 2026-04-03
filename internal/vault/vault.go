package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/markdown"
)

// Vault provides unified access to the entire vault — brain, memory, atlas, missions.
type Vault struct {
	Dir   string
	Index *index.Index
}

// New creates a Vault rooted at the given directory.
func New(dir string, idx *index.Index) *Vault {
	return &Vault{Dir: dir, Index: idx}
}

// Path joins segments onto the vault root.
func (v *Vault) Path(parts ...string) string {
	args := append([]string{v.Dir}, parts...)
	return filepath.Join(args...)
}

// Read parses a single markdown file by relative path.
func (v *Vault) Read(relPath string) (*markdown.Document, error) {
	return markdown.Parse(filepath.Join(v.Dir, relPath))
}

// Write creates or overwrites a markdown file with frontmatter + body.
func (v *Vault) Write(relPath string, fm map[string]interface{}, body string) error {
	path := filepath.Join(v.Dir, relPath)
	os.MkdirAll(filepath.Dir(path), 0755)
	return markdown.Write(path, fm, body)
}

// Filter constrains List results by frontmatter field.
type Filter struct {
	Field   string
	Value   string
	Exclude bool // if true, exclude matches instead of including
}

// List scans a subdirectory for .md files, optionally filtered.
func (v *Vault) List(subdir string, filters ...Filter) ([]*markdown.Document, error) {
	dir := filepath.Join(v.Dir, subdir)
	docs, err := markdown.ScanDir(dir)
	if err != nil {
		return nil, err
	}

	if len(filters) == 0 {
		return docs, nil
	}

	var result []*markdown.Document
	for _, doc := range docs {
		match := true
		for _, f := range filters {
			val := doc.Get(f.Field)
			if f.Exclude {
				if strings.EqualFold(val, f.Value) {
					match = false
					break
				}
			} else {
				if !strings.EqualFold(val, f.Value) {
					match = false
					break
				}
			}
		}
		if match {
			result = append(result, doc)
		}
	}
	return result, nil
}

// Search performs full-text search across the index.
// Returns empty results if no index is loaded.
func (v *Vault) Search(query string, limit int) ([]index.SearchResult, error) {
	if v.Index == nil {
		return nil, fmt.Errorf("no search index loaded — run with index enabled")
	}
	return v.Index.Search(query, limit)
}

// Status returns vault-wide statistics.
func (v *Vault) Status() map[string]interface{} {
	counts := make(map[string]int)
	filepath.Walk(v.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(v.Dir, path)
		parts := strings.SplitN(rel, string(os.PathSeparator), 2)
		counts[parts[0]]++
		return nil
	})

	total := 0
	for _, count := range counts {
		total += count
	}

	indexDocs := 0
	if v.Index != nil {
		indexDocs = v.Index.DocCount()
	}

	result := map[string]interface{}{
		"total_files": total,
		"index_docs":  indexDocs,
		"breakdown":   counts,
	}
	return result
}

// StatusJSON returns Status() as formatted JSON.
func (v *Vault) StatusJSON() (string, error) {
	data, err := json.MarshalIndent(v.Status(), "", "  ")
	return string(data), err
}

// Package index implements pure Go full-text search over vault .md files.
//
// No SQLite. No external dependencies. Documents are loaded into memory,
// tokenized, and indexed with BM25 field-boosted scoring. A tiered query
// cache (exact hash + Jaccard fuzzy) handles the hot path.
//
// On 7,600 .md files (~15MB text), startup takes 1-3 seconds on Apple Silicon.
// Search returns in microseconds for cached queries, milliseconds for cold.
package index

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// Index wraps an in-memory BM25 search engine built from vault .md files.
type Index struct {
	docs     []document
	engine   *bm25Engine
	cache    *queryCache
	facts    *factStore
	cross    *crossIndex // subject/tag/entity cross-references
	meta     map[string]*document // path → document for field lookups
	docCount int
	vaultDir string
}

// document is the internal representation of a vault file.
type document struct {
	Path        string
	Source      string
	Subject     string
	Title       string
	Tags        string
	Body        string
	Predicate   string
	Confidence  float64
	Importance  string
	Kind        string
	Created     string
	Triage      string
	Frontmatter map[string]interface{}
}

// SearchResult represents a single search hit.
type SearchResult struct {
	Path       string
	Source     string
	Subject    string
	Title      string
	Snippet    string
	Rank       float64
	Confidence float64
	Importance string
	Created    string
	Triage     string
}

// Build scans a vault directory and creates an in-memory search index.
func Build(vaultDir string, _ string) (*Index, error) {
	start := time.Now()

	// Scan all markdown files
	mdDocs, err := markdown.ScanDir(vaultDir)
	if err != nil {
		return nil, fmt.Errorf("scan vault: %w", err)
	}

	docs := make([]document, 0, len(mdDocs))
	meta := make(map[string]*document, len(mdDocs))

	for _, md := range mdDocs {
		rel, _ := filepath.Rel(vaultDir, md.Path)
		title := md.Get("title")
		if title == "" {
			title = md.Get("name")
		}
		kind := md.Get("kind")
		if kind == "" {
			kind = md.Get("type")
		}

		doc := document{
			Path:        rel,
			Source:      md.Get("source"),
			Subject:     md.Get("subject"),
			Title:       title,
			Tags:        strings.Join(md.GetTags(), " "),
			Body:        md.Body,
			Predicate:   md.Get("predicate"),
			Confidence:  md.GetFloat("confidence"),
			Importance:  md.Get("importance"),
			Kind:        kind,
			Created:     md.Get("created"),
			Triage:      md.Get("triage"),
			Frontmatter: md.Frontmatter,
		}

		docs = append(docs, doc)
		meta[rel] = &docs[len(docs)-1]
	}

	// Build BM25 inverted index
	engine := newBM25Engine(docs)
	cache := newQueryCache()

	// Build cross-reference index — connects docs sharing subjects, tags, entities
	cross := newCrossIndex()
	cross.build(docs)

	idx := &Index{
		docs:     docs,
		engine:   engine,
		cache:    cache,
		facts:    newFactStore(),
		cross:    cross,
		meta:     meta,
		docCount: len(docs),
		vaultDir: vaultDir,
	}

	// Auto-index memory facts
	idx.indexFacts(vaultDir)

	crossSubjects, crossTags, crossEntities := cross.Stats()
	elapsed := time.Since(start)
	log.Printf("Indexed %d documents in %v (BM25 + cross-ref: %d subjects, %d tags, %d entities)",
		len(docs), elapsed, crossSubjects, crossTags, crossEntities)

	return idx, nil
}

// Open is an alias for Build — there's no persistent index file to open.
// The second argument (indexPath) is ignored; kept for interface compatibility.
func Open(indexPath string) (*Index, error) {
	// Infer vaultDir from indexPath: indexPath was typically ~/modus/data/index.sqlite
	// The vault is at ~/modus/vault/
	home, _ := os.UserHomeDir()
	vaultDir := filepath.Join(home, "modus", "vault")
	if envDir := os.Getenv("MODUS_VAULT_DIR"); envDir != "" {
		vaultDir = envDir
	}
	return Build(vaultDir, "")
}

// Close is a no-op — no file handles to release.
func (idx *Index) Close() {}

// DocCount returns the number of indexed documents.
func (idx *Index) DocCount() int {
	return idx.docCount
}

// Search performs BM25 full-text search with tiered caching and field boosting.
func (idx *Index) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	// Tier 0-1: check cache (exact hash + Jaccard fuzzy)
	if cached, ok := idx.cache.get(query); ok {
		if len(cached) > limit {
			return cached[:limit], nil
		}
		return cached, nil
	}

	// Tier 2: BM25 search
	scored := idx.engine.search(query, limit*2) // oversample for dedup

	// OOD detection — reject garbage results
	if filterOOD(query, scored, 0.15) {
		empty := []SearchResult{}
		idx.cache.put(query, empty)
		return empty, nil
	}

	queryTerms := tokenize(query)
	var results []SearchResult
	seen := make(map[string]bool)

	for _, s := range scored {
		doc := idx.docs[s.docID]
		if seen[doc.Path] {
			continue
		}
		seen[doc.Path] = true

		snip := snippet(doc.Body, queryTerms, 200)
		results = append(results, SearchResult{
			Path:       doc.Path,
			Source:     doc.Source,
			Subject:    doc.Subject,
			Title:      doc.Title,
			Snippet:    snip,
			Rank:       normalizeScore(s.score),
			Confidence: doc.Confidence,
			Importance: doc.Importance,
			Created:    doc.Created,
			Triage:     doc.Triage,
		})

		if len(results) >= limit {
			break
		}
	}

	// Cache the results
	idx.cache.put(query, results)

	return results, nil
}

// SearchByField returns documents where a metadata field matches a value.
func (idx *Index) SearchByField(field string, value string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	var results []SearchResult
	valueLower := strings.ToLower(value)

	for _, doc := range idx.docs {
		var fieldVal string
		switch field {
		case "source":
			fieldVal = doc.Source
		case "subject":
			fieldVal = doc.Subject
		case "kind", "type":
			fieldVal = doc.Kind
		case "triage":
			fieldVal = doc.Triage
		case "importance":
			fieldVal = doc.Importance
		default:
			if v, ok := doc.Frontmatter[field]; ok {
				fieldVal = fmt.Sprintf("%v", v)
			}
		}

		if strings.ToLower(fieldVal) == valueLower {
			results = append(results, SearchResult{
				Path:       doc.Path,
				Source:     doc.Source,
				Subject:    doc.Subject,
				Title:      doc.Predicate,
				Confidence: doc.Confidence,
				Importance: doc.Importance,
				Created:    doc.Created,
				Triage:     doc.Triage,
			})
		}

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// Facts returns the in-memory fact store for agentic search.
func (idx *Index) Facts() *factStore {
	return idx.facts
}

// Connected returns documents cross-referenced to a query via subject, tag, and entity links.
func (idx *Index) Connected(query string, limit int) []DocRef {
	return idx.cross.Connected(query, limit)
}

// CrossRefStats returns cross-index statistics (subjects, tags, entities).
func (idx *Index) CrossRefStats() (int, int, int) {
	return idx.cross.Stats()
}

// SearchFacts searches memory facts by query.
func (idx *Index) SearchFacts(query string, limit int) []MemFact {
	return idx.facts.search(query, limit)
}

// FactsBySubject returns all active facts for a subject.
func (idx *Index) FactsBySubject(subject string, limit int) []MemFact {
	return idx.facts.bySubject(subject, limit)
}

// AllActiveFacts returns all active memory facts.
func (idx *Index) AllActiveFacts(limit int) []MemFact {
	return idx.facts.allActive(limit)
}

// FactCount returns total and active fact counts.
func (idx *Index) FactCount() (total, active int) {
	return idx.facts.count()
}

// indexFacts loads memory facts from vault/memory/facts/ into the fact store.
func (idx *Index) indexFacts(vaultDir string) {
	factsDir := filepath.Join(vaultDir, "memory", "facts")
	docs, err := markdown.ScanDir(factsDir)
	if err != nil {
		return
	}

	count := 0
	for _, doc := range docs {
		rel, _ := filepath.Rel(vaultDir, doc.Path)
		subject := doc.Get("subject")
		predicate := doc.Get("predicate")
		value := doc.Body
		if value == "" {
			value = doc.Get("value")
		}
		if subject == "" || predicate == "" {
			continue
		}

		confidence := doc.GetFloat("confidence")
		if confidence == 0 {
			confidence = 0.8
		}
		importance := doc.Get("importance")
		if importance == "" {
			importance = "medium"
		}
		memType := doc.Get("memory_type")
		if memType == "" {
			memType = "semantic"
		}
		created := doc.Get("created")
		if created == "" {
			created = doc.Get("created_at")
		}
		session := doc.Get("session_tag")
		if session == "" {
			session = "session-1"
		}
		isActive := 1
		if doc.Get("archived") == "true" || doc.Get("is_active") == "0" {
			isActive = 0
		}

		idx.facts.add(MemFact{
			ID:         rel,
			Subject:    subject,
			Predicate:  predicate,
			Value:      value,
			Confidence: confidence,
			Importance: importance,
			MemoryType: memType,
			CreatedAt:  created,
			SessionTag: session,
			IsActive:   isActive,
		})
		count++
	}

	if count > 0 {
		log.Printf("Indexed %d memory facts (in-memory)", count)
	}
}

package index

import (
	"sort"
	"strings"
	"time"
)

// MemFact represents an in-memory memory fact, replacing the SQL memory_facts table.
type MemFact struct {
	ID         string
	Subject    string
	Predicate  string
	Value      string
	Confidence float64
	Importance string
	MemoryType string
	CreatedAt  string
	SessionTag string
	IsActive   int
}

// factStore holds all memory facts in memory with term-based search.
type factStore struct {
	facts    []MemFact
	terms    map[string][]int // stemmed term → fact indices
	subjects map[string][]int // lowercase subject → fact indices
}

func newFactStore() *factStore {
	return &factStore{
		terms:    make(map[string][]int),
		subjects: make(map[string][]int),
	}
}

// add indexes a fact into the store.
func (fs *factStore) add(f MemFact) {
	idx := len(fs.facts)
	fs.facts = append(fs.facts, f)

	// Index all text fields
	content := f.Subject + " " + f.Predicate + " " + f.Value
	for _, token := range tokenize(content) {
		fs.terms[token] = append(fs.terms[token], idx)
	}

	// Subject index for exact lookups
	subj := strings.ToLower(strings.TrimSpace(f.Subject))
	fs.subjects[subj] = append(fs.subjects[subj], idx)
}

// recencyBoost returns a multiplier based on fact age.
// Hot (<24h): 1.5x, Warm (1-7d): 1.2x, Recent (7-30d): 1.0x, Cold (>30d): 0.8x.
// Inspired by Icarus/Hermes tiered memory — hot facts surface first.
func recencyBoost(createdAt string) float64 {
	if createdAt == "" {
		return 1.0
	}
	t, err := time.Parse("2006-01-02", createdAt[:min10(10, len(createdAt))])
	if err != nil {
		// Try RFC3339
		t, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return 1.0
		}
	}
	age := time.Since(t)
	switch {
	case age < 24*time.Hour:
		return 1.5 // hot
	case age < 7*24*time.Hour:
		return 1.2 // warm
	case age < 30*24*time.Hour:
		return 1.0 // recent
	default:
		return 0.8 // cold
	}
}

func min10(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// search finds facts matching a query, ranked by BM25-like scoring with recency boost.
func (fs *factStore) search(query string, limit int) []MemFact {
	if len(fs.facts) == 0 {
		return nil
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Score each fact by term overlap
	scores := make(map[int]float64)
	for _, term := range queryTerms {
		indices, ok := fs.terms[term]
		if !ok {
			continue
		}
		// Simple TF-IDF-like scoring
		idf := 1.0 / (1.0 + float64(len(indices))/float64(len(fs.facts)))
		for _, idx := range indices {
			if fs.facts[idx].IsActive == 0 {
				continue
			}
			scores[idx] += idf
			// Boost by confidence
			scores[idx] += fs.facts[idx].Confidence * 0.1
		}
	}

	// Apply recency boost — hot facts surface before cold ones
	for idx := range scores {
		scores[idx] *= recencyBoost(fs.facts[idx].CreatedAt)
	}

	// Sort by score
	type scored struct {
		idx   int
		score float64
	}
	var ranked []scored
	for idx, score := range scores {
		ranked = append(ranked, scored{idx, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}

	results := make([]MemFact, len(ranked))
	for i, r := range ranked {
		results[i] = fs.facts[r.idx]
	}
	return results
}

// bySubject returns all active facts for a given subject (case-insensitive).
func (fs *factStore) bySubject(subject string, limit int) []MemFact {
	indices := fs.subjects[strings.ToLower(strings.TrimSpace(subject))]
	var results []MemFact
	for _, idx := range indices {
		if fs.facts[idx].IsActive == 1 {
			results = append(results, fs.facts[idx])
		}
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results
}

// allActive returns all active facts, optionally limited.
func (fs *factStore) allActive(limit int) []MemFact {
	var results []MemFact
	for _, f := range fs.facts {
		if f.IsActive == 1 {
			results = append(results, f)
		}
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results
}

// count returns total and active fact counts.
func (fs *factStore) count() (total, active int) {
	total = len(fs.facts)
	for _, f := range fs.facts {
		if f.IsActive == 1 {
			active++
		}
	}
	return
}

// Tier returns the recency tier label: "hot", "warm", "recent", or "cold".
func (f *MemFact) Tier() string {
	if f.CreatedAt == "" {
		return "cold"
	}
	t, err := time.Parse("2006-01-02", f.CreatedAt[:min10(10, len(f.CreatedAt))])
	if err != nil {
		t, err = time.Parse(time.RFC3339, f.CreatedAt)
		if err != nil {
			return "cold"
		}
	}
	age := time.Since(t)
	switch {
	case age < 24*time.Hour:
		return "hot"
	case age < 7*24*time.Hour:
		return "warm"
	case age < 30*24*time.Hour:
		return "recent"
	default:
		return "cold"
	}
}

// StalenessWarning returns a warning if the fact is old.
func (f *MemFact) StalenessWarning() string {
	if f.CreatedAt == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", f.CreatedAt[:10])
	if err != nil {
		return ""
	}
	age := time.Since(t)
	if age > 90*24*time.Hour {
		return "⚠ >90d old"
	}
	if age > 30*24*time.Hour {
		return "⚠ >30d old"
	}
	return ""
}

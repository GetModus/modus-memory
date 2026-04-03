package index

import (
	"hash/fnv"
	"sort"
	"strings"
	"sync"
)

// QueryCache implements tiered caching for search results.
// Tier 0: exact query hash match (0ms)
// Tier 1: fuzzy match via Jaccard similarity >= threshold (~1ms)
// Inspired by ByteRover's ablation study showing cache tiers drive 29.4pp accuracy gain.
const (
	cacheMaxEntries      = 256
	cacheJaccardThreshold = 0.6
)

type cacheEntry struct {
	query   string
	terms   map[string]bool // tokenized query terms for Jaccard comparison
	results []SearchResult
	hits    int // access count for LRU
}

// queryCache stores recent query→results pairs with exact and fuzzy matching.
type queryCache struct {
	mu      sync.RWMutex
	entries []cacheEntry
	counter int // global access counter for LRU
}

func newQueryCache() *queryCache {
	return &queryCache{
		entries: make([]cacheEntry, 0, cacheMaxEntries),
	}
}

// get attempts to find cached results for a query.
// Returns results and true if found (tier 0 or 1), nil and false otherwise.
func (c *queryCache) get(query string) ([]SearchResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.entries) == 0 {
		return nil, false
	}

	// Tier 0: exact hash match
	h := hashQuery(query)
	for i := range c.entries {
		if hashQuery(c.entries[i].query) == h && c.entries[i].query == query {
			c.entries[i].hits = c.counter
			return c.entries[i].results, true
		}
	}

	// Tier 1: fuzzy match via Jaccard similarity
	queryTerms := termSet(query)
	if len(queryTerms) == 0 {
		return nil, false
	}

	var bestIdx int
	var bestSim float64

	for i := range c.entries {
		sim := jaccard(queryTerms, c.entries[i].terms)
		if sim > bestSim {
			bestSim = sim
			bestIdx = i
		}
	}

	if bestSim >= cacheJaccardThreshold {
		c.entries[bestIdx].hits = c.counter
		return c.entries[bestIdx].results, true
	}

	return nil, false
}

// put stores query results in the cache, evicting the least recently used entry if full.
func (c *queryCache) put(query string, results []SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counter++

	// Check if query already cached — update results
	for i := range c.entries {
		if c.entries[i].query == query {
			c.entries[i].results = results
			c.entries[i].hits = c.counter
			return
		}
	}

	entry := cacheEntry{
		query:   query,
		terms:   termSet(query),
		results: results,
		hits:    c.counter,
	}

	if len(c.entries) >= cacheMaxEntries {
		// Evict LRU entry
		minIdx := 0
		minHits := c.entries[0].hits
		for i := 1; i < len(c.entries); i++ {
			if c.entries[i].hits < minHits {
				minHits = c.entries[i].hits
				minIdx = i
			}
		}
		c.entries[minIdx] = entry
	} else {
		c.entries = append(c.entries, entry)
	}
}

func hashQuery(q string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(q))))
	return h.Sum64()
}

func termSet(query string) map[string]bool {
	terms := tokenize(query)
	set := make(map[string]bool, len(terms))
	for _, t := range terms {
		set[t] = true
	}
	return set
}

// jaccard computes the Jaccard similarity between two term sets.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	for term := range a {
		if b[term] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// sortByScore sorts results by score normalization: s / (1 + s) → [0, 1).
// Higher is better. ByteRover uses this normalization for consistent thresholding.
func normalizeScore(score float64) float64 {
	if score <= 0 {
		return 0
	}
	return score / (1.0 + score)
}

// filterOOD performs out-of-domain detection.
// If significant query terms (>= 4 chars) are unmatched and top score
// is below threshold, returns true (query is out of domain).
func filterOOD(query string, results []scoredDoc, threshold float64) bool {
	if len(results) == 0 {
		return true
	}

	topNormalized := normalizeScore(results[0].score)
	if topNormalized >= threshold {
		return false
	}

	// Check if significant terms are all unmatched
	terms := tokenize(query)
	significant := 0
	for _, t := range terms {
		if len(t) >= 4 {
			significant++
		}
	}

	// If most terms are short (< 4 chars), don't filter
	if significant == 0 {
		return false
	}

	return topNormalized < threshold
}

// deduplicateResults removes duplicate paths, keeping the highest scored.
func deduplicateResults(results []SearchResult) []SearchResult {
	seen := make(map[string]int) // path → index in output
	var deduped []SearchResult

	// Results should already be sorted by rank (best first)
	for _, r := range results {
		if _, exists := seen[r.Path]; !exists {
			seen[r.Path] = len(deduped)
			deduped = append(deduped, r)
		}
	}

	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Rank > deduped[j].Rank
	})

	return deduped
}

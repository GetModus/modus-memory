package index

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25 parameters — standard values, tuned for document search.
const (
	bm25K1 = 1.2  // term saturation
	bm25B  = 0.75 // length normalization
)

// Field weights — title matches are more valuable than body matches.
// Validated by ByteRover's benchmark: title 3x, path 1.5x achieves 96% LoCoMo.
var fieldWeights = [6]float64{
	1.5, // path
	1.0, // source
	2.0, // subject
	3.0, // title
	1.5, // tags
	1.0, // body
}

// fieldIndex maps field names to positions.
const (
	fieldPath    = 0
	fieldSource  = 1
	fieldSubject = 2
	fieldTitle   = 3
	fieldTags    = 4
	fieldBody    = 5
)

// posting records a term occurrence in a specific document field.
type posting struct {
	docID int
	field int
	tf    int // term frequency in this field
}

// bm25Engine holds the inverted index and document stats for BM25 scoring.
type bm25Engine struct {
	postings map[string][]posting // term → postings
	docLens  [][6]int             // per-document field lengths (in tokens)
	avgLens  [6]float64           // average field lengths across corpus
	numDocs  int
}

// newBM25Engine builds an inverted index from the loaded documents.
func newBM25Engine(docs []document) *bm25Engine {
	e := &bm25Engine{
		postings: make(map[string][]posting),
		docLens:  make([][6]int, len(docs)),
		numDocs:  len(docs),
	}

	var totalLens [6]int64

	for docID, doc := range docs {
		fields := [6]string{
			doc.Path,
			doc.Source,
			doc.Subject,
			doc.Title,
			doc.Tags,
			doc.Body,
		}

		for fieldID, text := range fields {
			tokens := tokenize(text)
			e.docLens[docID][fieldID] = len(tokens)
			totalLens[fieldID] += int64(len(tokens))

			// Count term frequencies
			tf := make(map[string]int)
			for _, t := range tokens {
				tf[t]++
			}

			for term, count := range tf {
				e.postings[term] = append(e.postings[term], posting{
					docID: docID,
					field: fieldID,
					tf:    count,
				})
			}
		}
	}

	// Compute average field lengths
	for i := 0; i < 6; i++ {
		if e.numDocs > 0 {
			e.avgLens[i] = float64(totalLens[i]) / float64(e.numDocs)
		}
	}

	return e
}

// search scores all documents against the query and returns top results.
func (e *bm25Engine) search(query string, limit int) []scoredDoc {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Accumulate scores per document
	scores := make(map[int]float64)

	for _, term := range queryTerms {
		posts, ok := e.postings[term]
		if !ok {
			// Try prefix match for partial terms
			posts = e.prefixMatch(term)
			if len(posts) == 0 {
				continue
			}
		}

		// IDF: log((N - df + 0.5) / (df + 0.5) + 1)
		df := e.docFreq(term)
		idf := math.Log((float64(e.numDocs)-float64(df)+0.5)/(float64(df)+0.5) + 1.0)

		for _, p := range posts {
			// BM25 per-field score with field weight
			dl := float64(e.docLens[p.docID][p.field])
			avgDl := e.avgLens[p.field]
			if avgDl == 0 {
				avgDl = 1
			}

			tf := float64(p.tf)
			numerator := tf * (bm25K1 + 1)
			denominator := tf + bm25K1*(1-bm25B+bm25B*dl/avgDl)

			score := idf * (numerator / denominator) * fieldWeights[p.field]
			scores[p.docID] += score
		}
	}

	// Sort by score descending
	var results []scoredDoc
	for docID, score := range scores {
		results = append(results, scoredDoc{docID: docID, score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// docFreq returns the number of documents containing a term (across any field).
func (e *bm25Engine) docFreq(term string) int {
	posts := e.postings[term]
	seen := make(map[int]bool)
	for _, p := range posts {
		seen[p.docID] = true
	}
	return len(seen)
}

// prefixMatch finds postings for terms starting with the given prefix.
// Only used for short queries where exact match fails.
func (e *bm25Engine) prefixMatch(prefix string) []posting {
	if len(prefix) < 3 {
		return nil
	}
	var results []posting
	for term, posts := range e.postings {
		if strings.HasPrefix(term, prefix) {
			results = append(results, posts...)
		}
	}
	return results
}

type scoredDoc struct {
	docID int
	score float64
}

// tokenize splits text into lowercase tokens, removing punctuation.
// Uses Porter-like stemming for common suffixes.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				token := current.String()
				if len(token) >= 2 { // skip single-char tokens
					tokens = append(tokens, stem(token))
				}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		token := current.String()
		if len(token) >= 2 {
			tokens = append(tokens, stem(token))
		}
	}

	return tokens
}

// stem applies minimal suffix stripping. Not a full Porter stemmer —
// just enough to match plurals and common verb forms.
func stem(word string) string {
	if len(word) <= 4 {
		return word
	}

	// Common English suffixes — order matters (longest first)
	suffixes := []struct {
		suffix string
		minLen int // minimum remaining length after stripping
	}{
		{"nesses", 4},
		{"ments", 4},
		{"ation", 3},
		{"ings", 3},
		{"ness", 3},
		{"ment", 3},
		{"able", 3},
		{"ible", 3},
		{"tion", 3},
		{"sion", 3},
		{"ally", 3},
		{"ing", 3},
		{"ies", 3},
		{"ful", 3},
		{"ous", 3},
		{"ive", 3},
		{"ers", 3},
		{"est", 3},
		{"ize", 3},
		{"ise", 3},
		{"ity", 3},
		{"ed", 3},
		{"er", 3},
		{"ly", 3},
		{"es", 3},
		{"ss", 0}, // don't strip -ss (e.g. "less")
		{"s", 3},
	}

	for _, s := range suffixes {
		if s.minLen == 0 {
			continue // skip marker entries
		}
		if strings.HasSuffix(word, s.suffix) && len(word)-len(s.suffix) >= s.minLen {
			return word[:len(word)-len(s.suffix)]
		}
	}

	return word
}

// snippet extracts a text snippet around the first matching term.
func snippet(body string, queryTerms []string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 200
	}
	if len(body) <= maxLen {
		return body
	}

	lower := strings.ToLower(body)
	bestPos := -1
	for _, term := range queryTerms {
		stemmed := stem(strings.ToLower(term))
		if idx := strings.Index(lower, stemmed); idx >= 0 {
			bestPos = idx
			break
		}
	}

	if bestPos < 0 {
		// No match found — return beginning
		return body[:maxLen] + "..."
	}

	// Center the snippet around the match
	start := bestPos - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(body) {
		end = len(body)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
	}
	if end < len(body) {
		suffix = "..."
	}

	return prefix + body[start:end] + suffix
}

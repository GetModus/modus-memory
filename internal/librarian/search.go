package librarian

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ExpandQuery asks the librarian to produce search-optimized terms for a query.
// This replaces the embedding channel — instead of computing vector similarity,
// we ask Gemma to bridge the semantic gap at query time.
//
// Example: "what's our fastest model" → "speed tok/s benchmark performance Gemma GLM"
func ExpandQuery(query string) []string {
	system := `You are a search query expander for a developer's personal knowledge base.
Given a natural language query, produce 3-5 alternative search phrases that would match
relevant documents using keyword search (FTS5). Include:
- Synonyms and related terms
- Technical equivalents
- Specific names/tools the user likely means
- Both formal and informal phrasings

Return ONLY a JSON array of strings. No explanation.
Example: ["original query terms", "synonym phrase", "technical equivalent", "specific tool names"]`

	user := fmt.Sprintf("Expand this query for keyword search: %q", query)

	response := Call(system, user, 150)
	if response == "" {
		return []string{query}
	}

	var expansions []string
	if err := ParseJSON(response, &expansions); err != nil {
		log.Printf("librarian/search: expansion parse failed: %v", err)
		return []string{query}
	}

	// Always include the original query
	result := []string{query}
	for _, exp := range expansions {
		exp = strings.TrimSpace(exp)
		if exp != "" && exp != query {
			result = append(result, exp)
		}
	}

	if len(result) > 6 {
		result = result[:6]
	}
	return result
}

// RankResults asks the librarian to score and rank search results by relevance.
// Takes the original query and a set of result snippets, returns ranked indices.
func RankResults(query string, results []ResultSnippet, topN int) []int {
	if len(results) == 0 {
		return nil
	}
	if len(results) <= topN {
		indices := make([]int, len(results))
		for i := range indices {
			indices[i] = i
		}
		return indices
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n   %s\n\n",
			i+1, r.Source, r.Title, truncate(r.Snippet, 200)))
	}

	system := `You rank search results for relevance. Given a query and numbered results,
return a JSON array of the most relevant result numbers (1-indexed), ordered by relevance.
Return ONLY the JSON array of integers. No explanation.`

	user := fmt.Sprintf("Query: %q\nReturn the top %d most relevant results:\n\n%s",
		query, topN, sb.String())

	response := Call(system, user, 100)
	if response == "" {
		return defaultIndices(topN, len(results))
	}

	var ranked []int
	if err := ParseJSON(response, &ranked); err != nil {
		log.Printf("librarian/search: rank parse failed: %v", err)
		return defaultIndices(topN, len(results))
	}

	// Convert 1-indexed to 0-indexed, validate
	var valid []int
	seen := map[int]bool{}
	for _, r := range ranked {
		idx := r - 1
		if idx >= 0 && idx < len(results) && !seen[idx] {
			valid = append(valid, idx)
			seen[idx] = true
		}
		if len(valid) >= topN {
			break
		}
	}
	return valid
}

// ResultSnippet is a lightweight search result for ranking.
type ResultSnippet struct {
	Source  string
	Title   string
	Snippet string
}

// SummarizeForCloud condenses search results into a token-efficient summary
// for cloud models. Instead of dumping raw results, the librarian produces
// a curated briefing.
func SummarizeForCloud(query string, results []ResultSnippet) string {
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s: %s\n",
			i+1, r.Source, r.Title, truncate(r.Snippet, 300)))
	}

	system := `You are a librarian summarizing search results for an AI assistant.
Produce a concise, information-dense summary that answers the query directly.
Include specific facts, numbers, and references. Cite result numbers in brackets.
Keep under 300 words. Do not add opinions or speculation.`

	user := fmt.Sprintf("Query: %q\n\nResults:\n%s\n\nSummarize the key findings.",
		query, sb.String())

	return Call(system, user, 400)
}

// ExtractFacts asks the librarian to extract structured facts from text.
func ExtractFacts(text string) []ExtractedFact {
	system := `Extract factual claims from the text as structured data.
Return a JSON array: [{"subject": "X", "predicate": "Y", "value": "Z"}]
Only extract concrete, verifiable facts. No opinions or speculation.
Keep subjects and predicates consistent and reusable.`

	user := fmt.Sprintf("Extract facts from:\n\n%s", truncate(text, 2000))

	response := Call(system, user, 500)
	if response == "" {
		return nil
	}

	var facts []ExtractedFact
	if err := ParseJSON(response, &facts); err != nil {
		log.Printf("librarian/search: fact parse failed: %v", err)
		return nil
	}
	return facts
}

// ExtractedFact is a subject/predicate/value triple.
type ExtractedFact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Value     string `json:"value"`
}

func defaultIndices(n, max int) []int {
	if n > max {
		n = max
	}
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	return indices
}

// ClassifyIntent asks the librarian to classify a query for routing.
// Returns one of the 8 query classes used by the agentic search system.
func ClassifyIntent(query string) string {
	system := `Classify the user's query intent. Return ONLY one of these exact labels:
- exact_lookup: asking for a specific fact (what is X, how many, which)
- entity_topic: asking about a specific entity or topic
- temporal: asking about time, history, changes, sequence
- multi_hop: asking about relationships, chains, why/how connections
- abstraction: asking for overview, comparison, big picture
- preference: asking about preferences, beliefs, philosophy
- update_check: checking current status or progress
- no_retrieval: greeting, command, or not a knowledge query

Return ONLY the label, nothing else.`

	response := Call(system, query, 20)
	response = strings.TrimSpace(strings.ToLower(response))

	valid := map[string]bool{
		"exact_lookup": true, "entity_topic": true, "temporal": true,
		"multi_hop": true, "abstraction": true, "preference": true,
		"update_check": true, "no_retrieval": true,
	}
	if valid[response] {
		return response
	}
	return "entity_topic"
}

// Briefing produces an end-of-cycle intelligence brief.
type Briefing struct {
	New          []string `json:"new"`
	MissionRelevant []string `json:"mission_relevant"`
	Contradictions  []string `json:"contradictions"`
	NeedsReview     []string `json:"needs_review"`
	CanWait         []string `json:"can_wait"`
}

// ProduceBriefing generates a structured intelligence brief from ingested items.
func ProduceBriefing(items []string, activeMissions []string) *Briefing {
	if len(items) == 0 {
		return &Briefing{}
	}

	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}

	missions := "None specified"
	if len(activeMissions) > 0 {
		missions = strings.Join(activeMissions, ", ")
	}

	system := `You produce intelligence briefings. Given a list of newly ingested items and active missions, answer exactly 5 questions as a JSON object:
{
  "new": ["what is new — key items ingested"],
  "mission_relevant": ["what matters to active missions"],
  "contradictions": ["what contradicts existing beliefs"],
  "needs_review": ["what deserves immediate review"],
  "can_wait": ["what can safely wait"]
}
Be specific and concise. Reference items by number.`

	user := fmt.Sprintf("Active missions: %s\n\nItems ingested:\n%s", missions, sb.String())

	response := Call(system, user, 500)
	if response == "" {
		return &Briefing{}
	}

	var b Briefing
	if err := ParseJSON(response, &b); err != nil {
		log.Printf("librarian/search: briefing parse failed: %v", err)
		return &Briefing{}
	}
	return &b
}

// FormatBriefing renders a Briefing as a readable markdown string.
func (b *Briefing) FormatBriefing() string {
	if b == nil {
		return "No briefing available."
	}

	var sb strings.Builder
	sections := []struct {
		title string
		items []string
	}{
		{"What's New", b.New},
		{"Mission-Relevant", b.MissionRelevant},
		{"Contradictions", b.Contradictions},
		{"Needs Review", b.NeedsReview},
		{"Can Wait", b.CanWait},
	}

	for _, s := range sections {
		if len(s.items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s\n", s.title))
		for _, item := range s.items {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Triage classifies a single item. For batch triage, see the wraith package.
func TriageItem(title, content string) (class string, reason string) {
	system := `Classify this content for a developer building AI infrastructure (local models, MLX, Apple Silicon, Go, memory systems, security, revenue generation).

Classify as ONE of:
- ADAPT: Directly actionable — tools, techniques, code, benchmarks to implement
- KEEP: Useful reference — industry news, specs, announcements in our domain
- MORE_INFO: Potentially useful but need full article to judge
- DISCARD: Irrelevant — entertainment, sales, politics, off-topic

Return ONLY the label followed by a colon and one-sentence reason.`

	user := fmt.Sprintf("Title: %s\nContent: %s", title, truncate(content, 500))
	response := Call(system, user, 100)

	for _, cls := range []string{"ADAPT", "KEEP", "MORE_INFO", "DISCARD"} {
		if strings.HasPrefix(strings.ToUpper(response), cls) {
			reason := response
			if idx := strings.Index(response, ":"); idx >= 0 {
				reason = strings.TrimSpace(response[idx+1:])
			}
			return cls, reason
		}
	}
	return "KEEP", response
}

// MarshalBriefing serializes a Briefing to JSON.
func (b *Briefing) MarshalBriefing() string {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

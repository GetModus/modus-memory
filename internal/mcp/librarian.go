package mcp

import (
	"fmt"
	"strings"

	"github.com/GetModus/modus-memory/internal/librarian"
)

// RegisterLibrarianTools adds a neutral MCP surface for the Librarian context
// shaping layer without depending on vault or WRAITH runtime state.
func RegisterLibrarianTools(srv *Server) {
	srv.AddTool("librarian_status", "Return Librarian backend identity and availability.", map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		payload := map[string]interface{}{
			"available": librarian.Available(),
			"backend":   librarian.BackendIdentity(),
		}
		return marshalIndented(payload)
	})

	srv.AddTool("librarian_expand_query", "Expand a natural-language query into search-friendly alternatives.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Natural-language query to expand"},
		},
		"required": []string{"query"},
	}, func(args map[string]interface{}) (string, error) {
		query := stringArg(args, "query")
		if query == "" {
			return "", fmt.Errorf("query is required")
		}
		payload := map[string]interface{}{
			"query":      query,
			"expansions": librarian.ExpandQuery(query),
		}
		return marshalIndented(payload)
	})

	srv.AddTool("librarian_rank_results", "Rank search results for relevance to a query.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Original search query"},
			"results": map[string]interface{}{
				"type":        "array",
				"description": "Result snippets to rank",
			},
			"top_n": map[string]interface{}{"type": "integer", "description": "Maximum number of ranked indices to return (default 5)"},
		},
		"required": []string{"query", "results"},
	}, func(args map[string]interface{}) (string, error) {
		query := stringArg(args, "query")
		if query == "" {
			return "", fmt.Errorf("query is required")
		}
		results, err := parseResultSnippets(args["results"])
		if err != nil {
			return "", err
		}
		topN := 5
		if n, ok := args["top_n"].(float64); ok && int(n) > 0 {
			topN = int(n)
		}
		payload := map[string]interface{}{
			"query":          query,
			"ranked_indices": librarian.RankResults(query, results, topN),
		}
		return marshalIndented(payload)
	})

	srv.AddTool("librarian_summarize_results", "Summarize result snippets into a compact briefing.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Original search query"},
			"results": map[string]interface{}{
				"type":        "array",
				"description": "Result snippets to summarize",
			},
		},
		"required": []string{"query", "results"},
	}, func(args map[string]interface{}) (string, error) {
		query := stringArg(args, "query")
		if query == "" {
			return "", fmt.Errorf("query is required")
		}
		results, err := parseResultSnippets(args["results"])
		if err != nil {
			return "", err
		}
		payload := map[string]interface{}{
			"query":   query,
			"summary": librarian.SummarizeForCloud(query, results),
		}
		return marshalIndented(payload)
	})

	srv.AddTool("librarian_extract_facts", "Extract structured subject/predicate/value facts from text.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{"type": "string", "description": "Source text to extract facts from"},
		},
		"required": []string{"text"},
	}, func(args map[string]interface{}) (string, error) {
		text := stringArg(args, "text")
		if text == "" {
			return "", fmt.Errorf("text is required")
		}
		payload := map[string]interface{}{
			"facts": librarian.ExtractFacts(text),
		}
		return marshalIndented(payload)
	})

	srv.AddTool("librarian_classify_intent", "Classify a query for retrieval routing.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Query to classify"},
		},
		"required": []string{"query"},
	}, func(args map[string]interface{}) (string, error) {
		query := stringArg(args, "query")
		if query == "" {
			return "", fmt.Errorf("query is required")
		}
		payload := map[string]interface{}{
			"query":  query,
			"intent": librarian.ClassifyIntent(query),
		}
		return marshalIndented(payload)
	})

	srv.AddTool("librarian_produce_briefing", "Produce a structured intelligence briefing from items and active missions.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"items":           map[string]interface{}{"type": "array", "description": "List of items to brief over"},
			"active_missions": map[string]interface{}{"type": "array", "description": "Optional active mission labels"},
		},
		"required": []string{"items"},
	}, func(args map[string]interface{}) (string, error) {
		items := stringSliceArg(args["items"])
		if len(items) == 0 {
			return "", fmt.Errorf("items is required")
		}
		payload := map[string]interface{}{
			"briefing": librarian.ProduceBriefing(items, stringSliceArg(args["active_missions"])),
		}
		return marshalIndented(payload)
	})
}

func parseResultSnippets(raw interface{}) ([]librarian.ResultSnippet, error) {
	rows, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("results must be an array")
	}

	out := make([]librarian.ResultSnippet, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("each result must be an object")
		}
		out = append(out, librarian.ResultSnippet{
			Source:  strings.TrimSpace(stringArg(m, "source")),
			Title:   strings.TrimSpace(stringArg(m, "title")),
			Snippet: strings.TrimSpace(stringArg(m, "snippet")),
		})
	}
	return out, nil
}

func stringSliceArg(raw interface{}) []string {
	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if s, ok := row.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

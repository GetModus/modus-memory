package mcp

import (
	"fmt"
	"strings"

	"github.com/GetModus/modus-memory/internal/learnings"
	"github.com/GetModus/modus-memory/internal/vault"
)

// RegisterLearningsTools adds MCP tools for MODUS operational learnings.
// These are console-level learnings that persist across model swaps.
func RegisterLearningsTools(srv *Server, v *vault.Vault) {
	vaultDir := v.Dir

	// --- List learnings by domain ---

	srv.AddTool("modus_learnings_list",
		"List MODUS operational learnings. Use format='prompt' for prompt-ready text any model can prepend. Omit domain to get stats overview.",
		map[string]interface{}{
			"domain": map[string]interface{}{"type": "string", "description": "Domain: search, triage, code, ingestion, architecture, operations, general. Omit for stats."},
			"format": map[string]interface{}{"type": "string", "description": "Output format: 'detail' (default), 'prompt' (for prompt injection)"},
			"limit":  map[string]interface{}{"type": "number", "description": "Max learnings to return (default 10)"},
		},
		func(args map[string]interface{}) (string, error) {
			domain, _ := args["domain"].(string)
			format, _ := args["format"].(string)
			limit := 10
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}

			// No domain → return stats overview
			if domain == "" {
				all := learnings.LoadAll(vaultDir)
				if len(all) == 0 {
					return "No learnings recorded yet.", nil
				}

				domains := make(map[string]int)
				types := make(map[string]int)
				totalReinforced := 0
				for _, l := range all {
					domains[l.Domain]++
					types[l.Type]++
					totalReinforced += l.Reinforced
				}

				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("## %d learnings\n\n", len(all)))
				sb.WriteString("**By domain:** ")
				for d, c := range domains {
					sb.WriteString(fmt.Sprintf("%s=%d ", d, c))
				}
				sb.WriteString("\n**By type:** ")
				for t, c := range types {
					sb.WriteString(fmt.Sprintf("%s=%d ", t, c))
				}
				sb.WriteString(fmt.Sprintf("\n**Total reinforcements:** %d\n", totalReinforced))
				return sb.String(), nil
			}

			// format=prompt → return prompt-ready text
			if format == "prompt" {
				text := learnings.LoadForPrompt(vaultDir, domain, limit)
				if text == "" {
					return "No learnings available for this domain yet.", nil
				}
				return text, nil
			}

			// Default: detailed listing
			results := learnings.LoadByDomain(vaultDir, domain, limit)
			if len(results) == 0 {
				return fmt.Sprintf("No learnings found for domain %q.", domain), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("## %d learnings for domain: %s\n\n", len(results), domain))
			for _, l := range results {
				reinforced := ""
				if l.Reinforced > 0 {
					reinforced = fmt.Sprintf(" (confirmed %dx)", l.Reinforced)
				}
				sb.WriteString(fmt.Sprintf("### [%s] %s%s\n", strings.ToUpper(l.Type), l.Summary, reinforced))
				sb.WriteString(fmt.Sprintf("- Domain: %s | Severity: %s | Source: %s\n", l.Domain, l.Severity, l.LearnedFrom))
				sb.WriteString(fmt.Sprintf("- Created: %s | Confidence: %.1f\n", l.Created, l.Confidence))
				if l.Body != "" {
					body := l.Body
					if len(body) > 500 {
						body = body[:500] + "..."
					}
					sb.WriteString(body + "\n")
				}
				sb.WriteString("\n")
			}
			return sb.String(), nil
		})

	// --- Record a new learning ---

	srv.AddTool("modus_learnings_record",
		"Record a new MODUS operational learning. Use when: a pattern succeeds, a mistake happens, the General corrects us, or an architectural decision is made. Learnings persist across model swaps — the console remembers what cartridges discover.",
		map[string]interface{}{
			"summary":      map[string]interface{}{"type": "string", "description": "One-line summary of the learning (required)"},
			"domain":       map[string]interface{}{"type": "string", "description": "Domain: search, triage, code, ingestion, architecture, operations, general"},
			"type":         map[string]interface{}{"type": "string", "description": "Type: mistake, pattern, decision, correction"},
			"severity":     map[string]interface{}{"type": "string", "description": "Severity: critical, high, medium, low"},
			"learned_from": map[string]interface{}{"type": "string", "description": "Which model or source discovered this (e.g. gpt-5.4, gemma-4, general)"},
			"body":         map[string]interface{}{"type": "string", "description": "Full content. Use sections: ## What happened, ## Learning, ## Apply when"},
		},
		func(args map[string]interface{}) (string, error) {
			summary, _ := args["summary"].(string)
			if summary == "" {
				return "", fmt.Errorf("summary is required")
			}

			domain, _ := args["domain"].(string)
			if domain == "" {
				domain = "general"
			}
			typ, _ := args["type"].(string)
			if typ == "" {
				typ = "pattern"
			}
			severity, _ := args["severity"].(string)
			if severity == "" {
				severity = "medium"
			}
			learnedFrom, _ := args["learned_from"].(string)
			if learnedFrom == "" {
				learnedFrom = "unknown"
			}
			body, _ := args["body"].(string)

			l := learnings.Learning{
				Summary:     summary,
				Domain:      domain,
				Type:        typ,
				Severity:    severity,
				Source:      "mcp-tool",
				LearnedFrom: learnedFrom,
				Body:        body,
			}

			if err := learnings.Record(vaultDir, l); err != nil {
				return "", fmt.Errorf("record learning: %w", err)
			}

			return fmt.Sprintf("Learning recorded: [%s/%s] %s", domain, typ, summary), nil
		})

	// --- Search learnings ---

	srv.AddTool("modus_learnings_search",
		"Search MODUS operational learnings by keyword. Searches across summaries, bodies, tags, and domains.",
		map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Search query"},
			"limit": map[string]interface{}{"type": "number", "description": "Max results (default 10)"},
		},
		func(args map[string]interface{}) (string, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return "", fmt.Errorf("query is required")
			}
			limit := 10
			if l, ok := args["limit"].(float64); ok {
				limit = int(l)
			}

			results := learnings.Search(vaultDir, query, limit)
			if len(results) == 0 {
				return fmt.Sprintf("No learnings matching %q.", query), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("## %d learnings matching: %s\n\n", len(results), query))
			for _, l := range results {
				sb.WriteString(fmt.Sprintf("- **[%s]** %s (domain: %s, confidence: %.1f)\n", l.Type, l.Summary, l.Domain, l.Confidence))
			}
			return sb.String(), nil
		})

	// --- Reinforce ---

	srv.AddTool("modus_learnings_reinforce",
		"Reinforce a learning that proved correct again. Bumps confirmation count.",
		map[string]interface{}{
			"slug": map[string]interface{}{"type": "string", "description": "The learning slug (filename without .md)"},
		},
		func(args map[string]interface{}) (string, error) {
			slug, _ := args["slug"].(string)
			if slug == "" {
				return "", fmt.Errorf("slug is required")
			}
			if err := learnings.Reinforce(vaultDir, slug); err != nil {
				return "", err
			}
			return fmt.Sprintf("Learning %q reinforced.", slug), nil
		})

	// --- Deprecate ---

	srv.AddTool("modus_learnings_deprecate",
		"Deprecate a learning that is no longer valid. Sets confidence to 0.",
		map[string]interface{}{
			"slug": map[string]interface{}{"type": "string", "description": "The learning slug (filename without .md)"},
		},
		func(args map[string]interface{}) (string, error) {
			slug, _ := args["slug"].(string)
			if slug == "" {
				return "", fmt.Errorf("slug is required")
			}
			if err := learnings.Deprecate(vaultDir, slug); err != nil {
				return "", err
			}
			return fmt.Sprintf("Learning %q deprecated.", slug), nil
		})

}

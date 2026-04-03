package mcp

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/librarian"
	"github.com/GetModus/modus-memory/internal/vault"
)

// RegisterVaultTools adds all vault MCP tools — replaces RegisterArchiveTools,
// RegisterAtlasTools, and RegisterQMTools with a unified set.
// Old tool names are registered as aliases for backward compatibility.
func RegisterVaultTools(srv *Server, v *vault.Vault) {
	// --- Search ---

	searchHandler := func(args map[string]interface{}) (string, error) {
		query, _ := args["query"].(string)
		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// If the librarian is available, expand the query for better recall
		var allResults []index.SearchResult
		if librarian.Available() {
			expansions := librarian.ExpandQuery(query)
			log.Printf("vault_search: librarian expanded %q → %d variants", query, len(expansions))
			seen := map[string]bool{}
			for _, exp := range expansions {
				results, err := v.Search(exp, limit)
				if err != nil {
					continue
				}
				for _, r := range results {
					if !seen[r.Path] {
						seen[r.Path] = true
						allResults = append(allResults, r)
					}
				}
			}
		} else {
			// Fallback: direct FTS5 search without librarian
			results, err := v.Search(query, limit)
			if err != nil {
				return "", err
			}
			allResults = results
		}

		// Cap at requested limit
		if len(allResults) > limit {
			allResults = allResults[:limit]
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d results for %q:\n\n", len(allResults), query))
		for i, r := range allResults {
			sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, r.Path))
			if r.Subject != "" {
				sb.WriteString(fmt.Sprintf("   Subject: %s\n", r.Subject))
			}
			if r.Snippet != "" {
				clean := strings.ReplaceAll(r.Snippet, "<b>", "**")
				clean = strings.ReplaceAll(clean, "</b>", "**")
				sb.WriteString(fmt.Sprintf("   %s\n", clean))
			}
			sb.WriteByte('\n')
		}

		// Append cross-reference hints — show connected docs the agent might want
		if v.Index != nil {
			refs := v.Index.Connected(query, 5)
			if len(refs) > 0 {
				// Filter out docs already in results
				resultPaths := make(map[string]bool)
				for _, r := range allResults {
					resultPaths[r.Path] = true
				}
				var extra []index.DocRef
				for _, ref := range refs {
					if !resultPaths[ref.Path] {
						extra = append(extra, ref)
					}
				}
				if len(extra) > 0 {
					sb.WriteString("**Cross-references** (connected docs not in results above):\n")
					for _, ref := range extra {
						title := ref.Title
						if title == "" {
							title = ref.Path
						}
						sb.WriteString(fmt.Sprintf("- [%s] %s `%s`\n", ref.Kind, title, ref.Path))
					}
				}
			}
		}

		return sb.String(), nil
	}

	searchSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Search query"},
			"limit": map[string]interface{}{"type": "integer", "description": "Max results (default 10)"},
		},
		"required": []string{"query"},
	}

	srv.AddTool("vault_search", "Search the vault — brain, memory, atlas, missions.", searchSchema, searchHandler)

	// --- Read ---

	srv.AddTool("vault_read", "Read a vault file by relative path.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "Relative path within vault (e.g. brain/hn/some-file.md)"},
		},
		"required": []string{"path"},
	}, func(args map[string]interface{}) (string, error) {
		relPath, _ := args["path"].(string)
		doc, err := v.Read(relPath)
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		for k, val := range doc.Frontmatter {
			sb.WriteString(fmt.Sprintf("%s: %v\n", k, val))
		}
		sb.WriteString("\n")
		sb.WriteString(doc.Body)
		return sb.String(), nil
	})

	// --- Write ---

	srv.AddTool("vault_write", "Write a vault file (frontmatter + body).", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":        map[string]interface{}{"type": "string", "description": "Relative path within vault"},
			"frontmatter": map[string]interface{}{"type": "object", "description": "YAML frontmatter fields"},
			"body":        map[string]interface{}{"type": "string", "description": "Markdown body"},
		},
		"required": []string{"path", "body"},
	}, func(args map[string]interface{}) (string, error) {
		relPath, _ := args["path"].(string)
		body, _ := args["body"].(string)
		fm := make(map[string]interface{})
		if fmRaw, ok := args["frontmatter"].(map[string]interface{}); ok {
			fm = fmRaw
		}
		if err := v.Write(relPath, fm, body); err != nil {
			return "", err
		}
		return fmt.Sprintf("Written: %s", relPath), nil
	})

	// --- List ---

	srv.AddTool("vault_list", "List vault files in a subdirectory, optionally filtered.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subdir":  map[string]interface{}{"type": "string", "description": "Subdirectory to list (e.g. brain/hn, memory/facts)"},
			"field":   map[string]interface{}{"type": "string", "description": "Filter by frontmatter field"},
			"value":   map[string]interface{}{"type": "string", "description": "Required value for field"},
			"exclude": map[string]interface{}{"type": "boolean", "description": "If true, exclude matches instead of including"},
			"limit":   map[string]interface{}{"type": "integer", "description": "Max results (default 50)"},
		},
		"required": []string{"subdir"},
	}, func(args map[string]interface{}) (string, error) {
		subdir, _ := args["subdir"].(string)
		limit := 50
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		var filters []vault.Filter
		if field, ok := args["field"].(string); ok && field != "" {
			val, _ := args["value"].(string)
			exclude, _ := args["exclude"].(bool)
			filters = append(filters, vault.Filter{Field: field, Value: val, Exclude: exclude})
		}

		docs, err := v.List(subdir, filters...)
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		count := 0
		for _, doc := range docs {
			if count >= limit {
				break
			}
			rel, _ := filepath.Rel(v.Dir, doc.Path)
			title := doc.Get("title")
			if title == "" {
				title = doc.Get("name")
			}
			if title == "" {
				title = doc.Get("subject")
			}
			if title != "" {
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", title, rel))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", rel))
			}
			count++
		}
		return fmt.Sprintf("%d files:\n\n%s", count, sb.String()), nil
	})

	// --- Status ---

	statusHandler := func(args map[string]interface{}) (string, error) {
		return v.StatusJSON()
	}

	srv.AddTool("vault_status", "Vault statistics — file counts, index size, cross-ref stats.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, statusHandler)

	// --- Memory Facts ---

	memoryFactsHandler := func(args map[string]interface{}) (string, error) {
		subject, _ := args["subject"].(string)
		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		docs, err := v.ListFacts(subject, limit)
		if err != nil {
			return "", err
		}
		if len(docs) == 0 {
			return "No memory facts found.", nil
		}

		var sb strings.Builder
		for _, doc := range docs {
			subj := doc.Get("subject")
			pred := doc.Get("predicate")
			conf := doc.Get("confidence")
			imp := doc.Get("importance")
			body := strings.TrimSpace(doc.Body)
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **%s** %s (confidence: %s, importance: %s)\n  %s\n\n", subj, pred, conf, imp, body))
		}
		return fmt.Sprintf("%d memory facts:\n\n%s", len(docs), sb.String()), nil
	}

	memoryFactsSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subject": map[string]interface{}{"type": "string", "description": "Filter by subject (optional)"},
			"limit":   map[string]interface{}{"type": "integer", "description": "Max results (default 20)"},
		},
	}

	srv.AddTool("memory_facts", "List episodic memory facts. Optionally filter by subject.", memoryFactsSchema, memoryFactsHandler)

	// --- Memory Search ---

	memorySearchHandler := func(args map[string]interface{}) (string, error) {
		query, _ := args["query"].(string)
		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// Search with optional librarian expansion
		queries := []string{query}
		if librarian.Available() {
			queries = librarian.ExpandQuery(query)
			log.Printf("memory_search: librarian expanded %q → %d variants", query, len(queries))
		}

		// Run in-memory fact search across all query variants, merge by subject|predicate
		seen := map[string]bool{}
		var merged []index.MemFact

		for _, q := range queries {
			facts := v.Index.SearchFacts(q, limit)
			for _, f := range facts {
				key := f.Subject + "|" + f.Predicate
				if !seen[key] {
					seen[key] = true
					merged = append(merged, f)
				}
			}
		}

		if len(merged) == 0 {
			return "No memory facts matched this query.", nil
		}

		// Cap at limit
		if len(merged) > limit {
			merged = merged[:limit]
		}

		// Reinforce accessed facts — FSRS recall event.
		// Each fact returned to an agent is a successful recall, strengthening stability.
		for _, f := range merged {
			if f.ID != "" {
				go v.ReinforceFact(f.ID) // async — don't block search response
			}
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d results (bm25+librarian, %d queries):\n\n",
			len(merged), len(queries)))
		for _, f := range merged {
			tier := f.Tier()
			line := fmt.Sprintf("- **%s** %s → %s (conf=%.2f, %s)",
				f.Subject, f.Predicate, truncateStr(f.Value, 120), f.Confidence, tier)
			if warn := f.StalenessWarning(); warn != "" {
				line += " " + warn
			}
			sb.WriteString(line + "\n")
		}
		return sb.String(), nil
	}

	memorySearchSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Search query"},
			"limit": map[string]interface{}{"type": "integer", "description": "Max results (default 10)"},
		},
		"required": []string{"query"},
	}

	srv.AddTool("memory_search", "Search episodic memory facts with librarian expansion and FSRS reinforcement.", memorySearchSchema, memorySearchHandler)

	// --- Memory Store ---

	memoryStoreHandler := func(args map[string]interface{}) (string, error) {
		subject, _ := args["subject"].(string)
		predicate, _ := args["predicate"].(string)
		value, _ := args["value"].(string)
		confidence := 0.8
		if c, ok := args["confidence"].(float64); ok {
			confidence = c
		}
		importance := "medium"
		if imp, ok := args["importance"].(string); ok {
			importance = imp
		}

		relPath, err := v.StoreFact(subject, predicate, value, confidence, importance)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Stored: %s %s → %s (confidence: %.2f)", subject, predicate, relPath, confidence), nil
	}

	memoryStoreSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subject":    map[string]interface{}{"type": "string"},
			"predicate":  map[string]interface{}{"type": "string"},
			"value":      map[string]interface{}{"type": "string"},
			"confidence": map[string]interface{}{"type": "number", "description": "0.0-1.0"},
			"importance": map[string]interface{}{"type": "string", "enum": []string{"critical", "high", "medium", "low"}},
		},
		"required": []string{"subject", "predicate", "value"},
	}

	srv.AddTool("memory_store", "Store a new episodic memory fact.", memoryStoreSchema, memoryStoreHandler)

	// --- Atlas: Entities ---

	srv.AddTool("atlas_list_entities", "List all entities in the knowledge graph.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		docs, err := v.ListEntities()
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d entities:\n\n", len(docs)))
		for _, doc := range docs {
			name := doc.Get("name")
			kind := doc.Get("kind")
			links := doc.WikiLinks()
			sb.WriteString(fmt.Sprintf("- **%s** (%s) — %d links\n", name, kind, len(links)))
		}
		return sb.String(), nil
	})

	// --- Atlas: Get Entity ---

	srv.AddTool("atlas_get_entity", "Get an entity page with beliefs and wiki-links.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string", "description": "Entity name or slug"},
		},
		"required": []string{"name"},
	}, func(args map[string]interface{}) (string, error) {
		name, _ := args["name"].(string)
		doc, err := v.GetEntity(name)
		if err != nil {
			return fmt.Sprintf("Entity %q not found.", name), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", doc.Get("name")))
		sb.WriteString(fmt.Sprintf("Kind: %s | Status: %s\n\n", doc.Get("kind"), doc.Get("status")))
		sb.WriteString(doc.Body)

		links := doc.WikiLinks()
		if len(links) > 0 {
			sb.WriteString("\n\n## Resolved Links\n")
			for _, link := range links {
				resolved := v.ResolveWikiLink(link)
				if resolved != "" {
					sb.WriteString(fmt.Sprintf("- [[%s]] → %s\n", link, resolved))
				} else {
					sb.WriteString(fmt.Sprintf("- [[%s]] → (not found)\n", link))
				}
			}
		}
		return sb.String(), nil
	})

	// --- Atlas: Beliefs ---

	srv.AddTool("atlas_list_beliefs", "List beliefs from the knowledge graph.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subject": map[string]interface{}{"type": "string", "description": "Filter by subject"},
			"limit":   map[string]interface{}{"type": "integer"},
		},
	}, func(args map[string]interface{}) (string, error) {
		subject, _ := args["subject"].(string)
		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		docs, err := v.ListBeliefs(subject, limit)
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		for _, doc := range docs {
			subj := doc.Get("subject")
			pred := doc.Get("predicate")
			conf := doc.Get("confidence")
			body := strings.TrimSpace(doc.Body)
			if len(body) > 100 {
				body = body[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **%s** %s (confidence: %s): %s\n", subj, pred, conf, body))
		}
		return fmt.Sprintf("%d beliefs:\n\n%s", len(docs), sb.String()), nil
	})

	// --- QM: Board ---

	srv.AddTool("qm_board", "Mission board — grouped by status.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		groups := v.MissionBoard()

		var sb strings.Builder
		for _, status := range []string{"active", "blocked", "planned", "completed"} {
			missions := groups[status]
			if len(missions) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("## %s (%d)\n", strings.ToUpper(status[:1])+status[1:], len(missions)))
			for _, m := range missions {
				title := m.Get("title")
				priority := m.Get("priority")
				sb.WriteString(fmt.Sprintf("- **%s** (priority: %s)\n", title, priority))
			}
			sb.WriteByte('\n')
		}
		return sb.String(), nil
	})

	// --- QM: Get Mission ---

	srv.AddTool("qm_get_mission", "Get a specific mission by slug or title.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"slug": map[string]interface{}{"type": "string", "description": "Mission slug or title"},
		},
		"required": []string{"slug"},
	}, func(args map[string]interface{}) (string, error) {
		slug, _ := args["slug"].(string)
		doc, err := v.GetMission(slug)
		if err != nil {
			return fmt.Sprintf("Mission %q not found.", slug), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", doc.Get("title")))
		sb.WriteString(fmt.Sprintf("Status: %s | Priority: %s\n", doc.Get("status"), doc.Get("priority")))
		sb.WriteString(fmt.Sprintf("Created: %s\n\n", doc.Get("created")))
		sb.WriteString(doc.Body)
		return sb.String(), nil
	})

	// --- QM: List Missions ---

	srv.AddTool("qm_list_missions", "List missions, optionally filtered by status.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status": map[string]interface{}{"type": "string", "description": "Filter: active, blocked, planned, completed"},
			"limit":  map[string]interface{}{"type": "integer"},
		},
	}, func(args map[string]interface{}) (string, error) {
		statusFilter, _ := args["status"].(string)
		limit := 30
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		docs, err := v.ListMissions(statusFilter, limit)
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		for _, m := range docs {
			status := m.Get("status")
			title := m.Get("title")
			priority := m.Get("priority")
			sb.WriteString(fmt.Sprintf("- [%s] **%s** (priority: %s)\n", status, title, priority))
		}
		return fmt.Sprintf("%d missions:\n\n%s", len(docs), sb.String()), nil
	})

	// --- QM: Create Mission ---

	srv.AddTool("qm_create_mission", "Create a new mission.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"title":       map[string]interface{}{"type": "string"},
			"description": map[string]interface{}{"type": "string"},
			"priority":    map[string]interface{}{"type": "string", "enum": []string{"critical", "high", "medium", "low"}},
		},
		"required": []string{"title", "description"},
	}, func(args map[string]interface{}) (string, error) {
		title, _ := args["title"].(string)
		description, _ := args["description"].(string)
		priority, _ := args["priority"].(string)

		path, err := v.CreateMission(title, description, priority)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Mission created: %s → %s", title, path), nil
	})

	// --- QM: Ship Clock ---

	srv.AddTool("qm_ship_clock", "Ship clock — days remaining to target.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		return v.ShipClockJSON()
	})

	// --- QM: Blueprints ---

	srv.AddTool("qm_blueprints", "List reusable mission blueprints.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"limit": map[string]interface{}{"type": "integer"},
		},
	}, func(args map[string]interface{}) (string, error) {
		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		docs, err := v.ListBlueprints(limit)
		if err != nil {
			return "No blueprints found.", nil
		}

		var sb strings.Builder
		for _, doc := range docs {
			name := doc.Get("name")
			kind := doc.Get("type")
			sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", name, kind))
		}
		return fmt.Sprintf("%d blueprints:\n\n%s", len(docs), sb.String()), nil
	})

	// --- Atlas: Trust Stage ---

	srv.AddTool("atlas_get_trust", "Get the current trust stage (1=Inform, 2=Recommend, 3=Act).", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		stage, config, err := v.GetTrustStage()
		if err != nil {
			return "", err
		}
		label := vault.TrustStageLabel(stage)
		updatedBy, _ := config["updated_by"].(string)
		return fmt.Sprintf("Trust: %s\nUpdated by: %s", label, updatedBy), nil
	})

	srv.AddTool("atlas_set_trust", "Set the trust stage (1-3). Operator only — MODUS never self-promotes.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"stage":      map[string]interface{}{"type": "integer", "description": "Trust stage: 1 (Inform), 2 (Recommend), 3 (Act)"},
			"updated_by": map[string]interface{}{"type": "string", "description": "Who is making this change"},
			"reason":     map[string]interface{}{"type": "string", "description": "Reason for the change"},
		},
		"required": []string{"stage", "updated_by"},
	}, func(args map[string]interface{}) (string, error) {
		stage := int(args["stage"].(float64))
		updatedBy, _ := args["updated_by"].(string)
		reason, _ := args["reason"].(string)
		if err := v.SetTrustStage(stage, updatedBy, reason); err != nil {
			return "", err
		}
		return fmt.Sprintf("Trust stage set to %d (%s) by %s", stage, vault.TrustStageLabel(stage), updatedBy), nil
	})

	// --- Atlas: Belief Decay ---

	srv.AddTool("atlas_decay_beliefs", "Run belief confidence decay sweep. Returns count of beliefs updated.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		n, err := v.DecayAllBeliefs()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Decayed %d beliefs.", n), nil
	})

	srv.AddTool("atlas_reinforce_belief", "Reinforce a belief's confidence (+0.05 independent, +0.02 same source).", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":   map[string]interface{}{"type": "string", "description": "Relative path to belief file"},
			"source": map[string]interface{}{"type": "string", "description": "Source of reinforcement"},
		},
		"required": []string{"path"},
	}, func(args map[string]interface{}) (string, error) {
		relPath, _ := args["path"].(string)
		source, _ := args["source"].(string)
		if err := v.ReinforceBelief(relPath, source); err != nil {
			return "", err
		}
		return fmt.Sprintf("Reinforced: %s", relPath), nil
	})

	srv.AddTool("atlas_weaken_belief", "Weaken a belief's confidence (-0.10, floor 0.05).", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "Relative path to belief file"},
		},
		"required": []string{"path"},
	}, func(args map[string]interface{}) (string, error) {
		relPath, _ := args["path"].(string)
		if err := v.WeakenBelief(relPath); err != nil {
			return "", err
		}
		return fmt.Sprintf("Weakened: %s", relPath), nil
	})

	// --- Atlas: PRs (Evolution Proposals) ---

	srv.AddTool("atlas_open_pr", "Open a new evolution proposal (PR) for the knowledge graph.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"title":       map[string]interface{}{"type": "string"},
			"opened_by":   map[string]interface{}{"type": "string"},
			"target_type": map[string]interface{}{"type": "string", "description": "entity, belief, or fact"},
			"target_id":   map[string]interface{}{"type": "string"},
			"reasoning":   map[string]interface{}{"type": "string"},
			"confidence":  map[string]interface{}{"type": "number"},
			"linked_belief_ids": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"title", "opened_by", "reasoning"},
	}, func(args map[string]interface{}) (string, error) {
		title, _ := args["title"].(string)
		openedBy, _ := args["opened_by"].(string)
		targetType, _ := args["target_type"].(string)
		targetID, _ := args["target_id"].(string)
		reasoning, _ := args["reasoning"].(string)
		confidence := 0.7
		if c, ok := args["confidence"].(float64); ok {
			confidence = c
		}
		var linkedIDs []string
		if arr, ok := args["linked_belief_ids"].([]interface{}); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					linkedIDs = append(linkedIDs, s)
				}
			}
		}
		path, err := v.OpenPR(title, openedBy, targetType, targetID, reasoning, confidence, linkedIDs)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("PR opened: %s", path), nil
	})

	srv.AddTool("atlas_merge_pr", "Merge an evolution PR. Reinforces linked beliefs. Operator only.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":      map[string]interface{}{"type": "string", "description": "Relative path to PR file"},
			"closed_by": map[string]interface{}{"type": "string"},
		},
		"required": []string{"path", "closed_by"},
	}, func(args map[string]interface{}) (string, error) {
		relPath, _ := args["path"].(string)
		closedBy, _ := args["closed_by"].(string)
		if err := v.MergePR(relPath, closedBy); err != nil {
			return "", err
		}
		return fmt.Sprintf("PR merged: %s (by %s). Linked beliefs reinforced.", relPath, closedBy), nil
	})

	srv.AddTool("atlas_reject_pr", "Reject an evolution PR. Weakens linked beliefs. Operator only.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":      map[string]interface{}{"type": "string", "description": "Relative path to PR file"},
			"closed_by": map[string]interface{}{"type": "string"},
			"reason":    map[string]interface{}{"type": "string"},
		},
		"required": []string{"path", "closed_by"},
	}, func(args map[string]interface{}) (string, error) {
		relPath, _ := args["path"].(string)
		closedBy, _ := args["closed_by"].(string)
		reason, _ := args["reason"].(string)
		if err := v.RejectPR(relPath, closedBy, reason); err != nil {
			return "", err
		}
		return fmt.Sprintf("PR rejected: %s (by %s). Linked beliefs weakened.", relPath, closedBy), nil
	})

	srv.AddTool("atlas_list_prs", "List evolution PRs, optionally filtered by status.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"status": map[string]interface{}{"type": "string", "description": "Filter: open, merged, rejected"},
		},
	}, func(args map[string]interface{}) (string, error) {
		status, _ := args["status"].(string)
		docs, err := v.ListPRs(status)
		if err != nil {
			return "", err
		}
		if len(docs) == 0 {
			return "No PRs found.", nil
		}
		var sb strings.Builder
		for _, doc := range docs {
			title := doc.Get("title")
			st := doc.Get("status")
			openedBy := doc.Get("opened_by")
			sb.WriteString(fmt.Sprintf("- [%s] **%s** (by %s)\n", st, title, openedBy))
		}
		return fmt.Sprintf("%d PRs:\n\n%s", len(docs), sb.String()), nil
	})

	// --- QM: Mission Dependencies ---

	srv.AddTool("qm_add_dependency", "Add a typed dependency between missions.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mission": map[string]interface{}{"type": "string", "description": "Mission slug that has the dependency"},
			"depends_on": map[string]interface{}{"type": "string", "description": "Mission slug it depends on"},
			"type": map[string]interface{}{"type": "string", "description": "blocks, informs, or enhances"},
		},
		"required": []string{"mission", "depends_on", "type"},
	}, func(args map[string]interface{}) (string, error) {
		mission, _ := args["mission"].(string)
		dep, _ := args["depends_on"].(string)
		depType, _ := args["type"].(string)
		if err := v.AddDependency(mission, dep, depType); err != nil {
			return "", err
		}
		return fmt.Sprintf("Dependency added: %s → %s (%s)", mission, dep, depType), nil
	})

	srv.AddTool("qm_remove_dependency", "Remove a dependency from a mission.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mission": map[string]interface{}{"type": "string", "description": "Mission slug"},
			"depends_on": map[string]interface{}{"type": "string", "description": "Dependency to remove"},
		},
		"required": []string{"mission", "depends_on"},
	}, func(args map[string]interface{}) (string, error) {
		mission, _ := args["mission"].(string)
		dep, _ := args["depends_on"].(string)
		if err := v.RemoveDependency(mission, dep); err != nil {
			return "", err
		}
		return fmt.Sprintf("Dependency removed: %s → %s", mission, dep), nil
	})

	srv.AddTool("qm_get_dependencies", "Get a mission's dependencies with satisfaction status and whether it can start.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mission": map[string]interface{}{"type": "string", "description": "Mission slug"},
		},
		"required": []string{"mission"},
	}, func(args map[string]interface{}) (string, error) {
		mission, _ := args["mission"].(string)
		deps, err := v.GetDependencies(mission)
		if err != nil {
			return "", err
		}

		// Check can_start
		canStart, blockers, _ := v.CanStart(mission)
		var sb strings.Builder

		if canStart {
			sb.WriteString(fmt.Sprintf("Mission %q: **ready to start**\n\n", mission))
		} else {
			sb.WriteString(fmt.Sprintf("Mission %q: **blocked** by %s\n\n", mission, strings.Join(blockers, ", ")))
		}

		if len(deps) == 0 {
			sb.WriteString("No dependencies.")
			return sb.String(), nil
		}

		for _, d := range deps {
			satisfied := "no"
			if s, ok := d["satisfied"].(bool); ok && s {
				satisfied = "yes"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s) — status: %s, satisfied: %s\n",
				d["slug"], d["type"], d["status"], satisfied))
		}
		return sb.String(), nil
	})

	// --- Memory: Fact Decay ---

	srv.AddTool("memory_decay_facts", "Run memory fact confidence decay sweep.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		n, err := v.DecayFacts()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Decayed %d memory facts.", n), nil
	})

	srv.AddTool("memory_archive_stale", "Archive stale memory facts below confidence threshold.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"threshold": map[string]interface{}{"type": "number", "description": "Confidence threshold (default 0.1)"},
		},
	}, func(args map[string]interface{}) (string, error) {
		threshold := 0.1
		if t, ok := args["threshold"].(float64); ok {
			threshold = t
		}
		n, err := v.ArchiveStaleFacts(threshold)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Archived %d stale facts (below %.2f confidence).", n, threshold), nil
	})

	// --- Memory: Reinforce Fact ---

	srv.AddTool("memory_reinforce", "Reinforce a memory fact after successful recall (FSRS stability growth).", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "Relative vault path to the fact (e.g. memory/facts/some-fact.md)"},
		},
		"required": []string{"path"},
	}, func(args map[string]interface{}) (string, error) {
		path, _ := args["path"].(string)
		if err := v.ReinforceFact(path); err != nil {
			return "", err
		}
		return fmt.Sprintf("Reinforced %s — stability increased, difficulty decreased.", path), nil
	})

	// --- Cross-Reference Query ---

	srv.AddTool("vault_connected", "Find all documents connected to a subject, entity, or tag. Returns facts, beliefs, entities, articles, learnings, and missions that share references.", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Subject, entity name, or tag to find connections for"},
			"limit": map[string]interface{}{"type": "integer", "description": "Max results (default 20)"},
		},
		"required": []string{"query"},
	}, func(args map[string]interface{}) (string, error) {
		query, _ := args["query"].(string)
		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		if v.Index == nil {
			return "Index not loaded.", nil
		}

		refs := v.Index.Connected(query, limit)
		if len(refs) == 0 {
			return fmt.Sprintf("No cross-references found for %q.", query), nil
		}

		return index.FormatConnected(refs), nil
	})

	// --- Distillation Status ---

	srv.AddTool("distill_status", "Check training pair collection and distillation readiness.", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{},
	}, func(args map[string]interface{}) (string, error) {
		home, _ := os.UserHomeDir()
		statusPath := filepath.Join(home, "modus", "data", "distill", "STATUS.md")
		data, err := os.ReadFile(statusPath)
		if err != nil {
			// Check raw pair counts
			sageDir := filepath.Join(v.Dir, "training", "sage")
			sageEntries, _ := os.ReadDir(sageDir)
			runsDir := filepath.Join(v.Dir, "experience", "runs")
			runEntries, _ := os.ReadDir(runsDir)
			return fmt.Sprintf("Distillation pipeline active. Sources: %d SAGE files, %d agent run logs. Run the distill cadence to generate dataset.", len(sageEntries), len(runEntries)), nil
		}
		return string(data), nil
	})
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

package mcp

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/librarian"
	"github.com/GetModus/modus-memory/internal/maintain"
	"github.com/GetModus/modus-memory/internal/memorykit"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trainer"
	"github.com/GetModus/modus-memory/internal/vault"
)

// RegisterVaultTools adds all vault MCP tools — replaces RegisterArchiveTools,
// RegisterAtlasTools, and RegisterQMTools with a unified set.
// Old tool names are registered as aliases for backward compatibility.
func RegisterVaultTools(srv *Server, v *vault.Vault) {
	mem := memorykit.New(v)
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

		// Prepend correction hints if any match the query
		if hints := v.FormatCorrectionHints(query); hints != "" {
			sb.WriteString(hints)
		}

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
		memoryTemperature, _ := args["memory_temperature"].(string)
		routeSubject, _ := args["route_subject"].(string)
		routeMission, _ := args["route_mission"].(string)
		capturedByOffice, _ := args["captured_by_office"].(string)
		lineageID, _ := args["lineage_id"].(string)
		environment, _ := args["environment"].(string)
		timeBand, _ := args["time_band"].(string)
		verificationMode, _ := args["verification_mode"].(string)
		recallMode, _ := args["recall_mode"].(string)
		recallHarness, _ := args["recall_harness"].(string)
		recallAdapter, _ := args["recall_adapter"].(string)
		workItemID, _ := args["work_item_id"].(string)
		var cueTerms []string
		switch raw := args["cue_terms"].(type) {
		case []interface{}:
			for _, item := range raw {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					cueTerms = append(cueTerms, strings.TrimSpace(s))
				}
			}
		case []string:
			for _, item := range raw {
				if strings.TrimSpace(item) != "" {
					cueTerms = append(cueTerms, strings.TrimSpace(item))
				}
			}
		}
		if strings.TrimSpace(recallHarness) == "" {
			recallHarness = "mcp"
		}
		if strings.TrimSpace(recallAdapter) == "" {
			recallAdapter = "mcp_tool"
		}
		if strings.TrimSpace(recallMode) == "" {
			recallMode = "manual_search"
		}
		recall, err := mem.Recall(memorykit.RecallRequest{
			Query: query,
			Limit: limit,
			Options: vault.FactSearchOptions{
				MemoryTemperature: memoryTemperature,
				RouteSubject:      routeSubject,
				RouteMission:      routeMission,
				CapturedByOffice:  capturedByOffice,
				LineageID:         lineageID,
				Environment:       environment,
				CueTerms:          cueTerms,
				TimeBand:          timeBand,
				VerificationMode:  verificationMode,
				WorkItemID:        strings.TrimSpace(workItemID),
			},
			Harness:            strings.TrimSpace(recallHarness),
			Adapter:            strings.TrimSpace(recallAdapter),
			Mode:               strings.TrimSpace(recallMode),
			ProducingOffice:    "librarian",
			ProducingSubsystem: "mcp_memory_search",
			StaffingContext:    "mcp_adapter",
			WorkItemID:         strings.TrimSpace(workItemID),
		})
		if err != nil {
			return "", err
		}
		if len(recall.Lines) == 0 {
			return "No memory facts matched this query.", nil
		}
		return strings.Join(recall.Lines, "\n"), nil
	}

	memorySearchSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query":              map[string]interface{}{"type": "string", "description": "Search query"},
			"limit":              map[string]interface{}{"type": "integer", "description": "Max results (default 10)"},
			"memory_temperature": map[string]interface{}{"type": "string", "description": "Optional hot or warm filter for runtime admission discipline"},
			"route_subject":      map[string]interface{}{"type": "string", "description": "Optional coarse retrieval subject route"},
			"route_mission":      map[string]interface{}{"type": "string", "description": "Optional coarse retrieval mission route"},
			"captured_by_office": map[string]interface{}{"type": "string", "description": "Optional coarse retrieval office route"},
			"lineage_id":         map[string]interface{}{"type": "string", "description": "Optional lineage route selector"},
			"environment":        map[string]interface{}{"type": "string", "description": "Optional environment route selector"},
			"cue_terms":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional retrieval cue packet"},
			"time_band":          map[string]interface{}{"type": "string", "description": "Optional time band such as recent or archive"},
			"verification_mode":  map[string]interface{}{"type": "string", "description": "Optional verification mode such as critical to reopen cited sources for each result"},
			"recall_mode":        map[string]interface{}{"type": "string", "description": "Optional recall mode label such as manual_search or automatic_hot_admission"},
			"recall_harness":     map[string]interface{}{"type": "string", "description": "Optional harness label for durable recall receipts"},
			"recall_adapter":     map[string]interface{}{"type": "string", "description": "Optional adapter label for durable recall receipts"},
			"work_item_id":       map[string]interface{}{"type": "string", "description": "Optional work-item lineage identifier"},
		},
		"required": []string{"query"},
	}

	srv.AddTool("memory_search", "Search episodic memory facts with durable recall receipts and reinforcement.", memorySearchSchema, memorySearchHandler)

	// --- Memory Store ---
	stringSliceArg := func(value interface{}) []string {
		var out []string
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
		case []string:
			for _, item := range v {
				if strings.TrimSpace(item) != "" {
					out = append(out, strings.TrimSpace(item))
				}
			}
		}
		return out
	}

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
		source, _ := args["source"].(string)
		sourceRef, _ := args["source_ref"].(string)
		sourceEventID, _ := args["source_event_id"].(string)
		lineageID, _ := args["lineage_id"].(string)
		mission, _ := args["mission"].(string)
		workItemID, _ := args["work_item_id"].(string)
		environment, _ := args["environment"].(string)
		observedAt, _ := args["observed_at"].(string)
		validFrom, _ := args["valid_from"].(string)
		validTo, _ := args["valid_to"].(string)
		memoryTemperature, _ := args["memory_temperature"].(string)
		relPath, err := mem.StoreFact(subject, predicate, value, confidence, importance, vault.FactWriteAuthority{
			ProducingOffice:     "main_brain",
			ProducingSubsystem:  "mcp_memory_store",
			StaffingContext:     "mcp",
			AuthorityScope:      ledger.ScopeRuntimeMemoryStore,
			TargetDomain:        "memory/facts",
			Source:              source,
			SourceRef:           sourceRef,
			SourceRefs:          stringSliceArg(args["source_refs"]),
			SourceEventID:       sourceEventID,
			LineageID:           lineageID,
			Mission:             mission,
			WorkItemID:          workItemID,
			Environment:         environment,
			ObservedAt:          observedAt,
			ValidFrom:           validFrom,
			ValidTo:             validTo,
			SupersedesPaths:     stringSliceArg(args["supersedes_paths"]),
			RelatedFactPaths:    stringSliceArg(args["related_fact_paths"]),
			RelatedEpisodePaths: stringSliceArg(args["related_episode_paths"]),
			RelatedEntityRefs:   stringSliceArg(args["related_entity_refs"]),
			RelatedMissionRefs:  stringSliceArg(args["related_mission_refs"]),
			CueTerms:            stringSliceArg(args["cue_terms"]),
			MemoryTemperature:   memoryTemperature,
			AllowApproval:       true,
			PromotionStatus:     "proposed",
			ProofRef:            "mcp-memory-store",
		})
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
			"source":     map[string]interface{}{"type": "string", "description": "Human-readable provenance label"},
			"source_ref": map[string]interface{}{"type": "string", "description": "Canonical source reference"},
			"source_refs": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Additional canonical source references",
			},
			"source_event_id": map[string]interface{}{"type": "string", "description": "Episode event_id backing this semantic fact"},
			"lineage_id":      map[string]interface{}{"type": "string", "description": "Shared lineage identifier across related episodes and facts"},
			"mission":         map[string]interface{}{"type": "string", "description": "Optional mission route anchor for this fact"},
			"work_item_id":    map[string]interface{}{"type": "string", "description": "Optional work-item lineage identifier"},
			"environment":     map[string]interface{}{"type": "string", "description": "Optional environment or operating context"},
			"observed_at":     map[string]interface{}{"type": "string", "description": "RFC3339 timestamp for when this fact was observed"},
			"valid_from":      map[string]interface{}{"type": "string", "description": "RFC3339 timestamp marking the start of the fact validity window"},
			"valid_to":        map[string]interface{}{"type": "string", "description": "RFC3339 timestamp marking the end of the fact validity window"},
			"supersedes_paths": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Older fact paths this new fact supersedes without deleting them",
			},
			"related_fact_paths": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Related semantic fact paths that should remain linked to this fact",
			},
			"related_episode_paths": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Related episodic memory paths that ground this fact",
			},
			"related_entity_refs": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Entity references structurally linked to this fact",
			},
			"related_mission_refs": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Mission references structurally linked to this fact",
			},
			"cue_terms": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Retrieval cues associated with this fact",
			},
			"memory_temperature": map[string]interface{}{"type": "string", "enum": []string{"hot", "warm"}},
		},
		"required": []string{"subject", "predicate", "value"},
	}

	srv.AddTool("memory_store", "Store a new episodic memory fact.", memoryStoreSchema, memoryStoreHandler)

	parseCaptureFacts := func(raw interface{}) []librarian.ExtractedFact {
		rows, ok := raw.([]interface{})
		if !ok {
			return nil
		}
		out := make([]librarian.ExtractedFact, 0, len(rows))
		seen := make(map[string]bool)
		for _, row := range rows {
			item, ok := row.(map[string]interface{})
			if !ok {
				continue
			}
			fact := librarian.ExtractedFact{
				Subject:   strings.TrimSpace(stringArg(item, "subject")),
				Predicate: strings.TrimSpace(stringArg(item, "predicate")),
				Value:     strings.TrimSpace(stringArg(item, "value")),
			}
			if fact.Subject == "" || fact.Predicate == "" || fact.Value == "" {
				continue
			}
			key := strings.ToLower(fact.Subject + "\x00" + fact.Predicate + "\x00" + fact.Value)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, fact)
		}
		return out
	}

	normalizeCapturePolicy := func(value string) string {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "minimal", "strict", "everything":
			return strings.ToLower(strings.TrimSpace(value))
		default:
			return "balanced"
		}
	}

	hasDurableCaptureSignal := func(text string) bool {
		text = strings.ToLower(strings.TrimSpace(text))
		if text == "" {
			return false
		}
		signals := []string{
			"i prefer", "prefers ", "preference", "always ", "never ",
			"do not ", "don't ", "from now on", "we decided", "decided ",
			"decision", "rename", "renamed ", "codename", "should always",
			"workflow", "rule", "policy", "correction", "instead of",
			"wants ", "likes ", "dislikes ", "constraint", "requirement",
		}
		for _, signal := range signals {
			if strings.Contains(text, signal) {
				return true
			}
		}
		return false
	}

	defaultCaptureImportance := func(eventKind, requested string) string {
		if strings.TrimSpace(requested) != "" {
			return requested
		}
		switch strings.ToLower(strings.TrimSpace(eventKind)) {
		case "decision", "correction":
			return "high"
		default:
			return "medium"
		}
	}

	defaultCaptureTemperature := func(eventKind, requested string, durable bool) string {
		if strings.TrimSpace(requested) != "" {
			return requested
		}
		switch strings.ToLower(strings.TrimSpace(eventKind)) {
		case "decision", "correction":
			return "hot"
		}
		if durable {
			return "hot"
		}
		return "warm"
	}

	decideCapture := func(policy string, actionableEvent, durableSignal, structuredContext, explicitFacts, meaningfulText bool, factCount int) (string, string, bool, bool) {
		switch policy {
		case "everything":
			storeEpisode := meaningfulText || structuredContext || factCount > 0
			storeFacts := factCount > 0
			switch {
			case storeEpisode && storeFacts:
				return "episode_and_facts", "everything policy stores both the turn trace and extracted facts", true, true
			case storeEpisode:
				return "episode_only", "everything policy stores the turn trace even without fact candidates", true, false
			default:
				return "skip", "turn was too thin to store even under everything policy", false, false
			}
		case "strict":
			storeFacts := factCount > 0 && (explicitFacts || actionableEvent || durableSignal)
			storeEpisode := actionableEvent || (storeFacts && meaningfulText)
			switch {
			case storeEpisode && storeFacts:
				return "episode_and_facts", "strict policy found a durable event with admissible facts", true, true
			case storeEpisode:
				return "episode_only", "strict policy kept the event trace but did not admit semantic facts", true, false
			case storeFacts:
				return "facts_only", "strict policy admitted explicit or strongly-signaled facts only", false, true
			default:
				return "skip", "strict policy found no durable signal worth persisting", false, false
			}
		case "minimal":
			storeFacts := explicitFacts && factCount > 0
			storeEpisode := actionableEvent && meaningfulText
			switch {
			case storeEpisode && storeFacts:
				return "episode_and_facts", "minimal policy kept the event trace and explicit facts", true, true
			case storeEpisode:
				return "episode_only", "minimal policy kept only the event trace", true, false
			case storeFacts:
				return "facts_only", "minimal policy stores only explicitly supplied facts", false, true
			default:
				return "skip", "minimal policy skips turns without explicit facts or a strong event kind", false, false
			}
		default:
			storeFacts := factCount > 0 && (explicitFacts || durableSignal || actionableEvent || structuredContext)
			storeEpisode := actionableEvent || storeFacts || (structuredContext && meaningfulText && durableSignal)
			switch {
			case storeEpisode && storeFacts:
				return "episode_and_facts", "balanced policy found enough durable signal to keep both trace and facts", true, true
			case storeEpisode:
				return "episode_only", "balanced policy kept the event trace but no semantic facts survived admission", true, false
			case storeFacts:
				return "facts_only", "balanced policy admitted facts without keeping the full turn trace", false, true
			default:
				return "skip", "balanced policy found no durable signal worth persisting", false, false
			}
		}
	}

	memoryCaptureHandler := func(args map[string]interface{}) (string, error) {
		text := stringArg(args, "text")
		if text == "" {
			return "", fmt.Errorf("text is required")
		}

		storeEpisodeAllowed := true
		if value, ok := args["store_episode"].(bool); ok {
			storeEpisodeAllowed = value
		}
		extractFactsAllowed := true
		if value, ok := args["extract_facts"].(bool); ok {
			extractFactsAllowed = value
		}
		dryRun := false
		if value, ok := args["dry_run"].(bool); ok {
			dryRun = value
		}

		source := stringArg(args, "source")
		if source == "" {
			source = "mcp capture"
		}
		sourceRef := stringArg(args, "source_ref")
		eventKind := stringArg(args, "event_kind")
		if eventKind == "" {
			eventKind = "interaction"
		}
		subject := stringArg(args, "subject")
		lineageID := stringArg(args, "lineage_id")
		mission := stringArg(args, "mission")
		workItemID := stringArg(args, "work_item_id")
		environment := stringArg(args, "environment")
		policy := normalizeCapturePolicy(stringArg(args, "policy"))
		explicitImportance := stringArg(args, "importance")
		confidence := 0.8
		if value, ok := args["confidence"].(float64); ok && value > 0 {
			confidence = value
		}
		maxFacts := 5
		if value, ok := args["max_facts"].(float64); ok && int(value) > 0 {
			maxFacts = int(value)
		}

		relatedFactPaths := stringSliceArg(args["related_fact_paths"])
		relatedEpisodePaths := stringSliceArg(args["related_episode_paths"])
		relatedEntityRefs := stringSliceArg(args["related_entity_refs"])
		relatedMissionRefs := stringSliceArg(args["related_mission_refs"])
		cueTerms := stringSliceArg(args["cue_terms"])
		sourceRefs := stringSliceArg(args["source_refs"])

		explicitFacts := parseCaptureFacts(args["facts"])
		actionableEvent := false
		switch strings.ToLower(strings.TrimSpace(eventKind)) {
		case "decision", "correction", "synthesis":
			actionableEvent = true
		}
		durableSignal := hasDurableCaptureSignal(text)
		structuredContext := subject != "" || mission != "" || workItemID != "" || environment != ""
		meaningfulText := len(strings.Fields(text)) >= 6
		candidateForExtraction := len(explicitFacts) > 0 || policy == "everything" || actionableEvent || durableSignal || structuredContext

		facts := explicitFacts
		factSource := "explicit"
		if len(facts) == 0 && extractFactsAllowed && candidateForExtraction {
			facts = librarian.ExtractFacts(text)
			factSource = "librarian"
		}
		if len(facts) > maxFacts {
			facts = facts[:maxFacts]
		}

		decision, reason, storeEpisode, storeFacts := decideCapture(policy, actionableEvent, durableSignal, structuredContext, len(explicitFacts) > 0, meaningfulText, len(facts))
		if !storeEpisodeAllowed {
			storeEpisode = false
			if decision == "episode_only" {
				decision = "skip"
			}
			if decision == "episode_and_facts" {
				if storeFacts {
					decision = "facts_only"
				} else {
					decision = "skip"
				}
			}
		}
		if !extractFactsAllowed && len(explicitFacts) == 0 {
			storeFacts = false
			if decision == "facts_only" {
				decision = "skip"
			}
			if decision == "episode_and_facts" {
				if storeEpisode {
					decision = "episode_only"
				} else {
					decision = "skip"
				}
			}
		}

		importance := defaultCaptureImportance(eventKind, explicitImportance)
		memoryTemperature := defaultCaptureTemperature(eventKind, stringArg(args, "memory_temperature"), durableSignal || actionableEvent)

		payload := map[string]interface{}{
			"status":                "captured",
			"policy":                policy,
			"decision":              decision,
			"reason":                reason,
			"dry_run":               dryRun,
			"candidate_for_capture": candidateForExtraction,
			"signals": map[string]interface{}{
				"actionable_event":   actionableEvent,
				"durable_text":       durableSignal,
				"structured_context": structuredContext,
				"meaningful_text":    meaningfulText,
				"explicit_facts":     len(explicitFacts) > 0,
			},
			"store_episode":       storeEpisode,
			"store_facts":         storeFacts,
			"episode_stored":      false,
			"episode_path":        "",
			"event_id":            "",
			"lineage_id":          lineageID,
			"fact_source":         factSource,
			"facts_extracted":     len(facts),
			"facts_stored":        0,
			"stored_fact_paths":   []string{},
			"provided_fact_count": len(explicitFacts),
			"importance":          importance,
			"memory_temperature":  memoryTemperature,
		}

		if decision == "skip" {
			payload["status"] = "no_op"
			if dryRun {
				payload["status"] = "dry_run"
			}
			return marshalIndented(payload)
		}
		if dryRun {
			payload["status"] = "dry_run"
			return marshalIndented(payload)
		}

		var sourceEventID string
		if storeEpisode {
			episodePath, storedEventID, err := mem.StoreEpisode(text, vault.EpisodeWriteAuthority{
				ProducingOffice:     "main_brain",
				ProducingSubsystem:  "mcp_memory_capture",
				StaffingContext:     "mcp",
				AuthorityScope:      ledger.ScopeRuntimeMemoryStore,
				TargetDomain:        "memory/episodes",
				Source:              source,
				SourceRef:           sourceRef,
				SourceRefs:          sourceRefs,
				ProofRef:            "mcp-memory-capture",
				PromotionStatus:     "observed",
				EventID:             stringArg(args, "event_id"),
				LineageID:           lineageID,
				EventKind:           eventKind,
				Subject:             subject,
				Mission:             mission,
				WorkItemID:          workItemID,
				Environment:         environment,
				CueTerms:            cueTerms,
				RelatedFactPaths:    relatedFactPaths,
				RelatedEpisodePaths: relatedEpisodePaths,
				RelatedEntityRefs:   relatedEntityRefs,
				RelatedMissionRefs:  relatedMissionRefs,
				AllowApproval:       true,
			})
			if err != nil {
				return "", err
			}
			sourceEventID = storedEventID
			payload["episode_stored"] = true
			payload["episode_path"] = episodePath
			payload["event_id"] = storedEventID
			if strings.TrimSpace(lineageID) == "" {
				payload["lineage_id"] = storedEventID
			}
		}

		storedFactPaths := make([]string, 0, len(facts))
		if storeFacts {
			effectiveLineageID := lineageID
			if effectiveLineageID == "" {
				effectiveLineageID = sourceEventID
			}
			for _, fact := range facts {
				if strings.TrimSpace(fact.Subject) == "" || strings.TrimSpace(fact.Predicate) == "" || strings.TrimSpace(fact.Value) == "" {
					continue
				}
				path, err := mem.StoreFact(fact.Subject, fact.Predicate, fact.Value, confidence, importance, vault.FactWriteAuthority{
					ProducingOffice:     "main_brain",
					ProducingSubsystem:  "mcp_memory_capture",
					StaffingContext:     "mcp",
					AuthorityScope:      ledger.ScopeRuntimeMemoryStore,
					TargetDomain:        "memory/facts",
					Source:              source,
					SourceRef:           sourceRef,
					SourceRefs:          sourceRefs,
					SourceEventID:       sourceEventID,
					LineageID:           effectiveLineageID,
					Mission:             mission,
					WorkItemID:          workItemID,
					Environment:         environment,
					CueTerms:            cueTerms,
					MemoryTemperature:   memoryTemperature,
					RelatedFactPaths:    relatedFactPaths,
					RelatedEpisodePaths: relatedEpisodePaths,
					RelatedEntityRefs:   relatedEntityRefs,
					RelatedMissionRefs:  relatedMissionRefs,
					AllowApproval:       true,
					PromotionStatus:     "proposed",
					ProofRef:            "mcp-memory-capture",
				})
				if err != nil {
					return "", err
				}
				storedFactPaths = append(storedFactPaths, path)
			}
		}

		payload["facts_stored"] = len(storedFactPaths)
		payload["stored_fact_paths"] = storedFactPaths
		return marshalIndented(payload)
	}

	memoryCaptureSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text":               map[string]interface{}{"type": "string", "description": "Turn transcript, meeting note, or interaction summary to capture into memory"},
			"subject":            map[string]interface{}{"type": "string", "description": "Primary subject for the episodic capture"},
			"event_kind":         map[string]interface{}{"type": "string", "description": "interaction, decision, observation, correction, synthesis"},
			"source":             map[string]interface{}{"type": "string", "description": "Human-readable provenance label"},
			"source_ref":         map[string]interface{}{"type": "string", "description": "Canonical source reference for this turn"},
			"source_refs":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"event_id":           map[string]interface{}{"type": "string", "description": "Optional explicit event identifier"},
			"lineage_id":         map[string]interface{}{"type": "string", "description": "Optional shared lineage identifier"},
			"mission":            map[string]interface{}{"type": "string", "description": "Optional mission route anchor"},
			"work_item_id":       map[string]interface{}{"type": "string", "description": "Optional work-item lineage identifier"},
			"environment":        map[string]interface{}{"type": "string", "description": "Optional operating context"},
			"cue_terms":          map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"importance":         map[string]interface{}{"type": "string", "enum": []string{"critical", "high", "medium", "low"}},
			"confidence":         map[string]interface{}{"type": "number", "description": "Default confidence applied to stored facts"},
			"memory_temperature": map[string]interface{}{"type": "string", "enum": []string{"hot", "warm"}},
			"policy":             map[string]interface{}{"type": "string", "enum": []string{"minimal", "balanced", "strict", "everything"}, "description": "Admission policy for deciding whether this turn becomes an episode, facts, both, or nothing"},
			"store_episode":      map[string]interface{}{"type": "boolean", "description": "Allow episodic capture. Defaults to true."},
			"extract_facts":      map[string]interface{}{"type": "boolean", "description": "Allow automatic fact extraction when explicit facts are not supplied. Defaults to true."},
			"dry_run":            map[string]interface{}{"type": "boolean", "description": "Evaluate the policy and return the decision without writing anything"},
			"max_facts":          map[string]interface{}{"type": "integer", "description": "Maximum number of extracted facts to persist. Defaults to 5."},
			"facts": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"subject":   map[string]interface{}{"type": "string"},
						"predicate": map[string]interface{}{"type": "string"},
						"value":     map[string]interface{}{"type": "string"},
					},
					"required": []string{"subject", "predicate", "value"},
				},
				"description": "Optional explicit fact candidates. If omitted and extract_facts is true, Homing may extract them from the turn text.",
			},
			"related_fact_paths":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"related_episode_paths": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"related_entity_refs":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"related_mission_refs":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
		"required": []string{"text"},
	}

	srv.AddTool("memory_capture", "Capture a turn in one pass with admission policy: evaluate the text, decide whether it deserves durable memory, then store an episode, facts, both, or nothing.", memoryCaptureSchema, memoryCaptureHandler)

	memoryEpisodeStoreHandler := func(args map[string]interface{}) (string, error) {
		body, _ := args["body"].(string)
		eventKind, _ := args["event_kind"].(string)
		subject, _ := args["subject"].(string)
		source, _ := args["source"].(string)
		sourceRef, _ := args["source_ref"].(string)
		eventID, _ := args["event_id"].(string)
		lineageID, _ := args["lineage_id"].(string)
		mission, _ := args["mission"].(string)
		workItemID, _ := args["work_item_id"].(string)
		environment, _ := args["environment"].(string)
		relPath, storedEventID, err := mem.StoreEpisode(body, vault.EpisodeWriteAuthority{
			ProducingOffice:     "main_brain",
			ProducingSubsystem:  "mcp_memory_episode_store",
			StaffingContext:     "mcp",
			AuthorityScope:      ledger.ScopeRuntimeMemoryStore,
			TargetDomain:        "memory/episodes",
			Source:              source,
			SourceRef:           sourceRef,
			SourceRefs:          stringSliceArg(args["source_refs"]),
			ProofRef:            "mcp-memory-episode-store",
			PromotionStatus:     "observed",
			EventID:             eventID,
			LineageID:           lineageID,
			EventKind:           eventKind,
			Subject:             subject,
			Mission:             mission,
			WorkItemID:          workItemID,
			Environment:         environment,
			RelatedFactPaths:    stringSliceArg(args["related_fact_paths"]),
			RelatedEpisodePaths: stringSliceArg(args["related_episode_paths"]),
			RelatedEntityRefs:   stringSliceArg(args["related_entity_refs"]),
			RelatedMissionRefs:  stringSliceArg(args["related_mission_refs"]),
			CueTerms:            stringSliceArg(args["cue_terms"]),
			AllowApproval:       true,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Stored episode: %s (event_id: %s)", relPath, storedEventID), nil
	}

	memoryEpisodeStoreSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"body":       map[string]interface{}{"type": "string", "description": "Raw or near-raw episodic trace"},
			"event_kind": map[string]interface{}{"type": "string", "description": "observation, interaction, decision, recall, correction, synthesis"},
			"subject":    map[string]interface{}{"type": "string", "description": "Primary entity or topic"},
			"source":     map[string]interface{}{"type": "string", "description": "Human-readable provenance label"},
			"source_ref": map[string]interface{}{"type": "string", "description": "Canonical source reference"},
			"source_refs": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"event_id":     map[string]interface{}{"type": "string", "description": "Optional explicit event identifier"},
			"lineage_id":   map[string]interface{}{"type": "string", "description": "Optional shared lineage identifier"},
			"mission":      map[string]interface{}{"type": "string", "description": "Optional mission route anchor for this episode"},
			"work_item_id": map[string]interface{}{"type": "string", "description": "Optional work-item lineage identifier"},
			"environment":  map[string]interface{}{"type": "string", "description": "Optional environment or operating context"},
			"related_fact_paths": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Related semantic fact paths linked to this episode",
			},
			"related_episode_paths": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Related episodic memory paths linked to this episode",
			},
			"related_entity_refs": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Entity references linked to this episode",
			},
			"related_mission_refs": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Mission references linked to this episode",
			},
			"cue_terms": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		"required": []string{"body"},
	}

	srv.AddTool("memory_episode_store", "Store a first-class episodic memory object with event identity, lineage, content hash, and cue terms.", memoryEpisodeStoreSchema, memoryEpisodeStoreHandler)

	// --- Memory Learn (Corrections) ---

	memoryLearnHandler := func(args map[string]interface{}) (string, error) {
		original, _ := args["original"].(string)
		corrected, _ := args["corrected"].(string)
		context, _ := args["context"].(string)
		createdBy, _ := args["created_by"].(string)

		relPath, err := v.StoreCorrection(original, corrected, context, createdBy)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Correction stored: \"%s\" → \"%s\" at %s", original, corrected, relPath), nil
	}

	memoryLearnSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"original":   map[string]interface{}{"type": "string", "description": "The original phrasing or value that was incorrect or suboptimal"},
			"corrected":  map[string]interface{}{"type": "string", "description": "The corrected phrasing or value"},
			"context":    map[string]interface{}{"type": "string", "description": "Why this correction matters — what went wrong"},
			"created_by": map[string]interface{}{"type": "string", "description": "Who created this correction (model name, 'user', etc.)"},
		},
		"required": []string{"original", "corrected"},
	}

	srv.AddTool("memory_learn", "Record a correction pattern. When the original phrasing is encountered in future searches, the corrected version is surfaced as an annotation. Corrections are non-destructive — original facts are never rewritten.", memoryLearnSchema, memoryLearnHandler)

	// --- Memory Trace (Task Execution) ---

	memoryTraceHandler := func(args map[string]interface{}) (string, error) {
		task, _ := args["task"].(string)
		outcome, _ := args["outcome"].(string)
		createdBy, _ := args["created_by"].(string)
		model, _ := args["model"].(string)
		durationSec := 0.0
		if d, ok := args["duration_sec"].(float64); ok {
			durationSec = d
		}

		var steps []string
		if s, ok := args["steps"].([]interface{}); ok {
			for _, item := range s {
				if str, ok := item.(string); ok {
					steps = append(steps, str)
				}
			}
		}

		var toolsUsed []string
		if t, ok := args["tools_used"].([]interface{}); ok {
			for _, item := range t {
				if str, ok := item.(string); ok {
					toolsUsed = append(toolsUsed, str)
				}
			}
		}

		relPath, err := v.StoreTrace(task, outcome, steps, durationSec, toolsUsed, createdBy, model)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Trace stored: %s → %s at %s", task, outcome, relPath), nil
	}

	memoryTraceSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task":         map[string]interface{}{"type": "string", "description": "What task was attempted"},
			"outcome":      map[string]interface{}{"type": "string", "description": "Result: success, failure, partial, etc."},
			"steps":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Steps taken during execution"},
			"duration_sec": map[string]interface{}{"type": "number", "description": "How long the task took in seconds"},
			"tools_used":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Which tools were used"},
			"created_by":   map[string]interface{}{"type": "string", "description": "Who generated this trace (model name, agent role, etc.)"},
			"model":        map[string]interface{}{"type": "string", "description": "Which model/backend produced this trace"},
		},
		"required": []string{"task", "outcome"},
	}

	srv.AddTool("memory_trace", "Record a task execution trace. Traces are provenance-rich procedural memory — they record what was tried, what worked, what failed. Surfaced by memory_search when similar tasks are queried.", memoryTraceSchema, memoryTraceHandler)

	// --- memory_tune: Advisory FSRS Tuning ---

	memoryTuneSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"apply": map[string]interface{}{"type": "boolean", "description": "If true, promote proposals into active FSRS config. Default: false (dry run — analysis only)."},
		},
	}

	memoryTuneHandler := func(args map[string]interface{}) (string, error) {
		report, err := v.AnalyzeFSRS()
		if err != nil {
			return "", fmt.Errorf("FSRS analysis failed: %w", err)
		}

		// Save the report
		relPath, err := v.SaveTuneReport(report)
		if err != nil {
			return "", fmt.Errorf("failed to save tune report: %w", err)
		}

		apply, _ := args["apply"].(bool)
		if apply && len(report.Proposals) > 0 {
			if err := v.ApplyTuneReport(report); err != nil {
				return "", fmt.Errorf("failed to apply tune proposals: %w", err)
			}
			return fmt.Sprintf("Report saved to %s. Proposals applied to active FSRS config.\n\n%s", relPath, vault.FormatTuneReport(report)), nil
		}

		return fmt.Sprintf("Report saved to %s. No changes applied (dry run).\n\n%s", relPath, vault.FormatTuneReport(report)), nil
	}

	srv.AddTool("memory_tune", "Analyze FSRS retention patterns and propose parameter adjustments. Dry run by default — set apply: true to promote proposals into active config. Reports are versioned under memory/fsrs-tuning/.", memoryTuneSchema, memoryTuneHandler)

	// --- memory_maintain: Vault Health Maintenance ---

	memoryMaintainSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"consolidate", "contradict", "bootstrap", "replay", "structural", "hot", "elder", "all", "apply"},
				"description": "Which maintenance pass to run. 'apply' executes approved review artifacts. Default: all.",
			},
		},
	}

	memoryMaintainHandler := func(args map[string]interface{}) (string, error) {
		mode := maintain.ModeAll
		if m, ok := args["mode"].(string); ok {
			mode = maintain.Mode(m)
		}

		report, err := maintain.Run(v, mode, false)
		if err != nil {
			return "", fmt.Errorf("maintenance failed: %w", err)
		}

		return maintain.FormatReport(report), nil
	}

	srv.AddTool("memory_maintain", "Run vault health maintenance. Detects duplicate facts (consolidate), contradictions (contradict), extracts new facts from vault prose (bootstrap), proposes semantic promotions from replayed episodes and recall receipts (replay), proposes additive structural-link backfill from hard evidence already present in memory (structural), reviews hot-tier memory governance (hot), and reviews elder-memory protection plus elder anomalies (elder). Approved structural, temporal, hot, and elder review artifacts are applied through mode: apply. All outputs are review artifacts under memory/maintenance/ — nothing is silently rewritten.", memoryMaintainSchema, memoryMaintainHandler)

	// --- memory_hot_transition_propose: explicit warm/hot review proposal ---

	memoryHotTransitionSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"fact_path": map[string]interface{}{"type": "string", "description": "Relative path to the fact under memory/facts/."},
			"proposed_temperature": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"hot", "warm"},
				"description": "Requested new memory temperature.",
			},
			"reason": map[string]interface{}{"type": "string", "description": "Why this fact should change admission tier."},
		},
		"required": []string{"fact_path", "proposed_temperature", "reason"},
	}

	memoryHotTransitionHandler := func(args map[string]interface{}) (string, error) {
		factPath, _ := args["fact_path"].(string)
		proposedTemperature, _ := args["proposed_temperature"].(string)
		reason, _ := args["reason"].(string)
		if err := maintain.WriteHotMemoryTransitionCandidate(v, maintain.HotMemoryTransitionCandidate{
			FactPath:            factPath,
			ProposedTemperature: proposedTemperature,
			Reason:              reason,
			ProducingOffice:     "memory_governance",
			ProducingSubsystem:  "mcp_hot_memory_transition",
			StaffingContext:     "operator_mcp",
			AuthorityScope:      ledger.ScopeCandidateHotMemoryReview,
			ProofRef:            "mcp-hot-memory-transition:" + factPath,
		}); err != nil {
			return "", fmt.Errorf("failed to write hot memory transition candidate: %w", err)
		}
		return fmt.Sprintf("Hot memory transition candidate created for %s -> %s. Review the artifact under memory/maintenance/, set status: approved when appropriate, then run memory_maintain with mode: apply.", factPath, proposedTemperature), nil
	}

	srv.AddTool("memory_hot_transition_propose", "Create an explicit review artifact proposing a warm/hot memory transition for a fact. This never mutates the fact directly; approval and memory_maintain apply are required.", memoryHotTransitionSchema, memoryHotTransitionHandler)

	// --- memory_temporal_transition_propose: explicit temporal/supersession review proposal ---

	memoryTemporalTransitionSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"fact_path": map[string]interface{}{"type": "string", "description": "Relative path to the fact under memory/facts/."},
			"proposed_temporal_status": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"active", "superseded", "expired"},
				"description": "Requested new temporal status.",
			},
			"reason":      map[string]interface{}{"type": "string", "description": "Why this fact should change temporal status."},
			"observed_at": map[string]interface{}{"type": "string", "description": "Optional RFC3339 timestamp marking when the fact was observed."},
			"valid_from":  map[string]interface{}{"type": "string", "description": "Optional RFC3339 timestamp marking when the fact became true."},
			"valid_to":    map[string]interface{}{"type": "string", "description": "Optional RFC3339 timestamp marking when the fact ceased to be true."},
			"superseded_by_path": map[string]interface{}{
				"type":        "string",
				"description": "Required when superseding: the newer fact path that should replace this one.",
			},
		},
		"required": []string{"fact_path", "proposed_temporal_status", "reason"},
	}

	memoryTemporalTransitionHandler := func(args map[string]interface{}) (string, error) {
		factPath, _ := args["fact_path"].(string)
		proposedTemporalStatus, _ := args["proposed_temporal_status"].(string)
		reason, _ := args["reason"].(string)
		observedAt, _ := args["observed_at"].(string)
		validFrom, _ := args["valid_from"].(string)
		validTo, _ := args["valid_to"].(string)
		supersededByPath, _ := args["superseded_by_path"].(string)
		if err := maintain.WriteFactTemporalTransitionCandidate(v, maintain.FactTemporalTransitionCandidate{
			FactPath:               factPath,
			ProposedTemporalStatus: proposedTemporalStatus,
			Reason:                 reason,
			ObservedAt:             observedAt,
			ValidFrom:              validFrom,
			ValidTo:                validTo,
			SupersededByPath:       supersededByPath,
			ProducingOffice:        "memory_governance",
			ProducingSubsystem:     "mcp_temporal_transition",
			StaffingContext:        "operator_mcp",
			AuthorityScope:         ledger.ScopeCandidateFactTemporalReview,
			ProofRef:               "mcp-temporal-transition:" + factPath,
		}); err != nil {
			return "", fmt.Errorf("failed to write temporal transition candidate: %w", err)
		}
		return fmt.Sprintf("Temporal transition candidate created for %s -> %s. Review the artifact under memory/maintenance/, set status: approved when appropriate, then run memory_maintain with mode: apply.", factPath, proposedTemporalStatus), nil
	}

	srv.AddTool("memory_temporal_transition_propose", "Create an explicit review artifact proposing a temporal or supersession transition for a fact. This never mutates the fact directly; approval and memory_maintain apply are required.", memoryTemporalTransitionSchema, memoryTemporalTransitionHandler)

	// --- memory_elder_transition_propose: explicit elder-memory review proposal ---

	memoryElderTransitionSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"fact_path": map[string]interface{}{"type": "string", "description": "Relative path to the fact under memory/facts/."},
			"proposed_protection_class": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"elder", "ordinary"},
				"description": "Requested elder-memory protection class.",
			},
			"reason": map[string]interface{}{"type": "string", "description": "Why this fact should gain or lose elder-memory protection."},
		},
		"required": []string{"fact_path", "proposed_protection_class", "reason"},
	}

	memoryElderTransitionHandler := func(args map[string]interface{}) (string, error) {
		factPath, _ := args["fact_path"].(string)
		proposedProtectionClass, _ := args["proposed_protection_class"].(string)
		reason, _ := args["reason"].(string)
		if err := maintain.WriteElderMemoryTransitionCandidate(v, maintain.ElderMemoryTransitionCandidate{
			FactPath:                factPath,
			ProposedProtectionClass: proposedProtectionClass,
			Reason:                  reason,
			ProducingOffice:         "memory_governance",
			ProducingSubsystem:      "mcp_elder_memory_transition",
			StaffingContext:         "operator_mcp",
			AuthorityScope:          ledger.ScopeCandidateElderMemoryReview,
			ProofRef:                "mcp-elder-memory-transition:" + factPath,
		}); err != nil {
			return "", fmt.Errorf("failed to write elder memory transition candidate: %w", err)
		}
		return fmt.Sprintf("Elder memory transition candidate created for %s -> %s. Review the artifact under memory/maintenance/, set status: approved when appropriate, then run memory_maintain with mode: apply.", factPath, proposedProtectionClass), nil
	}

	srv.AddTool("memory_elder_transition_propose", "Create an explicit review artifact proposing elder-memory protection for a fact. This never mutates the fact directly; approval and memory_maintain apply are required.", memoryElderTransitionSchema, memoryElderTransitionHandler)

	// --- memory_secure_state: write or verify the memory secure-state manifest ---

	memorySecureStateSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"write", "verify"},
				"description": "Write a fresh secure-state manifest or verify current memory state against the latest manifest and ledger root.",
			},
		},
	}

	memorySecureStateHandler := func(args map[string]interface{}) (string, error) {
		mode, _ := args["mode"].(string)
		if strings.TrimSpace(mode) == "" {
			mode = "verify"
		}
		mem := memorykit.New(v)
		switch mode {
		case "write":
			result, err := mem.WriteSecureStateManifest()
			if err != nil {
				return "", fmt.Errorf("write secure-state manifest: %w", err)
			}
			return fmt.Sprintf("Memory secure-state manifest written at %s with root %s over %d files. Canonical=%d, operational=%d, sealed=%d.", result.ManifestPath, result.Manifest.RootHash, result.Manifest.FileCount, result.Manifest.ClassCounts["canonical"], result.Manifest.ClassCounts["operational"], result.Manifest.ClassCounts["sealed"]), nil
		case "verify":
			result, err := mem.VerifySecureStateManifest()
			if err != nil {
				return "", fmt.Errorf("verify secure-state manifest: %w", err)
			}
			if result.Verified {
				return fmt.Sprintf("Memory secure-state verified against %s. Root %s matches current state and no rollback suspicion was detected.", result.ManifestPath, result.ExpectedRootHash), nil
			}
			return fmt.Sprintf("Memory secure-state verification failed against %s. expected=%s current=%s ledger=%s rollback_suspected=%t drift_paths=%d", result.ManifestPath, result.ExpectedRootHash, result.CurrentRootHash, result.LedgerRootHash, result.RollbackSuspected, len(result.DriftPaths)), nil
		default:
			return "", fmt.Errorf("unsupported mode: %s", mode)
		}
	}

	srv.AddTool("memory_secure_state", "Write or verify the memory secure-state manifest. This provides a mediated snapshot of the sovereign memory estate and checks the current state against the latest manifest and ledger root for drift or rollback suspicion.", memorySecureStateSchema, memorySecureStateHandler)

	// --- memory_evaluate: run the synthetic Grade S memory evaluation harness ---

	memoryEvaluateSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	memoryEvaluateHandler := func(args map[string]interface{}) (string, error) {
		result, err := mem.Evaluate()
		if err != nil {
			return "", fmt.Errorf("run memory evaluation: %w", err)
		}
		return fmt.Sprintf("Memory evaluation completed with score %.2f across %d cases (%d passed). Report written at %s.", result.Report.OverallScore, result.Report.TotalCases, result.Report.PassedCases, result.ReportPath), nil
	}

	srv.AddTool("memory_evaluate", "Run the Grade S synthetic memory evaluation harness. This scores interference recall, elder retention, replay promotion, hot-tier stale detection, and secure-state tamper and rollback detection, then writes a ledger-backed report under state/memory/evaluations/.", memoryEvaluateSchema, memoryEvaluateHandler)

	// --- memory_trial_run: run the live sovereign-vault memory trial harness ---

	memoryTrialRunSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	memoryTrialRunHandler := func(args map[string]interface{}) (string, error) {
		result, err := mem.RunTrials()
		if err != nil {
			return "", fmt.Errorf("run memory trials: %w", err)
		}
		return fmt.Sprintf("Memory trial report completed with score %.2f across %d live cases (%d passed). Report written at %s.", result.Report.OverallScore, result.Report.TotalCases, result.Report.PassedCases, result.ReportPath), nil
	}

	srv.AddTool("memory_trial_run", "Run the live sovereign-vault memory trial harness over authored trial cases under state/memory/trials/cases/. This grades the present memory estate rather than a synthetic fixture corpus.", memoryTrialRunSchema, memoryTrialRunHandler)

	// --- memory_readiness: write a pretesting readiness report over the live sovereign estate ---

	memoryReadinessSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	memoryReadinessHandler := func(args map[string]interface{}) (string, error) {
		result, err := mem.RunReadiness()
		if err != nil {
			return "", fmt.Errorf("run memory readiness audit: %w", err)
		}
		return fmt.Sprintf("Memory readiness report written at %s. status=%s issues=%d trial_score=%.2f evaluation_score=%.2f portability_score=%.2f secure_state_verified=%t", result.ReportPath, result.Report.Status, len(result.Report.Issues), result.Report.TrialScore, result.Report.EvaluationScore, result.Report.PortabilityScore, result.Report.SecureStateVerified), nil
	}

	srv.AddTool("memory_readiness", "Write a pretesting readiness report over the live sovereign memory estate. This refreshes the secure-state manifest, verifies it, inspects the live memory shelves and current reports, and writes a canonical readiness surface under state/memory/readiness/.", memoryReadinessSchema, memoryReadinessHandler)

	// --- memory_portability_audit: compare external Claude cache coverage against sovereign memory ---

	memoryPortabilitySchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cache_path": map[string]interface{}{
				"type":        "string",
				"description": "Optional override for the external Claude project memory directory. Defaults to the current project's ~/.claude cache path.",
			},
		},
	}

	memoryPortabilityHandler := func(args map[string]interface{}) (string, error) {
		cachePath, _ := args["cache_path"].(string)
		result, err := mem.AuditPortability(cachePath)
		if err != nil {
			return "", fmt.Errorf("run memory portability audit: %w", err)
		}
		return fmt.Sprintf("Memory portability audit written at %s. cache_present=%t files=%d covered=%d external_only=%d score=%.2f", result.ReportPath, result.Report.CachePresent, result.Report.TotalFiles, result.Report.CoveredFiles, result.Report.ExternalOnly, result.Report.CoverageScore), nil
	}

	srv.AddTool("memory_portability_audit", "Audit the current project's external Claude memory cache against sovereign MODUS memory. This writes a conservative coverage report under state/memory/portability/ without mutating any durable memory.", memoryPortabilitySchema, memoryPortabilityHandler)

	// --- memory_portability_queue: turn external-only residue into explicit migration work ---

	memoryPortabilityQueueHandler := func(args map[string]interface{}) (string, error) {
		cachePath, _ := args["cache_path"].(string)
		result, err := mem.BuildPortabilityQueue(cachePath)
		if err != nil {
			return "", fmt.Errorf("build memory portability queue: %w", err)
		}
		return fmt.Sprintf("Memory portability queue written at %s. items=%d critical=%d high=%d medium=%d low=%d", result.ReportPath, result.Report.TotalItems, result.Report.PriorityCounts["critical"], result.Report.PriorityCounts["high"], result.Report.PriorityCounts["medium"], result.Report.PriorityCounts["low"]), nil
	}

	srv.AddTool("memory_portability_queue", "Refresh the Claude-cache portability audit and write a read-only migration queue for external-only residue under state/memory/portability/. This never mutates durable memory.", memoryPortabilitySchema, memoryPortabilityQueueHandler)

	// --- memory_portability_archive: copy remaining external-only residue into sovereign archival custody ---

	memoryPortabilityArchiveHandler := func(args map[string]interface{}) (string, error) {
		cachePath, _ := args["cache_path"].(string)
		result, err := mem.ArchivePortabilityResidue(cachePath)
		if err != nil {
			return "", fmt.Errorf("archive memory portability residue: %w", err)
		}
		return fmt.Sprintf("Memory portability archive written at %s. archived=%d destination=%s", result.ReportPath, result.Report.ArchivedCount, result.Report.DestinationRoot), nil
	}

	srv.AddTool("memory_portability_archive", "Refresh the portability queue and copy the remaining external-only Claude cache residue into sovereign archival custody under vault/brain/claude-memory-archive/. This does not promote the archived content into canonical fact memory.", memoryPortabilitySchema, memoryPortabilityArchiveHandler)

	// --- memory_train: Offline Training Pipeline ---

	memoryTrainSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"model_path":    map[string]interface{}{"type": "string", "description": "Path to base model (GGUF or MLX). Required for training."},
			"generate_only": map[string]interface{}{"type": "boolean", "description": "If true, only generate training data without running training. Default: false."},
			"promote":       map[string]interface{}{"type": "boolean", "description": "If true, promote the best unpromoted adapter. Requires prior successful training run."},
			"note":          map[string]interface{}{"type": "string", "description": "Promotion note (why this adapter is being promoted)."},
		},
	}

	memoryTrainHandler := func(args map[string]interface{}) (string, error) {
		promote, _ := args["promote"].(bool)
		if promote {
			note, _ := args["note"].(string)
			best := trainer.BestUnpromotedRun(v.Dir)
			if best == nil {
				return "No unpromoted training runs found. Run training first.", nil
			}
			// Run promotion gate check first
			pass, reason := trainer.PromotionCheck(v.Dir, best)
			if !pass {
				return fmt.Sprintf("Promotion BLOCKED for run %s: %s", best.Timestamp, reason), nil
			}
			if err := trainer.PromoteRun(v.Dir, best, note); err != nil {
				return "", fmt.Errorf("promote failed: %w", err)
			}
			return fmt.Sprintf("Promoted adapter from run %s.\nGate: %s\nNote: %s", best.Timestamp, reason, note), nil
		}

		generateOnly, _ := args["generate_only"].(bool)
		outputDir := v.Path("memory", "training-data")

		if generateOnly {
			batch, err := trainer.GenerateBatch(v)
			if err != nil {
				return "", fmt.Errorf("generate failed: %w", err)
			}
			if err := trainer.WriteBatch(batch, outputDir); err != nil {
				return "", fmt.Errorf("write failed: %w", err)
			}
			sft, dpo := trainer.CountPairs(outputDir)
			return fmt.Sprintf("Training data generated: %d SFT pairs, %d DPO triples.\nOutput: %s\nMinimum for training: 50 pairs. Current: %d.",
				sft, dpo, outputDir, sft+dpo), nil
		}

		modelPath, _ := args["model_path"].(string)
		if modelPath == "" {
			return "model_path required for training. Use generate_only: true to just generate data.", nil
		}

		result, err := trainer.RunTrainingLoop(v, modelPath, outputDir)
		if err != nil {
			return "", fmt.Errorf("training loop failed: %w", err)
		}

		return trainer.FormatLoopResult(result), nil
	}

	srv.AddTool("memory_train", "Generate training data from vault activity and optionally train a LoRA adapter. Offline pipeline — does not affect serving. Adapter promotion requires explicit promote action.", memoryTrainSchema, memoryTrainHandler)

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
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "trust_office",
			Subsystem:      "mcp_atlas_set_trust",
			AuthorityScope: ledger.ScopeOperatorTrustStageChange,
			ActionClass:    ledger.ActionTrustStageTransitionRequest,
			TargetDomain:   "atlas/trust",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionApprovalRequired,
			SideEffects:    []string{"trust_stage_change_requested"},
			ProofRefs:      []string{"atlas/trust.md"},
			Signature: signature.Signature{
				ProducingOffice:    "trust_office",
				ProducingSubsystem: "mcp_atlas_set_trust",
				StaffingContext:    updatedBy,
				AuthorityScope:     ledger.ScopeOperatorTrustStageChange,
				ArtifactState:      "evidentiary",
				SourceRefs:         []string{"atlas/trust.md"},
				PromotionStatus:    "approved",
			},
			Metadata: map[string]interface{}{
				"stage":      stage,
				"updated_by": updatedBy,
				"reason":     reason,
			},
		})
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
			"mission":    map[string]interface{}{"type": "string", "description": "Mission slug that has the dependency"},
			"depends_on": map[string]interface{}{"type": "string", "description": "Mission slug it depends on"},
			"type":       map[string]interface{}{"type": "string", "description": "blocks, informs, or enhances"},
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
			"mission":    map[string]interface{}{"type": "string", "description": "Mission slug"},
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

package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/vault"
)

func TestMemoryCaptureStoresEpisodeAndFacts(t *testing.T) {
	v := vault.New(t.TempDir(), nil)
	srv := NewServer("test", "0")
	RegisterVaultTools(srv, v)

	if !srv.HasTool("memory_capture") {
		t.Fatal("expected memory_capture tool to be registered")
	}

	result, err := srv.CallTool("memory_capture", map[string]interface{}{
		"text":               "The General prefers TypeScript for new projects and wants concise commit messages.",
		"subject":            "General",
		"event_kind":         "interaction",
		"source":             "cursor session",
		"source_ref":         "cursor://chat/turn-1",
		"lineage_id":         "lin-general-preferences",
		"environment":        "cursor",
		"policy":             "balanced",
		"memory_temperature": "hot",
		"facts": []interface{}{
			map[string]interface{}{"subject": "General", "predicate": "prefers", "value": "TypeScript for new projects"},
			map[string]interface{}{"subject": "General", "predicate": "wants", "value": "concise commit messages"},
		},
	})
	if err != nil {
		t.Fatalf("memory_capture: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("parse memory_capture payload: %v", err)
	}
	if got := payload["decision"]; got != "episode_and_facts" {
		t.Fatalf("decision = %v, want episode_and_facts", got)
	}
	if got := payload["facts_stored"]; got != float64(2) {
		t.Fatalf("facts_stored = %v, want 2", got)
	}
	if got := payload["episode_stored"]; got != true {
		t.Fatalf("episode_stored = %v, want true", got)
	}

	episodeFiles, err := filepath.Glob(filepath.Join(v.Dir, "memory", "episodes", "*.md"))
	if err != nil {
		t.Fatalf("glob episodes: %v", err)
	}
	if len(episodeFiles) != 1 {
		t.Fatalf("expected 1 episode file, got %d", len(episodeFiles))
	}
	episodeRel, err := filepath.Rel(v.Dir, episodeFiles[0])
	if err != nil {
		t.Fatalf("rel episode path: %v", err)
	}
	episodeDoc, err := v.Read(episodeRel)
	if err != nil {
		t.Fatalf("read episode: %v", err)
	}
	if episodeDoc.Get("captured_by_subsystem") != "mcp_memory_capture" {
		t.Fatalf("captured_by_subsystem = %q, want mcp_memory_capture", episodeDoc.Get("captured_by_subsystem"))
	}

	factFiles, err := filepath.Glob(filepath.Join(v.Dir, "memory", "facts", "*.md"))
	if err != nil {
		t.Fatalf("glob facts: %v", err)
	}
	if len(factFiles) != 2 {
		t.Fatalf("expected 2 fact files, got %d", len(factFiles))
	}
	for _, factFile := range factFiles {
		relPath, err := filepath.Rel(v.Dir, factFile)
		if err != nil {
			t.Fatalf("rel fact path: %v", err)
		}
		doc, err := v.Read(relPath)
		if err != nil {
			t.Fatalf("read fact: %v", err)
		}
		if doc.Get("captured_by_subsystem") != "mcp_memory_capture" {
			t.Fatalf("captured_by_subsystem = %q, want mcp_memory_capture", doc.Get("captured_by_subsystem"))
		}
		if doc.Get("memory_temperature") != "hot" {
			t.Fatalf("memory_temperature = %q, want hot", doc.Get("memory_temperature"))
		}
		if doc.Get("lineage_id") != "lin-general-preferences" {
			t.Fatalf("lineage_id = %q, want lin-general-preferences", doc.Get("lineage_id"))
		}
		if strings.TrimSpace(doc.Get("source_event_id")) == "" {
			t.Fatal("expected source_event_id to be persisted")
		}
	}
}

func TestMemoryCaptureStrictDryRunSkipsCasualTurn(t *testing.T) {
	v := vault.New(t.TempDir(), nil)
	srv := NewServer("test", "0")
	RegisterVaultTools(srv, v)

	result, err := srv.CallTool("memory_capture", map[string]interface{}{
		"text":       "Thanks, sounds good.",
		"policy":     "strict",
		"dry_run":    true,
		"subject":    "General",
		"source":     "cursor session",
		"event_kind": "interaction",
	})
	if err != nil {
		t.Fatalf("memory_capture dry_run: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("parse dry_run payload: %v", err)
	}
	if got := payload["status"]; got != "dry_run" {
		t.Fatalf("status = %v, want dry_run", got)
	}
	if got := payload["decision"]; got != "skip" {
		t.Fatalf("decision = %v, want skip", got)
	}

	factFiles, err := filepath.Glob(filepath.Join(v.Dir, "memory", "facts", "*.md"))
	if err != nil {
		t.Fatalf("glob facts: %v", err)
	}
	if len(factFiles) != 0 {
		t.Fatalf("expected no fact files, got %d", len(factFiles))
	}
}

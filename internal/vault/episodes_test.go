package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/ledger"
)

func TestStoreEpisodeGovernedAssignsIdentityAndHash(t *testing.T) {
	v := testVault(t)

	relPath, eventID, err := v.StoreEpisodeGoverned("The General approved the memory sovereignty campaign.", EpisodeWriteAuthority{
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "episode_test",
		StaffingContext:     "operator_test",
		AuthorityScope:      ledger.ScopeOperatorMemoryStore,
		TargetDomain:        "memory/episodes",
		Source:              "operator request",
		SourceRef:           "vault/sessions/journal.md",
		SourceRefs:          []string{"vault/modus/memory-architecture-doctrine.md"},
		PromotionStatus:     "observed",
		EventKind:           "decision",
		Subject:             "Memory Sovereignty",
		Mission:             "Memory Sovereignty",
		WorkItemID:          "work-memory-sovereignty",
		Environment:         "operator-shell",
		RelatedFactPaths:    []string{"memory/facts/modus-memory-priority.md"},
		RelatedEpisodePaths: []string{"memory/episodes/evt-memory-origin.md"},
		RelatedEntityRefs:   []string{"General", "MODUS"},
		RelatedMissionRefs:  []string{"Memory Sovereignty", "Consciousness Loop"},
		CueTerms:            []string{"memory", "sovereignty", "campaign"},
		AllowApproval:       true,
	})
	if err != nil {
		t.Fatalf("StoreEpisodeGoverned: %v", err)
	}
	if !strings.HasPrefix(relPath, "memory/episodes/") {
		t.Fatalf("relPath = %q, want memory/episodes path", relPath)
	}
	if strings.TrimSpace(eventID) == "" {
		t.Fatal("eventID should not be empty")
	}

	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read episode: %v", err)
	}
	if doc.Get("type") != "memory_episode" {
		t.Fatalf("type = %q, want memory_episode", doc.Get("type"))
	}
	if doc.Get("event_id") != eventID {
		t.Fatalf("event_id = %q, want %q", doc.Get("event_id"), eventID)
	}
	if doc.Get("lineage_id") != eventID {
		t.Fatalf("lineage_id = %q, want default %q", doc.Get("lineage_id"), eventID)
	}
	if doc.Get("content_hash") == "" {
		t.Fatal("content_hash should not be empty")
	}
	if doc.Get("captured_by_office") != "memory_governance" {
		t.Fatalf("captured_by_office = %q, want memory_governance", doc.Get("captured_by_office"))
	}
	if doc.Get("event_kind") != "decision" {
		t.Fatalf("event_kind = %q, want decision", doc.Get("event_kind"))
	}
	if doc.Get("mission") != "Memory Sovereignty" {
		t.Fatalf("mission = %q, want Memory Sovereignty", doc.Get("mission"))
	}
	if doc.Get("work_item_id") != "work-memory-sovereignty" {
		t.Fatalf("work_item_id = %q, want work-memory-sovereignty", doc.Get("work_item_id"))
	}
	if doc.Get("environment") != "operator-shell" {
		t.Fatalf("environment = %q, want operator-shell", doc.Get("environment"))
	}
	if raw, ok := doc.Frontmatter["related_fact_paths"].([]interface{}); !ok || len(raw) != 1 || raw[0] != "memory/facts/modus-memory-priority.md" {
		t.Fatalf("related_fact_paths = %#v", doc.Frontmatter["related_fact_paths"])
	}
	if raw, ok := doc.Frontmatter["related_episode_paths"].([]interface{}); !ok || len(raw) != 1 || raw[0] != "memory/episodes/evt-memory-origin.md" {
		t.Fatalf("related_episode_paths = %#v", doc.Frontmatter["related_episode_paths"])
	}
	if raw, ok := doc.Frontmatter["related_entity_refs"].([]interface{}); !ok || len(raw) != 2 {
		t.Fatalf("related_entity_refs = %#v", doc.Frontmatter["related_entity_refs"])
	}
	if raw, ok := doc.Frontmatter["related_mission_refs"].([]interface{}); !ok || len(raw) != 2 {
		t.Fatalf("related_mission_refs = %#v", doc.Frontmatter["related_mission_refs"])
	}

	raw, ok := doc.Frontmatter["cue_terms"].([]interface{})
	if !ok || len(raw) != 3 {
		t.Fatalf("cue_terms = %#v, want 3 items", doc.Frontmatter["cue_terms"])
	}

	ledgerData, err := os.ReadFile(filepath.Join(v.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(ledgerData)), "\n")
	var rec map[string]interface{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != ledger.ActionMemoryEpisodeCreation {
		t.Fatalf("action_class = %v, want %s", rec["action_class"], ledger.ActionMemoryEpisodeCreation)
	}
}

func TestStoreFactGovernedPersistsEpisodeLineage(t *testing.T) {
	v := testVault(t)

	relPath, err := v.StoreFactGoverned("MODUS", "priority", "memory sovereignty", 0.95, "critical", FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "fact_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		Source:             "operator request",
		SourceRef:          "vault/sessions/journal.md",
		SourceEventID:      "evt-test-001",
		LineageID:          "lin-test-001",
		Mission:            "Memory Sovereignty",
		WorkItemID:         "work-memory-sovereignty",
		Environment:        "operator-shell",
		CueTerms:           []string{"memory", "priority", "sovereignty"},
		AllowApproval:      true,
		PromotionStatus:    "approved",
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}

	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read fact: %v", err)
	}
	if doc.Get("source_event_id") != "evt-test-001" {
		t.Fatalf("source_event_id = %q, want evt-test-001", doc.Get("source_event_id"))
	}
	if doc.Get("lineage_id") != "lin-test-001" {
		t.Fatalf("lineage_id = %q, want lin-test-001", doc.Get("lineage_id"))
	}
	if doc.Get("mission") != "Memory Sovereignty" {
		t.Fatalf("mission = %q, want Memory Sovereignty", doc.Get("mission"))
	}
	if doc.Get("work_item_id") != "work-memory-sovereignty" {
		t.Fatalf("work_item_id = %q, want work-memory-sovereignty", doc.Get("work_item_id"))
	}
	if doc.Get("environment") != "operator-shell" {
		t.Fatalf("environment = %q, want operator-shell", doc.Get("environment"))
	}
	raw, ok := doc.Frontmatter["cue_terms"].([]interface{})
	if !ok || len(raw) != 3 {
		t.Fatalf("cue_terms = %#v, want 3 items", doc.Frontmatter["cue_terms"])
	}
}

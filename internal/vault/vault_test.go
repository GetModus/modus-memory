package vault

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
)

// testVault creates a temporary vault directory with optional seed files.
func testVault(t *testing.T) *Vault {
	t.Helper()
	dir := t.TempDir()
	return New(dir, nil)
}

// seedFile writes a markdown file into the vault.
func seedFile(t *testing.T, v *Vault, relPath string, fm map[string]interface{}, body string) {
	t.Helper()
	path := filepath.Join(v.Dir, relPath)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := markdown.Write(path, fm, body); err != nil {
		t.Fatalf("seed %s: %v", relPath, err)
	}
}

// --- Core Vault ---

func TestNew(t *testing.T) {
	v := testVault(t)
	if v.Dir == "" {
		t.Fatal("vault dir is empty")
	}
	if v.Index != nil {
		t.Fatal("expected nil index")
	}
}

func TestPath(t *testing.T) {
	v := testVault(t)
	got := v.Path("memory", "facts", "test.md")
	want := filepath.Join(v.Dir, "memory", "facts", "test.md")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestReadWrite(t *testing.T) {
	v := testVault(t)

	fm := map[string]interface{}{"title": "test", "score": 42}
	err := v.Write("docs/test.md", fm, "Hello world")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	doc, err := v.Read("docs/test.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if doc.Get("title") != "test" {
		t.Errorf("title = %q, want %q", doc.Get("title"), "test")
	}
	if !strings.Contains(doc.Body, "Hello world") {
		t.Errorf("body missing expected content")
	}
}

func TestList(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "notes/a.md", map[string]interface{}{"status": "active"}, "A")
	seedFile(t, v, "notes/b.md", map[string]interface{}{"status": "done"}, "B")
	seedFile(t, v, "notes/c.md", map[string]interface{}{"status": "active"}, "C")

	// Unfiltered
	docs, err := v.List("notes")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("List returned %d docs, want 3", len(docs))
	}

	// Include filter
	active, err := v.List("notes", Filter{Field: "status", Value: "active"})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("List(status=active) returned %d, want 2", len(active))
	}

	// Exclude filter
	notDone, err := v.List("notes", Filter{Field: "status", Value: "done", Exclude: true})
	if err != nil {
		t.Fatalf("List exclude: %v", err)
	}
	if len(notDone) != 2 {
		t.Errorf("List(status!=done) returned %d, want 2", len(notDone))
	}
}

func TestStatus(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "brain/a.md", nil, "A")
	seedFile(t, v, "brain/b.md", nil, "B")
	seedFile(t, v, "memory/c.md", nil, "C")

	status := v.Status()
	total, ok := status["total_files"].(int)
	if !ok || total != 3 {
		t.Errorf("total_files = %v, want 3", status["total_files"])
	}

	breakdown := status["breakdown"].(map[string]int)
	if breakdown["brain"] != 2 {
		t.Errorf("brain count = %d, want 2", breakdown["brain"])
	}
}

func TestSearchNoIndex(t *testing.T) {
	v := testVault(t)
	_, err := v.Search("test", 10)
	if err == nil {
		t.Error("expected error when searching without index")
	}
}

// --- Facts ---

func TestStoreFact(t *testing.T) {
	v := testVault(t)

	relPath, err := v.StoreFact("Go", "is", "a programming language", 0.9, "high")
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}
	if !strings.HasPrefix(relPath, "memory/facts/") {
		t.Errorf("relPath = %q, want prefix memory/facts/", relPath)
	}

	// Verify file exists and has correct frontmatter
	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read stored fact: %v", err)
	}
	if doc.Get("subject") != "Go" {
		t.Errorf("subject = %q, want %q", doc.Get("subject"), "Go")
	}
	if doc.GetFloat("confidence") != 0.9 {
		t.Errorf("confidence = %v, want 0.9", doc.GetFloat("confidence"))
	}
	if doc.Get("memory_temperature") != "warm" {
		t.Errorf("memory_temperature = %q, want warm", doc.Get("memory_temperature"))
	}
	if doc.Get("created_at") == "" {
		t.Error("created_at should be populated")
	}
}

func TestStoreFactDefaults(t *testing.T) {
	v := testVault(t)

	_, err := v.StoreFact("X", "is", "Y", 0, "")
	if err != nil {
		t.Fatalf("StoreFact: %v", err)
	}

	docs, _ := v.ListFacts("X", 10)
	if len(docs) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(docs))
	}
	if docs[0].GetFloat("confidence") != 0.8 {
		t.Errorf("default confidence = %v, want 0.8", docs[0].GetFloat("confidence"))
	}
	if docs[0].Get("importance") != "medium" {
		t.Errorf("default importance = %q, want medium", docs[0].Get("importance"))
	}
	if docs[0].Get("memory_temperature") != "warm" {
		t.Errorf("default memory_temperature = %q, want warm", docs[0].Get("memory_temperature"))
	}
}

func TestStoreFactGovernedAllowsApprovedWrite(t *testing.T) {
	v := testVault(t)

	relPath, err := v.StoreFactGoverned("Go", "supports", "governed writes", 0.95, "high", FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "test_surface",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		Source:             "operator request",
		SourceRef:          "vault/sessions/journal.md",
		SourceRefs:         []string{"vault/sessions/journal.md", "vault/modus/memory-architecture-doctrine.md"},
		MemoryTemperature:  "hot",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}
	if !strings.HasPrefix(relPath, "memory/facts/") {
		t.Fatalf("relPath = %q, want fact path", relPath)
	}

	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read governed fact: %v", err)
	}
	if doc.Get("captured_by_office") != "memory_governance" {
		t.Errorf("captured_by_office = %q, want memory_governance", doc.Get("captured_by_office"))
	}
	if doc.Get("captured_by_subsystem") != "test_surface" {
		t.Errorf("captured_by_subsystem = %q, want test_surface", doc.Get("captured_by_subsystem"))
	}
	if doc.Get("source") != "operator request" {
		t.Errorf("source = %q, want operator request", doc.Get("source"))
	}
	if doc.Get("source_ref") != "vault/sessions/journal.md" {
		t.Errorf("source_ref = %q, want vault/sessions/journal.md", doc.Get("source_ref"))
	}
	if doc.Get("memory_temperature") != "hot" {
		t.Errorf("memory_temperature = %q, want hot", doc.Get("memory_temperature"))
	}
}

func TestStoreFactGovernedPersistsElderProtectionClass(t *testing.T) {
	v := testVault(t)

	relPath, err := v.StoreFactGoverned("Founding lesson", "requires", "explicit governance", 0.94, "critical", FactWriteAuthority{
		ProducingOffice:       "memory_governance",
		ProducingSubsystem:    "test_surface",
		StaffingContext:       "operator_test",
		AuthorityScope:        ledger.ScopeOperatorMemoryStore,
		TargetDomain:          "memory/facts",
		Source:                "campaign journal",
		SourceRef:             "vault/sessions/2026-04-14-grade-s-memory-program.md",
		MemoryProtectionClass: "elder",
		AllowApproval:         true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}

	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read governed fact: %v", err)
	}
	if doc.Get("memory_protection_class") != "elder" {
		t.Errorf("memory_protection_class = %q, want elder", doc.Get("memory_protection_class"))
	}
}

func TestStoreFactGovernedPersistsTemporalFieldsAndSupersedes(t *testing.T) {
	v := testVault(t)

	relPath, err := v.StoreFactGoverned("Scout lane", "uses", "gemini-2.5-flash", 0.93, "high", FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "test_surface",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		ObservedAt:         "2026-04-15T10:00:00Z",
		ValidFrom:          "2026-04-15T10:00:00Z",
		ValidTo:            "2026-04-20T10:00:00Z",
		SupersedesPaths:    []string{"memory/facts/scout-lane-legacy.md"},
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}

	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read governed fact: %v", err)
	}
	if doc.Get("observed_at") != "2026-04-15T10:00:00Z" {
		t.Fatalf("observed_at = %q, want 2026-04-15T10:00:00Z", doc.Get("observed_at"))
	}
	if doc.Get("valid_from") != "2026-04-15T10:00:00Z" {
		t.Fatalf("valid_from = %q, want 2026-04-15T10:00:00Z", doc.Get("valid_from"))
	}
	if doc.Get("valid_to") != "2026-04-20T10:00:00Z" {
		t.Fatalf("valid_to = %q, want 2026-04-20T10:00:00Z", doc.Get("valid_to"))
	}
	if doc.Get("temporal_status") != "active" {
		t.Fatalf("temporal_status = %q, want active", doc.Get("temporal_status"))
	}
	raw := doc.Frontmatter["supersedes_paths"]
	items, ok := raw.([]interface{})
	if !ok || len(items) != 1 || items[0] != "memory/facts/scout-lane-legacy.md" {
		t.Fatalf("supersedes_paths = %#v, want [memory/facts/scout-lane-legacy.md]", raw)
	}
}

func TestStoreFactGovernedBlocksUnapprovedWrite(t *testing.T) {
	v := testVault(t)

	_, err := v.StoreFactGoverned("Go", "supports", "unguarded writes", 0.95, "high", FactWriteAuthority{
		ProducingOffice:    "main_brain",
		ProducingSubsystem: "test_surface",
		StaffingContext:    "agent_test",
		AuthorityScope:     "trust_gated_memory_store",
		TargetDomain:       "memory/facts",
		AllowApproval:      false,
	})
	if err == nil {
		t.Fatal("expected trust gate error")
	}
	if !strings.Contains(err.Error(), "blocked by trust gate") {
		t.Fatalf("error = %v, want trust gate block", err)
	}
}

func TestStoreFactDuplicate(t *testing.T) {
	v := testVault(t)

	path1, _ := v.StoreFact("Go", "is", "fast", 0.9, "high")
	path2, _ := v.StoreFact("Go", "is", "compiled", 0.8, "high")

	if path1 == path2 {
		t.Error("duplicate slugs should produce different paths")
	}
}

func TestListFacts(t *testing.T) {
	v := testVault(t)

	v.StoreFact("Go", "is", "fast", 0.9, "high")
	v.StoreFact("Python", "is", "dynamic", 0.8, "medium")
	v.StoreFact("Go", "has", "goroutines", 0.85, "high")

	// All facts
	all, err := v.ListFacts("", 10)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListFacts('') = %d, want 3", len(all))
	}

	// Filter by subject
	goFacts, err := v.ListFacts("Go", 10)
	if err != nil {
		t.Fatalf("ListFacts(Go): %v", err)
	}
	if len(goFacts) != 2 {
		t.Errorf("ListFacts('Go') = %d, want 2", len(goFacts))
	}

	// Limit
	limited, _ := v.ListFacts("", 1)
	if len(limited) != 1 {
		t.Errorf("ListFacts limit=1 returned %d", len(limited))
	}
}

func TestSearchFactsNoIndex(t *testing.T) {
	v := testVault(t)

	v.StoreFact("Go", "is", "fast and concurrent", 0.9, "high")
	v.StoreFact("Python", "is", "slow but readable", 0.8, "medium")

	results, err := v.SearchFacts("fast", 10)
	if err != nil {
		t.Fatalf("SearchFacts: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchFacts('fast') = %d results, want 1", len(results))
	}
}

func TestSearchFactsReranksTowardProvenanceAndTemperature(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/provenance-rich.md", map[string]interface{}{
		"subject":            "General briefing preference",
		"predicate":          "prefers",
		"confidence":         0.82,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		"created":            time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		"source":             "session continuity",
		"source_ref":         "vault/sessions/journal.md",
		"captured_by_office": "librarian",
	}, "The General prefers concise briefings with actionable detail.")
	seedFile(t, v, "memory/facts/thin.md", map[string]interface{}{
		"subject":    "General briefing notes",
		"predicate":  "mentions",
		"confidence": 0.82,
		"importance": "high",
		"created_at": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		"created":    time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
	}, "The General prefers concise briefings and actionable detail.")

	idx, err := index.Build(v.Dir, "")
	if err != nil {
		t.Fatalf("Build index: %v", err)
	}
	defer idx.Close()
	v.Index = idx

	results, err := v.SearchFacts("General concise briefings actionable detail", 2)
	if err != nil {
		t.Fatalf("SearchFacts: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("SearchFacts returned %d results, want 2", len(results))
	}
	if !strings.Contains(results[0], "source session continuity") {
		t.Fatalf("top result should expose provenance-rich fact, got %q", results[0])
	}
	if !strings.Contains(results[0], "hot") {
		t.Fatalf("top result should expose hot temperature, got %q", results[0])
	}
}

func TestRankedFactHitsDeprioritizesSupersededFacts(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/route-legacy.md", map[string]interface{}{
		"subject":            "Scout lane",
		"predicate":          "uses",
		"confidence":         0.98,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         "2026-04-10T10:00:00Z",
		"observed_at":        "2026-04-10T10:00:00Z",
		"valid_from":         "2026-04-10T10:00:00Z",
		"valid_to":           "2026-04-14T10:00:00Z",
		"temporal_status":    "superseded",
		"superseded_by":      "memory/facts/route-current.md",
	}, "Scout lane uses qwen-3.6 on the CLI lane.")

	seedFile(t, v, "memory/facts/route-current.md", map[string]interface{}{
		"subject":            "Scout lane",
		"predicate":          "uses",
		"confidence":         0.92,
		"importance":         "high",
		"memory_temperature": "warm",
		"created_at":         "2026-04-15T10:00:00Z",
		"observed_at":        "2026-04-15T10:00:00Z",
		"valid_from":         "2026-04-15T10:00:00Z",
		"temporal_status":    "active",
	}, "Scout lane uses gemini-2.5-flash on the api lane.")

	hits, err := v.rankedFactHits("scout lane uses", 2, FactSearchOptions{})
	if err != nil {
		t.Fatalf("rankedFactHits: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].RelPath != "memory/facts/route-current.md" {
		t.Fatalf("top hit = %q, want memory/facts/route-current.md", hits[0].RelPath)
	}
	if hits[1].RelPath != "memory/facts/route-legacy.md" {
		t.Fatalf("second hit = %q, want memory/facts/route-legacy.md", hits[1].RelPath)
	}
}

func TestSearchFactsReranksTowardElderProtection(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/elder.md", map[string]interface{}{
		"subject":                 "Founding lesson",
		"predicate":               "states",
		"confidence":              0.91,
		"importance":              "high",
		"memory_temperature":      "warm",
		"memory_protection_class": "elder",
		"created_at":              time.Now().Add(-400 * 24 * time.Hour).Format(time.RFC3339),
		"created":                 time.Now().Add(-400 * 24 * time.Hour).Format(time.RFC3339),
		"source":                  "campaign journal",
		"source_ref":              "vault/sessions/2026-04-14-grade-s-memory-program.md",
		"captured_by_office":      "librarian",
	}, "Rare long-horizon memory should not be buried by freshness bias.")
	seedFile(t, v, "memory/facts/recent.md", map[string]interface{}{
		"subject":            "Founding lesson",
		"predicate":          "states",
		"confidence":         0.91,
		"importance":         "high",
		"memory_temperature": "warm",
		"created_at":         time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		"created":            time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}, "Rare long-horizon memory should not be buried by freshness bias.")

	idx, err := index.Build(v.Dir, "")
	if err != nil {
		t.Fatalf("Build index: %v", err)
	}
	defer idx.Close()
	v.Index = idx

	query := "founding lesson rare long-horizon memory freshness bias"
	hits, err := v.rankedFactHits(query, 2, FactSearchOptions{})
	if err != nil {
		t.Fatalf("rankedFactHits: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("rankedFactHits returned %d results, want 2", len(hits))
	}
	if hits[0].RelPath != "memory/facts/elder.md" {
		t.Fatalf("top hit = %s (score %.3f, class %q), want elder fact; second hit score %.3f class %q",
			hits[0].RelPath, hits[0].Score, hits[0].Doc.Get("memory_protection_class"),
			hits[1].Score, hits[1].Doc.Get("memory_protection_class"))
	}
	results, err := v.SearchFacts(query, 2)
	if err != nil {
		t.Fatalf("SearchFacts: %v", err)
	}
	if !strings.Contains(results[0], "elder") {
		t.Fatalf("formatted top result should expose elder protection, got %q", results[0])
	}
}

func TestSearchFactsWithOptionsFiltersByTemperature(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/hot.md", map[string]interface{}{
		"subject":            "Flagship posture",
		"predicate":          "codename",
		"confidence":         0.9,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         time.Now().Format(time.RFC3339),
	}, "brass lantern")
	seedFile(t, v, "memory/facts/warm.md", map[string]interface{}{
		"subject":            "Flagship posture archive",
		"predicate":          "codename",
		"confidence":         0.9,
		"importance":         "high",
		"memory_temperature": "warm",
		"created_at":         time.Now().Format(time.RFC3339),
	}, "quiet harbor")

	results, err := v.SearchFactsWithOptions("flagship posture codename", 5, FactSearchOptions{
		MemoryTemperature: "hot",
	})
	if err != nil {
		t.Fatalf("SearchFactsWithOptions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchFactsWithOptions returned %d results, want 1", len(results))
	}
	if !strings.Contains(results[0], "brass lantern") {
		t.Fatalf("expected hot fact in result, got %q", results[0])
	}
	if strings.Contains(results[0], "quiet harbor") {
		t.Fatalf("warm fact should have been filtered out, got %q", results[0])
	}
}

func TestRecallFactsWritesReceiptAndReinforces(t *testing.T) {
	v := testVault(t)
	factPath, err := v.StoreFactGoverned("General", "flagship", "brass lantern", 0.95, "high", FactWriteAuthority{
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "vault_test",
		StaffingContext:     "operator_test",
		AuthorityScope:      ledger.ScopeOperatorMemoryStore,
		TargetDomain:        "memory/facts",
		SourceEventID:       "evt-flagship",
		LineageID:           "evt-flagship",
		Mission:             "Memory Sovereignty",
		WorkItemID:          "work-123",
		Environment:         "operator-shell",
		RelatedFactPaths:    []string{"memory/facts/founding-law.md"},
		RelatedEpisodePaths: []string{"memory/episodes/evt-commissioning.md"},
		RelatedEntityRefs:   []string{"General", "MODUS"},
		RelatedMissionRefs:  []string{"Memory Sovereignty"},
		CueTerms:            []string{"flagship", "codename"},
		MemoryTemperature:   "hot",
		AllowApproval:       true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned: %v", err)
	}

	recall, err := v.RecallFacts(RecallRequest{
		Query: "flagship brass lantern",
		Limit: 3,
		Options: FactSearchOptions{
			MemoryTemperature: "hot",
			RouteSubject:      "General",
			RouteMission:      "Memory Sovereignty",
			LineageID:         "evt-flagship",
			Environment:       "operator-shell",
			WorkItemID:        "work-123",
			TimeBand:          "recent",
		},
		Harness:            "test_harness",
		Adapter:            "test_adapter",
		Mode:               "manual_search",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "vault_test",
		StaffingContext:    "operator_test",
		WorkItemID:         "work-123",
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	if len(recall.Lines) != 1 {
		t.Fatalf("RecallFacts returned %d lines, want 1", len(recall.Lines))
	}
	if !strings.HasPrefix(recall.ReceiptPath, "memory/recalls/") {
		t.Fatalf("ReceiptPath = %q, want memory/recalls path", recall.ReceiptPath)
	}

	receipt, err := v.Read(recall.ReceiptPath)
	if err != nil {
		t.Fatalf("Read recall receipt: %v", err)
	}
	if receipt.Get("work_item_id") != "work-123" {
		t.Fatalf("work_item_id = %q, want work-123", receipt.Get("work_item_id"))
	}
	if receipt.Get("memory_temperature_filter") != "hot" {
		t.Fatalf("memory_temperature_filter = %q, want hot", receipt.Get("memory_temperature_filter"))
	}
	if raw := receipt.Frontmatter["route_subjects"]; raw == nil {
		t.Fatal("expected route_subjects on receipt")
	}
	if raw := receipt.Frontmatter["route_missions"]; raw == nil {
		t.Fatal("expected route_missions on receipt")
	}
	if receipt.Get("route_work_item_id") != "work-123" {
		t.Fatalf("route_work_item_id = %q, want work-123", receipt.Get("route_work_item_id"))
	}
	if receipt.Get("route_lineage_id") != "evt-flagship" {
		t.Fatalf("route_lineage_id = %q, want evt-flagship", receipt.Get("route_lineage_id"))
	}
	if receipt.Get("route_environment") != "operator-shell" {
		t.Fatalf("route_environment = %q, want operator-shell", receipt.Get("route_environment"))
	}
	if receipt.Get("route_time_band") != "recent" {
		t.Fatalf("route_time_band = %q, want recent", receipt.Get("route_time_band"))
	}
	if raw := receipt.Frontmatter["linked_fact_paths"]; raw == nil {
		t.Fatal("expected linked_fact_paths on receipt")
	}
	if raw := receipt.Frontmatter["linked_episode_paths"]; raw == nil {
		t.Fatal("expected linked_episode_paths on receipt")
	}
	if raw := receipt.Frontmatter["linked_entity_refs"]; raw == nil {
		t.Fatal("expected linked_entity_refs on receipt")
	}
	if raw := receipt.Frontmatter["linked_mission_refs"]; raw == nil {
		t.Fatal("expected linked_mission_refs on receipt")
	}
	if len(recall.LinkedFactPaths) != 1 || recall.LinkedFactPaths[0] != "memory/facts/founding-law.md" {
		t.Fatalf("LinkedFactPaths = %#v", recall.LinkedFactPaths)
	}
	if len(recall.LinkedEpisodePaths) != 1 || recall.LinkedEpisodePaths[0] != "memory/episodes/evt-commissioning.md" {
		t.Fatalf("LinkedEpisodePaths = %#v", recall.LinkedEpisodePaths)
	}
	if len(recall.LinkedEntityRefs) != 2 {
		t.Fatalf("LinkedEntityRefs = %#v", recall.LinkedEntityRefs)
	}
	if len(recall.LinkedMissionRefs) != 1 || recall.LinkedMissionRefs[0] != "Memory Sovereignty" {
		t.Fatalf("LinkedMissionRefs = %#v", recall.LinkedMissionRefs)
	}
	if !strings.Contains(receipt.Body, factPath) {
		t.Fatalf("receipt body missing fact path %q", factPath)
	}
	if !strings.Contains(receipt.Body, "## Structural Links") {
		t.Fatalf("receipt body missing structural links section:\n%s", receipt.Body)
	}

	fact, err := v.Read(factPath)
	if err != nil {
		t.Fatalf("Read fact after recall: %v", err)
	}
	if fact.Get("last_accessed") == "" {
		t.Fatal("expected fact reinforcement after recall")
	}
}

func TestRecallFactsPreservesRankOrderInSelectedPaths(t *testing.T) {
	v := testVault(t)

	preferencePath, err := v.StoreFactGoverned("General", "preference", "wants acknowledgment/confirmation of receipt for iMessage texts", 0.91, "medium", FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "vault_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned preference: %v", err)
	}
	_, err = v.StoreFactGoverned("General correction: stop signing texts with -sent by Claude", "corrected", "General corrected: MODUS should not sign iMessages with Claude attribution.", 0.99, "critical", FactWriteAuthority{
		ProducingOffice:    "memory_governance",
		ProducingSubsystem: "vault_test",
		StaffingContext:    "operator_test",
		AuthorityScope:     ledger.ScopeOperatorMemoryStore,
		TargetDomain:       "memory/facts",
		AllowApproval:      true,
	})
	if err != nil {
		t.Fatalf("StoreFactGoverned correction: %v", err)
	}

	recall, err := v.RecallFacts(RecallRequest{
		Query: "acknowledgment iMessage texts",
		Limit: 3,
		Options: FactSearchOptions{
			RouteSubject: "General",
		},
		Harness:            "test_harness",
		Adapter:            "test_adapter",
		Mode:               "manual_search",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "vault_test",
		StaffingContext:    "operator_test",
	})
	if err != nil {
		t.Fatalf("RecallFacts: %v", err)
	}
	if len(recall.ResultPaths) == 0 {
		t.Fatal("expected ranked result paths")
	}
	if recall.ResultPaths[0] != preferencePath {
		t.Fatalf("top result path = %q, want %q", recall.ResultPaths[0], preferencePath)
	}

	receipt, err := v.Read(recall.ReceiptPath)
	if err != nil {
		t.Fatalf("Read recall receipt: %v", err)
	}
	raw, ok := receipt.Frontmatter["selected_paths"].([]interface{})
	if !ok || len(raw) == 0 {
		t.Fatalf("selected_paths missing or malformed: %#v", receipt.Frontmatter["selected_paths"])
	}
	first, ok := raw[0].(string)
	if !ok {
		t.Fatalf("selected_paths[0] has unexpected type: %#v", raw[0])
	}
	if strings.TrimSpace(first) != preferencePath {
		t.Fatalf("selected_paths[0] = %q, want %q", strings.TrimSpace(first), preferencePath)
	}
}

func TestRankedFactHitsUsesStructuralLinksForRouteScore(t *testing.T) {
	v := testVault(t)
	now := time.Now().Format(time.RFC3339)

	seedFile(t, v, "memory/facts/unlinked.md", map[string]interface{}{
		"subject":            "Operator shell",
		"predicate":          "records",
		"confidence":         0.91,
		"importance":         "high",
		"memory_temperature": "warm",
		"created_at":         now,
		"observed_at":        now,
	}, "memory charter briefing")

	seedFile(t, v, "memory/facts/linked.md", map[string]interface{}{
		"subject":              "Operator shell",
		"predicate":            "records",
		"confidence":           0.91,
		"importance":           "high",
		"memory_temperature":   "warm",
		"created_at":           now,
		"observed_at":          now,
		"related_entity_refs":  []interface{}{"General"},
		"related_mission_refs": []interface{}{"Memory Sovereignty"},
	}, "memory charter briefing")

	hits, err := v.rankedFactHits("memory charter briefing", 2, FactSearchOptions{
		RouteSubject: "General",
		RouteMission: "Memory Sovereignty",
	})
	if err != nil {
		t.Fatalf("rankedFactHits: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("rankedFactHits returned %d hits, want 2", len(hits))
	}
	if hits[0].RelPath != "memory/facts/linked.md" {
		t.Fatalf("top hit = %q, want linked structural fact", hits[0].RelPath)
	}
}

func TestDecayFactsSkipsElderProtected(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/elder.md", map[string]interface{}{
		"subject":                 "Founding lesson",
		"predicate":               "requires",
		"confidence":              0.9,
		"importance":              "medium",
		"memory_protection_class": "elder",
		"created":                 time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
		"created_at":              time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
		"last_accessed":           time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
		"stability":               14.0,
		"difficulty":              0.7,
	}, "Protect rare old memory.")

	updated, err := v.DecayFacts()
	if err != nil {
		t.Fatalf("DecayFacts: %v", err)
	}
	if updated != 0 {
		t.Fatalf("DecayFacts updated %d facts, want 0 for elder-protected memory", updated)
	}
	doc, err := v.Read("memory/facts/elder.md")
	if err != nil {
		t.Fatalf("Read elder fact: %v", err)
	}
	if doc.GetFloat("confidence") != 0.9 {
		t.Fatalf("confidence = %v, want unchanged 0.9", doc.GetFloat("confidence"))
	}
}

func TestArchiveStaleFactsSkipsElderProtected(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/elder.md", map[string]interface{}{
		"subject":                 "Founding lesson",
		"predicate":               "requires",
		"confidence":              0.04,
		"importance":              "medium",
		"memory_protection_class": "elder",
		"created_at":              time.Now().Add(-400 * 24 * time.Hour).Format(time.RFC3339),
	}, "Protect rare old memory.")

	archived, err := v.ArchiveStaleFacts(0.1)
	if err != nil {
		t.Fatalf("ArchiveStaleFacts: %v", err)
	}
	if archived != 0 {
		t.Fatalf("ArchiveStaleFacts archived %d facts, want 0 for elder-protected memory", archived)
	}
	doc, err := v.Read("memory/facts/elder.md")
	if err != nil {
		t.Fatalf("Read elder fact: %v", err)
	}
	if doc.Get("archived") == "true" {
		t.Fatal("elder-protected fact should not be archived silently")
	}
}

func TestSearchFactsWithOptionsPrioritizesRouteSubject(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/general-briefing.md", map[string]interface{}{
		"subject":            "General briefing preference",
		"predicate":          "style",
		"confidence":         0.95,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         time.Now().Format(time.RFC3339),
		"cue_terms":          []string{"briefing", "concise"},
	}, "concise and direct")
	seedFile(t, v, "memory/facts/scout-briefing.md", map[string]interface{}{
		"subject":            "Scout briefing pattern",
		"predicate":          "style",
		"confidence":         0.95,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         time.Now().Format(time.RFC3339),
		"cue_terms":          []string{"briefing", "concise"},
	}, "broad and exploratory")

	results, err := v.SearchFactsWithOptions("briefing concise", 1, FactSearchOptions{
		RouteSubject: "General briefing preference",
	})
	if err != nil {
		t.Fatalf("SearchFactsWithOptions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchFactsWithOptions returned %d results, want 1", len(results))
	}
	if !strings.Contains(results[0], "General briefing preference") {
		t.Fatalf("expected routed General briefing fact, got %q", results[0])
	}
}

func TestSearchFactsWithOptionsPrioritizesMissionLineageAndEnvironment(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/phase4-target.md", map[string]interface{}{
		"subject":            "Operator shell memory repair",
		"predicate":          "state",
		"confidence":         0.95,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         time.Now().Format(time.RFC3339),
		"mission":            "Memory Sovereignty",
		"work_item_id":       "work-memory-route",
		"lineage_id":         "lin-memory-route",
		"environment":        "operator-shell",
		"cue_terms":          []string{"operator", "shell", "memory"},
	}, "hierarchical retrieval repaired")
	seedFile(t, v, "memory/facts/phase4-decoy.md", map[string]interface{}{
		"subject":            "Operator shell memory repair",
		"predicate":          "state",
		"confidence":         0.95,
		"importance":         "high",
		"memory_temperature": "hot",
		"created_at":         time.Now().Format(time.RFC3339),
		"mission":            "Other Mission",
		"work_item_id":       "work-other-route",
		"lineage_id":         "lin-other-route",
		"environment":        "background-daemon",
		"cue_terms":          []string{"operator", "shell", "memory"},
	}, "decorative mismatch")

	results, err := v.SearchFactsWithOptions("operator shell memory", 1, FactSearchOptions{
		RouteSubject: "Operator shell memory repair",
		RouteMission: "Memory Sovereignty",
		WorkItemID:   "work-memory-route",
		LineageID:    "lin-memory-route",
		Environment:  "operator-shell",
	})
	if err != nil {
		t.Fatalf("SearchFactsWithOptions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchFactsWithOptions returned %d results, want 1", len(results))
	}
	if !strings.Contains(results[0], "hierarchical retrieval repaired") {
		t.Fatalf("expected routed target fact, got %q", results[0])
	}
}

func TestSearchFactsWithOptionsRespectsArchiveTimeBand(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/recent-flagship.md", map[string]interface{}{
		"subject":            "Flagship codename",
		"predicate":          "value",
		"confidence":         0.95,
		"importance":         "high",
		"memory_temperature": "warm",
		"created_at":         time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}, "brass lantern")
	seedFile(t, v, "memory/facts/archive-flagship.md", map[string]interface{}{
		"subject":            "Flagship codename",
		"predicate":          "value",
		"confidence":         0.95,
		"importance":         "high",
		"memory_temperature": "warm",
		"created_at":         time.Now().Add(-45 * 24 * time.Hour).Format(time.RFC3339),
	}, "quiet harbor")

	results, err := v.SearchFactsWithOptions("flagship codename", 1, FactSearchOptions{
		RouteSubject: "Flagship codename",
		TimeBand:     "archive",
	})
	if err != nil {
		t.Fatalf("SearchFactsWithOptions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchFactsWithOptions returned %d results, want 1", len(results))
	}
	if !strings.Contains(results[0], "quiet harbor") {
		t.Fatalf("expected archive-routed fact, got %q", results[0])
	}
}

// --- FSRS Decay ---

func TestFsrsRetrievability(t *testing.T) {
	// At t=S, R should be ~0.9
	r := fsrsRetrievability(60, 60)
	if math.Abs(r-0.9) > 0.001 {
		t.Errorf("R(S,S) = %f, want ~0.9", r)
	}

	// At t=0, R should be 1.0
	r = fsrsRetrievability(0, 60)
	if r != 1.0 {
		t.Errorf("R(0,S) = %f, want 1.0", r)
	}

	// Zero stability
	r = fsrsRetrievability(10, 0)
	if r != 0 {
		t.Errorf("R(t,0) = %f, want 0", r)
	}
}

func TestFsrsNewStability(t *testing.T) {
	newS := fsrsNewStability(60, 0.5, 0.9)
	if newS <= 60 {
		t.Errorf("stability should increase on recall, got %f", newS)
	}
}

func TestDecayFacts(t *testing.T) {
	v := testVault(t)

	// Seed a fact with stability already set and old enough to decay significantly.
	// With stability=60 days and 90 days elapsed, FSRS R = (1 + 90/(9*60))^-1 ≈ 0.857
	past := time.Now().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	seedFile(t, v, "memory/facts/old-fact.md", map[string]interface{}{
		"subject":    "test",
		"confidence": 0.9,
		"importance": "medium",
		"stability":  60.0,
		"difficulty": 0.5,
		"created":    past,
	}, "An old fact")

	// Seed a critical fact (should not decay)
	seedFile(t, v, "memory/facts/critical-fact.md", map[string]interface{}{
		"subject":    "critical",
		"confidence": 1.0,
		"importance": "critical",
		"created":    past,
	}, "Critical fact")

	updated, err := v.DecayFacts()
	if err != nil {
		t.Fatalf("DecayFacts: %v", err)
	}
	if updated != 1 {
		t.Errorf("DecayFacts updated %d facts, want 1", updated)
	}

	// Verify the old fact's confidence decreased
	doc, _ := v.Read("memory/facts/old-fact.md")
	newConf := doc.GetFloat("confidence")
	if newConf >= 0.9 {
		t.Errorf("confidence should have decayed from 0.9, got %f", newConf)
	}

	data, err := os.ReadFile(filepath.Join(v.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "memory_decay" {
		t.Fatalf("action_class = %v, want memory_decay", rec["action_class"])
	}
}

func TestReinforceFact(t *testing.T) {
	v := testVault(t)

	past := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	seedFile(t, v, "memory/facts/reinforce-me.md", map[string]interface{}{
		"subject":    "test",
		"confidence": 0.7,
		"importance": "medium",
		"stability":  60.0,
		"difficulty": 0.5,
		"created":    past,
	}, "Test fact")

	err := v.ReinforceFact("memory/facts/reinforce-me.md")
	if err != nil {
		t.Fatalf("ReinforceFact: %v", err)
	}

	doc, _ := v.Read("memory/facts/reinforce-me.md")
	newConf := doc.GetFloat("confidence")
	if newConf <= 0.7 {
		t.Errorf("confidence should increase after reinforcement, got %f", newConf)
	}
	newStab := doc.GetFloat("stability")
	if newStab <= 60 {
		t.Errorf("stability should increase after reinforcement, got %f", newStab)
	}
}

func TestArchiveStaleFacts(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/stale.md", map[string]interface{}{
		"subject":    "stale",
		"confidence": 0.05,
		"importance": "low",
	}, "Stale fact")

	seedFile(t, v, "memory/facts/fresh.md", map[string]interface{}{
		"subject":    "fresh",
		"confidence": 0.9,
		"importance": "high",
	}, "Fresh fact")

	seedFile(t, v, "memory/facts/critical-low.md", map[string]interface{}{
		"subject":    "critical",
		"confidence": 0.05,
		"importance": "critical",
	}, "Critical low — should not archive")

	archived, err := v.ArchiveStaleFacts(0.1)
	if err != nil {
		t.Fatalf("ArchiveStaleFacts: %v", err)
	}
	if archived != 1 {
		t.Errorf("archived %d, want 1", archived)
	}

	doc, _ := v.Read("memory/facts/stale.md")
	if doc.Get("archived") != "true" {
		t.Error("stale fact should be archived")
	}

	data, err := os.ReadFile(filepath.Join(v.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "memory_archival" {
		t.Fatalf("action_class = %v, want memory_archival", rec["action_class"])
	}
}

func TestTouchFact(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "memory/facts/touch-me.md", map[string]interface{}{
		"subject": "test",
	}, "Touch test")

	before := time.Now().Add(-time.Second)
	err := v.TouchFact("memory/facts/touch-me.md")
	if err != nil {
		t.Fatalf("TouchFact: %v", err)
	}

	doc, _ := v.Read("memory/facts/touch-me.md")
	accessed := doc.Get("last_accessed")
	if accessed == "" {
		t.Fatal("last_accessed not set")
	}
	ts, err := time.Parse(time.RFC3339, accessed)
	if err != nil {
		t.Fatalf("parse last_accessed: %v", err)
	}
	if ts.Before(before) {
		t.Error("last_accessed should be recent")
	}
}

// --- Beliefs ---

func TestListBeliefs(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/beliefs/b1.md", map[string]interface{}{
		"subject":   "Go",
		"predicate": "is_a",
	}, "Go is a language")
	seedFile(t, v, "atlas/beliefs/b2.md", map[string]interface{}{
		"subject":   "Python",
		"predicate": "uses",
	}, "Python uses GIL")

	all, err := v.ListBeliefs("", 10)
	if err != nil {
		t.Fatalf("ListBeliefs: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("ListBeliefs('') = %d, want 2", len(all))
	}

	goBeliefs, _ := v.ListBeliefs("Go", 10)
	if len(goBeliefs) != 1 {
		t.Errorf("ListBeliefs('Go') = %d, want 1", len(goBeliefs))
	}
}

func TestDecayAllBeliefs(t *testing.T) {
	v := testVault(t)

	past := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	seedFile(t, v, "atlas/beliefs/decay-me.md", map[string]interface{}{
		"subject":    "test",
		"predicate":  "uses",
		"confidence": 0.8,
		"created":    past,
	}, "Test belief")

	updated, err := v.DecayAllBeliefs()
	if err != nil {
		t.Fatalf("DecayAllBeliefs: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated %d beliefs, want 1", updated)
	}

	doc, _ := v.Read("atlas/beliefs/decay-me.md")
	newConf := doc.GetFloat("confidence")
	if newConf >= 0.8 {
		t.Errorf("belief should have decayed from 0.8, got %f", newConf)
	}

	data, err := os.ReadFile(filepath.Join(v.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "belief_decay" {
		t.Fatalf("action_class = %v, want belief_decay", rec["action_class"])
	}
}

func TestReinforceBelief(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/beliefs/reinforce.md", map[string]interface{}{
		"confidence": 0.7,
		"source":     "observation",
	}, "Test belief")

	// Independent source → +0.05
	err := v.ReinforceBelief("atlas/beliefs/reinforce.md", "experiment")
	if err != nil {
		t.Fatalf("ReinforceBelief: %v", err)
	}
	doc, _ := v.Read("atlas/beliefs/reinforce.md")
	if doc.GetFloat("confidence") != 0.75 {
		t.Errorf("confidence after independent reinforce = %v, want 0.75", doc.GetFloat("confidence"))
	}

	// Same source → +0.02
	err = v.ReinforceBelief("atlas/beliefs/reinforce.md", "observation")
	if err != nil {
		t.Fatalf("ReinforceBelief same source: %v", err)
	}
	doc, _ = v.Read("atlas/beliefs/reinforce.md")
	if doc.GetFloat("confidence") != 0.77 {
		t.Errorf("confidence after same-source reinforce = %v, want 0.77", doc.GetFloat("confidence"))
	}
}

func TestWeakenBelief(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/beliefs/weaken.md", map[string]interface{}{
		"confidence": 0.5,
	}, "Test")

	err := v.WeakenBelief("atlas/beliefs/weaken.md")
	if err != nil {
		t.Fatalf("WeakenBelief: %v", err)
	}
	doc, _ := v.Read("atlas/beliefs/weaken.md")
	if doc.GetFloat("confidence") != 0.4 {
		t.Errorf("confidence after weaken = %v, want 0.4", doc.GetFloat("confidence"))
	}
}

func TestWeakenBeliefFloor(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/beliefs/floor.md", map[string]interface{}{
		"confidence": 0.08,
	}, "Near floor")

	v.WeakenBelief("atlas/beliefs/floor.md")
	doc, _ := v.Read("atlas/beliefs/floor.md")
	if doc.GetFloat("confidence") != confidenceFloor {
		t.Errorf("confidence = %v, want floor %v", doc.GetFloat("confidence"), confidenceFloor)
	}
}

// --- Entities ---

func TestListEntities(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/entities/go.md", map[string]interface{}{"name": "Go"}, "Go entity")
	seedFile(t, v, "atlas/entities/python.md", map[string]interface{}{"name": "Python"}, "Python entity")

	entities, err := v.ListEntities()
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("ListEntities = %d, want 2", len(entities))
	}
}

func TestGetEntity(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/entities/go-lang.md", map[string]interface{}{"name": "Go Lang"}, "Go")

	// By name
	doc, err := v.GetEntity("Go Lang")
	if err != nil {
		t.Fatalf("GetEntity by name: %v", err)
	}
	if doc.Get("name") != "Go Lang" {
		t.Errorf("name = %q", doc.Get("name"))
	}

	// By slug
	doc, err = v.GetEntity("go-lang")
	if err != nil {
		t.Fatalf("GetEntity by slug: %v", err)
	}
	if doc.Get("name") != "Go Lang" {
		t.Errorf("name = %q", doc.Get("name"))
	}

	// Not found
	_, err = v.GetEntity("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent entity")
	}
}

// --- Missions ---

func TestCreateMission(t *testing.T) {
	v := testVault(t)

	path, err := v.CreateMission("Build the thing", "Make it work", "high")
	if err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	if !strings.Contains(path, "missions/active/") {
		t.Errorf("path = %q, want missions/active/", path)
	}

	// Verify contents
	doc, err := markdown.Parse(path)
	if err != nil {
		t.Fatalf("Parse mission: %v", err)
	}
	if doc.Get("title") != "Build the thing" {
		t.Errorf("title = %q", doc.Get("title"))
	}
	if doc.Get("priority") != "high" {
		t.Errorf("priority = %q", doc.Get("priority"))
	}
	if doc.Get("status") != "active" {
		t.Errorf("status = %q", doc.Get("status"))
	}
}

func TestCreateMissionDefaults(t *testing.T) {
	v := testVault(t)

	path, _ := v.CreateMission("Default mission", "Test", "")
	doc, _ := markdown.Parse(path)
	if doc.Get("priority") != "medium" {
		t.Errorf("default priority = %q, want medium", doc.Get("priority"))
	}
}

func TestListMissions(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "missions/active/m1.md", map[string]interface{}{
		"title": "M1", "status": "active",
	}, "M1")
	seedFile(t, v, "missions/active/m2.md", map[string]interface{}{
		"title": "M2", "status": "blocked",
	}, "M2")
	seedFile(t, v, "missions/completed/m3.md", map[string]interface{}{
		"title": "M3", "status": "completed",
	}, "M3")

	all, _ := v.ListMissions("", 10)
	if len(all) != 3 {
		t.Errorf("ListMissions('') = %d, want 3", len(all))
	}

	active, _ := v.ListMissions("active", 10)
	if len(active) != 1 {
		t.Errorf("ListMissions('active') = %d, want 1", len(active))
	}
}

func TestGetMission(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "missions/active/build-thing.md", map[string]interface{}{
		"title": "Build Thing", "status": "active",
	}, "Build it")

	doc, err := v.GetMission("build-thing")
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if doc.Get("title") != "Build Thing" {
		t.Errorf("title = %q", doc.Get("title"))
	}

	_, err = v.GetMission("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent mission")
	}
}

func TestMissionBoard(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "missions/active/a.md", map[string]interface{}{"status": "active"}, "A")
	seedFile(t, v, "missions/active/b.md", map[string]interface{}{"status": "blocked"}, "B")
	seedFile(t, v, "missions/completed/c.md", map[string]interface{}{"status": "completed"}, "C")

	board := v.MissionBoard()
	if len(board["active"]) != 1 {
		t.Errorf("active = %d, want 1", len(board["active"]))
	}
	if len(board["blocked"]) != 1 {
		t.Errorf("blocked = %d, want 1", len(board["blocked"]))
	}
	if len(board["completed"]) != 1 {
		t.Errorf("completed = %d, want 1", len(board["completed"]))
	}
}

// --- Trust ---

func TestGetTrustStageDefault(t *testing.T) {
	v := testVault(t)

	stage, config, err := v.GetTrustStage()
	if err != nil {
		t.Fatalf("GetTrustStage: %v", err)
	}
	if stage != 1 {
		t.Errorf("default stage = %d, want 1", stage)
	}
	if config["stage"] != 1 {
		t.Errorf("config stage = %v, want 1", config["stage"])
	}
}

func TestSetTrustStage(t *testing.T) {
	v := testVault(t)

	err := v.SetTrustStage(2, "General", "testing")
	if err != nil {
		t.Fatalf("SetTrustStage: %v", err)
	}

	stage, _, err := v.GetTrustStage()
	if err != nil {
		t.Fatalf("GetTrustStage: %v", err)
	}
	if stage != 2 {
		t.Errorf("stage = %d, want 2", stage)
	}
}

func TestSetTrustStageInvalid(t *testing.T) {
	v := testVault(t)

	if err := v.SetTrustStage(0, "test", ""); err == nil {
		t.Error("expected error for stage 0")
	}
	if err := v.SetTrustStage(4, "test", ""); err == nil {
		t.Error("expected error for stage 4")
	}
}

func TestTrustStageLabel(t *testing.T) {
	tests := []struct {
		stage int
		want  string
	}{
		{1, "Inform"},
		{2, "Recommend"},
		{3, "Act"},
		{99, "Unknown"},
	}
	for _, tt := range tests {
		label := TrustStageLabel(tt.stage)
		if !strings.Contains(label, tt.want) {
			t.Errorf("TrustStageLabel(%d) = %q, want containing %q", tt.stage, label, tt.want)
		}
	}
}

// --- PRs ---

func TestOpenPR(t *testing.T) {
	v := testVault(t)

	relPath, err := v.OpenPR("Test Proposal", "modus", "belief", "b1",
		"Because reasons", 0.8, []string{"belief-1"})
	if err != nil {
		t.Fatalf("OpenPR: %v", err)
	}
	if !strings.HasPrefix(relPath, "atlas/prs/") {
		t.Errorf("relPath = %q", relPath)
	}

	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read PR: %v", err)
	}
	if doc.Get("status") != "open" {
		t.Errorf("status = %q, want open", doc.Get("status"))
	}
	if doc.Get("opened_by") != "modus" {
		t.Errorf("opened_by = %q, want modus", doc.Get("opened_by"))
	}
}

func TestMergePR(t *testing.T) {
	v := testVault(t)

	// Need a belief to reinforce
	seedFile(t, v, "atlas/beliefs/linked.md", map[string]interface{}{
		"confidence": 0.7,
	}, "Linked belief")

	relPath, _ := v.OpenPR("Merge Test", "modus", "belief", "b1",
		"test", 0.8, []string{"atlas/beliefs/linked.md"})

	err := v.MergePR(relPath, "General")
	if err != nil {
		t.Fatalf("MergePR: %v", err)
	}

	doc, _ := v.Read(relPath)
	if doc.Get("status") != "merged" {
		t.Errorf("status = %q, want merged", doc.Get("status"))
	}
	if doc.Get("closed_by") != "General" {
		t.Errorf("closed_by = %q", doc.Get("closed_by"))
	}

	data, err := os.ReadFile(filepath.Join(v.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "promotion_merge" {
		t.Fatalf("action_class = %v, want promotion_merge", rec["action_class"])
	}
}

func TestRejectPR(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/beliefs/reject-linked.md", map[string]interface{}{
		"confidence": 0.7,
	}, "Linked belief")

	relPath, _ := v.OpenPR("Reject Test", "modus", "belief", "b1",
		"test", 0.8, []string{"atlas/beliefs/reject-linked.md"})

	err := v.RejectPR(relPath, "General", "not valid")
	if err != nil {
		t.Fatalf("RejectPR: %v", err)
	}

	doc, _ := v.Read(relPath)
	if doc.Get("status") != "rejected" {
		t.Errorf("status = %q, want rejected", doc.Get("status"))
	}

	data, err := os.ReadFile(filepath.Join(v.Dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "promotion_rejection" {
		t.Fatalf("action_class = %v, want promotion_rejection", rec["action_class"])
	}
}

func TestMergeAlreadyClosed(t *testing.T) {
	v := testVault(t)

	relPath, _ := v.OpenPR("Already Closed", "modus", "belief", "b1",
		"test", 0.8, nil)
	v.MergePR(relPath, "General")

	err := v.MergePR(relPath, "General")
	if err == nil {
		t.Error("expected error merging already-merged PR")
	}
}

func TestListPRs(t *testing.T) {
	v := testVault(t)

	v.OpenPR("PR1", "modus", "belief", "b1", "test", 0.8, nil)
	v.OpenPR("PR2", "modus", "belief", "b2", "test", 0.7, nil)

	prs, err := v.ListPRs("")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 2 {
		t.Errorf("ListPRs('') = %d, want 2", len(prs))
	}

	openPRs, _ := v.ListPRs("open")
	if len(openPRs) != 2 {
		t.Errorf("ListPRs('open') = %d, want 2", len(openPRs))
	}
}

// --- Dependencies ---

func TestAddDependency(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "missions/active/frontend.md", map[string]interface{}{
		"title": "Frontend", "status": "active",
	}, "Frontend")
	seedFile(t, v, "missions/active/backend.md", map[string]interface{}{
		"title": "Backend", "status": "active",
	}, "Backend")

	err := v.AddDependency("frontend", "backend", "blocks")
	if err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	deps, err := v.GetDependencies("frontend")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("deps = %d, want 1", len(deps))
	}
	if deps[0]["slug"] != "backend" {
		t.Errorf("dep slug = %v", deps[0]["slug"])
	}
}

func TestAddDependencyInvalidType(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "missions/active/a.md", map[string]interface{}{
		"title": "A", "status": "active",
	}, "A")
	seedFile(t, v, "missions/active/b.md", map[string]interface{}{
		"title": "B", "status": "active",
	}, "B")

	err := v.AddDependency("a", "b", "invalid")
	if err == nil {
		t.Error("expected error for invalid dep type")
	}
}

func TestCanStart(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "missions/active/child.md", map[string]interface{}{
		"title": "Child", "status": "active",
	}, "Child")
	seedFile(t, v, "missions/active/parent.md", map[string]interface{}{
		"title": "Parent", "status": "active",
	}, "Parent")

	v.AddDependency("child", "parent", "blocks")

	canStart, blockers, err := v.CanStart("child")
	if err != nil {
		t.Fatalf("CanStart: %v", err)
	}
	if canStart {
		t.Error("should not be able to start with unsatisfied blocker")
	}
	if len(blockers) != 1 || blockers[0] != "parent" {
		t.Errorf("blockers = %v", blockers)
	}
}

// --- Helpers ---

func TestSlugify(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Hello World", "hello-world"},
		{"Go_Lang 1.24", "go-lang-124"},
		{"Special!@#$Chars", "specialchars"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	tests := []string{
		"2026-04-04T10:00:00Z",
		"2026-04-04T10:00:00",
		"2026-04-04 10:00:00",
		"2026-04-04",
	}
	for _, ts := range tests {
		_, err := parseTime(ts)
		if err != nil {
			t.Errorf("parseTime(%q) failed: %v", ts, err)
		}
	}

	_, err := parseTime("not a date")
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestParseStringList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"[a, b, c]", 3},
		{"a, b", 2},
		{"single", 1},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseStringList(tt.input)
		if len(got) != tt.want {
			t.Errorf("parseStringList(%q) = %d items, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestResolveWikiLink(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "atlas/entities/go.md", map[string]interface{}{"name": "Go"}, "Go")
	seedFile(t, v, "atlas/beliefs/belief-test.md", map[string]interface{}{}, "Belief")
	seedFile(t, v, "knowledge/compiled/demo/topic.md", map[string]interface{}{"title": "Harness As Product"}, "Body")

	got := v.ResolveWikiLink("entity-go")
	if got != filepath.Join("atlas", "entities", "go.md") {
		t.Errorf("ResolveWikiLink('entity-go') = %q", got)
	}

	got = v.ResolveWikiLink("belief-test")
	if !strings.Contains(got, "belief-test.md") {
		t.Errorf("ResolveWikiLink('belief-test') = %q", got)
	}

	got = v.ResolveWikiLink("Harness As Product")
	if got != "knowledge/compiled/demo/topic.md" {
		t.Errorf("ResolveWikiLink('Harness As Product') = %q", got)
	}

	got = v.ResolveWikiLink("knowledge/compiled/demo/topic|Harness As Product")
	if got != "knowledge/compiled/demo/topic.md" {
		t.Errorf("ResolveWikiLink(path link) = %q", got)
	}

	got = v.ResolveWikiLink("knowledge/compiled/demo/topic#Section|Harness As Product")
	if got != "knowledge/compiled/demo/topic.md" {
		t.Errorf("ResolveWikiLink(path link with anchor) = %q", got)
	}

	got = v.ResolveWikiLink("vault/knowledge/compiled/demo/topic|Harness As Product")
	if got != "knowledge/compiled/demo/topic.md" {
		t.Errorf("ResolveWikiLink(vault-prefixed path link) = %q", got)
	}

	absPath := filepath.Join(v.Dir, "knowledge", "compiled", "demo", "topic.md")
	got = v.ResolveWikiLink(absPath + "#Section|Harness As Product")
	if got != "knowledge/compiled/demo/topic.md" {
		t.Errorf("ResolveWikiLink(absolute path link) = %q", got)
	}
}

func TestAuditWikiLinksFixesPathTargets(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "knowledge/compiled/demo/topic.md", map[string]interface{}{"title": "Harness As Product"}, "Body")
	seedFile(t, v, "brain/notes/source.md", map[string]interface{}{"title": "Source"}, strings.Join([]string{
		"See [[vault/knowledge/compiled/demo/topic.md|Harness As Product]].",
		"And [[/tmp/outside-vault.md|Outside]].",
		"And [[knowledge/compiled/demo/topic.md#Section|Topic Section]].",
	}, "\n"))

	audit, err := v.AuditWikiLinks(true)
	if err != nil {
		t.Fatalf("AuditWikiLinks(true): %v", err)
	}
	if audit.UpdatedDocs != 1 {
		t.Fatalf("UpdatedDocs = %d, want 1", audit.UpdatedDocs)
	}
	if len(audit.Rewrites) != 2 {
		t.Fatalf("Rewrites = %d, want 2", len(audit.Rewrites))
	}
	if len(audit.Issues) != 1 {
		t.Fatalf("Issues = %d, want 1", len(audit.Issues))
	}

	doc, err := v.Read("brain/notes/source.md")
	if err != nil {
		t.Fatalf("Read rewritten doc: %v", err)
	}
	if !strings.Contains(doc.Body, "[[knowledge/compiled/demo/topic|Harness As Product]]") {
		t.Fatalf("body missing canonicalized vault-relative link:\n%s", doc.Body)
	}
	if !strings.Contains(doc.Body, "[[knowledge/compiled/demo/topic#Section|Topic Section]]") {
		t.Fatalf("body missing canonicalized anchored link:\n%s", doc.Body)
	}
	if !strings.Contains(doc.Body, "[[/tmp/outside-vault.md|Outside]]") {
		t.Fatalf("outside-vault unresolved link should remain unchanged:\n%s", doc.Body)
	}
}

func TestAuditWikiLinksLeavesTitleLinksAlone(t *testing.T) {
	v := testVault(t)

	seedFile(t, v, "knowledge/compiled/demo/topic.md", map[string]interface{}{"title": "Harness As Product"}, "Body")
	seedFile(t, v, "brain/notes/source.md", map[string]interface{}{"title": "Source"}, "See [[Harness As Product]] and [[knowledge/compiled/demo/topic|Harness]].")

	audit, err := v.AuditWikiLinks(true)
	if err != nil {
		t.Fatalf("AuditWikiLinks(true): %v", err)
	}
	if len(audit.Rewrites) != 0 {
		t.Fatalf("Rewrites = %d, want 0", len(audit.Rewrites))
	}

	doc, err := v.Read("brain/notes/source.md")
	if err != nil {
		t.Fatalf("Read rewritten doc: %v", err)
	}
	if !strings.Contains(doc.Body, "[[Harness As Product]]") {
		t.Fatalf("title-based link should remain unchanged:\n%s", doc.Body)
	}
}

package maintain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/vault"
)

func setupVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"memory/facts", "memory/maintenance", "memory/episodes", "memory/recalls", "brain", "atlas/entities"} {
		os.MkdirAll(filepath.Join(dir, sub), 0755)
	}
	return vault.New(dir, nil), dir
}

func writeFact(t *testing.T, dir, name string, fm map[string]interface{}, body string) {
	t.Helper()
	path := filepath.Join(dir, "memory", "facts", name+".md")
	if err := markdown.Write(path, fm, body); err != nil {
		t.Fatalf("writeFact %s: %v", name, err)
	}
}

// --- Consolidate tests ---

func TestConsolidateDuplicates(t *testing.T) {
	v, dir := setupVault(t)

	// Two facts with same subject, similar body
	writeFact(t, dir, "go-lang-1", map[string]interface{}{
		"subject": "Go", "predicate": "is", "confidence": 0.9,
	}, "Go is a statically typed compiled programming language designed at Google")

	writeFact(t, dir, "go-lang-2", map[string]interface{}{
		"subject": "Go", "predicate": "is", "confidence": 0.7,
	}, "Go is a statically typed compiled language designed at Google by Rob Pike")

	n, actions, err := Consolidate(v)
	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 merge candidate, got %d", n)
	}
	if len(actions) < 1 {
		t.Error("expected at least 1 action")
	}

	// Check artifact was written
	maintenanceDir := filepath.Join(dir, "memory", "maintenance")
	files, _ := filepath.Glob(filepath.Join(maintenanceDir, "*merge*.md"))
	if len(files) != 1 {
		t.Errorf("expected 1 merge artifact, got %d", len(files))
	}

	// Verify artifact content
	if len(files) > 0 {
		doc, err := markdown.Parse(files[0])
		if err != nil {
			t.Fatalf("failed to parse merge artifact: %v", err)
		}
		if doc.Get("type") != "candidate_merge" {
			t.Errorf("type = %q, want candidate_merge", doc.Get("type"))
		}
		if doc.Get("status") != "pending" {
			t.Errorf("status = %q, want pending", doc.Get("status"))
		}
		raw, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatalf("read merge artifact: %v", err)
		}
		if !strings.Contains(string(raw), "producing_signature:") {
			t.Fatal("merge artifact missing producing_signature")
		}
	}

	ledgerData, err := os.ReadFile(filepath.Join(dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(ledgerData)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "review_candidate_generation" {
		t.Fatalf("action_class = %v, want review_candidate_generation", rec["action_class"])
	}
}

func TestConsolidateNoDuplicates(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "python", map[string]interface{}{
		"subject": "Python", "predicate": "is", "confidence": 0.9,
	}, "Python is a dynamic interpreted language")

	writeFact(t, dir, "rust", map[string]interface{}{
		"subject": "Rust", "predicate": "is", "confidence": 0.8,
	}, "Rust is a systems programming language focused on safety")

	n, _, err := Consolidate(v)
	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 merge candidates, got %d", n)
	}
}

func TestWriteBootstrapCandidateSignsPendingArtifact(t *testing.T) {
	v, dir := setupVault(t)

	err := WriteBootstrapCandidate(v, BootstrapCandidate{
		Subject:            "MODUS",
		Predicate:          "current_mood",
		Value:              "reflective",
		SourcePath:         "data/consciousness/identity.json",
		Confidence:         0.9,
		Importance:         "high",
		Method:             "consciousness-sync",
		ProducingOffice:    "main_brain",
		ProducingSubsystem: "consciousness",
		StaffingContext:    "cycle_10",
		ProofRef:           "consciousness:10:mood",
	})
	if err != nil {
		t.Fatalf("WriteBootstrapCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*bootstrap*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 bootstrap artifact, got %d", len(files))
	}

	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse bootstrap artifact: %v", err)
	}
	if doc.Get("type") != "candidate_bootstrap_fact" {
		t.Fatalf("type = %q, want candidate_bootstrap_fact", doc.Get("type"))
	}
	if doc.Get("status") != "pending" {
		t.Fatalf("status = %q, want pending", doc.Get("status"))
	}

	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read bootstrap artifact: %v", err)
	}
	if !strings.Contains(string(raw), "producing_signature:") {
		t.Fatal("bootstrap artifact missing producing_signature")
	}
}

func TestWriteHotMemoryTransitionCandidateSignsPendingArtifact(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "operator-priority", map[string]interface{}{
		"subject":            "Operator priority",
		"predicate":          "is",
		"memory_temperature": "warm",
		"importance":         "high",
	}, "Keep the General's thread clear and truthful.")

	if err := WriteHotMemoryTransitionCandidate(v, HotMemoryTransitionCandidate{
		FactPath:            "memory/facts/operator-priority.md",
		ProposedTemperature: "hot",
		Reason:              "This fact should ride in automatic active context.",
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "test_hot_review",
		AuthorityScope:      ledger.ScopeCandidateHotMemoryReview,
	}); err != nil {
		t.Fatalf("WriteHotMemoryTransitionCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*hot-memory*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 hot-memory artifact, got %d", len(files))
	}

	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse hot-memory artifact: %v", err)
	}
	if doc.Get("type") != "candidate_hot_memory_transition" {
		t.Fatalf("type = %q, want candidate_hot_memory_transition", doc.Get("type"))
	}
	if doc.Get("status") != "pending" {
		t.Fatalf("status = %q, want pending", doc.Get("status"))
	}
	if doc.Get("proposed_temperature") != "hot" {
		t.Fatalf("proposed_temperature = %q, want hot", doc.Get("proposed_temperature"))
	}
}

func TestReviewHotTierCreatesOverflowCandidate(t *testing.T) {
	v, dir := setupVault(t)

	now := time.Now().Format(time.RFC3339)
	for i := 0; i < vault.HotMemoryAdmissionCap+1; i++ {
		writeFact(t, dir, fmt.Sprintf("hot-%02d", i), map[string]interface{}{
			"subject":            fmt.Sprintf("Hot %02d", i),
			"predicate":          "matters",
			"memory_temperature": "hot",
			"importance":         "medium",
			"confidence":         0.8,
			"created_at":         now,
		}, "overflow review candidate")
	}

	n, actions, err := ReviewHotTier(v)
	if err != nil {
		t.Fatalf("ReviewHotTier failed: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least one hot-tier review candidate")
	}
	if len(actions) == 0 {
		t.Fatal("expected hot review actions")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*hot-memory*.md"))
	if len(files) == 0 {
		t.Fatal("expected hot-memory review artifact")
	}
	foundOverflow := false
	for _, file := range files {
		doc, err := markdown.Parse(file)
		if err != nil {
			t.Fatalf("parse hot review artifact: %v", err)
		}
		if doc.Get("review_class") == "overflow" {
			foundOverflow = true
		}
	}
	if !foundOverflow {
		t.Fatal("expected overflow hot review candidate")
	}
}

func TestApplyApprovedHotMemoryTransitionUpdatesFact(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "operator-priority", map[string]interface{}{
		"subject":            "Operator priority",
		"predicate":          "is",
		"memory_temperature": "warm",
		"importance":         "high",
		"confidence":         0.9,
		"created_at":         time.Now().Format(time.RFC3339),
	}, "Keep the General's thread clear and truthful.")

	if err := WriteHotMemoryTransitionCandidate(v, HotMemoryTransitionCandidate{
		FactPath:            "memory/facts/operator-priority.md",
		ProposedTemperature: "hot",
		Reason:              "Promote this fact into the automatic tier.",
		ProducingOffice:     "memory_governance",
		ProducingSubsystem:  "test_hot_apply",
		AuthorityScope:      ledger.ScopeCandidateHotMemoryReview,
	}); err != nil {
		t.Fatalf("WriteHotMemoryTransitionCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*hot-memory*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 hot-memory artifact, got %d", len(files))
	}
	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	doc.Set("status", "approved")
	if err := doc.Save(); err != nil {
		t.Fatalf("approve artifact: %v", err)
	}

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved failed: %v", err)
	}
	if result.HotTransitionsApplied != 1 {
		t.Fatalf("HotTransitionsApplied = %d, want 1", result.HotTransitionsApplied)
	}

	fact, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "operator-priority.md"))
	if err != nil {
		t.Fatalf("read fact: %v", err)
	}
	if fact.Get("memory_temperature") != "hot" {
		t.Fatalf("memory_temperature = %q, want hot", fact.Get("memory_temperature"))
	}
	if fact.Get("memory_temperature_previous") != "warm" {
		t.Fatalf("memory_temperature_previous = %q, want warm", fact.Get("memory_temperature_previous"))
	}
	if fact.Get("memory_temperature_review_artifact") == "" {
		t.Fatal("expected review artifact reference on fact")
	}
}

func TestReviewStructuralLinksCreatesBackfillCandidate(t *testing.T) {
	v, dir := setupVault(t)

	if err := markdown.Write(filepath.Join(dir, "atlas", "entities", "modus.md"), map[string]interface{}{
		"name": "MODUS",
		"kind": "system",
	}, "# MODUS\n"); err != nil {
		t.Fatalf("write entity: %v", err)
	}

	writeFact(t, dir, "modus-core", map[string]interface{}{
		"subject":         "MODUS",
		"predicate":       "operates",
		"mission":         "Memory Sovereignty",
		"source_event_id": "evt-structural",
		"lineage_id":      "lin-structural",
		"work_item_id":    "work-structural",
		"created_at":      time.Now().Format(time.RFC3339),
	}, "MODUS operates as a governed memory system.")

	writeFact(t, dir, "modus-purpose", map[string]interface{}{
		"subject":         "MODUS",
		"predicate":       "requires",
		"mission":         "Memory Sovereignty",
		"source_event_id": "evt-structural",
		"lineage_id":      "lin-structural",
		"work_item_id":    "work-structural",
		"created_at":      time.Now().Format(time.RFC3339),
	}, "MODUS requires governed structural memory.")

	if err := markdown.Write(filepath.Join(dir, "memory", "episodes", "evt-structural.md"), map[string]interface{}{
		"type":         "memory_episode",
		"event_id":     "evt-structural",
		"lineage_id":   "lin-structural",
		"subject":      "MODUS",
		"mission":      "Memory Sovereignty",
		"work_item_id": "work-structural",
		"created_at":   time.Now().Format(time.RFC3339),
	}, "Structural linking was discussed in this episode."); err != nil {
		t.Fatalf("write episode: %v", err)
	}

	n, actions, err := ReviewStructuralLinks(v)
	if err != nil {
		t.Fatalf("ReviewStructuralLinks failed: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least one structural-link review candidate")
	}
	if len(actions) == 0 {
		t.Fatal("expected structural review actions")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*structural-links*.md"))
	if len(files) == 0 {
		t.Fatal("expected structural-link review artifact")
	}

	var found *markdown.Document
	for _, file := range files {
		doc, err := markdown.Parse(file)
		if err != nil {
			t.Fatalf("parse structural artifact: %v", err)
		}
		if doc.Get("fact_path") == "memory/facts/modus-core.md" {
			found = doc
			break
		}
	}
	if found == nil {
		t.Fatal("expected structural artifact for primary MODUS fact")
	}
	if found.Get("type") != "candidate_structural_link_transition" {
		t.Fatalf("type = %q, want candidate_structural_link_transition", found.Get("type"))
	}
	if found.Get("status") != "pending" {
		t.Fatalf("status = %q, want pending", found.Get("status"))
	}

	if refs := stringSliceField(found.Frontmatter["proposed_related_fact_paths"]); len(refs) == 0 || refs[0] != "memory/facts/modus-purpose.md" {
		t.Fatalf("proposed_related_fact_paths = %#v, want memory/facts/modus-purpose.md", found.Frontmatter["proposed_related_fact_paths"])
	}
	if refs := stringSliceField(found.Frontmatter["proposed_related_episode_paths"]); len(refs) == 0 || refs[0] != "memory/episodes/evt-structural.md" {
		t.Fatalf("proposed_related_episode_paths = %#v, want memory/episodes/evt-structural.md", found.Frontmatter["proposed_related_episode_paths"])
	}
	if refs := stringSliceField(found.Frontmatter["proposed_related_entity_refs"]); len(refs) == 0 || refs[0] != "MODUS" {
		t.Fatalf("proposed_related_entity_refs = %#v, want MODUS", found.Frontmatter["proposed_related_entity_refs"])
	}
	if refs := stringSliceField(found.Frontmatter["proposed_related_mission_refs"]); len(refs) == 0 || refs[0] != "Memory Sovereignty" {
		t.Fatalf("proposed_related_mission_refs = %#v, want Memory Sovereignty", found.Frontmatter["proposed_related_mission_refs"])
	}
}

func TestApplyApprovedStructuralLinkTransitionUpdatesFact(t *testing.T) {
	v, dir := setupVault(t)

	if err := markdown.Write(filepath.Join(dir, "atlas", "entities", "modus.md"), map[string]interface{}{
		"name": "MODUS",
	}, "# MODUS\n"); err != nil {
		t.Fatalf("write entity: %v", err)
	}

	writeFact(t, dir, "modus-core", map[string]interface{}{
		"subject":    "MODUS",
		"predicate":  "operates",
		"created_at": time.Now().Format(time.RFC3339),
	}, "MODUS operates as a governed memory system.")
	writeFact(t, dir, "modus-related", map[string]interface{}{
		"subject":    "MODUS",
		"predicate":  "requires",
		"created_at": time.Now().Format(time.RFC3339),
	}, "MODUS requires durable structural memory.")
	if err := markdown.Write(filepath.Join(dir, "memory", "episodes", "evt-structural.md"), map[string]interface{}{
		"type":       "memory_episode",
		"event_id":   "evt-structural",
		"created_at": time.Now().Format(time.RFC3339),
	}, "Structural linking was discussed in this episode."); err != nil {
		t.Fatalf("write episode: %v", err)
	}

	if err := WriteStructuralLinkTransitionCandidate(v, StructuralLinkTransitionCandidate{
		FactPath:                    "memory/facts/modus-core.md",
		ProposedRelatedFactPaths:    []string{"memory/facts/modus-related.md"},
		ProposedRelatedEpisodePaths: []string{"memory/episodes/evt-structural.md"},
		ProposedRelatedEntityRefs:   []string{"MODUS"},
		ProposedRelatedMissionRefs:  []string{"Memory Sovereignty"},
		Reason:                      "Backfill structural links from proven runtime evidence.",
		ProducingOffice:             "memory_governance",
		ProducingSubsystem:          "test_structural_apply",
		AuthorityScope:              ledger.ScopeCandidateStructuralLinkReview,
	}); err != nil {
		t.Fatalf("WriteStructuralLinkTransitionCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*structural-links*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 structural artifact, got %d", len(files))
	}
	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	doc.Set("status", "approved")
	if err := doc.Save(); err != nil {
		t.Fatalf("approve artifact: %v", err)
	}

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved failed: %v", err)
	}
	if result.StructuralTransitionsApplied != 1 {
		t.Fatalf("StructuralTransitionsApplied = %d, want 1", result.StructuralTransitionsApplied)
	}

	fact, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "modus-core.md"))
	if err != nil {
		t.Fatalf("read fact: %v", err)
	}
	if refs := stringSliceField(fact.Frontmatter["related_fact_paths"]); len(refs) != 1 || refs[0] != "memory/facts/modus-related.md" {
		t.Fatalf("related_fact_paths = %#v, want [memory/facts/modus-related.md]", fact.Frontmatter["related_fact_paths"])
	}
	if refs := stringSliceField(fact.Frontmatter["related_episode_paths"]); len(refs) != 1 || refs[0] != "memory/episodes/evt-structural.md" {
		t.Fatalf("related_episode_paths = %#v, want [memory/episodes/evt-structural.md]", fact.Frontmatter["related_episode_paths"])
	}
	if refs := stringSliceField(fact.Frontmatter["related_entity_refs"]); len(refs) != 1 || refs[0] != "MODUS" {
		t.Fatalf("related_entity_refs = %#v, want [MODUS]", fact.Frontmatter["related_entity_refs"])
	}
	if refs := stringSliceField(fact.Frontmatter["related_mission_refs"]); len(refs) != 1 || refs[0] != "Memory Sovereignty" {
		t.Fatalf("related_mission_refs = %#v, want [Memory Sovereignty]", fact.Frontmatter["related_mission_refs"])
	}
	if fact.Get("structural_link_review_artifact") == "" {
		t.Fatal("expected structural_link_review_artifact on fact")
	}
	if len(stringSliceField(fact.Frontmatter["structural_link_history"])) == 0 {
		t.Fatal("expected structural_link_history on fact")
	}
}

func TestRunStructuralModeReportsCandidates(t *testing.T) {
	v, dir := setupVault(t)

	if err := markdown.Write(filepath.Join(dir, "atlas", "entities", "modus.md"), map[string]interface{}{
		"name": "MODUS",
	}, "# MODUS\n"); err != nil {
		t.Fatalf("write entity: %v", err)
	}

	writeFact(t, dir, "modus-core", map[string]interface{}{
		"subject":         "MODUS",
		"predicate":       "operates",
		"mission":         "Memory Sovereignty",
		"lineage_id":      "lin-structural",
		"work_item_id":    "work-structural",
		"source_event_id": "evt-structural",
		"created_at":      time.Now().Format(time.RFC3339),
	}, "MODUS operates as a governed memory system.")
	writeFact(t, dir, "modus-related", map[string]interface{}{
		"subject":         "MODUS",
		"predicate":       "requires",
		"mission":         "Memory Sovereignty",
		"lineage_id":      "lin-structural",
		"work_item_id":    "work-structural",
		"source_event_id": "evt-structural",
		"created_at":      time.Now().Format(time.RFC3339),
	}, "MODUS requires governed structural memory.")
	if err := markdown.Write(filepath.Join(dir, "memory", "episodes", "evt-structural.md"), map[string]interface{}{
		"type":         "memory_episode",
		"event_id":     "evt-structural",
		"lineage_id":   "lin-structural",
		"subject":      "MODUS",
		"mission":      "Memory Sovereignty",
		"work_item_id": "work-structural",
		"created_at":   time.Now().Format(time.RFC3339),
	}, "Structural linking was discussed in this episode."); err != nil {
		t.Fatalf("write episode: %v", err)
	}

	report, err := Run(v, ModeStructural, false)
	if err != nil {
		t.Fatalf("Run structural failed: %v", err)
	}
	if report.StructuralReviewed == 0 {
		t.Fatal("expected structural-reviewed count in report")
	}
	if !strings.Contains(FormatReport(report), "Structural link review candidates") {
		t.Fatal("expected structural summary line in formatted report")
	}
}

func TestWriteFactTemporalTransitionCandidate(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "scout-lane-legacy", map[string]interface{}{
		"subject":            "Scout lane",
		"predicate":          "uses",
		"memory_temperature": "warm",
		"importance":         "high",
		"confidence":         0.88,
		"created_at":         "2026-04-10T10:00:00Z",
		"observed_at":        "2026-04-10T10:00:00Z",
		"valid_from":         "2026-04-10T10:00:00Z",
		"temporal_status":    "active",
	}, "Scout lane uses qwen-3.6 on the CLI lane.")
	writeFact(t, dir, "scout-lane-current", map[string]interface{}{
		"subject":            "Scout lane",
		"predicate":          "uses",
		"memory_temperature": "warm",
		"importance":         "high",
		"confidence":         0.93,
		"created_at":         "2026-04-15T10:00:00Z",
		"observed_at":        "2026-04-15T10:00:00Z",
		"valid_from":         "2026-04-15T10:00:00Z",
		"temporal_status":    "active",
	}, "Scout lane uses gemini-2.5-flash on the api lane.")

	if err := WriteFactTemporalTransitionCandidate(v, FactTemporalTransitionCandidate{
		FactPath:               "memory/facts/scout-lane-legacy.md",
		ProposedTemporalStatus: "superseded",
		SupersededByPath:       "memory/facts/scout-lane-current.md",
		Reason:                 "New commissioned staffing replaced the old lane.",
		ReviewClass:            "supersession",
		ProducingOffice:        "memory_governance",
		ProducingSubsystem:     "test_temporal_review",
		AuthorityScope:         ledger.ScopeCandidateFactTemporalReview,
	}); err != nil {
		t.Fatalf("WriteFactTemporalTransitionCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*temporal-transition*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 temporal transition artifact, got %d", len(files))
	}
	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if doc.Get("type") != "candidate_fact_temporal_transition" {
		t.Fatalf("type = %q, want candidate_fact_temporal_transition", doc.Get("type"))
	}
	if doc.Get("superseded_by_path") != "memory/facts/scout-lane-current.md" {
		t.Fatalf("superseded_by_path = %q, want memory/facts/scout-lane-current.md", doc.Get("superseded_by_path"))
	}
	if doc.Get("proposed_temporal_status") != "superseded" {
		t.Fatalf("proposed_temporal_status = %q, want superseded", doc.Get("proposed_temporal_status"))
	}
}

func TestApplyApprovedFactTemporalTransitionUpdatesFactAndReplacement(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "scout-lane-legacy", map[string]interface{}{
		"subject":            "Scout lane",
		"predicate":          "uses",
		"memory_temperature": "warm",
		"importance":         "high",
		"confidence":         0.88,
		"created_at":         "2026-04-10T10:00:00Z",
		"observed_at":        "2026-04-10T10:00:00Z",
		"valid_from":         "2026-04-10T10:00:00Z",
		"temporal_status":    "active",
	}, "Scout lane uses qwen-3.6 on the CLI lane.")
	writeFact(t, dir, "scout-lane-current", map[string]interface{}{
		"subject":            "Scout lane",
		"predicate":          "uses",
		"memory_temperature": "warm",
		"importance":         "high",
		"confidence":         0.93,
		"created_at":         "2026-04-15T10:00:00Z",
		"observed_at":        "2026-04-15T10:00:00Z",
		"valid_from":         "2026-04-15T10:00:00Z",
		"temporal_status":    "active",
	}, "Scout lane uses gemini-2.5-flash on the api lane.")

	if err := WriteFactTemporalTransitionCandidate(v, FactTemporalTransitionCandidate{
		FactPath:               "memory/facts/scout-lane-legacy.md",
		ProposedTemporalStatus: "superseded",
		SupersededByPath:       "memory/facts/scout-lane-current.md",
		Reason:                 "New commissioned staffing replaced the old lane.",
		ReviewClass:            "supersession",
		ProducingOffice:        "memory_governance",
		ProducingSubsystem:     "test_temporal_apply",
		AuthorityScope:         ledger.ScopeCandidateFactTemporalReview,
	}); err != nil {
		t.Fatalf("WriteFactTemporalTransitionCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*temporal-transition*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 temporal transition artifact, got %d", len(files))
	}
	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	doc.Set("status", "approved")
	if err := doc.Save(); err != nil {
		t.Fatalf("approve artifact: %v", err)
	}

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved failed: %v", err)
	}
	if result.TemporalTransitionsApplied != 1 {
		t.Fatalf("TemporalTransitionsApplied = %d, want 1", result.TemporalTransitionsApplied)
	}

	legacyFact, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "scout-lane-legacy.md"))
	if err != nil {
		t.Fatalf("read legacy fact: %v", err)
	}
	if legacyFact.Get("temporal_status") != "superseded" {
		t.Fatalf("temporal_status = %q, want superseded", legacyFact.Get("temporal_status"))
	}
	if legacyFact.Get("superseded_by") != "memory/facts/scout-lane-current.md" {
		t.Fatalf("superseded_by = %q, want memory/facts/scout-lane-current.md", legacyFact.Get("superseded_by"))
	}
	if legacyFact.Get("temporal_review_artifact") == "" {
		t.Fatal("expected temporal review artifact reference on superseded fact")
	}
	if legacyFact.Get("valid_to") == "" {
		t.Fatal("expected valid_to to be set on superseded fact")
	}

	currentFact, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "scout-lane-current.md"))
	if err != nil {
		t.Fatalf("read current fact: %v", err)
	}
	raw := currentFact.Frontmatter["supersedes_paths"]
	items, ok := raw.([]interface{})
	if !ok || len(items) != 1 || items[0] != "memory/facts/scout-lane-legacy.md" {
		t.Fatalf("supersedes_paths = %#v, want [memory/facts/scout-lane-legacy.md]", raw)
	}
}

func TestReviewElderTierCreatesPromotionCandidate(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "founding-lesson", map[string]interface{}{
		"subject":                 "Founding lesson",
		"predicate":               "requires",
		"importance":              "high",
		"confidence":              0.93,
		"memory_temperature":      "warm",
		"memory_protection_class": "ordinary",
		"created_at":              time.Now().Add(-120 * 24 * time.Hour).Format(time.RFC3339),
		"source":                  "campaign journal",
		"source_ref":              "vault/sessions/2026-04-14-grade-s-memory-program.md",
		"mission":                 "Memory Sovereignty",
		"lineage_id":              "lineage-founding-lesson",
	}, "Rare but consequential memory must be governed explicitly.")

	n, actions, err := ReviewElderTier(v)
	if err != nil {
		t.Fatalf("ReviewElderTier failed: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least one elder review candidate")
	}
	if len(actions) == 0 {
		t.Fatal("expected elder review actions")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*elder-memory*.md"))
	if len(files) == 0 {
		t.Fatal("expected elder-memory review artifact")
	}
	foundPromotion := false
	for _, file := range files {
		doc, err := markdown.Parse(file)
		if err != nil {
			t.Fatalf("parse elder review artifact: %v", err)
		}
		if doc.Get("type") == "candidate_elder_memory_transition" && doc.Get("review_class") == "promotion" {
			foundPromotion = true
			if doc.Get("proposed_protection_class") != "elder" {
				t.Fatalf("proposed_protection_class = %q, want elder", doc.Get("proposed_protection_class"))
			}
		}
	}
	if !foundPromotion {
		t.Fatal("expected elder promotion candidate")
	}
}

func TestApplyApprovedElderMemoryTransitionUpdatesFact(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "founding-lesson", map[string]interface{}{
		"subject":                 "Founding lesson",
		"predicate":               "requires",
		"memory_temperature":      "warm",
		"memory_protection_class": "ordinary",
		"importance":              "high",
		"confidence":              0.91,
		"created_at":              time.Now().Add(-120 * 24 * time.Hour).Format(time.RFC3339),
	}, "Rare but consequential memory must be governed explicitly.")

	if err := WriteElderMemoryTransitionCandidate(v, ElderMemoryTransitionCandidate{
		FactPath:                "memory/facts/founding-lesson.md",
		ProposedProtectionClass: "elder",
		Reason:                  "Protect this rare long-horizon lesson from automatic burial.",
		ProducingOffice:         "memory_governance",
		ProducingSubsystem:      "test_elder_apply",
		AuthorityScope:          ledger.ScopeCandidateElderMemoryReview,
	}); err != nil {
		t.Fatalf("WriteElderMemoryTransitionCandidate failed: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*elder-memory*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 elder-memory artifact, got %d", len(files))
	}
	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	doc.Set("status", "approved")
	if err := doc.Save(); err != nil {
		t.Fatalf("approve artifact: %v", err)
	}

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved failed: %v", err)
	}
	if result.ElderTransitionsApplied != 1 {
		t.Fatalf("ElderTransitionsApplied = %d, want 1", result.ElderTransitionsApplied)
	}

	fact, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "founding-lesson.md"))
	if err != nil {
		t.Fatalf("read fact: %v", err)
	}
	if fact.Get("memory_protection_class") != "elder" {
		t.Fatalf("memory_protection_class = %q, want elder", fact.Get("memory_protection_class"))
	}
	if fact.Get("memory_protection_class_previous") != "ordinary" {
		t.Fatalf("memory_protection_class_previous = %q, want ordinary", fact.Get("memory_protection_class_previous"))
	}
	if fact.Get("memory_protection_review_artifact") == "" {
		t.Fatal("expected protection review artifact reference on fact")
	}
}

func TestReviewElderTierCreatesContradictionAnomalyForElderFact(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "elder-fact", map[string]interface{}{
		"subject":                 "General flagship",
		"predicate":               "codename",
		"confidence":              0.95,
		"importance":              "critical",
		"memory_protection_class": "elder",
		"created_at":              time.Now().Add(-400 * 24 * time.Hour).Format(time.RFC3339),
		"source_ref":              "vault/sessions/journal.md",
	}, "brass lantern")
	writeFact(t, dir, "competing-fact", map[string]interface{}{
		"subject":    "General flagship",
		"predicate":  "codename",
		"confidence": 0.81,
		"importance": "high",
		"created_at": time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339),
	}, "quiet harbor")

	if _, _, err := Contradict(v); err != nil {
		t.Fatalf("Contradict failed: %v", err)
	}
	n, actions, err := ReviewElderTier(v)
	if err != nil {
		t.Fatalf("ReviewElderTier failed: %v", err)
	}
	if n == 0 || len(actions) == 0 {
		t.Fatal("expected elder anomaly candidate from contradiction")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*elder-anomaly*.md"))
	if len(files) == 0 {
		t.Fatal("expected elder anomaly artifact")
	}
	foundContradiction := false
	for _, file := range files {
		doc, err := markdown.Parse(file)
		if err != nil {
			t.Fatalf("parse elder anomaly artifact: %v", err)
		}
		if doc.Get("anomaly_class") == "contradiction" {
			foundContradiction = true
			if doc.Get("fact_path") != "memory/facts/elder-fact.md" {
				t.Fatalf("fact_path = %q, want elder fact path", doc.Get("fact_path"))
			}
		}
	}
	if !foundContradiction {
		t.Fatal("expected contradiction anomaly for elder fact")
	}
}

// --- Contradict tests ---

func TestContradictConflictingFacts(t *testing.T) {
	v, dir := setupVault(t)

	// Same subject+predicate, different values
	writeFact(t, dir, "go-version-1", map[string]interface{}{
		"subject": "Go", "predicate": "latest version", "confidence": 0.9,
	}, "1.24")

	writeFact(t, dir, "go-version-2", map[string]interface{}{
		"subject": "Go", "predicate": "latest version", "confidence": 0.6,
	}, "1.22")

	n, actions, err := Contradict(v)
	if err != nil {
		t.Fatalf("Contradict failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 contradiction, got %d", n)
	}
	if len(actions) < 1 {
		t.Error("expected at least 1 action")
	}

	// Check artifact was written
	maintenanceDir := filepath.Join(dir, "memory", "maintenance")
	files, _ := filepath.Glob(filepath.Join(maintenanceDir, "*contradiction*.md"))
	if len(files) != 1 {
		t.Errorf("expected 1 contradiction artifact, got %d", len(files))
	}

	if len(files) > 0 {
		doc, err := markdown.Parse(files[0])
		if err != nil {
			t.Fatalf("failed to parse contradiction artifact: %v", err)
		}
		if doc.Get("type") != "candidate_contradiction" {
			t.Errorf("type = %q, want candidate_contradiction", doc.Get("type"))
		}
		if doc.Get("status") != "pending" {
			t.Errorf("status = %q, want pending", doc.Get("status"))
		}
		// Proposed winner should be higher confidence
		if doc.GetFloat("proposed_conf") < doc.GetFloat("competing_conf") {
			t.Error("proposed winner should have higher confidence")
		}
		raw, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatalf("read contradiction artifact: %v", err)
		}
		if !strings.Contains(string(raw), "producing_signature:") {
			t.Fatal("contradiction artifact missing producing_signature")
		}
	}

	ledgerData, err := os.ReadFile(filepath.Join(dir, "state", "operations", "operations.jsonl"))
	if err != nil {
		t.Fatalf("read operations ledger: %v", err)
	}
	var rec map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(ledgerData)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse ledger line: %v", err)
	}
	if rec["action_class"] != "review_candidate_generation" {
		t.Fatalf("action_class = %v, want review_candidate_generation", rec["action_class"])
	}
}

func TestContradictSameValues(t *testing.T) {
	v, dir := setupVault(t)

	// Same subject+predicate+value = not a contradiction (it's a duplicate)
	writeFact(t, dir, "go-creator-1", map[string]interface{}{
		"subject": "Go", "predicate": "created by", "confidence": 0.9,
	}, "Rob Pike, Ken Thompson, Robert Griesemer")

	writeFact(t, dir, "go-creator-2", map[string]interface{}{
		"subject": "Go", "predicate": "created by", "confidence": 0.7,
	}, "Rob Pike, Ken Thompson, Robert Griesemer")

	n, _, err := Contradict(v)
	if err != nil {
		t.Fatalf("Contradict failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 contradictions for same values, got %d", n)
	}
}

// --- Bootstrap tests ---

func TestBootstrapFromBrain(t *testing.T) {
	v, dir := setupVault(t)

	// Write a prose document in brain/
	brainPath := filepath.Join(dir, "brain", "architecture.md")
	markdown.Write(brainPath, map[string]interface{}{
		"title": "Architecture Notes",
	}, "MODUS uses Go for the core agent framework. The Archive runs on FastAPI. Python 3.14 is the runtime version.")

	n, actions, err := Bootstrap(v)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}
	if n == 0 {
		t.Error("expected at least 1 bootstrap candidate")
	}
	if len(actions) == 0 {
		t.Error("expected at least 1 action")
	}

	// Check artifacts were written
	maintenanceDir := filepath.Join(dir, "memory", "maintenance")
	files, _ := filepath.Glob(filepath.Join(maintenanceDir, "*bootstrap*.md"))
	if len(files) == 0 {
		t.Error("expected at least 1 bootstrap artifact")
	}

	// Verify artifact
	if len(files) > 0 {
		doc, err := markdown.Parse(files[0])
		if err != nil {
			t.Fatalf("failed to parse bootstrap artifact: %v", err)
		}
		if doc.Get("type") != "candidate_bootstrap_fact" {
			t.Errorf("type = %q, want candidate_bootstrap_fact", doc.Get("type"))
		}
		if doc.Get("method") != "heuristic-regex" {
			t.Errorf("method = %q, want heuristic-regex", doc.Get("method"))
		}
	}
}

func TestBootstrapDedup(t *testing.T) {
	v, dir := setupVault(t)

	// Existing fact
	writeFact(t, dir, "modus-uses-go", map[string]interface{}{
		"subject": "MODUS", "predicate": "uses", "confidence": 0.9,
	}, "Go")

	// Brain doc that mentions the same thing
	brainPath := filepath.Join(dir, "brain", "notes.md")
	markdown.Write(brainPath, map[string]interface{}{}, "MODUS uses Go for everything.")

	n, _, err := Bootstrap(v)
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// The "MODUS uses Go" fact should be deduped
	maintenanceDir := filepath.Join(dir, "memory", "maintenance")
	files, _ := filepath.Glob(filepath.Join(maintenanceDir, "*bootstrap*modus*uses*.md"))
	if len(files) > 0 {
		t.Errorf("expected MODUS/uses to be deduped, but got %d artifacts", len(files))
	}
	// n could be 0 or >0 depending on other extractions — just check the dedup worked
	_ = n
}

// --- Dispatcher tests ---

func TestRunAll(t *testing.T) {
	v, dir := setupVault(t)

	// Set up some data for each pass
	writeFact(t, dir, "dup-1", map[string]interface{}{
		"subject": "Test", "predicate": "is", "confidence": 0.9,
	}, "Test is a testing framework for Go applications and services")

	writeFact(t, dir, "dup-2", map[string]interface{}{
		"subject": "Test", "predicate": "is", "confidence": 0.7,
	}, "Test is a testing framework for Go applications")

	brainPath := filepath.Join(dir, "brain", "stack.md")
	markdown.Write(brainPath, map[string]interface{}{}, "Redis uses an in-memory data structure store.")

	report, err := Run(v, ModeAll, false)
	if err != nil {
		t.Fatalf("Run(all) failed: %v", err)
	}

	if report.Consolidated < 1 {
		t.Error("expected at least 1 consolidation candidate")
	}
	if report.Duration <= 0 {
		t.Error("expected positive duration")
	}

	formatted := FormatReport(report)
	if !strings.Contains(formatted, "Maintenance Report") {
		t.Error("missing report header")
	}
	if !strings.Contains(formatted, "review artifacts") {
		t.Error("missing review artifacts note")
	}
}

func TestRunInvalidMode(t *testing.T) {
	v, _ := setupVault(t)
	_, err := Run(v, "invalid", false)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

// --- Tokenizer / Jaccard tests ---

func TestJaccardSimilarity(t *testing.T) {
	// Identical strings
	sim := jaccardSimilarity("hello world", "hello world")
	if sim != 1.0 {
		t.Errorf("identical strings: sim = %.2f, want 1.0", sim)
	}

	// Completely different
	sim = jaccardSimilarity("hello world", "foo bar baz")
	if sim > 0.01 {
		t.Errorf("different strings: sim = %.2f, want ~0", sim)
	}

	// Partial overlap
	sim = jaccardSimilarity("Go is a compiled language", "Go is a compiled systems language")
	if sim < 0.5 {
		t.Errorf("partial overlap: sim = %.2f, expected > 0.5", sim)
	}

	// Empty strings
	sim = jaccardSimilarity("", "")
	if sim != 1.0 {
		t.Errorf("empty strings: sim = %.2f, want 1.0", sim)
	}
}

func TestExtractFactCandidates(t *testing.T) {
	body := "MODUS uses Go for the agent framework. Redis is a fast in-memory database. Python 3.14 is the runtime."

	facts := extractFactCandidates(body)
	if len(facts) == 0 {
		t.Fatal("expected at least 1 extracted fact")
	}

	// Check we got a "uses" pattern
	foundUses := false
	for _, f := range facts {
		if f.predicate == "uses" && strings.EqualFold(f.subject, "MODUS") {
			foundUses = true
		}
	}
	if !foundUses {
		t.Error("expected to extract 'MODUS uses Go'")
	}
}

func TestReplayCreatesCandidateFromEpisodeAndRecallEvidence(t *testing.T) {
	v, dir := setupVault(t)

	if err := markdown.Write(filepath.Join(dir, "memory", "episodes", "evt-test.md"), map[string]interface{}{
		"type":         "memory_episode",
		"event_id":     "evt-test",
		"lineage_id":   "lin-test",
		"mission":      "Memory Sovereignty",
		"work_item_id": "work-memory",
		"environment":  "operator-shell",
		"cue_terms":    []string{"modus", "go", "agent"},
	}, "MODUS uses Go for the agent framework."); err != nil {
		t.Fatalf("write episode: %v", err)
	}

	recallDir := filepath.Join(dir, "memory", "recalls", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(recallDir, 0o755); err != nil {
		t.Fatalf("mkdir recall dir: %v", err)
	}
	for idx := 1; idx <= 2; idx++ {
		if err := markdown.Write(filepath.Join(recallDir, fmt.Sprintf("recall-%d.md", idx)), map[string]interface{}{
			"type":               "memory_recall_receipt",
			"source_event_ids":   []string{"evt-test"},
			"lineage_ids":        []string{"lin-test"},
			"route_missions":     []string{"Memory Sovereignty"},
			"route_work_item_id": "work-memory",
			"route_environment":  "operator-shell",
			"route_cue_terms":    []string{"modus", "go"},
		}, "Recall body."); err != nil {
			t.Fatalf("write recall %d: %v", idx, err)
		}
	}

	n, actions, err := Replay(v)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("Replay produced %d candidates, want 1", n)
	}
	if len(actions) == 0 {
		t.Fatal("expected replay actions")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "memory", "maintenance", "*bootstrap*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 replay artifact, got %d", len(files))
	}

	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse replay artifact: %v", err)
	}
	if doc.Get("type") != "candidate_replay_fact" {
		t.Fatalf("type = %q, want candidate_replay_fact", doc.Get("type"))
	}
	if doc.Get("method") != "replay-consensus" {
		t.Fatalf("method = %q, want replay-consensus", doc.Get("method"))
	}
	if doc.Get("source_event_id") != "evt-test" {
		t.Fatalf("source_event_id = %q, want evt-test", doc.Get("source_event_id"))
	}
	if doc.Get("lineage_id") != "lin-test" {
		t.Fatalf("lineage_id = %q, want lin-test", doc.Get("lineage_id"))
	}
	if doc.Get("mission") != "Memory Sovereignty" {
		t.Fatalf("mission = %q, want Memory Sovereignty", doc.Get("mission"))
	}
	if doc.Get("work_item_id") != "work-memory" {
		t.Fatalf("work_item_id = %q, want work-memory", doc.Get("work_item_id"))
	}
	if doc.Get("environment") != "operator-shell" {
		t.Fatalf("environment = %q, want operator-shell", doc.Get("environment"))
	}
	if got := int(doc.GetFloat("evidence_episode_count")); got != 1 {
		t.Fatalf("evidence_episode_count = %d, want 1", got)
	}
	if got := int(doc.GetFloat("evidence_recall_count")); got != 2 {
		t.Fatalf("evidence_recall_count = %d, want 2", got)
	}
}

// --- Apply tests ---

func TestApplyApprovedBootstrap(t *testing.T) {
	v, dir := setupVault(t)

	// Write an approved bootstrap candidate
	writeFact(t, dir, "../maintenance/bootstrap-redis", map[string]interface{}{
		"type":       "candidate_bootstrap_fact",
		"status":     "approved",
		"subject":    "Redis",
		"predicate":  "is",
		"value":      "an in-memory data store",
		"confidence": 0.5,
		"importance": "medium",
	}, "# Bootstrap Fact Candidate\n\nRedis is an in-memory data store.")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.BootstrapPromoted != 1 {
		t.Errorf("BootstrapPromoted = %d, want 1", result.BootstrapPromoted)
	}

	// Verify fact was created
	facts, _ := markdown.ScanDir(v.Path("memory", "facts"))
	found := false
	for _, f := range facts {
		if strings.EqualFold(f.Get("subject"), "Redis") {
			found = true
		}
	}
	if !found {
		t.Error("expected Redis fact to be created")
	}

	// Verify artifact was marked applied
	maintenanceDocs, _ := markdown.ScanDir(v.Path("memory", "maintenance"))
	for _, doc := range maintenanceDocs {
		if doc.Get("type") == "candidate_bootstrap_fact" && doc.Get("status") != "applied" {
			t.Error("expected bootstrap artifact status to be 'applied'")
		}
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
	if rec["action_class"] != "maintenance_apply" {
		t.Fatalf("action_class = %v, want maintenance_apply", rec["action_class"])
	}
}

func TestApplyApprovedReplayPromotionPreservesProvenance(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "../maintenance/replay-modus-go", map[string]interface{}{
		"type":                   "candidate_replay_fact",
		"status":                 "approved",
		"subject":                "MODUS",
		"predicate":              "uses",
		"value":                  "Go for the agent framework",
		"source":                 "memory/episodes/evt-test.md",
		"source_refs":            []string{"memory/episodes/evt-test.md", "memory/recalls/2026-04-14/recall-1.md"},
		"source_event_id":        "evt-test",
		"lineage_id":             "lin-test",
		"cue_terms":              []string{"modus", "go", "agent"},
		"mission":                "Memory Sovereignty",
		"work_item_id":           "work-memory",
		"environment":            "operator-shell",
		"confidence":             0.79,
		"importance":             "high",
		"method":                 "replay-consensus",
		"evidence_episode_count": 1,
		"evidence_recall_count":  2,
	}, "# Replay Promotion Candidate\n\nMODUS uses Go for the agent framework.")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.ReplayPromoted != 1 {
		t.Fatalf("ReplayPromoted = %d, want 1", result.ReplayPromoted)
	}

	facts, err := markdown.ScanDir(filepath.Join(dir, "memory", "facts"))
	if err != nil {
		t.Fatalf("scan facts: %v", err)
	}
	var found *markdown.Document
	for _, doc := range facts {
		if strings.EqualFold(doc.Get("subject"), "MODUS") && strings.EqualFold(doc.Get("predicate"), "uses") {
			found = doc
			break
		}
	}
	if found == nil {
		t.Fatal("expected replay-promoted fact")
	}
	if found.Get("source_event_id") != "evt-test" {
		t.Fatalf("source_event_id = %q, want evt-test", found.Get("source_event_id"))
	}
	if found.Get("lineage_id") != "lin-test" {
		t.Fatalf("lineage_id = %q, want lin-test", found.Get("lineage_id"))
	}
	if found.Get("mission") != "Memory Sovereignty" {
		t.Fatalf("mission = %q, want Memory Sovereignty", found.Get("mission"))
	}
	if found.Get("work_item_id") != "work-memory" {
		t.Fatalf("work_item_id = %q, want work-memory", found.Get("work_item_id"))
	}
	if found.Get("environment") != "operator-shell" {
		t.Fatalf("environment = %q, want operator-shell", found.Get("environment"))
	}
	if raw := found.Frontmatter["cue_terms"]; raw == nil {
		t.Fatal("expected cue_terms on replay-promoted fact")
	}
}

func TestApplyApprovedSkipsPending(t *testing.T) {
	v, dir := setupVault(t)

	// Write a pending artifact — should NOT be applied
	writeFact(t, dir, "../maintenance/pending-merge", map[string]interface{}{
		"type":   "candidate_merge",
		"status": "pending",
	}, "Should not be applied.")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.MergesApplied != 0 {
		t.Errorf("should not apply pending artifacts, got %d merges", result.MergesApplied)
	}
}

func TestApplyViaRunDispatcher(t *testing.T) {
	v, dir := setupVault(t)

	// Write an approved bootstrap
	writeFact(t, dir, "../maintenance/bootstrap-test", map[string]interface{}{
		"type":       "candidate_bootstrap_fact",
		"status":     "approved",
		"subject":    "TestThing",
		"predicate":  "version",
		"value":      "1.0",
		"confidence": 0.5,
		"importance": "low",
	}, "Test bootstrap.")

	report, err := Run(v, ModeApply, false)
	if err != nil {
		t.Fatalf("Run(apply): %v", err)
	}
	if report.Bootstrapped != 1 {
		t.Errorf("Bootstrapped = %d, want 1", report.Bootstrapped)
	}
}

func TestApplyContradictionByPath(t *testing.T) {
	v, dir := setupVault(t)

	// Write two facts with identical confidence — the fragile case
	writeFact(t, dir, "go-version-new", map[string]interface{}{
		"subject": "Go", "predicate": "latest version", "confidence": 0.9,
	}, "1.24")

	writeFact(t, dir, "go-version-old", map[string]interface{}{
		"subject": "Go", "predicate": "latest version", "confidence": 0.9,
	}, "1.22")

	// Write a contradiction artifact with explicit paths (as contradict.go now produces)
	writeFact(t, dir, "../maintenance/contradiction-go-version", map[string]interface{}{
		"type":           "candidate_contradiction",
		"status":         "approved",
		"subject":        "Go",
		"predicate":      "latest version",
		"proposed_path":  "memory/facts/go-version-new.md",
		"competing_path": "memory/facts/go-version-old.md",
		"proposed_conf":  0.9,
		"competing_conf": 0.9,
		"winner":         "proposed",
	}, "# Contradiction\n\nKeep go-version-new.")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.ContradictionsResolved != 1 {
		t.Errorf("ContradictionsResolved = %d, want 1", result.ContradictionsResolved)
	}

	// Verify the OLD fact was archived, not the new one
	oldDoc, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "go-version-old.md"))
	if err != nil {
		t.Fatalf("parse old fact: %v", err)
	}
	if oldDoc.Get("archived") != "true" {
		t.Error("expected go-version-old to be archived")
	}

	newDoc, err := markdown.Parse(filepath.Join(dir, "memory", "facts", "go-version-new.md"))
	if err != nil {
		t.Fatalf("parse new fact: %v", err)
	}
	if newDoc.Get("archived") == "true" {
		t.Error("go-version-new should NOT be archived — it's the winner")
	}
}

func TestApplyContradictionCompetingWins(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "fact-a", map[string]interface{}{
		"subject": "X", "predicate": "is", "confidence": 0.8,
	}, "Value A")

	writeFact(t, dir, "fact-b", map[string]interface{}{
		"subject": "X", "predicate": "is", "confidence": 0.8,
	}, "Value B")

	// Reviewer decided competing (fact-b) is the winner
	writeFact(t, dir, "../maintenance/contradiction-x", map[string]interface{}{
		"type":           "candidate_contradiction",
		"status":         "resolved",
		"subject":        "X",
		"predicate":      "is",
		"proposed_path":  "memory/facts/fact-a.md",
		"competing_path": "memory/facts/fact-b.md",
		"proposed_conf":  0.8,
		"competing_conf": 0.8,
		"winner":         "competing",
	}, "# Contradiction\n\nKeep fact-b.")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.ContradictionsResolved != 1 {
		t.Errorf("ContradictionsResolved = %d, want 1", result.ContradictionsResolved)
	}

	// fact-a should be archived (proposed lost)
	aDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "fact-a.md"))
	if aDoc.Get("archived") != "true" {
		t.Error("fact-a should be archived — competing won")
	}

	// fact-b should survive
	bDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "fact-b.md"))
	if bDoc.Get("archived") == "true" {
		t.Error("fact-b should NOT be archived — it's the winner")
	}
}

// --- Gap 1: applyMerge was confidence-band only. Now uses explicit paths. ---
// This test creates two facts with IDENTICAL confidence (the fragile case)
// and verifies applyMerge archives the correct one by path, not by confidence scan.
func TestApplyMergeByPath(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "go-desc-long", map[string]interface{}{
		"subject": "Go", "predicate": "is", "confidence": 0.9,
	}, "Go is a statically typed compiled programming language designed at Google")

	writeFact(t, dir, "go-desc-short", map[string]interface{}{
		"subject": "Go", "predicate": "is", "confidence": 0.9,
	}, "Go is a compiled language")

	// Hand-craft an approved merge artifact with explicit paths
	writeFact(t, dir, "../maintenance/merge-go", map[string]interface{}{
		"type":          "candidate_merge",
		"status":        "approved",
		"stronger_subj": "Go",
		"stronger_pred": "is",
		"stronger_conf": 0.9,
		"stronger_path": "memory/facts/go-desc-long.md",
		"weaker_subj":   "Go",
		"weaker_pred":   "is",
		"weaker_conf":   0.9,
		"weaker_path":   "memory/facts/go-desc-short.md",
	}, "# Merge Candidate")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.MergesApplied != 1 {
		t.Errorf("MergesApplied = %d, want 1", result.MergesApplied)
	}

	// The short description should be archived
	shortDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "go-desc-short.md"))
	if shortDoc.Get("archived") != "true" {
		t.Error("go-desc-short should be archived — it's the weaker fact")
	}

	// The long description should survive
	longDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "go-desc-long.md"))
	if longDoc.Get("archived") == "true" {
		t.Error("go-desc-long should NOT be archived — it's the stronger fact")
	}
}

// --- Gap 2: contradict.go writes proposed_path/competing_path but no test verified it. ---
// This test runs Contradict() and asserts the artifact contains non-empty, valid paths.
func TestContradictArtifactContainsPaths(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "py-version-1", map[string]interface{}{
		"subject": "Python", "predicate": "latest version", "confidence": 0.8,
	}, "3.14")

	writeFact(t, dir, "py-version-2", map[string]interface{}{
		"subject": "Python", "predicate": "latest version", "confidence": 0.7,
	}, "3.12")

	_, _, err := Contradict(v)
	if err != nil {
		t.Fatalf("Contradict: %v", err)
	}

	maintenanceDir := filepath.Join(dir, "memory", "maintenance")
	files, _ := filepath.Glob(filepath.Join(maintenanceDir, "*contradiction*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 contradiction artifact, got %d", len(files))
	}

	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}

	proposedPath := doc.Get("proposed_path")
	competingPath := doc.Get("competing_path")

	if proposedPath == "" {
		t.Error("proposed_path is empty — contradict.go should store it")
	}
	if competingPath == "" {
		t.Error("competing_path is empty — contradict.go should store it")
	}

	// Paths should point to real files
	if _, err := os.Stat(filepath.Join(dir, proposedPath)); err != nil {
		t.Errorf("proposed_path %q does not exist: %v", proposedPath, err)
	}
	if _, err := os.Stat(filepath.Join(dir, competingPath)); err != nil {
		t.Errorf("competing_path %q does not exist: %v", competingPath, err)
	}

	// Proposed should have higher confidence
	if proposedPath == "" || competingPath == "" {
		return // already failed above
	}
	proposedDoc, _ := markdown.Parse(filepath.Join(dir, proposedPath))
	competingDoc, _ := markdown.Parse(filepath.Join(dir, competingPath))
	if proposedDoc.GetFloat("confidence") < competingDoc.GetFloat("confidence") {
		t.Error("proposed_path should point to the higher-confidence fact")
	}
}

// --- Gap 2b: consolidate.go now writes stronger_path/weaker_path — verify it. ---
func TestConsolidateArtifactContainsPaths(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "rust-desc-1", map[string]interface{}{
		"subject": "Rust", "predicate": "is", "confidence": 0.9,
	}, "Rust is a systems programming language focused on memory safety and performance")

	writeFact(t, dir, "rust-desc-2", map[string]interface{}{
		"subject": "Rust", "predicate": "is", "confidence": 0.7,
	}, "Rust is a systems programming language focused on memory safety")

	_, _, err := Consolidate(v)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	maintenanceDir := filepath.Join(dir, "memory", "maintenance")
	files, _ := filepath.Glob(filepath.Join(maintenanceDir, "*merge*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 merge artifact, got %d", len(files))
	}

	doc, err := markdown.Parse(files[0])
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}

	strongerPath := doc.Get("stronger_path")
	weakerPath := doc.Get("weaker_path")

	if strongerPath == "" {
		t.Error("stronger_path is empty — consolidate.go should store it")
	}
	if weakerPath == "" {
		t.Error("weaker_path is empty — consolidate.go should store it")
	}

	// Paths should point to real files
	if strongerPath != "" {
		if _, err := os.Stat(filepath.Join(dir, strongerPath)); err != nil {
			t.Errorf("stronger_path %q does not exist: %v", strongerPath, err)
		}
	}
	if weakerPath != "" {
		if _, err := os.Stat(filepath.Join(dir, weakerPath)); err != nil {
			t.Errorf("weaker_path %q does not exist: %v", weakerPath, err)
		}
	}
}

// --- Gap 5: Legacy fallback for contradiction artifacts without explicit paths. ---
// Verifies applyContradictionBySubject works for pre-migration artifacts.
func TestApplyContradictionLegacyFallback(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "node-version-new", map[string]interface{}{
		"subject": "Node", "predicate": "version", "confidence": 0.9,
	}, "22")

	writeFact(t, dir, "node-version-old", map[string]interface{}{
		"subject": "Node", "predicate": "version", "confidence": 0.5,
	}, "18")

	// Artifact WITHOUT proposed_path/competing_path — simulates pre-migration artifact.
	// Must fall back to subject+predicate+confidence matching.
	writeFact(t, dir, "../maintenance/contradiction-node", map[string]interface{}{
		"type":           "candidate_contradiction",
		"status":         "approved",
		"subject":        "Node",
		"predicate":      "version",
		"proposed_conf":  0.9,
		"competing_conf": 0.5,
		"winner":         "proposed",
	}, "# Contradiction\n\nKeep Node 22.")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.ContradictionsResolved != 1 {
		t.Errorf("ContradictionsResolved = %d, want 1", result.ContradictionsResolved)
	}

	// The 0.5 confidence fact should be archived via legacy fallback
	oldDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "node-version-old.md"))
	if oldDoc.Get("archived") != "true" {
		t.Error("node-version-old should be archived via legacy fallback")
	}

	newDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "node-version-new.md"))
	if newDoc.Get("archived") == "true" {
		t.Error("node-version-new should NOT be archived — it's the winner")
	}
}

func TestFormatApplyResultEmpty(t *testing.T) {
	result := &ApplyResult{}
	output := FormatApplyResult(result)
	if !strings.Contains(output, "No approved") {
		t.Error("should indicate no approved artifacts")
	}
}

// Legacy merge artifacts (pre-path) should still apply via subject/predicate/confidence fallback.
func TestApplyMergeLegacyFallback(t *testing.T) {
	v, dir := setupVault(t)

	writeFact(t, dir, "go-strong", map[string]interface{}{
		"subject": "Go", "predicate": "is", "confidence": 0.9,
	}, "Go is a compiled language")
	writeFact(t, dir, "go-weak", map[string]interface{}{
		"subject": "Go", "predicate": "is", "confidence": 0.5,
	}, "Go language")

	// Pre-migration merge artifact without stronger_path/weaker_path.
	writeFact(t, dir, "../maintenance/merge-go-legacy", map[string]interface{}{
		"type":          "candidate_merge",
		"status":        "approved",
		"stronger_subj": "Go",
		"stronger_pred": "is",
		"stronger_conf": 0.9,
		"weaker_subj":   "Go",
		"weaker_pred":   "is",
		"weaker_conf":   0.5,
	}, "# Legacy Merge Candidate")

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.MergesApplied != 1 {
		t.Fatalf("MergesApplied = %d, want 1", result.MergesApplied)
	}

	weakDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "go-weak.md"))
	if weakDoc.Get("archived") != "true" {
		t.Error("go-weak should be archived via legacy merge fallback")
	}
	strongDoc, _ := markdown.Parse(filepath.Join(dir, "memory", "facts", "go-strong.md"))
	if strongDoc.Get("archived") == "true" {
		t.Error("go-strong should not be archived")
	}
}

// End-to-end contract: detection writes path fields, approval flips status,
// apply uses those exact paths to archive the intended loser docs.
func TestDetectionToApplyPathContract(t *testing.T) {
	v, dir := setupVault(t)

	// Consolidate candidate pair.
	writeFact(t, dir, "rust-a", map[string]interface{}{
		"subject": "Rust", "predicate": "is", "confidence": 0.9,
	}, "Rust is a systems programming language focused on memory safety and speed")
	writeFact(t, dir, "rust-b", map[string]interface{}{
		"subject": "Rust", "predicate": "is", "confidence": 0.6,
	}, "Rust is a systems programming language focused on memory safety")

	// Contradiction candidate pair.
	writeFact(t, dir, "python-new", map[string]interface{}{
		"subject": "Python", "predicate": "latest version", "confidence": 0.9,
	}, "3.14")
	writeFact(t, dir, "python-old", map[string]interface{}{
		"subject": "Python", "predicate": "latest version", "confidence": 0.5,
	}, "3.12")

	if _, _, err := Consolidate(v); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if _, _, err := Contradict(v); err != nil {
		t.Fatalf("Contradict: %v", err)
	}

	maintenanceDocs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		t.Fatalf("scan maintenance: %v", err)
	}

	var mergeDoc, contradictionDoc *markdown.Document
	for _, doc := range maintenanceDocs {
		switch doc.Get("type") {
		case "candidate_merge":
			if strings.EqualFold(doc.Get("stronger_subj"), "Rust") {
				mergeDoc = doc
			}
		case "candidate_contradiction":
			if strings.EqualFold(doc.Get("subject"), "Python") {
				contradictionDoc = doc
			}
		}
	}
	if mergeDoc == nil || contradictionDoc == nil {
		t.Fatalf("expected both merge and contradiction artifacts")
	}

	mergeWeakerPath := mergeDoc.Get("weaker_path")
	contradictionLoserPath := contradictionDoc.Get("competing_path") // winner defaults to proposed
	if mergeWeakerPath == "" || contradictionLoserPath == "" {
		t.Fatalf("artifact paths missing (mergeWeakerPath=%q contradictionLoserPath=%q)", mergeWeakerPath, contradictionLoserPath)
	}

	// Approve both artifacts and apply.
	mergeDoc.Set("status", "approved")
	if err := mergeDoc.Save(); err != nil {
		t.Fatalf("save merge artifact: %v", err)
	}
	contradictionDoc.Set("status", "approved")
	if err := contradictionDoc.Save(); err != nil {
		t.Fatalf("save contradiction artifact: %v", err)
	}

	result, err := ApplyApproved(v)
	if err != nil {
		t.Fatalf("ApplyApproved: %v", err)
	}
	if result.MergesApplied != 1 {
		t.Fatalf("MergesApplied = %d, want 1", result.MergesApplied)
	}
	if result.ContradictionsResolved != 1 {
		t.Fatalf("ContradictionsResolved = %d, want 1", result.ContradictionsResolved)
	}

	mergeLoserDoc, err := markdown.Parse(filepath.Join(dir, mergeWeakerPath))
	if err != nil {
		t.Fatalf("parse merge loser fact: %v", err)
	}
	if mergeLoserDoc.Get("archived") != "true" {
		t.Error("merge loser was not archived by path contract")
	}

	contradictionLoserDoc, err := markdown.Parse(filepath.Join(dir, contradictionLoserPath))
	if err != nil {
		t.Fatalf("parse contradiction loser fact: %v", err)
	}
	if contradictionLoserDoc.Get("archived") != "true" {
		t.Error("contradiction loser was not archived by path contract")
	}
}

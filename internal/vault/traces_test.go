package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GetModus/modus-memory/internal/moduscfg"
)

func TestStoreTrace(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "traces"), 0755)
	v := New(dir, nil)

	steps := []string{"searched vault", "found 3 results", "synthesized answer"}
	tools := []string{"vault_search", "memory_facts"}

	relPath, err := v.StoreTrace("research auth patterns", "success", steps, 12.5, tools, "librarian", moduscfg.DefaultAssignment("librarian").Model)
	if err != nil {
		t.Fatalf("StoreTrace failed: %v", err)
	}
	if relPath == "" {
		t.Fatal("expected non-empty relPath")
	}

	// Verify file exists
	fullPath := filepath.Join(dir, relPath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatalf("trace file not created at %s", fullPath)
	}

	// Read it back and verify frontmatter
	doc, err := v.Read(relPath)
	if err != nil {
		t.Fatalf("Read trace failed: %v", err)
	}
	if doc.Get("task") != "research auth patterns" {
		t.Errorf("task = %q, want %q", doc.Get("task"), "research auth patterns")
	}
	if doc.Get("outcome") != "success" {
		t.Errorf("outcome = %q, want %q", doc.Get("outcome"), "success")
	}
	if doc.Get("memory_type") != "procedural" {
		t.Errorf("memory_type = %q, want %q", doc.Get("memory_type"), "procedural")
	}
	if doc.Get("created_by") != "librarian" {
		t.Errorf("created_by = %q, want %q", doc.Get("created_by"), "librarian")
	}
	if doc.Get("model") != moduscfg.DefaultAssignment("librarian").Model {
		t.Errorf("model = %q, want %q", doc.Get("model"), moduscfg.DefaultAssignment("librarian").Model)
	}

	// Verify body contains steps
	if !strings.Contains(doc.Body, "searched vault") {
		t.Error("body should contain step text")
	}
	if !strings.Contains(doc.Body, "vault_search") {
		t.Error("body should contain tool name")
	}
}

func TestSearchTraces(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "traces"), 0755)
	v := New(dir, nil)

	// Store two traces
	_, _ = v.StoreTrace("deploy frontend", "success", []string{"built", "deployed"}, 30.0, nil, "agent", "")
	_, _ = v.StoreTrace("fix auth bug", "failure", []string{"investigated", "could not reproduce"}, 60.0, nil, "agent", "")

	// Search for deploy
	matches, err := v.SearchTraces("deploy", 10)
	if err != nil {
		t.Fatalf("SearchTraces failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for 'deploy', got %d", len(matches))
	}
	if matches[0].Get("task") != "deploy frontend" {
		t.Errorf("matched wrong trace: %s", matches[0].Get("task"))
	}

	// Search for auth
	matches, err = v.SearchTraces("auth", 10)
	if err != nil {
		t.Fatalf("SearchTraces failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for 'auth', got %d", len(matches))
	}

	// Empty query returns all
	matches, err = v.SearchTraces("", 10)
	if err != nil {
		t.Fatalf("SearchTraces failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 traces for empty query, got %d", len(matches))
	}
}

func TestFormatTraceHints(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "traces"), 0755)
	v := New(dir, nil)

	// No traces
	hints := v.FormatTraceHints("anything")
	if hints != "" {
		t.Errorf("expected empty hints, got %q", hints)
	}

	// Add a trace
	_, _ = v.StoreTrace("build docker image", "success", nil, 45.0, nil, "agent", "")

	hints = v.FormatTraceHints("docker")
	if hints == "" {
		t.Error("expected non-empty trace hints")
	}
	if !strings.Contains(hints, "build docker image") {
		t.Error("hints should contain task name")
	}
}

func TestStoreTraceDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory", "traces"), 0755)
	v := New(dir, nil)

	path1, _ := v.StoreTrace("same task", "success", nil, 1.0, nil, "a", "")
	path2, _ := v.StoreTrace("same task", "failure", nil, 2.0, nil, "b", "")

	if path1 == path2 {
		t.Errorf("duplicate paths: %s and %s should differ", path1, path2)
	}
}

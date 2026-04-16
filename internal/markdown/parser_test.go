package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestParseFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "test.md", `---
title: Hello
score: 42
confidence: 0.85
---

Body content here.
`)

	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if doc.Get("title") != "Hello" {
		t.Errorf("title = %q, want Hello", doc.Get("title"))
	}
	if doc.GetFloat("score") != 42 {
		t.Errorf("score = %v, want 42", doc.GetFloat("score"))
	}
	if doc.GetFloat("confidence") != 0.85 {
		t.Errorf("confidence = %v, want 0.85", doc.GetFloat("confidence"))
	}
	if doc.Get("missing") != "" {
		t.Errorf("missing key should return empty string")
	}
	if doc.GetFloat("missing") != 0 {
		t.Errorf("missing float key should return 0")
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "plain.md", "# Just markdown\n\nNo frontmatter here.\n")

	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Frontmatter) != 0 {
		t.Errorf("expected empty frontmatter, got %v", doc.Frontmatter)
	}
	if doc.Body == "" {
		t.Error("body should not be empty")
	}
}

func TestParseEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "empty.md", "")

	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Body != "" {
		t.Errorf("body = %q, want empty", doc.Body)
	}
}

func TestParseNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/path.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSetAndGet(t *testing.T) {
	doc := &Document{Frontmatter: make(map[string]interface{})}
	doc.Set("key", "value")
	if doc.Get("key") != "value" {
		t.Errorf("Get after Set = %q, want value", doc.Get("key"))
	}

	// Set on nil frontmatter
	doc2 := &Document{}
	doc2.Set("key", "value")
	if doc2.Get("key") != "value" {
		t.Error("Set on nil frontmatter should initialize it")
	}
}

func TestGetTags(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "tags.md", `---
tags: [go, testing, vault]
---

Tagged doc.
`)

	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tags := doc.GetTags()
	if len(tags) != 3 {
		t.Fatalf("tags = %v, want 3 items", tags)
	}
	if tags[0] != "go" || tags[1] != "testing" || tags[2] != "vault" {
		t.Errorf("tags = %v", tags)
	}
}

func TestGetTagsString(t *testing.T) {
	doc := &Document{Frontmatter: map[string]interface{}{
		"tags": "a,b,c",
	}}
	tags := doc.GetTags()
	if len(tags) != 3 {
		t.Errorf("string tags = %v, want 3 items", tags)
	}
}

func TestGetTagsMissing(t *testing.T) {
	doc := &Document{Frontmatter: map[string]interface{}{}}
	tags := doc.GetTags()
	if tags != nil {
		t.Errorf("missing tags should return nil, got %v", tags)
	}
}

func TestWikiLinks(t *testing.T) {
	doc := &Document{
		Body: "See [[entity-go]] and [[belief-memory]] for details. Also [[mission-ship]].",
	}
	links := doc.WikiLinks()
	if len(links) != 3 {
		t.Fatalf("WikiLinks = %v, want 3", links)
	}
	if links[0] != "entity-go" || links[1] != "belief-memory" || links[2] != "mission-ship" {
		t.Errorf("links = %v", links)
	}
}

func TestWikiLinksStripsLabelAndAnchor(t *testing.T) {
	doc := &Document{
		Body: "See [[Agent Planning|planning dossier]] and [[Claude Code#Open Questions|Claude]].",
	}
	links := doc.WikiLinks()
	if len(links) != 2 {
		t.Fatalf("WikiLinks = %v, want 2", links)
	}
	if links[0] != "Agent Planning" || links[1] != "Claude Code" {
		t.Errorf("links = %v", links)
	}
}

func TestWikiLinksNormalizesAbsolutePathTargets(t *testing.T) {
	doc := &Document{
		Body: "See [[ /Users/modus/modus/vault/knowledge/compiled/demo/topic.md#Section|Harness As Product ]].",
	}
	links := doc.WikiLinks()
	if len(links) != 1 {
		t.Fatalf("WikiLinks = %v, want 1", links)
	}
	if links[0] != "/Users/modus/modus/vault/knowledge/compiled/demo/topic.md" {
		t.Errorf("links = %v", links)
	}
}

func TestWikiLinksNone(t *testing.T) {
	doc := &Document{Body: "No links here."}
	if len(doc.WikiLinks()) != 0 {
		t.Error("expected no wiki links")
	}
}

func TestWikiLinksIgnoreCode(t *testing.T) {
	doc := &Document{
		Body: strings.Join([]string{
			"Real link [[Agent Planning]].",
			"",
			"`inline [[Not A Link]] code`",
			"",
			"```python",
			"matrix = [[1, 2], [3, 4]]",
			"```",
		}, "\n"),
	}
	links := doc.WikiLinks()
	if len(links) != 1 {
		t.Fatalf("WikiLinks = %v, want 1", links)
	}
	if links[0] != "Agent Planning" {
		t.Errorf("links = %v", links)
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "save.md", `---
title: original
---

Original body.
`)

	doc, _ := Parse(path)
	doc.Set("title", "updated")
	doc.Body = "Updated body."

	if err := doc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reread, _ := Parse(path)
	if reread.Get("title") != "updated" {
		t.Errorf("title after save = %q, want updated", reread.Get("title"))
	}
}

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.md", "---\ntitle: A\n---\nA")
	writeTestFile(t, dir, "sub/b.md", "---\ntitle: B\n---\nB")
	writeTestFile(t, dir, "not-md.txt", "ignored")
	writeTestFile(t, dir, "discard/c.md", "---\ntitle: C\n---\nSkipped")

	docs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("ScanDir returned %d docs, want 2 (a.md + sub/b.md, not .txt or discard/)", len(docs))
	}
}

func TestScanDirEmpty(t *testing.T) {
	dir := t.TempDir()
	docs, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir empty: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs in empty dir, got %d", len(docs))
	}
}

// --- Writer ---

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "write-test.md")

	fm := map[string]interface{}{
		"title":      "Test",
		"score":      42,
		"confidence": 0.85,
		"active":     true,
	}

	err := Write(path, fm, "Hello world")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse written file: %v", err)
	}
	if doc.Get("title") != "Test" {
		t.Errorf("title = %q", doc.Get("title"))
	}
	if doc.GetFloat("score") != 42 {
		t.Errorf("score = %v", doc.GetFloat("score"))
	}
}

func TestWriteCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "dir", "file.md")

	err := Write(path, map[string]interface{}{"title": "deep"}, "body")
	if err != nil {
		t.Fatalf("Write with nested dir: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist at %s", path)
	}
}

func TestWriteStringSlice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slice.md")

	fm := map[string]interface{}{
		"tags": []string{"a", "b", "c"},
	}
	Write(path, fm, "body")

	data, _ := os.ReadFile(path)
	content := string(data)
	if !contains(content, "[a, b, c]") {
		t.Errorf("expected [a, b, c] in output, got:\n%s", content)
	}
}

func TestWriteNestedMaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested.md")

	fm := map[string]interface{}{
		"title": "test",
		"dependencies": []interface{}{
			map[string]interface{}{"slug": "backend", "type": "blocks"},
			map[string]interface{}{"slug": "auth", "type": "informs"},
		},
	}

	err := Write(path, fm, "body")
	if err != nil {
		t.Fatalf("Write nested maps: %v", err)
	}

	// Round-trip: parse it back and verify
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse nested: %v", err)
	}

	deps, ok := doc.Frontmatter["dependencies"]
	if !ok {
		t.Fatal("dependencies missing after round-trip")
	}

	arr, ok := deps.([]interface{})
	if !ok {
		t.Fatalf("dependencies type = %T, want []interface{}", deps)
	}
	if len(arr) != 2 {
		t.Fatalf("dependencies len = %d, want 2", len(arr))
	}

	first, ok := arr[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first dep type = %T, want map[string]interface{}", arr[0])
	}
	if first["slug"] != "backend" {
		t.Errorf("first dep slug = %v, want backend", first["slug"])
	}
	if first["type"] != "blocks" {
		t.Errorf("first dep type = %v, want blocks", first["type"])
	}
}

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"simple", false},
		{"has:colon", true},
		{"has#hash", true},
		{"[bracket]", true},
		{"plain text", false},
	}
	for _, tt := range tests {
		got := needsQuoting(tt.input)
		if got != tt.want {
			t.Errorf("needsQuoting(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

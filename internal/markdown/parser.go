package markdown

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document represents a parsed markdown file with YAML frontmatter.
type Document struct {
	Path        string                 // filesystem path
	Frontmatter map[string]interface{} // parsed YAML
	Body        string                 // markdown content after frontmatter
}

// Get returns a frontmatter value as a string, or empty string.
func (d *Document) Get(key string) string {
	v, ok := d.Frontmatter[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// GetFloat returns a frontmatter value as float64, or 0.
func (d *Document) GetFloat(key string) float64 {
	v, ok := d.Frontmatter[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0
	}
}

// GetTags returns the tags field as a string slice.
func (d *Document) GetTags() []string {
	v, ok := d.Frontmatter["tags"]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		tags := make([]string, 0, len(val))
		for _, t := range val {
			tags = append(tags, fmt.Sprintf("%v", t))
		}
		return tags
	case string:
		return strings.Split(val, ",")
	default:
		return nil
	}
}

// Set updates a frontmatter value.
func (d *Document) Set(key string, value interface{}) {
	if d.Frontmatter == nil {
		d.Frontmatter = make(map[string]interface{})
	}
	d.Frontmatter[key] = value
}

// Save writes the document back to disk with updated frontmatter.
func (d *Document) Save() error {
	return Write(d.Path, d.Frontmatter, d.Body)
}

// WikiLinks extracts all [[link]] references from the body.
func (d *Document) WikiLinks() []string {
	var links []string
	walkWikiLinks(d.Body, func(raw string) {
		link := normalizeWikiLink(raw)
		if link != "" {
			links = append(links, link)
		}
	})
	return links
}

func normalizeWikiLink(raw string) string {
	link := strings.TrimSpace(raw)
	if link == "" {
		return ""
	}
	if pipe := strings.Index(link, "|"); pipe >= 0 {
		link = strings.TrimSpace(link[:pipe])
	}
	if anchor := strings.Index(link, "#"); anchor >= 0 {
		link = strings.TrimSpace(link[:anchor])
	}
	return link
}

func walkWikiLinks(body string, fn func(raw string)) {
	inInlineCode := false
	inFence := false
	fenceMarker := ""
	lineStart := true

	for i := 0; i < len(body); {
		if lineStart && !inInlineCode {
			if strings.HasPrefix(body[i:], "```") {
				if inFence && fenceMarker == "```" {
					inFence = false
					fenceMarker = ""
				} else if !inFence {
					inFence = true
					fenceMarker = "```"
				}
				i += 3
				lineStart = false
				continue
			}
			if strings.HasPrefix(body[i:], "~~~") {
				if inFence && fenceMarker == "~~~" {
					inFence = false
					fenceMarker = ""
				} else if !inFence {
					inFence = true
					fenceMarker = "~~~"
				}
				i += 3
				lineStart = false
				continue
			}
		}

		if body[i] == '\n' {
			lineStart = true
			i++
			continue
		}

		if inFence {
			lineStart = false
			i++
			continue
		}

		if body[i] == '`' {
			inInlineCode = !inInlineCode
			lineStart = false
			i++
			continue
		}

		if !inInlineCode && strings.HasPrefix(body[i:], "[[") {
			close := strings.Index(body[i+2:], "]]")
			if close < 0 {
				return
			}
			fn(body[i+2 : i+2+close])
			i += close + 4
			lineStart = false
			continue
		}

		lineStart = false
		i++
	}
}

// Parse reads a markdown file and returns a Document.
func Parse(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	doc := &Document{
		Path:        path,
		Frontmatter: make(map[string]interface{}),
	}

	// Check for frontmatter
	if !scanner.Scan() {
		return doc, nil
	}
	firstLine := scanner.Text()
	if firstLine != "---" {
		// No frontmatter — entire file is body
		var body strings.Builder
		body.WriteString(firstLine)
		body.WriteByte('\n')
		for scanner.Scan() {
			body.WriteString(scanner.Text())
			body.WriteByte('\n')
		}
		doc.Body = body.String()
		return doc, nil
	}

	// Read frontmatter until closing ---
	var fmLines strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}
		fmLines.WriteString(line)
		fmLines.WriteByte('\n')
	}

	// Parse YAML
	if err := yaml.Unmarshal([]byte(fmLines.String()), &doc.Frontmatter); err != nil {
		// If YAML fails, treat entire content as body
		doc.Body = fmLines.String()
		return doc, nil
	}

	// Read body
	var body strings.Builder
	for scanner.Scan() {
		body.WriteString(scanner.Text())
		body.WriteByte('\n')
	}
	doc.Body = body.String()

	return doc, nil
}

// ScanDir recursively scans a directory for .md files and parses each.
func ScanDir(dir string) ([]*Document, error) {
	var docs []*Document

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		// Skip discard directories — cold storage, not indexed
		if info.IsDir() && info.Name() == "discard" {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		doc, err := Parse(path)
		if err != nil {
			return nil // skip unparseable files
		}
		docs = append(docs, doc)
		return nil
	})

	return docs, err
}

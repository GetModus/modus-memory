package markdown

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Write creates a markdown file with YAML frontmatter.
func Write(path string, frontmatter map[string]interface{}, body string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")

	for key, val := range frontmatter {
		if val == nil {
			continue
		}
		switch v := val.(type) {
		case []string:
			sb.WriteString(fmt.Sprintf("%s: [%s]\n", key, strings.Join(v, ", ")))
		case []interface{}:
			parts := make([]string, len(v))
			for i, item := range v {
				parts[i] = fmt.Sprintf("%v", item)
			}
			sb.WriteString(fmt.Sprintf("%s: [%s]\n", key, strings.Join(parts, ", ")))
		case bool:
			sb.WriteString(fmt.Sprintf("%s: %v\n", key, v))
		case float64, int:
			sb.WriteString(fmt.Sprintf("%s: %v\n", key, v))
		default:
			s := fmt.Sprintf("%v", v)
			if needsQuoting(s) {
				sb.WriteString(fmt.Sprintf("%s: %q\n", key, s))
			} else {
				sb.WriteString(fmt.Sprintf("%s: %s\n", key, s))
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(body))
	sb.WriteByte('\n')

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func needsQuoting(s string) bool {
	for _, c := range s {
		switch c {
		case ':', '#', '[', ']', '{', '}', '|', '>', '*', '&', '!', '%', '@', '`':
			return true
		}
	}
	return false
}

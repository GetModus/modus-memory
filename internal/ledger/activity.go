package ledger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NewOperationID creates a stable, machine-usable identifier for a consequential action lifecycle.
func NewOperationID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "op"
	}
	return prefix + "-" + time.Now().UTC().Format("20060102T150405.000000000Z07:00")
}

// Recent returns the newest ledger records that match the predicate.
func Recent(vaultDir string, limit int, keep func(Record) bool) ([]Record, error) {
	if limit <= 0 {
		limit = 10
	}
	path := filepath.Join(vaultDir, "state", "operations", "operations.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rows []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec Record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if keep != nil && !keep(rec) {
			continue
		}
		rows = append(rows, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Timestamp > rows[j].Timestamp
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

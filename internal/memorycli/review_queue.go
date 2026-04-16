package memorycli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

const ReviewQueueUsage = "review-queue [--status <pending|approved|rejected|applied|all>] [--limit <n>] [--json]"

type ReviewQueueItem struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Created     string `json:"created,omitempty"`
	FactPath    string `json:"fact_path,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Predicate   string `json:"predicate,omitempty"`
	ReviewClass string `json:"review_class,omitempty"`
}

type ReviewQueueSummary struct {
	GeneratedAt    string            `json:"generated_at"`
	VaultDir       string            `json:"vault_dir"`
	Statuses       []string          `json:"statuses"`
	Total          int               `json:"total"`
	CountsByType   map[string]int    `json:"counts_by_type"`
	CountsByStatus map[string]int    `json:"counts_by_status"`
	Items          []ReviewQueueItem `json:"items"`
}

type ReviewQueueResult struct {
	Summary  ReviewQueueSummary
	Rendered string
	JSON     bool
}

func ReviewQueue(vaultDir string, args []string) (ReviewQueueResult, error) {
	fs := flag.NewFlagSet("review-queue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	limit := fs.Int("limit", 20, "maximum number of queue items to print")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	var statuses stringListFlag
	fs.Var(&statuses, "status", "artifact status filter; may be repeated or comma-separated")

	if err := fs.Parse(args); err != nil {
		return ReviewQueueResult{}, err
	}

	filters := normalizeQueueStatuses(statuses.Values())
	if len(filters) == 0 {
		filters = []string{"pending"}
	}
	includeAll := len(filters) == 1 && filters[0] == "all"
	filterSet := make(map[string]bool, len(filters))
	for _, status := range filters {
		filterSet[status] = true
	}

	docs, err := markdown.ScanDir(filepath.Join(vaultDir, "memory", "maintenance"))
	if err != nil {
		return ReviewQueueResult{}, fmt.Errorf("scan maintenance artifacts: %w", err)
	}

	type sortableItem struct {
		item    ReviewQueueItem
		created time.Time
	}
	var items []sortableItem
	countsByType := map[string]int{}
	countsByStatus := map[string]int{}
	for _, doc := range docs {
		docType := strings.TrimSpace(doc.Get("type"))
		if !strings.HasPrefix(docType, "candidate_") {
			continue
		}
		status := firstNonEmpty(strings.TrimSpace(doc.Get("status")), "pending")
		if !includeAll && !filterSet[status] {
			continue
		}
		createdRaw := firstNonEmpty(strings.TrimSpace(doc.Get("created")), strings.TrimSpace(doc.Get("generated_at")))
		created := parseQueueTime(createdRaw)
		if !created.IsZero() {
			createdRaw = created.UTC().Format(time.RFC3339)
		}
		item := ReviewQueueItem{
			Path:        filepath.ToSlash(strings.TrimPrefix(doc.Path, vaultDir+"/")),
			Type:        docType,
			Status:      status,
			Created:     createdRaw,
			FactPath:    strings.TrimSpace(doc.Get("fact_path")),
			Subject:     strings.TrimSpace(doc.Get("subject")),
			Predicate:   strings.TrimSpace(doc.Get("predicate")),
			ReviewClass: strings.TrimSpace(doc.Get("review_class")),
		}
		items = append(items, sortableItem{item: item, created: created})
		countsByType[docType]++
		countsByStatus[status]++
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].created.Equal(items[j].created) {
			return items[i].item.Path < items[j].item.Path
		}
		return items[i].created.After(items[j].created)
	})

	maxItems := len(items)
	if *limit >= 0 && *limit < maxItems {
		maxItems = *limit
	}
	outItems := make([]ReviewQueueItem, 0, maxItems)
	for _, entry := range items[:maxItems] {
		outItems = append(outItems, entry.item)
	}

	summary := ReviewQueueSummary{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		VaultDir:       vaultDir,
		Statuses:       filters,
		Total:          len(items),
		CountsByType:   countsByType,
		CountsByStatus: countsByStatus,
		Items:          outItems,
	}
	return ReviewQueueResult{
		Summary:  summary,
		Rendered: renderReviewQueue(summary),
		JSON:     *jsonOut,
	}, nil
}

func renderReviewQueue(summary ReviewQueueSummary) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Review queue: %d artifact(s)", summary.Total))
	if len(summary.Statuses) > 0 {
		sb.WriteString(fmt.Sprintf(" [status=%s]", strings.Join(summary.Statuses, ",")))
	}
	sb.WriteString("\n")
	if len(summary.CountsByType) > 0 {
		sb.WriteString("By type:\n")
		for _, key := range sortedCountKeys(summary.CountsByType) {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", key, summary.CountsByType[key]))
		}
	}
	if len(summary.Items) == 0 {
		sb.WriteString("No matching review artifacts.\n")
		return sb.String()
	}
	sb.WriteString("Latest items:\n")
	for _, item := range summary.Items {
		label := firstNonEmpty(strings.TrimSpace(item.FactPath), strings.TrimSpace(item.Path))
		if item.Subject != "" || item.Predicate != "" {
			label = fmt.Sprintf("%s (%s / %s)", label, firstNonEmpty(item.Subject, "?"), firstNonEmpty(item.Predicate, "?"))
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s — %s\n", item.Status, item.Type, label))
	}
	return sb.String()
}

func MarshalReviewQueueJSON(summary ReviewQueueSummary) ([]byte, error) {
	return json.MarshalIndent(summary, "", "  ")
}

func normalizeQueueStatuses(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		switch value {
		case "pending", "approved", "rejected", "applied", "all":
		default:
			continue
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func parseQueueTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05 -0700 MST"} {
		if ts, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func sortedCountKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

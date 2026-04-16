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

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
)

const ResolveReviewUsage = "resolve-review [--status <pending|approved|rejected|applied|all>] [--type <candidate_...>] [--review-class <class>] [--fact-path <memory/facts/...>] --set-status <approved|rejected> --reason \"...\" [--limit <n>] [--json]"

type ResolveReviewItem struct {
	Path           string `json:"path"`
	Type           string `json:"type"`
	FactPath       string `json:"fact_path,omitempty"`
	Subject        string `json:"subject,omitempty"`
	Predicate      string `json:"predicate,omitempty"`
	ReviewClass    string `json:"review_class,omitempty"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
}

type ResolveReviewSummary struct {
	GeneratedAt    string              `json:"generated_at"`
	VaultDir       string              `json:"vault_dir"`
	Statuses       []string            `json:"statuses"`
	Types          []string            `json:"types,omitempty"`
	ReviewClasses  []string            `json:"review_classes,omitempty"`
	FactPaths      []string            `json:"fact_paths,omitempty"`
	SetStatus      string              `json:"set_status"`
	Reason         string              `json:"reason"`
	Matched        int                 `json:"matched"`
	Updated        int                 `json:"updated"`
	CountsByType   map[string]int      `json:"counts_by_type"`
	CountsByStatus map[string]int      `json:"counts_by_status"`
	Items          []ResolveReviewItem `json:"items"`
}

type ResolveReviewResult struct {
	Summary  ResolveReviewSummary
	Rendered string
	JSON     bool
}

func ResolveReview(vaultDir string, args []string) (ResolveReviewResult, error) {
	fs := flag.NewFlagSet("resolve-review", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var statuses stringListFlag
	var types stringListFlag
	var reviewClasses stringListFlag
	var factPaths stringListFlag
	setStatus := fs.String("set-status", "", "target artifact status: approved or rejected")
	reason := fs.String("reason", "", "why these artifacts are being resolved")
	limit := fs.Int("limit", 20, "maximum number of resolved items to print")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	fs.Var(&statuses, "status", "current artifact status filter; may be repeated or comma-separated")
	fs.Var(&types, "type", "artifact type filter; may be repeated or comma-separated")
	fs.Var(&reviewClasses, "review-class", "review class filter; may be repeated or comma-separated")
	fs.Var(&factPaths, "fact-path", "fact path filter; may be repeated or comma-separated")

	if err := fs.Parse(args); err != nil {
		return ResolveReviewResult{}, err
	}

	targetStatus := normalizeResolveTargetStatus(*setStatus)
	reasonText := strings.TrimSpace(*reason)
	if targetStatus == "" || reasonText == "" {
		return ResolveReviewResult{}, fmt.Errorf("usage: %s", ResolveReviewUsage)
	}

	statusFilters := normalizeQueueStatuses(statuses.Values())
	if len(statusFilters) == 0 {
		statusFilters = []string{"pending"}
	}
	includeAllStatuses := len(statusFilters) == 1 && statusFilters[0] == "all"
	statusSet := make(map[string]bool, len(statusFilters))
	for _, status := range statusFilters {
		statusSet[status] = true
	}

	typeFilters := normalizeNonEmptyList(types.Values())
	typeSet := make(map[string]bool, len(typeFilters))
	for _, value := range typeFilters {
		typeSet[value] = true
	}

	reviewClassFilters := normalizeLowerTrimmedList(reviewClasses.Values())
	reviewClassSet := make(map[string]bool, len(reviewClassFilters))
	for _, value := range reviewClassFilters {
		reviewClassSet[value] = true
	}

	factPathFilters := normalizeNonEmptyList(factPaths.Values())
	factPathSet := make(map[string]bool, len(factPathFilters))
	for _, value := range factPathFilters {
		factPathSet[value] = true
	}

	docs, err := markdown.ScanDir(filepath.Join(vaultDir, "memory", "maintenance"))
	if err != nil {
		return ResolveReviewResult{}, fmt.Errorf("scan maintenance artifacts: %w", err)
	}

	type sortableItem struct {
		item    ResolveReviewItem
		created time.Time
	}
	var items []sortableItem
	countsByType := map[string]int{}
	countsByStatus := map[string]int{}
	now := time.Now().UTC().Format(time.RFC3339)

	for _, doc := range docs {
		docType := strings.TrimSpace(doc.Get("type"))
		if !strings.HasPrefix(docType, "candidate_") {
			continue
		}
		if len(typeSet) > 0 && !typeSet[docType] {
			continue
		}

		currentStatus := firstNonEmpty(strings.TrimSpace(doc.Get("status")), "pending")
		if !includeAllStatuses && !statusSet[currentStatus] {
			continue
		}

		reviewClass := strings.ToLower(strings.TrimSpace(doc.Get("review_class")))
		if len(reviewClassSet) > 0 && !reviewClassSet[reviewClass] {
			continue
		}

		factPath := strings.TrimSpace(doc.Get("fact_path"))
		if len(factPathSet) > 0 && !factPathSet[factPath] {
			continue
		}

		createdRaw := firstNonEmpty(strings.TrimSpace(doc.Get("created")), strings.TrimSpace(doc.Get("generated_at")))
		created := parseQueueTime(createdRaw)
		doc.Set("status", targetStatus)
		doc.Set("review_resolution_at", now)
		doc.Set("review_resolution_reason", reasonText)
		doc.Set("review_resolution_actor", "memory_governance")
		doc.Set("review_resolution_mode", "operator_cli")
		if err := doc.Save(); err != nil {
			return ResolveReviewResult{}, fmt.Errorf("save %s: %w", doc.Path, err)
		}

		relPath := filepath.ToSlash(strings.TrimPrefix(doc.Path, vaultDir+"/"))
		if err := ledger.Append(vaultDir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "memory_cli_review_resolution",
			AuthorityScope: ledger.ScopeOperatorMemoryReview,
			ActionClass:    ledger.ActionReviewCandidateResolution,
			TargetDomain:   relPath,
			ResultStatus:   ledger.ResultCompleted,
			Decision:       ledger.DecisionApproved,
			SideEffects:    []string{"review_candidate_status_changed"},
			ProofRefs:      dedupeStringValues(relPath, factPath),
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "memory_cli_review_resolution",
				StaffingContext:    targetStatus,
				AuthorityScope:     ledger.ScopeOperatorMemoryReview,
				ArtifactState:      "evidentiary",
				SourceRefs:         dedupeStringValues(relPath, factPath),
				PromotionStatus:    targetStatus,
				ProofRef:           "review-resolution:" + relPath,
			},
			Metadata: map[string]interface{}{
				"type":            docType,
				"previous_status": currentStatus,
				"new_status":      targetStatus,
				"review_class":    reviewClass,
				"reason":          reasonText,
				"fact_path":       factPath,
			},
		}); err != nil {
			return ResolveReviewResult{}, fmt.Errorf("append ledger record for %s: %w", relPath, err)
		}

		item := ResolveReviewItem{
			Path:           relPath,
			Type:           docType,
			FactPath:       factPath,
			Subject:        strings.TrimSpace(doc.Get("subject")),
			Predicate:      strings.TrimSpace(doc.Get("predicate")),
			ReviewClass:    strings.TrimSpace(doc.Get("review_class")),
			PreviousStatus: currentStatus,
			NewStatus:      targetStatus,
		}
		items = append(items, sortableItem{item: item, created: created})
		countsByType[docType]++
		countsByStatus[targetStatus]++
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
	outItems := make([]ResolveReviewItem, 0, maxItems)
	for _, entry := range items[:maxItems] {
		outItems = append(outItems, entry.item)
	}

	summary := ResolveReviewSummary{
		GeneratedAt:    now,
		VaultDir:       vaultDir,
		Statuses:       statusFilters,
		Types:          typeFilters,
		ReviewClasses:  reviewClassFilters,
		FactPaths:      factPathFilters,
		SetStatus:      targetStatus,
		Reason:         reasonText,
		Matched:        len(items),
		Updated:        len(items),
		CountsByType:   countsByType,
		CountsByStatus: countsByStatus,
		Items:          outItems,
	}
	return ResolveReviewResult{
		Summary:  summary,
		Rendered: renderResolveReview(summary),
		JSON:     *jsonOut,
	}, nil
}

func renderResolveReview(summary ResolveReviewSummary) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Resolved %d review artifact(s) -> %s", summary.Updated, summary.SetStatus))
	if len(summary.Statuses) > 0 {
		sb.WriteString(fmt.Sprintf(" [from=%s]", strings.Join(summary.Statuses, ",")))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Reason: %s\n", summary.Reason))
	if len(summary.CountsByType) > 0 {
		sb.WriteString("By type:\n")
		for _, key := range sortedCountKeys(summary.CountsByType) {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", key, summary.CountsByType[key]))
		}
	}
	if len(summary.Items) == 0 {
		sb.WriteString("No matching review artifacts were updated.\n")
		return sb.String()
	}
	sb.WriteString("Latest resolved items:\n")
	for _, item := range summary.Items {
		label := firstNonEmpty(strings.TrimSpace(item.FactPath), strings.TrimSpace(item.Path))
		if item.Subject != "" || item.Predicate != "" {
			label = fmt.Sprintf("%s (%s / %s)", label, firstNonEmpty(item.Subject, "?"), firstNonEmpty(item.Predicate, "?"))
		}
		sb.WriteString(fmt.Sprintf("  [%s->%s] %s — %s\n", item.PreviousStatus, item.NewStatus, item.Type, label))
	}
	return sb.String()
}

func MarshalResolveReviewJSON(summary ResolveReviewSummary) ([]byte, error) {
	return json.MarshalIndent(summary, "", "  ")
}

func normalizeResolveTargetStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved":
		return "approved"
	case "rejected":
		return "rejected"
	default:
		return ""
	}
}

func normalizeNonEmptyList(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeLowerTrimmedList(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func dedupeStringValues(values ...string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

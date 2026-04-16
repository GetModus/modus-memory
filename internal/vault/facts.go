package vault

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trust"
)

// FSRS (Free Spaced Repetition Scheduler) parameters for memory decay.
// Dual-strength model: each fact has stability (how long until 90% recall drops)
// and difficulty (how hard it is to retain). Inspired by LACP's Mycelium Network
// and the FSRS-5 algorithm. Local adaptation: importance gates initial stability,
// memory type gates difficulty, and access-based reinforcement resets the clock.

// fsrsConfig holds per-importance FSRS parameters.
type fsrsConfig struct {
	InitialStability  float64 // days until R drops to 0.9 (S0)
	InitialDifficulty float64 // 0.0 (trivial) to 1.0 (very hard)
	Floor             float64 // minimum confidence (retrievability)
}

var fsrsConfigs = map[string]fsrsConfig{
	"critical": {InitialStability: 1e9, InitialDifficulty: 0, Floor: 1.0}, // never decays
	"high":     {InitialStability: 180, InitialDifficulty: 0.3, Floor: 0.3},
	"medium":   {InitialStability: 60, InitialDifficulty: 0.5, Floor: 0.1},
	"low":      {InitialStability: 14, InitialDifficulty: 0.7, Floor: 0.05},
}

const (
	HotMemoryAdmissionCap      = 12
	HotMemoryStaleReviewDays   = 30
	HotMemoryReviewArtifactDir = "memory/maintenance"
	ElderMemoryCap             = 24
	ElderMemoryStaleReviewDays = 365
)

// FactWriteAuthority describes the declared authority behind a canonical fact write.
type FactWriteAuthority struct {
	ProducingOffice       string
	ProducingSubsystem    string
	StaffingContext       string
	AuthorityScope        string
	TargetDomain          string
	Source                string
	SourceRef             string
	SourceRefs            []string
	ProofRef              string
	PromotionStatus       string
	MemoryTemperature     string
	AllowApproval         bool
	SourceEventID         string
	LineageID             string
	CueTerms              []string
	Mission               string
	WorkItemID            string
	Environment           string
	MemoryProtectionClass string
	MemorySecurityClass   string
	ObservedAt            string
	ValidFrom             string
	ValidTo               string
	TemporalStatus        string
	SupersedesPaths       []string
	RelatedFactPaths      []string
	RelatedEpisodePaths   []string
	RelatedEntityRefs     []string
	RelatedMissionRefs    []string
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeMemoryTemperature(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hot":
		return "hot"
	default:
		return "warm"
	}
}

func normalizeMemoryProtectionClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "elder":
		return "elder"
	default:
		return "ordinary"
	}
}

func normalizeMemorySecurityClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sealed":
		return "sealed"
	case "canonical":
		return "canonical"
	default:
		return "operational"
	}
}

func memorySecurityRank(value string) int {
	switch normalizeMemorySecurityClass(value) {
	case "sealed":
		return 3
	case "canonical":
		return 2
	default:
		return 1
	}
}

func deriveFactSecurityClass(importance, protectionClass, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return normalizeMemorySecurityClass(requested)
	}
	if normalizeMemoryProtectionClass(protectionClass) == "elder" {
		return "canonical"
	}
	if strings.EqualFold(strings.TrimSpace(importance), "critical") {
		return "canonical"
	}
	return "operational"
}

func effectiveFactSecurityClass(doc *markdown.Document) string {
	return deriveFactSecurityClass(doc.Get("importance"), doc.Get("memory_protection_class"), doc.Get("memory_security_class"))
}

func dedupeNonEmpty(values ...string) []string {
	seen := make(map[string]bool)
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

func normalizeRFC3339OrBlank(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	t, err := parseTime(value)
	if err != nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func normalizeTemporalStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "superseded":
		return "superseded"
	case "expired":
		return "expired"
	default:
		return "active"
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func importanceWeight(importance string) float64 {
	switch strings.ToLower(strings.TrimSpace(importance)) {
	case "critical":
		return 1.0
	case "high":
		return 0.8
	case "medium":
		return 0.5
	case "low":
		return 0.2
	default:
		return 0.4
	}
}

func temperatureWeight(temp string) float64 {
	switch normalizeMemoryTemperature(temp) {
	case "hot":
		return 1.0
	default:
		return 0.6
	}
}

func protectionWeight(class string) float64 {
	switch normalizeMemoryProtectionClass(class) {
	case "elder":
		return 1.0
	default:
		return 0.0
	}
}

func isElderProtected(doc *markdown.Document) bool {
	return normalizeMemoryProtectionClass(doc.Get("memory_protection_class")) == "elder"
}

func factTimestamp(doc *markdown.Document) string {
	return firstNonEmpty(doc.Get("observed_at"), doc.Get("valid_from"), doc.Get("created_at"), doc.Get("created"))
}

func factObservedTimestamp(doc *markdown.Document) string {
	return firstNonEmpty(doc.Get("observed_at"), doc.Get("created_at"), doc.Get("created"))
}

func factValidFromTimestamp(doc *markdown.Document) string {
	return firstNonEmpty(doc.Get("valid_from"), factObservedTimestamp(doc))
}

func factValidToTimestamp(doc *markdown.Document) string {
	return strings.TrimSpace(doc.Get("valid_to"))
}

func effectiveTemporalStatus(doc *markdown.Document) string {
	status := normalizeTemporalStatus(doc.Get("temporal_status"))
	if status == "active" {
		if validTo := factValidToTimestamp(doc); validTo != "" {
			if t, err := parseTime(validTo); err == nil && !t.After(time.Now()) {
				return "expired"
			}
		}
	}
	return status
}

func factFreshnessWeight(doc *markdown.Document) float64 {
	ts := factObservedTimestamp(doc)
	if ts == "" {
		return 0
	}
	t, err := parseTime(ts)
	if err != nil {
		return 0
	}
	age := time.Since(t)
	switch {
	case age <= 7*24*time.Hour:
		return 1.0
	case age <= 30*24*time.Hour:
		return 0.7
	case age <= 180*24*time.Hour:
		return 0.4
	default:
		return 0.1
	}
}

func factTemporalWeight(doc *markdown.Document) float64 {
	switch effectiveTemporalStatus(doc) {
	case "superseded":
		return -0.35
	case "expired":
		return -0.22
	default:
		return 0
	}
}

func formatValidityWindow(doc *markdown.Document) string {
	from := factValidFromTimestamp(doc)
	to := factValidToTimestamp(doc)
	switch {
	case from != "" && to != "":
		return fmt.Sprintf("valid %s to %s", from, to)
	case from != "":
		return fmt.Sprintf("valid from %s", from)
	case to != "":
		return fmt.Sprintf("valid until %s", to)
	default:
		return ""
	}
}

func factProvenanceWeight(doc *markdown.Document) float64 {
	score := 0.0
	if strings.TrimSpace(doc.Get("source")) != "" {
		score += 1
	}
	if strings.TrimSpace(doc.Get("source_ref")) != "" {
		score += 1
	}
	if strings.TrimSpace(doc.Get("captured_by_office")) != "" {
		score += 1
	}
	if strings.TrimSpace(doc.Get("captured_by_subsystem")) != "" {
		score += 1
	}
	if strings.TrimSpace(doc.Get("created_at")) != "" {
		score += 1
	}
	if strings.TrimSpace(doc.Get("memory_temperature")) != "" {
		score += 0.5
	}
	if isElderProtected(doc) {
		score += 0.5
	}
	if score > 5 {
		score = 5
	}
	return score / 5
}

func factSnippet(body, query string, limit int) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if limit <= 0 {
		limit = 200
	}
	if len(body) <= limit {
		return body
	}
	lowerBody := strings.ToLower(body)
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	if lowerQuery != "" {
		if idx := strings.Index(lowerBody, lowerQuery); idx >= 0 {
			start := maxInt(0, idx-(limit/3))
			end := minInt(len(body), start+limit)
			snippet := strings.TrimSpace(body[start:end])
			if start > 0 {
				snippet = "..." + snippet
			}
			if end < len(body) {
				snippet += "..."
			}
			return snippet
		}
	}
	return body[:limit-3] + "..."
}

func factBaseRankNoIndex(doc *markdown.Document, query string) float64 {
	if strings.TrimSpace(query) == "" {
		return 0.5
	}
	haystack := strings.ToLower(strings.Join([]string{
		doc.Get("subject"),
		doc.Get("predicate"),
		doc.Body,
	}, " "))
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return 0.5
	}
	matches := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	return float64(matches) / float64(len(terms))
}

type factSearchHit struct {
	RelPath string
	Doc     *markdown.Document
	Snippet string
	Score   float64
}

type FactSearchOptions struct {
	MemoryTemperature string
	VerificationMode  string
	RouteSubject      string
	RouteMission      string
	CapturedByOffice  string
	CueTerms          []string
	TimeBand          string
	WorkItemID        string
	LineageID         string
	Environment       string
}

type recallRoutePlan struct {
	Subjects    []string
	Missions    []string
	Office      string
	CueTerms    []string
	TimeBand    string
	WorkItemID  string
	LineageID   string
	Environment string
}

var hierarchyStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "for": true, "from": true,
	"how": true, "in": true, "is": true, "latest": true, "me": true, "my": true,
	"of": true, "on": true, "our": true, "recent": true, "the": true, "this": true,
	"to": true, "what": true, "which": true, "who": true, "why": true, "with": true,
}

func rankFactHit(doc *markdown.Document, baseRank float64) float64 {
	confidence := doc.GetFloat("confidence")
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return baseRank*0.5 +
		confidence*0.18 +
		importanceWeight(doc.Get("importance"))*0.12 +
		factProvenanceWeight(doc)*0.12 +
		protectionWeight(doc.Get("memory_protection_class"))*0.50 +
		temperatureWeight(doc.Get("memory_temperature"))*0.05 +
		factFreshnessWeight(doc)*0.03 +
		factTemporalWeight(doc)
}

func formatFactSearchHit(hit factSearchHit) string {
	subject := firstNonEmpty(hit.Doc.Get("subject"), hit.Doc.Get("title"), "fact")
	var facets []string
	if confidence := hit.Doc.GetFloat("confidence"); confidence > 0 {
		facets = append(facets, fmt.Sprintf("conf %.2f", confidence))
	}
	if importance := strings.TrimSpace(hit.Doc.Get("importance")); importance != "" {
		facets = append(facets, importance)
	}
	if isElderProtected(hit.Doc) {
		facets = append(facets, "elder")
	}
	if strings.EqualFold(strings.TrimSpace(hit.Doc.Get("correction_review_status")), "pending") || strings.EqualFold(strings.TrimSpace(hit.Doc.Get("stale_due_to_correction")), "true") {
		facets = append(facets, "correction review pending")
	}
	switch effectiveTemporalStatus(hit.Doc) {
	case "superseded":
		facets = append(facets, "superseded")
	case "expired":
		facets = append(facets, "expired")
	}
	facets = append(facets, normalizeMemoryTemperature(hit.Doc.Get("memory_temperature")))
	if validity := formatValidityWindow(hit.Doc); validity != "" {
		facets = append(facets, validity)
	}
	if supersededBy := strings.TrimSpace(hit.Doc.Get("superseded_by")); supersededBy != "" {
		facets = append(facets, "superseded_by "+supersededBy)
	}
	if count := len(docRelatedFactPaths(hit.Doc)); count > 0 {
		facets = append(facets, fmt.Sprintf("fact_links %d", count))
	}
	if count := len(docRelatedEpisodePaths(hit.Doc)); count > 0 {
		facets = append(facets, fmt.Sprintf("episode_links %d", count))
	}
	if count := len(docRelatedEntityRefs(hit.Doc)); count > 0 {
		facets = append(facets, fmt.Sprintf("entity_links %d", count))
	}
	if count := len(docRelatedMissionRefs(hit.Doc)); count > 0 {
		facets = append(facets, fmt.Sprintf("mission_links %d", count))
	}
	if source := firstNonEmpty(hit.Doc.Get("source"), hit.Doc.Get("source_ref")); source != "" {
		facets = append(facets, "source "+source)
	} else if office := strings.TrimSpace(hit.Doc.Get("captured_by_office")); office != "" {
		facets = append(facets, "captured_by "+office)
	}
	if len(facets) == 0 {
		return fmt.Sprintf("- **%s**: %s", subject, hit.Snippet)
	}
	return fmt.Sprintf("- **%s** [%s]: %s", subject, strings.Join(facets, ", "), hit.Snippet)
}

func docRelatedFactPaths(doc *markdown.Document) []string {
	return dedupeNonEmpty(stringSliceFrontmatter(doc.Frontmatter["related_fact_paths"])...)
}

func docRelatedEpisodePaths(doc *markdown.Document) []string {
	return dedupeNonEmpty(stringSliceFrontmatter(doc.Frontmatter["related_episode_paths"])...)
}

func docRelatedEntityRefs(doc *markdown.Document) []string {
	return dedupeNonEmpty(stringSliceFrontmatter(doc.Frontmatter["related_entity_refs"])...)
}

func docRelatedMissionRefs(doc *markdown.Document) []string {
	return dedupeNonEmpty(stringSliceFrontmatter(doc.Frontmatter["related_mission_refs"])...)
}

func hasExactFold(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func hasNormalizedMission(values []string, target string) bool {
	target = normalizeMissionKey(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if normalizeMissionKey(value) == target {
			return true
		}
	}
	return false
}

func matchesMemoryTemperature(doc *markdown.Document, wanted string) bool {
	wanted = normalizeMemoryTemperature(wanted)
	if strings.TrimSpace(wanted) == "" {
		return true
	}
	return normalizeMemoryTemperature(doc.Get("memory_temperature")) == wanted
}

func normalizeTimeBand(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "recent", "fresh", "current":
		return "recent"
	case "archive", "historical", "older", "past":
		return "archive"
	default:
		return ""
	}
}

func timeBandMatches(doc *markdown.Document, band string) bool {
	band = normalizeTimeBand(band)
	if band == "" {
		return true
	}
	ts := factTimestamp(doc)
	if ts == "" {
		return band != "recent"
	}
	t, err := parseTime(ts)
	if err != nil {
		return band != "recent"
	}
	age := time.Since(t)
	switch band {
	case "recent":
		return age <= 30*24*time.Hour
	case "archive":
		return age > 30*24*time.Hour
	default:
		return true
	}
}

func routeCueTerms(query string, explicit []string) []string {
	candidates := append([]string(nil), explicit...)
	for _, token := range strings.Fields(strings.ToLower(query)) {
		token = strings.TrimSpace(token)
		if len(token) <= 2 || hierarchyStopWords[token] {
			continue
		}
		candidates = append(candidates, token)
	}
	return normalizeCueTerms(candidates)
}

func deriveTimeBand(query, explicit string) string {
	if band := normalizeTimeBand(explicit); band != "" {
		return band
	}
	lower := strings.ToLower(query)
	switch {
	case strings.Contains(lower, "latest"), strings.Contains(lower, "recent"), strings.Contains(lower, "current"), strings.Contains(lower, "today"), strings.Contains(lower, "now"), strings.Contains(lower, "fresh"):
		return "recent"
	case strings.Contains(lower, "older"), strings.Contains(lower, "previous"), strings.Contains(lower, "past"), strings.Contains(lower, "history"), strings.Contains(lower, "historical"), strings.Contains(lower, "archive"):
		return "archive"
	default:
		return ""
	}
}

func uniqueSubjects(values []string) []string {
	return uniqueSorted(values)
}

func normalizeMissionKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func (v *Vault) subjectCandidates(query string) []string {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return nil
	}
	seen := make(map[string]bool)
	var subjects []string
	add := func(subject string) {
		subject = strings.TrimSpace(subject)
		if subject == "" || seen[strings.ToLower(subject)] {
			return
		}
		seen[strings.ToLower(subject)] = true
		subjects = append(subjects, subject)
	}

	if v.Index != nil {
		for _, fact := range v.Index.AllActiveFacts(0) {
			subject := strings.TrimSpace(fact.Subject)
			if subject == "" {
				continue
			}
			normalizedSubject := strings.ToLower(subject)
			if strings.Contains(normalizedQuery, normalizedSubject) || strings.Contains(normalizedSubject, normalizedQuery) {
				add(subject)
			}
		}
	} else {
		docs, err := markdown.ScanDir(v.Path("memory", "facts"))
		if err == nil {
			for _, doc := range docs {
				subject := strings.TrimSpace(doc.Get("subject"))
				if subject == "" {
					continue
				}
				normalizedSubject := strings.ToLower(subject)
				if strings.Contains(normalizedQuery, normalizedSubject) || strings.Contains(normalizedSubject, normalizedQuery) {
					add(subject)
				}
			}
		}
	}

	sort.SliceStable(subjects, func(i, j int) bool {
		return len(subjects[i]) > len(subjects[j])
	})
	if len(subjects) > 3 {
		subjects = subjects[:3]
	}
	return subjects
}

func (v *Vault) missionCandidates(query string) []string {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return nil
	}
	seen := make(map[string]bool)
	var missions []string
	add := func(mission string) {
		mission = strings.TrimSpace(mission)
		if mission == "" {
			return
		}
		key := normalizeMissionKey(mission)
		if seen[key] {
			return
		}
		seen[key] = true
		missions = append(missions, mission)
	}

	for _, subdir := range []string{"active", "completed"} {
		docs, err := markdown.ScanDir(v.Path("missions", subdir))
		if err != nil {
			continue
		}
		for _, doc := range docs {
			title := strings.TrimSpace(doc.Get("title"))
			slug := strings.TrimSuffix(filepath.Base(doc.Path), ".md")
			for _, candidate := range []string{title, slug} {
				if candidate == "" {
					continue
				}
				normalizedCandidate := normalizeMissionKey(candidate)
				if strings.Contains(normalizeMissionKey(normalizedQuery), normalizedCandidate) || strings.Contains(normalizedCandidate, normalizeMissionKey(normalizedQuery)) {
					add(firstNonEmpty(title, slug))
					break
				}
			}
		}
	}

	sort.SliceStable(missions, func(i, j int) bool {
		return len(missions[i]) > len(missions[j])
	})
	if len(missions) > 3 {
		missions = missions[:3]
	}
	return missions
}

func (v *Vault) buildRecallRoutePlan(query string, opts FactSearchOptions) recallRoutePlan {
	plan := recallRoutePlan{
		Office:      strings.TrimSpace(opts.CapturedByOffice),
		CueTerms:    routeCueTerms(query, opts.CueTerms),
		TimeBand:    deriveTimeBand(query, opts.TimeBand),
		WorkItemID:  strings.TrimSpace(opts.WorkItemID),
		LineageID:   strings.TrimSpace(opts.LineageID),
		Environment: strings.TrimSpace(opts.Environment),
	}
	if subject := strings.TrimSpace(opts.RouteSubject); subject != "" {
		plan.Subjects = []string{subject}
	} else {
		plan.Subjects = v.subjectCandidates(query)
	}
	if mission := strings.TrimSpace(opts.RouteMission); mission != "" {
		plan.Missions = []string{mission}
	} else {
		plan.Missions = v.missionCandidates(query)
	}
	return plan
}

func docCueTerms(doc *markdown.Document) []string {
	return normalizeCueTerms(stringSliceFrontmatter(doc.Frontmatter["cue_terms"]))
}

func routeScore(doc *markdown.Document, plan recallRoutePlan) float64 {
	score := 0.0
	subject := strings.TrimSpace(doc.Get("subject"))
	subjectLower := strings.ToLower(subject)
	for _, candidate := range plan.Subjects {
		candidateLower := strings.ToLower(strings.TrimSpace(candidate))
		if candidateLower != "" && subjectLower == candidateLower {
			score += 1.6
			break
		}
	}
	for _, candidate := range plan.Subjects {
		if hasExactFold(docRelatedEntityRefs(doc), candidate) {
			score += 1.05
			break
		}
	}
	docMission := normalizeMissionKey(doc.Get("mission"))
	for _, mission := range plan.Missions {
		if mission != "" && docMission != "" && docMission == normalizeMissionKey(mission) {
			score += 1.35
			break
		}
	}
	for _, mission := range plan.Missions {
		if hasNormalizedMission(docRelatedMissionRefs(doc), mission) {
			score += 0.9
			break
		}
	}
	if plan.WorkItemID != "" && strings.EqualFold(strings.TrimSpace(doc.Get("work_item_id")), plan.WorkItemID) {
		score += 1.25
	}
	if plan.LineageID != "" {
		docLineage := strings.TrimSpace(doc.Get("lineage_id"))
		docSourceEvent := strings.TrimSpace(doc.Get("source_event_id"))
		if strings.EqualFold(docLineage, plan.LineageID) || strings.EqualFold(docSourceEvent, plan.LineageID) {
			score += 1.15
		}
	}
	if plan.Environment != "" && strings.EqualFold(strings.TrimSpace(doc.Get("environment")), plan.Environment) {
		score += 0.8
	}
	if plan.Office != "" && strings.EqualFold(strings.TrimSpace(doc.Get("captured_by_office")), plan.Office) {
		score += 0.7
	}
	if len(plan.CueTerms) > 0 {
		docCues := docCueTerms(doc)
		for _, cue := range plan.CueTerms {
			for _, docCue := range docCues {
				if cue == docCue {
					score += 0.18
					break
				}
			}
		}
	}
	if plan.TimeBand != "" && timeBandMatches(doc, plan.TimeBand) {
		score += 0.45
	}
	return score
}

func hasRouteSelectors(plan recallRoutePlan) bool {
	return len(plan.Subjects) > 0 || len(plan.Missions) > 0 || plan.Office != "" || len(plan.CueTerms) > 0 || plan.TimeBand != "" || plan.WorkItemID != "" || plan.LineageID != "" || plan.Environment != ""
}

func (v *Vault) rankedFactHits(query string, limit int, opts FactSearchOptions) ([]factSearchHit, error) {
	if limit <= 0 {
		limit = 10
	}
	plan := v.buildRecallRoutePlan(query, opts)

	var routed []factSearchHit
	var fallback []factSearchHit
	if v.Index == nil {
		docs, err := markdown.ScanDir(v.Path("memory", "facts"))
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			if strings.TrimSpace(opts.MemoryTemperature) != "" && !matchesMemoryTemperature(doc, opts.MemoryTemperature) {
				continue
			}
			baseRank := factBaseRankNoIndex(doc, query)
			if strings.TrimSpace(query) != "" && baseRank == 0 {
				continue
			}
			relPath, _ := filepath.Rel(v.Dir, doc.Path)
			hit := factSearchHit{
				RelPath: relPath,
				Doc:     doc,
				Snippet: factSnippet(doc.Body, query, 200),
				Score:   rankFactHit(doc, baseRank),
			}
			if route := routeScore(doc, plan); route > 0 {
				hit.Score += route
				routed = append(routed, hit)
			} else {
				fallback = append(fallback, hit)
			}
		}
	} else {
		results, err := v.Index.Search(query, limit*4)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			if !strings.HasPrefix(result.Path, "memory/facts/") {
				continue
			}
			doc, err := v.Read(result.Path)
			if err != nil {
				continue
			}
			if strings.TrimSpace(opts.MemoryTemperature) != "" && !matchesMemoryTemperature(doc, opts.MemoryTemperature) {
				continue
			}
			hit := factSearchHit{
				RelPath: result.Path,
				Doc:     doc,
				Snippet: factSnippet(firstNonEmpty(result.Snippet, doc.Body), query, 200),
				Score:   rankFactHit(doc, result.Rank),
			}
			if route := routeScore(doc, plan); route > 0 {
				hit.Score += route
				routed = append(routed, hit)
			} else {
				fallback = append(fallback, hit)
			}
		}
	}

	sortHits := func(hits []factSearchHit) {
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].Score == hits[j].Score {
				return factTimestamp(hits[i].Doc) > factTimestamp(hits[j].Doc)
			}
			return hits[i].Score > hits[j].Score
		})
	}
	sortHits(routed)
	sortHits(fallback)

	var hits []factSearchHit
	if hasRouteSelectors(plan) {
		hits = append(hits, routed...)
		if len(hits) < limit {
			hits = append(hits, fallback...)
		}
	} else {
		hits = append(hits, fallback...)
	}

	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (v *Vault) factFrontmatter(subject, predicate string, confidence float64, importance string, auth *FactWriteAuthority) map[string]interface{} {
	if confidence <= 0 {
		confidence = 0.8
	}
	if importance == "" {
		importance = "medium"
	}

	now := time.Now().Format(time.RFC3339)
	temp := "warm"
	protectionClass := "ordinary"
	securityClass := deriveFactSecurityClass(importance, protectionClass, "")
	observedAt := ""
	validFrom := ""
	validTo := ""
	temporalStatus := "active"
	supersedesPaths := []string(nil)
	relatedFactPaths := []string(nil)
	relatedEpisodePaths := []string(nil)
	relatedEntityRefs := []string(nil)
	relatedMissionRefs := []string(nil)
	if auth != nil {
		temp = normalizeMemoryTemperature(auth.MemoryTemperature)
		protectionClass = normalizeMemoryProtectionClass(auth.MemoryProtectionClass)
		securityClass = deriveFactSecurityClass(importance, protectionClass, auth.MemorySecurityClass)
		observedAt = normalizeRFC3339OrBlank(auth.ObservedAt)
		validFrom = normalizeRFC3339OrBlank(auth.ValidFrom)
		validTo = normalizeRFC3339OrBlank(auth.ValidTo)
		temporalStatus = normalizeTemporalStatus(auth.TemporalStatus)
		supersedesPaths = dedupeNonEmpty(auth.SupersedesPaths...)
		relatedFactPaths = dedupeNonEmpty(auth.RelatedFactPaths...)
		relatedEpisodePaths = dedupeNonEmpty(auth.RelatedEpisodePaths...)
		relatedEntityRefs = dedupeNonEmpty(auth.RelatedEntityRefs...)
		relatedMissionRefs = dedupeNonEmpty(auth.RelatedMissionRefs...)
	}
	if observedAt == "" {
		observedAt = now
	}
	if validFrom == "" {
		validFrom = observedAt
	}
	if temporalStatus == "active" && validTo != "" {
		if t, err := parseTime(validTo); err == nil && !t.After(time.Now()) {
			temporalStatus = "expired"
		}
	}

	fm := map[string]interface{}{
		"subject":                 subject,
		"predicate":               predicate,
		"confidence":              confidence,
		"importance":              importance,
		"memory_type":             "semantic",
		"memory_temperature":      temp,
		"memory_protection_class": protectionClass,
		"memory_security_class":   securityClass,
		"created":                 now,
		"created_at":              now,
		"observed_at":             observedAt,
		"valid_from":              validFrom,
		"temporal_status":         temporalStatus,
	}
	if validTo != "" {
		fm["valid_to"] = validTo
	}
	if len(supersedesPaths) > 0 {
		fm["supersedes_paths"] = supersedesPaths
	}
	if len(relatedFactPaths) > 0 {
		fm["related_fact_paths"] = relatedFactPaths
	}
	if len(relatedEpisodePaths) > 0 {
		fm["related_episode_paths"] = relatedEpisodePaths
	}
	if len(relatedEntityRefs) > 0 {
		fm["related_entity_refs"] = relatedEntityRefs
	}
	if len(relatedMissionRefs) > 0 {
		fm["related_mission_refs"] = relatedMissionRefs
	}

	if auth == nil {
		return fm
	}

	if source := strings.TrimSpace(auth.Source); source != "" {
		fm["source"] = source
	}

	sourceRef := firstNonEmpty(auth.SourceRef)
	if sourceRef == "" && len(auth.SourceRefs) == 1 {
		sourceRef = auth.SourceRefs[0]
	}
	if sourceRef != "" {
		fm["source_ref"] = sourceRef
	}

	if lineage := dedupeNonEmpty(auth.SourceRefs...); len(lineage) > 0 {
		fm["source_lineage"] = lineage
	}
	if auth.ProducingOffice != "" {
		fm["captured_by_office"] = auth.ProducingOffice
	}
	if auth.ProducingSubsystem != "" {
		fm["captured_by_subsystem"] = auth.ProducingSubsystem
	}
	if auth.StaffingContext != "" {
		fm["staffing_context"] = auth.StaffingContext
	}
	if auth.PromotionStatus != "" {
		fm["promotion_status"] = auth.PromotionStatus
	}
	if auth.SourceEventID != "" {
		fm["source_event_id"] = auth.SourceEventID
	}
	if auth.LineageID != "" {
		fm["lineage_id"] = auth.LineageID
	}
	if cueTerms := normalizeCueTerms(auth.CueTerms); len(cueTerms) > 0 {
		fm["cue_terms"] = cueTerms
	}
	if mission := strings.TrimSpace(auth.Mission); mission != "" {
		fm["mission"] = mission
	}
	if workItemID := strings.TrimSpace(auth.WorkItemID); workItemID != "" {
		fm["work_item_id"] = workItemID
	}
	if environment := strings.TrimSpace(auth.Environment); environment != "" {
		fm["environment"] = environment
	}
	return fm
}

// Memory type difficulty modifiers. Procedural knowledge is hardest to forget,
// episodic is easiest (it's contextual and fades without reinforcement).
var memoryTypeDifficultyMod = map[string]float64{
	"semantic":   -0.1, // easier to retain (general knowledge)
	"episodic":   +0.2, // harder to retain (context-dependent)
	"procedural": -0.3, // hardest to forget (muscle memory analog)
}

// fsrsRetrievability computes R(t) = (1 + t/(9*S))^(-1)
// where t = elapsed days, S = stability. This is the FSRS power-law forgetting curve.
// R=0.9 when t=S (by definition of stability).
func fsrsRetrievability(elapsedDays, stability float64) float64 {
	if stability <= 0 {
		return 0
	}
	return math.Pow(1.0+elapsedDays/(9.0*stability), -1.0)
}

// fsrsNewStability computes updated stability after a successful recall.
// S' = S * (1 + e^(w) * (11-D) * S^(-0.2) * (e^(0.05*(1-R)) - 1))
// Simplified from FSRS-5. w=2.0 is the stability growth factor.
func fsrsNewStability(oldStability, difficulty, retrievability float64) float64 {
	w := 2.0             // growth factor — higher means faster stability growth on recall
	d := difficulty * 10 // scale to 0-10 range
	growth := math.Exp(w) * (11.0 - d) * math.Pow(oldStability, -0.2) * (math.Exp(0.05*(1.0-retrievability)) - 1.0)
	newS := oldStability * (1.0 + growth)
	if newS < oldStability {
		newS = oldStability // stability never decreases on recall
	}
	return newS
}

// DecayFacts sweeps all fact files and applies FSRS-based confidence decay.
// Confidence = retrievability R(t) = (1 + t/(9*S))^(-1), floored per importance.
// Returns the number of facts updated.
func (v *Vault) DecayFacts() (int, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, err
	}

	now := time.Now()
	updated := 0

	for _, doc := range docs {
		conf := doc.GetFloat("confidence")
		importance := doc.Get("importance")
		if importance == "" {
			importance = "medium"
		}

		cfg, ok := fsrsConfigs[importance]
		if !ok {
			cfg = fsrsConfigs["medium"]
		}

		// Critical facts never decay
		if cfg.InitialStability >= 1e8 {
			continue
		}
		if isElderProtected(doc) {
			continue
		}

		if conf <= cfg.Floor {
			continue
		}

		// Get or initialize stability
		stability := doc.GetFloat("stability")
		if stability <= 0 {
			stability = cfg.InitialStability
			// Apply memory type modifier to difficulty → affects initial stability
			memType := doc.Get("memory_type")
			if mod, ok := memoryTypeDifficultyMod[memType]; ok {
				adjustedDifficulty := cfg.InitialDifficulty + mod
				if adjustedDifficulty < 0 {
					adjustedDifficulty = 0
				}
				if adjustedDifficulty > 1.0 {
					adjustedDifficulty = 1.0
				}
				// Lower difficulty → higher stability
				stability = cfg.InitialStability * (1.0 + (0.5 - adjustedDifficulty))
			}
			doc.Set("stability", math.Round(stability*10)/10)
			doc.Set("difficulty", cfg.InitialDifficulty)
		}

		// Calculate days since last access or creation
		lastAccessed := doc.Get("last_accessed")
		if lastAccessed == "" {
			lastAccessed = doc.Get("last_decayed")
		}
		if lastAccessed == "" {
			lastAccessed = doc.Get("created")
		}
		if lastAccessed == "" {
			continue
		}

		t, err := parseTime(lastAccessed)
		if err != nil {
			continue
		}

		elapsedDays := now.Sub(t).Hours() / 24
		if elapsedDays < 0.5 {
			continue // too recent to decay
		}

		// FSRS retrievability: R(t) = (1 + t/(9*S))^(-1)
		newConf := fsrsRetrievability(elapsedDays, stability)
		newConf = math.Max(cfg.Floor, newConf)
		newConf = math.Round(newConf*1000) / 1000

		if newConf == conf {
			continue
		}

		doc.Set("confidence", newConf)
		doc.Set("last_decayed", now.Format(time.RFC3339))
		if err := doc.Save(); err != nil {
			continue
		}
		updated++
	}

	if updated > 0 {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "facts_decay",
			AuthorityScope: ledger.ScopeScheduledFactDecay,
			ActionClass:    ledger.ActionMemoryDecay,
			TargetDomain:   "memory/facts",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionAllowedWithProof,
			SideEffects:    []string{"fact_confidence_decayed"},
			ProofRefs:      []string{"memory/facts"},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "facts_decay",
				AuthorityScope:     ledger.ScopeScheduledFactDecay,
				ArtifactState:      "canonical",
				SourceRefs:         []string{"memory/facts"},
				PromotionStatus:    "advisory",
				ProofRef:           "facts-decay",
			},
			Metadata: map[string]interface{}{
				"updated_count": updated,
			},
		})
	}

	return updated, nil
}

// ReinforceFact increases a fact's confidence and stability after a successful recall.
// This is the FSRS "review" operation — accessing a fact proves it's still relevant,
// so stability grows and confidence resets toward 1.0.
func (v *Vault) ReinforceFact(relPath string) error {
	doc, err := v.Read(relPath)
	if err != nil {
		return err
	}

	now := time.Now()
	conf := doc.GetFloat("confidence")
	stability := doc.GetFloat("stability")
	difficulty := doc.GetFloat("difficulty")

	importance := doc.Get("importance")
	if importance == "" {
		importance = "medium"
	}
	cfg := fsrsConfigs[importance]

	// Initialize if missing
	if stability <= 0 {
		stability = cfg.InitialStability
	}
	if difficulty <= 0 {
		difficulty = cfg.InitialDifficulty
	}

	// Compute new stability: grows on each successful recall
	newStability := fsrsNewStability(stability, difficulty, conf)

	// Difficulty decreases slightly on successful recall (fact gets easier)
	newDifficulty := difficulty - 0.02
	if newDifficulty < 0.05 {
		newDifficulty = 0.05
	}

	// Confidence boost: asymptotic toward 1.0, small increment per access
	newConf := conf + (1.0-conf)*0.08
	if newConf > 0.99 {
		newConf = 0.99
	}

	// Track access count
	accessCount := 0
	if ac := doc.GetFloat("access_count"); ac > 0 {
		accessCount = int(ac)
	}
	accessCount++

	doc.Set("confidence", math.Round(newConf*1000)/1000)
	doc.Set("stability", math.Round(newStability*10)/10)
	doc.Set("difficulty", math.Round(newDifficulty*1000)/1000)
	doc.Set("last_accessed", now.Format(time.RFC3339))
	doc.Set("access_count", accessCount)

	return doc.Save()
}

// ArchiveStaleFacts marks facts below a confidence threshold as archived.
// Returns the number of facts archived.
func (v *Vault) ArchiveStaleFacts(threshold float64) (int, error) {
	if threshold <= 0 {
		threshold = 0.1
	}

	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return 0, err
	}

	archived := 0
	for _, doc := range docs {
		// Skip already archived
		if doc.Get("archived") == "true" {
			continue
		}
		// Skip critical facts
		if doc.Get("importance") == "critical" {
			continue
		}
		// Protected elder memory never archives silently.
		if isElderProtected(doc) {
			continue
		}

		conf := doc.GetFloat("confidence")
		if conf > 0 && conf < threshold {
			doc.Set("archived", true)
			doc.Set("archived_at", time.Now().Format(time.RFC3339))
			if err := doc.Save(); err != nil {
				continue
			}
			archived++
		}
	}

	if archived > 0 {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "facts_archive",
			AuthorityScope: ledger.ScopeScheduledFactArchival,
			ActionClass:    ledger.ActionMemoryArchival,
			TargetDomain:   "memory/facts",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionAllowedWithProof,
			SideEffects:    []string{"stale_facts_archived"},
			ProofRefs:      []string{"memory/facts"},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "facts_archive",
				AuthorityScope:     ledger.ScopeScheduledFactArchival,
				ArtifactState:      "canonical",
				SourceRefs:         []string{"memory/facts"},
				PromotionStatus:    "advisory",
				ProofRef:           "facts-archive",
			},
			Metadata: map[string]interface{}{
				"archived_count": archived,
				"threshold":      threshold,
			},
		})
	}

	return archived, nil
}

// TouchFact updates last_accessed on a fact, resetting its decay clock.
func (v *Vault) TouchFact(relPath string) error {
	doc, err := v.Read(relPath)
	if err != nil {
		return err
	}
	doc.Set("last_accessed", time.Now().Format(time.RFC3339))
	return doc.Save()
}

// ListFacts returns memory facts, optionally filtered by subject.
func (v *Vault) ListFacts(subject string, limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 20
	}

	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return nil, err
	}

	var result []*markdown.Document
	for _, doc := range docs {
		if len(result) >= limit {
			break
		}
		if subject != "" && !strings.EqualFold(doc.Get("subject"), subject) {
			continue
		}
		result = append(result, doc)
	}
	return result, nil
}

// SearchFacts searches memory facts via FTS, filtering to memory/facts/ paths.
// Falls back to listing all facts if no index is loaded.
func (v *Vault) SearchFacts(query string, limit int) ([]string, error) {
	return v.SearchFactsWithOptions(query, limit, FactSearchOptions{})
}

func (v *Vault) SearchFactsWithOptions(query string, limit int, opts FactSearchOptions) ([]string, error) {
	hits, err := v.rankedFactHits(query, limit, opts)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, hit := range hits {
		out = append(out, formatFactSearchHit(hit))
	}
	return out, nil
}

// StoreFact writes a new memory fact as a .md file.
func (v *Vault) StoreFact(subject, predicate, value string, confidence float64, importance string) (string, error) {
	if confidence <= 0 {
		confidence = 0.8
	}
	if importance == "" {
		importance = "medium"
	}

	slug := slugify(subject + "-" + predicate)
	if len(slug) > 80 {
		slug = slug[:80]
	}

	relPath := fmt.Sprintf("memory/facts/%s.md", slug)
	path := v.Path("memory", "facts", slug+".md")

	// Handle duplicates
	for i := 2; fileExists(path); i++ {
		slug2 := fmt.Sprintf("%s-%d", slug, i)
		relPath = fmt.Sprintf("memory/facts/%s.md", slug2)
		path = v.Path("memory", "facts", slug2+".md")
	}

	fm := v.factFrontmatter(subject, predicate, confidence, importance, nil)

	if err := markdown.Write(path, fm, value); err != nil {
		return "", err
	}
	if v.Index != nil {
		if refreshed, err := index.Build(v.Dir, ""); err == nil {
			v.Index.Close()
			v.Index = refreshed
		}
	}
	return relPath, nil
}

// StoreFactGoverned writes a canonical fact only after the trust gate has classified
// the declared authority and, when permitted, records the decision in the ledger.
func (v *Vault) StoreFactGoverned(subject, predicate, value string, confidence float64, importance string, auth FactWriteAuthority) (string, error) {
	decision, stage, err := trust.ClassifyAtCurrentStage(v.Dir, trust.Request{
		ProducingOffice:    auth.ProducingOffice,
		ProducingSubsystem: auth.ProducingSubsystem,
		ActionClass:        trust.ActionCanonicalMemoryMutation,
		TargetDomain:       firstNonEmpty(auth.TargetDomain, "memory/facts"),
		TouchedState:       []trust.StateClass{trust.StateKnowledge},
		HasPromotionPath:   auth.AllowApproval,
		RequestedAuthority: auth.AuthorityScope,
	})
	if err != nil {
		return "", err
	}
	if !trust.Permits(decision, auth.AllowApproval) {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:             auth.ProducingOffice,
			Subsystem:          auth.ProducingSubsystem,
			AuthorityScope:     auth.AuthorityScope,
			ActionClass:        string(trust.ActionCanonicalMemoryMutation),
			TargetDomain:       firstNonEmpty(auth.TargetDomain, "memory/facts"),
			ResultStatus:       ledger.ResultBlocked,
			Decision:           string(decision.Decision),
			SuggestedTransform: decision.SuggestedTransformation,
			Metadata: map[string]interface{}{
				"subject":          subject,
				"predicate":        predicate,
				"importance":       importance,
				"classifier_stage": stage,
				"reason":           decision.Reason,
			},
		})
		return "", fmt.Errorf("memory fact write blocked by trust gate: %s", decision.Reason)
	}

	if confidence <= 0 {
		confidence = 0.8
	}
	if importance == "" {
		importance = "medium"
	}

	slug := slugify(subject + "-" + predicate)
	if len(slug) > 80 {
		slug = slug[:80]
	}

	relPath := fmt.Sprintf("memory/facts/%s.md", slug)
	path := v.Path("memory", "facts", slug+".md")

	for i := 2; fileExists(path); i++ {
		slug2 := fmt.Sprintf("%s-%d", slug, i)
		relPath = fmt.Sprintf("memory/facts/%s.md", slug2)
		path = v.Path("memory", "facts", slug2+".md")
	}

	sourceRefs := append([]string{}, auth.SourceRefs...)
	if len(sourceRefs) == 0 && strings.TrimSpace(auth.SourceRef) != "" {
		sourceRefs = []string{auth.SourceRef}
	}
	if len(sourceRefs) == 1 && strings.TrimSpace(auth.SourceRef) == "" {
		auth.SourceRef = sourceRefs[0]
	}
	auth.SourceRefs = sourceRefs

	fm := v.factFrontmatter(subject, predicate, confidence, importance, &auth)
	if err := markdown.Write(path, fm, value); err != nil {
		return "", err
	}
	if v.Index != nil {
		if refreshed, err := index.Build(v.Dir, ""); err == nil {
			v.Index.Close()
			v.Index = refreshed
		}
	}
	if len(sourceRefs) == 0 {
		sourceRefs = []string{relPath}
	}

	_ = ledger.Append(v.Dir, ledger.Record{
		Office:         auth.ProducingOffice,
		Subsystem:      auth.ProducingSubsystem,
		AuthorityScope: auth.AuthorityScope,
		ActionClass:    ledger.ActionMemoryFactCreation,
		TargetDomain:   firstNonEmpty(auth.TargetDomain, "memory/facts"),
		ResultStatus:   ledger.ResultApplied,
		Decision:       string(decision.Decision),
		SideEffects:    []string{"memory_fact_created"},
		ProofRefs:      sourceRefs,
		Signature: signature.Signature{
			ProducingOffice:    auth.ProducingOffice,
			ProducingSubsystem: auth.ProducingSubsystem,
			StaffingContext:    auth.StaffingContext,
			AuthorityScope:     auth.AuthorityScope,
			ArtifactState:      "canonical",
			SourceRefs:         sourceRefs,
			PromotionStatus:    firstNonEmpty(auth.PromotionStatus, "approved"),
			ProofRef:           firstNonEmpty(auth.ProofRef, "memory-fact:"+relPath),
		},
		Metadata: map[string]interface{}{
			"subject":          subject,
			"predicate":        predicate,
			"importance":       importance,
			"confidence":       confidence,
			"rel_path":         relPath,
			"classifier_stage": stage,
			"trust_decision":   string(decision.Decision),
		},
	})
	return relPath, nil
}

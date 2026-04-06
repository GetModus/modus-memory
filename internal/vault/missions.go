package vault

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// MissionBoard returns missions grouped by status.
func (v *Vault) MissionBoard() map[string][]*markdown.Document {
	groups := map[string][]*markdown.Document{
		"active":    {},
		"planned":   {},
		"blocked":   {},
		"completed": {},
		"other":     {},
	}

	for _, subdir := range []string{"active", "completed"} {
		docs, err := markdown.ScanDir(v.Path("missions", subdir))
		if err != nil {
			continue
		}
		for _, doc := range docs {
			status := doc.Get("status")
			if _, ok := groups[status]; ok {
				groups[status] = append(groups[status], doc)
			} else {
				groups["other"] = append(groups["other"], doc)
			}
		}
	}
	return groups
}

// GetMission finds a mission by slug or title.
func (v *Vault) GetMission(slug string) (*markdown.Document, error) {
	slugLower := strings.ToLower(strings.ReplaceAll(slug, " ", "-"))

	for _, subdir := range []string{"active", "completed"} {
		docs, _ := markdown.ScanDir(v.Path("missions", subdir))
		for _, doc := range docs {
			base := strings.TrimSuffix(filepath.Base(doc.Path), ".md")
			if base == slugLower || strings.Contains(base, slugLower) ||
				strings.EqualFold(doc.Get("title"), slug) {
				return doc, nil
			}
		}
	}
	return nil, fmt.Errorf("mission %q not found", slug)
}

// ListMissions returns missions filtered by status.
func (v *Vault) ListMissions(status string, limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 30
	}

	var all []*markdown.Document
	for _, subdir := range []string{"active", "completed"} {
		docs, _ := markdown.ScanDir(v.Path("missions", subdir))
		all = append(all, docs...)
	}

	if status == "" {
		if len(all) > limit {
			return all[:limit], nil
		}
		return all, nil
	}

	var result []*markdown.Document
	for _, m := range all {
		if len(result) >= limit {
			break
		}
		if strings.EqualFold(m.Get("status"), status) {
			result = append(result, m)
		}
	}
	return result, nil
}

// CreateMission writes a new mission .md file.
func (v *Vault) CreateMission(title, description, priority string) (string, error) {
	if priority == "" {
		priority = "medium"
	}

	slug := slugify(title)
	relPath := fmt.Sprintf("missions/active/%s.md", slug)
	now := time.Now().Format("2006-01-02T15:04:05")

	fm := map[string]interface{}{
		"title":    title,
		"status":   "active",
		"priority": priority,
		"created":  now,
	}

	body := "# " + title + "\n\n" + description
	if err := v.Write(relPath, fm, body); err != nil {
		return "", err
	}
	return relPath, nil
}

// ShipClock returns the countdown to target.
func (v *Vault) ShipClock() (map[string]interface{}, error) {
	doc, err := markdown.Parse(v.Path("state", "ship-clock.md"))
	if err != nil {
		return nil, fmt.Errorf("ship clock not configured")
	}

	targetDate := doc.Get("target_date")
	targetARR := doc.Get("target_arr")

	daysLeft := 0
	t, err := time.Parse("2006-01-02", targetDate)
	if err == nil {
		daysLeft = int(math.Ceil(time.Until(t).Hours() / 24))
	}

	return map[string]interface{}{
		"target_date":    targetDate,
		"target_arr":     targetARR,
		"days_remaining": daysLeft,
		"status":         "tracking",
	}, nil
}

// ShipClockJSON returns ShipClock as formatted JSON.
func (v *Vault) ShipClockJSON() (string, error) {
	clock, err := v.ShipClock()
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(clock, "", "  ")
	return string(data), nil
}

// Dependency represents a typed relationship between missions.
type Dependency struct {
	Slug string `json:"slug"`
	Type string `json:"type"` // blocks, informs, enhances
}

// AddDependency adds a typed dependency to a mission's frontmatter.
// depType must be "blocks", "informs", or "enhances".
func (v *Vault) AddDependency(missionSlug, depSlug, depType string) error {
	if depType != "blocks" && depType != "informs" && depType != "enhances" {
		return fmt.Errorf("dependency type must be blocks, informs, or enhances (got %q)", depType)
	}

	// Verify both missions exist
	mission, err := v.GetMission(missionSlug)
	if err != nil {
		return fmt.Errorf("source mission: %w", err)
	}
	if _, err := v.GetMission(depSlug); err != nil {
		return fmt.Errorf("dependency mission: %w", err)
	}

	// Cycle detection for blocking deps
	if depType == "blocks" {
		if v.hasCycle(depSlug, missionSlug, 10) {
			return fmt.Errorf("adding this dependency would create a cycle")
		}
	}

	// Read existing dependencies
	deps := v.readDependencies(mission)

	// Check for duplicate
	for _, d := range deps {
		if d.Slug == depSlug {
			return fmt.Errorf("dependency on %q already exists", depSlug)
		}
	}

	deps = append(deps, Dependency{Slug: depSlug, Type: depType})
	v.writeDependencies(mission, deps)

	return mission.Save()
}

// RemoveDependency removes a dependency from a mission.
func (v *Vault) RemoveDependency(missionSlug, depSlug string) error {
	mission, err := v.GetMission(missionSlug)
	if err != nil {
		return err
	}

	deps := v.readDependencies(mission)
	var filtered []Dependency
	for _, d := range deps {
		if d.Slug != depSlug {
			filtered = append(filtered, d)
		}
	}

	v.writeDependencies(mission, filtered)
	return mission.Save()
}

// GetDependencies returns a mission's dependencies with satisfaction status.
func (v *Vault) GetDependencies(missionSlug string) ([]map[string]interface{}, error) {
	mission, err := v.GetMission(missionSlug)
	if err != nil {
		return nil, err
	}

	deps := v.readDependencies(mission)
	var result []map[string]interface{}

	for _, d := range deps {
		depMission, err := v.GetMission(d.Slug)
		satisfied := false
		depStatus := "unknown"
		if err == nil {
			depStatus = depMission.Get("status")
			satisfied = depStatus == "done" || depStatus == "archived" || depStatus == "completed"
		}
		result = append(result, map[string]interface{}{
			"slug":      d.Slug,
			"type":      d.Type,
			"status":    depStatus,
			"satisfied": satisfied,
		})
	}

	return result, nil
}

// CanStart checks if all blocking dependencies are satisfied.
// Returns (canStart, list of unsatisfied blockers).
func (v *Vault) CanStart(missionSlug string) (bool, []string, error) {
	deps, err := v.GetDependencies(missionSlug)
	if err != nil {
		return false, nil, err
	}

	var blockers []string
	for _, d := range deps {
		if d["type"] == "blocks" && !d["satisfied"].(bool) {
			blockers = append(blockers, d["slug"].(string))
		}
	}

	return len(blockers) == 0, blockers, nil
}

// CheckAutoCompletion checks if any group-archetype missions should auto-complete
// after the given mission was completed. Returns slugs of auto-completed missions.
func (v *Vault) CheckAutoCompletion(completedSlug string) ([]string, error) {
	allMissions, err := v.ListMissions("", 500)
	if err != nil {
		return nil, err
	}

	var autoCompleted []string
	for _, m := range allMissions {
		if m.Get("archetype") != "group" {
			continue
		}
		if m.Get("status") == "done" || m.Get("status") == "completed" || m.Get("status") == "archived" {
			continue
		}

		// Check if this group has a blocking dep on the completed mission
		deps := v.readDependencies(m)
		hasBlockingDep := false
		for _, d := range deps {
			if d.Slug == completedSlug && d.Type == "blocks" {
				hasBlockingDep = true
				break
			}
		}
		if !hasBlockingDep {
			continue
		}

		// Check if ALL blocking deps are now satisfied
		missionSlug := strings.TrimSuffix(filepath.Base(m.Path), ".md")
		canStart, _, _ := v.CanStart(missionSlug)
		if canStart {
			m.Set("status", "done")
			m.Set("completed_at", time.Now().Format(time.RFC3339))
			m.Set("auto_completed", true)
			m.Save()
			autoCompleted = append(autoCompleted, missionSlug)
		}
	}

	return autoCompleted, nil
}

// readDependencies extracts the dependencies array from a mission's frontmatter.
func (v *Vault) readDependencies(doc *markdown.Document) []Dependency {
	raw, ok := doc.Frontmatter["dependencies"]
	if !ok {
		return nil
	}

	// Handle []interface{} from YAML parsing
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	var deps []Dependency
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		slug, _ := m["slug"].(string)
		dtype, _ := m["type"].(string)
		if slug != "" {
			deps = append(deps, Dependency{Slug: slug, Type: dtype})
		}
	}
	return deps
}

// writeDependencies sets the dependencies array in a mission's frontmatter.
func (v *Vault) writeDependencies(doc *markdown.Document, deps []Dependency) {
	if len(deps) == 0 {
		delete(doc.Frontmatter, "dependencies")
		return
	}

	arr := make([]interface{}, len(deps))
	for i, d := range deps {
		arr[i] = map[string]interface{}{
			"slug": d.Slug,
			"type": d.Type,
		}
	}
	doc.Frontmatter["dependencies"] = arr
}

// hasCycle does DFS to check if adding missionSlug→depSlug would create a cycle.
func (v *Vault) hasCycle(fromSlug, targetSlug string, maxDepth int) bool {
	if maxDepth <= 0 || fromSlug == targetSlug {
		return fromSlug == targetSlug
	}

	mission, err := v.GetMission(fromSlug)
	if err != nil {
		return false
	}

	for _, d := range v.readDependencies(mission) {
		if d.Type == "blocks" && v.hasCycle(d.Slug, targetSlug, maxDepth-1) {
			return true
		}
	}
	return false
}

// ListBlueprints returns mission blueprints.
func (v *Vault) ListBlueprints(limit int) ([]*markdown.Document, error) {
	if limit <= 0 {
		limit = 20
	}
	docs, err := markdown.ScanDir(v.Path("missions", "blueprints"))
	if err != nil {
		return nil, err
	}
	if len(docs) > limit {
		return docs[:limit], nil
	}
	return docs, nil
}

package vault

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/markdown"
)

// FSRS (Free Spaced Repetition Scheduler) parameters for memory decay.
// Dual-strength model: each fact has stability (how long until 90% recall drops)
// and difficulty (how hard it is to retain). Inspired by LACP's Mycelium Network
// and the FSRS-5 algorithm. Local adaptation: importance gates initial stability,
// memory type gates difficulty, and access-based reinforcement resets the clock.

// fsrsConfig holds per-importance FSRS parameters.
type fsrsConfig struct {
	InitialStability float64 // days until R drops to 0.9 (S0)
	InitialDifficulty float64 // 0.0 (trivial) to 1.0 (very hard)
	Floor            float64 // minimum confidence (retrievability)
}

var fsrsConfigs = map[string]fsrsConfig{
	"critical": {InitialStability: 1e9, InitialDifficulty: 0, Floor: 1.0}, // never decays
	"high":     {InitialStability: 180, InitialDifficulty: 0.3, Floor: 0.3},
	"medium":   {InitialStability: 60, InitialDifficulty: 0.5, Floor: 0.1},
	"low":      {InitialStability: 14, InitialDifficulty: 0.7, Floor: 0.05},
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
	w := 2.0 // growth factor — higher means faster stability growth on recall
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
			doc.Set("stability", math.Round(stability*10) / 10)
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
	if limit <= 0 {
		limit = 10
	}

	// Fallback: scan directory if no index
	if v.Index == nil {
		docs, err := v.ListFacts("", limit)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, doc := range docs {
			subj := doc.Get("subject")
			val := doc.Body
			if len(val) > 200 {
				val = val[:197] + "..."
			}
			// Basic keyword match
			if query != "" && !strings.Contains(strings.ToLower(subj+val), strings.ToLower(query)) {
				continue
			}
			out = append(out, fmt.Sprintf("- **%s**: %s", subj, strings.TrimSpace(val)))
		}
		return out, nil
	}

	results, err := v.Index.Search(query, limit*3)
	if err != nil {
		return nil, err
	}

	var out []string
	count := 0
	for _, r := range results {
		if !strings.HasPrefix(r.Path, "memory/facts/") {
			continue
		}
		if count >= limit {
			break
		}
		out = append(out, fmt.Sprintf("- **%s**: %s", r.Subject, r.Snippet))
		count++
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

	fm := map[string]interface{}{
		"subject":     subject,
		"predicate":   predicate,
		"confidence":  confidence,
		"importance":  importance,
		"memory_type": "semantic",
		"created":     "now",
	}

	if err := v.Write(relPath, fm, value); err != nil {
		return "", err
	}
	return relPath, nil
}

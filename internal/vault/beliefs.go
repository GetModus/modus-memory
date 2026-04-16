package vault

import (
	"math"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
)

// Per-predicate decay rates (per day). Matches Python atlas/graph/constants.py.
var decayRates = map[string]float64{
	"is_a":         0.001,
	"created_by":   0.001,
	"member_of":    0.002,
	"contains":     0.005,
	"depends_on":   0.005,
	"uses":         0.01,
	"blocks":       0.03,
	"blocked_by":   0.03,
	"has_property": 0.015,
}

const (
	defaultDecayRate = 0.01
	confidenceFloor  = 0.05
	reinforceIndep   = 0.05
	reinforceSame    = 0.02
	weakenAmount     = 0.10
)

// DecayAllBeliefs sweeps all belief files in atlas/beliefs/ and applies
// per-predicate linear confidence decay. Returns the number of beliefs updated.
func (v *Vault) DecayAllBeliefs() (int, error) {
	docs, err := v.List("atlas/beliefs")
	if err != nil {
		return 0, err
	}

	now := time.Now()
	updated := 0

	for _, doc := range docs {
		conf := doc.GetFloat("confidence")
		if conf <= confidenceFloor {
			continue // Already at floor
		}

		predicate := doc.Get("predicate")
		rate := defaultDecayRate
		if r, ok := decayRates[predicate]; ok {
			rate = r
		}

		// Calculate days since last decay (or creation)
		lastDecayed := doc.Get("last_decayed")
		if lastDecayed == "" {
			lastDecayed = doc.Get("created")
		}
		if lastDecayed == "" {
			// No timestamp — skip this belief
			continue
		}

		t, err := parseTime(lastDecayed)
		if err != nil {
			continue
		}

		days := now.Sub(t).Hours() / 24
		if days < 1 {
			continue // Don't decay more than once per day
		}

		// Apply linear decay: confidence = max(floor, confidence - rate * days)
		newConf := math.Max(confidenceFloor, conf-rate*days)
		if newConf == conf {
			continue
		}

		doc.Set("confidence", math.Round(newConf*1000)/1000) // 3 decimal places
		doc.Set("last_decayed", now.Format(time.RFC3339))
		if err := doc.Save(); err != nil {
			continue
		}
		updated++
	}

	if updated > 0 {
		_ = ledger.Append(v.Dir, ledger.Record{
			Office:         "memory_governance",
			Subsystem:      "beliefs_decay",
			AuthorityScope: ledger.ScopeScheduledBeliefDecay,
			ActionClass:    ledger.ActionBeliefDecay,
			TargetDomain:   "atlas/beliefs",
			ResultStatus:   ledger.ResultApplied,
			Decision:       ledger.DecisionAllowedWithProof,
			SideEffects:    []string{"belief_confidence_decayed"},
			ProofRefs:      []string{"atlas/beliefs"},
			Signature: signature.Signature{
				ProducingOffice:    "memory_governance",
				ProducingSubsystem: "beliefs_decay",
				AuthorityScope:     ledger.ScopeScheduledBeliefDecay,
				ArtifactState:      "canonical",
				SourceRefs:         []string{"atlas/beliefs"},
				PromotionStatus:    "advisory",
				ProofRef:           "beliefs-decay",
			},
			Metadata: map[string]interface{}{
				"updated_count": updated,
			},
		})
	}

	return updated, nil
}

// ReinforceBelief boosts the confidence of a belief file.
// Independent source: +0.05, same source: +0.02. Capped at 1.0.
func (v *Vault) ReinforceBelief(relPath, source string) error {
	doc, err := v.Read(relPath)
	if err != nil {
		return err
	}

	conf := doc.GetFloat("confidence")
	existingSource := doc.Get("source")

	boost := reinforceIndep
	if source != "" && source == existingSource {
		boost = reinforceSame
	}

	newConf := math.Min(1.0, conf+boost)
	doc.Set("confidence", math.Round(newConf*1000)/1000)
	doc.Set("last_reinforced", time.Now().Format(time.RFC3339))

	return doc.Save()
}

// WeakenBelief reduces the confidence of a belief by 0.10, with a floor of 0.05.
func (v *Vault) WeakenBelief(relPath string) error {
	doc, err := v.Read(relPath)
	if err != nil {
		return err
	}

	conf := doc.GetFloat("confidence")
	newConf := math.Max(confidenceFloor, conf-weakenAmount)
	doc.Set("confidence", math.Round(newConf*1000)/1000)
	doc.Set("last_weakened", time.Now().Format(time.RFC3339))

	return doc.Save()
}

// GetBelief reads a single belief by relative path.
func (v *Vault) GetBelief(relPath string) (*markdown.Document, error) {
	return v.Read(relPath)
}

// parseTime tries common timestamp formats.
func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &time.ParseError{Value: s}
}

// Package trainer generates training data from vault activity and orchestrates
// offline adapter training. Per Codex revision: no self-certification, promotion
// is manual with a non-regression loss gate against the last promoted baseline.
package trainer

import (
	"fmt"
	"strings"

	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/vault"
)

// SFTPair is a supervised fine-tuning example in chat format.
type SFTPair struct {
	Messages []ChatMessage `json:"messages"`
	Source   string        `json:"source"` // provenance: "correction", "trace", "fact", "maintenance"
}

// ChatMessage is a role/content pair for training data.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DPOTriple is a direct preference optimization example.
type DPOTriple struct {
	Prompt   string `json:"prompt"`
	Chosen   string `json:"chosen"`
	Rejected string `json:"rejected"`
	Source   string `json:"source"`
}

// TrainingBatch holds all generated training pairs from a vault scan.
type TrainingBatch struct {
	SFT []SFTPair
	DPO []DPOTriple
}

// GenerateBatch orchestrates all signal miners and returns a combined batch.
func GenerateBatch(v *vault.Vault) (*TrainingBatch, error) {
	batch := &TrainingBatch{}

	// Mine from corrections (highest signal)
	corrSFT, err := MineCorrections(v)
	if err != nil {
		return nil, fmt.Errorf("mine corrections: %w", err)
	}
	batch.SFT = append(batch.SFT, corrSFT...)

	// Mine from successful traces
	traceSFT, err := MineTraces(v)
	if err != nil {
		return nil, fmt.Errorf("mine traces: %w", err)
	}
	batch.SFT = append(batch.SFT, traceSFT...)

	// Mine from high-confidence facts
	factSFT, err := MineFacts(v)
	if err != nil {
		return nil, fmt.Errorf("mine facts: %w", err)
	}
	batch.SFT = append(batch.SFT, factSFT...)

	// Mine from approved maintenance outcomes
	maintSFT, err := MineMaintenanceOutcomes(v)
	if err != nil {
		return nil, fmt.Errorf("mine maintenance: %w", err)
	}
	batch.SFT = append(batch.SFT, maintSFT...)

	// Mine DPO from reinforced vs decayed facts
	dpo, err := MineSearchSignals(v)
	if err != nil {
		return nil, fmt.Errorf("mine search signals: %w", err)
	}
	batch.DPO = append(batch.DPO, dpo...)

	return batch, nil
}

// MineCorrections generates SFT pairs from memory/corrections/.
// Each correction becomes a "given X, prefer Y" training example.
func MineCorrections(v *vault.Vault) ([]SFTPair, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "corrections"))
	if err != nil {
		return nil, nil // directory may not exist
	}

	var pairs []SFTPair
	for _, doc := range docs {
		original := doc.Get("original")
		corrected := doc.Get("corrected")
		context := doc.Get("context")
		if original == "" || corrected == "" {
			continue
		}

		userPrompt := fmt.Sprintf("Search the vault for: %s", original)
		assistantResp := fmt.Sprintf("Note: \"%s\" should be interpreted as \"%s\".", original, corrected)
		if context != "" {
			assistantResp += fmt.Sprintf(" Context: %s", context)
		}

		pairs = append(pairs, SFTPair{
			Messages: []ChatMessage{
				{Role: "system", Content: librarianSystemPrompt},
				{Role: "user", Content: userPrompt},
				{Role: "assistant", Content: assistantResp},
			},
			Source: "correction",
		})
	}
	return pairs, nil
}

// MineTraces generates SFT pairs from successful execution traces.
// Only mines traces with outcome "success" — failed traces are excluded.
func MineTraces(v *vault.Vault) ([]SFTPair, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "traces"))
	if err != nil {
		return nil, nil
	}

	var pairs []SFTPair
	for _, doc := range docs {
		if doc.Get("outcome") != "success" {
			continue // only train on successful outcomes
		}

		task := doc.Get("task")
		if task == "" {
			continue
		}

		userPrompt := fmt.Sprintf("How should I approach: %s", task)
		assistantResp := strings.TrimSpace(doc.Body)
		if assistantResp == "" {
			continue
		}

		// Truncate long traces
		if len(assistantResp) > 2000 {
			assistantResp = assistantResp[:2000]
		}

		pairs = append(pairs, SFTPair{
			Messages: []ChatMessage{
				{Role: "system", Content: librarianSystemPrompt},
				{Role: "user", Content: userPrompt},
				{Role: "assistant", Content: assistantResp},
			},
			Source: "trace",
		})
	}
	return pairs, nil
}

// MineFacts generates SFT pairs from high-confidence, frequently-accessed facts.
// Only uses facts with confidence >= 0.8 and access_count >= 2.
func MineFacts(v *vault.Vault) ([]SFTPair, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return nil, nil
	}

	var pairs []SFTPair
	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}

		conf := doc.GetFloat("confidence")
		accessCount := doc.GetFloat("access_count")
		if conf < 0.8 || accessCount < 2 {
			continue // only high-quality, validated facts
		}

		subject := doc.Get("subject")
		predicate := doc.Get("predicate")
		value := strings.TrimSpace(doc.Body)
		if subject == "" || value == "" {
			continue
		}

		userPrompt := fmt.Sprintf("What do you know about %s %s?", subject, predicate)
		pairs = append(pairs, SFTPair{
			Messages: []ChatMessage{
				{Role: "system", Content: librarianSystemPrompt},
				{Role: "user", Content: userPrompt},
				{Role: "assistant", Content: value},
			},
			Source: "fact",
		})
	}
	return pairs, nil
}

// MineMaintenanceOutcomes generates SFT from approved maintenance artifacts.
// Per Codex: only train on approved outcomes, not raw unreviewed proposals.
func MineMaintenanceOutcomes(v *vault.Vault) ([]SFTPair, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "maintenance"))
	if err != nil {
		return nil, nil
	}

	var pairs []SFTPair
	for _, doc := range docs {
		status := doc.Get("status")
		if status != "approved" && status != "resolved" {
			continue // only approved/resolved outcomes
		}

		docType := doc.Get("type")
		body := strings.TrimSpace(doc.Body)
		if body == "" {
			continue
		}

		var userPrompt string
		switch docType {
		case "candidate_merge":
			subj := doc.Get("stronger_subj")
			userPrompt = fmt.Sprintf("Should these facts about %q be merged?", subj)
		case "candidate_contradiction":
			subj := doc.Get("subject")
			pred := doc.Get("predicate")
			userPrompt = fmt.Sprintf("Which value is correct for %s %s?", subj, pred)
		case "candidate_bootstrap_fact":
			subj := doc.Get("subject")
			pred := doc.Get("predicate")
			userPrompt = fmt.Sprintf("Is this a valid fact: %s %s?", subj, pred)
		default:
			continue
		}

		if len(body) > 2000 {
			body = body[:2000]
		}

		pairs = append(pairs, SFTPair{
			Messages: []ChatMessage{
				{Role: "system", Content: librarianSystemPrompt},
				{Role: "user", Content: userPrompt},
				{Role: "assistant", Content: body},
			},
			Source: "maintenance",
		})
	}
	return pairs, nil
}

// MineSearchSignals generates DPO triples from reinforced vs decayed facts.
// Reinforced facts (high stability, high access) are "chosen".
// Decayed facts (low confidence, no access) are "rejected".
func MineSearchSignals(v *vault.Vault) ([]DPOTriple, error) {
	docs, err := markdown.ScanDir(v.Path("memory", "facts"))
	if err != nil {
		return nil, nil
	}

	var reinforced, decayed []*markdown.Document
	for _, doc := range docs {
		if doc.Get("archived") == "true" {
			continue
		}
		conf := doc.GetFloat("confidence")
		accessCount := doc.GetFloat("access_count")
		stability := doc.GetFloat("stability")

		if conf >= 0.8 && accessCount >= 3 && stability >= 100 {
			reinforced = append(reinforced, doc)
		} else if conf < 0.3 && accessCount == 0 {
			decayed = append(decayed, doc)
		}
	}

	var triples []DPOTriple
	// Pair reinforced with decayed facts on similar subjects
	for _, r := range reinforced {
		for _, d := range decayed {
			if !strings.EqualFold(r.Get("subject"), d.Get("subject")) {
				continue
			}
			prompt := fmt.Sprintf("What is %s %s?", r.Get("subject"), r.Get("predicate"))
			triples = append(triples, DPOTriple{
				Prompt:   prompt,
				Chosen:   strings.TrimSpace(r.Body),
				Rejected: strings.TrimSpace(d.Body),
				Source:   "search-signal",
			})
		}
	}

	return triples, nil
}

const librarianSystemPrompt = `You are a personal knowledge librarian. You manage a vault of facts, corrections, and traces. Answer precisely from stored knowledge. When uncertain, say so. Never fabricate facts.`

package trainer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

// WriteBatch writes SFT and DPO pairs to JSONL files in the output directory.
func WriteBatch(batch *TrainingBatch, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	if len(batch.SFT) > 0 {
		if err := writeJSONL(filepath.Join(outputDir, "sft_librarian.jsonl"), batch.SFT); err != nil {
			return fmt.Errorf("write SFT: %w", err)
		}
	}

	if len(batch.DPO) > 0 {
		if err := writeJSONL(filepath.Join(outputDir, "dpo_librarian.jsonl"), batch.DPO); err != nil {
			return fmt.Errorf("write DPO: %w", err)
		}
	}

	return nil
}

// Consolidate reads all SFT JSONL files in inputDir, deduplicates,
// and produces a 90/10 train/valid split under outputDir/mlx/.
func Consolidate(inputDir, outputDir string) (train, valid int, err error) {
	mlxDir := filepath.Join(outputDir, "mlx")
	if err := os.MkdirAll(mlxDir, 0755); err != nil {
		return 0, 0, err
	}

	// Read all SFT pairs
	sftPath := filepath.Join(inputDir, "sft_librarian.jsonl")
	pairs, err := readSFTJSONL(sftPath)
	if err != nil {
		return 0, 0, fmt.Errorf("read SFT: %w", err)
	}

	// Deduplicate by first user message content
	seen := make(map[string]bool)
	var unique []SFTPair
	for _, p := range pairs {
		key := ""
		for _, m := range p.Messages {
			if m.Role == "user" {
				key = m.Content
				break
			}
		}
		if key != "" && !seen[key] {
			seen[key] = true
			unique = append(unique, p)
		}
	}

	// Shuffle
	rand.Shuffle(len(unique), func(i, j int) {
		unique[i], unique[j] = unique[j], unique[i]
	})

	// 90/10 split
	splitIdx := len(unique) * 9 / 10
	if splitIdx == len(unique) && len(unique) > 1 {
		splitIdx = len(unique) - 1
	}

	trainSet := unique[:splitIdx]
	validSet := unique[splitIdx:]

	// Write as messages-format JSONL (compatible with mlx_lm.lora)
	if err := writeSFTJSONL(filepath.Join(mlxDir, "train.jsonl"), trainSet); err != nil {
		return 0, 0, err
	}
	if err := writeSFTJSONL(filepath.Join(mlxDir, "valid.jsonl"), validSet); err != nil {
		return 0, 0, err
	}

	return len(trainSet), len(validSet), nil
}

// MinPairsReached checks if there are enough training pairs to begin training.
// Threshold: 50 SFT pairs.
func MinPairsReached(inputDir string) bool {
	sftPath := filepath.Join(inputDir, "sft_librarian.jsonl")
	pairs, err := readSFTJSONL(sftPath)
	if err != nil {
		return false
	}
	return len(pairs) >= 50
}

// CountPairs returns the number of SFT and DPO pairs in the directory.
func CountPairs(dir string) (sft, dpo int) {
	if pairs, err := readSFTJSONL(filepath.Join(dir, "sft_librarian.jsonl")); err == nil {
		sft = len(pairs)
	}
	if f, err := os.Open(filepath.Join(dir, "dpo_librarian.jsonl")); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			dpo++
		}
		f.Close()
	}
	return
}

func writeJSONL(path string, items interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	switch v := items.(type) {
	case []SFTPair:
		for _, item := range v {
			// Write in mlx_lm format: {"messages": [...]}
			out := map[string]interface{}{
				"messages": item.Messages,
			}
			if err := enc.Encode(out); err != nil {
				return err
			}
		}
	case []DPOTriple:
		for _, item := range v {
			if err := enc.Encode(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeSFTJSONL(path string, pairs []SFTPair) error {
	return writeJSONL(path, pairs)
}

func readSFTJSONL(path string) ([]SFTPair, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var pairs []SFTPair
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Parse {"messages": [...]}
		var raw struct {
			Messages []ChatMessage `json:"messages"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		pairs = append(pairs, SFTPair{Messages: raw.Messages})
	}
	return pairs, scanner.Err()
}

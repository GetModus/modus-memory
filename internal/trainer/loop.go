package trainer

import (
	"fmt"
	"time"

	"github.com/GetModus/modus-memory/internal/vault"
)

// LoopResult holds the outcome of a full training pipeline run.
type LoopResult struct {
	SFTPairs    int
	DPOPairs    int
	TrainResult *TrainResult
	TrainRun    *TrainRun
	Skipped     bool
	SkipReason  string
}

// RunTrainingLoop executes the full offline training pipeline:
//  1. GenerateBatch — mine signals from vault
//  2. WriteBatch + Consolidate — produce train/valid splits
//  3. Check MinPairsReached (threshold: 50)
//  4. Train (subprocess, OOM-safe)
//  5. Log training run to memory/training-runs/
//
// Does NOT auto-promote. Promotion requires explicit PromoteRun call.
func RunTrainingLoop(v *vault.Vault, modelPath, outputDir string) (*LoopResult, error) {
	result := &LoopResult{}

	// Step 1: Generate training pairs
	batch, err := GenerateBatch(v)
	if err != nil {
		return nil, fmt.Errorf("generate batch: %w", err)
	}

	result.SFTPairs = len(batch.SFT)
	result.DPOPairs = len(batch.DPO)

	if len(batch.SFT) == 0 && len(batch.DPO) == 0 {
		result.Skipped = true
		result.SkipReason = "no training signals found in vault"
		return result, nil
	}

	// Step 2: Write and consolidate
	if err := WriteBatch(batch, outputDir); err != nil {
		return nil, fmt.Errorf("write batch: %w", err)
	}

	train, valid, err := Consolidate(outputDir, outputDir)
	if err != nil {
		return nil, fmt.Errorf("consolidate: %w", err)
	}

	// Step 3: Check minimum threshold
	if train+valid < 50 {
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("only %d pairs (need 50 minimum)", train+valid)
		return result, nil
	}

	// Step 4: Train
	cfg := DefaultTrainConfig()
	cfg.ModelPath = modelPath
	cfg.DataDir = outputDir

	trainResult, err := Train(cfg)
	if err != nil {
		return nil, fmt.Errorf("train: %w", err)
	}
	result.TrainResult = trainResult

	// Step 5: Log the run
	timestamp := time.Now().Format("2006-01-02-150405")
	run := &TrainRun{
		Timestamp:   timestamp,
		ModelBase:   modelPath,
		AdapterPath: trainResult.AdapterPath,
		SFTPairs:    result.SFTPairs,
		DPOPairs:    result.DPOPairs,
		TrainLoss:   trainResult.TrainLoss,
		ValLoss:     trainResult.ValLoss,
		DurationSec: trainResult.DurationSec,
	}
	result.TrainRun = run

	if err := LogTrainRun(v.Dir, run); err != nil {
		return result, fmt.Errorf("log training run: %w", err)
	}

	return result, nil
}

// FormatLoopResult renders a training loop result as human-readable text.
func FormatLoopResult(r *LoopResult) string {
	if r.Skipped {
		return fmt.Sprintf("Training skipped: %s\nSFT pairs: %d, DPO pairs: %d",
			r.SkipReason, r.SFTPairs, r.DPOPairs)
	}

	status := "FAILED"
	if r.TrainResult != nil && r.TrainResult.Success {
		status = "SUCCESS"
	}

	s := fmt.Sprintf("Training %s\nSFT pairs: %d, DPO pairs: %d\n",
		status, r.SFTPairs, r.DPOPairs)

	if r.TrainResult != nil {
		s += fmt.Sprintf("Duration: %.1fs\n", r.TrainResult.DurationSec)
		if r.TrainResult.TrainLoss > 0 {
			s += fmt.Sprintf("Train loss: %.4f\n", r.TrainResult.TrainLoss)
		}
		if r.TrainResult.ValLoss > 0 {
			s += fmt.Sprintf("Val loss: %.4f\n", r.TrainResult.ValLoss)
		}
		if r.TrainResult.AdapterPath != "" {
			s += fmt.Sprintf("Adapter: %s\n", r.TrainResult.AdapterPath)
		}
		if r.TrainResult.Error != "" {
			s += fmt.Sprintf("Error: %s\n", r.TrainResult.Error)
		}
	}

	if r.TrainRun != nil && !r.TrainRun.Promoted {
		s += "\nAdapter NOT promoted. Run `memory_train` with `promote: true` to promote.\n"
		s += "Promotion includes a non-regression loss gate against the last promoted baseline."
	}

	return s
}

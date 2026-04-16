package trainer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/trust"
)

// TrainConfig holds parameters for a training run on the model-training lane.
type TrainConfig struct {
	ModelPath  string // path to base GGUF or MLX model
	DataDir    string // directory containing mlx/train.jsonl + mlx/valid.jsonl
	AdapterDir string // output directory for adapter weights
	LR         float64
	Rank       int
	Iters      int
	BatchSize  int
	NumLayers  int
	MaxSeqLen  int
}

// DefaultTrainConfig returns sensible defaults for librarian fine-tuning.
func DefaultTrainConfig() TrainConfig {
	return TrainConfig{
		LR:        1e-5,
		Rank:      16,
		Iters:     1000,
		BatchSize: 1,
		NumLayers: 16,
		MaxSeqLen: 2048,
	}
}

// TrainResult holds the outcome of a training run.
type TrainResult struct {
	Success     bool
	AdapterPath string
	TrainLoss   float64
	ValLoss     float64
	DurationSec float64
	Command     string
	Stdout      string
	Error       string
}

// TrainRun is a persisted record of a training run.
type TrainRun struct {
	Timestamp     string  `json:"timestamp"`
	ModelBase     string  `json:"model_base"`
	AdapterPath   string  `json:"adapter_path"`
	SFTPairs      int     `json:"sft_pairs"`
	DPOPairs      int     `json:"dpo_pairs"`
	TrainLoss     float64 `json:"train_loss"`
	ValLoss       float64 `json:"val_loss"`
	DurationSec   float64 `json:"duration_sec"`
	Promoted      bool    `json:"promoted"`
	PromotedAt    string  `json:"promoted_at,omitempty"`
	PromotionNote string  `json:"promotion_note,omitempty"`
}

// Train executes training as a subprocess (OOM isolation).
// Primary training lane: mlx_lm.lora (macOS Apple Silicon).
// This is separate from the commissioned serving runtime, which may use Ollama.
// Returns result with parsed metrics.
func Train(cfg TrainConfig) (*TrainResult, error) {
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("model path required")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data directory required")
	}
	if cfg.AdapterDir == "" {
		cfg.AdapterDir = filepath.Join(cfg.DataDir, "adapters")
	}

	os.MkdirAll(cfg.AdapterDir, 0755)

	// Build mlx_lm command
	args := []string{
		"-m", "mlx_lm", "lora",
		"--model", cfg.ModelPath,
		"--data", filepath.Join(cfg.DataDir, "mlx"),
		"--adapter-path", cfg.AdapterDir,
		"--train",
		"--batch-size", strconv.Itoa(cfg.BatchSize),
		"--iters", strconv.Itoa(cfg.Iters),
		"--learning-rate", fmt.Sprintf("%.1e", cfg.LR),
		"--num-layers", strconv.Itoa(cfg.NumLayers),
		"--max-seq-length", strconv.Itoa(cfg.MaxSeqLen),
		"--mask-prompt",
		"--steps-per-report", "50",
		"--steps-per-eval", "200",
		"--save-every", "200",
	}

	// Find Python
	pythonPath := findPython()
	cmd := exec.Command(pythonPath, args...)
	cmd.Dir = cfg.DataDir

	start := time.Now()

	// Capture output
	output, err := cmd.CombinedOutput()
	duration := time.Since(start).Seconds()

	result := &TrainResult{
		Command:     fmt.Sprintf("%s %s", pythonPath, strings.Join(args, " ")),
		DurationSec: duration,
		Stdout:      string(output),
	}

	if err != nil {
		result.Error = err.Error()
		result.Success = false
		return result, nil // not a Go error — training failed
	}

	result.Success = true
	result.AdapterPath = cfg.AdapterDir

	// Parse loss from output
	result.TrainLoss = parseLastLoss(string(output), "Train")
	result.ValLoss = parseLastLoss(string(output), "Val")

	return result, nil
}

// LogTrainRun writes a training run record to memory/training-runs/.
func LogTrainRun(vaultDir string, run *TrainRun) error {
	runDir := filepath.Join(vaultDir, "memory", "training-runs")
	os.MkdirAll(runDir, 0755)

	path := filepath.Join(runDir, run.Timestamp+".json")
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ListTrainRuns reads all training run records.
func ListTrainRuns(vaultDir string) ([]*TrainRun, error) {
	runDir := filepath.Join(vaultDir, "memory", "training-runs")
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, nil // directory may not exist
	}

	var runs []*TrainRun
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(runDir, entry.Name()))
		if err != nil {
			continue
		}
		var run TrainRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		runs = append(runs, &run)
	}
	return runs, nil
}

// BestUnpromotedRun returns the training run with the lowest validation loss
// that hasn't been promoted yet, or nil if none exist.
func BestUnpromotedRun(vaultDir string) *TrainRun {
	runs, _ := ListTrainRuns(vaultDir)
	var best *TrainRun
	for _, r := range runs {
		if r.Promoted || r.ValLoss <= 0 {
			continue
		}
		if best == nil || r.ValLoss < best.ValLoss {
			best = r
		}
	}
	return best
}

// LastPromotedRun returns the most recently promoted run, or nil.
func LastPromotedRun(vaultDir string) *TrainRun {
	runs, _ := ListTrainRuns(vaultDir)
	var last *TrainRun
	for _, r := range runs {
		if !r.Promoted {
			continue
		}
		if last == nil || r.PromotedAt > last.PromotedAt {
			last = r
		}
	}
	return last
}

// PromotionCheck validates that a candidate run meets promotion criteria:
//   - Must have positive val_loss
//   - Must improve over baseline (last promoted run's val_loss), if a baseline exists
//   - Returns (pass, reason)
func PromotionCheck(vaultDir string, candidate *TrainRun) (bool, string) {
	if candidate.ValLoss <= 0 {
		return false, "candidate has no validation loss recorded"
	}

	baseline := LastPromotedRun(vaultDir)
	if baseline == nil {
		// No prior promoted run — first promotion, allow if val_loss is reasonable
		return true, fmt.Sprintf("first promotion (no baseline). val_loss=%.4f", candidate.ValLoss)
	}

	if baseline.ValLoss <= 0 {
		// Baseline has no loss — can't compare, allow with warning
		return true, fmt.Sprintf("baseline has no val_loss. candidate val_loss=%.4f", candidate.ValLoss)
	}

	if candidate.ValLoss >= baseline.ValLoss {
		return false, fmt.Sprintf("candidate val_loss %.4f >= baseline %.4f (run %s). No improvement — promotion blocked.",
			candidate.ValLoss, baseline.ValLoss, baseline.Timestamp)
	}

	improvement := (baseline.ValLoss - candidate.ValLoss) / baseline.ValLoss * 100
	return true, fmt.Sprintf("val_loss improved %.1f%% (%.4f → %.4f, baseline: run %s)",
		improvement, baseline.ValLoss, candidate.ValLoss, baseline.Timestamp)
}

// PromoteRun marks a training run as promoted after passing promotion checks.
// Returns error if promotion check fails.
func PromoteRun(vaultDir string, run *TrainRun, note string) error {
	pass, reason := PromotionCheck(vaultDir, run)
	if !pass {
		return fmt.Errorf("promotion blocked: %s", reason)
	}
	decision, stage, err := trust.ClassifyAtCurrentStage(vaultDir, trust.Request{
		ProducingOffice:    "training_governance",
		ProducingSubsystem: "trainer",
		ActionClass:        trust.ActionCodeOrHarnessMutation,
		TargetDomain:       "memory/training-runs",
		TouchedState:       []trust.StateClass{trust.StateOperational, trust.StateKnowledge},
		RequestedAuthority: ledger.ScopeManualModelPromotion,
	})
	if err != nil {
		return err
	}
	if !trust.Permits(decision, true) {
		return fmt.Errorf("model promotion blocked by trust gate: %s", decision.Reason)
	}

	run.Promoted = true
	run.PromotedAt = time.Now().Format(time.RFC3339)
	run.PromotionNote = fmt.Sprintf("%s | gate: %s", note, reason)
	if err := LogTrainRun(vaultDir, run); err != nil {
		return err
	}
	return ledger.Append(vaultDir, ledger.Record{
		Office:         "training_governance",
		Subsystem:      "trainer",
		AuthorityScope: ledger.ScopeManualModelPromotion,
		ActionClass:    ledger.ActionModelPromotion,
		TargetDomain:   "memory/training-runs",
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionApproved,
		SideEffects:    []string{"training_run_promoted"},
		ProofRefs:      []string{"memory/training-runs"},
		Signature: signature.Signature{
			ProducingOffice:    "training_governance",
			ProducingSubsystem: "trainer",
			StaffingContext:    run.Timestamp,
			AuthorityScope:     ledger.ScopeManualModelPromotion,
			ArtifactState:      "canonical",
			SourceRefs:         []string{"memory/training-runs"},
			PromotionStatus:    "approved",
			ProofRef:           "train-promotion:" + run.Timestamp,
		},
		Metadata: map[string]interface{}{
			"classifier_stage": stage,
			"timestamp":        run.Timestamp,
			"val_loss":         run.ValLoss,
			"promotion_note":   run.PromotionNote,
			"trust_decision":   string(decision.Decision),
		},
	})
}

// findPython locates a suitable Python interpreter.
func findPython() string {
	// Try venv first
	home, _ := os.UserHomeDir()
	venvPython := filepath.Join(home, "modus", ".venv", "bin", "python3")
	if _, err := os.Stat(venvPython); err == nil {
		return venvPython
	}
	// Fall back to system
	if path, err := exec.LookPath("python3"); err == nil {
		return path
	}
	return "python3"
}

var lossRE = regexp.MustCompile(`(?i)(Train|Val)\s+[Ll]oss:\s*([\d.]+)`)

func parseLastLoss(output, prefix string) float64 {
	var last float64
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		matches := lossRE.FindStringSubmatch(scanner.Text())
		if len(matches) >= 3 && strings.EqualFold(matches[1], prefix) {
			if val, err := strconv.ParseFloat(matches[2], 64); err == nil {
				last = val
			}
		}
	}
	return last
}

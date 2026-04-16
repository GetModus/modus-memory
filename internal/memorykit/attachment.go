package memorykit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/codex"
	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/vault"
)

const (
	attachmentHarnessName         = "memory_attachment"
	attachmentRecallMode          = "automatic_hot_admission"
	defaultAttachmentRecallLimit  = 6
	defaultAttachmentEnvironment  = "carrier_attachment"
	defaultAttachmentOffice       = "memory_governance"
	defaultAttachmentRecallOffice = "librarian"
	defaultGeminiAttachmentModel  = "gemini-2.5-flash"
	defaultOllamaAttachmentModel  = "gemma4:26b"
)

// AttachmentRunOptions configures a sovereign carrier run where MODUS Memory
// retrieves context before the carrier executes and records receipts after it
// returns.
type AttachmentRunOptions struct {
	Carrier      string
	Prompt       string
	Model        string
	WorkDir      string
	Target       string
	Ephemeral    bool
	RecallLimit  int
	StoreEpisode bool
	Subject      string
	WorkItemID   string
}

// AttachmentRunResult captures the direct operator-facing receipts from an
// attached carrier run.
type AttachmentRunResult struct {
	Carrier           string   `json:"carrier"`
	Model             string   `json:"model,omitempty"`
	Prompt            string   `json:"prompt"`
	Output            string   `json:"output,omitempty"`
	IsError           bool     `json:"is_error"`
	ThreadID          string   `json:"thread_id,omitempty"`
	DurationSec       float64  `json:"duration_sec"`
	MemoryApplied     bool     `json:"memory_applied"`
	RecallReceiptPath string   `json:"recall_receipt_path"`
	RecallLines       []string `json:"recall_lines,omitempty"`
	TracePath         string   `json:"trace_path,omitempty"`
	EpisodePath       string   `json:"episode_path,omitempty"`
	EpisodeEventID    string   `json:"episode_event_id,omitempty"`
}

type attachmentCarrierOptions struct {
	Prompt    string
	Model     string
	WorkDir   string
	Target    string
	Ephemeral bool
}

type attachmentCarrierResult struct {
	Text     string
	IsError  bool
	ThreadID string
	Model    string
}

type attachmentCarrierRunner interface {
	Run(context.Context, attachmentCarrierOptions) (attachmentCarrierResult, error)
}

type codexAttachmentRunner struct{}
type claudeAttachmentRunner struct{}
type qwenAttachmentRunner struct{}
type geminiAttachmentRunner struct{}
type hermesAttachmentRunner struct{}
type openClawAttachmentRunner struct{}
type ollamaAttachmentRunner struct{}
type opencodeAttachmentRunner struct{}

var attachmentRunnerResolver = defaultAttachmentRunner

func (codexAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	result, err := codex.Run(ctx, codex.RunOptions{
		Prompt:    opts.Prompt,
		Model:     opts.Model,
		WorkDir:   opts.WorkDir,
		Ephemeral: opts.Ephemeral,
	})
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:     result.Text,
		IsError:  result.IsError,
		ThreadID: result.ThreadID,
		Model:    strings.TrimSpace(opts.Model),
	}, nil
}

// RunAttachedCarrier executes the first sovereign attachment lane for a carrier
// through MODUS Memory.
func (k *Kernel) RunAttachedCarrier(ctx context.Context, opts AttachmentRunOptions) (AttachmentRunResult, error) {
	return k.runAttachedCarrier(ctx, opts, attachmentRunnerResolver(opts.Carrier))
}

func defaultAttachmentRunner(carrier string) attachmentCarrierRunner {
	switch normalizeAttachmentCarrier(carrier) {
	case "codex":
		return codexAttachmentRunner{}
	case "claude":
		return claudeAttachmentRunner{}
	case "qwen":
		return qwenAttachmentRunner{}
	case "gemini":
		return geminiAttachmentRunner{}
	case "hermes":
		return hermesAttachmentRunner{}
	case "openclaw":
		return openClawAttachmentRunner{}
	case "ollama":
		return ollamaAttachmentRunner{}
	case "opencode":
		return opencodeAttachmentRunner{}
	default:
		return nil
	}
}

func (k *Kernel) runAttachedCarrier(ctx context.Context, opts AttachmentRunOptions, runner attachmentCarrierRunner) (AttachmentRunResult, error) {
	carrier := normalizeAttachmentCarrier(opts.Carrier)
	if carrier == "" {
		carrier = "codex"
	}
	if runner == nil {
		return AttachmentRunResult{}, fmt.Errorf("unsupported attachment carrier: %s", carrier)
	}

	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return AttachmentRunResult{}, fmt.Errorf("empty attachment prompt")
	}

	recallLimit := opts.RecallLimit
	if recallLimit <= 0 {
		recallLimit = defaultAttachmentRecallLimit
	}

	recall, err := k.Recall(RecallRequest{
		Query:              prompt,
		Limit:              recallLimit,
		Options:            vault.FactSearchOptions{MemoryTemperature: "hot"},
		Harness:            attachmentHarnessName,
		Adapter:            "carrier_" + carrier,
		Mode:               attachmentRecallMode,
		ProducingOffice:    defaultAttachmentRecallOffice,
		ProducingSubsystem: "carrier_attachment_recall",
		StaffingContext:    carrier + "_attachment",
		WorkItemID:         strings.TrimSpace(opts.WorkItemID),
	})
	if err != nil {
		return AttachmentRunResult{}, fmt.Errorf("attachment recall failed: %w", err)
	}

	augmentedPrompt := composeAttachmentPrompt(prompt, recall)
	started := time.Now()
	carrierResult, runErr := runner.Run(ctx, attachmentCarrierOptions{
		Prompt:    augmentedPrompt,
		Model:     strings.TrimSpace(opts.Model),
		WorkDir:   strings.TrimSpace(opts.WorkDir),
		Target:    strings.TrimSpace(opts.Target),
		Ephemeral: opts.Ephemeral,
	})
	durationSec := time.Since(started).Seconds()

	result := AttachmentRunResult{
		Carrier:           carrier,
		Model:             attachmentFirstNonEmpty(strings.TrimSpace(carrierResult.Model), strings.TrimSpace(opts.Model)),
		Prompt:            prompt,
		Output:            strings.TrimSpace(carrierResult.Text),
		IsError:           carrierResult.IsError || runErr != nil,
		ThreadID:          strings.TrimSpace(carrierResult.ThreadID),
		DurationSec:       durationSec,
		MemoryApplied:     len(recall.Lines) > 0,
		RecallReceiptPath: recall.ReceiptPath,
		RecallLines:       append([]string(nil), recall.Lines...),
	}

	tracePath, traceErr := k.Vault.StoreTrace(
		traceTaskLabel(carrier, opts.Subject, prompt),
		traceOutcomeLabel(result.IsError),
		traceSteps(carrier, recall, result, runErr),
		durationSec,
		[]string{"memory_recall", carrier},
		defaultAttachmentOffice,
		result.Model,
	)
	if traceErr == nil {
		result.TracePath = tracePath
	}

	if opts.StoreEpisode {
		episodePath, episodeEventID, episodeErr := k.StoreEpisode(
			buildAttachmentEpisodeBody(carrier, recall, result, runErr),
			vault.EpisodeWriteAuthority{
				ProducingOffice:     defaultAttachmentOffice,
				ProducingSubsystem:  "carrier_attachment",
				StaffingContext:     carrier + "_attachment",
				AuthorityScope:      ledger.ScopeOperatorMemoryStore,
				TargetDomain:        "memory/episodes",
				Source:              attachmentHarnessName,
				SourceRef:           recall.ReceiptPath,
				SourceRefs:          attachmentSourceRefs(recall.ReceiptPath, tracePath),
				ProofRef:            "carrier-attachment:" + carrier,
				PromotionStatus:     "observed",
				EventKind:           "interaction",
				Subject:             deriveAttachmentSubject(opts.Subject, prompt, carrier),
				WorkItemID:          strings.TrimSpace(opts.WorkItemID),
				Environment:         defaultAttachmentEnvironment,
				RelatedFactPaths:    append(append([]string(nil), recall.ResultPaths...), recall.LinkedFactPaths...),
				RelatedEpisodePaths: append([]string(nil), recall.LinkedEpisodePaths...),
				RelatedEntityRefs:   append([]string(nil), recall.LinkedEntityRefs...),
				RelatedMissionRefs:  append([]string(nil), recall.LinkedMissionRefs...),
				CueTerms:            []string{carrier, "memory-attachment"},
				AllowApproval:       true,
			},
		)
		if episodeErr == nil {
			result.EpisodePath = episodePath
			result.EpisodeEventID = episodeEventID
		}
		if episodeErr != nil && runErr == nil {
			return result, fmt.Errorf("attachment episode capture failed: %w", episodeErr)
		}
	}

	if traceErr != nil && runErr == nil {
		return result, fmt.Errorf("attachment trace store failed: %w", traceErr)
	}
	if runErr != nil {
		return result, fmt.Errorf("carrier %s execution failed: %w", carrier, runErr)
	}
	return result, nil
}

func normalizeAttachmentCarrier(carrier string) string {
	switch strings.ToLower(strings.TrimSpace(carrier)) {
	case "", "codex", "codex-cli", "codex-app":
		return "codex"
	case "claude", "claude-code":
		return "claude"
	case "qwen", "qwen-cli":
		return "qwen"
	case "gemini", "gemini-cli":
		return "gemini"
	case "hermes", "hermes-cli":
		return "hermes"
	case "openclaw", "openclaw-cli":
		return "openclaw"
	case "ollama", "ollama-cli":
		return "ollama"
	case "opencode", "open-code":
		return "opencode"
	default:
		return strings.ToLower(strings.TrimSpace(carrier))
	}
}

func composeAttachmentPrompt(prompt string, recall RecallResult) string {
	var body strings.Builder
	body.WriteString("MODUS Memory attachment was consulted before this run.\n")
	body.WriteString(fmt.Sprintf("Recall receipt: %s\n", recall.ReceiptPath))
	if len(recall.Lines) == 0 {
		body.WriteString("No hot memory matched this request.\n\n")
	} else {
		body.WriteString("Use the following recalled memory when it is relevant. Do not invent beyond it.\n")
		for _, line := range recall.Lines {
			body.WriteString("- ")
			body.WriteString(strings.TrimSpace(line))
			body.WriteByte('\n')
		}
		if len(recall.LinkedEntityRefs) > 0 || len(recall.LinkedMissionRefs) > 0 || len(recall.LinkedFactPaths) > 0 || len(recall.LinkedEpisodePaths) > 0 {
			body.WriteString("Structural links from recalled memory:\n")
			if len(recall.LinkedEntityRefs) > 0 {
				body.WriteString(fmt.Sprintf("- linked entities: %s\n", strings.Join(recall.LinkedEntityRefs, ", ")))
			}
			if len(recall.LinkedMissionRefs) > 0 {
				body.WriteString(fmt.Sprintf("- linked missions: %s\n", strings.Join(recall.LinkedMissionRefs, ", ")))
			}
			if len(recall.LinkedFactPaths) > 0 {
				body.WriteString(fmt.Sprintf("- linked facts: %s\n", strings.Join(recall.LinkedFactPaths, ", ")))
			}
			if len(recall.LinkedEpisodePaths) > 0 {
				body.WriteString(fmt.Sprintf("- linked episodes: %s\n", strings.Join(recall.LinkedEpisodePaths, ", ")))
			}
		}
		body.WriteByte('\n')
	}
	body.WriteString("User request:\n")
	body.WriteString(strings.TrimSpace(prompt))
	body.WriteByte('\n')
	return body.String()
}

func deriveAttachmentSubject(subject, prompt, carrier string) string {
	if trimmed := strings.TrimSpace(subject); trimmed != "" {
		return trimmed
	}
	prompt = strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if prompt == "" {
		return fmt.Sprintf("%s attachment run", carrier)
	}
	if len(prompt) > 96 {
		return prompt[:96]
	}
	return prompt
}

func traceTaskLabel(carrier, subject, prompt string) string {
	base := deriveAttachmentSubject(subject, prompt, carrier)
	return fmt.Sprintf("attached %s run: %s", carrier, base)
}

func traceOutcomeLabel(isError bool) string {
	if isError {
		return "failure"
	}
	return "success"
}

func traceSteps(carrier string, recall RecallResult, result AttachmentRunResult, runErr error) []string {
	steps := []string{
		fmt.Sprintf("recalled hot memory through %s with receipt %s", carrier, recall.ReceiptPath),
		fmt.Sprintf("executed %s carrier with memory-attached prompt", carrier),
	}
	if len(recall.Lines) > 0 {
		steps = append(steps, fmt.Sprintf("applied %d hot-memory lines to the carrier prompt", len(recall.Lines)))
	} else {
		steps = append(steps, "continued without matched hot-memory lines")
	}
	if result.ThreadID != "" {
		steps = append(steps, fmt.Sprintf("carrier thread id %s", result.ThreadID))
	}
	if runErr != nil {
		steps = append(steps, fmt.Sprintf("carrier execution returned error: %v", runErr))
	} else if result.IsError {
		steps = append(steps, "carrier returned an error-shaped result")
	} else {
		steps = append(steps, "carrier execution completed successfully")
	}
	return steps
}

func buildAttachmentEpisodeBody(carrier string, recall RecallResult, result AttachmentRunResult, runErr error) string {
	var body strings.Builder
	body.WriteString("# Carrier Attachment Episode\n\n")
	body.WriteString(fmt.Sprintf("Carrier: `%s`\n\n", carrier))
	body.WriteString(fmt.Sprintf("Recall receipt: `%s`\n", recall.ReceiptPath))
	if result.TracePath != "" {
		body.WriteString(fmt.Sprintf("Trace: `%s`\n", result.TracePath))
	}
	if result.ThreadID != "" {
		body.WriteString(fmt.Sprintf("Thread ID: `%s`\n", result.ThreadID))
	}
	body.WriteString(fmt.Sprintf("Duration: `%.2fs`\n", result.DurationSec))
	body.WriteString(fmt.Sprintf("Outcome: `%s`\n\n", traceOutcomeLabel(result.IsError)))
	body.WriteString("## Prompt\n\n")
	body.WriteString(strings.TrimSpace(result.Prompt))
	body.WriteString("\n\n## Recalled Hot Memory\n\n")
	if len(recall.Lines) == 0 {
		body.WriteString("No hot memory matched this request.\n")
	} else {
		for _, line := range recall.Lines {
			body.WriteString("- ")
			body.WriteString(strings.TrimSpace(line))
			body.WriteByte('\n')
		}
	}
	body.WriteString("\n## Carrier Output\n\n")
	if strings.TrimSpace(result.Output) == "" {
		if runErr != nil {
			body.WriteString(runErr.Error())
			body.WriteByte('\n')
		} else {
			body.WriteString("(no output)\n")
		}
	} else {
		body.WriteString(strings.TrimSpace(result.Output))
		body.WriteByte('\n')
	}
	return strings.TrimSpace(body.String())
}

func attachmentSourceRefs(recallPath, tracePath string) []string {
	refs := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(recallPath); trimmed != "" {
		refs = append(refs, trimmed)
	}
	if trimmed := strings.TrimSpace(tracePath); trimmed != "" {
		refs = append(refs, trimmed)
	}
	return refs
}

func attachmentFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (claudeAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	args := []string{
		"-p",
		"--output-format", "json",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
	}
	if model := attachmentFirstNonEmpty(opts.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, opts.Prompt)
	stdout, _, err := runAttachmentCommand(ctx, "claude", args, opts.WorkDir, "", nil)
	if text, isError, model, parseErr := parseClaudeAttachmentOutput(stdout); parseErr == nil {
		return attachmentCarrierResult{
			Text:    text,
			IsError: isError,
			Model:   attachmentFirstNonEmpty(model, opts.Model),
		}, nil
	}
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return attachmentCarrierResult{}, fmt.Errorf("no response from claude")
	}
	return attachmentCarrierResult{
		Text:  trimmed,
		Model: strings.TrimSpace(opts.Model),
	}, nil
}

func (qwenAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	args := []string{
		"-p", opts.Prompt,
		"-o", "json",
		"--chat-recording=false",
	}
	if model := attachmentFirstNonEmpty(opts.Model); model != "" {
		args = append(args, "-m", model)
	}
	stdout, _, err := runAttachmentCommand(ctx, "qwen", args, opts.WorkDir, "", append(os.Environ(), "NO_COLOR=1"))
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	text, err := parseQwenAttachmentOutput(stdout)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:  text,
		Model: attachmentFirstNonEmpty(opts.Model, "qwen-3.6"),
	}, nil
}

func (geminiAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	model := attachmentFirstNonEmpty(opts.Model, defaultGeminiAttachmentModel)
	args := []string{"-m", model, "-p", opts.Prompt}
	stdout, _, err := runAttachmentCommand(ctx, "gemini", args, opts.WorkDir, "", nil)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:  cleanGeminiAttachmentOutput(stdout),
		Model: model,
	}, nil
}

func (hermesAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	args := []string{"chat", "-q", opts.Prompt, "-Q", "--source", "tool"}
	if model := attachmentFirstNonEmpty(opts.Model); model != "" {
		args = append(args, "-m", model)
	}
	stdout, _, err := runAttachmentCommand(ctx, "hermes", args, opts.WorkDir, "", nil)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:  cleanHermesAttachmentOutput(stdout),
		Model: strings.TrimSpace(opts.Model),
	}, nil
}

func (openClawAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	args := []string{"agent", "--local", "--json", "--message", opts.Prompt}
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return attachmentCarrierResult{}, fmt.Errorf("openclaw attachment requires a carrier target; pass --target <agent>, --target +15555550123, or --target session:<id>")
	}
	switch {
	case strings.HasPrefix(target, "session:"):
		args = append(args, "--session-id", strings.TrimSpace(strings.TrimPrefix(target, "session:")))
	case strings.HasPrefix(target, "+"):
		args = append(args, "--to", target)
	default:
		args = append(args, "--agent", target)
	}
	stdout, stderr, err := runAttachmentCommand(ctx, "openclaw", args, opts.WorkDir, "", nil)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	stream := stdout
	if strings.TrimSpace(stream) == "" {
		stream = stderr
	}
	text, err := parseOpenClawAttachmentOutput(stream)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:  text,
		Model: strings.TrimSpace(opts.Model),
	}, nil
}

func (ollamaAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	model := attachmentFirstNonEmpty(opts.Model, defaultOllamaAttachmentModel)
	args := []string{"run", model, opts.Prompt}
	stdout, _, err := runAttachmentCommand(ctx, "ollama", args, opts.WorkDir, "", append(os.Environ(), "NO_COLOR=1"))
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:  strings.TrimSpace(stdout),
		Model: model,
	}, nil
}

func (opencodeAttachmentRunner) Run(ctx context.Context, opts attachmentCarrierOptions) (attachmentCarrierResult, error) {
	args := []string{"run", "--format", "json", "--dangerously-skip-permissions"}
	if model := attachmentFirstNonEmpty(opts.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, opts.Prompt)
	stdout, _, err := runAttachmentCommand(ctx, "opencode", args, opts.WorkDir, "", nil)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	text, isError, model, err := parseOpenCodeAttachmentOutput(stdout)
	if err != nil {
		return attachmentCarrierResult{}, err
	}
	return attachmentCarrierResult{
		Text:    text,
		IsError: isError,
		Model:   attachmentFirstNonEmpty(model, opts.Model),
	}, nil
}

func runAttachmentCommand(ctx context.Context, binary string, args []string, workDir, stdin string, env []string) (string, string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", "", fmt.Errorf("%s not found: %w", binary, err)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = workDir
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return stdout.String(), stderrText, fmt.Errorf("%s: %w: %s", binary, err, stderrText)
		}
		return stdout.String(), "", fmt.Errorf("%s: %w", binary, err)
	}
	return stdout.String(), stderr.String(), nil
}

func parseQwenAttachmentOutput(stdout string) (string, error) {
	var lastText string
	for _, rawLine := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var entry struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.Type == "result" && strings.TrimSpace(entry.Result) != "" {
			return strings.TrimSpace(entry.Result), nil
		}
		var msg struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.Type == "assistant" {
			for _, part := range msg.Message.Content {
				if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
					lastText = strings.TrimSpace(part.Text)
				}
			}
		}
	}
	if lastText != "" {
		return lastText, nil
	}
	var entries []json.RawMessage
	if json.Unmarshal([]byte(stdout), &entries) == nil {
		for _, raw := range entries {
			var entry struct {
				Type   string `json:"type"`
				Result string `json:"result"`
			}
			if json.Unmarshal(raw, &entry) == nil && entry.Type == "result" && strings.TrimSpace(entry.Result) != "" {
				return strings.TrimSpace(entry.Result), nil
			}
		}
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed != "" {
		return trimmed, nil
	}
	return "", fmt.Errorf("no response from qwen CLI")
}

func parseOpenClawAttachmentOutput(stdout string) (string, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "", fmt.Errorf("no response from openclaw")
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed, nil
	}
	if text := extractAttachmentText(payload); text != "" {
		return text, nil
	}
	return trimmed, nil
}

func parseClaudeAttachmentOutput(stdout string) (string, bool, string, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "", false, "", fmt.Errorf("no response from claude")
	}
	var payload struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		return strings.TrimSpace(payload.Result), payload.IsError, strings.TrimSpace(payload.Model), nil
	}
	return trimmed, false, "", nil
}

func parseOpenCodeAttachmentOutput(stdout string) (string, bool, string, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "", false, "", fmt.Errorf("no response from opencode")
	}
	var lastText string
	var lastModel string
	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var entry struct {
			Type  string `json:"type"`
			Model struct {
				ID string `json:"id"`
			} `json:"model"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
			Message struct {
				Parts []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil {
			switch entry.Type {
			case "message.part.updated":
				for _, part := range entry.Message.Parts {
					if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
						lastText = strings.TrimSpace(part.Text)
						lastModel = strings.TrimSpace(entry.Model.ID)
					}
				}
			case "text":
				if text := strings.TrimSpace(entry.Part.Text); text != "" {
					lastText = text
					lastModel = strings.TrimSpace(entry.Model.ID)
				}
			case "error":
				if text := strings.TrimSpace(entry.Error.Message); text != "" {
					return text, true, strings.TrimSpace(entry.Model.ID), nil
				}
			}
		}
	}
	if cleaned := cleanOpenCodeAttachmentOutput(lastText); cleaned != "" {
		return cleaned, false, lastModel, nil
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if text := extractAttachmentText(payload); text != "" {
			return cleanOpenCodeAttachmentOutput(text), false, "", nil
		}
	}
	return cleanOpenCodeAttachmentOutput(trimmed), false, "", nil
}

func cleanGeminiAttachmentOutput(stdout string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(stdout, "\r", ""))
	const prefix = "MCP issues detected. Run /mcp list for status."
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	return trimmed
}

func cleanHermesAttachmentOutput(stdout string) string {
	lines := strings.Split(strings.ReplaceAll(stdout, "\r", ""), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "session_id:"):
			continue
		case strings.Contains(trimmed, "Hermes") && (strings.HasPrefix(trimmed, "╭") || strings.HasPrefix(trimmed, "╰")):
			continue
		default:
			kept = append(kept, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func cleanOpenCodeAttachmentOutput(stdout string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(stdout, "\r", ""))
	if trimmed == "" {
		return ""
	}
	if idx := strings.LastIndex(trimmed, "RESPONSE:"); idx >= 0 {
		return strings.TrimSpace(trimmed[idx+len("RESPONSE:"):])
	}
	return trimmed
}

func extractAttachmentText(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"reply", "response", "output", "text", "content", "message"} {
			if candidate, ok := typed[key]; ok {
				if text := extractAttachmentText(candidate); text != "" {
					return text
				}
			}
		}
		for _, candidate := range typed {
			if text := extractAttachmentText(candidate); text != "" {
				return text
			}
		}
	case []interface{}:
		for _, candidate := range typed {
			if text := extractAttachmentText(candidate); text != "" {
				return text
			}
		}
	case string:
		return strings.TrimSpace(typed)
	}
	return ""
}

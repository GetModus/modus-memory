package memorykit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
)

type CarrierProbeOptions struct {
	Carriers       []string
	Prompt         string
	Model          string
	WorkDir        string
	RecallLimit    int
	StoreEpisode   bool
	OpenClawTarget string
	WorkItemID     string
}

type CarrierProbeEntry struct {
	Carrier           string  `json:"carrier"`
	Status            string  `json:"status"`
	Model             string  `json:"model,omitempty"`
	DurationSec       float64 `json:"duration_sec"`
	IsError           bool    `json:"is_error"`
	Error             string  `json:"error,omitempty"`
	ThreadID          string  `json:"thread_id,omitempty"`
	RecallReceiptPath string  `json:"recall_receipt_path,omitempty"`
	TracePath         string  `json:"trace_path,omitempty"`
	EpisodePath       string  `json:"episode_path,omitempty"`
	OutputPreview     string  `json:"output_preview,omitempty"`
}

type CarrierProbeReport struct {
	Version         int                 `json:"version"`
	GeneratedAt     string              `json:"generated_at"`
	Prompt          string              `json:"prompt"`
	Carriers        []string            `json:"carriers"`
	TotalCarriers   int                 `json:"total_carriers"`
	SuccessfulCount int                 `json:"successful_count"`
	FailedCount     int                 `json:"failed_count"`
	Entries         []CarrierProbeEntry `json:"entries"`
	Signature       signature.Signature `json:"signature"`
}

type CarrierProbeResult struct {
	ReportPath   string
	MarkdownPath string
	Report       CarrierProbeReport
}

func (k *Kernel) ProbeCarriers(ctx context.Context, opts CarrierProbeOptions) (CarrierProbeResult, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		prompt = "Reply with exactly: nominal."
	}
	carriers := normalizeProbeCarriers(opts.Carriers)
	if len(carriers) == 0 {
		return CarrierProbeResult{}, fmt.Errorf("at least one carrier is required")
	}

	entries := make([]CarrierProbeEntry, 0, len(carriers))
	successful := 0
	failed := 0
	for _, carrier := range carriers {
		runOpts := AttachmentRunOptions{
			Carrier:      carrier,
			Prompt:       prompt,
			Model:        strings.TrimSpace(opts.Model),
			WorkDir:      strings.TrimSpace(opts.WorkDir),
			RecallLimit:  opts.RecallLimit,
			StoreEpisode: opts.StoreEpisode,
			WorkItemID:   strings.TrimSpace(opts.WorkItemID),
		}
		if carrier == "openclaw" {
			runOpts.Target = firstNonEmptyProbe(strings.TrimSpace(opts.OpenClawTarget), "main")
		}
		result, err := k.RunAttachedCarrier(ctx, runOpts)
		entry := CarrierProbeEntry{
			Carrier:           carrier,
			Status:            "passed",
			Model:             strings.TrimSpace(result.Model),
			DurationSec:       result.DurationSec,
			IsError:           result.IsError,
			ThreadID:          strings.TrimSpace(result.ThreadID),
			RecallReceiptPath: strings.TrimSpace(result.RecallReceiptPath),
			TracePath:         strings.TrimSpace(result.TracePath),
			EpisodePath:       strings.TrimSpace(result.EpisodePath),
			OutputPreview:     truncateProbePreview(result.Output),
		}
		if err != nil {
			entry.Status = "failed"
			entry.Error = err.Error()
		} else if result.IsError {
			entry.Status = "failed"
			entry.Error = "carrier returned an error-shaped result"
		}
		if entry.Status == "passed" {
			successful++
		} else {
			failed++
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Status == entries[j].Status {
			return entries[i].Carrier < entries[j].Carrier
		}
		return entries[i].Status < entries[j].Status
	})

	report := CarrierProbeReport{
		Version:         1,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Prompt:          prompt,
		Carriers:        carriers,
		TotalCarriers:   len(carriers),
		SuccessfulCount: successful,
		FailedCount:     failed,
		Entries:         entries,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_carrier_probe",
			StaffingContext:    "carrier_practice",
			AuthorityScope:     ledger.ScopeRuntimeMemoryCarrierProbe,
			ArtifactState:      "derived",
			SourceRefs:         []string{"state/memory/carriers/probes/latest.json", "state/memory/carriers/probes/latest.md"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-carrier-probe:" + time.Now().UTC().Format("20060102T150405Z"),
		}.EnsureTimestamp(),
	}

	jsonRelPath := filepath.ToSlash(filepath.Join("state", "memory", "carriers", "probes", "latest.json"))
	mdRelPath := filepath.ToSlash(filepath.Join("state", "memory", "carriers", "probes", "latest.md"))
	jsonAbsPath := filepath.Join(k.Vault.Dir, jsonRelPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, mdRelPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return CarrierProbeResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return CarrierProbeResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return CarrierProbeResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_carrier_probe",
		"generated_at":        report.GeneratedAt,
		"prompt":              report.Prompt,
		"carriers":            report.Carriers,
		"total_carriers":      report.TotalCarriers,
		"successful_count":    report.SuccessfulCount,
		"failed_count":        report.FailedCount,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Carrier Probe Report\n\n")
	body.WriteString(fmt.Sprintf("Prompt: `%s`\n\n", report.Prompt))
	body.WriteString(fmt.Sprintf("Carriers: `%s`\n\n", strings.Join(report.Carriers, ", ")))
	body.WriteString(fmt.Sprintf("Passed: `%d`\n\n", report.SuccessfulCount))
	body.WriteString(fmt.Sprintf("Failed: `%d`\n\n", report.FailedCount))
	body.WriteString("This report reflects live sovereign attachment probes. Each carrier was run through the memory attachment lane rather than judged as a direct memory client.\n\n")
	body.WriteString("## Entries\n\n")
	for _, entry := range report.Entries {
		body.WriteString(fmt.Sprintf("### %s\n\n", entry.Carrier))
		body.WriteString(fmt.Sprintf("Status: `%s`\n\n", entry.Status))
		body.WriteString(fmt.Sprintf("Duration: `%.2fs`\n\n", entry.DurationSec))
		if entry.Model != "" {
			body.WriteString(fmt.Sprintf("Model: `%s`\n\n", entry.Model))
		}
		if entry.ThreadID != "" {
			body.WriteString(fmt.Sprintf("Thread ID: `%s`\n\n", entry.ThreadID))
		}
		if entry.RecallReceiptPath != "" {
			body.WriteString(fmt.Sprintf("Recall receipt: `%s`\n\n", entry.RecallReceiptPath))
		}
		if entry.TracePath != "" {
			body.WriteString(fmt.Sprintf("Trace: `%s`\n\n", entry.TracePath))
		}
		if entry.EpisodePath != "" {
			body.WriteString(fmt.Sprintf("Episode: `%s`\n\n", entry.EpisodePath))
		}
		if entry.OutputPreview != "" {
			body.WriteString(fmt.Sprintf("Output preview: `%s`\n\n", entry.OutputPreview))
		}
		if entry.Error != "" {
			body.WriteString(fmt.Sprintf("Error: `%s`\n\n", entry.Error))
		}
	}
	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return CarrierProbeResult{}, err
	}

	status := ledger.ResultCompleted
	if report.FailedCount > 0 {
		status = ledger.ResultFailed
	}
	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_carrier_probe",
		AuthorityScope: ledger.ScopeRuntimeMemoryCarrierProbe,
		ActionClass:    ledger.ActionMemoryCarrierProbe,
		TargetDomain:   jsonRelPath,
		ResultStatus:   status,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_carrier_probe_written"},
		ProofRefs:      []string{jsonRelPath, mdRelPath},
		Signature:      report.Signature,
		Metadata: map[string]interface{}{
			"carriers":         report.Carriers,
			"successful_count": report.SuccessfulCount,
			"failed_count":     report.FailedCount,
		},
	})

	return CarrierProbeResult{
		ReportPath:   jsonRelPath,
		MarkdownPath: mdRelPath,
		Report:       report,
	}, nil
}

func MarshalCarrierProbeJSON(report CarrierProbeReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func RenderCarrierProbeSummary(report CarrierProbeReport) string {
	var sb strings.Builder
	sb.WriteString("Carrier probe written at state/memory/carriers/probes/latest.json\n")
	sb.WriteString(fmt.Sprintf("Passed: %d\n", report.SuccessfulCount))
	sb.WriteString(fmt.Sprintf("Failed: %d\n", report.FailedCount))
	for _, entry := range report.Entries {
		sb.WriteString(fmt.Sprintf("%s: %s", entry.Carrier, entry.Status))
		if entry.Error != "" {
			sb.WriteString(" — ")
			sb.WriteString(entry.Error)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func normalizeProbeCarriers(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			name := normalizeAttachmentCarrier(item)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func truncateProbePreview(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len(text) <= 160 {
		return text
	}
	return strings.TrimSpace(text[:157]) + "..."
}

func firstNonEmptyProbe(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

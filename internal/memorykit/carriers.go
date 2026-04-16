package memorykit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
)

var carrierLookPath = exec.LookPath

type CarrierAuditEntry struct {
	Carrier          string   `json:"carrier"`
	Class            string   `json:"class"`
	BinaryCommand    string   `json:"binary_command"`
	BinaryFound      bool     `json:"binary_found"`
	BinaryPath       string   `json:"binary_path,omitempty"`
	WrapperCommand   string   `json:"wrapper_command,omitempty"`
	WrapperOnPath    bool     `json:"wrapper_on_path"`
	WrapperPath      string   `json:"wrapper_path,omitempty"`
	RepoWrapperFound bool     `json:"repo_wrapper_found"`
	RepoWrapperPath  string   `json:"repo_wrapper_path,omitempty"`
	RequiresTarget   bool     `json:"requires_target"`
	RecommendedLane  string   `json:"recommended_lane"`
	Status           string   `json:"status"`
	SampleCommand    string   `json:"sample_command,omitempty"`
	Notes            []string `json:"notes,omitempty"`
}

type CarrierAuditReport struct {
	Version        int                 `json:"version"`
	GeneratedAt    string              `json:"generated_at"`
	TotalCarriers  int                 `json:"total_carriers"`
	ReadyCount     int                 `json:"ready_count"`
	MissingCount   int                 `json:"missing_count"`
	DormantCount   int                 `json:"dormant_count"`
	CountsByStatus map[string]int      `json:"counts_by_status"`
	CountsByLane   map[string]int      `json:"counts_by_lane"`
	Entries        []CarrierAuditEntry `json:"entries"`
	Signature      signature.Signature `json:"signature"`
}

type CarrierAuditResult struct {
	ReportPath   string
	MarkdownPath string
	Report       CarrierAuditReport
}

type carrierSpec struct {
	Name           string
	Binary         string
	Wrapper        string
	RequiresTarget bool
	DormantByLaw   bool
	Notes          []string
}

func (k *Kernel) AuditCarriers() (CarrierAuditResult, error) {
	specs := []carrierSpec{
		{Name: "codex", Binary: "codex", Wrapper: "modus-codex", Notes: []string{"Recommended for Codex CLI and adjacent shell workflows through sovereign attachment."}},
		{Name: "qwen", Binary: "qwen", Wrapper: "modus-qwen", Notes: []string{"CLI carrier; best treated as an attachment carrier rather than a direct memory client."}},
		{Name: "gemini", Binary: "gemini", Wrapper: "modus-gemini", Notes: []string{"Useful cloud lane for attachment probes and operator shell work."}},
		{Name: "ollama", Binary: "ollama", Wrapper: "modus-ollama", Notes: []string{"Local carrier; live probes should still be treated with the usual discretion."}},
		{Name: "hermes", Binary: "hermes", Wrapper: "modus-hermes", Notes: []string{"Hermes remains a shell carrier and should use the attachment lane."}},
		{Name: "openclaw", Binary: "openclaw", Wrapper: "modus-openclaw", RequiresTarget: true, Notes: []string{"OpenClaw requires a target; the wrapper defaults that target to `main`."}},
		{Name: "opencode", Binary: "opencode", Wrapper: "modus-opencode", Notes: []string{"OpenCode is treated as a plain shell carrier with JSON-friendly output parsing."}},
		{Name: "claude", Binary: "claude", Wrapper: "", DormantByLaw: true, Notes: []string{"Supported by the attachment code path, but doctrinally treated as dormant unless the local estate is actually live."}},
	}

	repoRoot := filepath.Clean(filepath.Join(k.Vault.Dir, ".."))
	entries := make([]CarrierAuditEntry, 0, len(specs))
	countsByStatus := map[string]int{}
	countsByLane := map[string]int{}
	ready := 0
	missing := 0
	dormant := 0

	for _, spec := range specs {
		entry := CarrierAuditEntry{
			Carrier:        spec.Name,
			Class:          "sovereign_attachment_carrier",
			BinaryCommand:  spec.Binary,
			WrapperCommand: spec.Wrapper,
			RequiresTarget: spec.RequiresTarget,
			Notes:          append([]string(nil), spec.Notes...),
		}
		if path, err := carrierLookPath(spec.Binary); err == nil {
			entry.BinaryFound = true
			entry.BinaryPath = path
		}
		if spec.Wrapper != "" {
			if path, err := carrierLookPath(spec.Wrapper); err == nil {
				entry.WrapperOnPath = true
				entry.WrapperPath = path
			}
			repoWrapper := filepath.Join(repoRoot, "scripts", spec.Wrapper)
			if st, err := os.Stat(repoWrapper); err == nil && !st.IsDir() {
				entry.RepoWrapperFound = true
				entry.RepoWrapperPath = repoWrapper
			}
		}
		if spec.DormantByLaw {
			if entry.BinaryFound {
				entry.Notes = append(entry.Notes, "Binary is present locally, but this carrier remains doctrinally dormant until the estate is explicitly restored to active service.")
			} else {
				entry.Notes = append(entry.Notes, "Carrier is supported by code path only; keep it out of the active fleet until the local estate is restored.")
			}
		}
		entry.RecommendedLane, entry.Status = classifyCarrierAuditEntry(entry, spec)
		entry.SampleCommand = sampleCarrierAuditCommand(entry)
		countsByStatus[entry.Status]++
		countsByLane[entry.RecommendedLane]++
		switch entry.Status {
		case "ready":
			ready++
		case "dormant":
			dormant++
		default:
			missing++
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		si := carrierStatusPriority(entries[i].Status)
		sj := carrierStatusPriority(entries[j].Status)
		if si == sj {
			return entries[i].Carrier < entries[j].Carrier
		}
		return si < sj
	})

	report := CarrierAuditReport{
		Version:        1,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		TotalCarriers:  len(entries),
		ReadyCount:     ready,
		MissingCount:   missing,
		DormantCount:   dormant,
		CountsByStatus: countsByStatus,
		CountsByLane:   countsByLane,
		Entries:        entries,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_carrier_audit",
			StaffingContext:    "carrier_practice",
			AuthorityScope:     ledger.ScopeRuntimeMemoryCarrierAudit,
			ArtifactState:      "derived",
			SourceRefs:         []string{"state/memory/carriers/latest.json", "state/memory/carriers/latest.md"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-carrier-audit:" + time.Now().UTC().Format("20060102T150405Z"),
		}.EnsureTimestamp(),
	}

	jsonRelPath := filepath.ToSlash(filepath.Join("state", "memory", "carriers", "latest.json"))
	mdRelPath := filepath.ToSlash(filepath.Join("state", "memory", "carriers", "latest.md"))
	jsonAbsPath := filepath.Join(k.Vault.Dir, jsonRelPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, mdRelPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return CarrierAuditResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return CarrierAuditResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return CarrierAuditResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_carrier_audit",
		"generated_at":        report.GeneratedAt,
		"total_carriers":      report.TotalCarriers,
		"ready_count":         report.ReadyCount,
		"missing_count":       report.MissingCount,
		"dormant_count":       report.DormantCount,
		"counts_by_status":    report.CountsByStatus,
		"counts_by_lane":      report.CountsByLane,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Carrier Audit\n\n")
	body.WriteString(fmt.Sprintf("Carriers inspected: `%d`\n\n", report.TotalCarriers))
	body.WriteString(fmt.Sprintf("Ready: `%d`\n\n", report.ReadyCount))
	body.WriteString(fmt.Sprintf("Missing: `%d`\n\n", report.MissingCount))
	body.WriteString(fmt.Sprintf("Dormant by doctrine: `%d`\n\n", report.DormantCount))
	body.WriteString("This audit is substrate-only. It checks local binaries, wrapper availability, target requirements, and doctrinal lane selection. It does not perform live model execution.\n\n")
	body.WriteString("## Carriers\n\n")
	for _, entry := range report.Entries {
		body.WriteString(fmt.Sprintf("### %s\n\n", entry.Carrier))
		body.WriteString(fmt.Sprintf("Status: `%s`\n\n", entry.Status))
		body.WriteString(fmt.Sprintf("Recommended lane: `%s`\n\n", entry.RecommendedLane))
		body.WriteString(fmt.Sprintf("Binary command: `%s`\n\n", entry.BinaryCommand))
		body.WriteString(fmt.Sprintf("Binary found: `%t`\n\n", entry.BinaryFound))
		if entry.BinaryPath != "" {
			body.WriteString(fmt.Sprintf("Binary path: `%s`\n\n", entry.BinaryPath))
		}
		if entry.WrapperCommand != "" {
			body.WriteString(fmt.Sprintf("Wrapper command: `%s`\n\n", entry.WrapperCommand))
			body.WriteString(fmt.Sprintf("Wrapper on PATH: `%t`\n\n", entry.WrapperOnPath))
			body.WriteString(fmt.Sprintf("Repo wrapper present: `%t`\n\n", entry.RepoWrapperFound))
		}
		if entry.RequiresTarget {
			body.WriteString("Requires target: `true`\n\n")
		}
		if entry.SampleCommand != "" {
			body.WriteString(fmt.Sprintf("Sample command: `%s`\n\n", entry.SampleCommand))
		}
		for _, note := range entry.Notes {
			body.WriteString(fmt.Sprintf("- %s\n", note))
		}
		body.WriteString("\n")
	}
	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return CarrierAuditResult{}, err
	}

	status := ledger.ResultCompleted
	if report.ReadyCount == 0 {
		status = ledger.ResultFailed
	}
	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_carrier_audit",
		AuthorityScope: ledger.ScopeRuntimeMemoryCarrierAudit,
		ActionClass:    ledger.ActionMemoryCarrierAudit,
		TargetDomain:   jsonRelPath,
		ResultStatus:   status,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_carrier_audit_written"},
		ProofRefs:      []string{jsonRelPath, mdRelPath},
		Signature:      report.Signature,
		Metadata: map[string]interface{}{
			"ready_count":   report.ReadyCount,
			"missing_count": report.MissingCount,
			"dormant_count": report.DormantCount,
		},
	})

	return CarrierAuditResult{
		ReportPath:   jsonRelPath,
		MarkdownPath: mdRelPath,
		Report:       report,
	}, nil
}

func classifyCarrierAuditEntry(entry CarrierAuditEntry, spec carrierSpec) (lane, status string) {
	if spec.DormantByLaw {
		return "dormant", "dormant"
	}
	if !entry.BinaryFound {
		return "none", "missing"
	}
	if entry.WrapperOnPath || entry.RepoWrapperFound {
		return "wrapper_attachment", "ready"
	}
	return "raw_attachment", "ready"
}

func sampleCarrierAuditCommand(entry CarrierAuditEntry) string {
	switch entry.RecommendedLane {
	case "wrapper_attachment":
		if entry.Carrier == "openclaw" {
			return "modus-openclaw --target main \"Reply with exactly: nominal.\""
		}
		if entry.WrapperCommand != "" {
			return fmt.Sprintf("%s \"Reply with exactly: nominal.\"", entry.WrapperCommand)
		}
	case "raw_attachment":
		if entry.Carrier == "openclaw" {
			return "modus memory attach --carrier openclaw --target main --prompt \"Reply with exactly: nominal.\""
		}
		return fmt.Sprintf("modus memory attach --carrier %s --prompt \"Reply with exactly: nominal.\"", entry.Carrier)
	case "dormant":
		return ""
	}
	return ""
}

func carrierStatusPriority(status string) int {
	switch status {
	case "ready":
		return 0
	case "dormant":
		return 1
	default:
		return 2
	}
}

func MarshalCarrierAuditJSON(report CarrierAuditReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func RenderCarrierAuditSummary(report CarrierAuditReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Carrier audit written at state/memory/carriers/latest.json\n"))
	sb.WriteString(fmt.Sprintf("Ready: %d\n", report.ReadyCount))
	sb.WriteString(fmt.Sprintf("Missing: %d\n", report.MissingCount))
	sb.WriteString(fmt.Sprintf("Dormant: %d\n", report.DormantCount))
	for _, entry := range report.Entries {
		sb.WriteString(fmt.Sprintf("%s: %s via %s\n", entry.Carrier, entry.Status, entry.RecommendedLane))
	}
	return sb.String()
}

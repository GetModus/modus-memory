package memorykit

import (
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

const (
	portabilityCoverageExplicitEquivalent = "explicit_runtime_equivalent"
	portabilityCoverageExactCounterpart   = "exact_vault_counterpart"
	portabilityCoverageCited              = "cited_into_sovereign_memory"
	portabilityCoverageExternalOnly       = "external_only"
)

type PortabilityAuditEntry struct {
	CachePath        string   `json:"cache_path"`
	FileClass        string   `json:"file_class"`
	CoverageClass    string   `json:"coverage_class"`
	CounterpartPaths []string `json:"counterpart_paths,omitempty"`
	CitationPaths    []string `json:"citation_paths,omitempty"`
	Notes            []string `json:"notes,omitempty"`
}

type PortabilityAuditReport struct {
	Version        int                     `json:"version"`
	GeneratedAt    string                  `json:"generated_at"`
	CachePath      string                  `json:"cache_path"`
	CachePresent   bool                    `json:"cache_present"`
	TotalFiles     int                     `json:"total_files"`
	CoveredFiles   int                     `json:"covered_files"`
	ExternalOnly   int                     `json:"external_only"`
	CoverageScore  float64                 `json:"coverage_score"`
	ClassCounts    map[string]int          `json:"class_counts"`
	CoverageCounts map[string]int          `json:"coverage_counts"`
	Entries        []PortabilityAuditEntry `json:"entries"`
	Signature      signature.Signature     `json:"signature"`
}

type PortabilityAuditResult struct {
	ReportPath   string
	MarkdownPath string
	Report       PortabilityAuditReport
}

type PortabilityQueueEntry struct {
	CachePath      string `json:"cache_path"`
	FileClass      string `json:"file_class"`
	Priority       string `json:"priority"`
	ProposedAction string `json:"proposed_action"`
	TargetSurface  string `json:"target_surface"`
	Reason         string `json:"reason"`
}

type PortabilityQueueReport struct {
	Version        int                     `json:"version"`
	GeneratedAt    string                  `json:"generated_at"`
	SourceAudit    string                  `json:"source_audit"`
	TotalItems     int                     `json:"total_items"`
	PriorityCounts map[string]int          `json:"priority_counts"`
	Entries        []PortabilityQueueEntry `json:"entries"`
	Signature      signature.Signature     `json:"signature"`
}

type PortabilityQueueResult struct {
	ReportPath   string
	MarkdownPath string
	Report       PortabilityQueueReport
}

type PortabilityArchiveReport struct {
	Version         int                 `json:"version"`
	GeneratedAt     string              `json:"generated_at"`
	SourceQueue     string              `json:"source_queue"`
	DestinationRoot string              `json:"destination_root"`
	ArchivedCount   int                 `json:"archived_count"`
	ArchivedPaths   []string            `json:"archived_paths"`
	Signature       signature.Signature `json:"signature"`
}

type PortabilityArchiveResult struct {
	ReportPath   string
	MarkdownPath string
	Report       PortabilityArchiveReport
}

type portabilityCacheFile struct {
	RelPath string
	Base    string
}

func (k *Kernel) AuditPortability(cacheDir string) (PortabilityAuditResult, error) {
	var err error
	cacheDir, err = resolvePortabilityCacheDir(k.Vault.Dir, cacheDir)
	if err != nil {
		return PortabilityAuditResult{}, err
	}

	cacheFiles, cachePresent, err := scanPortabilityCacheFiles(cacheDir)
	if err != nil {
		return PortabilityAuditResult{}, err
	}
	vaultIndex, err := scanVaultFileBasenameIndex(k.Vault.Dir)
	if err != nil {
		return PortabilityAuditResult{}, err
	}
	citationIndex, err := k.buildPortabilityCitationIndex(cacheFiles)
	if err != nil {
		return PortabilityAuditResult{}, err
	}

	reportPath := filepath.ToSlash(filepath.Join("state", "memory", "portability", "latest.json"))
	markdownPath := filepath.ToSlash(filepath.Join("state", "memory", "portability", "latest.md"))
	coverageCounts := map[string]int{
		portabilityCoverageExplicitEquivalent: 0,
		portabilityCoverageExactCounterpart:   0,
		portabilityCoverageCited:              0,
		portabilityCoverageExternalOnly:       0,
	}
	classCounts := map[string]int{}
	entries := make([]PortabilityAuditEntry, 0, len(cacheFiles))

	for _, file := range cacheFiles {
		entry := PortabilityAuditEntry{
			CachePath: file.RelPath,
			FileClass: classifyPortabilityFile(file.RelPath),
		}
		classCounts[entry.FileClass]++

		if counterparts, notes := resolveExplicitPortabilityCounterparts(k.Vault.Dir, file.RelPath); len(counterparts) > 0 {
			entry.CoverageClass = portabilityCoverageExplicitEquivalent
			entry.CounterpartPaths = dedupeSortedStrings(counterparts)
			entry.Notes = append(entry.Notes, notes...)
		} else if counterparts := portabilityExactCounterparts(file.Base, vaultIndex); len(counterparts) > 0 {
			entry.CoverageClass = portabilityCoverageExactCounterpart
			entry.CounterpartPaths = counterparts
		} else if citations := dedupeSortedStrings(citationIndex[strings.ToLower(file.RelPath)]); len(citations) > 0 {
			entry.CoverageClass = portabilityCoverageCited
			entry.CitationPaths = citations
		} else {
			entry.CoverageClass = portabilityCoverageExternalOnly
			entry.Notes = append(entry.Notes, portabilityExternalOnlyNotes(file.RelPath)...)
		}

		coverageCounts[entry.CoverageClass]++
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		pi := portabilityCoveragePriority(entries[i].CoverageClass)
		pj := portabilityCoveragePriority(entries[j].CoverageClass)
		if pi == pj {
			return entries[i].CachePath < entries[j].CachePath
		}
		return pi < pj
	})

	coveredFiles := coverageCounts[portabilityCoverageExplicitEquivalent] + coverageCounts[portabilityCoverageExactCounterpart] + coverageCounts[portabilityCoverageCited]
	coverageScore := 1.0
	if len(entries) > 0 {
		coverageScore = float64(coveredFiles) / float64(len(entries))
	}

	report := PortabilityAuditReport{
		Version:        1,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		CachePath:      cacheDir,
		CachePresent:   cachePresent,
		TotalFiles:     len(entries),
		CoveredFiles:   coveredFiles,
		ExternalOnly:   coverageCounts[portabilityCoverageExternalOnly],
		CoverageScore:  coverageScore,
		ClassCounts:    classCounts,
		CoverageCounts: coverageCounts,
		Entries:        entries,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_portability",
			StaffingContext:    "external_cache_audit",
			AuthorityScope:     ledger.ScopeRuntimeMemoryPortability,
			ArtifactState:      "derived",
			SourceRefs:         []string{reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           "memory-portability:" + time.Now().UTC().Format("20060102T150405Z"),
		}.EnsureTimestamp(),
	}

	jsonAbsPath := filepath.Join(k.Vault.Dir, reportPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, markdownPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return PortabilityAuditResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return PortabilityAuditResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return PortabilityAuditResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_portability_audit",
		"generated_at":        report.GeneratedAt,
		"cache_path":          report.CachePath,
		"cache_present":       report.CachePresent,
		"total_files":         report.TotalFiles,
		"covered_files":       report.CoveredFiles,
		"external_only":       report.ExternalOnly,
		"coverage_score":      report.CoverageScore,
		"class_counts":        report.ClassCounts,
		"coverage_counts":     report.CoverageCounts,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Portability Audit\n\n")
	body.WriteString(fmt.Sprintf("Cache path: `%s`\n\n", report.CachePath))
	body.WriteString(fmt.Sprintf("Cache present: `%t`\n\n", report.CachePresent))
	body.WriteString(fmt.Sprintf("Files inspected: `%d`\n\n", report.TotalFiles))
	body.WriteString(fmt.Sprintf("Covered by sovereign memory surfaces: `%d`\n\n", report.CoveredFiles))
	body.WriteString(fmt.Sprintf("External-only residue: `%d`\n\n", report.ExternalOnly))
	body.WriteString(fmt.Sprintf("Coverage score: `%.2f`\n\n", report.CoverageScore))
	body.WriteString("Coverage classes:\n")
	body.WriteString(fmt.Sprintf("- explicit runtime equivalent: `%d`\n", report.CoverageCounts[portabilityCoverageExplicitEquivalent]))
	body.WriteString(fmt.Sprintf("- exact vault counterpart: `%d`\n", report.CoverageCounts[portabilityCoverageExactCounterpart]))
	body.WriteString(fmt.Sprintf("- cited into sovereign memory: `%d`\n", report.CoverageCounts[portabilityCoverageCited]))
	body.WriteString(fmt.Sprintf("- external only: `%d`\n\n", report.CoverageCounts[portabilityCoverageExternalOnly]))
	body.WriteString("This audit is conservative. It only claims coverage when MODUS can prove one of three things: an explicit runtime equivalent, an exact sovereign counterpart by filename, or a provenance-bearing citation into sovereign memory. Everything else remains external residue until migrated or superseded by explicit law.\n\n")

	if len(entries) == 0 {
		body.WriteString("No external cache files were detected for this project.\n")
	} else {
		body.WriteString("## External-Only Residue\n\n")
		externalOnly := portabilityEntriesByCoverage(entries, portabilityCoverageExternalOnly)
		if len(externalOnly) == 0 {
			body.WriteString("No external-only residue was detected.\n\n")
		} else {
			for _, entry := range externalOnly {
				body.WriteString(fmt.Sprintf("### %s\n\n", entry.CachePath))
				body.WriteString(fmt.Sprintf("Class: `%s`\n\n", entry.FileClass))
				for _, note := range entry.Notes {
					body.WriteString(fmt.Sprintf("- %s\n", note))
				}
				body.WriteString("\n")
			}
		}

		body.WriteString("## Covered Files\n\n")
		covered := portabilityCoveredEntries(entries)
		if len(covered) == 0 {
			body.WriteString("No explicit sovereign counterparts or citations were detected.\n")
		} else {
			for _, entry := range covered {
				body.WriteString(fmt.Sprintf("### %s\n\n", entry.CachePath))
				body.WriteString(fmt.Sprintf("Coverage: `%s`\n\n", entry.CoverageClass))
				if len(entry.CounterpartPaths) > 0 {
					body.WriteString("Counterparts:\n")
					for _, rel := range entry.CounterpartPaths {
						body.WriteString(fmt.Sprintf("- %s\n", rel))
					}
				}
				if len(entry.CitationPaths) > 0 {
					body.WriteString("Cited by sovereign surfaces:\n")
					for _, rel := range entry.CitationPaths {
						body.WriteString(fmt.Sprintf("- %s\n", rel))
					}
				}
				if len(entry.Notes) > 0 {
					for _, note := range entry.Notes {
						body.WriteString(fmt.Sprintf("- %s\n", note))
					}
				}
				body.WriteString("\n")
			}
		}
	}

	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return PortabilityAuditResult{}, err
	}

	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_portability",
		AuthorityScope: ledger.ScopeRuntimeMemoryPortability,
		ActionClass:    ledger.ActionMemoryPortabilityAudit,
		TargetDomain:   reportPath,
		ResultStatus:   ledger.ResultCompleted,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_portability_audit_written"},
		ProofRefs:      []string{reportPath, markdownPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_portability",
			StaffingContext:    "external_cache_audit",
			AuthorityScope:     ledger.ScopeRuntimeMemoryPortability,
			ArtifactState:      "derived",
			SourceRefs:         []string{reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           report.Signature.ProofRef,
		},
		Metadata: map[string]interface{}{
			"cache_present":  report.CachePresent,
			"cache_path":     report.CachePath,
			"total_files":    report.TotalFiles,
			"covered_files":  report.CoveredFiles,
			"external_only":  report.ExternalOnly,
			"coverage_score": report.CoverageScore,
		},
	})

	return PortabilityAuditResult{
		ReportPath:   reportPath,
		MarkdownPath: markdownPath,
		Report:       report,
	}, nil
}

func (k *Kernel) BuildPortabilityQueue(cacheDir string) (PortabilityQueueResult, error) {
	audit, err := k.AuditPortability(cacheDir)
	if err != nil {
		return PortabilityQueueResult{}, err
	}

	entries := make([]PortabilityQueueEntry, 0, audit.Report.ExternalOnly)
	priorityCounts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}

	for _, entry := range audit.Report.Entries {
		if entry.CoverageClass != portabilityCoverageExternalOnly {
			continue
		}
		queueEntry := PortabilityQueueEntry{
			CachePath:      entry.CachePath,
			FileClass:      entry.FileClass,
			Priority:       portabilityQueuePriority(entry),
			ProposedAction: portabilityQueueAction(entry),
			TargetSurface:  portabilityQueueTarget(entry),
			Reason:         portabilityQueueReason(entry),
		}
		priorityCounts[queueEntry.Priority]++
		entries = append(entries, queueEntry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		pi := portabilityQueuePriorityRank(entries[i].Priority)
		pj := portabilityQueuePriorityRank(entries[j].Priority)
		if pi == pj {
			return entries[i].CachePath < entries[j].CachePath
		}
		return pi < pj
	})

	reportPath := filepath.ToSlash(filepath.Join("state", "memory", "portability", "queue.json"))
	markdownPath := filepath.ToSlash(filepath.Join("state", "memory", "portability", "queue.md"))
	report := PortabilityQueueReport{
		Version:        1,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		SourceAudit:    audit.ReportPath,
		TotalItems:     len(entries),
		PriorityCounts: priorityCounts,
		Entries:        entries,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_portability",
			StaffingContext:    "migration_queue",
			AuthorityScope:     ledger.ScopeRuntimeMemoryPortability,
			ArtifactState:      "derived",
			SourceRefs:         []string{audit.ReportPath, audit.MarkdownPath, reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           "memory-portability-queue:" + time.Now().UTC().Format("20060102T150405Z"),
		}.EnsureTimestamp(),
	}

	jsonAbsPath := filepath.Join(k.Vault.Dir, reportPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, markdownPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return PortabilityQueueResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return PortabilityQueueResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return PortabilityQueueResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_portability_queue",
		"generated_at":        report.GeneratedAt,
		"source_audit":        report.SourceAudit,
		"total_items":         report.TotalItems,
		"priority_counts":     report.PriorityCounts,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Portability Migration Queue\n\n")
	body.WriteString(fmt.Sprintf("Source audit: `%s`\n\n", report.SourceAudit))
	body.WriteString(fmt.Sprintf("Items queued: `%d`\n\n", report.TotalItems))
	body.WriteString("Priority counts:\n")
	body.WriteString(fmt.Sprintf("- critical: `%d`\n", report.PriorityCounts["critical"]))
	body.WriteString(fmt.Sprintf("- high: `%d`\n", report.PriorityCounts["high"]))
	body.WriteString(fmt.Sprintf("- medium: `%d`\n", report.PriorityCounts["medium"]))
	body.WriteString(fmt.Sprintf("- low: `%d`\n\n", report.PriorityCounts["low"]))
	body.WriteString("This queue is read-only. It turns external-only Claude cache residue into explicit migration work without mutating sovereign memory or silently deleting the carrier copy.\n\n")
	if len(entries) == 0 {
		body.WriteString("No unresolved external-only items were found.\n")
	} else {
		for _, entry := range entries {
			body.WriteString(fmt.Sprintf("## %s\n\n", entry.CachePath))
			body.WriteString(fmt.Sprintf("Priority: `%s`\n\n", entry.Priority))
			body.WriteString(fmt.Sprintf("Proposed action: `%s`\n\n", entry.ProposedAction))
			body.WriteString(fmt.Sprintf("Target surface: `%s`\n\n", entry.TargetSurface))
			body.WriteString(fmt.Sprintf("Reason: %s\n\n", entry.Reason))
		}
	}

	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return PortabilityQueueResult{}, err
	}

	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_portability",
		AuthorityScope: ledger.ScopeRuntimeMemoryPortability,
		ActionClass:    ledger.ActionMemoryPortabilityQueue,
		TargetDomain:   reportPath,
		ResultStatus:   ledger.ResultCompleted,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_portability_queue_written"},
		ProofRefs:      []string{audit.ReportPath, audit.MarkdownPath, reportPath, markdownPath},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_portability",
			StaffingContext:    "migration_queue",
			AuthorityScope:     ledger.ScopeRuntimeMemoryPortability,
			ArtifactState:      "derived",
			SourceRefs:         []string{audit.ReportPath, audit.MarkdownPath, reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           report.Signature.ProofRef,
		},
		Metadata: map[string]interface{}{
			"source_audit": audit.ReportPath,
			"total_items":  report.TotalItems,
			"critical":     report.PriorityCounts["critical"],
			"high":         report.PriorityCounts["high"],
			"medium":       report.PriorityCounts["medium"],
			"low":          report.PriorityCounts["low"],
		},
	})

	return PortabilityQueueResult{
		ReportPath:   reportPath,
		MarkdownPath: markdownPath,
		Report:       report,
	}, nil
}

func (k *Kernel) ArchivePortabilityResidue(cacheDir string) (PortabilityArchiveResult, error) {
	resolvedCacheDir, err := resolvePortabilityCacheDir(k.Vault.Dir, cacheDir)
	if err != nil {
		return PortabilityArchiveResult{}, err
	}
	queue, err := k.BuildPortabilityQueue(resolvedCacheDir)
	if err != nil {
		return PortabilityArchiveResult{}, err
	}

	destinationRoot := filepath.ToSlash(filepath.Join("brain", "claude-memory-archive"))
	var archivedPaths []string
	for _, entry := range queue.Report.Entries {
		srcAbs := filepath.Join(resolvedCacheDir, filepath.FromSlash(entry.CachePath))
		destRel := filepath.ToSlash(filepath.Join(destinationRoot, entry.CachePath))
		destAbs := filepath.Join(k.Vault.Dir, filepath.FromSlash(destRel))
		if err := archivePortabilityFile(srcAbs, destAbs, entry); err != nil {
			return PortabilityArchiveResult{}, fmt.Errorf("archive %s: %w", entry.CachePath, err)
		}
		archivedPaths = append(archivedPaths, destRel)
	}

	reportPath := filepath.ToSlash(filepath.Join("state", "memory", "portability", "archive.json"))
	markdownPath := filepath.ToSlash(filepath.Join("state", "memory", "portability", "archive.md"))
	report := PortabilityArchiveReport{
		Version:         1,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		SourceQueue:     queue.ReportPath,
		DestinationRoot: destinationRoot,
		ArchivedCount:   len(archivedPaths),
		ArchivedPaths:   dedupeSortedStrings(archivedPaths),
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_portability",
			StaffingContext:    "archive_external_residue",
			AuthorityScope:     ledger.ScopeRuntimeMemoryPortability,
			ArtifactState:      "derived",
			SourceRefs:         []string{queue.ReportPath, queue.MarkdownPath, reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           "memory-portability-archive:" + time.Now().UTC().Format("20060102T150405Z"),
		}.EnsureTimestamp(),
	}

	jsonAbsPath := filepath.Join(k.Vault.Dir, reportPath)
	mdAbsPath := filepath.Join(k.Vault.Dir, markdownPath)
	if err := os.MkdirAll(filepath.Dir(jsonAbsPath), 0o755); err != nil {
		return PortabilityArchiveResult{}, err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return PortabilityArchiveResult{}, err
	}
	if err := os.WriteFile(jsonAbsPath, append(data, '\n'), 0o644); err != nil {
		return PortabilityArchiveResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_portability_archive",
		"generated_at":        report.GeneratedAt,
		"source_queue":        report.SourceQueue,
		"destination_root":    report.DestinationRoot,
		"archived_count":      report.ArchivedCount,
		"producing_signature": report.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Portability Archive\n\n")
	body.WriteString(fmt.Sprintf("Source queue: `%s`\n\n", report.SourceQueue))
	body.WriteString(fmt.Sprintf("Destination root: `%s`\n\n", report.DestinationRoot))
	body.WriteString(fmt.Sprintf("Archived files: `%d`\n\n", report.ArchivedCount))
	body.WriteString("These files were copied into sovereign archival custody. This does not by itself promote their contents into canonical fact memory; it merely ends carrier-only residence.\n\n")
	for _, rel := range report.ArchivedPaths {
		body.WriteString(fmt.Sprintf("- %s\n", rel))
	}
	body.WriteString("\n")
	if err := markdown.Write(mdAbsPath, mdFrontmatter, body.String()); err != nil {
		return PortabilityArchiveResult{}, err
	}

	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_portability",
		AuthorityScope: ledger.ScopeRuntimeMemoryPortability,
		ActionClass:    ledger.ActionMemoryPortabilityArchive,
		TargetDomain:   reportPath,
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_portability_archive_written"},
		ProofRefs:      append([]string{queue.ReportPath, queue.MarkdownPath, reportPath, markdownPath}, report.ArchivedPaths...),
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_portability",
			StaffingContext:    "archive_external_residue",
			AuthorityScope:     ledger.ScopeRuntimeMemoryPortability,
			ArtifactState:      "derived",
			SourceRefs:         []string{queue.ReportPath, queue.MarkdownPath, reportPath, markdownPath},
			PromotionStatus:    "observed",
			ProofRef:           report.Signature.ProofRef,
		},
		Metadata: map[string]interface{}{
			"source_queue":     queue.ReportPath,
			"destination_root": destinationRoot,
			"archived_count":   report.ArchivedCount,
		},
	})

	return PortabilityArchiveResult{
		ReportPath:   reportPath,
		MarkdownPath: markdownPath,
		Report:       report,
	}, nil
}

func defaultClaudeProjectMemoryDir(vaultDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	projectRoot := filepath.Dir(vaultDir)
	projectKey := strings.TrimPrefix(filepath.Clean(projectRoot), string(os.PathSeparator))
	projectKey = "-" + strings.ReplaceAll(projectKey, string(os.PathSeparator), "-")
	return filepath.Join(home, ".claude", "projects", projectKey, "memory")
}

func resolvePortabilityCacheDir(vaultDir, cacheDir string) (string, error) {
	cacheDir = strings.TrimSpace(cacheDir)
	if cacheDir == "" {
		cacheDir = defaultClaudeProjectMemoryDir(vaultDir)
	}
	if cacheDir == "" {
		return "", fmt.Errorf("could not derive Claude project memory directory")
	}
	return filepath.Clean(cacheDir), nil
}

func scanPortabilityCacheFiles(cacheDir string) ([]portabilityCacheFile, bool, error) {
	info, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("cache path is not a directory: %s", cacheDir)
	}

	var files []portabilityCacheFile
	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(cacheDir, path)
		if err != nil {
			return err
		}
		files = append(files, portabilityCacheFile{
			RelPath: filepath.ToSlash(rel),
			Base:    filepath.Base(path),
		})
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files, true, nil
}

func scanVaultFileBasenameIndex(vaultDir string) (map[string][]string, error) {
	index := make(map[string][]string)
	err := filepath.Walk(vaultDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(vaultDir, path)
		if err != nil {
			return err
		}
		key := strings.ToLower(filepath.Base(path))
		index[key] = append(index[key], filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	for key, paths := range index {
		index[key] = dedupeSortedStrings(paths)
	}
	return index, nil
}

func (k *Kernel) buildPortabilityCitationIndex(cacheFiles []portabilityCacheFile) (map[string][]string, error) {
	index := make(map[string][]string)
	if len(cacheFiles) == 0 {
		return index, nil
	}

	roots := []string{
		filepath.Join(k.Vault.Dir, "memory", "facts"),
		filepath.Join(k.Vault.Dir, "memory", "episodes"),
		filepath.Join(k.Vault.Dir, "memory", "recalls"),
		filepath.Join(k.Vault.Dir, "memory", "maintenance"),
		filepath.Join(k.Vault.Dir, "sessions"),
	}
	var docs []*markdown.Document
	for _, root := range roots {
		scanned, err := markdown.ScanDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		docs = append(docs, scanned...)
	}

	for _, doc := range docs {
		frontmatterStrings := collectPortabilityFrontmatterStrings(doc.Frontmatter)
		if len(frontmatterStrings) == 0 {
			continue
		}
		docRel := relativeVaultPath(k.Vault.Dir, doc.Path)
		for _, cacheFile := range cacheFiles {
			cacheKey := strings.ToLower(cacheFile.RelPath)
			baseKey := strings.ToLower(cacheFile.Base)
			for _, candidate := range frontmatterStrings {
				candidate = strings.ToLower(candidate)
				if strings.Contains(candidate, cacheKey) || strings.Contains(candidate, baseKey) {
					index[cacheKey] = append(index[cacheKey], docRel)
					break
				}
			}
		}
	}
	for key, paths := range index {
		index[key] = dedupeSortedStrings(paths)
	}
	return index, nil
}

func collectPortabilityFrontmatterStrings(value interface{}) []string {
	var values []string
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, nested := range typed {
			values = append(values, collectPortabilityFrontmatterStrings(nested)...)
		}
	case []interface{}:
		for _, nested := range typed {
			values = append(values, collectPortabilityFrontmatterStrings(nested)...)
		}
	case []string:
		values = append(values, typed...)
	case string:
		if strings.TrimSpace(typed) != "" {
			values = append(values, typed)
		}
	}
	return values
}

func classifyPortabilityFile(relPath string) string {
	base := strings.ToLower(filepath.Base(relPath))
	switch {
	case base == "memory.md":
		return "root_memory"
	case strings.HasPrefix(base, "continuity_"):
		return "continuity"
	case strings.HasPrefix(base, "session_prep_"):
		return "session_prep"
	case strings.HasPrefix(base, "feedback_"):
		return "feedback"
	case strings.HasPrefix(base, "user_"):
		return "user"
	case strings.HasPrefix(base, "project_"):
		return "project"
	case strings.HasPrefix(base, "reference_"):
		return "reference"
	case strings.HasPrefix(base, "."):
		return "runtime_state"
	default:
		return "other"
	}
}

func resolveExplicitPortabilityCounterparts(vaultDir, relPath string) ([]string, []string) {
	counterparts, notes := explicitPortabilityCounterparts(relPath)
	if len(counterparts) == 0 {
		return nil, nil
	}
	var existing []string
	for _, rel := range counterparts {
		if _, err := os.Stat(explicitPortabilityAbsPath(vaultDir, rel)); err == nil {
			existing = append(existing, rel)
		}
	}
	if len(existing) == 0 {
		return nil, nil
	}
	return dedupeSortedStrings(existing), notes
}

func explicitPortabilityAbsPath(vaultDir, rel string) string {
	repoRoot := filepath.Dir(vaultDir)
	switch {
	case strings.HasPrefix(rel, "data/"), strings.HasPrefix(rel, "logs/"):
		return filepath.Join(repoRoot, filepath.FromSlash(rel))
	default:
		return filepath.Join(vaultDir, filepath.FromSlash(rel))
	}
}

func explicitPortabilityCounterparts(relPath string) ([]string, []string) {
	base := strings.ToLower(filepath.Base(relPath))
	switch {
	case base == "memory.md":
		return []string{"state/memory/portability/MEMORY.md"}, []string{"Carrier root memory has an explicit sovereign supersession note naming the canonical memory surfaces."}
	case base == "continuity_session_journal.md":
		return []string{"sessions/journal.md"}, []string{"Claude continuity journal has an explicit sovereign counterpart in the active session journal."}
	case strings.HasPrefix(base, "continuity_session_journal_archive_"):
		return []string{"sessions/journal.md"}, []string{"Archived Claude continuity journals are superseded by the sovereign session journal and project journals."}
	case strings.HasPrefix(base, "session_prep_") && strings.HasSuffix(base, ".md"):
		return []string{"data/session-prep.md"}, []string{"Session prep now lives as a derived runtime surface rather than a Claude-side cache file."}
	default:
		return nil, nil
	}
}

func portabilityExactCounterparts(base string, index map[string][]string) []string {
	key := strings.ToLower(strings.TrimSpace(base))
	if key == "" || portabilityAmbiguousBasename(key) {
		return nil
	}
	return dedupeSortedStrings(index[key])
}

func portabilityAmbiguousBasename(base string) bool {
	switch base {
	case "memory.md":
		return true
	default:
		return false
	}
}

func portabilityExternalOnlyNotes(relPath string) []string {
	base := strings.ToLower(filepath.Base(relPath))
	switch {
	case base == "memory.md":
		return []string{"Root Claude project memory still has no one-to-one sovereign counterpart. This remains a live portability risk until its contents are explicitly superseded or migrated."}
	case strings.HasPrefix(base, "feedback_"):
		return []string{"Feedback law is still sitting in the Claude cache without an explicit sovereign counterpart or citation trail."}
	case strings.HasPrefix(base, "user_"), strings.HasPrefix(base, "project_"), strings.HasPrefix(base, "reference_"):
		return []string{"This operator context file remains external until it is mirrored, cited, or superseded inside the sovereign vault."}
	default:
		return []string{"No explicit sovereign counterpart or provenance-bearing citation was detected."}
	}
}

func portabilityCoveragePriority(class string) int {
	switch class {
	case portabilityCoverageExternalOnly:
		return 0
	case portabilityCoverageExplicitEquivalent:
		return 1
	case portabilityCoverageExactCounterpart:
		return 2
	case portabilityCoverageCited:
		return 3
	default:
		return 4
	}
}

func portabilityEntriesByCoverage(entries []PortabilityAuditEntry, coverage string) []PortabilityAuditEntry {
	filtered := make([]PortabilityAuditEntry, 0)
	for _, entry := range entries {
		if entry.CoverageClass == coverage {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func portabilityCoveredEntries(entries []PortabilityAuditEntry) []PortabilityAuditEntry {
	filtered := make([]PortabilityAuditEntry, 0)
	for _, entry := range entries {
		if entry.CoverageClass != portabilityCoverageExternalOnly {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func portabilityQueuePriority(entry PortabilityAuditEntry) string {
	base := strings.ToLower(filepath.Base(entry.CachePath))
	switch entry.FileClass {
	case "root_memory":
		return "critical"
	case "feedback", "user", "reference":
		return "high"
	case "project":
		if portabilityStrategicProject(base) {
			return "high"
		}
		return "medium"
	case "runtime_state":
		return "medium"
	default:
		return "low"
	}
}

func portabilityQueuePriorityRank(priority string) int {
	switch priority {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func portabilityQueueAction(entry PortabilityAuditEntry) string {
	switch entry.FileClass {
	case "root_memory":
		return "supersede_root_carrier_memory"
	case "feedback":
		return "promote_feedback_law"
	case "user":
		return "promote_user_law_or_memory"
	case "reference":
		return "mirror_reference_into_vault"
	case "project":
		return "mirror_project_context_into_vault"
	case "runtime_state":
		return "replace_carrier_runtime_state"
	default:
		return "review_for_sovereign_home"
	}
}

func portabilityQueueTarget(entry PortabilityAuditEntry) string {
	switch entry.FileClass {
	case "root_memory":
		return "vault/sessions/journal.md plus sovereign memory artifacts"
	case "feedback":
		return "vault/modus/implementation-mandates.md or adjacent doctrine"
	case "user":
		return "vault/memory/facts and related sovereign user doctrine"
	case "reference":
		return "vault/brain or sovereign reference notes with provenance"
	case "project":
		return "vault/missions or sovereign project notes with provenance"
	case "runtime_state":
		return "vault/state or runtime-owned data surfaces"
	default:
		return "sovereign vault surface to be chosen explicitly"
	}
}

func portabilityQueueReason(entry PortabilityAuditEntry) string {
	switch entry.FileClass {
	case "root_memory":
		return "The root Claude project memory index still exists as an external source of truth and must be explicitly superseded before carrier dependence can be considered closed."
	case "feedback":
		return "Feedback law should not survive only in the carrier cache. It belongs in sovereign doctrine or an explicit continuity surface."
	case "user":
		return "User-law and bond memory should live in sovereign memory and doctrine rather than in the carrier cache."
	case "reference":
		return "Operational reference material remains external and therefore fragile across carrier changes."
	case "project":
		return "Project memory that remains external cannot yet be counted as sovereign project continuity."
	case "runtime_state":
		return "Carrier-side runtime state should not be relied upon once equivalent sovereign runtime surfaces exist."
	default:
		return "This cache file remains external-only and needs an explicit sovereign home or explicit retirement."
	}
}

func portabilityStrategicProject(base string) bool {
	switch base {
	case "project_modus_os_go.md",
		"project_web_console.md",
		"project_wiki_loop.md",
		"project_mcp_revenue.md",
		"project_mcp_memory_server_launch.md",
		"project_homefront_ai_build.md",
		"project_homefront_v11_ai.md",
		"project_homefront_accessibility.md":
		return true
	default:
		return false
	}
}

func archivePortabilityFile(srcAbs, destAbs string, entry PortabilityQueueEntry) error {
	doc, err := markdown.Parse(srcAbs)
	if err == nil {
		fm := make(map[string]interface{}, len(doc.Frontmatter)+6)
		for key, value := range doc.Frontmatter {
			fm[key] = value
		}
		if strings.TrimSpace(fmt.Sprintf("%v", fm["title"])) == "" && strings.TrimSpace(fmt.Sprintf("%v", fm["name"])) != "" {
			fm["title"] = fmt.Sprintf("%v", fm["name"])
		}
		if strings.TrimSpace(fmt.Sprintf("%v", fm["title"])) == "" {
			fm["title"] = filepath.Base(srcAbs)
		}
		existingType := ""
		if value, ok := fm["type"]; ok {
			existingType = strings.TrimSpace(fmt.Sprintf("%v", value))
		}
		fm["type"] = firstNonEmptyString(existingType, "claude_carrier_archive")
		fm["carrier_source_ref"] = srcAbs
		fm["carrier_archived_at"] = time.Now().UTC().Format(time.RFC3339)
		fm["carrier_portability_priority"] = entry.Priority
		fm["carrier_portability_action"] = entry.ProposedAction
		fm["carrier_portability_status"] = "archived"
		return markdown.Write(destAbs, fm, doc.Body)
	}

	data, readErr := os.ReadFile(srcAbs)
	if readErr != nil {
		return readErr
	}
	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destAbs, data, 0o644)
}

func dedupeSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	var deduped []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	sort.Strings(deduped)
	return deduped
}

func relativeVaultPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

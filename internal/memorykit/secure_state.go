package memorykit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GetModus/modus-memory/internal/ledger"
	"github.com/GetModus/modus-memory/internal/markdown"
	"github.com/GetModus/modus-memory/internal/signature"
	"github.com/GetModus/modus-memory/internal/vault"
)

var ErrSealedMemoryUnavailable = errors.New("sealed memory requires payload protection and is not yet available in the plaintext vault")

type SecureStateEntry struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	SecurityClass string `json:"security_class"`
	ContentHash   string `json:"content_hash"`
}

type SecureStateManifest struct {
	Version          int                 `json:"version"`
	GeneratedAt      string              `json:"generated_at"`
	Generation       int                 `json:"generation"`
	RootHash         string              `json:"root_hash"`
	PreviousRootHash string              `json:"previous_root_hash,omitempty"`
	FileCount        int                 `json:"file_count"`
	ClassCounts      map[string]int      `json:"class_counts"`
	Entries          []SecureStateEntry  `json:"entries"`
	Signature        signature.Signature `json:"signature"`
}

type SecureStateManifestResult struct {
	ManifestPath string
	MarkdownPath string
	Manifest     SecureStateManifest
}

type SecureStateVerification struct {
	ManifestPath      string
	MarkdownPath      string
	ExpectedRootHash  string
	CurrentRootHash   string
	LedgerRootHash    string
	Verified          bool
	RollbackSuspected bool
	DriftPaths        []string
	CurrentFileCount  int
	ExpectedFileCount int
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func deriveKernelFactSecurityClass(importance string, auth vault.FactWriteAuthority) string {
	if strings.TrimSpace(auth.MemorySecurityClass) != "" {
		switch strings.ToLower(strings.TrimSpace(auth.MemorySecurityClass)) {
		case "sealed":
			return "sealed"
		case "canonical":
			return "canonical"
		default:
			return "operational"
		}
	}
	if strings.EqualFold(strings.TrimSpace(auth.MemoryProtectionClass), "elder") {
		return "canonical"
	}
	if strings.EqualFold(strings.TrimSpace(importance), "critical") {
		return "canonical"
	}
	if strings.TrimSpace(auth.Mission) != "" && strings.EqualFold(strings.TrimSpace(importance), "high") {
		return "canonical"
	}
	return "operational"
}

func deriveKernelEpisodeSecurityClass(auth vault.EpisodeWriteAuthority) string {
	if strings.TrimSpace(auth.MemorySecurityClass) != "" {
		switch strings.ToLower(strings.TrimSpace(auth.MemorySecurityClass)) {
		case "sealed":
			return "sealed"
		case "canonical":
			return "canonical"
		default:
			return "operational"
		}
	}
	if strings.TrimSpace(auth.Mission) != "" || strings.TrimSpace(auth.WorkItemID) != "" {
		return "canonical"
	}
	switch strings.ToLower(strings.TrimSpace(auth.EventKind)) {
	case "decision", "correction":
		return "canonical"
	default:
		return "operational"
	}
}

func (k *Kernel) WriteSecureStateManifest() (SecureStateManifestResult, error) {
	entries, err := k.scanSecureStateEntries()
	if err != nil {
		return SecureStateManifestResult{}, err
	}

	rootHash := secureStateRootHash(entries)
	classCounts := map[string]int{"operational": 0, "canonical": 0, "sealed": 0}
	for _, entry := range entries {
		classCounts[entry.SecurityClass]++
	}

	jsonPath := filepath.Join(k.Vault.Dir, "state", "memory", "latest.json")
	mdPath := filepath.Join(k.Vault.Dir, "state", "memory", "latest.md")
	previousRoot := ""
	generation := 1
	if existing, err := readSecureStateManifest(jsonPath); err == nil {
		previousRoot = existing.RootHash
		generation = existing.Generation + 1
	}

	manifest := SecureStateManifest{
		Version:          1,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Generation:       generation,
		RootHash:         rootHash,
		PreviousRootHash: previousRoot,
		FileCount:        len(entries),
		ClassCounts:      classCounts,
		Entries:          entries,
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_secure_state",
			StaffingContext:    "manifest_write",
			AuthorityScope:     ledger.ScopeRuntimeMemoryStateManifest,
			ArtifactState:      "canonical",
			SourceRefs:         []string{"memory/facts", "memory/episodes", "memory/recalls", "memory/maintenance"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-state-manifest:" + rootHash,
		}.EnsureTimestamp(),
	}

	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		return SecureStateManifestResult{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return SecureStateManifestResult{}, err
	}
	if err := os.WriteFile(jsonPath, append(data, '\n'), 0o644); err != nil {
		return SecureStateManifestResult{}, err
	}

	mdFrontmatter := map[string]interface{}{
		"type":                "memory_secure_state_manifest",
		"generated_at":        manifest.GeneratedAt,
		"generation":          manifest.Generation,
		"root_hash":           manifest.RootHash,
		"previous_root_hash":  manifest.PreviousRootHash,
		"file_count":          manifest.FileCount,
		"class_counts":        manifest.ClassCounts,
		"producing_signature": manifest.Signature,
	}
	var body strings.Builder
	body.WriteString("# Memory Secure State Manifest\n\n")
	body.WriteString(fmt.Sprintf("Generation: `%d`\n\n", manifest.Generation))
	body.WriteString(fmt.Sprintf("Root hash: `%s`\n\n", manifest.RootHash))
	body.WriteString(fmt.Sprintf("Files covered: `%d`\n\n", manifest.FileCount))
	body.WriteString("Protection classes:\n")
	body.WriteString(fmt.Sprintf("- operational: `%d`\n", manifest.ClassCounts["operational"]))
	body.WriteString(fmt.Sprintf("- canonical: `%d`\n", manifest.ClassCounts["canonical"]))
	body.WriteString(fmt.Sprintf("- sealed: `%d`\n\n", manifest.ClassCounts["sealed"]))
	body.WriteString("This manifest is a rollback and drift checkpoint over `memory/facts`, `memory/episodes`, `memory/recalls`, and `memory/maintenance`.\n")
	if err := markdown.Write(mdPath, mdFrontmatter, body.String()); err != nil {
		return SecureStateManifestResult{}, err
	}

	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_secure_state",
		AuthorityScope: ledger.ScopeRuntimeMemoryStateManifest,
		ActionClass:    ledger.ActionMemoryStateManifestWrite,
		TargetDomain:   "state/memory/latest.json",
		ResultStatus:   ledger.ResultApplied,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_state_manifest_written"},
		ProofRefs:      []string{"state/memory/latest.json", "state/memory/latest.md"},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_secure_state",
			StaffingContext:    "manifest_write",
			AuthorityScope:     ledger.ScopeRuntimeMemoryStateManifest,
			ArtifactState:      "canonical",
			SourceRefs:         []string{"state/memory/latest.json", "state/memory/latest.md"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-state-manifest:" + rootHash,
		},
		Metadata: map[string]interface{}{
			"generation":   manifest.Generation,
			"root_hash":    manifest.RootHash,
			"file_count":   manifest.FileCount,
			"class_counts": manifest.ClassCounts,
		},
	})

	return SecureStateManifestResult{
		ManifestPath: "state/memory/latest.json",
		MarkdownPath: "state/memory/latest.md",
		Manifest:     manifest,
	}, nil
}

func (k *Kernel) VerifySecureStateManifest() (SecureStateVerification, error) {
	jsonPath := filepath.Join(k.Vault.Dir, "state", "memory", "latest.json")
	manifest, err := readSecureStateManifest(jsonPath)
	if err != nil {
		return SecureStateVerification{}, err
	}

	entries, err := k.scanSecureStateEntries()
	if err != nil {
		return SecureStateVerification{}, err
	}
	currentRoot := secureStateRootHash(entries)
	drift := secureStateDrift(manifest.Entries, entries)
	ledgerRoot := latestManifestRootHashFromLedger(k.Vault.Dir)
	rollback := ledgerRoot != "" && ledgerRoot != manifest.RootHash

	result := SecureStateVerification{
		ManifestPath:      "state/memory/latest.json",
		MarkdownPath:      "state/memory/latest.md",
		ExpectedRootHash:  manifest.RootHash,
		CurrentRootHash:   currentRoot,
		LedgerRootHash:    ledgerRoot,
		Verified:          currentRoot == manifest.RootHash && !rollback,
		RollbackSuspected: rollback,
		DriftPaths:        drift,
		CurrentFileCount:  len(entries),
		ExpectedFileCount: manifest.FileCount,
	}

	status := ledger.ResultCompleted
	if !result.Verified {
		status = ledger.ResultFailed
	}
	_ = ledger.Append(k.Vault.Dir, ledger.Record{
		Office:         "memory_governance",
		Subsystem:      "memory_secure_state",
		AuthorityScope: ledger.ScopeRuntimeMemoryStateManifest,
		ActionClass:    ledger.ActionMemoryStateManifestVerify,
		TargetDomain:   "state/memory/latest.json",
		ResultStatus:   status,
		Decision:       ledger.DecisionAllowedWithProof,
		SideEffects:    []string{"memory_state_manifest_verified"},
		ProofRefs:      []string{"state/memory/latest.json"},
		Signature: signature.Signature{
			ProducingOffice:    "memory_governance",
			ProducingSubsystem: "memory_secure_state",
			StaffingContext:    "manifest_verify",
			AuthorityScope:     ledger.ScopeRuntimeMemoryStateManifest,
			ArtifactState:      "derived",
			SourceRefs:         []string{"state/memory/latest.json"},
			PromotionStatus:    "observed",
			ProofRef:           "memory-state-verify:" + manifest.RootHash,
		},
		Metadata: map[string]interface{}{
			"expected_root_hash": manifest.RootHash,
			"current_root_hash":  currentRoot,
			"ledger_root_hash":   ledgerRoot,
			"rollback_suspected": rollback,
			"drift_count":        len(drift),
		},
	})

	return result, nil
}

func (k *Kernel) scanSecureStateEntries() ([]SecureStateEntry, error) {
	type scanTarget struct {
		Subdir        string
		DefaultKind   string
		DefaultClass  string
		ClassResolver func(*markdown.Document) string
	}
	targets := []scanTarget{
		{
			Subdir:       "memory/facts",
			DefaultKind:  "fact",
			DefaultClass: "operational",
			ClassResolver: func(doc *markdown.Document) string {
				return effectiveManifestClass(doc.Get("memory_security_class"), doc.Get("memory_protection_class"), doc.Get("importance"))
			},
		},
		{
			Subdir:       "memory/episodes",
			DefaultKind:  "episode",
			DefaultClass: "operational",
			ClassResolver: func(doc *markdown.Document) string {
				return effectiveManifestClass(doc.Get("memory_security_class"), "", "")
			},
		},
		{
			Subdir:       "memory/recalls",
			DefaultKind:  "recall",
			DefaultClass: "operational",
			ClassResolver: func(doc *markdown.Document) string {
				return effectiveManifestClass(doc.Get("memory_security_class"), "", "")
			},
		},
		{
			Subdir:       "memory/maintenance",
			DefaultKind:  "maintenance",
			DefaultClass: "canonical",
			ClassResolver: func(doc *markdown.Document) string {
				return effectiveManifestClass(doc.Get("memory_security_class"), "elder", "critical")
			},
		},
	}

	var entries []SecureStateEntry
	for _, target := range targets {
		docs, err := markdown.ScanDir(k.Vault.Path(strings.Split(target.Subdir, "/")...))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, doc := range docs {
			raw, err := os.ReadFile(doc.Path)
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(k.Vault.Dir, doc.Path)
			if err != nil {
				return nil, err
			}
			kind := strings.TrimSpace(doc.Get("type"))
			if kind == "" {
				kind = target.DefaultKind
			}
			entries = append(entries, SecureStateEntry{
				Path:          filepath.ToSlash(rel),
				Kind:          kind,
				SecurityClass: firstNonEmptyString(target.ClassResolver(doc), target.DefaultClass),
				ContentHash:   hashBytes(raw),
			})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func effectiveManifestClass(requested, protectionClass, importance string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		switch strings.ToLower(requested) {
		case "sealed":
			return "sealed"
		case "canonical":
			return "canonical"
		default:
			return "operational"
		}
	}
	if strings.EqualFold(strings.TrimSpace(protectionClass), "elder") || strings.EqualFold(strings.TrimSpace(importance), "critical") {
		return "canonical"
	}
	return "operational"
}

func secureStateRootHash(entries []SecureStateEntry) string {
	h := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(h, "%s|%s|%s|%s\n", entry.Path, entry.Kind, entry.SecurityClass, entry.ContentHash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readSecureStateManifest(path string) (SecureStateManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SecureStateManifest{}, err
	}
	var manifest SecureStateManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return SecureStateManifest{}, err
	}
	return manifest, nil
}

func secureStateDrift(expected, current []SecureStateEntry) []string {
	expectedMap := make(map[string]SecureStateEntry, len(expected))
	currentMap := make(map[string]SecureStateEntry, len(current))
	for _, entry := range expected {
		expectedMap[entry.Path] = entry
	}
	for _, entry := range current {
		currentMap[entry.Path] = entry
	}
	paths := make(map[string]bool)
	for path, entry := range expectedMap {
		cur, ok := currentMap[path]
		if !ok || cur.ContentHash != entry.ContentHash || cur.SecurityClass != entry.SecurityClass {
			paths[path] = true
		}
	}
	for path, entry := range currentMap {
		exp, ok := expectedMap[path]
		if !ok || exp.ContentHash != entry.ContentHash || exp.SecurityClass != entry.SecurityClass {
			paths[path] = true
		}
	}
	var drift []string
	for path := range paths {
		drift = append(drift, path)
	}
	sort.Strings(drift)
	return drift
}

func latestManifestRootHashFromLedger(vaultDir string) string {
	path := filepath.Join(vaultDir, "state", "operations", "operations.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var record struct {
			ActionClass string                 `json:"action_class"`
			Metadata    map[string]interface{} `json:"metadata"`
		}
		if err := json.Unmarshal([]byte(lines[i]), &record); err != nil {
			continue
		}
		if record.ActionClass != ledger.ActionMemoryStateManifestWrite {
			continue
		}
		if value, ok := record.Metadata["root_hash"].(string); ok {
			return value
		}
	}
	return ""
}

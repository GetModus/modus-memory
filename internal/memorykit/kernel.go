package memorykit

import (
	"context"

	"github.com/GetModus/modus-memory/internal/vault"
)

// Attachment is the direct harness-facing memory contract. Adapters can call
// this interface without knowing whether the backing store is exposed through a
// tool surface, a local database, or both.
type Attachment interface {
	StoreEpisode(body string, auth vault.EpisodeWriteAuthority) (string, string, error)
	StoreFact(subject, predicate, value string, confidence float64, importance string, auth vault.FactWriteAuthority) (string, error)
	Recall(req RecallRequest) (RecallResult, error)
	SearchFacts(query string, limit int, opts vault.FactSearchOptions) ([]string, error)
	HotContext(query string, limit int) ([]string, error)
	RunTrials() (TrialReportResult, error)
	RunReadiness() (ReadinessReportResult, error)
	AuditCarriers() (CarrierAuditResult, error)
	ProbeCarriers(context.Context, CarrierProbeOptions) (CarrierProbeResult, error)
	AuditPortability(cacheDir string) (PortabilityAuditResult, error)
	BuildPortabilityQueue(cacheDir string) (PortabilityQueueResult, error)
	ArchivePortabilityResidue(cacheDir string) (PortabilityArchiveResult, error)
	Evaluate() (EvaluationReportResult, error)
	WriteSecureStateManifest() (SecureStateManifestResult, error)
	VerifySecureStateManifest() (SecureStateVerification, error)
}

type RecallRequest = vault.RecallRequest
type RecallResult = vault.RecallResult

// Kernel is the sovereign local memory core. Tool surfaces such as MCP, the
// agent registry, and the TUI should wrap this rather than owning memory
// behavior themselves.
type Kernel struct {
	Vault *vault.Vault
}

func New(v *vault.Vault) *Kernel {
	return &Kernel{Vault: v}
}

func (k *Kernel) StoreEpisode(body string, auth vault.EpisodeWriteAuthority) (string, string, error) {
	if auth.MemorySecurityClass == "sealed" {
		return "", "", ErrSealedMemoryUnavailable
	}
	auth.MemorySecurityClass = deriveKernelEpisodeSecurityClass(auth)
	return k.Vault.StoreEpisodeGoverned(body, auth)
}

func (k *Kernel) StoreFact(subject, predicate, value string, confidence float64, importance string, auth vault.FactWriteAuthority) (string, error) {
	if auth.MemorySecurityClass == "sealed" {
		return "", ErrSealedMemoryUnavailable
	}
	auth.MemorySecurityClass = deriveKernelFactSecurityClass(importance, auth)
	return k.Vault.StoreFactGoverned(subject, predicate, value, confidence, importance, auth)
}

func (k *Kernel) Recall(req RecallRequest) (RecallResult, error) {
	return k.Vault.RecallFacts(vault.RecallRequest(req))
}

func (k *Kernel) SearchFacts(query string, limit int, opts vault.FactSearchOptions) ([]string, error) {
	result, err := k.Recall(RecallRequest{
		Query:              query,
		Limit:              limit,
		Options:            opts,
		Harness:            "memorykit",
		Adapter:            "kernel",
		Mode:               "manual_search",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "memory_kernel",
		StaffingContext:    "direct_kernel",
	})
	if err != nil {
		return nil, err
	}
	return result.Lines, nil
}

func (k *Kernel) HotContext(query string, limit int) ([]string, error) {
	result, err := k.Recall(RecallRequest{
		Query:              query,
		Limit:              limit,
		Options:            vault.FactSearchOptions{MemoryTemperature: "hot"},
		Harness:            "memorykit",
		Adapter:            "kernel",
		Mode:               "automatic_hot_admission",
		ProducingOffice:    "librarian",
		ProducingSubsystem: "memory_kernel",
		StaffingContext:    "hot_context",
	})
	if err != nil {
		return nil, err
	}
	return result.Lines, nil
}

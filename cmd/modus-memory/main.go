// modus-memory is a standalone sovereign memory kernel and MCP server.
//
// A single Go binary that provides local-first agent memory over the MCP
// protocol and a shell-first governance surface. Route-aware retrieval,
// episodic identity, recall receipts, governed review, portability audit,
// readiness reporting, and synthetic plus live evaluation all live in the
// same runtime.
//
// Usage:
//
//	modus-memory                    — start MCP server on stdio
//	modus-memory --vault ~/notes    — use custom vault directory
//	modus-memory version            — print version
//	modus-memory health             — check vault health
//	modus-memory attach --carrier <codex|claude|qwen|gemini|ollama|hermes|openclaw|opencode> --prompt "..." [--target ...] — run a carrier through sovereign memory attachment
//	modus-memory propose-hot --fact-path <memory/facts/...> --temperature <hot|warm> --reason "..." — create a governed hot-tier review artifact
//	modus-memory propose-structural --fact-path <memory/facts/...> [--related-fact <path>] [--related-episode <path>] [--related-entity <name>] [--related-mission <name>] --reason "..." — create a governed structural-link review artifact
//	modus-memory propose-temporal --fact-path <memory/facts/...> --status <active|superseded|expired> --reason "..." [--observed-at ...] [--valid-from ...] [--valid-to ...] [--superseded-by <memory/facts/...>] — create a governed temporal review artifact
//	modus-memory propose-elder --fact-path <memory/facts/...> --protection-class <elder|ordinary> --reason "..." — create a governed elder-memory review artifact
//	modus-memory review-queue [--status <pending|approved|rejected|applied|all>] [--limit <n>] [--json] — inspect governed memory review debt
//	modus-memory resolve-review [--status <pending|approved|rejected|applied|all>] [--type <candidate_...>] [--review-class <class>] [--fact-path <memory/facts/...>] --set-status <approved|rejected> --reason "..." [--limit <n>] [--json] — resolve governed review artifacts explicitly
//	modus-memory carrier-audit [--json] — inspect local carrier and wrapper readiness without live execution
//	modus-memory carrier-probe --carriers <codex,qwen,...> [--prompt "..."] [--json] — run live sovereign attachment probes against selected carriers
//	modus-memory import khoj <file> — import from Khoj export (ZIP or JSON)
//	modus-memory activate <key>    — activate Pro license
//	modus-memory refresh           — re-validate Pro license
//	modus-memory deactivate        — remove Pro license
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GetModus/modus-memory/internal/index"
	"github.com/GetModus/modus-memory/internal/librarian"
	"github.com/GetModus/modus-memory/internal/llamacpp"
	mcpsrv "github.com/GetModus/modus-memory/internal/mcp"
	"github.com/GetModus/modus-memory/internal/memorycli"
	"github.com/GetModus/modus-memory/internal/memorykit"
	"github.com/GetModus/modus-memory/internal/vault"
)

const version = "0.5.0"

func main() {
	// Parse flags
	vaultDir := ""
	modelPath := ""
	gpuLayers := -1 // -1 = offload all
	modelCtx := 0   // 0 = model default
	noLLM := false
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--vault", "-v":
			if i+1 < len(args) {
				vaultDir = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				modelPath = args[i+1]
				i++
			}
		case "--gpu-layers":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					gpuLayers = n
				}
				i++
			}
		case "--ctx":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					modelCtx = n
				}
				i++
			}
		case "--no-llm":
			noLLM = true
		case "version", "--version":
			fmt.Printf("modus-memory %s\n", version)
			os.Exit(0)
		case "health":
			vd := resolveVaultDir(vaultDir)
			runHealth(vd)
			os.Exit(0)
		case "attach":
			vd := resolveVaultDir(vaultDir)
			runAttach(vd, args[i+1:])
			os.Exit(0)
		case "propose-hot":
			vd := resolveVaultDir(vaultDir)
			runProposeHot(vd, args[i+1:])
			os.Exit(0)
		case "propose-structural":
			vd := resolveVaultDir(vaultDir)
			runProposeStructural(vd, args[i+1:])
			os.Exit(0)
		case "propose-temporal":
			vd := resolveVaultDir(vaultDir)
			runProposeTemporal(vd, args[i+1:])
			os.Exit(0)
		case "propose-elder":
			vd := resolveVaultDir(vaultDir)
			runProposeElder(vd, args[i+1:])
			os.Exit(0)
		case "review-queue":
			vd := resolveVaultDir(vaultDir)
			runReviewQueue(vd, args[i+1:])
			os.Exit(0)
		case "resolve-review":
			vd := resolveVaultDir(vaultDir)
			runResolveReview(vd, args[i+1:])
			os.Exit(0)
		case "carrier-audit":
			vd := resolveVaultDir(vaultDir)
			runCarrierAudit(vd, args[i+1:])
			os.Exit(0)
		case "carrier-probe":
			vd := resolveVaultDir(vaultDir)
			runCarrierProbe(vd, args[i+1:])
			os.Exit(0)
		case "import":
			vd := resolveVaultDir(vaultDir)
			runImport(vd, args[i+1:])
			os.Exit(0)
		case "activate":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Usage: modus-memory activate <license-key>")
				os.Exit(1)
			}
			if err := activateLicense(args[i+1]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "deactivate":
			if err := deactivateLicense(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "refresh":
			if err := refreshLicense(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		case "status":
			lic := loadLicense()
			if lic.valid {
				fmt.Printf("Tier: %s\n", lic.tier)
				if lic.state != nil {
					fmt.Printf("Email: %s\n", lic.state.Email)
					if lic.state.ExpiresAt != "" {
						fmt.Printf("Renews: %s\n", lic.state.ExpiresAt)
					}
					fmt.Printf("Validated: %s\n", lic.state.ValidatedAt)
				}
			} else {
				fmt.Printf("Tier: free\n")
				fmt.Printf("Reason: %s\n", lic.reason)
				fmt.Printf("\nUpgrade: https://modus-memory.lemonsqueezy.com\n")
			}
			os.Exit(0)
		}
	}

	vaultDir = resolveVaultDir(vaultDir)

	// Ensure vault directory exists
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create vault directory %s: %v\n", vaultDir, err)
		os.Exit(1)
	}

	// Ensure core subdirectories exist
	for _, sub := range []string{"memory/facts", "memory/corrections", "memory/traces", "memory/fsrs-tuning", "memory/maintenance", "memory/training-runs", "memory/training-data", "brain", "atlas"} {
		os.MkdirAll(filepath.Join(vaultDir, sub), 0755)
	}

	// Redirect log output to stderr (stdout is MCP protocol)
	log.SetOutput(os.Stderr)
	log.SetPrefix("[modus-memory] ")

	// Check license
	lic := loadLicense()
	isPro := lic.valid && lic.tier == TierPro

	// Build search index
	idx, err := index.Build(vaultDir, "")
	if err != nil {
		log.Printf("Warning: index build failed: %v (starting with empty index)", err)
	}

	// Enforce free tier doc limit
	if !isPro && idx != nil && idx.DocCount() > FreeDocLimit {
		log.Printf("Free tier: %d documents exceeds %d limit. Upgrade to Pro for unlimited.", idx.DocCount(), FreeDocLimit)
		log.Printf("Upgrade at: https://modus.ai/memory")
	}

	// Create vault
	v := vault.New(vaultDir, idx)

	// Load tuned FSRS parameters (if any)
	if err := v.LoadTunedFSRS(); err != nil {
		log.Printf("Warning: failed to load tuned FSRS config: %v", err)
	}

	// Initialize LLM backend
	if !noLLM {
		initBackend(modelPath, gpuLayers, modelCtx)
	} else {
		log.Printf("LLM disabled (--no-llm)")
	}

	// Create MCP server with memory tools only
	srv := mcpsrv.NewServer("modus-memory", version)
	mcpsrv.RegisterMemoryTools(srv, v, isPro)

	tier := "free"
	if isPro {
		tier = "pro"
	}
	log.Printf("modus-memory %s [%s] — vault: %s, %d docs indexed, backend: %s", version, tier, vaultDir, idx.DocCount(), librarian.BackendIdentity())

	// Run MCP stdio loop
	srv.Run()
}

// resolveVaultDir determines the vault directory from flag, env, or default.
func resolveVaultDir(flagDir string) string {
	if flagDir != "" {
		return flagDir
	}
	if envDir := os.Getenv("MODUS_VAULT_DIR"); envDir != "" {
		return envDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "modus", "vault")
}

// runImport dispatches to the appropriate import converter.
func runImport(vaultDir string, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: modus-memory import <format> <file>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Formats:")
		fmt.Fprintln(os.Stderr, "  khoj    — Khoj AI export (ZIP or JSON)")
		os.Exit(1)
	}

	format := args[0]
	file := args[1]

	switch format {
	case "khoj":
		runImportKhoj(file, vaultDir)
	default:
		fmt.Fprintf(os.Stderr, "Unknown import format: %s\n", format)
		os.Exit(1)
	}
}

func runAttach(vaultDir string, args []string) {
	carrier := "codex"
	prompt := ""
	model := ""
	workDir := ""
	target := ""
	subject := ""
	workItemID := ""
	recallLimit := 6
	jsonOut := false
	noEpisode := false
	ephemeral := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--carrier":
			if i+1 < len(args) {
				carrier = args[i+1]
				i++
			}
		case "--prompt":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				model = args[i+1]
				i++
			}
		case "--workdir":
			if i+1 < len(args) {
				workDir = args[i+1]
				i++
			}
		case "--target":
			if i+1 < len(args) {
				target = args[i+1]
				i++
			}
		case "--subject":
			if i+1 < len(args) {
				subject = args[i+1]
				i++
			}
		case "--work-item-id":
			if i+1 < len(args) {
				workItemID = args[i+1]
				i++
			}
		case "--recall-limit":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					recallLimit = n
				}
				i++
			}
		case "--json":
			jsonOut = true
		case "--no-episode":
			noEpisode = true
		case "--ephemeral":
			ephemeral = true
		}
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "attach: read stdin: %v\n", err)
			os.Exit(1)
		}
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "Usage: modus-memory attach --carrier <codex|claude|qwen|gemini|ollama|hermes|openclaw|opencode> --prompt \"...\" [--target ...] [--json]")
		os.Exit(1)
	}

	idx, err := index.Build(vaultDir, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "attach: build index: %v\n", err)
		os.Exit(1)
	}

	kernel := memorykit.New(vault.New(vaultDir, idx))
	result, err := kernel.RunAttachedCarrier(context.Background(), memorykit.AttachmentRunOptions{
		Carrier:      strings.TrimSpace(carrier),
		Prompt:       prompt,
		Model:        strings.TrimSpace(model),
		WorkDir:      strings.TrimSpace(workDir),
		Target:       strings.TrimSpace(target),
		Ephemeral:    ephemeral,
		RecallLimit:  recallLimit,
		StoreEpisode: !noEpisode,
		Subject:      strings.TrimSpace(subject),
		WorkItemID:   strings.TrimSpace(workItemID),
	})

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "attach: encode result: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Carrier: %s\n", result.Carrier)
		if result.Model != "" {
			fmt.Printf("Model: %s\n", result.Model)
		}
		fmt.Printf("Recall receipt: %s\n", result.RecallReceiptPath)
		if result.TracePath != "" {
			fmt.Printf("Trace: %s\n", result.TracePath)
		}
		if result.EpisodePath != "" {
			fmt.Printf("Episode: %s\n", result.EpisodePath)
		}
		if result.ThreadID != "" {
			fmt.Printf("Thread ID: %s\n", result.ThreadID)
		}
		fmt.Printf("Duration: %.2fs\n", result.DurationSec)
		fmt.Println()
		fmt.Println(result.Output)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "attach: %v\n", err)
		os.Exit(1)
	}
}

// initBackend tries embedded → HTTP → disabled, in that order.
func initBackend(modelPath string, gpuLayers, nCtx int) {
	// Try embedded backend if model path provided and llamacpp available
	if modelPath != "" && llamacpp.Available() {
		backend, err := librarian.NewEmbeddedBackend(modelPath, gpuLayers, nCtx)
		if err != nil {
			log.Printf("Embedded backend failed: %v — falling back to HTTP", err)
		} else {
			librarian.SetBackend(backend)
			log.Printf("Backend: embedded (%s)", modelPath)
			return
		}
	} else if modelPath != "" && !llamacpp.Available() {
		log.Printf("--model specified but llamacpp not available (built with nollamacpp?) — falling back to HTTP")
	}

	// Try HTTP backend
	endpoint := librarian.ResolveEndpoint()
	httpBackend := librarian.NewHTTPBackend(endpoint)
	if httpBackend.Available() {
		librarian.SetBackend(httpBackend)
		log.Printf("Backend: HTTP (%s)", endpoint)
		return
	}

	// No backend available
	log.Printf("Backend: disabled (no embedded model, HTTP not reachable)")
}

// runHealth prints vault statistics.
func runHealth(vaultDir string) {
	idx, err := index.Build(vaultDir, "")
	if err != nil {
		fmt.Printf("Vault: %s\n", vaultDir)
		fmt.Printf("Status: ERROR — %v\n", err)
		return
	}

	totalFacts, activeFacts := idx.FactCount()
	subjects, tags, entities := idx.CrossRefStats()

	fmt.Printf("modus-memory %s\n", version)
	fmt.Printf("Vault: %s\n", vaultDir)
	fmt.Printf("Documents: %d\n", idx.DocCount())
	fmt.Printf("Facts: %d total, %d active\n", totalFacts, activeFacts)
	fmt.Printf("Cross-refs: %d subjects, %d tags, %d entities\n", subjects, tags, entities)
	fmt.Printf("Librarian: %s\n", librarian.BackendIdentity())
}

func runProposeHot(vaultDir string, args []string) {
	result, err := memorycli.ProposeHot(vaultDir, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "propose-hot: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result.Message)
}

func runProposeStructural(vaultDir string, args []string) {
	result, err := memorycli.ProposeStructural(vaultDir, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "propose-structural: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result.Message)
}

func runProposeTemporal(vaultDir string, args []string) {
	result, err := memorycli.ProposeTemporal(vaultDir, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "propose-temporal: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result.Message)
}

func runProposeElder(vaultDir string, args []string) {
	result, err := memorycli.ProposeElder(vaultDir, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "propose-elder: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(result.Message)
}

func runReviewQueue(vaultDir string, args []string) {
	result, err := memorycli.ReviewQueue(vaultDir, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review-queue: %v\n", err)
		os.Exit(1)
	}
	if result.JSON {
		data, err := memorycli.MarshalReviewQueueJSON(result.Summary)
		if err != nil {
			fmt.Fprintf(os.Stderr, "review-queue: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Print(result.Rendered)
}

func runResolveReview(vaultDir string, args []string) {
	result, err := memorycli.ResolveReview(vaultDir, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve-review: %v\n", err)
		os.Exit(1)
	}
	if result.JSON {
		data, err := memorycli.MarshalResolveReviewJSON(result.Summary)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve-review: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Print(result.Rendered)
}

func runCarrierAudit(vaultDir string, args []string) {
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
		}
	}
	kernel := memorykit.New(vault.New(vaultDir, nil))
	result, err := kernel.AuditCarriers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "carrier-audit: %v\n", err)
		os.Exit(1)
	}
	if jsonOut {
		data, err := memorykit.MarshalCarrierAuditJSON(result.Report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "carrier-audit: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Print(memorykit.RenderCarrierAuditSummary(result.Report))
}

func runCarrierProbe(vaultDir string, args []string) {
	fs := flag.NewFlagSet("carrier-probe", flag.ExitOnError)
	carriers := fs.String("carriers", "", "comma-separated carrier list")
	prompt := fs.String("prompt", "Reply with exactly: nominal.", "probe prompt")
	model := fs.String("model", "", "optional carrier model override")
	workDir := fs.String("workdir", "", "optional working directory")
	recallLimit := fs.Int("recall-limit", 4, "maximum hot-memory lines to recall before each carrier run")
	storeEpisode := fs.Bool("store-episode", false, "store an episodic receipt for each probe")
	openclawTarget := fs.String("openclaw-target", "main", "target for openclaw probes")
	workItemID := fs.String("work-item-id", "", "optional work item lineage")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "carrier-probe: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*carriers) == "" {
		fmt.Fprintln(os.Stderr, "Usage: modus-memory carrier-probe --carriers <codex,qwen,...> [--prompt \"...\"] [--json]")
		os.Exit(1)
	}
	kernel := memorykit.New(vault.New(vaultDir, nil))
	result, err := kernel.ProbeCarriers(context.Background(), memorykit.CarrierProbeOptions{
		Carriers:       strings.Split(*carriers, ","),
		Prompt:         strings.TrimSpace(*prompt),
		Model:          strings.TrimSpace(*model),
		WorkDir:        strings.TrimSpace(*workDir),
		RecallLimit:    *recallLimit,
		StoreEpisode:   *storeEpisode,
		OpenClawTarget: strings.TrimSpace(*openclawTarget),
		WorkItemID:     strings.TrimSpace(*workItemID),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "carrier-probe: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		data, err := memorykit.MarshalCarrierProbeJSON(result.Report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "carrier-probe: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Print(memorykit.RenderCarrierProbeSummary(result.Report))
}

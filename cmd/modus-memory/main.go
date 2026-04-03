// modus-memory is a standalone MCP memory server.
//
// A single Go binary that provides personal memory over the MCP protocol.
// BM25 full-text search, FSRS spaced-repetition decay, cross-referencing,
// librarian query expansion — all in ~6MB, zero dependencies. Completely free.
//
// Usage:
//
//	modus-memory                    — start MCP server on stdio
//	modus-memory --vault ~/notes    — use custom vault directory
//	modus-memory version            — print version
//	modus-memory health             — check vault health
//	modus-memory import khoj <file> — import from Khoj export (ZIP or JSON)
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/GetModus/modus-memory/internal/index"
	mcpsrv "github.com/GetModus/modus-memory/internal/mcp"
	"github.com/GetModus/modus-memory/internal/vault"
)

const version = "0.2.0"

func main() {
	// Parse flags
	vaultDir := ""
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--vault", "-v":
			if i+1 < len(args) {
				vaultDir = args[i+1]
				i++
			}
		case "version", "--version":
			fmt.Printf("modus-memory %s\n", version)
			os.Exit(0)
		case "health":
			vd := resolveVaultDir(vaultDir)
			runHealth(vd)
			os.Exit(0)
		case "import":
			vd := resolveVaultDir(vaultDir)
			runImport(vd, args[i+1:])
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
	for _, sub := range []string{"memory/facts", "brain", "atlas"} {
		os.MkdirAll(filepath.Join(vaultDir, sub), 0755)
	}

	// Redirect log output to stderr (stdout is MCP protocol)
	log.SetOutput(os.Stderr)
	log.SetPrefix("[modus-memory] ")

	// Build search index
	idx, err := index.Build(vaultDir, "")
	if err != nil {
		log.Printf("Warning: index build failed: %v (starting with empty index)", err)
	}

	// Create vault
	v := vault.New(vaultDir, idx)

	// Create MCP server with all 11 memory tools
	srv := mcpsrv.NewServer("modus-memory", version)
	mcpsrv.RegisterMemoryTools(srv, v)

	log.Printf("modus-memory %s — vault: %s, %d docs indexed", version, vaultDir, idx.DocCount())

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
}

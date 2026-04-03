package mcp

import (
	"fmt"

	"github.com/GetModus/modus-memory/internal/vault"
)

// ProFeatures are the MCP tools gated behind a Pro license.
var ProFeatures = map[string]bool{
	"memory_reinforce":  true, // FSRS reinforcement
	"memory_decay_facts": true, // FSRS decay sweep
	"vault_connected":   true, // Cross-reference query
}

// RegisterMemoryTools registers only the memory-relevant MCP tools for the
// standalone modus-memory server. This is a strict subset of RegisterVaultTools.
//
// Free tier tools (always available):
//
//	vault_search, vault_read, vault_write, vault_list, vault_status,
//	memory_facts, memory_search, memory_store
//
// Pro tier tools (require license):
//
//	memory_reinforce, memory_decay_facts, vault_connected
func RegisterMemoryTools(srv *Server, v *vault.Vault, isPro bool) {
	RegisterVaultTools(srv, v)

	// Remove non-memory tools — keep only the core memory tools
	keep := map[string]bool{
		"vault_search":  true,
		"vault_read":    true,
		"vault_write":   true,
		"vault_list":    true,
		"vault_status":  true,
		"memory_facts":  true,
		"memory_search": true,
		"memory_store":  true,
	}

	// Free tier doc limit — wrap vault_write and memory_store with limit check
	if !isPro {
		wrapWithDocLimit(srv, "vault_write", v, 1000)
		wrapWithDocLimit(srv, "memory_store", v, 1000)
	}

	// Add Pro tools if licensed
	if isPro {
		for name := range ProFeatures {
			keep[name] = true
		}
	}

	for name := range srv.tools {
		if !keep[name] {
			delete(srv.tools, name)
			delete(srv.handlers, name)
		}
	}

	// If not Pro, add stub tools that explain the upgrade
	if !isPro {
		for name := range ProFeatures {
			toolName := name
			srv.AddTool(toolName,
				fmt.Sprintf("[Pro] %s — requires modus-memory Pro. Visit https://modus.ai/memory to upgrade.", toolName),
				map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
				func(args map[string]interface{}) (string, error) {
					return fmt.Sprintf("This feature requires modus-memory Pro ($10/mo).\n\nUpgrade at: https://modus.ai/memory\nThen run: modus-memory activate <license-key>\n\nPro includes: FSRS memory decay, cross-referencing, librarian query expansion, and unlimited documents."), nil
				})
		}
	}
}

// wrapWithDocLimit wraps a write tool handler with a document count check.
func wrapWithDocLimit(srv *Server, toolName string, v *vault.Vault, limit int) {
	original := srv.handlers[toolName]
	if original == nil {
		return
	}
	srv.handlers[toolName] = func(args map[string]interface{}) (string, error) {
		if v.Index != nil && v.Index.DocCount() >= limit {
			return fmt.Sprintf("Free tier limit reached: %d documents (max %d).\n\nUpgrade to Pro for unlimited documents: https://modus.ai/memory\nThen run: modus-memory activate <license-key>", v.Index.DocCount(), limit), nil
		}
		return original(args)
	}
}

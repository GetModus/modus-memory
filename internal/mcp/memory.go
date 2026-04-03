package mcp

import (
	"github.com/GetModus/modus-memory/internal/vault"
)

// RegisterMemoryTools registers the 11 MCP tools for the modus-memory server.
// All features are free and unrestricted.
//
// Tools:
//
//	vault_search, vault_read, vault_write, vault_list, vault_status,
//	memory_facts, memory_search, memory_store,
//	memory_reinforce, memory_decay_facts, vault_connected
func RegisterMemoryTools(srv *Server, v *vault.Vault) {
	RegisterVaultTools(srv, v)

	// Keep only the 11 memory-relevant tools
	keep := map[string]bool{
		"vault_search":      true,
		"vault_read":        true,
		"vault_write":       true,
		"vault_list":        true,
		"vault_status":      true,
		"memory_facts":      true,
		"memory_search":     true,
		"memory_store":      true,
		"memory_reinforce":  true,
		"memory_decay_facts": true,
		"vault_connected":   true,
	}

	for name := range srv.tools {
		if !keep[name] {
			delete(srv.tools, name)
			delete(srv.handlers, name)
		}
	}
}

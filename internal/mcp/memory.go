package mcp

import "github.com/GetModus/modus-memory/internal/vault"

// RegisterMemoryTools registers only the memory-relevant MCP tools for the
// standalone modus-memory server. This is a strict subset of RegisterVaultTools.
//
// All Homing tools are now available to every user. The isPro parameter
// remains only as a compatibility seam for older callers.
func RegisterMemoryTools(srv *Server, v *vault.Vault, isPro bool) {
	RegisterVaultTools(srv, v)

	// Remove non-memory tools — keep only the memory surface.
	keep := map[string]bool{
		"vault_search":                       true,
		"vault_read":                         true,
		"vault_write":                        true,
		"vault_list":                         true,
		"vault_status":                       true,
		"memory_facts":                       true,
		"memory_capture":                     true,
		"memory_episode_store":               true,
		"memory_search":                      true,
		"memory_store":                       true,
		"memory_learn":                       true,
		"memory_trace":                       true,
		"memory_maintain":                    true,
		"memory_hot_transition_propose":      true,
		"memory_temporal_transition_propose": true,
		"memory_elder_transition_propose":    true,
		"memory_secure_state":                true,
		"memory_evaluate":                    true,
		"memory_readiness":                   true,
		"memory_trial_run":                   true,
		"memory_portability_audit":           true,
		"memory_portability_queue":           true,
		"memory_portability_archive":         true,
		"memory_reinforce":                   true,
		"memory_decay_facts":                 true,
		"memory_tune":                        true,
		"memory_train":                       true,
		"vault_connected":                    true,
	}

	for name := range srv.tools {
		if !keep[name] {
			delete(srv.tools, name)
			delete(srv.handlers, name)
		}
	}
}

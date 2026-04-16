package trust

import "strings"

// missionOwners are offices allowed to directly mutate mission state,
// subject to trust stage and proof requirements.
var missionOwners = map[string]bool{
	"mission_office": true,
	"main_brain":     true,
}

// candidateOwners are offices broadly allowed to emit candidate artifacts
// when signatures and proof are preserved.
var candidateOwners = map[string]bool{
	"librarian":            true,
	"main_brain":           true,
	"mission_office":       true,
	"self_improvement":     true,
	"self-improvement":     true,
	"wraith":               true,
	"scout":                true,
	"analyst":              true,
	"heartbeat":            true,
	"session_lineage":      true,
	"session-lineage":      true,
	"continuity_diagnostics": true,
}

func normalizeOffice(office string) string {
	return strings.ToLower(strings.TrimSpace(office))
}

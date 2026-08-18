package skill

import "embed"

// FS is the host-agnostic skill tree (SKILL.md + references).
//
//go:embed SKILL.md references
var FS embed.FS

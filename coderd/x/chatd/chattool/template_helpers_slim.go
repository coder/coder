//go:build slim

package chattool

import "github.com/google/uuid"

// IsTemplateAllowed reports whether a template ID is allowed by the resolved
// allowlist. A nil or empty allowlist means all templates are allowed.
func IsTemplateAllowed(allowlist map[uuid.UUID]bool, id uuid.UUID) bool {
	if len(allowlist) == 0 {
		return true
	}
	return allowlist[id]
}

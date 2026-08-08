// Package idemetadata is the single source of truth for the app names and
// family grouping used in workspace usage reporting. It is a leaf package so
// both agent and coderd code can import it.
package idemetadata

import (
	"strings"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// MaxAppNameLength is the maximum length of an app name in runes. Longer
// names are truncated before storage.
const MaxAppNameLength = 64

// MaxSessionCountEntries bounds distinct app names per stats report;
// overflow aggregates under AppNameUnknown. Applied by both agent and
// server.
const MaxSessionCountEntries = 64

// Canonical app names for Coder's built-in session types.
const (
	AppNameVSCode          = "vscode"
	AppNameJetBrains       = "jetbrains"
	AppNameSSH             = "ssh"
	AppNameReconnectingPTY = "reconnecting_pty"
	AppNameUnknown         = "unknown"
)

// families maps each known app name and alias to its family, named after
// the family's canonical app name, to bound metric-label cardinality. App
// names are stored ungrouped, so extending this map needs no migration.
var families = map[string]string{
	AppNameVSCode:          AppNameVSCode,
	"vscode_insiders":      AppNameVSCode,
	"cursor":               AppNameVSCode,
	"windsurf":             AppNameVSCode,
	"positron":             AppNameVSCode,
	"vscodium":             AppNameVSCode,
	"codium":               AppNameVSCode,
	"antigravity":          AppNameVSCode,
	"trae":                 AppNameVSCode,
	"kiro":                 AppNameVSCode,
	"devin":                AppNameVSCode,
	AppNameJetBrains:       AppNameJetBrains,
	AppNameSSH:             AppNameSSH,
	AppNameReconnectingPTY: AppNameReconnectingPTY,
}

// Family returns the family for the given app name. Matching is
// case-insensitive and alias-aware; unknown names map to AppNameUnknown.
func Family(appName string) string {
	if family, ok := families[canonicalKey(appName)]; ok {
		return family
	}
	return AppNameUnknown
}

// Normalize prepares a client-supplied app name for storage: it strips null
// bytes (Postgres TEXT rejects them), truncates to MaxAppNameLength runes,
// trims surrounding whitespace, lowercases, and folds hyphens to
// underscores. Empty becomes AppNameUnknown.
func Normalize(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	appName = utilstrings.Truncate(appName, MaxAppNameLength)
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return AppNameUnknown
	}
	return canonicalKey(appName)
}

// canonicalKey lowercases and folds hyphens to underscores so spellings like
// "reconnecting-pty" and "vscode-insiders" match their canonical form.
func canonicalKey(appName string) string {
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}

// Package idemetadata defines the app names and family grouping used in
// workspace usage reporting. It is a leaf package importable by both agent
// and coderd code.
package idemetadata

import (
	"strings"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// MaxAppNameLength is the maximum app name length in runes; longer names
// are truncated before storage.
const MaxAppNameLength = 64

// MaxSessionCountEntries bounds distinct app names per stats report;
// overflow aggregates under AppNameUnknown.
const MaxSessionCountEntries = 64

// Canonical app names for Coder's built-in session types.
const (
	AppNameVSCode          = "vscode"
	AppNameJetBrains       = "jetbrains"
	AppNameZed             = "zed"
	AppNameSSH             = "ssh"
	AppNameReconnectingPTY = "reconnecting_pty"
	AppNameUnknown         = "unknown"
)

// families groups app names for metric labels and the Connection_Type enum,
// never for storage, so adding an alias needs no migration and a missing one
// only costs an AppNameUnknown label. Names are the ids Coder's registry
// modules use, listed ahead of the clients that will report them.
var families = map[string]string{
	AppNameVSCode:     AppNameVSCode,
	"vscode_insiders": AppNameVSCode,
	"vscode_web":      AppNameVSCode,
	"code_server":     AppNameVSCode,
	"cursor":          AppNameVSCode,
	"windsurf":        AppNameVSCode,
	"positron":        AppNameVSCode,
	"vscodium":        AppNameVSCode,
	"codium":          AppNameVSCode,
	"antigravity":     AppNameVSCode,
	"trae":            AppNameVSCode,
	"kiro":            AppNameVSCode,
	"devin":           AppNameVSCode,

	AppNameJetBrains:       AppNameJetBrains,
	AppNameZed:             AppNameZed,
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
// trims whitespace, lowercases, and folds hyphens to underscores.
//
// An empty name becomes AppNameUnknown. An unset CODER_SSH_SESSION_TYPE or
// --usage-app instead means plain SSH, resolved by those callers first.
func Normalize(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	appName = utilstrings.Truncate(appName, MaxAppNameLength)
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return AppNameUnknown
	}
	return canonicalKey(appName)
}

// canonicalKey lowercases and folds hyphens to underscores.
func canonicalKey(appName string) string {
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}

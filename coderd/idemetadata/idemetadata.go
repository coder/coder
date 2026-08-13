// Package idemetadata defines the app names and family grouping used in
// workspace usage reporting. It is a leaf package importable by both agent
// and coderd code.
package idemetadata

import (
	"strings"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// MaxAppNameLength is the maximum app name length in runes.
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
// never for storage, so a missing alias only costs an AppNameUnknown label.
// Names are the ids Coder's registry modules use.
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

	AppNameJetBrains: AppNameJetBrains,
	// Zed connects over plain SSH, so its sessions belong in the SSH bucket
	// until a durable per-app rollup exists.
	AppNameZed:             AppNameSSH,
	AppNameSSH:             AppNameSSH,
	AppNameReconnectingPTY: AppNameReconnectingPTY,
}

// Family returns the family for an app name, or AppNameUnknown. Matching is
// case-insensitive and alias-aware.
func Family(appName string) string {
	if family, ok := families[canonicalKey(appName)]; ok {
		return family
	}
	return AppNameUnknown
}

// Normalize prepares a client-supplied app name for storage, stripping null
// bytes that Postgres TEXT rejects. An empty name becomes AppNameUnknown; an
// unset session type instead means plain SSH, which callers resolve first.
func Normalize(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	// Trim before truncating so padding does not spend the budget, and after
	// in case the cut lands in whitespace.
	appName = strings.TrimSpace(appName)
	appName = strings.TrimSpace(utilstrings.Truncate(appName, MaxAppNameLength))
	if appName == "" {
		return AppNameUnknown
	}
	return canonicalKey(appName)
}

func canonicalKey(appName string) string {
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}

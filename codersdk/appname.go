package codersdk

import (
	"strings"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// Server-side vocabulary, not part of the HTTP API.
// @typescript-ignore MaxAppNameLength, MaxSessionCountEntries, AppNameVSCode, AppNameJetBrains, AppNameSSH, AppNameReconnectingPTY, AppNameUnknown

// MaxAppNameLength is the maximum app name length in runes.
const MaxAppNameLength = 64

// MaxSessionCountEntries bounds distinct app names per stats report.
// Overflow aggregates under AppNameUnknown.
const MaxSessionCountEntries = 64

// Normalized app names for Coder's built-in session types.
const (
	AppNameVSCode          = "vscode"
	AppNameJetBrains       = "jetbrains"
	AppNameSSH             = "ssh"
	AppNameReconnectingPTY = "reconnecting_pty"
	AppNameUnknown         = "unknown"
)

// appNameFamilies groups app names for callers that need a bounded value,
// such as a metric label or the Connection_Type enum. It is never used for
// storage, so a missing alias only costs an AppNameUnknown label. Keys are
// the IDs Coder's registry modules use.
var appNameFamilies = map[string]string{
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
	// Zed has no Connection_Type or session count field of its own, so it
	// rolls up under SSH. The raw name still reaches storage.
	"zed":                  AppNameSSH,
	AppNameSSH:             AppNameSSH,
	AppNameReconnectingPTY: AppNameReconnectingPTY,
}

// AppNameFamily normalizes an app name and returns its family, or
// AppNameUnknown.
func AppNameFamily(appName string) string {
	if family, ok := appNameFamilies[NormalizeAppName(appName)]; ok {
		return family
	}
	return AppNameUnknown
}

// NormalizeAppName prepares a client-supplied app name for storage and
// lookup. It strips the null bytes Postgres TEXT rejects, trims, truncates to
// MaxAppNameLength runes, lowercases, and folds hyphens to underscores. An
// empty name becomes AppNameUnknown; callers resolve an unset session type to
// AppNameSSH before calling this.
func NormalizeAppName(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	// Trim before truncating so padding does not spend the budget, and after
	// in case the cut lands in whitespace.
	appName = strings.TrimSpace(appName)
	appName = strings.TrimSpace(utilstrings.Truncate(appName, MaxAppNameLength))
	if appName == "" {
		return AppNameUnknown
	}
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}

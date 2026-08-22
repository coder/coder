package codersdk

import (
	"slices"
	"strings"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// maxAppNameLength caps an app name in runes.
const maxAppNameLength = 64

// AppFamilyName is the bounded set of names that arbitrary app names group
// into, for callers that need a fixed value such as a metric label or the
// Connection_Type enum. Server-side only, not part of the HTTP API.
//
// @typescript-ignore AppFamilyName
type AppFamilyName string

const (
	AppFamilyVSCode          AppFamilyName = "vscode"
	AppFamilyJetBrains       AppFamilyName = "jetbrains"
	AppFamilySSH             AppFamilyName = "ssh"
	AppFamilyReconnectingPTY AppFamilyName = "reconnecting_pty"
	AppFamilyUnknown         AppFamilyName = "unknown"
)

// appNameFamilies never gates storage, so a missing alias only costs an
// AppFamilyUnknown label. Keys are the IDs Coder's registry modules use.
var appNameFamilies = map[string]AppFamilyName{
	"vscode":          AppFamilyVSCode,
	"vscode_insiders": AppFamilyVSCode,
	"vscode_web":      AppFamilyVSCode,
	"code_server":     AppFamilyVSCode,
	"cursor":          AppFamilyVSCode,
	"windsurf":        AppFamilyVSCode,
	"positron":        AppFamilyVSCode,
	"vscodium":        AppFamilyVSCode,
	"codium":          AppFamilyVSCode,
	"antigravity":     AppFamilyVSCode,
	"trae":            AppFamilyVSCode,
	"kiro":            AppFamilyVSCode,
	"devin":           AppFamilyVSCode,

	"jetbrains": AppFamilyJetBrains,
	// Zed has no Connection_Type or session count field of its own, so it
	// rolls up under SSH. The raw name still reaches storage.
	"zed":              AppFamilySSH,
	"ssh":              AppFamilySSH,
	"reconnecting_pty": AppFamilyReconnectingPTY,
}

// AttributedAppFamilies are the families usage reporting has somewhere to
// put. Every value in appNameFamilies must appear here or its sessions go
// uncounted, which TestEveryFamilyIsAttributed enforces.
var AttributedAppFamilies = []AppFamilyName{
	AppFamilyVSCode,
	AppFamilyJetBrains,
	AppFamilySSH,
	AppFamilyReconnectingPTY,
}

// AppNamesInFamily returns the app names belonging to a family, sorted so
// query parameters stay stable across calls.
func AppNamesInFamily(family AppFamilyName) []string {
	var names []string
	for appName, appFamily := range appNameFamilies {
		if appFamily == family {
			names = append(names, appName)
		}
	}
	slices.Sort(names)
	return names
}

// AppNameFamily normalizes an app name and returns its family, or
// AppFamilyUnknown.
func AppNameFamily(appName string) AppFamilyName {
	if family, ok := appNameFamilies[NormalizeAppName(appName)]; ok {
		return family
	}
	return AppFamilyUnknown
}

// NormalizeAppName prepares a client-supplied app name for storage and
// lookup: it strips the null bytes Postgres TEXT rejects, trims, truncates,
// lowercases, and folds hyphens to underscores. Empty becomes
// AppFamilyUnknown.
func NormalizeAppName(appName string) string {
	appName = strings.ReplaceAll(appName, "\x00", "")
	// Trim before truncating so padding does not spend the budget, and after
	// in case the cut lands in whitespace.
	appName = strings.TrimSpace(appName)
	appName = strings.TrimSpace(utilstrings.Truncate(appName, maxAppNameLength))
	if appName == "" {
		return string(AppFamilyUnknown)
	}
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}

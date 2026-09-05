package codersdk

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode"

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

// attributedAppFamilies are the families usage reporting has somewhere to
// put, and the single definition of the families the session count read
// queries know about. Every value in appNameFamilies must appear here or its
// sessions go uncounted, which TestEveryFamilyIsAttributed enforces.
//
// Adding a family here is not enough on its own: see AttributedAppFamilies.
var attributedAppFamilies = []AppFamilyName{
	AppFamilyVSCode,
	AppFamilyJetBrains,
	AppFamilySSH,
	AppFamilyReconnectingPTY,
}

// AttributedAppFamilies returns the families session count reporting can
// attribute to, in registry order. It is the source of truth that dbauthz
// validates the query parameter against, so a family that exists here but not
// in the queries (or the reverse) fails loudly instead of silently reporting
// zero.
func AttributedAppFamilies() []AppFamilyName {
	return slices.Clone(attributedAppFamilies)
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

// SessionCountAppFamilies returns the session count attribution registry:
// every attributed family mapped to its sorted app names. The session count
// read queries take it as one parameter, so a new app name reaches every
// caller by being added to appNameFamilies alone.
//
// A new family costs more. sqlc output columns are static, so each family
// needs, in addition to its attributedAppFamilies entry, one probe expression
// in every query that reports per-family session counts (see
// coderd/database/queries/workspaceagentstats.sql and insights.sql) plus the
// matching output column. dbauthz validation rejects a registry whose
// families do not match AttributedAppFamilies, and
// TestAttributedAppFamiliesMatchQueries pins that list to what the queries
// probe, so neither half can drift unnoticed.
func SessionCountAppFamilies() map[AppFamilyName][]string {
	families := make(map[AppFamilyName][]string, len(attributedAppFamilies))
	for _, family := range attributedAppFamilies {
		families[family] = AppNamesInFamily(family)
	}
	return families
}

// SessionCountAppFamiliesJSON is SessionCountAppFamilies marshaled for the
// jsonb parameter the session count read queries accept.
func SessionCountAppFamiliesJSON() json.RawMessage {
	// Marshaling a map with a string-keyed type cannot fail.
	data, err := json.Marshal(SessionCountAppFamilies())
	if err != nil {
		panic("developer error: marshal session count app families: " + err.Error())
	}
	return data
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
// lookup: it strips control characters, which covers both the null bytes
// Postgres TEXT rejects and the escape sequences that would otherwise reach
// logs and terminals, then trims, truncates, lowercases, and folds hyphens to
// underscores. Empty becomes AppFamilyUnknown.
func NormalizeAppName(appName string) string {
	appName = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, appName)
	// Trim before truncating so padding does not spend the budget, and after
	// in case the cut lands in whitespace.
	appName = strings.TrimSpace(appName)
	appName = strings.TrimSpace(utilstrings.Truncate(appName, maxAppNameLength))
	if appName == "" {
		return string(AppFamilyUnknown)
	}
	return strings.ReplaceAll(strings.ToLower(appName), "-", "_")
}

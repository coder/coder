package codersdk

import (
	"encoding/json"
	"strings"
	"unicode"

	"golang.org/x/xerrors"

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

// appNameFamilies is the only place an app name is attributed to a family.
// Storage keeps the raw app name, so a missing alias only costs an
// AppFamilyUnknown attribution rather than a dropped session. Keys are the
// IDs Coder's registry modules use, normalized as NormalizeAppName leaves
// them.
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

// SessionCountAppFamilies returns the app-to-family attribution registry: one
// entry per known app name, mapped to the family it reports under. Callers
// that need a fixed family value derive it from this map, so registering a
// new app or family means editing appNameFamilies alone.
func SessionCountAppFamilies() map[string]AppFamilyName {
	families := make(map[string]AppFamilyName, len(appNameFamilies))
	for appName, family := range appNameFamilies {
		families[appName] = family
	}
	return families
}

// SessionCountAppFamiliesJSON is SessionCountAppFamilies marshaled as the
// jsonb object of app name to family name that the minute aggregation queries
// decompose with jsonb_each_text. Queries join it by app name, so no query
// names a family and no family needs its own column or probe.
func SessionCountAppFamiliesJSON() json.RawMessage {
	// Marshaling a map with string-kinded keys and values cannot fail.
	data, err := json.Marshal(SessionCountAppFamilies())
	if err != nil {
		panic("developer error: marshal session count app families: " + err.Error())
	}
	return data
}

// SessionCountsByFamily folds per-app session counts, as the session count
// queries report them, into per-family totals. Counts are additive: an agent
// running Cursor and VS Code at once contributes both to the VS Code family.
// App names with no registry entry total under AppFamilyUnknown rather than
// being dropped.
func SessionCountsByFamily(appCounts map[string]int64) map[AppFamilyName]int64 {
	familyCounts := make(map[AppFamilyName]int64, len(appCounts))
	for appName, count := range appCounts {
		familyCounts[AppNameFamily(appName)] += count
	}
	return familyCounts
}

// SessionCountsByFamilyJSON is SessionCountsByFamily over the jsonb object of
// app name to session count that the session count queries return. An absent
// or JSON null object means no sessions, not an error, because a query with
// no matching rows aggregates to SQL NULL.
func SessionCountsByFamilyJSON(appCounts json.RawMessage) (map[AppFamilyName]int64, error) {
	var counts map[string]int64
	if len(appCounts) > 0 {
		if err := json.Unmarshal(appCounts, &counts); err != nil {
			return nil, xerrors.Errorf("unmarshal session counts by app name: %w", err)
		}
	}
	return SessionCountsByFamily(counts), nil
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

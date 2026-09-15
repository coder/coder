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

// SessionCountApp describes the presentation of a recognized session app.
type SessionCountApp struct {
	// DisplayName is the human-readable app name.
	DisplayName string `json:"display_name"`
	// Icon is an optional path to a bundled icon, relative to the server root.
	Icon string `json:"icon,omitempty"`
}

type sessionApp struct {
	family AppFamilyName
	SessionCountApp
}

// sessionApps owns app attribution and presentation. Keys are normalized app
// names. Unregistered names keep their identity in storage and use the
// unknown family when aggregated. Apps without a bundled icon leave Icon
// empty so clients render a generic icon next to the display name.
var sessionApps = map[string]sessionApp{
	"vscode":          {AppFamilyVSCode, SessionCountApp{"VS Code", "/icon/code.svg"}},
	"vscode_insiders": {AppFamilyVSCode, SessionCountApp{"VS Code Insiders", "/icon/code-insiders.svg"}},
	"vscode_web":      {AppFamilyVSCode, SessionCountApp{"VS Code Web", "/icon/code.svg"}},
	"code_server":     {AppFamilyVSCode, SessionCountApp{"code-server", "/icon/code.svg"}},
	"cursor":          {AppFamilyVSCode, SessionCountApp{"Cursor", "/icon/cursor.svg"}},
	"windsurf":        {AppFamilyVSCode, SessionCountApp{"Windsurf", "/icon/windsurf.svg"}},
	"positron":        {AppFamilyVSCode, SessionCountApp{"Positron", "/icon/positron.svg"}},
	"vscodium":        {AppFamilyVSCode, SessionCountApp{"VSCodium", ""}},
	"codium":          {AppFamilyVSCode, SessionCountApp{"VSCodium", ""}},
	"antigravity":     {AppFamilyVSCode, SessionCountApp{"Antigravity", "/icon/antigravity.svg"}},
	"trae":            {AppFamilyVSCode, SessionCountApp{"Trae", ""}},
	"kiro":            {AppFamilyVSCode, SessionCountApp{"Kiro", "/icon/kiro.svg"}},
	"devin":           {AppFamilyVSCode, SessionCountApp{"Devin", "/icon/devin.svg"}},
	"jetbrains":       {AppFamilyJetBrains, SessionCountApp{"JetBrains", "/icon/jetbrains.svg"}},
	// Zed speaks SSH and contributes to the SSH compatibility total.
	"zed":              {AppFamilySSH, SessionCountApp{"Zed", "/icon/zed.svg"}},
	"ssh":              {AppFamilySSH, SessionCountApp{"SSH", "/icon/terminal.svg"}},
	"reconnecting_pty": {AppFamilyReconnectingPTY, SessionCountApp{"Web Terminal", ""}},
}

// lookupSessionApp normalizes appName and returns its registry entry.
func lookupSessionApp(appName string) (sessionApp, bool) {
	app, ok := sessionApps[NormalizeAppName(appName)]
	return app, ok
}

// SessionCountAppMetadata returns presentation metadata for a recognized app
// name. Unrecognized names return false and should be shown as-is.
func SessionCountAppMetadata(appName string) (SessionCountApp, bool) {
	app, ok := lookupSessionApp(appName)
	return app.SessionCountApp, ok
}

// SessionCountAppFamilies returns a copy of the registry's app-to-family mapping.
func SessionCountAppFamilies() map[string]AppFamilyName {
	families := make(map[string]AppFamilyName, len(sessionApps))
	for name, app := range sessionApps {
		families[name] = app.family
	}
	return families
}

// SessionCountAppFamiliesJSON returns the registry as JSON for query-side
// family aggregation without hardcoded app names in SQL.
func SessionCountAppFamiliesJSON() json.RawMessage {
	// Marshaling a map with string-kinded keys and values cannot fail.
	data, err := json.Marshal(SessionCountAppFamilies())
	if err != nil {
		panic("developer error: marshal session count app families: " + err.Error())
	}
	return data
}

// SessionCountsByFamily sums app counts by family, including AppFamilyUnknown.
func SessionCountsByFamily(appCounts map[string]int64) map[AppFamilyName]int64 {
	familyCounts := make(map[AppFamilyName]int64, len(appCounts))
	for appName, count := range appCounts {
		familyCounts[AppNameFamily(appName)] += count
	}
	return familyCounts
}

// DecodeSessionCounts decodes per-app counts returned by the database. Absent
// and JSON null payloads return an empty map so APIs serialize them as objects.
func DecodeSessionCounts(appCounts json.RawMessage) (map[string]int64, error) {
	var counts map[string]int64
	if len(appCounts) > 0 {
		if err := json.Unmarshal(appCounts, &counts); err != nil {
			return nil, xerrors.Errorf("unmarshal session counts by app name: %w", err)
		}
	}
	if counts == nil {
		counts = make(map[string]int64)
	}
	return counts, nil
}

// AppNameFamily normalizes an app name and returns its family, or
// AppFamilyUnknown.
func AppNameFamily(appName string) AppFamilyName {
	if app, ok := lookupSessionApp(appName); ok {
		return app.family
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

// DecodeAppFamilyMap decodes a JSONB payload keyed by app family. An absent
// payload decodes to an empty map, but a malformed one is an error so that
// callers report the failure instead of reporting zero usage.
func DecodeAppFamilyMap[V any](raw json.RawMessage) (map[AppFamilyName]V, error) {
	if len(raw) == 0 {
		return map[AppFamilyName]V{}, nil
	}
	var decoded map[AppFamilyName]V
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, xerrors.Errorf("unmarshal session family map: %w", err)
	}
	return decoded, nil
}

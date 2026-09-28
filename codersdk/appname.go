package codersdk

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// maxAppNameLength caps an app name in runes.
const maxAppNameLength = 64

// AppFamilyName is the bounded set of groups an app name falls into, such as
// vscode or ssh. A name Coder does not recognize falls under unknown.
type AppFamilyName string

const (
	AppFamilyVSCode          AppFamilyName = "vscode"
	AppFamilyJetBrains       AppFamilyName = "jetbrains"
	AppFamilySSH             AppFamilyName = "ssh"
	AppFamilyReconnectingPTY AppFamilyName = "reconnecting_pty"
	// AppFamilySFTP only comes from history the sftp_mins column recorded.
	AppFamilySFTP    AppFamilyName = "sftp"
	AppFamilyUnknown AppFamilyName = "unknown"
)

// AppNameOverflow sums the app names past the per-report cap.
const AppNameOverflow = "overflow"

// SessionCountApp is one app's session count and how to present it.
type SessionCountApp struct {
	Count int64 `json:"count"`
	// DisplayName is the registry's name for a known app, otherwise the
	// normalized identifier itself.
	DisplayName string `json:"display_name"`
	// Icon is a bundled path under /icon/, empty if the app has none.
	Icon string `json:"icon,omitempty"`
	// Family is the group this app totals under.
	Family AppFamilyName `json:"family"`
}

type sessionApp struct {
	family      AppFamilyName
	displayName string
	icon        string
}

// sessionApps keys attribution and presentation by normalized registry module
// ID. An unregistered ID shows as-is under AppFamilyUnknown.
var sessionApps = map[string]sessionApp{
	"vscode":          {AppFamilyVSCode, "VS Code", "/icon/code.svg"},
	"vscode_insiders": {AppFamilyVSCode, "VS Code Insiders", "/icon/code-insiders.svg"},
	"vscode_web":      {AppFamilyVSCode, "VS Code Web", "/icon/code.svg"},
	"code_server":     {AppFamilyVSCode, "code-server", "/icon/code.svg"},
	"cursor":          {AppFamilyVSCode, "Cursor", "/icon/cursor.svg"},
	"windsurf":        {AppFamilyVSCode, "Windsurf", "/icon/windsurf.svg"},
	"positron":        {AppFamilyVSCode, "Positron", "/icon/positron.svg"},
	"vscodium":        {AppFamilyVSCode, "VSCodium", ""},
	"codium":          {AppFamilyVSCode, "VSCodium", ""},
	"antigravity":     {AppFamilyVSCode, "Antigravity", "/icon/antigravity.svg"},
	"trae":            {AppFamilyVSCode, "Trae", ""},
	"kiro":            {AppFamilyVSCode, "Kiro", "/icon/kiro.svg"},
	"devin":           {AppFamilyVSCode, "Devin", "/icon/devin.svg"},
	"jetbrains":       {AppFamilyJetBrains, "JetBrains", "/icon/jetbrains.svg"},
	// No agent reports sftp; the family covers the sftp_mins history.
	"sftp": {AppFamilySFTP, "SFTP", "/icon/terminal.svg"},
	// Zed speaks SSH, so it counts toward the SSH total.
	"zed":              {AppFamilySSH, "Zed", "/icon/zed.svg"},
	"ssh":              {AppFamilySSH, "SSH", "/icon/terminal.svg"},
	"reconnecting_pty": {AppFamilyReconnectingPTY, "Web Terminal", ""},
}

// SessionCountApps pairs each count with its presentation, keyed by app name.
func SessionCountApps(counts map[string]int64) map[string]SessionCountApp {
	apps := make(map[string]SessionCountApp, len(counts))
	for appName, count := range counts {
		app := sessionApps[NormalizeAppName(appName)]
		apps[appName] = SessionCountApp{
			Count:       count,
			DisplayName: cmp.Or(app.displayName, appName),
			Icon:        app.icon,
			Family:      cmp.Or(app.family, AppFamilyUnknown),
		}
	}
	return apps
}

// SessionCountAppFamilies returns a copy of the registry's app-to-family mapping.
func SessionCountAppFamilies() map[string]AppFamilyName {
	families := make(map[string]AppFamilyName, len(sessionApps))
	for appName, app := range sessionApps {
		families[appName] = app.family
	}
	return families
}

// SumByFamily totals a per-app map by family. An unregistered name totals
// under AppFamilyUnknown.
func SumByFamily(byApp map[string]int64) map[AppFamilyName]int64 {
	byFamily := make(map[AppFamilyName]int64)
	for appName, value := range byApp {
		byFamily[AppNameFamily(appName)] += value
	}
	return byFamily
}

// UnionByFamily folds per-app template IDs into the distinct set each family
// was seen in, ordered by app name.
func UnionByFamily(byApp map[string][]uuid.UUID) map[AppFamilyName][]uuid.UUID {
	byFamily := make(map[AppFamilyName][]uuid.UUID, len(byApp))
	seen := make(map[AppFamilyName]map[uuid.UUID]struct{}, len(byApp))
	for _, appName := range slices.Sorted(maps.Keys(byApp)) {
		family := AppNameFamily(appName)
		if seen[family] == nil {
			seen[family] = map[uuid.UUID]struct{}{}
		}
		for _, id := range byApp[appName] {
			if _, ok := seen[family][id]; ok {
				continue
			}
			seen[family][id] = struct{}{}
			byFamily[family] = append(byFamily[family], id)
		}
	}
	return byFamily
}

// AppNameFamily normalizes an app name and returns its family, or
// AppFamilyUnknown.
func AppNameFamily(appName string) AppFamilyName {
	if app, ok := sessionApps[NormalizeAppName(appName)]; ok {
		return app.family
	}
	return AppFamilyUnknown
}

// NormalizeAppName prepares a client-supplied app name for storage and
// lookup: it strips control characters, then trims, truncates, lowercases,
// and folds hyphens to underscores. Empty becomes AppFamilyUnknown.
// Stripping control characters covers both the null bytes Postgres TEXT
// rejects and escape sequences that would otherwise reach logs and terminals.
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

// appMapValue is what an app-keyed JSONB payload holds.
type appMapValue interface {
	int64 | []uuid.UUID
}

// DecodeAppMap decodes an app-keyed JSONB payload. An absent or SQL NULL
// payload decodes as empty; a malformed one errors rather than reading as zero.
func DecodeAppMap[V appMapValue](raw json.RawMessage) (map[string]V, error) {
	var decoded map[string]V
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, xerrors.Errorf("unmarshal session app map: %w", err)
		}
	}
	if decoded == nil {
		decoded = map[string]V{}
	}
	return decoded, nil
}

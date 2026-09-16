package codersdk

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
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
	// AppFamilySFTP only ever comes from history the fixed sftp_mins column
	// recorded. No agent reports it.
	AppFamilySFTP    AppFamilyName = "sftp"
	AppFamilyUnknown AppFamilyName = "unknown"
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
	"sftp":      AppFamilySFTP,
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
	return maps.Clone(appNameFamilies)
}

// SumByFamily folds a per-app map into per-family totals. Values are
// additive, so usage two apps of one family share counts in each. App names
// with no registry entry total under AppFamilyUnknown rather than being
// dropped.
func SumByFamily(byApp map[string]int64) map[AppFamilyName]int64 {
	byFamily := make(map[AppFamilyName]int64, len(byApp))
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

// SessionCountsByFamilyJSON is SumByFamily over the jsonb object of
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
	return SumByFamily(counts), nil
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

// DecodeAppMap decodes a JSONB payload keyed by app name. An absent payload
// decodes to an empty map, but a malformed one is an error so that callers
// report the failure instead of reporting zero usage.
func DecodeAppMap[V any](raw json.RawMessage) (map[string]V, error) {
	if len(raw) == 0 {
		return map[string]V{}, nil
	}
	var decoded map[string]V
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, xerrors.Errorf("unmarshal session app map: %w", err)
	}
	return decoded, nil
}

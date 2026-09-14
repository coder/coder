package agentmcp

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
)

// pluginEnvAllowList names the base environment variables a plugin
// stdio server inherits. Everything else in the agent's environment is
// dropped before the entry's own env is applied.
var pluginEnvAllowList = map[string]struct{}{
	"PATH":                {},
	"HOME":                {},
	"USER":                {},
	"LOGNAME":             {},
	"SHELL":               {},
	"TERM":                {},
	"TMPDIR":              {},
	"TZ":                  {},
	"LANG":                {},
	"LANGUAGE":            {},
	"HTTP_PROXY":          {},
	"HTTPS_PROXY":         {},
	"NO_PROXY":            {},
	"http_proxy":          {},
	"https_proxy":         {},
	"no_proxy":            {},
	"SSL_CERT_FILE":       {},
	"SSL_CERT_DIR":        {},
	"NODE_EXTRA_CA_CERTS": {},
	"REQUESTS_CA_BUNDLE":  {},
}

// pluginEnvAllowListWindows names the additional variables a Windows
// process needs to start and locate its profile and temp directories.
// Windows variable names are case-insensitive, so lookups on Windows
// compare in upper case against this list and pluginEnvAllowList.
var pluginEnvAllowListWindows = map[string]struct{}{
	"SYSTEMROOT":   {},
	"WINDIR":       {},
	"COMSPEC":      {},
	"PATHEXT":      {},
	"TEMP":         {},
	"TMP":          {},
	"USERPROFILE":  {},
	"USERNAME":     {},
	"HOMEDRIVE":    {},
	"HOMEPATH":     {},
	"APPDATA":      {},
	"LOCALAPPDATA": {},
	"PROGRAMDATA":  {},
	"PROGRAMFILES": {},
}

// pluginEnvAllowPrefixes are inherited by prefix match.
var pluginEnvAllowPrefixes = []string{"LC_", "XDG_"}

// filterPluginEnv returns the KEY=value entries of env whose key is on
// the allow-list, preserving order. Entries without "=" are dropped.
func filterPluginEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || !pluginEnvAllowed(key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func pluginEnvAllowed(key string) bool {
	if _, ok := pluginEnvAllowList[key]; ok {
		return true
	}
	if runtime.GOOS == "windows" {
		upper := strings.ToUpper(key)
		if _, ok := pluginEnvAllowList[upper]; ok {
			return true
		}
		if _, ok := pluginEnvAllowListWindows[upper]; ok {
			return true
		}
		key = upper
	}
	for _, prefix := range pluginEnvAllowPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// PluginDataDir returns the PLUGIN_DATA directory for the plugin named
// name installed at root: <home>/.coder/plugin-data/<name>-<hash>, where
// hash is the first 12 hex characters of sha256(root). Keying by root
// keeps two installations of the same plugin name from sharing state.
func PluginDataDir(home, name, root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(home, ".coder", "plugin-data", name+"-"+hex.EncodeToString(sum[:])[:12])
}

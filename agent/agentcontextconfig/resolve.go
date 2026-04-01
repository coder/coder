package agentcontextconfig

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolvePath resolves a single path that may be absolute,
// home-relative (~/ or ~), or relative to the given base
// directory. Returns an absolute path. Empty input returns empty.
func ResolvePath(raw, baseDir string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case raw == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return home
	case strings.HasPrefix(raw, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, raw[2:])
	case filepath.IsAbs(raw):
		return raw
	default:
		if baseDir == "" {
			return ""
		}
		return filepath.Join(baseDir, raw)
	}
}

// ResolvePaths splits a comma-separated list of paths and
// resolves each entry independently. Empty entries and entries
// that resolve to empty strings are skipped.
func ResolvePaths(raw, baseDir string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if resolved := ResolvePath(p, baseDir); resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

package agentcontext

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// CanonicalizePath produces the canonical form of a user-
// supplied path. The result is absolute, has ~ expanded, has
// path-traversal segments collapsed, and has symlinks resolved
// when the target exists. The path is left lexically clean if
// it does not yet exist (so adding a not-yet-created directory
// remains possible).
//
// CanonicalizePath returns the original input when it is empty.
func CanonicalizePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", xerrors.New("path is empty")
	}

	// Expand ~ and ~/ prefixes against the current user's home
	// directory. Other ~user forms are not supported on
	// purpose; the agent runs as a known user.
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", xerrors.Errorf("expand home dir: %w", err)
		}
		if raw == "~" {
			raw = home
		} else {
			raw = filepath.Join(home, raw[2:])
		}
	}

	if !filepath.IsAbs(raw) {
		// Fail closed: relative paths could mean different
		// things depending on the agent's working directory at
		// add-time, so require the caller to absolutize first.
		return "", xerrors.Errorf("path %q is not absolute", raw)
	}

	cleaned := filepath.Clean(raw)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved, nil
	}
	return cleaned, nil
}

// ValidateSourcePath enforces the path-validation rules from
// the RFC's Authorization section. It rejects:
//
//   - Paths containing ".." segments after expansion.
//   - Paths resolving outside the supplied allowedRoots, unless
//     allowedRoots is empty (which disables the check).
//
// allowedRoots are canonicalized lazily; missing roots are
// silently skipped so a workspace with no $HOME does not break
// validation for project-relative roots.
func ValidateSourcePath(canonical string, allowedRoots []string) error {
	if canonical == "" {
		return xerrors.New("path is empty")
	}
	// filepath.Clean drops "." but leaves ".." when no parent
	// is available. Reject defensively.
	for _, part := range strings.Split(canonical, string(os.PathSeparator)) {
		if part == ".." {
			return xerrors.Errorf("path %q contains parent traversal segments", canonical)
		}
	}

	if len(allowedRoots) == 0 {
		return nil
	}

	// Build canonical, deduplicated allowed roots. Missing
	// roots (e.g. an unconfigured ~/.claude/) are skipped.
	roots := make([]string, 0, len(allowedRoots))
	seen := make(map[string]struct{}, len(allowedRoots))
	for _, raw := range allowedRoots {
		c, err := CanonicalizePath(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		roots = append(roots, c)
	}
	if len(roots) == 0 {
		// All configured roots were invalid; treat as "deny
		// everything" so misconfiguration fails closed.
		return xerrors.Errorf("path %q is not inside any allowed root", canonical)
	}

	for _, root := range roots {
		if pathHasPrefix(canonical, root) {
			return nil
		}
	}
	return xerrors.Errorf("path %q is not inside any allowed root", canonical)
}

// pathHasPrefix reports whether path is equal to or a
// descendant of prefix. Both arguments must already be clean,
// absolute paths.
func pathHasPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	withSep := prefix
	if !strings.HasSuffix(withSep, string(os.PathSeparator)) {
		withSep += string(os.PathSeparator)
	}
	return strings.HasPrefix(path, withSep)
}

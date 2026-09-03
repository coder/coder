package agentcontext

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// lexicalPath returns raw as a cleaned, absolute path with ~
// expanded against the current user's home and symlinks left
// unresolved.
func lexicalPath(raw string) (string, error) {
	return lexicalPathIn(os.UserHomeDir, raw)
}

// lexicalPathIn is lexicalPath with home injected. home is called
// only when a ~ prefix needs expanding, so an absolute path resolves
// even when home is unavailable.
func lexicalPathIn(home func() (string, error), raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", xerrors.New("path is empty")
	}

	// ~user forms are intentionally unsupported.
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		h, err := home()
		if err != nil {
			return "", xerrors.Errorf("expand home dir: %w", err)
		}
		if raw == "~" {
			raw = h
		} else {
			raw = filepath.Join(h, raw[2:])
		}
	}

	if !filepath.IsAbs(raw) {
		// Relative paths are ambiguous; require an absolute path.
		return "", xerrors.Errorf("path %q is not absolute", raw)
	}

	return filepath.Clean(raw), nil
}

// CanonicalizePath returns lexicalPath with symlinks resolved
// when the target exists.
func CanonicalizePath(raw string) (string, error) {
	return resolveCanonicalPath(os.UserHomeDir, raw)
}

// CanonicalizePathIn is CanonicalizePath with ~ expanded against
// the given home directory instead of the current user's.
func CanonicalizePathIn(home string, raw string) (string, error) {
	return resolveCanonicalPath(func() (string, error) { return home, nil }, raw)
}

// resolveCanonicalPath implements both CanonicalizePath and
// CanonicalizePathIn.
func resolveCanonicalPath(home func() (string, error), raw string) (string, error) {
	cleaned, err := lexicalPathIn(home, raw)
	if err != nil {
		return "", err
	}
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

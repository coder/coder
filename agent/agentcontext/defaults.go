package agentcontext

// defaultBuiltinRoots returns the scan roots layered after
// user-added sources and before working-directory discovery. These mirror the
// agentcontextconfig API resolves at every chat hydrate. The
// list is intentionally tolerant of missing entries; the
// resolver silently skips canonicalization failures and
// non-existent paths.
func defaultBuiltinRoots() []string {
	return []string{
		// User-level Coder config.
		"~/.coder",
		"~/.coder/skills",
		// Claude Code plugin cache. Claude's .claude-plugin
		// manifests are not read.
		"~/.claude/plugins/cache",
	}
}

// defaultAllowedRoots returns the allow-list applied to runtime
// AddSource calls when ManagerOptions.AllowedRoots is empty.
// The set matches the RFC's authorization section: the home
// directory's Coder and Claude config trees. The Manager
// appends the working directory lazily on every check, which
// picks up the workspace's resolved path even when the manifest
// is loaded after agent init.
func defaultAllowedRoots() []string {
	return []string{"~", "~/.coder", "~/.claude"}
}

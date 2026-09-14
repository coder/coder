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
		// Claude Code plugin cache. The resolver does not read
		// Claude's .claude-plugin manifests; the directory is
		// watched so a later classification of its contents does
		// not surprise the watcher with an unwatched root. Agent
		// Plugins (plugin.json) are discovered from the plugins/
		// and .agents/plugins/ containers under each scan root,
		// including ~/.coder above.
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

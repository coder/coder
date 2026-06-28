package agentcontext

// defaultBuiltinRoots returns the scan roots layered in front
// of any user-added sources. These mirror the paths the legacy
// agentcontextconfig API resolves at every chat hydrate. The
// list is intentionally tolerant of missing entries; the
// resolver silently skips canonicalization failures and
// non-existent paths.
func defaultBuiltinRoots() []string {
	return []string{
		// User-level Coder config.
		"~/.coder",
		"~/.coder/skills",
		// Claude Code plugin cache, picked up by the plugin
		// RFC follow-up. v1 ignores plugin manifests, but
		// watching the directory now prevents a surprise
		// dirty bit when the resolver eventually classifies
		// them.
		"~/.claude/plugins/cache",
	}
}

// defaultAllowedRoots returns the allow-list applied to runtime
// AddSource calls when ManagerOptions.AllowedRoots is empty.
// The set is the home directory's Coder and Claude config
// subtrees only. Bare ~ is deliberately excluded: allowing it
// would make the entire home directory (including ~/.ssh and
// other secret-bearing files) a valid source root. The Manager
// appends the working directory lazily on every check, which
// picks up the workspace's resolved path even when the manifest
// is loaded after agent init.
func defaultAllowedRoots() []string {
	return []string{"~/.coder", "~/.claude"}
}

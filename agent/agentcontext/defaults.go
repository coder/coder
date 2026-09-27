package agentcontext

// globalInstructionsRoot is the directory whose instruction files apply to
// the whole conversation rather than to a directory tree.
const globalInstructionsRoot = "~/.coder"

// defaultBuiltinRoots returns the scan roots layered after
// user-added sources and before working-directory discovery. These mirror the
// agentcontextconfig API resolves at every chat hydrate. The
// list is intentionally tolerant of missing entries; the
// resolver silently skips canonicalization failures and
// non-existent paths.
func defaultBuiltinRoots() []string {
	return []string{
		// User-level Coder config.
		globalInstructionsRoot,
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
// The set matches the RFC's authorization section: the home
// directory's Coder and Claude config trees. The Manager
// appends the working directory lazily on every check, which
// picks up the workspace's resolved path even when the manifest
// is loaded after agent init.
func defaultAllowedRoots() []string {
	return []string{"~", "~/.coder", "~/.claude"}
}

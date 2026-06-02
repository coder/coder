package agentcontext

// DefaultBuiltinRoots returns the built-in scan roots that the
// resolver layers in front of any user-added sources. These
// mirror the paths the legacy agentcontextconfig API resolves
// at every chat hydrate, so the new resolver covers the same
// surface area without callers having to keep the lists in
// sync.
//
// The slice is intentionally tolerant of missing entries; the
// resolver silently skips canonicalization failures and
// non-existent paths.
func DefaultBuiltinRoots() []string {
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

// DefaultAllowedRoots returns the allow-list applied to runtime
// AddSource calls. The set matches the RFC's authorization
// section: the home directory's Coder and Claude config trees.
// The Manager appends the working directory lazily on every
// check, which picks up the workspace's resolved path even when
// the manifest is loaded after agent init.
func DefaultAllowedRoots() []string {
	return []string{"~", "~/.coder", "~/.claude"}
}

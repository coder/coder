package agent

import (
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
)

// defaultContextRoots returns the built-in scan roots layered
// in front of any user-added sources. These mirror the paths
// the existing agentcontextconfig API resolves at every chat
// hydrate so the new resolver covers the same surface area.
//
// The slice is intentionally tolerant of missing entries; the
// resolver silently skips canonicalization failures and
// non-existent paths.
func defaultContextRoots() []string {
	roots := make([]string, 0, 8)

	// Working directory is added by the manager itself via the
	// WorkingDir option, so we do not include it here.

	// User home Coder config (~/.coder, ~/.coder/skills).
	roots = append(roots, "~/.coder", "~/.coder/skills")

	// Claude Code plugin cache, picked up by the plugin RFC
	// follow-up. v1 ignores plugin manifests but watching the
	// directory is harmless and prevents a surprise dirty bit
	// when the resolver eventually classifies them.
	roots = append(roots, "~/.claude/plugins/cache")

	// Project-relative ".agents/skills" requires a working
	// directory to anchor. We let the manager append the
	// working directory itself, and the resolver picks up
	// nested ".agents/skills" automatically.

	return roots
}

// initialContextSources translates the boot-time
// CODER_AGENT_EXP_*_DIRS env vars into agentcontext.Source
// entries. This preserves the "set it on the template" workflow
// while the user-facing CLI for source CRUD ships in a follow-up.
func initialContextSources(cfg agentcontextconfig.Config, workingDir func() string) []agentcontext.Source {
	base := ""
	if workingDir != nil {
		base = workingDir()
	}

	seen := make(map[string]struct{})
	var sources []agentcontext.Source
	add := func(path string) {
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		sources = append(sources, agentcontext.Source{Path: path})
	}
	for _, p := range agentcontextconfig.ResolvePaths(cfg.InstructionsDirs, base) {
		add(p)
	}
	for _, p := range agentcontextconfig.ResolvePaths(cfg.SkillsDirs, base) {
		add(p)
	}
	for _, p := range agentcontextconfig.ResolvePaths(cfg.MCPConfigFiles, base) {
		add(p)
	}
	return sources
}

// defaultContextAllowedRoots returns the allow-list applied to
// runtime AddSource calls. The set matches the RFC's authorization
// section: the home directory's Coder + Claude config trees. The
// Manager appends the working directory lazily on every check,
// which picks up the workspace's resolved path even when the
// manifest is loaded after agent init.
func defaultContextAllowedRoots() []string {
	return []string{"~", "~/.coder", "~/.claude"}
}

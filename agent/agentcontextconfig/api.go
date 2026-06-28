package agentcontextconfig

import (
	"cmp"
	"os"
	"strings"
)

// Env var names for context configuration. Prefixed with EXP_
// to indicate these are experimental and may change.
const (
	EnvInstructionsDirs = "CODER_AGENT_EXP_INSTRUCTIONS_DIRS"
	EnvSkillsDirs       = "CODER_AGENT_EXP_SKILLS_DIRS"
	EnvMCPConfigFiles   = "CODER_AGENT_EXP_MCP_CONFIG_FILES"

	// EnvInstructionsFile and EnvSkillMetaFile are retained only so
	// ClearEnvVars still strips them from the environment. The agent
	// no longer honors a custom instruction or skill-meta filename:
	// the context resolver keys on fixed conventional names
	// (AGENTS.md, CLAUDE.md, .cursorrules, and SKILL.md). Nothing
	// parses these values.
	EnvInstructionsFile = "CODER_AGENT_EXP_INSTRUCTIONS_FILE"
	EnvSkillMetaFile    = "CODER_AGENT_EXP_SKILL_META_FILE"
)

// Default values for agent-internal configuration. These are
// used when the corresponding env vars are unset.
//
// DefaultSkillsDir is a comma-separated list of home-scoped and
// project-scoped skill roots; the context resolver applies
// builtin/home precedence when the same skill name appears in
// more than one root.
const (
	DefaultInstructionsDir = "~/.coder"
	DefaultSkillsDir       = "~/.coder/skills,.agents/skills"
	DefaultMCPConfigFile   = ".mcp.json"
)

// Config holds the agent's context configuration.
// Defaults are applied by NewAPI, not by the zero value.
type Config struct {
	InstructionsDirs string
	SkillsDirs       string
	MCPConfigFiles   string
}

// applyDefaults fills zero-valued fields with their defaults.
func (c Config) applyDefaults() Config {
	c.InstructionsDirs = cmp.Or(c.InstructionsDirs, DefaultInstructionsDir)
	c.SkillsDirs = cmp.Or(c.SkillsDirs, DefaultSkillsDir)
	c.MCPConfigFiles = cmp.Or(c.MCPConfigFiles, DefaultMCPConfigFile)
	return c
}

// ReadEnvConfig reads the CODER_AGENT_EXP_* environment
// variables, falling back to defaults for unset values.
func ReadEnvConfig() Config {
	return Config{
		InstructionsDirs: strings.TrimSpace(os.Getenv(EnvInstructionsDirs)),
		SkillsDirs:       strings.TrimSpace(os.Getenv(EnvSkillsDirs)),
		MCPConfigFiles:   strings.TrimSpace(os.Getenv(EnvMCPConfigFiles)),
	}.applyDefaults()
}

// envVarKeys returns every CODER_AGENT_EXP_* env var key
// recognized by the context configuration subsystem, including
// the ignored filename overrides so ClearEnvVars still strips
// them.
func envVarKeys() []string {
	return []string{
		EnvInstructionsDirs, EnvInstructionsFile,
		EnvSkillsDirs, EnvSkillMetaFile, EnvMCPConfigFiles,
	}
}

// ClearEnvVars removes the CODER_AGENT_EXP_* environment
// variables from the current process so they are not
// inherited by child processes.
func ClearEnvVars() {
	for _, key := range envVarKeys() {
		_ = os.Unsetenv(key)
	}
}

// API resolves the agent's MCP configuration file paths for the
// MCP manager. The working directory closure is evaluated lazily
// so it picks up the workspace directory once the manifest loads.
type API struct {
	workingDir func() string
	cfg        Config
}

// NewAPI creates a context configuration API. The working
// directory closure is evaluated lazily per call.
func NewAPI(workingDir func() string, cfg Config) *API {
	if workingDir == nil {
		workingDir = func() string { return "" }
	}
	return &API{workingDir: workingDir, cfg: cfg.applyDefaults()}
}

// MCPConfigFiles returns the resolved MCP configuration file
// paths for the agent's MCP manager.
func (api *API) MCPConfigFiles() []string {
	return ResolvePaths(api.cfg.MCPConfigFiles, api.workingDir())
}

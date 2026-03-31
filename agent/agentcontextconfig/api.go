package agentcontextconfig

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Env var names for context configuration.
const (
	EnvInstructionsDir  = "CODER_AGENT_INSTRUCTIONS_DIR"
	EnvInstructionsFile = "CODER_AGENT_INSTRUCTIONS_FILE"
	EnvSkillsDir        = "CODER_AGENT_SKILLS_DIR"
	EnvSkillMetaFile    = "CODER_AGENT_SKILL_META_FILE"
	EnvMCPConfigFile    = "CODER_AGENT_MCP_CONFIG_FILE"
)

// Defaults used when env vars are unset.
const (
	DefaultInstructionsDir  = "~/.coder"
	DefaultInstructionsFile = "AGENTS.md"
	DefaultSkillsDir        = ".agents/skills"
	DefaultSkillMetaFile    = "SKILL.md"
	DefaultMCPConfigFile    = ".mcp.json"
)

// API exposes the resolved context configuration through the
// agent's HTTP API.
type API struct {
	logger slog.Logger
	config workspacesdk.ContextConfigResponse
}

// NewAPI reads context configuration from environment variables,
// resolves all paths relative to workingDir, and returns an API
// handler that serves the result.
func NewAPI(logger slog.Logger, workingDir string) *API {
	return &API{
		logger: logger,
		config: BuildConfig(workingDir),
	}
}

// BuildConfig reads env vars and resolves paths. Exported for
// use by the MCP manager and tests.
func BuildConfig(workingDir string) workspacesdk.ContextConfigResponse {
	instructionsDir := os.Getenv(EnvInstructionsDir)
	if instructionsDir == "" {
		instructionsDir = DefaultInstructionsDir
	}
	instructionsFile := os.Getenv(EnvInstructionsFile)
	if instructionsFile == "" {
		instructionsFile = DefaultInstructionsFile
	} else {
		instructionsFile = strings.TrimSpace(instructionsFile)
	}
	skillsDir := os.Getenv(EnvSkillsDir)
	if skillsDir == "" {
		skillsDir = DefaultSkillsDir
	}
	skillMetaFile := os.Getenv(EnvSkillMetaFile)
	if skillMetaFile == "" {
		skillMetaFile = DefaultSkillMetaFile
	} else {
		skillMetaFile = strings.TrimSpace(skillMetaFile)
	}
	mcpConfigFile := os.Getenv(EnvMCPConfigFile)
	if mcpConfigFile == "" {
		mcpConfigFile = DefaultMCPConfigFile
	}

	return workspacesdk.ContextConfigResponse{
		InstructionsDirs: ResolvePaths(instructionsDir, workingDir),
		InstructionsFile: instructionsFile,
		SkillsDirs:       ResolvePaths(skillsDir, workingDir),
		SkillMetaFile:    skillMetaFile,
		MCPConfigFiles:   ResolvePaths(mcpConfigFile, workingDir),
	}
}

// Config returns the resolved config for use by other agent
// components (e.g. MCP manager).
func (api *API) Config() workspacesdk.ContextConfigResponse {
	return api.config
}

// Routes returns the HTTP handler for the context config
// endpoint.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", api.handleGet)
	return r
}

func (api *API) handleGet(rw http.ResponseWriter, r *http.Request) {
	httpapi.Write(r.Context(), rw, http.StatusOK, api.config)
}

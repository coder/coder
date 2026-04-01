package agentcontextconfig

import (
	"cmp"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Env var names for context configuration. Prefixed with EXP_
// to indicate these are experimental and may change.
const (
	EnvInstructionsDirs = "CODER_AGENT_EXP_INSTRUCTIONS_DIRS"
	EnvInstructionsFile = "CODER_AGENT_EXP_INSTRUCTIONS_FILE"
	EnvSkillsDirs       = "CODER_AGENT_EXP_SKILLS_DIRS"
	EnvSkillMetaFile    = "CODER_AGENT_EXP_SKILL_META_FILE"
	EnvMCPConfigFiles   = "CODER_AGENT_EXP_MCP_CONFIG_FILES"
)

// Defaults are defined in codersdk/workspacesdk so both
// the agent and server can reference them without a
// cross-layer import.

// API exposes the resolved context configuration through the
// agent's HTTP API.
type API struct {
	config workspacesdk.ContextConfigResponse
}

// NewAPI reads context configuration from environment variables,
// resolves all paths relative to workingDir, and returns an API
// handler that serves the result.
func NewAPI(workingDir string) *API {
	return &API{
		config: Config(workingDir),
	}
}

// Config reads env vars and resolves paths. Exported for use
// by the MCP manager and tests.
func Config(workingDir string) workspacesdk.ContextConfigResponse {
	// TrimSpace is applied only to file-name vars (basenames)
	// because stray whitespace would silently break lookups.
	// Directory vars go through ResolvePaths which splits on
	// commas and trims each element.
	instructionsDir := cmp.Or(os.Getenv(EnvInstructionsDirs), workspacesdk.DefaultInstructionsDir)
	instructionsFile := cmp.Or(strings.TrimSpace(os.Getenv(EnvInstructionsFile)), workspacesdk.DefaultInstructionsFile)
	skillsDir := cmp.Or(os.Getenv(EnvSkillsDirs), workspacesdk.DefaultSkillsDir)
	skillMetaFile := cmp.Or(strings.TrimSpace(os.Getenv(EnvSkillMetaFile)), workspacesdk.DefaultSkillMetaFile)
	mcpConfigFile := cmp.Or(os.Getenv(EnvMCPConfigFiles), workspacesdk.DefaultMCPConfigFile)

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

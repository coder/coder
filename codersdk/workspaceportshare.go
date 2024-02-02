package codersdk

const (
	WorkspaceAgentPortShareLevelOwner         WorkspacePortShareLevel = 0
	WorkspaceAgentPortShareLevelAuthenticated WorkspacePortShareLevel = 1
	WorkspaceAgentPortShareLevelPublic        WorkspacePortShareLevel = 2
)

type (
	WorkspacePortShareLevel                   int
	UpdateWorkspaceAgentPortShareLevelRequest struct {
		AgentName  string `json:"agent_name"`
		Port       int32  `json:"port"`
		ShareLevel int32  `json:"share_level"`
	}
)

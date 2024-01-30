package codersdk

const (
	WorkspaceAgentPortSharingLevelOwner         WorkspacePortSharingLevel = 0
	WorkspaceAgentPortSharingLevelAuthenticated WorkspacePortSharingLevel = 1
	WorkspaceAgentPortSharingLevelPublic        WorkspacePortSharingLevel = 2
)

type WorkspacePortSharingLevel int
type UpdateWorkspaceAgentPortSharingLevelRequest struct {
	AgentName  string `json:"agent_name"`
	Port       int    `json:"port"`
	ShareLevel int    `json:"share_level"`
}

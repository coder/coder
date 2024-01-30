package codersdk

type UpdatePortSharingLevelRequest struct {
	AgentName  string `json:"agent_name"`
	Port       int    `json:"port"`
	ShareLevel int    `json:"share_level"`
}

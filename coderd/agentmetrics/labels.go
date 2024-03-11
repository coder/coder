package agentmetrics

const (
	LabelAgentName     = "agent_name"
	LabelUsername      = "username"
	LabelTemplateName  = "template_name"
	LabelWorkspaceName = "workspace_name"
)

var (
	LabelAll        = []string{LabelAgentName, LabelTemplateName, LabelUsername, LabelWorkspaceName}
	LabelAgentStats = []string{LabelAgentName, LabelUsername, LabelWorkspaceName}
)

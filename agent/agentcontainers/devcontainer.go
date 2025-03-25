package agentcontainers

import (
	"fmt"

	"github.com/coder/coder/v2/codersdk"
)

func DevcontainerStartupScript(dc codersdk.WorkspaceAgentDevcontainer, script codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScript {
	cmd := fmt.Sprintf("devcontainer up --workspace-folder %q", dc.WorkspaceFolder)
	if dc.ConfigPath != "" {
		cmd = fmt.Sprintf("%s --config %q", cmd, dc.ConfigPath)
	}
	script.Script = cmd
	return script
}

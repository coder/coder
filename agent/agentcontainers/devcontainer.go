package agentcontainers

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

func DevcontainerStartupScript(dc codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentScript {
	script := fmt.Sprintf("devcontainer up --workspace-folder %q", dc.WorkspaceFolder)
	if dc.ConfigPath != "" {
		script = fmt.Sprintf("%s --config %q", script, dc.ConfigPath)
	}
	return codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.Nil, // TODO(mafredri): Add a devcontainer log source?
		LogPath:     "",
		Script:      script,
		Cron:        "",
		Timeout:     0,
		DisplayName: fmt.Sprintf("Dev Container (%s)", dc.WorkspaceFolder),
	}
}

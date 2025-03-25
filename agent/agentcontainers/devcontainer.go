package agentcontainers

import (
	"context"
	"fmt"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/codersdk"
)

func DevcontainerStartupScript(dc codersdk.WorkspaceAgentDevcontainer, script codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScript {
	cmd := fmt.Sprintf("devcontainer up --workspace-folder %q", dc.WorkspaceFolder)
	if dc.ConfigPath != "" {
		cmd = fmt.Sprintf("%s --config %q", cmd, dc.ConfigPath)
	}
	script.RunOnStart = false
	script.Script = cmd
	return script
}

func ExtractDevcontainerScripts(
	logger slog.Logger,
	expandPath func(string) (string, error),
	devcontainers []codersdk.WorkspaceAgentDevcontainer,
	scripts []codersdk.WorkspaceAgentScript,
) (other []codersdk.WorkspaceAgentScript, devcontainerScripts []codersdk.WorkspaceAgentScript) {
	for _, dc := range devcontainers {
		dc = expandDevcontainerPaths(logger, expandPath, dc)
		for _, script := range scripts {
			// The devcontainer scripts match the devcontainer ID for
			// identification.
			if script.ID == dc.ID {
				devcontainerScripts = append(devcontainerScripts, DevcontainerStartupScript(dc, script))
			} else {
				other = append(other, script)
			}
		}
	}

	return other, devcontainerScripts
}

func expandDevcontainerPaths(logger slog.Logger, expandPath func(string) (string, error), dc codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
	var err error
	if dc.WorkspaceFolder, err = expandPath(dc.WorkspaceFolder); err != nil {
		logger.Warn(context.Background(), "expand devcontainer workspace folder failed", slog.Error(err))
	}
	if dc.ConfigPath != "" {
		if dc.ConfigPath, err = expandPath(dc.ConfigPath); err != nil {
			logger.Warn(context.Background(), "expand devcontainer config path failed", slog.Error(err))
		}
	}
	return dc
}

package agentcontainers

import (
	"context"
	"fmt"
	"strings"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/codersdk"
)

const devcontainerUpScriptTemplate = `
if ! which devcontainer > /dev/null 2>&1; then
  echo "ERROR: Unable to start devcontainer, @devcontainers/cli is not installed."
  exit 1
fi
devcontainer up %s
`

// DevcontainerStartupScript returns a script that starts a devcontainer.
func DevcontainerStartupScript(dc codersdk.WorkspaceAgentDevcontainer, script codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScript {
	var args []string
	args = append(args, fmt.Sprintf("--workspace-folder %q", dc.WorkspaceFolder))
	if dc.ConfigPath != "" {
		args = append(args, fmt.Sprintf("--config %q", dc.ConfigPath))
	}
	cmd := fmt.Sprintf(devcontainerUpScriptTemplate, strings.Join(args, " "))
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

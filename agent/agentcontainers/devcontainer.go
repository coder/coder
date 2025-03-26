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

// ExtractAndInitializeDevcontainerScripts extracts devcontainer scripts from
// the given scripts and devcontainers. The devcontainer scripts are removed
// from the returned scripts so that they can be run separately.
//
// Dev Containers have an inherent dependency on start scripts, since they
// initialize the workspace (e.g. git clone, npm install, etc). This is
// important if e.g. a Coder module to install @devcontainer/cli is used.
func ExtractAndInitializeDevcontainerScripts(
	logger slog.Logger,
	expandPath func(string) (string, error),
	devcontainers []codersdk.WorkspaceAgentDevcontainer,
	scripts []codersdk.WorkspaceAgentScript,
) (filteredScripts []codersdk.WorkspaceAgentScript, devcontainerScripts []codersdk.WorkspaceAgentScript) {
ScriptLoop:
	for _, script := range scripts {
		for _, dc := range devcontainers {
			// The devcontainer scripts match the devcontainer ID for
			// identification.
			if script.ID == dc.ID {
				dc = expandDevcontainerPaths(logger, expandPath, dc)
				devcontainerScripts = append(devcontainerScripts, devcontainerStartupScript(dc, script))
				continue ScriptLoop
			}
		}

		filteredScripts = append(filteredScripts, script)
	}

	return filteredScripts, devcontainerScripts
}

func devcontainerStartupScript(dc codersdk.WorkspaceAgentDevcontainer, script codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScript {
	var args []string
	args = append(args, fmt.Sprintf("--workspace-folder %q", dc.WorkspaceFolder))
	if dc.ConfigPath != "" {
		args = append(args, fmt.Sprintf("--config %q", dc.ConfigPath))
	}
	cmd := fmt.Sprintf(devcontainerUpScriptTemplate, strings.Join(args, " "))
	script.Script = cmd
	// Disable RunOnStart, scripts have this set so that when devcontainers
	// have not been enabled, a warning will be surfaced in the agent logs.
	script.RunOnStart = false
	return script
}

func expandDevcontainerPaths(logger slog.Logger, expandPath func(string) (string, error), dc codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
	if wf, err := expandPath(dc.WorkspaceFolder); err != nil {
		logger.Warn(context.Background(), "expand devcontainer workspace folder failed", slog.Error(err))
	} else {
		dc.WorkspaceFolder = wf
	}
	if dc.ConfigPath != "" {
		if cp, err := expandPath(dc.ConfigPath); err != nil {
			logger.Warn(context.Background(), "expand devcontainer config path failed", slog.Error(err))
		} else {
			dc.ConfigPath = cp
		}
	}
	return dc
}

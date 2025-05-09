package agentcontainers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// DevcontainerLocalFolderLabel is the label that contains the path to
	// the local workspace folder for a devcontainer.
	DevcontainerLocalFolderLabel = "devcontainer.local_folder"
	// DevcontainerConfigFileLabel is the label that contains the path to
	// the devcontainer.json configuration file.
	DevcontainerConfigFileLabel = "devcontainer.config_file"
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
	devcontainers []codersdk.WorkspaceAgentDevcontainer,
	scripts []codersdk.WorkspaceAgentScript,
) (filteredScripts []codersdk.WorkspaceAgentScript, devcontainerScripts []codersdk.WorkspaceAgentScript) {
ScriptLoop:
	for _, script := range scripts {
		for _, dc := range devcontainers {
			// The devcontainer scripts match the devcontainer ID for
			// identification.
			if script.ID == dc.ID {
				devcontainerScripts = append(devcontainerScripts, devcontainerStartupScript(dc, script))
				continue ScriptLoop
			}
		}

		filteredScripts = append(filteredScripts, script)
	}

	return filteredScripts, devcontainerScripts
}

func devcontainerStartupScript(dc codersdk.WorkspaceAgentDevcontainer, script codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScript {
	args := []string{
		"--log-format json",
		fmt.Sprintf("--workspace-folder %q", dc.WorkspaceFolder),
	}
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

// ExpandAllDevcontainerPaths expands all devcontainer paths in the given
// devcontainers. This is required by the devcontainer CLI, which requires
// absolute paths for the workspace folder and config path.
func ExpandAllDevcontainerPaths(logger slog.Logger, expandPath func(string) (string, error), devcontainers []codersdk.WorkspaceAgentDevcontainer) []codersdk.WorkspaceAgentDevcontainer {
	expanded := make([]codersdk.WorkspaceAgentDevcontainer, 0, len(devcontainers))
	for _, dc := range devcontainers {
		expanded = append(expanded, expandDevcontainerPaths(logger, expandPath, dc))
	}
	return expanded
}

func expandDevcontainerPaths(logger slog.Logger, expandPath func(string) (string, error), dc codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
	logger = logger.With(slog.F("devcontainer", dc.Name), slog.F("workspace_folder", dc.WorkspaceFolder), slog.F("config_path", dc.ConfigPath))

	if wf, err := expandPath(dc.WorkspaceFolder); err != nil {
		logger.Warn(context.Background(), "expand devcontainer workspace folder failed", slog.Error(err))
	} else {
		dc.WorkspaceFolder = wf
	}
	if dc.ConfigPath != "" {
		// Let expandPath handle home directory, otherwise assume relative to
		// workspace folder or absolute.
		if dc.ConfigPath[0] == '~' {
			if cp, err := expandPath(dc.ConfigPath); err != nil {
				logger.Warn(context.Background(), "expand devcontainer config path failed", slog.Error(err))
			} else {
				dc.ConfigPath = cp
			}
		} else {
			dc.ConfigPath = relativePathToAbs(dc.WorkspaceFolder, dc.ConfigPath)
		}
	}
	return dc
}

func relativePathToAbs(workdir, path string) string {
	path = os.ExpandEnv(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(workdir, path)
	}
	return path
}

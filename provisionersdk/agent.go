package provisionersdk

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

var (
	// These used to be hard-coded, but after growing significantly more complex
	// it made sense to put them in their own files (e.g. for linting).
	//go:embed scripts/bootstrap_windows.ps1
	windowsScript string
	//go:embed scripts/bootstrap_linux.sh
	linuxScript string
	//go:embed scripts/bootstrap_darwin.sh
	darwinScript string

	// A mapping of operating-system ($GOOS) to architecture ($GOARCH)
	// to agent install and run script. ${DOWNLOAD_URL} is replaced
	// with strings.ReplaceAll() when being consumed. ${ARCH} is replaced
	// with the architecture when being provided.
	agentScripts = map[string]map[string]string{
		"windows": {
			"amd64": windowsScript,
			"arm64": windowsScript,
		},
		"linux": {
			"amd64": linuxScript,
			"arm64": linuxScript,
			"armv7": linuxScript,
		},
		"darwin": {
			"amd64": darwinScript,
			"arm64": darwinScript,
		},
	}
)

// AgentScriptEnv returns a key-pair of scripts that are consumed
// by the Coder Terraform Provider. See:
// https://github.com/coder/terraform-provider-coder/blob/main/internal/provider/provider.go#L97
func AgentScriptEnv() map[string]string {
	env := map[string]string{}
	for operatingSystem, scripts := range agentScripts {
		for architecture, script := range scripts {
			script := strings.ReplaceAll(script, "${ARCH}", architecture)
			env[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", operatingSystem, architecture)] = script
		}
	}
	return env
}

// DefaultDisplayApps returns the default display applications to enable
// if none are specified in a template.
func DefaultDisplayApps() *proto.DisplayApps {
	return &proto.DisplayApps{
		Vscode:               true,
		VscodeInsiders:       false,
		WebTerminal:          true,
		PortForwardingHelper: true,
		SshHelper:            true,
	}
}

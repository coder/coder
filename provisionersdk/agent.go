package provisionersdk

import (
	"fmt"
	"strings"
)

var (
	// On Windows, VS Code Remote requires a parent process of the
	// executing shell to be named "sshd", otherwise it fails. See:
	// https://github.com/microsoft/vscode-remote-release/issues/5699
	windowsScript = `$ProgressPreference = "SilentlyContinue"
Invoke-WebRequest -Uri ${ACCESS_URL}bin/coder-windows-${ARCH}.exe -OutFile $env:TEMP\sshd.exe
Set-MpPreference -DisableRealtimeMonitoring $true -ExclusionPath $env:TEMP\sshd.exe
$env:CODER_AGENT_AUTH = "${AUTH_TYPE}"
$env:CODER_AGENT_URL = "${ACCESS_URL}"
Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -PassThru`

	linuxScript = `#!/usr/bin/env sh
set -eux pipefail
trap 'echo === Agent script exited with non-zero code. Sleeping infinitely to preserve logs... && sleep 86400' EXIT
BINARY_DIR=$(mktemp -d -t coder.XXXXXX)
BINARY_NAME=coder
BINARY_URL=${ACCESS_URL}bin/coder-linux-${ARCH}
cd $BINARY_DIR
if command -v curl >/dev/null 2>&1; then
	curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}"
elif command -v wget >/dev/null 2>&1; then
	wget -q "${BINARY_URL}" -O "${BINARY_NAME}"
elif command -v busybox >/dev/null 2>&1; then
	busybox wget -q "${BINARY_URL}" -O "${BINARY_NAME}"
else
	echo "error: no download tool found, please install curl, wget or busybox wget"
	exit 1
fi
chmod +x $BINARY_NAME
export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME agent`

	darwinScript = `#!/usr/bin/env sh
set -eux pipefail
trap 'echo === Agent script exited with non-zero code. Sleeping infinitely to preserve logs... && sleep 86400' EXIT
BINARY_DIR=$(mktemp -d -t coder.XXXXXX)
BINARY_NAME=coder
cd $BINARY_DIR
curl -fsSL --compressed "${ACCESS_URL}bin/coder-darwin-${ARCH}" -o "${BINARY_NAME}"
chmod +x $BINARY_NAME
export CODER_AGENT_AUTH="${AUTH_TYPE}"
export CODER_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME agent`

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

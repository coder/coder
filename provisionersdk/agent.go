package provisionersdk

import "fmt"

var (
	// A mapping of operating-system ($GOOS) to architecture ($GOARCH)
	// to agent install and run script. ${DOWNLOAD_URL} is replaced
	// with strings.ReplaceAll() when being consumed.
	agentScripts = map[string]map[string]string{
		// On Windows, VS Code Remote requires a parent process of the
		// executing shell to be named "sshd", otherwise it fails. See:
		// https://github.com/microsoft/vscode-remote-release/issues/5699
		"windows": {
			"amd64": `
$ProgressPreference = "SilentlyContinue"
Invoke-WebRequest -Uri ${ACCESS_URL}bin/coder-windows-amd64.exe -OutFile $env:TEMP\sshd.exe
$env:CODER_AUTH = "${AUTH_TYPE}"
$env:CODER_URL = "${ACCESS_URL}"
Start-Process -FilePath $env:TEMP\sshd.exe -ArgumentList "agent" -PassThru
`,
		},
		"linux": {
			"amd64": `
#!/usr/bin/env sh
set -eu pipefail
export BINARY_LOCATION=$(mktemp -d -t tmp.coderXXXXX)/coder
curl -fsSL ${ACCESS_URL}bin/coder-linux-amd64 -o $BINARY_LOCATION
chmod +x $BINARY_LOCATION
export CODER_AUTH="${AUTH_TYPE}"
export CODER_URL="${ACCESS_URL}"
exec $BINARY_LOCATION agent
`,
		},
		"darwin": {
			"amd64": `
#!/usr/bin/env sh
set -eu pipefail
export BINARY_LOCATION=$(mktemp -d -t tmp.coderXXXXX)/coder
curl -fsSL ${ACCESS_URL}bin/coder-darwin-amd64 -o $BINARY_LOCATION
chmod +x $BINARY_LOCATION
export CODER_AUTH="${AUTH_TYPE}"
export CODER_URL="${ACCESS_URL}"
exec $BINARY_LOCATION agent
`,
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
			env[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", operatingSystem, architecture)] = script
		}
	}
	return env
}

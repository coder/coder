package provisionersdk

import (
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

var (
	// A mapping of operating-system ($GOOS) to architecture ($GOARCH)
	// to agent install and run script. ${DOWNLOAD_URL} is replaced
	// with strings.ReplaceAll() when being consumed.
	agentScript = map[string]map[string]string{
		"windows": {
			"amd64": `
$ProgressPreference = "SilentlyContinue"
$ErrorActionPreference = "Stop"
Invoke-WebRequest -Uri ${DOWNLOAD_URL} -OutFile $env:TEMP\coder.exe
Start-Process -FilePath $env:TEMP\coder.exe workspaces agent
`,
		},
		"linux": {
			"amd64": `
#!/usr/bin/env sh
set -eu pipefail
BINARY_LOCATION=$(mktemp -d)/coder
curl -fsSL ${DOWNLOAD_URL} -o $BINARY_LOCATION
chmod +x $BINARY_LOCATION
exec $BINARY_LOCATION agent
`,
		},
		"darwin": {
			"amd64": `
#!/usr/bin/env sh
set -eu pipefail
BINARY_LOCATION=$(mktemp -d)/coder
curl -fsSL ${DOWNLOAD_URL} -o $BINARY_LOCATION
chmod +x $BINARY_LOCATION
exec $BINARY_LOCATION agent
`,
		},
	}
)

// AgentBinaryName returns the binary name for an operating system and agent.
func AgentBinaryName(operatingSystem, architecture string) (string, error) {

}

// AgentScript returns an installation script for the specified operating system
// and architecture.
//
// baseURL is
func AgentScript(operatingSystem, architecture string, baseURL *url.URL) (string, error) {
	architectures, ok := agentScript[operatingSystem]
	if !ok {
		list := []string{}
		for key := range agentScript {
			list = append(list, key)
		}
		return "", xerrors.Errorf("operating system %q not supported. must be in: %v", operatingSystem, list)
	}
	script, ok := architectures[architecture]
	if !ok {
		list := []string{}
		for key := range architectures {
			list = append(list, key)
		}
		return "", xerrors.Errorf("architecture %q not supported for %q. must be in: %v", architecture, operatingSystem, list)
	}
	if !strings.HasPrefix(baseURL.Path, "/api/v2/downloads") {

	}

	return strings.ReplaceAll(script, "${DOWNLOAD_URL}", baseURL.String()), nil
}

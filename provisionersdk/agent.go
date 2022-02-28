package provisionersdk

import (
	"fmt"
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

// AgentScript returns an installation script for the specified operating system
// and architecture.
func AgentScript(coderURL *url.URL, operatingSystem, architecture string) (string, error) {
	architectures, exists := agentScript[operatingSystem]
	if !exists {
		list := []string{}
		for key := range agentScript {
			list = append(list, key)
		}
		return "", xerrors.Errorf("operating system %q not supported. must be in: %v", operatingSystem, list)
	}
	script, exists := architectures[architecture]
	if !exists {
		list := []string{}
		for key := range architectures {
			list = append(list, key)
		}
		return "", xerrors.Errorf("architecture %q not supported for %q. must be in: %v", architecture, operatingSystem, list)
	}
	parsed, err := coderURL.Parse(fmt.Sprintf("/bin/coder-%s-%s", operatingSystem, architecture))
	if err != nil {
		return "", xerrors.Errorf("parse url: %w", err)
	}
	return strings.ReplaceAll(script, "${DOWNLOAD_URL}", parsed.String()), nil
}

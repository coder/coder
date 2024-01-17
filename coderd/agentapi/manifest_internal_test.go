package agentapi

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

func Test_vscodeProxyURI(t *testing.T) {
	t.Parallel()

	coderAccessURL, err := url.Parse("https://coder.com")
	require.NoError(t, err)

	accessURLWithPort, err := url.Parse("https://coder.com:8080")
	require.NoError(t, err)

	basicApp := appurl.ApplicationURL{
		Prefix:        "prefix",
		AppSlugOrPort: "slug",
		AgentName:     "agent",
		WorkspaceName: "workspace",
		Username:      "user",
	}

	cases := []struct {
		Name        string
		App         appurl.ApplicationURL
		AccessURL   *url.URL
		AppHostname string
		Expected    string
	}{
		{
			// No hostname proxies through the access url.
			Name:        "NoHostname",
			AccessURL:   coderAccessURL,
			AppHostname: "",
			App:         basicApp,
			Expected:    coderAccessURL.String(),
		},
		{
			Name:        "NoHostnameAccessURLPort",
			AccessURL:   accessURLWithPort,
			AppHostname: "",
			App:         basicApp,
			Expected:    accessURLWithPort.String(),
		},
		{
			Name:        "Hostname",
			AccessURL:   coderAccessURL,
			AppHostname: "*.apps.coder.com",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com", basicApp.String()),
		},
		{
			Name:        "HostnameWithAccessURLPort",
			AccessURL:   accessURLWithPort,
			AppHostname: "*.apps.coder.com",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com:%s", basicApp.String(), accessURLWithPort.Port()),
		},
		{
			Name:        "HostnameWithPort",
			AccessURL:   coderAccessURL,
			AppHostname: "*.apps.coder.com:4444",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com:%s", basicApp.String(), "4444"),
		},
		{
			// Port from hostname takes precedence over access url port.
			Name:        "HostnameWithPortAccessURLWithPort",
			AccessURL:   accessURLWithPort,
			AppHostname: "*.apps.coder.com:4444",
			App:         basicApp,
			Expected:    fmt.Sprintf("https://%s.apps.coder.com:%s", basicApp.String(), "4444"),
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			require.NotNilf(t, c.AccessURL, "AccessURL is required")

			output := vscodeProxyURI(c.App, c.AccessURL, c.AppHostname)
			require.Equal(t, c.Expected, output)
		})
	}
}

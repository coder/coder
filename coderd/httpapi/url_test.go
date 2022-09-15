package httpapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpapi"
)

func TestSplitSubdomain(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name              string
		Host              string
		ExpectedSubdomain string
		ExpectedRest      string
		ExpectedErr       string
	}{
		{
			Name:              "Empty",
			Host:              "",
			ExpectedSubdomain: "",
			ExpectedRest:      "",
			ExpectedErr:       "no subdomain",
		},
		{
			Name:              "NoSubdomain",
			Host:              "com",
			ExpectedSubdomain: "",
			ExpectedRest:      "",
			ExpectedErr:       "no subdomain",
		},
		{
			Name:              "Domain",
			Host:              "coder.com",
			ExpectedSubdomain: "coder",
			ExpectedRest:      "com",
			ExpectedErr:       "",
		},
		{
			Name:              "Subdomain",
			Host:              "subdomain.coder.com",
			ExpectedSubdomain: "subdomain",
			ExpectedRest:      "coder.com",
			ExpectedErr:       "",
		},
		{
			Name:              "DoubleSubdomain",
			Host:              "subdomain1.subdomain2.coder.com",
			ExpectedSubdomain: "subdomain1",
			ExpectedRest:      "subdomain2.coder.com",
			ExpectedErr:       "",
		},
		{
			Name:              "WithPort",
			Host:              "subdomain.coder.com:8080",
			ExpectedSubdomain: "subdomain",
			ExpectedRest:      "coder.com:8080",
			ExpectedErr:       "",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			subdomain, rest, err := httpapi.SplitSubdomain(c.Host)
			if c.ExpectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.ExpectedErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, c.ExpectedSubdomain, subdomain)
			require.Equal(t, c.ExpectedRest, rest)
		})
	}
}

func TestApplicationURLString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		URL      httpapi.ApplicationURL
		Expected string
	}{
		{
			Name:     "Empty",
			URL:      httpapi.ApplicationURL{},
			Expected: "------.",
		},
		{
			Name: "AppName",
			URL: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
			Expected: "app--agent--workspace--user.coder.com",
		},
		{
			Name: "Port",
			URL: httpapi.ApplicationURL{
				AppName:       "",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
			Expected: "8080--agent--workspace--user.coder.com",
		},
		{
			Name: "Both",
			URL: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
			// Prioritizes port over app name.
			Expected: "8080--agent--workspace--user.coder.com",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.Expected, c.URL.String())
		})
	}
}

func TestParseSubdomainAppURL(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name          string
		Host          string
		Expected      httpapi.ApplicationURL
		ExpectedError string
	}{
		{
			Name:          "Invalid_Split",
			Host:          "com",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "no subdomain",
		},
		{
			Name:          "Invalid_Empty",
			Host:          "example.com",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace.Agent--App",
			Host:          "workspace.agent--app.coder.com",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace--App",
			Host:          "workspace--app.coder.com",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_App--Workspace--User",
			Host:          "app--workspace--user.coder.com",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_TooManyComponents",
			Host:          "1--2--3--4--5.coder.com",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		// Correct
		{
			Name: "AppName--Agent--Workspace--User",
			Host: "app--agent--workspace--user.coder.com",
			Expected: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
		},
		{
			Name: "Port--Agent--Workspace--User",
			Host: "8080--agent--workspace--user.coder.com",
			Expected: httpapi.ApplicationURL{
				AppName:       "",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "coder.com",
			},
		},
		{
			Name: "DeepSubdomain",
			Host: "app--agent--workspace--user.dev.dean-was-here.coder.com",
			Expected: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
				BaseHostname:  "dev.dean-was-here.coder.com",
			},
		},
		{
			Name: "HyphenatedNames",
			Host: "app-name--agent-name--workspace-name--user-name.coder.com",
			Expected: httpapi.ApplicationURL{
				AppName:       "app-name",
				Port:          0,
				AgentName:     "agent-name",
				WorkspaceName: "workspace-name",
				Username:      "user-name",
				BaseHostname:  "coder.com",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			app, err := httpapi.ParseSubdomainAppURL(c.Host)
			if c.ExpectedError == "" {
				require.NoError(t, err)
				require.Equal(t, c.Expected, app, "expected app")
			} else {
				require.ErrorContains(t, err, c.ExpectedError, "expected error")
			}
		})
	}
}

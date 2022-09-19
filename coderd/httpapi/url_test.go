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
	}{
		{
			Name:              "Empty",
			Host:              "",
			ExpectedSubdomain: "",
			ExpectedRest:      "",
		},
		{
			Name:              "NoSubdomain",
			Host:              "com",
			ExpectedSubdomain: "com",
			ExpectedRest:      "",
		},
		{
			Name:              "Domain",
			Host:              "coder.com",
			ExpectedSubdomain: "coder",
			ExpectedRest:      "com",
		},
		{
			Name:              "Subdomain",
			Host:              "subdomain.coder.com",
			ExpectedSubdomain: "subdomain",
			ExpectedRest:      "coder.com",
		},
		{
			Name:              "DoubleSubdomain",
			Host:              "subdomain1.subdomain2.coder.com",
			ExpectedSubdomain: "subdomain1",
			ExpectedRest:      "subdomain2.coder.com",
		},
		{
			Name:              "WithPort",
			Host:              "subdomain.coder.com:8080",
			ExpectedSubdomain: "subdomain",
			ExpectedRest:      "coder.com:8080",
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			subdomain, rest := httpapi.SplitSubdomain(c.Host)
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
			Expected: "------",
		},
		{
			Name: "AppName",
			URL: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
			Expected: "app--agent--workspace--user",
		},
		{
			Name: "Port",
			URL: httpapi.ApplicationURL{
				AppName:       "",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
			Expected: "8080--agent--workspace--user",
		},
		{
			Name: "Both",
			URL: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
			// Prioritizes port over app name.
			Expected: "8080--agent--workspace--user",
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
		Subdomain     string
		Expected      httpapi.ApplicationURL
		ExpectedError string
	}{
		{
			Name:          "Invalid_Empty",
			Subdomain:     "test",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace.Agent--App",
			Subdomain:     "workspace.agent--app",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace--App",
			Subdomain:     "workspace--app",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_App--Workspace--User",
			Subdomain:     "app--workspace--user",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_TooManyComponents",
			Subdomain:     "1--2--3--4--5",
			Expected:      httpapi.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		// Correct
		{
			Name:      "AppName--Agent--Workspace--User",
			Subdomain: "app--agent--workspace--user",
			Expected: httpapi.ApplicationURL{
				AppName:       "app",
				Port:          0,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
		},
		{
			Name:      "Port--Agent--Workspace--User",
			Subdomain: "8080--agent--workspace--user",
			Expected: httpapi.ApplicationURL{
				AppName:       "",
				Port:          8080,
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
		},
		{
			Name:      "HyphenatedNames",
			Subdomain: "app-name--agent-name--workspace-name--user-name",
			Expected: httpapi.ApplicationURL{
				AppName:       "app-name",
				Port:          0,
				AgentName:     "agent-name",
				WorkspaceName: "workspace-name",
				Username:      "user-name",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			app, err := httpapi.ParseSubdomainAppURL(c.Subdomain)
			if c.ExpectedError == "" {
				require.NoError(t, err)
				require.Equal(t, c.Expected, app, "expected app")
			} else {
				require.ErrorContains(t, err, c.ExpectedError, "expected error")
			}
		})
	}
}

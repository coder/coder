package appurl_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

func TestApplicationURLString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		URL      appurl.ApplicationURL
		Expected string
	}{
		{
			Name:     "Empty",
			URL:      appurl.ApplicationURL{},
			Expected: "------",
		},
		{
			Name: "AppName",
			URL: appurl.ApplicationURL{
				AppSlugOrPort: "app",
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
			Expected: "app--agent--workspace--user",
		},
		{
			Name: "Port",
			URL: appurl.ApplicationURL{
				AppSlugOrPort: "8080",
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
			Expected: "8080--agent--workspace--user",
		},
		{
			Name: "Prefix",
			URL: appurl.ApplicationURL{
				Prefix:        "yolo---",
				AppSlugOrPort: "app",
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
			Expected: "yolo---app--agent--workspace--user",
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
		Expected      appurl.ApplicationURL
		ExpectedError string
	}{
		{
			Name:          "Invalid_Empty",
			Subdomain:     "test",
			Expected:      appurl.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace.Agent--App",
			Subdomain:     "workspace.agent--app",
			Expected:      appurl.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_Workspace--App",
			Subdomain:     "workspace--app",
			Expected:      appurl.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_App--Workspace--User",
			Subdomain:     "app--workspace--user",
			Expected:      appurl.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		{
			Name:          "Invalid_TooManyComponents",
			Subdomain:     "1--2--3--4--5",
			Expected:      appurl.ApplicationURL{},
			ExpectedError: "invalid application url format",
		},
		// Correct
		{
			Name:      "AppName--Agent--Workspace--User",
			Subdomain: "app--agent--workspace--user",
			Expected: appurl.ApplicationURL{
				AppSlugOrPort: "app",
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
		},
		{
			Name:      "Port--Agent--Workspace--User",
			Subdomain: "8080--agent--workspace--user",
			Expected: appurl.ApplicationURL{
				AppSlugOrPort: "8080",
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
		},
		{
			Name:      "HyphenatedNames",
			Subdomain: "app-slug--agent-name--workspace-name--user-name",
			Expected: appurl.ApplicationURL{
				AppSlugOrPort: "app-slug",
				AgentName:     "agent-name",
				WorkspaceName: "workspace-name",
				Username:      "user-name",
			},
		},
		{
			Name:      "Prefix",
			Subdomain: "dean---was---here---app--agent--workspace--user",
			Expected: appurl.ApplicationURL{
				Prefix:        "dean---was---here---",
				AppSlugOrPort: "app",
				AgentName:     "agent",
				WorkspaceName: "workspace",
				Username:      "user",
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			app, err := appurl.ParseSubdomainAppURL(c.Subdomain)
			if c.ExpectedError == "" {
				require.NoError(t, err)
				require.Equal(t, c.Expected, app, "expected app")
			} else {
				require.ErrorContains(t, err, c.ExpectedError, "expected error")
			}
		})
	}
}

func TestCompileHostnamePattern(t *testing.T) {
	t.Parallel()

	type matchCase struct {
		input string
		// empty string denotes no match
		match string
	}

	type testCase struct {
		name          string
		pattern       string
		errorContains string
		// expectedRegex only needs to contain the inner part of the regex, not
		// the prefix and suffix checks.
		expectedRegex string
		matchCases    []matchCase
	}

	testCases := []testCase{
		{
			name:          "Invalid_ContainsHTTP",
			pattern:       "http://*.hi.com",
			errorContains: "must not contain a scheme",
		},
		{
			name:          "Invalid_ContainsHTTPS",
			pattern:       "https://*.hi.com",
			errorContains: "must not contain a scheme",
		},
		{
			name:          "Invalid_ContainsPort",
			pattern:       "*.hi.com:8080",
			errorContains: "must not contain a port",
		},
		{
			name:          "Invalid_StartPeriod",
			pattern:       ".hi.com",
			errorContains: "must not start or end with a period",
		},
		{
			name:          "Invalid_EndPeriod",
			pattern:       "hi.com.",
			errorContains: "must not start or end with a period",
		},
		{
			name:          "Invalid_Empty",
			pattern:       "",
			errorContains: "must contain at least two labels",
		},
		{
			name:          "Invalid_SingleLabel",
			pattern:       "hi",
			errorContains: "must contain at least two labels",
		},
		{
			name:          "Invalid_NoWildcard",
			pattern:       "hi.com",
			errorContains: "must contain exactly one asterisk",
		},
		{
			name:          "Invalid_MultipleWildcards",
			pattern:       "**.hi.com",
			errorContains: "must contain exactly one asterisk",
		},
		{
			name:          "Invalid_WildcardNotFirst",
			pattern:       "hi.*.com",
			errorContains: "must only contain an asterisk at the beginning",
		},
		{
			name:          "Invalid_BadLabel1",
			pattern:       "*.h_i.com",
			errorContains: "contains invalid label",
		},
		{
			name:          "Invalid_BadLabel2",
			pattern:       "*.hi-.com",
			errorContains: "contains invalid label",
		},
		{
			name:          "Invalid_BadLabel3",
			pattern:       "*.-hi.com",
			errorContains: "contains invalid label",
		},

		{
			name:          "Valid_Simple",
			pattern:       "*.hi",
			expectedRegex: `([^.]+)\.hi`,
			matchCases: []matchCase{
				{
					input: "hi",
					match: "",
				},
				{
					input: "hi.com",
					match: "",
				},
				{
					input: "hi.hi.hi",
					match: "",
				},
				{
					input: "abcd.hi",
					match: "abcd",
				},
				{
					input: "abcd.hi.",
					match: "abcd",
				},
				{
					input: "  abcd.hi.  ",
					match: "abcd",
				},
				{
					input: "abcd.hi:8080",
					match: "abcd",
				},
				{
					input: "ab__invalid__cd-.hi",
					// Invalid subdomains still match the pattern because they
					// managed to make it to the webserver anyways.
					match: "ab__invalid__cd-",
				},
			},
		},
		{
			name:          "Valid_MultiLevel",
			pattern:       "*.hi.com",
			expectedRegex: `([^.]+)\.hi\.com`,
			matchCases: []matchCase{
				{
					input: "hi.com",
					match: "",
				},
				{
					input: "abcd.hi.com",
					match: "abcd",
				},
				{
					input: "ab__invalid__cd-.hi.com",
					match: "ab__invalid__cd-",
				},
			},
		},
		{
			name:          "Valid_WildcardSuffix1",
			pattern:       `*a.hi.com`,
			expectedRegex: `([^.]+)a\.hi\.com`,
			matchCases: []matchCase{
				{
					input: "hi.com",
					match: "",
				},
				{
					input: "abcd.hi.com",
					match: "",
				},
				{
					input: "ab__invalid__cd-.hi.com",
					match: "",
				},
				{
					input: "abcda.hi.com",
					match: "abcd",
				},
				{
					input: "ab__invalid__cd-a.hi.com",
					match: "ab__invalid__cd-",
				},
			},
		},
		{
			name:          "Valid_WildcardSuffix2",
			pattern:       `*-test.hi.com`,
			expectedRegex: `([^.]+)-test\.hi\.com`,
			matchCases: []matchCase{
				{
					input: "hi.com",
					match: "",
				},
				{
					input: "abcd.hi.com",
					match: "",
				},
				{
					input: "ab__invalid__cd-.hi.com",
					match: "",
				},
				{
					input: "abcd-test.hi.com",
					match: "abcd",
				},
				{
					input: "ab__invalid__cd-test.hi.com",
					match: "ab__invalid__cd",
				},
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			regex, err := appurl.CompileHostnamePattern(c.pattern)
			if c.errorContains == "" {
				require.NoError(t, err)

				expected := `^\s*` + c.expectedRegex + `\.?(:\d+)?\s*$`
				require.Equal(t, expected, regex.String(), "generated regex does not match")

				for i, m := range c.matchCases {
					m := m
					t.Run(fmt.Sprintf("MatchCase%d", i), func(t *testing.T) {
						t.Parallel()

						match, ok := appurl.ExecuteHostnamePattern(regex, m.input)
						if m.match == "" {
							require.False(t, ok)
						} else {
							require.True(t, ok)
							require.Equal(t, m.match, match)
						}
					})
				}
			} else {
				require.Error(t, err)
				require.ErrorContains(t, err, c.errorContains)
			}
		})
	}
}

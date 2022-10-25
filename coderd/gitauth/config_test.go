package gitauth_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
)

func TestConvertYAML(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		Name   string
		Input  []codersdk.GitAuthConfig
		Output []*gitauth.Config
		Error  string
	}{{
		Name: "InvalidType",
		Input: []codersdk.GitAuthConfig{{
			Type: "moo",
		}},
		Error: "unknown git provider type",
	}, {
		Name: "InvalidID",
		Input: []codersdk.GitAuthConfig{{
			Type: codersdk.GitProviderGitHub,
			ID:   "$hi$",
		}},
		Error: "doesn't have a valid id",
	}, {
		Name: "NoClientID",
		Input: []codersdk.GitAuthConfig{{
			Type: codersdk.GitProviderGitHub,
		}},
		Error: "client_id must be provided",
	}, {
		Name: "NoClientSecret",
		Input: []codersdk.GitAuthConfig{{
			Type:     codersdk.GitProviderGitHub,
			ClientID: "example",
		}},
		Error: "client_secret must be provided",
	}, {
		Name: "DuplicateType",
		Input: []codersdk.GitAuthConfig{{
			Type:         codersdk.GitProviderGitHub,
			ClientID:     "example",
			ClientSecret: "example",
		}, {
			Type: codersdk.GitProviderGitHub,
		}},
		Error: "multiple github git auth providers provided",
	}, {
		Name: "InvalidRegex",
		Input: []codersdk.GitAuthConfig{{
			Type:         codersdk.GitProviderGitHub,
			ClientID:     "example",
			ClientSecret: "example",
			Regex:        `\K`,
		}},
		Error: "compile regex for git auth provider",
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			output, err := gitauth.ConvertConfig(tc.Input, &url.URL{})
			if tc.Error != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.Error)
				return
			}
			require.Equal(t, tc.Output, output)
		})
	}
}

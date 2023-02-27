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
			Type: string(codersdk.GitProviderGitHub),
			ID:   "$hi$",
		}},
		Error: "doesn't have a valid id",
	}, {
		Name: "NoClientID",
		Input: []codersdk.GitAuthConfig{{
			Type: string(codersdk.GitProviderGitHub),
		}},
		Error: "client_id must be provided",
	}, {
		Name: "NoClientSecret",
		Input: []codersdk.GitAuthConfig{{
			Type:     string(codersdk.GitProviderGitHub),
			ClientID: "example",
		}},
		Error: "client_secret must be provided",
	}, {
		Name: "DuplicateType",
		Input: []codersdk.GitAuthConfig{{
			Type:         string(codersdk.GitProviderGitHub),
			ClientID:     "example",
			ClientSecret: "example",
		}, {
			Type: string(codersdk.GitProviderGitHub),
		}},
		Error: "multiple github git auth providers provided",
	}, {
		Name: "InvalidRegex",
		Input: []codersdk.GitAuthConfig{{
			Type:         string(codersdk.GitProviderGitHub),
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

	t.Run("CustomScopesAndEndpoint", func(t *testing.T) {
		t.Parallel()
		config, err := gitauth.ConvertConfig([]codersdk.GitAuthConfig{{
			Type:         string(codersdk.GitProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
			AuthURL:      "https://auth.com",
			TokenURL:     "https://token.com",
			Scopes:       []string{"read"},
		}}, &url.URL{})
		require.NoError(t, err)
		require.Equal(t, "https://auth.com?client_id=id&redirect_uri=%2Fgitauth%2Fgitlab%2Fcallback&response_type=code&scope=read", config[0].AuthCodeURL(""))
	})
}

package gitauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestRefreshToken(t *testing.T) {
	t.Parallel()
	t.Run("FalseIfNoRefresh", func(t *testing.T) {
		t.Parallel()
		config := &gitauth.Config{
			NoRefresh: true,
		}
		_, refreshed, err := config.RefreshToken(context.Background(), nil, database.GitAuthLink{
			OAuthExpiry: time.Time{},
		})
		require.NoError(t, err)
		require.False(t, refreshed)
	})
	t.Run("FalseIfTokenSourceFails", func(t *testing.T) {
		t.Parallel()
		config := &gitauth.Config{
			OAuth2Config: &testutil.OAuth2Config{
				TokenSourceFunc: func() (*oauth2.Token, error) {
					return nil, xerrors.New("failure")
				},
			},
		}
		_, refreshed, err := config.RefreshToken(context.Background(), nil, database.GitAuthLink{})
		require.NoError(t, err)
		require.False(t, refreshed)
	})
	t.Run("ValidateServerError", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failure"))
		}))
		config := &gitauth.Config{
			OAuth2Config: &testutil.OAuth2Config{},
			ValidateURL:  srv.URL,
		}
		_, _, err := config.RefreshToken(context.Background(), nil, database.GitAuthLink{})
		require.ErrorContains(t, err, "Failure")
	})
	t.Run("ValidateFailure", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Not permitted"))
		}))
		config := &gitauth.Config{
			OAuth2Config: &testutil.OAuth2Config{},
			ValidateURL:  srv.URL,
		}
		_, refreshed, err := config.RefreshToken(context.Background(), nil, database.GitAuthLink{})
		require.NoError(t, err)
		require.False(t, refreshed)
	})
	t.Run("ValidateNoUpdate", func(t *testing.T) {
		t.Parallel()
		validated := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			close(validated)
		}))
		accessToken := "testing"
		config := &gitauth.Config{
			OAuth2Config: &testutil.OAuth2Config{
				Token: &oauth2.Token{
					AccessToken: accessToken,
				},
			},
			ValidateURL: srv.URL,
		}
		_, valid, err := config.RefreshToken(context.Background(), nil, database.GitAuthLink{
			OAuthAccessToken: accessToken,
		})
		require.NoError(t, err)
		require.True(t, valid)
		<-validated
	})
	t.Run("Updates", func(t *testing.T) {
		t.Parallel()
		config := &gitauth.Config{
			ID: "test",
			OAuth2Config: &testutil.OAuth2Config{
				Token: &oauth2.Token{
					AccessToken: "updated",
				},
			},
		}
		db := dbfake.New()
		link := dbgen.GitAuthLink(t, db, database.GitAuthLink{
			ProviderID:       config.ID,
			OAuthAccessToken: "initial",
		})
		_, valid, err := config.RefreshToken(context.Background(), db, link)
		require.NoError(t, err)
		require.True(t, valid)
	})
}

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
	}, {
		Name: "NoDeviceURL",
		Input: []codersdk.GitAuthConfig{{
			Type:         string(codersdk.GitProviderGitLab),
			ClientID:     "example",
			ClientSecret: "example",
			DeviceFlow:   true,
		}},
		Error: "device auth url must be provided",
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

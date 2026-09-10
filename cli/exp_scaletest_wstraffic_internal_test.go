//go:build !slim

package cli

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestCreateWorkspaceAppConfig(t *testing.T) {
	t.Parallel()

	newClient := func(t *testing.T, rawURL string) *codersdk.Client {
		t.Helper()
		u, err := url.Parse(rawURL)
		require.NoError(t, err)
		return codersdk.New(u)
	}

	ws := codersdk.Workspace{
		ID:        uuid.New(),
		Name:      "myws",
		OwnerName: "alice",
	}

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		cfg, err := createWorkspaceAppConfig(newClient(t, "https://coder.example.com"), "", "", ws, codersdk.WorkspaceAgent{Name: "main"})
		require.NoError(t, err)
		require.Empty(t, cfg.Name)
		require.Empty(t, cfg.URL)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		agent := codersdk.WorkspaceAgent{Name: "main"}
		_, err := createWorkspaceAppConfig(newClient(t, "https://coder.example.com"), "", "missing", ws, agent)
		require.Error(t, err)
	})

	t.Run("PathAppHasTrailingSlash", func(t *testing.T) {
		t.Parallel()
		agent := codersdk.WorkspaceAgent{
			Name: "main",
			Apps: []codersdk.WorkspaceApp{{Slug: "wsec", Subdomain: false}},
		}
		cfg, err := createWorkspaceAppConfig(newClient(t, "https://coder.example.com"), "*.apps.example.com", "wsec", ws, agent)
		require.NoError(t, err)
		require.Equal(t, "wsec", cfg.Name)
		// The trailing slash avoids coderd's path-app normalization redirect,
		// which the scaletest client rejects.
		require.Equal(t, "https://coder.example.com/@alice/myws.main/apps/wsec/", cfg.URL)
	})

	t.Run("SubdomainApp", func(t *testing.T) {
		t.Parallel()
		agent := codersdk.WorkspaceAgent{
			Name: "main",
			Apps: []codersdk.WorkspaceApp{{Slug: "wsec", Subdomain: true, SubdomainName: "wsec--main--myws--alice"}},
		}
		cfg, err := createWorkspaceAppConfig(newClient(t, "https://coder.example.com"), "*.apps.example.com", "wsec", ws, agent)
		require.NoError(t, err)
		require.Equal(t, "https://wsec--main--myws--alice.apps.example.com", cfg.URL)
	})

	t.Run("SubdomainAppRequiresAppHost", func(t *testing.T) {
		t.Parallel()
		agent := codersdk.WorkspaceAgent{
			Name: "main",
			Apps: []codersdk.WorkspaceApp{{Slug: "wsec", Subdomain: true, SubdomainName: "wsec--main--myws--alice"}},
		}
		_, err := createWorkspaceAppConfig(newClient(t, "https://coder.example.com"), "", "wsec", ws, agent)
		require.Error(t, err)
	})
}

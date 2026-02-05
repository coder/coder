package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/pretty"
)

func TestCurrentOrganization(t *testing.T) {
	t.Parallel()

	// This test emulates 2 cases:
	// 1. The user is not a part of the default organization, but only belongs to one.
	// 2. The user is connecting to an older Coder instance.
	t.Run("no-default", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.New()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]codersdk.Organization{
				{
					MinimalOrganization: codersdk.MinimalOrganization{
						ID:   orgID,
						Name: "not-default",
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					IsDefault: false,
				},
			})
		}))
		defer srv.Close()

		client := codersdk.New(must(url.Parse(srv.URL)))
		inv, root := clitest.New(t, "organizations", "show", "selected")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(orgID.String())
	})
}

func TestOrganizationDelete(t *testing.T) {
	t.Parallel()

	t.Run("Yes", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.New()
		var deleteCalled atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/my-org":
				_ = json.NewEncoder(w).Encode(codersdk.Organization{
					MinimalOrganization: codersdk.MinimalOrganization{
						ID:   orgID,
						Name: "my-org",
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				})
			case r.Method == http.MethodDelete && r.URL.Path == fmt.Sprintf("/api/v2/organizations/%s", orgID.String()):
				deleteCalled.Store(true)
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client := codersdk.New(must(url.Parse(server.URL)))
		inv, root := clitest.New(t, "organizations", "delete", "my-org", "--yes")
		clitest.SetupConfig(t, client, root)

		require.NoError(t, inv.Run())
		require.True(t, deleteCalled.Load(), "expected delete request")
	})

	t.Run("Prompted", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.New()
		var deleteCalled atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/my-org":
				_ = json.NewEncoder(w).Encode(codersdk.Organization{
					MinimalOrganization: codersdk.MinimalOrganization{
						ID:   orgID,
						Name: "my-org",
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				})
			case r.Method == http.MethodDelete && r.URL.Path == fmt.Sprintf("/api/v2/organizations/%s", orgID.String()):
				deleteCalled.Store(true)
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client := codersdk.New(must(url.Parse(server.URL)))
		inv, root := clitest.New(t, "organizations", "delete", "my-org")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		pty.ExpectMatch(fmt.Sprintf("Delete organization %s?", pretty.Sprint(cliui.DefaultStyles.Code, "my-org")))
		pty.WriteLine("yes")

		require.NoError(t, <-execDone)
		require.True(t, deleteCalled.Load(), "expected delete request")
	})

	t.Run("Default", func(t *testing.T) {
		t.Parallel()

		orgID := uuid.New()
		var deleteCalled atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/default":
				_ = json.NewEncoder(w).Encode(codersdk.Organization{
					MinimalOrganization: codersdk.MinimalOrganization{
						ID:   orgID,
						Name: "default",
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					IsDefault: true,
				})
			case r.Method == http.MethodDelete:
				deleteCalled.Store(true)
				w.WriteHeader(http.StatusOK)
			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client := codersdk.New(must(url.Parse(server.URL)))
		inv, root := clitest.New(t, "organizations", "delete", "default", "--yes")
		clitest.SetupConfig(t, client, root)

		err := inv.Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "default organization")
		require.False(t, deleteCalled.Load(), "expected no delete request")
	})
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

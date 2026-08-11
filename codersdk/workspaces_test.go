package codersdk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAllWorkspaces(t *testing.T) {
	t.Parallel()

	// newClient serves pages of the given sizes, recording the limit and offset
	// of every request. count is reported as the total on every response.
	newClient := func(t *testing.T, count int, pageSizes ...int) (*codersdk.Client, *[][2]int) {
		t.Helper()
		var requests [][2]int
		page := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			requests = append(requests, [2]int{limit, offset})

			rows := 0
			if page < len(pageSizes) {
				rows = pageSizes[page]
			}
			page++
			workspaces := make([]codersdk.Workspace, rows)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(codersdk.WorkspacesResponse{
				Workspaces: workspaces,
				Count:      count,
			})
		}))
		t.Cleanup(srv.Close)

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		return codersdk.New(u), &requests
	}

	t.Run("SinglePage", func(t *testing.T) {
		t.Parallel()

		client, requests := newClient(t, 3, 3)
		ctx := testutil.Context(t, testutil.WaitShort)

		workspaces, err := client.AllWorkspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces, 3)
		require.Equal(t, [][2]int{{codersdk.WorkspacePageSize, 0}}, *requests)
	})

	t.Run("AdvancesByPageSize", func(t *testing.T) {
		t.Parallel()

		const count = codersdk.WorkspacePageSize*2 + 10
		client, requests := newClient(t, count,
			codersdk.WorkspacePageSize, codersdk.WorkspacePageSize, 10)
		ctx := testutil.Context(t, testutil.WaitShort)

		workspaces, err := client.AllWorkspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces, count)
		require.Equal(t, [][2]int{
			{codersdk.WorkspacePageSize, 0},
			{codersdk.WorkspacePageSize, codersdk.WorkspacePageSize},
			{codersdk.WorkspacePageSize, codersdk.WorkspacePageSize * 2},
		}, *requests)
	})

	// A page can be shorter than the requested limit because the endpoint drops
	// rows after applying the limit, so a short page must not end the scan.
	t.Run("ShortPageContinues", func(t *testing.T) {
		t.Parallel()

		const count = codersdk.WorkspacePageSize + 20
		client, requests := newClient(t, count, 40, 20)
		ctx := testutil.Context(t, testutil.WaitShort)

		workspaces, err := client.AllWorkspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Len(t, workspaces, 60)
		require.Len(t, *requests, 2)
	})

	t.Run("EmptyResult", func(t *testing.T) {
		t.Parallel()

		client, requests := newClient(t, 0, 0)
		ctx := testutil.Context(t, testutil.WaitShort)

		workspaces, err := client.AllWorkspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err)
		require.Empty(t, workspaces)
		require.Len(t, *requests, 1)
	})

	t.Run("OverridesCallerPagination", func(t *testing.T) {
		t.Parallel()

		client, requests := newClient(t, 1, 1)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := client.AllWorkspaces(ctx, codersdk.WorkspaceFilter{
			Limit:  codersdk.WorkspacePageSize + 50,
			Offset: 500,
		})
		require.NoError(t, err)
		require.Equal(t, [][2]int{{codersdk.WorkspacePageSize, 0}}, *requests)
	})
}

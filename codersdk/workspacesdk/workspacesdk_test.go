package workspacesdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"tailscale.com/net/tsaddr"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestWorkspaceRewriteDERPMap(t *testing.T) {
	t.Parallel()
	// This test ensures that RewriteDERPMap mutates built-in DERPs with the
	// client access URL.
	dm := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				EmbeddedRelay: true,
				RegionID:      1,
				Nodes: []*tailcfg.DERPNode{{
					HostName: "bananas.org",
					DERPPort: 1,
				}},
			},
		},
	}
	parsed, err := url.Parse("https://coconuts.org:44558")
	require.NoError(t, err)
	client := workspacesdk.New(codersdk.New(parsed))
	client.RewriteDERPMap(dm)
	region := dm.Regions[1]
	require.True(t, region.EmbeddedRelay)
	require.Len(t, region.Nodes, 1)
	node := region.Nodes[0]
	require.Equal(t, "coconuts.org", node.HostName)
	require.Equal(t, 44558, node.DERPPort)
}

func TestWorkspaceDialerFailure(t *testing.T) {
	t.Parallel()

	// Setup.
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	// Given: a mock HTTP server which mimicks an unreachable database when calling the coordination endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: codersdk.DatabaseNotReachable,
			Detail:  "oops",
		})
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	// When: calling the coordination endpoint.
	dialer := workspacesdk.NewWebsocketDialer(logger, u, &websocket.DialOptions{})
	_, err = dialer.Dial(ctx, nil)

	// Then: an error indicating a database issue is returned, to conditionalize the behavior of the caller.
	require.ErrorIs(t, err, codersdk.ErrDatabaseNotReachable)
}

func TestClient_IsCoderConnectRunning(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workspaceagents/connection", r.URL.Path)
		httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.AgentConnectionInfo{
			HostnameSuffix: "test",
		})
	}))
	defer srv.Close()

	apiURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	sdkClient := codersdk.New(apiURL)
	client := workspacesdk.New(sdkClient)

	// Right name, right IP
	expectedName := fmt.Sprintf(tailnet.IsCoderConnectEnabledFmtString, "test")
	ctxResolveExpected := workspacesdk.WithTestOnlyCoderContextResolver(ctx,
		&fakeResolver{t: t, hostMap: map[string][]net.IP{
			expectedName: {net.ParseIP(tsaddr.CoderServiceIPv6().String())},
		}})

	result, err := client.IsCoderConnectRunning(ctxResolveExpected, workspacesdk.CoderConnectQueryOptions{})
	require.NoError(t, err)
	require.True(t, result)

	// Wrong name
	result, err = client.IsCoderConnectRunning(ctxResolveExpected, workspacesdk.CoderConnectQueryOptions{HostnameSuffix: "coder"})
	require.NoError(t, err)
	require.False(t, result)

	// Not found
	ctxResolveNotFound := workspacesdk.WithTestOnlyCoderContextResolver(ctx,
		&fakeResolver{t: t, err: &net.DNSError{IsNotFound: true}})
	result, err = client.IsCoderConnectRunning(ctxResolveNotFound, workspacesdk.CoderConnectQueryOptions{})
	require.NoError(t, err)
	require.False(t, result)

	// Some other error
	ctxResolverErr := workspacesdk.WithTestOnlyCoderContextResolver(ctx,
		&fakeResolver{t: t, err: xerrors.New("a bad thing happened")})
	_, err = client.IsCoderConnectRunning(ctxResolverErr, workspacesdk.CoderConnectQueryOptions{})
	require.Error(t, err)

	// Right name, wrong IP
	ctxResolverWrongIP := workspacesdk.WithTestOnlyCoderContextResolver(ctx,
		&fakeResolver{t: t, hostMap: map[string][]net.IP{
			expectedName: {net.ParseIP("2001::34")},
		}})
	result, err = client.IsCoderConnectRunning(ctxResolverWrongIP, workspacesdk.CoderConnectQueryOptions{})
	require.NoError(t, err)
	require.False(t, result)
}

type fakeResolver struct {
	t       testing.TB
	hostMap map[string][]net.IP
	err     error
}

func (f *fakeResolver) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	assert.Equal(f.t, "ip6", network)
	return f.hostMap[host], f.err
}

// TestFileEdit_MarshalEmitsDeprecatedKeys pins the coderd->agent
// wire compatibility: the request JSON must keep carrying the
// pre-rename search/replace keys so running agents on older versions
// decode edits while coderd upgrades first.
func TestFileEdit_MarshalEmitsDeprecatedKeys(t *testing.T) {
	t.Parallel()

	edit := workspacesdk.FileEdit{OldText: "old", NewText: "new", ReplaceAll: true}
	data, err := json.Marshal(edit)
	require.NoError(t, err)

	// The pre-rename FileEdit shape an old agent decodes into.
	var oldAgent struct {
		Search     string `json:"search"`
		Replace    string `json:"replace"`
		ReplaceAll bool   `json:"replace_all"`
	}
	require.NoError(t, json.Unmarshal(data, &oldAgent))
	require.Equal(t, "old", oldAgent.Search)
	require.Equal(t, "new", oldAgent.Replace)
	require.True(t, oldAgent.ReplaceAll)

	// The new decode path sees the same values.
	var round workspacesdk.FileEdit
	require.NoError(t, json.Unmarshal(data, &round))
	require.Equal(t, edit, round)
}

// TestFileEdit_UnmarshalAcceptsDeprecatedKeys pins the decode side
// of FileEdit's deprecated-key fallback.
func TestFileEdit_UnmarshalAcceptsDeprecatedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want workspacesdk.FileEdit
	}{
		{
			name: "OldKeysOnly",
			in:   `{"search":"old","replace":"new"}`,
			want: workspacesdk.FileEdit{OldText: "old", NewText: "new"},
		},
		{
			name: "OldKeysWithReplaceAll",
			in:   `{"search":"old","replace":"new","replace_all":true}`,
			want: workspacesdk.FileEdit{OldText: "old", NewText: "new", ReplaceAll: true},
		},
		{
			name: "NewKeysWinWhenSet",
			in:   `{"old_text":"o","new_text":"n","search":"old","replace":"legacy"}`,
			want: workspacesdk.FileEdit{OldText: "o", NewText: "n"},
		},
		{
			name: "ExplicitEmptyNewTextPreserved",
			in:   `{"old_text":"o","new_text":"","search":"old","replace":"legacy"}`,
			want: workspacesdk.FileEdit{OldText: "o", NewText: ""},
		},
		{
			name: "OldKeysDeletion",
			in:   `{"search":"old","replace":""}`,
			want: workspacesdk.FileEdit{OldText: "old", NewText: ""},
		},
		{
			name: "CaseVariantsRejected",
			in:   `{"SEARCH":"old","REPLACE":"new"}`,
			want: workspacesdk.FileEdit{},
		},
		{
			name: "ExactKeyWinsOverCaseVariant",
			in:   `{"search":"old","SEARCH":"evil","replace":"new"}`,
			want: workspacesdk.FileEdit{OldText: "old", NewText: "new"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got workspacesdk.FileEdit
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

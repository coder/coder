package agentrelay_test

import (
	"context"
	"crypto/ed25519"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentrelay"
	"github.com/coder/coder/v2/coderd/x/workspacerelay"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// controlPlane stands in for coderd. It answers with the lease and the
// credential a request arrived with.
func controlPlane(t *testing.T) *url.URL {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/echo", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Seen-Lease", r.Header.Get(workspacerelay.LeaseHeader))
		rw.Header().Set("Seen-Token", r.Header.Get(codersdk.SessionTokenHeader))
		rw.Header().Set("Seen-Host", r.Host)
		_, _ = io.Copy(rw, r.Body)
	})
	mux.HandleFunc("/api/v2/stream", func(rw http.ResponseWriter, r *http.Request) {
		flusher, ok := rw.(http.Flusher)
		if !assert.True(t, ok) {
			return
		}
		_, _ = rw.Write([]byte("first\n"))
		flusher.Flush()
		// The rest only follows once the caller has read the start.
		<-r.Context().Done()
	})
	mux.HandleFunc("/api/v2/socket", func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(r.Header.Get(workspacerelay.LeaseHeader)))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	return parsed
}

func TestRelay(t *testing.T) {
	t.Parallel()

	t.Run("VouchesForRequests", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		target := controlPlane(t)
		var asked atomic.Int32
		relay, err := agentrelay.New(agentrelay.Options{
			Logger:       testutil.Logger(t),
			Clock:        clock,
			ControlPlane: target,
			Lease: func(_ context.Context, req agentsdk.WorkspaceRelayLeaseRequest) (agentsdk.WorkspaceRelayLease, error) {
				asked.Add(1)
				// The daemon proves the key it made.
				assert.NoError(t, workspacerelay.VerifyHolder(req.PublicKey, req.Signature, req.Timestamp, clock.Now()))
				assert.Len(t, req.PublicKey, ed25519.PublicKeySize)
				return agentsdk.WorkspaceRelayLease{Lease: "the-lease", ExpiresAt: clock.Now().Add(time.Minute), Generation: 1}, nil
			},
		})
		require.NoError(t, err)
		defer relay.Close()

		// There is no address before there is a lease.
		require.Empty(t, relay.URL())
		relay.Ensure(ctx)
		require.NotEmpty(t, relay.URL())
		relay.Ensure(ctx)
		require.EqualValues(t, 1, asked.Load(), "a relay that serves does not ask again by itself")

		// seen returns what the control plane saw of a request.
		seen := func(lease string) http.Header {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, relay.URL()+"/api/v2/echo", http.NoBody)
			require.NoError(t, err)
			req.Header.Set(codersdk.SessionTokenHeader, "the-callers-token")
			if lease != "" {
				req.Header.Set(workspacerelay.LeaseHeader, lease)
			}
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			return res.Header
		}

		// The daemon attaches its lease and nothing else. The caller's
		// credential is the caller's.
		header := seen("")
		require.Equal(t, "the-lease", header.Get("Seen-Lease"))
		require.Equal(t, "the-callers-token", header.Get("Seen-Token"))
		require.Equal(t, target.Host, header.Get("Seen-Host"))
		// A caller cannot bring a lease of its own.
		header = seen("somebody-elses-lease")
		require.Equal(t, "the-lease", header.Get("Seen-Lease"))

		// A lease that has run out is not attached. The control plane
		// decides what that means.
		clock.Advance(2 * time.Minute)
		header = seen("")
		require.Empty(t, header.Get("Seen-Lease"))
	})

	t.Run("StreamsAndUpgrades", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		relay, err := agentrelay.New(agentrelay.Options{
			Logger:       testutil.Logger(t),
			ControlPlane: controlPlane(t),
			Lease: func(context.Context, agentsdk.WorkspaceRelayLeaseRequest) (agentsdk.WorkspaceRelayLease, error) {
				return agentsdk.WorkspaceRelayLease{Lease: "the-lease", ExpiresAt: time.Now().Add(time.Minute)}, nil
			},
		})
		require.NoError(t, err)
		defer relay.Close()
		relay.Ensure(ctx)

		// A response is passed on as it arrives.
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, relay.URL()+"/api/v2/stream", nil)
		require.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		start := make([]byte, len("first\n"))
		_, err = io.ReadFull(res.Body, start)
		require.NoError(t, err)
		require.Equal(t, "first\n", string(start))
		cancel()

		// A WebSocket is carried, and vouched for.
		//nolint:bodyclose // The response of a WebSocket handshake has no body to close.
		conn, _, err := websocket.Dial(ctx, relay.URL()+"/api/v2/socket", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "")
		_, message, err := conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, "the-lease", string(message))
	})

	t.Run("NoLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		for _, tc := range []struct {
			status int
			serves bool
		}{
			// The relay is off: processes keep the address the template
			// gave them.
			{status: http.StatusNotFound, serves: false},
			{status: http.StatusInternalServerError, serves: false},
			// Another instance holds the relay, so the relay is on.
			// Processes are given the relay's address already, and
			// their requests go out without a lease until there is one.
			{status: http.StatusConflict, serves: true},
		} {
			relay, err := agentrelay.New(agentrelay.Options{
				Logger:       testutil.Logger(t),
				ControlPlane: controlPlane(t),
				Lease: func(context.Context, agentsdk.WorkspaceRelayLeaseRequest) (agentsdk.WorkspaceRelayLease, error) {
					return agentsdk.WorkspaceRelayLease{}, codersdk.NewTestError(tc.status, http.MethodPost, "/api/v2/workspaceagents/me/relay-lease")
				},
			})
			require.NoError(t, err)
			relay.Ensure(ctx)
			require.Equal(t, tc.serves, relay.URL() != "", "status %d", tc.status)
			if tc.serves {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, relay.URL()+"/api/v2/echo", http.NoBody)
				require.NoError(t, err)
				res, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				_ = res.Body.Close()
				require.Empty(t, res.Header.Get("Seen-Lease"))
			}
			require.NoError(t, relay.Close())
		}
	})
}

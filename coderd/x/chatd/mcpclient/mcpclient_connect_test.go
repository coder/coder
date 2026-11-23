package mcpclient_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/safedial"
)

// blackHoleListener accepts TCP connections and never responds,
// simulating a server (or an edge in front of it) that silently
// drops requests. Returned connections are tracked so the test can
// terminate them.
type blackHoleListener struct {
	ln net.Listener

	mu    sync.Mutex
	conns []net.Conn
}

func newBlackHoleListener(t *testing.T) *blackHoleListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	bh := &blackHoleListener{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			bh.mu.Lock()
			bh.conns = append(bh.conns, conn)
			bh.mu.Unlock()
		}
	}()
	t.Cleanup(bh.close)
	return bh
}

func (b *blackHoleListener) close() {
	_ = b.ln.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.conns {
		_ = c.Close()
	}
	b.conns = nil
}

func (b *blackHoleListener) url() string {
	return "http://" + b.ln.Addr().String()
}

// TestConnectAll_BlackHoledServerBudget is the acceptance test for
// the connect budget: one black-holed server must not delay turn
// preparation beyond the budget, and healthy servers' tools must
// still be discovered. Without external budget enforcement the SDK
// blocks several times past the context deadline (observed: 12s
// for a 2s deadline) because its transport detaches the context.
// loopbackHTTPClient returns a guarded MCP client that allows
// loopback, where every test server in this file listens.
func loopbackHTTPClient() *http.Client {
	return mcpclient.NewHTTPClient(nil, safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
}

func TestConnectAll_BlackHoledServerBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	bh := newBlackHoleListener(t)
	healthy := newTestMCPServer(t, echoTool())

	reaperDone := make(chan struct{}, 2)
	timeout := 1 * time.Second

	start := time.Now()
	tools, summaries, cleanup := mcpclient.ConnectAllForTest(ctx, logger,
		[]mcpclient.Server{
			makeConfig("blackhole", bh.url()),
			makeConfig("healthy", healthy.URL),
		},
		timeout,
		func() { reaperDone <- struct{}{} },
	)
	elapsed := time.Since(start)
	t.Cleanup(cleanup)

	// The budget must hold: well under the SDK's unbounded
	// behavior (6x the deadline), with margin for slow CI.
	require.Less(t, elapsed, 4*timeout,
		"ConnectAll took %s, budget was %s", elapsed, timeout)
	require.Equal(t, []string{"healthy__echo"}, toolNames(tools))

	// Per-server outcomes are summarized for logs and debug runs,
	// sorted by slug.
	require.Len(t, summaries, 2)
	require.Equal(t, "blackhole", summaries[0].Slug)
	require.Equal(t, mcpclient.ConnectOutcomeTimeout, summaries[0].Outcome)
	require.NotEmpty(t, summaries[0].Error)
	require.GreaterOrEqual(t, summaries[0].DurationMS, timeout.Milliseconds())
	require.Equal(t, "healthy", summaries[1].Slug)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[1].Outcome)
	require.Equal(t, 1, summaries[1].ToolCount)

	// Terminating the black-holed connections unblocks the
	// abandoned connect goroutine; the reaper must then drain its
	// result and exit.
	bh.close()
	select {
	case <-reaperDone:
	case <-time.After(30 * time.Second):
		t.Fatal("reaper did not exit after black-holed connections were closed")
	}
}

// TestConnectAll_SlowServerStillConnects proves that a server that
// is slow but within the budget still connects and serves tools.
func TestConnectAll_SlowServerStillConnects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "slow", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	cfg := makeConfig("slow", ts.URL)
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, loopbackHTTPClient())
	t.Cleanup(cleanup)

	require.Equal(t, []string{"slow__echo"}, toolNames(tools))
}

// TestConnectAll_LateServerReaped proves that when a server only
// responds after the budget expired, ConnectAll has long returned
// and the abandoned connect goroutine's late result is drained by
// the reaper so nothing leaks.
func TestConnectAll_LateServerReaped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "late", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	// The server holds every request until ConnectAll has given up on it.
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	reaperDone := make(chan struct{}, 1)
	timeout := 500 * time.Millisecond

	start := time.Now()
	tools, summaries, cleanup := mcpclient.ConnectAllForTest(ctx, logger,
		[]mcpclient.Server{makeConfig("late", ts.URL)},
		timeout,
		func() { reaperDone <- struct{}{} },
	)
	elapsed := time.Since(start)
	t.Cleanup(cleanup)
	close(release)

	require.Less(t, elapsed, 4*timeout,
		"ConnectAll took %s, budget was %s", elapsed, timeout)
	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeTimeout, summaries[0].Outcome)

	select {
	case <-reaperDone:
	case <-time.After(30 * time.Second):
		t.Fatal("reaper did not exit after the late server responded")
	}
}

// TestConnectAll_CleanupPromptWhenServerWedges proves that the
// cleanup function returns promptly even when a connected session's
// server has stopped responding, so a wedged server cannot stall
// the generation loop at step boundaries. The session teardown
// DELETE is held server-side while cleanup must already have
// returned.
func TestConnectAll_CleanupPromptWhenServerWedges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "wedge", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	// Stateful handler so closing the session sends a DELETE.
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil,
	)

	var deleteArrived atomic.Bool
	releaseDelete := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteArrived.Store(true)
			select {
			case <-releaseDelete:
			case <-r.Context().Done():
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDelete) }) }
	// Registered after ts.Close so it runs before it, letting the
	// wedged DELETE finish and the session unwind before the
	// server waits for outstanding requests.
	t.Cleanup(release)

	cfg := makeConfig("wedge", ts.URL)
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, loopbackHTTPClient())
	require.Equal(t, []string{"wedge__echo"}, toolNames(tools))

	start := time.Now()
	cleanup()
	elapsed := time.Since(start)
	require.Less(t, elapsed, 1*time.Second,
		"cleanup took %s with a wedged server", elapsed)

	// The teardown must still happen in the background: the wedged
	// DELETE arrives even though cleanup already returned.
	require.Eventually(t, deleteArrived.Load,
		10*time.Second, 10*time.Millisecond,
		"session close DELETE never reached the server")
	release()
}

// TestConnectAll_NoToolsWedgedCloseWithinBudget proves that a
// server whose session yields no usable tools cannot stall
// ConnectAll past the connect budget when its teardown DELETE
// wedges. The discarded session must be closed off the caller's
// path, and the teardown must still happen in the background.
func TestConnectAll_NoToolsWedgedCloseWithinBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Stateful handler with no registered tools so closing the
	// discarded session sends a DELETE.
	srv := mcp.NewServer(&mcp.Implementation{Name: "notools", Version: "1.0.0"}, nil)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv }, nil,
	)

	var deleteArrived atomic.Bool
	releaseDelete := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteArrived.Store(true)
			select {
			case <-releaseDelete:
			case <-r.Context().Done():
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDelete) }) }
	t.Cleanup(release)

	cfg := makeConfig("notools", ts.URL)
	start := time.Now()
	tools, summaries, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, loopbackHTTPClient())
	elapsed := time.Since(start)
	t.Cleanup(cleanup)

	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeNoTools, summaries[0].Outcome)
	require.Less(t, elapsed, 5*time.Second,
		"ConnectAll took %s with a wedged no-tools teardown", elapsed)

	require.Eventually(t, deleteArrived.Load,
		10*time.Second, 10*time.Millisecond,
		"discarded session close DELETE never reached the server")
	release()
}

// connectRequest is one request an MCP client sent while connecting.
type connectRequest struct {
	method    string
	rpcMethod string
}

// connectAnswer overrides the MCP server's answer to one request.
type connectAnswer int

const (
	// answerServe lets the server check the bearer token and serve.
	answerServe connectAnswer = iota
	answerUnauthorized
	// answerUnauthorizedNoChallenge is a 401 without WWW-Authenticate,
	// like a server that rejects a forwarded Coder header signature.
	answerUnauthorizedNoChallenge
	// answerUnauthorizedInsufficientScope is a 401 whose Bearer challenge
	// says the token lacks a scope, which RFC 6750 sends with a 403.
	answerUnauthorizedInsufficientScope
	// answerUnauthorizedLowercaseScheme is the recorded 401 with the
	// scheme in lowercase.
	answerUnauthorizedLowercaseScheme
	answerForbidden
	answerInternalError
	// answerStall holds the request until the client gives up.
	answerStall
)

// newAuthCheckingMCPServer serves the echo tool over transport to
// requests that carry "Bearer "+accessToken, and answers any other request
// with the recorded 401. answer, called once per request and never
// concurrently, can override that. The returned counter counts the
// requests not served.
func newAuthCheckingMCPServer(
	t *testing.T,
	transport mcpclient.Transport,
	accessToken string,
	answer func(connectRequest) connectAnswer,
) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "authed", Version: "1.0.0"}, nil)
	tool := echoTool()
	srv.AddTool(tool.tool, tool.handler)
	var handler http.Handler
	switch transport {
	case mcpclient.TransportSSE:
		handler = mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	default:
		handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	}

	var (
		mu       sync.Mutex
		unserved atomic.Int32
	)
	stop := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := connectRequest{method: r.Method}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var msg struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(body, &msg)
			req.rpcMethod = msg.Method
		}
		mu.Lock()
		got := answer(req)
		mu.Unlock()
		if got == answerServe && r.Header.Get("Authorization") != "Bearer "+accessToken {
			got = answerUnauthorized
		}
		if got != answerServe {
			unserved.Add(1)
		}
		switch got {
		case answerUnauthorized:
			_ = chattest.WriteRecordedUnauthorized(w)
		case answerUnauthorizedNoChallenge:
			http.Error(w, "invalid request signature", http.StatusUnauthorized)
		case answerUnauthorizedInsufficientScope:
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="admin"`)
			w.WriteHeader(http.StatusUnauthorized)
		case answerUnauthorizedLowercaseScheme:
			w.Header().Set("WWW-Authenticate", "bearer"+strings.TrimPrefix(chattest.RecordedUnauthorizedChallenge, "Bearer"))
			w.WriteHeader(http.StatusUnauthorized)
		case answerForbidden:
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		case answerInternalError:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		case answerStall:
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(func() {
		close(stop)
		ts.CloseClientConnections()
		ts.Close()
	})
	return ts, &unserved
}

// TestConnectAll_UnauthorizedOutcome pins when a connect counts as an
// OAuth token rejection: the connect failed without timing out, the last
// request other than the session DELETE got a 401 with a Bearer challenge
// that does not ask for more scope, and a stored OAuth2 token was
// attached. chatd refreshes the token and reconnects only for
// that outcome.
func TestConnectAll_UnauthorizedOutcome(t *testing.T) {
	t.Parallel()

	const accessToken = "stored-access-token"
	always := func(a connectAnswer) func() func(connectRequest) connectAnswer {
		return func() func(connectRequest) connectAnswer {
			return func(connectRequest) connectAnswer { return a }
		}
	}
	on := func(match func(connectRequest) bool, a connectAnswer) func() func(connectRequest) connectAnswer {
		return func() func(connectRequest) connectAnswer {
			return func(req connectRequest) connectAnswer {
				if match(req) {
					return a
				}
				return answerServe
			}
		}
	}
	isGET := func(req connectRequest) bool { return req.method == http.MethodGet }
	isToolsList := func(req connectRequest) bool { return req.rpcMethod == "tools/list" }
	firstDiscover := func() func(connectRequest) connectAnswer {
		seen := false
		return func(req connectRequest) connectAnswer {
			if req.rpcMethod == "server/discover" && !seen {
				seen = true
				return answerUnauthorized
			}
			return answerServe
		}
	}
	unauthorizedGETThen := func(a connectAnswer) func() func(connectRequest) connectAnswer {
		return func() func(connectRequest) connectAnswer {
			return func(req connectRequest) connectAnswer {
				switch {
				case isGET(req):
					return answerUnauthorized
				case isToolsList(req):
					return a
				}
				return answerServe
			}
		}
	}

	cases := []struct {
		name string
		// revoked makes the server reject the stored token.
		revoked bool
		// noToken attaches no stored token.
		noToken bool
		answer  func() func(connectRequest) connectAnswer
		// want is the outcome per transport. The SSE transport fails
		// its whole connect when its GET event stream or its first
		// POST is rejected, so those 401s end an SSE connect.
		wantStreamable string
		wantSSE        string
	}{
		{name: "RevokedToken", revoked: true, answer: always(answerServe), wantStreamable: "unauthorized", wantSSE: "unauthorized"},
		{name: "NoTokenAttached", revoked: true, noToken: true, answer: always(answerServe), wantStreamable: "error", wantSSE: "error"},
		// Only a Bearer challenge blames the token (RFC 6750 section 3).
		{name: "UnauthorizedWithoutChallenge", answer: always(answerUnauthorizedNoChallenge), wantStreamable: "error", wantSSE: "error"},
		{name: "UnauthorizedInsufficientScope", answer: always(answerUnauthorizedInsufficientScope), wantStreamable: "error", wantSSE: "error"},
		{name: "UnauthorizedLowercaseScheme", answer: always(answerUnauthorizedLowercaseScheme), wantStreamable: "unauthorized", wantSSE: "unauthorized"},
		{name: "Forbidden", answer: always(answerForbidden), wantStreamable: "error", wantSSE: "error"},
		{name: "UnauthorizedGETOnly", answer: on(isGET, answerUnauthorized), wantStreamable: "connected", wantSSE: "unauthorized"},
		{name: "UnauthorizedFirstDiscoverOnly", answer: firstDiscover, wantStreamable: "connected", wantSSE: "unauthorized"},
		{name: "UnauthorizedGETThenToolsListStalls", answer: unauthorizedGETThen(answerStall), wantStreamable: "timeout", wantSSE: "unauthorized"},
		{name: "UnauthorizedGETThenToolsListFails", answer: unauthorizedGETThen(answerInternalError), wantStreamable: "error", wantSSE: "unauthorized"},
		{name: "UnauthorizedToolsListOnly", answer: on(isToolsList, answerUnauthorized), wantStreamable: "unauthorized", wantSSE: "unauthorized"},
	}
	for _, transport := range []mcpclient.Transport{mcpclient.TransportStreamableHTTP, mcpclient.TransportSSE} {
		for _, tc := range cases {
			t.Run(string(transport)+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				want := tc.wantStreamable
				if transport == mcpclient.TransportSSE {
					want = tc.wantSSE
				}
				serverToken := accessToken
				if tc.revoked {
					serverToken = "another-access-token"
				}
				ts, unserved := newAuthCheckingMCPServer(t, transport, serverToken, tc.answer())
				srv := mcpclient.Server{
					ID:        uuid.New(),
					Slug:      "authed",
					URL:       ts.URL,
					Transport: transport,
					UserAuth:  mcpclient.UserAuthOAuth2,
				}
				var tokens []database.MCPServerUserToken
				if !tc.noToken {
					tokens = append(tokens, database.MCPServerUserToken{
						MCPServerConfigID: srv.ID,
						AccessToken:       accessToken,
						RefreshToken:      "stored-refresh-token",
						TokenType:         "Bearer",
						Expiry:            sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
					})
				}
				ctx := testutil.Context(t, testutil.WaitShort)
				if want == string(mcpclient.ConnectOutcomeTimeout) {
					// A deadline on ctx ends the connect like the 10 s connect
					// timeout does, without the wait.
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
					t.Cleanup(cancel)
				}
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

				_, summaries, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{srv}, tokens, uuid.New(), nil, nil, loopbackHTTPClient())
				t.Cleanup(cleanup)

				require.Len(t, summaries, 1)
				require.Equal(t, want, string(summaries[0].Outcome), "connect error: %s", summaries[0].Error)
				require.Positive(t, unserved.Load(), "the override never answered a request")
			})
		}
	}
}

// sameTokenRefresher returns the rejected token for the next connect, as
// chatd does for a token issued seconds earlier, and records the tokens
// that are rejected again.
type sameTokenRefresher struct {
	mu            sync.Mutex
	rejectedAgain []database.MCPServerUserToken
}

func (*sameTokenRefresher) RefreshRejected(_ context.Context, _ mcpclient.Server, tok database.MCPServerUserToken) (database.MCPServerUserToken, bool) {
	return tok, true
}

func (r *sameTokenRefresher) RejectedAgain(_ context.Context, _ mcpclient.Server, tok database.MCPServerUserToken) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejectedAgain = append(r.rejectedAgain, tok)
}

func (r *sameTokenRefresher) rejectedAgainCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rejectedAgain)
}

// TestConnectAllWithRefresh_RejectedTokenGrace pins the wait before a
// server gets a token it just rejected: ConnectAllWithRefresh sends the
// token again only after 2 s on its clock, so a server whose token store
// catches up within that time connects. A connect that ends during the
// wait neither reconnects nor reports the token as rejected again.
func TestConnectAllWithRefresh_RejectedTokenGrace(t *testing.T) {
	t.Parallel()

	const accessToken = "fresh-access-token"
	for _, tc := range []struct {
		name string
		// cancel ends the connect's context during the grace.
		cancel      bool
		wantOutcome string
	}{
		{name: "ReconnectsAfterGrace", wantOutcome: "connected"},
		{name: "CanceledDuringGrace", cancel: true, wantOutcome: "unauthorized"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The server rejects the token until its token store catches
			// up, which the test makes happen during the grace.
			var caughtUp atomic.Bool
			ts, _ := newAuthCheckingMCPServer(t, mcpclient.TransportStreamableHTTP, accessToken, func(connectRequest) connectAnswer {
				if caughtUp.Load() {
					return answerServe
				}
				return answerUnauthorized
			})
			srv := mcpclient.Server{
				ID:        uuid.New(),
				Slug:      "grace",
				URL:       ts.URL,
				Transport: mcpclient.TransportStreamableHTTP,
				UserAuth:  mcpclient.UserAuthOAuth2,
			}
			clock := quartz.NewMock(t)
			grace := clock.Trap().NewTimer("mcpclient", "rejected_token_grace")
			defer grace.Close()
			refresher := &sameTokenRefresher{}
			ctx := testutil.Context(t, testutil.WaitShort)
			connectCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			type connected struct {
				toolCount int
				summaries []mcpclient.ConnectSummary
				cleanup   func()
			}
			done := make(chan connected, 1)
			go func() {
				tools, summaries, cleanup := mcpclient.ConnectAllWithRefresh(
					connectCtx, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
					mcpclient.ConnectAllOptions{
						Servers: []mcpclient.Server{srv},
						Tokens: []database.MCPServerUserToken{{
							MCPServerConfigID: srv.ID,
							AccessToken:       accessToken,
							TokenType:         "Bearer",
						}},
						UserID:     uuid.New(),
						HTTPClient: loopbackHTTPClient(),
						Refresher:  refresher,
						Clock:      clock,
					},
				)
				done <- connected{toolCount: len(tools), summaries: summaries, cleanup: cleanup}
			}()

			call := grace.MustWait(ctx)
			require.Equal(t, 2*time.Second, call.Duration, "grace")
			if tc.cancel {
				cancel()
			} else {
				caughtUp.Store(true)
			}
			call.MustRelease(ctx)
			if !tc.cancel {
				clock.Advance(call.Duration).MustWait(ctx)
			}
			var got connected
			select {
			case got = <-done:
			case <-ctx.Done():
				t.Fatal("the connect never returned")
			}
			t.Cleanup(got.cleanup)

			require.Len(t, got.summaries, 1)
			require.Equal(t, tc.wantOutcome, string(got.summaries[0].Outcome), "connect error: %s", got.summaries[0].Error)
			if !tc.cancel {
				require.Positive(t, got.toolCount, "tools from the connect after the grace")
			}
			require.Zero(t, refresher.rejectedAgainCount(), "a token the server accepted or never saw again is not rejected again")
		})
	}
}

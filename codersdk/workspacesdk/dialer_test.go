package workspacesdk_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestWebsocketDialer_TokenController(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)

	fTokenProv := newFakeTokenController(ctx, t)
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)

	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: time.Hour,
		DERPMapFn:              func() *tailcfg.DERPMap { return &tailcfg.DERPMap{} },
	})
	require.NoError(t, err)

	dialTokens := make(chan string, 1)
	wsErr := make(chan error, 1)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-ctx.Done():
			t.Error("timed out sending token")
		case dialTokens <- r.URL.Query().Get("resume_token"):
			// OK
		}

		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		wsCtx, nc := codersdk.WebsocketNetConn(ctx, sws, websocket.MessageBinary)
		// streamID can be empty because we don't call RPCs in this test.
		wsErr <- svc.ServeConnV2(wsCtx, nc, tailnet.StreamID{})
	}))
	defer svr.Close()
	svrURL, err := url.Parse(svr.URL)
	require.NoError(t, err)

	uut := workspacesdk.NewWebsocketDialer(logger, svrURL, &websocket.DialOptions{})

	clientCh := make(chan tailnet.ControlProtocolClients, 1)
	go func() {
		clients, err := uut.Dial(ctx, fTokenProv)
		assert.NoError(t, err)
		clientCh <- clients
	}()

	call := testutil.RequireRecvCtx(ctx, t, fTokenProv.tokenCalls)
	call <- tokenResponse{"test token", true}
	gotToken := <-dialTokens
	require.Equal(t, "test token", gotToken)

	clients := testutil.RequireRecvCtx(ctx, t, clientCh)
	clients.Closer.Close()

	err = testutil.RequireRecvCtx(ctx, t, wsErr)
	require.NoError(t, err)

	clientCh = make(chan tailnet.ControlProtocolClients, 1)
	go func() {
		clients, err := uut.Dial(ctx, fTokenProv)
		assert.NoError(t, err)
		clientCh <- clients
	}()

	call = testutil.RequireRecvCtx(ctx, t, fTokenProv.tokenCalls)
	call <- tokenResponse{"test token", false}
	gotToken = <-dialTokens
	require.Equal(t, "", gotToken)

	clients = testutil.RequireRecvCtx(ctx, t, clientCh)
	clients.Closer.Close()

	err = testutil.RequireRecvCtx(ctx, t, wsErr)
	require.NoError(t, err)
}

func TestWebsocketDialer_NoTokenController(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)

	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)

	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: time.Hour,
		DERPMapFn:              func() *tailcfg.DERPMap { return &tailcfg.DERPMap{} },
	})
	require.NoError(t, err)

	dialTokens := make(chan string, 1)
	wsErr := make(chan error, 1)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-ctx.Done():
			t.Error("timed out sending token")
		case dialTokens <- r.URL.Query().Get("resume_token"):
			// OK
		}

		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		wsCtx, nc := codersdk.WebsocketNetConn(ctx, sws, websocket.MessageBinary)
		// streamID can be empty because we don't call RPCs in this test.
		wsErr <- svc.ServeConnV2(wsCtx, nc, tailnet.StreamID{})
	}))
	defer svr.Close()
	svrURL, err := url.Parse(svr.URL)
	require.NoError(t, err)

	uut := workspacesdk.NewWebsocketDialer(logger, svrURL, &websocket.DialOptions{})

	clientCh := make(chan tailnet.ControlProtocolClients, 1)
	go func() {
		clients, err := uut.Dial(ctx, nil)
		assert.NoError(t, err)
		clientCh <- clients
	}()

	gotToken := <-dialTokens
	require.Equal(t, "", gotToken)

	clients := testutil.RequireRecvCtx(ctx, t, clientCh)
	clients.Closer.Close()

	err = testutil.RequireRecvCtx(ctx, t, wsErr)
	require.NoError(t, err)
}

func TestWebsocketDialer_ResumeTokenFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)

	fTokenProv := newFakeTokenController(ctx, t)
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)

	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: time.Hour,
		DERPMapFn:              func() *tailcfg.DERPMap { return &tailcfg.DERPMap{} },
	})
	require.NoError(t, err)

	dialTokens := make(chan string, 1)
	wsErr := make(chan error, 1)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resumeToken := r.URL.Query().Get("resume_token")
		select {
		case <-ctx.Done():
			t.Error("timed out sending token")
		case dialTokens <- resumeToken:
			// OK
		}

		if resumeToken != "" {
			httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
				Message: workspacesdk.CoordinateAPIInvalidResumeToken,
				Validations: []codersdk.ValidationError{
					{Field: "resume_token", Detail: workspacesdk.CoordinateAPIInvalidResumeToken},
				},
			})
			return
		}
		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		wsCtx, nc := codersdk.WebsocketNetConn(ctx, sws, websocket.MessageBinary)
		// streamID can be empty because we don't call RPCs in this test.
		wsErr <- svc.ServeConnV2(wsCtx, nc, tailnet.StreamID{})
	}))
	defer svr.Close()
	svrURL, err := url.Parse(svr.URL)
	require.NoError(t, err)

	uut := workspacesdk.NewWebsocketDialer(logger, svrURL, &websocket.DialOptions{})

	errCh := make(chan error, 1)
	go func() {
		_, err := uut.Dial(ctx, fTokenProv)
		errCh <- err
	}()

	call := testutil.RequireRecvCtx(ctx, t, fTokenProv.tokenCalls)
	call <- tokenResponse{"test token", true}
	gotToken := <-dialTokens
	require.Equal(t, "test token", gotToken)

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.Error(t, err)

	// redial should not use the token
	clientCh := make(chan tailnet.ControlProtocolClients, 1)
	go func() {
		clients, err := uut.Dial(ctx, fTokenProv)
		assert.NoError(t, err)
		clientCh <- clients
	}()
	gotToken = <-dialTokens
	require.Equal(t, "", gotToken)

	clients := testutil.RequireRecvCtx(ctx, t, clientCh)
	require.Error(t, err)
	clients.Closer.Close()
	err = testutil.RequireRecvCtx(ctx, t, wsErr)
	require.NoError(t, err)

	// Successful dial should reset to using token again
	go func() {
		_, err := uut.Dial(ctx, fTokenProv)
		errCh <- err
	}()
	call = testutil.RequireRecvCtx(ctx, t, fTokenProv.tokenCalls)
	call <- tokenResponse{"test token", true}
	gotToken = <-dialTokens
	require.Equal(t, "test token", gotToken)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.Error(t, err)
}

func TestWebsocketDialer_UplevelVersion(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sVer := apiversion.New(proto.CurrentMajor, proto.CurrentMinor-1)

		// the following matches what Coderd does;
		// c.f. coderd/workspaceagents.go: workspaceAgentClientCoordinate
		cVer := r.URL.Query().Get("version")
		if err := sVer.Validate(cVer); err != nil {
			httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
				Message: workspacesdk.AgentAPIMismatchMessage,
				Validations: []codersdk.ValidationError{
					{Field: "version", Detail: err.Error()},
				},
			})
			return
		}
	}))
	svrURL, err := url.Parse(svr.URL)
	require.NoError(t, err)

	uut := workspacesdk.NewWebsocketDialer(logger, svrURL, &websocket.DialOptions{})

	errCh := make(chan error, 1)
	go func() {
		_, err := uut.Dial(ctx, nil)
		errCh <- err
	}()

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, workspacesdk.AgentAPIMismatchMessage, sdkErr.Message)
	require.NotEmpty(t, sdkErr.Helper)
}

type fakeResumeTokenController struct {
	ctx        context.Context
	t          testing.TB
	tokenCalls chan chan tokenResponse
}

func (*fakeResumeTokenController) New(tailnet.ResumeTokenClient) tailnet.CloserWaiter {
	panic("not implemented")
}

func (f *fakeResumeTokenController) Token() (string, bool) {
	call := make(chan tokenResponse)
	select {
	case <-f.ctx.Done():
		f.t.Error("timeout on Token() call")
	case f.tokenCalls <- call:
		// OK
	}
	select {
	case <-f.ctx.Done():
		f.t.Error("timeout on Token() response")
		return "", false
	case r := <-call:
		return r.token, r.ok
	}
}

var _ tailnet.ResumeTokenController = &fakeResumeTokenController{}

func newFakeTokenController(ctx context.Context, t testing.TB) *fakeResumeTokenController {
	return &fakeResumeTokenController{
		ctx:        ctx,
		t:          t,
		tokenCalls: make(chan chan tokenResponse),
	}
}

type tokenResponse struct {
	token string
	ok    bool
}

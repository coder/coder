package aibridge

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibclient "github.com/coder/coder/v2/aibridge/client"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	aibheaders "github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/aibridge/x/proxy/responsesws"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// responsesWebSocketHandshakeTimeout bounds the upstream WebSocket handshake,
// across every key a centralized pool fails over to.
const responsesWebSocketHandshakeTimeout = 30 * time.Second

// maxHandshakeErrorBodyBytes bounds the error body relayed to the client
// when a handshake is refused.
const maxHandshakeErrorBodyBytes = 64 << 10

// responsesWebSocketStrippedHeaders are client headers never sent upstream
// in the WebSocket handshake: the client's own handshake headers, which the
// gateway's handshake replaces, and browser and Coder credentials.
// [aibheaders.BuildUpstreamHeaders] already removes hop-by-hop, provider
// auth, proxy, and actor headers.
var responsesWebSocketStrippedHeaders = []string{
	"Sec-WebSocket-Key",
	"Sec-WebSocket-Version",
	"Sec-WebSocket-Extensions",
	"Sec-WebSocket-Protocol",
	"Sec-WebSocket-Accept",
	"Origin",
	"Cookie",
	codersdk.SessionTokenHeader,
	agplaibridge.HeaderCoderToken,
	agplaibridge.HeaderCoderRequestID,
}

// errUpstreamBinary ends a socket whose upstream sent a binary message,
// which the Responses API never does.
var errUpstreamBinary = xerrors.New("upstream sent a binary message")

// actorHeaderNamer is implemented by providers that forward actor headers
// upstream.
type actorHeaderNamer interface {
	ActorHeaderNames() map[string]string
}

// servesResponsesWebSocket reports whether prov serves Responses API
// WebSocket mode on its Responses route.
func servesResponsesWebSocket(prov provider.Provider) bool {
	ws, ok := prov.(interface{ ServesResponsesWebSocket() bool })
	return ok && ws.ServesResponsesWebSocket()
}

// responsesWebSocketHandler serves OpenAI Responses API WebSocket mode on an
// OpenAI provider's bridged Responses route. It relays every frame unchanged
// through a [responsesws.Session], which records one interception per
// response.create. It never injects MCP tools.
type responsesWebSocketHandler struct {
	provider provider.Provider
	// recorder must record synchronously: the session awaits each record.
	recorder recorder.Recorder
	logger   slog.Logger
	client   *http.Client
	clock    quartz.Clock
}

func newResponsesWebSocketHandler(prov provider.Provider, rec recorder.Recorder, logger slog.Logger) *responsesWebSocketHandler {
	return &responsesWebSocketHandler{
		provider: prov,
		recorder: rec,
		logger:   logger.Named("responses_websocket"),
		client:   &http.Client{Transport: utils.NewStreamingTransport()},
		clock:    quartz.NewReal(),
	}
}

// serve handles one upgrade. Until the client is upgraded every failure is a
// plain HTTP error, so clients can fall back to HTTP.
func (h *responsesWebSocketHandler) serve(w http.ResponseWriter, r *http.Request, acquire aibcontext.SocketAcquirer) {
	ctx := r.Context()
	client := aibclient.GuessClient(r)
	sessionID := aibclient.GuessSessionID(client, r)
	logger := h.logger.With(
		slog.F("provider", h.provider.Name()),
		slog.F("client", string(client)),
		slog.F("client_session_id", sessionID),
		slog.F("user_agent", r.UserAgent()),
	)

	actor := aibcontext.ActorFromContext(ctx)
	if actor == nil {
		logger.Warn(ctx, "no actor found in context")
		http.Error(w, "no actor found", http.StatusBadRequest)
		return
	}
	// Like an HTTP request, a socket opened through Agent Firewall carries
	// its correlation headers, recorded on every interception of the socket.
	// Fail closed on partial or malformed headers.
	agentFirewallSessionID, agentFirewallSeqNumber, err := aibheaders.ExtractAgentFirewallHeaders(r)
	if err != nil {
		logger.Warn(ctx, "rejecting Responses WebSocket with invalid agent firewall headers", slog.Error(err))
		http.Error(w, "invalid agent firewall headers", http.StatusBadRequest)
		return
	}
	// Refuse a handshake the client could never complete before it takes
	// a lease or an upstream connection and key attempt.
	if !validClientHandshake(r) {
		logger.Debug(ctx, "refusing invalid Responses WebSocket handshake")
		if conn, err := websocket.Accept(w, r, responsesWebSocketAcceptOptions); err == nil {
			// validClientHandshake refused a handshake Accept took.
			_ = conn.Close(websocket.StatusInternalError, "invalid handshake")
		}
		return
	}

	lease, err := acquire(ctx, actor.ID, h.provider.Name())
	if err != nil {
		logger.Debug(ctx, "responses websocket refused", slog.Error(err))
		respErr, ok := errors.AsType[*intercept.ResponseError](err)
		if !ok {
			respErr = intercept.NewResponseError("failed to open Responses WebSocket", intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, http.StatusServiceUnavailable, 0)
		}
		//nolint:bodyclose // writeHTTPResponse closes it.
		writeHTTPResponse(w, respErr.ToResponse())
		return
	}
	// Released last, once the session finished recording.
	defer lease.Release()
	// The session and every record it makes run under the lease, which
	// outlives the request and the bridge that served it.
	socketCtx := aibcontext.AsActor(lease.Context(), actor.ID, actor.Email, actor.Metadata)

	cred, err := h.provider.ResolveCredential(r)
	if err != nil {
		logger.Warn(ctx, "failed to resolve upstream credential", slog.Error(err))
		//nolint:bodyclose // writeHTTPResponse closes it.
		writeHTTPResponse(w, intercept.NewResponseError("failed to resolve upstream credential", intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, http.StatusInternalServerError, 0).ToResponse())
		return
	}

	upstream, refusal := h.dialUpstream(socketCtx, logger, r, actor, cred)
	if refusal != nil {
		writeHTTPResponse(w, refusal)
		return
	}

	conn, err := websocket.Accept(w, r, responsesWebSocketAcceptOptions)
	if err != nil {
		// Accept already wrote the HTTP error. Send it before closing
		// upstream, which waits for upstream to answer the close.
		logger.Debug(ctx, "failed to accept Responses WebSocket", slog.Error(err))
		_ = http.NewResponseController(w).Flush()
		_ = upstream.Close(websocket.StatusGoingAway, "client handshake failed")
		return
	}
	// Both handshakes completed under the concurrency limit. The open
	// socket is bounded by the socket caps instead.
	if release := agplaibridge.ConcurrencySlotReleaseFromContext(ctx); release != nil {
		release()
	}
	conn.SetReadLimit(responsesws.MaxClientFrameBytes)
	upstream.SetReadLimit(responsesws.MaxUpstreamFrameBytes)

	logger.Debug(ctx, "responses websocket opened", slog.F("credential_kind", string(cred.Kind())), slog.F("credential_hint", cred.Hint()))
	h.relay(socketCtx, logger, conn, upstream, responsesws.Options{
		Provider:        h.provider,
		Recorder:        h.recorder,
		Admit:           aibcontext.CreateAdmissionFromContext(ctx),
		Logger:          logger,
		Client:          string(client),
		ClientSessionID: sessionID,
		UserAgent:       r.UserAgent(),
		CredentialKind:  cred.Kind(),
		CredentialHint:  cred.Hint(),

		AgentFirewallSessionID:      agentFirewallSessionID,
		AgentFirewallSequenceNumber: agentFirewallSeqNumber,
	})
}

var responsesWebSocketAcceptOptions = &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled}

// validClientHandshake reports whether r passes the checks websocket.Accept
// makes beyond those of [aibheaders.IsWebSocketUpgrade]: protocol version
// 13, one 16 byte Sec-WebSocket-Key, and, from a browser, an Origin on the
// request's host. Accept refuses every request it rejects.
func validClientHandshake(r *http.Request) bool {
	keys := r.Header.Values("Sec-WebSocket-Key")
	if !r.ProtoAtLeast(1, 1) || r.Header.Get("Sec-WebSocket-Version") != "13" || len(keys) != 1 {
		return false
	}
	if key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keys[0])); err != nil || len(key) != 16 {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	return true
}

// dialUpstream opens the upstream WebSocket under ctx. A centralized pool
// fails over across its keys on the handshake statuses HTTP requests fail
// over on; a BYOK credential is tried once. On failure it returns the HTTP
// response the client is refused with.
func (h *responsesWebSocketHandler) dialUpstream(ctx context.Context, logger slog.Logger, r *http.Request, actor *aibcontext.Actor, cred credential.Credential) (*websocket.Conn, *http.Response) {
	ctx, cancel := context.WithTimeout(ctx, responsesWebSocketHandshakeTimeout)
	defer cancel()
	// The handshake runs under the lease, which outlives the request, but a
	// client that left before its upgrade no longer needs the socket.
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()

	target, err := responsesWebSocketURL(h.provider.BaseURL())
	if err != nil {
		logger.Warn(ctx, "invalid provider base URL", slog.Error(err))
		return nil, upstreamHandshakeFailure(nil)
	}
	dial := func(secret string) (*websocket.Conn, *http.Response, error) {
		//nolint:bodyclose // Dial owns the response body.
		return websocket.Dial(ctx, target, &websocket.DialOptions{
			HTTPClient:      h.client,
			HTTPHeader:      h.upstreamHeaders(r, actor, secret),
			CompressionMode: websocket.CompressionDisabled,
		})
	}

	if byok, ok := credential.AsBYOK(cred); ok {
		//nolint:bodyclose // Dial owns the response body.
		conn, resp, err := dial(byok.Secret)
		if err != nil {
			logger.Debug(ctx, "upstream WebSocket handshake failed", slog.Error(err))
			return nil, upstreamHandshakeFailure(resp)
		}
		return conn, nil
	}
	pool, ok := credential.AsCentralizedPool(cred)
	if !ok {
		logger.Warn(ctx, "unsupported credential for Responses WebSocket", slog.F("credential_kind", string(cred.Kind())))
		return nil, upstreamHandshakeFailure(nil)
	}
	walker := pool.Pool.Walker()
	defer func() { pool.Pool.RecordAttempts(walker.Attempts()) }()
	for {
		key, keyErr := pool.NextKey(walker)
		if keyErr != nil {
			logger.Warn(ctx, "key pool exhausted", slog.Error(keyErr))
			return nil, intercept.ResponseErrorFromKeyPool(keyErr).ToResponse()
		}
		//nolint:bodyclose // Dial owns the response body.
		conn, resp, err := dial(key.Value())
		if err == nil {
			return conn, nil
		}
		logger.Debug(credential.WithCredentialInfo(ctx, cred), "upstream WebSocket handshake failed", slog.Error(err))
		if ctx.Err() == nil && pool.Pool.MarkKeyOnStatus(ctx, key, resp, logger) {
			continue
		}
		return nil, upstreamHandshakeFailure(resp)
	}
}

// upstreamHeaders returns the upstream handshake headers: the client's
// forwardable headers, the actor headers, and secret as the only credential.
func (h *responsesWebSocketHandler) upstreamHeaders(r *http.Request, actor *aibcontext.Actor, secret string) http.Header {
	authHeader := h.provider.AuthHeader()
	sdkHeader := http.Header{}
	sdkHeader.Set(authHeader, "Bearer "+secret)
	var actorHeaderNames map[string]string
	if namer, ok := h.provider.(actorHeaderNamer); ok {
		actorHeaderNames = namer.ActorHeaderNames()
	}
	// Strip the client's values before actor headers are applied, so an
	// actor header configured at one of these names still reaches upstream,
	// as it does over HTTP.
	clientHeader := r.Header.Clone()
	for _, name := range responsesWebSocketStrippedHeaders {
		clientHeader.Del(name)
	}
	return aibheaders.BuildUpstreamHeaders(sdkHeader, clientHeader, authHeader, actorHeaderNames, actor)
}

// responsesWebSocketURL returns the upstream Responses WebSocket URL of a
// provider base URL. Like HTTP requests, it keeps the base URL's query.
func responsesWebSocketURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", xerrors.Errorf("parse base URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return "", xerrors.Errorf("unsupported base URL scheme %q", u.Scheme)
	}
	return u.JoinPath(provider.OpenAIResponsesRoute).String(), nil
}

// upstreamHandshakeFailure returns the response a client is refused with
// after the upstream handshake failed with resp: an upstream error response
// is relayed, anything else, including a network error, is a 502.
func upstreamHandshakeFailure(resp *http.Response) *http.Response {
	if resp != nil && resp.StatusCode >= http.StatusBadRequest {
		return resp
	}
	return intercept.NewResponseError("upstream WebSocket handshake failed", intercept.OpenAIErrTypeAPI, intercept.OpenAIErrCodeServer, http.StatusBadGateway, 0).ToResponse()
}

// writeHTTPResponse writes resp's status, content type, retry hint, and a
// bounded body to w. Like the HTTP path, a retry hint upstream gave only in
// OpenAI's retry-after-ms header becomes a Retry-After in whole seconds.
func writeHTTPResponse(w http.ResponseWriter, resp *http.Response) {
	for _, name := range []string{"Content-Type", "Retry-After"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	if w.Header().Get("Retry-After") == "" {
		if retryAfter := keypool.ParseRetryAfter(resp); retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
		}
	}
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		_, _ = io.Copy(w, io.LimitReader(resp.Body, maxHandshakeErrorBodyBytes))
		_ = resp.Body.Close()
	}
}

// relayEnd is how a socket ends: the close status sent to each side and the
// cause open interceptions end with. A zero code means that side already
// closed.
type relayEnd struct {
	clientCode     websocket.StatusCode
	clientReason   string
	upstreamCode   websocket.StatusCode
	upstreamReason string
	cause          error
}

// relay pumps text messages between the client and upstream through a
// session until either side ends or ctx, the lease context, ends. It closes
// both connections and the session before it returns.
func (h *responsesWebSocketHandler) relay(ctx context.Context, logger slog.Logger, client, upstreamConn *websocket.Conn, opts responsesws.Options) {
	upstream := newUpstreamMessageConn(ctx, upstreamConn)
	sess, err := responsesws.NewSession(ctx, upstream, opts)
	if err != nil {
		logger.Error(ctx, "failed to start Responses WebSocket session", slog.Error(err))
		upstream.setClose(websocket.StatusGoingAway, "")
		_ = upstream.Close()
		_ = client.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("failed to start session"))
		return
	}

	// relayCtx stops the pumps. It is canceled only after both connections
	// were sent their close frames: coder/websocket closes a connection
	// without one when the context of a pending Read or Write ends.
	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// connCtx never ends: reads and writes on the client end when it closes.
	connCtx := context.WithoutCancel(ctx)
	_ = httpapi.NewWSWatcher(h.clock, nil).Watch(relayCtx, logger, client)

	ends := make(chan *relayEnd, 2)
	go func() { ends <- pumpClient(relayCtx, connCtx, client, sess) }()
	go func() { ends <- pumpUpstream(relayCtx, connCtx, client, sess) }()

	pending := 2
	var end *relayEnd
	for end == nil && pending > 0 {
		select {
		case e := <-ends:
			pending--
			end = e
		case <-ctx.Done():
			end = goingAwayEnd(ctx)
		}
	}
	switch {
	case end != nil:
	case ctx.Err() != nil:
		// The pumps saw the lease end before this loop did.
		end = goingAwayEnd(ctx)
	default:
		end = internalErrorEnd(xerrors.New("relay ended without a cause"))
	}

	// Tell both sides why the socket ends, then end the session, which
	// records every open interception.
	upstream.setClose(end.upstreamCode, end.upstreamReason)
	var clientClosed sync.WaitGroup
	if end.clientCode != 0 {
		clientClosed.Go(func() { _ = client.Close(end.clientCode, end.clientReason) })
	}
	if err := sess.Close(end.cause); err != nil {
		logger.Debug(ctx, "upstream close failed", slog.Error(err))
	}
	clientClosed.Wait()
	cancel()
	_ = client.CloseNow()
	for ; pending > 0; pending-- {
		<-ends
	}
	logger.Debug(ctx, "responses websocket closed",
		slog.F("client_close_code", end.clientCode.String()),
		slog.F("upstream_close_code", end.upstreamCode.String()),
		slog.Error(end.cause),
	)
}

// pumpClient forwards client messages through the session until the client
// or the session ends. It returns nil when the session ended first, since
// pumpUpstream reports why.
func pumpClient(relayCtx, connCtx context.Context, client *websocket.Conn, sess *responsesws.Session) *relayEnd {
	for {
		typ, msg, err := client.Read(connCtx)
		if err != nil {
			if relayCtx.Err() != nil {
				return nil
			}
			return clientEnd(err)
		}
		if typ != websocket.MessageText {
			return &relayEnd{
				clientCode:     websocket.StatusUnsupportedData,
				clientReason:   "binary messages are not supported",
				upstreamCode:   websocket.StatusGoingAway,
				upstreamReason: "client connection closed",
				cause:          xerrors.New("client sent a binary message"),
			}
		}
		if err := sess.Send(relayCtx, msg); err != nil {
			if relayCtx.Err() != nil || errors.Is(err, responsesws.ErrClosed) {
				return nil
			}
			return internalErrorEnd(err)
		}
	}
}

// pumpUpstream forwards session frames to the client until the session or
// the client ends.
func pumpUpstream(relayCtx, connCtx context.Context, client *websocket.Conn, sess *responsesws.Session) *relayEnd {
	for {
		frame, err := sess.Recv(relayCtx)
		if err != nil {
			if relayCtx.Err() != nil {
				return nil
			}
			return upstreamEnd(err)
		}
		if err := client.Write(connCtx, websocket.MessageText, frame); err != nil {
			if relayCtx.Err() != nil {
				return nil
			}
			return clientEnd(err)
		}
	}
}

// clientEnd ends a socket whose client connection ended with err. A client
// close status is passed on to upstream; coder/websocket already answered
// it, and already closed the client with StatusMessageTooBig for an
// oversized message.
func clientEnd(err error) *relayEnd {
	end := &relayEnd{upstreamCode: websocket.StatusGoingAway, upstreamReason: "client connection closed", cause: err}
	if closeErr, ok := errors.AsType[websocket.CloseError](err); ok {
		end.upstreamCode = peerCloseCode(closeErr.Code)
		end.upstreamReason = closeErr.Reason
	}
	return end
}

// upstreamEnd ends a socket whose session ended with err after relaying
// every frame. An upstream close status is passed on to the client.
func upstreamEnd(err error) *relayEnd {
	end := &relayEnd{cause: err}
	if closeErr, ok := errors.AsType[websocket.CloseError](err); ok {
		end.clientCode = peerCloseCode(closeErr.Code)
		end.clientReason = closeErr.Reason
		return end
	}
	switch {
	case errors.Is(err, websocket.ErrMessageTooBig):
		end.clientCode = websocket.StatusMessageTooBig
		end.clientReason = "upstream message too big"
	case errors.Is(err, errUpstreamBinary):
		end.clientCode = websocket.StatusInternalError
		end.clientReason = errUpstreamBinary.Error()
		end.upstreamCode = websocket.StatusUnsupportedData
	default:
		end.clientCode = websocket.StatusInternalError
		end.clientReason = "upstream connection lost"
	}
	return end
}

// goingAwayEnd ends a socket whose lease context ended, at its maximum
// lifetime or on shutdown.
func goingAwayEnd(ctx context.Context) *relayEnd {
	cause := context.Cause(ctx)
	reason := httpapi.WebsocketCloseSprintf("%s", cause.Error())
	return &relayEnd{
		clientCode:     websocket.StatusGoingAway,
		clientReason:   reason,
		upstreamCode:   websocket.StatusGoingAway,
		upstreamReason: reason,
		cause:          cause,
	}
}

func internalErrorEnd(err error) *relayEnd {
	return &relayEnd{
		clientCode:     websocket.StatusInternalError,
		clientReason:   httpapi.WebsocketCloseSprintf("internal error"),
		upstreamCode:   websocket.StatusGoingAway,
		upstreamReason: "client connection closed",
		cause:          err,
	}
}

// peerCloseCode returns the status to pass on for a close status received
// from the other side: a status that may not be sent in a close frame
// becomes a normal closure or an internal error.
func peerCloseCode(code websocket.StatusCode) websocket.StatusCode {
	switch {
	case code == websocket.StatusNoStatusRcvd:
		return websocket.StatusNormalClosure
	case code >= 1000 && code <= 1003, code >= 1007 && code <= 1014, code >= 3000 && code <= 4999:
		return code
	default:
		return websocket.StatusInternalError
	}
}

// upstreamMessageConn adapts the upstream connection to
// responsesws.MessageConn.
//
// coder/websocket closes a connection without a close frame when the
// context of a pending Read or Write ends, and the session ends its context
// before it closes upstream. So the adapter never hands coder/websocket a
// cancelable context: when a Write context ends, it closes upstream with its
// close status instead. Reads run on a reader goroutine, so a Read whose
// context ends returns at once: the session records the end of its open
// interceptions once Read returns, and the close handshake takes up to 5
// seconds against an upstream that does not answer, longer than a socket's
// share of a shutdown.
type upstreamMessageConn struct {
	conn *websocket.Conn
	// lease decides the default close status: going away once it ended,
	// else a normal closure.
	lease context.Context
	// reads carries each message or error the reader goroutine read.
	reads chan upstreamRead
	// closing is closed once Close was called. It stops the reader
	// goroutine and pending Reads.
	closing chan struct{}

	mu     sync.Mutex
	code   websocket.StatusCode
	reason string

	closeOnce sync.Once
	closeErr  error
}

type upstreamRead struct {
	typ websocket.MessageType
	msg []byte
	err error
}

func newUpstreamMessageConn(lease context.Context, conn *websocket.Conn) *upstreamMessageConn {
	u := &upstreamMessageConn{conn: conn, lease: lease, reads: make(chan upstreamRead), closing: make(chan struct{})}
	go u.readUpstream()
	return u
}

// readUpstream reads upstream for Read until a read fails or Close was
// called. It reads at most one message ahead of Read.
func (u *upstreamMessageConn) readUpstream() {
	for {
		typ, msg, err := u.conn.Read(context.Background())
		select {
		case u.reads <- upstreamRead{typ: typ, msg: msg, err: err}:
		case <-u.closing:
			return
		}
		if err != nil {
			return
		}
	}
}

func (u *upstreamMessageConn) Read(ctx context.Context) ([]byte, error) {
	var r upstreamRead
	select {
	case r = <-u.reads:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-u.closing:
		return nil, net.ErrClosed
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.typ != websocket.MessageText {
		u.setClose(websocket.StatusUnsupportedData, "binary messages are not supported")
		return nil, errUpstreamBinary
	}
	return r.msg, nil
}

func (u *upstreamMessageConn) Write(ctx context.Context, msg []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = u.Close() })
	defer stop()
	return u.conn.Write(context.WithoutCancel(ctx), websocket.MessageText, msg)
}

// setClose sets the status Close sends, unless one was set.
func (u *upstreamMessageConn) setClose(code websocket.StatusCode, reason string) {
	if code == 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.code == 0 {
		u.code, u.reason = code, httpapi.WebsocketCloseSprintf("%s", reason)
	}
}

// Close closes upstream with the status set by setClose. It is idempotent
// and safe for concurrent use.
func (u *upstreamMessageConn) Close() error {
	u.closeOnce.Do(func() {
		close(u.closing)
		u.mu.Lock()
		if u.code == 0 {
			if u.lease.Err() != nil {
				u.code, u.reason = websocket.StatusGoingAway, httpapi.WebsocketCloseSprintf("%s", context.Cause(u.lease).Error())
			} else {
				u.code = websocket.StatusNormalClosure
			}
		}
		code, reason := u.code, u.reason
		u.mu.Unlock()
		u.closeErr = u.conn.Close(code, reason)
	})
	return u.closeErr
}

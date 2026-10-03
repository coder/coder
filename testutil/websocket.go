package testutil

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"

	"cdr.dev/slog/v3"
)

// InMemWebsocketRoundTripper allows you to "dial" an HTTP handler that sets up a websocket using only in-memory
// primitives. No TCP or OS networking needed. CtxMutator gives you explicit control over the context the handler sees.
//
// Example:
//
//		rt := testutil.InMemWebsocketRoundTripper{
//			Handler: MyHandler,
//			CtxMutator: func(ctx context.Context) context.Context {
//				ctx = httpmw.WithWorkspaceParam(ctx, ws)
//				ctx = dbauthz.As(ctx, mySubject(userID, orgID))
//				return ctx
//			},
//	        Logger: logger.Named("roundtripper"),
//		}
//		clientSock, _, err := websocket.Dial(ctx, "wss://local.test/", &websocket.DialOptions{
//			HTTPClient: &http.Client{Transport: rt},
//		})
type InMemWebsocketRoundTripper struct {
	Logger     slog.Logger
	Handler    http.Handler
	CtxMutator func(ctx context.Context) context.Context
}

func (i InMemWebsocketRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	i.Logger.Debug(context.Background(), "round trip start")
	defer i.Logger.Debug(context.Background(), "round trip end")
	newCtx := i.CtxMutator(request.Context())
	request = request.WithContext(newCtx)
	serverP, clientP := net.Pipe()
	var _ io.ReadWriteCloser = clientP // compile time check that response body is OK for websocket
	response := &http.Response{
		Header: make(http.Header),
		Body:   clientP,
	}
	rw := newInMemWebsocketResponseWriter(response, serverP)
	go func() {
		i.Handler.ServeHTTP(rw, request)
		if !rw.hijacked {
			i.Logger.Debug(context.Background(), "closing connection after handler did not hijack")
			// If the handler didn't hijack the connection, we should close it when the handler finishes.
			// This prevents a 3s delay in websocket.Dial() reading the non-upgraded response.
			_ = serverP.Close()
		}
	}()
	select {
	case <-newCtx.Done():
		return nil, newCtx.Err()
	case <-rw.gotHeaders:
		return response, nil
	}
}

func newInMemWebsocketResponseWriter(resp *http.Response, conn net.Conn) *inMemWebsocketResponseWriter {
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	return &inMemWebsocketResponseWriter{
		r:          resp,
		b:          bufio.NewReadWriter(r, w),
		gotHeaders: make(chan struct{}),
		conn:       conn,
	}
}

type inMemWebsocketResponseWriter struct {
	r          *http.Response
	b          *bufio.ReadWriter
	gotHeaders chan struct{}
	hijacked   bool
	conn       net.Conn
}

func (rw *inMemWebsocketResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw.hijacked = true
	return rw.conn, rw.b, nil
}

func (rw *inMemWebsocketResponseWriter) Header() http.Header {
	return rw.r.Header
}

func (rw *inMemWebsocketResponseWriter) Write([]byte) (int, error) {
	n, err := rw.b.Write([]byte{})
	return n, err
}

func (rw *inMemWebsocketResponseWriter) WriteHeader(statusCode int) {
	rw.r.StatusCode = statusCode
	close(rw.gotHeaders)
}

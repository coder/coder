package mcpclient

import (
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"golang.org/x/xerrors"
)

// InProcessRoute serves MCP requests for one origin and path prefix with
// an in-process handler instead of dialing out. Coderd uses it for its
// built-in MCP servers, whose URLs point at the deployment's own access
// URL: dialing that URL fails when it resolves to a loopback or private
// address that the SSRF guard blocks, and serving in-process needs no
// network at all.
type InProcessRoute struct {
	// Origin is the scheme and host requests must target. Its path,
	// query, and fragment are ignored.
	Origin *url.URL
	// PathPrefix is the path prefix requests must fall under. It must
	// start and end with a slash.
	PathPrefix string
	// Handler serves matching requests. It sees the request URL as the
	// client sent it.
	Handler http.Handler
}

// WithInProcessRoute returns a copy of base whose matching requests are
// served by route.Handler. Every other request still goes through base's
// transport and its SSRF guard. A request only matches when its scheme,
// host, and port equal the route origin and its path is already clean and
// below PathPrefix, so no spelling of another destination can skip the
// guard.
func WithInProcessRoute(base *http.Client, route InProcessRoute) *http.Client {
	if base == nil || base.Transport == nil {
		base = NewHTTPClient(base)
	}
	if route.Origin == nil || route.Handler == nil || route.Origin.Host == "" ||
		!strings.HasPrefix(route.PathPrefix, "/") || !strings.HasSuffix(route.PathPrefix, "/") {
		return base
	}
	client := *base
	client.Transport = &inProcessTransport{
		base:    base.Transport,
		scheme:  strings.ToLower(route.Origin.Scheme),
		host:    strings.ToLower(route.Origin.Hostname()),
		port:    normalizedPort(route.Origin),
		prefix:  route.PathPrefix,
		handler: route.Handler,
	}
	return &client
}

type inProcessTransport struct {
	base    http.RoundTripper
	scheme  string
	host    string
	port    string
	prefix  string
	handler http.Handler
}

func normalizedPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

func (t *inProcessTransport) matches(u *url.URL) bool {
	if strings.ToLower(u.Scheme) != t.scheme ||
		strings.ToLower(u.Hostname()) != t.host ||
		normalizedPort(u) != t.port ||
		u.User != nil || u.RawPath != "" {
		return false
	}
	return path.Clean(u.Path) == u.Path && strings.HasPrefix(u.Path, t.prefix)
}

func (t *inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.matches(req.URL) {
		return t.base.RoundTrip(req)
	}
	return t.serve(req)
}

// serve runs the handler in a goroutine and returns once it sends
// headers, streaming the body through a pipe so server-sent events work.
func (t *inProcessTransport) serve(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	serverReq := req.Clone(ctx)
	serverReq.RequestURI = req.URL.RequestURI()
	serverReq.Host = req.URL.Host
	serverReq.RemoteAddr = "127.0.0.1:0"
	if serverReq.Body == nil {
		serverReq.Body = http.NoBody
	}

	pr, pw := io.Pipe()
	w := &pipeResponseWriter{
		header: http.Header{},
		body:   pw,
		ready:  make(chan struct{}),
	}
	go func() {
		defer func() {
			if p := recover(); p != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = pw.CloseWithError(xerrors.Errorf("in-process MCP handler panicked: %v", p))
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = pw.Close()
		}()
		t.handler.ServeHTTP(w, serverReq)
	}()

	select {
	case <-w.ready:
	case <-ctx.Done():
		_ = pr.CloseWithError(ctx.Err())
		return nil, ctx.Err()
	}
	return &http.Response{
		Status:        http.StatusText(w.status),
		StatusCode:    w.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        w.sentHeader,
		Body:          pr,
		ContentLength: -1,
		Request:       req,
	}, nil
}

// pipeResponseWriter is an http.ResponseWriter whose body streams into a
// pipe read by the client.
type pipeResponseWriter struct {
	header     http.Header
	body       *io.PipeWriter
	once       sync.Once
	ready      chan struct{}
	status     int
	sentHeader http.Header
}

func (w *pipeResponseWriter) Header() http.Header { return w.header }

func (w *pipeResponseWriter) WriteHeader(status int) {
	w.once.Do(func() {
		w.status = status
		w.sentHeader = w.header.Clone()
		close(w.ready)
	})
}

func (w *pipeResponseWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(p)
}

// Flush implements http.Flusher. Writes already reach the reader
// unbuffered.
func (*pipeResponseWriter) Flush() {}

package aibridged

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"cdr.dev/slog"
)

// fallthroughRouter is a simple reverse-proxy implementation to enable
type fallthroughRouter struct {
	provider Provider
	logger   slog.Logger
}

func newFallthroughRouter(provider Provider, logger slog.Logger) *fallthroughRouter {
	return &fallthroughRouter{
		provider: provider,
		logger:   logger,
	}
}

func (f *fallthroughRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upURL, err := url.Parse(f.provider.BaseURL())
	if err != nil {
		f.logger.Error(r.Context(), "failed to parse provider base URL", slog.Error(err))
		http.Error(w, "request error", http.StatusBadGateway)
		return
	}

	// Build a reverse proxy to the upstream.
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Set scheme/host to upstream.
			req.URL.Scheme = upURL.Scheme
			req.URL.Host = upURL.Host

			// Preserve the stripped path from the incoming request and ensure leading slash.
			p := r.URL.Path
			if len(p) == 0 || p[0] != '/' {
				p = "/" + p
			}
			req.URL.Path = p
			req.URL.RawPath = ""

			// Preserve query string.
			req.URL.RawQuery = r.URL.RawQuery

			// Set Host header for upstream.
			req.Host = upURL.Host

			// Copy headers from client.
			req.Header = r.Header.Clone()

			// Standard proxy headers.
			host, _, herr := net.SplitHostPort(r.RemoteAddr)
			if herr != nil {
				host = r.RemoteAddr
			}
			if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
				req.Header.Set("X-Forwarded-For", prior+", "+host)
			} else {
				req.Header.Set("X-Forwarded-For", host)
			}
			req.Header.Set("X-Forwarded-Host", r.Host)
			if r.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
			// Avoid default Go user-agent if none provided.
			if _, ok := req.Header["User-Agent"]; !ok {
				req.Header.Set("User-Agent", "aibridged") // TODO: use build tag.
			}

			// Inject provider auth.
			switch f.provider.(type) {
			case *OpenAIProvider:
				req.Header.Set("Authorization", "Bearer "+f.provider.Key())
			case *AnthropicMessagesProvider:
				req.Header.Set("x-api-key", f.provider.Key())
			}
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, e error) {
			f.logger.Error(req.Context(), "reverse proxy error", slog.Error(e), slog.F("path", req.URL.Path))
			http.Error(rw, "upstream proxy error", http.StatusBadGateway)
		},
	}

	// Transport tuned for streaming (no response header timeout).
	proxy.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy.ServeHTTP(w, r)
}

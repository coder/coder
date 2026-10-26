package agentnative

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/testutil"
)

func TestOpenCodeRPCBoundsAndSanitizes(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"oversize", "malformed", "content-type", "rejected", "redirect"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			var forwarded atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forwarded.Add(1); w.WriteHeader(200) }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, password, _ := r.BasicAuth()
				if password != "private-secret" {
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch kind {
				case "oversize":
					_, _ = fmt.Fprintf(w, `{"text":"%s"}`, strings.Repeat("x", openCodeMaxResponse))
				case "malformed":
					_, _ = fmt.Fprint(w, "private-secret")
				case "content-type":
					w.Header().Set("Content-Type", "text/plain")
					_, _ = fmt.Fprint(w, `{"text":"private-secret"}`)
				case "rejected":
					w.WriteHeader(409)
					_, _ = fmt.Fprint(w, "private-secret")
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				}
			}))
			defer server.Close()
			client := &openCodeRPC{endpoint: server.URL, password: "private-secret", clock: quartz.NewMock(t), http: openCodeHTTPClient(), ctx: t.Context()}
			var result any
			err := client.request(t.Context(), "POST", "/api/test", map[string]string{"text": "private-secret"}, &result)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-secret")
			require.Zero(t, forwarded.Load(), "a redirect must never forward authentication")
		})
	}
}

func TestOpenCodeRPCRequestTimeout(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	client := &openCodeRPC{endpoint: server.URL, password: "secret", clock: clock, http: openCodeHTTPClient(), ctx: ctx}
	result := make(chan error, 1)
	go func() { var value any; result <- client.request(ctx, "GET", "/api/test", nil, &value) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request not sent")
	}
	clock.Advance(10 * time.Second).MustWait(ctx)
	select {
	case err := <-result:
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	case <-ctx.Done():
		t.Fatal("request not canceled")
	}
}

func TestOpenCodeRPCSSEIdleTimeout(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"version": openCodeVersion})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"ready\",\"type\":\"server.connected\",\"data\":{}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	events := make(chan openCodeNativeEvent, 1)
	client, err := newOpenCodeRPC(ctx, ctx, openCodeDescriptor{URL: server.URL, Password: strings.Repeat("a", 64)}, clock, openCodeHTTPClient(), func(event openCodeNativeEvent) { events <- event })
	require.NoError(t, err)
	defer client.close()
	clock.Advance(45 * time.Second).MustWait(ctx)
	select {
	case event := <-events:
		require.Equal(t, "transport.lost", event.Type)
	case <-ctx.Done():
		t.Fatal("SSE idle deadline did not disconnect")
	}
}

func TestOpenCodeRPCSSEFrameBound(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"version": openCodeVersion})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"ready\",\"type\":\"server.connected\",\"data\":{}}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", openCodeMaxEvent+1))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	events := make(chan openCodeNativeEvent, 1)
	client, err := newOpenCodeRPC(ctx, ctx, openCodeDescriptor{URL: server.URL, Password: strings.Repeat("a", 64)}, quartz.NewMock(t), openCodeHTTPClient(), func(event openCodeNativeEvent) { events <- event })
	require.NoError(t, err)
	defer client.close()
	close(release)
	select {
	case event := <-events:
		require.Equal(t, "transport.lost", event.Type)
	case <-ctx.Done():
		t.Fatal("oversized SSE frame did not disconnect")
	}
}

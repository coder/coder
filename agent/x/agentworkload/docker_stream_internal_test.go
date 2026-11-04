//go:build linux

package agentworkload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestDockerAttachInputAndResize(t *testing.T) {
	t.Parallel()
	queries := make(chan string, 2)
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries <- r.URL.RawQuery
		if strings.HasSuffix(r.URL.Path, "/resize") {
			w.WriteHeader(http.StatusOK)
			return
		}
		defer close(finished)
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = rw.Flush()
		data, err := io.ReadAll(conn)
		if err == nil {
			_, _ = conn.Write(data)
		}
	}))
	t.Cleanup(server.Close)
	host := "tcp://" + strings.TrimPrefix(server.URL, "http://")
	cli, err := client.New(client.WithHost(host), client.WithAPIVersion("1.54"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	d := &dockerRuntime{client: cli, config: Config{DockerHost: host}}
	ctx := testutil.Context(t, testutil.WaitLong)
	stream, err := d.AttachInput(ctx, "container")
	require.NoError(t, err)
	defer stream.Close()
	require.Contains(t, testutil.RequireReceive(ctx, t, queries), "stdin=1")
	input := stream.(inputAttachment)
	_, err = input.WriteContext(ctx, []byte("input"))
	require.NoError(t, err)
	require.NoError(t, input.CloseWrite())
	data, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.Equal(t, "input", string(data))
	receive(t, finished)
	require.NoError(t, d.Resize(ctx, "container", 132, 43))
	require.Equal(t, "h=43&w=132", testutil.RequireReceive(ctx, t, queries))
}

func TestDockerAttachContext(t *testing.T) {
	t.Parallel()
	for _, handshake := range []bool{false, true} {
		t.Run(map[bool]string{false: "CancelHandshake", true: "StreamOutlivesSetup"}[handshake], func(t *testing.T) {
			t.Parallel()
			accepted := make(chan struct{})
			send := make(chan struct{})
			finished := make(chan struct{})
			expected := frame(1, []byte("after setup cancellation"))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				defer close(finished)
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer conn.Close()
				close(accepted)
				if handshake {
					_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
					_ = rw.Flush()
					<-send
					_, _ = conn.Write(expected)
				} else {
					_, _ = io.Copy(io.Discard, conn)
				}
			}))
			t.Cleanup(server.Close)
			host := "tcp://" + strings.TrimPrefix(server.URL, "http://")
			cli, err := client.New(client.WithHost(host), client.WithAPIVersion("1.54"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, cli.Close()) })
			rt := &dockerRuntime{client: cli, config: Config{DockerHost: host}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if handshake {
				stream, err := rt.Attach(ctx, "container")
				require.NoError(t, err)
				defer stream.Close()
				cancel()
				close(send)
				data, err := io.ReadAll(stream)
				require.NoError(t, err)
				require.Equal(t, expected, data)
			} else {
				done := make(chan struct{})
				var attachErr error
				go func() { defer close(done); _, attachErr = rt.Attach(ctx, "container") }()
				receive(t, accepted)
				cancel()
				receive(t, done)
				require.Error(t, attachErr)
			}
			receive(t, finished)
		})
	}
}

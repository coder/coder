package exitnode_test

import (
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/testutil"
)

func TestSniffHost_TLS(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	// Drive a real ClientHello with the standard library. The handshake will
	// never complete because nobody answers, so it runs in the background and
	// is abandoned when the pipe closes.
	tlsClient := tls.Client(clientSide, &tls.Config{
		ServerName: "Secure.Example.COM",
		MinVersion: tls.VersionTLS12,
		//nolint:gosec // The handshake is never completed.
		InsecureSkipVerify: true,
	})
	handshakeErr := make(chan error, 1)
	go func() { handshakeErr <- tlsClient.Handshake() }()

	host, replay, err := exitnode.SniffHost(serverSide, testutil.WaitShort)
	require.NoError(t, err)
	require.Equal(t, "secure.example.com", host)

	// The peeked record must be replayed intact: a TLS handshake record
	// header is the first thing the destination would read.
	header := make([]byte, 5)
	_, err = io.ReadFull(replay, header)
	require.NoError(t, err)
	require.Equal(t, byte(0x16), header[0])

	_ = clientSide.Close()
	ctx := testutil.Context(t, testutil.WaitShort)
	_ = testutil.RequireReceive(ctx, t, handshakeErr)
}

func TestSniffHost_HTTP(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	request := "GET /path HTTP/1.1\r\nHost: Web.Example.com:8080\r\nUser-Agent: test\r\n\r\nbody"
	go func() { _, _ = io.WriteString(clientSide, request) }()

	host, replay, err := exitnode.SniffHost(serverSide, testutil.WaitShort)
	require.NoError(t, err)
	require.Equal(t, "web.example.com", host)

	got := make([]byte, len(request))
	_, err = io.ReadFull(replay, got)
	require.NoError(t, err)
	require.Equal(t, request, string(got))
}

func TestSniffHost_HTTPSplitWrites(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	// The Host header arrives in a second segment; the sniffer must keep
	// reading until the headers are complete.
	go func() {
		_, _ = io.WriteString(clientSide, "POST / HTTP/1.1\r\n")
		_, _ = io.WriteString(clientSide, "Host: split.example\r\nContent-Length: 0\r\n\r\n")
	}()

	host, _, err := exitnode.SniffHost(serverSide, testutil.WaitShort)
	require.NoError(t, err)
	require.Equal(t, "split.example", host)
}

func TestSniffHost_UnknownProtocolDoesNotWait(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	banner := "SSH-2.0-OpenSSH_9.6\r\n"
	go func() { _, _ = io.WriteString(clientSide, banner) }()

	// A generous timeout proves the sniffer returns as soon as it sees a
	// protocol it does not understand rather than waiting for the deadline.
	start := time.Now()
	host, replay, err := exitnode.SniffHost(serverSide, testutil.WaitLong)
	require.NoError(t, err)
	require.Empty(t, host)
	require.Less(t, time.Since(start), testutil.WaitLong/2)

	got := make([]byte, len(banner))
	_, err = io.ReadFull(replay, got)
	require.NoError(t, err)
	require.Equal(t, banner, string(got))
}

func TestSniffHost_SilentClient(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	// Server-speaks-first protocols send nothing; the deadline must return
	// control with an empty host and a usable connection.
	host, replay, err := exitnode.SniffHost(serverSide, testutil.IntervalMedium)
	require.NoError(t, err)
	require.Empty(t, host)

	// The deadline was cleared, so a later read on the returned conn works.
	go func() { _, _ = io.WriteString(clientSide, "late") }()
	got := make([]byte, 4)
	_, err = io.ReadFull(replay, got)
	require.NoError(t, err)
	require.Equal(t, "late", string(got))
}

func TestSniffHost_ClientHangsUp(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()

	go func() {
		_, _ = io.WriteString(clientSide, "GET / HTTP/1.1\r\n")
		_ = clientSide.Close()
	}()

	host, replay, err := exitnode.SniffHost(serverSide, testutil.WaitShort)
	require.ErrorIs(t, err, io.EOF)
	require.Empty(t, host)

	// Whatever was read is still replayed so the caller can decide.
	got, readErr := io.ReadAll(replay)
	require.NoError(t, readErr)
	require.Equal(t, "GET / HTTP/1.1\r\n", string(got))
}

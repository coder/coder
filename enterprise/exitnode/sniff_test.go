package exitnode_test

import (
	"crypto/tls"
	"io"
	"net"
	"strings"
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

// TestSniffHost_Stream covers clients that speak first: HTTP in one or more
// segments, a protocol the sniffer does not understand, and a client that
// hangs up mid-request. In every case whatever was read is replayed.
func TestSniffHost_Stream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		writes   []string
		hangUp   bool
		timeout  time.Duration
		wantHost string
		wantErr  error
		// wantFast proves the sniffer returns as soon as it sees a protocol
		// it does not understand rather than waiting for the deadline.
		wantFast bool
	}{
		{
			name:     "HTTP",
			writes:   []string{"GET /path HTTP/1.1\r\nHost: Web.Example.com:8080\r\nUser-Agent: test\r\n\r\nbody"},
			timeout:  testutil.WaitShort,
			wantHost: "web.example.com",
		},
		{
			// The Host header arrives in a second segment; the sniffer must
			// keep reading until the headers are complete.
			name:     "HTTPSplitWrites",
			writes:   []string{"POST / HTTP/1.1\r\n", "Host: split.example\r\nContent-Length: 0\r\n\r\n"},
			timeout:  testutil.WaitShort,
			wantHost: "split.example",
		},
		{
			name:     "UnknownProtocolDoesNotWait",
			writes:   []string{"SSH-2.0-OpenSSH_9.6\r\n"},
			timeout:  testutil.WaitLong,
			wantFast: true,
		},
		{
			name:    "ClientHangsUp",
			writes:  []string{"GET / HTTP/1.1\r\n"},
			hangUp:  true,
			timeout: testutil.WaitShort,
			wantErr: io.EOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clientSide, serverSide := net.Pipe()
			defer clientSide.Close()
			defer serverSide.Close()

			go func() {
				for _, w := range tt.writes {
					_, _ = io.WriteString(clientSide, w)
				}
				if tt.hangUp {
					_ = clientSide.Close()
				}
			}()

			start := time.Now()
			host, replay, err := exitnode.SniffHost(serverSide, tt.timeout)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantHost, host)
			if tt.wantFast {
				require.Less(t, time.Since(start), tt.timeout/2)
			}

			// Everything written has been consumed by the sniffer by now, so
			// closing the client bounds the replay read.
			_ = clientSide.Close()
			got, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.Equal(t, strings.Join(tt.writes, ""), string(got))
		})
	}
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

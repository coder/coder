package agentssh

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSSHShutdownWaitsForConnectionLog(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	release := make(chan struct{})
	releaseLog := sync.OnceFunc(func() { close(release) })
	defer releaseLog()
	sink := &blockedConnectionLogSink{entered: make(chan slog.SinkEntry, 1), release: release}
	server, err := NewServer(ctx, slog.Make(sink), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, &Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		releaseLog()
		_ = server.Close()
	})
	require.NoError(t, server.UpdateHostSigner(42))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		// #nosec G106 - The test connects to its own server.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)
	defer client.Close()
	destination, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer destination.Close()
	forward, err := client.Dial("tcp", destination.Addr().String())
	require.NoError(t, err)
	defer forward.Close()
	closeDone := make(chan error, 1)
	go func() { closeDone <- server.Close() }()
	entry := testutil.RequireReceive(ctx, t, sink.entered)
	require.Contains(t, entry.Fields, codersdk.DisconnectReasonServerShutdown.SlogField())
	// Forwarding-only connections rely on this completion log. Keep their
	// shutdown wait-group entry until every sink has accepted the reason.
	server.mu.RLock()
	tracked := len(server.conns)
	server.mu.RUnlock()
	require.Equal(t, 1, tracked)
	releaseLog()
	require.NoError(t, testutil.RequireReceive(ctx, t, closeDone))
	require.Error(t, testutil.RequireReceive(ctx, t, serveDone))
}

type blockedConnectionLogSink struct {
	entered chan slog.SinkEntry
	release <-chan struct{}
}

func (s *blockedConnectionLogSink) LogEntry(_ context.Context, entry slog.SinkEntry) {
	if entry.Message != "ssh connection complete" {
		return
	}
	s.entered <- entry
	<-s.release
}

func (*blockedConnectionLogSink) Sync() {}

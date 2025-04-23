package cli_test

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/net/tsaddr"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestConnectExists_Running(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var root cli.RootCmd
	cmd, err := root.Command(root.AGPL())
	require.NoError(t, err)

	inv := (&serpent.Invocation{
		Command: cmd,
		Args:    []string{"connect", "exists", "test.example"},
	}).WithContext(withCoderConnectRunning(ctx))
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	inv.Stdout = stdout
	inv.Stderr = stderr
	err = inv.Run()
	require.NoError(t, err)
}

func TestConnectExists_NotRunning(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var root cli.RootCmd
	cmd, err := root.Command(root.AGPL())
	require.NoError(t, err)

	inv := (&serpent.Invocation{
		Command: cmd,
		Args:    []string{"connect", "exists", "test.example"},
	}).WithContext(withCoderConnectNotRunning(ctx))
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	inv.Stdout = stdout
	inv.Stderr = stderr
	err = inv.Run()
	require.ErrorIs(t, err, cli.ErrSilent)
}

type fakeResolver struct {
	shouldReturnSuccess bool
}

func (f *fakeResolver) LookupIP(_ context.Context, _, _ string) ([]net.IP, error) {
	if f.shouldReturnSuccess {
		return []net.IP{net.ParseIP(tsaddr.CoderServiceIPv6().String())}, nil
	}
	return nil, &net.DNSError{IsNotFound: true}
}

func withCoderConnectRunning(ctx context.Context) context.Context {
	return workspacesdk.WithTestOnlyCoderContextResolver(ctx, &fakeResolver{shouldReturnSuccess: true})
}

func withCoderConnectNotRunning(ctx context.Context) context.Context {
	return workspacesdk.WithTestOnlyCoderContextResolver(ctx, &fakeResolver{shouldReturnSuccess: false})
}

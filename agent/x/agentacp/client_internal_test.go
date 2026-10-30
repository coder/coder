package agentacp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	acp "github.com/coder/acp-go-sdk"
)

func TestPermissionsAndCallbacks(t *testing.T) {
	t.Parallel()
	c := &discoveryClient{}
	require.NoError(t, c.SessionUpdate(context.Background(), acp.SessionNotification{}))
	for _, test := range []struct {
		options  []acp.PermissionOption
		canceled bool
		want     string
	}{
		{options: []acp.PermissionOption{{OptionId: "once", Kind: acp.PermissionOptionKindAllowOnce}, {OptionId: "always", Kind: acp.PermissionOptionKindAllowAlways}}, want: "always"},
		{options: []acp.PermissionOption{{OptionId: "once", Kind: acp.PermissionOptionKindAllowOnce}}, want: "once"},
		{options: []acp.PermissionOption{{OptionId: "deny", Kind: acp.PermissionOptionKindRejectAlways}}},
		{options: []acp.PermissionOption{{OptionId: "always", Kind: acp.PermissionOptionKindAllowAlways}}, canceled: true},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		if test.canceled {
			cancel()
		}
		result, err := c.RequestPermission(ctx, acp.RequestPermissionRequest{Options: test.options})
		cancel()
		require.NoError(t, err)
		if test.want == "" {
			require.NotNil(t, result.Outcome.Cancelled) //nolint:misspell // ACP uses this spelling.
		} else {
			require.Equal(t, test.want, string(result.Outcome.Selected.OptionId))
		}
	}
	_, err := c.ReadTextFile(context.Background(), acp.ReadTextFileRequest{})
	require.Error(t, err)
	_, err = c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{})
	require.Error(t, err)
	_, err = c.CreateTerminal(context.Background(), acp.CreateTerminalRequest{})
	require.Error(t, err)
	_, err = c.KillTerminal(context.Background(), acp.KillTerminalRequest{})
	require.Error(t, err)
	_, err = c.TerminalOutput(context.Background(), acp.TerminalOutputRequest{})
	require.Error(t, err)
	_, err = c.ReleaseTerminal(context.Background(), acp.ReleaseTerminalRequest{})
	require.Error(t, err)
	_, err = c.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{})
	require.Error(t, err)
}

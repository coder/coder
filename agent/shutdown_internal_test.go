package agent

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestAgentWorkspaceShutdown(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"stop", "delete", "empty", "start", "error", "offline", "timeout"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			a := &agent{hardCtx: ctx, logger: testutil.Logger(t), clock: clock}
			buildID := uuid.NewString()
			called := make(chan struct{})
			client := &shutdownTestClient{lookup: func(ctx context.Context) (*proto.WorkspaceShutdown, error) {
				close(called) // A second attempt fails the test.
				switch outcome {
				case "error":
					return nil, xerrors.New("control plane unavailable")
				case "timeout":
					<-ctx.Done()
					return nil, ctx.Err()
				case "empty":
					return &proto.WorkspaceShutdown{}, nil
				default:
					return &proto.WorkspaceShutdown{BuildId: buildID, Transition: outcome, BuildReason: "initiator"}, nil
				}
			}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				if outcome == "offline" {
					a.lookupWorkspaceShutdown(nil)
				} else {
					a.lookupWorkspaceShutdown(client)
				}
			}()
			if outcome == "timeout" {
				testutil.TryReceive(ctx, t, called)
				clock.Advance(time.Second).MustWait(ctx)
			}
			testutil.TryReceive(ctx, t, done)
			cause := a.sshShutdownCause()
			if outcome == "stop" || outcome == "delete" {
				require.Equal(t, codersdk.DisconnectReasonWorkspaceStopped, cause.Reason)
				require.Equal(t, buildID, cause.BuildID)
				require.Equal(t, outcome, cause.Transition)
				require.Equal(t, "initiator", cause.BuildReason)
			} else {
				require.Equal(t, codersdk.DisconnectReasonServerShutdown, cause.Reason)
				require.Empty(t, cause.BuildID)
			}
		})
	}
}

type shutdownTestClient struct {
	proto.DRPCAgentClient213
	lookup func(context.Context) (*proto.WorkspaceShutdown, error)
}

func (c *shutdownTestClient) GetWorkspaceShutdown(ctx context.Context, _ *emptypb.Empty) (*proto.WorkspaceShutdown, error) {
	return c.lookup(ctx)
}

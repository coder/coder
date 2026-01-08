package runner

import (
	"context"
	"time"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func (r *Runner) apply(ctx context.Context, stage string, req *sdkproto.ApplyRequest) (
	*sdkproto.ApplyComplete, *proto.FailedJob,
) {
	// use the notStopped so that if we attempt to gracefully cancel, the stream
	// will still be available for us to send the cancel to the provisioner
	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Apply{Apply: req}})
	if err != nil {
		return nil, r.failedWorkspaceBuildf("start provision: %s", err)
	}
	nevermind := make(chan struct{})
	defer close(nevermind)
	go func() {
		select {
		case <-nevermind:
			return
		case <-r.notStopped.Done():
			return
		case <-r.notCanceled.Done():
			_ = r.session.Send(&sdkproto.Request{
				Type: &sdkproto.Request_Cancel{
					Cancel: &sdkproto.CancelRequest{},
				},
			})
		}
	}()

	for {
		msg, err := r.session.Recv()
		if err != nil {
			return nil, r.failedWorkspaceBuildf("recv workspace provision: %s", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
			r.logProvisionerJobLog(context.Background(), msgType.Log.Level, "workspace provisioner job logged",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
				slog.F("workspace_build_id", r.job.GetWorkspaceBuild().WorkspaceBuildId),
			)

			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     stage,
			})
		case *sdkproto.Response_Apply:
			return msgType.Apply, nil
		default:
			return nil, r.failedJobf("unexpected plan response type %T", msg.Type)
		}
	}
}

package runner

import (
	"context"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func (r *Runner) plan(ctx context.Context, stage string, req *sdkproto.PlanRequest) (*sdkproto.PlanComplete, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Plan{Plan: req}})
	if err != nil {
		return nil, r.failedJobf("send plan request: %v", err)
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
			return nil, r.failedJobf("receive plan response: %v", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
			r.logProvisionerJobLog(context.Background(), msgType.Log.Level, "terraform planning",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)

			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     stage,
			})
		case *sdkproto.Response_Plan:
			return msgType.Plan, nil
		default:
			return nil, r.failedJobf("unexpected plan response type %T", msg.Type)
		}
	}
}

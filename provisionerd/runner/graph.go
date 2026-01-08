package runner

import (
	"context"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func (r *Runner) graph(ctx context.Context, req *sdkproto.GraphRequest) (*sdkproto.GraphComplete, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Graph{Graph: req}})
	if err != nil {
		return nil, r.failedJobf("send graph request: %v", err)
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
			return nil, r.failedJobf("receive graph response: %v", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
			r.logProvisionerJobLog(context.Background(), msgType.Log.Level, "terraform graphing",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)

			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     "Graphing Infrastructure",
			})
		case *sdkproto.Response_Graph:
			return msgType.Graph, nil
		default:
			return nil, r.failedJobf("unexpected graph response type %T", msg.Type)
		}
	}
}

package runner

import (
	"context"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func (r *Runner) init(ctx context.Context, omitModules bool, templateArchive []byte) (*sdkproto.InitComplete, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Init{Init: &sdkproto.InitRequest{
		TemplateSourceArchive: templateArchive,
		OmitModuleFiles:       omitModules,
	}}})
	if err != nil {
		return nil, r.failedJobf("send init request: %v", err)
	}

	for {
		msg, err := r.session.Recv()
		if err != nil {
			return nil, r.failedJobf("receive init response: %v", err)
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
				Stage:     "Initializing Terraform Directory",
			})
		case *sdkproto.Response_DataUpload:
			continue // Only for template imports
		case *sdkproto.Response_ChunkPiece:
			continue // Only for template imports
		case *sdkproto.Response_Init:
			return msgType.Init, nil
		default:
			return nil, r.failedJobf("unexpected init response type %T", msg.Type)
		}
	}
}

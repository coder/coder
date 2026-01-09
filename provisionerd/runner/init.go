package runner

import (
	"bytes"
	"context"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

//nolint:revive
func (r *Runner) init(ctx context.Context, omitModules bool, templateArchive []byte, moduleTar []byte) (*sdkproto.InitComplete, *proto.FailedJob) {
	ctx, span := r.startTrace(ctx, tracing.FuncName())
	defer span.End()

	// If `moduleTar` is populated, `init` will send it over in multiple parts. This
	// It must be called before the initial request to populate the correct hash if
	// there is data to send. This is safe to call on nil or empty slices.
	data, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleTar)

	hash := []byte{}
	if len(moduleTar) > 0 {
		hash = data.DataHash
	}

	err := r.session.Send(&sdkproto.Request{Type: &sdkproto.Request_Init{Init: &sdkproto.InitRequest{
		TemplateSourceArchive: templateArchive,
		OmitModuleFiles:       omitModules,
		InitialModuleTarHash:  hash,
	}}})
	if err != nil {
		return nil, r.failedJobf("send init request: %v", err)
	}

	// If the module tar exists, send over the data.
	if len(moduleTar) > 0 {
		err = r.session.Send(&sdkproto.Request{
			Type: &sdkproto.Request_File{
				File: &sdkproto.FileUpload{
					Type: &sdkproto.FileUpload_DataUpload{
						DataUpload: data,
					},
				},
			},
		})
		if err != nil {
			return nil, r.failedJobf("send module files data upload: %v", err)
		}

		for _, c := range chunks {
			err = r.session.Send(&sdkproto.Request{
				Type: &sdkproto.Request_File{
					File: &sdkproto.FileUpload{
						Type: &sdkproto.FileUpload_ChunkPiece{
							ChunkPiece: c,
						},
					},
				},
			})
			if err != nil {
				return nil, r.failedJobf("send module files chunk: %v", err)
			}
		}
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

	var moduleFilesUpload *sdkproto.DataBuilder
	for {
		msg, err := r.session.Recv()
		if err != nil {
			return nil, r.failedJobf("receive init response: %v", err)
		}
		switch msgType := msg.Type.(type) {
		case *sdkproto.Response_Log:
			r.logProvisionerJobLog(context.Background(), msgType.Log.Level, "terraform initialization",
				slog.F("level", msgType.Log.Level),
				slog.F("output", msgType.Log.Output),
			)

			r.queueLog(ctx, &proto.Log{
				Source:    proto.LogSource_PROVISIONER,
				Level:     msgType.Log.Level,
				CreatedAt: time.Now().UnixMilli(),
				Output:    msgType.Log.Output,
				Stage:     "Initializing Terraform Directory",
			})
		case *sdkproto.Response_DataUpload:
			if omitModules {
				return nil, r.failedJobf("received unexpected module files data upload when omitModules is true")
			}
			c := msgType.DataUpload
			if c.UploadType != sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES {
				return nil, r.failedJobf("invalid data upload type: %q", c.UploadType)
			}

			if moduleFilesUpload != nil {
				return nil, r.failedJobf("multiple module data uploads received, only expect 1")
			}

			moduleFilesUpload, err = sdkproto.NewDataBuilder(c)
			if err != nil {
				return nil, r.failedJobf("create data builder: %s", err.Error())
			}
		case *sdkproto.Response_ChunkPiece:
			if omitModules {
				return nil, r.failedJobf("received unexpected module files data upload when omitModules is true")
			}
			c := msgType.ChunkPiece
			if moduleFilesUpload == nil {
				return nil, r.failedJobf("received chunk piece before module files data upload")
			}

			_, err := moduleFilesUpload.Add(c)
			if err != nil {
				return nil, r.failedJobf("module files, add chunk piece: %s", err.Error())
			}
		case *sdkproto.Response_Init:
			if moduleFilesUpload != nil {
				// If files were uploaded in multiple chunks, put them back together.
				moduleFilesData, err := moduleFilesUpload.Complete()
				if err != nil {
					return nil, r.failedJobf("complete module files data upload: %s", err.Error())
				}

				if !bytes.Equal(msgType.Init.ModuleFilesHash, moduleFilesUpload.Hash) {
					return nil, r.failedJobf("module files hash mismatch, uploaded: %x, expected: %x", moduleFilesUpload.Hash, msgType.Init.ModuleFilesHash)
				}
				msgType.Init.ModuleFiles = moduleFilesData
			}

			return msgType.Init, nil
		default:
			return nil, r.failedJobf("unexpected init response type %T", msg.Type)
		}
	}
}

package provisionersdk

import (
	"io"

	"golang.org/x/xerrors"

	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// HandleReceivingDataUpload can download a multi-part file from a proto stream.
// The stream is expected to be closed by the caller.
func HandleReceivingDataUpload(stream interface {
	Recv() (*sdkproto.FileUpload, error)
},
) (*sdkproto.DataBuilder, error) {
	var file *sdkproto.DataBuilder
UploadFileStream:
	for {
		msg, err := stream.Recv()
		if err != nil {
			if xerrors.Is(err, io.EOF) {
				// Do not return an EOF here, as it is a "retryable error" in the client context.
				// This failure indicates the download stream was closed prematurely, and it is a
				// fatal error.
				return nil, xerrors.Errorf("stream closed before file download complete")
			}
			return nil, xerrors.Errorf("receive file download: %w", err)
		}

		switch typed := msg.Type.(type) {
		case *sdkproto.FileUpload_Error:
			return nil, xerrors.Errorf("download file: %s", typed.Error.Error)
		case *sdkproto.FileUpload_DataUpload:
			if file != nil {
				return nil, xerrors.New("unexpected file download while waiting for file completion")
			}

			file, err = sdkproto.NewDataBuilder(&sdkproto.DataUpload{
				UploadType: typed.DataUpload.UploadType,
				DataHash:   typed.DataUpload.DataHash,
				FileSize:   typed.DataUpload.FileSize,
				Chunks:     typed.DataUpload.Chunks,
			})
			if err != nil {
				return nil, xerrors.Errorf("unable to create file download: %w", err)
			}

			if file.IsDone() {
				// If a file is 0 bytes, we can consider it done immediately.
				// This should never really happen in practice, but we handle it gracefully.
				break UploadFileStream
			}
		case *sdkproto.FileUpload_ChunkPiece:
			if file == nil {
				return nil, xerrors.New("unexpected chunk piece while waiting for file upload")
			}

			done, err := file.Add(&sdkproto.ChunkPiece{
				Data:         typed.ChunkPiece.Data,
				FullDataHash: typed.ChunkPiece.FullDataHash,
				PieceIndex:   typed.ChunkPiece.PieceIndex,
			})
			if err != nil {
				return nil, xerrors.Errorf("unable to add a chunk piece: %w", err)
			}

			if done {
				break UploadFileStream
			}
		default:
			// This should never happen
			return nil, xerrors.Errorf("received unknown file upload message type: %T", msg.Type)
		}
	}

	// This needs to be called again by the caller to retrieve the final payload.
	// It is called here to do a hash check and ensure the file is correct.
	_, err := file.Complete()
	if err != nil {
		return nil, xerrors.Errorf("complete file upload: %w", err)
	}

	return file, nil
}

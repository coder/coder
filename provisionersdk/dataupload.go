package provisionersdk

import (
	"io"

	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"golang.org/x/xerrors"
)

func HandleReceivingDataUpload(stream interface {
	Recv() (*proto.UploadFileRequest, error)
}) (*sdkproto.DataBuilder, error) {
	var file *sdkproto.DataBuilder
UploadFileStream:
	for {
		msg, err := stream.Recv()
		if err != nil {
			if xerrors.Is(err, io.EOF) {
				return nil, xerrors.Errorf("stream closed before file download complete")
			}
			return nil, xerrors.Errorf("receive file download: %w", err)
		}

		switch typed := msg.Type.(type) {
		case *proto.UploadFileRequest_Error:
			return nil, xerrors.Errorf("download file: %s", typed.Error.Error)
		case *proto.UploadFileRequest_DataUpload:
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
		case *proto.UploadFileRequest_ChunkPiece:
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
		}
	}

	// TODO: Should we do this here?
	_, err := file.Complete()
	if err != nil {
		return nil, xerrors.Errorf("complete file upload: %w", err)
	}

	return file, nil
}

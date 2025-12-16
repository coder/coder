package provisionerdserver_test

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	proto "github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestUploadFileLargeModuleFiles tests the UploadFile RPC with large module files
func TestUploadFileLargeModuleFiles(t *testing.T) {
	t.Parallel()

	// Create server
	server, db, _, _ := setup(t, false, &overrides{
		externalAuthConfigs: []*externalauth.Config{{}},
	})

	testSizes := []int{
		0,                             // Empty file
		512,                           // A small file
		drpcsdk.MaxMessageSize + 1024, // Just over the limit
		drpcsdk.MaxMessageSize * 2,    // 2x the limit
		sdkproto.ChunkSize*3 + 512,    // Multiple chunks with partial last
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d_bytes", size), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)

			// Generate test module files data
			moduleData := make([]byte, size)
			_, err := crand.Read(moduleData)
			require.NoError(t, err)

			// Convert to upload format
			upload, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleData)

			stream := newMockUploadStream(upload, chunks...)

			// Execute upload
			err = server.UploadFile(stream)
			require.NoError(t, err)

			// Upload should be done
			require.True(t, stream.isDone(), "stream should be done after upload")

			// Verify file was stored in database
			hashString := fmt.Sprintf("%x", upload.DataHash)
			file, err := db.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{
				Hash:      hashString,
				CreatedBy: uuid.Nil, // Provisionerd creates with Nil UUID
			})
			require.NoError(t, err)
			require.Equal(t, hashString, file.Hash)
			require.Equal(t, moduleData, file.Data)
			require.Equal(t, "application/x-tar", file.Mimetype)

			// Try to upload it again, and it should still be successful
			stream = newMockUploadStream(upload, chunks...)
			err = server.UploadFile(stream)
			require.NoError(t, err, "re-upload should succeed without error")
			require.True(t, stream.isDone(), "stream should be done after re-upload")
		})
	}
}

// TestUploadFileErrorScenarios tests various error conditions in file upload
func TestUploadFileErrorScenarios(t *testing.T) {
	t.Parallel()

	//nolint:dogsled
	server, _, _, _ := setup(t, false, &overrides{
		externalAuthConfigs: []*externalauth.Config{{}},
	})

	// Generate test data
	moduleData := make([]byte, sdkproto.ChunkSize*2)
	_, err := crand.Read(moduleData)
	require.NoError(t, err)

	upload, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleData)

	t.Run("chunk_before_upload", func(t *testing.T) {
		t.Parallel()

		stream := newMockUploadStream(nil, chunks[0])

		err := server.UploadFile(stream)
		require.ErrorContains(t, err, "unexpected chunk piece while waiting for file upload")
		require.True(t, stream.isDone(), "stream should be done after error")
	})

	t.Run("duplicate_upload", func(t *testing.T) {
		t.Parallel()

		stream := &mockUploadStream{
			done:     make(chan struct{}),
			messages: make(chan *sdkproto.FileUpload, 2),
		}

		up := &sdkproto.FileUpload{Type: &sdkproto.FileUpload_DataUpload{DataUpload: upload}}

		// Send it twice
		stream.messages <- up
		stream.messages <- up

		err := server.UploadFile(stream)
		require.ErrorContains(t, err, "unexpected file download while waiting for file completion")
		require.True(t, stream.isDone(), "stream should be done after error")
	})

	t.Run("unsupported_upload_type", func(t *testing.T) {
		t.Parallel()

		//nolint:govet // Ignore lock copy
		cpy := *upload
		cpy.UploadType = sdkproto.DataUploadType_UPLOAD_TYPE_UNKNOWN // Set to an unsupported type
		stream := newMockUploadStream(&cpy, chunks...)

		err := server.UploadFile(stream)
		require.ErrorContains(t, err, "unsupported file upload type")
		require.True(t, stream.isDone(), "stream should be done after error")
	})
}

type mockUploadStream struct {
	done     chan struct{}
	messages chan *sdkproto.FileUpload
}

func (m mockUploadStream) SendAndClose(empty *proto.Empty) error {
	close(m.done)
	return nil
}

func (m mockUploadStream) Recv() (*sdkproto.FileUpload, error) {
	msg, ok := <-m.messages
	if !ok {
		return nil, xerrors.New("no more messages to receive")
	}
	return msg, nil
}
func (*mockUploadStream) Context() context.Context { panic(errUnimplemented) }
func (*mockUploadStream) MsgSend(msg drpc.Message, enc drpc.Encoding) error {
	panic(errUnimplemented)
}

func (*mockUploadStream) MsgRecv(msg drpc.Message, enc drpc.Encoding) error {
	panic(errUnimplemented)
}
func (*mockUploadStream) CloseSend() error { panic(errUnimplemented) }
func (*mockUploadStream) Close() error     { panic(errUnimplemented) }
func (m *mockUploadStream) isDone() bool {
	select {
	case <-m.done:
		return true
	default:
		return false
	}
}

func newMockUploadStream(up *sdkproto.DataUpload, chunks ...*sdkproto.ChunkPiece) *mockUploadStream {
	stream := &mockUploadStream{
		done:     make(chan struct{}),
		messages: make(chan *sdkproto.FileUpload, 1+len(chunks)),
	}
	if up != nil {
		stream.messages <- &sdkproto.FileUpload{Type: &sdkproto.FileUpload_DataUpload{DataUpload: up}}
	}

	for _, chunk := range chunks {
		stream.messages <- &sdkproto.FileUpload{Type: &sdkproto.FileUpload_ChunkPiece{ChunkPiece: chunk}}
	}
	close(stream.messages)
	return stream
}

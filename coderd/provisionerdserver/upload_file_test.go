package provisionerdserver_test

import (
	crand "crypto/rand"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	proto "github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestUploadFileLargeModuleFiles tests the UploadFile RPC with large module files
func TestUploadFileLargeModuleFiles(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := testutil.Logger(t)
	db := dbmem.New()

	// Create server
	server := provisionerdserver.New(ctx, &provisionerdserver.Options{
		Database: db,
		Logger:   logger,
	})

	testSizes := []int{
		drpcsdk.MaxMessageSize + 1024, // Just over 4MB
		drpcsdk.MaxMessageSize * 2,    // 8MB
		sdkproto.ChunkSize*3 + 512,    // Multiple chunks with partial last
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d_bytes", size), func(t *testing.T) {
			// Generate test module files data
			moduleData := make([]byte, size)
			_, err := crand.Read(moduleData)
			require.NoError(t, err)

			// Convert to upload format
			upload, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleData)

			// Create mock stream
			stream := &mockUploadStream{
				requests: make([]*proto.UploadFileRequest, 0),
				response: &proto.Empty{},
			}

			// Add DataUpload request
			stream.requests = append(stream.requests, &proto.UploadFileRequest{
				Type: &proto.UploadFileRequest_DataUpload{
					DataUpload: &proto.DataUpload{
						UploadType: proto.DataUploadType(upload.UploadType),
						DataHash:   upload.DataHash,
						FileSize:   upload.FileSize,
						Chunks:     upload.Chunks,
					},
				},
			})

			// Add chunk requests
			for _, chunk := range chunks {
				stream.requests = append(stream.requests, &proto.UploadFileRequest{
					Type: &proto.UploadFileRequest_ChunkPiece{
						ChunkPiece: &proto.ChunkPiece{
							Data:         chunk.Data,
							FullDataHash: chunk.FullDataHash,
							PieceIndex:   chunk.PieceIndex,
						},
					},
				})
			}

			// Execute upload
			err = server.UploadFile(stream)
			require.NoError(t, err)

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
		})
	}
}

// TestUploadFileErrorScenarios tests various error conditions in file upload
func TestUploadFileErrorScenarios(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	db := dbmem.New()

	server := provisionerdserver.New(ctx, &provisionerdserver.Options{
		Database: db,
		Logger:   logger,
	})

	// Generate test data
	moduleData := make([]byte, sdkproto.ChunkSize*2)
	_, err := crand.Read(moduleData)
	require.NoError(t, err)

	upload, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleData)

	t.Run("chunk_before_upload", func(t *testing.T) {
		stream := &mockUploadStream{
			requests: []*proto.UploadFileRequest{
				// Send chunk before DataUpload
				{
					Type: &proto.UploadFileRequest_ChunkPiece{
						ChunkPiece: &proto.ChunkPiece{
							Data:         chunks[0].Data,
							FullDataHash: chunks[0].FullDataHash,
							PieceIndex:   chunks[0].PieceIndex,
						},
					},
				},
			},
			response: &proto.Empty{},
		}

		err := server.UploadFile(stream)
		require.ErrorContains(t, err, "unexpected chunk piece while waiting for file upload")
	})

	t.Run("duplicate_upload", func(t *testing.T) {
		stream := &mockUploadStream{
			requests: []*proto.UploadFileRequest{
				{
					Type: &proto.UploadFileRequest_DataUpload{
						DataUpload: &proto.DataUpload{
							UploadType: proto.DataUploadType(upload.UploadType),
							DataHash:   upload.DataHash,
							FileSize:   upload.FileSize,
							Chunks:     upload.Chunks,
						},
					},
				},
				// Send another DataUpload
				{
					Type: &proto.UploadFileRequest_DataUpload{
						DataUpload: &proto.DataUpload{
							UploadType: proto.DataUploadType(upload.UploadType),
							DataHash:   upload.DataHash,
							FileSize:   upload.FileSize,
							Chunks:     upload.Chunks,
						},
					},
				},
			},
			response: &proto.Empty{},
		}

		err := server.UploadFile(stream)
		require.ErrorContains(t, err, "unexpected file upload while waiting for file completion")
	})

	t.Run("unsupported_upload_type", func(t *testing.T) {
		stream := &mockUploadStream{
			requests: []*proto.UploadFileRequest{
				{
					Type: &proto.UploadFileRequest_DataUpload{
						DataUpload: &proto.DataUpload{
							UploadType: proto.DataUploadType_UPLOAD_TYPE_UNKNOWN,
							DataHash:   upload.DataHash,
							FileSize:   upload.FileSize,
							Chunks:     upload.Chunks,
						},
					},
				},
			},
			response: &proto.Empty{},
		}

		// Add all chunks
		for _, chunk := range chunks {
			stream.requests = append(stream.requests, &proto.UploadFileRequest{
				Type: &proto.UploadFileRequest_ChunkPiece{
					ChunkPiece: &proto.ChunkPiece{
						Data:         chunk.Data,
						FullDataHash: chunk.FullDataHash,
						PieceIndex:   chunk.PieceIndex,
					},
				},
			})
		}

		err := server.UploadFile(stream)
		require.ErrorContains(t, err, "unsupported file upload type")
	})
}

// TestUploadFileDuplicateHandling tests that duplicate files are handled correctly
func TestUploadFileDuplicateHandling(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	db := dbmem.New()

	server := provisionerdserver.New(ctx, &provisionerdserver.Options{
		Database: db,
		Logger:   logger,
	})

	// Generate test data
	moduleData := make([]byte, 1024)
	_, err := crand.Read(moduleData)
	require.NoError(t, err)

	upload, chunks := sdkproto.BytesToDataUpload(sdkproto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, moduleData)

	// Create upload request
	createUploadStream := func() *mockUploadStream {
		stream := &mockUploadStream{
			requests: []*proto.UploadFileRequest{
				{
					Type: &proto.UploadFileRequest_DataUpload{
						DataUpload: &proto.DataUpload{
							UploadType: proto.DataUploadType(upload.UploadType),
							DataHash:   upload.DataHash,
							FileSize:   upload.FileSize,
							Chunks:     upload.Chunks,
						},
					},
				},
			},
			response: &proto.Empty{},
		}

		for _, chunk := range chunks {
			stream.requests = append(stream.requests, &proto.UploadFileRequest{
				Type: &proto.UploadFileRequest_ChunkPiece{
					ChunkPiece: &proto.ChunkPiece{
						Data:         chunk.Data,
						FullDataHash: chunk.FullDataHash,
						PieceIndex:   chunk.PieceIndex,
					},
				},
			})
		}
		return stream
	}

	// Upload file first time
	err = server.UploadFile(createUploadStream())
	require.NoError(t, err)

	// Upload same file again - should not error (duplicate handling)
	err = server.UploadFile(createUploadStream())
	require.NoError(t, err)

	// Verify only one file exists in database
	hashString := fmt.Sprintf("%x", upload.DataHash)
	file, err := db.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{
		Hash:      hashString,
		CreatedBy: uuid.Nil,
	})
	require.NoError(t, err)
	require.Equal(t, moduleData, file.Data)
}

// mockUploadStream implements the upload stream interface for testing
type mockUploadStream struct {
	requests []*proto.UploadFileRequest
	response *proto.Empty
	index    int
}

func (m *mockUploadStream) Recv() (*proto.UploadFileRequest, error) {
	if m.index >= len(m.requests) {
		return nil, context.Canceled // EOF
	}
	req := m.requests[m.index]
	m.index++
	return req, nil
}

func (m *mockUploadStream) SendAndClose(resp *proto.Empty) error {
	m.response = resp
	return nil
}

func (m *mockUploadStream) Context() context.Context {
	return context.Background()
}

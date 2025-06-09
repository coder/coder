package proto_test

import (
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

// Fuzz must be run manually with the `-fuzz` flag to generate random test cases.
// By default, it only runs the added seed corpus cases.
// go test -fuzz=FuzzBytesToDataUpload
func FuzzBytesToDataUpload(f *testing.F) {
	// Cases to always run in standard `go test` runs.
	always := [][]byte{
		{},
		[]byte("1"),
		[]byte("small"),
	}
	for _, data := range always {
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		first, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, data)

		builder, err := proto.NewDataBuilder(first)
		require.NoError(t, err)

		var done bool
		for _, chunk := range chunks {
			require.False(t, done)
			done, err = builder.Add(chunk)
			require.NoError(t, err)
		}

		if len(chunks) > 0 {
			require.True(t, done)
		}

		finalData, err := builder.Complete()
		require.NoError(t, err)
		require.Equal(t, data, finalData)
	})
}

// TestBytesToDataUpload tests the BytesToDataUpload function and the DataBuilder
// with large random data uploads.
func TestBytesToDataUpload(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		// Generate random data
		//nolint:gosec // Just a unit test
		chunkCount := 1 + rand.Intn(3)
		size := (chunkCount * proto.ChunkSize) + (rand.Int() % proto.ChunkSize)
		data := make([]byte, size)
		_, err := crand.Read(data)
		require.NoError(t, err)

		first, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, data)
		builder, err := proto.NewDataBuilder(first)
		require.NoError(t, err)

		// Try to add some bad chunks
		_, err = builder.Add(&proto.ChunkPiece{Data: []byte{}, FullDataHash: make([]byte, 32)})
		require.ErrorContains(t, err, "data hash does not match")

		// Verify 'Complete' fails before adding any chunks
		_, err = builder.Complete()
		require.ErrorContains(t, err, "data upload is not complete")

		// Add the chunks
		var done bool
		for _, chunk := range chunks {
			require.False(t, done, "data upload should not be complete before adding all chunks")

			done, err = builder.Add(chunk)
			require.NoError(t, err, "chunk %d should be added successfully", chunk.PieceIndex)
		}
		require.True(t, done, "data upload should be complete after adding all chunks")

		// Try to add another chunk after completion
		done, err = builder.Add(chunks[0])
		require.ErrorContains(t, err, "data upload is already complete")
		require.True(t, done, "still complete")

		// Verify the final data matches the original
		got, err := builder.Complete()
		require.NoError(t, err)

		require.Equal(t, data, got, "final data should match the original data")
	}
}

// TestLargeModulePayload tests handling of module files that exceed the 4MB protobuf limit
func TestLargeModulePayload(t *testing.T) {
	t.Parallel()

	// Test with data larger than MaxMessageSize (4MB)
	testSizes := []int{
		drpcsdk.MaxMessageSize + 1,           // Just over the limit
		drpcsdk.MaxMessageSize * 2,           // Double the limit
		drpcsdk.MaxMessageSize + proto.ChunkSize/2, // Partial chunk over limit
	}

	for _, size := range testSizes {
		t.Run(fmt.Sprintf("size_%d_bytes", size), func(t *testing.T) {
			// Generate large random data to simulate module files
			data := make([]byte, size)
			_, err := crand.Read(data)
			require.NoError(t, err)

			// Convert to upload format
			upload, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, data)

			// Verify upload metadata
			require.Equal(t, proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, upload.UploadType)
			require.Equal(t, int64(size), upload.FileSize)
			require.Greater(t, upload.Chunks, int32(0))
			require.Len(t, upload.DataHash, 32) // SHA256 hash

			// Verify chunk count calculation
			expectedChunks := (int32(size) + proto.ChunkSize - 1) / proto.ChunkSize
			require.Equal(t, expectedChunks, upload.Chunks)
			require.Len(t, chunks, int(expectedChunks))

			// Create builder and process chunks
			builder, err := proto.NewDataBuilder(upload)
			require.NoError(t, err)

			// Process all chunks in order
			for i, chunk := range chunks {
				require.Equal(t, int32(i), chunk.PieceIndex)
				require.Equal(t, upload.DataHash, chunk.FullDataHash)

				done, err := builder.Add(chunk)
				require.NoError(t, err)

				// Should only be done on the last chunk
				if i == len(chunks)-1 {
					require.True(t, done)
				} else {
					require.False(t, done)
				}
			}

			// Complete and verify
			finalData, err := builder.Complete()
			require.NoError(t, err)
			require.Equal(t, data, finalData)
		})
	}
}

// TestDataBuilderErrorScenarios tests various error conditions
func TestDataBuilderErrorScenarios(t *testing.T) {
	t.Parallel()

	// Create test data
	data := make([]byte, proto.ChunkSize*2+100)
	_, err := crand.Read(data)
	require.NoError(t, err)

	upload, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, data)

	t.Run("invalid_hash_length", func(t *testing.T) {
		invalidUpload := *upload
		invalidUpload.DataHash = []byte("short")
		_, err := proto.NewDataBuilder(&invalidUpload)
		require.ErrorContains(t, err, "data hash must be 32 bytes")
	})

	t.Run("wrong_chunk_hash", func(t *testing.T) {
		builder, err := proto.NewDataBuilder(upload)
		require.NoError(t, err)

		wrongChunk := *chunks[0]
		wrongChunk.FullDataHash = make([]byte, 32) // Wrong hash
		_, err = builder.Add(&wrongChunk)
		require.ErrorContains(t, err, "data hash does not match")
	})

	t.Run("out_of_order_chunks", func(t *testing.T) {
		builder, err := proto.NewDataBuilder(upload)
		require.NoError(t, err)

		// Try to add chunk 1 before chunk 0
		_, err = builder.Add(chunks[1])
		require.ErrorContains(t, err, "chunks ordering")
	})

	t.Run("chunk_too_large", func(t *testing.T) {
		builder, err := proto.NewDataBuilder(upload)
		require.NoError(t, err)

		// Create a chunk that would exceed the expected size
		oversizedChunk := *chunks[0]
		oversizedChunk.Data = make([]byte, int(upload.FileSize)+1)
		_, err = builder.Add(&oversizedChunk)
		require.ErrorContains(t, err, "data exceeds expected size")
	})

	t.Run("add_after_completion", func(t *testing.T) {
		builder, err := proto.NewDataBuilder(upload)
		require.NoError(t, err)

		// Add all chunks
		for _, chunk := range chunks {
			_, err := builder.Add(chunk)
			require.NoError(t, err)
		}

		// Try to add another chunk
		_, err = builder.Add(chunks[0])
		require.ErrorContains(t, err, "data upload is already complete")
	})

	t.Run("complete_before_all_chunks", func(t *testing.T) {
		builder, err := proto.NewDataBuilder(upload)
		require.NoError(t, err)

		// Add only first chunk
		_, err = builder.Add(chunks[0])
		require.NoError(t, err)

		// Try to complete
		_, err = builder.Complete()
		require.ErrorContains(t, err, "data upload is not complete")
	})
}

// TestChunkSizeBoundaries tests edge cases around chunk size boundaries
func TestChunkSizeBoundaries(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		size int
	}{
		{"exactly_one_chunk", proto.ChunkSize},
		{"one_byte_over_chunk", proto.ChunkSize + 1},
		{"exactly_two_chunks", proto.ChunkSize * 2},
		{"partial_last_chunk", proto.ChunkSize*2 + proto.ChunkSize/2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.size)
			_, err := crand.Read(data)
			require.NoError(t, err)

			upload, chunks := proto.BytesToDataUpload(proto.DataUploadType_UPLOAD_TYPE_MODULE_FILES, data)

			// Verify chunk sizes
			totalSize := 0
			for i, chunk := range chunks {
				if i < len(chunks)-1 {
					// All chunks except last should be full size
					require.Equal(t, proto.ChunkSize, len(chunk.Data))
				} else {
					// Last chunk can be partial
					require.LessOrEqual(t, len(chunk.Data), proto.ChunkSize)
					require.Greater(t, len(chunk.Data), 0)
				}
				totalSize += len(chunk.Data)
			}
			require.Equal(t, tc.size, totalSize)

			// Verify round-trip
			builder, err := proto.NewDataBuilder(upload)
			require.NoError(t, err)

			for _, chunk := range chunks {
				_, err := builder.Add(chunk)
				require.NoError(t, err)
			}

			finalData, err := builder.Complete()
			require.NoError(t, err)
			require.Equal(t, data, finalData)
		})
	}
}

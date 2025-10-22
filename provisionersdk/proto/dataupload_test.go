package proto_test

import (
	crand "crypto/rand"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

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
		//nolint:gosec // Just a unit test
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

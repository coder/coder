package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet/proto"
)

func TestCoordinateResponse_Chunked(t *testing.T) {
	t.Parallel()

	t.Run("NoChunkingNeeded", func(t *testing.T) {
		t.Parallel()
		resp := &proto.CoordinateResponse{
			PeerUpdates: make([]*proto.CoordinateResponse_PeerUpdate, 100),
		}
		chunks := resp.Chunked()
		require.Len(t, chunks, 1)
		require.Equal(t, resp, chunks[0])
	})

	t.Run("ExactLimit", func(t *testing.T) {
		t.Parallel()
		resp := &proto.CoordinateResponse{
			PeerUpdates: make([]*proto.CoordinateResponse_PeerUpdate, 1024),
		}
		chunks := resp.Chunked()
		require.Len(t, chunks, 1)
		require.Equal(t, resp, chunks[0])
	})

	t.Run("MultipleChunks", func(t *testing.T) {
		t.Parallel()
		n := 1024*3 + 500
		resp := &proto.CoordinateResponse{
			PeerUpdates: make([]*proto.CoordinateResponse_PeerUpdate, n),
		}
		chunks := resp.Chunked()
		require.Len(t, chunks, 4)
		total := 0
		for _, c := range chunks {
			total += len(c.GetPeerUpdates())
		}
		require.Equal(t, n, total)
	})

	t.Run("EmptyResponse", func(t *testing.T) {
		t.Parallel()
		resp := &proto.CoordinateResponse{}
		chunks := resp.Chunked()
		require.Len(t, chunks, 1)
	})
}

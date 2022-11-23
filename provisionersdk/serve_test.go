package provisionersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"storj.io/drpc/drpcerr"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestProvisionerSDK(t *testing.T) {
	t.Parallel()
	t.Run("Serve", func(t *testing.T) {
		t.Parallel()
		client, server := provisionersdk.MemTransportPipe()
		defer client.Close()
		defer server.Close()

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		go func() {
			err := provisionersdk.Serve(ctx, &proto.DRPCProvisionerUnimplementedServer{}, &provisionersdk.ServeOptions{
				Listener: server,
			})
			assert.NoError(t, err)
		}()

		api := proto.NewDRPCProvisionerClient(client)
		stream, err := api.Parse(context.Background(), &proto.Parse_Request{})
		require.NoError(t, err)
		_, err = stream.Recv()
		require.Equal(t, drpcerr.Unimplemented, int(drpcerr.Code(err)))
	})

	t.Run("ServeClosedPipe", func(t *testing.T) {
		t.Parallel()
		client, server := provisionersdk.MemTransportPipe()
		_ = client.Close()
		_ = server.Close()

		err := provisionersdk.Serve(context.Background(), &proto.DRPCProvisionerUnimplementedServer{}, &provisionersdk.ServeOptions{
			Listener: server,
		})
		require.NoError(t, err)
	})
}

package provisionersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
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
			err := provisionersdk.Serve(ctx, unimplementedServer{}, &provisionersdk.ServeOptions{
				Listener:      server,
				WorkDirectory: t.TempDir(),
			})
			assert.NoError(t, err)
		}()

		api := proto.NewDRPCProvisionerClient(client)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s, err := api.Session(context.Background())
		require.NoError(t, err)
		err = s.Send(&proto.Request{Type: &proto.Request_Config{Config: &proto.Config{}}})
		require.NoError(t, err)

		err = s.Send(&proto.Request{Type: &proto.Request_Parse{Parse: &proto.ParseRequest{}}})
		require.NoError(t, err)
		msg, err := s.Recv()
		require.NoError(t, err)
		require.Equal(t, "unimplemented", msg.GetParse().GetError())

		err = s.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{}}})
		require.NoError(t, err)
		msg, err = s.Recv()
		require.NoError(t, err)
		// Plan has no error so that we're allowed to run Apply
		require.Equal(t, "", msg.GetPlan().GetError())

		err = s.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{}}})
		require.NoError(t, err)
		msg, err = s.Recv()
		require.NoError(t, err)
		require.Equal(t, "unimplemented", msg.GetApply().GetError())
	})

	t.Run("ServeClosedPipe", func(t *testing.T) {
		t.Parallel()
		client, server := provisionersdk.MemTransportPipe()
		_ = client.Close()
		_ = server.Close()

		err := provisionersdk.Serve(context.Background(), unimplementedServer{}, &provisionersdk.ServeOptions{
			Listener:      server,
			WorkDirectory: t.TempDir(),
		})
		require.NoError(t, err)
	})
}

type unimplementedServer struct{}

func (_ unimplementedServer) Parse(_ *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	return &proto.ParseComplete{Error: "unimplemented"}
}

func (_ unimplementedServer) Plan(_ *provisionersdk.Session, _ *proto.PlanRequest, _ <-chan struct{}) *proto.PlanComplete {
	return &proto.PlanComplete{}
}

func (_ unimplementedServer) Apply(_ *provisionersdk.Session, _ *proto.ApplyRequest, _ <-chan struct{}) *proto.ApplyComplete {
	return &proto.ApplyComplete{Error: "unimplemented"}
}

package provisionersdk_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestProvisionerSDK(t *testing.T) {
	t.Parallel()
	t.Run("ServeListener", func(t *testing.T) {
		t.Parallel()
		client, server := drpc.MemTransportPipe()
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
		s, err := api.Session(ctx)
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
		client, server := drpc.MemTransportPipe()
		_ = client.Close()
		_ = server.Close()

		err := provisionersdk.Serve(context.Background(), unimplementedServer{}, &provisionersdk.ServeOptions{
			Listener:      server,
			WorkDirectory: t.TempDir(),
		})
		require.NoError(t, err)
	})

	t.Run("ServeConn", func(t *testing.T) {
		t.Parallel()
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancelFunc()
		srvErr := make(chan error, 1)
		go func() {
			err := provisionersdk.Serve(ctx, unimplementedServer{}, &provisionersdk.ServeOptions{
				Conn:          server,
				WorkDirectory: t.TempDir(),
			})
			srvErr <- err
		}()

		api := proto.NewDRPCProvisionerClient(drpcconn.New(client))
		s, err := api.Session(ctx)
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

		// Check provisioner closes when the connection does
		err = s.Close()
		require.NoError(t, err)
		err = api.DRPCConn().Close()
		require.NoError(t, err)
		select {
		case <-ctx.Done():
			t.Fatal("timeout waiting for provisioner")
		case err = <-srvErr:
			require.NoError(t, err)
		}
	})
}

type unimplementedServer struct{}

func (unimplementedServer) Parse(_ *provisionersdk.Session, _ *proto.ParseRequest, _ <-chan struct{}) *proto.ParseComplete {
	return &proto.ParseComplete{Error: "unimplemented"}
}

func (unimplementedServer) Plan(_ *provisionersdk.Session, _ *proto.PlanRequest, _ <-chan struct{}) *proto.PlanComplete {
	return &proto.PlanComplete{}
}

func (unimplementedServer) Apply(_ *provisionersdk.Session, _ *proto.ApplyRequest, _ <-chan struct{}) *proto.ApplyComplete {
	return &proto.ApplyComplete{Error: "unimplemented"}
}

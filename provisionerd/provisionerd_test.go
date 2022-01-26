package provisionerd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcconn"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
)

func TestProvisionerd(t *testing.T) {
	t.Parallel()
	t.Run("Example", func(t *testing.T) {
		t.Parallel()

		api := provisionerd.New(func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return setupClient(t, &provisionerdTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					return nil, nil
				},
				updateJob: func(stream proto.DRPCProvisionerDaemon_UpdateJobStream) error {
					for {
						_, _ = stream.Recv()
					}
					return nil
				},
			}), nil
		}, provisionerd.Provisioners{}, &provisionerd.Options{})
		defer api.Close()
	})
}

func setupClient(t *testing.T, server *provisionerdTestServer) proto.DRPCProvisionerDaemonClient {
	mux := drpcmux.New()
	err := proto.DRPCRegisterProvisionerDaemon(mux, server)
	require.NoError(t, err)
	srv := drpcserver.New(mux)

	clientConn, serverConn := provisionersdk.TransportPipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	go func() {
		_ = srv.ServeOne(context.Background(), serverConn)
	}()
	return proto.NewDRPCProvisionerDaemonClient(drpcconn.New((clientConn)))
}

type provisionerdTestServer struct {
	acquireJob  func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error)
	updateJob   func(stream proto.DRPCProvisionerDaemon_UpdateJobStream) error
	completeJob func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error)
}

func (p *provisionerdTestServer) AcquireJob(ctx context.Context, empty *proto.Empty) (*proto.AcquiredJob, error) {
	return p.acquireJob(ctx, empty)
}

func (p *provisionerdTestServer) UpdateJob(stream proto.DRPCProvisionerDaemon_UpdateJobStream) error {
	return p.updateJob(stream)
}

func (p *provisionerdTestServer) CompleteJob(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
	return p.completeJob(ctx, job)
}

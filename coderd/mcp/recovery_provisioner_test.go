package mcp_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionerd"
	daemonproto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// recoveryProvisioner exercises the actual worker failure protocol. Its first
// delete apply fails; subsequent applies succeed without editing database state.
type recoveryProvisioner struct {
	token   string
	deletes atomic.Int32
}

func (*recoveryProvisioner) Init(*provisionersdk.Session, *provisionersdk.InitRequest, <-chan struct{}) *proto.InitComplete {
	return &proto.InitComplete{}
}

func (*recoveryProvisioner) Parse(*provisionersdk.Session, *proto.ParseRequest, <-chan struct{}) *proto.ParseComplete {
	return &proto.ParseComplete{}
}

func (*recoveryProvisioner) Plan(*provisionersdk.Session, *proto.PlanRequest, <-chan struct{}) *proto.PlanComplete {
	return &proto.PlanComplete{}
}

func (p *recoveryProvisioner) Graph(_ *provisionersdk.Session, req *proto.GraphRequest, _ <-chan struct{}) *proto.GraphComplete {
	if req.GetMetadata().GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY {
		return &proto.GraphComplete{}
	}
	return echo.ProvisionGraphWithAgent(p.token)[0].GetGraph()
}

func (p *recoveryProvisioner) Apply(_ *provisionersdk.Session, req *proto.ApplyRequest, _ <-chan struct{}) *proto.ApplyComplete {
	if req.GetMetadata().GetWorkspaceTransition() == proto.WorkspaceTransition_DESTROY && p.deletes.Add(1) == 1 {
		return &proto.ApplyComplete{Error: "injected delete apply failure"}
	}
	return &proto.ApplyComplete{}
}

func startRecoveryProvisioner(t *testing.T, api *coderd.API, token string) *recoveryProvisioner {
	t.Helper()
	worker := &recoveryProvisioner{token: token}
	client, server := drpcsdk.MemTransportPipe()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); _ = client.Close(); _ = server.Close() })
	directory := t.TempDir()
	go func() {
		assert.NoError(t, provisionersdk.Serve(ctx, worker, &provisionersdk.ServeOptions{Listener: server, WorkDirectory: directory, Logger: testutil.Logger(t)}))
	}()
	connected := make(chan struct{})
	daemon := provisionerd.New(func(ctx context.Context) (daemonproto.DRPCProvisionerDaemonClient, error) {
		return api.CreateInMemoryTaggedProvisionerDaemon(ctx, "recovery", []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho}, nil)
	}, &provisionerd.Options{
		Logger: testutil.Logger(t), UpdateInterval: 250 * time.Millisecond, ForceCancelInterval: 5 * time.Second,
		Connector: provisionerd.LocalProvisioners{"echo": proto.NewDRPCProvisionerClient(client)}, InitConnectionCh: connected,
	})
	closer := coderdtest.NewProvisionerDaemonCloser(daemon)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })
	select {
	case <-connected:
	case <-testutil.Context(t, testutil.WaitLong).Done():
		t.Fatal("provisioner did not connect")
	}
	return worker
}

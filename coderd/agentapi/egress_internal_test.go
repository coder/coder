package agentapi

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"storj.io/drpc"
	"tailscale.com/tailcfg"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestBuildEgressConfig(t *testing.T) {
	t.Parallel()

	first := uuid.New()
	second := uuid.New()
	firstReplica := uuid.New()
	secondReplica := uuid.New()
	config := buildEgressConfig([]database.GetTemplateExitNodeReplicasRow{
		{ExitNodeID: first, Position: 0, ReplicaID: uuid.NullUUID{UUID: firstReplica, Valid: true}, WireguardEndpoints: []string{"203.0.113.1:41641"}},
		{ExitNodeID: first, Position: 0, ReplicaID: uuid.NullUUID{UUID: secondReplica, Valid: true}, WireguardEndpoints: []string{"203.0.113.2:41641"}},
		{ExitNodeID: second, Position: 1},
	}, database.Template{ExitNodeEnforce: true}, nil, nil)

	require.Equal(t, []*agentproto.EgressExitNode{
		{Id: first[:], ReplicaIds: [][]byte{firstReplica[:], secondReplica[:]}},
		{Id: second[:]},
	}, config.ExitNodes)
	require.Equal(t, int32(codersdk.ExitNodeTailnetPort), config.ExitNodePort)
	require.True(t, config.Enforce)
	require.Equal(t, []string{"udp/203.0.113.1:41641", "udp/203.0.113.2:41641"}, config.ControlPlaneHosts)
}

func TestStreamEgressConfigPubsub(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	controller := gomock.NewController(t)
	store := dbmock.NewMockStore(controller)
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { require.NoError(t, ps.Close()) })
	clock := quartz.NewMock(t)
	templateID := uuid.New()
	exitNodeID := uuid.New()
	firstReplica := uuid.New()
	secondReplica := uuid.New()
	workspaceID := uuid.New()

	store.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{TemplateID: templateID}, nil)
	gomock.InOrder(
		store.EXPECT().GetTemplateByID(gomock.Any(), templateID).Return(database.Template{}, nil),
		store.EXPECT().GetTemplateExitNodeReplicas(gomock.Any(), gomock.Any()).Return([]database.GetTemplateExitNodeReplicasRow{{
			ExitNodeID: exitNodeID,
			ReplicaID:  uuid.NullUUID{UUID: firstReplica, Valid: true},
		}}, nil),
		store.EXPECT().GetTemplateByID(gomock.Any(), templateID).Return(database.Template{}, nil),
		store.EXPECT().GetTemplateExitNodeReplicas(gomock.Any(), gomock.Any()).Return([]database.GetTemplateExitNodeReplicasRow{{
			ExitNodeID: exitNodeID,
			ReplicaID:  uuid.NullUUID{UUID: secondReplica, Valid: true},
		}}, nil),
	)

	api := &ManifestAPI{
		WorkspaceID: workspaceID,
		Database:    store,
		DerpMapFn:   func() *tailcfg.DERPMap { return nil },
		Clock:       clock,
		Pubsub:      ps,
	}
	stream := newEgressTestStream(ctx)
	done := make(chan error, 1)
	go func() {
		done <- api.StreamEgressConfig(&agentproto.StreamEgressConfigRequest{}, stream)
	}()

	first := <-stream.configs
	require.Equal(t, firstReplica[:], first.ExitNodes[0].ReplicaIds[0])
	require.NoError(t, ps.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(exitNodeID.String())))
	second := <-stream.configs
	require.Equal(t, secondReplica[:], second.ExitNodes[0].ReplicaIds[0])
	cancel()
	require.NoError(t, <-done)
}

type egressTestStream struct {
	ctx     context.Context
	configs chan *agentproto.EgressConfig
	mu      sync.Mutex
	closed  bool
}

func newEgressTestStream(ctx context.Context) *egressTestStream {
	return &egressTestStream{ctx: ctx, configs: make(chan *agentproto.EgressConfig, 2)}
}

func (s *egressTestStream) Send(config *agentproto.EgressConfig) error {
	s.configs <- config
	return nil
}
func (s *egressTestStream) Context() context.Context { return s.ctx }
func (s *egressTestStream) MsgSend(msg drpc.Message, _ drpc.Encoding) error {
	s.configs <- msg.(*agentproto.EgressConfig)
	return nil
}
func (*egressTestStream) MsgRecv(drpc.Message, drpc.Encoding) error { panic("not used") }
func (*egressTestStream) CloseSend() error                          { return nil }
func (s *egressTestStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

package tailnet_test

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/database/pubsub"
	"github.com/coder/coder/enterprise/tailnet"
	agpl "github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

func TestCoordinatorSingle(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub.NewInMemory(), emptyDerpMapFn)
		require.NoError(t, err)
		defer coordinator.Close()

		client, server := net.Pipe()
		sendNode, errChan := agpl.ServeCoordinator(client, func(update agpl.CoordinatorNodeUpdate) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(server, id, uuid.New())
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&agpl.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		err = client.Close()
		require.NoError(t, err)
		<-errChan
		<-closeChan
	})

	t.Run("AgentWithoutClients", func(t *testing.T) {
		t.Parallel()
		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub.NewInMemory(), emptyDerpMapFn)
		require.NoError(t, err)
		defer coordinator.Close()

		client, server := net.Pipe()
		sendNode, errChan := agpl.ServeCoordinator(client, func(update agpl.CoordinatorNodeUpdate) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(server, id, "")
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&agpl.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		err = client.Close()
		require.NoError(t, err)
		<-errChan
		<-closeChan
	})

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()

		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub.NewInMemory(), emptyDerpMapFn)
		require.NoError(t, err)
		defer coordinator.Close()

		agentWS, agentServerWS := net.Pipe()
		defer agentWS.Close()
		agentNodeChan := make(chan []*agpl.Node)
		sendAgentNode, agentErrChan := agpl.ServeCoordinator(agentWS, func(update agpl.CoordinatorNodeUpdate) error {
			agentNodeChan <- update.Nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&agpl.Node{PreferredDERP: 1})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*agpl.Node)
		sendClientNode, clientErrChan := agpl.ServeCoordinator(clientWS, func(update agpl.CoordinatorNodeUpdate) error {
			clientNodeChan <- update.Nodes
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		agentNodes := <-clientNodeChan
		require.Len(t, agentNodes, 1)
		sendClientNode(&agpl.Node{PreferredDERP: 2})
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&agpl.Node{PreferredDERP: 3})
		agentNodes = <-clientNodeChan
		require.Len(t, agentNodes, 1)

		// Close the agent WebSocket so a new one can connect.
		err = agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		// Create a new agent connection. This is to simulate a reconnect!
		agentWS, agentServerWS = net.Pipe()
		defer agentWS.Close()
		agentNodeChan = make(chan []*agpl.Node)
		_, agentErrChan = agpl.ServeCoordinator(agentWS, func(update agpl.CoordinatorNodeUpdate) error {
			agentNodeChan <- update.Nodes
			return nil
		})
		closeAgentChan = make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		// Ensure the existing listening client sends it's node immediately!
		clientNodes = <-agentNodeChan
		require.Len(t, clientNodes, 1)

		err = agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		err = clientWS.Close()
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan
	})

	t.Run("SendsDERPMap", func(t *testing.T) {
		t.Parallel()

		derpMapFn := func() *tailcfg.DERPMap {
			return &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					1: {
						RegionID: 1,
						Nodes: []*tailcfg.DERPNode{
							{
								Name:     "derp1",
								RegionID: 1,
								HostName: "derp1.example.com",
								// blah
							},
						},
					},
				},
			}
		}

		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub.NewInMemory(), derpMapFn)
		require.NoError(t, err)
		defer coordinator.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()
		agentWS, agentServerWS := net.Pipe()
		defer agentWS.Close()
		agentUpdateChan := make(chan agpl.CoordinatorNodeUpdate)
		sendAgentNode, agentErrChan := agpl.ServeCoordinator(agentWS, func(update agpl.CoordinatorNodeUpdate) error {
			agentUpdateChan <- update
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&agpl.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientUpdateChan := make(chan agpl.CoordinatorNodeUpdate)
		sendClientNode, clientErrChan := agpl.ServeCoordinator(clientWS, func(update agpl.CoordinatorNodeUpdate) error {
			clientUpdateChan <- update
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		select {
		case clientUpdate := <-clientUpdateChan:
			require.Equal(t, derpMapFn(), clientUpdate.DERPMap)
			require.Len(t, clientUpdate.Nodes, 1)
		case <-ctx.Done():
			t.Fatal("timed out")
		}
		sendClientNode(&agpl.Node{})
		agentUpdate := <-agentUpdateChan
		require.Equal(t, derpMapFn(), agentUpdate.DERPMap)
		require.Len(t, agentUpdate.Nodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&agpl.Node{})
		select {
		case clientUpdate := <-clientUpdateChan:
			require.Equal(t, derpMapFn(), clientUpdate.DERPMap)
			require.Len(t, clientUpdate.Nodes, 1)
		case <-ctx.Done():
			t.Fatal("timed out")
		}

		err = agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		err = clientWS.Close()
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan
	})
}

func TestCoordinatorHA(t *testing.T) {
	t.Parallel()

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()

		_, pubsub := dbtestutil.NewDB(t)

		coordinator1, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub, emptyDerpMapFn)
		require.NoError(t, err)
		defer coordinator1.Close()

		agentWS, agentServerWS := net.Pipe()
		defer agentWS.Close()
		agentNodeChan := make(chan []*agpl.Node)
		sendAgentNode, agentErrChan := agpl.ServeCoordinator(agentWS, func(update agpl.CoordinatorNodeUpdate) error {
			agentNodeChan <- update.Nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator1.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&agpl.Node{PreferredDERP: 1})
		require.Eventually(t, func() bool {
			return coordinator1.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		coordinator2, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub, emptyDerpMapFn)
		require.NoError(t, err)
		defer coordinator2.Close()

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*agpl.Node)
		sendClientNode, clientErrChan := agpl.ServeCoordinator(clientWS, func(update agpl.CoordinatorNodeUpdate) error {
			clientNodeChan <- update.Nodes
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator2.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		agentNodes := <-clientNodeChan
		require.Len(t, agentNodes, 1)
		sendClientNode(&agpl.Node{PreferredDERP: 2})
		_ = sendClientNode
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&agpl.Node{PreferredDERP: 3})
		agentNodes = <-clientNodeChan
		require.Len(t, agentNodes, 1)

		// Close the agent WebSocket so a new one can connect.
		require.NoError(t, agentWS.Close())
		require.NoError(t, agentServerWS.Close())
		<-agentErrChan
		<-closeAgentChan

		// Create a new agent connection. This is to simulate a reconnect!
		agentWS, agentServerWS = net.Pipe()
		defer agentWS.Close()
		agentNodeChan = make(chan []*agpl.Node)
		_, agentErrChan = agpl.ServeCoordinator(agentWS, func(update agpl.CoordinatorNodeUpdate) error {
			agentNodeChan <- update.Nodes
			return nil
		})
		closeAgentChan = make(chan struct{})
		go func() {
			err := coordinator1.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		// Ensure the existing listening client sends it's node immediately!
		clientNodes = <-agentNodeChan
		require.Len(t, clientNodes, 1)

		err = agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		err = clientWS.Close()
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan
	})
}

func emptyDerpMapFn() *tailcfg.DERPMap {
	return &tailcfg.DERPMap{}
}

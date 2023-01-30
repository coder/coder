package tailnet_test

import (
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/enterprise/tailnet"
	agpl "github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

func TestCoordinatorSingle(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), database.NewPubsubInMemory())
		require.NoError(t, err)
		defer coordinator.Close()

		client, server := net.Pipe()
		sendNode, errChan := agpl.ServeCoordinator(client, func(node []*agpl.Node) error {
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
		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), database.NewPubsubInMemory())
		require.NoError(t, err)
		defer coordinator.Close()

		client, server := net.Pipe()
		sendNode, errChan := agpl.ServeCoordinator(client, func(node []*agpl.Node) error {
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

		coordinator, err := tailnet.NewCoordinator(slogtest.Make(t, nil), database.NewPubsubInMemory())
		require.NoError(t, err)
		defer coordinator.Close()

		agentWS, agentServerWS := net.Pipe()
		defer agentWS.Close()
		agentNodeChan := make(chan []*agpl.Node)
		sendAgentNode, agentErrChan := agpl.ServeCoordinator(agentWS, func(nodes []*agpl.Node) error {
			agentNodeChan <- nodes
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
		clientNodeChan := make(chan []*agpl.Node)
		sendClientNode, clientErrChan := agpl.ServeCoordinator(clientWS, func(nodes []*agpl.Node) error {
			clientNodeChan <- nodes
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
		sendClientNode(&agpl.Node{})
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&agpl.Node{})
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
		_, agentErrChan = agpl.ServeCoordinator(agentWS, func(nodes []*agpl.Node) error {
			agentNodeChan <- nodes
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
}

func TestCoordinatorHA(t *testing.T) {
	t.Parallel()

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()

		_, pubsub := dbtestutil.NewDB(t)

		coordinator1, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub)
		require.NoError(t, err)
		defer coordinator1.Close()

		agentWS, agentServerWS := net.Pipe()
		defer agentWS.Close()
		agentNodeChan := make(chan []*agpl.Node)
		sendAgentNode, agentErrChan := agpl.ServeCoordinator(agentWS, func(nodes []*agpl.Node) error {
			agentNodeChan <- nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator1.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&agpl.Node{})
		require.Eventually(t, func() bool {
			return coordinator1.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		coordinator2, err := tailnet.NewCoordinator(slogtest.Make(t, nil), pubsub)
		require.NoError(t, err)
		defer coordinator2.Close()

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*agpl.Node)
		sendClientNode, clientErrChan := agpl.ServeCoordinator(clientWS, func(nodes []*agpl.Node) error {
			clientNodeChan <- nodes
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
		sendClientNode(&agpl.Node{})
		_ = sendClientNode
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&agpl.Node{})
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
		_, agentErrChan = agpl.ServeCoordinator(agentWS, func(nodes []*agpl.Node) error {
			agentNodeChan <- nodes
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

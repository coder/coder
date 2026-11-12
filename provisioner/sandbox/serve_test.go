package sandbox_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/agentsandbox/agentsandboxtest"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisioner/sandbox"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

// session drives one provisioner session the way a provisioner daemon does.
type session struct {
	t      *testing.T
	stream proto.DRPCProvisioner_SessionClient
}

func (s *session) send(req *proto.Request) *proto.Response {
	s.t.Helper()
	require.NoError(s.t, s.stream.Send(req))
	for {
		res, err := s.stream.Recv()
		require.NoError(s.t, err)
		if res.GetLog() == nil {
			return res
		}
	}
}

func TestSandboxProvisioner(t *testing.T) {
	t.Parallel()
	cluster := agentsandboxtest.NewCluster(t)

	client, server := drpcsdk.MemTransportPipe()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitMedium))
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancel()
	})
	go func() {
		err := sandbox.Serve(ctx, &sandbox.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{Listener: server, WorkDirectory: t.TempDir()},
			Kube:         cluster.Kube,
			Namespace:    cluster.Namespace,
		})
		assert.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(client)
	archive := testutil.CreateTar(t, map[string]string{"sandbox.yaml": "warm_pool: pool\n"})
	workspaceID := uuid.NewString()
	metadata := func(transition proto.WorkspaceTransition) *proto.Metadata {
		return &proto.Metadata{WorkspaceId: workspaceID, WorkspaceTransition: transition}
	}
	open := func(templateArchive []byte) (*session, *proto.InitComplete) {
		stream, err := api.Session(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = stream.Close() })
		s := &session{t: t, stream: stream}
		require.NoError(t, stream.Send(&proto.Request{Type: &proto.Request_Config{Config: &proto.Config{}}}))
		init := s.send(&proto.Request{Type: &proto.Request_Init{Init: &proto.InitRequest{TemplateSourceArchive: templateArchive}}})
		return s, init.GetInit()
	}

	// A template without a pool is refused when it is imported.
	_, init := open(testutil.CreateTar(t, map[string]string{"sandbox.yaml": "os: linux\n"}))
	require.Contains(t, init.GetError(), "warm_pool")

	// Importing a template claims nothing. It says what a build would make.
	s, init := open(archive)
	require.Empty(t, init.GetError())
	require.Empty(t, s.send(&proto.Request{Type: &proto.Request_Parse{Parse: &proto.ParseRequest{}}}).GetParse().GetError())
	require.Empty(t, s.send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
		Metadata: metadata(proto.WorkspaceTransition_START),
	}}}).GetPlan().GetError())
	planned := s.send(&proto.Request{Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
		Metadata: metadata(proto.WorkspaceTransition_START), Source: proto.GraphSource_SOURCE_PLAN,
	}}}).GetGraph()
	require.Empty(t, planned.GetError())
	require.Len(t, planned.GetResources(), 1)
	require.Len(t, planned.GetResources()[0].GetAgents(), 1)
	require.Empty(t, planned.GetResources()[0].GetAgents()[0].GetInstanceId())
	_, claimed := cluster.ClaimedPod("coder-" + workspaceID)
	require.False(t, claimed)

	// Starting a workspace claims a pod. The daemon in it is whoever the
	// cluster says that pod is, and never gets its owner's credentials.
	s, _ = open(archive)
	require.Empty(t, s.send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
		Metadata: metadata(proto.WorkspaceTransition_START),
	}}}).GetPlan().GetError())
	applied := s.send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{
		Metadata: metadata(proto.WorkspaceTransition_START),
	}}}).GetApply()
	require.Empty(t, applied.GetError())
	pod, claimed := cluster.ClaimedPod("coder-" + workspaceID)
	require.True(t, claimed)
	var state map[string]any
	require.NoError(t, json.Unmarshal(applied.GetState(), &state))
	require.Equal(t, pod.UID, state["pod_uid"])
	require.Equal(t, cluster.Namespace, state["namespace"])
	built := s.send(&proto.Request{Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
		Metadata: metadata(proto.WorkspaceTransition_START), Source: proto.GraphSource_SOURCE_STATE,
	}}}).GetGraph()
	require.Empty(t, built.GetError())
	require.Len(t, built.GetResources(), 1)
	agent := built.GetResources()[0].GetAgents()[0]
	require.Equal(t, "k8s-pod:"+pod.UID, agent.GetInstanceId())
	require.Equal(t, "no_user_data", agent.GetApiKeyScope())
	require.Empty(t, agent.GetToken())

	// Deleting the workspace gives the pod up and leaves nothing.
	s, _ = open(archive)
	require.Empty(t, s.send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
		Metadata: metadata(proto.WorkspaceTransition_DESTROY), State: applied.GetState(),
	}}}).GetPlan().GetError())
	destroyed := s.send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{
		Metadata: metadata(proto.WorkspaceTransition_DESTROY),
	}}}).GetApply()
	require.Empty(t, destroyed.GetError())
	require.Empty(t, destroyed.GetState())
	_, claimed = cluster.ClaimedPod("coder-" + workspaceID)
	require.False(t, claimed)
	gone := s.send(&proto.Request{Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
		Metadata: metadata(proto.WorkspaceTransition_DESTROY), Source: proto.GraphSource_SOURCE_STATE,
	}}}).GetGraph()
	require.Empty(t, gone.GetError())
	require.Empty(t, gone.GetResources())
}

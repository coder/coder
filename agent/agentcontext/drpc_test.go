package agentcontext_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcerr"

	"github.com/coder/coder/v2/agent/agentcontext"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

// fakeDRPCClient stubs out the DRPCAgentClient210 surface for
// the parts of the interface the adapter exercises. Only
// PushContextState is implemented; every other method panics
// because the adapter never calls them.
type fakeDRPCClient struct {
	agentproto.DRPCAgentClient210
	lastReq *agentproto.PushContextStateRequest
	resp    *agentproto.PushContextStateResponse
	err     error
}

func (f *fakeDRPCClient) PushContextState(_ context.Context, req *agentproto.PushContextStateRequest) (*agentproto.PushContextStateResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	if f.resp == nil {
		return &agentproto.PushContextStateResponse{Accepted: true}, nil
	}
	return f.resp, nil
}

func TestDRPCPusher_HappyPathSerializesAllFields(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{}
	pusher := agentcontext.NewDRPCPusher(client)

	req := &agentcontext.PushRequest{
		Version:       7,
		AggregateHash: [32]byte{0xaa, 0xbb, 0xcc},
		Initial:       true,
		SchemaVersion: 1,
		SnapshotError: "watcher degraded",
		Resources: []agentcontext.Resource{
			{
				ID:          "instruction_file:/tmp/AGENTS.md",
				Kind:        agentcontext.KindInstructionFile,
				Source:      "/tmp/AGENTS.md",
				ContentHash: [32]byte{0x01, 0x02},
				Payload:     []byte("body"),
				SizeBytes:   4,
				Status:      agentcontext.StatusOK,
				Description: "tagline",
				SourcePath:  "/tmp",
			},
			{
				ID:        "skill:/tmp/.agents/skills/foo",
				Kind:      agentcontext.KindSkill,
				Source:    "/tmp/.agents/skills/foo",
				Status:    agentcontext.StatusInvalid,
				Error:     "bad frontmatter",
				SizeBytes: 99,
			},
		},
	}

	resp, err := pusher.PushContextState(context.Background(), req)
	require.NoError(t, err)
	require.True(t, resp.Accepted)

	pb := client.lastReq
	require.NotNil(t, pb)
	require.Equal(t, uint64(7), pb.Version)
	require.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, pb.AggregateHash)
	require.True(t, pb.Initial)
	require.Equal(t, uint64(1), pb.SchemaVersion)
	require.Equal(t, "watcher degraded", pb.SnapshotError)

	require.Len(t, pb.Resources, 2)
	instr := pb.Resources[0]
	require.Equal(t, "instruction_file:/tmp/AGENTS.md", instr.Id)
	require.Equal(t, agentproto.ContextResource_INSTRUCTION_FILE, instr.Kind)
	require.Equal(t, agentproto.ContextResource_OK, instr.Status)
	require.Equal(t, []byte("body"), instr.Payload)
	require.Equal(t, "tagline", instr.Description)
	require.NotNil(t, instr.SourcePath)
	require.Equal(t, "/tmp", *instr.SourcePath)

	skill := pb.Resources[1]
	require.Equal(t, agentproto.ContextResource_SKILL, skill.Kind)
	require.Equal(t, agentproto.ContextResource_INVALID, skill.Status)
	require.Nil(t, skill.SourcePath, "empty user source must remain optional/nil")
}

func TestDRPCPusher_UnimplementedTranslated(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{err: drpcerr.WithCode(drpcerr.WithCode(context.Canceled, 0), drpcerr.Unimplemented)}
	pusher := agentcontext.NewDRPCPusher(client)

	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{})
	require.ErrorIs(t, err, agentcontext.ErrPushUnimplemented)
}

func TestDRPCPusher_PropagatesOtherErrors(t *testing.T) {
	t.Parallel()
	want := drpcerr.WithCode(context.DeadlineExceeded, 42)
	client := &fakeDRPCClient{err: want}
	pusher := agentcontext.NewDRPCPusher(client)

	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDRPCPusher_NilClientErrors(t *testing.T) {
	t.Parallel()
	pusher := agentcontext.NewDRPCPusher(nil)
	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{})
	require.Error(t, err)
}

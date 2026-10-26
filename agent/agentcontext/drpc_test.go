package agentcontext_test

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc/drpcerr"

	"github.com/coder/coder/v2/agent/agentcontext"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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
			{
				ID:          "skill:/tmp/.agents/skills/code-review",
				Kind:        agentcontext.KindSkill,
				Source:      "/tmp/.agents/skills/code-review",
				ContentHash: [32]byte{0x03},
				Payload:     []byte("---\nname: code-review\n---\nbody\n"),
				SizeBytes:   31,
				Status:      agentcontext.StatusOK,
				Name:        "code-review",
				Description: "Critical review for Go PRs.",
				SourcePath:  "/tmp",
			},
			{
				ID:          "mcp_config:/tmp/.mcp.json",
				Kind:        agentcontext.KindMCPConfig,
				Source:      "/tmp/.mcp.json",
				ContentHash: [32]byte{0x04},
				SizeBytes:   412,
				Status:      agentcontext.StatusOK,
				SourcePath:  "/tmp",
			},
			{
				ID:          "mcp_server:github",
				Kind:        agentcontext.KindMCPServer,
				Source:      "github",
				Name:        "github",
				ContentHash: [32]byte{0x05},
				SizeBytes:   138,
				Status:      agentcontext.StatusOK,
				Description: "GitHub MCP server (1 tool)",
				SourcePath:  "/tmp/.mcp.json",
				Tools: []agentcontext.MCPTool{{
					Name:        "create_issue",
					Description: "Create a GitHub issue",
					InputSchema: map[string]any{
						"type":     "object",
						"required": []any{"title"},
					},
				}},
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
	require.Equal(t, "watcher degraded", pb.SnapshotError)

	require.Len(t, pb.Resources, 5)

	// Instruction file: wire-flat fields plus typed body.
	instr := pb.Resources[0]
	require.Equal(t, "/tmp/AGENTS.md", instr.Source)
	require.Equal(t, agentproto.ContextResource_OK, instr.Status)
	require.NotNil(t, instr.SourcePath)
	require.Equal(t, "/tmp", *instr.SourcePath)
	instrBody := instr.GetInstructionFile()
	require.NotNil(t, instrBody, "instruction_file body must be set")
	require.Equal(t, []byte("body"), instrBody.GetContent())
	require.Nil(t, instr.GetSkill())
	require.Nil(t, instr.GetMcpConfig())
	require.Nil(t, instr.GetMcpServer())

	// Skill with INVALID status still has the skill body set so
	// coderd can attribute the failure to the correct kind.
	invalidSkill := pb.Resources[1]
	require.Equal(t, agentproto.ContextResource_INVALID, invalidSkill.Status)
	require.Equal(t, "bad frontmatter", invalidSkill.Error)
	require.NotNil(t, invalidSkill.GetSkill(), "skill body must be set even when status != OK")
	require.Nil(t, invalidSkill.SourcePath, "empty user source must remain optional/nil")

	// OK skill: meta + name + description populated.
	skill := pb.Resources[2]
	skillBody := skill.GetSkill()
	require.NotNil(t, skillBody)
	require.Equal(t, []byte("---\nname: code-review\n---\nbody\n"), skillBody.GetMeta())
	require.Equal(t, "code-review", skillBody.GetName())
	require.Equal(t, "Critical review for Go PRs.", skillBody.GetDescription())

	// MCP config: body present but empty. SizeBytes / ContentHash
	// on the outer resource still detect changes.
	mcpCfg := pb.Resources[3]
	require.Equal(t, uint64(412), mcpCfg.SizeBytes)
	require.NotNil(t, mcpCfg.GetMcpConfig(), "mcp_config body must be set")

	// MCP server: structured tool list with input schema.
	mcpSrv := pb.Resources[4]
	srvBody := mcpSrv.GetMcpServer()
	require.NotNil(t, srvBody)
	require.Equal(t, "github", srvBody.GetServerName())
	require.Equal(t, "GitHub MCP server (1 tool)", srvBody.GetDescription())
	require.Len(t, srvBody.GetTools(), 1)
	tool := srvBody.GetTools()[0]
	require.Equal(t, "create_issue", tool.GetName())
	require.Equal(t, "Create a GitHub issue", tool.GetDescription())
	require.NotNil(t, tool.GetInputSchema(), "input_schema must be set when supplied")
	require.Equal(t, "object", tool.GetInputSchema().GetFields()["type"].GetStringValue())
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

// pluginPushRequest returns a request carrying every plugin-related
// field the wire can express.
func pluginPushRequest() *agentcontext.PushRequest {
	return &agentcontext.PushRequest{
		Version:       3,
		AggregateHash: [32]byte{0xaa},
		Resources: []agentcontext.Resource{
			{
				ID:            "plugin:/tmp/plugins/p",
				Kind:          agentcontext.KindPlugin,
				Source:        "/tmp/plugins/p",
				Name:          "p",
				PluginVersion: "1.0.0",
				Description:   "A plugin",
				Status:        agentcontext.StatusOK,
				ContentHash:   [32]byte{0x07},
				SizeBytes:     42,
			},
			{
				ID:          "skill:/tmp/plugins/p/skills/s",
				Kind:        agentcontext.KindSkill,
				Source:      "/tmp/plugins/p/skills/s",
				Name:        "s",
				PluginName:  "p",
				Description: "Plugin skill",
				Payload:     []byte("---\nname: s\n---\n"),
				Status:      agentcontext.StatusOK,
			},
			{
				ID:         "mcp_server:p/srv",
				Kind:       agentcontext.KindMCPServer,
				Source:     "p/srv",
				Name:       "srv",
				PluginName: "p",
				Status:     agentcontext.StatusOK,
				Tools:      []agentcontext.MCPTool{{Name: "echo"}},
			},
			{
				ID:     "skill:/tmp/skills/plain",
				Kind:   agentcontext.KindSkill,
				Source: "/tmp/skills/plain",
				Name:   "plain",
				Status: agentcontext.StatusOK,
			},
		},
	}
}

func TestDRPCPusher_NestsPluginResourcesInTheirPlugin(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{}
	pusher := agentcontext.NewDRPCPusher(client)

	req := pluginPushRequest()
	shadowedPlugin := agentcontext.Resource{
		ID:     "plugin:/tmp/shadowed/p",
		Kind:   agentcontext.KindPlugin,
		Source: "/tmp/shadowed/p",
		Name:   "p",
		Status: agentcontext.StatusExcluded,
	}
	namesake := agentcontext.Resource{
		ID:     "skill:/tmp/skills/p",
		Kind:   agentcontext.KindSkill,
		Source: "/tmp/skills/p",
		Name:   "p",
		Status: agentcontext.StatusOK,
	}
	orphan := agentcontext.Resource{
		ID:         "skill:/tmp/plugins/gone/skills/s",
		Kind:       agentcontext.KindSkill,
		Source:     "/tmp/plugins/gone/skills/s",
		Name:       "s",
		PluginName: "gone",
		Status:     agentcontext.StatusOK,
	}
	req.Resources = append(req.Resources, shadowedPlugin, namesake, orphan)
	_, err := pusher.PushContextState(context.Background(), req)
	require.NoError(t, err)

	pb := client.lastReq
	require.Len(t, pb.Resources, 4, "plugin resources must be nested, and orphans dropped")

	plugin := pb.Resources[0]
	require.Equal(t, "/tmp/plugins/p", plugin.Source)
	require.Equal(t, agentproto.ContextResource_OK, plugin.Status)
	require.Equal(t, uint64(42), plugin.SizeBytes)
	body := plugin.GetPlugin()
	require.NotNil(t, body, "plugin body must be set")
	require.Equal(t, "p", body.GetName())
	require.Equal(t, "1.0.0", body.GetVersion())
	require.Equal(t, "A plugin", body.GetDescription())

	require.Len(t, body.GetSkills(), 1)
	skill := body.GetSkills()[0]
	require.Equal(t, "/tmp/plugins/p/skills/s", skill.Source)
	require.Equal(t, "s", skill.GetSkill().GetName())

	require.Len(t, body.GetMcpServers(), 1)
	srv := body.GetMcpServers()[0]
	require.Equal(t, "p/srv", srv.Source)
	require.Equal(t, "srv", srv.GetMcpServer().GetServerName())

	require.Equal(t, "/tmp/skills/plain", pb.Resources[1].Source)
	require.NotNil(t, pb.Resources[1].GetSkill())

	shadowed := pb.Resources[2]
	require.Equal(t, "/tmp/shadowed/p", shadowed.Source)
	require.Empty(t, shadowed.GetPlugin().GetSkills())
	require.Empty(t, shadowed.GetPlugin().GetMcpServers())

	require.Equal(t, "/tmp/skills/p", pb.Resources[3].Source)
	require.NotNil(t, pb.Resources[3].GetSkill())
}

// coderd rejects a push whose resource error exceeds 4096 bytes, so the
// pusher truncates every Error on a UTF-8 boundary.
func TestDRPCPusher_TruncatesResourceError(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{}
	pusher := agentcontext.NewDRPCPusher(client)

	long := "a" + strings.Repeat("é", 3000)
	exact := strings.Repeat("x", workspacesdk.MaxContextErrorBytes)
	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{
		Resources: []agentcontext.Resource{
			{ID: "instruction_file:/tmp/AGENTS.md", Kind: agentcontext.KindInstructionFile, Source: "/tmp/AGENTS.md", Status: agentcontext.StatusInvalid, Error: long},
			{ID: "instruction_file:/tmp/CLAUDE.md", Kind: agentcontext.KindInstructionFile, Source: "/tmp/CLAUDE.md", Status: agentcontext.StatusInvalid, Error: "short"},
			{ID: "instruction_file:/tmp/.cursorrules", Kind: agentcontext.KindInstructionFile, Source: "/tmp/.cursorrules", Status: agentcontext.StatusInvalid, Error: exact},
		},
	})
	require.NoError(t, err)

	got := client.lastReq.Resources[0].Error
	require.Len(t, got, 4095, "a two-byte rune straddling the cap is dropped")
	require.True(t, utf8.ValidString(got))
	require.True(t, strings.HasPrefix(long, got))
	require.Equal(t, "short", client.lastReq.Resources[1].Error)
	require.Equal(t, exact, client.lastReq.Resources[2].Error, "an error of exactly the cap is kept")
}

// proto3 string fields must be valid UTF-8, so a resource error with
// invalid UTF-8 must not fail the push.
func TestDRPCPusher_ResourceErrorInvalidUTF8(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{}
	pusher := agentcontext.NewDRPCPusher(client)
	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{Resources: []agentcontext.Resource{{
		ID:     "plugin:/tmp/x",
		Kind:   agentcontext.KindPlugin,
		Source: "/tmp/x",
		Status: agentcontext.StatusInvalid,
		Error:  "bad \xff\xfe bytes",
	}}})
	require.NoError(t, err)

	got := client.lastReq.Resources[0].Error
	require.True(t, utf8.ValidString(got))
	require.Equal(t, "bad \uFFFD bytes", got)
	_, err = proto.Marshal(client.lastReq)
	require.NoError(t, err)
}

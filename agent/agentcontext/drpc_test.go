package agentcontext_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentcontext"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeDRPCClient stubs out the DRPCAgentClient210 surface for
// the parts of the interface the adapter exercises. Only
// PushContextState is implemented; every other method panics
// because the adapter never calls them.
type fakeDRPCClient struct {
	agentproto.DRPCAgentClient210
	lastReq *agentproto.PushContextStateRequest
	reqs    []*agentproto.PushContextStateRequest
	resp    *agentproto.PushContextStateResponse
	err     error
	// reject, when set and err is nil, fails the requests it returns
	// an error for.
	reject func(*agentproto.PushContextStateRequest) error
}

func (f *fakeDRPCClient) PushContextState(_ context.Context, req *agentproto.PushContextStateRequest) (*agentproto.PushContextStateResponse, error) {
	f.lastReq = req
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.reject != nil {
		if err := f.reject(req); err != nil {
			return nil, err
		}
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
	orphan := agentcontext.Resource{
		ID:         "skill:/tmp/plugins/gone/skills/s",
		Kind:       agentcontext.KindSkill,
		Source:     "/tmp/plugins/gone/skills/s",
		Name:       "s",
		PluginName: "gone",
		Status:     agentcontext.StatusOK,
	}
	req.Resources = append(append([]agentcontext.Resource{shadowedPlugin}, req.Resources...), orphan)
	_, err := pusher.PushContextState(context.Background(), req)
	require.NoError(t, err)

	pb := client.lastReq
	require.Len(t, pb.Resources, 3, "plugin resources must be nested, and orphans dropped")

	shadowed := pb.Resources[0]
	require.Equal(t, "/tmp/shadowed/p", shadowed.Source)
	require.Empty(t, shadowed.GetPlugin().GetSkills())
	require.Empty(t, shadowed.GetPlugin().GetMcpServers())

	plugin := pb.Resources[1]
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
	require.Empty(t, skill.GetSkill().GetPluginName())

	require.Len(t, body.GetMcpServers(), 1)
	srv := body.GetMcpServers()[0]
	require.Equal(t, "p/srv", srv.Source)
	require.Equal(t, "srv", srv.GetMcpServer().GetServerName())
	require.Empty(t, srv.GetMcpServer().GetPluginName())

	require.Equal(t, "/tmp/skills/plain", pb.Resources[2].Source)
	require.NotNil(t, pb.Resources[2].GetSkill())
}

const pluginNotice = "Agent Plugins could not be loaded. Check the workspace agent logs for details."

func hasPluginData(req *agentproto.PushContextStateRequest) bool {
	return slices.ContainsFunc(req.GetResources(), func(r *agentproto.ContextResource) bool {
		return r.GetPlugin() != nil
	})
}

// rejectPluginData fails every request that carries plugin data.
func rejectPluginData(req *agentproto.PushContextStateRequest) error {
	if hasPluginData(req) {
		return xerrors.New("unknown resource body")
	}
	return nil
}

// requireStripped asserts that pb is pluginPushRequest without its plugin
// data, with the aggregate hash recomputed over what was sent.
func requireStripped(t *testing.T, pb *agentproto.PushContextStateRequest) {
	t.Helper()
	require.Len(t, pb.Resources, 1, "plugin and plugin-attributed resources must be dropped")
	require.Equal(t, "/tmp/skills/plain", pb.Resources[0].Source)
	req := pluginPushRequest()
	sent := slices.DeleteFunc(req.Resources, func(r agentcontext.Resource) bool {
		return r.Kind == agentcontext.KindPlugin || r.PluginName != ""
	})
	want := agentcontext.ComputeAggregateHash(agentcontext.HashedResources(sent))
	require.Equal(t, want[:], pb.AggregateHash)
}

func TestDRPCPusher_RejectedPluginDataRetriedWithout(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{reject: rejectPluginData}
	sink := testutil.NewFakeSink(t)
	pusher := agentcontext.NewDRPCPusher(client)
	pusher.Logger = slog.Make(sink)

	resp, err := pusher.PushContextState(context.Background(), pluginPushRequest())
	require.NoError(t, err)
	require.True(t, resp.Accepted)
	require.Len(t, client.reqs, 2)
	require.True(t, hasPluginData(client.reqs[0]))
	requireStripped(t, client.reqs[1])
	require.Equal(t, pluginNotice, client.reqs[1].SnapshotError)

	// Later pushes skip the full request.
	_, err = pusher.PushContextState(context.Background(), pluginPushRequest())
	require.NoError(t, err)
	require.Len(t, client.reqs, 3)
	requireStripped(t, client.reqs[2])
	require.Equal(t, pluginNotice, client.reqs[2].SnapshotError)

	warnings := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelWarn })
	require.Len(t, warnings, 1)
}

func TestDRPCPusher_RejectedPushNotRetried(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		req  *agentcontext.PushRequest
		err  error
	}{
		{
			name: "NoPluginData",
			req: &agentcontext.PushRequest{Resources: []agentcontext.Resource{
				{ID: "skill:/tmp/skills/plain", Kind: agentcontext.KindSkill, Source: "/tmp/skills/plain", Name: "plain", Status: agentcontext.StatusOK},
			}},
			err: xerrors.New("rejected"),
		},
		{
			name: "Canceled",
			req:  pluginPushRequest(),
			err:  xerrors.Errorf("push: %w", context.Canceled),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeDRPCClient{err: tc.err}
			pusher := agentcontext.NewDRPCPusher(client)

			_, err := pusher.PushContextState(context.Background(), tc.req)
			require.ErrorIs(t, err, tc.err)
			require.Len(t, client.reqs, 1)
		})
	}
}

func TestDRPCPusher_FailedRetryReturnsOriginalError(t *testing.T) {
	t.Parallel()
	original := xerrors.New("original")
	client := &fakeDRPCClient{}
	client.reject = func(req *agentproto.PushContextStateRequest) error {
		if hasPluginData(req) {
			return original
		}
		return xerrors.New("retry")
	}
	pusher := agentcontext.NewDRPCPusher(client)

	_, err := pusher.PushContextState(context.Background(), pluginPushRequest())
	require.ErrorIs(t, err, original)
	require.Len(t, client.reqs, 2)
	require.False(t, hasPluginData(client.reqs[1]))

	// The failed retry does not stop the next push from sending
	// plugin data.
	client.reject = nil
	_, err = pusher.PushContextState(context.Background(), pluginPushRequest())
	require.NoError(t, err)
	require.Len(t, client.reqs, 3)
	require.True(t, hasPluginData(client.reqs[2]))
	require.Empty(t, client.reqs[2].SnapshotError)
}

func TestDRPCPusher_UnimplementedWithPluginDataNotRetried(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{err: drpcerr.WithCode(xerrors.New("unknown rpc"), drpcerr.Unimplemented)}
	pusher := agentcontext.NewDRPCPusher(client)

	_, err := pusher.PushContextState(context.Background(), pluginPushRequest())
	require.ErrorIs(t, err, agentcontext.ErrPushUnimplemented)
	require.Len(t, client.reqs, 1)
}

func TestDRPCPusher_PluginNoticeJoinsSnapshotError(t *testing.T) {
	t.Parallel()

	t.Run("Joined", func(t *testing.T) {
		t.Parallel()
		client := &fakeDRPCClient{reject: rejectPluginData}
		pusher := agentcontext.NewDRPCPusher(client)
		req := pluginPushRequest()
		req.SnapshotError = "watcher degraded"

		_, err := pusher.PushContextState(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, "watcher degraded; "+pluginNotice, client.lastReq.SnapshotError)
	})

	t.Run("Truncated", func(t *testing.T) {
		t.Parallel()
		client := &fakeDRPCClient{reject: rejectPluginData}
		pusher := agentcontext.NewDRPCPusher(client)
		req := pluginPushRequest()
		req.SnapshotError = strings.Repeat("é", workspacesdk.MaxContextErrorBytes)

		_, err := pusher.PushContextState(context.Background(), req)
		require.NoError(t, err)
		got := client.lastReq.SnapshotError
		require.LessOrEqual(t, len(got), workspacesdk.MaxContextErrorBytes)
		require.True(t, utf8.ValidString(got))
	})
}

func TestDRPCPusher_NewPusherSendsPluginData(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{reject: rejectPluginData}
	_, err := agentcontext.NewDRPCPusher(client).PushContextState(context.Background(), pluginPushRequest())
	require.NoError(t, err)
	require.False(t, hasPluginData(client.lastReq))

	client.reject = nil
	_, err = agentcontext.NewDRPCPusher(client).PushContextState(context.Background(), pluginPushRequest())
	require.NoError(t, err)
	require.True(t, hasPluginData(client.lastReq))
}

// coderd rejects a push whose resource error exceeds 4096 bytes, so the
// pusher truncates every Error on a UTF-8 boundary.
func TestDRPCPusher_TruncatesResourceError(t *testing.T) {
	t.Parallel()
	client := &fakeDRPCClient{}
	pusher := agentcontext.NewDRPCPusher(client)

	long := "a" + strings.Repeat("é", 3000)
	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{
		Resources: []agentcontext.Resource{
			{ID: "instruction_file:/tmp/AGENTS.md", Kind: agentcontext.KindInstructionFile, Source: "/tmp/AGENTS.md", Status: agentcontext.StatusInvalid, Error: long},
			{ID: "instruction_file:/tmp/CLAUDE.md", Kind: agentcontext.KindInstructionFile, Source: "/tmp/CLAUDE.md", Status: agentcontext.StatusInvalid, Error: "short"},
		},
	})
	require.NoError(t, err)

	got := client.lastReq.Resources[0].Error
	require.Len(t, got, 4095, "a two-byte rune straddling the cap is dropped")
	require.True(t, utf8.ValidString(got))
	require.True(t, strings.HasPrefix(long, got))
	require.Equal(t, "short", client.lastReq.Resources[1].Error)
}

// proto3 string fields must be valid UTF-8, so a resource error built
// from file content with invalid UTF-8 must not fail the push.
func TestDRPCPusher_ResourceErrorInvalidUTF8(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(root, ".agents", "plugins", "x", "plugin.json"), "{\"$schema\": \"\xff\", \"name\": \"x\"}")
	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})
	require.Len(t, snap.Resources, 1)
	require.False(t, utf8.ValidString(snap.Resources[0].Error), "the fixture must produce an invalid UTF-8 error")

	client := &fakeDRPCClient{}
	pusher := agentcontext.NewDRPCPusher(client)
	_, err := pusher.PushContextState(context.Background(), &agentcontext.PushRequest{Resources: snap.Resources})
	require.NoError(t, err)

	got := client.lastReq.Resources[0].Error
	require.True(t, utf8.ValidString(got))
	require.Contains(t, got, "is not an Agent Plugins schema")
	_, err = proto.Marshal(client.lastReq)
	require.NoError(t, err)
}

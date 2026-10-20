package agentapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestPushContextState(t *testing.T) {
	t.Parallel()

	now := dbtime.Time(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	agentID := uuid.New()
	clock := quartz.NewMock(t)
	clock.Set(now)

	makeAPI := func(t *testing.T) (*agentapi.ContextAPI, *dbmock.MockStore) {
		t.Helper()
		ctrl := gomock.NewController(t)
		dbm := dbmock.NewMockStore(ctrl)
		return &agentapi.ContextAPI{
			AgentID:  agentID,
			Log:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
			Clock:    clock,
			Database: dbm,
		}, dbm
	}

	// expectInTx wires the dbmock so InTx invokes the closure on the
	// same mock; tests then set per-method expectations on the same
	// dbm. The push transaction must run at repeatable read isolation
	// so concurrent pushes cannot clobber each other.
	expectInTx := func(dbm *dbmock.MockStore) {
		dbm.EXPECT().InTx(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
			func(f func(database.Store) error, opts *database.TxOptions) error {
				require.NotNil(t, opts)
				require.Equal(t, sql.LevelRepeatableRead, opts.Isolation)
				return f(dbm)
			},
		)
	}

	t.Run("DisabledReturnsUnimplemented", func(t *testing.T) {
		t.Parallel()

		// No InTx or query expectations: a disabled push must return
		// before touching the store. The Unimplemented dRPC code is
		// load-bearing; the agent's DRPCPusher translates it into
		// ErrPushUnimplemented, which stops its RunPush loop instead
		// of retrying with backoff.
		api, _ := makeAPI(t)
		api.Disabled = true

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version:       1,
			AggregateHash: []byte{0x01, 0x02},
			Initial:       true,
		})
		require.Error(t, err)
		require.Nil(t, resp)
		require.EqualValues(t, drpcerr.Unimplemented, drpcerr.Code(err))
	})

	t.Run("AcceptsInitialPush", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextResource{}, nil).Times(2)
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), database.DeleteStaleWorkspaceAgentContextResourcesParams{
			WorkspaceAgentID: agentID,
			ActiveSources:    []string{"/home/coder/.mcp.json", "/home/coder/AGENTS.md"},
		}).Return(nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version:       1,
			AggregateHash: []byte{0x01, 0x02, 0x03},
			Initial:       true,
			Resources: []*agentproto.ContextResource{
				instructionResource("/home/coder/AGENTS.md", "hello"),
				mcpConfigResource("/home/coder/.mcp.json"),
			},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	})

	t.Run("DirtyMarkerInvokedAfterCommit", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		marker := &fakeDirtyMarker{}
		api.DirtyMarker = marker
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextResource{}, nil).Times(1)
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), gomock.Any()).
			Return(nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version:       1,
			AggregateHash: []byte{0xaa, 0xbb},
			SnapshotError: "watcher degraded",
			Initial:       true,
			Resources: []*agentproto.ContextResource{
				instructionResource("/home/coder/AGENTS.md", "hello"),
			},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
		// The marker runs inside the push transaction and its returned
		// callback publishes only after the transaction commits.
		require.Equal(t, 1, marker.called)
		require.Equal(t, 1, marker.published)
		require.Equal(t, agentID, marker.gotAgent)
		require.Equal(t, []byte{0xaa, 0xbb}, marker.gotHash)
		require.Equal(t, "watcher degraded", marker.gotErr)
	})

	t.Run("DirtyMarkerSkippedOnDrop", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		marker := &fakeDirtyMarker{}
		api.DirtyMarker = marker
		expectInTx(dbm)

		// A non-initial push at a version not strictly greater than the
		// stored one is dropped before any write; hydration and the
		// dirty fan-out must not run.
		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{Version: 5}, nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version:       2,
			AggregateHash: []byte{0x01},
			Resources: []*agentproto.ContextResource{
				instructionResource("/home/coder/AGENTS.md", "hello"),
			},
		})
		require.NoError(t, err)
		require.False(t, resp.GetAccepted())
		require.Equal(t, 0, marker.called)
		require.Equal(t, 0, marker.published)
	})

	t.Run("RejectsEmptyAndDuplicateSources", func(t *testing.T) {
		t.Parallel()

		t.Run("Empty", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version: 1,
				Initial: true,
				Resources: []*agentproto.ContextResource{
					instructionResource("", "x"),
				},
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "empty source")
		})

		t.Run("Duplicate", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version: 1,
				Initial: true,
				Resources: []*agentproto.ContextResource{
					instructionResource("/a", "x"),
					instructionResource("/a", "y"),
				},
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "duplicate source")
		})
	})

	t.Run("RejectsUnknownStatus", func(t *testing.T) {
		t.Parallel()

		api, _ := makeAPI(t)
		// STATUS_UNSPECIFIED is the zero value and must be rejected so
		// every persisted row has a meaningful status.
		resource := instructionResource("/a", "x")
		resource.Status = agentproto.ContextResource_STATUS_UNSPECIFIED
		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version:   1,
			Initial:   true,
			Resources: []*agentproto.ContextResource{resource},
		})
		require.Error(t, err)
		require.Nil(t, resp)
	})

	t.Run("RejectsMissingBody", func(t *testing.T) {
		t.Parallel()

		api, _ := makeAPI(t)
		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
			Resources: []*agentproto.ContextResource{
				{
					Source:      "/a",
					ContentHash: []byte{0x01},
					Status:      agentproto.ContextResource_OK,
					// Body deliberately unset.
				},
			},
		})
		require.Error(t, err)
		require.Nil(t, resp)
		require.Contains(t, err.Error(), "missing body")
	})

	t.Run("StaleVersionDropped", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		// Existing version 5 stored; incoming version 3 with initial=false
		// is a replay/out-of-order push and must be silently dropped
		// (accepted=false) without writing.
		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{Version: 5}, nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 3,
			Initial: false,
			Resources: []*agentproto.ContextResource{
				instructionResource("/a", "stale"),
			},
		})
		require.NoError(t, err)
		require.False(t, resp.GetAccepted())
	})

	t.Run("SameVersionReplayDropped", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{Version: 5}, nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 5,
			Initial: false,
		})
		require.NoError(t, err)
		require.False(t, resp.GetAccepted())
	})

	t.Run("InitialOverwritesLowerVersion", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		// Agent rebooted: in-memory counter back to 1 but the stored
		// version from the previous process boot is 5. initial=true is
		// authoritative and the push is accepted.
		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{Version: 5}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextResource{}, nil)
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), gomock.Any()).
			Return(nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
			Resources: []*agentproto.ContextResource{
				instructionResource("/a", "fresh"),
			},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	})

	t.Run("PrunesStaleResources", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{Version: 1}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextResource{}, nil)
		// Even with one active resource the prune call still runs so
		// any resource not in the active set is removed in the same
		// transaction.
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), database.DeleteStaleWorkspaceAgentContextResourcesParams{
			WorkspaceAgentID: agentID,
			ActiveSources:    []string{"/a"},
		}).Return(nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 2,
			Initial: false,
			Resources: []*agentproto.ContextResource{
				instructionResource("/a", "still here"),
			},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	})

	t.Run("EmptyResourceListAcceptedAndPrunesAll", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)
		// Active sources is an explicitly empty slice (not nil) so the
		// generated SQL deletes every row for this agent rather than
		// no-oping on a NULL array.
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), database.DeleteStaleWorkspaceAgentContextResourcesParams{
			WorkspaceAgentID: agentID,
			ActiveSources:    []string{},
		}).Return(nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
	})

	t.Run("PersistsAllKnownBodyVariants", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		api.Experiments = codersdk.Experiments{codersdk.ExperimentAgentPlugins}
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)

		gotKinds := map[database.WorkspaceAgentContextBodyKind][]byte{}
		gotSources := map[string]database.UpsertWorkspaceAgentContextResourceParams{}
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Times(7).
			DoAndReturn(func(_ context.Context, arg database.UpsertWorkspaceAgentContextResourceParams) (database.WorkspaceAgentContextResource, error) {
				gotKinds[arg.BodyKind] = arg.Body
				gotSources[arg.Source] = arg
				return database.WorkspaceAgentContextResource{}, nil
			})

		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), gomock.Any()).Return(nil)

		mcpServer := mcpServerResource("/srv/mcp/echo", "echo", "echo server")
		plugin := nestServers(
			nestSkills(pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
				skillResource("/a/.agents/plugins/acme/skills/deploy", "deploy", "deploys")),
			mcpServerResource("acme/echo", "echo", "plugin echo"))
		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
			Resources: []*agentproto.ContextResource{
				instructionResource("/a/AGENTS.md", "hi"),
				skillResource("/a/.agents/skills/example/SKILL.md", "example", "an example"),
				mcpConfigResource("/a/.mcp.json"),
				mcpServer,
				plugin,
			},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())

		require.Contains(t, gotSources, "acme/echo")
		require.Equal(t, database.WorkspaceAgentContextBodyKindMcpServer, gotSources["acme/echo"].BodyKind)
		var pluginServerBody map[string]any
		require.NoError(t, json.Unmarshal(gotSources["acme/echo"].Body, &pluginServerBody))
		require.Equal(t, "acme", pluginServerBody["plugin_name"])
		require.Equal(t, "echo", pluginServerBody["server_name"])

		const pluginSkillSource = "/a/.agents/plugins/acme/skills/deploy"
		require.Contains(t, gotSources, pluginSkillSource)
		require.Equal(t, database.WorkspaceAgentContextBodyKindSkill, gotSources[pluginSkillSource].BodyKind)
		var pluginSkillBody map[string]any
		require.NoError(t, json.Unmarshal(gotSources[pluginSkillSource].Body, &pluginSkillBody))
		require.Equal(t, "acme", pluginSkillBody["plugin_name"])
		require.Empty(t, plugin.GetPlugin().GetSkills()[0].GetSkill().GetPluginName(), "the request must not be modified")

		require.Contains(t, gotKinds, database.WorkspaceAgentContextBodyKindInstructionFile)
		require.Contains(t, gotKinds, database.WorkspaceAgentContextBodyKindSkill)
		require.Contains(t, gotKinds, database.WorkspaceAgentContextBodyKindMcpConfig)
		require.Contains(t, gotKinds, database.WorkspaceAgentContextBodyKindMcpServer)
		require.Contains(t, gotKinds, database.WorkspaceAgentContextBodyKindPlugin)
		require.JSONEq(t, `{"name":"acme","version":"1.0.0"}`, string(gotKinds[database.WorkspaceAgentContextBodyKindPlugin]))

		// Confirm each body deserializes as JSON; the actual proto
		// roundtrip is exercised by the resolver tests on the agent
		// side. We just sanity-check the encoding here.
		for kind, body := range gotKinds {
			var raw map[string]any
			err := json.Unmarshal(body, &raw)
			require.NoErrorf(t, err, "kind %q body not valid JSON: %s", kind, string(body))
		}
	})

	t.Run("DropsPluginDataWithoutExperiment", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)

		var gotSources []string
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Times(2).
			DoAndReturn(func(_ context.Context, arg database.UpsertWorkspaceAgentContextResourceParams) (database.WorkspaceAgentContextResource, error) {
				gotSources = append(gotSources, arg.Source)
				return database.WorkspaceAgentContextResource{}, nil
			})
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), database.DeleteStaleWorkspaceAgentContextResourcesParams{
			WorkspaceAgentID: agentID,
			ActiveSources:    []string{"/a/.agents/skills/example/SKILL.md", "/srv/mcp/echo"},
		}).Return(nil)

		plugin := nestServers(
			nestSkills(pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
				skillResource("/a/.agents/plugins/acme/skills/deploy", "deploy", "deploys")),
			mcpServerResource("acme/echo", "echo", "plugin echo"))
		namelessPlugin := pluginResource("/a/.agents/plugins/broken", "", "")
		namelessPlugin.Status = agentproto.ContextResource_INVALID
		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
			Resources: []*agentproto.ContextResource{
				skillResource("/a/.agents/skills/example/SKILL.md", "example", "an example"),
				mcpServerResource("/srv/mcp/echo", "echo", "echo server"),
				plugin,
				namelessPlugin,
			},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
		require.Equal(t, []string{"/a/.agents/skills/example/SKILL.md", "/srv/mcp/echo"}, gotSources)
	})

	t.Run("DroppedPluginDataKeepsRequestIndexes", func(t *testing.T) {
		t.Parallel()

		api, _ := makeAPI(t)
		_, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
			Resources: []*agentproto.ContextResource{
				pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
				skillResource("", "x", "x"),
			},
		})
		require.ErrorContains(t, err, "resource at index 1 has empty source")
	})

	t.Run("RejectsInvalidPluginFields", func(t *testing.T) {
		t.Parallel()

		const acmeRoot = "/a/.agents/plugins/acme"
		const winRoot = `C:\Users\u\.agents\plugins\winacme`
		acme := func() *agentproto.ContextResource { return pluginResource(acmeRoot, "acme", "1.0.0") }
		echo := func() *agentproto.ContextResource { return mcpServerResource("acme/echo", "echo", "plugin echo") }
		broken := pluginResource("/b/.agents/plugins/broken", "broken", "1.0.0")
		broken.Status = agentproto.ContextResource_INVALID
		// hugeLen exceeds every per-field string cap but not the
		// per-resource body cap, so the plugin checks reject it. An error
		// that quotes huge exceeds maxErrLen.
		const (
			hugeLen   = 200 * 1024
			maxErrLen = 4096
		)
		huge := strings.Repeat("a", hugeLen)
		for name, tc := range map[string]struct {
			resources []*agentproto.ContextResource
			err       string
		}{
			"PluginInvalidName":     {resources: []*agentproto.ContextResource{pluginResource("/z/.agents/plugins/bad", "acme..x", "1.0.0")}, err: "plugin name"},
			"PluginNameTooLong":     {resources: []*agentproto.ContextResource{pluginResource("/z/.agents/plugins/long", huge, "1.0.0")}, err: fmt.Sprintf("plugin name is %d bytes", hugeLen)},
			"OKPluginNoName":        {resources: []*agentproto.ContextResource{pluginResource("/z/.agents/plugins/noname", "", "1.0.0")}, err: "plugin name is required on an OK plugin resource"},
			"DuplicateOKPluginName": {resources: []*agentproto.ContextResource{pluginResource("/b/.agents/plugins/acme", "acme", "2.0.0"), acme()}, err: `OK plugin name "acme" is used by both "/b/.agents/plugins/acme" and "/a/.agents/plugins/acme"`},
			"SkillPluginName":       {resources: []*agentproto.ContextResource{acme(), withSkillPlugin(skillResource(acmeRoot+"/skills/x", "x", "x"), "acme")}, err: "plugin_name must be empty"},
			"MCPPluginName":         {resources: []*agentproto.ContextResource{acme(), withServerPlugin(echo(), "acme")}, err: "plugin_name must be empty"},
			"NestedSkillPluginName": {resources: []*agentproto.ContextResource{nestSkills(acme(), withSkillPlugin(skillResource(acmeRoot+"/skills/x", "x", "x"), "acme"))}, err: "plugin_name must be empty"},
			"NestedInNonOKPlugin":   {resources: []*agentproto.ContextResource{nestSkills(broken, skillResource("/b/.agents/plugins/broken/skills/x", "x", "x"))}, err: "a plugin whose status is not OK must not nest resources"},
			"NestedNilSkill":        {resources: []*agentproto.ContextResource{nestSkills(acme(), nil)}, err: "skills[0] must carry a skill body"},
			"NestedServerAsSkill":   {resources: []*agentproto.ContextResource{nestSkills(acme(), echo())}, err: "skills[0] must carry a skill body"},
			"NestedPlugin":          {resources: []*agentproto.ContextResource{nestSkills(acme(), pluginResource(acmeRoot+"/skills/inner", "inner", "1.0.0"))}, err: "skills[0] must carry a skill body"},
			"NestedSkillAsServer":   {resources: []*agentproto.ContextResource{nestServers(acme(), skillResource(acmeRoot+"/skills/x", "x", "x"))}, err: "mcp_servers[0] must carry an mcp_server body"},
			"NestedEmptySource":     {resources: []*agentproto.ContextResource{echo(), nestSkills(acme(), skillResource("", "x", "x"))}, err: "resource at index 1 skills[0] has empty source"},
			"NestedOversizedSource": {resources: []*agentproto.ContextResource{nestServers(acme(), mcpServerResource("acme/"+huge, huge, "plugin echo"))}, err: fmt.Sprintf("resource at index 0 mcp_servers[0] has %d byte source", hugeLen+len("acme/"))},
			"NestedDuplicateSource": {resources: []*agentproto.ContextResource{nestServers(acme(), echo(), echo())}, err: `duplicate source "acme/echo"`},
			// A workspace server can take the source of a plugin server.
			"NestedSourceOfWorkspaceServer":    {resources: []*agentproto.ContextResource{mcpServerResource("acme/echo", "acme/echo", "workspace"), nestServers(acme(), echo())}, err: `duplicate source "acme/echo"`},
			"MCPSourceFormat":                  {resources: []*agentproto.ContextResource{nestServers(acme(), mcpServerResource("echo", "echo", "plugin echo"))}, err: `plugin MCP server source must be "acme/<server_name>"`},
			"MCPSourceFormatLongServerName":    {resources: []*agentproto.ContextResource{nestServers(acme(), mcpServerResource("acme/echo", huge, "plugin echo"))}, err: `plugin MCP server source must be "acme/<server_name>"`},
			"MCPEmptyServerName":               {resources: []*agentproto.ContextResource{nestServers(acme(), mcpServerResource("acme/", "", "plugin echo"))}, err: "server_name is required"},
			"SkillOutsidePlugin":               {resources: []*agentproto.ContextResource{nestSkills(acme(), skillResource("/home/u/.agents/skills/x", "x", "x"))}, err: `plugin skill source must be under "/a/.agents/plugins/acme/skills/"`},
			"SkillUnderOtherPluginRoot":        {resources: []*agentproto.ContextResource{nestSkills(acme(), skillResource("/b/.agents/plugins/acme/skills/x", "x", "x"))}, err: `plugin skill source must be under "/a/.agents/plugins/acme/skills/"`},
			"SkillUnderRootNotSkills":          {resources: []*agentproto.ContextResource{nestSkills(acme(), skillResource(acmeRoot+"/other/x", "x", "x"))}, err: `plugin skill source must be under "/a/.agents/plugins/acme/skills/"`},
			"SkillUnderBackslashRootNotSkills": {resources: []*agentproto.ContextResource{nestSkills(pluginResource(winRoot, "winacme", "1.0.0"), skillResource(winRoot+`\docs\x`, "x", "x"))}, err: "plugin skill source must be under"},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				api, _ := makeAPI(t)
				api.Experiments = codersdk.Experiments{codersdk.ExperimentAgentPlugins}
				_, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
					Version:   1,
					Initial:   true,
					Resources: tc.resources,
				})
				require.ErrorContains(t, err, tc.err)
				require.Less(t, len(err.Error()), maxErrLen, "error must not quote uncapped input")
			})
		}
	})

	t.Run("RejectsPluginNameWithoutExperiment", func(t *testing.T) {
		t.Parallel()

		api, _ := makeAPI(t)
		_, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 1,
			Initial: true,
			Resources: []*agentproto.ContextResource{
				withSkillPlugin(skillResource("/a/.agents/plugins/acme/skills/x", "x", "x"), "acme"),
			},
		})
		require.ErrorContains(t, err, "plugin_name must be empty")
	})

	t.Run("AcceptsValidPluginData", func(t *testing.T) {
		t.Parallel()

		const winRoot = `C:\Users\u\.agents\plugins\acme`
		nameless := pluginResource("/a/.agents/plugins/broken", "", "")
		nameless.Status = agentproto.ContextResource_INVALID
		nameless.Error = "name is required"
		shadowed := pluginResource("/b/.agents/plugins/acme", "acme", "2.0.0")
		shadowed.Status = agentproto.ContextResource_EXCLUDED
		shadowed.Error = "shadowed by /a/.agents/plugins/acme"
		for name, resources := range map[string][]*agentproto.ContextResource{
			"NonOKPluginWithoutName": {nameless},
			"PluginSkillWithBackslashSource": {
				nestSkills(pluginResource(winRoot, "acme", "1.0.0"), skillResource(winRoot+`\skills\deploy`, "deploy", "deploys")),
			},
			"NonOKPluginSharingOKPluginName": {
				pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
				shadowed,
			},
			"NestedResourcesAfterTheirPlugin": {
				nestServers(
					nestSkills(pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
						skillResource("/a/.agents/plugins/acme/skills/deploy", "deploy", "deploys")),
					mcpServerResource("acme/echo", "echo", "plugin echo")),
				skillResource("/a/.agents/skills/plain", "plain", "plain"),
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				var wantSources []string
				for _, r := range resources {
					wantSources = append(wantSources, r.Source)
					for _, nested := range slices.Concat(r.GetPlugin().GetSkills(), r.GetPlugin().GetMcpServers()) {
						wantSources = append(wantSources, nested.Source)
					}
				}

				api, dbm := makeAPI(t)
				api.Experiments = codersdk.Experiments{codersdk.ExperimentAgentPlugins}
				expectInTx(dbm)

				dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
					Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
				dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
					Return(database.WorkspaceAgentContextSnapshot{}, nil)
				var gotSources []string
				dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
					Times(len(wantSources)).
					DoAndReturn(func(_ context.Context, arg database.UpsertWorkspaceAgentContextResourceParams) (database.WorkspaceAgentContextResource, error) {
						gotSources = append(gotSources, arg.Source)
						return database.WorkspaceAgentContextResource{}, nil
					})
				dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), gomock.Any()).Return(nil)

				resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
					Version:   1,
					Initial:   true,
					Resources: resources,
				})
				require.NoError(t, err)
				require.True(t, resp.GetAccepted())
				require.Equal(t, wantSources, gotSources)
			})
		}
	})

	t.Run("SourceErrorsPrecedeDuplicateOKPluginName", func(t *testing.T) {
		t.Parallel()

		for name, tc := range map[string]struct {
			resources []*agentproto.ContextResource
			err       string
		}{
			"DuplicateSource": {
				resources: []*agentproto.ContextResource{
					pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
					pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
				},
				err: "duplicate source",
			},
			"EmptySource": {
				resources: []*agentproto.ContextResource{
					pluginResource("/a/.agents/plugins/acme", "acme", "1.0.0"),
					pluginResource("", "acme", "2.0.0"),
				},
				err: "resource at index 1 has empty source",
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				api, _ := makeAPI(t)
				api.Experiments = codersdk.Experiments{codersdk.ExperimentAgentPlugins}
				_, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
					Version:   1,
					Initial:   true,
					Resources: tc.resources,
				})
				require.ErrorContains(t, err, tc.err)
			})
		}
	})

	t.Run("NonOKStatusStillPersisted", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)
		expectInTx(dbm)

		dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, errNoRows())
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)

		var got database.UpsertWorkspaceAgentContextResourceParams
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, arg database.UpsertWorkspaceAgentContextResourceParams) (database.WorkspaceAgentContextResource, error) {
				got = arg
				return database.WorkspaceAgentContextResource{}, nil
			})
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), gomock.Any()).Return(nil)

		oversized := instructionResource("/a/AGENTS.md", "")
		oversized.Status = agentproto.ContextResource_OVERSIZE
		oversized.SizeBytes = 65 * 1024
		oversized.Error = "file exceeds 64KiB per-resource cap"

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version:   1,
			Initial:   true,
			Resources: []*agentproto.ContextResource{oversized},
		})
		require.NoError(t, err)
		require.True(t, resp.GetAccepted())
		require.Equal(t, database.WorkspaceAgentContextBodyKindInstructionFile, got.BodyKind)
		require.Equal(t, database.WorkspaceAgentContextResourceStatusOversize, got.Status)
		require.Equal(t, int64(65*1024), got.SizeBytes)
		require.Equal(t, "file exceeds 64KiB per-resource cap", got.Error)
	})

	t.Run("SerializationConflictRetries", func(t *testing.T) {
		t.Parallel()

		api, dbm := makeAPI(t)

		// First attempt: the closure runs fully but the commit fails
		// with a serialization error because a concurrent push won the
		// race. Second attempt: the re-read gate sees the winner's
		// committed version and drops this push. The response must
		// report accepted=false even though the first attempt reached
		// the accepting branch before rolling back.
		gomock.InOrder(
			dbm.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(f func(database.Store) error, opts *database.TxOptions) error {
					require.Equal(t, sql.LevelRepeatableRead, opts.Isolation)
					err := f(dbm)
					require.NoError(t, err)
					return &pq.Error{Code: "40001"}
				},
			),
			dbm.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(f func(database.Store) error, _ *database.TxOptions) error {
					return f(dbm)
				},
			),
		)
		gomock.InOrder(
			dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
				Return(database.WorkspaceAgentContextSnapshot{}, errNoRows()),
			dbm.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
				Return(database.WorkspaceAgentContextSnapshot{Version: 7}, nil),
		)
		dbm.EXPECT().UpsertWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextSnapshot{}, nil)
		dbm.EXPECT().UpsertWorkspaceAgentContextResource(gomock.Any(), gomock.Any()).
			Return(database.WorkspaceAgentContextResource{}, nil)
		dbm.EXPECT().DeleteStaleWorkspaceAgentContextResources(gomock.Any(), gomock.Any()).Return(nil)

		resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
			Version: 6,
			Initial: false,
			Resources: []*agentproto.ContextResource{
				instructionResource("/a", "racy"),
			},
		})
		require.NoError(t, err)
		require.False(t, resp.GetAccepted())
	})

	t.Run("ServerSideLimits", func(t *testing.T) {
		t.Parallel()

		// All limit violations fail validation before the transaction
		// starts, so no database expectations are needed.
		t.Run("TooManyResources", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resources := make([]*agentproto.ContextResource, 0, 1001)
			for i := 0; i < 1001; i++ {
				resources = append(resources, instructionResource("/r/"+string(rune('a'+i%26))+"/"+uuid.NewString(), "x"))
			}
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version:   1,
				Initial:   true,
				Resources: resources,
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "resource cap")
		})

		t.Run("TooManyNestedResources", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			plugin := pluginResource("/p/acme", "acme", "1.0.0")
			for i := 0; i < 1000; i++ {
				plugin = nestSkills(plugin, skillResource(fmt.Sprintf("/p/acme/skills/s%d", i), "s", "s"))
			}
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version:   1,
				Initial:   true,
				Resources: []*agentproto.ContextResource{plugin},
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "has 1001 resources, exceeds 1000 resource cap")
		})

		t.Run("VersionOverflowsInt64", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version: uint64(math.MaxInt64) + 1,
				Initial: true,
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "int64 range")
		})

		t.Run("SourceTooLong", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version: 1,
				Initial: true,
				Resources: []*agentproto.ContextResource{
					instructionResource("/"+strings.Repeat("a", 1024), "x"),
				},
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "byte cap")
		})

		t.Run("BodyTooLarge", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			// 256KiB of content base64-expands past the 256KiB body cap.
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version: 1,
				Initial: true,
				Resources: []*agentproto.ContextResource{
					instructionResource("/big", strings.Repeat("x", 256*1024)),
				},
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "byte cap")
		})

		t.Run("AggregateTooLarge", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			// 25 resources just under the per-resource cap together
			// exceed the 4MiB aggregate cap.
			content := strings.Repeat("x", 140*1024)
			resources := make([]*agentproto.ContextResource, 0, 25)
			for i := 0; i < 25; i++ {
				resources = append(resources, instructionResource("/agg/"+uuid.NewString(), content))
			}
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version:   1,
				Initial:   true,
				Resources: resources,
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "aggregate body size")
		})

		t.Run("ContentHashTooLong", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resource := instructionResource("/a", "x")
			resource.ContentHash = make([]byte, 65)
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version:   1,
				Initial:   true,
				Resources: []*agentproto.ContextResource{resource},
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "byte cap")
		})

		t.Run("SnapshotErrorTooLong", func(t *testing.T) {
			t.Parallel()
			api, _ := makeAPI(t)
			resp, err := api.PushContextState(context.Background(), &agentproto.PushContextStateRequest{
				Version:       1,
				Initial:       true,
				SnapshotError: strings.Repeat("e", 4097),
			})
			require.Error(t, err)
			require.Nil(t, resp)
			require.Contains(t, err.Error(), "byte cap")
		})
	})
}

// errNoRows returns the database "no rows" sentinel for the mocks;
// the handler uses errors.Is(err, sql.ErrNoRows) to recognize first
// pushes vs. updates.
func errNoRows() error {
	return sql.ErrNoRows
}

func instructionResource(source, content string) *agentproto.ContextResource {
	return &agentproto.ContextResource{
		Source:      source,
		ContentHash: []byte{0xaa, 0xbb, 0xcc},
		Status:      agentproto.ContextResource_OK,
		SizeBytes:   uint64(len(content)),
		Body: &agentproto.ContextResource_InstructionFile{
			InstructionFile: &agentproto.InstructionFileBody{
				Content: []byte(content),
			},
		},
	}
}

func skillResource(source, name, description string) *agentproto.ContextResource {
	return &agentproto.ContextResource{
		Source:      source,
		ContentHash: []byte{0x01, 0x02, 0x03},
		Status:      agentproto.ContextResource_OK,
		Body: &agentproto.ContextResource_Skill{
			Skill: &agentproto.SkillMetaBody{
				Meta:        []byte("---\nname: " + name + "\n---\nbody"),
				Name:        name,
				Description: description,
			},
		},
	}
}

func mcpConfigResource(source string) *agentproto.ContextResource {
	return &agentproto.ContextResource{
		Source:      source,
		ContentHash: []byte{0xde, 0xad, 0xbe, 0xef},
		Status:      agentproto.ContextResource_OK,
		Body: &agentproto.ContextResource_McpConfig{
			McpConfig: &agentproto.MCPConfigBody{},
		},
	}
}

func mcpServerResource(source, serverName, description string) *agentproto.ContextResource {
	return &agentproto.ContextResource{
		Source:      source,
		ContentHash: []byte{0x10, 0x20, 0x30},
		Status:      agentproto.ContextResource_OK,
		Body: &agentproto.ContextResource_McpServer{
			McpServer: &agentproto.MCPServerBody{
				ServerName:  serverName,
				Description: description,
			},
		},
	}
}

func pluginResource(source, name, version string) *agentproto.ContextResource {
	return &agentproto.ContextResource{
		Source:      source,
		ContentHash: []byte{0x40, 0x50, 0x60},
		Status:      agentproto.ContextResource_OK,
		Body: &agentproto.ContextResource_Plugin{
			Plugin: &agentproto.PluginBody{
				Name:    name,
				Version: version,
			},
		},
	}
}

func withSkillPlugin(r *agentproto.ContextResource, name string) *agentproto.ContextResource {
	r.GetSkill().PluginName = name
	return r
}

func withServerPlugin(r *agentproto.ContextResource, name string) *agentproto.ContextResource {
	r.GetMcpServer().PluginName = name
	return r
}

func nestSkills(plugin *agentproto.ContextResource, skills ...*agentproto.ContextResource) *agentproto.ContextResource {
	plugin.GetPlugin().Skills = append(plugin.GetPlugin().Skills, skills...)
	return plugin
}

func nestServers(plugin *agentproto.ContextResource, servers ...*agentproto.ContextResource) *agentproto.ContextResource {
	plugin.GetPlugin().McpServers = append(plugin.GetPlugin().McpServers, servers...)
	return plugin
}

// fakeDirtyMarker is a test double for agentapi.ContextDirtyMarker. It records
// the in-transaction call and counts callback invocations so tests can assert
// the marker runs inside the push transaction and publishes only after commit.
type fakeDirtyMarker struct {
	called    int
	published int
	gotAgent  uuid.UUID
	gotHash   []byte
	gotErr    string
}

func (f *fakeDirtyMarker) HydrateAndMarkChatsDirty(_ context.Context, _ database.Store, agentID uuid.UUID, aggregateHash []byte, snapshotError string, _ time.Time) (func(), error) {
	f.called++
	f.gotAgent = agentID
	f.gotHash = aggregateHash
	f.gotErr = snapshotError
	return func() { f.published++ }, nil
}

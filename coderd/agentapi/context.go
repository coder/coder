package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Server-side caps on a single PushContextState request. The agent
// enforces its own caps (64KiB per resource payload, 2MiB aggregate,
// 500 resources; see agent/agentcontext/resolve.go), but coderd
// cannot trust a workspace process, so pushes are re-validated here
// with headroom above the agent caps:
//
//   - workspacesdk.MaxContextResources allows excluded stub entries
//     past the agent's 500-resource cap.
//   - maxContextResourceBodyBytes covers protojson and base64
//     expansion of a 64KiB payload.
//   - maxContextAggregateBodyBytes matches the 4MiB DRPC message
//     cap so the invariant survives transport changes.
//   - The string and hash caps bound the remaining row columns;
//     source doubles as a btree primary key column, which PostgreSQL
//     limits to roughly 2704 bytes per index entry.
const (
	maxContextResourceBodyBytes  = 256 * 1024
	maxContextAggregateBodyBytes = 4 * 1024 * 1024
	maxContextHashBytes          = 64
)

// ContextAPI implements the v2.10 PushContextState RPC. It persists
// the latest pushed snapshot per workspace agent across two tables
// (workspace_agent_context_snapshots and
// workspace_agent_context_resources) so later phases can hydrate
// chats and surface drift to the dashboard.
//
// The handler is a pure write path: nothing else in coderd reads
// these rows yet. If a bug here returns errors the agent's RunPush
// loop backs off and the workspace keeps behaving exactly like it
// did before v2.10.
type ContextAPI struct {
	AgentID uuid.UUID
	// Workspace caches workspace fields for the duration of the agent
	// connection so dbauthz can authorize against the workspace RBAC
	// object without re-fetching the workspace on every push.
	Workspace *CachedWorkspaceFields
	Log       slog.Logger
	Clock     quartz.Clock
	Database  database.Store
	// DirtyMarker hydrates chats from, and marks chats dirty against, the
	// snapshot persisted by a push. It is nil when chatd is not running,
	// in which case PushContextState stays a pure write path.
	DirtyMarker ContextDirtyMarker
	// Disabled rejects every push with a dRPC Unimplemented code. The
	// agent's DRPCPusher translates that code into ErrPushUnimplemented,
	// which terminates its RunPush loop for the life of the connection,
	// exactly as if coderd predated the v2.10 Agent API. This is the
	// deployment-wide kill switch for context sync write load
	// (CODER_DISABLE_WORKSPACE_AGENT_CONTEXT_SYNC).
	Disabled bool
	// Experiments gates Agent Plugins data. With agent-plugins disabled,
	// plugin data is dropped but the aggregate hash is stored as sent.
	Experiments codersdk.Experiments
}

// ContextDirtyMarker hydrates chats from, and marks chats dirty against, a
// freshly persisted agent context snapshot. It is implemented by chatd and
// injected at coderd construction so this package neither imports the chat
// domain nor performs chat-authorized writes directly.
type ContextDirtyMarker interface {
	// HydrateAndMarkChatsDirty runs inside the PushContextState
	// transaction using the supplied store. It hydrates chats for the
	// agent that have no pinned hash yet (no dirty event) and flips
	// already-pinned chats whose hash differs from aggregateHash. It
	// returns a callback that publishes the resulting dirty watch events;
	// the caller invokes it only after the transaction commits. The
	// callback is a no-op when nothing transitioned to dirty.
	HydrateAndMarkChatsDirty(ctx context.Context, tx database.Store, agentID uuid.UUID, aggregateHash []byte, snapshotError string, now time.Time) (publishDirty func(), err error)
}

// PushContextState persists a snapshot pushed by the workspace
// agent. The transaction upserts the snapshot row, upserts each
// resource, then deletes any resources whose source is not in the
// incoming set so the stored snapshot and resource table always
// agree. It runs at repeatable read isolation (with retries) so two
// concurrent pushes cannot interleave their writes; the loser of the
// conflict re-runs the version gate against the winner's committed
// state.
//
// Returns accepted = false (without writing) when the push is a
// replay or out-of-order resend: the agent's per-process version
// counter is monotonic, and only an initial = true push from a
// freshly-booted agent resets that baseline. Replays and stale
// retransmits leave the stored state untouched.
//
// Authorization happens in dbauthz: every query in the transaction
// authorizes the actor (the agent's token subject) against the
// workspace that owns the agent.
func (a *ContextAPI) PushContextState(ctx context.Context, req *agentproto.PushContextStateRequest) (*agentproto.PushContextStateResponse, error) {
	if a.Disabled {
		// The Unimplemented code (not a plain error) is what tells the
		// agent to stop pushing instead of retrying with backoff.
		return nil, drpcerr.WithCode(
			xerrors.New("agentapi: workspace agent context sync is disabled on this deployment"),
			drpcerr.Unimplemented,
		)
	}
	if req == nil {
		return nil, xerrors.New("agentapi: PushContextState request is nil")
	}
	if err := validateContextPushRequest(req); err != nil {
		return nil, err
	}

	rows, err := validateAndConvertContextResources(req.Resources, a.Experiments)
	if err != nil {
		return nil, err
	}

	// Attach the cached workspace RBAC object so dbauthz can take its
	// fast path. On failure (or when unset, e.g. prebuilds) dbauthz
	// falls back to fetching the workspace by agent ID.
	if a.Workspace != nil {
		injected, err := a.Workspace.ContextInject(ctx)
		if err != nil {
			a.Log.Debug(ctx, "failed to inject cached workspace RBAC object", slog.Error(err))
		} else {
			ctx = injected
		}
	}

	clock := a.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	now := dbtime.Time(clock.Now())

	activeSources := make([]string, 0, len(rows))
	for _, r := range rows {
		activeSources = append(activeSources, r.Source)
	}
	sort.Strings(activeSources)

	var accepted bool
	// publishDirty is captured from the final (committed) attempt and
	// invoked after the transaction commits; ReadModifyUpdate may re-run
	// the closure on serialization conflicts.
	var publishDirty func()
	err = database.ReadModifyUpdate(a.Database, func(tx database.Store) error {
		// The closure re-runs on serialization conflicts; reset any
		// state carried over from a rolled-back attempt.
		accepted = false
		publishDirty = nil

		existing, err := tx.GetLatestWorkspaceAgentContextSnapshot(ctx, a.AgentID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// No previous snapshot; first push always wins.
		case err != nil:
			return xerrors.Errorf("get latest snapshot: %w", err)
		default:
			// Accept either a fresh agent process (initial) or
			// a strictly newer version. Out-of-order or replayed
			// pushes leave the stored state untouched.
			//
			//nolint:gosec // existing.Version is a uint64 round-tripped via BIGINT; non-negative by construction.
			if !req.Initial && req.Version <= uint64(existing.Version) {
				return nil
			}
		}

		_, err = tx.UpsertWorkspaceAgentContextSnapshot(ctx, database.UpsertWorkspaceAgentContextSnapshotParams{
			WorkspaceAgentID: a.AgentID,
			//nolint:gosec // Bounded by validateContextPushRequest.
			Version:       int64(req.Version),
			AggregateHash: append([]byte(nil), req.AggregateHash...),
			SnapshotError: req.SnapshotError,
			ReceivedAt:    now,
		})
		if err != nil {
			return xerrors.Errorf("upsert snapshot: %w", err)
		}

		for _, r := range rows {
			r.WorkspaceAgentID = a.AgentID
			r.Now = now
			_, err = tx.UpsertWorkspaceAgentContextResource(ctx, r)
			if err != nil {
				return xerrors.Errorf("upsert resource %q: %w", r.Source, err)
			}
		}

		err = tx.DeleteStaleWorkspaceAgentContextResources(ctx, database.DeleteStaleWorkspaceAgentContextResourcesParams{
			WorkspaceAgentID: a.AgentID,
			ActiveSources:    activeSources,
		})
		if err != nil {
			return xerrors.Errorf("delete stale resources: %w", err)
		}

		// Hydrate and dirty chats against the snapshot just written, in the
		// same transaction so a concurrent refresh cannot interleave with
		// the version gate. Events are published only after commit.
		if a.DirtyMarker != nil {
			publishDirty, err = a.DirtyMarker.HydrateAndMarkChatsDirty(ctx, tx, a.AgentID, req.AggregateHash, req.SnapshotError, now)
			if err != nil {
				return xerrors.Errorf("hydrate and mark chats dirty: %w", err)
			}
		}

		accepted = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	if !accepted {
		a.Log.Debug(ctx, "PushContextState dropped: replay or out-of-order",
			slog.F("agent_id", a.AgentID),
			slog.F("version", req.Version),
			slog.F("initial", req.Initial),
		)
		return &agentproto.PushContextStateResponse{Accepted: false}, nil
	}

	// The snapshot committed; fan out dirty watch events to chats whose
	// pinned context drifted from this push.
	if publishDirty != nil {
		publishDirty()
	}

	a.Log.Debug(ctx, "PushContextState accepted",
		slog.F("agent_id", a.AgentID),
		slog.F("version", req.Version),
		slog.F("initial", req.Initial),
		slog.F("resources", len(rows)),
	)
	return &agentproto.PushContextStateResponse{Accepted: true}, nil
}

// validateContextPushRequest enforces the request-level caps: counts
// and sizes a compromised workspace could otherwise inflate to DoS
// coderd or bloat the database. The resource count includes resources
// nested in plugins.
func validateContextPushRequest(req *agentproto.PushContextStateRequest) error {
	if req.Version > math.MaxInt64 {
		return xerrors.Errorf("agentapi: PushContextState version %d exceeds int64 range", req.Version)
	}
	if len(req.AggregateHash) > maxContextHashBytes {
		return xerrors.Errorf("agentapi: PushContextState aggregate hash is %d bytes, exceeds %d byte cap", len(req.AggregateHash), maxContextHashBytes)
	}
	if len(req.SnapshotError) > workspacesdk.MaxContextErrorBytes {
		return xerrors.Errorf("agentapi: PushContextState snapshot error is %d bytes, exceeds %d byte cap", len(req.SnapshotError), workspacesdk.MaxContextErrorBytes)
	}
	if n := contextResourceCount(req.Resources); n > workspacesdk.MaxContextResources {
		return xerrors.Errorf("agentapi: PushContextState has %d resources, exceeds %d resource cap", n, workspacesdk.MaxContextResources)
	}
	return nil
}

func contextResourceCount(resources []*agentproto.ContextResource) int {
	n := len(resources)
	for _, r := range resources {
		plugin := r.GetPlugin()
		n += len(plugin.GetSkills()) + len(plugin.GetMcpServers())
	}
	return n
}

// validateAndConvertContextResources translates wire resources into
// upsert parameters while rejecting structurally invalid input:
//
//   - empty, oversized, or duplicate sources (the PK depends on
//     uniqueness and indexes the source column),
//   - unknown body variants,
//   - invalid plugin names, misplaced nested resources, and OK plugin
//     names used more than once,
//   - unknown status enum values,
//   - per-resource and aggregate body sizes past the server caps.
//
// A plugin resource becomes one row for the plugin and one row per
// nested skill and MCP server. plugin_name is the plugin's name on each
// nested row and on the row of an OK plugin. With the agent-plugins
// experiment off, plugin resources and their nested resources are
// dropped without validation. The plugin name uniqueness check runs
// only after every resource passes its own checks.
//
// Validation is deliberately strict here so a misbehaving agent
// cannot poison the snapshot table. Phase 2 readers can then trust
// that every row maps to a known proto variant.
//
// WorkspaceAgentID and Now are left unset; the caller fills them at
// upsert time.
func validateAndConvertContextResources(resources []*agentproto.ContextResource, experiments codersdk.Experiments) ([]database.UpsertWorkspaceAgentContextResourceParams, error) {
	pluginsEnabled := experiments.Enabled(codersdk.ExperimentAgentPlugins)
	c := contextRowConverter{
		rows: make([]database.UpsertWorkspaceAgentContextResourceParams, 0, len(resources)),
		seen: make(map[string]struct{}, len(resources)),
	}
	var okPlugins []*agentproto.ContextResource
	for i, r := range resources {
		if r == nil {
			return nil, xerrors.Errorf("agentapi: PushContextState resource at index %d is nil", i)
		}
		_, isPlugin := r.Body.(*agentproto.ContextResource_Plugin)
		if isPlugin && !pluginsEnabled {
			continue
		}
		location := fmt.Sprintf("resource at index %d", i)
		var pluginName string
		if isPlugin && r.Status == agentproto.ContextResource_OK {
			pluginName = r.GetPlugin().GetName()
		}
		if err := c.add(r, location, pluginName); err != nil {
			return nil, err
		}
		if !isPlugin {
			continue
		}
		if err := c.addNested(r, location); err != nil {
			return nil, err
		}
		if r.Status == agentproto.ContextResource_OK {
			okPlugins = append(okPlugins, r)
		}
	}
	if err := validateUniquePluginNames(okPlugins); err != nil {
		return nil, err
	}
	return c.rows, nil
}

// contextRowConverter accumulates a push's rows, the sources seen so
// far, and the aggregate body size.
type contextRowConverter struct {
	rows               []database.UpsertWorkspaceAgentContextResourceParams
	seen               map[string]struct{}
	aggregateBodyBytes int
}

// add validates r and appends its row with pluginName as its
// plugin_name. location names r in errors that cannot quote its source.
func (c *contextRowConverter) add(r *agentproto.ContextResource, location, pluginName string) error {
	if r.Source == "" {
		return xerrors.Errorf("agentapi: PushContextState %s has empty source", location)
	}
	if len(r.Source) > workspacesdk.MaxContextSourceBytes {
		return xerrors.Errorf("agentapi: PushContextState %s has %d byte source, exceeds %d byte cap", location, len(r.Source), workspacesdk.MaxContextSourceBytes)
	}
	if _, ok := c.seen[r.Source]; ok {
		return xerrors.Errorf("agentapi: PushContextState duplicate source %q", r.Source)
	}
	c.seen[r.Source] = struct{}{}

	if len(r.GetSourcePath()) > workspacesdk.MaxContextSourceBytes {
		return xerrors.Errorf("resource %q: source path is %d bytes, exceeds %d byte cap", r.Source, len(r.GetSourcePath()), workspacesdk.MaxContextSourceBytes)
	}
	if len(r.Error) > workspacesdk.MaxContextErrorBytes {
		return xerrors.Errorf("resource %q: error is %d bytes, exceeds %d byte cap", r.Source, len(r.Error), workspacesdk.MaxContextErrorBytes)
	}
	if len(r.ContentHash) > maxContextHashBytes {
		return xerrors.Errorf("resource %q: content hash is %d bytes, exceeds %d byte cap", r.Source, len(r.ContentHash), maxContextHashBytes)
	}
	if r.SizeBytes > math.MaxInt64 {
		return xerrors.Errorf("resource %q: size %d exceeds int64 range", r.Source, r.SizeBytes)
	}

	kind, body, err := marshalContextResourceBody(r)
	if err != nil {
		return xerrors.Errorf("resource %q: %w", r.Source, err)
	}
	if len(body) > maxContextResourceBodyBytes {
		return xerrors.Errorf("resource %q: body is %d bytes, exceeds %d byte cap", r.Source, len(body), maxContextResourceBodyBytes)
	}
	if err := validatePluginName(r); err != nil {
		return xerrors.Errorf("resource %q: %w", r.Source, err)
	}
	c.aggregateBodyBytes += len(body)
	if c.aggregateBodyBytes > maxContextAggregateBodyBytes {
		return xerrors.Errorf("agentapi: PushContextState aggregate body size exceeds %d byte cap", maxContextAggregateBodyBytes)
	}
	status, err := contextResourceStatus(r.Status)
	if err != nil {
		return xerrors.Errorf("resource %q: %w", r.Source, err)
	}

	//nolint:exhaustruct // WorkspaceAgentID and Now are filled by the caller at upsert time.
	c.rows = append(c.rows, database.UpsertWorkspaceAgentContextResourceParams{
		Source:      r.Source,
		SourcePath:  r.GetSourcePath(),
		BodyKind:    kind,
		Body:        body,
		ContentHash: append([]byte(nil), r.ContentHash...),
		//nolint:gosec // Bounded above.
		SizeBytes:  int64(r.SizeBytes),
		Status:     status,
		Error:      r.Error,
		PluginName: pluginName,
	})
	return nil
}

// addNested validates and appends the skills and MCP servers nested in
// plugin resource r, which must have passed add. location names r.
//
//   - only an OK plugin may nest resources,
//   - each entry of skills must carry a skill body and a source under
//     "<plugin source>/skills/", with either path separator,
//   - each entry of mcp_servers must carry an mcp_server body with a
//     non-empty server_name and the source "<name>/<server_name>".
//
// The skills/ check is a string prefix, not path containment. Errors
// quote only capped or validated strings.
func (c *contextRowConverter) addNested(r *agentproto.ContextResource, location string) error {
	plugin := r.GetPlugin()
	if len(plugin.GetSkills())+len(plugin.GetMcpServers()) == 0 {
		return nil
	}
	if r.Status != agentproto.ContextResource_OK {
		return xerrors.Errorf("resource %q: a plugin whose status is not OK must not nest resources", r.Source)
	}
	name := plugin.GetName()
	skillsPrefix := strings.ReplaceAll(r.Source, `\`, "/") + "/skills/"
	for i, skill := range plugin.GetSkills() {
		if _, ok := skill.GetBody().(*agentproto.ContextResource_Skill); !ok {
			return xerrors.Errorf("resource %q: skills[%d] must carry a skill body", r.Source, i)
		}
		if err := c.add(skill, fmt.Sprintf("%s skills[%d]", location, i), name); err != nil {
			return err
		}
		if !strings.HasPrefix(strings.ReplaceAll(skill.Source, `\`, "/"), skillsPrefix) {
			return xerrors.Errorf("resource %q: plugin skill source must be under %q", skill.Source, r.Source+"/skills/")
		}
	}
	for i, server := range plugin.GetMcpServers() {
		if _, ok := server.GetBody().(*agentproto.ContextResource_McpServer); !ok {
			return xerrors.Errorf("resource %q: mcp_servers[%d] must carry an mcp_server body", r.Source, i)
		}
		if err := c.add(server, fmt.Sprintf("%s mcp_servers[%d]", location, i), name); err != nil {
			return err
		}
		serverName := server.GetMcpServer().GetServerName()
		if serverName == "" {
			return xerrors.Errorf("resource %q: plugin MCP server_name is required", server.Source)
		}
		if server.Source != name+"/"+serverName {
			return xerrors.Errorf("resource %q: plugin MCP server source must be %q", server.Source, name+"/<server_name>")
		}
	}
	return nil
}

// validatePluginName checks the name of a plugin resource: an OK plugin
// must have one, and a non-empty name must pass
// workspacesdk.ValidatePluginName.
func validatePluginName(r *agentproto.ContextResource) error {
	plugin, ok := r.Body.(*agentproto.ContextResource_Plugin)
	if !ok {
		return nil
	}
	name := plugin.Plugin.GetName()
	if name == "" {
		if r.Status == agentproto.ContextResource_OK {
			return xerrors.New("plugin name is required on an OK plugin resource")
		}
		return nil
	}
	return workspacesdk.ValidatePluginName(name)
}

// validateUniquePluginNames rejects two OK plugin resources with the
// same name.
func validateUniquePluginNames(plugins []*agentproto.ContextResource) error {
	sources := make(map[string]string, len(plugins))
	for _, r := range plugins {
		name := r.GetPlugin().GetName()
		if first, ok := sources[name]; ok {
			return xerrors.Errorf("agentapi: PushContextState OK plugin name %q is used by both %q and %q", name, first, r.Source)
		}
		sources[name] = r.Source
	}
	return nil
}

// marshalContextResourceBody picks the body variant set on the wire
// resource and returns the (body_kind, body_jsonb) pair stored in
// the resource row. The body is protojson encoded so the schema can
// be evolved by adding fields to the proto without coderd changes,
// and a future reader can round-trip back to the proto type by
// switching on body_kind.
//
// Body is always populated, even on non-OK statuses: the wire
// guarantees the oneof variant is set so coderd can still attribute
// the failure to a known kind. For variants with no content fields
// (mcp_config), an empty JSON object is stored. A plugin body is stored
// without skills and mcp_servers.
func marshalContextResourceBody(r *agentproto.ContextResource) (kind database.WorkspaceAgentContextBodyKind, body []byte, err error) {
	switch b := r.Body.(type) {
	case *agentproto.ContextResource_InstructionFile:
		payload := b.InstructionFile
		if payload == nil {
			payload = &agentproto.InstructionFileBody{}
		}
		body, err = marshalBody(payload)
		return database.WorkspaceAgentContextBodyKindInstructionFile, body, err
	case *agentproto.ContextResource_Skill:
		payload := b.Skill
		if payload == nil {
			payload = &agentproto.SkillMetaBody{}
		}
		body, err = marshalBody(payload)
		return database.WorkspaceAgentContextBodyKindSkill, body, err
	case *agentproto.ContextResource_McpConfig:
		payload := b.McpConfig
		if payload == nil {
			payload = &agentproto.MCPConfigBody{}
		}
		body, err = marshalBody(payload)
		return database.WorkspaceAgentContextBodyKindMcpConfig, body, err
	case *agentproto.ContextResource_McpServer:
		payload := b.McpServer
		if payload == nil {
			payload = &agentproto.MCPServerBody{}
		}
		body, err = marshalBody(payload)
		return database.WorkspaceAgentContextBodyKindMcpServer, body, err
	case *agentproto.ContextResource_AcpHarness:
		payload := b.AcpHarness
		if payload == nil {
			payload = &agentproto.ACPHarnessBody{}
		}
		body, err = marshalBody(payload)
		return database.WorkspaceAgentContextBodyKindAcpHarness, body, err
	case *agentproto.ContextResource_Plugin:
		body, err = marshalBody(&agentproto.PluginBody{
			Name:        b.Plugin.GetName(),
			Version:     b.Plugin.GetVersion(),
			Description: b.Plugin.GetDescription(),
		})
		return database.WorkspaceAgentContextBodyKindPlugin, body, err
	case nil:
		return "", nil, xerrors.Errorf("missing body variant; status %s requires a typed body", r.Status)
	default:
		return "", nil, xerrors.Errorf("unsupported body variant %T", r.Body)
	}
}

// contextBodyMarshalOptions produces deterministic-ish JSON for the
// body so the stored value compares equal across pushes that yield
// equivalent protos. Strict canonicalization (RFC 8785) is not
// required here; the enum column plus the protojson round trip give
// us a stable enough store.
var contextBodyMarshalOptions = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
}

// marshalBody is a small wrapper around protojson.Marshal that
// keeps the body encoding in one place; future phases that read
// these rows mirror the call with protojson.Unmarshal into the
// matching proto.Message.
func marshalBody(msg proto.Message) ([]byte, error) {
	out, err := contextBodyMarshalOptions.Marshal(msg)
	if err != nil {
		return nil, xerrors.Errorf("marshal body: %w", err)
	}
	return out, nil
}

// contextResourceStatus translates the wire status enum to the
// database enum. STATUS_UNSPECIFIED is rejected: every well-formed
// snapshot row needs an explicit status so cache invalidation, dirty
// fan-out, and the Sources drawer can reason about partial pushes
// deterministically.
func contextResourceStatus(s agentproto.ContextResource_Status) (database.WorkspaceAgentContextResourceStatus, error) {
	switch s {
	case agentproto.ContextResource_OK:
		return database.WorkspaceAgentContextResourceStatusOk, nil
	case agentproto.ContextResource_OVERSIZE:
		return database.WorkspaceAgentContextResourceStatusOversize, nil
	case agentproto.ContextResource_UNREADABLE:
		return database.WorkspaceAgentContextResourceStatusUnreadable, nil
	case agentproto.ContextResource_INVALID:
		return database.WorkspaceAgentContextResourceStatusInvalid, nil
	case agentproto.ContextResource_EXCLUDED:
		return database.WorkspaceAgentContextResourceStatusExcluded, nil
	default:
		return "", xerrors.Errorf("unknown status %d", s)
	}
}

package agentcontext

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/structpb"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// DRPCPusher adapts a generated DRPCAgentClient to the
// agentcontext.Pusher interface. The adapter is the only place
// that knows about the wire protobuf types; the rest of the
// package operates on the Go Snapshot/Resource value types.
//
// Use NewDRPCPusher to construct an instance. Per-request retries
// are handled by Manager.RunPush. When coderd rejects a push that
// carries Agent Plugins data and accepts the same push without it,
// the pusher omits plugin data from every later push it sends and
// reports the rejection in SnapshotError.
type DRPCPusher struct {
	// Logger receives a warning when coderd rejects plugin data. The
	// zero value discards it.
	Logger slog.Logger

	client agentproto.DRPCAgentClient210

	mu              sync.Mutex
	pluginsRejected bool
}

// NewDRPCPusher wraps the supplied drpc client. The client must
// implement the v2.10 Agent API.
func NewDRPCPusher(client agentproto.DRPCAgentClient210) *DRPCPusher {
	return &DRPCPusher{client: client}
}

// PushContextState satisfies the Pusher interface.
//
// drpc returns an Unimplemented error when the peer's service
// definition does not include the RPC. The adapter translates
// that into ErrPushUnimplemented so RunPush stops gracefully
// when an old coderd is on the other end.
//
// Any other error on a push that carries plugin data is retried
// once without it. If that retry succeeds, the pusher omits plugin
// data from then on; otherwise the first error is returned.
func (p *DRPCPusher) PushContextState(ctx context.Context, req *PushRequest) (*PushResponse, error) {
	if p == nil || p.client == nil {
		return nil, xerrors.New("agentcontext: DRPCPusher has no client")
	}
	p.mu.Lock()
	rejected := p.pluginsRejected
	p.mu.Unlock()
	if rejected {
		return p.send(ctx, withoutPluginData(req))
	}

	resp, err := p.send(ctx, req)
	// TODO(jscottmiller): remove this retry and the pluginsRejected latch
	// once Agent Plugins have been stable in production for a few releases.
	if err == nil || errors.Is(err, ErrPushUnimplemented) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		ctx.Err() != nil || !slices.ContainsFunc(req.Resources, isPluginData) {
		return resp, err
	}
	resp, retryErr := p.send(ctx, withoutPluginData(req))
	if retryErr != nil {
		return nil, err
	}
	p.mu.Lock()
	p.pluginsRejected = true
	p.mu.Unlock()
	p.Logger.Warn(ctx, "coderd rejected Agent Plugins data; pushing context without plugins on this connection",
		slog.Error(err))
	return resp, nil
}

func (p *DRPCPusher) send(ctx context.Context, req *PushRequest) (*PushResponse, error) {
	resp, err := p.client.PushContextState(ctx, pushRequestToProto(req))
	if err != nil {
		if drpcerr.Code(err) == drpcerr.Unimplemented {
			return nil, ErrPushUnimplemented
		}
		return nil, err
	}
	return &PushResponse{Accepted: resp.GetAccepted()}, nil
}

// pluginsRejectedNotice omits coderd's error, which names internal
// fields and host paths; the agent log carries it.
const pluginsRejectedNotice = "Agent Plugins could not be loaded. Check the workspace agent logs for details."

func isPluginData(r Resource) bool {
	return r.Kind == KindPlugin || r.PluginName != ""
}

// withoutPluginData returns a copy of req without KindPlugin
// resources or resources with a PluginName, with the aggregate hash
// recomputed and pluginsRejectedNotice appended to SnapshotError.
func withoutPluginData(req *PushRequest) *PushRequest {
	out := *req
	out.Resources = slices.DeleteFunc(slices.Clone(req.Resources), isPluginData)
	out.AggregateHash = ComputeAggregateHash(driftResources(out.Resources))
	notice := pluginsRejectedNotice
	if out.SnapshotError != "" {
		notice = out.SnapshotError + "; " + notice
	}
	out.SnapshotError = truncateUTF8Bytes(strings.ToValidUTF8(notice, "\uFFFD"), workspacesdk.MaxContextErrorBytes)
	return &out
}

// pushRequestToProto converts the Go push payload to its
// generated protobuf equivalent. The Kind on each Resource
// selects which body variant of the proto oneof is set; a body
// is always set (zero-valued if necessary) so coderd can tell
// the kind even when Status != OK. Each Error has invalid UTF-8
// replaced, since proto3 strings must be valid UTF-8 or the whole
// request fails to marshal, and is truncated to
// workspacesdk.MaxContextErrorBytes.
//
// Each resource with a PluginName is nested in the StatusOK KindPlugin
// resource of that name. req must hold such a plugin for every
// PluginName: a resource without one is dropped, but AggregateHash
// still covers it.
func pushRequestToProto(req *PushRequest) *agentproto.PushContextStateRequest {
	pb := &agentproto.PushContextStateRequest{
		Version:       req.Version,
		AggregateHash: append([]byte(nil), req.AggregateHash[:]...),
		Initial:       req.Initial,
		SnapshotError: req.SnapshotError,
		Resources:     make([]*agentproto.ContextResource, 0, len(req.Resources)),
	}
	plugins := make(map[string]*agentproto.PluginBody)
	for _, r := range req.Resources {
		if r.PluginName != "" {
			continue
		}
		entry := resourceToProto(r)
		pb.Resources = append(pb.Resources, entry)
		if r.Kind == KindPlugin && r.Status == StatusOK {
			plugins[r.Name] = entry.GetPlugin()
		}
	}
	for _, r := range req.Resources {
		plugin, ok := plugins[r.PluginName]
		if r.PluginName == "" || !ok {
			continue
		}
		switch r.Kind {
		case KindSkill:
			plugin.Skills = append(plugin.Skills, resourceToProto(r))
		case KindMCPServer:
			plugin.McpServers = append(plugin.McpServers, resourceToProto(r))
		}
	}
	return pb
}

func resourceToProto(r Resource) *agentproto.ContextResource {
	entry := &agentproto.ContextResource{
		Source:      r.Source,
		ContentHash: append([]byte(nil), r.ContentHash[:]...),
		Status:      resourceStatusToProto(r.Status),
		SizeBytes:   r.SizeBytes,
		Error:       truncateUTF8Bytes(strings.ToValidUTF8(r.Error, "\uFFFD"), workspacesdk.MaxContextErrorBytes),
	}
	setResourceBody(entry, r)
	if r.SourcePath != "" {
		sp := r.SourcePath
		entry.SourcePath = &sp
	}
	return entry
}

// truncateUTF8Bytes cuts s to at most limit bytes without splitting a
// UTF-8 sequence.
func truncateUTF8Bytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// setResourceBody picks the proto oneof variant for r's Kind and
// populates the kind-specific fields from r. A body is set even
// when status is not OK so coderd can attribute the failure to a
// known kind. Unknown kinds leave the body unset; the recipient
// can surface that as "kind not recognized".
func setResourceBody(entry *agentproto.ContextResource, r Resource) {
	switch r.Kind {
	case KindInstructionFile:
		entry.Body = &agentproto.ContextResource_InstructionFile{
			InstructionFile: &agentproto.InstructionFileBody{
				Content: append([]byte(nil), r.Payload...),
			},
		}
	case KindSkill:
		entry.Body = &agentproto.ContextResource_Skill{
			Skill: &agentproto.SkillMetaBody{
				Meta:        append([]byte(nil), r.Payload...),
				Name:        r.Name,
				Description: r.Description,
			},
		}
	case KindMCPConfig:
		// MCPConfigBody is intentionally empty: secrets in env
		// blocks must not leave the agent.
		entry.Body = &agentproto.ContextResource_McpConfig{
			McpConfig: &agentproto.MCPConfigBody{},
		}
	case KindMCPServer:
		entry.Body = &agentproto.ContextResource_McpServer{
			McpServer: &agentproto.MCPServerBody{
				ServerName:  serverNameOrSource(r),
				Description: r.Description,
				Tools:       mcpToolsToProto(r.Tools),
			},
		}
	case KindPlugin:
		entry.Body = &agentproto.ContextResource_Plugin{
			Plugin: &agentproto.PluginBody{
				Name:        r.Name,
				Version:     r.PluginVersion,
				Description: r.Description,
			},
		}
	}
}

// serverNameOrSource returns r.Name when populated and falls
// back to r.Source so providers that have not yet adopted the
// Name field still produce a usable wire value.
func serverNameOrSource(r Resource) string {
	if r.Name != "" {
		return r.Name
	}
	return r.Source
}

// mcpToolsToProto converts the Go MCPTool slice to its wire
// representation. InputSchema is marshaled via structpb.NewStruct;
// schemas that fail to convert are dropped from the wire copy
// (the resource ContentHash still detects the change) and the
// tool ships with InputSchema unset rather than failing the
// whole push.
func mcpToolsToProto(in []MCPTool) []*agentproto.MCPTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]*agentproto.MCPTool, 0, len(in))
	for _, t := range in {
		entry := &agentproto.MCPTool{
			Name:        t.Name,
			Description: t.Description,
		}
		if len(t.InputSchema) > 0 {
			if s, err := structpb.NewStruct(t.InputSchema); err == nil {
				entry.InputSchema = s
			}
		}
		out = append(out, entry)
	}
	return out
}

// resourceStatusToProto maps a ResourceStatus to its proto enum.
func resourceStatusToProto(s ResourceStatus) agentproto.ContextResource_Status {
	switch s {
	case StatusOK:
		return agentproto.ContextResource_OK
	case StatusOversize:
		return agentproto.ContextResource_OVERSIZE
	case StatusUnreadable:
		return agentproto.ContextResource_UNREADABLE
	case StatusInvalid:
		return agentproto.ContextResource_INVALID
	case StatusExcluded:
		return agentproto.ContextResource_EXCLUDED
	default:
		return agentproto.ContextResource_STATUS_UNSPECIFIED
	}
}

// Ensure DRPCPusher continues to satisfy the Pusher interface
// even if the interface gains methods in the future.
var _ Pusher = (*DRPCPusher)(nil)

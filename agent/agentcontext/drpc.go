package agentcontext

import (
	"context"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/structpb"
	"storj.io/drpc/drpcerr"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

// DRPCPusher adapts a generated DRPCAgentClient to the
// agentcontext.Pusher interface. The adapter is the only place
// that knows about the wire protobuf types; the rest of the
// package operates on the Go Snapshot/Resource value types.
//
// Use NewDRPCPusher to construct an instance. The pusher's
// behavior is identical to invoking PushContextState directly:
// per-request retries are handled by Manager.RunPush.
type DRPCPusher struct {
	client agentproto.DRPCAgentClient210
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
func (p *DRPCPusher) PushContextState(ctx context.Context, req *PushRequest) (*PushResponse, error) {
	if p == nil || p.client == nil {
		return nil, xerrors.New("agentcontext: DRPCPusher has no client")
	}
	resp, err := p.client.PushContextState(ctx, pushRequestToProto(req))
	if err != nil {
		if drpcerr.Code(err) == drpcerr.Unimplemented {
			return nil, ErrPushUnimplemented
		}
		return nil, err
	}
	return &PushResponse{Accepted: resp.GetAccepted()}, nil
}

// pushRequestToProto converts the Go push payload to its
// generated protobuf equivalent. The Kind on each Resource
// selects which body variant of the proto oneof is set; a body
// is always set (zero-valued if necessary) so coderd can tell
// the kind even when Status != OK.
func pushRequestToProto(req *PushRequest) *agentproto.PushContextStateRequest {
	pb := &agentproto.PushContextStateRequest{
		Version:       req.Version,
		AggregateHash: append([]byte(nil), req.AggregateHash[:]...),
		Initial:       req.Initial,
		SnapshotError: req.SnapshotError,
		Resources:     make([]*agentproto.ContextResource, 0, len(req.Resources)),
	}
	for i := range req.Resources {
		r := req.Resources[i]
		entry := &agentproto.ContextResource{
			Source:      r.Source,
			ContentHash: append([]byte(nil), r.ContentHash[:]...),
			Status:      resourceStatusToProto(r.Status),
			SizeBytes:   r.SizeBytes,
			Error:       r.Error,
		}
		setResourceBody(entry, r)
		if r.SourcePath != "" {
			sp := r.SourcePath
			entry.SourcePath = &sp
		}
		pb.Resources = append(pb.Resources, entry)
	}
	return pb
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

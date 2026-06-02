package agentcontext

import (
	"context"

	"golang.org/x/xerrors"
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
// generated protobuf equivalent.
func pushRequestToProto(req *PushRequest) *agentproto.PushContextStateRequest {
	pb := &agentproto.PushContextStateRequest{
		Version:       req.Version,
		AggregateHash: append([]byte(nil), req.AggregateHash[:]...),
		Initial:       req.Initial,
		SchemaVersion: req.SchemaVersion,
		SnapshotError: req.SnapshotError,
		Resources:     make([]*agentproto.ContextResource, 0, len(req.Resources)),
	}
	for i := range req.Resources {
		r := req.Resources[i]
		entry := &agentproto.ContextResource{
			Id:          r.ID,
			Kind:        resourceKindToProto(r.Kind),
			Source:      r.Source,
			ContentHash: append([]byte(nil), r.ContentHash[:]...),
			Payload:     append([]byte(nil), r.Payload...),
			Status:      resourceStatusToProto(r.Status),
			SizeBytes:   r.SizeBytes,
			Error:       r.Error,
			Description: r.Description,
		}
		if r.SourcePath != "" {
			sp := r.SourcePath
			entry.SourcePath = &sp
		}
		pb.Resources = append(pb.Resources, entry)
	}
	return pb
}

// resourceKindToProto maps a ResourceKind to its proto enum.
// Unknown kinds map to KIND_UNSPECIFIED so future kinds added
// to the Go enum without a matching proto bump do not crash.
func resourceKindToProto(k ResourceKind) agentproto.ContextResource_Kind {
	switch k {
	case KindInstructionFile:
		return agentproto.ContextResource_INSTRUCTION_FILE
	case KindSkill:
		return agentproto.ContextResource_SKILL
	case KindMCPConfig:
		return agentproto.ContextResource_MCP_CONFIG
	case KindMCPServer:
		return agentproto.ContextResource_MCP_SERVER
	case KindPlugin:
		return agentproto.ContextResource_PLUGIN
	case KindHook:
		return agentproto.ContextResource_HOOK
	case KindSubagent:
		return agentproto.ContextResource_SUBAGENT
	case KindCommand:
		return agentproto.ContextResource_COMMAND
	default:
		return agentproto.ContextResource_KIND_UNSPECIFIED
	}
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

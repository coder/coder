package aibridged

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/coder/coder/v2/aibridged/proto"
)

type Metadata map[string]any

func (m Metadata) MarshalForProto() map[string]*anypb.Any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]*anypb.Any, len(m))
	for k, v := range m {
		if sv, err := structpb.NewValue(v); err == nil {
			if av, err := anypb.New(sv); err == nil {
				out[k] = av
			}
		}
	}
	return out
}

type Tracker interface {
	TrackTokensUsage(ctx context.Context, sessionID, msgID string, model Model, promptTokens, completionTokens int64, metadata Metadata) error
	TrackPromptUsage(ctx context.Context, sessionID, msgID string, model Model, prompt string, metadata Metadata) error
	TrackToolUsage(ctx context.Context, sessionID, msgID string, model Model, name string, args any, injected bool, metadata Metadata) error
}

var _ Tracker = &DRPCTracker{}

// DRPCTracker tracks usage by calling RPC endpoints on a given dRPC client.
type DRPCTracker struct {
	client proto.DRPCAIBridgeDaemonClient
}

func NewDRPCTracker(client proto.DRPCAIBridgeDaemonClient) *DRPCTracker {
	return &DRPCTracker{client}
}

func (d *DRPCTracker) TrackTokensUsage(ctx context.Context, sessionID, msgID string, model Model, promptTokens, completionTokens int64, metadata Metadata) error {
	_, err := d.client.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
		SessionId:    sessionID,
		MsgId:        msgID,
		InputTokens:  promptTokens,
		OutputTokens: completionTokens,
		Metadata:     metadata.MarshalForProto(),
	})
	return err
}

func (d *DRPCTracker) TrackPromptUsage(ctx context.Context, sessionID, msgID string, model Model, prompt string, metadata Metadata) error {
	_, err := d.client.TrackUserPrompt(ctx, &proto.TrackUserPromptRequest{
		SessionId: sessionID,
		MsgId:     msgID,
		Prompt:    prompt,
		Metadata:  metadata.MarshalForProto(),
	})
	return err
}

func (d *DRPCTracker) TrackToolUsage(ctx context.Context, sessionID, msgID string, model Model, name string, args any, injected bool, metadata Metadata) error {
	var (
		serialized []byte
		err        error
	)

	switch val := args.(type) {
	case string:
		serialized = []byte(val)
	case []byte:
		serialized = val
	default:
		serialized, err = json.Marshal(args)
		if err != nil {
			return xerrors.Errorf("marshal tool usage args: %w", err)
		}
	}

	_, err = d.client.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
		SessionId: sessionID,
		MsgId:     msgID,
		Tool:      name,
		Input:     string(serialized),
		Injected:  injected,
		Metadata:  metadata.MarshalForProto(),
	})
	return err
}

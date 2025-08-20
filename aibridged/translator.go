package aibridged

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/coder/coder/v2/aibridged/proto"

	"github.com/coder/aibridge"
)

var _ aibridge.RecorderClient = &translator{}

// translator satisfies the aibridge.RecorderClient interface and translates calls into dRPC calls to aibridgedserver.
type translator struct {
	client proto.DRPCRecorderClient
}

func (t *translator) RecordSession(ctx context.Context, req *aibridge.SessionRequest) error {
	_, err := t.client.RecordSession(ctx, &proto.RecordSessionRequest{
		SessionId:   req.SessionID,
		InitiatorId: req.InitiatorID,
		Provider:    req.Provider,
		Model:       req.Model,
	})
	return err
}

func (t *translator) RecordPromptUsage(ctx context.Context, req *aibridge.PromptUsageRequest) error {
	_, err := t.client.RecordPromptUsage(ctx, &proto.RecordPromptUsageRequest{
		SessionId: req.SessionID,
		MsgId:     req.MsgID,
		Prompt:    req.Prompt,
		Metadata:  MarshalForProto(req.Metadata),
	})
	return err
}

func (t *translator) RecordTokenUsage(ctx context.Context, req *aibridge.TokenUsageRequest) error {
	_, err := t.client.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		SessionId:    req.SessionID,
		MsgId:        req.MsgID,
		InputTokens:  req.Input,
		OutputTokens: req.Output,
		Metadata:     MarshalForProto(req.Metadata),
	})
	return err
}

func (t *translator) RecordToolUsage(ctx context.Context, req *aibridge.ToolUsageRequest) error {
	serialized, err := json.Marshal(req.Args)
	if err != nil {
		return xerrors.Errorf("serialize tool %q args: %w", req.Name, err)
	}

	_, err = t.client.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		SessionId: req.SessionID,
		MsgId:     req.MsgID,
		Tool:      req.Name,
		Input:     string(serialized),
		Injected:  req.Injected,
		Metadata:  MarshalForProto(req.Metadata),
	})
	return err
}

func MarshalForProto(in aibridge.Metadata) map[string]*anypb.Any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*anypb.Any, len(in))
	for k, v := range in {
		if sv, err := structpb.NewValue(v); err == nil {
			if av, err := anypb.New(sv); err == nil {
				out[k] = av
			}
		}
	}
	return out
}

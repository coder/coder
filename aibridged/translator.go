package aibridged

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/coder/coder/v2/aibridged/proto"

	"github.com/coder/aibridge"
)

var _ aibridge.Recorder = &recorderTranslation{}

// recorderTranslation satisfies the aibridge.Recorder interface and translates calls into dRPC calls to aibridgedserver.
type recorderTranslation struct {
	client proto.DRPCRecorderClient
}

func (t *recorderTranslation) RecordSession(ctx context.Context, req *aibridge.SessionRequest) error {
	_, err := t.client.RecordSession(ctx, &proto.RecordSessionRequest{
		SessionId:   req.SessionID,
		InitiatorId: req.InitiatorID,
		Provider:    req.Provider,
		Model:       req.Model,
	})
	return err
}

func (t *recorderTranslation) RecordPromptUsage(ctx context.Context, req *aibridge.PromptUsageRequest) error {
	_, err := t.client.RecordPromptUsage(ctx, &proto.RecordPromptUsageRequest{
		SessionId: req.SessionID,
		MsgId:     req.MsgID,
		Prompt:    req.Prompt,
		Metadata:  marshalForProto(req.Metadata),
	})
	return err
}

func (t *recorderTranslation) RecordTokenUsage(ctx context.Context, req *aibridge.TokenUsageRequest) error {
	_, err := t.client.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		SessionId:    req.SessionID,
		MsgId:        req.MsgID,
		InputTokens:  req.Input,
		OutputTokens: req.Output,
		Metadata:     marshalForProto(req.Metadata),
	})
	return err
}

func (t *recorderTranslation) RecordToolUsage(ctx context.Context, req *aibridge.ToolUsageRequest) error {
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
		Metadata:  marshalForProto(req.Metadata),
	})
	return err
}

// marshalForProto will attempt to convert from aibridge.Metadata into a proto-friendly map[string]*anypb.Any.
// If any marshaling fails, rather return a map with the error details since we don't want to fail Record* funcs if metadata can't encode,
// since it's, well, metadata.
func marshalForProto(in aibridge.Metadata) map[string]*anypb.Any {
	out := make(map[string]*anypb.Any, len(in))
	if len(in) == 0 {
		return out
	}

	// Instead of returning error, just encode error into metadata.
	encodeErr := func(err error) map[string]*anypb.Any {
		errVal, _ := anypb.New(structpb.NewStringValue(err.Error()))
		mdVal, _ := anypb.New(structpb.NewStringValue(fmt.Sprintf("%+v", in)))
		return map[string]*anypb.Any{
			"error":    errVal,
			"metadata": mdVal,
		}
	}

	for k, v := range in {
		sv, err := structpb.NewValue(v)
		if err != nil {
			return encodeErr(err)
		}

		av, err := anypb.New(sv)
		if err != nil {
			return encodeErr(err)
		}

		out[k] = av
	}
	return out
}

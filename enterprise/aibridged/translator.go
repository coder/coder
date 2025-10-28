package aibridged

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/enterprise/aibridged/proto"

	"github.com/coder/aibridge"
)

var _ aibridge.Recorder = &recorderTranslation{}

// recorderTranslation satisfies the aibridge.Recorder interface and translates calls into dRPC calls to aibridgedserver.
type recorderTranslation struct {
	apiKeyID string
	client   proto.DRPCRecorderClient
}

func (t *recorderTranslation) RecordInterception(ctx context.Context, req *aibridge.InterceptionRecord) error {
	_, err := t.client.RecordInterception(ctx, &proto.RecordInterceptionRequest{
		Id:          req.ID,
		ApiKeyId:    t.apiKeyID,
		InitiatorId: req.InitiatorID,
		Provider:    req.Provider,
		Model:       req.Model,
		Metadata:    marshalForProto(req.Metadata),
		StartedAt:   timestamppb.New(req.StartedAt),
	})
	return err
}

func (t *recorderTranslation) RecordInterceptionEnded(ctx context.Context, req *aibridge.InterceptionRecordEnded) error {
	_, err := t.client.RecordInterceptionEnded(ctx, &proto.RecordInterceptionEndedRequest{
		Id:      req.ID,
		EndedAt: timestamppb.New(req.EndedAt),
	})
	return err
}

func (t *recorderTranslation) RecordPromptUsage(ctx context.Context, req *aibridge.PromptUsageRecord) error {
	_, err := t.client.RecordPromptUsage(ctx, &proto.RecordPromptUsageRequest{
		InterceptionId: req.InterceptionID,
		MsgId:          req.MsgID,
		Prompt:         req.Prompt,
		Metadata:       marshalForProto(req.Metadata),
		CreatedAt:      timestamppb.New(req.CreatedAt),
	})
	return err
}

func (t *recorderTranslation) RecordTokenUsage(ctx context.Context, req *aibridge.TokenUsageRecord) error {
	_, err := t.client.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		InterceptionId: req.InterceptionID,
		MsgId:          req.MsgID,
		InputTokens:    req.Input,
		OutputTokens:   req.Output,
		Metadata:       marshalForProto(req.Metadata),
		CreatedAt:      timestamppb.New(req.CreatedAt),
	})
	return err
}

func (t *recorderTranslation) RecordToolUsage(ctx context.Context, req *aibridge.ToolUsageRecord) error {
	serialized, err := json.Marshal(req.Args)
	if err != nil {
		return xerrors.Errorf("serialize tool %q args: %w", req.Tool, err)
	}

	var invErr *string
	if req.InvocationError != nil {
		invErr = ptr.Ref(req.InvocationError.Error())
	}

	_, err = t.client.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		InterceptionId:  req.InterceptionID,
		MsgId:           req.MsgID,
		ServerUrl:       req.ServerURL,
		Tool:            req.Tool,
		Input:           string(serialized),
		Injected:        req.Injected,
		InvocationError: invErr,
		Metadata:        marshalForProto(req.Metadata),
		CreatedAt:       timestamppb.New(req.CreatedAt),
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

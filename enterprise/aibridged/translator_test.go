package aibridged //nolint:testpackage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/aibridge"
	abpb "github.com/coder/coder/v2/enterprise/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

type mockClient struct {
	abpb.DRPCRecorderClient
	got *abpb.RecordInterceptionRequest
}

func (mc *mockClient) RecordInterception(ctx context.Context, in *abpb.RecordInterceptionRequest) (*abpb.RecordInterceptionResponse, error) {
	mc.got = in
	return &abpb.RecordInterceptionResponse{}, nil
}

func mustAnypbNew(t *testing.T, src proto.Message) *anypb.Any {
	ret, err := anypb.New(src)
	require.NoError(t, err)
	return ret
}

func TestRecordInterception(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		apiKeyID  string
		userAgent string
		in        aibridge.InterceptionRecord
		expect    *abpb.RecordInterceptionRequest
	}{
		{
			name:      "simple",
			apiKeyID:  "key",
			userAgent: "user-agent",
			in: aibridge.InterceptionRecord{
				ID:          uuid.UUID{1}.String(),
				InitiatorID: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata:    map[string]any{"some": "data"},
				StartedAt:   time.UnixMicro(123),
			},
			expect: &abpb.RecordInterceptionRequest{
				Id:          uuid.UUID{1}.String(),
				ApiKeyId:    "key",
				InitiatorId: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata: map[string]*anypb.Any{
					"some":           mustAnypbNew(t, structpb.NewStringValue("data")),
					MetaKeyUserAgent: mustAnypbNew(t, structpb.NewStringValue("user-agent")),
				},
				StartedAt: timestamppb.New(time.UnixMicro(123)),
			},
		},
		{
			name:     "empty-user-agent",
			apiKeyID: "key",
			in: aibridge.InterceptionRecord{
				ID:          uuid.UUID{1}.String(),
				InitiatorID: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata:    map[string]any{"some": "data"},
				StartedAt:   time.UnixMicro(123),
			},
			expect: &abpb.RecordInterceptionRequest{
				Id:          uuid.UUID{1}.String(),
				ApiKeyId:    "key",
				InitiatorId: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata: map[string]*anypb.Any{
					"some": mustAnypbNew(t, structpb.NewStringValue("data")),
				},
				StartedAt: timestamppb.New(time.UnixMicro(123)),
			},
		},
		{
			name:      "overrides-user-agent",
			apiKeyID:  "key",
			userAgent: "user-agent",
			in: aibridge.InterceptionRecord{
				ID:          uuid.UUID{1}.String(),
				InitiatorID: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata:    map[string]any{MetaKeyUserAgent: "key-already-set"},
				StartedAt:   time.UnixMicro(123),
			},
			expect: &abpb.RecordInterceptionRequest{
				Id:          uuid.UUID{1}.String(),
				ApiKeyId:    "key",
				InitiatorId: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata: map[string]*anypb.Any{
					MetaKeyUserAgent: mustAnypbNew(t, structpb.NewStringValue("user-agent")),
				},
				StartedAt: timestamppb.New(time.UnixMicro(123)),
			},
		},
		{
			name:      "user-agent-empty-metadata",
			apiKeyID:  "key",
			userAgent: "user-agent",
			in: aibridge.InterceptionRecord{
				ID:          uuid.UUID{1}.String(),
				InitiatorID: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				StartedAt:   time.UnixMicro(123),
			},
			expect: &abpb.RecordInterceptionRequest{
				Id:          uuid.UUID{1}.String(),
				ApiKeyId:    "key",
				InitiatorId: uuid.UUID{2}.String(),
				Provider:    "prov",
				Model:       "model",
				Metadata: map[string]*anypb.Any{
					MetaKeyUserAgent: mustAnypbNew(t, structpb.NewStringValue("user-agent")),
				},
				StartedAt: timestamppb.New(time.UnixMicro(123)),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			mc := &mockClient{}
			rt := &recorderTranslation{
				apiKeyID:  tc.apiKeyID,
				client:    mc,
				userAgent: tc.userAgent,
			}
			rt.RecordInterception(ctx, &tc.in)
			require.Equal(t, tc.expect, mc.got)
		})
	}
}

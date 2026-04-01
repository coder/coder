package chatdebug //nolint:testpackage // Uses unexported debug-model helpers.

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

type scriptedModel struct {
	provider      string
	model         string
	generateFn    func(context.Context, fantasy.Call) (*fantasy.Response, error)
	streamFn      func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)
	generateObjFn func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error)
	streamObjFn   func(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error)
}

func (s *scriptedModel) Generate(
	ctx context.Context,
	call fantasy.Call,
) (*fantasy.Response, error) {
	if s.generateFn == nil {
		return &fantasy.Response{}, nil
	}
	return s.generateFn(ctx, call)
}

func (s *scriptedModel) Stream(
	ctx context.Context,
	call fantasy.Call,
) (fantasy.StreamResponse, error) {
	if s.streamFn == nil {
		return fantasy.StreamResponse(func(func(fantasy.StreamPart) bool) {}), nil
	}
	return s.streamFn(ctx, call)
}

func (s *scriptedModel) GenerateObject(
	ctx context.Context,
	call fantasy.ObjectCall,
) (*fantasy.ObjectResponse, error) {
	if s.generateObjFn == nil {
		return &fantasy.ObjectResponse{}, nil
	}
	return s.generateObjFn(ctx, call)
}

func (s *scriptedModel) StreamObject(
	ctx context.Context,
	call fantasy.ObjectCall,
) (fantasy.ObjectStreamResponse, error) {
	if s.streamObjFn == nil {
		return fantasy.ObjectStreamResponse(func(func(fantasy.ObjectStreamPart) bool) {}), nil
	}
	return s.streamObjFn(ctx, call)
}

func (s *scriptedModel) Provider() string { return s.provider }
func (s *scriptedModel) Model() string    { return s.model }

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func TestDebugModel_Provider(t *testing.T) {
	t.Parallel()

	inner := &stubModel{provider: "provider-a", model: "model-a"}
	model := &debugModel{inner: inner}

	require.Equal(t, inner.Provider(), model.Provider())
}

func TestDebugModel_Model(t *testing.T) {
	t.Parallel()

	inner := &stubModel{provider: "provider-a", model: "model-a"}
	model := &debugModel{inner: inner}

	require.Equal(t, inner.Model(), model.Model())
}

func TestDebugModel_Disabled(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)

	svc := NewService(db, testutil.Logger(t), nil)
	respWant := &fantasy.Response{FinishReason: fantasy.FinishReasonStop}
	inner := &scriptedModel{
		generateFn: func(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
			_, ok := StepFromContext(ctx)
			require.False(t, ok)
			require.Nil(t, attemptSinkFromContext(ctx))
			return respWant, nil
		},
	}

	model := &debugModel{
		inner: inner,
		svc:   svc,
		opts: RecorderOptions{
			ChatID:  chatID,
			OwnerID: ownerID,
		},
	}

	resp, err := model.Generate(context.Background(), fantasy.Call{})
	require.NoError(t, err)
	require.Same(t, respWant, resp)
}

func TestDebugModel_Generate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()

	call := fantasy.Call{
		Prompt:          fantasy.Prompt{fantasy.NewUserMessage("hello")},
		MaxOutputTokens: int64Ptr(128),
		Temperature:     float64Ptr(0.25),
	}
	respWant := &fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.TextContent{Text: "hello"},
			fantasy.ToolCallContent{ToolCallID: "tool-1", ToolName: "tool", Input: `{}`},
			fantasy.SourceContent{ID: "source-1", Title: "docs", URL: "https://example.com"},
		},
		FinishReason: fantasy.FinishReasonStop,
		Usage:        fantasy.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14},
		Warnings:     []fantasy.CallWarning{{Message: "warning"}},
	}

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)
	db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.InsertChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, runID, params.RunID)
			require.EqualValues(t, 1, params.StepNumber)
			require.Equal(t, string(OperationGenerate), params.Operation)
			require.Equal(t, string(StatusInProgress), params.Status)
			require.JSONEq(t, `{"messages":[{"role":"user","parts":[{"type":"text","text":"hello","text_length":5}]}],"options":{"max_output_tokens":128,"temperature":0.25},"provider_option_count":0}`,
				string(params.NormalizedRequest.RawMessage))
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)
	db.EXPECT().UpdateChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, stepID, params.ID)
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, string(StatusCompleted), params.Status.String)
			require.True(t, params.NormalizedResponse.Valid)
			require.JSONEq(t, `{"content":[{"type":"text","text":"hello"},{"type":"tool-call","tool_call_id":"tool-1","tool_name":"tool","arguments":"{}","input_length":2},{"type":"source","title":"docs","url":"https://example.com"}],"finish_reason":"stop","usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0},"warnings":[{"type":"","message":"warning"}]}`,
				string(params.NormalizedResponse.RawMessage))
			require.True(t, params.Usage.Valid)
			require.JSONEq(t, `{"input_tokens":10,"output_tokens":4,"total_tokens":14,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}`,
				string(params.Usage.RawMessage))
			require.True(t, params.Attempts.Valid)
			require.JSONEq(t, `[]`, string(params.Attempts.RawMessage))
			require.False(t, params.Error.Valid)
			require.False(t, params.Metadata.Valid)
			require.True(t, params.FinishedAt.Valid)
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &scriptedModel{
		generateFn: func(ctx context.Context, got fantasy.Call) (*fantasy.Response, error) {
			require.Equal(t, call, got)
			stepCtx, ok := StepFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, runID, stepCtx.RunID)
			require.Equal(t, stepID, stepCtx.StepID)
			require.NotNil(t, attemptSinkFromContext(ctx))
			return respWant, nil
		},
	}

	model := &debugModel{
		inner: inner,
		svc:   svc,
		opts:  RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.Generate(ctx, call)
	require.NoError(t, err)
	require.Same(t, respWant, resp)
}

func TestDebugModel_GeneratePersistsAttemptsWithoutResponseClose(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"message":"hello","api_key":"super-secret"}`,
			string(body))
		require.Equal(t, "Bearer top-secret", req.Header.Get("Authorization"))

		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("X-API-Key", "response-secret")
		rw.WriteHeader(http.StatusCreated)
		_, _ = rw.Write([]byte(`{"token":"response-secret","safe":"ok"}`))
	}))
	defer server.Close()

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)
	db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).Return(
		database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil,
	)
	db.EXPECT().UpdateChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, stepID, params.ID)
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, string(StatusCompleted), params.Status.String)
			require.True(t, params.Attempts.Valid)

			var attempts []Attempt
			require.NoError(t, json.Unmarshal(params.Attempts.RawMessage, &attempts))
			require.Len(t, attempts, 1)
			require.Equal(t, 1, attempts[0].Number)
			require.Equal(t, RedactedValue, attempts[0].RequestHeaders["Authorization"])
			require.JSONEq(t,
				`{"message":"hello","api_key":"[REDACTED]"}`,
				string(attempts[0].RequestBody),
			)
			require.Equal(t, http.StatusCreated, attempts[0].ResponseStatus)
			require.Equal(t, "application/json", attempts[0].ResponseHeaders["Content-Type"])
			require.Equal(t, RedactedValue, attempts[0].ResponseHeaders["X-Api-Key"])
			require.JSONEq(t,
				`{"token":"[REDACTED]","safe":"ok"}`,
				string(attempts[0].ResponseBody),
			)
			require.Empty(t, attempts[0].Error)
			require.GreaterOrEqual(t, attempts[0].DurationMs, int64(0))
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &scriptedModel{
		generateFn: func(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
			client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}
			req, err := http.NewRequestWithContext(
				ctx,
				http.MethodPost,
				server.URL,
				strings.NewReader(`{"message":"hello","api_key":"super-secret"}`),
			)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer top-secret")
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"token":"response-secret","safe":"ok"}`, string(body))
			require.NoError(t, resp.Body.Close())
			return &fantasy.Response{FinishReason: fantasy.FinishReasonStop}, nil
		},
	}

	model := &debugModel{
		inner: inner,
		svc:   svc,
		opts:  RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.Generate(ctx, fantasy.Call{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestDebugModel_GenerateError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	wantErr := &testError{message: "boom"}

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)
	db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).Return(
		database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil,
	)
	db.EXPECT().UpdateChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, string(StatusError), params.Status.String)
			require.False(t, params.NormalizedResponse.Valid)
			require.False(t, params.Usage.Valid)
			require.True(t, params.Error.Valid)
			require.JSONEq(t, `{"message":"boom","type":"*chatdebug.testError"}`,
				string(params.Error.RawMessage))
			require.False(t, params.Metadata.Valid)
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			generateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
				return nil, wantErr
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.Generate(ctx, fantasy.Call{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)
}

func TestDebugModel_Stream(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	errPart := xerrors.New("chunk failed")
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hel"},
		{Type: fantasy.StreamPartTypeToolCall, ID: "tool-call-1", ToolCallName: "tool"},
		{Type: fantasy.StreamPartTypeSource, ID: "source-1", URL: "https://example.com", Title: "docs"},
		{Type: fantasy.StreamPartTypeWarnings, Warnings: []fantasy.CallWarning{{Message: "w1"}, {Message: "w2"}}},
		{Type: fantasy.StreamPartTypeError, Error: errPart},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 8, OutputTokens: 3, TotalTokens: 11}},
	}

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)
	db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).Return(
		database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil,
	)
	db.EXPECT().UpdateChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, string(StatusCompleted), params.Status.String)
			require.True(t, params.NormalizedResponse.Valid)
			require.JSONEq(t, `{"content":[{"type":"text","text":"hel"},{"type":"tool-call","tool_call_id":"tool-call-1","tool_name":"tool"}],"finish_reason":"stop","usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}}`,
				string(params.NormalizedResponse.RawMessage))
			require.True(t, params.Usage.Valid)
			require.JSONEq(t, `{"input_tokens":8,"output_tokens":3,"total_tokens":11,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}`,
				string(params.Usage.RawMessage))
			require.True(t, params.Metadata.Valid)
			require.JSONEq(t, `{"stream_summary":{"finish_reason":"stop","text_delta_count":1,"tool_call_count":1,"source_count":1,"warning_count":2,"error_count":1,"last_error":"chunk failed","part_count":6}}`,
				string(params.Metadata.RawMessage))
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamFn: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				stepCtx, ok := StepFromContext(ctx)
				require.True(t, ok)
				require.Equal(t, stepID, stepCtx.StepID)
				require.NotNil(t, attemptSinkFromContext(ctx))
				return partsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.Stream(ctx, fantasy.Call{})
	require.NoError(t, err)

	got := make([]fantasy.StreamPart, 0, len(parts))
	for part := range seq {
		got = append(got, part)
	}

	require.Equal(t, parts, got)
}

func TestDebugModel_StreamObject(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	parts := []fantasy.ObjectStreamPart{
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ob"},
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ject"},
		{Type: fantasy.ObjectStreamPartTypeObject, Object: map[string]any{"value": "object"}},
		{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}},
	}

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)
	db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).Return(
		database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil,
	)
	db.EXPECT().UpdateChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, string(StatusCompleted), params.Status.String)
			require.True(t, params.NormalizedResponse.Valid)
			require.JSONEq(t, `{"content":[{"type":"text","text":"object"}],"finish_reason":"stop","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}}`,
				string(params.NormalizedResponse.RawMessage))
			require.True(t, params.Usage.Valid)
			require.JSONEq(t, `{"input_tokens":5,"output_tokens":2,"total_tokens":7,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}`,
				string(params.Usage.RawMessage))
			require.True(t, params.Metadata.Valid)
			require.JSONEq(t, `{"structured_output":true,"stream_summary":{"finish_reason":"stop","object_part_count":1,"text_delta_count":2,"error_count":0,"warning_count":0,"part_count":4,"structured_output":true}}`,
				string(params.Metadata.RawMessage))
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamObjFn: func(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
				stepCtx, ok := StepFromContext(ctx)
				require.True(t, ok)
				require.Equal(t, stepID, stepCtx.StepID)
				require.NotNil(t, attemptSinkFromContext(ctx))
				return objectPartsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.StreamObject(ctx, fantasy.ObjectCall{})
	require.NoError(t, err)

	got := make([]fantasy.ObjectStreamPart, 0, len(parts))
	for part := range seq {
		got = append(got, part)
	}

	require.Equal(t, parts, got)
}

func TestDebugModel_StreamEarlyStop(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	stepID := uuid.New()
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "first"},
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "second"},
	}

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID:                       chatID,
		DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
	}, nil)
	db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).Return(
		database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil,
	)
	db.EXPECT().UpdateChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, string(StatusInterrupted), params.Status.String)
			require.True(t, params.NormalizedResponse.Valid)
			require.JSONEq(t, `{"content":[{"type":"text","text":"first"}],"finish_reason":"","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}}`,
				string(params.NormalizedResponse.RawMessage))
			require.False(t, params.Usage.Valid)
			require.True(t, params.Metadata.Valid)
			require.JSONEq(t, `{"stream_summary":{"text_delta_count":1,"tool_call_count":0,"source_count":0,"warning_count":0,"error_count":0,"part_count":1}}`,
				string(params.Metadata.RawMessage))
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		},
	)

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
				return partsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.Stream(ctx, fantasy.Call{})
	require.NoError(t, err)

	count := 0
	for part := range seq {
		require.Equal(t, parts[0], part)
		count++
		break
	}
	require.Equal(t, 1, count)
}

func objectPartsToSeq(parts []fantasy.ObjectStreamPart) fantasy.ObjectStreamResponse {
	return func(yield func(fantasy.ObjectStreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	}
}

func partsToSeq(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	}
}

func int64Ptr(v int64) *int64 { return &v }

func float64Ptr(v float64) *float64 { return &v }

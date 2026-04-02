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

func expectDebugLoggingOverride(
	t *testing.T,
	db *dbmock.MockStore,
	chatID uuid.UUID,
	enabled bool,
) {
	t.Helper()

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
		ID: chatID,
		DebugLogsEnabledOverride: sql.NullBool{
			Bool:  enabled,
			Valid: true,
		},
	}, nil)
}

func expectCreateStepNumberWithRequestValidity(
	t *testing.T,
	db *dbmock.MockStore,
	runID uuid.UUID,
	chatID uuid.UUID,
	stepNumber int32,
	op Operation,
	normalizedRequestValid bool,
) uuid.UUID {
	t.Helper()

	stepID := uuid.New()
	db.EXPECT().
		InsertChatDebugStep(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatDebugStepParams{})).
		DoAndReturn(func(_ context.Context, params database.InsertChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, runID, params.RunID)
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, stepNumber, params.StepNumber)
			require.Equal(t, string(op), params.Operation)
			require.Equal(t, string(StatusInProgress), params.Status)
			require.Equal(t, normalizedRequestValid, params.NormalizedRequest.Valid)

			return database.ChatDebugStep{
				ID:         stepID,
				RunID:      runID,
				ChatID:     chatID,
				StepNumber: params.StepNumber,
				Operation:  params.Operation,
				Status:     params.Status,
			}, nil
		})

	return stepID
}

func expectCreateStepNumber(
	t *testing.T,
	db *dbmock.MockStore,
	runID uuid.UUID,
	chatID uuid.UUID,
	stepNumber int32,
	op Operation,
) uuid.UUID {
	t.Helper()

	return expectCreateStepNumberWithRequestValidity(
		t,
		db,
		runID,
		chatID,
		stepNumber,
		op,
		true,
	)
}

func expectCreateStep(
	t *testing.T,
	db *dbmock.MockStore,
	runID uuid.UUID,
	chatID uuid.UUID,
	op Operation,
) uuid.UUID {
	t.Helper()

	return expectCreateStepNumber(t, db, runID, chatID, 1, op)
}

func expectUpdateStep(
	t *testing.T,
	db *dbmock.MockStore,
	stepID uuid.UUID,
	chatID uuid.UUID,
	status Status,
	assertFn func(database.UpdateChatDebugStepParams),
) {
	t.Helper()

	db.EXPECT().
		UpdateChatDebugStep(gomock.Any(), gomock.AssignableToTypeOf(database.UpdateChatDebugStepParams{})).
		DoAndReturn(func(_ context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, stepID, params.ID)
			require.Equal(t, chatID, params.ChatID)
			require.True(t, params.Status.Valid)
			require.Equal(t, string(status), params.Status.String)
			require.True(t, params.FinishedAt.Valid)

			if assertFn != nil {
				assertFn(params)
			}

			return database.ChatDebugStep{
				ID:     stepID,
				ChatID: chatID,
				Status: params.Status.String,
			}, nil
		})
}

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

	expectDebugLoggingOverride(t, db, chatID, true)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.False(t, params.Error.Valid)
		require.False(t, params.Metadata.Valid)
	})

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &scriptedModel{
		generateFn: func(ctx context.Context, got fantasy.Call) (*fantasy.Response, error) {
			require.Equal(t, call, got)
			stepCtx, ok := StepFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, runID, stepCtx.RunID)
			require.Equal(t, chatID, stepCtx.ChatID)
			require.Equal(t, int32(1), stepCtx.StepNumber)
			require.Equal(t, OperationGenerate, stepCtx.Operation)
			require.NotEqual(t, uuid.Nil, stepCtx.StepID)
			require.NotNil(t, attemptSinkFromContext(ctx))
			return respWant, nil
		},
	}

	model := &debugModel{
		inner: inner,
		svc:   svc,
		opts:  RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
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

	expectDebugLoggingOverride(t, db, chatID, true)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.Attempts.Valid)
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)

		var attempts []Attempt
		require.NoError(t, json.Unmarshal(params.Attempts.RawMessage, &attempts))
		require.Len(t, attempts, 1)
		require.Equal(t, attemptStatusCompleted, attempts[0].Status)
		require.Equal(t, http.StatusCreated, attempts[0].ResponseStatus)
	})

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
	t.Cleanup(func() { CleanupStepCounter(runID) })
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
	wantErr := &testError{message: "boom"}

	expectDebugLoggingOverride(t, db, chatID, true)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.False(t, params.NormalizedResponse.Valid)
		require.False(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.True(t, params.Error.Valid)
		require.False(t, params.Metadata.Valid)
	})

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
	t.Cleanup(func() { CleanupStepCounter(runID) })
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.Generate(ctx, fantasy.Call{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)
}

func TestStepStatusForError(t *testing.T) {
	t.Parallel()

	t.Run("Canceled", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusInterrupted, stepStatusForError(context.Canceled))
	})

	t.Run("DeadlineExceeded", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusInterrupted, stepStatusForError(context.DeadlineExceeded))
	})

	t.Run("OtherError", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusError, stepStatusForError(xerrors.New("boom")))
	})
}

func TestDebugModel_Stream(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	errPart := xerrors.New("chunk failed")
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hel"},
		{Type: fantasy.StreamPartTypeToolCall, ID: "tool-call-1", ToolCallName: "tool"},
		{Type: fantasy.StreamPartTypeSource, ID: "source-1", URL: "https://example.com", Title: "docs"},
		{Type: fantasy.StreamPartTypeWarnings, Warnings: []fantasy.CallWarning{{Message: "w1"}, {Message: "w2"}}},
		{Type: fantasy.StreamPartTypeError, Error: errPart},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 8, OutputTokens: 3, TotalTokens: 11}},
	}

	expectDebugLoggingOverride(t, db, chatID, true)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.True(t, params.Error.Valid)
		require.True(t, params.Metadata.Valid)
	})

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamFn: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				stepCtx, ok := StepFromContext(ctx)
				require.True(t, ok)
				require.Equal(t, runID, stepCtx.RunID)
				require.Equal(t, chatID, stepCtx.ChatID)
				require.Equal(t, int32(1), stepCtx.StepNumber)
				require.Equal(t, OperationStream, stepCtx.Operation)
				require.NotEqual(t, uuid.Nil, stepCtx.StepID)
				require.NotNil(t, attemptSinkFromContext(ctx))
				return partsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
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
	parts := []fantasy.ObjectStreamPart{
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ob"},
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ject"},
		{Type: fantasy.ObjectStreamPartTypeObject, Object: map[string]any{"value": "object"}},
		{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}},
	}

	expectDebugLoggingOverride(t, db, chatID, true)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.False(t, params.Error.Valid)
		require.True(t, params.Metadata.Valid)
	})

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamObjFn: func(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
				stepCtx, ok := StepFromContext(ctx)
				require.True(t, ok)
				require.Equal(t, runID, stepCtx.RunID)
				require.Equal(t, chatID, stepCtx.ChatID)
				require.Equal(t, int32(1), stepCtx.StepNumber)
				require.Equal(t, OperationStream, stepCtx.Operation)
				require.NotEqual(t, uuid.Nil, stepCtx.StepID)
				require.NotNil(t, attemptSinkFromContext(ctx))
				return objectPartsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.StreamObject(ctx, fantasy.ObjectCall{})
	require.NoError(t, err)

	got := make([]fantasy.ObjectStreamPart, 0, len(parts))
	for part := range seq {
		got = append(got, part)
	}

	require.Equal(t, parts, got)
}

func TestDebugModel_StreamRejectsNilSequence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
				var nilStream fantasy.StreamResponse
				return nilStream, nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: uuid.New()},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })

	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(_ database.UpdateChatDebugStepParams) {})

	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.Stream(ctx, fantasy.Call{})
	require.Nil(t, seq)
	require.EqualError(t, err, "chatdebug: language model returned nil stream response")
}

func TestDebugModel_StreamObjectRejectsNilSequence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &scriptedModel{
			streamObjFn: func(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
				var nilStream fantasy.ObjectStreamResponse
				return nilStream, nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: uuid.New()},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })

	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(_ database.UpdateChatDebugStepParams) {})

	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.StreamObject(ctx, fantasy.ObjectCall{})
	require.Nil(t, seq)
	require.EqualError(t, err, "chatdebug: language model returned nil object stream response")
}

func TestDebugModel_StreamEarlyStop(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "first"},
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "second"},
	}

	expectDebugLoggingOverride(t, db, chatID, true)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusInterrupted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.False(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.False(t, params.Error.Valid)
		require.True(t, params.Metadata.Valid)
	})

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
	t.Cleanup(func() { CleanupStepCounter(runID) })
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

func TestStreamErrorStatus(t *testing.T) {
	t.Parallel()

	t.Run("CancellationBecomesInterrupted", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusInterrupted, streamErrorStatus(StatusCompleted, context.Canceled))
	})

	t.Run("DeadlineExceededBecomesInterrupted", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusInterrupted, streamErrorStatus(StatusCompleted, context.DeadlineExceeded))
	})

	t.Run("NilErrorBecomesError", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusError, streamErrorStatus(StatusCompleted, nil))
	})

	t.Run("ExistingErrorWins", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, StatusError, streamErrorStatus(StatusError, context.Canceled))
	})
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

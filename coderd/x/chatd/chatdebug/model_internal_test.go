package chatdebug

import (
	"context"
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

	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func TestDebugModel_Provider(t *testing.T) {
	t.Parallel()

	inner := &chattest.FakeModel{ProviderName: "provider-a", ModelName: "model-a"}
	model := &debugModel{inner: inner}

	require.Equal(t, inner.Provider(), model.Provider())
}

func TestDebugModel_Model(t *testing.T) {
	t.Parallel()

	inner := &chattest.FakeModel{ProviderName: "provider-a", ModelName: "model-a"}
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
	inner := &chattest.FakeModel{
		GenerateFn: func(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
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

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &chattest.FakeModel{
		GenerateFn: func(ctx context.Context, got fantasy.Call) (*fantasy.Response, error) {
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

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &chattest.FakeModel{
		GenerateFn: func(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
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

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
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

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamFn: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
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

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamObjectFn: func(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
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

// TestDebugModel_StreamCompletedAfterFinish verifies that when a consumer
// stops iteration after receiving a finish part, the step is marked as
// completed rather than interrupted.
func TestDebugModel_StreamCompletedAfterFinish(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}

	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 5, OutputTokens: 1, TotalTokens: 6}},
	}
	seq := wrapStreamSeq(context.Background(), handle, partsToSeq(parts))

	// Consumer reads through the finish part then breaks. The wrapper
	// should finalize as completed, not interrupted.
	for part := range seq {
		if part.Type == fantasy.StreamPartTypeFinish {
			break
		}
	}

	handle.mu.Lock()
	status := handle.status
	handle.mu.Unlock()
	require.Equal(t, StatusCompleted, status)
}

// TestDebugModel_StreamInterruptedBeforeFinish verifies that when a consumer
// stops iteration before receiving a finish part, the step is marked as
// interrupted.
func TestDebugModel_StreamInterruptedBeforeFinish(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}

	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
		{Type: fantasy.StreamPartTypeTextDelta, Delta: " world"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
	}
	seq := wrapStreamSeq(context.Background(), handle, partsToSeq(parts))

	// Consumer reads the first delta then breaks before finish.
	count := 0
	for range seq {
		count++
		if count == 1 {
			break
		}
	}
	require.Equal(t, 1, count)

	handle.mu.Lock()
	status := handle.status
	handle.mu.Unlock()
	require.Equal(t, StatusInterrupted, status)
}

func TestDebugModel_StreamRejectsNilSequence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
				var nilStream fantasy.StreamResponse
				return nilStream, nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: uuid.New()},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.Stream(ctx, fantasy.Call{})
	require.Nil(t, seq)
	require.ErrorIs(t, err, ErrNilModelResult)
}

func TestDebugModel_StreamObjectRejectsNilSequence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamObjectFn: func(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
				var nilStream fantasy.ObjectStreamResponse
				return nilStream, nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: uuid.New()},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.StreamObject(ctx, fantasy.ObjectCall{})
	require.Nil(t, seq)
	require.ErrorIs(t, err, ErrNilModelResult)
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

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
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

func TestDebugModel_GenerateObject(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	call := fantasy.ObjectCall{
		Prompt:          fantasy.Prompt{fantasy.NewUserMessage("summarize")},
		SchemaName:      "Summary",
		MaxOutputTokens: int64Ptr(256),
	}
	respWant := &fantasy.ObjectResponse{
		RawText:      `{"title":"test"}`,
		FinishReason: fantasy.FinishReasonStop,
		Usage:        fantasy.Usage{InputTokens: 5, OutputTokens: 3, TotalTokens: 8},
	}

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &chattest.FakeModel{
		GenerateObjectFn: func(ctx context.Context, got fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			require.Equal(t, call, got)
			stepCtx, ok := StepFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, runID, stepCtx.RunID)
			require.Equal(t, chatID, stepCtx.ChatID)
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

	resp, err := model.GenerateObject(ctx, call)
	require.NoError(t, err)
	require.Same(t, respWant, resp)
}

func TestDebugModel_GenerateObjectError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	wantErr := &testError{message: "object boom"}

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
				return nil, wantErr
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: uuid.New()},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.GenerateObject(ctx, fantasy.ObjectCall{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)
}

func TestDebugModel_GenerateObjectRejectsNilResponse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
				return nil, nil //nolint:nilnil // Intentionally testing nil response handling.
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: uuid.New()},
	}
	t.Cleanup(func() { CleanupStepCounter(runID) })
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.GenerateObject(ctx, fantasy.ObjectCall{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrNilModelResult)
}

func TestWrapStreamSeq_CompletedNotDowngradedByCtxCancel(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}

	// Create a context that we cancel after the stream finishes.
	ctx, cancel := context.WithCancel(context.Background())

	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 5, OutputTokens: 1, TotalTokens: 6}},
	}
	seq := wrapStreamSeq(ctx, handle, partsToSeq(parts))

	//nolint:revive // Intentionally consuming iterator to trigger side-effects.
	for range seq {
	}

	// Cancel the context after the stream has been fully consumed
	// and finalized. The status should remain completed.
	cancel()

	handle.mu.Lock()
	status := handle.status
	handle.mu.Unlock()
	require.Equal(t, StatusCompleted, status)
}

func TestWrapObjectStreamSeq_CompletedNotDowngradedByCtxCancel(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}

	ctx, cancel := context.WithCancel(context.Background())

	parts := []fantasy.ObjectStreamPart{
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "obj"},
		{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 3, OutputTokens: 1, TotalTokens: 4}},
	}
	seq := wrapObjectStreamSeq(ctx, handle, objectPartsToSeq(parts))

	//nolint:revive // Intentionally consuming iterator to trigger side-effects.
	for range seq {
	}

	cancel()

	handle.mu.Lock()
	status := handle.status
	handle.mu.Unlock()
	require.Equal(t, StatusCompleted, status)
}

func TestWrapStreamSeq_DroppedStreamFinalizedOnCtxCancel(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
	}

	// Create the wrapped stream but never iterate it.
	_ = wrapStreamSeq(ctx, handle, partsToSeq(parts))

	// Cancel the context; the AfterFunc safety net should finalize
	// the step as interrupted.
	cancel()

	// AfterFunc fires asynchronously; give it a moment.
	require.Eventually(t, func() bool {
		handle.mu.Lock()
		defer handle.mu.Unlock()
		return handle.status == StatusInterrupted
	}, testutil.WaitShort, testutil.IntervalFast)
}

func int64Ptr(v int64) *int64 { return &v }

func float64Ptr(v float64) *float64 { return &v }

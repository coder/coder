package chatdebug

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func expectDebugLoggingEnabled(
	t *testing.T,
	db *dbmock.MockStore,
	ownerID uuid.UUID,
) {
	t.Helper()

	db.EXPECT().GetChatDebugLoggingAllowUsers(gomock.Any()).Return(true, nil)
	db.EXPECT().GetUserChatDebugLoggingEnabled(gomock.Any(), ownerID).Return(true, nil)
}

func expectCreateStepWithRequestValidity(
	t *testing.T,
	db *dbmock.MockStore,
	runID uuid.UUID,
	chatID uuid.UUID,
	op Operation,
	normalizedRequestValid bool,
) uuid.UUID {
	t.Helper()

	stepID := uuid.New()

	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
	db.EXPECT().AcquireLock(gomock.Any(), database.LockIDChatDebugRunSteps(runID)).Return(nil)
	db.EXPECT().
		InsertChatDebugStep(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatDebugStepParams{})).
		DoAndReturn(func(_ context.Context, params database.InsertChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, runID, params.RunID)
			require.Equal(t, chatID, params.ChatID)
			require.Zero(t, params.StepNumber, "the database numbers the step")
			require.Equal(t, string(op), params.Operation)
			require.Equal(t, string(StatusInProgress), params.Status)
			require.Equal(t, normalizedRequestValid, params.NormalizedRequest.Valid)

			return database.ChatDebugStep{
				ID:        stepID,
				RunID:     runID,
				ChatID:    chatID,
				Operation: params.Operation,
				Status:    params.Status,
			}, nil
		})

	return stepID
}

func expectCreateStep(
	t *testing.T,
	db *dbmock.MockStore,
	runID uuid.UUID,
	chatID uuid.UUID,
	op Operation,
) uuid.UUID {
	t.Helper()

	return expectCreateStepWithRequestValidity(t, db, runID, chatID, op, true)
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

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		// Clean successes (no prior error) leave the error column
		// as SQL NULL rather than sending jsonClear.
		require.False(t, params.Error.Valid)
		require.False(t, params.Metadata.Valid)

		// Verify actual JSON content so a broken tag or field
		// rename is caught rather than only checking .Valid.
		var usage fantasy.Usage
		require.NoError(t, json.Unmarshal(params.Usage.RawMessage, &usage))
		require.EqualValues(t, 10, usage.InputTokens)
		require.EqualValues(t, 4, usage.OutputTokens)
		require.EqualValues(t, 14, usage.TotalTokens)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(params.NormalizedResponse.RawMessage, &resp))
		require.Equal(t, "stop", resp["finish_reason"])
	})

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &chattest.FakeModel{
		GenerateFn: func(ctx context.Context, got fantasy.Call) (*fantasy.Response, error) {
			require.Equal(t, call, got)
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

	expectDebugLoggingEnabled(t, db, ownerID)
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

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.False(t, params.NormalizedResponse.Valid)
		require.False(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.True(t, params.Error.Valid)
		require.False(t, params.Metadata.Valid)

		var errPayload normalizedErrorPayload
		require.NoError(t, json.Unmarshal(params.Error.RawMessage, &errPayload))
		require.Equal(t, "boom", errPayload.Message)
		require.Equal(t, "*chatdebug.testError", errPayload.Type)
	})

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
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.Generate(ctx, fantasy.Call{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)
}

// TestDebugModel_GenerateRetryClearsError verifies that when a Generate
// call fails and is retried on the same reused step, a successful retry
// explicitly overwrites the stored error payload with JSONB null via
// the jsonClear sentinel.  Without this, COALESCE would preserve the
// stale error and AggregateRunSummary would flag the run as errored.
func TestDebugModel_GenerateRetryClearsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	wantErr := &testError{message: "transient"}

	// Allow enablement check twice, once per Generate call.
	db.EXPECT().GetChatDebugLoggingAllowUsers(gomock.Any()).Return(true, nil).Times(2)
	db.EXPECT().GetUserChatDebugLoggingEnabled(gomock.Any(), ownerID).Return(true, nil).Times(2)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)

	// First finalization: error.
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.Error.Valid, "error payload must be present on first (failed) finalization")
		require.NotEqual(t, json.RawMessage("null"), params.Error.RawMessage,
			"first finalization should carry the real error, not JSONB null")
	})

	// Second finalization: success with explicit error clear.
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.Error.Valid,
			"error field must be Valid (JSONB null) so COALESCE overwrites the previous error")
		require.JSONEq(t, "null", string(params.Error.RawMessage),
			"successful retry must send JSONB null to clear the stale error")
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
	})

	callCount := 0
	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				callCount++
				if callCount == 1 {
					return nil, wantErr
				}
				return &fantasy.Response{
					FinishReason: fantasy.FinishReasonStop,
					Usage:        fantasy.Usage{InputTokens: 5, OutputTokens: 2},
				}, nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}

	ctx := ReuseStep(ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID}))

	// First call: fails.
	resp, err := model.Generate(ctx, fantasy.Call{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, wantErr)

	// Second call: succeeds, reuses the same step and clears the error.
	resp, err = model.Generate(ctx, fantasy.Call{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 2, callCount)
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

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.True(t, params.Error.Valid)
		require.True(t, params.Metadata.Valid)

		// Verify usage JSON content matches the finish part.
		var usage normalizedUsage
		require.NoError(t, json.Unmarshal(params.Usage.RawMessage, &usage))
		require.EqualValues(t, 8, usage.InputTokens)
		require.EqualValues(t, 3, usage.OutputTokens)
		require.EqualValues(t, 11, usage.TotalTokens)

		// Verify the response payload captures the streamed content.
		var resp normalizedResponsePayload
		require.NoError(t, json.Unmarshal(params.NormalizedResponse.RawMessage, &resp))
		require.Equal(t, "stop", resp.FinishReason)
		require.NotEmpty(t, resp.Content, "stream response should capture content parts")
		require.Equal(t, "hel", resp.Content[0].Text, "stream response should capture the streamed text")

		// Verify error payload comes from the stream error part.
		var errPayload normalizedErrorPayload
		require.NoError(t, json.Unmarshal(params.Error.RawMessage, &errPayload))
		require.Equal(t, "chunk failed", errPayload.Message)

		// Verify metadata contains stream_summary.
		var meta map[string]any
		require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
		summary, ok := meta["stream_summary"].(map[string]any)
		require.True(t, ok, "metadata must contain stream_summary")
		require.EqualValues(t, 1, summary["text_delta_count"])
		require.EqualValues(t, 1, summary["tool_call_count"])
		require.EqualValues(t, 1, summary["source_count"])
		require.EqualValues(t, 1, summary["error_count"])
	})

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamFn: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
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
	parts := []fantasy.ObjectStreamPart{
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ob"},
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ject"},
		{Type: fantasy.ObjectStreamPartTypeObject, Object: map[string]any{"value": "object"}},
		{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}},
	}

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		// Clean successes (no prior error) leave the error column
		// as SQL NULL rather than sending jsonClear.
		require.False(t, params.Error.Valid)
		require.True(t, params.Metadata.Valid)

		// Verify usage JSON content matches the finish part.
		var usage normalizedUsage
		require.NoError(t, json.Unmarshal(params.Usage.RawMessage, &usage))
		require.EqualValues(t, 5, usage.InputTokens)
		require.EqualValues(t, 2, usage.OutputTokens)
		require.EqualValues(t, 7, usage.TotalTokens)

		// Verify the object response payload.
		var resp normalizedObjectResponsePayload
		require.NoError(t, json.Unmarshal(params.NormalizedResponse.RawMessage, &resp))
		require.Equal(t, "stop", resp.FinishReason)
		require.True(t, resp.StructuredOutput)
		// "ob" + "ject" = 6 runes.
		require.Equal(t, 6, resp.RawTextLength)

		// Verify metadata contains structured_output flag.
		var meta map[string]any
		require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
		require.Equal(t, true, meta["structured_output"])
		summary, ok := meta["stream_summary"].(map[string]any)
		require.True(t, ok, "metadata must contain stream_summary")
		require.EqualValues(t, 2, summary["text_delta_count"])
		require.EqualValues(t, 1, summary["object_part_count"])
	})

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamObjectFn: func(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
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

// TestDebugModel_StreamCompletedAfterFinish verifies that when a consumer
// stops iteration after receiving a finish part, the step is marked as
// completed rather than interrupted.
func TestDebugModel_StreamCompletedAfterFinish(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{InputTokens: 5, OutputTokens: 1, TotalTokens: 6}},
	}

	// The mock expectation for UpdateStep with StatusCompleted is the
	// assertion: if the wrapper chose StatusInterrupted instead, the
	// mock would reject the call.
	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, nil)

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return partsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.Stream(ctx, fantasy.Call{})
	require.NoError(t, err)

	// Consumer reads the finish part then breaks. This should still
	// be considered a completed stream, not interrupted.
	for part := range seq {
		if part.Type == fantasy.StreamPartTypeFinish {
			break
		}
	}
	// gomock verifies UpdateStep was called with StatusCompleted.
}

// TestDebugModel_StreamInterruptedBeforeFinish verifies that when a consumer
// stops iteration before receiving a finish part, the step is marked as
// interrupted.
func TestDebugModel_StreamInterruptedBeforeFinish(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
		{Type: fantasy.StreamPartTypeTextDelta, Delta: " world"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
	}

	// The mock expectation for UpdateStep with StatusInterrupted is the
	// assertion: breaking before the finish part means interrupted.
	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusInterrupted, nil)

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return partsToSeq(parts), nil
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	seq, err := model.Stream(ctx, fantasy.Call{})
	require.NoError(t, err)

	// Consumer reads the first delta then breaks before finish.
	count := 0
	for range seq {
		count++
		if count == 1 {
			break
		}
	}
	require.Equal(t, 1, count)
	// gomock verifies UpdateStep was called with StatusInterrupted.
}

func TestDebugModel_StreamRejectsNilSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		inner *chattest.FakeModel

		stream       func(context.Context, *debugModel) (bool, error)
		wantMetadata map[string]any
	}{
		{
			name: "Stream",
			inner: &chattest.FakeModel{
				StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
					var nilStream fantasy.StreamResponse
					return nilStream, nil
				},
			},
			stream: func(ctx context.Context, model *debugModel) (bool, error) {
				seq, err := model.Stream(ctx, fantasy.Call{})
				return seq == nil, err
			},
		},
		{
			name: "StreamObject",
			inner: &chattest.FakeModel{
				StreamObjectFn: func(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
					var nilStream fantasy.ObjectStreamResponse
					return nilStream, nil
				},
			},
			stream: func(ctx context.Context, model *debugModel) (bool, error) {
				seq, err := model.StreamObject(ctx, fantasy.ObjectCall{})
				return seq == nil, err
			},
			wantMetadata: map[string]any{"structured_output": true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := dbmock.NewMockStore(gomock.NewController(t))
			chatID, ownerID, runID := uuid.New(), uuid.New(), uuid.New()
			expectDebugLoggingEnabled(t, db, ownerID)
			stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
			expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
				require.False(t, params.NormalizedResponse.Valid)
				require.False(t, params.Usage.Valid)
				require.True(t, params.Attempts.Valid)
				require.True(t, params.Error.Valid)
				var errPayload normalizedErrorPayload
				require.NoError(t, json.Unmarshal(params.Error.RawMessage, &errPayload))
				require.Contains(t, errPayload.Message, "nil")

				require.Equal(t, tc.wantMetadata != nil, params.Metadata.Valid)
				if tc.wantMetadata != nil {
					var meta map[string]any
					require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
					require.Equal(t, tc.wantMetadata, meta)
				}
			})

			model := &debugModel{
				inner: tc.inner,
				svc:   NewService(db, testutil.Logger(t), nil),
				opts:  RecorderOptions{ChatID: chatID, OwnerID: ownerID},
			}
			ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

			gotNil, err := tc.stream(ctx, model)
			require.True(t, gotNil)
			require.ErrorIs(t, err, ErrNilModelResult)
		})
	}
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

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationStream)
	expectUpdateStep(t, db, stepID, chatID, StatusInterrupted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.False(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.False(t, params.Error.Valid)
		require.True(t, params.Metadata.Valid)

		// Verify that the partial response captures the single
		// consumed text delta.
		var resp normalizedResponsePayload
		require.NoError(t, json.Unmarshal(params.NormalizedResponse.RawMessage, &resp))
		require.NotEmpty(t, resp.Content)
		// Finish reason is empty because consumer stopped before
		// the finish part.
		require.Empty(t, resp.FinishReason)

		// Verify stream_summary reflects partial consumption.
		var meta map[string]any
		require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
		summary, ok := meta["stream_summary"].(map[string]any)
		require.True(t, ok, "metadata must contain stream_summary")
		require.EqualValues(t, 1, summary["text_delta_count"])
	})

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

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusCompleted, func(params database.UpdateChatDebugStepParams) {
		require.True(t, params.NormalizedResponse.Valid)
		require.True(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.False(t, params.Error.Valid)
		// GenerateObject always passes structured_output metadata.
		require.True(t, params.Metadata.Valid)

		// Verify usage JSON content.
		var usage normalizedUsage
		require.NoError(t, json.Unmarshal(params.Usage.RawMessage, &usage))
		require.EqualValues(t, 5, usage.InputTokens)
		require.EqualValues(t, 3, usage.OutputTokens)
		require.EqualValues(t, 8, usage.TotalTokens)

		// Verify the object response payload.
		var resp normalizedObjectResponsePayload
		require.NoError(t, json.Unmarshal(params.NormalizedResponse.RawMessage, &resp))
		require.Equal(t, "stop", resp.FinishReason)
		require.True(t, resp.StructuredOutput)
		// RawText is `{"title":"test"}` = 16 runes.
		require.Equal(t, 16, resp.RawTextLength)

		// Verify metadata contains structured_output flag.
		var meta map[string]any
		require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
		require.Equal(t, true, meta["structured_output"])
	})

	svc := NewService(db, testutil.Logger(t), nil)
	inner := &chattest.FakeModel{
		GenerateObjectFn: func(ctx context.Context, got fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
			require.Equal(t, call, got)
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

	resp, err := model.GenerateObject(ctx, call)
	require.NoError(t, err)
	require.Same(t, respWant, resp)
}

func TestDebugModel_GenerateObjectError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	ownerID := uuid.New()
	runID := uuid.New()
	wantErr := &testError{message: "object boom"}

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.False(t, params.NormalizedResponse.Valid)
		require.False(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.True(t, params.Error.Valid)
		// GenerateObject always passes structured_output metadata.
		require.True(t, params.Metadata.Valid)

		var errPayload normalizedErrorPayload
		require.NoError(t, json.Unmarshal(params.Error.RawMessage, &errPayload))
		require.Equal(t, "object boom", errPayload.Message)
		require.Equal(t, "*chatdebug.testError", errPayload.Type)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
		require.Equal(t, true, meta["structured_output"])
	})

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
				return nil, wantErr
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
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
	ownerID := uuid.New()
	runID := uuid.New()

	expectDebugLoggingEnabled(t, db, ownerID)
	stepID := expectCreateStep(t, db, runID, chatID, OperationGenerate)
	expectUpdateStep(t, db, stepID, chatID, StatusError, func(params database.UpdateChatDebugStepParams) {
		require.False(t, params.NormalizedResponse.Valid)
		require.False(t, params.Usage.Valid)
		require.True(t, params.Attempts.Valid)
		require.True(t, params.Error.Valid)
		// GenerateObject always passes structured_output metadata.
		require.True(t, params.Metadata.Valid)

		var errPayload normalizedErrorPayload
		require.NoError(t, json.Unmarshal(params.Error.RawMessage, &errPayload))
		require.Contains(t, errPayload.Message, "nil")

		var meta map[string]any
		require.NoError(t, json.Unmarshal(params.Metadata.RawMessage, &meta))
		require.Equal(t, true, meta["structured_output"])
	})

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateObjectFn: func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
				return nil, nil //nolint:nilnil // Intentionally testing nil response handling.
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, OwnerID: ownerID},
	}
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})

	resp, err := model.GenerateObject(ctx, fantasy.ObjectCall{})
	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrNilModelResult)
}

type streamWrapperParts[P any] struct {
	delta, finish, errorPart P
	withError                func(P, error) P
	isFinish                 func(P) bool
}

func TestStreamWrapperStatus(t *testing.T) {
	t.Parallel()

	usage := fantasy.Usage{InputTokens: 3, OutputTokens: 1, TotalTokens: 4, ReasoningTokens: 2, CacheCreationTokens: 5, CacheReadTokens: 6}
	t.Run("Stream", func(t *testing.T) {
		t.Parallel()
		runStreamWrapperStatusCases(t, wrapStreamSeq, streamWrapperParts[fantasy.StreamPart]{
			delta:     fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "hello"},
			finish:    fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: usage},
			errorPart: fantasy.StreamPart{Type: fantasy.StreamPartTypeError},
			withError: func(part fantasy.StreamPart, err error) fantasy.StreamPart {
				part.Error = err
				return part
			},
			isFinish: func(part fantasy.StreamPart) bool { return part.Type == fantasy.StreamPartTypeFinish },
		})
	})
	t.Run("StreamObject", func(t *testing.T) {
		t.Parallel()
		runStreamWrapperStatusCases(t, wrapObjectStreamSeq, streamWrapperParts[fantasy.ObjectStreamPart]{
			delta:     fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "obj"},
			finish:    fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: usage},
			errorPart: fantasy.ObjectStreamPart{Type: fantasy.ObjectStreamPartTypeError},
			withError: func(part fantasy.ObjectStreamPart, err error) fantasy.ObjectStreamPart {
				part.Error = err
				return part
			},
			isFinish: func(part fantasy.ObjectStreamPart) bool { return part.Type == fantasy.ObjectStreamPartTypeFinish },
		})
	})
}

func runStreamWrapperStatusCases[P any](
	t *testing.T,
	wrap func(context.Context, *stepHandle, iter.Seq[P]) iter.Seq[P],
	parts streamWrapperParts[P],
) {
	t.Helper()

	failed := parts.withError(parts.errorPart, xerrors.New("chunk failed"))
	usage := &normalizedUsage{InputTokens: 3, OutputTokens: 1, TotalTokens: 4, ReasoningTokens: 2, CacheCreationTokens: 5, CacheReadTokens: 6}
	finished := streamRecord{Status: StatusCompleted, FinishReason: "stop", Usage: usage, Summarized: true}
	failedThenFinished := streamRecord{Status: StatusError, FinishReason: "stop", Usage: usage, ErrorCount: 1, LastError: "chunk failed", HadError: true, Summarized: true}
	interrupted := streamRecord{Status: StatusInterrupted, Summarized: true}
	// The safety net finalizes without a response, usage or summary.
	abandoned := streamRecord{Status: StatusInterrupted}
	tests := []struct {
		name                string
		parts               []P
		stopAfter, cancelAt int
		dropped             bool
		want                streamRecord
	}{
		{name: "Finished", parts: []P{parts.delta, parts.finish}, want: finished},
		{name: "StoppedAfterFinish", parts: []P{parts.finish, parts.delta}, stopAfter: 1, want: finished},
		{name: "StoppedBeforeFinish", parts: []P{parts.delta, parts.finish}, stopAfter: 1, want: interrupted},
		{name: "EndedWithoutFinish", parts: []P{parts.delta}, want: interrupted},
		{name: "ErrorPart", parts: []P{failed, parts.finish}, want: failedThenFinished},
		{name: "ErrorOnOtherPart", parts: []P{parts.withError(parts.delta, xerrors.New("chunk failed")), parts.finish}, want: failedThenFinished},
		{name: "ErrorPartWithoutError", parts: []P{parts.errorPart}, want: streamRecord{Status: StatusError, ErrorCount: 1, LastError: "stream error part with nil error", HadError: true, Summarized: true}},
		{name: "StoppedAfterErrorPart", parts: []P{failed, parts.finish}, stopAfter: 1, want: streamRecord{Status: StatusError, ErrorCount: 1, LastError: "chunk failed", HadError: true, Summarized: true}},
		{name: "CanceledErrorPart", parts: []P{parts.withError(parts.errorPart, context.Canceled)}, want: streamRecord{Status: StatusInterrupted, ErrorCount: 1, LastError: "context canceled", HadError: true, Summarized: true}},
		{name: "CanceledWhileYieldingFinish", parts: []P{parts.delta, parts.finish}, cancelAt: 2, want: finished},
		{name: "CanceledBeforeFinish", parts: []P{parts.delta, parts.finish}, cancelAt: 1, want: abandoned},
		{name: "DroppedThenCanceled", parts: []P{parts.delta, parts.finish}, dropped: true, want: abandoned},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				handle, store := newRecordedStep(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cancelAndWait := func() {
					cancel()
					synctest.Wait()
				}

				seq := wrap(ctx, handle, slices.Values(tc.parts))
				if !tc.dropped {
					held := 0
					for range seq {
						held++
						if held == tc.cancelAt {
							cancelAndWait()
						}
						if held == tc.stopAfter {
							break
						}
					}
				}
				cancelAndWait()

				require.Equal(t, tc.want, recordedStream(t, handle, store))
			})
		})
	}

	// The consumer stops right after cancel, so the safety net and the
	// consumer race to finalize. Either may win, but only one may write.
	t.Run("StoppedRightAfterCancel", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			handle, store := newRecordedStep(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			seq := wrap(ctx, handle, slices.Values([]P{parts.delta, parts.finish}))
			cancel()
			for range seq {
				break
			}
			synctest.Wait()

			require.Equal(t, StatusInterrupted, recordedStream(t, handle, store).Status)
		})
	})
}

// stepWriteStore records the step updates that reach the database.
type stepWriteStore struct {
	database.Store
	mu      sync.Mutex
	updates []database.UpdateChatDebugStepParams
}

func (s *stepWriteStore) UpdateChatDebugStep(_ context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, params)
	return database.ChatDebugStep{ID: params.ID, ChatID: params.ChatID}, nil
}

func newRecordedStep(t *testing.T) (*stepHandle, *stepWriteStore) {
	t.Helper()

	store := &stepWriteStore{}
	return &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
		svc:     NewService(store, testutil.Logger(t), nil),
	}, store
}

type streamRecord struct {
	Status       Status
	FinishReason string
	// Usage is the stored usage column, nil when it is NULL.
	Usage      *normalizedUsage
	ErrorCount int
	LastError  string
	HadError   bool
	// Summarized is false when the safety net, not the wrapper,
	// finalized the step and recorded no stream summary.
	Summarized bool
}

func recordedStream(t *testing.T, handle *stepHandle, store *stepWriteStore) streamRecord {
	t.Helper()

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.updates, 1, "the step must be finalized exactly once")
	stored := store.updates[0]

	handle.mu.Lock()
	defer handle.mu.Unlock()
	var resp struct {
		FinishReason string          `json:"finish_reason"`
		Usage        normalizedUsage `json:"usage"`
	}
	var meta struct {
		Summary struct {
			FinishReason string `json:"finish_reason"`
			ErrorCount   int    `json:"error_count"`
			LastError    string `json:"last_error"`
		} `json:"stream_summary"`
	}
	for dst, src := range map[any]any{&resp: handle.response, &meta: handle.metadata} {
		raw, err := json.Marshal(src)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, dst))
	}
	require.Equal(t, resp.FinishReason, meta.Summary.FinishReason)
	var usage *normalizedUsage
	if stored.Usage.Valid {
		usage = &normalizedUsage{}
		require.NoError(t, json.Unmarshal(stored.Usage.RawMessage, usage))
		require.Equal(t, *usage, resp.Usage, "the response usage must match the stored usage")
	} else {
		require.Zero(t, resp.Usage)
	}
	return streamRecord{
		Status:       handle.status,
		FinishReason: resp.FinishReason,
		Usage:        usage,
		ErrorCount:   meta.Summary.ErrorCount,
		LastError:    meta.Summary.LastError,
		HadError:     handle.hadError,
		Summarized:   handle.metadata != nil,
	}
}

func int64Ptr(v int64) *int64 { return &v }

func float64Ptr(v float64) *float64 { return &v }

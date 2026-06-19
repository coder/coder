package chatdebug

import (
	"context"
	"testing"

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

// deferredModel builds an errors-only debugModel (FullRecording=false)
// whose context carries a lazy run ensurer returning a fixed run.
func deferredModel(
	t *testing.T,
	db *dbmock.MockStore,
	inner fantasy.LanguageModel,
	chatID, runID uuid.UUID,
) (*debugModel, context.Context) {
	t.Helper()

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: inner,
		svc:   svc,
		opts:  RecorderOptions{ChatID: chatID, FullRecording: false},
	}
	ctx := WithErrorRunEnsurer(context.Background(), func() (*RunContext, bool) {
		return &RunContext{RunID: runID, ChatID: chatID, Kind: KindChatTurn}, true
	})
	return model, ctx
}

// expectDeferredErrorStep asserts a single minimal error step is created
// and finalized with status=error and no normalized request.
func expectDeferredErrorStep(t *testing.T, db *dbmock.MockStore, runID, chatID uuid.UUID) {
	t.Helper()
	stepID := uuid.New()
	db.EXPECT().
		InsertChatDebugStep(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatDebugStepParams{})).
		DoAndReturn(func(_ context.Context, params database.InsertChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, runID, params.RunID)
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, string(StatusInProgress), params.Status)
			require.False(t, params.NormalizedRequest.Valid)
			return database.ChatDebugStep{
				ID:         stepID,
				RunID:      runID,
				ChatID:     chatID,
				StepNumber: params.StepNumber,
				Operation:  params.Operation,
				Status:     params.Status,
			}, nil
		})
	db.EXPECT().
		UpdateChatDebugStep(gomock.Any(), gomock.AssignableToTypeOf(database.UpdateChatDebugStepParams{})).
		DoAndReturn(func(_ context.Context, params database.UpdateChatDebugStepParams) (database.ChatDebugStep, error) {
			require.Equal(t, stepID, params.ID)
			require.Equal(t, string(StatusError), params.Status.String)
			require.True(t, params.Error.Valid)
			require.False(t, params.NormalizedResponse.Valid)
			require.False(t, params.Usage.Valid)
			require.False(t, params.Attempts.Valid)
			return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
		})
}

func TestDeferred_GenericGenerateCapturesMinimalStep(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	expectDeferredErrorStep(t, db, runID, chatID)

	inner := &chattest.FakeModel{
		GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
			return nil, xerrors.New("boom: unexpected failure")
		},
	}
	model, ctx := deferredModel(t, db, inner, chatID, runID)

	_, err := model.Generate(ctx, fantasy.Call{})
	require.Error(t, err)
}

func TestDeferred_NonGenericGeneratePersistsNothing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	// No InsertChatDebugStep expectation: any persistence fails the test.

	inner := &chattest.FakeModel{
		GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
			return nil, xerrors.New("401 unauthorized: invalid x-api-key")
		},
	}
	model, ctx := deferredModel(t, db, inner, chatID, runID)

	_, err := model.Generate(ctx, fantasy.Call{})
	require.Error(t, err)
}

func TestDeferred_SuccessPersistsNothing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()

	respWant := &fantasy.Response{FinishReason: fantasy.FinishReasonStop}
	inner := &chattest.FakeModel{
		GenerateFn: func(ctx context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			_, ok := StepFromContext(ctx)
			require.False(t, ok)
			return respWant, nil
		},
	}
	model, ctx := deferredModel(t, db, inner, chatID, runID)

	resp, err := model.Generate(ctx, fantasy.Call{})
	require.NoError(t, err)
	require.Same(t, respWant, resp)
}

func TestDeferred_CanceledPersistsNothing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()

	inner := &chattest.FakeModel{
		GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
			return nil, context.Canceled
		},
	}
	model, ctx := deferredModel(t, db, inner, chatID, runID)

	_, err := model.Generate(ctx, fantasy.Call{})
	require.Error(t, err)
}

func TestDeferred_NoEnsurerPersistsNothing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()

	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{
		inner: &chattest.FakeModel{
			GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
				return nil, xerrors.New("boom: unexpected failure")
			},
		},
		svc:  svc,
		opts: RecorderOptions{ChatID: chatID, FullRecording: false},
	}

	// No ensurer in context: nothing can be persisted.
	_, err := model.Generate(context.Background(), fantasy.Call{})
	require.Error(t, err)
}

func TestDeferred_RunCreatedAtMostOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()

	// Two failing generate calls share one lazily-created run. Both
	// steps persist, but the ensurer create func runs only once.
	expectDeferredErrorStep(t, db, runID, chatID)
	expectDeferredErrorStep(t, db, runID, chatID)

	inner := &chattest.FakeModel{
		GenerateFn: func(context.Context, fantasy.Call) (*fantasy.Response, error) {
			return nil, xerrors.New("boom: unexpected failure")
		},
	}
	svc := NewService(db, testutil.Logger(t), nil)
	model := &debugModel{inner: inner, svc: svc, opts: RecorderOptions{ChatID: chatID}}

	var createCalls int
	ctx := WithErrorRunEnsurer(context.Background(), func() (*RunContext, bool) {
		createCalls++
		return &RunContext{RunID: runID, ChatID: chatID, Kind: KindChatTurn}, true
	})

	_, err := model.Generate(ctx, fantasy.Call{})
	require.Error(t, err)
	_, err = model.Generate(ctx, fantasy.Call{})
	require.Error(t, err)
	require.Equal(t, 1, createCalls)
}

package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestQuickgenDebugRun(t *testing.T) {
	t.Parallel()

	const prompt = "fix the flaky heartbeat test"
	chat := database.Chat{
		ID:          uuid.New(),
		OwnerID:     uuid.New(),
		Title:       "Fix the flaky heartbeat test",
		TitleSource: database.ChatTitleSourceFallback,
	}
	hidden := mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, codersdk.ChatMessageText("model context"))
	hidden.ID = 3
	user := mustChatMessage(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, codersdk.ChatMessageText(prompt))
	user.ID = 5
	tip := hidden
	tip.ID = 8
	messages := []database.ChatMessage{hidden, user, tip}

	generators := []struct {
		name       string
		kind       chatdebug.RunKind
		trigger    int64
		historyTip int64
		provider   string
		model      string
		label      string
		object     map[string]any
		generate   func(ctx context.Context, t *testing.T, db *dbmock.MockStore, svc *chatdebug.Service, resolved resolvedModelCall) bool
	}{
		{
			name:       "TitleGeneration",
			kind:       chatdebug.KindTitleGeneration,
			trigger:    user.ID,
			historyTip: tip.ID,
			provider:   string(database.AIProviderTypeOpenaiCompat),
			model:      "configured-model",
			label:      prompt,
			object:     map[string]any{"title": "Flaky heartbeat test"},
			generate: func(ctx context.Context, t *testing.T, db *dbmock.MockStore, svc *chatdebug.Service, resolved resolvedModelCall) bool {
				db.EXPECT().UpdateChatTitleByID(gomock.Any(), gomock.Any()).
					Return(chatWithGeneratedTitle(chat, "Flaky heartbeat test"), nil).AnyTimes()
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				server := titleOverrideTestServer(db, logger)
				events := subscribeChatWatchEvents(t, server.pubsub, chat.OwnerID)
				server.maybeGenerateChatTitle(ctx, chat, messages, nil, resolved, logger, svc)
				return len(events) == 1
			},
		},
		{
			name:       "TurnStatusLabel",
			kind:       chatdebug.KindQuickgen,
			trigger:    21,
			historyTip: 34,
			provider:   "resolved-provider",
			model:      "resolved-model",
			label:      "Turn status label",
			object:     map[string]any{"label": "Fixed flaky test"},
			generate: func(ctx context.Context, t *testing.T, _ *dbmock.MockStore, svc *chatdebug.Service, resolved resolvedModelCall) bool {
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				return generateTurnStatusLabel(ctx, chat, database.ChatStatusWaiting, "All tests pass.", resolved, codersdk.DefaultChatMaxGenerationRetries, logger, svc, 21, 34) != ""
			},
		},
	}
	outcomes := []struct {
		name       string
		createErr  error
		modelErr   error
		wantStatus chatdebug.Status
	}{
		{name: "Completed", wantStatus: chatdebug.StatusCompleted},
		{name: "ModelFails", modelErr: xerrors.New("model failed"), wantStatus: chatdebug.StatusError},
		{name: "CreateFails", createErr: xerrors.New("insert failed")},
	}

	for _, gen := range generators {
		for _, outcome := range outcomes {
			t.Run(gen.name+"/"+outcome.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)
				db := dbmock.NewMockStore(gomock.NewController(t))
				svc := chatdebug.NewService(db, testutil.Logger(t), nil, chatdebug.WithAlwaysEnable(true))
				runID := uuid.New()

				db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, params database.InsertChatDebugRunParams) (database.ChatDebugRun, error) {
						params.StartedAt, params.UpdatedAt = sql.NullTime{}, sql.NullTime{}
						require.Equal(t, database.InsertChatDebugRunParams{
							ChatID:              chat.ID,
							TriggerMessageID:    sql.NullInt64{Int64: gen.trigger, Valid: true},
							HistoryTipMessageID: sql.NullInt64{Int64: gen.historyTip, Valid: true},
							Kind:                string(gen.kind),
							Status:              string(chatdebug.StatusInProgress),
							Provider:            sql.NullString{String: gen.provider, Valid: true},
							Model:               sql.NullString{String: gen.model, Valid: true},
							Summary:             pqtype.NullRawMessage{RawMessage: json.RawMessage(`{"first_message":"` + gen.label + `"}`), Valid: true},
						}, params)
						if outcome.createErr != nil {
							return database.ChatDebugRun{}, outcome.createErr
						}
						return database.ChatDebugRun{
							ID:                  runID,
							ChatID:              params.ChatID,
							TriggerMessageID:    params.TriggerMessageID,
							HistoryTipMessageID: params.HistoryTipMessageID,
							Kind:                params.Kind,
							Status:              params.Status,
							Provider:            params.Provider,
							Model:               params.Model,
						}, nil
					}).Times(1)
				if outcome.createErr == nil {
					db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil).Times(1)
					db.EXPECT().UpdateChatDebugRun(gomock.Any(), gomock.Any()).
						DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
							require.Equal(t, runID, params.ID)
							require.Equal(t, chat.ID, params.ChatID)
							require.Equal(t, sql.NullString{String: string(outcome.wantStatus), Valid: true}, params.Status)
							var summary map[string]any
							require.NoError(t, json.Unmarshal(params.Summary.RawMessage, &summary))
							require.Equal(t, gen.label, summary["first_message"])
							return database.ChatDebugRun{ID: runID, ChatID: chat.ID}, nil
						}).Times(1)
				}

				var runs []*chatdebug.RunContext
				model := &chattest.FakeModel{
					GenerateObjectFn: func(ctx context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
						if run, ok := chatdebug.RunFromContext(ctx); ok {
							runs = append(runs, run)
						}
						if outcome.modelErr != nil {
							return nil, outcome.modelErr
						}
						return &fantasy.ObjectResponse{Object: gen.object}, nil
					},
				}
				resolved := resolvedModelCall{
					model:            chatprovider.NewModel(model, nil),
					dbConfig:         database.ChatModelConfig{ID: uuid.New(), Model: "configured-model"},
					resolvedProvider: "resolved-provider",
					resolvedModel:    "resolved-model",
					route:            aiGatewayModelRoute{Provider: database.AIProvider{Type: database.AIProviderTypeOpenaiCompat}},
				}

				require.Equal(t, outcome.modelErr == nil, gen.generate(ctx, t, db, svc, resolved))
				if outcome.createErr != nil {
					require.Empty(t, runs)
					return
				}
				require.NotEmpty(t, runs)
				for _, run := range runs {
					require.Equal(t, chatdebug.RunContext{
						RunID:               runID,
						ChatID:              chat.ID,
						TriggerMessageID:    gen.trigger,
						HistoryTipMessageID: gen.historyTip,
						Provider:            gen.provider,
						Model:               gen.model,
					}, *run)
				}
			})
		}
	}
}

func TestPrepareManualTitleDebugRunCreateFailure(t *testing.T) {
	t.Parallel()

	db := dbmock.NewMockStore(gomock.NewController(t))
	db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).
		Return(database.ChatDebugRun{}, xerrors.New("insert failed")).Times(1)

	ctx, finish := prepareManualTitleDebugRun(testutil.Context(t, testutil.WaitLong),
		chatdebug.NewService(db, testutil.Logger(t), nil), database.Chat{ID: uuid.New()}, resolvedModelCall{}, nil)
	_, ok := chatdebug.RunFromContext(ctx)
	require.False(t, ok)
	// The mock expects no finalize calls.
	finish(nil)
}

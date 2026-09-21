package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestPlanMemoryConsolidation(t *testing.T) {
	t.Parallel()

	entries := []chattool.MemoryIndexEntry{{Name: "alpha"}, {Name: "alpha-copy"}, {Name: "beta"}, {Name: "stale"}}
	plan := planMemoryConsolidation(memoryConsolidation{
		Merge: []memoryConsolidationMerge{
			{Keep: " Alpha ", Drop: []string{"alpha-copy", "missing", "alpha"}},
			// beta is claimed here, so the later delete of it is dropped.
			{Keep: "beta", Drop: []string{"nothing-known"}},
		},
		Delete: []string{"stale", "beta", "alpha-copy", "unknown"},
	}, entries)

	require.Equal(t, []memoryConsolidationMerge{{Keep: "alpha", Drop: []string{"alpha-copy"}}}, plan.Merge)
	require.Equal(t, []string{"stale", "beta"}, plan.Delete, "a merge with no valid drops releases its kept name")
}

type memoryConsolidationStore struct {
	memories map[string]chattool.Memory
	locked   bool
}

func (s *memoryConsolidationStore) Get(_ context.Context, name string) (chattool.Memory, error) {
	memory, ok := s.memories[name]
	if !ok {
		return chattool.Memory{}, chattool.ErrMemoryNotFound
	}
	return memory, nil
}

func (s *memoryConsolidationStore) List(context.Context) ([]chattool.MemoryIndexEntry, error) {
	entries := make([]chattool.MemoryIndexEntry, 0, len(s.memories))
	for _, memory := range s.memories {
		entries = append(entries, chattool.MemoryIndexEntry{Name: memory.Name, Description: memory.Description})
	}
	return entries, nil
}

func (s *memoryConsolidationStore) Count(context.Context) (int64, error) {
	return int64(len(s.memories)), nil
}

func (s *memoryConsolidationStore) Upsert(_ context.Context, input chattool.MemoryInput) (chattool.Memory, error) {
	memory := chattool.Memory{Name: input.Name, Description: input.Description, Body: input.Body}
	s.memories[input.Name] = memory
	return memory, nil
}

func (s *memoryConsolidationStore) Insert(ctx context.Context, input chattool.MemoryInput) (chattool.Memory, error) {
	if _, exists := s.memories[input.Name]; exists {
		return chattool.Memory{}, chattool.ErrMemoryExists
	}
	return s.Upsert(ctx, input)
}

func (s *memoryConsolidationStore) Delete(_ context.Context, name string) error {
	if _, ok := s.memories[name]; !ok {
		return chattool.ErrMemoryNotFound
	}
	delete(s.memories, name)
	return nil
}

func (s *memoryConsolidationStore) Lock(context.Context) error {
	s.locked = true
	return nil
}

func (s *memoryConsolidationStore) InTx(fn func(chattool.MemoryStore) error) error { return fn(s) }

func TestApplyMemoryConsolidation(t *testing.T) {
	t.Parallel()

	t.Run("MergesVerbatimAndDeletes", func(t *testing.T) {
		t.Parallel()

		store := &memoryConsolidationStore{memories: map[string]chattool.Memory{
			"alpha":      {Name: "alpha", Description: "Alpha", Body: "first"},
			"alpha-copy": {Name: "alpha-copy", Description: "Copy", Body: "second"},
			"stale":      {Name: "stale", Body: "old"},
			"gone":       {Name: "gone", Body: "already deleted elsewhere"},
		}}
		delete(store.memories, "gone")

		deleted, merged, err := applyMemoryConsolidation(t.Context(), store, memoryConsolidation{
			Merge:  []memoryConsolidationMerge{{Keep: "alpha", Drop: []string{"alpha-copy", "vanished"}}},
			Delete: []string{"stale", "gone"},
		})
		require.NoError(t, err)
		require.Equal(t, 2, deleted)
		require.Equal(t, 1, merged)
		require.Equal(t, []string{"alpha"}, slices.Sorted(maps.Keys(store.memories)))
		require.Equal(t, "first\n\nsecond", store.memories["alpha"].Body)
		require.Equal(t, "Alpha", store.memories["alpha"].Description, "the kept description survives")
	})

	t.Run("LeavesDropsThatWouldOverflowTheBody", func(t *testing.T) {
		t.Parallel()

		big := strings.Repeat("x", chattool.MaxMemoryBodyBytes-10)
		store := &memoryConsolidationStore{memories: map[string]chattool.Memory{
			"alpha": {Name: "alpha", Body: big},
			"small": {Name: "small", Body: "tiny"},
			"large": {Name: "large", Body: strings.Repeat("y", 100)},
		}}

		_, merged, err := applyMemoryConsolidation(t.Context(), store, memoryConsolidation{
			Merge: []memoryConsolidationMerge{{Keep: "alpha", Drop: []string{"large", "small"}}},
		})
		require.NoError(t, err)
		require.Equal(t, 1, merged)
		require.Equal(t, []string{"alpha", "large"}, slices.Sorted(maps.Keys(store.memories)))
		require.Equal(t, big+"\n\ntiny", store.memories["alpha"].Body)
	})
}

func TestConsolidateMemories(t *testing.T) {
	t.Parallel()

	newProjectChat := func() database.Chat {
		return database.Chat{
			ID:                uuid.New(),
			OwnerID:           uuid.New(),
			OrganizationID:    uuid.New(),
			ProjectID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
			LastModelConfigID: uuid.New(),
		}
	}
	newServer := func(t *testing.T, db database.Store, roundTripper http.RoundTripper) *Server {
		t.Helper()
		return &Server{
			db:                       db,
			logger:                   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			clock:                    quartz.NewReal(),
			configCache:              newChatConfigCache(t.Context(), db, quartz.NewReal()),
			experiments:              codersdk.ExperimentsKnown,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(&aibridgeTestFactory{rt: roundTripper}),
		}
	}
	expectModelResolution := func(db *dbmock.MockStore, chat database.Chat) []*gomock.Call {
		providerID := uuid.New()
		config := database.ChatModelConfig{
			ID:             chat.LastModelConfigID,
			Model:          "gpt-4o-mini",
			Enabled:        true,
			OrganizationID: chat.OrganizationID,
			AIProviderID:   uuid.NullUUID{UUID: providerID, Valid: true},
		}
		return []*gomock.Call{
			db.EXPECT().GetChatGatewayAPIKey(gomock.Any(), database.GetChatGatewayAPIKeyParams{
				UserID:    chat.OwnerID,
				TokenName: GatewayTokenName(chat.OwnerID),
			}).Return(database.APIKey{
				ID:        uuid.NewString(),
				ExpiresAt: time.Now().Add(syntheticAPIKeyLifetime),
			}, nil),
			db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), chat.LastModelConfigID).Return(config, nil),
			db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(
				aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil,
			),
		}
	}
	objectResponse := func(t *testing.T, object any) *http.Response {
		t.Helper()
		objectJSON, err := json.Marshal(object)
		require.NoError(t, err)
		responseJSON, err := json.Marshal(map[string]any{
			"id":         "resp_memory_consolidation",
			"object":     "response",
			"created_at": 0,
			"status":     "completed",
			"model":      "gpt-4o-mini",
			"output": []map[string]any{{
				"id":   "msg_memory_consolidation",
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": string(objectJSON),
				}},
			}},
			"usage": map[string]any{
				"input_tokens":  1,
				"output_tokens": 1,
				"total_tokens":  2,
			},
		})
		require.NoError(t, err)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(responseJSON))),
		}
	}
	projectRows := func(names ...string) []database.GetChatProjectMemoriesByProjectIDRow {
		rows := make([]database.GetChatProjectMemoriesByProjectIDRow, len(names))
		for i, name := range names {
			rows[i] = database.GetChatProjectMemoriesByProjectIDRow{ChatProjectMemory: database.ChatProjectMemory{Name: name, Description: name + " description"}}
		}
		return rows
	}
	inTx := func(db *dbmock.MockStore) *gomock.Call {
		return db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
			func(fn func(database.Store) error, _ *database.TxOptions) error {
				return fn(db)
			},
		)
	}
	inOrder := func(calls []*gomock.Call) {
		args := make([]any, len(calls))
		for i, call := range calls {
			args[i] = call
		}
		gomock.InOrder(args...)
	}
	// expectCommon covers the scope check and count that every run starts with.
	expectCommon := func(db *dbmock.MockStore, chat database.Chat, count int64) []*gomock.Call {
		return []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{ID: chat.ProjectID.UUID, Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(count, nil),
		}
	}
	lockID := func(chat database.Chat) int64 {
		return database.GenLockID("chat-memory-consolidation:" + chat.ProjectID.UUID.String())
	}

	t.Run("BelowCapDoesNothing", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		inOrder(expectCommon(db, chat, chattool.MaxMemories-1))

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("RecentRunIsRateLimited", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		calls := expectCommon(db, chat, chattool.MaxMemories)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), lockID(chat)).Return(true, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{
				ID:                   chat.ProjectID.UUID,
				MemoryConsolidatedAt: sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true},
			}, nil),
		)
		inOrder(calls)

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("LockBusySkips", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		calls := expectCommon(db, chat, chattool.MaxMemories)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), lockID(chat)).Return(false, nil),
		)
		inOrder(calls)

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("ModelFailureStillCountsAsARun", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		server := newServer(t, db, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, xerrors.New("model unavailable")
		}))
		calls := expectCommon(db, chat, chattool.MaxMemories)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), lockID(chat)).Return(true, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{ID: chat.ProjectID.UUID}, nil),
			// The attempt is recorded before the model call so a failing
			// provider cannot be retried on every turn.
			db.EXPECT().UpdateChatProjectMemoryConsolidatedAt(gomock.Any(), gomock.Any()).Return(nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows("alpha"), nil),
		)
		calls = append(calls, expectModelResolution(db, chat)...)
		inOrder(calls)

		modelCtx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
		t.Cleanup(cancel)
		server.consolidateMemories(modelCtx, slogtest.Make(t, nil), chat)
	})

	t.Run("AppliesThePlanUnderTheScopeLock", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		var capturedPrompt string
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			capturedPrompt = string(body)
			response := objectResponse(t, map[string]any{
				"merge":  []map[string]any{{"keep": "alpha", "drop": []string{"alpha-copy"}}},
				"delete": []string{"stale", "not-a-memory"},
			})
			response.Request = req
			return response, nil
		}))
		row := func(name, body string) database.GetChatProjectMemoryByNameRow {
			return database.GetChatProjectMemoryByNameRow{ChatProjectMemory: database.ChatProjectMemory{Name: name, Description: name + " description", Body: body}}
		}
		byName := func(name string) database.GetChatProjectMemoryByNameParams {
			return database.GetChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: name}
		}

		calls := expectCommon(db, chat, chattool.MaxMemories)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), lockID(chat)).Return(true, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{ID: chat.ProjectID.UUID}, nil),
			db.EXPECT().UpdateChatProjectMemoryConsolidatedAt(gomock.Any(), gomock.AssignableToTypeOf(database.UpdateChatProjectMemoryConsolidatedAtParams{})).DoAndReturn(
				func(_ context.Context, params database.UpdateChatProjectMemoryConsolidatedAtParams) error {
					require.Equal(t, chat.ProjectID.UUID, params.ID)
					require.WithinDuration(t, time.Now(), params.MemoryConsolidatedAt, time.Minute)
					return nil
				},
			),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows("alpha", "alpha-copy", "stale"), nil),
		)
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			// The scope lock serializes the run with save_memory and extraction.
			db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil),
			db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), byName("alpha")).Return(row("alpha", "body-one"), nil),
			db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), byName("alpha-copy")).Return(row("alpha-copy", "body-two"), nil),
			// Upsert of an existing name runs the cap check that finds the row.
			inTx(db),
			db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil),
			db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), byName("alpha")).Return(row("alpha", "body-one"), nil),
			db.EXPECT().UpsertChatProjectMemoryByName(gomock.Any(), database.UpsertChatProjectMemoryByNameParams{
				ProjectID:      chat.ProjectID.UUID,
				OrganizationID: chat.OrganizationID,
				Name:           "alpha",
				Description:    "alpha description",
				Body:           "body-one\n\nbody-two",
				SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
				CreatedBy:      chat.OwnerID,
			}).Return(database.ChatProjectMemory{}, nil),
			db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: "alpha-copy"}).Return(nil),
			db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: "stale"}).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)

		require.Contains(t, capturedPrompt, "alpha-copy description")
		require.NotContains(t, capturedPrompt, "body-one", "only the index is sent, never bodies")
	})
}

func TestMaybeConsolidateMemoriesAsync(t *testing.T) {
	t.Parallel()

	t.Run("SkipsChatsWithoutMemory", func(t *testing.T) {
		t.Parallel()

		// No database is wired, so reaching the worker would panic.
		server := &Server{experiments: codersdk.ExperimentsKnown}
		projectID := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		server.maybeConsolidateMemoriesAsync(t.Context(), slogtest.Make(t, nil), database.Chat{ID: uuid.New(), ProjectID: projectID, ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true}})
		server.maybeConsolidateMemoriesAsync(t.Context(), slogtest.Make(t, nil), database.Chat{ID: uuid.New()})
		(&Server{}).maybeConsolidateMemoriesAsync(t.Context(), slogtest.Make(t, nil), database.Chat{ID: uuid.New(), ProjectID: projectID})
	})

	t.Run("RunsConsolidation", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := database.Chat{
			ID:             uuid.New(),
			OwnerID:        uuid.New(),
			OrganizationID: uuid.New(),
			ProjectID:      uuid.NullUUID{UUID: uuid.New(), Valid: true},
		}
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
		db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil)
		db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(5), nil)

		serverCtx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		server := &Server{
			ctx:                      serverCtx,
			db:                       db,
			logger:                   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			clock:                    quartz.NewReal(),
			configCache:              newChatConfigCache(t.Context(), db, quartz.NewReal()),
			experiments:              codersdk.ExperimentsKnown,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(&aibridgeTestFactory{}),
		}

		server.maybeConsolidateMemoriesAsync(t.Context(), slogtest.Make(t, nil), chat)
		server.drainInflight()
	})
}

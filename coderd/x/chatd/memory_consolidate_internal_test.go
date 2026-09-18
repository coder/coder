package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

func TestValidateMemoryConsolidationMutations(t *testing.T) {
	t.Parallel()

	now := time.Now()
	memories := []chattool.Memory{
		{Name: "old-a", Description: "Old A", Body: "A", UpdatedAt: now.Add(-48 * time.Hour)},
		{Name: "old-b", Description: "Old B", Body: "B", UpdatedAt: now.Add(-48 * time.Hour)},
		{Name: "fresh", Description: "Fresh", Body: "Fresh", UpdatedAt: now.Add(-time.Hour)},
	}
	proposed := []memoryConsolidationMutation{
		{Op: "merge", Into: "merged", From: []string{"old-a", "old-b"}, Description: "Merged", Body: "Merged body"},
		{Op: "update", Name: "fresh", Description: "Changed", Body: "Changed body"},
	}
	for range memoryConsolidationMaxMutations - 2 {
		proposed = append(proposed, memoryConsolidationMutation{Op: "delete", Name: "old-a"})
	}

	mutations := validateMemoryConsolidationMutations(proposed, memories, now)
	require.Len(t, mutations, 1)
	require.Equal(t, "merge", mutations[0].Op)
	require.NotContains(t, mutations, memoryConsolidationMutation{Op: "update", Name: "fresh", Description: "Changed", Body: "Changed body"})

	t.Run("AllowsOneSourceWhenTargetExists", func(t *testing.T) {
		t.Parallel()

		mutations := validateMemoryConsolidationMutations([]memoryConsolidationMutation{{
			Op:          "merge",
			Into:        "old-a",
			From:        []string{"old-b"},
			Description: "Merged",
			Body:        "Merged body",
		}}, memories, now)
		require.Len(t, mutations, 1)
	})

	t.Run("RequiresTwoSourcesWhenTargetIsNew", func(t *testing.T) {
		t.Parallel()

		mutations := validateMemoryConsolidationMutations([]memoryConsolidationMutation{{
			Op:          "merge",
			Into:        "merged",
			From:        []string{"old-a"},
			Description: "Merged",
			Body:        "Merged body",
		}}, memories, now)
		require.Empty(t, mutations)
	})

	t.Run("RequiresASourceWhenTargetExists", func(t *testing.T) {
		t.Parallel()

		// Every source is filtered out (unknown or the target itself), which
		// would leave a bare overwrite of the existing memory.
		mutations := validateMemoryConsolidationMutations([]memoryConsolidationMutation{{
			Op:          "merge",
			Into:        "old-a",
			From:        []string{"old-a", "missing"},
			Description: "Merged",
			Body:        "Merged body",
		}}, memories, now)
		require.Empty(t, mutations)
	})
}

type memoryConsolidationStore struct {
	memories map[string]chattool.Memory
	// limit, when set, rejects new names once len(memories) reaches it,
	// mirroring insertUnderCap.
	limit  int
	locked bool
}

func (s *memoryConsolidationStore) Get(_ context.Context, name string) (chattool.Memory, error) {
	memory, ok := s.memories[name]
	if !ok {
		return chattool.Memory{}, chattool.ErrMemoryNotFound
	}
	return memory, nil
}

func (s *memoryConsolidationStore) GetForUpdate(ctx context.Context, name string) (chattool.Memory, error) {
	return s.Get(ctx, name)
}

func (s *memoryConsolidationStore) Lock(context.Context) error {
	s.locked = true
	return nil
}

func (*memoryConsolidationStore) List(context.Context) ([]chattool.MemoryIndexEntry, error) {
	return nil, nil
}

func (*memoryConsolidationStore) ListFull(context.Context) ([]chattool.Memory, error) {
	return nil, nil
}

func (s *memoryConsolidationStore) Count(context.Context) (int64, error) {
	return int64(len(s.memories)), nil
}

func (s *memoryConsolidationStore) Upsert(_ context.Context, input chattool.MemoryInput) (chattool.Memory, error) {
	if _, exists := s.memories[input.Name]; !exists && s.limit > 0 && len(s.memories) >= s.limit {
		return chattool.Memory{}, chattool.ErrMemoryLimit
	}
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
	delete(s.memories, name)
	return nil
}
func (s *memoryConsolidationStore) InTx(fn func(chattool.MemoryStore) error) error { return fn(s) }

func TestApplyMemoryConsolidationMutations(t *testing.T) {
	t.Parallel()

	now := time.Now()
	snapshot := []chattool.Memory{{Name: "memory", Description: "Old", Body: "Old body", UpdatedAt: now.Add(-48 * time.Hour)}}
	store := &memoryConsolidationStore{memories: map[string]chattool.Memory{
		"memory": {Name: "memory", Description: "Old", Body: "Old body", UpdatedAt: now.Add(-47 * time.Hour)},
	}}

	applied, err := applyMemoryConsolidationMutations(t.Context(), store, []memoryConsolidationMutation{{
		Op:          "update",
		Name:        "memory",
		Description: "Consolidated",
		Body:        "Consolidated body",
	}}, snapshot, now)
	require.NoError(t, err)
	require.Empty(t, applied)
	require.Equal(t, "Old", store.memories["memory"].Description)
	require.True(t, store.locked, "the scope lock precedes row locks")

	t.Run("MergeIntoNewNameAtCapFreesSourcesFirst", func(t *testing.T) {
		t.Parallel()

		old := now.Add(-48 * time.Hour)
		snapshot := []chattool.Memory{
			{Name: "dup-a", Description: "A", Body: "A", UpdatedAt: old},
			{Name: "dup-b", Description: "B", Body: "B", UpdatedAt: old},
		}
		store := &memoryConsolidationStore{limit: 2, memories: map[string]chattool.Memory{
			"dup-a": snapshot[0],
			"dup-b": snapshot[1],
		}}

		applied, err := applyMemoryConsolidationMutations(t.Context(), store, []memoryConsolidationMutation{{
			Op:          "merge",
			Into:        "merged",
			From:        []string{"dup-a", "dup-b"},
			Description: "Merged",
			Body:        "Merged body",
		}}, snapshot, now)
		require.NoError(t, err)
		require.Len(t, applied, 1)
		require.Equal(t, []string{"merged"}, slices.Sorted(maps.Keys(store.memories)))
	})

	t.Run("SkipsMergeWhenTargetAppearedSinceSnapshot", func(t *testing.T) {
		t.Parallel()

		old := now.Add(-48 * time.Hour)
		snapshot := []chattool.Memory{
			{Name: "dup-a", Description: "A", Body: "A", UpdatedAt: old},
			{Name: "dup-b", Description: "B", Body: "B", UpdatedAt: old},
		}
		store := &memoryConsolidationStore{memories: map[string]chattool.Memory{
			"dup-a":  snapshot[0],
			"dup-b":  snapshot[1],
			"merged": {Name: "merged", Description: "Fresh", Body: "Written by a user meanwhile", UpdatedAt: now},
		}}

		applied, err := applyMemoryConsolidationMutations(t.Context(), store, []memoryConsolidationMutation{{
			Op:          "merge",
			Into:        "merged",
			From:        []string{"dup-a", "dup-b"},
			Description: "Merged",
			Body:        "Merged body",
		}}, snapshot, now)
		require.NoError(t, err)
		require.Empty(t, applied)
		require.Equal(t, "Fresh", store.memories["merged"].Description)
	})
}

func TestMemoryConsolidationCandidates(t *testing.T) {
	t.Parallel()

	now := time.Now()
	candidates := memoryConsolidationCandidates([]chattool.Memory{
		{Name: "newest", UpdatedAt: now.Add(-48 * time.Hour)},
		{Name: "protected", UpdatedAt: now.Add(-time.Hour)},
		{Name: "oldest", UpdatedAt: now.Add(-96 * time.Hour)},
	}, now)
	require.Equal(t, []string{"oldest", "newest"}, []string{candidates[0].Name, candidates[1].Name})
	require.Len(t, candidates, 2)
}

func TestFormatMemoryConsolidationInput(t *testing.T) {
	t.Parallel()

	input := formatMemoryConsolidationInput([]chattool.Memory{{Name: "memory", Description: "Description", Body: string(make([]byte, memoryConsolidationBodyBytes+1))}})
	require.LessOrEqual(t, len(input), memoryConsolidationInputBytes)
	require.Contains(t, input, `name="memory"`)
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
	newPersonalChat := func() database.Chat {
		return database.Chat{
			ID:                uuid.New(),
			OwnerID:           uuid.New(),
			OrganizationID:    uuid.New(),
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
	projectRows := func(memories []chattool.Memory) []database.GetChatProjectMemoriesByProjectIDRow {
		rows := make([]database.GetChatProjectMemoriesByProjectIDRow, len(memories))
		for i, memory := range memories {
			rows[i] = database.GetChatProjectMemoriesByProjectIDRow{ChatProjectMemory: database.ChatProjectMemory{
				Name:        memory.Name,
				Description: memory.Description,
				Body:        memory.Body,
				UpdatedAt:   memory.UpdatedAt,
			}}
		}
		return rows
	}
	personalRows := func(memories []chattool.Memory) []database.GetChatUserMemoriesByUserAndOrganizationRow {
		rows := make([]database.GetChatUserMemoriesByUserAndOrganizationRow, len(memories))
		for i, memory := range memories {
			rows[i] = database.GetChatUserMemoriesByUserAndOrganizationRow{ChatUserMemory: database.ChatUserMemory{
				Name:        memory.Name,
				Description: memory.Description,
				Body:        memory.Body,
				UpdatedAt:   memory.UpdatedAt,
			}}
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

	t.Run("SkipsBelowMinimumWithoutModelCall", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(5), nil),
		)

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("SkipsWhenDebounced", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{
				Status:    database.ChatMemoryConsolidationStatusSucceeded,
				StartedAt: time.Now().Add(-time.Hour),
			}, nil),
		)

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("SkipsWhenLockBusy", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(false, nil),
		)
		inOrder(calls)

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("AppliesMutationsAndPrunes", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		now := time.Now()
		memories := []chattool.Memory{
			{Name: "alpha", Description: "alpha description", Body: "alpha body", UpdatedAt: now.Add(-72 * time.Hour)},
			{Name: "alpha-copy", Description: "alpha copy description", Body: "alpha copy body", UpdatedAt: now.Add(-72 * time.Hour)},
			{Name: "alpha-secondary", Description: "alpha secondary description", Body: "alpha secondary body", UpdatedAt: now.Add(-72 * time.Hour)},
			{Name: "stale", Description: "stale description", Body: "stale body", UpdatedAt: now.Add(-72 * time.Hour)},
			{Name: "fresh", Description: "fresh description", Body: "fresh body", UpdatedAt: now.Add(-time.Hour)},
		}
		for i := 1; i <= 10; i++ {
			memories = append(memories, chattool.Memory{
				Name:        fmt.Sprintf("old-%d", i),
				Description: "old description",
				Body:        "old body",
				UpdatedAt:   now.Add(-72 * time.Hour),
			})
		}

		var capturedPrompt string
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			capturedPrompt = string(body)
			mutations := []map[string]any{
				{
					"op":          "merge",
					"name":        "",
					"into":        "alpha",
					"from":        []string{"alpha-copy", "alpha-secondary"},
					"description": "Combined alpha memory",
					"body":        "Combined alpha body.",
					"reason":      "",
				},
				{"op": "delete", "name": "stale", "into": "", "from": []string{}, "description": "", "body": "", "reason": "superseded"},
				{"op": "update", "name": "fresh", "into": "", "from": []string{}, "description": "Changed", "body": "Changed body.", "reason": ""},
			}
			for i := 1; i <= 10; i++ {
				mutations = append(mutations, map[string]any{"op": "delete", "name": fmt.Sprintf("old-%d", i), "into": "", "from": []string{}, "description": "", "body": "", "reason": ""})
			}
			response := objectResponse(t, map[string]any{"mutations": mutations})
			response.Request = req
			return response, nil
		}))
		record := database.ChatMemoryConsolidation{ID: uuid.New()}
		memoryByName := make(map[string]chattool.Memory, len(memories))
		for _, memory := range memories {
			memoryByName[memory.Name] = memory
		}
		lockedRow := func(memory chattool.Memory) database.ChatProjectMemory {
			return database.ChatProjectMemory{
				Name:        memory.Name,
				Description: memory.Description,
				Body:        memory.Body,
				UpdatedAt:   memory.UpdatedAt,
			}
		}
		forUpdate := func(name string) *gomock.Call {
			return db.EXPECT().GetChatProjectMemoryByNameForUpdate(gomock.Any(), database.GetChatProjectMemoryByNameForUpdateParams{ProjectID: chat.ProjectID.UUID, Name: name}).Return(lockedRow(memoryByName[name]), nil)
		}

		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(true, nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
			db.EXPECT().InsertChatMemoryConsolidation(gomock.Any(), database.InsertChatMemoryConsolidationParams{
				OrganizationID: chat.OrganizationID,
				ProjectID:      chat.ProjectID,
				Model:          "gpt-4o-mini",
				MemoriesBefore: 30,
			}).Return(record, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows(memories), nil),
			inTx(db),
			// The scope lock precedes every row lock, matching save_memory.
			db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil),
			forUpdate("alpha-copy"),
			forUpdate("alpha-secondary"),
			forUpdate("alpha"),
			// Sources are removed before the target is written so a merge at
			// the cap has room.
			db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: "alpha-copy"}).Return(nil),
			db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: "alpha-secondary"}).Return(nil),
			// The merge target is written through the cap-checking transaction,
			// which reuses the enclosing one.
			inTx(db),
			db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil),
			db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), database.GetChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: "alpha"}).Return(database.GetChatProjectMemoryByNameRow{}, nil),
			db.EXPECT().UpsertChatProjectMemoryByName(gomock.Any(), database.UpsertChatProjectMemoryByNameParams{
				ProjectID:      chat.ProjectID.UUID,
				OrganizationID: chat.OrganizationID,
				Name:           "alpha",
				Description:    "Combined alpha memory",
				Body:           "Combined alpha body.",
				SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
				CreatedBy:      chat.OwnerID,
			}).Return(database.ChatProjectMemory{}, nil),
			forUpdate("stale"),
			db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: "stale"}).Return(nil),
		)
		for i := 1; i <= 5; i++ {
			name := fmt.Sprintf("old-%d", i)
			calls = append(calls,
				forUpdate(name),
				db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{
					ProjectID: chat.ProjectID.UUID,
					Name:      fmt.Sprintf("old-%d", i),
				}).Return(nil))
		}
		calls = append(calls,
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(21), nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, record.ID, params.ID)
					require.Equal(t, database.ChatMemoryConsolidationStatusSucceeded, params.Status)
					require.Equal(t, int32(21), params.MemoriesAfter)
					require.Empty(t, params.Error)
					var journal []codersdk.ChatMemoryMutation
					require.NoError(t, json.Unmarshal(params.Mutations, &journal))
					require.LessOrEqual(t, len(journal), memoryConsolidationMaxMutations)
					require.Equal(t, []codersdk.ChatMemoryMutation{
						{Op: "merge", Into: "alpha", From: []string{"alpha-copy", "alpha-secondary"}},
						{Op: "delete", Name: "stale", Reason: "superseded"},
						{Op: "delete", Name: "old-1"},
						{Op: "delete", Name: "old-2"},
						{Op: "delete", Name: "old-3"},
						{Op: "delete", Name: "old-4"},
						{Op: "delete", Name: "old-5"},
					}, journal)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByProject(gomock.Any(), database.PruneChatMemoryConsolidationsByProjectParams{
				ProjectID: chat.ProjectID.UUID,
				KeepCount: memoryConsolidationKeepRecords,
			}).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)

		require.Contains(t, capturedPrompt, "alpha body")
		require.Contains(t, capturedPrompt, "alpha copy body")
		// Protected memories are never candidates, so they are not sent.
		require.NotContains(t, capturedPrompt, "fresh body")
		require.Contains(t, capturedPrompt, "last 24 hours")
		require.Contains(t, capturedPrompt, "at least two existing memories")
	})

	t.Run("ModelErrorFinishesFailed", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		server := newServer(t, db, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, xerrors.New("model unavailable")
		}))
		record := database.ChatMemoryConsolidation{ID: uuid.New()}
		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(true, nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
			db.EXPECT().InsertChatMemoryConsolidation(gomock.Any(), gomock.Any()).Return(record, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows([]chattool.Memory{{Name: "old", Description: "old", Body: "old", UpdatedAt: time.Now().Add(-48 * time.Hour)}}), nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, record.ID, params.ID)
					require.Equal(t, database.ChatMemoryConsolidationStatusFailed, params.Status)
					require.Equal(t, int32(30), params.MemoriesAfter)
					require.NotEmpty(t, params.Error)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByProject(gomock.Any(), database.PruneChatMemoryConsolidationsByProjectParams{
				ProjectID: chat.ProjectID.UUID,
				KeepCount: memoryConsolidationKeepRecords,
			}).Return(nil),
		)
		inOrder(calls)

		modelCtx, cancel := context.WithTimeout(t.Context(), testutil.WaitShort)
		t.Cleanup(cancel)
		server.consolidateMemories(modelCtx, slogtest.Make(t, nil), chat)
	})

	t.Run("ForcedAtCapIgnoresDebounce", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		var modelCalls int
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			modelCalls++
			response := objectResponse(t, map[string]any{"mutations": []any{}})
			response.Request = req
			return response, nil
		}))
		record := database.ChatMemoryConsolidation{ID: uuid.New()}
		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(chattool.MaxMemories), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{
				Status:         database.ChatMemoryConsolidationStatusSucceeded,
				StartedAt:      time.Now().Add(-time.Hour),
				MemoriesBefore: chattool.MaxMemories,
				MemoriesAfter:  chattool.MaxMemories - 4,
			}, nil),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(true, nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{
				Status:         database.ChatMemoryConsolidationStatusSucceeded,
				StartedAt:      time.Now().Add(-time.Hour),
				MemoriesBefore: chattool.MaxMemories,
				MemoriesAfter:  chattool.MaxMemories - 4,
			}, nil),
			db.EXPECT().InsertChatMemoryConsolidation(gomock.Any(), gomock.Any()).Return(record, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows([]chattool.Memory{{Name: "old", Description: "old", Body: "old", UpdatedAt: time.Now().Add(-72 * time.Hour)}}), nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, database.ChatMemoryConsolidationStatusSkipped, params.Status)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByProject(gomock.Any(), database.PruneChatMemoryConsolidationsByProjectParams{
				ProjectID: chat.ProjectID.UUID,
				KeepCount: memoryConsolidationKeepRecords,
			}).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
		require.Equal(t, 1, modelCalls)
	})

	t.Run("AtCapRunWithoutProgressStillDebounces", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		inOrder([]*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(chattool.MaxMemories), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{
				Status:         database.ChatMemoryConsolidationStatusSkipped,
				StartedAt:      time.Now().Add(-time.Hour),
				MemoriesBefore: chattool.MaxMemories,
				MemoriesAfter:  chattool.MaxMemories,
			}, nil),
		})

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("AtCapSkipsFreshRunningWithoutModelCall", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(chattool.MaxMemories), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{
				Status:    database.ChatMemoryConsolidationStatusRunning,
				StartedAt: time.Now().Add(-time.Minute),
			}, nil),
		)

		newServer(t, db, nil).consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("PersonalScopeUsesUserQueries", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newPersonalChat()
		latestParams := database.GetLatestChatMemoryConsolidationByUserParams{UserID: chat.OwnerID, OrganizationID: chat.OrganizationID}
		countParams := database.CountChatUserMemoriesByUserAndOrganizationParams{UserID: chat.OwnerID, OrganizationID: chat.OrganizationID}
		memoryParams := database.GetChatUserMemoriesByUserAndOrganizationParams{UserID: chat.OwnerID, OrganizationID: chat.OrganizationID}
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{"mutations": []any{}})
			response.Request = req
			return response, nil
		}))
		record := database.ChatMemoryConsolidation{ID: uuid.New()}
		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetUserChatPersonalMemoryEnabled(gomock.Any(), chat.OwnerID).Return("true", nil),
			db.EXPECT().CountChatUserMemoriesByUserAndOrganization(gomock.Any(), countParams).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByUser(gomock.Any(), latestParams).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(true, nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByUser(gomock.Any(), latestParams).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
			db.EXPECT().InsertChatMemoryConsolidation(gomock.Any(), database.InsertChatMemoryConsolidationParams{
				OrganizationID: chat.OrganizationID,
				UserID:         uuid.NullUUID{UUID: chat.OwnerID, Valid: true},
				Model:          "gpt-4o-mini",
				MemoriesBefore: 30,
			}).Return(record, nil),
			db.EXPECT().GetChatUserMemoriesByUserAndOrganization(gomock.Any(), memoryParams).Return(personalRows(nil), nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, database.ChatMemoryConsolidationStatusSkipped, params.Status)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByUser(gomock.Any(), database.PruneChatMemoryConsolidationsByUserParams{
				UserID:         chat.OwnerID,
				OrganizationID: chat.OrganizationID,
				KeepCount:      memoryConsolidationKeepRecords,
			}).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("NoMutationsFinishesSkipped", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{"mutations": []any{}})
			response.Request = req
			return response, nil
		}))
		record := database.ChatMemoryConsolidation{ID: uuid.New()}
		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(true, nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatMemoryConsolidation{}, sql.ErrNoRows),
			db.EXPECT().InsertChatMemoryConsolidation(gomock.Any(), gomock.Any()).Return(record, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, record.ID, params.ID)
					require.Equal(t, database.ChatMemoryConsolidationStatusSkipped, params.Status)
					require.Equal(t, int32(30), params.MemoriesAfter)
					require.Empty(t, params.Error)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByProject(gomock.Any(), database.PruneChatMemoryConsolidationsByProjectParams{
				ProjectID: chat.ProjectID.UUID,
				KeepCount: memoryConsolidationKeepRecords,
			}).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})
}

func TestMaybeConsolidateMemoriesAsync(t *testing.T) {
	t.Parallel()

	t.Run("ParentChatSkips", func(t *testing.T) {
		t.Parallel()

		server := &Server{experiments: codersdk.ExperimentsKnown}
		server.maybeConsolidateMemoriesAsync(t.Context(), slogtest.Make(t, nil), database.Chat{
			ID:           uuid.New(),
			ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
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

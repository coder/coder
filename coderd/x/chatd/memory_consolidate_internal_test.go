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

	mutations := validateMemoryConsolidationMutations(proposed, memories, memories, now)
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
		}}, memories, memories, now)
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
		}}, memories, memories, now)
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
		}}, memories, memories, now)
		require.Empty(t, mutations)
	})

	t.Run("RejectsMemoriesOutsideTheWindow", func(t *testing.T) {
		t.Parallel()

		// The full snapshot also holds old-c, but the model only saw old-a
		// and old-b, so nothing may touch old-c: not as a source, a target,
		// or a supposedly new merge name.
		window := []chattool.Memory{
			{Name: "old-a", Description: "Old A", Body: "A", UpdatedAt: now.Add(-48 * time.Hour)},
			{Name: "old-b", Description: "Old B", Body: "B", UpdatedAt: now.Add(-48 * time.Hour)},
		}
		snapshot := append([]chattool.Memory{{Name: "old-c", Description: "Old C", Body: "C", UpdatedAt: now.Add(-72 * time.Hour)}}, window...)
		mutations := validateMemoryConsolidationMutations([]memoryConsolidationMutation{
			{Op: "delete", Name: "old-c"},
			{Op: "merge", Into: "old-a", From: []string{"old-c"}, Description: "Merged", Body: "Merged body"},
			{Op: "merge", Into: "old-c", From: []string{"old-a", "old-b"}, Description: "Merged", Body: "Merged body"},
			{Op: "update", Name: "old-b", Description: "Changed", Body: "Changed body"},
		}, window, snapshot, now)
		require.Len(t, mutations, 1)
		require.Equal(t, "old-b", mutations[0].Name)
	})

	t.Run("RejectsOverlappingMutations", func(t *testing.T) {
		t.Parallel()

		mutations := validateMemoryConsolidationMutations([]memoryConsolidationMutation{
			{Op: "update", Name: "old-a", Description: "Changed", Body: "Changed body"},
			{Op: "merge", Into: "merged", From: []string{"old-a", "old-b"}, Description: "Merged", Body: "Merged body"},
			{Op: "delete", Name: "old-b"},
		}, memories, memories, now)
		require.Len(t, mutations, 2)
		require.Equal(t, "update", mutations[0].Op)
		// The merge lost old-a to the update, so old-b stays free for the
		// delete.
		require.Equal(t, "delete", mutations[1].Op)
	})

	t.Run("RejectsMergeIntoClaimedTarget", func(t *testing.T) {
		t.Parallel()

		mutations := validateMemoryConsolidationMutations([]memoryConsolidationMutation{
			{Op: "merge", Into: "merged", From: []string{"old-a", "old-b"}, Description: "Merged", Body: "Merged body"},
			{Op: "update", Name: "old-b", Description: "Changed", Body: "Changed body"},
		}, memories, memories, now)
		require.Len(t, mutations, 1)
		require.Equal(t, "merge", mutations[0].Op)
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

	// Bodies are replaced wholesale by updates and merges, so the model must
	// see every byte it may rewrite.
	body := strings.Repeat("x", 8*1024)
	input := formatMemoryConsolidationInput([]chattool.Memory{{Name: "memory", Description: "Description", Body: body}})
	require.Contains(t, input, `name="memory"`)
	require.Contains(t, input, body)
}

func TestMemoryConsolidationWindow(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 40*1024)
	candidates := []chattool.Memory{
		{Name: "first", Body: body},
		{Name: "second", Body: body},
		{Name: "third", Body: body},
	}

	window, next := memoryConsolidationWindow(candidates, 0)
	require.Equal(t, []string{"first", "second"}, []string{window[0].Name, window[1].Name})
	require.Len(t, window, 2)
	require.Equal(t, int32(2), next)
	for _, memory := range window {
		require.Equal(t, body, memory.Body, "bodies are never truncated")
	}

	window, next = memoryConsolidationWindow(candidates, next)
	require.Len(t, window, 1)
	require.Equal(t, "third", window[0].Name)
	require.Equal(t, int32(0), next, "the window wraps once it reaches the end")

	t.Run("OversizedCandidateStillFits", func(t *testing.T) {
		t.Parallel()

		window, next := memoryConsolidationWindow([]chattool.Memory{
			{Name: "huge", Body: strings.Repeat("x", memoryConsolidationInputBytes)},
			{Name: "small", Body: "small"},
		}, 0)
		require.Len(t, window, 1)
		require.Equal(t, "huge", window[0].Name)
		require.Equal(t, int32(1), next)
	})

	t.Run("StaleCursorRestarts", func(t *testing.T) {
		t.Parallel()

		window, next := memoryConsolidationWindow([]chattool.Memory{{Name: "only", Body: "only"}}, 5)
		require.Len(t, window, 1)
		require.Equal(t, int32(0), next)
	})
}

func TestMemoryConsolidationDebounced(t *testing.T) {
	t.Parallel()

	scope := memoryConsolidationScope{organizationID: uuid.New(), projectID: uuid.New()}
	now := time.Now()
	check := func(t *testing.T, record database.ChatMemoryConsolidation, count int64) (bool, int32) {
		t.Helper()
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), scope.projectID).Return(record, nil)
		return memoryConsolidationDebounced(t.Context(), db, scope, count, now, slogtest.Make(t, nil))
	}

	t.Run("AtCapPartialSkipContinues", func(t *testing.T) {
		t.Parallel()

		debounced, next := check(t, database.ChatMemoryConsolidation{
			Status:          database.ChatMemoryConsolidationStatusSkipped,
			StartedAt:       now.Add(-time.Minute),
			MemoriesBefore:  chattool.MaxMemories,
			MemoriesAfter:   chattool.MaxMemories,
			NextWindowStart: 7,
		}, chattool.MaxMemories)
		require.False(t, debounced)
		require.Equal(t, int32(7), next)
	})

	t.Run("AtCapFailureRetriesSooner", func(t *testing.T) {
		t.Parallel()

		failed := database.ChatMemoryConsolidation{
			Status:          database.ChatMemoryConsolidationStatusFailed,
			StartedAt:       now.Add(-time.Minute),
			NextWindowStart: 4,
		}
		debounced, next := check(t, failed, chattool.MaxMemories)
		require.True(t, debounced)
		require.Equal(t, int32(4), next)

		failed.StartedAt = now.Add(-memoryConsolidationFailureRetry - time.Minute)
		debounced, next = check(t, failed, chattool.MaxMemories)
		require.False(t, debounced)
		require.Equal(t, int32(4), next)

		debounced, _ = check(t, failed, 30)
		require.True(t, debounced, "below the cap a failure waits for the full debounce")
	})

	t.Run("AtCapFullSkipDebounces", func(t *testing.T) {
		t.Parallel()

		debounced, next := check(t, database.ChatMemoryConsolidation{
			Status:         database.ChatMemoryConsolidationStatusSkipped,
			StartedAt:      now.Add(-time.Minute),
			MemoriesBefore: chattool.MaxMemories,
			MemoriesAfter:  chattool.MaxMemories,
		}, chattool.MaxMemories)
		require.True(t, debounced)
		require.Equal(t, int32(0), next)
	})

	t.Run("BelowCapKeepsCursorWhileDebounced", func(t *testing.T) {
		t.Parallel()

		debounced, next := check(t, database.ChatMemoryConsolidation{
			Status:          database.ChatMemoryConsolidationStatusSkipped,
			StartedAt:       now.Add(-time.Minute),
			NextWindowStart: 3,
		}, 30)
		require.True(t, debounced)
		require.Equal(t, int32(3), next)
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
				ProjectID:      chat.ProjectID.UUID,
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
					require.Equal(t, int32(0), params.NextWindowStart, "every candidate fit in one window")
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

	t.Run("AllMutationsStaleFinishesSkipped", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		stale := chattool.Memory{Name: "stale", Description: "stale", Body: "stale", UpdatedAt: time.Now().Add(-72 * time.Hour)}
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{"mutations": []map[string]any{
				{"op": "delete", "name": "stale", "into": "", "from": []string{}, "description": "", "body": "", "reason": ""},
			}})
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
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows([]chattool.Memory{stale}), nil),
			inTx(db),
			db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil),
			// A user rewrote the memory after the snapshot, so the delete is
			// no longer current and nothing is applied.
			db.EXPECT().GetChatProjectMemoryByNameForUpdate(gomock.Any(), database.GetChatProjectMemoryByNameForUpdateParams{ProjectID: chat.ProjectID.UUID, Name: "stale"}).Return(database.ChatProjectMemory{Name: "stale", UpdatedAt: time.Now()}, nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, database.ChatMemoryConsolidationStatusSkipped, params.Status)
					require.Equal(t, int32(30), params.MemoriesAfter)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByProject(gomock.Any(), gomock.Any()).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("ModelErrorFinishesFailedAndKeepsCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		server := newServer(t, db, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, xerrors.New("model unavailable")
		}))
		record := database.ChatMemoryConsolidation{ID: uuid.New()}
		// The previous run stopped partway through; a failed retry must not
		// advance past the window it never got an answer for.
		previous := database.ChatMemoryConsolidation{
			Status:          database.ChatMemoryConsolidationStatusSkipped,
			StartedAt:       time.Now().Add(-48 * time.Hour),
			NextWindowStart: 1,
		}
		calls := []*gomock.Call{
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(30), nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(previous, nil),
		}
		calls = append(calls, expectModelResolution(db, chat)...)
		calls = append(calls,
			inTx(db),
			db.EXPECT().TryAcquireLock(gomock.Any(), memoryConsolidationScopeForChat(chat).lockID()).Return(true, nil),
			db.EXPECT().GetLatestChatMemoryConsolidationByProject(gomock.Any(), chat.ProjectID.UUID).Return(previous, nil),
			db.EXPECT().InsertChatMemoryConsolidation(gomock.Any(), gomock.Any()).Return(record, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows([]chattool.Memory{
				{Name: "old", Description: "old", Body: "old", UpdatedAt: time.Now().Add(-72 * time.Hour)},
				{Name: "older", Description: "older", Body: "older", UpdatedAt: time.Now().Add(-48 * time.Hour)},
			}), nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, record.ID, params.ID)
					require.Equal(t, database.ChatMemoryConsolidationStatusFailed, params.Status)
					require.Equal(t, int32(30), params.MemoriesAfter)
					require.NotEmpty(t, params.Error)
					require.Equal(t, int32(1), params.NextWindowStart)
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

	t.Run("PartialWindowRecordsResumeCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newProjectChat()
		body := strings.Repeat("x", 40*1024)
		memories := []chattool.Memory{
			{Name: "first", Description: "first", Body: body, UpdatedAt: time.Now().Add(-96 * time.Hour)},
			{Name: "second", Description: "second", Body: body, UpdatedAt: time.Now().Add(-72 * time.Hour)},
			{Name: "third", Description: "third", Body: body, UpdatedAt: time.Now().Add(-48 * time.Hour)},
		}
		var capturedPrompt string
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			capturedPrompt = string(requestBody)
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
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(projectRows(memories), nil),
			db.EXPECT().FinishChatMemoryConsolidation(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, params database.FinishChatMemoryConsolidationParams) (database.ChatMemoryConsolidation, error) {
					require.Equal(t, database.ChatMemoryConsolidationStatusSkipped, params.Status)
					require.Equal(t, int32(2), params.NextWindowStart)
					return database.ChatMemoryConsolidation{}, nil
				},
			),
			db.EXPECT().PruneChatMemoryConsolidationsByProject(gomock.Any(), gomock.Any()).Return(nil),
		)
		inOrder(calls)

		server.consolidateMemories(t.Context(), slogtest.Make(t, nil), chat)
		require.Contains(t, capturedPrompt, `name=\"first\"`)
		require.Contains(t, capturedPrompt, `name=\"second\"`)
		require.NotContains(t, capturedPrompt, `name=\"third\"`)
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

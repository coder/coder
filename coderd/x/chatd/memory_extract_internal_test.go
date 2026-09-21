package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestRenderMemoryTranscript(t *testing.T) {
	t.Parallel()

	message := func(t *testing.T, id int64, role database.ChatMessageRole, text string, revision int64) database.ChatMessage {
		t.Helper()
		encoded, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             id,
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: encoded.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       revision,
		}
	}

	messages := []database.ChatMessage{
		message(t, 1, database.ChatMessageRoleUser, "old user detail", 3),
		message(t, 2, database.ChatMessageRoleTool, "tool output", 5),
		message(t, 3, database.ChatMessageRoleAssistant, "assistant restatement", 5),
		message(t, 4, database.ChatMessageRoleUser, "new user detail", 5),
	}
	transcript := renderMemoryTranscript(messages, 3, 5)
	require.NotContains(t, transcript, "old user detail")
	require.NotContains(t, transcript, "tool output")
	require.NotContains(t, transcript, "assistant restatement")
	require.Contains(t, transcript, "new user detail")

	// A turn committed after the pass captured its history version waits
	// for the next pass instead of being sent twice.
	fenced := renderMemoryTranscript(append(messages,
		message(t, 5, database.ChatMessageRoleUser, "mid-pass detail", 8),
	), 3, 5)
	require.Contains(t, fenced, "new user detail")
	require.NotContains(t, fenced, "mid-pass detail")

	largeTranscript := renderMemoryTranscript([]database.ChatMessage{
		message(t, 5, database.ChatMessageRoleUser, strings.Repeat("界", memoryExtractionTranscriptMaxBytes)+"tail", 5),
	}, 3, 1<<62)
	require.LessOrEqual(t, len(largeTranscript), memoryExtractionTranscriptMaxBytes)
	require.True(t, utf8.ValidString(largeTranscript))
	require.True(t, strings.HasPrefix(largeTranscript, "[truncated] "))
	require.True(t, strings.HasSuffix(largeTranscript, "tail"))
}

func TestMemoryTurns(t *testing.T) {
	t.Parallel()

	message := func(t *testing.T, role database.ChatMessageRole, parts []codersdk.ChatMessagePart) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		return database.ChatMessage{
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: content.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       5,
		}
	}
	user := func(t *testing.T, text string) database.ChatMessage {
		return message(t, database.ChatMessageRoleUser, []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	}
	saveCall := func(t *testing.T, id string) database.ChatMessage {
		return message(t, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolCall(id, chattool.SaveMemoryToolName, []byte(`{}`)),
		})
	}
	saveResult := func(t *testing.T, id string, isError bool) database.ChatMessage {
		return message(t, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(id, chattool.SaveMemoryToolName, []byte(`{}`), isError, false),
		})
	}

	t.Run("SuccessfulSaveSuppressesOnlyItsTurn", func(t *testing.T) {
		t.Parallel()
		transcript := renderMemoryTranscript([]database.ChatMessage{
			user(t, "first turn"), saveCall(t, "save-1"), saveResult(t, "save-1", false),
			user(t, "second turn"),
		}, 3, 1<<62)
		require.NotContains(t, transcript, "first turn")
		require.Contains(t, transcript, "second turn")
	})

	t.Run("FailedSaveDoesNotSuppressExtraction", func(t *testing.T) {
		t.Parallel()
		transcript := renderMemoryTranscript([]database.ChatMessage{
			user(t, "first turn"), saveCall(t, "save-1"), saveResult(t, "save-1", true),
		}, 3, 1<<62)
		require.Contains(t, transcript, "first turn")
	})

	t.Run("EveryTurnSuppressedYieldsEmptyTranscript", func(t *testing.T) {
		t.Parallel()
		transcript := renderMemoryTranscript([]database.ChatMessage{
			user(t, "first turn"), saveCall(t, "save-1"), saveResult(t, "save-1", false),
		}, 3, 1<<62)
		require.Empty(t, transcript)
	})
}

func TestNormalizeMemoryExtraction(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeMemoryExtraction(memoryExtractionUpsert{
		Name:        "Release_Notes",
		Description: "<project-memory>Durable release process</project-memory>",
		Body:        "<project-memory>Run the checklist.</project-memory>",
	})
	require.NoError(t, err)
	require.Equal(t, "release_notes", normalized.Name)
	require.Equal(t, "Durable release process", normalized.Description)
	require.Equal(t, "Run the checklist.", normalized.Body)

	_, err = normalizeMemoryExtraction(memoryExtractionUpsert{
		Name:        "invalid name",
		Description: "Description",
		Body:        "Body",
	})
	require.Error(t, err)
}

func TestExtractMemories(t *testing.T) {
	t.Parallel()

	newChat := func() database.Chat {
		return database.Chat{
			ID:                uuid.New(),
			OwnerID:           uuid.New(),
			OrganizationID:    uuid.New(),
			ProjectID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
			LastModelConfigID: uuid.New(),
			HistoryVersion:    6,
		}
	}
	newServer := func(t *testing.T, db database.Store, roundTripper http.RoundTripper) *Server {
		t.Helper()
		return &Server{
			db:                       db,
			logger:                   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			clock:                    quartz.NewReal(),
			experiments:              codersdk.ExperimentsKnown,
			aibridgeTransportFactory: aibridgeTestFactoryPointer(&aibridgeTestFactory{rt: roundTripper}),
		}
	}
	message := func(t *testing.T, id int64, role database.ChatMessageRole, text string, revision int64) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             id,
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: content.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       revision,
		}
	}
	expectModelResolution := func(db *dbmock.MockStore, chat database.Chat) {
		providerID := uuid.New()
		config := database.ChatModelConfig{
			ID:             chat.LastModelConfigID,
			Model:          "gpt-4o-mini",
			Enabled:        true,
			OrganizationID: chat.OrganizationID,
			AIProviderID:   uuid.NullUUID{UUID: providerID, Valid: true},
		}
		db.EXPECT().GetChatGatewayAPIKey(gomock.Any(), database.GetChatGatewayAPIKeyParams{
			UserID:    chat.OwnerID,
			TokenName: GatewayTokenName(chat.OwnerID),
		}).Return(database.APIKey{
			ID:        uuid.NewString(),
			ExpiresAt: time.Now().Add(syntheticAPIKeyLifetime),
		}, nil)
		db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), chat.LastModelConfigID).Return(config, nil)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(
			aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil,
		)
	}
	objectResponse := func(t *testing.T, object any) *http.Response {
		t.Helper()
		objectJSON, err := json.Marshal(object)
		require.NoError(t, err)
		responseJSON, err := json.Marshal(map[string]any{
			"id":         "resp_project_memory",
			"object":     "response",
			"created_at": 0,
			"status":     "completed",
			"model":      "gpt-4o-mini",
			"output": []map[string]any{{
				"id":   "msg_project_memory",
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

	claimedUntil := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// expectClaim mirrors the claim and release that bracket every pass.
	expectClaim := func(t *testing.T, db *dbmock.MockStore, chat database.Chat, cursor int64) {
		t.Helper()
		db.EXPECT().ClaimChatMemoryExtraction(gomock.Any(), gomock.AssignableToTypeOf(database.ClaimChatMemoryExtractionParams{})).
			DoAndReturn(func(_ context.Context, arg database.ClaimChatMemoryExtractionParams) (database.ChatMemoryCursor, error) {
				assert.Equal(t, chat.ID, arg.ChatID)
				return database.ChatMemoryCursor{ChatID: chat.ID, HistoryVersion: cursor, ClaimedUntil: sql.NullTime{Time: claimedUntil, Valid: true}}, nil
			})
		db.EXPECT().ReleaseChatMemoryExtraction(gomock.Any(), database.ReleaseChatMemoryExtractionParams{ChatID: chat.ID, ClaimedUntil: claimedUntil}).Return(nil)
	}
	// expectInsertTx runs the transactional cap check against the same mock.
	expectInsertTx := func(db *dbmock.MockStore) {
		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(db)
		})
		db.EXPECT().AcquireLock(gomock.Any(), gomock.Any()).Return(nil)
	}
	expectProject := func(db *dbmock.MockStore, chat database.Chat) *gomock.Call {
		return db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil)
	}
	advanceCursor := func(db *dbmock.MockStore, chat database.Chat) *gomock.Call {
		return db.EXPECT().UpsertChatMemoryCursor(gomock.Any(), database.UpsertChatMemoryCursorParams{
			ChatID:         chat.ID,
			HistoryVersion: chat.HistoryVersion,
		}).Return(database.ChatMemoryCursor{}, nil)
	}

	t.Run("SkipsWhenCursorCurrent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		expectClaim(t, db, chat, chat.HistoryVersion)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("OutsideProjectAdvancesCursorWithoutModelCall", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		chat.ProjectID = uuid.NullUUID{}
		expectClaim(t, db, chat, 0)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
		// Consuming these turns keeps them out of any project the chat
		// joins later.
		advanceCursor(db, chat)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("ProjectLookupFailureKeepsCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		expectClaim(t, db, chat, 0)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
		db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{}, xerrors.New("connection reset"))

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("ExitsWhenAnotherExtractorHoldsTheClaim", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		db.EXPECT().ClaimChatMemoryExtraction(gomock.Any(), gomock.Any()).Return(database.ChatMemoryCursor{}, sql.ErrNoRows)

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("SkipsModelCallWhenAgentSavedMemoryThisTurn", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		encoded, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolCall("call-1", chattool.SaveMemoryToolName, []byte(`{"name":"x"}`)),
		})
		require.NoError(t, err)
		saveCall := database.ChatMessage{
			ID:             2,
			Role:           database.ChatMessageRoleAssistant,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: encoded.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       5,
		}
		resultContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult("call-1", chattool.SaveMemoryToolName, []byte(`{}`), false, false),
		})
		require.NoError(t, err)
		saveResult := database.ChatMessage{
			ID:             3,
			Role:           database.ChatMessageRoleTool,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: resultContent.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       5,
		}
		expectClaim(t, db, chat, 3)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "remember this", 5),
				saveCall,
				saveResult,
			}, nil),
			// The cursor advances without loading memories or calling the model.
			advanceCursor(db, chat),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
		)

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("AppliesUpsertsAndAdvancesCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		var capturedPrompt string
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			capturedPrompt = string(body)
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{
					{
						"name":        "Release_Notes",
						"description": "<project-memory>Durable release process</project-memory>",
						"body":        "<project-memory>Run the release checklist.</project-memory>",
					},
					{
						"name":        "Bad Name!",
						"description": "Ignored",
						"body":        "Ignored",
					},
				},
			})
			response.Request = req
			return response, nil
		}))
		validUpsert := database.InsertChatProjectMemoryParams{
			ID:             uuid.NullUUID{},
			ProjectID:      chat.ProjectID.UUID,
			OrganizationID: chat.OrganizationID,
			Name:           "release_notes",
			Description:    "Durable release process",
			Body:           "Run the release checklist.",
			SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
			CreatedBy:      chat.OwnerID,
		}

		expectClaim(t, db, chat, 3)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "old detail", 2),
				message(t, 2, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		expectInsertTx(db)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(0), nil),
			db.EXPECT().InsertChatProjectMemory(gomock.Any(), validUpsert).Return(database.ChatProjectMemory{}, nil),
			advanceCursor(db, chat),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)

		require.Contains(t, capturedPrompt, "new durable detail")
		require.NotContains(t, capturedPrompt, "old detail")
		// The extractor receives the guidance for the chat's scope.
		require.Contains(t, capturedPrompt, "people on this project")
		require.NotContains(t, capturedPrompt, "Do not save project details")
		// Deletion is intentionally absent from the extraction schema.
		require.NotContains(t, capturedPrompt, `"deletes"`)
	})

	t.Run("DrainsTurnsCompletedDuringExtraction", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{"upserts": []map[string]any{}})
			response.Request = req
			return response, nil
		}))
		advanced := chat
		advanced.HistoryVersion = chat.HistoryVersion + 2

		// First pass: cursor 3 to 6. The chat moves to 8 meanwhile.
		expectClaim(t, db, chat, 3)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			advanceCursor(db, chat),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(advanced, nil),
		)
		// Second pass: the same extractor reclaims and drains 6 to 8, which
		// holds no new user text, so the cursor advances without a model call.
		expectClaim(t, db, chat, chat.HistoryVersion)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(advanced, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			advanceCursor(db, advanced),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(advanced, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("HandsOffWhenDrainBudgetRunsOut", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		serverCtx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		server := newServer(t, db, nil)
		server.ctx = serverCtx
		server.cancel = cancel

		// Every pass finds another completed turn with nothing to extract,
		// so the extractor keeps draining until the budget is spent.
		for i := range memoryExtractionMaxDrains {
			current := chat
			current.HistoryVersion = chat.HistoryVersion + int64(i)
			next := current
			next.HistoryVersion = current.HistoryVersion + 1
			expectClaim(t, db, chat, current.HistoryVersion-1)
			gomock.InOrder(
				db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(current, nil),
				expectProject(db, chat),
				db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return(nil, nil),
				advanceCursor(db, current),
				db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(next, nil),
			)
		}
		// The successor extractor claims once more for the leftover window.
		handoff := make(chan struct{})
		db.EXPECT().ClaimChatMemoryExtraction(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, database.ClaimChatMemoryExtractionParams) (database.ChatMemoryCursor, error) {
			close(handoff)
			return database.ChatMemoryCursor{}, sql.ErrNoRows
		})

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
		select {
		case <-handoff:
		case <-time.After(10 * time.Second):
			t.Fatal("successor extractor never claimed the leftover window")
		}
	})

	t.Run("CompactedHistoryStillReachesExtraction", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		var gotArgs database.GetChatMessagesForMemoryExtractionParams
		var sentTranscript string
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			sentTranscript = string(body)
			response := objectResponse(t, map[string]any{"upserts": []map[string]any{}})
			response.Request = req
			return response, nil
		}))
		// Compaction replays the user turn as a model-only row; the original
		// user-visible row must still be what the extractor reads.
		replayed := message(t, 2, database.ChatMessageRoleUser, "durable fact from before compaction", 5)
		replayed.Visibility = database.ChatMessageVisibilityModel
		expectClaim(t, db, chat, 3)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg database.GetChatMessagesForMemoryExtractionParams) ([]database.ChatMessage, error) {
				gotArgs = arg
				return []database.ChatMessage{
					message(t, 1, database.ChatMessageRoleUser, "durable fact from before compaction", 5),
					replayed,
				}, nil
			}),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			advanceCursor(db, chat),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
		require.Equal(t, database.GetChatMessagesForMemoryExtractionParams{ChatID: chat.ID, AfterRevision: 3}, gotArgs)
		require.Equal(t, 1, strings.Count(sentTranscript, "durable fact from before compaction"), "the replayed model-only row is not sent twice")
	})

	t.Run("NeverOverwritesExistingMemory", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{{
					"name":        "release_notes",
					"description": "Hallucinated rewrite",
					"body":        "Deploy day is Friday.",
				}},
			})
			response.Request = req
			return response, nil
		}))
		expectClaim(t, db, chat, 3)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "when do we deploy?", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		expectInsertTx(db)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(0), nil),
			db.EXPECT().InsertChatProjectMemory(gomock.Any(), database.InsertChatProjectMemoryParams{
				ID:             uuid.NullUUID{},
				ProjectID:      chat.ProjectID.UUID,
				OrganizationID: chat.OrganizationID,
				Name:           "release_notes",
				Description:    "Hallucinated rewrite",
				Body:           "Deploy day is Friday.",
				SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
				CreatedBy:      chat.OwnerID,
			}).Return(database.ChatProjectMemory{}, &pq.Error{Code: "23505"}),
			// A unique constraint conflict leaves the existing memory alone.
			advanceCursor(db, chat),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("RespectsCapForNewNames", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{{
					"name":        "release_notes",
					"description": "Durable release process",
					"body":        "Run the release checklist.",
				}},
			})
			response.Request = req
			return response, nil
		}))

		expectClaim(t, db, chat, 0)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		expectInsertTx(db)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(chattool.MaxMemories), nil),
			advanceCursor(db, chat),
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("StorageFailureDoesNotAdvanceCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{{
					"name":        "release_notes",
					"description": "Durable release process",
					"body":        "Run the release checklist.",
				}},
			})
			response.Request = req
			return response, nil
		}))

		expectClaim(t, db, chat, 0)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		expectInsertTx(db)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(0), nil),
			// A transient insert failure keeps the cursor so the window is retried.
			db.EXPECT().InsertChatProjectMemory(gomock.Any(), gomock.Any()).Return(database.ChatProjectMemory{}, xerrors.New("connection reset")),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("ModelFailureDoesNotAdvanceCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"model failed"}}`)),
				Request:    req,
			}, nil
		}))

		expectClaim(t, db, chat, 0)
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			expectProject(db, chat),
			db.EXPECT().GetChatMessagesForMemoryExtraction(gomock.Any(), gomock.Any()).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})
}

func TestResolveMemoryScope(t *testing.T) {
	t.Parallel()
	t.Run("ExperimentDisabledSkipsDatabaseAndModelCalls", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}
		server.extractMemories(t.Context(), slogtest.Make(t, nil), database.Chat{ID: uuid.New()})
	})
	t.Run("PromotedTurnStillRunningSkipsExtraction", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		serverCtx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		server := &Server{ctx: serverCtx, cancel: cancel, db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		// FinishTurn promoted a queued message; that turn's own completion
		// runs extraction for both turns, so nothing touches the database.
		server.maybeExtractMemoriesAsync(t.Context(), slogtest.Make(t, nil), database.Chat{ID: uuid.New(), Status: database.ChatStatusRunning})
	})
	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{Name: "platform"}, nil)
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, scope, status := server.resolveMemoryScope(context.Background(), database.Chat{ID: uuid.New(), OwnerID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.Equal(t, memoryScopeAvailable, status)
		require.Equal(t, "platform", scope.Label)
	})
	t.Run("OutsideProject", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, status := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), OwnerID: uuid.New(), OrganizationID: uuid.New()})
		require.Equal(t, memoryScopeNone, status)
	})
	t.Run("ProjectLookupFailure", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{}, xerrors.New("connection reset"))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, status := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.Equal(t, memoryScopeUnavailable, status)
	})
}

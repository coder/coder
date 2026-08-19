package coderd

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// ExtractChatParam authorizes the read, then GetAIBridgeChatCost authorizes it
// again. A denial on the second check means the ACL changed in between (a
// read-authz race). Assert it surfaces as 404, not 500.
func TestGetChatCostSurfacesReadAuthzRace(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	dbm := dbmock.NewMockStore(ctrl)
	chat := database.Chat{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerID:        uuid.New(),
	}

	dbm.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
	dbm.EXPECT().GetAIBridgeChatCost(gomock.Any(), chat.ID).Return(
		database.GetAIBridgeChatCostRow{},
		dbauthz.NotAuthorizedError{Err: sql.ErrNoRows},
	)

	api := &API{Options: &Options{Database: dbm}}
	rtr := chi.NewRouter()
	rtr.With(httpmw.ExtractChatParam(dbm)).Get("/chats/{chat}/cost", api.getChatCost)

	req := httptest.NewRequest(http.MethodGet, "/chats/"+chat.ID.String()+"/cost", nil)
	rec := httptest.NewRecorder()
	rtr.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// AI Gateway attributes a subagent's requests to the chat that spawned it, so
// a subagent request must be answered with its root chat's tree cost.
func TestGetChatCostQueriesRootChat(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	dbm := dbmock.NewMockStore(ctrl)
	rootID := uuid.New()
	child := database.Chat{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerID:        uuid.New(),
		ParentChatID:   uuid.NullUUID{UUID: rootID, Valid: true},
		RootChatID:     uuid.NullUUID{UUID: rootID, Valid: true},
	}

	dbm.EXPECT().GetChatByID(gomock.Any(), child.ID).Return(child, nil)
	dbm.EXPECT().GetAIBridgeChatCost(gomock.Any(), rootID).Return(
		database.GetAIBridgeChatCostRow{
			TotalCostMicros:      250,
			RequestCount:         2,
			UnpricedRequestCount: 1,
		},
		nil,
	)

	api := &API{Options: &Options{Database: dbm}}
	rtr := chi.NewRouter()
	rtr.With(httpmw.ExtractChatParam(dbm)).Get("/chats/{chat}/cost", api.getChatCost)

	req := httptest.NewRequest(http.MethodGet, "/chats/"+child.ID.String()+"/cost", nil)
	rec := httptest.NewRecorder()
	rtr.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var cost codersdk.ChatCost
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cost))
	require.Equal(t, child.ID, cost.ChatID)
	require.Equal(t, int64(250), cost.TotalCostMicros)
	require.Equal(t, int64(2), cost.RequestCount)
	require.Equal(t, int64(1), cost.UnpricedRequestCount)
}

func TestGetChatCostFallsBackToParentChat(t *testing.T) {
	t.Parallel()

	dbm := dbmock.NewMockStore(gomock.NewController(t))
	parentID := uuid.New()
	// chats.parent_chat_id and chats.root_chat_id are both ON DELETE SET NULL,
	// so deleting a root leaves descendants with only a parent.
	child := database.Chat{
		ID:           uuid.New(),
		OwnerID:      uuid.New(),
		ParentChatID: uuid.NullUUID{UUID: parentID, Valid: true},
	}

	dbm.EXPECT().GetChatByID(gomock.Any(), child.ID).Return(child, nil)
	dbm.EXPECT().GetAIBridgeChatCost(gomock.Any(), parentID).Return(
		database.GetAIBridgeChatCostRow{TotalCostMicros: 125, RequestCount: 1},
		nil,
	)

	api := &API{Options: &Options{Database: dbm}}
	rtr := chi.NewRouter()
	rtr.With(httpmw.ExtractChatParam(dbm)).Get("/chats/{chat}/cost", api.getChatCost)

	req := httptest.NewRequest(http.MethodGet, "/chats/"+child.ID.String()+"/cost", nil)
	rec := httptest.NewRecorder()
	rtr.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	var cost codersdk.ChatCost
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cost))
	require.Equal(t, int64(125), cost.TotalCostMicros)
}

func TestEnrichChatAgentIDs(t *testing.T) {
	t.Parallel()
	newAPI := func(t *testing.T) (*API, *dbmock.MockStore) {
		t.Helper()
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		return &API{Options: &Options{Database: mDB, Logger: logger}}, mDB
	}
	workspaceID, otherWorkspaceID := uuid.New(), uuid.New()
	rootAgentID, otherAgentID := uuid.New(), uuid.New()
	latestBuildID, otherLatestBuildID := uuid.New(), uuid.New()
	latestBuildIDs := map[uuid.UUID]uuid.UUID{workspaceID: latestBuildID, otherWorkspaceID: otherLatestBuildID}
	row := func(workspaceID, id uuid.UUID, parentID uuid.NullUUID, name string) database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow {
		return database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{
			WorkspaceID: workspaceID,
			BuildID:     latestBuildIDs[workspaceID],
			WorkspaceAgent: database.WorkspaceAgent{
				ID:       id,
				ParentID: parentID,
				Name:     name,
			},
		}
	}
	t.Run("batch selection and shared workspace", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, ids []uuid.UUID) ([]database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow, error) {
			require.ElementsMatch(t, []uuid.UUID{workspaceID, otherWorkspaceID}, ids)
			return []database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{
				row(workspaceID, uuid.New(), uuid.NullUUID{UUID: rootAgentID, Valid: true}, "sub"), row(workspaceID, rootAgentID, uuid.NullUUID{}, "root"), row(otherWorkspaceID, otherAgentID, uuid.NullUUID{}, "root"),
			}, nil
		}).Times(1)
		chats := []codersdk.Chat{{WorkspaceID: &workspaceID, Children: []codersdk.Chat{{WorkspaceID: &workspaceID}}}, {WorkspaceID: &otherWorkspaceID}}
		api.enrichChatsWithMissingAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Equal(t, rootAgentID, *chats[0].AgentID)
		require.Equal(t, rootAgentID, *chats[0].Children[0].AgentID)
		require.Equal(t, otherAgentID, *chats[1].AgentID)
		require.Equal(t, latestBuildID, *chats[0].BuildID)
		require.Equal(t, latestBuildID, *chats[0].Children[0].BuildID)
		require.Equal(t, otherLatestBuildID, *chats[1].BuildID)
	})
	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), gomock.Any()).Return(nil, xerrors.New("boom"))
		chats := []codersdk.Chat{{WorkspaceID: &workspaceID}, {WorkspaceID: &otherWorkspaceID}}
		api.enrichChatsWithMissingAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Nil(t, chats[0].AgentID)
		require.Nil(t, chats[1].AgentID)
	})
	t.Run("selection error keeps persisted values", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), []uuid.UUID{workspaceID}).Return([]database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{row(workspaceID, uuid.New(), uuid.NullUUID{UUID: rootAgentID, Valid: true}, "sub")}, nil)
		bound := otherAgentID
		boundBuildID := uuid.New()
		chats := []codersdk.Chat{{}, {WorkspaceID: &workspaceID}, {WorkspaceID: &workspaceID, AgentID: &bound, BuildID: &boundBuildID}}
		api.repairChatAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Nil(t, chats[1].AgentID)
		require.Nil(t, chats[1].BuildID)
		require.Equal(t, bound, *chats[2].AgentID)
		require.Equal(t, boundBuildID, *chats[2].BuildID)
	})
	t.Run("repairs stale and keeps valid bindings", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		secondRootAgentID := uuid.New()
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), []uuid.UUID{workspaceID}).Return([]database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{
			row(workspaceID, rootAgentID, uuid.NullUUID{}, "a"),
			row(workspaceID, secondRootAgentID, uuid.NullUUID{}, "b"),
		}, nil)
		stale, valid := uuid.New(), secondRootAgentID
		staleBuildID, validBuildID := uuid.New(), uuid.New()
		chats := []codersdk.Chat{
			{WorkspaceID: &workspaceID, AgentID: &stale, BuildID: &staleBuildID},
			{WorkspaceID: &workspaceID, AgentID: &valid, BuildID: &validBuildID},
		}
		api.repairChatAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Equal(t, rootAgentID, *chats[0].AgentID)
		require.Equal(t, secondRootAgentID, *chats[1].AgentID)
		require.Equal(t, latestBuildID, *chats[0].BuildID)
		require.Equal(t, validBuildID, *chats[1].BuildID)
	})
	t.Run("list mode skips bound chats entirely", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), []uuid.UUID{workspaceID}).Return([]database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{
			row(workspaceID, rootAgentID, uuid.NullUUID{}, "root"),
		}, nil).Times(1)
		stale := uuid.New()
		chats := []codersdk.Chat{
			{WorkspaceID: &workspaceID},
			{WorkspaceID: &otherWorkspaceID, AgentID: &stale},
		}
		api.enrichChatsWithMissingAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Equal(t, rootAgentID, *chats[0].AgentID)
		require.Equal(t, stale, *chats[1].AgentID)
	})
	t.Run("no bound workspaces skips the query", func(t *testing.T) {
		t.Parallel()
		api, _ := newAPI(t)
		chats := []codersdk.Chat{{AgentID: &rootAgentID}, {}}
		api.repairChatAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Equal(t, rootAgentID, *chats[0].AgentID)
		require.Nil(t, chats[1].AgentID)
	})
}

func TestValidateChatModelProviderOptions_AnthropicThinkingDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		display string
		wantErr string
	}{
		{name: "Summarized", display: "summarized"},
		{name: "Omitted", display: " omitted "},
		{name: "Empty", display: " "},
		{
			name:    "Invalid",
			display: "summrized",
			wantErr: "provider_options.anthropic.thinking_display must be one of summarized, omitted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			display := tt.display
			err := validateChatModelProviderOptions(&codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
					ThinkingDisplay: &display,
				},
			})
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChatModelConfigProviderModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		model      string
		provider   database.AIProvider
		wantErr    bool
		wantDetail string
	}{
		{
			name:  "OpenRouterNameWithOpenAITypeAndSlashModel",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name: "openrouter",
				Type: database.AIProviderTypeOpenai,
			},
			wantErr:    true,
			wantDetail: "Change the AI provider type to openrouter or openai-compat.",
		},
		{
			name:  "OpenRouterNameWithWhitespaceAndCase",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name: " OpenRouter ",
				Type: database.AIProviderTypeOpenai,
			},
			wantErr:    true,
			wantDetail: "Change the AI provider type to openrouter or openai-compat.",
		},
		{
			name:  "OpenRouterHostWithOpenAITypeAndSlashModel",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name:    "private-relay",
				Type:    database.AIProviderTypeOpenai,
				BaseUrl: "https://openrouter.ai/api/v1",
			},
			wantErr:    true,
			wantDetail: "Change the AI provider type to openrouter or openai-compat.",
		},
		{
			name:  "OpenRouterHostWithPort",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name:    "private-relay",
				Type:    database.AIProviderTypeOpenai,
				BaseUrl: "https://openrouter.ai:443/api/v1",
			},
			wantErr:    true,
			wantDetail: "Change the AI provider type to openrouter or openai-compat.",
		},
		{
			name:  "OpenRouterSubdomainWithOpenAIType",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name:    "private-relay",
				Type:    database.AIProviderTypeOpenai,
				BaseUrl: "https://api.openrouter.ai/v1",
			},
			wantErr:    true,
			wantDetail: "Change the AI provider type to openrouter or openai-compat.",
		},
		{
			name:  "OpenRouterTypeAllowsSlashModel",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name: "openrouter",
				Type: database.AIProviderTypeOpenrouter,
			},
		},
		{
			name:  "OpenAICompatTypeAllowsSlashModel",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name: "openrouter",
				Type: database.AIProviderTypeOpenaiCompat,
			},
		},
		{
			name:  "PrivateOpenAIProxyAllowsSlashModel",
			model: "anthropic/claude-opus-4.6",
			provider: database.AIProvider{
				Name:    "private-relay",
				Type:    database.AIProviderTypeOpenai,
				BaseUrl: "https://llm-relay.internal/v1",
			},
		},
		{
			name:  "OpenRouterNameWithPlainModelAllowed",
			model: "gpt-4.1",
			provider: database.AIProvider{
				Name: "openrouter",
				Type: database.AIProviderTypeOpenai,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := validateChatModelConfigProviderModel(tt.provider, tt.model)
			if tt.wantErr {
				require.NotNil(t, got)
				require.Contains(t, got.Response.Detail, tt.wantDetail)
				return
			}
			require.Nil(t, got)
		})
	}
}

func TestWriteChatFileErrorUnavailable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	rec := httptest.NewRecorder()
	handled := writeChatFileError(ctx, rec, xerrors.Errorf("link files: %w", chatstate.ErrChatFileUnavailable))
	require.True(t, handled)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var response codersdk.Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.Equal(t, "Chat attachment unavailable.", response.Message)
	require.Equal(t, "An attachment is no longer available. Upload it again and retry.", response.Detail)
}

func TestRewriteChatStartWorkspaceManualUpdateResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		resp           codersdk.Response
		fallbackDetail string
		wantDetail     string
	}{
		{
			name: "NoValidationsAndEmptyDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "missing required parameter",
		},
		{
			name: "NoValidationsAndExistingDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
				Detail:  "region must be set before the workspace can start",
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "missing required parameter: region must be set before the workspace can start",
		},
		{
			name: "ValidationsAndEmptyDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
				Validations: []codersdk.ValidationError{{
					Field:  "region",
					Detail: "region must be set before the workspace can start",
				}},
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "wrapped missing required parameter",
		},
		{
			name: "ValidationsAndExistingDetail",
			resp: codersdk.Response{
				Message: "missing required parameter",
				Detail:  "region must be set before the workspace can start",
				Validations: []codersdk.ValidationError{{
					Field:  "region",
					Detail: "region must be set before the workspace can start",
				}},
			},
			fallbackDetail: "wrapped missing required parameter",
			wantDetail:     "region must be set before the workspace can start",
		},
	}

	const retryInstructions = "Use read_template before retrying start_workspace."
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := rewriteChatStartWorkspaceManualUpdateResponse(tt.resp, tt.fallbackDetail, retryInstructions)
			require.Equal(t, retryInstructions, got.Message)
			require.Equal(t, tt.wantDetail, got.Detail)
			require.Equal(t, tt.resp.Validations, got.Validations)
		})
	}
}

// Every ChatModelCallConfig field must classify a config as non-zero when set,
// or unmarshalChatModelCallConfig hides it from API responses while the stored
// value stays active. Fails when a new field is added without a sample here.
func TestIsZeroChatModelCallConfigCoversEveryField(t *testing.T) {
	t.Parallel()

	sampled := codersdk.ChatModelCallConfig{
		MaxOutputTokens:  ptr.Ref(int64(4096)),
		Temperature:      ptr.Ref(0.7),
		TopP:             ptr.Ref(0.9),
		TopK:             ptr.Ref(int64(40)),
		PresencePenalty:  ptr.Ref(0.1),
		FrequencyPenalty: ptr.Ref(0.2),
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
			Default: ptr.Ref("medium"),
		},
		OpenAIConfig: &codersdk.ChatModelOpenAIConfig{
			UseResponsesAPI: ptr.Ref(true),
		},
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{},
		},
	}

	require.True(t, isZeroChatModelCallConfig(nil))
	require.True(t, isZeroChatModelCallConfig(&codersdk.ChatModelCallConfig{}))

	sampledValue := reflect.ValueOf(sampled)
	for i := 0; i < sampledValue.NumField(); i++ {
		field := sampledValue.Type().Field(i)
		require.Falsef(t, sampledValue.Field(i).IsZero(),
			"field %s needs a non-zero sample value", field.Name)

		config := &codersdk.ChatModelCallConfig{}
		reflect.ValueOf(config).Elem().Field(i).Set(sampledValue.Field(i))
		require.Falsef(t, isZeroChatModelCallConfig(config),
			"isZeroChatModelCallConfig ignores field %s", field.Name)
	}
}

// TestChatWorkspaceConnAuthorizer verifies the chatd workspace-dial
// authorizer against real RBAC state: revoking the owner's
// organization-workspace-access role after a chat is bound must deny
// subsequent internal dials.
func TestChatWorkspaceConnAuthorizer(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	authorize := chatWorkspaceConnAuthorizer(db, rbac.NewAuthorizer(prometheus.NewRegistry()))

	// Plain members receive workspace access through the org's
	// default member roles (organization-workspace-access).
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      owner.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
	})

	require.NoError(t, authorize(ctx, owner.ID, ws.ID))

	// Suspending the owner must deny dials even though their roles
	// still authorize workspace access.
	_, err := db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
		ID:        owner.ID,
		Status:    database.UserStatusSuspended,
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	require.ErrorContains(t, authorize(ctx, owner.ID, ws.ID), "not active")
	_, err = db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
		ID:        owner.ID,
		Status:    database.UserStatusActive,
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, authorize(ctx, owner.ID, ws.ID))

	// Revoke workspace access by clearing the org's default member
	// roles, the documented mechanism for revoking capabilities that
	// plain members otherwise hold.
	_, err = db.UpdateOrganization(ctx, database.UpdateOrganizationParams{
		ID:                    org.ID,
		UpdatedAt:             org.UpdatedAt,
		Name:                  org.Name,
		DisplayName:           org.DisplayName,
		Description:           org.Description,
		Icon:                  org.Icon,
		DefaultOrgMemberRoles: []string{},
	})
	require.NoError(t, err)

	require.Error(t, authorize(ctx, owner.ID, ws.ID))
}

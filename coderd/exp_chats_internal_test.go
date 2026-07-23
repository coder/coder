package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestEnrichMissingChatAgentIDs(t *testing.T) {
	t.Parallel()
	newAPI := func(t *testing.T) (*API, *dbmock.MockStore) {
		t.Helper()
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		return &API{Options: &Options{Database: mDB, Logger: logger}}, mDB
	}
	workspaceID, otherWorkspaceID := uuid.New(), uuid.New()
	rootAgentID, otherAgentID := uuid.New(), uuid.New()
	row := func(workspaceID, id uuid.UUID, parentID uuid.NullUUID, name string) database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow {
		return database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{
			WorkspaceID: workspaceID,
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
		api.enrichChatWithWorkspaceAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Equal(t, rootAgentID, *chats[0].AgentID)
		require.Equal(t, rootAgentID, *chats[0].Children[0].AgentID)
		require.Equal(t, otherAgentID, *chats[1].AgentID)
	})
	t.Run("query error", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), gomock.Any()).Return(nil, xerrors.New("boom"))
		chats := []codersdk.Chat{{WorkspaceID: &workspaceID}, {WorkspaceID: &otherWorkspaceID}}
		api.enrichChatWithWorkspaceAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Nil(t, chats[0].AgentID)
		require.Nil(t, chats[1].AgentID)
	})
	t.Run("selection error and skips bound or unbound", func(t *testing.T) {
		t.Parallel()
		api, mDB := newAPI(t)
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceIDs(gomock.Any(), []uuid.UUID{workspaceID}).Return([]database.GetWorkspaceAgentsInLatestBuildByWorkspaceIDsRow{row(workspaceID, uuid.New(), uuid.NullUUID{UUID: rootAgentID, Valid: true}, "sub")}, nil)
		bound := otherAgentID
		chats := []codersdk.Chat{{}, {WorkspaceID: &workspaceID}, {WorkspaceID: &workspaceID, AgentID: &bound}}
		api.enrichChatWithWorkspaceAgentIDs(testutil.Context(t, testutil.WaitShort), chats)
		require.Nil(t, chats[1].AgentID)
		require.Equal(t, bound, *chats[2].AgentID)
	})
}

// TestResolveChatStartVersion covers the version pinning decision
// behind the RequireActiveVersion auto-update for chat workspace
// starts. Wiring into the build path is covered end-to-end in
// coderd/x/chatd tests.
func TestResolveChatStartVersion(t *testing.T) {
	t.Parallel()

	v1 := uuid.New()
	v2 := uuid.New()

	tests := []struct {
		name          string
		transition    codersdk.WorkspaceTransition
		latestVersion uuid.UUID
		activeVersion uuid.UUID
		wantVersion   uuid.UUID
		wantUpdated   bool
	}{
		{
			name:          "StartVersionMismatchBumps",
			transition:    codersdk.WorkspaceTransitionStart,
			latestVersion: v1,
			activeVersion: v2,
			wantVersion:   v2,
			wantUpdated:   true,
		},
		{
			name:          "StartVersionMatchPinsWithoutUpdate",
			transition:    codersdk.WorkspaceTransitionStart,
			latestVersion: v1,
			activeVersion: v1,
			wantVersion:   v1,
			wantUpdated:   false,
		},
		{
			name:          "StopIgnoresActiveVersion",
			transition:    codersdk.WorkspaceTransitionStop,
			latestVersion: v1,
			activeVersion: v2,
			wantVersion:   uuid.Nil,
			wantUpdated:   false,
		},
		{
			name:          "DeleteIgnoresActiveVersion",
			transition:    codersdk.WorkspaceTransitionDelete,
			latestVersion: v1,
			activeVersion: v2,
			wantVersion:   uuid.Nil,
			wantUpdated:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := codersdk.CreateWorkspaceBuildRequest{Transition: tt.transition}
			got, updated := resolveChatStartVersion(req, tt.latestVersion, tt.activeVersion)
			require.Equal(t, tt.wantVersion, got.TemplateVersionID)
			require.Equal(t, tt.wantUpdated, updated)
			require.Equal(t, tt.transition, got.Transition)
		})
	}
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

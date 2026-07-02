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

func TestEnrichChatAgentIDs(t *testing.T) {
	t.Parallel()

	newAPI := func(t *testing.T) (*API, *dbmock.MockStore) {
		t.Helper()
		mDB := dbmock.NewMockStore(gomock.NewController(t))
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		return &API{
			Options: &Options{
				Database: mDB,
				Logger:   logger,
			},
		}, mDB
	}

	t.Run("ResolvesRootAgentSkippingSubAgent", func(t *testing.T) {
		t.Parallel()

		var (
			ctx         = testutil.Context(t, testutil.WaitShort)
			workspaceID = uuid.New()
			rootAgentID = uuid.New()
		)
		api, mDB := newAPI(t)

		// The sub-agent is returned first to prove selection is not
		// positional.
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{
				{
					ID:       uuid.New(),
					ParentID: uuid.NullUUID{UUID: rootAgentID, Valid: true},
					Name:     "dev-container",
				},
				{
					ID:   rootAgentID,
					Name: "main",
				},
			}, nil)

		chats := []codersdk.Chat{{WorkspaceID: &workspaceID}}
		api.enrichChatAgentIDs(ctx, chats)

		require.NotNil(t, chats[0].AgentID)
		require.Equal(t, rootAgentID, *chats[0].AgentID)
	})

	t.Run("DeduplicatesLookupsAndEnrichesChildren", func(t *testing.T) {
		t.Parallel()

		var (
			ctx         = testutil.Context(t, testutil.WaitShort)
			workspaceID = uuid.New()
			agentID     = uuid.New()
		)
		api, mDB := newAPI(t)

		// A single lookup serves the root chat and its child; gomock
		// fails the test on a second call.
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{{ID: agentID, Name: "main"}}, nil).
			Times(1)

		chats := []codersdk.Chat{{
			WorkspaceID: &workspaceID,
			Children:    []codersdk.Chat{{WorkspaceID: &workspaceID}},
		}}
		api.enrichChatAgentIDs(ctx, chats)

		require.NotNil(t, chats[0].AgentID)
		require.Equal(t, agentID, *chats[0].AgentID)
		require.NotNil(t, chats[0].Children[0].AgentID)
		require.Equal(t, agentID, *chats[0].Children[0].AgentID)
	})

	t.Run("LeavesNullOnError", func(t *testing.T) {
		t.Parallel()

		var (
			ctx         = testutil.Context(t, testutil.WaitShort)
			workspaceID = uuid.New()
		)
		api, mDB := newAPI(t)

		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return(nil, xerrors.New("boom"))

		chats := []codersdk.Chat{{WorkspaceID: &workspaceID}}
		api.enrichChatAgentIDs(ctx, chats)

		require.Nil(t, chats[0].AgentID)
	})

	t.Run("SkipsChatsWithoutWorkspaceOrWithAgent", func(t *testing.T) {
		t.Parallel()

		var (
			ctx         = testutil.Context(t, testutil.WaitShort)
			workspaceID = uuid.New()
			existing    = uuid.New()
		)
		// No database expectations: neither chat should trigger a
		// lookup.
		api, _ := newAPI(t)

		chats := []codersdk.Chat{
			{},
			{WorkspaceID: &workspaceID, AgentID: &existing},
		}
		api.enrichChatAgentIDs(ctx, chats)

		require.Nil(t, chats[0].AgentID)
		require.Equal(t, existing, *chats[1].AgentID)
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

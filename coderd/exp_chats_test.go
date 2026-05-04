package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/shopspring/decimal"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/externalauth"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	chatProviderAPIKeySizeLimit = 10240
	missingCentralKeyMessage    = "API key is required when central API key is enabled."
)

func chatDeploymentValues(t testing.TB) *codersdk.DeploymentValues {
	t.Helper()

	values := coderdtest.DeploymentValues(t)
	return values
}

func newChatClient(t testing.TB, overrides ...func(*coderdtest.Options)) *codersdk.ExperimentalClient {
	t.Helper()

	opts := &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	}
	for _, override := range overrides {
		override(opts)
	}
	client := coderdtest.New(t, opts)
	return codersdk.NewExperimentalClient(client)
}

func newChatClientWithDeploymentValues(
	t testing.TB,
	values *codersdk.DeploymentValues,
) *codersdk.ExperimentalClient {
	t.Helper()

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: values,
	})
	return codersdk.NewExperimentalClient(client)
}

func newChatClientWithDatabase(t testing.TB, overrides ...func(*coderdtest.Options)) (*codersdk.ExperimentalClient, database.Store) {
	t.Helper()

	opts := &coderdtest.Options{
		DeploymentValues: chatDeploymentValues(t),
	}
	for _, override := range overrides {
		override(opts)
	}
	client, db := coderdtest.NewWithDatabase(t, opts)
	return codersdk.NewExperimentalClient(client), db
}

type failNextChatSystemPromptStore struct {
	database.Store

	failNextGetChatIncludeDefaultSystemPrompt    atomic.Bool
	failNextGetChatSystemPromptConfig            atomic.Bool
	failNextUpsertChatIncludeDefaultSystemPrompt atomic.Bool
}

func (s *failNextChatSystemPromptStore) GetChatIncludeDefaultSystemPrompt(ctx context.Context) (bool, error) {
	if s.failNextGetChatIncludeDefaultSystemPrompt.CompareAndSwap(true, false) {
		return false, stderrors.New("forced include-default read failure")
	}
	return s.Store.GetChatIncludeDefaultSystemPrompt(ctx)
}

func (s *failNextChatSystemPromptStore) UpsertChatIncludeDefaultSystemPrompt(ctx context.Context, includeDefault bool) error {
	if s.failNextUpsertChatIncludeDefaultSystemPrompt.CompareAndSwap(true, false) {
		return stderrors.New("forced include-default upsert failure")
	}
	return s.Store.UpsertChatIncludeDefaultSystemPrompt(ctx, includeDefault)
}

func (s *failNextChatSystemPromptStore) GetChatSystemPromptConfig(ctx context.Context) (database.GetChatSystemPromptConfigRow, error) {
	if s.failNextGetChatSystemPromptConfig.CompareAndSwap(true, false) {
		return database.GetChatSystemPromptConfigRow{}, stderrors.New("forced chat system prompt configuration read failure")
	}
	return s.Store.GetChatSystemPromptConfig(ctx)
}

// failNextUpdateChatModelConfigStore shares its failure state across InTx
// wrappers so tests can force a specific in-transaction model-config update to
// return sql.ErrNoRows.
type failNextUpdateChatModelConfigStore struct {
	database.Store

	failNextUpdateChatModelConfig   *atomic.Bool
	failNextUpdateChatModelConfigID uuid.UUID
}

func newFailNextUpdateChatModelConfigStore(store database.Store) *failNextUpdateChatModelConfigStore {
	return &failNextUpdateChatModelConfigStore{
		Store:                         store,
		failNextUpdateChatModelConfig: &atomic.Bool{},
	}
}

func (s *failNextUpdateChatModelConfigStore) InTx(function func(database.Store) error, txOpts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return function(&failNextUpdateChatModelConfigStore{
			Store:                           tx,
			failNextUpdateChatModelConfig:   s.failNextUpdateChatModelConfig,
			failNextUpdateChatModelConfigID: s.failNextUpdateChatModelConfigID,
		})
	}, txOpts)
}

func (s *failNextUpdateChatModelConfigStore) UpdateChatModelConfig(
	ctx context.Context,
	arg database.UpdateChatModelConfigParams,
) (database.ChatModelConfig, error) {
	if arg.ID == s.failNextUpdateChatModelConfigID &&
		s.failNextUpdateChatModelConfig.CompareAndSwap(true, false) {
		return database.ChatModelConfig{}, sql.ErrNoRows
	}
	return s.Store.UpdateChatModelConfig(ctx, arg)
}

func requireChatUsageLimitExceededError(
	t *testing.T,
	err error,
	wantSpentMicros int64,
	wantLimitMicros int64,
	wantResetsAt time.Time,
) *codersdk.ChatUsageLimitExceededResponse {
	t.Helper()

	sdkErr, ok := codersdk.AsError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	require.Equal(t, "Chat usage limit exceeded.", sdkErr.Message)

	limitErr := codersdk.ChatUsageLimitExceededFrom(err)
	require.NotNil(t, limitErr)
	require.Equal(t, "Chat usage limit exceeded.", limitErr.Message)
	require.Equal(t, wantSpentMicros, limitErr.SpentMicros)
	require.Equal(t, wantLimitMicros, limitErr.LimitMicros)
	require.True(
		t,
		limitErr.ResetsAt.Equal(wantResetsAt),
		"expected resets_at %s, got %s",
		wantResetsAt.UTC().Format(time.RFC3339),
		limitErr.ResetsAt.UTC().Format(time.RFC3339),
	)

	return limitErr
}

func enableDailyChatUsageLimit(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	limitMicros int64,
) time.Time {
	t.Helper()

	_, err := db.UpsertChatUsageLimitConfig(
		dbauthz.AsSystemRestricted(ctx),
		database.UpsertChatUsageLimitConfigParams{
			Enabled:            true,
			DefaultLimitMicros: limitMicros,
			Period:             string(codersdk.ChatUsageLimitPeriodDay),
		},
	)
	require.NoError(t, err)

	_, periodEnd := chatd.ComputeUsagePeriodBounds(time.Now(), codersdk.ChatUsageLimitPeriodDay)
	return periodEnd
}

func insertAssistantCostMessage(
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	modelConfigID uuid.UUID,
	totalCostMicros int64,
) {
	t.Helper()

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chatID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfigID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		Content:         assistantContent,
		TotalCostMicros: sql.NullInt64{Int64: totalCostMicros, Valid: true},
	})
}

func TestPostChats(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMock()
		client := newChatClient(t, func(opts *coderdtest.Options) {
			opts.Auditor = mAudit
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Use a member with agents-access instead of the owner to
		// verify least-privilege access.
		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		chat, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID, Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello from chats route tests",
				},
			},
		})
		require.NoError(t, err)

		require.NotEqual(t, uuid.Nil, chat.ID)
		require.Equal(t, member.ID, chat.OwnerID)
		require.Equal(t, modelConfig.ID, chat.LastModelConfigID)
		require.Equal(t, "hello from chats route tests", chat.Title)
		require.NotZero(t, chat.CreatedAt)
		require.NotZero(t, chat.UpdatedAt)
		require.Nil(t, chat.WorkspaceID)
		require.NotNil(t, chat.RootChatID)
		require.Equal(t, chat.ID, *chat.RootChatID)

		chatResult, err := memberClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		messagesResult, err := memberClient.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Equal(t, chat.ID, chatResult.ID)

		foundUserMessage := false
		for _, message := range messagesResult.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type == codersdk.ChatMessagePartTypeText &&
					part.Text == "hello from chats route tests" {
					foundUserMessage = true
					break
				}
			}
		}
		require.True(t, foundUserMessage)
		require.True(t, mAudit.Contains(t, database.AuditLog{
			Action:         database.AuditActionCreate,
			ResourceType:   database.ResourceTypeChat,
			ResourceID:     chat.ID,
			ResourceTarget: chat.ID.String()[:8],
			UserID:         member.ID,
		}))
	})

	t.Run("MemberWithoutAgentsAccess", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Member without agents-access should be denied.
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "this should fail",
				},
			},
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("HidesSystemPromptMessages", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "verify hidden system prompt",
				},
			},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, message := range messagesResult.Messages {
			require.NotEqual(t, codersdk.ChatMessageRoleSystem, message.Role)
		}
	})

	t.Run("WithPerChatSystemPrompt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello with system prompt",
				},
			},
			SystemPrompt: "You are a Go expert.",
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, chat.ID)

		// Use the DB directly to see system messages, which are
		// hidden from the public API.
		dbMessages, err := db.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		// Expect: deployment system prompt, per-chat system prompt,
		// workspace awareness, user message.
		var systemMessages []database.ChatMessage
		for _, msg := range dbMessages {
			if msg.Role == database.ChatMessageRoleSystem {
				systemMessages = append(systemMessages, msg)
			}
		}
		require.GreaterOrEqual(t, len(systemMessages), 2,
			"expected at least deployment + per-chat system messages")

		// The per-chat system prompt should be the second system
		// message and contain the user-specified text.
		foundPerChat := false
		for _, msg := range systemMessages {
			if msg.Content.Valid {
				raw := string(msg.Content.RawMessage)
				if strings.Contains(raw, "You are a Go expert.") {
					foundPerChat = true
					break
				}
			}
		}
		require.True(t, foundPerChat,
			"per-chat system prompt not found in system messages")
	})

	t.Run("PerChatSystemPromptEmpty", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello without system prompt",
				},
			},
			SystemPrompt: "",
		})
		require.NoError(t, err)

		dbMessages, err := db.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		// No per-chat system prompt should be present.
		for _, msg := range dbMessages {
			if msg.Role == database.ChatMessageRoleSystem && msg.Content.Valid {
				raw := string(msg.Content.RawMessage)
				require.NotContains(t, raw, "You are a Go expert.",
					"unexpected per-chat system prompt in messages")
			}
		}
	})

	t.Run("PerChatSystemPromptTooLong", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		longPrompt := strings.Repeat("a", 10001)
		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID, Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			SystemPrompt: longPrompt,
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("WorkspaceNotAccessible", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(
			t,
			"Workspace not found or you do not have access to this resource",
			sdkErr.Message,
		)
	})

	t.Run("WorkspaceAccessibleButNoSSH", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		orgAdminClientRaw, _ := coderdtest.CreateAnotherUser(
			t,
			adminClient.Client,
			firstUser.OrganizationID,
			rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID),
			rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID),
		)
		orgAdminClient := codersdk.NewExperimentalClient(orgAdminClientRaw)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		_, err := orgAdminClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(
			t,
			"Workspace not found or you do not have access to this resource",
			sdkErr.Message,
		)
	})

	t.Run("WorkspaceNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		workspaceID := uuid.New()
		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(
			t,
			"Workspace not found or you do not have access to this resource",
			sdkErr.Message,
		)
	})

	t.Run("WorkspaceSelectsFirstAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, chat.WorkspaceID)
		require.Equal(t, workspaceBuild.Workspace.ID, *chat.WorkspaceID)
		require.Equal(t, modelConfig.ID, chat.LastModelConfigID)
	})

	t.Run("MissingDefaultModelConfig", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "No default chat model config is configured.", sdkErr.Message)
	})

	t.Run("EmptyContent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        nil,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Content is required.", sdkErr.Message)
		require.Equal(t, "Content cannot be empty.", sdkErr.Detail)
	})

	t.Run("EmptyText", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "   ",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].text cannot be empty.", sdkErr.Detail)
	})

	t.Run("UnsupportedPartType", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartType("image"),
					Text: "hello",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, `content[0].type "image" is not supported.`, sdkErr.Detail)
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)
		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)

		existingChat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "existing-limit-chat",
		})
		insertAssistantCostMessage(t, db, existingChat.ID, modelConfig.ID, 100)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "over limit",
			}},
		})
		requireChatUsageLimitExceededError(t, err, 100, 100, wantResetsAt)
	})

	t.Run("NilOrganizationID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: uuid.Nil,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "organization_id is required.", sdkErr.Message)
	})

	t.Run("NonMemberOrganization", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		// Create a second organization via the database since the
		// API endpoint is enterprise-only.
		secondOrg := dbgen.Organization(t, db, database.Organization{})

		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: secondOrg.ID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "You are not a member of the specified organization.", sdkErr.Message)
	})

	t.Run("CrossOrgWorkspaceMismatch", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		}).WithAgent().Do()

		// Create a second organization and add the admin as a member
		// so the request passes the membership check but fails on
		// the workspace org mismatch.
		secondOrg := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: secondOrg.ID,
			UserID:         firstUser.UserID,
		})

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: secondOrg.ID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
			WorkspaceID: &workspaceBuild.Workspace.ID,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Workspace does not belong to the specified organization.", sdkErr.Message)
	})
}

func TestPostChats_ClientType(t *testing.T) {
	t.Parallel()

	client := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)

	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	newChat := func(t *testing.T, clientType codersdk.ChatClientType) codersdk.Chat {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "client type test",
			}},
			ClientType: clientType,
		})
		require.NoError(t, err)
		return chat
	}

	t.Run("DefaultIsAPI", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Omit ClientType entirely — should default to "api".
		chat := newChat(t, "")
		require.Equal(t, codersdk.ChatClientTypeAPI, chat.ClientType)

		got, err := memberClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatClientTypeAPI, got.ClientType)
	})

	t.Run("ExplicitAPI", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		chat := newChat(t, codersdk.ChatClientTypeAPI)
		require.Equal(t, codersdk.ChatClientTypeAPI, chat.ClientType)

		got, err := memberClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatClientTypeAPI, got.ClientType)
	})

	t.Run("ExplicitUI", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		chat := newChat(t, codersdk.ChatClientTypeUI)
		require.Equal(t, codersdk.ChatClientTypeUI, chat.ClientType)

		got, err := memberClient.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatClientTypeUI, got.ClientType)
	})

	t.Run("InvalidClientType", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "bad client type",
			}},
			ClientType: "bogus",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Invalid client_type")
	})
}

func TestListChats(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		firstChatA, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "first owner chat",
				},
			},
		})
		require.NoError(t, err)

		firstChatB, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "second owner chat",
				},
			},
		})
		require.NoError(t, err)

		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		memberDBChat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "member chat only",
		})

		chats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, chats, 2)

		chatIndexes := make(map[uuid.UUID]int, len(chats))
		chatsByID := make(map[uuid.UUID]codersdk.Chat, len(chats))
		for i, chat := range chats {
			chatIndexes[chat.ID] = i
			chatsByID[chat.ID] = chat

			require.Equal(t, firstUser.UserID, chat.OwnerID)
			require.Equal(t, modelConfig.ID, chat.LastModelConfigID)
			// The chat may have been picked up by the background
			// processor (via signalWake) before we list, so
			// accept any active status.
			require.Contains(t, []codersdk.ChatStatus{
				codersdk.ChatStatusPending,
				codersdk.ChatStatusRunning,
				codersdk.ChatStatusError,
				codersdk.ChatStatusWaiting,
				codersdk.ChatStatusCompleted,
			}, chat.Status, "unexpected chat status: %s", chat.Status)
			require.NotZero(t, chat.CreatedAt)
			require.NotZero(t, chat.UpdatedAt)
			require.Nil(t, chat.ParentChatID)
			require.Nil(t, chat.WorkspaceID)
			require.NotNil(t, chat.RootChatID)
			require.Equal(t, chat.ID, *chat.RootChatID)
			require.NotNil(t, chat.DiffStatus)
			require.Equal(t, chat.ID, chat.DiffStatus.ChatID)
		}
		require.Contains(t, chatsByID, firstChatA.ID)
		require.Contains(t, chatsByID, firstChatB.ID)
		require.NotContains(t, chatsByID, memberDBChat.ID)
		require.Equal(t, "first owner chat", chatsByID[firstChatA.ID].Title)
		require.Equal(t, "second owner chat", chatsByID[firstChatB.ID].Title)

		for i := 1; i < len(chats); i++ {
			require.False(t, chats[i-1].UpdatedAt.Before(chats[i].UpdatedAt))
		}
		// The list is already verified as sorted by UpdatedAt
		// descending (loop above). We intentionally do NOT
		// compare positions using the creation-time UpdatedAt
		// values because signalWake() may trigger background
		// processing that mutates UpdatedAt between CreateChat
		// and ListChats.

		memberChats, err := memberClient.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, memberChats, 1)
		require.Equal(t, memberDBChat.ID, memberChats[0].ID)
		require.Equal(t, member.ID, memberChats[0].OwnerID)
		require.Equal(t, "member chat only", memberChats[0].Title)
		require.NotNil(t, memberChats[0].RootChatID)
		require.Equal(t, memberChats[0].ID, *memberChats[0].RootChatID)
		require.NotNil(t, memberChats[0].DiffStatus)
		require.Equal(t, memberChats[0].ID, memberChats[0].DiffStatus.ChatID)
	})

	t.Run("OrgMemberWithoutAgentsAccessCannotAccessOwnChats", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a member without agents-access and insert a chat
		// owned by them via system context. Without agents-access,
		// the member has no ResourceChat permissions at all, so
		// listing returns 0 chats (SQL auth filter) and getting
		// a specific chat returns 404 (dbauthz wraps as not found).
		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "member chat",
		})

		// Listing chats returns empty because the SQL auth
		// filter excludes chats the member cannot read.
		chats, err := memberClient.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, chats, 0)

		// Getting a specific chat returns 404 because dbauthz
		// wraps authorization failures as not-found.
		err = memberClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Title: ptr.Ref("new title"),
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		unauthenticatedClient := codersdk.NewExperimentalClient(codersdk.New(client.URL))
		_, err := unauthenticatedClient.ListChats(ctx, nil)
		requireSDKError(t, err, http.StatusUnauthorized)
	})
	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create 5 chats.
		const totalChats = 5
		createdChats := make([]codersdk.Chat, 0, totalChats)
		for i := 0; i < totalChats; i++ {
			chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content: []codersdk.ChatInputPart{
					{
						Type: codersdk.ChatInputPartTypeText,
						Text: fmt.Sprintf("chat-%d", i),
					},
				},
			})
			require.NoError(t, err)
			createdChats = append(createdChats, chat)
		}

		// Wait for all chats to reach a terminal status so
		// updated_at is stable before paginating.
		for _, c := range createdChats {
			require.Eventually(t, func() bool {
				all, listErr := client.ListChats(ctx, nil)
				if listErr != nil {
					return false
				}
				for _, ch := range all {
					if ch.ID == c.ID {
						return ch.Status != codersdk.ChatStatusPending && ch.Status != codersdk.ChatStatusRunning
					}
				}
				return false
			}, testutil.WaitLong, testutil.IntervalFast)
		}

		// Fetch first page with limit=2.
		page1, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Limit: 2},
		})
		require.NoError(t, err)
		require.Len(t, page1, 2)

		// Fetch second page using after_id from last item of page 1.
		page2, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{
				AfterID: uuid.MustParse(page1[len(page1)-1].ID.String()),
				Limit:   2,
			},
		})
		require.NoError(t, err)
		require.Len(t, page2, 2)

		// Ensure page1 and page2 have no overlap.
		page1IDs := make(map[uuid.UUID]struct{})
		for _, c := range page1 {
			page1IDs[c.ID] = struct{}{}
		}
		for _, c := range page2 {
			_, overlap := page1IDs[c.ID]
			require.False(t, overlap, "page2 should not contain items from page1")
		}

		// Fetch third page — should have 1 remaining chat.
		page3, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{
				AfterID: uuid.MustParse(page2[len(page2)-1].ID.String()),
				Limit:   2,
			},
		})
		require.NoError(t, err)
		require.Len(t, page3, 1)

		// All 5 chats should be accounted for.
		allIDs := make(map[uuid.UUID]struct{})
		for _, c := range append(append(page1, page2...), page3...) {
			allIDs[c.ID] = struct{}{}
		}
		for _, c := range createdChats {
			_, found := allIDs[c.ID]
			require.True(t, found, "chat %s should appear in paginated results", c.ID)
		}

		// Fetch with offset=3, limit=2 — should return 2 chats.
		offsetPage, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Offset: 3, Limit: 2},
		})
		require.NoError(t, err)
		require.Len(t, offsetPage, 2)

		// No limit should return all chats.
		allChats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, allChats, totalChats)
	})

	// Test that a pinned chat with an old updated_at appears on page 1.
	t.Run("PinnedOnFirstPage", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create the chat that will later be pinned. It gets the
		// earliest updated_at because it is inserted first.
		pinnedChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "pinned-chat",
			}},
		})
		require.NoError(t, err)

		// Fill page 1 with newer chats so the pinned chat would
		// normally be pushed off the first page (default limit 50).
		const fillerCount = 51
		fillerChats := make([]codersdk.Chat, 0, fillerCount)
		for i := range fillerCount {
			c, createErr := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content: []codersdk.ChatInputPart{{
					Type: codersdk.ChatInputPartTypeText,
					Text: fmt.Sprintf("filler-%d", i),
				}},
			})
			require.NoError(t, createErr)
			fillerChats = append(fillerChats, c)
		}

		// Wait for all chats to reach a terminal status so
		// updated_at is stable before paginating. A single
		// polling loop checks every chat per tick to avoid
		// O(N) separate Eventually loops.
		allCreated := append([]codersdk.Chat{pinnedChat}, fillerChats...)
		pending := make(map[uuid.UUID]struct{}, len(allCreated))
		for _, c := range allCreated {
			pending[c.ID] = struct{}{}
		}
		testutil.Eventually(ctx, t, func(_ context.Context) bool {
			all, listErr := client.ListChats(ctx, &codersdk.ListChatsOptions{
				Pagination: codersdk.Pagination{Limit: fillerCount + 10},
			})
			if listErr != nil {
				return false
			}
			for _, ch := range all {
				if _, ok := pending[ch.ID]; ok && ch.Status != codersdk.ChatStatusPending && ch.Status != codersdk.ChatStatusRunning {
					delete(pending, ch.ID)
				}
			}
			return len(pending) == 0
		}, testutil.IntervalFast)

		// Pin the earliest chat.
		err = client.UpdateChat(ctx, pinnedChat.ID, codersdk.UpdateChatRequest{
			PinOrder: ptr.Ref(int32(1)),
		})
		require.NoError(t, err)

		// Fetch page 1 with default limit (50).
		page1, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Limit: 50},
		})
		require.NoError(t, err)

		// The pinned chat must appear on page 1.
		page1IDs := make(map[uuid.UUID]struct{}, len(page1))
		for _, c := range page1 {
			page1IDs[c.ID] = struct{}{}
		}
		_, found := page1IDs[pinnedChat.ID]
		require.True(t, found, "pinned chat should appear on page 1")

		// The pinned chat should be the first item in the list.
		require.Equal(t, pinnedChat.ID, page1[0].ID, "pinned chat should be first")
	})

	// Test cursor pagination with a mix of pinned and unpinned chats.
	t.Run("CursorWithPins", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create 5 chats: 2 will be pinned, 3 unpinned.
		const totalChats = 5
		createdChats := make([]codersdk.Chat, 0, totalChats)
		for i := range totalChats {
			c, createErr := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content: []codersdk.ChatInputPart{{
					Type: codersdk.ChatInputPartTypeText,
					Text: fmt.Sprintf("cursor-pin-chat-%d", i),
				}},
			})
			require.NoError(t, createErr)
			createdChats = append(createdChats, c)
		}

		// Wait for all chats to reach terminal status.
		// Check each chat by ID rather than fetching the full list.
		testutil.Eventually(ctx, t, func(_ context.Context) bool {
			for _, c := range createdChats {
				ch, err := client.GetChat(ctx, c.ID)
				require.NoError(t, err, "GetChat should succeed for just-created chat %s", c.ID)
				if ch.Status == codersdk.ChatStatusPending || ch.Status == codersdk.ChatStatusRunning {
					return false
				}
			}
			return true
		}, testutil.IntervalFast)

		// Pin the first two chats (oldest updated_at).
		err := client.UpdateChat(ctx, createdChats[0].ID, codersdk.UpdateChatRequest{
			PinOrder: ptr.Ref(int32(1)),
		})
		require.NoError(t, err)
		err = client.UpdateChat(ctx, createdChats[1].ID, codersdk.UpdateChatRequest{
			PinOrder: ptr.Ref(int32(1)),
		})
		require.NoError(t, err)

		// Paginate with limit=2 using cursor (after_id).
		const pageSize = 2
		maxPages := totalChats/pageSize + 2
		var allPaginated []codersdk.Chat
		var afterID uuid.UUID
		for range maxPages {
			opts := &codersdk.ListChatsOptions{
				Pagination: codersdk.Pagination{Limit: pageSize},
			}
			if afterID != uuid.Nil {
				opts.Pagination.AfterID = afterID
			}
			page, listErr := client.ListChats(ctx, opts)
			require.NoError(t, listErr)
			if len(page) == 0 {
				break
			}
			allPaginated = append(allPaginated, page...)
			afterID = page[len(page)-1].ID
		}

		// All chats should appear exactly once.
		seenIDs := make(map[uuid.UUID]struct{}, len(allPaginated))
		for _, c := range allPaginated {
			_, dup := seenIDs[c.ID]
			require.False(t, dup, "chat %s appeared more than once", c.ID)
			seenIDs[c.ID] = struct{}{}
		}
		require.Len(t, seenIDs, totalChats, "all chats should appear in paginated results")

		// Pinned chats should come before unpinned ones, and
		// within the pinned group, lower pin_order sorts first.
		pinnedSeen := false
		unpinnedSeen := false
		for _, c := range allPaginated {
			if c.PinOrder > 0 {
				require.False(t, unpinnedSeen, "pinned chat %s appeared after unpinned chat", c.ID)
				pinnedSeen = true
			} else {
				unpinnedSeen = true
			}
		}
		require.True(t, pinnedSeen, "at least one pinned chat should exist")

		// Verify within-pinned ordering: pin_order=1 before
		// pin_order=2 (the -pin_order DESC column).
		require.Equal(t, createdChats[0].ID, allPaginated[0].ID,
			"pin_order=1 chat should be first")
		require.Equal(t, createdChats[1].ID, allPaginated[1].ID,
			"pin_order=2 chat should be second")
	})

	t.Run("ChildChatsEmbeddedNotStandalone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a parent chat via the API.
		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "root chat with children",
				},
			},
		})
		require.NoError(t, err)

		// Insert child chats directly via the database.
		child1 := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child one",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		child2 := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child two",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		// Also create a standalone root chat to verify it still appears.
		standalone, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "standalone root chat",
				},
			},
		})
		require.NoError(t, err)

		chats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)

		// Only root chats should appear at the top level.
		rootIDs := make(map[uuid.UUID]struct{}, len(chats))
		for _, c := range chats {
			rootIDs[c.ID] = struct{}{}
			require.Nil(t, c.ParentChatID, "top-level entry should have no parent")
		}
		require.Contains(t, rootIDs, parentChat.ID)
		require.Contains(t, rootIDs, standalone.ID)
		require.NotContains(t, rootIDs, child1.ID, "child1 should not appear at top level")
		require.NotContains(t, rootIDs, child2.ID, "child2 should not appear at top level")

		// Find the parent in the list and verify children are embedded.
		var parent codersdk.Chat
		for _, c := range chats {
			if c.ID == parentChat.ID {
				parent = c
				break
			}
		}
		require.Len(t, parent.Children, 2, "parent should embed 2 children")

		// Children are ordered by created_at DESC (newest first).
		childIDs := []uuid.UUID{parent.Children[0].ID, parent.Children[1].ID}
		require.Equal(t, child2.ID, childIDs[0])
		require.Equal(t, child1.ID, childIDs[1])

		// Verify each child has correct parent/root references.
		for _, child := range parent.Children {
			require.NotNil(t, child.ParentChatID)
			require.Equal(t, parentChat.ID, *child.ParentChatID)
			require.NotNil(t, child.RootChatID)
			require.Equal(t, parentChat.ID, *child.RootChatID)
		}

		// Standalone root chat should have an empty children slice.
		for _, c := range chats {
			if c.ID == standalone.ID {
				require.NotNil(t, c.Children)
				require.Empty(t, c.Children)
				break
			}
		}
	})

	t.Run("PaginationCountsOnlyRootChats", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create 3 root chats, each with 2 children.
		for i := range 3 {
			parent, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: user.OrganizationID,
				Content: []codersdk.ChatInputPart{
					{
						Type: codersdk.ChatInputPartTypeText,
						Text: fmt.Sprintf("parent %d", i),
					},
				},
			})
			require.NoError(t, err)
			for j := range 2 {
				_ = dbgen.Chat(t, db, database.Chat{
					OrganizationID:    user.OrganizationID,
					OwnerID:           user.UserID,
					LastModelConfigID: modelConfig.ID,
					Title:             fmt.Sprintf("child %d-%d", i, j),
					ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
					RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
				})
			}
		}

		// Request with limit=2: should get 2 root chats (not 2 of
		// the 9 total chats). Each root should have its children.
		chats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Pagination: codersdk.Pagination{Limit: 2},
		})
		require.NoError(t, err)
		require.Len(t, chats, 2, "limit should apply to root chats only")
		for _, c := range chats {
			require.Nil(t, c.ParentChatID)
			require.Len(t, c.Children, 2, "each root should embed its 2 children")
		}
	})
}

func TestListChatModels(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		models, err := client.ListChatModels(ctx)
		require.NoError(t, err)

		var openAIProvider *codersdk.ChatModelProvider
		for i := range models.Providers {
			if models.Providers[i].Provider == "openai" {
				openAIProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, openAIProvider)
		require.True(t, openAIProvider.Available)

		foundModel := false
		for _, model := range openAIProvider.Models {
			if model.Provider == "openai" && model.Model == "gpt-4o-mini" {
				foundModel = true
				break
			}
		}
		require.True(t, foundModel)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		unauthenticatedClient := codersdk.NewExperimentalClient(codersdk.New(client.URL))
		_, err := unauthenticatedClient.ListChatModels(ctx)
		requireSDKError(t, err, http.StatusUnauthorized)
	})

	t.Run("CentralOnlyProviderAvailable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		models, err := client.ListChatModels(ctx)
		require.NoError(t, err)

		var openAIProvider *codersdk.ChatModelProvider
		for i := range models.Providers {
			if models.Providers[i].Provider == "openai" {
				openAIProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, openAIProvider)
		require.True(t, openAIProvider.Available)
	})

	t.Run("UserOnlyProviderRequiresUserKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "anthropic",
			Model:        "claude-sonnet",
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)

		models, err := client.ListChatModels(ctx)
		require.NoError(t, err)

		var anthropicProvider *codersdk.ChatModelProvider
		for i := range models.Providers {
			if models.Providers[i].Provider == "anthropic" {
				anthropicProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, anthropicProvider)
		require.False(t, anthropicProvider.Available)
		require.Equal(t, codersdk.ChatModelProviderUnavailableReasonUserAPIKeyRequired, anthropicProvider.UnavailableReason)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-api-key",
		})
		require.NoError(t, err)

		models, err = client.ListChatModels(ctx)
		require.NoError(t, err)

		anthropicProvider = nil
		for i := range models.Providers {
			if models.Providers[i].Provider == "anthropic" {
				anthropicProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, anthropicProvider)
		require.True(t, anthropicProvider.Available)
	})

	t.Run("CentralAndUserWithFallback", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:                   "google",
			APIKey:                     "central-api-key",
			CentralAPIKeyEnabled:       ptr.Ref(true),
			AllowUserAPIKey:            ptr.Ref(true),
			AllowCentralAPIKeyFallback: ptr.Ref(true),
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "google",
			Model:        "gemini-1.5-pro",
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)

		models, err := client.ListChatModels(ctx)
		require.NoError(t, err)

		var googleProvider *codersdk.ChatModelProvider
		for i := range models.Providers {
			if models.Providers[i].Provider == "google" {
				googleProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, googleProvider)
		require.True(t, googleProvider.Available)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-api-key",
		})
		require.NoError(t, err)

		models, err = client.ListChatModels(ctx)
		require.NoError(t, err)

		googleProvider = nil
		for i := range models.Providers {
			if models.Providers[i].Provider == "google" {
				googleProvider = &models.Providers[i]
				break
			}
		}
		require.NotNil(t, googleProvider)
		require.True(t, googleProvider.Available)
	})

	t.Run("DisabledProvidersAndModelsAreFilteredOut", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := chatDeploymentValues(t)
		values.AI.BridgeConfig.LegacyOpenAI.Key = serpent.String("deployment-openai-key")
		client := newChatClientWithDeploymentValues(t, values)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)

		models, err := client.ListChatModels(ctx)
		require.NoError(t, err)
		require.Len(t, models.Providers, 1)
		require.Equal(t, "openai", models.Providers[0].Provider)
		require.Len(t, models.Providers[0].Models, 1)
		require.Equal(t, "gpt-4o-mini", models.Providers[0].Models[0].Model)

		enabled := false
		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			Enabled: &enabled,
		})
		require.NoError(t, err)

		models, err = client.ListChatModels(ctx)
		require.NoError(t, err)
		require.Empty(t, models.Providers)
	})
}

func TestWatchChats(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "watch route created event",
				},
			},
		})
		require.NoError(t, err)

		for {
			var payload codersdk.ChatWatchEvent
			err = wsjson.Read(ctx, conn, &payload)
			require.NoError(t, err)

			if payload.Kind == codersdk.ChatWatchEventKindCreated &&
				payload.Chat.ID == createdChat.ID {
				break
			}
		}
	})
	t.Run("CreatedEventIncludesAllChatFields", func(t *testing.T) {
		t.Parallel()

		// This test verifies that the pubsub "created" event
		// carries a fully-populated codersdk.Chat. Exhaustive
		// field-level coverage of the converter is handled by
		// TestChat_AllFieldsPopulated (db2sdk) and
		// TestChat_JSONRoundTrip (codersdk). This integration
		// test only checks that key fields survive the full
		// API → pubsub → websocket pipeline.
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "watch route fields completeness test",
				},
			},
		})
		require.NoError(t, err)

		var got codersdk.Chat
		testutil.Eventually(ctx, t, func(_ context.Context) bool {
			var payload codersdk.ChatWatchEvent
			if readErr := wsjson.Read(ctx, conn, &payload); readErr != nil {
				return false
			}
			if payload.Kind == codersdk.ChatWatchEventKindCreated &&
				payload.Chat.ID == createdChat.ID {
				got = payload.Chat
				return true
			}
			return false
		}, testutil.IntervalFast, "expected a created event for chat %s", createdChat.ID)

		require.Equal(t, createdChat.ID, got.ID)
		require.Equal(t, createdChat.OwnerID, got.OwnerID)
		require.Equal(t, modelConfig.ID, got.LastModelConfigID)
		require.Equal(t, createdChat.Title, got.Title)
		require.Equal(t, codersdk.ChatStatusPending, got.Status)
		require.NotNil(t, got.RootChatID)
		require.Equal(t, createdChat.ID, *got.RootChatID)
		require.NotZero(t, got.CreatedAt)
		require.NotZero(t, got.UpdatedAt)
	})

	t.Run("DiffStatusChangeIncludesDiffStatus", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		rawClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
		})
		client := codersdk.NewExperimentalClient(rawClient)
		db := api.Database
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Insert a chat and a diff status row.
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "diff status watch test",
		})
		refreshedAt := time.Now().UTC().Truncate(time.Second)
		staleAt := refreshedAt.Add(time.Hour)
		_, err := db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				Url:             sql.NullString{String: "https://github.com/coder/coder/pull/99", Valid: true},
				GitBranch:       "feature/test",
				GitRemoteOrigin: "git@github.com:coder/coder.git",
				StaleAt:         staleAt,
			},
		)
		require.NoError(t, err)
		_, err = db.UpsertChatDiffStatus(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusParams{
				ChatID:           chat.ID,
				Url:              sql.NullString{String: "https://github.com/coder/coder/pull/99", Valid: true},
				PullRequestState: sql.NullString{String: "open", Valid: true},
				Additions:        42,
				Deletions:        7,
				ChangedFiles:     5,
				RefreshedAt:      refreshedAt,
				StaleAt:          staleAt,
			},
		)
		require.NoError(t, err)

		// Open the watch WebSocket.
		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		// Publish a diff_status_change event via pubsub,
		// mimicking what PublishDiffStatusChange does after
		// it reads the diff status from the DB.
		dbStatus, err := db.GetChatDiffStatusByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		sdkDiffStatus := db2sdk.ChatDiffStatus(chat.ID, &dbStatus)
		event := codersdk.ChatWatchEvent{
			Kind: codersdk.ChatWatchEventKindDiffStatusChange,
			Chat: codersdk.Chat{
				ID:         chat.ID,
				OwnerID:    chat.OwnerID,
				Title:      chat.Title,
				Status:     codersdk.ChatStatus(chat.Status),
				CreatedAt:  chat.CreatedAt,
				UpdatedAt:  chat.UpdatedAt,
				DiffStatus: &sdkDiffStatus,
			},
		}
		payload, err := json.Marshal(event)
		require.NoError(t, err)

		// Publish the event in a goroutine that keeps retrying.
		// When the WebSocket Dial returns, the server has completed
		// the HTTP upgrade but may not have called SubscribeWithErr
		// yet. If we publish only once, the message can arrive
		// before the subscription is active and be silently dropped,
		// causing the read loop to block until the context deadline.
		// Re-publishing on a short ticker guarantees that at least
		// one publish lands after the subscription is ready.
		publishDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(testutil.IntervalFast)
			defer ticker.Stop()
			for {
				// Publish immediately on the first iteration,
				// then again on each tick.
				_ = api.Pubsub.Publish(coderdpubsub.ChatWatchEventChannel(user.UserID), payload)
				select {
				case <-publishDone:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()

		var received codersdk.ChatWatchEvent
		for {
			err = wsjson.Read(ctx, conn, &received)
			require.NoError(t, err)

			if received.Kind == codersdk.ChatWatchEventKindDiffStatusChange &&
				received.Chat.ID == chat.ID {
				break
			}
		}
		close(publishDone)

		// Verify the event carries the full DiffStatus.
		require.NotNil(t, received.Chat.DiffStatus, "diff_status_change event must include DiffStatus")
		ds := received.Chat.DiffStatus
		require.Equal(t, chat.ID, ds.ChatID)
		require.NotNil(t, ds.URL)
		require.Equal(t, "https://github.com/coder/coder/pull/99", *ds.URL)
		require.NotNil(t, ds.PullRequestState)
		require.Equal(t, "open", *ds.PullRequestState)
		require.EqualValues(t, 42, ds.Additions)
		require.EqualValues(t, 7, ds.Deletions)
		require.EqualValues(t, 5, ds.ChangedFiles)
	})
	t.Run("ArchiveAndUnarchiveEmitEventsForDescendants", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "watch root chat",
				},
			},
		})
		require.NoError(t, err)

		childOne := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "watch child 1",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		childTwo := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "watch child 2",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		collectLifecycleEvents := func(expectedKind codersdk.ChatWatchEventKind) map[uuid.UUID]codersdk.ChatWatchEvent {
			t.Helper()

			events := make(map[uuid.UUID]codersdk.ChatWatchEvent, 3)
			for len(events) < 3 {
				var payload codersdk.ChatWatchEvent
				err = wsjson.Read(ctx, conn, &payload)
				require.NoError(t, err)
				if payload.Kind != expectedKind {
					continue
				}
				events[payload.Chat.ID] = payload
			}
			return events
		}

		assertLifecycleEvents := func(events map[uuid.UUID]codersdk.ChatWatchEvent, archived bool) {
			t.Helper()

			require.Len(t, events, 3)
			for _, chatID := range []uuid.UUID{parentChat.ID, childOne.ID, childTwo.ID} {
				payload, ok := events[chatID]
				require.True(t, ok, "missing event for chat %s", chatID)
				require.Equal(t, archived, payload.Chat.Archived)
			}
		}

		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)
		deletedEvents := collectLifecycleEvents(codersdk.ChatWatchEventKindDeleted)
		assertLifecycleEvents(deletedEvents, true)

		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		require.NoError(t, err)
		createdEvents := collectLifecycleEvents(codersdk.ChatWatchEventKindCreated)
		assertLifecycleEvents(createdEvents, false)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		unauthenticatedClient := codersdk.New(client.URL)
		res, err := unauthenticatedClient.Request(
			ctx,
			http.MethodGet,
			"/api/experimental/chats/watch",
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func TestListChatProviders(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)

		var openAIProvider *codersdk.ChatProviderConfig
		for i := range providers {
			if providers[i].Provider == "openai" {
				openAIProvider = &providers[i]
				break
			}
		}
		require.NotNil(t, openAIProvider)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, openAIProvider.Source)
		require.True(t, openAIProvider.Enabled)
		require.True(t, openAIProvider.HasAPIKey)
	})

	t.Run("IgnoresDeploymentKeyWhenCentralKeyDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := chatDeploymentValues(t)
		values.AI.BridgeConfig.LegacyOpenAI.Key = serpent.String("deployment-openai-key")
		client := newChatClientWithDeploymentValues(t, values)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "openai",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)
		require.False(t, provider.HasAPIKey)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		for _, listed := range providers {
			if listed.Provider == "openai" {
				require.False(t, listed.HasAPIKey)
				return
			}
		}
		t.Fatal("openai provider not found")
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := memberClient.ListChatProviders(ctx)
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestCreateChatProvider(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI Primary",
			APIKey:      "test-api-key",
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, provider.ID)
		require.Equal(t, "openai", provider.Provider)
		require.Equal(t, "OpenAI Primary", provider.DisplayName)
		require.True(t, provider.Enabled)
		require.True(t, provider.HasAPIKey)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, provider.Source)
	})

	t.Run("AllowsBedrockWithCentralAPIKeyEnabledWithoutStoredKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "bedrock",
			DisplayName:          "AWS Bedrock",
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, provider.ID)
		require.Equal(t, "bedrock", provider.Provider)
		require.Equal(t, "AWS Bedrock", provider.DisplayName)
		require.True(t, provider.Enabled)
		require.False(t, provider.HasAPIKey)
		require.True(t, provider.CentralAPIKeyEnabled)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, provider.Source)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		for _, listed := range providers {
			if listed.Provider == "bedrock" {
				require.False(t, listed.HasAPIKey)
				return
			}
		}
		t.Fatal("bedrock provider not found")
	})

	t.Run("ReportsBedrockAmbientFallbackForUserConfigs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:                   "bedrock",
			DisplayName:                "AWS Bedrock Fallback",
			CentralAPIKeyEnabled:       ptr.Ref(true),
			AllowUserAPIKey:            ptr.Ref(true),
			AllowCentralAPIKeyFallback: ptr.Ref(true),
		})
		require.NoError(t, err)
		require.False(t, provider.HasAPIKey)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, provider.ID, configs[0].ProviderID)
		require.Equal(t, provider.Provider, configs[0].Provider)
		require.False(t, configs[0].HasUserAPIKey)
		require.True(t, configs[0].HasCentralAPIKeyFallback)
	})

	t.Run("AllowsBedrockWithExplicitAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "bedrock",
			DisplayName:          "AWS Bedrock Token",
			APIKey:               "bedrock-bearer-token",
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		require.NoError(t, err)
		require.Equal(t, "bedrock", provider.Provider)
		require.Equal(t, "AWS Bedrock Token", provider.DisplayName)
		require.True(t, provider.HasAPIKey)
		require.True(t, provider.CentralAPIKeyEnabled)
	})

	t.Run("RejectsMissingCentralAPIKeyForNonBedrock", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "openai",
			DisplayName:          "OpenAI",
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, missingCentralKeyMessage, sdkErr.Message)
	})

	t.Run("InvalidProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "not-a-provider",
			APIKey:   "test-api-key",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid provider.", sdkErr.Message)
	})

	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		_, err = client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "other-api-key",
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat provider already exists.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := memberClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "member-key",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("DefaultsPolicyFields", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)
		require.True(t, provider.CentralAPIKeyEnabled)
		require.False(t, provider.AllowUserAPIKey)
		require.False(t, provider.AllowCentralAPIKeyFallback)
	})

	t.Run("UserOnlyDoesNotRequireCentralKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "openai",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)
		require.False(t, provider.CentralAPIKeyEnabled)
		require.True(t, provider.AllowUserAPIKey)
		require.False(t, provider.AllowCentralAPIKeyFallback)
		require.False(t, provider.HasAPIKey)
	})

	t.Run("RejectsDeploymentBackedCentralKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := chatDeploymentValues(t)
		values.AI.BridgeConfig.LegacyOpenAI.Key = serpent.String("deployment-openai-key")
		client := newChatClientWithDeploymentValues(t, values)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, missingCentralKeyMessage, sdkErr.Message)
	})

	t.Run("RejectsInvalidPolicyTuple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		testCases := []struct {
			name     string
			central  bool
			user     bool
			fallback bool
		}{
			{
				name:     "NoneEnabled",
				central:  false,
				user:     false,
				fallback: false,
			},
			{
				name:     "FallbackWithoutCentral",
				central:  false,
				user:     true,
				fallback: true,
			},
			{
				name:     "FallbackWithoutUser",
				central:  true,
				user:     false,
				fallback: true,
			},
		}

		for _, testCase := range testCases {
			_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
				Provider:                   "openai",
				APIKey:                     "test-api-key",
				CentralAPIKeyEnabled:       ptr.Ref(testCase.central),
				AllowUserAPIKey:            ptr.Ref(testCase.user),
				AllowCentralAPIKeyFallback: ptr.Ref(testCase.fallback),
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equalf(t, "Invalid credential policy.", sdkErr.Message, "case %s", testCase.name)
		}
	})

	t.Run("RejectsTooLargeAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   strings.Repeat("a", chatProviderAPIKeySizeLimit+1),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "API key too large.", sdkErr.Message)
		require.Equal(t, fmt.Sprintf("API key exceeds maximum size of %d bytes", chatProviderAPIKeySizeLimit), sdkErr.Detail)
	})

	t.Run("AllowsMaxSizedAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   strings.Repeat("a", chatProviderAPIKeySizeLimit),
		})
		require.NoError(t, err)
		require.True(t, provider.HasAPIKey)
	})
}

func TestUpdateChatProvider(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		enabled := false
		baseURL := "https://example.com/v1"
		updated, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "OpenAI Updated",
			Enabled:     &enabled,
			BaseURL:     &baseURL,
		})
		require.NoError(t, err)
		require.Equal(t, provider.ID, updated.ID)
		require.Equal(t, "OpenAI Updated", updated.DisplayName)
		require.False(t, updated.Enabled)
		require.Equal(t, baseURL, updated.BaseURL)
	})

	t.Run("AllowsClearingBedrockAPIKeyWithCentralAPIKeyEnabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "bedrock",
			DisplayName:          "AWS Bedrock",
			APIKey:               "bedrock-bearer-token",
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		require.NoError(t, err)
		require.True(t, provider.HasAPIKey)
		require.True(t, provider.CentralAPIKeyEnabled)

		updated, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			APIKey:               ptr.Ref(""),
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		require.NoError(t, err)
		require.Equal(t, provider.ID, updated.ID)
		require.Equal(t, "bedrock", updated.Provider)
		require.False(t, updated.HasAPIKey)
		require.True(t, updated.CentralAPIKeyEnabled)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UpdateChatProvider(ctx, uuid.New(), codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "missing",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidProviderID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		res, err := client.Request(
			ctx,
			http.MethodPatch,
			"/api/experimental/chats/providers/not-a-uuid",
			codersdk.UpdateChatProviderConfigRequest{DisplayName: "ignored"},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat provider ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		provider, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		_, err = memberClient.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "member update",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("AppliesPolicyOverrides", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		updated, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)
		require.True(t, updated.AllowUserAPIKey)
		require.False(t, updated.CentralAPIKeyEnabled)
		require.False(t, updated.HasAPIKey)
	})

	t.Run("RejectsDeploymentBackedCentralKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := chatDeploymentValues(t)
		values.AI.BridgeConfig.LegacyOpenAI.Key = serpent.String("deployment-openai-key")
		client := newChatClientWithDeploymentValues(t, values)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "openai",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, missingCentralKeyMessage, sdkErr.Message)
	})

	t.Run("RejectsClearingLastCentralKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			APIKey: ptr.Ref(""),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, missingCentralKeyMessage, sdkErr.Message)
	})

	t.Run("RejectsEnablingCentralKeyWithoutKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "openai",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			CentralAPIKeyEnabled: ptr.Ref(true),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, missingCentralKeyMessage, sdkErr.Message)
	})

	t.Run("RejectsInvalidPolicyTuple", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		testCases := []struct {
			name     string
			central  bool
			user     bool
			fallback bool
		}{
			{
				name:     "NoneEnabled",
				central:  false,
				user:     false,
				fallback: false,
			},
			{
				name:     "FallbackWithoutCentral",
				central:  false,
				user:     true,
				fallback: true,
			},
			{
				name:     "FallbackWithoutUser",
				central:  true,
				user:     false,
				fallback: true,
			},
		}

		for _, testCase := range testCases {
			_, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
				CentralAPIKeyEnabled:       ptr.Ref(testCase.central),
				AllowUserAPIKey:            ptr.Ref(testCase.user),
				AllowCentralAPIKeyFallback: ptr.Ref(testCase.fallback),
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equalf(t, "Invalid credential policy.", sdkErr.Message, "case %s", testCase.name)
		}
	})

	t.Run("RejectsTooLargeAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			APIKey: ptr.Ref(strings.Repeat("a", chatProviderAPIKeySizeLimit+1)),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "API key too large.", sdkErr.Message)
		require.Equal(t, fmt.Sprintf("API key exceeds maximum size of %d bytes", chatProviderAPIKeySizeLimit), sdkErr.Detail)
	})

	t.Run("AllowsMaxSizedAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		updated, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			APIKey: ptr.Ref(strings.Repeat("a", chatProviderAPIKeySizeLimit)),
		})
		require.NoError(t, err)
		require.True(t, updated.HasAPIKey)
	})
}

func TestDeleteChatProvider(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		err = client.DeleteChatProvider(ctx, provider.ID)
		require.NoError(t, err)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		for _, listed := range providers {
			require.NotEqual(t, provider.ID, listed.ID)
		}
	})

	t.Run("SuccessWithHistoricalChats", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		providerToDelete, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:        "openai",
			APIKey:          "delete-api-key",
			AllowUserAPIKey: ptr.Ref(true),
		})
		require.NoError(t, err)

		deleteContextLimit := int64(4096)
		deleteIsDefault := true
		configToDelete, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     providerToDelete.Provider,
			Model:        "gpt-4o-delete-provider",
			ContextLimit: &deleteContextLimit,
			IsDefault:    &deleteIsDefault,
		})
		require.NoError(t, err)

		keepProvider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "anthropic",
			APIKey:   "keep-api-key",
		})
		require.NoError(t, err)

		keepContextLimit := int64(8192)
		keepConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     keepProvider.Provider,
			Model:        "claude-keep-provider",
			ContextLimit: &keepContextLimit,
		})
		require.NoError(t, err)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			ModelConfigID:  ptr.Ref(configToDelete.ID),
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "provider delete history " + t.Name(),
			}},
		})
		require.NoError(t, err)
		require.Equal(t, configToDelete.ID, chat.LastModelConfigID)

		insertAssistantCostMessage(t, db, chat.ID, configToDelete.ID, 500)

		_, err = client.UpsertUserChatProviderKey(ctx, providerToDelete.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-delete-key",
		})
		require.NoError(t, err)

		userKeys, err := db.GetUserChatProviderKeys(dbauthz.AsSystemRestricted(ctx), firstUser.UserID)
		require.NoError(t, err)
		require.Len(t, userKeys, 1)
		require.Equal(t, providerToDelete.ID, userKeys[0].ChatProviderID)

		err = client.DeleteChatProvider(ctx, providerToDelete.ID)
		require.NoError(t, err)

		_, err = db.GetChatProviderByID(dbauthz.AsSystemRestricted(ctx), providerToDelete.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		foundKeepProvider := false
		for _, listed := range providers {
			require.NotEqual(t, providerToDelete.ID, listed.ID)
			if listed.ID == keepProvider.ID {
				foundKeepProvider = true
			}
		}
		require.True(t, foundKeepProvider)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		foundDeletedConfig := false
		foundKeepConfig := false
		for _, config := range configs {
			if config.ID == configToDelete.ID {
				foundDeletedConfig = true
			}
			if config.ID == keepConfig.ID {
				foundKeepConfig = true
				require.True(t, config.IsDefault)
			}
		}
		require.False(t, foundDeletedConfig)
		require.True(t, foundKeepConfig)

		defaultConfig, err := db.GetDefaultChatModelConfig(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, keepConfig.ID, defaultConfig.ID)

		_, err = db.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), configToDelete.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		gotChat, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, gotChat.ID)
		require.Equal(t, configToDelete.ID, gotChat.LastModelConfigID)

		messages, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		foundHistoricalMessage := false
		for _, message := range messages.Messages {
			if message.ModelConfigID != nil && *message.ModelConfigID == configToDelete.ID {
				foundHistoricalMessage = true
				break
			}
		}
		require.True(t, foundHistoricalMessage)

		userKeys, err = db.GetUserChatProviderKeys(dbauthz.AsSystemRestricted(ctx), firstUser.UserID)
		require.NoError(t, err)
		require.Empty(t, userKeys)
	})

	t.Run("SuccessWithHistoricalChatsAndNoReplacementConfig", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "only-provider-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		isDefault := true
		config, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     provider.Provider,
			Model:        "gpt-4o-only-provider",
			ContextLimit: &contextLimit,
			IsDefault:    &isDefault,
		})
		require.NoError(t, err)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			ModelConfigID:  ptr.Ref(config.ID),
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "only provider delete history " + t.Name(),
			}},
		})
		require.NoError(t, err)
		require.Equal(t, config.ID, chat.LastModelConfigID)

		insertAssistantCostMessage(t, db, chat.ID, config.ID, 250)

		err = client.DeleteChatProvider(ctx, provider.ID)
		require.NoError(t, err)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		for _, listed := range providers {
			require.NotEqual(t, provider.ID, listed.ID)
		}

		_, err = db.GetChatProviderByID(dbauthz.AsSystemRestricted(ctx), provider.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		_, err = db.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), config.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		_, err = db.GetDefaultChatModelConfig(dbauthz.AsSystemRestricted(ctx))
		require.ErrorIs(t, err, sql.ErrNoRows)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Empty(t, configs)

		gotChat, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, config.ID, gotChat.LastModelConfigID)

		messages, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		foundHistoricalMessage := false
		for _, message := range messages.Messages {
			if message.ModelConfigID != nil && *message.ModelConfigID == config.ID {
				foundHistoricalMessage = true
				break
			}
		}
		require.True(t, foundHistoricalMessage)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		err := client.DeleteChatProvider(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidProviderID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		res, err := client.Request(
			ctx,
			http.MethodDelete,
			"/api/experimental/chats/providers/not-a-uuid",
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat provider ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		provider, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		err = memberClient.DeleteChatProvider(ctx, provider.ID)
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestChatProviderAPIKeysFromDeploymentValues(t *testing.T) {
	t.Parallel()

	t.Run("DoesNotReuseBridgeConfig", func(t *testing.T) {
		t.Parallel()

		values := chatDeploymentValues(t)
		values.AI.BridgeConfig.LegacyOpenAI.Key = serpent.String("deployment-openai-key")
		values.AI.BridgeConfig.LegacyAnthropic.Key = serpent.String("deployment-anthropic-key")
		values.AI.BridgeConfig.LegacyOpenAI.BaseURL = serpent.String("https://custom-openai.example.com")

		keys := coderd.ChatProviderAPIKeysFromDeploymentValues(values)
		require.Equal(t, chatprovider.ProviderAPIKeys{}, keys)
	})

	t.Run("NilDeploymentValues", func(t *testing.T) {
		t.Parallel()

		keys := coderd.ChatProviderAPIKeysFromDeploymentValues(nil)
		require.Equal(t, chatprovider.ProviderAPIKeys{}, keys)
	})
}

func TestUserChatProviderConfigs(t *testing.T) {
	t.Parallel()

	requireUserProviderConfig := func(t *testing.T, configs []codersdk.UserChatProviderConfig, provider string) codersdk.UserChatProviderConfig {
		t.Helper()

		for _, config := range configs {
			if config.Provider == provider {
				return config
			}
		}

		t.Fatalf("provider %q not found", provider)
		return codersdk.UserChatProviderConfig{}
	}

	requireNoUserProviderConfig := func(t *testing.T, configs []codersdk.UserChatProviderConfig, provider string) {
		t.Helper()

		for _, config := range configs {
			if config.Provider == provider {
				t.Fatalf("provider %q unexpectedly found", provider)
			}
		}
	}

	t.Run("ListOnlyUserKeyProviders", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		anthropicProvider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "google",
			APIKey:   "central-api-key",
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, anthropicProvider.ID, configs[0].ProviderID)
		require.Equal(t, anthropicProvider.Provider, configs[0].Provider)
	})

	t.Run("ListReportsHasUserAPIKeyFalse", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, provider.ID, configs[0].ProviderID)
		require.False(t, configs[0].HasUserAPIKey)
	})

	t.Run("ListHidesDisabledProviderEvenWithSavedKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		require.NoError(t, err)

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			Enabled: ptr.Ref(false),
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		require.Empty(t, configs)
		requireNoUserProviderConfig(t, configs, "anthropic")
	})

	t.Run("ListHidesUserKeyDisabledProviderAndRestoresOnReEnable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		require.NoError(t, err)

		centralAPIKey := "central-key"
		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			APIKey:               &centralAPIKey,
			CentralAPIKeyEnabled: ptr.Ref(true),
			AllowUserAPIKey:      ptr.Ref(false),
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		require.Empty(t, configs)
		requireNoUserProviderConfig(t, configs, "anthropic")

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			AllowUserAPIKey: ptr.Ref(true),
		})
		require.NoError(t, err)

		configs, err = client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed := requireUserProviderConfig(t, configs, "anthropic")
		require.Equal(t, provider.ID, listed.ProviderID)
		require.True(t, listed.HasUserAPIKey)
		require.False(t, listed.HasCentralAPIKeyFallback)
	})

	t.Run("UpsertCreatesKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:                   "anthropic",
			APIKey:                     "central-key",
			CentralAPIKeyEnabled:       ptr.Ref(true),
			AllowUserAPIKey:            ptr.Ref(true),
			AllowCentralAPIKeyFallback: ptr.Ref(true),
		})
		require.NoError(t, err)

		config, err := client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		require.NoError(t, err)
		require.Equal(t, provider.ID, config.ProviderID)
		require.Equal(t, provider.Provider, config.Provider)
		require.Equal(t, provider.DisplayName, config.DisplayName)
		require.True(t, config.HasUserAPIKey)
		require.True(t, config.HasCentralAPIKeyFallback)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed := requireUserProviderConfig(t, configs, "anthropic")
		require.Equal(t, provider.ID, listed.ProviderID)
		require.Equal(t, provider.DisplayName, listed.DisplayName)
		require.True(t, listed.HasUserAPIKey)
		require.True(t, listed.HasCentralAPIKeyFallback)
	})

	t.Run("ListRecomputesFallbackAvailability", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := chatDeploymentValues(t)
		values.AI.BridgeConfig.LegacyOpenAI.Key = serpent.String("deployment-openai-key")
		client := newChatClientWithDeploymentValues(t, values)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:                   "openai",
			APIKey:                     "test-central-key",
			AllowUserAPIKey:            ptr.Ref(true),
			AllowCentralAPIKeyFallback: ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed := requireUserProviderConfig(t, configs, "openai")
		require.True(t, listed.HasCentralAPIKeyFallback)

		_, err = client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			CentralAPIKeyEnabled:       ptr.Ref(false),
			AllowCentralAPIKeyFallback: ptr.Ref(false),
		})
		require.NoError(t, err)

		configs, err = client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed = requireUserProviderConfig(t, configs, "openai")
		require.False(t, listed.HasCentralAPIKeyFallback)
	})

	t.Run("UpsertUpdatesKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "key-1",
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "key-2",
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed := requireUserProviderConfig(t, configs, "anthropic")
		require.True(t, listed.HasUserAPIKey)
	})

	t.Run("UpsertRejectsMissingProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UpsertUserChatProviderKey(ctx, uuid.New(), codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("UpsertRejectsDisabledProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			Enabled:              ptr.Ref(false),
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Provider is disabled.", sdkErr.Message)
	})

	t.Run("UpsertRejectsProviderWithoutUserKeys", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "google",
			APIKey:   "central-api-key",
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Provider does not allow user API keys.", sdkErr.Message)
	})

	t.Run("UpsertRejectsEmptyAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "API key is required.", sdkErr.Message)
	})

	t.Run("DeleteRemovesKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "user-key",
		})
		require.NoError(t, err)

		configs, err := client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed := requireUserProviderConfig(t, configs, "anthropic")
		require.True(t, listed.HasUserAPIKey)

		err = client.DeleteUserChatProviderKey(ctx, provider.ID)
		require.NoError(t, err)

		configs, err = client.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed = requireUserProviderConfig(t, configs, "anthropic")
		require.False(t, listed.HasUserAPIKey)

		err = client.DeleteUserChatProviderKey(ctx, provider.ID)
		require.NoError(t, err)
	})

	t.Run("OtherUserDoesNotSeeKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)

		provider, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = adminClient.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: "admin-user-key",
		})
		require.NoError(t, err)

		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		configs, err := memberClient.ListUserChatProviderConfigs(ctx)
		require.NoError(t, err)
		listed := requireUserProviderConfig(t, configs, "anthropic")
		require.Equal(t, provider.ID, listed.ProviderID)
		require.False(t, listed.HasUserAPIKey)
	})
}

func TestUpsertUserChatProviderKey(t *testing.T) {
	t.Parallel()

	t.Run("RejectsTooLargeAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: strings.Repeat("a", chatProviderAPIKeySizeLimit+1),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "API key too large.", sdkErr.Message)
		require.Equal(t, fmt.Sprintf("API key exceeds maximum size of %d bytes", chatProviderAPIKeySizeLimit), sdkErr.Detail)
	})

	t.Run("AllowsMaxSizedAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:             "anthropic",
			CentralAPIKeyEnabled: ptr.Ref(false),
			AllowUserAPIKey:      ptr.Ref(true),
		})
		require.NoError(t, err)

		config, err := client.UpsertUserChatProviderKey(ctx, provider.ID, codersdk.CreateUserChatProviderKeyRequest{
			APIKey: strings.Repeat("a", chatProviderAPIKeySizeLimit),
		})
		require.NoError(t, err)
		require.True(t, config.HasUserAPIKey)
	})
}

func TestListChatModelConfigs(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, configs)

		found := false
		for _, config := range configs {
			if config.ID == modelConfig.ID {
				found = true
				require.Equal(t, "openai", config.Provider)
				require.Equal(t, "gpt-4o-mini", config.Model)
				require.True(t, config.IsDefault)
			}
		}
		require.True(t, found)
	})

	t.Run("AdminIncludesDisabledModelConfigs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		enabled := false
		disabledConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-disabled",
			DisplayName:  "GPT-4o Disabled",
			Enabled:      &enabled,
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)
		require.False(t, disabledConfig.Enabled)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)

		found := false
		for _, config := range configs {
			if config.ID == disabledConfig.ID {
				found = true
				require.False(t, config.Enabled)
				require.Equal(t, disabledConfig.DisplayName, config.DisplayName)
			}
		}
		require.True(t, found)
	})

	t.Run("NonAdminExcludesDisabledModelConfigs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		enabledConfig := createChatModelConfig(t, adminClient)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		contextLimit := int64(4096)
		enabled := false
		_, err := adminClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-disabled",
			DisplayName:  "GPT-4o Disabled",
			Enabled:      &enabled,
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)

		configs, err := memberClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, enabledConfig.ID, configs[0].ID)
		require.True(t, configs[0].Enabled)
	})

	t.Run("DeserializesLegacyPricingJSON", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		legacyOptions := json.RawMessage(`{"input_price_per_million_tokens":0.15,"output_price_per_million_tokens":0.6,"cache_read_price_per_million_tokens":0.03,"cache_write_price_per_million_tokens":0.3}`)
		storedConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Provider:             "openai",
			Model:                "gpt-4o-mini-legacy",
			DisplayName:          "GPT-4o Mini Legacy",
			CreatedBy:            uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
			UpdatedBy:            uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
			ContextLimit:         4096,
			CompressionThreshold: 80,
			Options:              legacyOptions,
		})

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, storedConfig.ID, configs[0].ID)
		requireChatModelPricing(t, configs[0].ModelConfig, &codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decRef("0.15"),
				OutputPricePerMillionTokens:     decRef("0.6"),
				CacheReadPricePerMillionTokens:  decRef("0.03"),
				CacheWritePricePerMillionTokens: decRef("0.3"),
			},
		})
	})

	t.Run("SuccessForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		modelConfig := createChatModelConfig(t, adminClient)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		// Non-admin users should see only enabled model configs.
		configs, err := memberClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, configs)

		found := false
		for _, config := range configs {
			if config.ID == modelConfig.ID {
				found = true
				require.Equal(t, "openai", config.Provider)
				require.Equal(t, "gpt-4o-mini", config.Model)
			}
		}
		require.True(t, found)
	})
}

func TestCreateChatModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		isDefault := true
		pricing := &codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decRef("0.15"),
				OutputPricePerMillionTokens:     decRef("0.6"),
				CacheReadPricePerMillionTokens:  decRef("0.03"),
				CacheWritePricePerMillionTokens: decRef("0.3"),
			},
		}
		modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
			IsDefault:    &isDefault,
			ModelConfig:  pricing,
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, modelConfig.ID)
		require.Equal(t, "openai", modelConfig.Provider)
		require.Equal(t, "gpt-4o-mini", modelConfig.Model)
		require.EqualValues(t, 4096, modelConfig.ContextLimit)
		require.True(t, modelConfig.IsDefault)
		requireChatModelPricing(t, modelConfig.ModelConfig, pricing)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		requireChatModelPricing(t, configs[0].ModelConfig, pricing)
	})

	t.Run("RejectsNegativePricing", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
			ModelConfig: &codersdk.ChatModelCallConfig{
				Cost: &codersdk.ModelCostConfig{
					InputPricePerMillionTokens: decRef("-0.01"),
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid model config.", sdkErr.Message)
		require.Equal(
			t,
			"cost.input_price_per_million_tokens must be greater than or equal to zero",
			sdkErr.Detail,
		)
	})

	t.Run("MissingContextLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider: "openai",
			Model:    "gpt-4o-mini",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Context limit is required.", sdkErr.Message)
	})

	t.Run("ProviderNotConfigured", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		contextLimit := int64(4096)
		_, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat provider is not configured.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		_, err = memberClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			ContextLimit: &contextLimit,
		})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestUpdateChatModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		contextLimit := int64(8192)
		pricing := &codersdk.ChatModelCallConfig{
			Cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decRef("0.2"),
				OutputPricePerMillionTokens:     decRef("0.8"),
				CacheReadPricePerMillionTokens:  decRef("0.04"),
				CacheWritePricePerMillionTokens: decRef("0.4"),
			},
		}
		updated, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName:  "GPT-4o Mini Updated",
			ContextLimit: &contextLimit,
			ModelConfig:  pricing,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, updated.ID)
		require.Equal(t, "GPT-4o Mini Updated", updated.DisplayName)
		require.EqualValues(t, 8192, updated.ContextLimit)
		requireChatModelPricing(t, updated.ModelConfig, pricing)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		requireChatModelPricing(t, configs[0].ModelConfig, pricing)
	})

	t.Run("DisablePreservesRecordAndHidesItFromNonAdmins", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		modelConfig := createChatModelConfig(t, adminClient)

		enabled := false
		updated, err := adminClient.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			Enabled: &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, updated.ID)
		require.False(t, updated.Enabled)

		adminConfigs, err := adminClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)

		foundForAdmin := false
		for _, config := range adminConfigs {
			if config.ID == modelConfig.ID {
				foundForAdmin = true
				require.False(t, config.Enabled)
			}
		}
		require.True(t, foundForAdmin)

		memberConfigs, err := memberClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		for _, config := range memberConfigs {
			require.NotEqual(t, modelConfig.ID, config.ID)
		}
	})

	t.Run("ReEnableRestoresVisibilityForNonAdmins", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := adminClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "test-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		enabled := false
		modelConfig, err := adminClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "openai",
			Model:        "gpt-4o-reenable",
			DisplayName:  "GPT-4o Re-enable",
			Enabled:      &enabled,
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)
		require.False(t, modelConfig.Enabled)

		memberConfigs, err := memberClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)

		foundForMember := false
		for _, config := range memberConfigs {
			if config.ID == modelConfig.ID {
				foundForMember = true
			}
		}
		require.False(t, foundForMember)

		enabled = true
		updated, err := adminClient.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			Enabled: &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, updated.ID)
		require.True(t, updated.Enabled)

		memberConfigs, err = memberClient.ListChatModelConfigs(ctx)
		require.NoError(t, err)

		foundForMember = false
		for _, config := range memberConfigs {
			if config.ID == modelConfig.ID {
				foundForMember = true
				require.True(t, config.Enabled)
			}
		}
		require.True(t, foundForMember)
	})

	t.Run("RejectsNegativePricing", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		_, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			ModelConfig: &codersdk.ChatModelCallConfig{
				Cost: &codersdk.ModelCostConfig{
					OutputPricePerMillionTokens: decRef("-1.0"),
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid model config.", sdkErr.Message)
		require.Equal(
			t,
			"cost.output_price_per_million_tokens must be greater than or equal to zero",
			sdkErr.Detail,
		)
	})

	t.Run("ProviderNotConfigured", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		_, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			Provider: "anthropic",
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat provider is not configured.", sdkErr.Message)
	})

	t.Run("NotFoundWhenTargetRowDisappearsInTx", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		rawDB, pubsub := dbtestutil.NewDB(t)
		store := newFailNextUpdateChatModelConfigStore(rawDB)
		client := codersdk.NewExperimentalClient(coderdtest.New(t, &coderdtest.Options{
			Database:         store,
			Pubsub:           pubsub,
			DeploymentValues: chatDeploymentValues(t),
		}))
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		store.failNextUpdateChatModelConfigID = modelConfig.ID
		store.failNextUpdateChatModelConfig.Store(true)

		_, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName: "missing in tx",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InternalServerErrorWhenDefaultCandidateDisappearsInTx", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		rawDB, pubsub := dbtestutil.NewDB(t)
		store := newFailNextUpdateChatModelConfigStore(rawDB)
		client := codersdk.NewExperimentalClient(coderdtest.New(t, &coderdtest.Options{
			Database:         store,
			Pubsub:           pubsub,
			DeploymentValues: chatDeploymentValues(t),
		}))
		_ = coderdtest.CreateFirstUser(t, client.Client)
		defaultConfig := createChatModelConfig(t, client)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "anthropic",
			APIKey:   "candidate-api-key",
		})
		require.NoError(t, err)

		contextLimit := int64(4096)
		isDefault := false
		candidateConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:     "anthropic",
			Model:        "claude-3-5-sonnet",
			ContextLimit: &contextLimit,
			IsDefault:    &isDefault,
		})
		require.NoError(t, err)

		store.failNextUpdateChatModelConfigID = candidateConfig.ID
		store.failNextUpdateChatModelConfig.Store(true)

		_, err = client.UpdateChatModelConfig(ctx, defaultConfig.ID, codersdk.UpdateChatModelConfigRequest{
			IsDefault: ptr.Ref(false),
		})
		sdkErr := requireSDKError(t, err, http.StatusInternalServerError)
		require.Equal(t, "Failed to update chat model config.", sdkErr.Message)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UpdateChatModelConfig(ctx, uuid.New(), codersdk.UpdateChatModelConfigRequest{
			DisplayName: "missing",
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidContextLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		contextLimit := int64(0)
		_, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			ContextLimit: &contextLimit,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Context limit must be greater than zero.", sdkErr.Message)
	})

	t.Run("InvalidModelConfigID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		res, err := client.Request(
			ctx,
			http.MethodPatch,
			"/api/experimental/chats/model-configs/not-a-uuid",
			codersdk.UpdateChatModelConfigRequest{DisplayName: "ignored"},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat model config ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		modelConfig := createChatModelConfig(t, adminClient)
		_, err := memberClient.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName: "member update",
		})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestDeleteChatModelConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		err := client.DeleteChatModelConfig(ctx, modelConfig.ID)
		require.NoError(t, err)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		for _, config := range configs {
			require.NotEqual(t, modelConfig.ID, config.ID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		err := client.DeleteChatModelConfig(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidModelConfigID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		res, err := client.Request(
			ctx,
			http.MethodDelete,
			"/api/experimental/chats/model-configs/not-a-uuid",
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat model config ID.", sdkErr.Message)
	})

	t.Run("ForbiddenForOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		modelConfig := createChatModelConfig(t, adminClient)
		err := memberClient.DeleteChatModelConfig(ctx, modelConfig.ID)
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestGetChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "get chat route payload",
				},
			},
		})
		require.NoError(t, err)

		chatResult, err := client.GetChat(ctx, createdChat.ID)
		require.NoError(t, err)
		messagesResult, err := client.GetChatMessages(ctx, createdChat.ID, nil)
		require.NoError(t, err)
		require.Equal(t, createdChat.ID, chatResult.ID)
		require.Equal(t, firstUser.UserID, chatResult.OwnerID)
		require.Equal(t, modelConfig.ID, chatResult.LastModelConfigID)
		require.Equal(t, "get chat route payload", chatResult.Title)
		require.NotZero(t, chatResult.CreatedAt)
		require.NotZero(t, chatResult.UpdatedAt)
		require.NotEmpty(t, messagesResult.Messages)
		require.Empty(t, messagesResult.QueuedMessages)

		foundUserMessage := false
		for _, message := range messagesResult.Messages {
			require.Equal(t, createdChat.ID, message.ChatID)
			require.NotEqual(t, codersdk.ChatMessageRoleSystem, message.Role)
			for _, part := range message.Content {
				if message.Role == codersdk.ChatMessageRoleUser &&
					part.Type == codersdk.ChatMessagePartTypeText &&
					part.Text == "get chat route payload" {
					foundUserMessage = true
				}
			}
		}
		require.True(t, foundUserMessage)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)
		_, err = otherClient.GetChat(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("FilesHydrated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "hydrated.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with a text + file part.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "check file hydration"}, {Type: codersdk.ChatInputPartTypeFile, FileID: uploadResp.ID},
			},
		})
		require.NoError(t, err)

		// GET the chat — files must be hydrated with all metadata fields.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, 1)
		f := chatResult.Files[0]
		require.Equal(t, uploadResp.ID, f.ID)
		require.Equal(t, firstUser.UserID, f.OwnerID)
		require.NotEqual(t, uuid.Nil, f.OrganizationID)
		require.Equal(t, "image/png", f.MimeType)
		require.Equal(t, "hydrated.png", f.Name)
		require.NotZero(t, f.CreatedAt)
	})

	// ToolCreatedFilesLinked exercises the DB path that chatd uses
	// when a tool (e.g. propose_plan) creates a file: InsertChatFile
	// then LinkChatFiles. This is a DB-level test because driving
	// the full chatd tool-call pipeline requires an LLM mock.
	t.Run("ToolCreatedFilesLinked", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, store := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create a chat via the API so all metadata is set up.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "tool file test"},
			},
		})
		require.NoError(t, err)

		// Mimic what chatd's StoreFile closure does:
		// 1. InsertChatFile
		// 2. LinkChatFiles
		//nolint:gocritic // Using AsChatd to mimic the chatd background worker.
		chatdCtx := dbauthz.AsChatd(ctx)
		fileRow, err := store.InsertChatFile(chatdCtx, database.InsertChatFileParams{
			OwnerID:        firstUser.UserID,
			OrganizationID: firstUser.OrganizationID,
			Name:           "plan.md",
			Mimetype:       "text/markdown",
			Data:           []byte("# Plan"),
		})
		require.NoError(t, err)

		rejected, err := store.LinkChatFiles(chatdCtx, database.LinkChatFilesParams{
			ChatID:       chat.ID,
			MaxFileLinks: int32(codersdk.MaxChatFileIDs),
			FileIds:      []uuid.UUID{fileRow.ID},
		})
		require.NoError(t, err)
		require.Equal(t, int32(0), rejected, "0 rejected = all files linked")

		// Verify via the API that the file appears in the chat.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, 1)
		f := chatResult.Files[0]
		require.Equal(t, fileRow.ID, f.ID)
		require.Equal(t, firstUser.UserID, f.OwnerID)
		require.Equal(t, firstUser.OrganizationID, f.OrganizationID)
		require.Equal(t, "plan.md", f.Name)
		require.Equal(t, "text/markdown", f.MimeType)

		// Fill up to the cap by inserting more files via the
		// chatd DB path, then verify the cap is enforced.
		for i := 1; i < codersdk.MaxChatFileIDs; i++ {
			extra, err := store.InsertChatFile(chatdCtx, database.InsertChatFileParams{
				OwnerID:        firstUser.UserID,
				OrganizationID: firstUser.OrganizationID,
				Name:           fmt.Sprintf("file%d.md", i),
				Mimetype:       "text/markdown",
				Data:           []byte("data"),
			})
			require.NoError(t, err)
			_, err = store.LinkChatFiles(chatdCtx, database.LinkChatFilesParams{
				ChatID:       chat.ID,
				MaxFileLinks: int32(codersdk.MaxChatFileIDs),
				FileIds:      []uuid.UUID{extra.ID},
			})
			require.NoError(t, err)
		}

		// Chat should now have exactly MaxChatFileIDs files.
		chatResult, err = client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, codersdk.MaxChatFileIDs)

		// Attempt to add one more file — should be rejected (0 rows).
		overflow, err := store.InsertChatFile(chatdCtx, database.InsertChatFileParams{
			OwnerID:        firstUser.UserID,
			OrganizationID: firstUser.OrganizationID,
			Name:           "overflow.md",
			Mimetype:       "text/markdown",
			Data:           []byte("too many"),
		})
		require.NoError(t, err)
		rejected, err = store.LinkChatFiles(chatdCtx, database.LinkChatFilesParams{
			ChatID:       chat.ID,
			MaxFileLinks: int32(codersdk.MaxChatFileIDs),
			FileIds:      []uuid.UUID{overflow.ID},
		})
		require.NoError(t, err)
		require.Equal(t, int32(1), rejected, "cap should reject the 21st file")

		// Re-appending an already-linked ID at cap should succeed
		// (dedup means no array growth).
		rejected, err = store.LinkChatFiles(chatdCtx, database.LinkChatFilesParams{
			ChatID:       chat.ID,
			MaxFileLinks: int32(codersdk.MaxChatFileIDs),
			FileIds:      []uuid.UUID{fileRow.ID},
		})
		require.NoError(t, err)
		// ON CONFLICT DO NOTHING returns 0 rows when the link
		// already exists, which is fine — the file is still linked.
		require.Equal(t, int32(0), rejected, "dedup of existing ID should be a no-op")

		// Count should still be exactly MaxChatFileIDs.
		chatResult, err = client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, codersdk.MaxChatFileIDs)
	})

	t.Run("GetChatEmbedsChildren", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent for getChat",
				},
			},
		})
		require.NoError(t, err)

		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child for getChat",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		// Fetching the root chat should embed its children.
		result, err := client.GetChat(ctx, parentChat.ID)
		require.NoError(t, err)
		require.Len(t, result.Children, 1)
		require.Equal(t, child.ID, result.Children[0].ID)
		require.NotNil(t, result.Children[0].ParentChatID)
		require.Equal(t, parentChat.ID, *result.Children[0].ParentChatID)

		// Fetching a child chat should not have children.
		childResult, err := client.GetChat(ctx, child.ID)
		require.NoError(t, err)
		require.NotNil(t, childResult.Children)
		require.Empty(t, childResult.Children)

		// An archived root should still embed its cascaded
		// archived children (guards against the filter getting
		// hardcoded to false).
		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		archivedResult, err := client.GetChat(ctx, parentChat.ID)
		require.NoError(t, err)
		require.True(t, archivedResult.Archived, "root should be archived")
		require.Len(t, archivedResult.Children, 1, "archived root should embed its archived child")
		require.Equal(t, child.ID, archivedResult.Children[0].ID)
		require.True(t, archivedResult.Children[0].Archived, "embedded child should be archived")
	})
}

func TestPatchChat(t *testing.T) {
	t.Parallel()

	createChat := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, text string) codersdk.Chat {
		t.Helper()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: orgID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: text,
				},
			},
		})
		require.NoError(t, err)
		return chat
	}

	getChat := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) codersdk.Chat {
		t.Helper()

		chat, err := client.GetChat(ctx, chatID)
		require.NoError(t, err)
		return chat
	}

	createStoredChat := func(
		ctx context.Context,
		t *testing.T,
		db database.Store,
		ownerID uuid.UUID,
		orgID uuid.UUID,
		modelConfigID uuid.UUID,
		title string,
	) codersdk.Chat {
		t.Helper()

		dbChat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    orgID,
			OwnerID:           ownerID,
			LastModelConfigID: modelConfigID,
			Title:             title,
		})
		return db2sdk.Chat(dbChat, nil, nil)
	}

	// waitChatSettled polls the chat until its background title-generation
	// daemon has left the Pending/Running state. Without this, an immediate
	// UpdateChat can hit a 409 (title regeneration in progress).
	waitChatSettled := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) {
		t.Helper()
		require.Eventually(t, func() bool {
			c, err := client.GetChat(ctx, chatID)
			if err != nil {
				return false
			}
			return c.Status != codersdk.ChatStatusPending &&
				c.Status != codersdk.ChatStatusRunning
		}, testutil.WaitShort, testutil.IntervalFast)
	}

	t.Run("PlanMode", func(t *testing.T) {
		t.Parallel()

		t.Run("SetToPlan", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client := newChatClient(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "set plan mode")
			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				PlanMode: ptr.Ref(codersdk.ChatPlanModePlan),
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, codersdk.ChatPlanModePlan, updated.PlanMode)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})

		t.Run("Clear", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client := newChatClient(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "clear plan mode")
			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				PlanMode: ptr.Ref(codersdk.ChatPlanModePlan),
			})
			require.NoError(t, err)

			err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				PlanMode: ptr.Ref(codersdk.ChatPlanMode("")),
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Empty(t, updated.PlanMode)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})

		t.Run("RejectsInvalidValue", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client := newChatClient(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "invalid plan mode")
			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				PlanMode: ptr.Ref(codersdk.ChatPlanMode("invalid")),
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, "Invalid plan_mode value.", sdkErr.Message)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})
	})

	t.Run("WorkspaceBinding", func(t *testing.T) {
		t.Parallel()

		t.Run("BindValidWorkspace", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			modelConfig := createChatModelConfig(t, client)

			workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: firstUser.OrganizationID,
				OwnerID:        firstUser.UserID,
			}).WithAgent().Do()
			chat := createStoredChat(
				ctx,
				t,
				db,
				firstUser.UserID,
				firstUser.OrganizationID,
				modelConfig.ID,
				"bind workspace",
			)

			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				WorkspaceID: &workspaceBuild.Workspace.ID,
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.NotNil(t, updated.WorkspaceID)
			require.Equal(t, workspaceBuild.Workspace.ID, *updated.WorkspaceID)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})

		t.Run("WorkspaceNotFound", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			modelConfig := createChatModelConfig(t, client)

			chat := createStoredChat(
				ctx,
				t,
				db,
				firstUser.UserID,
				firstUser.OrganizationID,
				modelConfig.ID,
				"missing workspace",
			)
			workspaceID := uuid.New()
			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				WorkspaceID: &workspaceID,
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, "Workspace not found or you do not have access to this resource", sdkErr.Message)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})

		t.Run("RejectsCrossOrgWorkspaceBinding", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			modelConfig := createChatModelConfig(t, client)

			secondOrg := dbgen.Organization(t, db, database.Organization{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{
				OrganizationID: secondOrg.ID,
				UserID:         firstUser.UserID,
			})
			workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: secondOrg.ID,
				OwnerID:        firstUser.UserID,
			}).WithAgent().Do()
			chat := createStoredChat(
				ctx,
				t,
				db,
				firstUser.UserID,
				firstUser.OrganizationID,
				modelConfig.ID,
				"cross org workspace binding",
			)

			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				WorkspaceID: &workspaceBuild.Workspace.ID,
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, "Workspace does not belong to this chat's organization.", sdkErr.Message)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})

		t.Run("ClearWorkspaceBinding", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			mAudit := audit.NewMock()
			client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
				opts.Auditor = mAudit
			})
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			modelConfig := createChatModelConfig(t, client)

			workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: firstUser.OrganizationID,
				OwnerID:        firstUser.UserID,
			}).WithAgent().Do()
			chat := createStoredChat(
				ctx,
				t,
				db,
				firstUser.UserID,
				firstUser.OrganizationID,
				modelConfig.ID,
				"clear workspace binding",
			)

			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				WorkspaceID: &workspaceBuild.Workspace.ID,
			})
			require.NoError(t, err)

			workspaceID := uuid.Nil
			err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				WorkspaceID: &workspaceID,
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Nil(t, updated.WorkspaceID)
			require.Nil(t, updated.BuildID)
			require.Nil(t, updated.AgentID)
			require.True(t, mAudit.Contains(t, database.AuditLog{
				Action:       database.AuditActionWrite,
				ResourceType: database.ResourceTypeChat,
				ResourceID:   chat.ID,
				UserID:       firstUser.UserID,
			}))
		})
	})

	t.Run("Title", func(t *testing.T) {
		t.Parallel()

		t.Run("Rename", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "original title")

			waitChatSettled(ctx, t, client, chat.ID)

			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				Title: ptr.Ref("renamed title"),
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, "renamed title", updated.Title)
		})

		t.Run("TrimsWhitespace", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "before trim")

			waitChatSettled(ctx, t, client, chat.ID)

			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				Title: ptr.Ref("   padded title   "),
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, "padded title", updated.Title)
		})

		t.Run("RejectsEmpty", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "keep original")

			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				Title: ptr.Ref("   "),
			})
			requireSDKError(t, err, http.StatusBadRequest)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, chat.Title, updated.Title)
		})

		t.Run("RejectsTooLong", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "keep original length")

			tooLong := strings.Repeat("a", 201)
			err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				Title: ptr.Ref(tooLong),
			})
			requireSDKError(t, err, http.StatusBadRequest)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, chat.Title, updated.Title)
		})

		t.Run("LengthBoundaries", func(t *testing.T) {
			t.Parallel()

			cases := []struct {
				name     string
				title    string
				expectOK bool
				storedAs string
			}{
				{
					name:     "ExactlyMaxASCII",
					title:    strings.Repeat("a", 200),
					expectOK: true,
					storedAs: strings.Repeat("a", 200),
				},
				{
					name:     "OneOverMaxASCII",
					title:    strings.Repeat("a", 201),
					expectOK: false,
				},
				{
					name:     "ExactlyMaxMultiByte",
					title:    strings.Repeat("é", 200),
					expectOK: true,
					storedAs: strings.Repeat("é", 200),
				},
				{
					name:     "OneOverMaxMultiByte",
					title:    strings.Repeat("é", 201),
					expectOK: false,
				},
				{
					name:     "TrimsDownToMax",
					title:    "   " + strings.Repeat("a", 200) + "   ",
					expectOK: true,
					storedAs: strings.Repeat("a", 200),
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					ctx := testutil.Context(t, testutil.WaitLong)
					client := newChatClient(t)
					firstUser := coderdtest.CreateFirstUser(t, client.Client)
					_ = createChatModelConfig(t, client)

					chat := createChat(ctx, t, client, firstUser.OrganizationID, "boundary baseline")
					waitChatSettled(ctx, t, client, chat.ID)

					err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
						Title: ptr.Ref(tc.title),
					})
					updated := getChat(ctx, t, client, chat.ID)
					if tc.expectOK {
						require.NoError(t, err)
						require.Equal(t, tc.storedAs, updated.Title)
					} else {
						requireSDKError(t, err, http.StatusBadRequest)
						require.Equal(t, chat.Title, updated.Title)
					}
				})
			}
		})

		t.Run("PreservesUpdatedAt", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			clientRaw := coderdtest.New(t, &coderdtest.Options{
				DeploymentValues: chatDeploymentValues(t),
				Database:         db,
				Pubsub:           ps,
			})
			client := codersdk.NewExperimentalClient(clientRaw)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "rename me")
			waitChatSettled(ctx, t, client, chat.ID)

			past := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
			_, err := sqlDB.ExecContext(ctx,
				"UPDATE chats SET updated_at = $1 WHERE id = $2",
				past, chat.ID,
			)
			require.NoError(t, err)

			err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				Title: ptr.Ref("renamed in place"),
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, "renamed in place", updated.Title)
			require.WithinDuration(t, past, updated.UpdatedAt, time.Second,
				"rename bumped updated_at; it should be preserved to keep list ordering stable")
		})

		t.Run("NoOpWhenTitleUnchanged", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			clientRaw := coderdtest.New(t, &coderdtest.Options{
				DeploymentValues: chatDeploymentValues(t),
				Database:         db,
				Pubsub:           ps,
			})
			client := codersdk.NewExperimentalClient(clientRaw)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "steady title")
			waitChatSettled(ctx, t, client, chat.ID)

			past := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
			_, err := sqlDB.ExecContext(ctx,
				"UPDATE chats SET title = $1, updated_at = $2 WHERE id = $3",
				"steady title", past, chat.ID,
			)
			require.NoError(t, err)

			err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
				Title: ptr.Ref("steady title"),
			})
			require.NoError(t, err)

			updated := getChat(ctx, t, client, chat.ID)
			require.Equal(t, "steady title", updated.Title)
			require.WithinDuration(t, past, updated.UpdatedAt, time.Second,
				"no-op rename bumped updated_at; it should have been short-circuited before the write")
		})

		t.Run("PublishesWatchEvent", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			client := newChatClient(t)
			firstUser := coderdtest.CreateFirstUser(t, client.Client)
			_ = createChatModelConfig(t, client)

			chat := createChat(ctx, t, client, firstUser.OrganizationID, "announce me")

			waitChatSettled(ctx, t, client, chat.ID)

			conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
			require.NoError(t, err)
			defer conn.Close(websocket.StatusNormalClosure, "done")

			go func() {
				_ = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
					Title: ptr.Ref("announced name"),
				})
			}()

			var received codersdk.ChatWatchEvent
			for {
				if err := wsjson.Read(ctx, conn, &received); err != nil {
					break
				}
				if received.Kind == codersdk.ChatWatchEventKindTitleChange &&
					received.Chat.ID == chat.ID {
					require.Equal(t, "announced name", received.Chat.Title)
					return
				}
			}
			t.Fatalf("did not observe title_change event for chat %s", chat.ID)
		})
	})
}

func TestArchiveChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMock()
		client := newChatClient(t, func(o *coderdtest.Options) {
			o.Auditor = mAudit
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chatToArchive, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "archive me",
				},
			},
		})
		require.NoError(t, err)

		chatToKeep, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "keep me",
				},
			},
		})
		require.NoError(t, err)

		chatsBeforeArchive, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, chatsBeforeArchive, 2)

		err = client.UpdateChat(ctx, chatToArchive.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// Default (no filter) returns only non-archived chats.
		allChats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.Len(t, allChats, 1)
		require.Equal(t, chatToKeep.ID, allChats[0].ID)

		// archived:false returns only non-archived chats.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)
		require.Len(t, activeChats, 1)
		require.Equal(t, chatToKeep.ID, activeChats[0].ID)
		require.False(t, activeChats[0].Archived)

		// archived:true returns only archived chats.
		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		require.Len(t, archivedChats, 1)
		require.Equal(t, chatToArchive.ID, archivedChats[0].ID)
		require.True(t, archivedChats[0].Archived)

		require.True(t, mAudit.Contains(t, database.AuditLog{
			Action:         database.AuditActionWrite,
			ResourceType:   database.ResourceTypeChat,
			ResourceID:     chatToArchive.ID,
			ResourceTarget: chatToArchive.ID.String()[:8],
			UserID:         firstUser.UserID,
		}))
	})
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		err := client.UpdateChat(ctx, uuid.New(), codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ArchivesChildren", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a parent chat via the API.
		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent chat",
				},
			},
		})
		require.NoError(t, err)

		// Insert child chats directly via the database.
		child1 := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child 1",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		child2 := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child 2",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		// Archive the parent via the API.
		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// archived:false should exclude the entire archived family.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)
		for _, c := range activeChats {
			require.NotEqual(t, parentChat.ID, c.ID, "parent should not appear")
			require.NotEqual(t, child1.ID, c.ID, "child1 should not appear")
			require.NotEqual(t, child2.ID, c.ID, "child2 should not appear")
		}

		// Verify children are archived directly in the DB.
		dbChild1, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child1.ID)
		require.NoError(t, err)
		require.True(t, dbChild1.Archived, "child1 should be archived")

		dbChild2, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child2.ID)
		require.NoError(t, err)
		require.True(t, dbChild2.Archived, "child2 should be archived")

		// archived:true should return the parent with both
		// cascaded children embedded.
		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		var foundParent *codersdk.Chat
		for _, chat := range archivedChats {
			if chat.ID == parentChat.ID {
				foundParent = &chat
				break
			}
		}
		require.NotNil(t, foundParent, "parent should appear in archived list")
		require.True(t, foundParent.Archived, "parent should be archived")
		require.Len(t, foundParent.Children, 2, "both archived children should be embedded under the archived parent")
		childIDs := map[uuid.UUID]bool{}
		for _, child := range foundParent.Children {
			require.True(t, child.Archived, "embedded child should be archived")
			childIDs[child.ID] = true
		}
		require.True(t, childIDs[child1.ID], "child1 should be embedded under archived parent")
		require.True(t, childIDs[child2.ID], "child2 should be embedded under archived parent")
	})

	t.Run("AllowsChildChatArchiveIndividually", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a parent chat via the API.
		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent",
				},
			},
		})
		require.NoError(t, err)

		// Insert a child chat directly via the database.
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		// Individual child archive is permitted and leaves the
		// parent active; the invariant is one-way.
		err = client.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		dbChild, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
		require.NoError(t, err)
		require.True(t, dbChild.Archived, "child should be archived")

		dbParent, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), parentChat.ID)
		require.NoError(t, err)
		require.False(t, dbParent.Archived, "parent should stay active")

		// Archived child is hidden under an active parent.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{Query: "archived:false"})
		require.NoError(t, err)
		var activeParent *codersdk.Chat
		for i := range activeChats {
			if activeChats[i].ID == parentChat.ID {
				activeParent = &activeChats[i]
				break
			}
		}
		require.NotNil(t, activeParent, "parent should appear in active list")
		for _, c := range activeParent.Children {
			require.NotEqual(t, child.ID, c.ID, "archived child must not appear under active parent")
		}

		// Nor does the child surface in the archived list (only
		// roots paginate there).
		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{Query: "archived:true"})
		require.NoError(t, err)
		for _, c := range archivedChats {
			require.NotEqual(t, child.ID, c.ID, "archived child should not surface as a root in archived list")
		}
	})
}

func TestUnarchiveChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "archive then unarchive me",
				},
			},
		})
		require.NoError(t, err)

		// Archive the chat first.
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// Verify it's archived.
		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		require.Len(t, archivedChats, 1)
		require.True(t, archivedChats[0].Archived)
		// Unarchive the chat.
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		require.NoError(t, err)

		// Verify it's no longer archived.
		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)
		require.Len(t, activeChats, 1)
		require.Equal(t, chat.ID, activeChats[0].ID)
		require.False(t, activeChats[0].Archived)

		// No archived chats remain.
		archivedChats, err = client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		require.Empty(t, archivedChats)
	})

	t.Run("UnarchivesChildren", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent chat",
				},
			},
		})
		require.NoError(t, err)

		child1 := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child 1",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		child2 := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child 2",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		require.NoError(t, err)

		activeChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:false",
		})
		require.NoError(t, err)

		// Children no longer appear as top-level entries.
		// They are embedded inside the parent's Children field.
		var foundParent *codersdk.Chat
		for _, chat := range activeChats {
			require.NotEqual(t, child1.ID, chat.ID, "child1 should not appear at top level")
			require.NotEqual(t, child2.ID, chat.ID, "child2 should not appear at top level")
			if chat.ID == parentChat.ID {
				foundParent = &chat
			}
		}
		require.NotNil(t, foundParent, "parent should be listed as active")
		require.False(t, foundParent.Archived)

		// Verify children are embedded and unarchived.
		require.Len(t, foundParent.Children, 2)
		childIDs := map[uuid.UUID]bool{}
		for _, child := range foundParent.Children {
			require.False(t, child.Archived)
			childIDs[child.ID] = true
		}
		require.True(t, childIDs[child1.ID], "child1 should be embedded")
		require.True(t, childIDs[child2.ID], "child2 should be embedded")

		archivedChats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{
			Query: "archived:true",
		})
		require.NoError(t, err)
		for _, chat := range archivedChats {
			require.NotEqual(t, parentChat.ID, chat.ID, "parent should not remain archived")
		}

		dbParent, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), parentChat.ID)
		require.NoError(t, err)
		require.False(t, dbParent.Archived, "parent should be unarchived")

		dbChild1, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child1.ID)
		require.NoError(t, err)
		require.False(t, dbChild1.Archived, "child1 should be unarchived")

		dbChild2, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child2.ID)
		require.NoError(t, err)
		require.False(t, dbChild2.Archived, "child2 should be unarchived")
	})

	t.Run("NotArchived", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "not archived",
				},
			},
		})
		require.NoError(t, err)

		// Trying to unarchive a non-archived chat should fail.
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("RejectsChildChatWhenParentArchived", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a parent chat via the API.
		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent",
				},
			},
		})
		require.NoError(t, err)

		// Insert a child directly via the database, then archive the
		// parent so the whole family is archived (cascade).
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		err = client.UpdateChat(ctx, parentChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// Unarchiving the child while the parent stays archived
		// must be rejected. Otherwise the child becomes a ghost
		// (active list excludes the parent, archived list's child
		// query filters archived=true so the now-unarchived child
		// is also excluded).
		err = client.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		requireSDKError(t, err, http.StatusBadRequest)

		dbChild, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
		require.NoError(t, err)
		require.True(t, dbChild.Archived, "child should still be archived")
	})

	t.Run("AllowsChildChatWhenParentNotArchived", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		parentChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "parent",
				},
			},
		})
		require.NoError(t, err)

		// Simulate legacy lone-archived child (from before the
		// child-archive gate existed) by inserting it directly
		// with archived=true while the parent is not archived.
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "legacy child",
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		_, err = db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
		require.NoError(t, err)

		// Unarchiving the child is permitted because the parent is
		// already active; this is the recovery path for legacy
		// data.
		err = client.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		require.NoError(t, err)

		dbChild, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
		require.NoError(t, err)
		require.False(t, dbChild.Archived, "child should be unarchived")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		err := client.UpdateChat(ctx, uuid.New(), codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestChatPinOrder(t *testing.T) {
	t.Parallel()

	createChat := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, title string) codersdk.Chat {
		t.Helper()

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: orgID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: title,
				},
			},
		})
		require.NoError(t, err)
		return chat
	}

	getChat := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID) codersdk.Chat {
		t.Helper()

		chat, err := client.GetChat(ctx, chatID)
		require.NoError(t, err)
		return chat
	}

	t.Run("PinReorderAndUnpin", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		first := createChat(ctx, t, client, firstUser.OrganizationID, "first pinned chat")
		second := createChat(ctx, t, client, firstUser.OrganizationID, "second pinned chat")
		third := createChat(ctx, t, client, firstUser.OrganizationID, "third pinned chat")

		err := client.UpdateChat(ctx, first.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
		require.NoError(t, err)
		err = client.UpdateChat(ctx, second.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
		require.NoError(t, err)
		err = client.UpdateChat(ctx, third.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
		require.NoError(t, err)

		first = getChat(ctx, t, client, first.ID)
		second = getChat(ctx, t, client, second.ID)
		third = getChat(ctx, t, client, third.ID)
		require.EqualValues(t, 1, first.PinOrder)
		require.EqualValues(t, 2, second.PinOrder)
		require.EqualValues(t, 3, third.PinOrder)

		err = client.UpdateChat(ctx, third.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
		require.NoError(t, err)

		first = getChat(ctx, t, client, first.ID)
		second = getChat(ctx, t, client, second.ID)
		third = getChat(ctx, t, client, third.ID)
		require.EqualValues(t, 2, first.PinOrder)
		require.EqualValues(t, 3, second.PinOrder)
		require.EqualValues(t, 1, third.PinOrder)

		err = client.UpdateChat(ctx, first.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(0))})
		require.NoError(t, err)

		first = getChat(ctx, t, client, first.ID)
		second = getChat(ctx, t, client, second.ID)
		third = getChat(ctx, t, client, third.ID)
		require.Zero(t, first.PinOrder)
		require.EqualValues(t, 2, second.PinOrder)
		require.EqualValues(t, 1, third.PinOrder)
	})

	t.Run("ArchiveClearsPinOrder", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		first := createChat(ctx, t, client, firstUser.OrganizationID, "pinned then archived")
		second := createChat(ctx, t, client, firstUser.OrganizationID, "stays pinned")

		// Pin both.
		err := client.UpdateChat(ctx, first.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
		require.NoError(t, err)
		err = client.UpdateChat(ctx, second.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})
		require.NoError(t, err)

		// Archive the first — pin_order should be cleared.
		err = client.UpdateChat(ctx, first.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		first = getChat(ctx, t, client, first.ID)
		second = getChat(ctx, t, client, second.ID)
		require.Zero(t, first.PinOrder, "archived chat should have pin_order 0")
		require.True(t, first.Archived)
		// The remaining pin keeps its original position. The next
		// pin/unpin/reorder operation compacts via ROW_NUMBER().
		require.EqualValues(t, 2, second.PinOrder, "remaining pin keeps original position")
	})

	t.Run("RejectsNegative", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat := createChat(ctx, t, client, firstUser.OrganizationID, "negative pin order")
		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(-1))})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Pin order must be non-negative.", sdkErr.Message)

		chat = getChat(ctx, t, client, chat.ID)
		require.Zero(t, chat.PinOrder)
	})

	t.Run("RejectsChildChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		parentChat := createChat(ctx, t, client, firstUser.OrganizationID, "parent chat")

		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "child chat",
			Status:            database.ChatStatusCompleted,
			ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		})

		err := client.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{PinOrder: ptr.Ref(int32(1))})

		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot pin a child chat.", sdkErr.Message)

		result := getChat(ctx, t, client, child.ID)
		require.Zero(t, result.PinOrder)
	})
}

func TestPostChatMessages(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message for post route test",
				},
			},
		})
		require.NoError(t, err)

		hasTextPart := func(parts []codersdk.ChatMessagePart, want string) bool {
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text == want {
					return true
				}
			}
			return false
		}

		messageText := "post message route success " + uuid.NewString()
		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: messageText,
				},
			},
		})
		require.NoError(t, err)

		if created.Queued {
			require.Nil(t, created.Message)
			require.NotNil(t, created.QueuedMessage)
			require.Equal(t, chat.ID, created.QueuedMessage.ChatID)
			require.NotZero(t, created.QueuedMessage.ID)
			require.True(t, hasTextPart(created.QueuedMessage.Content, messageText))

			require.Eventually(t, func() bool {
				messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
				if getErr != nil {
					return false
				}

				for _, queued := range messagesResult.QueuedMessages {
					if queued.ID == created.QueuedMessage.ID &&
						queued.ChatID == chat.ID &&
						hasTextPart(queued.Content, messageText) {
						return true
					}
				}
				for _, message := range messagesResult.Messages {
					if message.Role == codersdk.ChatMessageRoleUser && hasTextPart(message.Content, messageText) {
						return true
					}
				}
				return false
			}, testutil.WaitLong, testutil.IntervalFast)
		} else {
			require.Nil(t, created.QueuedMessage)
			require.NotNil(t, created.Message)
			require.Equal(t, chat.ID, created.Message.ChatID)
			require.Equal(t, codersdk.ChatMessageRoleUser, created.Message.Role)
			require.NotZero(t, created.Message.ID)
			require.True(t, hasTextPart(created.Message.Content, messageText))

			require.Eventually(t, func() bool {
				messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
				if getErr != nil {
					return false
				}
				for _, message := range messagesResult.Messages {
					if message.ID == created.Message.ID &&
						message.Role == codersdk.ChatMessageRoleUser &&
						hasTextPart(message.Content, messageText) {
						return true
					}
				}
				return false
			}, testutil.WaitLong, testutil.IntervalFast)
		}
	})

	t.Run("MemberWithoutAgentsAccess", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a member without agents-access and insert a
		// chat owned by them via system context. Without
		// agents-access the member has no ResourceChat
		// permissions, so the ChatParam middleware returns 404
		// before the handler can check agents-access.
		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "member chat",
		})

		_, err := memberClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "this should fail",
				},
			},
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("EmptyText", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message for validation test",
				},
			},
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "   ",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].text cannot be empty.", sdkErr.Detail)
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "initial message for usage-limit test",
			}},
		})
		require.NoError(t, err)

		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)
		insertAssistantCostMessage(t, db, chat.ID, modelConfig.ID, 100)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "over limit",
			}},
		})
		requireChatUsageLimitExceededError(t, err, 100, 100, wantResetsAt)
	})

	t.Run("ChatNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		_, err := client.CreateChatMessage(ctx, uuid.New(), codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ArchivedChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
		})
		require.NoError(t, err)

		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "should fail",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "archived")
	})
}

func waitForChatWatchStatusChangeEvent(
	ctx context.Context,
	t *testing.T,
	conn *websocket.Conn,
	chatID uuid.UUID,
) codersdk.ChatWatchEvent {
	t.Helper()

	for {
		var payload codersdk.ChatWatchEvent
		err := wsjson.Read(ctx, conn, &payload)
		require.NoError(t, err)
		if payload.Kind == codersdk.ChatWatchEventKindStatusChange && payload.Chat.ID == chatID {
			return payload
		}
	}
}

func TestSendMessageWithModelOverrideUpdatesLastModelConfigID(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	user := coderdtest.CreateFirstUser(t, client.Client)
	modelConfigA := createChatModelConfig(t, client)
	modelConfigB := createAdditionalChatModelConfig(t, client, "openai", "gpt-4o-mini-override-"+uuid.NewString())

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: modelConfigA.ID,
		Title:             "mid-chat model switch direct send",
	})

	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "switch to model b",
		}},
		ModelConfigID: ptr.Ref(modelConfigB.ID),
	})
	require.NoError(t, err)
	require.False(t, resp.Queued)
	require.NotNil(t, resp.Message)
	require.NotNil(t, resp.Message.ModelConfigID)
	require.Equal(t, modelConfigB.ID, *resp.Message.ModelConfigID)

	storedChat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Equal(t, modelConfigB.ID, storedChat.LastModelConfigID)

	messages, err := db.GetChatMessagesByChatID(dbauthz.AsSystemRestricted(ctx), database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.True(t, messages[0].ModelConfigID.Valid)
	require.Equal(t, modelConfigB.ID, messages[0].ModelConfigID.UUID)
}

func TestSendMessageQueuesEffectiveModelConfigID(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	user := coderdtest.CreateFirstUser(t, client.Client)
	modelConfigA := createChatModelConfig(t, client)
	modelConfigB := createAdditionalChatModelConfig(t, client, "openai", "gpt-4o-mini-queued-"+uuid.NewString())

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: modelConfigA.ID,
		Title:             "mid-chat model switch queued send",
	})

	_, err := db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "queue this with model b",
		}},
		ModelConfigID: ptr.Ref(modelConfigB.ID),
		BusyBehavior:  codersdk.ChatBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, resp.Queued)
	require.NotNil(t, resp.QueuedMessage)
	require.NotNil(t, resp.QueuedMessage.ModelConfigID)
	require.Equal(t, modelConfigB.ID, *resp.QueuedMessage.ModelConfigID)

	queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Len(t, queuedMessages, 1)
	require.True(t, queuedMessages[0].ModelConfigID.Valid)
	require.Equal(t, modelConfigB.ID, queuedMessages[0].ModelConfigID.UUID)

	storedChat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Equal(t, modelConfigA.ID, storedChat.LastModelConfigID)
}

func TestQueuedMessageWithoutOverrideCapturesEnqueueTimeModel(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	user := coderdtest.CreateFirstUser(t, client.Client)
	modelConfigA := createChatModelConfig(t, client)
	modelConfigB := createAdditionalChatModelConfig(t, client, "openai", "gpt-4o-mini-later-"+uuid.NewString())

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: modelConfigA.ID,
		Title:             "capture queued enqueue-time model",
	})

	_, err := db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "queue with stored model",
		}},
		BusyBehavior: codersdk.ChatBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, resp.Queued)
	require.NotNil(t, resp.QueuedMessage)
	require.NotNil(t, resp.QueuedMessage.ModelConfigID)
	require.Equal(t, modelConfigA.ID, *resp.QueuedMessage.ModelConfigID)

	_, err = db.UpdateChatLastModelConfigByID(dbauthz.AsSystemRestricted(ctx), database.UpdateChatLastModelConfigByIDParams{
		ID:                chat.ID,
		LastModelConfigID: modelConfigB.ID,
	})
	require.NoError(t, err)

	queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.NoError(t, err)
	require.Len(t, queuedMessages, 1)
	require.True(t, queuedMessages[0].ModelConfigID.Valid)
	require.Equal(t, modelConfigA.ID, queuedMessages[0].ModelConfigID.UUID)
}

func TestSubsequentSendWithoutOverrideUsesPersistedModel(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	user := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelConfig(t, client)
	modelConfigB := createAdditionalChatModelConfig(t, client, "openai", "gpt-4o-mini-persisted-"+uuid.NewString())

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    user.OrganizationID,
		OwnerID:           user.UserID,
		LastModelConfigID: modelConfigB.ID,
		Title:             "subsequent send uses persisted model",
	})

	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "reuse the persisted model",
		}},
	})
	require.NoError(t, err)
	require.False(t, resp.Queued)
	require.NotNil(t, resp.Message)
	require.NotNil(t, resp.Message.ModelConfigID)
	require.Equal(t, modelConfigB.ID, *resp.Message.ModelConfigID)

	messages, err := db.GetChatMessagesByChatID(dbauthz.AsSystemRestricted(ctx), database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.True(t, messages[0].ModelConfigID.Valid)
	require.Equal(t, modelConfigB.ID, messages[0].ModelConfigID.UUID)
}

func TestWatchChatsStatusChangeCarriesUpdatedLastModelConfigID(t *testing.T) {
	t.Parallel()

	t.Run("DirectSend", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfigA := createChatModelConfig(t, client)
		modelConfigB := createAdditionalChatModelConfig(t, client, "openai", "gpt-4o-mini-watch-direct-"+uuid.NewString())

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfigA.ID,
			Title:             "watch direct model switch",
		})

		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "watch the direct send override",
			}},
			ModelConfigID: ptr.Ref(modelConfigB.ID),
		})
		require.NoError(t, err)

		event := waitForChatWatchStatusChangeEvent(ctx, t, conn, chat.ID)
		require.Equal(t, modelConfigB.ID, event.Chat.LastModelConfigID)
	})

	t.Run("QueuedPromotion", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfigA := createChatModelConfig(t, client)
		modelConfigB := createAdditionalChatModelConfig(t, client, "openai", "gpt-4o-mini-watch-promote-"+uuid.NewString())

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfigA.ID,
			Title:             "watch queued promotion model switch",
		})

		_, err := db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusRunning,
			WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
			StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
			LastError:   sql.NullString{},
		})
		require.NoError(t, err)

		queuedResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "queue the promoted model override",
			}},
			ModelConfigID: ptr.Ref(modelConfigB.ID),
			BusyBehavior:  codersdk.ChatBusyBehaviorQueue,
		})
		require.NoError(t, err)
		require.True(t, queuedResp.Queued)
		require.NotNil(t, queuedResp.QueuedMessage)

		_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusWaiting,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		require.NoError(t, err)

		conn, err := client.Dial(ctx, "/api/experimental/chats/watch", nil)
		require.NoError(t, err)
		defer conn.Close(websocket.StatusNormalClosure, "done")

		promoteRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedResp.QueuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusOK, promoteRes.StatusCode)

		event := waitForChatWatchStatusChangeEvent(ctx, t, conn, chat.ID)
		require.Equal(t, modelConfigB.ID, event.Chat.LastModelConfigID)
	})
}

func TestChatMessageWithFileReferences(t *testing.T) {
	t.Parallel()

	// createChat is a helper that creates a chat so we can post messages to it.
	createChatForTest := func(t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID) codersdk.Chat {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: orgID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "initial message",
			}},
		})
		require.NoError(t, err)
		return chat
	}

	t.Run("FileReferenceOnly", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client, firstUser.OrganizationID)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "main.go",
				StartLine: 10,
				EndLine:   15,
				Content:   "func broken() {}",
			}},
		})
		require.NoError(t, err)

		// File-reference parts are stored as structured parts.
		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "main.go" &&
				part.StartLine == 10 &&
				part.EndLine == 15 &&
				part.Content == "func broken() {}"
		}

		var found bool
		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, message := range messagesResult.Messages {
				if message.Role != codersdk.ChatMessageRoleUser {
					continue
				}
				for _, part := range message.Content {
					if checkFileRef(part) {
						found = true
						return true
					}
				}
			}
			// The message may have been queued.
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							found = true
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
		require.True(t, found, "expected to find file-reference part in stored message")
	})

	t.Run("FileReferenceSingleLine", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client, firstUser.OrganizationID)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "lib/utils.ts",
				StartLine: 42,
				EndLine:   42,
				Content:   "const x = 1;",
			}},
		})
		require.NoError(t, err)

		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "lib/utils.ts" &&
				part.StartLine == 42 &&
				part.EndLine == 42 &&
				part.Content == "const x = 1;"
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, msg := range messagesResult.Messages {
				for _, part := range msg.Content {
					if checkFileRef(part) {
						return true
					}
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("FileReferenceWithoutContent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client, firstUser.OrganizationID)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "README.md",
				StartLine: 1,
				EndLine:   1,
				// No code content — just a file reference.
			}},
		})
		require.NoError(t, err)

		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "README.md" &&
				part.StartLine == 1 &&
				part.EndLine == 1 &&
				part.Content == ""
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, msg := range messagesResult.Messages {
				for _, part := range msg.Content {
					if checkFileRef(part) {
						return true
					}
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("FileReferenceWithCode", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client, firstUser.OrganizationID)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "server.go",
				StartLine: 5,
				EndLine:   8,
				Content:   "func main() {\n\tfmt.Println()\n}",
			}},
		})
		require.NoError(t, err)

		checkFileRef := func(part codersdk.ChatMessagePart) bool {
			return part.Type == codersdk.ChatMessagePartTypeFileReference &&
				part.FileName == "server.go" &&
				part.StartLine == 5 &&
				part.EndLine == 8 &&
				part.Content == "func main() {\n\tfmt.Println()\n}"
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}
			for _, msg := range messagesResult.Messages {
				for _, part := range msg.Content {
					if checkFileRef(part) {
						return true
					}
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					for _, part := range queued.Content {
						if checkFileRef(part) {
							return true
						}
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("InterleavedTextAndFileReferences", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client, firstUser.OrganizationID)

		created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Please review these two issues:",
				},
				{
					Type:      codersdk.ChatInputPartTypeFileReference,
					FileName:  "a.go",
					StartLine: 1,
					EndLine:   3,
					Content:   "line1\nline2\nline3",
				},
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "first issue",
				},
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "and also:",
				},
				{
					Type:      codersdk.ChatInputPartTypeFileReference,
					FileName:  "b.go",
					StartLine: 10,
					EndLine:   10,
					Content:   "return nil",
				},
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "second issue",
				},
			},
		})
		require.NoError(t, err)

		// Verify that all six parts are stored in order with
		// correct types: text, file-reference, text, text,
		// file-reference, text.
		type wantPart struct {
			typ       codersdk.ChatMessagePartType
			text      string
			fileName  string
			startLine int
			endLine   int
			content   string
		}
		want := []wantPart{
			{typ: codersdk.ChatMessagePartTypeText, text: "Please review these two issues:"},
			{typ: codersdk.ChatMessagePartTypeFileReference, fileName: "a.go", startLine: 1, endLine: 3, content: "line1\nline2\nline3"},
			{typ: codersdk.ChatMessagePartTypeText, text: "first issue"},
			{typ: codersdk.ChatMessagePartTypeText, text: "and also:"},
			{typ: codersdk.ChatMessagePartTypeFileReference, fileName: "b.go", startLine: 10, endLine: 10, content: "return nil"},
			{typ: codersdk.ChatMessagePartTypeText, text: "second issue"},
		}

		require.Eventually(t, func() bool {
			messagesResult, getErr := client.GetChatMessages(ctx, chat.ID, nil)
			if getErr != nil {
				return false
			}

			checkParts := func(parts []codersdk.ChatMessagePart) bool {
				if len(parts) != len(want) {
					return false
				}
				for i, w := range want {
					p := parts[i]
					if p.Type != w.typ {
						return false
					}
					switch w.typ {
					case codersdk.ChatMessagePartTypeText:
						if p.Text != w.text {
							return false
						}
					case codersdk.ChatMessagePartTypeFileReference:
						if p.FileName != w.fileName ||
							p.StartLine != w.startLine ||
							p.EndLine != w.endLine ||
							p.Content != w.content {
							return false
						}
					}
				}
				return true
			}

			for _, msg := range messagesResult.Messages {
				if msg.Role == codersdk.ChatMessageRoleUser && checkParts(msg.Content) {
					return true
				}
			}
			if created.Queued && created.QueuedMessage != nil {
				for _, queued := range messagesResult.QueuedMessages {
					if checkParts(queued.Content) {
						return true
					}
				}
			}
			return false
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("EmptyFileName", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)
		chat := createChatForTest(t, client, firstUser.OrganizationID)

		_, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "",
				StartLine: 1,
				EndLine:   1,
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Equal(t, "content[0].file_name cannot be empty for file-reference.", sdkErr.Detail)
	})

	t.Run("CreateChatWithFileReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// File references should also work in the initial CreateChat call.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type:      codersdk.ChatInputPartTypeFileReference,
				FileName:  "bug.py",
				StartLine: 7,
				EndLine:   7,
				Content:   "x = None",
			}},
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, chat.ID)

		// Title is derived from the text parts. For file-references
		// the formatted text becomes the title source.
		require.NotEmpty(t, chat.Title)
	})
}

func TestChatMessageWithFiles(t *testing.T) {
	t.Parallel()

	t.Run("FileOnly", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with text first.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message",
				},
			},
		})
		require.NoError(t, err)

		// Send a file-only message (no text).
		resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		// Verify the message was accepted.
		if resp.Queued {
			require.NotNil(t, resp.QueuedMessage)
		} else {
			require.NotNil(t, resp.Message)
			require.Equal(t, codersdk.ChatMessageRoleUser, resp.Message.Role)
		}
	})

	t.Run("TextAndFile", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with text first.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message",
				},
			},
		})
		require.NoError(t, err)

		// Send a message with both text and file.
		resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "here is an image",
				},
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		if resp.Queued {
			require.NotNil(t, resp.QueuedMessage)
		} else {
			require.NotNil(t, resp.Message)
			require.Equal(t, codersdk.ChatMessageRoleUser, resp.Message.Role)
		}

		// Verify file parts omit inline data in the API response.
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, msg := range messagesResult.Messages {
			for _, part := range msg.Content {
				if part.Type == codersdk.ChatMessagePartTypeFile {
					require.True(t, part.FileID.Valid, "file part should have a valid file_id")
					require.Equal(t, uploadResp.ID, part.FileID.UUID)
					require.Nil(t, part.Data, "file data should not be sent when file_id is present")
				}
			}
		}
	})

	t.Run("FileOnlyOnCreate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a new chat with only a file part.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		// With no text, chatTitleFromMessage("") returns "New Chat".
		require.Equal(t, "New Chat", chat.Title)
		require.Len(t, chat.Files, 1)
		f := chat.Files[0]
		require.Equal(t, uploadResp.ID, f.ID)
		require.Equal(t, firstUser.UserID, f.OwnerID)
		require.NotEqual(t, uuid.Nil, f.OrganizationID)
		require.Equal(t, "image/png", f.MimeType)
		require.Equal(t, "test.png", f.Name)
		require.NotZero(t, f.CreatedAt)
	})

	t.Run("InvalidFileID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create a chat with text first.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "initial message",
				},
			},
		})
		require.NoError(t, err)

		// Send a message with a non-existent file ID.
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uuid.New(),
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid input part.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "does not exist")
	})

	t.Run("FilesLinkedOnSend", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create a text-only chat (no files initially).
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "no files yet"},
			},
		})
		require.NoError(t, err)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "linked.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Send a message with the file.
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "here is a file"},
				{Type: codersdk.ChatInputPartTypeFile, FileID: uploadResp.ID},
			},
		})
		require.NoError(t, err)

		// GET the chat — file should be linked.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, 1)
		require.Equal(t, uploadResp.ID, chatResult.Files[0].ID)
		require.Equal(t, "linked.png", chatResult.Files[0].Name)
	})

	t.Run("DedupFileIDs", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "dedup.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with a file.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "first mention"}, {Type: codersdk.ChatInputPartTypeFile, FileID: uploadResp.ID},
			},
		})
		require.NoError(t, err)

		// Send another message with the SAME file.
		msgResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "same file again"},
				{Type: codersdk.ChatInputPartTypeFile, FileID: uploadResp.ID},
			},
		})
		require.NoError(t, err)
		require.Empty(t, msgResp.Warnings, "dedup below cap should not produce warnings")

		// GET — should have exactly 1 file (deduped by SQL DISTINCT).
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, 1, "duplicate file IDs should be deduped")
		require.Equal(t, uploadResp.ID, chatResult.Files[0].ID)
	})

	t.Run("FileCapExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)

		// Upload MaxChatFileIDs files.
		fileIDs := make([]uuid.UUID, 0, codersdk.MaxChatFileIDs)
		for i := range codersdk.MaxChatFileIDs {
			resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", fmt.Sprintf("file%d.png", i), bytes.NewReader(pngData))
			require.NoError(t, err)
			fileIDs = append(fileIDs, resp.ID)
		}

		// Create a chat using all MaxChatFileIDs files.
		parts := []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "max files"},
		}
		for _, fid := range fileIDs {
			parts = append(parts, codersdk.ChatInputPart{Type: codersdk.ChatInputPartTypeFile, FileID: fid})
		}
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{OrganizationID: firstUser.OrganizationID, Content: parts})
		require.NoError(t, err)
		require.Empty(t, chat.Warnings, "creating a chat at exactly the cap should not warn")
		require.Len(t, chat.Files, codersdk.MaxChatFileIDs, "all files should be linked on creation")

		// Upload one more file.
		extraResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "one-too-many.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Sending a message with the extra file should succeed
		// (message goes through) but the file should NOT be linked
		// (cap enforced in SQL). The response includes a warning.
		msgResp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "one too many"},
				{Type: codersdk.ChatInputPartTypeFile, FileID: extraResp.ID},
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, msgResp.Warnings, "response should warn about unlinked files")
		require.Contains(t, msgResp.Warnings[0], "file linking skipped")

		// The extra file should NOT appear in the chat's files.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, codersdk.MaxChatFileIDs,
			"file count should not exceed the cap")

		// Sending a message referencing an already-linked file
		// should succeed with no warnings (dedup, no array growth).
		msgResp2, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "re-reference existing"},
				{Type: codersdk.ChatInputPartTypeFile, FileID: fileIDs[0]},
			},
		})
		require.NoError(t, err)
		require.Empty(t, msgResp2.Warnings, "re-referencing an existing file should not warn")
	})

	t.Run("FileCapOnCreate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)

		// Upload MaxChatFileIDs + 1 files.
		fileIDs := make([]uuid.UUID, 0, codersdk.MaxChatFileIDs+1)
		for i := range codersdk.MaxChatFileIDs + 1 {
			resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", fmt.Sprintf("create%d.png", i), bytes.NewReader(pngData))
			require.NoError(t, err)
			fileIDs = append(fileIDs, resp.ID)
		}

		// Create a chat with all files (one over the cap).
		parts := []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "over cap on create"},
		}
		for _, fid := range fileIDs {
			parts = append(parts, codersdk.ChatInputPart{Type: codersdk.ChatInputPartTypeFile, FileID: fid})
		}
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{OrganizationID: firstUser.OrganizationID, Content: parts})
		require.NoError(t, err, "chat creation should succeed even when cap is exceeded")
		require.NotEmpty(t, chat.Warnings, "response should warn about unlinked files")
		require.Contains(t, chat.Warnings[0], "file linking skipped")

		// Only MaxChatFileIDs files should actually be linked.
		// With SQL-level batch rejection, ALL files are rejected
		// when the result would exceed the cap. Since we're
		// sending MaxChatFileIDs+1 files, the deduped count is
		// 21 > 20, so 0 rows are affected and all files are
		// unlinked.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, chatResult.Files, "no files should be linked when batch exceeds cap")
	})
}

func TestPatchChatMessage(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello before edit",
				},
			},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		edited, err := client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello after edit",
				},
			},
		})
		require.NoError(t, err)
		// The edited message is soft-deleted and a new one is inserted,
		// so the returned ID will differ from the original.
		require.NotEqual(t, userMessageID, edited.Message.ID)
		require.Equal(t, codersdk.ChatMessageRoleUser, edited.Message.Role)

		foundEditedText := false
		for _, part := range edited.Message.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "hello after edit" {
				foundEditedText = true
			}
		}
		require.True(t, foundEditedText)

		messagesResult, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		foundEditedInChat := false
		foundOriginalInChat := false
		for _, message := range messagesResult.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type != codersdk.ChatMessagePartTypeText {
					continue
				}
				if part.Text == "hello after edit" {
					foundEditedInChat = true
				}
				if part.Text == "hello before edit" {
					foundOriginalInChat = true
				}
			}
		}
		require.True(t, foundEditedInChat)
		require.False(t, foundOriginalInChat)
	})

	t.Run("PreservesFileID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Create a chat with a text + file part.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "before edit with file",
				},
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)

		// Find the user message ID.
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		// Edit the message: new text, same file_id.
		edited, err := client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "after edit with file",
				},
				{
					Type:   codersdk.ChatInputPartTypeFile,
					FileID: uploadResp.ID,
				},
			},
		})
		require.NoError(t, err)
		// The edited message is soft-deleted and a new one is inserted,
		// so the returned ID will differ from the original.
		require.NotEqual(t, userMessageID, edited.Message.ID)

		// Assert the edit response preserves the file_id.
		var foundText, foundFile bool
		for _, part := range edited.Message.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "after edit with file" {
				foundText = true
			}
			if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid && part.FileID.UUID == uploadResp.ID {
				foundFile = true
				require.Nil(t, part.Data, "file data should not be sent when file_id is present")
			}
		}
		require.True(t, foundText, "edited message should contain updated text")
		require.True(t, foundFile, "edited message should preserve file_id")

		// GET the chat messages and verify the file_id persists.
		messagesResult, err = client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var foundTextInChat, foundFileInChat bool
		for _, message := range messagesResult.Messages {
			if message.Role != codersdk.ChatMessageRoleUser {
				continue
			}
			for _, part := range message.Content {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text == "after edit with file" {
					foundTextInChat = true
				}
				if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid && part.FileID.UUID == uploadResp.ID {
					foundFileInChat = true
					require.Nil(t, part.Data, "file data should not be sent when file_id is present")
				}
			}
		}
		require.True(t, foundTextInChat, "chat should contain edited text")
		require.True(t, foundFileInChat, "chat should preserve file_id after edit")
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello before edit",
			}},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)
		insertAssistantCostMessage(t, db, chat.ID, modelConfig.ID, 100)

		_, err = client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "edited over limit",
			}},
		})
		requireChatUsageLimitExceededError(t, err, 100, 100, wantResetsAt)
	})

	t.Run("MessageNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		require.NoError(t, err)

		_, err = client.EditChatMessage(ctx, chat.ID, 999999, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "edited",
				},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Chat message not found.", sdkErr.Message)
	})

	t.Run("InvalidMessageID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "hello",
				},
			},
		})
		require.NoError(t, err)

		res, err := client.Request(
			ctx,
			http.MethodPatch,
			fmt.Sprintf("/api/experimental/chats/%s/messages/not-an-int", chat.ID),
			codersdk.EditChatMessageRequest{
				Content: []codersdk.ChatInputPart{
					{
						Type: codersdk.ChatInputPartTypeText,
						Text: "ignored",
					},
				},
			},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat message ID.", sdkErr.Message)
	})

	t.Run("FilesLinkedOnEdit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create a text-only chat.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "before file edit"},
			},
		})
		require.NoError(t, err)

		// Upload a file.
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploadResp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "edit-linked.png", bytes.NewReader(pngData))
		require.NoError(t, err)

		// Find the user message ID.
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var userMessageID int64
		for _, msg := range messagesResult.Messages {
			if msg.Role == codersdk.ChatMessageRoleUser {
				userMessageID = msg.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		// Edit the message to include the file.
		_, err = client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "after file edit"},
				{Type: codersdk.ChatInputPartTypeFile, FileID: uploadResp.ID},
			},
		})
		require.NoError(t, err)

		// GET the chat — file should be linked.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, 1)
		f := chatResult.Files[0]
		require.Equal(t, uploadResp.ID, f.ID)
		require.Equal(t, "edit-linked.png", f.Name)
		require.Equal(t, "image/png", f.MimeType)
	})

	t.Run("CapExceededOnEdit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		// Create a chat with MaxChatFileIDs files already linked.
		parts := []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "fill to cap"},
		}
		pngData := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		for i := range codersdk.MaxChatFileIDs {
			up, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", fmt.Sprintf("cap-%d.png", i), bytes.NewReader(pngData))
			require.NoError(t, err)
			parts = append(parts, codersdk.ChatInputPart{Type: codersdk.ChatInputPartTypeFile, FileID: up.ID})
		}
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{OrganizationID: firstUser.OrganizationID, Content: parts})
		require.NoError(t, err)
		require.Empty(t, chat.Warnings, "all files should link on create")

		// Find the user message.
		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var userMessageID int64
		for _, msg := range messagesResult.Messages {
			if msg.Role == codersdk.ChatMessageRoleUser {
				userMessageID = msg.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		// Upload one more file and try to link via edit.
		extra, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "one-too-many.png", bytes.NewReader(pngData))
		require.NoError(t, err)
		edited, err := client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "edit with extra file"},
				{Type: codersdk.ChatInputPartTypeFile, FileID: extra.ID},
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, edited.Warnings, "edit should surface cap warning")
		require.Contains(t, edited.Warnings[0], "file linking skipped")

		// Verify the cap is still enforced.
		chatResult, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, chatResult.Files, codersdk.MaxChatFileIDs,
			"file count should not exceed the cap")
	})

	t.Run("ArchivedChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello before edit",
			}},
		})
		require.NoError(t, err)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var userMessageID int64
		for _, message := range messagesResult.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessageID = message.ID
				break
			}
		}
		require.NotZero(t, userMessageID)

		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: ptr.Ref(true),
		})
		require.NoError(t, err)

		_, err = client.EditChatMessage(ctx, chat.ID, userMessageID, codersdk.EditChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "should fail",
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "archived")
	})
}

func TestStreamChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		const initialMessage = "stream chat route initial message"
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: initialMessage,
				},
			},
		})
		require.NoError(t, err)

		events, closer, err := client.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer closer.Close()

		hasTextPart := func(parts []codersdk.ChatMessagePart, want string) bool {
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text == want {
					return true
				}
			}
			return false
		}

		foundInitialUserMessage := false
		for !foundInitialUserMessage {
			select {
			case <-ctx.Done():
				require.FailNow(t, "timed out waiting for expected stream chat event")
			case event, ok := <-events:
				require.True(t, ok, "stream closed before expected event")
				require.Equal(t, chat.ID, event.ChatID)
				require.NotEqual(t, codersdk.ChatStreamEventTypeError, event.Type)

				if event.Type == codersdk.ChatStreamEventTypeMessage &&
					event.Message != nil &&
					event.Message.Role == codersdk.ChatMessageRoleUser &&
					hasTextPart(event.Message.Content, initialMessage) {
					foundInitialUserMessage = true
				}
			}
		}
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		unauthenticatedClient := codersdk.New(client.URL)
		res, err := unauthenticatedClient.Request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/stream", uuid.New()),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func TestInterruptChat(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "interrupt route test",
		})

		runningWorkerID := uuid.New()
		var err error
		chat, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusRunning,
			WorkerID:    uuid.NullUUID{UUID: runningWorkerID, Valid: true},
			StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
		})

		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, chat.Status)
		require.True(t, chat.WorkerID.Valid)
		require.True(t, chat.StartedAt.Valid)
		require.True(t, chat.HeartbeatAt.Valid)

		interrupted, err := client.InterruptChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, interrupted.ID)
		require.Equal(t, codersdk.ChatStatusWaiting, interrupted.Status)

		persisted, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, persisted.Status)
		require.False(t, persisted.WorkerID.Valid)
		require.False(t, persisted.StartedAt.Valid)
		require.False(t, persisted.HeartbeatAt.Valid)
	})

	t.Run("ChatNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.InterruptChat(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestRegenerateChatTitle(t *testing.T) {
	t.Parallel()

	t.Run("ChatNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.RegenerateChatTitle(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("UpdateDenied", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		clientRaw, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			Authorizer: &coderdtest.FakeAuthorizer{
				ConditionalReturn: func(_ context.Context, _ rbac.Subject, action policy.Action, object rbac.Object) error {
					if action == policy.ActionUpdate && object.Type == rbac.ResourceChat.Type {
						return xerrors.New("denied")
					}
					return nil
				},
			},
			DeploymentValues: chatDeploymentValues(t),
		})
		client := codersdk.NewExperimentalClient(clientRaw)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "chat with update denied",
		})

		_, err := client.RegenerateChatTitle(ctx, chat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)
		_, err = otherClient.RegenerateChatTitle(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "chat for unauthenticated regeneration",
			}},
		})
		require.NoError(t, err)

		unauthenticatedClient := codersdk.NewExperimentalClient(codersdk.New(client.URL))
		_, err = unauthenticatedClient.RegenerateChatTitle(ctx, chat.ID)
		requireSDKError(t, err, http.StatusUnauthorized)
	})

	t.Run("UsageLimitExceeded", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "chat over usage limit",
			}},
		})
		require.NoError(t, err)

		wantResetsAt := enableDailyChatUsageLimit(ctx, t, db, 100)
		insertAssistantCostMessage(t, db, chat.ID, modelConfig.ID, 100)

		_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		require.NoError(t, err)

		_, err = client.RegenerateChatTitle(ctx, chat.ID)
		limitErr := codersdk.ChatUsageLimitExceededFrom(err)
		require.NotNil(t, limitErr)
		require.Equal(t, "Chat usage limit exceeded.", limitErr.Message)
		require.Equal(t, int64(100), limitErr.SpentMicros)
		require.Equal(t, int64(100), limitErr.LimitMicros)
		require.True(
			t,
			limitErr.ResetsAt.Equal(wantResetsAt),
			"expected resets_at %s, got %s",
			wantResetsAt.UTC().Format(time.RFC3339),
			limitErr.ResetsAt.UTC().Format(time.RFC3339),
		)
	})

	t.Run("AlreadyInProgress", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "chat with lock held",
		})

		_, err := db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Valid: true},
			StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},

			LastError: sql.NullString{},
		})
		require.NoError(t, err)

		res, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/title/regenerate", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusConflict, res.StatusCode)

		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, "Title regeneration already in progress for this chat.", resp.Message)
	})

	t.Run("PendingWithoutWorker", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "pending chat without worker",
		})

		var err error
		chat, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusPending,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},

			LastError: sql.NullString{},
		})
		require.NoError(t, err)

		before, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		res, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/title/regenerate", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusConflict, res.StatusCode)

		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, "Title regeneration already in progress for this chat.", resp.Message)

		persisted, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusPending, persisted.Status)
		require.False(t, persisted.WorkerID.Valid)
		require.True(t, persisted.UpdatedAt.Equal(before.UpdatedAt))
	})

	t.Run("RegenerationFailure", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "test chat",
				},
			},
		})
		require.NoError(t, err)

		// Wait for background processing triggered by signalWake
		// to finish before setting the status, otherwise the
		// processor may update updated_at concurrently.
		require.Eventually(t, func() bool {
			c, getErr := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
			if getErr != nil {
				return false
			}
			return c.Status != database.ChatStatusPending && c.Status != database.ChatStatusRunning
		}, testutil.WaitShort, testutil.IntervalFast)

		_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusCompleted,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		require.NoError(t, err)

		before, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		_, err = client.RegenerateChatTitle(ctx, chat.ID)
		requireSDKError(t, err, http.StatusInternalServerError)

		after, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.True(t, after.UpdatedAt.Equal(before.UpdatedAt))
	})
}

func TestProposeChatTitle(t *testing.T) {
	t.Parallel()

	t.Run("ChatNotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.ProposeChatTitle(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("UpdateDenied", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		clientRaw, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			Authorizer: &coderdtest.FakeAuthorizer{
				ConditionalReturn: func(_ context.Context, _ rbac.Subject, action policy.Action, object rbac.Object) error {
					if action == policy.ActionUpdate && object.Type == rbac.ResourceChat.Type {
						return xerrors.New("denied")
					}
					return nil
				},
			},
			DeploymentValues: chatDeploymentValues(t),
		})
		client := codersdk.NewExperimentalClient(clientRaw)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "chat with update denied",
		})

		_, err := client.ProposeChatTitle(ctx, chat.ID)

		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("DoesNotPersistTitleOrBumpUpdatedAt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "test chat"},
			},
		})
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			c, getErr := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
			if getErr != nil {
				return false
			}
			return c.Status != database.ChatStatusPending && c.Status != database.ChatStatusRunning
		}, testutil.WaitShort, testutil.IntervalFast)

		before, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		_, err = client.ProposeChatTitle(ctx, chat.ID)
		requireSDKError(t, err, http.StatusInternalServerError)

		after, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Equal(t, before.Title, after.Title,
			"propose must not persist the suggested title")
		require.True(t, after.UpdatedAt.Equal(before.UpdatedAt),
			"propose must not bump updated_at")
	})
}

func TestGetChatDiffStatus(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		rawClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
			ExternalAuthConfigs: []*externalauth.Config{
				{
					ID:    "gitlab-test",
					Type:  "gitlab",
					Regex: regexp.MustCompile(`github\.com`),
				},
			},
		})
		client := codersdk.NewExperimentalClient(rawClient)
		db := api.Database

		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		noCachedStatusChat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "get diff status route no cache",
		})

		noCachedChat, err := client.GetChat(ctx, noCachedStatusChat.ID)
		require.NoError(t, err)
		require.Equal(t, noCachedStatusChat.ID, noCachedChat.ID)
		require.Nil(t, noCachedChat.DiffStatus)

		cachedStatusChat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "get diff status route cached",
		})

		refreshedAt := time.Now().UTC().Truncate(time.Second)
		staleAt := refreshedAt.Add(time.Hour)
		_, err = db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          cachedStatusChat.ID,
				Url:             sql.NullString{},
				GitBranch:       "feature/diff-status",
				GitRemoteOrigin: "git@github.com:coder/coder.git",
				StaleAt:         staleAt,
			},
		)
		require.NoError(t, err)

		_, err = db.UpsertChatDiffStatus(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusParams{
				ChatID: cachedStatusChat.ID,
				Url:    sql.NullString{},
				PullRequestState: sql.NullString{
					String: " open ",
					Valid:  true,
				},
				ChangesRequested: true,
				Additions:        11,
				Deletions:        4,
				ChangedFiles:     3,
				RefreshedAt:      refreshedAt,
				StaleAt:          staleAt,
			},
		)
		require.NoError(t, err)

		cachedChat, err := client.GetChat(ctx, cachedStatusChat.ID)
		require.NoError(t, err)
		require.Equal(t, cachedStatusChat.ID, cachedChat.ID)
		require.NotNil(t, cachedChat.DiffStatus)
		cachedStatus := cachedChat.DiffStatus
		require.Equal(t, cachedStatusChat.ID, cachedStatus.ChatID)
		require.NotNil(t, cachedStatus.URL)
		require.Equal(t, "https://github.com/coder/coder/tree/feature/diff-status", *cachedStatus.URL)
		require.NotNil(t, cachedStatus.PullRequestState)
		require.Equal(t, "open", *cachedStatus.PullRequestState)
		require.True(t, cachedStatus.ChangesRequested)
		require.EqualValues(t, 11, cachedStatus.Additions)
		require.EqualValues(t, 4, cachedStatus.Deletions)
		require.EqualValues(t, 3, cachedStatus.ChangedFiles)
		require.NotNil(t, cachedStatus.RefreshedAt)
		require.WithinDuration(t, refreshedAt, *cachedStatus.RefreshedAt, time.Second)
		require.NotNil(t, cachedStatus.StaleAt)
		require.WithinDuration(t, staleAt, *cachedStatus.StaleAt, time.Second)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)
		_, err = otherClient.GetChat(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestGetChatDiffContents(t *testing.T) {
	t.Parallel()

	t.Run("SuccessWithCachedRepositoryReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		rawClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues: chatDeploymentValues(t),
			ExternalAuthConfigs: []*externalauth.Config{
				{
					ID:    "gitlab-test",
					Type:  "gitlab",
					Regex: regexp.MustCompile(`gitlab\.example\.com`),
				},
			},
		})
		client := codersdk.NewExperimentalClient(rawClient)
		db := api.Database
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "diff contents with cached repository reference",
		})

		_, err := db.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				Url:             sql.NullString{},
				GitBranch:       "feature/cached-diff",
				GitRemoteOrigin: "https://gitlab.example.com/acme/project.git",
				StaleAt:         time.Now().UTC().Add(time.Hour),
			},
		)
		require.NoError(t, err)

		diffContents, err := client.GetChatDiffContents(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, diffContents.ChatID)
		require.NotNil(t, diffContents.Provider)
		require.Equal(t, "gitlab", *diffContents.Provider)
		require.NotNil(t, diffContents.RemoteOrigin)
		require.Equal(t, "https://gitlab.example.com/acme/project.git", *diffContents.RemoteOrigin)
		require.NotNil(t, diffContents.Branch)
		require.Equal(t, "feature/cached-diff", *diffContents.Branch)
		require.Nil(t, diffContents.PullRequestURL)
		require.Empty(t, diffContents.Diff)
	})

	t.Run("SuccessWithoutCachedReference", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "diff contents test",
				},
			},
		})
		require.NoError(t, err)

		diffContents, err := client.GetChatDiffContents(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, diffContents.ChatID)
		require.Nil(t, diffContents.Provider)
		require.Nil(t, diffContents.RemoteOrigin)
		require.Nil(t, diffContents.Branch)
		require.Nil(t, diffContents.PullRequestURL)
		require.Empty(t, diffContents.Diff)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "private chat",
				},
			},
		})
		require.NoError(t, err)

		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)
		_, err = otherClient.GetChatDiffContents(ctx, createdChat.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestDeleteChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "delete queued message route test",
		})

		deleteContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("queued message for delete route"),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: deleteContent,
			},
		)
		require.NoError(t, err)

		res, err := client.Request(
			ctx,
			http.MethodDelete,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, queued := range messagesResult.QueuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}

		queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		for _, queued := range queuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}
	})

	t.Run("InvalidQueuedMessageID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "delete queued invalid id",
		})

		invalidRes, err := client.Request(
			ctx,
			http.MethodDelete,
			fmt.Sprintf("/api/experimental/chats/%s/queue/not-an-int", chat.ID),
			nil,
		)
		require.NoError(t, err)

		defer invalidRes.Body.Close()

		err = codersdk.ReadBodyAsError(invalidRes)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid queued message ID.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "invalid syntax")
	})
}

func TestPromoteChatQueuedMessage(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued message route test",
		})

		const queuedText = "queued message for promote route"
		queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(queuedText),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: queuedContent,
			},
		)
		require.NoError(t, err)

		promoteRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusOK, promoteRes.StatusCode)

		var promoted codersdk.ChatMessage
		err = json.NewDecoder(promoteRes.Body).Decode(&promoted)
		require.NoError(t, err)
		require.NotZero(t, promoted.ID)
		require.Equal(t, chat.ID, promoted.ChatID)
		require.Equal(t, codersdk.ChatMessageRoleUser, promoted.Role)

		foundPromotedText := false
		for _, part := range promoted.Content {
			if part.Type == codersdk.ChatMessagePartTypeText &&
				part.Text == queuedText {
				foundPromotedText = true
				break
			}
		}
		require.True(t, foundPromotedText)

		messagesResult, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		for _, queued := range messagesResult.QueuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}

		queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		for _, queued := range queuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}
	})

	t.Run("PromotesAlreadyQueuedMessageAfterLimitReached", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)
		enableDailyChatUsageLimit(ctx, t, db, 100)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued usage limit",
		})

		const queuedText = "queued message for promote route"

		queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(queuedText),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: queuedContent,
			},
		)
		require.NoError(t, err)

		insertAssistantCostMessage(t, db, chat.ID, modelConfig.ID, 100)

		_, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:          chat.ID,
			Status:      database.ChatStatusWaiting,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		require.NoError(t, err)

		promoteRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusOK, promoteRes.StatusCode)

		var promoted codersdk.ChatMessage
		err = json.NewDecoder(promoteRes.Body).Decode(&promoted)
		require.NoError(t, err)
		require.NotZero(t, promoted.ID)
		require.Equal(t, chat.ID, promoted.ChatID)
		require.Equal(t, codersdk.ChatMessageRoleUser, promoted.Role)

		foundPromotedText := false
		for _, part := range promoted.Content {
			if part.Type == codersdk.ChatMessagePartTypeText && part.Text == queuedText {
				foundPromotedText = true
				break
			}
		}
		require.True(t, foundPromotedText)

		queuedMessages, err := db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		for _, queued := range queuedMessages {
			require.NotEqual(t, queuedMessage.ID, queued.ID)
		}
	})

	t.Run("InvalidQueuedMessageID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued invalid id",
		})

		invalidRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/not-an-int/promote", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer invalidRes.Body.Close()

		err = codersdk.ReadBodyAsError(invalidRes)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid queued message ID.", sdkErr.Message)
		require.Contains(t, sdkErr.Detail, "invalid syntax")
	})

	t.Run("MemberWithoutAgentsAccess", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a member without agents-access. Without
		// agents-access the member has no ResourceChat
		// permissions, so the ChatParam middleware returns 404
		// before the handler can check agents-access.
		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued no agents access",
		})

		queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("queued message no agents access"),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: queuedContent,
			},
		)
		require.NoError(t, err)

		promoteRes, err := memberClient.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusNotFound, promoteRes.StatusCode)
	})

	t.Run("ArchivedChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "promote queued archived",
		})

		queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("queued"),
		})
		require.NoError(t, err)
		queuedMessage, err := db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chat.ID,
				Content: queuedContent,
			},
		)
		require.NoError(t, err)

		// Archive the chat.
		_, err = db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		promoteRes, err := client.Request(
			ctx,
			http.MethodPost,
			fmt.Sprintf("/api/experimental/chats/%s/queue/%d/promote", chat.ID, queuedMessage.ID),
			nil,
		)
		require.NoError(t, err)
		defer promoteRes.Body.Close()
		require.Equal(t, http.StatusBadRequest, promoteRes.StatusCode)
		promoteErr := codersdk.ReadBodyAsError(promoteRes)
		var promoteSDKErr *codersdk.Error
		require.ErrorAs(t, promoteErr, &promoteSDKErr)
		require.Contains(t, promoteSDKErr.Message, "archived")
	})
}

func TestChatUsageLimitOverrideRoutes(t *testing.T) {
	t.Parallel()

	t.Run("UpsertUserOverrideRequiresPositiveSpendLimit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)

		res, err := client.Request(
			ctx,
			http.MethodPut,
			fmt.Sprintf("/api/experimental/chats/usage-limits/overrides/%s", member.ID),
			map[string]any{},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat usage limit override.", sdkErr.Message)
		require.Equal(t, "Spend limit must be greater than 0.", sdkErr.Detail)
	})

	t.Run("UpsertUserOverrideMissingUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UpsertChatUsageLimitOverride(ctx, uuid.New(), codersdk.UpsertChatUsageLimitOverrideRequest{
			SpendLimitMicros: 7_000_000,
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "User not found.", sdkErr.Message)
	})

	t.Run("DeleteUserOverrideMissingUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		err := client.DeleteChatUsageLimitOverride(ctx, uuid.New())
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "User not found.", sdkErr.Message)
	})

	t.Run("DeleteUserOverrideMissingOverride", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)

		err := client.DeleteChatUsageLimitOverride(ctx, member.ID)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat usage limit override not found.", sdkErr.Message)
	})

	t.Run("UpdateUserOverride", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)

		_, err := client.UpsertChatUsageLimitOverride(ctx, member.ID, codersdk.UpsertChatUsageLimitOverrideRequest{
			SpendLimitMicros: 5_000_000,
		})
		require.NoError(t, err)

		override, err := client.UpsertChatUsageLimitOverride(ctx, member.ID, codersdk.UpsertChatUsageLimitOverrideRequest{
			SpendLimitMicros: 10_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, member.ID, override.UserID)
		require.NotNil(t, override.SpendLimitMicros)
		require.EqualValues(t, 10_000_000, *override.SpendLimitMicros)

		config, err := client.GetChatUsageLimitConfig(ctx)
		require.NoError(t, err)
		require.Len(t, config.Overrides, 1)
		require.Equal(t, member.ID, config.Overrides[0].UserID)
		require.NotNil(t, config.Overrides[0].SpendLimitMicros)
		require.EqualValues(t, 10_000_000, *config.Overrides[0].SpendLimitMicros)
	})

	t.Run("UpsertGroupOverrideIncludesMemberCount", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: member.ID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: database.PrebuildsSystemUserID})

		override, err := client.UpsertChatUsageLimitGroupOverride(ctx, group.ID, codersdk.UpsertChatUsageLimitGroupOverrideRequest{
			SpendLimitMicros: 7_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, group.ID, override.GroupID)
		require.EqualValues(t, 1, override.MemberCount)
		require.NotNil(t, override.SpendLimitMicros)
		require.EqualValues(t, 7_000_000, *override.SpendLimitMicros)

		config, err := client.GetChatUsageLimitConfig(ctx)
		require.NoError(t, err)

		var listed *codersdk.ChatUsageLimitGroupOverride
		for i := range config.GroupOverrides {
			if config.GroupOverrides[i].GroupID == group.ID {
				listed = &config.GroupOverrides[i]
				break
			}
		}
		require.NotNil(t, listed)
		require.EqualValues(t, 1, listed.MemberCount)
	})

	t.Run("UpdateGroupOverride", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: firstUser.UserID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: member.ID})

		_, err := client.UpsertChatUsageLimitGroupOverride(ctx, group.ID, codersdk.UpsertChatUsageLimitGroupOverrideRequest{
			SpendLimitMicros: 5_000_000,
		})
		require.NoError(t, err)

		override, err := client.UpsertChatUsageLimitGroupOverride(ctx, group.ID, codersdk.UpsertChatUsageLimitGroupOverrideRequest{
			SpendLimitMicros: 10_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, group.ID, override.GroupID)
		require.EqualValues(t, 2, override.MemberCount)
		require.NotNil(t, override.SpendLimitMicros)
		require.EqualValues(t, 10_000_000, *override.SpendLimitMicros)

		config, err := client.GetChatUsageLimitConfig(ctx)
		require.NoError(t, err)
		require.Len(t, config.GroupOverrides, 1)
		require.Equal(t, group.ID, config.GroupOverrides[0].GroupID)
		require.EqualValues(t, 2, config.GroupOverrides[0].MemberCount)
		require.NotNil(t, config.GroupOverrides[0].SpendLimitMicros)
		require.EqualValues(t, 10_000_000, *config.GroupOverrides[0].SpendLimitMicros)
	})

	t.Run("UpsertGroupOverrideMissingGroup", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		_ = coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UpsertChatUsageLimitGroupOverride(ctx, uuid.New(), codersdk.UpsertChatUsageLimitGroupOverrideRequest{
			SpendLimitMicros: 7_000_000,
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Group not found.", sdkErr.Message)
	})

	t.Run("DeleteGroupOverrideMissingOverride", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})

		err := client.DeleteChatUsageLimitGroupOverride(ctx, group.ID)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat usage limit group override not found.", sdkErr.Message)
	})
}

func TestPostChatFile(t *testing.T) {
	t.Parallel()

	t.Run("Success/PNG", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		// Valid PNG header + padding.
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("MissingFilename", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "", bytes.NewReader(data))
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Filename is required")
		require.Contains(t, sdkErr.Detail, "Content-Disposition")
	})

	t.Run("Success/TextPlain", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		data := []byte(`This is a test paste.
With multiple lines.
`)
		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "test.txt", bytes.NewReader(data))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("Success/TextPlainRefinesToJSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "pasted-text.txt", bytes.NewReader([]byte(`{"ok":true}`)))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("Success/TextPlainRefinesToCSV", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "pasted-text.txt", bytes.NewReader([]byte(`name,count
widgets,3
`)))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("UnsupportedContentType", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "application/zip", "test.zip", bytes.NewReader([]byte("PK")))
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("SVGBlocked", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/svg+xml", "test.svg", bytes.NewReader([]byte("<svg></svg>")))
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("ContentSniffingRejectsPNGAsText", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		// Valid 1x1 PNG declared as text/plain should still be rejected.
		data := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x04, 0x00, 0x00, 0x00, 0xB5, 0x1C, 0x0C,
			0x02, 0x00, 0x00, 0x00, 0x0B, 0x49, 0x44, 0x41,
			0x54, 0x78, 0xDA, 0x63, 0xFC, 0xFF, 0x1F, 0x00,
			0x03, 0x03, 0x02, 0x00, 0xEF, 0x9A, 0x1A, 0x2A,
			0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
			0xAE, 0x42, 0x60, 0x82,
		}
		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "test.txt", bytes.NewReader(data))
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "does not match")
	})

	t.Run("ContentSniffingRejectsPlainTextAsJSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "application/json", "payload.json", bytes.NewReader([]byte("not actually json")))
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "does not match")
	})

	t.Run("TooLarge", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		// 10 MB + 1 byte, with valid PNG header to pass media type check.
		data := make([]byte, 10<<20+1)
		copy(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
		_, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.Error(t, err)
	})

	t.Run("Success/TextPlainHTMLLikeContent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		data := []byte(`<!DOCTYPE html>
<html><body><p>Paste me as plain text.</p></body></html>
`)
		resp, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "snippet.txt", bytes.NewReader(data))
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, resp.ID)
	})

	t.Run("MissingOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		res, err := client.Request(ctx, http.MethodPost, "/api/experimental/chats/files", bytes.NewReader(data), func(r *http.Request) {
			r.Header.Set("Content-Type", "image/png")
		})

		require.NoError(t, err)
		defer res.Body.Close()
		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Missing organization")
	})

	t.Run("InvalidOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		res, err := client.Request(ctx, http.MethodPost, "/api/experimental/chats/files?organization=not-a-uuid", bytes.NewReader(data), func(r *http.Request) {
			r.Header.Set("Content-Type", "image/png")
		})
		require.NoError(t, err)
		defer res.Body.Close()
		err = codersdk.ReadBodyAsError(res)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Invalid organization ID")
	})

	t.Run("WrongOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		_, err := client.UploadChatFile(ctx, uuid.New(), "image/png", "test.png", bytes.NewReader(data))
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		// dbauthz returns 404 or 500 depending on how the org lookup
		// fails; 403 is also possible. Any non-success code is valid.
		require.GreaterOrEqual(t, sdkErr.StatusCode(), http.StatusBadRequest,
			"expected error status, got %d", sdkErr.StatusCode())
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		unauthed := codersdk.NewExperimentalClient(codersdk.New(client.URL))
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		_, err := unauthed.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		requireSDKError(t, err, http.StatusUnauthorized)
	})

	t.Run("MemberWithoutAgentsAccess", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		// Member without agents-access should be denied.
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		_, err := memberClient.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestGetChatFile(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)

		got, contentType, err := client.GetChatFile(ctx, uploaded.ID)
		require.NoError(t, err)
		require.Equal(t, "image/png", contentType)
		require.Equal(t, data, got)
	})

	t.Run("CacheHeaders", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, "private, max-age=31536000, immutable", res.Header.Get("Cache-Control"))
		require.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
		require.Contains(t, res.Header.Get("Content-Disposition"), "inline")
		require.Contains(t, res.Header.Get("Content-Disposition"), "test.png")
	})

	t.Run("PDFServedAsAttachment", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "application/pdf", "report.pdf", bytes.NewReader([]byte("%PDF-1.7\n")))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, "application/pdf", res.Header.Get("Content-Type"))
		require.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))

		disposition, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition"))
		require.NoError(t, err)
		require.Equal(t, "attachment", disposition)
		require.Equal(t, "report.pdf", params["filename"])
	})

	t.Run("LongFilename", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		longName := strings.Repeat("a", 300) + ".png"
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", longName, bytes.NewReader(data))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		// Filename should be truncated to chatfiles.MaxStoredFileNameBytes (255) bytes.
		cd := res.Header.Get("Content-Disposition")
		require.Contains(t, cd, "inline")
		require.Contains(t, cd, strings.Repeat("a", 255))
		require.NotContains(t, cd, strings.Repeat("a", 256))
	})

	t.Run("UnicodeFilename", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		// Upload with a non-ASCII filename using RFC 5987 encoding,
		// which is what the frontend sends for Unicode filenames.
		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "スクリーンショット.png", bytes.NewReader(data))
		require.NoError(t, err)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/files/%s", uploaded.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		cd := res.Header.Get("Content-Disposition")
		require.Contains(t, cd, "inline")
		_, params, err := mime.ParseMediaType(cd)
		require.NoError(t, err)
		require.Equal(t, "スクリーンショット.png", params["filename"])
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client.Client)

		_, _, err := client.GetChatFile(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client.Client)

		res, err := client.Request(ctx, http.MethodGet,
			"/api/experimental/chats/files/not-a-uuid", nil)
		require.NoError(t, err)
		defer res.Body.Close()
		err = codersdk.ReadBodyAsError(res)
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("OtherUserForbidden", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)

		data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 64)...)
		uploaded, err := client.UploadChatFile(ctx, firstUser.OrganizationID, "image/png", "test.png", bytes.NewReader(data))
		require.NoError(t, err)

		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)
		_, _, err = otherClient.GetChatFile(ctx, uploaded.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

type chatCostTestFixture struct {
	Client            *codersdk.ExperimentalClient
	DB                database.Store
	ModelConfigID     uuid.UUID
	ChatID            uuid.UUID
	EarliestCreatedAt time.Time
	LatestCreatedAt   time.Time
}

// safeOptions returns an explicit time window around the fixture messages to
// avoid app-time/database-time boundary flakes in summary tests.
func (f chatCostTestFixture) safeOptions() codersdk.ChatCostSummaryOptions {
	return codersdk.ChatCostSummaryOptions{
		StartDate: f.EarliestCreatedAt.Add(-time.Minute),
		EndDate:   f.LatestCreatedAt.Add(time.Minute),
	}
}

func seedChatCostFixture(t *testing.T) chatCostTestFixture {
	t.Helper()

	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "test chat",
	})

	msg1 := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 100, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 50, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 500, Valid: true},
		RuntimeMs:       sql.NullInt64{Int64: 1500, Valid: true},
	})
	msg2 := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 100, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 50, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 500, Valid: true},
		RuntimeMs:       sql.NullInt64{Int64: 2500, Valid: true},
	})
	results := []database.ChatMessage{msg1, msg2}
	require.Len(t, results, 2)

	earliestCreatedAt := results[0].CreatedAt
	latestCreatedAt := results[0].CreatedAt
	for _, msg := range results {
		if msg.CreatedAt.Before(earliestCreatedAt) {
			earliestCreatedAt = msg.CreatedAt
		}
		if msg.CreatedAt.After(latestCreatedAt) {
			latestCreatedAt = msg.CreatedAt
		}
	}

	return chatCostTestFixture{
		Client:            client,
		DB:                db,
		ModelConfigID:     modelConfig.ID,
		ChatID:            chat.ID,
		EarliestCreatedAt: earliestCreatedAt,
		LatestCreatedAt:   latestCreatedAt,
	}
}

func assertChatCostSummary(t *testing.T, summary codersdk.ChatCostSummary, modelConfigID, chatID uuid.UUID) {
	t.Helper()

	require.Equal(t, int64(1000), summary.TotalCostMicros)
	require.Equal(t, int64(2), summary.PricedMessageCount)
	require.Equal(t, int64(0), summary.UnpricedMessageCount)
	require.Equal(t, int64(200), summary.TotalInputTokens)
	require.Equal(t, int64(100), summary.TotalOutputTokens)
	require.Equal(t, int64(4000), summary.TotalRuntimeMs)

	require.Len(t, summary.ByModel, 1)
	require.Equal(t, modelConfigID, summary.ByModel[0].ModelConfigID)
	require.Equal(t, int64(1000), summary.ByModel[0].TotalCostMicros)
	require.Equal(t, int64(2), summary.ByModel[0].MessageCount)
	require.Equal(t, int64(4000), summary.ByModel[0].TotalRuntimeMs)

	require.Len(t, summary.ByChat, 1)
	require.Equal(t, chatID, summary.ByChat[0].RootChatID)
	require.Equal(t, int64(1000), summary.ByChat[0].TotalCostMicros)
	require.Equal(t, int64(2), summary.ByChat[0].MessageCount)
	require.Equal(t, int64(4000), summary.ByChat[0].TotalRuntimeMs)
}

func TestChatCostSummary(t *testing.T) {
	t.Parallel()

	t.Run("BasicSummary", func(t *testing.T) {
		t.Parallel()

		f := seedChatCostFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Use a window derived from DB timestamps to avoid time boundary flakes.
		summary, err := f.Client.GetChatCostSummary(ctx, "me", f.safeOptions())
		require.NoError(t, err)
		assertChatCostSummary(t, summary, f.ModelConfigID, f.ChatID)
	})
}

func TestChatCostSummary_AfterModelDeletion(t *testing.T) {
	t.Parallel()

	f := seedChatCostFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	options := f.safeOptions()

	// Baseline: use DB-derived timestamps to avoid time boundary flakes.
	summary, err := f.Client.GetChatCostSummary(ctx, "me", options)
	require.NoError(t, err)
	assertChatCostSummary(t, summary, f.ModelConfigID, f.ChatID)

	// Soft-delete the model config.
	err = f.Client.DeleteChatModelConfig(ctx, f.ModelConfigID)
	require.NoError(t, err)

	// Costs must survive the deletion unchanged within the same safe window.
	summary, err = f.Client.GetChatCostSummary(ctx, "me", options)
	require.NoError(t, err)
	assertChatCostSummary(t, summary, f.ModelConfigID, f.ChatID)
}

func TestChatCostSummary_AdminDrilldown(t *testing.T) {
	t.Parallel()

	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)
	modelConfig := createChatModelConfig(t, client)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           member.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "member chat",
	})

	message := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 200, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 100, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 750, Valid: true},
	})

	options := codersdk.ChatCostSummaryOptions{
		// Pad the DB-assigned timestamp so the query window cannot race it.
		StartDate: message.CreatedAt.Add(-time.Minute),
		EndDate:   message.CreatedAt.Add(time.Minute),
	}

	t.Run("AdminCanDrilldown", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		summary, err := client.GetChatCostSummary(ctx, member.ID.String(), options)
		require.NoError(t, err)
		require.Equal(t, int64(750), summary.TotalCostMicros)
		require.Equal(t, int64(1), summary.PricedMessageCount)
	})

	t.Run("MemberCannotDrilldownOtherUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := memberClient.GetChatCostSummary(ctx, firstUser.UserID.String(), options)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestChatCostUsers(t *testing.T) {
	t.Parallel()

	seedCtx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)
	firstUserRecord, err := db.GetUserByID(dbauthz.AsSystemRestricted(seedCtx), firstUser.UserID)
	require.NoError(t, err)
	modelConfig := createChatModelConfig(t, client)

	adminChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "admin chat",
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          adminChat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 100, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 50, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 300, Valid: true},
	})

	memberChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           member.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "member chat",
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          memberChat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 200, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 100, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 800, Valid: true},
	})

	t.Run("AdminCanListUsers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := client.GetChatCostUsers(ctx, codersdk.ChatCostUsersOptions{})
		require.NoError(t, err)
		require.Equal(t, int64(2), resp.Count)
		require.Len(t, resp.Users, 2)
		require.Equal(t, member.ID, resp.Users[0].UserID)
		require.Equal(t, member.Username, resp.Users[0].Username)
		require.Equal(t, int64(800), resp.Users[0].TotalCostMicros)
		require.Equal(t, int64(1), resp.Users[0].MessageCount)
		require.Equal(t, int64(1), resp.Users[0].ChatCount)
		require.Equal(t, firstUser.UserID, resp.Users[1].UserID)
		require.Equal(t, firstUserRecord.Username, resp.Users[1].Username)
		require.Equal(t, int64(300), resp.Users[1].TotalCostMicros)
	})

	t.Run("AdminCanFilterAndPaginateUsers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := client.GetChatCostUsers(ctx, codersdk.ChatCostUsersOptions{
			Username: member.Username,
			Pagination: codersdk.Pagination{
				Limit:  1,
				Offset: 0,
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), resp.Count)
		require.Len(t, resp.Users, 1)
		require.Equal(t, member.ID, resp.Users[0].UserID)
		require.Equal(t, member.Username, resp.Users[0].Username)
	})

	t.Run("MemberCannotListUsers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := memberClient.GetChatCostUsers(ctx, codersdk.ChatCostUsersOptions{})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})
}

func TestChatCostSummary_DateRange(t *testing.T) {
	t.Parallel()

	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "date range test",
	})

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 100, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 50, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 500, Valid: true},
	})

	now := time.Now()

	t.Run("MessageInRange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		summary, err := client.GetChatCostSummary(ctx, "me", codersdk.ChatCostSummaryOptions{
			StartDate: now.Add(-time.Hour),
			EndDate:   now.Add(time.Hour),
		})
		require.NoError(t, err)
		require.Equal(t, int64(500), summary.TotalCostMicros)
		require.Equal(t, int64(1), summary.PricedMessageCount)
	})

	t.Run("MessageOutOfRange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		summary, err := client.GetChatCostSummary(ctx, "me", codersdk.ChatCostSummaryOptions{
			StartDate: now.Add(time.Hour),
			EndDate:   now.Add(2 * time.Hour),
		})
		require.NoError(t, err)
		require.Equal(t, int64(0), summary.TotalCostMicros)
		require.Equal(t, int64(0), summary.PricedMessageCount)
	})
}

func TestChatCostSummary_UnpricedMessages(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		LastModelConfigID: modelConfig.ID,
		Title:             "unpriced test",
	})

	pricedMessage := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		InputTokens:     sql.NullInt64{Int64: 100, Valid: true},
		OutputTokens:    sql.NullInt64{Int64: 50, Valid: true},
		TotalCostMicros: sql.NullInt64{Int64: 500, Valid: true},
	})

	unpricedMessage := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
		Role:          database.ChatMessageRoleAssistant,
		InputTokens:   sql.NullInt64{Int64: 200, Valid: true},
		OutputTokens:  sql.NullInt64{Int64: 75, Valid: true},
	})

	earliestCreatedAt := pricedMessage.CreatedAt
	latestCreatedAt := pricedMessage.CreatedAt
	if unpricedMessage.CreatedAt.Before(earliestCreatedAt) {
		earliestCreatedAt = unpricedMessage.CreatedAt
	}
	if unpricedMessage.CreatedAt.After(latestCreatedAt) {
		latestCreatedAt = unpricedMessage.CreatedAt
	}
	options := codersdk.ChatCostSummaryOptions{
		// Pad the DB-assigned timestamps to avoid time boundary flakes.
		StartDate: earliestCreatedAt.Add(-time.Minute),
		EndDate:   latestCreatedAt.Add(time.Minute),
	}

	summary, err := client.GetChatCostSummary(ctx, "me", options)
	require.NoError(t, err)

	require.Equal(t, int64(500), summary.TotalCostMicros)
	require.Equal(t, int64(1), summary.PricedMessageCount)
	require.Equal(t, int64(1), summary.UnpricedMessageCount)
	require.Equal(t, int64(300), summary.TotalInputTokens)
	require.Equal(t, int64(125), summary.TotalOutputTokens)
}

func requireChatModelPricing(
	t *testing.T,
	actual *codersdk.ChatModelCallConfig,
	expected *codersdk.ChatModelCallConfig,
) {
	t.Helper()
	require.NotNil(t, actual)
	require.NotNil(t, expected)

	require.NotNil(t, actual.Cost)
	require.NotNil(t, expected.Cost)
	require.NotNil(t, actual.Cost.InputPricePerMillionTokens)
	require.NotNil(t, actual.Cost.OutputPricePerMillionTokens)
	require.NotNil(t, actual.Cost.CacheReadPricePerMillionTokens)
	require.NotNil(t, actual.Cost.CacheWritePricePerMillionTokens)

	require.True(t, expected.Cost.InputPricePerMillionTokens.Equal(*actual.Cost.InputPricePerMillionTokens))
	require.True(t, expected.Cost.OutputPricePerMillionTokens.Equal(*actual.Cost.OutputPricePerMillionTokens))
	require.True(t, expected.Cost.CacheReadPricePerMillionTokens.Equal(*actual.Cost.CacheReadPricePerMillionTokens))
	require.True(t, expected.Cost.CacheWritePricePerMillionTokens.Equal(*actual.Cost.CacheWritePricePerMillionTokens))
}

func decRef(value string) *decimal.Decimal {
	d := decimal.RequireFromString(value)
	return &d
}

func TestWatchChatDesktop(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		createdChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "desktop no workspace test",
				},
			},
		})
		require.NoError(t, err)

		// Try to connect to the desktop endpoint — should fail because
		// chat has no workspace.
		res, err := client.Request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/stream/desktop", createdChat.ID),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
}

// TestWatchChatGitAuthz is the regression test for CODAGT-184. The
// git-watcher handler opens a bidirectional websocket into the
// workspace agent and streams repository diffs; before the fix it only
// enforced chat:read, so a chat owner who lost workspace SSH /
// application-connect access (e.g. by being demoted from owner to
// template-admin after the chat was bound) could keep exfiltrating
// repository contents.
//
// Other behaviors (no-workspace 400, websocket proxy plumbing,
// disconnected-agent 400) are covered by the mock-based TestWatchChatGit
// in coderd/workspaceagents_internal_test.go.
func TestWatchChatGitAuthz(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	// adminClient = first user (site: owner). Creates the chat below
	// and is demoted after the chat is bound.
	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	_ = createChatModelConfig(t, adminClient)

	// A second owner is needed to run UpdateUserRoles on the first
	// user, since the server refuses self-demotion.
	secondAdminClient, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID, rbac.RoleOwner())

	// The workspace owner is a distinct user so that stripping
	// adminClient's site roles fully removes its workspace
	// SSH/ApplicationConnect. If the workspace were owned by
	// adminClient, the user would retain SSH via the org-member role
	// regardless of site-role demotion.
	_, workspaceOwner := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)

	workspaceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: firstUser.OrganizationID,
		OwnerID:        workspaceOwner.ID,
	}).WithAgent().Do()

	chat, err := adminClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeText, Text: "codagt-184"},
		},
	})
	require.NoError(t, err)

	// Bind the chat to the workspace while adminClient still has
	// site-wide workspace:ssh via the owner role.
	err = adminClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
		WorkspaceID: &workspaceBuild.Workspace.ID,
	})
	require.NoError(t, err)

	// Demote adminClient via the second owner. template-admin grants
	// workspace:read (site) but not workspace:ssh or
	// workspace:application_connect; agents-access preserves
	// chat:create|read|update on chats the user owns, so the
	// demoted user still passes ExtractChatParam for their own chat.
	_, err = secondAdminClient.UpdateUserRoles(ctx, firstUser.UserID.String(), codersdk.UpdateRoles{
		Roles: []string{rbac.RoleTemplateAdmin().String()},
	})
	require.NoError(t, err)

	_, err = secondAdminClient.UpdateOrganizationMemberRoles(ctx, firstUser.OrganizationID, firstUser.UserID.String(), codersdk.UpdateRoles{
		Roles: []string{rbac.RoleAgentsAccess()},
	})
	require.NoError(t, err)

	res, err := adminClient.Request(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/api/experimental/chats/%s/stream/git", chat.ID),
		nil,
	)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

func createChatModelConfig(t *testing.T, client *codersdk.ExperimentalClient) codersdk.ChatModelConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai",
		APIKey:   "test-api-key",
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)
	return modelConfig
}

func createAdditionalChatModelConfig(
	t *testing.T,
	client *codersdk.ExperimentalClient,
	provider string,
	model string,
) codersdk.ChatModelConfig {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	contextLimit := int64(4096)
	isDefault := false
	modelConfig, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     provider,
		Model:        model,
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)
	return modelConfig
}

func createDisabledChatModelConfig(
	t *testing.T,
	client *codersdk.ExperimentalClient,
	provider string,
	model string,
) codersdk.ChatModelConfig {
	t.Helper()

	modelConfig := createAdditionalChatModelConfig(t, client, provider, model)
	ctx := testutil.Context(t, testutil.WaitLong)
	updated, err := client.UpdateChatModelConfig(ctx, modelConfig.ID, codersdk.UpdateChatModelConfigRequest{
		Enabled: ptr.Ref(false),
	})
	require.NoError(t, err)
	return updated
}

//nolint:tparallel,paralleltest // Subtests share a single coderdtest instance.
func TestChatSystemPrompt(t *testing.T) {
	t.Parallel()

	adminClient, db := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	_ = createChatModelConfig(t, adminClient)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	const workspaceAwareness = "There is no workspace associated with this chat yet. Create one using the create_workspace tool before using workspace tools like execute, read_file, write_file, etc."

	updateChatSystemPrompt := func(t *testing.T, ctx context.Context, req codersdk.UpdateChatSystemPromptRequest) {
		t.Helper()

		err := adminClient.UpdateChatSystemPrompt(ctx, req)
		require.NoError(t, err)
	}

	getChatSystemPrompt := func(t *testing.T, ctx context.Context) codersdk.ChatSystemPromptResponse {
		t.Helper()

		resp, err := adminClient.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
		return resp
	}

	assertInjectedSystemMessages := func(t *testing.T, ctx context.Context, wantResolvedPrompt string) {
		t.Helper()

		chat, err := adminClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: fmt.Sprintf("system prompt composition %s", t.Name()),
				},
			},
		})
		require.NoError(t, err)

		messages, err := db.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		var systemTexts []string
		for _, message := range messages {
			if message.Role != database.ChatMessageRoleSystem {
				continue
			}
			parts, err := chatprompt.ParseContent(message)
			require.NoError(t, err)
			require.Len(t, parts, 1)
			require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
			systemTexts = append(systemTexts, parts[0].Text)
		}

		if wantResolvedPrompt == "" {
			require.Equal(t, []string{workspaceAwareness}, systemTexts)
			return
		}

		require.Equal(t, []string{wantResolvedPrompt, workspaceAwareness}, systemTexts)
	}

	t.Run("ReturnsEmptyWhenUnset", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		resp := getChatSystemPrompt(t, ctx)
		require.Equal(t, "", resp.SystemPrompt)
		require.True(t, resp.IncludeDefaultSystemPrompt, "should default to true")
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt, "should return the built-in default prompt for preview")
	})

	t.Run("AdminCanSet", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "You are a helpful coding assistant.",
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})

		resp := getChatSystemPrompt(t, ctx)
		require.Equal(t, "You are a helpful coding assistant.", resp.SystemPrompt)
		require.True(t, resp.IncludeDefaultSystemPrompt)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
	})

	t.Run("AdminCanUnset", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		// Unset by sending an empty string.
		updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "",
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})

		resp := getChatSystemPrompt(t, ctx)
		require.Empty(t, resp.SystemPrompt)
		require.True(t, resp.IncludeDefaultSystemPrompt)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
	})

	t.Run("ToggleIncludeDefault", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "",
			IncludeDefaultSystemPrompt: ptr.Ref(false),
		})

		resp := getChatSystemPrompt(t, ctx)
		require.Empty(t, resp.SystemPrompt)
		require.False(t, resp.IncludeDefaultSystemPrompt)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)

		updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "",
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})

		resp = getChatSystemPrompt(t, ctx)
		require.Empty(t, resp.SystemPrompt)
		require.True(t, resp.IncludeDefaultSystemPrompt)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
	})

	t.Run("PreservesIncludeDefaultWhenOmitted", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		rawDB, pubsub := dbtestutil.NewDB(t)
		store := &failNextChatSystemPromptStore{Store: rawDB}
		client := codersdk.NewExperimentalClient(coderdtest.New(t, &coderdtest.Options{
			Database:         store,
			Pubsub:           pubsub,
			DeploymentValues: chatDeploymentValues(t),
		}))
		_ = coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		err := client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "",
			IncludeDefaultSystemPrompt: ptr.Ref(false),
		})
		require.NoError(t, err)

		store.failNextGetChatIncludeDefaultSystemPrompt.Store(true)
		store.failNextUpsertChatIncludeDefaultSystemPrompt.Store(true)

		err = client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt: "Omitted toggle request",
		})
		require.NoError(t, err)

		resp, err := client.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
		require.Equal(t, "Omitted toggle request", resp.SystemPrompt)
		require.False(t, resp.IncludeDefaultSystemPrompt)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
	})

	t.Run("ExistingCustomPromptDefaultsIncludeDefaultOff", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		legacyClient, legacyDB := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, legacyClient.Client)
		_ = createChatModelConfig(t, legacyClient)

		require.NoError(t, legacyDB.UpsertChatSystemPrompt(dbauthz.AsSystemRestricted(ctx), "Legacy custom instructions"))

		resp, err := legacyClient.GetChatSystemPrompt(ctx)
		require.NoError(t, err)
		require.Equal(t, "Legacy custom instructions", resp.SystemPrompt)
		require.False(t, resp.IncludeDefaultSystemPrompt)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)

		chat, err := legacyClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: fmt.Sprintf("legacy custom prompt %s", t.Name()),
			}},
		})
		require.NoError(t, err)

		messages, err := legacyDB.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		var systemTexts []string
		for _, message := range messages {
			if message.Role != database.ChatMessageRoleSystem {
				continue
			}
			parts, err := chatprompt.ParseContent(message)
			require.NoError(t, err)
			require.Len(t, parts, 1)
			require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
			systemTexts = append(systemTexts, parts[0].Text)
		}

		require.Equal(t, []string{"Legacy custom instructions", workspaceAwareness}, systemTexts)
	})

	t.Run("DefaultSystemPromptPreview", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		resp := getChatSystemPrompt(t, ctx)
		require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
		require.NotEmpty(t, resp.DefaultSystemPrompt, "built-in default prompt should not be empty")
	})

	t.Run("SavesBothFieldsTogether", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "Custom instructions for all users.",
			IncludeDefaultSystemPrompt: ptr.Ref(false),
		})

		resp := getChatSystemPrompt(t, ctx)
		require.Equal(t, "Custom instructions for all users.", resp.SystemPrompt)
		require.False(t, resp.IncludeDefaultSystemPrompt)

		updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "Different instructions.",
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})

		resp = getChatSystemPrompt(t, ctx)
		require.Equal(t, "Different instructions.", resp.SystemPrompt)
		require.True(t, resp.IncludeDefaultSystemPrompt)
	})

	t.Run("PromptComposition", func(t *testing.T) {
		t.Run("DefaultOnlyWhenToggleOnAndEmpty", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)

			updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
				SystemPrompt:               "",
				IncludeDefaultSystemPrompt: ptr.Ref(true),
			})

			resp := getChatSystemPrompt(t, ctx)
			require.Empty(t, resp.SystemPrompt)
			require.True(t, resp.IncludeDefaultSystemPrompt)
			require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
			assertInjectedSystemMessages(t, ctx, chatd.DefaultSystemPrompt)
		})

		t.Run("BothWhenToggleOnAndNonEmpty", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)

			updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
				SystemPrompt:               "Custom instructions",
				IncludeDefaultSystemPrompt: ptr.Ref(true),
			})

			resp := getChatSystemPrompt(t, ctx)
			require.Equal(t, "Custom instructions", resp.SystemPrompt)
			require.True(t, resp.IncludeDefaultSystemPrompt)
			require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
			assertInjectedSystemMessages(t, ctx, chatd.DefaultSystemPrompt+"\n\nCustom instructions")
		})

		t.Run("CustomOnlyWhenToggleOff", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)

			updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
				SystemPrompt:               "Custom only",
				IncludeDefaultSystemPrompt: ptr.Ref(false),
			})

			resp := getChatSystemPrompt(t, ctx)
			require.Equal(t, "Custom only", resp.SystemPrompt)
			require.False(t, resp.IncludeDefaultSystemPrompt)
			require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
			assertInjectedSystemMessages(t, ctx, "Custom only")
		})

		t.Run("EmptyWhenToggleOffAndEmpty", func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)

			updateChatSystemPrompt(t, ctx, codersdk.UpdateChatSystemPromptRequest{
				SystemPrompt:               "",
				IncludeDefaultSystemPrompt: ptr.Ref(false),
			})

			resp := getChatSystemPrompt(t, ctx)
			require.Empty(t, resp.SystemPrompt)
			require.False(t, resp.IncludeDefaultSystemPrompt)
			require.Equal(t, chatd.DefaultSystemPrompt, resp.DefaultSystemPrompt)
			assertInjectedSystemMessages(t, ctx, "")
		})
	})

	t.Run("CreateChatFallsBackToDefaultWhenSystemPromptConfigReadFailsWithIncludeDefaultEnabled", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		rawDB, pubsub := dbtestutil.NewDB(t)
		store := &failNextChatSystemPromptStore{Store: rawDB}
		client := codersdk.NewExperimentalClient(coderdtest.New(t, &coderdtest.Options{
			Database:         store,
			Pubsub:           pubsub,
			DeploymentValues: chatDeploymentValues(t),
		}))
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		err := client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "Keep custom instructions",
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})
		require.NoError(t, err)

		store.failNextGetChatSystemPromptConfig.Store(true)
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: fmt.Sprintf("config-read fallback %s", t.Name()),
			}},
		})
		require.NoError(t, err)

		messages, err := rawDB.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		var systemTexts []string
		for _, message := range messages {
			if message.Role != database.ChatMessageRoleSystem {
				continue
			}
			parts, err := chatprompt.ParseContent(message)
			require.NoError(t, err)
			require.Len(t, parts, 1)
			require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
			systemTexts = append(systemTexts, parts[0].Text)
		}

		require.Equal(t, []string{chatd.DefaultSystemPrompt, workspaceAwareness}, systemTexts)
	})

	t.Run("CreateChatFallbackIgnoresDisabledPreferenceWhenConfigReadFails", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		rawDB, pubsub := dbtestutil.NewDB(t)
		store := &failNextChatSystemPromptStore{Store: rawDB}
		client := codersdk.NewExperimentalClient(coderdtest.New(t, &coderdtest.Options{
			Database:         store,
			Pubsub:           pubsub,
			DeploymentValues: chatDeploymentValues(t),
		}))
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		err := client.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "Do not use the default prompt",
			IncludeDefaultSystemPrompt: ptr.Ref(false),
		})
		require.NoError(t, err)

		// A config read failure loses all admin preferences, including
		// include_default=false, so chat creation falls back to the built-in default.
		store.failNextGetChatSystemPromptConfig.Store(true)
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: fmt.Sprintf("config-read fallback %s", t.Name()),
			}},
		})
		require.NoError(t, err)

		messages, err := rawDB.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		var systemTexts []string
		for _, message := range messages {
			if message.Role != database.ChatMessageRoleSystem {
				continue
			}
			parts, err := chatprompt.ParseContent(message)
			require.NoError(t, err)
			require.Len(t, parts, 1)
			require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
			systemTexts = append(systemTexts, parts[0].Text)
		}

		require.Equal(t, []string{chatd.DefaultSystemPrompt, workspaceAwareness}, systemTexts)
	})

	t.Run("NonAdminFails", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		err := memberClient.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               "This should fail.",
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})
		requireSDKError(t, err, http.StatusForbidden)

		_, err = memberClient.GetChatSystemPrompt(ctx)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("UnauthenticatedFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		anonClient := codersdk.NewExperimentalClient(codersdk.New(adminClient.URL))
		_, err := anonClient.GetChatSystemPrompt(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})

	t.Run("TooLong", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		tooLong := strings.Repeat("a", 131073)
		err := adminClient.UpdateChatSystemPrompt(ctx, codersdk.UpdateChatSystemPromptRequest{
			SystemPrompt:               tooLong,
			IncludeDefaultSystemPrompt: ptr.Ref(true),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "System prompt exceeds maximum length.", sdkErr.Message)
	})
}

//nolint:tparallel,paralleltest // Subtests share a single coderdtest instance.
func TestChatPlanModeInstructions(t *testing.T) {
	t.Parallel()

	adminClient, _ := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	_ = createChatModelConfig(t, adminClient)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	updateChatPlanModeInstructions := func(t *testing.T, ctx context.Context, req codersdk.UpdateChatPlanModeInstructionsRequest) {
		t.Helper()

		err := adminClient.UpdateChatPlanModeInstructions(ctx, req)
		require.NoError(t, err)
	}

	getChatPlanModeInstructions := func(t *testing.T, ctx context.Context) codersdk.ChatPlanModeInstructionsResponse {
		t.Helper()

		resp, err := adminClient.GetChatPlanModeInstructions(ctx)
		require.NoError(t, err)
		return resp
	}

	roundTripTests := []struct {
		name    string
		updates []string
		want    string
	}{
		{
			name: "DefaultGETReturnsEmpty",
			want: "",
		},
		{
			name:    "PUTThenGETRoundTrips",
			updates: []string{"Use plan mode for multi-step changes."},
			want:    "Use plan mode for multi-step changes.",
		},
	}
	for _, tt := range roundTripTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)

			for _, instructions := range tt.updates {
				updateChatPlanModeInstructions(t, ctx, codersdk.UpdateChatPlanModeInstructionsRequest{
					PlanModeInstructions: instructions,
				})
			}

			resp := getChatPlanModeInstructions(t, ctx)
			require.Equal(t, tt.want, resp.PlanModeInstructions)
		})
	}

	t.Run("OversizedPayloadReturns400", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		tooLong := strings.Repeat("a", 131073)

		err := adminClient.UpdateChatPlanModeInstructions(ctx, codersdk.UpdateChatPlanModeInstructionsRequest{
			PlanModeInstructions: tooLong,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Plan mode instructions exceed maximum length.", sdkErr.Message)
	})

	t.Run("NonAdminGETReturns404", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := memberClient.GetChatPlanModeInstructions(ctx)
		requireSDKError(t, err, http.StatusNotFound)
	})
}

//nolint:tparallel,paralleltest // Setting subtests share per-setting coderdtest instances.
func TestChatModelOverrides(t *testing.T) {
	t.Parallel()

	type overrideResponse struct {
		context       codersdk.ChatModelOverrideContext
		modelConfigID string
		isMalformed   bool
	}

	type settingTest struct {
		name     string
		context  codersdk.ChatModelOverrideContext
		dbGet    func(context.Context, database.Store) (string, error)
		dbUpsert func(context.Context, database.Store, string) error
	}

	settingPath := func(overrideContext codersdk.ChatModelOverrideContext) string {
		return "/api/experimental/chats/config/model-override/" + string(overrideContext)
	}

	getOverride := func(
		ctx context.Context,
		client *codersdk.ExperimentalClient,
		overrideContext codersdk.ChatModelOverrideContext,
	) (overrideResponse, error) {
		resp, err := client.GetChatModelOverride(ctx, overrideContext)
		if err != nil {
			return overrideResponse{}, err
		}
		return overrideResponse{
			context:       resp.Context,
			modelConfigID: resp.ModelConfigID,
			isMalformed:   resp.IsMalformed,
		}, nil
	}

	putOverride := func(
		ctx context.Context,
		client *codersdk.ExperimentalClient,
		overrideContext codersdk.ChatModelOverrideContext,
		modelConfigID string,
	) error {
		return client.UpdateChatModelOverride(
			ctx,
			overrideContext,
			codersdk.UpdateChatModelOverrideRequest{ModelConfigID: modelConfigID},
		)
	}

	settings := []settingTest{
		{
			name:    "General",
			context: codersdk.ChatModelOverrideContextGeneral,
			dbGet: func(ctx context.Context, db database.Store) (string, error) {
				return db.GetChatGeneralModelOverride(dbauthz.AsSystemRestricted(ctx))
			},
			dbUpsert: func(ctx context.Context, db database.Store, value string) error {
				return db.UpsertChatGeneralModelOverride(dbauthz.AsSystemRestricted(ctx), value)
			},
		},
		{
			name:    "Explore",
			context: codersdk.ChatModelOverrideContextExplore,
			dbGet: func(ctx context.Context, db database.Store) (string, error) {
				return db.GetChatExploreModelOverride(dbauthz.AsSystemRestricted(ctx))
			},
			dbUpsert: func(ctx context.Context, db database.Store, value string) error {
				return db.UpsertChatExploreModelOverride(dbauthz.AsSystemRestricted(ctx), value)
			},
		},
		{
			name:    "TitleGeneration",
			context: codersdk.ChatModelOverrideContextTitleGeneration,
			dbGet: func(ctx context.Context, db database.Store) (string, error) {
				return db.GetChatTitleGenerationModelOverride(dbauthz.AsSystemRestricted(ctx))
			},
			dbUpsert: func(ctx context.Context, db database.Store, value string) error {
				return db.UpsertChatTitleGenerationModelOverride(dbauthz.AsSystemRestricted(ctx), value)
			},
		},
	}

	for _, setting := range settings {
		t.Run(setting.name, func(t *testing.T) {
			adminClient, db := newChatClientWithDatabase(t)
			firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
			defaultModel := createChatModelConfig(t, adminClient)
			openAIModel := createAdditionalChatModelConfig(
				t,
				adminClient,
				defaultModel.Provider,
				"gpt-4.1-mini-"+string(setting.context),
			)
			disabledModel := createDisabledChatModelConfig(
				t,
				adminClient,
				defaultModel.Provider,
				"gpt-4.1-disabled-"+string(setting.context),
			)
			memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
			memberClient := codersdk.NewExperimentalClient(memberClientRaw)

			t.Run("DefaultGETReturnsEmpty", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				resp, err := getOverride(ctx, adminClient, setting.context)
				require.NoError(t, err)
				require.Equal(t, setting.context, resp.context)
				require.Empty(t, resp.modelConfigID)
				require.False(t, resp.isMalformed)

				raw, err := setting.dbGet(ctx, db)
				require.NoError(t, err)
				require.Empty(t, raw, "expected empty stored override for %s", settingPath(setting.context))
			})

			t.Run("AdminCanSetAndClear", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				err := putOverride(ctx, adminClient, setting.context, openAIModel.ID.String())
				require.NoError(t, err)

				raw, err := setting.dbGet(ctx, db)
				require.NoError(t, err)
				require.Equal(t, openAIModel.ID.String(), raw, "expected stored override for %s", settingPath(setting.context))

				resp, err := getOverride(ctx, adminClient, setting.context)
				require.NoError(t, err)
				require.Equal(t, setting.context, resp.context)
				require.Equal(t, openAIModel.ID.String(), resp.modelConfigID)
				require.False(t, resp.isMalformed)

				err = putOverride(ctx, adminClient, setting.context, "")
				require.NoError(t, err)

				raw, err = setting.dbGet(ctx, db)
				require.NoError(t, err)
				require.Empty(t, raw, "expected cleared override for %s", settingPath(setting.context))

				resp, err = getOverride(ctx, adminClient, setting.context)
				require.NoError(t, err)
				require.Equal(t, setting.context, resp.context)
				require.Empty(t, resp.modelConfigID)
				require.False(t, resp.isMalformed)
			})

			t.Run("MalformedStoredOverrideIsReportedAndCanBeCleared", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				require.NoError(t, setting.dbUpsert(ctx, db, "not-a-uuid"))

				resp, err := getOverride(ctx, adminClient, setting.context)
				require.NoError(t, err)
				require.Equal(t, setting.context, resp.context)
				require.Empty(t, resp.modelConfigID)
				require.True(t, resp.isMalformed)

				err = putOverride(ctx, adminClient, setting.context, "")
				require.NoError(t, err)

				raw, err := setting.dbGet(ctx, db)
				require.NoError(t, err)
				require.Empty(t, raw, "expected malformed override to be cleared for %s", settingPath(setting.context))

				resp, err = getOverride(ctx, adminClient, setting.context)
				require.NoError(t, err)
				require.Equal(t, setting.context, resp.context)
				require.Empty(t, resp.modelConfigID)
				require.False(t, resp.isMalformed)
			})

			t.Run("InvalidUUIDReturns400", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				err := putOverride(ctx, adminClient, setting.context, "not-a-uuid")
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid model_config_id.", sdkErr.Message)
				require.Equal(t, "Value \"not-a-uuid\" is not a valid UUID.", sdkErr.Detail)
			})

			t.Run("DisabledModelReturns400", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				err := putOverride(ctx, adminClient, setting.context, disabledModel.ID.String())
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid model_config_id.", sdkErr.Message)
			})

			t.Run("UnknownModelReturns400", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				unknownModelID := uuid.New()

				err := putOverride(ctx, adminClient, setting.context, unknownModelID.String())
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "Invalid model_config_id.", sdkErr.Message)
			})

			t.Run("NonAdminGETReturns404", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				_, err := getOverride(ctx, memberClient, setting.context)
				requireSDKError(t, err, http.StatusNotFound)
			})

			t.Run("NonAdminPUTReturns403", func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)

				err := putOverride(ctx, memberClient, setting.context, defaultModel.ID.String())
				requireSDKError(t, err, http.StatusForbidden)
			})
		})
	}

	t.Run("UnknownContextReturns400", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient.Client)
		unknownContext := codersdk.ChatModelOverrideContext("not-a-context")

		_, err := getOverride(ctx, adminClient, unknownContext)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat model override context.", sdkErr.Message)
		require.Equal(
			t,
			`Expected one of general, explore, title_generation. Got "not-a-context".`,
			sdkErr.Detail,
		)

		err = putOverride(ctx, adminClient, unknownContext, "")
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid chat model override context.", sdkErr.Message)
		require.Equal(
			t,
			`Expected one of general, explore, title_generation. Got "not-a-context".`,
			sdkErr.Detail,
		)
	})

	t.Run("NonAdminUnknownContextUsesAuthResponse", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		unknownContext := codersdk.ChatModelOverrideContext("not-a-context")

		_, err := getOverride(ctx, memberClient, unknownContext)
		requireSDKError(t, err, http.StatusNotFound)

		err = putOverride(ctx, memberClient, unknownContext, "")
		requireSDKError(t, err, http.StatusForbidden)
	})
}

func TestChatDesktopEnabled(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsFalseWhenUnset", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient.Client)

		resp, err := adminClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.False(t, resp.EnableDesktop)
	})

	t.Run("AdminCanSetTrue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient.Client)

		err := adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		require.NoError(t, err)

		resp, err := adminClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.True(t, resp.EnableDesktop)
	})

	t.Run("AdminCanSetFalse", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient.Client)

		// Set true first, then set false.
		err := adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		require.NoError(t, err)

		err = adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: false,
		})
		require.NoError(t, err)

		resp, err := adminClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.False(t, resp.EnableDesktop)
	})

	t.Run("NonAdminCanRead", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		err := adminClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		require.NoError(t, err)

		resp, err := memberClient.GetChatDesktopEnabled(ctx)
		require.NoError(t, err)
		require.True(t, resp.EnableDesktop)
	})

	t.Run("NonAdminWriteFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		err := memberClient.UpdateChatDesktopEnabled(ctx, codersdk.UpdateChatDesktopEnabledRequest{
			EnableDesktop: true,
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("UnauthenticatedFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient.Client)

		anonClient := codersdk.NewExperimentalClient(codersdk.New(adminClient.URL))
		_, err := anonClient.GetChatDesktopEnabled(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
	})
}

func TestChatDebugLoggingSettings(t *testing.T) {
	t.Parallel()

	t.Run("DefaultDisabled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		adminResp, err := adminClient.GetChatDebugLogging(ctx)
		require.NoError(t, err)
		require.False(t, adminResp.AllowUsers)
		require.False(t, adminResp.ForcedByDeployment)

		userResp, err := memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.False(t, userResp.DebugLoggingEnabled)
		require.False(t, userResp.UserToggleAllowed)
		require.False(t, userResp.ForcedByDeployment)
	})

	t.Run("AdminAllowsUsersToOptIn", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		err := adminClient.UpdateChatDebugLogging(ctx, codersdk.UpdateChatDebugLoggingAllowUsersRequest{
			AllowUsers: true,
		})
		require.NoError(t, err)

		userResp, err := memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.False(t, userResp.DebugLoggingEnabled)
		require.True(t, userResp.UserToggleAllowed)
		require.False(t, userResp.ForcedByDeployment)

		err = memberClient.UpdateUserChatDebugLogging(ctx, codersdk.UpdateUserChatDebugLoggingRequest{
			DebugLoggingEnabled: true,
		})
		require.NoError(t, err)

		userResp, err = memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.True(t, userResp.DebugLoggingEnabled)
		require.True(t, userResp.UserToggleAllowed)
		require.False(t, userResp.ForcedByDeployment)

		// Admin revocation must flip the user's effective state even
		// while the stored opt-in is true. A regression that kept
		// returning the stored opt-in would be masked if the user had
		// already opted out, so we revoke here before the user touches
		// their setting.
		err = adminClient.UpdateChatDebugLogging(ctx, codersdk.UpdateChatDebugLoggingAllowUsersRequest{
			AllowUsers: false,
		})
		require.NoError(t, err)

		userResp, err = memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.False(t, userResp.DebugLoggingEnabled)
		require.False(t, userResp.UserToggleAllowed)
		require.False(t, userResp.ForcedByDeployment)

		// Re-allowing must restore the previously stored opt-in
		// without requiring the user to opt in again.
		err = adminClient.UpdateChatDebugLogging(ctx, codersdk.UpdateChatDebugLoggingAllowUsersRequest{
			AllowUsers: true,
		})
		require.NoError(t, err)

		userResp, err = memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.True(t, userResp.DebugLoggingEnabled, "stored opt-in must survive an admin allow/revoke cycle")
		require.True(t, userResp.UserToggleAllowed)
		require.False(t, userResp.ForcedByDeployment)

		// User can explicitly opt back out while admin still allows the
		// toggle. This exercises the UpsertUserChatDebugLoggingEnabled
		// success path for the false value.
		err = memberClient.UpdateUserChatDebugLogging(ctx, codersdk.UpdateUserChatDebugLoggingRequest{
			DebugLoggingEnabled: false,
		})
		require.NoError(t, err)

		userResp, err = memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.False(t, userResp.DebugLoggingEnabled)
		require.True(t, userResp.UserToggleAllowed)
		require.False(t, userResp.ForcedByDeployment)
	})

	t.Run("UserWriteFailsWhenAdminDisabled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		err := memberClient.UpdateUserChatDebugLogging(ctx, codersdk.UpdateUserChatDebugLoggingRequest{
			DebugLoggingEnabled: true,
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("NonAdminCannotManageAdminSetting", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		_, err := memberClient.GetChatDebugLogging(ctx)
		requireSDKError(t, err, http.StatusNotFound)

		err = memberClient.UpdateChatDebugLogging(ctx, codersdk.UpdateChatDebugLoggingAllowUsersRequest{
			AllowUsers: true,
		})
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("DeploymentForceEnablesDebugLogging", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		values := chatDeploymentValues(t)
		values.AI.Chat.DebugLoggingEnabled = serpent.Bool(true)
		adminClient := newChatClientWithDeploymentValues(t, values)
		firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		adminResp, err := adminClient.GetChatDebugLogging(ctx)
		require.NoError(t, err)
		require.False(t, adminResp.AllowUsers)
		require.True(t, adminResp.ForcedByDeployment)

		userResp, err := memberClient.GetUserChatDebugLogging(ctx)
		require.NoError(t, err)
		require.True(t, userResp.DebugLoggingEnabled)
		require.False(t, userResp.UserToggleAllowed)
		require.True(t, userResp.ForcedByDeployment)

		err = memberClient.UpdateUserChatDebugLogging(ctx, codersdk.UpdateUserChatDebugLoggingRequest{
			DebugLoggingEnabled: false,
		})
		requireSDKError(t, err, http.StatusConflict)
	})

	t.Run("UnauthenticatedUserReadFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		adminClient := newChatClient(t)
		coderdtest.CreateFirstUser(t, adminClient.Client)

		anonClient := codersdk.NewExperimentalClient(codersdk.New(adminClient.URL))
		_, err := anonClient.GetUserChatDebugLogging(ctx)
		requireSDKError(t, err, http.StatusUnauthorized)
	})
}

// seedChatDebugRun inserts a debug run for a chat, bypassing the chatd
// service so HTTP handlers can be exercised in isolation. Steps are
// inserted separately via seedChatDebugStep.
func seedChatDebugRun(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	startedAt time.Time,
) database.ChatDebugRun {
	t.Helper()

	run, err := db.InsertChatDebugRun(dbauthz.AsSystemRestricted(ctx), database.InsertChatDebugRunParams{
		ChatID:    chatID,
		Kind:      string(codersdk.ChatDebugRunKindChatTurn),
		Status:    string(codersdk.ChatDebugStatusInProgress),
		Provider:  sql.NullString{String: "openai", Valid: true},
		Model:     sql.NullString{String: "gpt-4o-mini", Valid: true},
		StartedAt: sql.NullTime{Time: startedAt, Valid: true},
		UpdatedAt: sql.NullTime{Time: startedAt, Valid: true},
	})
	require.NoError(t, err)
	return run
}

func seedChatDebugStep(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	run database.ChatDebugRun,
	stepNumber int32,
) database.ChatDebugStep {
	t.Helper()

	step, err := db.InsertChatDebugStep(dbauthz.AsSystemRestricted(ctx), database.InsertChatDebugStepParams{
		RunID:      run.ID,
		ChatID:     run.ChatID,
		StepNumber: stepNumber,
		Operation:  string(codersdk.ChatDebugStepOperationStream),
		Status:     string(codersdk.ChatDebugStatusCompleted),
	})
	require.NoError(t, err)
	return step
}

func TestChatDebugRuns(t *testing.T) {
	t.Parallel()

	t.Run("ListReturnsRunsNewestFirst", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-runs-list",
		})

		base := time.Now().UTC().Add(-time.Hour).Round(time.Second)
		older := seedChatDebugRun(ctx, t, db, chat.ID, base)
		newer := seedChatDebugRun(ctx, t, db, chat.ID, base.Add(10*time.Minute))

		runs, err := memberClient.GetChatDebugRuns(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, runs, 2)
		require.Equal(t, newer.ID, runs[0].ID, "newest run must come first")
		require.Equal(t, older.ID, runs[1].ID)
		require.Equal(t, codersdk.ChatDebugRunKindChatTurn, runs[0].Kind)
		require.Equal(t, codersdk.ChatDebugStatusInProgress, runs[0].Status)
	})

	t.Run("ListCapsAt100", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-runs-cap",
		})

		base := time.Now().UTC().Add(-24 * time.Hour).Round(time.Second)
		// Seed 101 runs with monotonically increasing started_at. The
		// handler caps at 100, so the oldest run (i=0) must be excluded
		// and the remaining runs must be returned newest-first.
		seeded := make([]database.ChatDebugRun, 101)
		for i := range seeded {
			seeded[i] = seedChatDebugRun(ctx, t, db, chat.ID, base.Add(time.Duration(i)*time.Minute))
		}

		runs, err := client.GetChatDebugRuns(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, runs, 100, "list must be capped at maxDebugRuns")
		require.Equal(t, seeded[100].ID, runs[0].ID, "newest seeded run must come first")
		require.Equal(t, seeded[1].ID, runs[99].ID, "oldest retained run must be last, proving the cap drops the oldest")
		returned := make(map[uuid.UUID]struct{}, len(runs))
		for _, r := range runs {
			returned[r.ID] = struct{}{}
		}
		require.NotContains(t, returned, seeded[0].ID, "oldest seeded run must be excluded by the cap")
	})

	t.Run("ReturnsEmptyListWhenNoRuns", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-runs-empty",
		})

		// Guard against a regression from `make([]..., 0, n)` to
		// `var summaries []...`, which would silently serialize as
		// `null` instead of `[]`.
		runs, err := client.GetChatDebugRuns(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, runs, "runs slice must be non-nil even when empty")
		require.Empty(t, runs)
	})

	t.Run("NonExistentChatReturns404", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		coderdtest.CreateFirstUser(t, client.Client)

		_, err := client.GetChatDebugRuns(ctx, uuid.New())
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("NonOwnerCannotList", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Chat owned by the first (admin) user.
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-runs-other-owner",
		})

		seedChatDebugRun(ctx, t, db, chat.ID, time.Now().UTC())

		otherClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID, rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID))
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)

		_, err := otherClient.GetChatDebugRuns(ctx, chat.ID)

		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestChatDebugRun(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsRunWithSteps", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-run-detail",
		})

		run := seedChatDebugRun(ctx, t, db, chat.ID, time.Now().UTC())
		firstStep := seedChatDebugStep(ctx, t, db, run, 1)
		secondStep := seedChatDebugStep(ctx, t, db, run, 2)

		got, err := client.GetChatDebugRun(ctx, chat.ID, run.ID)
		require.NoError(t, err)
		require.Equal(t, run.ID, got.ID)
		require.Equal(t, chat.ID, got.ChatID)
		require.Equal(t, codersdk.ChatDebugRunKindChatTurn, got.Kind)
		require.Equal(t, codersdk.ChatDebugStatusInProgress, got.Status)
		require.NotNil(t, got.Provider)
		require.Equal(t, "openai", *got.Provider)
		require.Len(t, got.Steps, 2)
		require.Equal(t, firstStep.ID, got.Steps[0].ID)
		require.Equal(t, secondStep.ID, got.Steps[1].ID)
		require.Equal(t, codersdk.ChatDebugStepOperationStream, got.Steps[0].Operation)
	})

	t.Run("ReturnsRunWithoutSteps", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-run-empty",
		})
		run := seedChatDebugRun(ctx, t, db, chat.ID, time.Now().UTC())

		got, err := client.GetChatDebugRun(ctx, chat.ID, run.ID)
		require.NoError(t, err)
		require.Equal(t, run.ID, got.ID)
		require.NotNil(t, got.Steps, "steps slice must be non-nil even when empty")
		require.Empty(t, got.Steps)
	})

	t.Run("InvalidRunIDReturns400", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-run-bad-uuid",
		})

		// Issue a raw request with a non-UUID run ID to exercise the
		// handler's parser path.
		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/debug/runs/not-a-uuid", chat.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("NonExistentRunReturns404", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-run-missing",
		})

		_, err := client.GetChatDebugRun(ctx, chat.ID, uuid.New())

		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("RunOnOtherChatReturns404", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Two chats owned by the same user. A run on chat A must not
		// be addressable through chat B's URL.
		chatA := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-run-chat-a",
		})
		chatB := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "debug-run-chat-b",
		})

		runOnA := seedChatDebugRun(ctx, t, db, chatA.ID, time.Now().UTC())

		_, err := client.GetChatDebugRun(ctx, chatB.ID, runOnA.ID)

		requireSDKError(t, err, http.StatusNotFound)
	})
}

func TestChatAdvisorConfig_GetDefault(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.AdvisorConfig{}, resp)
}

func TestChatAdvisorConfig_Update(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	want := codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   5,
		MaxOutputTokens: 1024,
		ReasoningEffort: "high",
	}

	err := adminClient.UpdateChatAdvisorConfig(ctx, want)
	require.NoError(t, err)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, want, resp)
}

func TestChatAdvisorConfig_MemberCannotWriteButCanRead(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	want := codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   2,
		MaxOutputTokens: 256,
	}

	err := adminClient.UpdateChatAdvisorConfig(ctx, want)
	require.NoError(t, err)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, want, resp)

	err = memberClient.UpdateChatAdvisorConfig(ctx, codersdk.UpdateAdvisorConfigRequest{
		Enabled: true,
	})
	requireSDKError(t, err, http.StatusForbidden)

	// Members must still be able to read the advisor config: the dbauthz
	// layer only requires an authenticated actor, and the GET handler has
	// no RBAC check because the admin settings UI and chatd runtime are
	// the planned consumers. This assertion pins that behavior so a
	// future RBAC tightening is a deliberate change.
	memberResp, err := memberClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, want, memberResp)

	resp, err = adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, want, resp)
}

func TestChatAdvisorConfig_NegativeMaxUsesPerRunRejected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	err := adminClient.UpdateChatAdvisorConfig(ctx, codersdk.UpdateAdvisorConfigRequest{
		MaxUsesPerRun: -1,
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "max_uses_per_run")
	require.Contains(t, sdkErr.Message, "-1")
	require.Contains(t, sdkErr.Message, "non-negative")
}

func TestChatAdvisorConfig_NegativeMaxOutputTokensRejected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	err := adminClient.UpdateChatAdvisorConfig(ctx, codersdk.UpdateAdvisorConfigRequest{
		MaxOutputTokens: -1,
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, "max_output_tokens")
	require.Contains(t, sdkErr.Message, "-1")
	require.Contains(t, sdkErr.Message, "non-negative")
}

func TestChatAdvisorConfig_RoundTripModelConfigID(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	modelConfig := createChatModelConfig(t, adminClient)

	want := codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 2048,
		ModelConfigID:   modelConfig.ID,
		ReasoningEffort: "medium",
	}

	err := adminClient.UpdateChatAdvisorConfig(ctx, want)
	require.NoError(t, err)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, want, resp)
}

func TestChatAdvisorConfig_InvalidReasoningEffort(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	err := adminClient.UpdateChatAdvisorConfig(ctx, codersdk.UpdateAdvisorConfigRequest{
		ReasoningEffort: "ultra",
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, `reasoning_effort "ultra"`)
	require.Contains(t, sdkErr.Message, "not valid")
}

func TestChatAdvisorConfig_InvalidModelConfigID(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	unknownID := uuid.New()
	err := adminClient.UpdateChatAdvisorConfig(ctx, codersdk.UpdateAdvisorConfigRequest{
		ModelConfigID: unknownID,
	})
	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Contains(t, sdkErr.Message, unknownID.String())
	require.Contains(t, sdkErr.Message, "does not match any existing model config")
}

func TestChatAdvisorConfig_RoundTripZeroValues(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	want := codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   0,
		MaxOutputTokens: 0,
	}

	err := adminClient.UpdateChatAdvisorConfig(ctx, want)
	require.NoError(t, err)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, want, resp)
}

// TestChatAdvisorConfig_OverwriteClearsPreviousValues pins PUT to
// full-replace semantics. A second write with zero-valued fields must
// clear every field set by a prior non-zero write, so nothing leaks if
// someone later introduces merge/patch semantics.
func TestChatAdvisorConfig_OverwriteClearsPreviousValues(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	modelConfig := createChatModelConfig(t, adminClient)

	rich := codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   5,
		MaxOutputTokens: 1024,
		ModelConfigID:   modelConfig.ID,
		ReasoningEffort: "high",
	}
	err := adminClient.UpdateChatAdvisorConfig(ctx, rich)
	require.NoError(t, err)

	sparse := codersdk.AdvisorConfig{Enabled: true}
	err = adminClient.UpdateChatAdvisorConfig(ctx, sparse)
	require.NoError(t, err)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, sparse, resp)
}

// TestChatAdvisorConfig_CanBeDisabledAfterEnabled pins the feature
// gate's "off" path. The downstream runtime gates the advisor tool and
// prompt guidance on Enabled, so a regression that silently drops or
// ignores Enabled: false on PUT would leave the feature stuck on.
func TestChatAdvisorConfig_CanBeDisabledAfterEnabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	err := adminClient.UpdateChatAdvisorConfig(ctx, codersdk.AdvisorConfig{
		Enabled:       true,
		MaxUsesPerRun: 2,
	})
	require.NoError(t, err)

	enabledResp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.True(t, enabledResp.Enabled)

	err = adminClient.UpdateChatAdvisorConfig(ctx, codersdk.AdvisorConfig{
		Enabled: false,
	})
	require.NoError(t, err)

	disabledResp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.False(t, disabledResp.Enabled)
}

func TestChatAdvisorConfig_ClampsNegativeStoredValues(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	stored := `{"enabled":true,"max_uses_per_run":-3,"max_output_tokens":-99}`
	err := db.UpsertChatAdvisorConfig(dbauthz.AsSystemRestricted(ctx), stored)
	require.NoError(t, err)

	resp, err := adminClient.GetChatAdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   0,
		MaxOutputTokens: 0,
	}, resp)

	raw, err := db.GetChatAdvisorConfig(dbauthz.AsSystemRestricted(ctx))
	require.NoError(t, err)
	require.JSONEq(t, stored, raw)
}

// TestChatAdvisorConfig_CorruptStoredJSONReturnsError pins that the GET
// handler surfaces a 500 when the stored site_configs row contains bytes
// that are not valid JSON. Unlike the neighboring chat config endpoints,
// this handler unmarshals the raw string server-side, so DB corruption
// must not present as a default-valued 200.
func TestChatAdvisorConfig_CorruptStoredJSONReturnsError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	adminClient, db := newChatClientWithDatabase(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	err := db.UpsertChatAdvisorConfig(dbauthz.AsSystemRestricted(ctx), "not-json")
	require.NoError(t, err)

	_, err = adminClient.GetChatAdvisorConfig(ctx)
	sdkErr := requireSDKError(t, err, http.StatusInternalServerError)
	require.Contains(t, sdkErr.Message, "invalid")
}

// TestChatAdvisorConfig_UnauthenticatedFails pins that the advisor config
// endpoints are gated by apiKeyMiddleware at the /chats route level. The
// handler itself has no auth check, so this test protects against a future
// route restructuring that would accidentally expose these settings.
func TestChatAdvisorConfig_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	adminClient := newChatClient(t)
	coderdtest.CreateFirstUser(t, adminClient.Client)

	anonClient := codersdk.NewExperimentalClient(codersdk.New(adminClient.URL))
	_, err := anonClient.GetChatAdvisorConfig(ctx)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())

	err = anonClient.UpdateChatAdvisorConfig(ctx, codersdk.UpdateAdvisorConfigRequest{
		Enabled: true,
	})
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())
}

func TestChatWorkspaceTTL(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	adminClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)
	anonClient := codersdk.NewExperimentalClient(codersdk.New(adminClient.URL))

	// Default value is 0 (disabled) when nothing has been configured.
	resp, err := adminClient.GetChatWorkspaceTTL(ctx)
	require.NoError(t, err, "get default")
	require.Equal(t, int64(0), resp.WorkspaceTTLMillis, "default should be 0")

	// Admin can set a positive TTL (2h = 7_200_000 ms).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 7_200_000,
	})
	require.NoError(t, err, "admin set 2h")

	resp, err = adminClient.GetChatWorkspaceTTL(ctx)
	require.NoError(t, err, "get after set")
	require.Equal(t, int64(7_200_000), resp.WorkspaceTTLMillis, "should return 7200000 ms (2h)")

	// Non-admin can read the value.
	resp, err = memberClient.GetChatWorkspaceTTL(ctx)
	require.NoError(t, err, "member get")
	require.Equal(t, int64(7_200_000), resp.WorkspaceTTLMillis, "member should see same value")

	// Admin can set back to zero (disabled / template default).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 0,
	})
	require.NoError(t, err, "admin set 0")

	resp, err = adminClient.GetChatWorkspaceTTL(ctx)
	require.NoError(t, err, "get after zero")
	require.Equal(t, int64(0), resp.WorkspaceTTLMillis, "should be 0 after reset")

	// Non-admin write is forbidden.
	err = memberClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 3_600_000,
	})
	requireSDKError(t, err, http.StatusForbidden)

	// Unauthenticated read is rejected.
	_, err = anonClient.GetChatWorkspaceTTL(ctx)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr, "anon get")
	require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode(), "anon should get 401")

	// Validation: negative duration.
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: -3_600_000,
	})
	requireSDKError(t, err, http.StatusBadRequest)

	// Validation: less than 1 minute (30s = 30_000 ms).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 30_000,
	})
	requireSDKError(t, err, http.StatusBadRequest)

	// Boundary: just under 1 minute should be rejected (59_999 ms).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 59_999,
	})
	requireSDKError(t, err, http.StatusBadRequest)

	// Boundary: exactly 1 minute should succeed (60_000 ms).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 60_000,
	})
	require.NoError(t, err, "exactly 1 minute should be accepted")

	// Boundary: exactly 30 days should succeed (720h = 2_592_000_000 ms).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 2_592_000_000,
	})
	require.NoError(t, err, "720h (exactly 30 days) should be accepted")

	// Validation: exceeds 30-day maximum (721h = 2_595_600_000 ms).
	err = adminClient.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
		WorkspaceTTLMillis: 2_595_600_000,
	})
	requireSDKError(t, err, http.StatusBadRequest)
}

func TestChatRetentionDays(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	adminClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	// Default value is 30 (days) when nothing has been configured.
	resp, err := adminClient.GetChatRetentionDays(ctx)
	require.NoError(t, err, "get default")
	require.Equal(t, int32(30), resp.RetentionDays, "default should be 30")

	// Admin can set retention days to 90.
	err = adminClient.UpdateChatRetentionDays(ctx, codersdk.UpdateChatRetentionDaysRequest{
		RetentionDays: 90,
	})
	require.NoError(t, err, "admin set 90")

	resp, err = adminClient.GetChatRetentionDays(ctx)
	require.NoError(t, err, "get after set")
	require.Equal(t, int32(90), resp.RetentionDays, "should return 90")

	// Non-admin member can read the value.
	resp, err = memberClient.GetChatRetentionDays(ctx)
	require.NoError(t, err, "member get")
	require.Equal(t, int32(90), resp.RetentionDays, "member should see same value")

	// Non-admin member cannot write.
	err = memberClient.UpdateChatRetentionDays(ctx, codersdk.UpdateChatRetentionDaysRequest{RetentionDays: 7})
	requireSDKError(t, err, http.StatusForbidden)

	// Admin can disable purge by setting 0.
	err = adminClient.UpdateChatRetentionDays(ctx, codersdk.UpdateChatRetentionDaysRequest{
		RetentionDays: 0,
	})
	require.NoError(t, err, "admin set 0")

	resp, err = adminClient.GetChatRetentionDays(ctx)
	require.NoError(t, err, "get after zero")
	require.Equal(t, int32(0), resp.RetentionDays, "should be 0 after disable")

	// Validation: negative value is rejected.
	err = adminClient.UpdateChatRetentionDays(ctx, codersdk.UpdateChatRetentionDaysRequest{
		RetentionDays: -1,
	})
	requireSDKError(t, err, http.StatusBadRequest)

	// Validation: exceeding the 3650-day maximum is rejected.
	err = adminClient.UpdateChatRetentionDays(ctx, codersdk.UpdateChatRetentionDaysRequest{
		RetentionDays: 3651, // retentionDaysMaximum + 1; keep in sync with coderd/exp_chats.go.
	})
	requireSDKError(t, err, http.StatusBadRequest)
}

func TestChatAutoArchiveDays(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	adminClient := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, adminClient.Client)
	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, adminClient.Client, firstUser.OrganizationID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)

	// Default value is DefaultChatAutoArchiveDays (0, disabled) when
	// nothing has been configured.
	resp, err := adminClient.GetChatAutoArchiveDays(ctx)
	require.NoError(t, err, "get default")
	require.Equal(t, codersdk.DefaultChatAutoArchiveDays, resp.AutoArchiveDays, "default should match DefaultChatAutoArchiveDays")

	// Admin can set auto-archive days to 45.
	err = adminClient.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{
		AutoArchiveDays: 45,
	})
	require.NoError(t, err, "admin set 45")

	resp, err = adminClient.GetChatAutoArchiveDays(ctx)
	require.NoError(t, err, "get after set")
	require.Equal(t, int32(45), resp.AutoArchiveDays, "should return 45")

	// Non-admin member can read the value (same as retention days).
	memberResp, err := memberClient.GetChatAutoArchiveDays(ctx)
	require.NoError(t, err, "member read")
	require.Equal(t, int32(45), memberResp.AutoArchiveDays, "member sees same value")

	// Non-admin member cannot write.
	err = memberClient.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{AutoArchiveDays: 7})
	requireSDKError(t, err, http.StatusForbidden)

	// Admin can disable auto-archive by setting 0.
	err = adminClient.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{
		AutoArchiveDays: 0,
	})
	require.NoError(t, err, "admin set 0")

	resp, err = adminClient.GetChatAutoArchiveDays(ctx)
	require.NoError(t, err, "get after zero")
	require.Equal(t, int32(0), resp.AutoArchiveDays, "should be 0 after disable")

	// An aggressive value of 1 is accepted (no pre-warn to break).
	err = adminClient.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{
		AutoArchiveDays: 1,
	})
	require.NoError(t, err, "admin set 1")

	// Validation: negative value is rejected.
	err = adminClient.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{
		AutoArchiveDays: -1,
	})
	requireSDKError(t, err, http.StatusBadRequest)

	// Validation: exceeding the 3650-day maximum is rejected.
	err = adminClient.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{
		AutoArchiveDays: 3651, // autoArchiveDaysMaximum + 1; keep in sync with coderd/exp_chats.go.
	})
	requireSDKError(t, err, http.StatusBadRequest)
}

//nolint:tparallel // subtests share state via client, firstUser, modelConfig
func TestUserChatCompactionThresholds(t *testing.T) {
	t.Parallel()

	client, _ := newChatClientWithDatabase(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	t.Run("EmptyByDefault", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		thresholds, err := client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Empty(t, thresholds.Thresholds)
	})

	t.Run("PutAndGet", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		override, err := client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 75,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, override.ModelConfigID)
		require.EqualValues(t, 75, override.ThresholdPercent)

		thresholds, err := client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Len(t, thresholds.Thresholds, 1)
		require.Equal(t, modelConfig.ID, thresholds.Thresholds[0].ModelConfigID)
		require.EqualValues(t, 75, thresholds.Thresholds[0].ThresholdPercent)
	})

	t.Run("UpsertChangesValue", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 50,
		})
		require.NoError(t, err)

		override, err := client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 75,
		})
		require.NoError(t, err)
		require.EqualValues(t, 75, override.ThresholdPercent)

		thresholds, err := client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Len(t, thresholds.Thresholds, 1)
		require.EqualValues(t, 75, thresholds.Thresholds[0].ThresholdPercent)
	})

	t.Run("BoundaryValues", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		override, err := client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 0,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, override.ThresholdPercent)

		thresholds, err := client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Len(t, thresholds.Thresholds, 1)
		require.EqualValues(t, 0, thresholds.Thresholds[0].ThresholdPercent)

		override, err = client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 100,
		})
		require.NoError(t, err)
		require.EqualValues(t, 100, override.ThresholdPercent)

		thresholds, err = client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Len(t, thresholds.Thresholds, 1)
		require.EqualValues(t, 100, thresholds.Thresholds[0].ThresholdPercent)
	})

	t.Run("ValidationRejectsInvalid", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: -1,
		})
		requireSDKError(t, err, http.StatusBadRequest)

		_, err = client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 101,
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("Delete", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		err := client.DeleteUserChatCompactionThreshold(ctx, modelConfig.ID)
		require.NoError(t, err)

		thresholds, err := client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Empty(t, thresholds.Thresholds)
	})

	t.Run("DeleteIdempotent", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		err := client.DeleteUserChatCompactionThreshold(ctx, modelConfig.ID)
		require.NoError(t, err)
	})

	t.Run("NonExistentModelConfig", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		fakeID := uuid.New()
		_, err := client.UpdateUserChatCompactionThreshold(ctx, fakeID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 50,
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("IsolatedPerUser", func(t *testing.T) { //nolint:paralleltest // subtests share parent state
		ctx := testutil.Context(t, testutil.WaitLong)

		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		override, err := client.UpdateUserChatCompactionThreshold(ctx, modelConfig.ID, codersdk.UpdateUserChatCompactionThresholdRequest{
			ThresholdPercent: 75,
		})
		require.NoError(t, err)
		require.Equal(t, modelConfig.ID, override.ModelConfigID)
		require.EqualValues(t, 75, override.ThresholdPercent)

		adminThresholds, err := client.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Len(t, adminThresholds.Thresholds, 1)
		require.Equal(t, modelConfig.ID, adminThresholds.Thresholds[0].ModelConfigID)
		require.EqualValues(t, 75, adminThresholds.Thresholds[0].ThresholdPercent)

		memberThresholds, err := memberClient.GetUserChatCompactionThresholds(ctx)
		require.NoError(t, err)
		require.Empty(t, memberThresholds.Thresholds)
	})
}

//nolint:tparallel // Subtests share a single coderdtest instance and run sequentially.
func TestChatTemplateAllowlist(t *testing.T) {
	t.Parallel()

	// Shared setup: one coderdtest instance with two real templates.
	// Subtests that need valid template IDs use these.
	client, store := newChatClientWithDatabase(t)
	admin := coderdtest.CreateFirstUser(t, client.Client)
	tmpl1 := dbgen.Template(t, store, database.Template{
		OrganizationID: admin.OrganizationID,
		CreatedBy:      admin.UserID,
	})
	tmpl2 := dbgen.Template(t, store, database.Template{
		OrganizationID: admin.OrganizationID,
		CreatedBy:      admin.UserID,
	})
	deprecatedTmpl := dbgen.Template(t, store, database.Template{
		OrganizationID: admin.OrganizationID,
		CreatedBy:      admin.UserID,
	})
	//nolint:gocritic // Owner context needed to deprecate the template in test setup.
	ownerRoles, err := rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand()
	require.NoError(t, err)
	err = store.UpdateTemplateAccessControlByID(dbauthz.As(context.Background(), rbac.Subject{
		ID:    "owner",
		Roles: rbac.Roles(ownerRoles),
		Scope: rbac.ExpandableScope(rbac.ScopeAll),
	}), database.UpdateTemplateAccessControlByIDParams{
		ID:         deprecatedTmpl.ID,
		Deprecated: "this template is deprecated",
	})
	require.NoError(t, err, "deprecate template")

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("ReturnsEmptyWhenUnset", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := client.GetChatTemplateAllowlist(ctx)
		require.NoError(t, err)
		require.Empty(t, resp.TemplateIDs)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("AdminCanSet", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		ids := []string{tmpl1.ID.String(), tmpl2.ID.String()}
		err := client.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{TemplateIDs: ids})
		require.NoError(t, err)
		resp, err := client.GetChatTemplateAllowlist(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, ids, resp.TemplateIDs)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("AdminCanClear", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		err := client.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{TemplateIDs: []string{}})
		require.NoError(t, err)
		resp, err := client.GetChatTemplateAllowlist(ctx)
		require.NoError(t, err)
		require.Empty(t, resp.TemplateIDs)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("NonAdminReadFails", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, admin.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		_, err := memberClient.GetChatTemplateAllowlist(ctx)
		requireSDKError(t, err, http.StatusNotFound)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("NonAdminWriteFails", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, admin.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)
		// Uses a random UUID — hits 404 before template validation.
		err := memberClient.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{TemplateIDs: []string{uuid.NewString()}})
		requireSDKError(t, err, http.StatusNotFound)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("UnauthenticatedFails", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		anonClient := codersdk.NewExperimentalClient(codersdk.New(client.URL))
		// Uses a random UUID — hits 401 before template validation.
		err := anonClient.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{TemplateIDs: []string{uuid.NewString()}})
		requireSDKError(t, err, http.StatusUnauthorized)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("InvalidUUIDRejected", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		err := client.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{TemplateIDs: []string{"not-a-uuid"}})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("NonexistentTemplateRejected", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		err := client.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{TemplateIDs: []string{uuid.NewString()}})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("DeprecatedTemplateRejected", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		err := client.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{
			TemplateIDs: []string{deprecatedTmpl.ID.String()},
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	//nolint:paralleltest // Sequential: subtests share a single coderdtest instance.
	t.Run("DeduplicatesIDs", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		id := tmpl1.ID.String()
		err := client.UpdateChatTemplateAllowlist(ctx, codersdk.ChatTemplateAllowlist{
			TemplateIDs: []string{id, id, id},
		})
		require.NoError(t, err)
		resp, err := client.GetChatTemplateAllowlist(ctx)
		require.NoError(t, err)
		require.Len(t, resp.TemplateIDs, 1)
		require.Equal(t, id, resp.TemplateIDs[0])
	})
}

func TestGetChatsByWorkspace(t *testing.T) {
	t.Parallel()

	client, db := newChatClientWithDatabase(t)
	user := coderdtest.CreateFirstUser(t, client.Client)
	modelConfig := createChatModelConfig(t, client)

	// Helper to create a workspace owned by the test user.
	newWorkspace := func() dbfake.WorkspaceBuildBuilder {
		return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent()
	}

	// Helper to insert a chat linked to a workspace.
	insertChat := func(ctx context.Context, title string, workspaceID uuid.UUID) database.Chat {
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             title,
			WorkspaceID:       uuid.NullUUID{UUID: workspaceID, Valid: true},
		})
		return chat
	}

	t.Run("EmptyRequestReturnsEmptyMap", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		result, err := client.GetChatsByWorkspace(ctx, []uuid.UUID{})
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("WorkspaceWithNoChatsOmitted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newWorkspace().Do()

		result, err := client.GetChatsByWorkspace(ctx, []uuid.UUID{ws.Workspace.ID})
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("ReturnsChatLinkedToWorkspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newWorkspace().Do()
		chat := insertChat(ctx, "workspace chat", ws.Workspace.ID)

		result, err := client.GetChatsByWorkspace(ctx, []uuid.UUID{ws.Workspace.ID})
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, chat.ID, result[ws.Workspace.ID])
	})

	t.Run("ArchivedChatsExcluded", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newWorkspace().Do()
		chat := insertChat(ctx, "soon to be archived", ws.Workspace.ID)

		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		result, err := client.GetChatsByWorkspace(ctx, []uuid.UUID{ws.Workspace.ID})
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("ReturnsLatestNonArchivedChat", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := newWorkspace().Do()

		// Insert an older chat and archive it.
		olderChat := insertChat(ctx, "older archived", ws.Workspace.ID)
		err := client.UpdateChat(ctx, olderChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)})
		require.NoError(t, err)

		// Insert two active chats — the second is newer due to insert
		// ordering and should win the "latest" selection in Go after
		// the SQL returns both ordered by updated_at DESC.
		_ = insertChat(ctx, "older active", ws.Workspace.ID)
		newerChat := insertChat(ctx, "newer active", ws.Workspace.ID)

		result, err := client.GetChatsByWorkspace(ctx, []uuid.UUID{ws.Workspace.ID})
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, newerChat.ID, result[ws.Workspace.ID])
	})

	t.Run("MultipleWorkspaces", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		wsA := newWorkspace().Do()
		wsB := newWorkspace().Do()
		wsC := newWorkspace().Do()

		chatA := insertChat(ctx, "chat for workspace A", wsA.Workspace.ID)
		chatB := insertChat(ctx, "chat for workspace B", wsB.Workspace.ID)

		// Query all three workspaces; C has no chats.
		result, err := client.GetChatsByWorkspace(ctx, []uuid.UUID{
			wsA.Workspace.ID,
			wsB.Workspace.ID,
			wsC.Workspace.ID,
		})
		require.NoError(t, err)
		require.Len(t, result, 2)
		require.Equal(t, chatA.ID, result[wsA.Workspace.ID])
		require.Equal(t, chatB.ID, result[wsB.Workspace.ID])
		_, hasC := result[wsC.Workspace.ID]
		require.False(t, hasC, "workspace C should not appear in result")
	})

	t.Run("RejectsTooManyWorkspaceIDs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		ids := make([]uuid.UUID, 26)
		for i := range ids {
			ids[i] = uuid.New()
		}

		_, err := client.GetChatsByWorkspace(ctx, ids)
		require.Error(t, err)
		requireSDKError(t, err, http.StatusBadRequest)
	})
}

func TestSubmitToolResults(t *testing.T) {
	t.Parallel()

	// setupRequiresAction creates a chat via the DB with dynamic tools,
	// inserts an assistant message containing tool-call parts for each
	// given toolCallID, and sets the chat status to requires_action.
	// It returns the chat row so callers can exercise the endpoint.
	setupRequiresAction := func(
		ctx context.Context,
		t *testing.T,
		db database.Store,
		ownerID uuid.UUID,
		organizationID uuid.UUID,
		modelConfigID uuid.UUID,
		dynamicToolName string,
		toolCallIDs []string,
	) database.Chat {
		t.Helper()

		// Marshal dynamic tools into the chat row.
		dynamicTools := []mcp.Tool{{
			Name:        dynamicToolName,
			Description: "a test dynamic tool",
			InputSchema: mcp.ToolInputSchema{Type: "object"},
		}}
		dtJSON, err := json.Marshal(dynamicTools)
		require.NoError(t, err)

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    organizationID,
			OwnerID:           ownerID,
			LastModelConfigID: modelConfigID,
			Title:             "tool-results-test",
			DynamicTools:      pqtype.NullRawMessage{RawMessage: dtJSON, Valid: true},
		})

		// Build assistant message with tool-call parts.
		parts := make([]codersdk.ChatMessagePart, 0, len(toolCallIDs))
		for _, id := range toolCallIDs {
			parts = append(parts, codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: id,
				ToolName:   dynamicToolName,
				Args:       json.RawMessage(`{"key":"value"}`),
			})
		}
		content, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)

		_ = dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID:        chat.ID,
			ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
			Role:          database.ChatMessageRoleAssistant,
			Content:       content,
		})

		// Transition to requires_action.
		chat, err = db.UpdateChatStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateChatStatusParams{
			ID:     chat.ID,
			Status: database.ChatStatusRequiresAction,
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRequiresAction, chat.Status)

		return chat
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_abc", "call_def"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		err := client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_abc", Output: json.RawMessage(`"result_a"`)},
				{ToolCallID: "call_def", Output: json.RawMessage(`"result_b"`)},
			},
		})
		require.NoError(t, err)

		// Verify status is no longer requires_action. The chatd
		// loop may have already picked the chat up and
		// transitioned it further (pending → running → …), so we
		// accept any non-requires_action status.
		gotChat, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotEqual(t, codersdk.ChatStatusRequiresAction, gotChat.Status,
			"chat should no longer be in requires_action after submitting tool results")

		// Verify tool-result messages were persisted.
		msgsResp, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)

		var toolResultCount int
		for _, msg := range msgsResp.Messages {
			if msg.Role == codersdk.ChatMessageRoleTool {
				toolResultCount++
			}
		}
		require.Equal(t, len(toolCallIDs), toolResultCount,
			"expected one tool-result message per submitted result")
	})

	t.Run("WrongStatus", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a chat that is NOT in requires_action status.
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "wrong-status-test",
		})

		err := client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_xyz", Output: json.RawMessage(`"nope"`)},
			},
		})
		requireSDKError(t, err, http.StatusConflict)
	})

	t.Run("MissingResult", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_one", "call_two"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		// Submit only one of the two required results.
		err := client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_one", Output: json.RawMessage(`"partial"`)},
			},
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("UnexpectedResult", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_real"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		// Submit a result with a wrong tool_call_id.
		err := client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_bogus", Output: json.RawMessage(`"wrong"`)},
			},
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("InvalidJSONOutput", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_json"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		// We must bypass the SDK client because json.RawMessage
		// rejects invalid JSON during json.Marshal. A raw HTTP
		// request lets the invalid payload reach the server so we
		// can verify server-side validation.
		rawBody := `{"results":[{"tool_call_id":"call_json","output":not-json,"is_error":false}]}`
		url := client.URL.JoinPath(fmt.Sprintf("/api/experimental/chats/%s/tool-results", chat.ID)).String()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(rawBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("DuplicateToolCallID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_dup1", "call_dup2"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		err := client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_dup1", Output: json.RawMessage(`"result_a"`)},
				{ToolCallID: "call_dup1", Output: json.RawMessage(`"result_b"`)},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "Duplicate tool_call_id")
	})

	t.Run("EmptyResults", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_empty"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		err := client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{},
		})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("NotFoundForDifferentUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_other"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		// Create a second user and try to submit tool results
		// to user A's chat.
		otherClientRaw, _ := coderdtest.CreateAnotherUser(
			t, client.Client, user.OrganizationID,
			rbac.ScopedRoleAgentsAccess(user.OrganizationID),
		)
		otherClient := codersdk.NewExperimentalClient(otherClientRaw)

		err := otherClient.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_other", Output: json.RawMessage(`"nope"`)},
			},
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("MemberWithoutAgentsAccess", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Create a member without agents-access. Without
		// agents-access the member has no ResourceChat
		// permissions, so the ChatParam middleware returns 404
		// before the handler can check agents-access.
		memberClientRaw, member := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		memberClient := codersdk.NewExperimentalClient(memberClientRaw)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_noaccess"}

		chat := setupRequiresAction(ctx, t, db, member.ID, firstUser.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		err := memberClient.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_noaccess", Output: json.RawMessage(`"should fail"`)},
			},
		})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ArchivedChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		const toolName = "my_dynamic_tool"
		toolCallIDs := []string{"call_archived"}

		chat := setupRequiresAction(ctx, t, db, user.UserID, user.OrganizationID, modelConfig.ID, toolName, toolCallIDs)

		// Archive the chat.
		_, err := db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)

		err = client.SubmitToolResults(ctx, chat.ID, codersdk.SubmitToolResultsRequest{
			Results: []codersdk.ToolResult{
				{ToolCallID: "call_archived", Output: json.RawMessage(`"should fail"`)},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "archived")
	})
}

func TestPostChats_DynamicToolValidation(t *testing.T) {
	t.Parallel()

	t.Run("TooManyTools", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		tools := make([]codersdk.DynamicTool, 251)
		for i := range tools {
			tools[i] = codersdk.DynamicTool{
				Name: fmt.Sprintf("tool-%d", i),
			}
		}

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
			UnsafeDynamicTools: tools,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Too many dynamic tools.", sdkErr.Message)
	})

	t.Run("EmptyToolName", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
			UnsafeDynamicTools: []codersdk.DynamicTool{
				{Name: ""},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Dynamic tool name must not be empty.", sdkErr.Message)
	})

	t.Run("DuplicateToolName", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
			UnsafeDynamicTools: []codersdk.DynamicTool{
				{Name: "dup-tool"},
				{Name: "dup-tool"},
			},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Duplicate dynamic tool name.", sdkErr.Message)
	})
}

// requireActiveVersionStore always returns RequireActiveVersion: true so
// tests can exercise relevant code paths without an enterprise license.
type requireActiveVersionStore struct{}

func (requireActiveVersionStore) GetTemplateAccessControl(_ database.Template) dbauthz.TemplateAccessControl {
	return dbauthz.TemplateAccessControl{RequireActiveVersion: true}
}

func (requireActiveVersionStore) SetTemplateAccessControl(_ context.Context, _ database.Store, _ uuid.UUID, _ dbauthz.TemplateAccessControl) error {
	return nil
}

func TestChatStartWorkspace_RequireActiveVersion(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{})
	var store dbauthz.AccessControlStore = requireActiveVersionStore{}
	api.AccessControlStore.Store(&store)
	db := api.Database
	user := coderdtest.CreateFirstUser(t, rawClient)

	// Given: active template version v1 plus workspace stopped on v1.
	wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        user.UserID,
		OrganizationID: user.OrganizationID,
	}).Seed(database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStop,
	}).Do()
	tmplID := wsResp.Workspace.TemplateID
	v1ID := wsResp.Build.TemplateVersionID

	// Given: a new active version v2 is published.
	v2Resp := dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: tmplID, Valid: true},
		OrganizationID: user.OrganizationID,
		CreatedBy:      user.UserID,
	}).Do()
	v2 := v2Resp.TemplateVersion
	require.NotEqual(t, v1ID, v2.ID, "v2 must differ from v1")

	// When: we start the workspace through chatStartWorkspace.
	build, err := coderd.ChatStartWorkspace(api, ctx, user.UserID, wsResp.Workspace.ID,
		codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		})

	// Then: the build is auto-updated to the active version.
	require.NoError(t, err)
	require.Equal(t, v2.ID, build.TemplateVersionID, "build must be on the active version")
	require.Nil(t, build.TemplateVersionPresetID, "no preset must be applied")
}

func TestGetChatMessages_Pagination(t *testing.T) {
	t.Parallel()

	// seedChat creates a chat and inserts `count` user messages, returning
	// the chat and the inserted message IDs in the order they were
	// persisted (ascending). Callers use these IDs as cursor values.
	seedChat := func(
		t *testing.T,
		db database.Store,
		ownerID uuid.UUID,
		organizationID uuid.UUID,
		modelConfigID uuid.UUID,
		count int,
	) (database.Chat, []int64) {
		t.Helper()

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    organizationID,
			OwnerID:           ownerID,
			LastModelConfigID: modelConfigID,
			Title:             "pagination-test",
		})

		ids := make([]int64, count)
		for i := range count {
			content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageText(fmt.Sprintf("msg %d", i)),
			})
			require.NoError(t, err)

			message := dbgen.ChatMessage(t, db, database.ChatMessage{
				ChatID:        chat.ID,
				CreatedBy:     uuid.NullUUID{UUID: ownerID, Valid: true},
				ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
				Role:          database.ChatMessageRoleUser,
				Content:       content,
			})
			ids[i] = message.ID
		}
		return chat, ids
	}

	seedQueuedMessage := func(
		ctx context.Context,
		t *testing.T,
		db database.Store,
		chatID uuid.UUID,
	) {
		t.Helper()

		content, err := json.Marshal([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("queued"),
		})
		require.NoError(t, err)
		_, err = db.InsertChatQueuedMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatQueuedMessageParams{
				ChatID:  chatID,
				Content: content,
			},
		)
		require.NoError(t, err)
	}

	t.Run("NoCursorReturnsAllDESCPlusQueued", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 5)
		seedQueuedMessage(ctx, t, db, chat.ID)

		resp, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		require.Len(t, resp.Messages, 5)
		require.False(t, resp.HasMore)
		require.Len(t, resp.QueuedMessages, 1)

		want := []int64{ids[4], ids[3], ids[2], ids[1], ids[0]}
		got := make([]int64, len(resp.Messages))
		for i, m := range resp.Messages {
			got[i] = m.ID
		}
		require.Equal(t, want, got)
	})

	t.Run("BeforeIDReturnsOlderAndSuppressesQueued", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 5)
		seedQueuedMessage(ctx, t, db, chat.ID)

		resp, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
			BeforeID: ids[2],
		})
		require.NoError(t, err)
		require.False(t, resp.HasMore)
		require.Empty(t, resp.QueuedMessages)

		want := []int64{ids[1], ids[0]}
		got := make([]int64, len(resp.Messages))
		for i, m := range resp.Messages {
			got[i] = m.ID
		}
		require.Equal(t, want, got)
	})

	t.Run("AfterIDReturnsNewerInASCOrderForMonotonicPolling", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 5)
		seedQueuedMessage(ctx, t, db, chat.ID)

		resp, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
			AfterID: ids[1],
		})
		require.NoError(t, err)
		require.False(t, resp.HasMore)
		require.Empty(t, resp.QueuedMessages)

		// ASC order so a polling caller can advance its cursor to
		// max(returned_ids) without gaps.
		want := []int64{ids[2], ids[3], ids[4]}
		got := make([]int64, len(resp.Messages))
		for i, m := range resp.Messages {
			got[i] = m.ID
		}
		require.Equal(t, want, got)
	})

	t.Run("AfterAndBeforeIDReturnsOpenRange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 5)
		seedQueuedMessage(ctx, t, db, chat.ID)

		resp, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
			AfterID:  ids[0],
			BeforeID: ids[4],
		})
		require.NoError(t, err)
		require.False(t, resp.HasMore)
		require.Empty(t, resp.QueuedMessages)

		want := []int64{ids[3], ids[2], ids[1]}
		got := make([]int64, len(resp.Messages))
		for i, m := range resp.Messages {
			got[i] = m.ID
		}
		require.Equal(t, want, got)
	})

	t.Run("LimitCapsAfterIDPageToOldestAndSetsHasMore", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 5)
		// Seed a queued message so the Empty assertion below verifies
		// the cursor suppresses queued rows, not just that none exist.
		seedQueuedMessage(ctx, t, db, chat.ID)

		resp, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
			AfterID: ids[0],
			Limit:   2,
		})
		require.NoError(t, err)
		require.True(t, resp.HasMore)
		require.Empty(t, resp.QueuedMessages)

		// The ASC polling path returns the OLDEST unseen messages
		// first. A burst larger than `limit` would otherwise silently
		// drop the oldest rows between polls on the DESC path.
		want := []int64{ids[1], ids[2]}
		got := make([]int64, len(resp.Messages))
		for i, m := range resp.Messages {
			got[i] = m.ID
		}
		require.Equal(t, want, got)
	})

	t.Run("NegativeAfterIDReturns400", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, _ := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 1)

		res, err := client.Request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/messages?after_id=-1", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var sdkResp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&sdkResp))
		require.Equal(t, "Query parameters have invalid values.", sdkResp.Message)
		require.True(t,
			slices.ContainsFunc(sdkResp.Validations, func(v codersdk.ValidationError) bool {
				return v.Field == "after_id"
			}),
			"expected validation error for after_id field",
		)
	})

	t.Run("NonNumericAfterIDReturns400", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, _ := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 1)

		res, err := client.Request(
			ctx,
			http.MethodGet,
			fmt.Sprintf("/api/experimental/chats/%s/messages?after_id=abc", chat.ID),
			nil,
		)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)

		var sdkResp codersdk.Response
		require.NoError(t, json.NewDecoder(res.Body).Decode(&sdkResp))
		require.Equal(t, "Query parameters have invalid values.", sdkResp.Message)
		require.True(t,
			slices.ContainsFunc(sdkResp.Validations, func(v codersdk.ValidationError) bool {
				return v.Field == "after_id"
			}),
			"expected validation error for after_id field",
		)
	})

	t.Run("AfterIDAtOrAboveMaxReturnsEmpty", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 3)
		// Seed a queued message to prove the cursor path suppresses
		// it even when nothing else comes back.
		seedQueuedMessage(ctx, t, db, chat.ID)

		// The steady-state polling case: the caller already has every
		// message, so after_id equals the largest seen id. The server
		// must return an empty page, not the last row again.
		resp, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
			AfterID: ids[len(ids)-1],
		})
		require.NoError(t, err)
		require.Empty(t, resp.Messages)
		require.False(t, resp.HasMore)
		require.Empty(t, resp.QueuedMessages)
	})

	t.Run("AfterIDGreaterThanOrEqualBeforeIDReturns400", func(t *testing.T) {
		t.Parallel()

		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, 3)

		// Transposed cursors: after >= before. Fail loudly rather
		// than return an empty page indistinguishable from
		// "no messages in this range."
		for _, tc := range []struct {
			name   string
			after  int64
			before int64
		}{
			{"Transposed", ids[2], ids[0]},
			{"Equal", ids[1], ids[1]},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				_, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
					AfterID:  tc.after,
					BeforeID: tc.before,
				})
				sdkErr := requireSDKError(t, err, http.StatusBadRequest)
				require.Equal(t, "after_id must be less than before_id.", sdkErr.Message)
			})
		}
	})

	t.Run("AfterIDPollingWalksBurstWithoutGaps", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		user := coderdtest.CreateFirstUser(t, client.Client)
		modelConfig := createChatModelConfig(t, client)

		// Simulate a polling client that has already acknowledged the
		// first message (cursor = ids[0]) when a burst of
		// `burstSize` new messages arrives. With `limit=pageSize` and
		// `burstSize > pageSize`, the naive DESC-ordered path would
		// silently drop the oldest rows between polls. The ASC
		// dispatch lets the client walk the whole burst by advancing
		// after_id to max(returned_ids) on each tick.
		const burstSize = 60
		const pageSize = 25
		// Seed burstSize+1 rows; ids[0] is the "already acknowledged"
		// message the client saw before the burst.
		chat, ids := seedChat(t, db, user.UserID, user.OrganizationID, modelConfig.ID, burstSize+1)

		var seen []int64
		cursor := ids[0]
		maxPages := (burstSize / pageSize) + 2
		for range maxPages {
			resp, err := client.GetChatMessages(ctx, chat.ID, &codersdk.ChatMessagesPaginationOptions{
				AfterID: cursor,
				Limit:   pageSize,
			})
			require.NoError(t, err)
			if len(resp.Messages) == 0 {
				require.False(t, resp.HasMore)
				break
			}
			for _, m := range resp.Messages {
				seen = append(seen, m.ID)
			}
			// Advance to max(returned). On the ASC path this is the
			// last element of the returned slice.
			cursor = resp.Messages[len(resp.Messages)-1].ID
			if !resp.HasMore {
				break
			}
		}
		require.Equal(t, ids[1:], seen,
			"polling walk must return every burst row exactly once in ascending order")
	})
}

func requireSDKError(t *testing.T, err error, expectedStatus int) *codersdk.Error {
	t.Helper()

	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, expectedStatus, sdkErr.StatusCode())
	return sdkErr
}

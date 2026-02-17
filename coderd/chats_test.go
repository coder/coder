package coderd_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChats(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Test Chat",
		})
		require.NoError(t, err)
		require.Equal(t, "Test Chat", chat.Title)
		require.Equal(t, user.UserID, chat.OwnerID)
		require.Equal(t, codersdk.ChatStatusPending, chat.Status)
	})

	t.Run("CreateTitleFromMessage", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Build a new feature",
		})
		require.NoError(t, err)
		require.Equal(t, "Build a new feature", chat.Title)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Create two chats.
		chatWithStatus, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Message: "Chat 1"})
		require.NoError(t, err)
		chatWithoutStatus, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Message: "Chat 2"})
		require.NoError(t, err)

		_, err = api.Database.UpsertChatDiffStatus(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusParams{
				ChatID:           chatWithStatus.ID,
				Url:              sql.NullString{String: "https://github.com/octocat/hello-world/pull/99", Valid: true},
				PullRequestState: sql.NullString{String: "open", Valid: true},
				ChangesRequested: true,
				Additions:        17,
				Deletions:        4,
				ChangedFiles:     3,
				RefreshedAt:      time.Now().UTC(),
				StaleAt:          time.Now().UTC().Add(time.Minute),
			},
		)
		require.NoError(t, err)

		chats, err := client.ListChats(ctx)
		require.NoError(t, err)
		require.Len(t, chats, 2)

		chatsByID := make(map[uuid.UUID]codersdk.Chat, len(chats))
		for _, chat := range chats {
			require.NotNil(t, chat.DiffStatus)
			require.Equal(t, chat.ID, chat.DiffStatus.ChatID)
			chatsByID[chat.ID] = chat
		}

		withStatus, ok := chatsByID[chatWithStatus.ID]
		require.True(t, ok)
		require.NotNil(t, withStatus.DiffStatus.URL)
		require.Equal(t, "https://github.com/octocat/hello-world/pull/99", *withStatus.DiffStatus.URL)
		require.NotNil(t, withStatus.DiffStatus.PullRequestState)
		require.Equal(t, "open", *withStatus.DiffStatus.PullRequestState)
		require.True(t, withStatus.DiffStatus.ChangesRequested)
		require.EqualValues(t, 17, withStatus.DiffStatus.Additions)
		require.EqualValues(t, 4, withStatus.DiffStatus.Deletions)
		require.EqualValues(t, 3, withStatus.DiffStatus.ChangedFiles)
		require.NotNil(t, withStatus.DiffStatus.RefreshedAt)
		require.NotNil(t, withStatus.DiffStatus.StaleAt)

		withoutStatus, ok := chatsByID[chatWithoutStatus.ID]
		require.True(t, ok)
		require.Nil(t, withoutStatus.DiffStatus.URL)
		require.Nil(t, withoutStatus.DiffStatus.PullRequestState)
		require.False(t, withoutStatus.DiffStatus.ChangesRequested)
		require.EqualValues(t, 0, withoutStatus.DiffStatus.Additions)
		require.EqualValues(t, 0, withoutStatus.DiffStatus.Deletions)
		require.EqualValues(t, 0, withoutStatus.DiffStatus.ChangedFiles)
		require.Nil(t, withoutStatus.DiffStatus.RefreshedAt)
		require.Nil(t, withoutStatus.DiffStatus.StaleAt)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Message: "Test Chat"})
		require.NoError(t, err)

		result, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, result.Chat.ID)
		userMessage, ok := firstChatMessageByRole(result.Messages, "user")
		require.True(t, ok)
		require.Equal(t, "user", userMessage.Role)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Message: "Test Chat"})
		require.NoError(t, err)

		err = client.DeleteChat(ctx, chat.ID)
		require.NoError(t, err)

		// Verify it's deleted.
		_, err = client.GetChat(ctx, chat.ID)
		require.Error(t, err)
	})

	t.Run("CreateMessage", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{Message: "Test Chat"})
		require.NoError(t, err)

		before, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)

		messages, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Role:    "user",
			Content: json.RawMessage(`"Hello, AI!"`),
		})
		require.NoError(t, err)
		require.Len(t, messages, len(before.Messages)+1)
		require.Equal(t, "user", messages[len(messages)-1].Role)

		// Verify messages were saved.
		result, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, result.Messages, len(before.Messages)+1)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Use a random UUID that doesn't exist.
		randomID := uuid.New()
		_, err := client.GetChat(ctx, randomID)
		require.Error(t, err)
	})
}

func TestChatDiffStatus(t *testing.T) {
	t.Parallel()

	t.Run("NoPullRequestDetected", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Summarize the latest work.",
		})
		require.NoError(t, err)

		status, err := client.GetChatDiffStatus(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, status.ChatID)
		require.Nil(t, status.URL)
		require.Nil(t, status.PullRequestState)
		require.False(t, status.ChangesRequested)
		require.EqualValues(t, 0, status.Additions)
		require.EqualValues(t, 0, status.Deletions)
		require.EqualValues(t, 0, status.ChangedFiles)
		require.Nil(t, status.RefreshedAt)
		require.Nil(t, status.StaleAt)
	})

	t.Run("RefreshAndCache", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		var apiCallCount atomic.Int32
		githubServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			apiCallCount.Add(1)
			switch r.URL.Path {
			case "/repos/octocat/hello-world/pulls/42":
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`{
					"state": "open",
					"additions": 12,
					"deletions": 3,
					"changed_files": 4
				}`))
			case "/repos/octocat/hello-world/pulls/42/reviews":
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`[
					{
						"id": 10,
						"state": "CHANGES_REQUESTED",
						"user": { "login": "reviewer-one" }
					},
					{
						"id": 20,
						"state": "APPROVED",
						"user": { "login": "reviewer-two" }
					}
				]`))
			default:
				http.NotFound(rw, r)
			}
		}))
		t.Cleanup(githubServer.Close)

		githubURL, err := url.Parse(githubServer.URL)
		require.NoError(t, err)

		rewriteTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "api.github.com" {
				cloned := req.Clone(req.Context())
				cloned.URL = &url.URL{
					Scheme:   githubURL.Scheme,
					Host:     githubURL.Host,
					Path:     req.URL.Path,
					RawPath:  req.URL.RawPath,
					RawQuery: req.URL.RawQuery,
				}
				return http.DefaultTransport.RoundTrip(cloned)
			}
			return http.DefaultTransport.RoundTrip(req)
		})

		api.HTTPClient = &http.Client{
			Transport: rewriteTransport,
		}

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Please review the latest changes.",
		})
		require.NoError(t, err)

		_, err = api.Database.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:  chat.ID,
				Url:     sql.NullString{String: "https://github.com/octocat/hello-world/pull/42", Valid: true},
				StaleAt: time.Now().UTC().Add(-time.Minute),
			},
		)
		require.NoError(t, err)

		status, err := client.GetChatDiffStatus(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, status.ChatID)
		require.NotNil(t, status.URL)
		require.Equal(t, "https://github.com/octocat/hello-world/pull/42", *status.URL)
		require.NotNil(t, status.PullRequestState)
		require.Equal(t, "open", *status.PullRequestState)
		require.True(t, status.ChangesRequested)
		require.EqualValues(t, 12, status.Additions)
		require.EqualValues(t, 3, status.Deletions)
		require.EqualValues(t, 4, status.ChangedFiles)
		require.NotNil(t, status.RefreshedAt)
		require.NotNil(t, status.StaleAt)
		require.EqualValues(t, 2, apiCallCount.Load())

		_, err = client.GetChatDiffStatus(ctx, chat.ID)
		require.NoError(t, err)
		require.EqualValues(t, 2, apiCallCount.Load())
	})
}

func TestChatDiffContents(t *testing.T) {
	t.Parallel()

	t.Run("FromCachedPullRequestURL", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		const expectedDiff = "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")\n"

		githubServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/octocat/hello-world/pulls/42" {
				http.NotFound(rw, r)
				return
			}

			if strings.Contains(r.Header.Get("Accept"), "application/vnd.github.diff") {
				rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = rw.Write([]byte(expectedDiff))
				return
			}

			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{
				"state": "open",
				"additions": 1,
				"deletions": 0,
				"changed_files": 1
			}`))
		}))
		t.Cleanup(githubServer.Close)

		githubURL, err := url.Parse(githubServer.URL)
		require.NoError(t, err)

		rewriteTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "api.github.com" {
				cloned := req.Clone(req.Context())
				cloned.URL = &url.URL{
					Scheme:   githubURL.Scheme,
					Host:     githubURL.Host,
					Path:     req.URL.Path,
					RawPath:  req.URL.RawPath,
					RawQuery: req.URL.RawQuery,
				}
				return http.DefaultTransport.RoundTrip(cloned)
			}
			return http.DefaultTransport.RoundTrip(req)
		})

		api.HTTPClient = &http.Client{
			Transport: rewriteTransport,
		}

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Please inspect the latest pull request.",
		})
		require.NoError(t, err)

		_, err = api.Database.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:  chat.ID,
				Url:     sql.NullString{String: "https://github.com/octocat/hello-world/pull/42", Valid: true},
				StaleAt: time.Now().UTC().Add(-time.Minute),
			},
		)
		require.NoError(t, err)

		diff, err := client.GetChatDiffContents(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, diff.ChatID)
		require.NotNil(t, diff.PullRequestURL)
		require.Equal(t, "https://github.com/octocat/hello-world/pull/42", *diff.PullRequestURL)
		require.Equal(t, expectedDiff, diff.Diff)
	})
}

func TestCreateChat_SystemPromptUnauthorized(t *testing.T) {
	t.Parallel()

	ownerClient := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitShort)

	_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
		Message:      "Test chat",
		SystemPrompt: "Only admins should be able to set this.",
	})
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
}

func TestCreateChat_DefaultSystemPrompt(t *testing.T) {
	t.Parallel()

	defaultSystemPrompt := "Use explicit step-by-step planning."
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: coderdtest.DeploymentValues(t, func(cfg *codersdk.DeploymentValues) {
			cfg.AI.Chat.SystemPrompt = serpent.String(defaultSystemPrompt)
		}),
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Input: &codersdk.ChatInput{
			Parts: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "Build a test feature."},
			},
		},
	})
	require.NoError(t, err)

	result, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(result.Messages), 2)
	require.Equal(t, "system", result.Messages[0].Role)

	var gotSystemPrompt string
	require.NoError(t, json.Unmarshal(result.Messages[0].Content, &gotSystemPrompt))
	require.Equal(t, defaultSystemPrompt, gotSystemPrompt)
}

func TestCreateChat_StructuredInput(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: coderdtest.DeploymentValues(t, func(cfg *codersdk.DeploymentValues) {
			cfg.AI.Chat.SystemPrompt = serpent.String("")
		}),
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Input: &codersdk.ChatInput{
			Parts: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "Plan rollout"},
				{Type: codersdk.ChatInputPartTypeText, Text: "for chat defaults"},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Plan rollout for chat defaults", chat.Title)

	result, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)

	userMessage, ok := firstChatMessageByRole(result.Messages, "user")
	require.True(t, ok)

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(userMessage.Content, &parts))
	require.Len(t, parts, 2)
	require.Equal(t, "text", parts[0].Type)
	require.Equal(t, "Plan rollout", parts[0].Text)
	require.Equal(t, "text", parts[1].Type)
	require.Equal(t, "for chat defaults", parts[1].Text)
}

func TestCreateChat_StructuredInputUnsupportedType(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Input: &codersdk.ChatInput{
			Parts: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartType("file"), Text: "README.md"},
			},
		},
	})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
}

func TestCreateChat_SystemPromptAuthorized(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: coderdtest.DeploymentValues(t, func(cfg *codersdk.DeploymentValues) {
			cfg.AI.Chat.SystemPrompt = serpent.String("Use the deployment default.")
		}),
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	systemPrompt := "You are a strict coding assistant."
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Input: &codersdk.ChatInput{
			Parts: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "Build a test feature."},
			},
		},
		SystemPrompt: systemPrompt,
	})
	require.NoError(t, err)

	result, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, result.Messages, 2)
	require.Equal(t, "system", result.Messages[0].Role)
	require.Equal(t, "user", result.Messages[1].Role)

	var gotSystemPrompt string
	require.NoError(t, json.Unmarshal(result.Messages[0].Content, &gotSystemPrompt))
	require.Equal(t, systemPrompt, gotSystemPrompt)
}

func TestChatModels_NoEnabledModels(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	catalog, err := client.ListChatModels(ctx)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatModelProvider{
		{
			Provider:          "openai",
			Available:         false,
			UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
			Models:            []codersdk.ChatModel{},
		},
		{
			Provider:          "anthropic",
			Available:         false,
			UnavailableReason: codersdk.ChatModelProviderUnavailableMissingAPIKey,
			Models:            []codersdk.ChatModel{},
		},
	}, catalog.Providers)
}

func TestChatModels_NoEnabledModelsUsesMergedProviderKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai")
	t.Setenv("ANTHROPIC_API_KEY", "")

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "anthropic",
		APIKey:   "provider-anthropic",
	})
	require.NoError(t, err)

	catalog, err := client.ListChatModels(ctx)
	require.NoError(t, err)
	require.Equal(t, []codersdk.ChatModelProvider{
		{
			Provider:  "openai",
			Available: true,
			Models:    []codersdk.ChatModel{},
		},
		{
			Provider:  "anthropic",
			Available: true,
			Models:    []codersdk.ChatModel{},
		},
	}, catalog.Providers)
}

func firstChatMessageByRole(messages []codersdk.ChatMessage, role string) (codersdk.ChatMessage, bool) {
	for _, message := range messages {
		if message.Role == role {
			return message, true
		}
	}
	return codersdk.ChatMessage{}, false
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

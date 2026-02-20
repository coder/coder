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
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
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

	t.Run("GetIncludesMessageUsage", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Test usage fields",
		})
		require.NoError(t, err)

		_, err = api.Database.InsertChatMessage(
			dbauthz.AsSystemRestricted(ctx),
			database.InsertChatMessageParams{
				ChatID:  chat.ID,
				Role:    "assistant",
				Content: pqtype.NullRawMessage{RawMessage: json.RawMessage(`"usage response"`), Valid: true},
				Hidden:  false,
				InputTokens: sql.NullInt64{
					Int64: 120,
					Valid: true,
				},
				OutputTokens: sql.NullInt64{
					Int64: 45,
					Valid: true,
				},
				ReasoningTokens: sql.NullInt64{
					Int64: 12,
					Valid: true,
				},
				CacheCreationTokens: sql.NullInt64{
					Int64: 3,
					Valid: true,
				},
				CacheReadTokens: sql.NullInt64{
					Int64: 9,
					Valid: true,
				},
				TotalTokens: sql.NullInt64{
					Int64: 165,
					Valid: true,
				},
				ContextLimit: sql.NullInt64{
					Int64: 200000,
					Valid: true,
				},
			},
		)
		require.NoError(t, err)

		result, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)

		assistantMessage, ok := firstChatMessageByRole(result.Messages, "assistant")
		require.True(t, ok)
		require.NotNil(t, assistantMessage.InputTokens)
		require.Equal(t, int64(120), *assistantMessage.InputTokens)
		require.NotNil(t, assistantMessage.OutputTokens)
		require.Equal(t, int64(45), *assistantMessage.OutputTokens)
		require.NotNil(t, assistantMessage.ReasoningTokens)
		require.Equal(t, int64(12), *assistantMessage.ReasoningTokens)
		require.NotNil(t, assistantMessage.CacheCreationTokens)
		require.Equal(t, int64(3), *assistantMessage.CacheCreationTokens)
		require.NotNil(t, assistantMessage.CacheReadTokens)
		require.Equal(t, int64(9), *assistantMessage.CacheReadTokens)
		require.NotNil(t, assistantMessage.TotalTokens)
		require.Equal(t, int64(165), *assistantMessage.TotalTokens)
		require.NotNil(t, assistantMessage.ContextLimit)
		require.Equal(t, int64(200000), *assistantMessage.ContextLimit)

		userMessage, ok := firstChatMessageByRole(result.Messages, "user")
		require.True(t, ok)
		require.Nil(t, userMessage.InputTokens)
		require.Nil(t, userMessage.OutputTokens)
		require.Nil(t, userMessage.ReasoningTokens)
		require.Nil(t, userMessage.CacheCreationTokens)
		require.Nil(t, userMessage.CacheReadTokens)
		require.Nil(t, userMessage.TotalTokens)
		require.Nil(t, userMessage.ContextLimit)
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

	t.Run("BranchRefWithoutPullRequest", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		githubServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/octocat/hello-world/pulls" {
				http.NotFound(rw, r)
				return
			}
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`[]`))
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
		api.HTTPClient = &http.Client{Transport: rewriteTransport}

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Show me branch-only diff status.",
		})
		require.NoError(t, err)

		_, err = api.Database.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				GitBranch:       "feature/branch-only",
				GitRemoteOrigin: "https://github.com/octocat/hello-world.git",
				StaleAt:         time.Now().UTC().Add(-time.Minute),
			},
		)
		require.NoError(t, err)

		status, err := client.GetChatDiffStatus(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, status.ChatID)
		require.NotNil(t, status.URL)
		require.Equal(t, "https://github.com/octocat/hello-world/tree/feature%2Fbranch-only", *status.URL)
		require.Nil(t, status.PullRequestState)
		require.False(t, status.ChangesRequested)
		require.EqualValues(t, 0, status.Additions)
		require.EqualValues(t, 0, status.Deletions)
		require.EqualValues(t, 0, status.ChangedFiles)
		require.Nil(t, status.RefreshedAt)
		require.NotNil(t, status.StaleAt)
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

	t.Run("FromBranchReferenceWithoutPullRequest", func(t *testing.T) {
		t.Parallel()
		client, _, api := coderdtest.NewWithAPI(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		const expectedDiff = "diff --git a/main.go b/main.go\n+fmt.Println(\"branch\")\n"

		githubServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/repos/octocat/hello-world/pulls":
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`[]`))
				return
			case r.URL.Path == "/repos/octocat/hello-world":
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`{"default_branch":"main"}`))
				return
			case strings.HasPrefix(r.URL.Path, "/repos/octocat/hello-world/compare/"):
				if strings.Contains(r.Header.Get("Accept"), "application/vnd.github.diff") {
					rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
					_, _ = rw.Write([]byte(expectedDiff))
					return
				}
			}

			http.NotFound(rw, r)
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
			Message: "Please inspect branch changes.",
		})
		require.NoError(t, err)

		_, err = api.Database.UpsertChatDiffStatusReference(
			dbauthz.AsSystemRestricted(ctx),
			database.UpsertChatDiffStatusReferenceParams{
				ChatID:          chat.ID,
				GitBranch:       "feature/branch-only",
				GitRemoteOrigin: "https://github.com/octocat/hello-world.git",
				StaleAt:         time.Now().UTC().Add(-time.Minute),
			},
		)
		require.NoError(t, err)

		status, err := client.GetChatDiffStatus(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, status.URL)
		require.Equal(t, "https://github.com/octocat/hello-world/tree/feature%2Fbranch-only", *status.URL)
		require.Nil(t, status.PullRequestState)

		diff, err := client.GetChatDiffContents(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, diff.ChatID)
		require.NotNil(t, diff.Provider)
		require.Equal(t, "github", *diff.Provider)
		require.NotNil(t, diff.RemoteOrigin)
		require.Equal(t, "https://github.com/octocat/hello-world", *diff.RemoteOrigin)
		require.NotNil(t, diff.Branch)
		require.Equal(t, "feature/branch-only", *diff.Branch)
		require.Nil(t, diff.PullRequestURL)
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
	require.Len(t, userMessage.Parts, 2)
	require.Equal(t, codersdk.ChatMessagePartTypeText, userMessage.Parts[0].Type)
	require.Equal(t, "Plan rollout", userMessage.Parts[0].Text)
	require.Equal(t, codersdk.ChatMessagePartTypeText, userMessage.Parts[1].Type)
	require.Equal(t, "for chat defaults", userMessage.Parts[1].Text)
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

func TestCreateChat_WorkspaceSelectionAuthorization(t *testing.T) {
	t.Parallel()

	ownerClient := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	workspace, workspaceAgentID := createWorkspaceWithAgent(t, ownerClient, owner.OrganizationID)

	t.Run("WorkspaceIDAuthorized", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		workspaceID := workspace.ID
		chat, err := ownerClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:     "Use my workspace.",
			WorkspaceID: &workspaceID,
		})
		require.NoError(t, err)
		require.NotNil(t, chat.WorkspaceID)
		require.Equal(t, workspaceID, *chat.WorkspaceID)
	})

	t.Run("WorkspaceIDUnauthorized", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		workspaceID := workspace.ID
		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:     "Try to use someone else's workspace.",
			WorkspaceID: &workspaceID,
		})
		require.Error(t, err)

		sdkErr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "Workspace not found or you do not have access to this resource", sdkErr.Message)
	})

	t.Run("WorkspaceAgentIDAuthorized", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		workspaceID := workspace.ID
		agentID := workspaceAgentID
		chat, err := ownerClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:          "Use my workspace agent.",
			WorkspaceID:      &workspaceID,
			WorkspaceAgentID: &agentID,
		})
		require.NoError(t, err)
		require.NotNil(t, chat.WorkspaceAgentID)
		require.Equal(t, agentID, *chat.WorkspaceAgentID)
	})

	t.Run("WorkspaceAgentIDUnauthorized", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)
		agentID := workspaceAgentID
		_, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:          "Try to use someone else's workspace agent.",
			WorkspaceAgentID: &agentID,
		})
		require.Error(t, err)

		sdkErr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "Workspace agent not found or you do not have access to this resource", sdkErr.Message)
	})
}

func TestCreateChat_LocalWorkspaceBootstrap(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
		DeploymentValues: coderdtest.DeploymentValues(
			t,
			func(cfg *codersdk.DeploymentValues) {
				cfg.AI.Chat.LocalWorkspace = serpent.Bool(true)
			},
		),
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Message:       "Bootstrap local workspace chat.",
		WorkspaceMode: codersdk.ChatWorkspaceModeLocal,
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatWorkspaceModeLocal, chat.WorkspaceMode)
	require.NotNil(t, chat.WorkspaceID)
	require.NotNil(t, chat.WorkspaceAgentID)

	got, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatWorkspaceModeLocal, got.Chat.WorkspaceMode)
	require.NotNil(t, got.Chat.WorkspaceID)
	require.NotNil(t, got.Chat.WorkspaceAgentID)
	require.Equal(t, *chat.WorkspaceID, *got.Chat.WorkspaceID)
	require.Equal(t, *chat.WorkspaceAgentID, *got.Chat.WorkspaceAgentID)
}

func TestCreateChat_HierarchyMetadata(t *testing.T) {
	t.Parallel()

	t.Run("CreateRootChat", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		rootChat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Create a root chat.",
		})
		require.NoError(t, err)
		require.Nil(t, rootChat.ParentChatID)
		require.NotNil(t, rootChat.RootChatID)
		require.Equal(t, rootChat.ID, *rootChat.RootChatID)

		got, err := client.GetChat(ctx, rootChat.ID)
		require.NoError(t, err)
		require.Nil(t, got.Chat.ParentChatID)
		require.NotNil(t, got.Chat.RootChatID)
		require.Equal(t, rootChat.ID, *got.Chat.RootChatID)

		chats, err := client.ListChats(ctx)
		require.NoError(t, err)
		require.Len(t, chats, 1)
		require.Nil(t, chats[0].ParentChatID)
		require.NotNil(t, chats[0].RootChatID)
		require.Equal(t, rootChat.ID, *chats[0].RootChatID)
	})

	t.Run("CreateChildChatWithValidParent", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		parent, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Create a parent chat.",
		})
		require.NoError(t, err)

		parentID := parent.ID
		child, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:      "Create a child chat.",
			ParentChatID: &parentID,
		})
		require.NoError(t, err)
		require.NotNil(t, child.ParentChatID)
		require.Equal(t, parent.ID, *child.ParentChatID)
		require.NotNil(t, child.RootChatID)
		require.Equal(t, parent.ID, *child.RootChatID)

		gotChild, err := client.GetChat(ctx, child.ID)
		require.NoError(t, err)
		require.NotNil(t, gotChild.Chat.ParentChatID)
		require.Equal(t, parent.ID, *gotChild.Chat.ParentChatID)
		require.NotNil(t, gotChild.Chat.RootChatID)
		require.Equal(t, parent.ID, *gotChild.Chat.RootChatID)

		chats, err := client.ListChats(ctx)
		require.NoError(t, err)
		require.Len(t, chats, 2)

		byID := make(map[uuid.UUID]codersdk.Chat, len(chats))
		for _, chat := range chats {
			byID[chat.ID] = chat
		}

		require.Contains(t, byID, parent.ID)
		require.Contains(t, byID, child.ID)
		require.Nil(t, byID[parent.ID].ParentChatID)
		require.NotNil(t, byID[parent.ID].RootChatID)
		require.Equal(t, parent.ID, *byID[parent.ID].RootChatID)
		require.NotNil(t, byID[child.ID].ParentChatID)
		require.Equal(t, parent.ID, *byID[child.ID].ParentChatID)
		require.NotNil(t, byID[child.ID].RootChatID)
		require.Equal(t, parent.ID, *byID[child.ID].RootChatID)
	})

	t.Run("RejectParentFromAnotherOwner", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		parent, err := ownerClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Message: "Owner parent chat.",
		})
		require.NoError(t, err)

		parentID := parent.ID
		_, err = memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:      "Attempt child chat.",
			ParentChatID: &parentID,
		})
		require.Error(t, err)

		sdkErr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "Parent chat not found or you do not have access to this resource", sdkErr.Message)
	})

	t.Run("RejectWorkspaceMismatch", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		parentWorkspace, parentWorkspaceAgentID := createWorkspaceWithAgent(t, client, user.OrganizationID)
		otherWorkspace, _ := createWorkspaceWithAgent(t, client, user.OrganizationID)

		parentWorkspaceID := parentWorkspace.ID
		parentAgentID := parentWorkspaceAgentID
		parent, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:          "Parent chat with workspace.",
			WorkspaceID:      &parentWorkspaceID,
			WorkspaceAgentID: &parentAgentID,
		})
		require.NoError(t, err)

		parentID := parent.ID
		otherWorkspaceID := otherWorkspace.ID
		_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			Message:      "Child chat with mismatched workspace.",
			ParentChatID: &parentID,
			WorkspaceID:  &otherWorkspaceID,
		})
		require.Error(t, err)

		sdkErr := coderdtest.SDKError(t, err)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "Child chat workspace must match parent chat workspace.", sdkErr.Message)
	})
}

func TestChatModels_NoEnabledModels(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	catalog, err := client.ListChatModels(ctx)
	require.NoError(t, err)
	require.Len(t, catalog.Providers, 8)
	for _, provider := range catalog.Providers {
		require.False(t, provider.Available)
		require.Equal(t, codersdk.ChatModelProviderUnavailableMissingAPIKey, provider.UnavailableReason)
		require.Empty(t, provider.Models)
	}
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
	require.Len(t, catalog.Providers, 8)

	byProvider := make(map[string]codersdk.ChatModelProvider, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		byProvider[provider.Provider] = provider
	}

	require.True(t, byProvider["openai"].Available)
	require.Empty(t, byProvider["openai"].UnavailableReason)
	require.Empty(t, byProvider["openai"].Models)

	require.True(t, byProvider["anthropic"].Available)
	require.Empty(t, byProvider["anthropic"].UnavailableReason)
	require.Empty(t, byProvider["anthropic"].Models)

	for _, provider := range []string{
		"azure",
		"bedrock",
		"google",
		"openai-compat",
		"openrouter",
		"vercel",
	} {
		entry := byProvider[provider]
		require.False(t, entry.Available)
		require.Equal(t, codersdk.ChatModelProviderUnavailableMissingAPIKey, entry.UnavailableReason)
		require.Empty(t, entry.Models)
	}
}

func TestChatProviders(t *testing.T) {
	t.Parallel()

	t.Run("CreateAdminAllowed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "openai-key",
		})
		require.NoError(t, err)
		require.Equal(t, "openai", provider.Provider)
		require.Equal(t, "OpenAI", provider.DisplayName)
		require.True(t, provider.Enabled)
		require.True(t, provider.HasAPIKey)
	})

	t.Run("CreateNonAdminForbidden", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := memberClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	})

	t.Run("CreateConflict", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		_, err = client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key-2",
		})
		require.Error(t, err)
		require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	})

	t.Run("UpdateAdminAllowed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "openai-key",
		})
		require.NoError(t, err)

		enabled := false
		updated, err := client.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "OpenAI Updated",
			Enabled:     &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, provider.ID, updated.ID)
		require.Equal(t, "OpenAI Updated", updated.DisplayName)
		require.False(t, updated.Enabled)
		require.True(t, updated.HasAPIKey)
	})

	t.Run("UpdateNonAdminForbidden", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider, err := ownerClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		enabled := false
		_, err = memberClient.UpdateChatProvider(ctx, provider.ID, codersdk.UpdateChatProviderConfigRequest{
			DisplayName: "Should Fail",
			Enabled:     &enabled,
		})
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	})

	t.Run("DeleteAdminAllowed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		err = client.DeleteChatProvider(ctx, provider.ID)
		require.NoError(t, err)

		providers, err := client.ListChatProviders(ctx)
		require.NoError(t, err)
		require.Len(t, providers, 8)
		for _, provider := range providers {
			require.NotEqual(t, codersdk.ChatProviderConfigSourceDatabase, provider.Source)
		}
	})

	t.Run("DeleteNonAdminForbidden", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider, err := ownerClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		err = memberClient.DeleteChatProvider(ctx, provider.ID)
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	})

}

func TestChatProviders_ListIncludesSupportedProvidersAndEnvPresets(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)

	providers, err := client.ListChatProviders(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 8)

	byProvider := make(map[string]codersdk.ChatProviderConfig, len(providers))
	for _, provider := range providers {
		byProvider[provider.Provider] = provider
	}

	openai := byProvider["openai"]
	require.Equal(t, codersdk.ChatProviderConfigSourceEnvPreset, openai.Source)
	require.Equal(t, uuid.Nil, openai.ID)
	require.True(t, openai.Enabled)
	require.True(t, openai.HasAPIKey)

	anthropic := byProvider["anthropic"]
	require.Equal(t, codersdk.ChatProviderConfigSourceSupported, anthropic.Source)
	require.Equal(t, uuid.Nil, anthropic.ID)
	require.False(t, anthropic.Enabled)
	require.False(t, anthropic.HasAPIKey)

	_, err = client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openrouter",
		APIKey:   "openrouter-key",
	})
	require.NoError(t, err)

	providers, err = client.ListChatProviders(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 8)
	byProvider = make(map[string]codersdk.ChatProviderConfig, len(providers))
	for _, provider := range providers {
		byProvider[provider.Provider] = provider
	}

	openrouter := byProvider["openrouter"]
	require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, openrouter.Source)
	require.NotEqual(t, uuid.Nil, openrouter.ID)
	require.True(t, openrouter.Enabled)
	require.True(t, openrouter.HasAPIKey)
}

func TestChatModelConfigs(t *testing.T) {
	t.Parallel()

	t.Run("CreateAdminAllowed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		config, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:    "openai",
			Model:       "gpt-4.1",
			DisplayName: "GPT 4.1",
		})
		require.NoError(t, err)
		require.Equal(t, "openai", config.Provider)
		require.Equal(t, "gpt-4.1", config.Model)
		require.Equal(t, "GPT 4.1", config.DisplayName)
		require.True(t, config.Enabled)
	})

	t.Run("CreateNonAdminForbidden", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := memberClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider: "openai",
			Model:    "gpt-4.1",
		})
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	})

	t.Run("UpdateAdminAllowed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		config, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:    "openai",
			Model:       "gpt-4.1",
			DisplayName: "GPT 4.1",
		})
		require.NoError(t, err)

		enabled := false
		updated, err := client.UpdateChatModelConfig(ctx, config.ID, codersdk.UpdateChatModelConfigRequest{
			Model:       "gpt-4.1-mini",
			DisplayName: "GPT 4.1 Mini",
			Enabled:     &enabled,
		})
		require.NoError(t, err)
		require.Equal(t, config.ID, updated.ID)
		require.Equal(t, "openai", updated.Provider)
		require.Equal(t, "gpt-4.1-mini", updated.Model)
		require.Equal(t, "GPT 4.1 Mini", updated.DisplayName)
		require.False(t, updated.Enabled)
	})

	t.Run("UpdateNonAdminForbidden", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := ownerClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		config, err := ownerClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider: "openai",
			Model:    "gpt-4.1",
		})
		require.NoError(t, err)

		_, err = memberClient.UpdateChatModelConfig(ctx, config.ID, codersdk.UpdateChatModelConfigRequest{
			DisplayName: "Should Fail",
		})
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	})

	t.Run("DeleteAdminAllowed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := client.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		config, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider: "openai",
			Model:    "gpt-4.1",
		})
		require.NoError(t, err)

		err = client.DeleteChatModelConfig(ctx, config.ID)
		require.NoError(t, err)

		configs, err := client.ListChatModelConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, configs, 0)
	})

	t.Run("DeleteNonAdminForbidden", func(t *testing.T) {
		t.Parallel()

		ownerClient := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := ownerClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider: "openai",
			APIKey:   "openai-key",
		})
		require.NoError(t, err)

		config, err := ownerClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider: "openai",
			Model:    "gpt-4.1",
		})
		require.NoError(t, err)

		err = memberClient.DeleteChatModelConfig(ctx, config.ID)
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	})
}

func createWorkspaceWithAgent(
	t *testing.T,
	client *codersdk.Client,
	organizationID uuid.UUID,
) (codersdk.Workspace, uuid.UUID) {
	t.Helper()

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, organizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, organizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	workspace.LatestBuild = coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.NotEmpty(t, workspace.LatestBuild.Resources)
	require.NotEmpty(t, workspace.LatestBuild.Resources[0].Agents)

	return workspace, workspace.LatestBuild.Resources[0].Agents[0].ID
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

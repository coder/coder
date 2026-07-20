package slackd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeProposalChat records proposal-outcome chat interactions.
type fakeProposalChat struct {
	mu     sync.Mutex
	sends  []chatd.SendMessageOptions
	adds   []uuid.UUID
	addErr error

	sent chan chatd.SendMessageOptions
}

func newFakeProposalChat() *fakeProposalChat {
	return &fakeProposalChat{sent: make(chan chatd.SendMessageOptions, 16)}
}

func (f *fakeProposalChat) SendMessage(_ context.Context, opts chatd.SendMessageOptions) (chatd.SendMessageResult, error) {
	f.mu.Lock()
	f.sends = append(f.sends, opts)
	f.mu.Unlock()
	f.sent <- opts
	return chatd.SendMessageResult{}, nil
}

func (f *fakeProposalChat) AddChatMCPServerID(_ context.Context, chatID, serverID uuid.UUID) (database.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return database.Chat{}, f.addErr
	}
	f.adds = append(f.adds, serverID)
	return database.Chat{ID: chatID}, nil
}

func (f *fakeProposalChat) snapshot() (sends []chatd.SendMessageOptions, adds []uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]chatd.SendMessageOptions(nil), f.sends...),
		append([]uuid.UUID(nil), f.adds...)
}

type proposalsTestDeps struct {
	db     database.Store
	sqlDB  *sql.DB
	chat   *fakeProposalChat
	webAPI *fakeWebAPI
	api    *ProposalsAPI

	ownerID    uuid.UUID
	chatRow    database.Chat
	slackUser  string
	otherUser  database.User
	proposalID uuid.UUID
}

// newProposalsTest seeds a chat owner (the requester, linked to Slack
// user USENDER), another user, a chat, and a pending proposal, and
// returns a ProposalsAPI wired to fakes.
func newProposalsTest(t *testing.T, req chattool.MCPServerProposalRequest) *proposalsTestDeps {
	t.Helper()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	owner := dbgen.User(t, db, database.User{})
	other := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chatRow := dbgen.Chat(t, db, database.Chat{
		OwnerID:           owner.ID,
		OrganizationID:    org.ID,
		LastModelConfigID: modelConfig.ID,
	})

	// The requesting user is linked to Slack user USENDER; unlinked
	// Slack users resolve to the fallback owner (otherUser), so their
	// Cancel clicks never match the requester.
	const providerID = "slack-test"
	linkSlackIdentity(t, db, providerID, owner.ID, "USENDER")

	requestJSON, err := json.Marshal(req)
	require.NoError(t, err)
	proposal, err := db.InsertMCPServerProposal(ctx, database.InsertMCPServerProposalParams{
		ID:          uuid.New(),
		ChatID:      chatRow.ID,
		RequesterID: owner.ID,
		Channel:     "C123",
		ThreadTs:    "1700000000.000100",
		MessageTs:   "1700000002.000300",
		Request:     requestJSON,
	})
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fakeChat := newFakeProposalChat()
	webAPI := &fakeWebAPI{botUID: "UBOT"}
	api, err := NewProposalsAPI(ProposalsAPIOptions{
		Logger:           logger,
		Database:         db,
		Chat:             fakeChat,
		WebAPI:           webAPI,
		AccessURL:        testutil.MustURL(t, "https://coder.example.com"),
		ResolveSlackUser: NewSlackUserResolver(logger, db, providerID, other.ID),
		BackgroundCtx:    context.Background(),
		AuthPollInterval: 10 * time.Millisecond,
		AuthPollTimeout:  2 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(api.Close)

	return &proposalsTestDeps{
		db:         db,
		sqlDB:      sqlDB,
		chat:       fakeChat,
		webAPI:     webAPI,
		api:        api,
		ownerID:    owner.ID,
		chatRow:    chatRow,
		slackUser:  "USENDER",
		otherUser:  other,
		proposalID: proposal.ID,
	}
}

// oauthManualRequest is a proposal request with explicit oauth2
// credentials, so accepting does not need discovery.
func oauthManualRequest() chattool.MCPServerProposalRequest {
	return chattool.MCPServerProposalRequest{
		DisplayName:    "Linear",
		Slug:           "linear",
		URL:            "https://mcp.linear.app/mcp",
		Transport:      "streamable_http",
		AuthType:       "oauth2",
		OAuth2ClientID: "client-id",
		OAuth2AuthURL:  "https://auth.example.com/authorize",
		OAuth2TokenURL: "https://auth.example.com/token",
	}
}

func noAuthRequest() chattool.MCPServerProposalRequest {
	return chattool.MCPServerProposalRequest{
		DisplayName: "Docs",
		Slug:        "docs",
		URL:         "https://mcp.example.com",
		Transport:   "streamable_http",
		AuthType:    "none",
	}
}

// doProposal performs one handler invocation as userID and returns the
// recorder.
func doProposal(t *testing.T, handler func(http.ResponseWriter, *http.Request, uuid.UUID), method string, proposalID, userID uuid.UUID) *httptest.ResponseRecorder {
	return doProposalBody(t, handler, method, proposalID, userID, nil)
}

func doProposalBody(t *testing.T, handler func(http.ResponseWriter, *http.Request, uuid.UUID), method string, proposalID, userID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var bodyReader *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = strings.NewReader(string(data))
	} else {
		bodyReader = strings.NewReader("")
	}
	r := httptest.NewRequest(method, "/", bodyReader)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("mcpProposal", proposalID.String())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	handler(rec, r, userID)
	return rec
}

func TestMCPProposalGet(t *testing.T) {
	t.Parallel()

	t.Run("PendingHidesSecrets", func(t *testing.T) {
		t.Parallel()
		req := oauthManualRequest()
		req.OAuth2ClientSecret = "super-secret"
		req.APIKeyValue = "also-secret"
		req.CustomHeaders = map[string]string{"X-Secret": "header-secret"}
		deps := newProposalsTest(t, req)

		rec := doProposal(t, deps.api.getProposal, http.MethodGet, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)

		body := rec.Body.String()
		require.NotContains(t, body, "super-secret")
		require.NotContains(t, body, "also-secret")
		require.NotContains(t, body, "header-secret")

		var proposal codersdk.MCPServerProposal
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &proposal))
		require.Equal(t, codersdk.MCPServerProposalStatusPending, proposal.Status)
		require.Equal(t, "Linear", proposal.DisplayName)
		require.Equal(t, "oauth2", proposal.AuthType)
		require.True(t, proposal.HasOAuth2ClientCredentials)
		require.True(t, proposal.HasAPIKey)
		require.True(t, proposal.HasCustomHeaders)
		require.Equal(t, uuid.Nil, proposal.MCPServerConfigID)
	})

	t.Run("PendingReturnsOnlySecretPlaceholders", func(t *testing.T) {
		t.Parallel()
		req := oauthManualRequest()
		req.OAuth2ClientSecretPlaceholder = "Paste the OAuth2 client secret"
		req.APIKeyValuePlaceholder = "Paste the API key"
		req.CustomHeaderPlaceholders = map[string]string{"X-Secret": "Paste the header value"}
		req.Instructions = "Create the credentials in the service settings, then paste them below."
		deps := newProposalsTest(t, req)

		rec := doProposal(t, deps.api.getProposal, http.MethodGet, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var proposal codersdk.MCPServerProposal
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &proposal))
		require.Equal(t, "Paste the OAuth2 client secret", proposal.SecretPlaceholders.OAuth2ClientSecret)
		require.Equal(t, "Paste the API key", proposal.SecretPlaceholders.APIKeyValue)
		require.Equal(t, map[string]string{"X-Secret": "Paste the header value"}, proposal.SecretPlaceholders.CustomHeaders)
		require.Equal(t, req.Instructions, proposal.Instructions)
		require.NotContains(t, rec.Body.String(), "super-secret")
	})

	t.Run("Accepted", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, oauthManualRequest())

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doProposal(t, deps.api.getProposal, http.MethodGet, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var proposal codersdk.MCPServerProposal
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &proposal))
		require.Equal(t, codersdk.MCPServerProposalStatusAccepted, proposal.Status)
		require.NotEqual(t, uuid.Nil, proposal.MCPServerConfigID)
		require.False(t, proposal.Authenticated)
		require.Contains(t, proposal.ConnectURL, proposal.MCPServerConfigID.String())
		require.NotContains(t, rec.Body.String(), "client-id")
	})

	t.Run("WrongUserForbiddenWithoutIdentity", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())

		rec := doProposal(t, deps.api.getProposal, http.MethodGet, deps.proposalID, deps.otherUser.ID)
		require.Equal(t, http.StatusForbidden, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, "Another user must authorize")
		// The response must not disclose who has to authorize it.
		require.NotContains(t, body, deps.ownerID.String())
		require.NotContains(t, body, deps.slackUser)
	})

	t.Run("Missing", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())

		rec := doProposal(t, deps.api.getProposal, http.MethodGet, uuid.New(), deps.ownerID)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), "expired or was already handled")
	})

	t.Run("Rejected", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		rec := doProposal(t, deps.api.rejectProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusNoContent, rec.Code)

		rec = doProposal(t, deps.api.getProposal, http.MethodGet, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), "expired or was already handled")
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		expireProposal(t, deps.sqlDB, deps.proposalID)

		rec := doProposal(t, deps.api.getProposal, http.MethodGet, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), "expired or was already handled")
	})
}

// expireProposal backdates the proposal past the TTL.
func expireProposal(t *testing.T, sqlDB *sql.DB, proposalID uuid.UUID) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := sqlDB.ExecContext(ctx,
		"UPDATE mcp_server_proposals SET created_at = created_at - INTERVAL '25 hours' WHERE id = $1",
		proposalID)
	require.NoError(t, err)
}

func TestMCPProposalAccept(t *testing.T) {
	t.Parallel()

	t.Run("OAuth2HappyPath", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, oauthManualRequest())

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.NotEqual(t, uuid.Nil, resp.MCPServerConfigID)
		require.False(t, resp.Authenticated)
		require.Equal(t,
			"https://coder.example.com/api/experimental/mcp/servers/"+resp.MCPServerConfigID.String()+"/oauth2/connect",
			resp.ConnectURL)

		// The config is personal-scoped to the chat owner and carries
		// the manual oauth2 credentials.
		ctx := testutil.Context(t, testutil.WaitLong)
		config, err := deps.db.GetMCPServerConfigByID(ctx, resp.MCPServerConfigID)
		require.NoError(t, err)
		require.True(t, config.OwnerID.Valid)
		require.Equal(t, deps.ownerID, config.OwnerID.UUID)
		require.Equal(t, "client-id", config.OAuth2ClientID)
		require.True(t, config.Enabled)

		// The server was enabled for the proposing chat and the card
		// was replaced in place (pending "finishing authentication").
		_, adds := deps.chat.snapshot()
		require.Equal(t, []uuid.UUID{resp.MCPServerConfigID}, adds)
		updates := deps.webAPI.updates()
		require.Len(t, updates, 1)
		require.Equal(t, "C123", updates[0].Channel)
		require.Equal(t, "1700000002.000300", updates[0].TS)

		// No system message yet: the auth poll reports the outcome.
		// (The poll deadline has not been reached in this test.)
		sends, _ := deps.chat.snapshot()
		require.Empty(t, sends)

		// The proposal row records the accepted state.
		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "accepted", proposal.Status)
		require.Equal(t, resp.MCPServerConfigID, proposal.MCPServerConfigID.UUID)
		require.True(t, proposal.AcceptedAt.Valid)
	})

	t.Run("UserSuppliedCustomHeaderSecrets", func(t *testing.T) {
		t.Parallel()
		req := chattool.MCPServerProposalRequest{
			DisplayName: "Headers",
			Slug:        "headers-" + uuid.NewString(),
			URL:         "https://headers.example.com/mcp",
			Transport:   "streamable_http",
			AuthType:    "custom_headers",
			CustomHeaders: map[string]string{
				"X-Static": "agent-secret",
			},
			CustomHeaderPlaceholders: map[string]string{
				"X-API-Key": "Paste your API key",
				"X-Account": "Paste your account token",
			},
		}
		deps := newProposalsTest(t, req)
		values := codersdk.AcceptMCPServerProposalRequest{CustomHeaders: map[string]string{
			"X-API-Key": "user-api-key",
			"X-Account": "user-account-token",
		}}

		rec := doProposalBody(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID, values)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		ctx := testutil.Context(t, testutil.WaitLong)
		config, err := deps.db.GetMCPServerConfigByID(ctx, resp.MCPServerConfigID)
		require.NoError(t, err)
		var headers map[string]string
		require.NoError(t, json.Unmarshal([]byte(config.CustomHeaders), &headers))
		require.Equal(t, map[string]string{
			"X-Static":  "agent-secret",
			"X-API-Key": "user-api-key",
			"X-Account": "user-account-token",
		}, headers)

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.NotContains(t, string(proposal.Request), "user-api-key")
		require.NotContains(t, string(proposal.Request), "user-account-token")
	})

	t.Run("PlaceholderValidationKeepsProposalPending", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			values codersdk.AcceptMCPServerProposalRequest
		}{
			{name: "Missing"},
			{name: "Undeclared", values: codersdk.AcceptMCPServerProposalRequest{OAuth2ClientSecret: "unexpected"}},
			{name: "Blank", values: codersdk.AcceptMCPServerProposalRequest{APIKeyValue: "   "}},
			{name: "UnexpectedCustomHeader", values: codersdk.AcceptMCPServerProposalRequest{
				APIKeyValue: "secret",
				CustomHeaders: map[string]string{
					"X-Extra": "unexpected",
				},
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				req := chattool.MCPServerProposalRequest{
					DisplayName:            "API key",
					Slug:                   "api-key-" + uuid.NewString(),
					URL:                    "https://api-key.example.com/mcp",
					Transport:              "streamable_http",
					AuthType:               "api_key",
					APIKeyHeader:           "Authorization",
					APIKeyValuePlaceholder: "Paste your API key",
				}
				deps := newProposalsTest(t, req)
				rec := doProposalBody(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID, tt.values)
				require.Equal(t, http.StatusBadRequest, rec.Code)

				ctx := testutil.Context(t, testutil.WaitLong)
				proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
				require.NoError(t, err)
				require.Equal(t, "pending", proposal.Status)
			})
		}
	})

	t.Run("UserSuppliedOAuth2ClientSecret", func(t *testing.T) {
		t.Parallel()
		req := oauthManualRequest()
		req.Slug = "oauth-secret-" + uuid.NewString()
		req.OAuth2ClientSecretPlaceholder = "Paste the OAuth2 client secret"
		deps := newProposalsTest(t, req)

		rec := doProposalBody(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID,
			codersdk.AcceptMCPServerProposalRequest{OAuth2ClientSecret: "user-client-secret"})
		require.Equal(t, http.StatusOK, rec.Code)
		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		ctx := testutil.Context(t, testutil.WaitLong)
		config, err := deps.db.GetMCPServerConfigByID(ctx, resp.MCPServerConfigID)
		require.NoError(t, err)
		require.Equal(t, "user-client-secret", config.OAuth2ClientSecret)

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.NotContains(t, string(proposal.Request), "user-client-secret")
	})

	t.Run("RepeatedAcceptIgnoresLaterSecret", func(t *testing.T) {
		t.Parallel()
		req := chattool.MCPServerProposalRequest{
			DisplayName:            "API key",
			Slug:                   "repeat-secret-" + uuid.NewString(),
			URL:                    "https://api-key.example.com/mcp",
			Transport:              "streamable_http",
			AuthType:               "api_key",
			APIKeyHeader:           "Authorization",
			APIKeyValuePlaceholder: "Paste your API key",
		}
		deps := newProposalsTest(t, req)

		first := doProposalBody(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID,
			codersdk.AcceptMCPServerProposalRequest{APIKeyValue: " first-secret "})
		require.Equal(t, http.StatusOK, first.Code)
		second := doProposalBody(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID,
			codersdk.AcceptMCPServerProposalRequest{APIKeyValue: "second-secret"})
		require.Equal(t, http.StatusOK, second.Code)

		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(first.Body.Bytes(), &resp))
		ctx := testutil.Context(t, testutil.WaitLong)
		config, err := deps.db.GetMCPServerConfigByID(ctx, resp.MCPServerConfigID)
		require.NoError(t, err)
		require.Equal(t, " first-secret ", config.APIKeyValue)
	})

	t.Run("RepeatBeforeAuthReturnsSameConfig", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, oauthManualRequest())

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var first codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))

		rec = doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var second codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &second))

		require.Equal(t, first.MCPServerConfigID, second.MCPServerConfigID)
		require.Equal(t, first.ConnectURL, second.ConnectURL)

		// Notifications and card updates fired once, on the accepting
		// transition.
		require.Len(t, deps.webAPI.updates(), 1)
		_, adds := deps.chat.snapshot()
		require.Len(t, adds, 1)
	})

	t.Run("RepeatAfterAuthNoDuplicateNotifications", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, oauthManualRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var first codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))

		// Complete authentication; the poll posts the success message.
		_, err := deps.db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: first.MCPServerConfigID,
			UserID:            deps.ownerID,
			AccessToken:       "token",
			RefreshToken:      "refresh",
			TokenType:         "Bearer",
		})
		require.NoError(t, err)
		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		require.Contains(t, messageText(t, msg), "finished authenticating")

		rec = doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var second codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &second))
		require.Equal(t, first.MCPServerConfigID, second.MCPServerConfigID)
		require.True(t, second.Authenticated)
		require.Empty(t, second.ConnectURL)

		// No extra system messages or card updates from the repeat.
		sends, _ := deps.chat.snapshot()
		require.Len(t, sends, 1)
	})

	t.Run("ConcurrentDoublePost", func(t *testing.T) {
		t.Parallel()
		req := chattool.MCPServerProposalRequest{
			DisplayName:            "Concurrent API key",
			Slug:                   "concurrent-api-key-" + uuid.NewString(),
			URL:                    "https://api-key.example.com/mcp",
			Transport:              "streamable_http",
			AuthType:               "api_key",
			APIKeyHeader:           "Authorization",
			APIKeyValuePlaceholder: "Paste your API key",
		}
		deps := newProposalsTest(t, req)

		var wg sync.WaitGroup
		results := make([]*httptest.ResponseRecorder, 2)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = doProposalBody(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID,
					codersdk.AcceptMCPServerProposalRequest{APIKeyValue: fmt.Sprintf("secret-%d", i)})
			}()
		}
		wg.Wait()

		var configIDs []uuid.UUID
		for _, rec := range results {
			require.Equal(t, http.StatusOK, rec.Code)
			var resp codersdk.AcceptMCPServerProposalResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			configIDs = append(configIDs, resp.MCPServerConfigID)
		}
		// The row lock serializes the accepts: both requests observe
		// the same config, and exactly one exists.
		require.Equal(t, configIDs[0], configIDs[1])
		ctx := testutil.Context(t, testutil.WaitLong)
		configs, err := deps.db.GetEnabledMCPServerConfigs(ctx, deps.ownerID)
		require.NoError(t, err)
		personal := 0
		for _, config := range configs {
			if config.OwnerID.Valid {
				personal++
				require.Contains(t, []string{"secret-0", "secret-1"}, config.APIKeyValue)
			}
		}
		require.Equal(t, 1, personal)
		// One-shot notifications despite the double POST.
		require.Len(t, deps.webAPI.updates(), 1)
		_, adds := deps.chat.snapshot()
		require.Len(t, adds, 1)
	})

	t.Run("NoAuthPath", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.True(t, resp.Authenticated)
		require.Empty(t, resp.ConnectURL)

		// Green card plus the continue system message.
		require.Len(t, deps.webAPI.updates(), 1)
		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		text := messageText(t, msg)
		require.Contains(t, text, "[system]")
		require.Contains(t, text, "accepted")
		require.Contains(t, text, "next generation step")
		require.Equal(t, chatd.SendMessageBusyBehaviorInterrupt, msg.BusyBehavior)
		require.Equal(t, deps.chatRow.ID, msg.ChatID)
	})

	t.Run("CreateFailureRollsBack", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, chattool.MCPServerProposalRequest{
			DisplayName: "Broken",
			Slug:        "broken",
			URL:         "https://mcp.example.com",
			// Violates the transport check constraint, failing the
			// insert inside the accept transaction.
			Transport: "bogus",
			AuthType:  "none",
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusInternalServerError, rec.Code)

		// The transaction rolled back: the proposal stays pending, no
		// config exists, and no notifications fired. The user can
		// retry.
		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "pending", proposal.Status)
		require.False(t, proposal.MCPServerConfigID.Valid)
		require.Empty(t, deps.webAPI.updates())
		sends, adds := deps.chat.snapshot()
		require.Empty(t, sends)
		require.Empty(t, adds)
	})

	t.Run("EnableFailure", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		deps.chat.addErr = xerrors.New("enable exploded")
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)

		// Partial-success card plus the retry system message.
		require.Len(t, deps.webAPI.updates(), 1)
		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		text := messageText(t, msg)
		require.Contains(t, text, "enabling it for this chat failed")
		require.Contains(t, text, "enable_mcp_server")
	})

	t.Run("AuthPollSuccess", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, oauthManualRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		_, err := deps.db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: resp.MCPServerConfigID,
			UserID:            deps.ownerID,
			AccessToken:       "token",
			RefreshToken:      "refresh",
			TokenType:         "Bearer",
		})
		require.NoError(t, err)

		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		text := messageText(t, msg)
		require.Contains(t, text, "finished authenticating")
		require.Contains(t, text, "next generation step")
		// The pending card turned green on observed auth completion.
		require.Eventually(t, func() bool {
			return len(deps.webAPI.updates()) == 2
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("AuthPollTimeout", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, oauthManualRequest())
		deps.api.authPollTimeout = 50 * time.Millisecond
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp codersdk.AcceptMCPServerProposalResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		text := messageText(t, msg)
		require.Contains(t, text, "has not finished authenticating")
		require.Contains(t, text, resp.ConnectURL)
	})

	t.Run("WrongUserForbidden", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.otherUser.ID)
		require.Equal(t, http.StatusForbidden, rec.Code)
		require.NotContains(t, rec.Body.String(), deps.ownerID.String())
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		expireProposal(t, deps.sqlDB, deps.proposalID)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestMCPProposalReject(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.rejectProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusNoContent, rec.Code)

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "rejected", proposal.Status)

		// Gray card update plus the rejection system message.
		updates := deps.webAPI.updates()
		require.Len(t, updates, 1)
		require.Equal(t, "C123", updates[0].Channel)
		require.Equal(t, "1700000002.000300", updates[0].TS)
		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		text := messageText(t, msg)
		require.Contains(t, text, "rejected")
		require.Contains(t, text, "NOT created")
	})

	t.Run("RejectAfterAcceptFails", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		testutil.TryReceive(ctx, t, deps.chat.sent)

		rec = doProposal(t, deps.api.rejectProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusNotFound, rec.Code)

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "accepted", proposal.Status)
	})
}

// cancelCallback builds a Slack block-actions interaction for the
// given action.
func cancelCallback(slackUserID, actionID, value string) slack.InteractionCallback {
	return slack.InteractionCallback{
		Type: slack.InteractionTypeBlockActions,
		User: slack.User{ID: slackUserID},
		Channel: slack.Channel{GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: "C123"},
		}},
		Message: slack.Message{Msg: slack.Msg{Timestamp: "1700000002.000300"}},
		ActionCallback: slack.ActionCallbacks{
			BlockActions: []*slack.BlockAction{{
				ActionID: actionID,
				Value:    value,
			}},
		},
	}
}

func TestMCPProposalCancel(t *testing.T) {
	t.Parallel()

	t.Run("CancelClick", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		deps.api.HandleBlockActions(ctx, cancelCallback(deps.slackUser, chattool.MCPProposalCancelActionID, deps.proposalID.String()))

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "rejected", proposal.Status)
		require.Len(t, deps.webAPI.updates(), 1)
		msg := testutil.TryReceive(ctx, t, deps.chat.sent)
		require.Contains(t, messageText(t, msg), "rejected")
	})

	t.Run("WrongSlackUser", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		deps.api.HandleBlockActions(ctx, cancelCallback("UIMPOSTOR", chattool.MCPProposalCancelActionID, deps.proposalID.String()))

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "pending", proposal.Status)
		ephemerals := deps.webAPI.ephemerals()
		require.Len(t, ephemerals, 1)
		require.Equal(t, "UIMPOSTOR", ephemerals[0].User)
		require.Empty(t, deps.webAPI.updates())
	})

	t.Run("ExpiredProposal", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)
		expireProposal(t, deps.sqlDB, deps.proposalID)

		deps.api.HandleBlockActions(ctx, cancelCallback(deps.slackUser, chattool.MCPProposalCancelActionID, deps.proposalID.String()))

		// The card is replaced with the expired message; nothing else
		// happens.
		require.Len(t, deps.webAPI.updates(), 1)
		sends, _ := deps.chat.snapshot()
		require.Empty(t, sends)
	})

	t.Run("MissingProposal", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		deps.api.HandleBlockActions(ctx, cancelCallback(deps.slackUser, chattool.MCPProposalCancelActionID, uuid.NewString()))

		require.Len(t, deps.webAPI.updates(), 1)
		sends, _ := deps.chat.snapshot()
		require.Empty(t, sends)
	})

	t.Run("AlreadyAccepted", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
		require.Equal(t, http.StatusOK, rec.Code)
		testutil.TryReceive(ctx, t, deps.chat.sent)
		acceptUpdates := len(deps.webAPI.updates())

		deps.api.HandleBlockActions(ctx, cancelCallback(deps.slackUser, chattool.MCPProposalCancelActionID, deps.proposalID.String()))

		ephemerals := deps.webAPI.ephemerals()
		require.Len(t, ephemerals, 1)
		require.Equal(t, deps.slackUser, ephemerals[0].User)
		require.Len(t, deps.webAPI.updates(), acceptUpdates)
		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "accepted", proposal.Status)
	})

	t.Run("ReviewButtonIgnored", func(t *testing.T) {
		t.Parallel()
		deps := newProposalsTest(t, noAuthRequest())
		ctx := testutil.Context(t, testutil.WaitLong)

		deps.api.HandleBlockActions(ctx, cancelCallback(deps.slackUser, "mcp_proposal_review", deps.proposalID.String()))

		proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
		require.NoError(t, err)
		require.Equal(t, "pending", proposal.Status)
		require.Empty(t, deps.webAPI.updates())
		require.Empty(t, deps.webAPI.ephemerals())
		sends, _ := deps.chat.snapshot()
		require.Empty(t, sends)
	})

	t.Run("InteractiveEventAckedAndRouted", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		owner, _ := seedOwner(t, db)
		chat := newFakeChatSubmitter()
		socket := newFakeSocketClient()

		deps := newProposalsTest(t, noAuthRequest())
		server, _ := newTestServerWithWebAPI(t, db, chat, owner.ID, "", socket)
		server.proposals = deps.api
		server.Start(ctx)

		socket.events <- socketmode.Event{
			Type:    socketmode.EventTypeInteractive,
			Request: &socketmode.Request{EnvelopeID: "env-1"},
			Data:    cancelCallback(deps.slackUser, chattool.MCPProposalCancelActionID, deps.proposalID.String()),
		}

		req := testutil.TryReceive(ctx, t, socket.acked)
		assert.Equal(t, "env-1", req.EnvelopeID)
		require.Eventually(t, func() bool {
			proposal, err := deps.db.GetMCPServerProposalByID(ctx, deps.proposalID)
			return err == nil && proposal.Status == "rejected"
		}, testutil.WaitLong, testutil.IntervalFast)
	})
}

func TestMCPProposalOAuth2Discovery(t *testing.T) {
	t.Parallel()

	// A stub MCP server plus authorization server implementing RFC
	// 9728 discovery and RFC 7591 registration.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource":              srv.URL,
			"authorization_servers": []string{srv.URL},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint":         srv.URL + "/token",
			"registration_endpoint":  srv.URL + "/register",
			"scopes_supported":       []string{"read", "write"},
		})
	})
	var registeredRedirect string
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.RedirectURIs) > 0 {
			registeredRedirect = payload.RedirectURIs[0]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":     "dcr-client",
			"client_secret": "dcr-secret",
		})
	})

	deps := newProposalsTest(t, chattool.MCPServerProposalRequest{
		DisplayName: "Discovered",
		Slug:        "discovered",
		URL:         srv.URL,
		Transport:   "streamable_http",
		AuthType:    "oauth2",
	})

	rec := doProposal(t, deps.api.acceptProposal, http.MethodPost, deps.proposalID, deps.ownerID)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp codersdk.AcceptMCPServerProposalResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	ctx := testutil.Context(t, testutil.WaitLong)
	config, err := deps.db.GetMCPServerConfigByID(ctx, resp.MCPServerConfigID)
	require.NoError(t, err)
	require.Equal(t, "dcr-client", config.OAuth2ClientID)
	require.Equal(t, "dcr-secret", config.OAuth2ClientSecret)
	require.Equal(t, srv.URL+"/authorize", config.OAuth2AuthURL)
	require.Equal(t, srv.URL+"/token", config.OAuth2TokenURL)
	require.Equal(t, "read write", config.OAuth2Scopes)
	// The registered callback URL points at the created config.
	require.Contains(t, registeredRedirect, config.ID.String())
	require.True(t, strings.HasPrefix(registeredRedirect, "https://coder.example.com/"))
}

func TestValidateMCPServerOAuth2Discovery(t *testing.T) {
	t.Parallel()

	t.Run("MissingRegistrationEndpoint", func(t *testing.T) {
		t.Parallel()
		mux := http.NewServeMux()
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srv.URL,
				"authorization_servers": []string{srv.URL},
			})
		})
		mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srv.URL,
				"authorization_endpoint": srv.URL + "/authorize",
				"token_endpoint":         srv.URL + "/token",
			})
		})

		err := ValidateMCPServerOAuth2Discovery(t.Context(), srv.Client(), srv.URL)
		require.ErrorContains(t, err, "does not advertise a registration_endpoint")
	})

	t.Run("DiscoveryFailure", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)

		err := ValidateMCPServerOAuth2Discovery(t.Context(), srv.Client(), srv.URL)
		require.ErrorContains(t, err, "protected resource discovery")
	})

	t.Run("MetadataOnly", func(t *testing.T) {
		t.Parallel()
		mux := http.NewServeMux()
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              srv.URL,
				"authorization_servers": []string{srv.URL},
			})
		})
		mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 srv.URL,
				"authorization_endpoint": srv.URL + "/authorize",
				"token_endpoint":         srv.URL + "/token",
				"registration_endpoint":  srv.URL + "/register",
			})
		})
		registrationCalls := 0
		mux.HandleFunc("/register", func(http.ResponseWriter, *http.Request) {
			registrationCalls++
		})

		require.NoError(t, ValidateMCPServerOAuth2Discovery(t.Context(), srv.Client(), srv.URL))
		require.Zero(t, registrationCalls)
	})
}

// messageText concatenates the text parts of a system message.
func messageText(t *testing.T, opts chatd.SendMessageOptions) string {
	t.Helper()
	var sb strings.Builder
	for _, part := range opts.Content {
		_, _ = sb.WriteString(part.Text)
	}
	return sb.String()
}

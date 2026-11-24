package chatd_test

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

const (
	reconnectSlug        = "linear"
	reconnectStoredToken = "stored-access-token"
	reconnectStoredRT    = "stored-refresh-token"
	reconnectTurnMarker  = "turn-marker-"
)

// rejectingMCPServer serves an echo tool to requests whose bearer token it
// accepts, and answers every other request with the recorded 401. It
// speaks only the protocol versions before 2026-07-28: it rejects
// server/discover, so the go-sdk client falls back to initialize, which
// rows 7, 10 and 15 reject once.
type rejectingMCPServer struct {
	*httptest.Server
	handler http.Handler
	// inject401, called under mu with the request's bearer token, answers
	// a request with the 401 whatever its token.
	inject401 func(rpcMethod, token string) bool

	mu       sync.Mutex
	accepted map[string]bool
	requests []mcpServerRequest
	// noChallenge sends every 401 without a WWW-Authenticate challenge.
	noChallenge bool
}

type mcpServerRequest struct {
	chatID        string
	rpcMethod     string
	authorization string
	rejected      bool
}

func newRejectingMCPServer(t *testing.T, inject401 func(rpcMethod, token string) bool) *rejectingMCPServer {
	t.Helper()
	srv := newTestMCPServer("reconnect-mcp")
	addTestMCPTextTool(srv, "echo", "Echoes the input", "echo: ")
	s := &rejectingMCPServer{
		handler:   testMCPHTTPHandler(srv),
		inject401: inject401,
		accepted:  map[string]bool{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *rejectingMCPServer) accept(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accepted[accessToken] = true
}

// rejectWithoutChallenge answers every later 401 without a
// WWW-Authenticate challenge, like a server that rejects the forwarded
// Coder header signature rather than the access token.
func (s *rejectingMCPServer) rejectWithoutChallenge() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noChallenge = true
}

func (s *rejectingMCPServer) log() []mcpServerRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func (s *rejectingMCPServer) serve(w http.ResponseWriter, r *http.Request) {
	req := mcpServerRequest{
		chatID:        r.Header.Get(chatprovider.HeaderCoderChatID),
		authorization: r.Header.Get("Authorization"),
	}
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var msg struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &msg)
		req.rpcMethod = msg.Method
	}
	token, hasBearer := strings.CutPrefix(req.authorization, "Bearer ")
	s.mu.Lock()
	injected := s.inject401 != nil && s.inject401(req.rpcMethod, token)
	req.rejected = injected || !hasBearer || !s.accepted[token]
	s.requests = append(s.requests, req)
	noChallenge := s.noChallenge
	s.mu.Unlock()

	if !req.rejected {
		if req.rpcMethod == "server/discover" {
			writeMethodNotFound(w, r)
			return
		}
		s.handler.ServeHTTP(w, r)
		return
	}
	if noChallenge {
		http.Error(w, "invalid request signature", http.StatusUnauthorized)
		return
	}
	_ = chattest.WriteRecordedUnauthorized(w)
}

// writeMethodNotFound answers a JSON-RPC request with the JSON-RPC 2.0
// method not found error.
func writeMethodNotFound(w http.ResponseWriter, r *http.Request) {
	var msg struct {
		ID json.RawMessage `json:"id"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"Method not found"}}`, cmp.Or(string(msg.ID), "null"))
}

// firstInitialize401 rejects the first initialize request only.
func firstInitialize401() func(rpcMethod, token string) bool {
	return firstInitialize401For("")()
}

// firstInitialize401For returns a factory for a rejection of the first
// initialize request that carries token, or any token when token is
// empty.
func firstInitialize401For(token string) func() func(rpcMethod, token string) bool {
	return func() func(rpcMethod, token string) bool {
		seen := false
		return func(rpcMethod, got string) bool {
			if rpcMethod != "initialize" || seen || (token != "" && got != token) {
				return false
			}
			seen = true
			return true
		}
	}
}

// everyOtherInitialize401 rejects the first initialize request and every
// other one after it, so each step's first connect gets a 401 and a
// reconnect right after it does not.
func everyOtherInitialize401() func(rpcMethod, token string) bool {
	initializes := 0
	return func(rpcMethod, _ string) bool {
		if rpcMethod != "initialize" {
			return false
		}
		initializes++
		return initializes%2 == 1
	}
}

// tokenAnswer selects how the token endpoint answers every refresh.
type tokenAnswer int

const (
	// tokenAnswerRotate issues a new token pair that the MCP server
	// accepts. Like Cloudflare workers-oauth-provider, it also accepts the
	// previous refresh token.
	tokenAnswerRotate tokenAnswer = iota
	// tokenAnswerRotateRejected issues a new token pair that the MCP
	// server rejects.
	tokenAnswerRotateRejected
	// tokenAnswerRotateExpiring issues a new token pair that the MCP
	// server accepts and that expires within golang.org/x/oauth2's
	// 10 second expiry window, so every step's expiry refresh replaces
	// it.
	tokenAnswerRotateExpiring
	tokenAnswerInvalidGrant
	tokenAnswerUnavailable
)

// reconnectTokenServer is an OAuth token endpoint for one grant.
type reconnectTokenServer struct {
	*httptest.Server
	calls  atomic.Int32
	answer tokenAnswer
	mcp    *rejectingMCPServer
	grant  *chattest.TokenGrant
}

func newReconnectTokenServer(t *testing.T, answer tokenAnswer, mcpSrv *rejectingMCPServer) *reconnectTokenServer {
	t.Helper()
	s := &reconnectTokenServer{answer: answer, mcp: mcpSrv, grant: chattest.NewTokenGrant("issued-", reconnectStoredRT, true)}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *reconnectTokenServer) serve(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	writeJSON := func(status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
	// invalid_grant "Grant not found" is the Cloudflare
	// workers-oauth-provider answer for a revoked grant.
	const grantNotFound = `{"error":"invalid_grant","error_description":"Grant not found"}`
	switch s.answer {
	case tokenAnswerUnavailable:
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "Service Unavailable")
		return
	case tokenAnswerInvalidGrant:
		writeJSON(http.StatusBadRequest, grantNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSON(http.StatusBadRequest, `{"error":"invalid_request"}`)
		return
	}
	accessToken, issuedRT, ok := s.grant.Rotate(r.PostForm.Get("refresh_token"))
	if !ok {
		writeJSON(http.StatusBadRequest, grantNotFound)
		return
	}
	expiresIn := 3600
	switch s.answer {
	case tokenAnswerRotate:
		s.mcp.accept(accessToken)
	case tokenAnswerRotateExpiring:
		s.mcp.accept(accessToken)
		expiresIn = 5
	}
	writeJSON(http.StatusOK, fmt.Sprintf(`{"access_token":%q,"token_type":"bearer","expires_in":%d,"refresh_token":%q}`, accessToken, expiresIn, issuedRT))
}

// reconnectEnv is one coderd deployment with one OAuth MCP server
// configured, used by one row.
type reconnectEnv struct {
	t      *testing.T
	client *codersdk.Client
	exp    *codersdk.ExperimentalClient
	api    *coderd.API
	// sqlDB is a direct connection to the deployment's database.
	sqlDB  *sql.DB
	orgID  uuid.UUID
	userID uuid.UUID
	cfg    database.MCPServerConfig
	mcp    *rejectingMCPServer
	tokens *reconnectTokenServer

	// clock times the waits before a rejected token is sent again, which
	// advanceGraces ends at once and counts in graces.
	clock  *quartz.Mock
	graces atomic.Int32

	llmMu       sync.Mutex
	llmRequests map[string]int
	offered     map[string][]string
	// modelHold, when set, holds the first model request of every turn
	// until the test opens it.
	modelHold *chattest.Gate
}

func newReconnectEnv(t *testing.T, mcpSrv *rejectingMCPServer, tokens *reconnectTokenServer) *reconnectEnv {
	t.Helper()
	e := &reconnectEnv{
		t:           t,
		mcp:         mcpSrv,
		tokens:      tokens,
		llmRequests: map[string]int{},
		offered:     map[string][]string{},
		clock:       quartz.NewMock(t),
	}
	grace := e.clock.Trap().NewTimer("mcpclient", "rejected_token_grace")
	t.Cleanup(grace.Close)
	go e.advanceGraces(t.Context(), grace)
	values := coderdtest.DeploymentValues(t)
	values.AI.Chat.DebugLoggingEnabled = serpent.Bool(true)
	// The database coderdtest would create, kept with a direct connection.
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	pgps, ok := ps.(*pubsub.PGPubsub)
	require.True(t, ok)
	e.sqlDB = sqlDB
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:      values,
		Database:              db,
		ReplicaSyncPubsub:     pgps,
		ChatMCPReconnectClock: e.clock,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	first := coderdtest.CreateFirstUser(t, client)
	e.client, e.api = client, api
	e.orgID, e.userID = first.OrganizationID, first.UserID
	e.exp = codersdk.NewExperimentalClient(client)

	llmURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		marker := turnMarkerOf(req)
		e.llmMu.Lock()
		e.llmRequests[marker]++
		first := e.llmRequests[marker] == 1
		if first {
			e.offered[marker] = offeredToolNames(t, req)
		}
		hold := e.modelHold
		e.llmMu.Unlock()
		if first {
			if hold != nil {
				_ = hold.Wait(req.Context())
			}
			// A tool call makes every turn run more than one step.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(reconnectSlug+"__echo", `{"input":"ping"}`),
			)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	coderdtest.CreateOpenAICompatChatModel(t, e.exp, llmURL)

	e.cfg = dbgen.MCPServerConfig(t, api.Database, database.MCPServerConfig{
		OrganizationID:      e.orgID,
		DisplayName:         "Linear",
		Slug:                reconnectSlug,
		Url:                 mcpSrv.URL,
		AuthType:            "oauth2",
		OAuth2ClientID:      "reconnect-client",
		OAuth2TokenURL:      tokens.URL,
		Enabled:             true,
		ForwardCoderHeaders: true,
		CreatedBy:           uuid.NullUUID{UUID: e.userID, Valid: true},
		UpdatedBy:           uuid.NullUUID{UUID: e.userID, Valid: true},
	})
	return e
}

// advanceGraces ends every wait before a rejected token is sent again as
// soon as chatd starts it, and counts the waits.
func (e *reconnectEnv) advanceGraces(ctx context.Context, grace *quartz.Trap) {
	for {
		call, err := grace.Wait(ctx)
		if err != nil {
			return
		}
		e.graces.Add(1)
		if call.Release(ctx) != nil {
			return
		}
		// The released call started the grace timer, the clock's only
		// timer, so advancing to the next event ends it.
		_, advanced := e.clock.AdvanceNext()
		select {
		case <-advanced.Done():
		case <-ctx.Done():
			return
		}
	}
}

func turnMarkerOf(req *chattest.OpenAIRequest) string {
	for _, msg := range slices.Backward(req.Messages) {
		if msg.Role != "user" {
			continue
		}
		if idx := strings.Index(msg.Content, reconnectTurnMarker); idx >= 0 {
			return msg.Content[idx:]
		}
	}
	return ""
}

func reconnectSystemCtx(ctx context.Context) context.Context {
	//nolint:gocritic // Seeding and reading token state requires system access.
	return dbauthz.AsSystemRestricted(ctx)
}

func (e *reconnectEnv) seedToken(refreshToken string, expiry sql.NullTime) {
	e.t.Helper()
	dbgen.MCPServerUserToken(e.t, e.api.Database, database.MCPServerUserToken{
		MCPServerConfigID: e.cfg.ID,
		UserID:            e.userID,
		AccessToken:       reconnectStoredToken,
		RefreshToken:      refreshToken,
		Expiry:            expiry,
	})
}

func (e *reconnectEnv) tokenRow() database.MCPServerUserToken {
	e.t.Helper()
	ctx := testutil.Context(e.t, testutil.WaitShort)
	row, err := e.api.Database.GetMCPServerUserToken(reconnectSystemCtx(ctx), database.GetMCPServerUserTokenParams{
		MCPServerConfigID: e.cfg.ID,
		UserID:            e.userID,
	})
	require.NoError(e.t, err)
	return row
}

// authConnected reads auth_connected from the MCP server list and counts
// the token endpoint calls the list made.
func (e *reconnectEnv) authConnected() (connected bool, listCalls int) {
	e.t.Helper()
	ctx := testutil.Context(e.t, testutil.WaitLong)
	before := e.tokens.calls.Load()
	configs, err := e.client.MCPServerConfigs(ctx, e.orgID)
	require.NoError(e.t, err)
	idx := slices.IndexFunc(configs, func(cfg codersdk.MCPServerConfig) bool { return cfg.ID == e.cfg.ID })
	require.NotEqual(e.t, -1, idx)
	return configs[idx].AuthConnected, int(e.tokens.calls.Load() - before)
}

// startTurn sends one user message, creating the chat when chatID is nil,
// and returns the chat and the turn's marker.
func (e *reconnectEnv) startTurn(chatID uuid.UUID) (uuid.UUID, string) {
	e.t.Helper()
	ctx := testutil.Context(e.t, testutil.WaitLong)
	marker := reconnectTurnMarker + uuid.NewString()
	content := []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Use the echo tool. " + marker}}
	if chatID == uuid.Nil {
		chat, err := e.exp.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: e.orgID,
			Content:        content,
			MCPServerIDs:   []uuid.UUID{e.cfg.ID},
		})
		require.NoError(e.t, err)
		return chat.ID, marker
	}
	_, err := e.exp.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{Content: content})
	require.NoError(e.t, err)
	return chatID, marker
}

// lockTokenTable takes an exclusive lock on mcp_server_user_tokens, which
// holds every turn at its token read with no deadline, and returns the
// function that releases it. The function is safe to call more than once.
func (e *reconnectEnv) lockTokenTable(ctx context.Context) (unlock func()) {
	e.t.Helper()
	tx, err := e.sqlDB.BeginTx(ctx, nil)
	require.NoError(e.t, err)
	_, err = tx.ExecContext(ctx, "LOCK TABLE mcp_server_user_tokens IN ACCESS EXCLUSIVE MODE")
	if err != nil {
		_ = tx.Rollback()
		require.NoError(e.t, err)
	}
	var once sync.Once
	return func() { once.Do(func() { _ = tx.Rollback() }) }
}

// waitTokenReads waits until n turns wait on the lock from lockTokenTable
// at their token read.
func (e *reconnectEnv) waitTokenReads(ctx context.Context, n int) {
	e.t.Helper()
	testutil.Eventually(ctx, e.t, func(ctx context.Context) bool {
		var waiting int
		err := e.sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND query LIKE '-- name: GetMCPServerUserTokensByUserID %'`).Scan(&waiting)
		return err == nil && waiting == n
	}, testutil.IntervalFast)
}

// holdFirstModelRequests makes the model hold the first request of every
// later turn at gate, after the turn's first MCP connect.
func (e *reconnectEnv) holdFirstModelRequests(gate *chattest.Gate) {
	e.llmMu.Lock()
	defer e.llmMu.Unlock()
	e.modelHold = gate
}

// sendWhileBusy sends one user message to a busy chat, which queues it in
// the lane that busy names, and returns the message's marker.
func (e *reconnectEnv) sendWhileBusy(chatID uuid.UUID, busy codersdk.ChatBusyBehavior) string {
	e.t.Helper()
	ctx := testutil.Context(e.t, testutil.WaitLong)
	marker := reconnectTurnMarker + uuid.NewString()
	resp, err := e.exp.CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Use the echo tool. " + marker}},
		BusyBehavior: busy,
	})
	require.NoError(e.t, err)
	require.True(e.t, resp.Queued, "the message must wait in the queue while the chat runs")
	return marker
}

// waitTurn waits until the model has answered the turn's tool result and
// the chat daemon is idle.
func (e *reconnectEnv) waitTurn(chatID uuid.UUID, marker string) {
	e.t.Helper()
	ctx := testutil.Context(e.t, testutil.WaitSuperLong)
	testutil.Eventually(ctx, e.t, func(ctx context.Context) bool {
		e.llmMu.Lock()
		answered := e.llmRequests[marker] >= 2
		e.llmMu.Unlock()
		if !answered {
			return false
		}
		chat, err := e.exp.GetChat(ctx, chatID)
		return err == nil && (chat.Status == codersdk.ChatStatusWaiting || chat.Status == codersdk.ChatStatusError)
	}, testutil.IntervalFast)
	require.NoError(e.t, chatd.WaitUntilIdleForTest(ctx, e.api.ChatDaemon()))
}

func (e *reconnectEnv) offeredTools(marker string) []string {
	e.llmMu.Lock()
	defer e.llmMu.Unlock()
	return slices.Clone(e.offered[marker])
}

// mcpConnectEntry is one MCP connect outcome from a chat debug run.
type mcpConnectEntry struct {
	Outcome   string `json:"outcome"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error"`
}

// lastTurnConnects returns the MCP connect outcomes of the chat's latest
// turn, one per generation step, in step order.
func (e *reconnectEnv) lastTurnConnects(chatID uuid.UUID) []mcpConnectEntry {
	e.t.Helper()
	ctx := testutil.Context(e.t, testutil.WaitLong)
	runs, err := e.exp.GetChatDebugRuns(ctx, chatID)
	require.NoError(e.t, err)
	runs = slices.DeleteFunc(runs, func(run codersdk.ChatDebugRunSummary) bool {
		return run.Kind != codersdk.ChatDebugRunKindChatTurn
	})
	require.NotEmpty(e.t, runs, "the turn recorded no chat_turn debug run")
	slices.SortFunc(runs, func(a, b codersdk.ChatDebugRunSummary) int { return a.StartedAt.Compare(b.StartedAt) })
	raw, err := json.Marshal(runs[len(runs)-1].Summary["mcp_connect"])
	require.NoError(e.t, err)
	var entries []mcpConnectEntry
	require.NoError(e.t, json.Unmarshal(raw, &entries))
	require.NotEmpty(e.t, entries, "the turn recorded no MCP connect outcome")
	return entries
}

// describeMCPRequests renders the first MCP requests of a turn for the
// test log, as rpc method, token and answer.
func describeMCPRequests(requests []mcpServerRequest) string {
	parts := make([]string, 0, 8)
	for _, req := range requests[:min(len(requests), 8)] {
		token := strings.TrimPrefix(req.authorization, "Bearer ")
		if token == "" {
			token = "no-token"
		}
		answer := "ok"
		if req.rejected {
			answer = "401"
		}
		parts = append(parts, fmt.Sprintf("%s[%s]=%s", cmp.Or(req.rpcMethod, "GET/DELETE"), token, answer))
	}
	return strings.Join(parts, " ")
}

// requireStoredTokenRejectedFirst checks that the MCP server first got the
// stored access token and answered it with a 401.
func requireStoredTokenRejectedFirst(t *testing.T, requests []mcpServerRequest) {
	t.Helper()
	stored := "Bearer " + reconnectStoredToken
	require.NotEmpty(t, requests)
	require.Equal(t, stored, requests[0].authorization, "the first MCP request must carry the stored access token")
	for _, req := range requests {
		if req.authorization != stored {
			break
		}
		if req.rejected {
			return
		}
	}
	t.Fatal("the MCP server never answered the stored access token with a 401")
}

// tokenRowWant is the token material a row must hold after its turns.
type tokenRowWant int

const (
	tokenRowStored tokenRowWant = iota
	tokenRowIssued
	// tokenRowCleared is a token marked for reconnect, whose token
	// material the mark clears.
	tokenRowCleared
)

type reconnectTurnWant struct {
	// tokenCalls is the token endpoint calls in the turn. With
	// callsPerStep set, the turn must make that many calls per step
	// instead.
	tokenCalls   int
	callsPerStep int
	// step1, when set, is the first step's connect outcome.
	step1      string
	step1Tools bool
	// noAuthorization requires every MCP request in the turn to carry no
	// Authorization header.
	noAuthorization bool
	// graceWaits is how often the turn waited before it sent a rejected
	// token again.
	graceWaits int
}

// TestMCPServerOAuth2Reconnect401 runs full chat turns against an MCP server
// that rejects the stored OAuth access token with a 401. A turn refreshes
// the token once, even before its stored expiry, and reconnects once; a
// token that is still rejected after a short grace is marked for reconnect,
// so the MCP server list reports it as not connected. The grace runs on a
// mock clock that the env advances at once. Token endpoint calls are HTTP
// calls: golang.org/x/oauth2 retries every failed token request once with
// the other client auth style, so a failed refresh attempt is 2 calls.
func TestMCPServerOAuth2Reconnect401(t *testing.T) {
	t.Parallel()

	const (
		rejectedReason  = "mcp server rejected the access token (HTTP 401)"
		refreshedReason = "mcp server rejected the refreshed access token (HTTP 401)"
		noRTReason      = "mcp server rejected the access token (HTTP 401): no refresh token"
	)
	hourAhead := sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}
	hourAgo := sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}

	rows := []struct {
		name string
		// storedAccepted makes the MCP server accept the stored access
		// token.
		storedAccepted bool
		noRefreshToken bool
		expiry         sql.NullTime
		answer         tokenAnswer
		inject401      func() func(rpcMethod, token string) bool
		// noChallenge sends the 401s without a WWW-Authenticate challenge.
		noChallenge bool
		turns       []reconnectTurnWant
		wantRow     tokenRowWant
		// wantReason is the exact failure reason, or its prefix when
		// reasonPrefix is set.
		wantReason    string
		reasonPrefix  bool
		wantConnected bool
		wantListCalls int
		// skipFirstRejection skips the check that the MCP server first
		// rejected the stored token, for rows that refresh before
		// connecting.
		skipFirstRejection bool
	}{
		{
			name:          "1 refresh works",
			expiry:        hourAhead,
			answer:        tokenAnswerRotate,
			turns:         []reconnectTurnWant{{tokenCalls: 1, step1: "connected", step1Tools: true}},
			wantRow:       tokenRowIssued,
			wantConnected: true,
		},
		{
			name:   "2 invalid_grant",
			expiry: hourAhead,
			answer: tokenAnswerInvalidGrant,
			turns: []reconnectTurnWant{
				{tokenCalls: 2, step1: "unauthorized"},
				{tokenCalls: 0, noAuthorization: true},
			},
			wantRow:      tokenRowCleared,
			wantReason:   rejectedReason,
			reasonPrefix: true,
		},
		{
			name:       "3 still 401 after refresh",
			expiry:     hourAhead,
			answer:     tokenAnswerRotateRejected,
			turns:      []reconnectTurnWant{{tokenCalls: 1, step1: "unauthorized", graceWaits: 1}},
			wantRow:    tokenRowCleared,
			wantReason: refreshedReason,
		},
		{
			// With no stored expiry the token may still be valid, so it
			// gets the grace. Row 17 covers a future expiry.
			name:           "4 no refresh token",
			noRefreshToken: true,
			answer:         tokenAnswerRotate,
			turns:          []reconnectTurnWant{{tokenCalls: 0, step1: "unauthorized", graceWaits: 1}},
			wantRow:        tokenRowCleared,
			wantReason:     noRTReason,
		},
		{
			// A token endpoint error that is not a grant error leaves the
			// token unmarked, so the list keeps reporting it as connected.
			// TestRefreshOAuth2TokenProviderErrors in mcpoauth owns which
			// errors count: invalid_client is one that does not, because
			// signing in again cannot fix a deleted client.
			name:   "5 token endpoint 503",
			expiry: hourAhead,
			answer: tokenAnswerUnavailable,
			turns: []reconnectTurnWant{
				{tokenCalls: 2, step1: "unauthorized"},
				{tokenCalls: 2, step1: "unauthorized"},
			},
			wantConnected: true,
		},
		{
			name:           "7 spurious 401 on the first initialize",
			storedAccepted: true,
			expiry:         hourAhead,
			answer:         tokenAnswerRotate,
			inject401:      firstInitialize401,
			turns:          []reconnectTurnWant{{tokenCalls: 1, step1: "connected", step1Tools: true}},
			wantRow:        tokenRowIssued,
			wantConnected:  true,
		},
		{
			name:   "8 invalid_grant with no stored expiry",
			answer: tokenAnswerInvalidGrant,
			turns: []reconnectTurnWant{
				{tokenCalls: 2, step1: "unauthorized"},
				{tokenCalls: 0, noAuthorization: true},
			},
			wantRow:      tokenRowCleared,
			wantReason:   rejectedReason,
			reasonPrefix: true,
		},
		{
			name:               "10 401 once after an expiry refresh",
			expiry:             hourAgo,
			answer:             tokenAnswerRotate,
			inject401:          firstInitialize401,
			turns:              []reconnectTurnWant{{tokenCalls: 1, step1: "connected", step1Tools: true, graceWaits: 1}},
			wantRow:            tokenRowIssued,
			wantConnected:      true,
			skipFirstRejection: true,
		},
		{
			// The failed expiry refresh counts as the turn's attempt, so
			// the 401 for the expired token triggers no forced refresh.
			name:          "11 expired token and token endpoint 503",
			expiry:        hourAgo,
			answer:        tokenAnswerUnavailable,
			turns:         []reconnectTurnWant{{callsPerStep: 2, step1: "unauthorized"}},
			wantListCalls: 2,
		},
		{
			// A 401 without a Bearer challenge, such as a rejected
			// forwarded Coder header signature, says nothing about the
			// token, so the token is neither refreshed nor marked.
			name:           "14 401 without a Bearer challenge",
			storedAccepted: true,
			expiry:         hourAhead,
			answer:         tokenAnswerRotate,
			inject401:      func() func(string, string) bool { return func(string, string) bool { return true } },
			noChallenge:    true,
			turns:          []reconnectTurnWant{{tokenCalls: 0, step1: "error"}},
			wantConnected:  true,
		},
		{
			// A provider whose token store is eventually consistent can
			// reject a token it issued seconds earlier.
			name:      "15 401 once after a forced refresh",
			expiry:    hourAhead,
			answer:    tokenAnswerRotate,
			inject401: firstInitialize401For("issued-access-1"),
			turns: []reconnectTurnWant{
				{tokenCalls: 1, step1: "connected", step1Tools: true, graceWaits: 1},
			},
			wantRow:       tokenRowIssued,
			wantConnected: true,
		},
		{
			// The grace reconnect is the turn's one attempt for the token,
			// so a later step's 401 gets no grace and no mark.
			name:           "17 no refresh token, 401 on every step's first connect",
			storedAccepted: true,
			noRefreshToken: true,
			expiry:         hourAhead,
			answer:         tokenAnswerRotate,
			inject401:      everyOtherInitialize401,
			turns: []reconnectTurnWant{
				{tokenCalls: 0, step1: "connected", step1Tools: true, graceWaits: 1},
			},
			wantConnected: true,
		},
		{
			// Nothing can make an expired token without a refresh token
			// valid again, so its 401 marks it at once, with no grace.
			name:           "18 expired token without a refresh token",
			noRefreshToken: true,
			expiry:         hourAgo,
			answer:         tokenAnswerRotate,
			turns:          []reconnectTurnWant{{tokenCalls: 0, step1: "unauthorized", graceWaits: 0}},
			wantRow:        tokenRowCleared,
			wantReason:     noRTReason,
		},
		{
			// Each step's expiry refresh issues a token that the MCP server
			// rejects on its first connect. Step 1's grace is the turn's one
			// attempt for the token, so step 2's 401 gets no grace.
			name:               "19 expiry refresh in every step, 401 on every step's first connect",
			expiry:             hourAgo,
			answer:             tokenAnswerRotateExpiring,
			inject401:          everyOtherInitialize401,
			turns:              []reconnectTurnWant{{callsPerStep: 1, step1: "connected", step1Tools: true, graceWaits: 1}},
			wantRow:            tokenRowIssued,
			wantConnected:      true,
			wantListCalls:      1,
			skipFirstRejection: true,
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			var inject401 func(rpcMethod, token string) bool
			if row.inject401 != nil {
				inject401 = row.inject401()
			}
			mcpSrv := newRejectingMCPServer(t, inject401)
			if row.storedAccepted {
				mcpSrv.accept(reconnectStoredToken)
			}
			if row.noChallenge {
				mcpSrv.rejectWithoutChallenge()
			}
			tokens := newReconnectTokenServer(t, row.answer, mcpSrv)
			e := newReconnectEnv(t, mcpSrv, tokens)
			refreshToken := reconnectStoredRT
			if row.noRefreshToken {
				refreshToken = ""
			}
			e.seedToken(refreshToken, row.expiry)

			chatID := uuid.Nil
			for i, want := range row.turns {
				callsBefore := tokens.calls.Load()
				gracesBefore := e.graces.Load()
				requestsBefore := len(mcpSrv.log())
				var marker string
				chatID, marker = e.startTurn(chatID)
				e.waitTurn(chatID, marker)
				calls := int(tokens.calls.Load() - callsBefore)
				graces := int(e.graces.Load() - gracesBefore)
				connects := e.lastTurnConnects(chatID)
				t.Logf("turn %d: token_calls=%d graces=%d steps=%d step1=%+v mcp=%s", i+1, calls, graces, len(connects), connects[0],
					describeMCPRequests(mcpSrv.log()[requestsBefore:]))
				assert.Equal(t, want.graceWaits, graces, "turn %d: waits before a rejected token is sent again", i+1)

				if want.callsPerStep > 0 {
					assert.Equal(t, want.callsPerStep*len(connects), calls,
						"turn %d: token endpoint calls for %d steps", i+1, len(connects))
				} else {
					assert.Equal(t, want.tokenCalls, calls, "turn %d: token endpoint calls", i+1)
				}
				require.GreaterOrEqual(t, len(connects), 2, "turn %d must run 2 or more steps", i+1)
				if want.step1 != "" {
					assert.Equal(t, want.step1, connects[0].Outcome, "turn %d: step 1 connect outcome (error %q)", i+1, connects[0].Error)
					offered := e.offeredTools(marker)
					if want.step1Tools {
						assert.Positive(t, connects[0].ToolCount, "turn %d: step 1 tool count", i+1)
						assert.Contains(t, offered, reconnectSlug+"__echo", "turn %d: step 1 tools", i+1)
					} else {
						assert.Zero(t, connects[0].ToolCount, "turn %d: step 1 tool count", i+1)
						assert.NotContains(t, offered, reconnectSlug+"__echo", "turn %d: step 1 tools", i+1)
					}
				}
				if want.noAuthorization {
					turnRequests := mcpSrv.log()[requestsBefore:]
					require.NotEmpty(t, turnRequests)
					var sent []string
					for _, req := range turnRequests {
						if req.authorization != "" {
							sent = append(sent, req.authorization)
						}
					}
					assert.Empty(t, sent, "turn %d: a dead token must not be sent", i+1)
				}
			}

			if !row.skipFirstRejection {
				requireStoredTokenRejectedFirst(t, mcpSrv.log())
			}
			tok := e.tokenRow()
			switch row.wantRow {
			case tokenRowIssued:
				// Every token endpoint call of these rows issues a new pair,
				// so the row holds the pair of the last call.
				issued := tokens.calls.Load()
				assert.Equal(t, fmt.Sprintf("issued-access-%d", issued), tok.AccessToken)
				assert.Equal(t, fmt.Sprintf("issued-refresh-%d", issued), tok.RefreshToken)
			case tokenRowCleared:
				assert.Empty(t, tok.AccessToken)
				assert.Empty(t, tok.RefreshToken)
			default:
				assert.Equal(t, reconnectStoredToken, tok.AccessToken)
				assert.Equal(t, refreshToken, tok.RefreshToken)
			}
			if row.reasonPrefix {
				assert.True(t, strings.HasPrefix(tok.OauthRefreshFailureReason, row.wantReason),
					"failure reason %q must start with %q", tok.OauthRefreshFailureReason, row.wantReason)
			} else {
				assert.Equal(t, row.wantReason, tok.OauthRefreshFailureReason)
			}
			connected, listCalls := e.authConnected()
			assert.Equal(t, row.wantConnected, connected, "auth_connected")
			assert.Equal(t, row.wantListCalls, listCalls, "token endpoint calls from the list")
		})
	}

	// Rows 12 and 13: a message sent while turn 1 runs becomes a new prompt
	// on the same runner. A queued message becomes turn 2 when turn 1
	// finishes; a steered message goes into history before turn 1's next
	// model call and is the prompt of every later step. The token endpoint
	// answers 503 and the token is never marked, so each prompt spends its
	// own forced refresh: one failed attempt, 2 calls.
	for _, row := range []struct {
		name string
		busy codersdk.ChatBusyBehavior
	}{
		{name: "12 queued message on the same runner", busy: codersdk.ChatBusyBehaviorQueue},
		{name: "13 steered message on the same runner", busy: codersdk.ChatBusyBehaviorSteer},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			mcpSrv := newRejectingMCPServer(t, nil)
			tokens := newReconnectTokenServer(t, tokenAnswerUnavailable, mcpSrv)
			e := newReconnectEnv(t, mcpSrv, tokens)
			e.seedToken(reconnectStoredRT, hourAhead)
			hold := chattest.NewGate()
			defer hold.Open()
			e.holdFirstModelRequests(hold)

			ctx := testutil.Context(t, testutil.WaitSuperLong)
			chatID, _ := e.startTurn(uuid.Nil)
			select {
			case <-hold.Arrived(1):
			case <-ctx.Done():
				t.Fatal("turn 1 never reached the model")
			}
			// Turn 1's first step has connected and refreshed by now.
			firstCalls := tokens.calls.Load()
			marker := e.sendWhileBusy(chatID, row.busy)
			hold.Open()
			e.waitTurn(chatID, marker)

			assert.EqualValues(t, 2, firstCalls, "token endpoint calls in turn 1's first step")
			assert.EqualValues(t, 2, tokens.calls.Load()-firstCalls, "token endpoint calls after it, the new prompt included")
			requireStoredTokenRejectedFirst(t, mcpSrv.log())
			tok := e.tokenRow()
			assert.Equal(t, reconnectStoredToken, tok.AccessToken)
			assert.Equal(t, reconnectStoredRT, tok.RefreshToken)
			assert.Empty(t, tok.OauthRefreshFailureReason)
			connected, listCalls := e.authConnected()
			assert.True(t, connected)
			assert.Zero(t, listCalls)
		})
	}

	// Row 9: two turns for the same user hit the 401 at the same time. Both
	// turns wait at their token read until the test releases them
	// together, so both connect with the stored token and the turn that
	// refreshes second must take the first turn's refresh instead of
	// spending the refresh token again. Every other wait between a turn's
	// token read and its save has a deadline (the connect, the lease wait
	// and the token request), so a hold there would race the other turn's
	// start.
	t.Run("9 two concurrent turns", func(t *testing.T) {
		t.Parallel()
		mcpSrv := newRejectingMCPServer(t, nil)
		tokens := newReconnectTokenServer(t, tokenAnswerRotate, mcpSrv)
		e := newReconnectEnv(t, mcpSrv, tokens)
		e.seedToken(reconnectStoredRT, hourAhead)

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		unlock := e.lockTokenTable(ctx)
		defer unlock()
		chatA, markerA := e.startTurn(uuid.Nil)
		chatB, markerB := e.startTurn(uuid.Nil)
		e.waitTokenReads(ctx, 2)
		unlock()
		e.waitTurn(chatA, markerA)
		e.waitTurn(chatB, markerB)
		assert.EqualValues(t, 1, tokens.calls.Load(), "token endpoint calls across both turns")

		stored := "Bearer " + reconnectStoredToken
		for _, turn := range []struct {
			chatID uuid.UUID
			marker string
		}{{chatA, markerA}, {chatB, markerB}} {
			var first mcpServerRequest
			for _, req := range mcpSrv.log() {
				if req.chatID == turn.chatID.String() {
					first = req
					break
				}
			}
			assert.Equal(t, stored, first.authorization, "chat %s: first MCP request token", turn.chatID)
			assert.True(t, first.rejected, "chat %s: first MCP request must get a 401", turn.chatID)
			connects := e.lastTurnConnects(turn.chatID)
			t.Logf("chat %s: steps=%d step1=%+v", turn.chatID, len(connects), connects[0])
			assert.Equal(t, "connected", connects[0].Outcome, "chat %s: step 1 connect outcome (error %q)", turn.chatID, connects[0].Error)
			assert.Positive(t, connects[0].ToolCount)
			assert.Contains(t, e.offeredTools(turn.marker), reconnectSlug+"__echo")
		}

		chatC, markerC := e.startTurn(uuid.Nil)
		e.waitTurn(chatC, markerC)
		assert.EqualValues(t, 1, tokens.calls.Load(), "a third turn needs no refresh")
		assert.Contains(t, e.offeredTools(markerC), reconnectSlug+"__echo")
		tok := e.tokenRow()
		assert.Equal(t, "issued-access-1", tok.AccessToken)
		assert.Equal(t, "issued-refresh-1", tok.RefreshToken)
		assert.Empty(t, tok.OauthRefreshFailureReason)
		connected, listCalls := e.authConnected()
		assert.True(t, connected)
		assert.Zero(t, listCalls)
	})
}

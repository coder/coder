package mcpbuiltin_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/mcpbuiltin"
	"github.com/coder/safedial"
)

const (
	testSigningSecret = "0123456789abcdef0123456789abcdef"
	testSlackToken    = "xoxb-test-token" //nolint:gosec // Fake token for a fake Slack API.
	testGitHubToken   = "ghp_test_token"  //nolint:gosec // Fake token for a fake GitHub API.
)

var loopbackPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("::1/128"),
}

type fakeResolver struct {
	threads map[uuid.UUID]mcpbuiltin.SlackThread
	err     error
}

func (f fakeResolver) SlackThread(_ context.Context, identity mcpbuiltin.Identity) (mcpbuiltin.SlackThread, bool, error) {
	if f.err != nil {
		return mcpbuiltin.SlackThread{}, false, f.err
	}
	thread, ok := f.threads[identity.ChatID]
	return thread, ok, nil
}

// testEnv runs the built-in servers below the same path coderd uses.
type testEnv struct {
	server  *httptest.Server
	handler *mcpbuiltin.Handler
	chatID  uuid.UUID
	ownerID uuid.UUID
	// failingChatID makes the signing secret lookup fail, like a
	// database error would.
	failingChatID uuid.UUID
}

// internalErrorText is the text of internal errors that must never reach
// callers.
const internalErrorText = "pq: connection to 10.1.2.3 refused"

func (e testEnv) endpoint(server string, query url.Values) string {
	u := e.server.URL + mcpbuiltin.EndpointPath(server)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// newTestEnv starts the built-in servers. When origin is set, it is the
// Slack thread of the env's chat.
func newTestEnv(t *testing.T, origin *mcpbuiltin.SlackThread) testEnv {
	t.Helper()
	return newTestEnvWithResolverError(t, origin, nil)
}

func newTestEnvWithResolverError(t *testing.T, origin *mcpbuiltin.SlackThread, resolverErr error) testEnv {
	t.Helper()
	env := testEnv{chatID: uuid.New(), ownerID: uuid.New(), failingChatID: uuid.New()}
	threads := map[uuid.UUID]mcpbuiltin.SlackThread{}
	if origin != nil {
		threads[env.chatID] = *origin
	}
	h := mcpbuiltin.New(mcpbuiltin.Options{
		Logger:            slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		AccessURL:         &url.URL{Scheme: "https", Host: "coder.example.com"},
		AllowedPrefixes:   loopbackPrefixes,
		AllowInsecureHTTP: true,
		Identity: mcpbuiltin.HMACVerifier{
			Secrets: func(_ context.Context, _ string, chatID uuid.UUID) ([]string, error) {
				if chatID == env.failingChatID {
					return nil, xerrors.New(internalErrorText)
				}
				if chatID != env.chatID {
					return nil, nil
				}
				return []string{"unrelated-secret", testSigningSecret}, nil
			},
		},
		Conversations: fakeResolver{threads: threads, err: resolverErr},
	})
	r := chi.NewRouter()
	r.Mount(mcpbuiltin.BasePath, h)
	env.handler = h
	env.server = httptest.NewServer(r)
	t.Cleanup(env.server.Close)
	return env
}

// signingTransport adds the bearer token and, when secret is set, the
// signed Coder identity headers chatd sends.
type signingTransport struct {
	token   string
	secret  string
	headers map[string]string
	// base sends the request; nil means http.DefaultTransport.
	base http.RoundTripper
}

func (s signingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}
	if s.secret != "" {
		var body []byte
		if req.Body != nil {
			var err error
			body, err = io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		hash := sha256.Sum256(body)
		canonical := strings.Join([]string{
			"v1", ts, req.Method, req.URL.RequestURI(), hex.EncodeToString(hash[:]),
			"owner=" + req.Header.Get(mcpbuiltin.HeaderCoderOwnerID),
			"chat=" + req.Header.Get(mcpbuiltin.HeaderCoderChatID),
			"subchat=" + req.Header.Get(mcpbuiltin.HeaderCoderSubchatID),
			"workspace=" + req.Header.Get(mcpbuiltin.HeaderCoderWorkspaceID),
		}, "\n")
		mac := hmac.New(sha256.New, []byte(s.secret))
		_, _ = mac.Write([]byte(canonical))
		req.Header.Set(mcpbuiltin.HeaderCoderSignatureTimestamp, ts)
		req.Header.Set(mcpbuiltin.HeaderCoderSignature, "v1="+hex.EncodeToString(mac.Sum(nil)))
	}
	if s.base != nil {
		return s.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func (e testEnv) signed(token string) signingTransport {
	return signingTransport{
		token:  token,
		secret: testSigningSecret,
		headers: map[string]string{
			mcpbuiltin.HeaderCoderOwnerID: e.ownerID.String(),
			mcpbuiltin.HeaderCoderChatID:  e.chatID.String(),
		},
	}
}

func connect(t *testing.T, endpoint string, rt http.RoundTripper) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: rt},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			_, _ = text.WriteString(tc.Text)
		}
	}
	return text.String(), res.IsError
}

type slackCall struct {
	method string
	auth   string
	form   url.Values
}

// fakeSlack records Slack Web API calls and answers from responses,
// defaulting to {"ok":true}.
type fakeSlack struct {
	server    *httptest.Server
	mu        sync.Mutex
	calls     []slackCall
	responses map[string]string
	// handle, when set, answers calls it returns true for. Guarded by mu.
	handle func(method string, form url.Values) (string, bool)
}

func newFakeSlack(t *testing.T, responses map[string]string) *fakeSlack {
	t.Helper()
	f := &fakeSlack{responses: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, r.ParseForm()) {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		method := strings.TrimPrefix(r.URL.Path, "/api/")
		f.mu.Lock()
		f.calls = append(f.calls, slackCall{method: method, auth: r.Header.Get("Authorization"), form: r.PostForm})
		resp, ok := f.responses[method]
		if f.handle != nil {
			if r, handled := f.handle(method, r.PostForm); handled {
				resp, ok = r, true
			}
		}
		f.mu.Unlock()
		if !ok {
			resp = `{"ok":true}`
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(resp))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeSlack) query(extra ...string) url.Values {
	q := url.Values{mcpbuiltin.SlackOptionAPIBaseURL: {f.server.URL + "/api"}}
	for i := 0; i+1 < len(extra); i += 2 {
		q.Set(extra[i], extra[i+1])
	}
	return q
}

func (f *fakeSlack) callsFor(method string) []slackCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []slackCall
	for _, c := range f.calls {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

func TestMissingToken(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, nil)
	for _, server := range []string{mcpbuiltin.ServerSlack, mcpbuiltin.ServerGitHub} {
		t.Run(server, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.endpoint(server, nil), strings.NewReader(`{}`))
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Bearer")
		})
	}
}

// postStatus sends one raw MCP request and returns its status and body.
func postStatus(t *testing.T, endpoint string, rt http.RoundTripper) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(`{}`))
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: rt}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestSignatureRequired(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, nil)
	for _, server := range []string{mcpbuiltin.ServerSlack, mcpbuiltin.ServerGitHub} {
		t.Run(server, func(t *testing.T) {
			t.Parallel()
			// A token alone, even for the default upstream, is not
			// enough: coderd must not relay anonymous requests.
			status, body := postStatus(t, env.endpoint(server, nil), signingTransport{token: testSlackToken})
			assert.Equal(t, http.StatusUnauthorized, status, body)
			assert.Contains(t, body, "Missing Coder identity signature")

			// Identity headers without a signature are not trusted.
			unsigned := env.signed(testSlackToken)
			unsigned.secret = ""
			status, _ = postStatus(t, env.endpoint(server, nil), unsigned)
			assert.Equal(t, http.StatusUnauthorized, status)
		})
	}

	t.Run("InProcess", func(t *testing.T) {
		t.Parallel()
		accessURL := &url.URL{Scheme: "https", Host: "coder.invalid"}
		client := mcpclient.WithInProcessRoute(mcpclient.NewHTTPClient(nil), env.handler.InProcessRoute(accessURL))
		endpoint := accessURL.JoinPath(mcpbuiltin.EndpointPath(mcpbuiltin.ServerSlack)).String()

		status, body := postStatus(t, endpoint, signingTransport{token: testSlackToken, base: client.Transport})
		assert.Equal(t, http.StatusUnauthorized, status, body)
		rt := env.signed(testSlackToken)
		rt.base = client.Transport
		status, body = postStatus(t, endpoint, rt)
		assert.NotEqual(t, http.StatusUnauthorized, status, body)
	})
}

func TestInvalidSignature(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, nil)

	t.Run("WrongSecret", func(t *testing.T) {
		t.Parallel()
		rt := env.signed(testSlackToken)
		rt.secret = "wrong-secret"
		status, body := postStatus(t, env.endpoint(mcpbuiltin.ServerSlack, nil), rt)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Contains(t, body, "Invalid Coder identity signature")
	})

	t.Run("LookupErrorIsNotShown", func(t *testing.T) {
		t.Parallel()
		rt := env.signed(testSlackToken)
		rt.headers[mcpbuiltin.HeaderCoderChatID] = env.failingChatID.String()
		status, body := postStatus(t, env.endpoint(mcpbuiltin.ServerSlack, nil), rt)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.NotContains(t, body, internalErrorText)
		assert.NotContains(t, body, "10.1.2.3")
	})
}

func TestBadOptions(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, nil)
	tests := []struct {
		name   string
		server string
		query  url.Values
		detail string
	}{
		{name: "UnknownOption", server: mcpbuiltin.ServerSlack, query: url.Values{"api_url": {"https://slack.com/api"}}, detail: "unknown option"},
		{name: "InsecureScheme", server: mcpbuiltin.ServerSlack, query: url.Values{"api_base_url": {"http://slack.example.com/api"}}, detail: "must use https"},
		{name: "PrivateAddress", server: mcpbuiltin.ServerSlack, query: url.Values{"api_base_url": {"https://10.0.0.1/api"}}, detail: "blocked address"},
		{name: "LinkLocalAddress", server: mcpbuiltin.ServerGitHub, query: url.Values{"api_base_url": {"https://169.254.169.254/api/v3"}}, detail: "blocked address"},
		{name: "Credentials", server: mcpbuiltin.ServerGitHub, query: url.Values{"api_base_url": {"https://user:pass@ghe.example.com/api/v3"}}, detail: "credentials"},
		{name: "BadPinThread", server: mcpbuiltin.ServerSlack, query: url.Values{"pin_thread": {"maybe"}}, detail: "true or false"},
		{name: "UnknownToolset", server: mcpbuiltin.ServerGitHub, query: url.Values{"toolsets": {"repos,bogus"}}, detail: "bogus"},
		{name: "EmptyToolsets", server: mcpbuiltin.ServerGitHub, query: url.Values{"toolsets": {" , "}}, detail: "at least one toolset"},
		{name: "GHESPath", server: mcpbuiltin.ServerGitHub, query: url.Values{"api_base_url": {"https://ghe.example.com/v3"}}, detail: "/api/v3"},
		{name: "GitHubWebURL", server: mcpbuiltin.ServerGitHub, query: url.Values{"api_base_url": {"https://github.com"}}, detail: "REST API URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, body := postStatus(t, env.endpoint(tt.server, tt.query), env.signed(testSlackToken))
			assert.Equal(t, http.StatusBadRequest, status, body)
			assert.Contains(t, body, tt.detail)
		})
	}
}

func TestSlackTools(t *testing.T) {
	t.Parallel()

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, nil)
		slack := newFakeSlack(t, nil)
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))
		res, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		var names []string
		for _, tool := range res.Tools {
			names = append(names, tool.Name)
		}
		assert.ElementsMatch(t, []string{
			"slack_send_message",
			"slack_report_status",
			"slack_react_to_message",
			"slack_read_thread_replies",
			"slack_get_user_info",
			"slack_search_messages",
		}, names)
	})

	t.Run("DefaultsToOriginThread", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, &mcpbuiltin.SlackThread{TeamID: "T1", Channel: "C123", ThreadTS: "1700000000.000100"})
		chatID := env.chatID
		slack := newFakeSlack(t, map[string]string{
			"chat.postMessage":      `{"ok":true,"channel":"C123","ts":"1700000000.000200"}`,
			"conversations.replies": `{"ok":true,"messages":[{"ts":"1700000000.000100","text":"help"}],"has_more":false}`,
		})
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))

		text, isErr := callTool(t, session, "slack_send_message", map[string]any{
			"message": "**Done**: see [the PR](https://github.com/coder/coder/pull/1)",
			"actions": []map[string]any{{"text": "Open PR", "url": "https://github.com/coder/coder/pull/1"}},
		})
		require.False(t, isErr, text)
		posts := slack.callsFor("chat.postMessage")
		require.Len(t, posts, 1)
		post := posts[0]
		assert.Equal(t, "Bearer "+testSlackToken, post.auth)
		assert.Equal(t, "C123", post.form.Get("channel"))
		assert.Equal(t, "1700000000.000100", post.form.Get("thread_ts"))
		assert.Equal(t, "*Done*: see <https://github.com/coder/coder/pull/1|the PR>", post.form.Get("text"))
		blocks := post.form.Get("blocks")
		assert.Contains(t, blocks, `"type":"actions"`)
		assert.Contains(t, blocks, "https://coder.example.com/agents/"+chatID.String()+"|View chat")
		// Sending clears the thread status.
		statuses := slack.callsFor("assistant.threads.setStatus")
		require.Len(t, statuses, 1)
		assert.Equal(t, "", statuses[0].form.Get("status"))
		assert.Equal(t, "C123", statuses[0].form.Get("channel_id"))

		text, isErr = callTool(t, session, "slack_react_to_message", map[string]any{"reaction": ":eyes:"})
		require.False(t, isErr, text)
		reactions := slack.callsFor("reactions.add")
		require.Len(t, reactions, 1)
		assert.Equal(t, "eyes", reactions[0].form.Get("name"))
		assert.Equal(t, "C123", reactions[0].form.Get("channel"))
		assert.Equal(t, "1700000000.000100", reactions[0].form.Get("timestamp"))

		text, isErr = callTool(t, session, "slack_read_thread_replies", nil)
		require.False(t, isErr, text)
		assert.Contains(t, text, `"text":"help"`)
		replies := slack.callsFor("conversations.replies")
		require.Len(t, replies, 1)
		assert.Equal(t, "C123", replies[0].form.Get("channel"))
		assert.Equal(t, "1700000000.000100", replies[0].form.Get("ts"))
		assert.Equal(t, "50", replies[0].form.Get("limit"))

		text, isErr = callTool(t, session, "slack_report_status", map[string]any{
			"status":           "Looking into it...",
			"loading_messages": []string{"Reading code...", "Running tests..."},
		})
		require.False(t, isErr, text)
		statuses = slack.callsFor("assistant.threads.setStatus")
		require.Len(t, statuses, 2)
		assert.Equal(t, "Looking into it...", statuses[1].form.Get("status"))
		assert.Equal(t, "1700000000.000100", statuses[1].form.Get("thread_ts"))
		assert.JSONEq(t, `["Reading code...","Running tests..."]`, statuses[1].form.Get("loading_messages"))
	})

	t.Run("PinThread", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, &mcpbuiltin.SlackThread{Channel: "C123", ThreadTS: "1.1"})
		slack := newFakeSlack(t, nil)
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query(mcpbuiltin.SlackOptionPinThread, "true")), env.signed(testSlackToken))

		text, isErr := callTool(t, session, "slack_send_message", map[string]any{"message": "hi", "channel": "C999"})
		require.True(t, isErr)
		assert.Contains(t, text, "only acts on the Slack thread")
		assert.Empty(t, slack.callsFor("chat.postMessage"))

		text, isErr = callTool(t, session, "slack_send_message", map[string]any{"message": "hi"})
		require.False(t, isErr, text)
		require.Len(t, slack.callsFor("chat.postMessage"), 1)
	})

	t.Run("PinThreadWithoutOrigin", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, nil)
		slack := newFakeSlack(t, nil)
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query(mcpbuiltin.SlackOptionPinThread, "true")), env.signed(testSlackToken))

		for name, args := range map[string]map[string]any{
			"slack_send_message":        {"message": "hi", "channel": "C42"},
			"slack_report_status":       {"status": "working", "channel": "C42", "thread_ts": "1.1"},
			"slack_react_to_message":    {"reaction": "eyes", "channel": "C42", "ts": "1.1"},
			"slack_read_thread_replies": {"channel": "C42", "thread_ts": "1.1"},
		} {
			text, isErr := callTool(t, session, name, args)
			require.True(t, isErr, name)
			assert.Contains(t, text, "did not start from one", name)
		}
		slack.mu.Lock()
		defer slack.mu.Unlock()
		assert.Empty(t, slack.calls)
	})

	t.Run("PinThreadReactions", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, &mcpbuiltin.SlackThread{Channel: "C123", ThreadTS: "1.1"})
		slack := newFakeSlack(t, nil)
		slack.mu.Lock()
		slack.handle = func(method string, form url.Values) (string, bool) {
			if method != "conversations.replies" {
				return "", false
			}
			// Slack returns the thread parent plus the requested reply
			// when it exists in the thread.
			messages := `{"ts":"1.1","thread_ts":"1.1"}`
			if form.Get("oldest") == "1.5" {
				messages += `,{"ts":"1.5","thread_ts":"1.1"}`
			}
			return `{"ok":true,"messages":[` + messages + `]}`, true
		}
		slack.mu.Unlock()
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query(mcpbuiltin.SlackOptionPinThread, "true")), env.signed(testSlackToken))

		text, isErr := callTool(t, session, "slack_react_to_message", map[string]any{"reaction": "eyes"})
		require.False(t, isErr, text)
		text, isErr = callTool(t, session, "slack_react_to_message", map[string]any{"reaction": "eyes", "ts": "1.5"})
		require.False(t, isErr, text)

		text, isErr = callTool(t, session, "slack_react_to_message", map[string]any{"reaction": "eyes", "ts": "9.9"})
		require.True(t, isErr)
		assert.Contains(t, text, "only reacts to messages in the Slack thread")
		text, isErr = callTool(t, session, "slack_react_to_message", map[string]any{"reaction": "eyes", "channel": "C999", "ts": "1.1"})
		require.True(t, isErr)
		assert.Contains(t, text, "only acts on the Slack thread")

		reactions := slack.callsFor("reactions.add")
		require.Len(t, reactions, 2)
		assert.Equal(t, "1.1", reactions[0].form.Get("timestamp"))
		assert.Equal(t, "1.5", reactions[1].form.Get("timestamp"))
		for _, r := range reactions {
			assert.Equal(t, "C123", r.form.Get("channel"))
		}
		replies := slack.callsFor("conversations.replies")
		require.Len(t, replies, 2, "the thread root needs no lookup")
		assert.Equal(t, "1.1", replies[0].form.Get("ts"))
		assert.Equal(t, "true", replies[0].form.Get("inclusive"))
	})

	t.Run("BroadcastMentions", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, &mcpbuiltin.SlackThread{Channel: "C123", ThreadTS: "1.1"})
		slack := newFakeSlack(t, nil)
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))

		const message = "<!here> deploy done <!channel> <!everyone|everyone> <!subteam^S123|@oncall> `<!here>` <@U01234ABCDE>"
		text, isErr := callTool(t, session, "slack_send_message", map[string]any{"message": message})
		require.False(t, isErr, text)
		text, isErr = callTool(t, session, "slack_send_message", map[string]any{"message": "<!here> heads up", "allow_broadcast": true})
		require.False(t, isErr, text)

		posts := slack.callsFor("chat.postMessage")
		require.Len(t, posts, 2)
		assert.Equal(t, "@here deploy done @channel @everyone @oncall `@here` <@U01234ABCDE>", posts[0].form.Get("text"))
		assert.NotContains(t, posts[0].form.Get("blocks"), "<!")
		assert.Equal(t, "<!here> heads up", posts[1].form.Get("text"))
	})

	t.Run("ResolverErrorIsNotShown", func(t *testing.T) {
		t.Parallel()
		env := newTestEnvWithResolverError(t, nil, xerrors.New(internalErrorText))
		slack := newFakeSlack(t, nil)
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))
		text, isErr := callTool(t, session, "slack_send_message", map[string]any{"message": "hi"})
		require.True(t, isErr)
		assert.Contains(t, text, "Could not look up the Slack thread")
		assert.NotContains(t, text, "10.1.2.3")
	})

	t.Run("NoOriginNeedsChannel", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, nil)
		slack := newFakeSlack(t, nil)
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))

		text, isErr := callTool(t, session, "slack_send_message", map[string]any{"message": "hi"})
		require.True(t, isErr)
		assert.Contains(t, text, "channel is required")

		text, isErr = callTool(t, session, "slack_send_message", map[string]any{"message": "hi", "channel": "C42"})
		require.False(t, isErr, text)
		posts := slack.callsFor("chat.postMessage")
		require.Len(t, posts, 1)
		assert.Equal(t, "C42", posts[0].form.Get("channel"))
		assert.Empty(t, posts[0].form.Get("thread_ts"))
		// A top-level post has no thread status to clear.
		assert.Empty(t, slack.callsFor("assistant.threads.setStatus"))
	})

	t.Run("LookupTools", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, nil)
		slack := newFakeSlack(t, map[string]string{
			"users.info":      `{"ok":true,"user":{"id":"U123","name":"alex"}}`,
			"search.messages": `{"ok":true,"messages":{"total":1,"matches":[{"text":"deploy done"}]}}`,
		})
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))

		text, isErr := callTool(t, session, "slack_get_user_info", map[string]any{"user_id": "U123"})
		require.False(t, isErr, text)
		assert.JSONEq(t, `{"id":"U123","name":"alex"}`, text)

		text, isErr = callTool(t, session, "slack_search_messages", map[string]any{"query": "deploy", "count": 500})
		require.False(t, isErr, text)
		assert.Contains(t, text, "deploy done")
		searches := slack.callsFor("search.messages")
		require.Len(t, searches, 1)
		assert.Equal(t, "100", searches[0].form.Get("count"))
	})

	t.Run("SlackErrorIsToolError", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, nil)
		slack := newFakeSlack(t, map[string]string{
			"chat.postMessage": `{"ok":false,"error":"missing_scope","needed":"chat:write"}`,
		})
		session := connect(t, env.endpoint(mcpbuiltin.ServerSlack, slack.query()), env.signed(testSlackToken))
		text, isErr := callTool(t, session, "slack_send_message", map[string]any{"message": "hi", "channel": "C1"})
		require.True(t, isErr)
		assert.Contains(t, text, "missing_scope")
		assert.Contains(t, text, "chat:write")
		assert.NotContains(t, text, testSlackToken)
	})
}

func TestGitHubTools(t *testing.T) {
	t.Parallel()

	type ghCall struct {
		method, path, auth string
	}
	var (
		mu    sync.Mutex
		calls []ghCall
	)
	gh := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, ghCall{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization")})
		mu.Unlock()
		rw.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/user":
			_, _ = rw.Write([]byte(`{"login":"octocat","id":1,"html_url":"https://ghe.example.com/octocat"}`))
		case "/api/v3/repos/coder/coder/branches":
			rw.WriteHeader(http.StatusNotFound)
			_, _ = rw.Write([]byte(`{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`))
		default:
			rw.WriteHeader(http.StatusNotFound)
			_, _ = rw.Write([]byte(`{"message":"Not Found"}`))
		}
	}))
	t.Cleanup(gh.Close)

	env := newTestEnv(t, nil)
	apiBase := gh.URL + "/api/v3"

	t.Run("ToolsetsAndCalls", func(t *testing.T) {
		t.Parallel()
		session := connect(t, env.endpoint(mcpbuiltin.ServerGitHub, url.Values{
			mcpbuiltin.GitHubOptionAPIBaseURL: {apiBase},
			mcpbuiltin.GitHubOptionToolsets:   {"context,repos"},
		}), env.signed(testGitHubToken))

		res, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		names := map[string]bool{}
		for _, tool := range res.Tools {
			names[tool.Name] = true
		}
		assert.True(t, names["get_me"], "context toolset")
		assert.True(t, names["create_or_update_file"], "repos toolset")
		assert.False(t, names["list_issues"], "issues toolset is not selected")

		text, isErr := callTool(t, session, "get_me", map[string]any{})
		require.False(t, isErr, text)
		assert.Contains(t, text, "octocat")

		text, isErr = callTool(t, session, "list_branches", map[string]any{"owner": "coder", "repo": "coder"})
		require.True(t, isErr, text)
		assert.Contains(t, text, "404")
		assert.NotContains(t, text, testGitHubToken)

		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, calls)
		for _, c := range calls {
			assert.Equal(t, "Bearer "+testGitHubToken, c.auth, c.path)
		}
		assert.Equal(t, "/api/v3/user", calls[0].path)
	})

	t.Run("ReadOnly", func(t *testing.T) {
		t.Parallel()
		session := connect(t, env.endpoint(mcpbuiltin.ServerGitHub, url.Values{
			mcpbuiltin.GitHubOptionAPIBaseURL: {apiBase},
			mcpbuiltin.GitHubOptionToolsets:   {"repos"},
			mcpbuiltin.GitHubOptionReadOnly:   {"true"},
		}), env.signed(testGitHubToken))
		res, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		names := map[string]bool{}
		for _, tool := range res.Tools {
			names[tool.Name] = true
		}
		assert.True(t, names["get_file_contents"])
		assert.False(t, names["create_or_update_file"])
	})

	t.Run("DefaultToolsets", func(t *testing.T) {
		t.Parallel()
		// Listing tools makes no upstream call to api.github.com.
		session := connect(t, env.endpoint(mcpbuiltin.ServerGitHub, nil), env.signed(testGitHubToken))
		res, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		names := map[string]bool{}
		for _, tool := range res.Tools {
			names[tool.Name] = true
		}
		assert.True(t, names["list_issues"])
		assert.True(t, names["get_me"])
	})
}

// TestChatdSignedRequests checks that the signature chatd's MCP client
// sends verifies here, so chat context works end to end, both over the
// network and through the in-process route chatd uses for the access URL.
func TestChatdSignedRequests(t *testing.T) {
	t.Parallel()

	for _, inProcess := range []bool{false, true} {
		name := "Network"
		if inProcess {
			name = "InProcess"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t, &mcpbuiltin.SlackThread{Channel: "C777", ThreadTS: "1700000000.000300"})
			slack := newFakeSlack(t, nil)

			endpoint := env.endpoint(mcpbuiltin.ServerSlack, slack.query())
			httpClient := mcpclient.NewHTTPClient(nil, safedial.WithAllowedPrefixes(loopbackPrefixes...))
			if inProcess {
				// The access URL does not resolve, so the request only
				// succeeds if it never leaves the process.
				accessURL := &url.URL{Scheme: "https", Host: "coder.invalid"}
				u, err := url.Parse(endpoint)
				require.NoError(t, err)
				u.Scheme, u.Host = accessURL.Scheme, accessURL.Host
				endpoint = u.String()
				httpClient = mcpclient.WithInProcessRoute(httpClient, env.handler.InProcessRoute(accessURL))
			}

			cfg := mcpclient.Server{
				ID:                  uuid.New(),
				Slug:                "slack",
				URL:                 endpoint,
				Transport:           mcpclient.TransportStreamableHTTP,
				Headers:             map[string]string{"Authorization": "Bearer " + testSlackToken},
				ForwardCoderHeaders: true,
				SigningSecret:       testSigningSecret,
			}
			coderHeaders := chatprovider.CoderHeaders(database.Chat{ID: env.chatID, OwnerID: env.ownerID})
			tools, _, cleanup := mcpclient.ConnectAll(
				t.Context(), slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				[]mcpclient.Server{cfg}, nil, uuid.Nil, nil, coderHeaders, httpClient,
			)
			t.Cleanup(cleanup)

			var send fantasy.AgentTool
			for _, tool := range tools {
				if tool.Info().Name == "slack__slack_send_message" {
					send = tool
				}
			}
			require.NotNil(t, send, "chatd should discover the Slack tools")
			resp, err := send.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "slack__slack_send_message", Input: `{"message":"hi"}`})
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)

			posts := slack.callsFor("chat.postMessage")
			require.Len(t, posts, 1)
			assert.Equal(t, "C777", posts[0].form.Get("channel"))
			assert.Equal(t, "1700000000.000300", posts[0].form.Get("thread_ts"))
		})
	}
}

func TestParseSlackConversationKey(t *testing.T) {
	t.Parallel()
	thread, err := mcpbuiltin.ParseSlackConversationKey("T1/C2/1700000000.000100")
	require.NoError(t, err)
	assert.Equal(t, mcpbuiltin.SlackThread{TeamID: "T1", Channel: "C2", ThreadTS: "1700000000.000100"}, thread)

	for _, bad := range []string{"", "T1/C2", "T1//1.2", "T1/C2/", "a/b/c/d"} {
		_, err := mcpbuiltin.ParseSlackConversationKey(bad)
		assert.Error(t, err, bad)
	}
}

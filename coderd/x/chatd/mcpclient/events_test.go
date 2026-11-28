package mcpclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

type eventsTestParams struct {
	mcp.ParamsBase
	Name   string  `json:"name"`
	Cursor *string `json:"cursor"`
}

type eventsTestList struct {
	mcp.ResultBase
	Events     []mcpclient.EventType `json:"events"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

type eventsTestEmpty struct{ mcp.ResultBase }

type eventsTestRequest struct {
	method string
	header http.Header
	params map[string]json.RawMessage
}

// eventsTestServer is an MCP server that offers the events methods and
// records each events/* request it receives.
type eventsTestServer struct {
	url      string
	mu       sync.Mutex
	requests []eventsTestRequest
}

func (es *eventsTestServer) recorded() []eventsTestRequest {
	es.mu.Lock()
	defer es.mu.Unlock()
	return append([]eventsTestRequest(nil), es.requests...)
}

var eventsExtension = map[string]any{mcpclient.EventsExtensionID: map[string]any{}}

func newEventsTestServer(t *testing.T, extensions map[string]any) *eventsTestServer {
	t.Helper()
	opts := &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{Extensions: extensions}}
	srv := mcp.NewServer(&mcp.Implementation{Name: "events", Version: "1.0.0"}, opts)
	eventType := func(name, inputSchema string) mcpclient.EventType {
		return mcpclient.EventType{Name: name, Delivery: []mcpclient.DeliveryMode{mcpclient.DeliveryPoll}, InputSchema: json.RawMessage(inputSchema)}
	}
	annotated := func(n int) string {
		props := make([]string, n)
		for i := range props {
			props[i] = fmt.Sprintf(`"p%d":{"type":"string","x-mcp-header":"H%d"}`, i, i)
		}
		return `{"type":"object","properties":{` + strings.Join(props, ",") + `}}`
	}
	pages := map[string]*eventsTestList{
		"": {NextCursor: "page2", Events: []mcpclient.EventType{
			eventType("a.created", `{"type":"object","properties":{"region":{"type":"string","x-mcp-header":"Region"},"filter":{"type":"object","properties":{"team":{"type":"string","x-mcp-header":"Team"}}},"source":{"type":"object","properties":{"repo":{"type":"object","properties":{"owner":{"type":"object","properties":{"login":{"type":"string","x-mcp-header":"Login"},"kind":{"type":"string","x-mcp-header":"Kind"}}}}}}}}}`),
			eventType("bad.items", `{"type":"object","properties":{"tags":{"type":"array","items":{"type":"string","x-mcp-header":"Tag"}}}}`),
			eventType("bad.number", `{"type":"object","properties":{"limit":{"type":"number","x-mcp-header":"Limit"}}}`),
			eventType("bad.repeat", `{"type":"object","properties":{"a":{"type":"string","x-mcp-header":"Key"},"b":{"type":"string","x-mcp-header":"key"}}}`),
		}},
		"page2": {Events: []mcpclient.EventType{eventType("b.created", `{"type":"object"}`), eventType("bad.many", annotated(33))}},
	}
	// A type keeps at most 32 annotations and a list 1024: bad.many has
	// one too many, and full.31 would pass 1024 after a.created's 4.
	for i := range 32 {
		pages["page2"].Events = append(pages["page2"].Events, eventType(fmt.Sprintf("full.%d", i), annotated(32)))
	}
	require.NoError(t, errors.Join(
		mcp.AddReceivingCustomMethod(srv, "events/list", func(_ context.Context, _ *mcp.ServerSession, p *eventsTestParams) (*eventsTestList, error) {
			return pages[ptr.NilToEmpty(p.Cursor)], nil
		}),
		mcp.AddReceivingCustomMethod(srv, "events/poll", func(_ context.Context, _ *mcp.ServerSession, p *eventsTestParams) (*mcpclient.PollResult, error) {
			if p.Name == "forbidden" {
				return nil, &jsonrpc.Error{Code: -32024, Message: "Forbidden", Data: json.RawMessage(`{"reason":"revoked"}`)}
			}
			return &mcpclient.PollResult{Cursor: ptr.Ref("c1"), NextPollMs: 30000}, nil
		}),
		mcp.AddReceivingCustomMethod(srv, "events/subscribe", func(context.Context, *mcp.ServerSession, *eventsTestParams) (*mcpclient.SubscribeResult, error) {
			return &mcpclient.SubscribeResult{ID: "sub_1", Cursor: ptr.Ref("c1")}, nil
		}),
		mcp.AddReceivingCustomMethod(srv, "events/unsubscribe", func(context.Context, *mcp.ServerSession, *eventsTestParams) (*eventsTestEmpty, error) {
			return &eventsTestEmpty{}, nil
		}),
	))

	es := &eventsTestServer{}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var msg struct {
			ID     json.RawMessage            `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &msg)
		if strings.HasPrefix(msg.Method, "events/") {
			es.mu.Lock()
			es.requests = append(es.requests, eventsTestRequest{method: msg.Method, header: r.Header.Clone(), params: msg.Params})
			es.mu.Unlock()
			// A server that rejects the routing headers answers 400.
			if string(msg.Params["name"]) == `"mismatch"` {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32020,"message":"header mismatch"}}`, msg.ID)
				return
			}
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	es.url = ts.URL
	return es
}

func connectEventsTest(t *testing.T, es *eventsTestServer) *mcpclient.EventsSession {
	t.Helper()
	cfg := database.MCPServerConfig{ID: uuid.New(), Slug: "events", Url: es.url, Transport: "streamable_http", AuthType: "none", ForwardCoderHeaders: true}
	coderHeaders := map[string]string{"X-Coder-Owner-Id": "owner-1"}
	s, err := mcpclient.ConnectEvents(testutil.Context(t, testutil.WaitShort), slogtest.Make(t, nil), cfg, nil, coderHeaders, testMCPHTTPClient(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEventsRequests(t *testing.T) {
	t.Parallel()
	args := map[string]any{
		"region": "Hello, 世界", "filter": map[string]string{"team": "core"}, "other": 1,
		"source": map[string]any{"repo": map[string]any{"owner": map[string]any{"login": "octo", "kind": "org"}}},
	}
	hook := "https://coder.example.com/hook"
	cases := []struct {
		method string
		call   func(context.Context, *mcpclient.EventsSession) error
		params map[string]string
	}{
		{
			method: "events/poll",
			call: func(ctx context.Context, s *mcpclient.EventsSession) error {
				_, err := s.Poll(ctx, mcpclient.PollParams{Name: "a.created", Arguments: args})
				return err
			},
			params: map[string]string{"cursor": `null`},
		},
		{
			method: "events/subscribe",
			call: func(ctx context.Context, s *mcpclient.EventsSession) error {
				_, err := s.Subscribe(ctx, mcpclient.SubscribeParams{
					Name: "a.created", Arguments: args,
					Delivery: mcpclient.WebhookDelivery{URL: hook, Secret: "whsec_c2VjcmV0"},
				})
				return err
			},
			params: map[string]string{"cursor": `null`, "delivery": `{"mode":"webhook","url":"` + hook + `","secret":"whsec_c2VjcmV0"}`},
		},
		{
			method: "events/unsubscribe",
			call: func(ctx context.Context, s *mcpclient.EventsSession) error {
				return s.Unsubscribe(ctx, "a.created", args, hook)
			},
			params: map[string]string{"delivery": `{"url":"` + hook + `"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			es := newEventsTestServer(t, eventsExtension)
			s := connectEventsTest(t, es)
			require.NoError(t, tc.call(testutil.Context(t, testutil.WaitShort), s))

			requests := es.recorded()
			require.NotEmpty(t, requests)
			req := requests[len(requests)-1]
			require.Equal(t, tc.method, req.method)
			assert.Equal(t, tc.method, req.header.Get("Mcp-Method"))
			assert.Equal(t, "a.created", req.header.Get("Mcp-Name"))
			assert.Equal(t, "owner-1", req.header.Get("X-Coder-Owner-Id"))
			// The encoded form is the example in the transport's Value Encoding table.
			assert.Equal(t, "=?base64?SGVsbG8sIOS4lueVjA==?=", req.header.Get("Mcp-Param-Region"))
			assert.Equal(t, "core", req.header.Get("Mcp-Param-Team"))
			// Sibling leaves four levels deep each keep their own path.
			assert.Equal(t, "octo", req.header.Get("Mcp-Param-Login"))
			assert.Equal(t, "org", req.header.Get("Mcp-Param-Kind"))
			for key, want := range tc.params {
				assert.JSONEq(t, want, string(req.params[key]), key)
			}
		})
	}
}

func TestEventsList(t *testing.T) {
	t.Parallel()
	s := connectEventsTest(t, newEventsTestServer(t, eventsExtension))
	require.True(t, s.Supported())

	types, err := s.List(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	names := make([]string, 0, len(types))
	for _, et := range types {
		names = append(names, et.Name)
	}
	want := []string{"a.created", "b.created"}
	for i := range 31 {
		want = append(want, fmt.Sprintf("full.%d", i))
	}
	assert.Equal(t, want, names)
}

func TestEventsTokenForAnotherServer(t *testing.T) {
	t.Parallel()
	es := newEventsTestServer(t, eventsExtension)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := database.MCPServerConfig{ID: uuid.New(), Slug: "events", Url: es.url, Transport: "streamable_http", AuthType: "oauth2"}
	tok := &database.MCPServerUserToken{MCPServerConfigID: uuid.New(), AccessToken: "other-server-token"}
	s, err := mcpclient.ConnectEvents(ctx, slogtest.Make(t, nil), cfg, tok, nil, testMCPHTTPClient(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	_, err = s.List(ctx)
	require.NoError(t, err)
	requests := es.recorded()
	require.NotEmpty(t, requests)
	for _, req := range requests {
		assert.Empty(t, req.header.Get("Authorization"))
	}
}

func TestEventsUnsupported(t *testing.T) {
	t.Parallel()
	es := newEventsTestServer(t, nil)
	s := connectEventsTest(t, es)
	assert.False(t, s.Supported())

	_, err := s.Poll(testutil.Context(t, testutil.WaitShort), mcpclient.PollParams{Name: "a.created"})
	require.ErrorIs(t, err, mcpclient.ErrEventsUnsupported)
	assert.Empty(t, es.recorded())
}

func TestEventsErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		event    string
		code     int64
		data     string
		requests []string
	}{
		{event: "forbidden", code: -32024, data: `{"reason":"revoked"}`},
		// HeaderMismatch lists (two pages) again and retries once.
		{event: "mismatch", code: -32020, requests: []string{"events/list", "events/list", "events/poll", "events/list", "events/list", "events/poll"}},
	}
	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			t.Parallel()
			es := newEventsTestServer(t, eventsExtension)
			s := connectEventsTest(t, es)
			_, err := s.Poll(testutil.Context(t, testutil.WaitShort), mcpclient.PollParams{Name: tc.event})
			var eventsErr *mcpclient.EventsError
			require.ErrorAs(t, err, &eventsErr)
			assert.Equal(t, tc.code, eventsErr.Code)
			if tc.data != "" {
				assert.JSONEq(t, tc.data, string(eventsErr.Data))
			}
			if tc.requests != nil {
				var methods []string
				for _, req := range es.recorded() {
					methods = append(methods, req.method)
				}
				assert.Equal(t, tc.requests, methods)
			}
		})
	}
}

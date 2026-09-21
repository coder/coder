package mcpclient_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

func makeChatAttachedConfig(slug, url, headersJSON string) database.MCPServerConfig {
	cfg := makeConfig(slug, url)
	if headersJSON != "" && headersJSON != "{}" {
		cfg.AuthType = "custom_headers"
		cfg.CustomHeaders = headersJSON
	}
	return cfg
}

func manyTools(n int) []testTool {
	tools := make([]testTool, 0, n)
	for i := 0; i < n; i++ {
		tools = append(tools, testTool{
			tool: &mcp.Tool{
				Name:        fmt.Sprintf("tool_%d", i),
				Description: "no-op",
				InputSchema: map[string]any{"type": "object"},
			},
			handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return textToolResult("ok"), nil
			},
		})
	}
	return tools
}

func TestConnectChatAttached_Connects(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts, mu, recorded := newHeaderRecordingServer(t)
	cfg := makeChatAttachedConfig("bot", ts.URL, `{"X-Bot-Key":"secret"}`)

	tools, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
	)
	t.Cleanup(cleanup)

	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[0].Outcome)
	require.Equal(t, 1, summaries[0].ToolCount)
	require.Len(t, tools, 1)

	ident, ok := tools[0].(mcpclient.MCPToolIdentifier)
	require.True(t, ok, "chat-attached tool must expose its config ID")
	require.Equal(t, cfg.ID, ident.MCPServerConfigID())

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Name: tools[0].Info().Name, Input: "{}"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	for _, h := range *recorded {
		require.Equal(t, "secret", h.Get("X-Bot-Key"))
	}
}

func TestConnectChatAttached_ForwardsCoderHeadersWhenOptedIn(t *testing.T) {
	t.Parallel()

	for _, forward := range []bool{true, false} {
		t.Run(strconv.FormatBool(forward), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			ts, mu, recorded := newHeaderRecordingServer(t)
			cfg := makeChatAttachedConfig("bot", ts.URL, "")
			cfg.ForwardCoderHeaders = forward
			ownerID := uuid.NewString()
			coderHeaders := map[string]string{chatprovider.HeaderCoderOwnerID: ownerID}

			tools, _, cleanup := mcpclient.ConnectChatAttachedForTest(
				ctx, logger, []database.MCPServerConfig{cfg}, coderHeaders, nil, testutil.WaitLong,
			)
			t.Cleanup(cleanup)
			require.Len(t, tools, 1)

			_, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Input: "{}"})
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			require.NotEmpty(t, *recorded)
			for _, h := range *recorded {
				if forward {
					require.Equal(t, ownerID, h.Get(chatprovider.HeaderCoderOwnerID))
				} else {
					require.Empty(t, h.Get(chatprovider.HeaderCoderOwnerID))
				}
			}
		})
	}
}

func TestConnectChatAttached_RejectsTooManyTools(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, manyTools(65)...)
	cfg := makeChatAttachedConfig("bot", ts.URL, "")

	tools, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
	)
	t.Cleanup(cleanup)

	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
	require.Contains(t, summaries[0].Error, "maximum is 64")
}

func TestConnectChatAttached_RejectsOversizedToolDefinition(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	big := testTool{
		tool: &mcp.Tool{
			Name:        "big",
			Description: strings.Repeat("a", 64<<10+1),
			InputSchema: map[string]any{"type": "object"},
		},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return textToolResult("ok"), nil
		},
	}
	ts := newTestMCPServer(t, big)
	cfg := makeChatAttachedConfig("bot", ts.URL, "")

	tools, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
	)
	t.Cleanup(cleanup)

	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
	require.Contains(t, summaries[0].Error, "tool definition exceeds maximum size")
}

func TestConnectChatAttached_RejectsOversizedAggregateToolDefinitions(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Each definition is under the 64 KiB per-tool cap; the sum is over
	// the 256 KiB aggregate cap.
	var tools []testTool
	for i := 0; i < 5; i++ {
		tools = append(tools, testTool{
			tool: &mcp.Tool{
				Name:        fmt.Sprintf("big_%d", i),
				Description: strings.Repeat("a", 60<<10),
				InputSchema: map[string]any{"type": "object"},
			},
			handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return textToolResult("ok"), nil
			},
		})
	}
	ts := newTestMCPServer(t, tools...)
	cfg := makeChatAttachedConfig("bot", ts.URL, "")

	got, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
	)
	t.Cleanup(cleanup)

	require.Empty(t, got)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
	require.Contains(t, summaries[0].Error, "maximum total size")
}

func TestConnectChatAttached_ToolResultCap(t *testing.T) {
	t.Parallel()

	const maxBytes = mcpclient.MaxChatAttachedToolResultBytesForTest
	for _, tc := range []struct {
		name      string
		result    string
		sensitive []string
		asError   bool
		asImage   bool
		wantError bool
	}{
		{name: "OverCap", result: strings.Repeat("a", maxBytes+1), wantError: true},
		{name: "ShortSecretInflatesOverCap", result: strings.Repeat("prod", (maxBytes-1024)/4), sensitive: []string{"prod"}, wantError: true},
		{name: "ServerErrorOverCap", result: strings.Repeat("a", maxBytes+1), asError: true, wantError: true},
		{name: "Base64InflatesOverCap", result: strings.Repeat("a", maxBytes*3/4+1), asImage: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			huge := testTool{
				tool: &mcp.Tool{
					Name:        "huge",
					Description: "returns a huge result",
					InputSchema: map[string]any{"type": "object"},
				},
				handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					switch {
					case tc.asError:
						return nil, xerrors.New(tc.result)
					case tc.asImage:
						return &mcp.CallToolResult{Content: []mcp.Content{
							&mcp.ImageContent{Data: []byte(tc.result), MIMEType: "image/png"},
						}}, nil
					}
					return textToolResult(tc.result), nil
				},
			}
			ts := newTestMCPServer(t, huge)
			cfg := makeChatAttachedConfig("bot", ts.URL, "")

			tools, _, cleanup := mcpclient.ConnectChatAttachedForTest(
				ctx, logger, []database.MCPServerConfig{cfg}, nil,
				map[uuid.UUID][]string{cfg.ID: tc.sensitive}, testutil.WaitLong,
			)
			t.Cleanup(cleanup)
			require.Len(t, tools, 1)

			resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Input: "{}"})
			require.NoError(t, err)
			require.Equal(t, tc.wantError, resp.IsError)
			if tc.wantError {
				require.Contains(t, resp.Content, "exceeded maximum size")
			}
			require.LessOrEqual(t, len(resp.Content), maxBytes)
		})
	}
}

func TestConnectChatAttached_RedactsSensitiveValues(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// The "&" is JSON-escaped when structured content is encoded, so a
	// redactor that only scans the encoded text would miss it.
	const secret = "super&secret&token"

	// The server URL is only known after the listener starts, so the
	// tool definitions read it from this variable at ListTools time.
	var serverURL string
	leaky := testTool{
		tool: &mcp.Tool{
			Name:        "leaky",
			InputSchema: map[string]any{"type": "object"},
		},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result := textToolResult("result mentions " + secret + " and " + serverURL)
			result.StructuredContent = map[string]any{"token": secret, "url": serverURL}
			return result, nil
		},
	}
	failing := testTool{
		tool: &mcp.Tool{
			Name:        "failing",
			Description: "always fails",
			InputSchema: map[string]any{"type": "object"},
		},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, xerrors.Errorf("upstream rejected %s at %s", secret, serverURL)
		},
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	t.Cleanup(ts.Close)
	serverURL = ts.URL
	leaky.tool.Description = "Talks to " + serverURL + " using " + secret + ". Send the X-Bot-Key header."
	leaky.tool.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"token": map[string]any{
				"type":        "string",
				"description": "defaults to " + secret + " for " + serverURL,
			},
		},
	}
	srv.AddTool(leaky.tool, leaky.handler)
	srv.AddTool(failing.tool, failing.handler)

	cfg := makeChatAttachedConfig("bot", ts.URL, `{"X-Bot-Key":"`+secret+`"}`)
	sensitive := map[uuid.UUID][]string{cfg.ID: {ts.URL, secret, "X-Bot-Key", "D"}}

	tools, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, sensitive, testutil.WaitLong,
	)
	t.Cleanup(cleanup)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[0].Outcome)
	require.Len(t, tools, 2)

	byName := map[string]fantasy.AgentTool{}
	for _, tool := range tools {
		byName[tool.Info().Name] = tool
	}
	leakyTool, ok := byName["bot__leaky"]
	require.True(t, ok, "tool names: %v", byName)
	failingTool, ok := byName["bot__failing"]
	require.True(t, ok)

	info := leakyTool.Info()
	require.Equal(t, "Talks to [REDACTED] using [REDACTED]. Send the [REDACTED] header.", info.Description)
	require.NotContains(t, info.Description, ts.URL)
	prop, ok := info.Parameters["token"].(map[string]any)
	require.True(t, ok, "parameters: %#v", info.Parameters)
	require.Equal(t, "defaults to [REDACTED] for [REDACTED]", prop["description"])

	resp, err := leakyTool.Run(ctx, fantasy.ToolCall{ID: "call-1", Input: "{}"})
	require.NoError(t, err)
	require.NotContains(t, resp.Content, "secret")
	require.NotContains(t, resp.Content, ts.URL)
	require.Contains(t, resp.Content, "result mentions [REDACTED] and [REDACTED]")
	require.Contains(t, resp.Content, `"token":"[REDACTED]"`)

	resp, err = failingTool.Run(ctx, fantasy.ToolCall{ID: "call-2", Input: "{}"})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.NotContains(t, resp.Content, "secret")
	require.NotContains(t, resp.Content, ts.URL)
	require.Contains(t, resp.Content, "[REDACTED]")

	// A connect failure must not leak the URL into the persisted summary
	// either. A refused dial makes net/http embed the full request URL,
	// including a path credential, in the error.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	brokenURL := closed.URL + "/t/pathtoken?key=querytoken"
	brokenCfg := makeChatAttachedConfig("broken", brokenURL, "")
	_, summaries, cleanup = mcpclient.ConnectChatAttachedForTest(
		ctx, logger, []database.MCPServerConfig{brokenCfg}, nil,
		map[uuid.UUID][]string{brokenCfg.ID: {brokenURL}}, testutil.WaitLong,
	)
	t.Cleanup(cleanup)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
	require.Contains(t, summaries[0].Error, "connection refused")
	require.NotContains(t, summaries[0].Error, "pathtoken")
	require.NotContains(t, summaries[0].Error, "querytoken")
}

func TestConnectChatAttached_BodyCap(t *testing.T) {
	t.Parallel()

	t.Run("UnknownLength", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("a"), mcpclient.MaxChatAttachedHTTPResponseBytesForTest+1))
		}))
		t.Cleanup(ts.Close)
		cfg := makeChatAttachedConfig("bot", ts.URL, "")

		tools, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
			ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
		)
		t.Cleanup(cleanup)
		require.Empty(t, tools)
		require.Len(t, summaries, 1)
		require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
		require.Contains(t, summaries[0].Error, "exceeds maximum size")
	})

	t.Run("ContentLength", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		size := mcpclient.MaxChatAttachedHTTPResponseBytesForTest + 1
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", strconv.Itoa(size))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("a"), size))
		}))
		t.Cleanup(ts.Close)
		cfg := makeChatAttachedConfig("bot", ts.URL, "")

		tools, summaries, cleanup := mcpclient.ConnectChatAttachedForTest(
			ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
		)
		t.Cleanup(cleanup)
		require.Empty(t, tools)
		require.Len(t, summaries, 1)
		require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
		require.Contains(t, summaries[0].Error, "exceeds maximum size")
	})

	t.Run("EventStream", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		// One call streams more than the body cap in total, but each
		// event stays under it, so the stream must not be cut off.
		chatty := testTool{
			tool: &mcp.Tool{Name: "chatty", InputSchema: map[string]any{"type": "object"}},
			handler: func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				for i := 0; i < 5; i++ {
					_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
						ProgressToken: "t",
						Message:       strings.Repeat("a", mcpclient.MaxChatAttachedHTTPResponseBytesForTest/4),
						Progress:      float64(i),
					})
				}
				return textToolResult("done"), nil
			},
		}
		ts := newTestMCPServer(t, chatty)
		cfg := makeChatAttachedConfig("bot", ts.URL, "")

		tools, _, cleanup := mcpclient.ConnectChatAttachedForTest(
			ctx, logger, []database.MCPServerConfig{cfg}, nil, nil, testutil.WaitLong,
		)
		t.Cleanup(cleanup)
		require.Len(t, tools, 1)

		resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Input: "{}"})
		require.NoError(t, err)
		require.False(t, resp.IsError, resp.Content)
		require.Contains(t, resp.Content, "done")
	})
}

type stubTool struct {
	name     string
	configID uuid.UUID
}

func (s *stubTool) Info() fantasy.ToolInfo { return fantasy.ToolInfo{Name: s.name} }
func (*stubTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("stub"), nil
}
func (*stubTool) ProviderOptions() fantasy.ProviderOptions   { return nil }
func (*stubTool) SetProviderOptions(fantasy.ProviderOptions) {}
func (s *stubTool) MCPServerConfigID() uuid.UUID             { return s.configID }

func TestAppendChatAttached_ExistingToolsWin(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	a := &stubTool{name: "a", configID: uuid.New()}
	b := &stubTool{name: "b", configID: uuid.New()}
	bPrime := &stubTool{name: "b", configID: uuid.New()}
	c := &stubTool{name: "c", configID: uuid.New()}

	got := mcpclient.AppendChatAttached(ctx, logger,
		[]fantasy.AgentTool{a, b},
		[]fantasy.AgentTool{bPrime, c},
	)
	require.Len(t, got, 3)
	require.Same(t, a, got[0])
	require.Same(t, b, got[1], "the existing tool must win the name collision")
	require.Same(t, c, got[2])

	orig := []fantasy.AgentTool{a}
	require.Equal(t, orig, mcpclient.AppendChatAttached(ctx, logger, orig, nil))
}

func TestOrgConnectHasNoCaps(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, manyTools(65)...)
	cfg := makeConfig("org", ts.URL)

	tools, summaries, cleanup := mcpclient.ConnectAllForTest(
		ctx, logger, []database.MCPServerConfig{cfg}, testutil.WaitLong, nil,
	)
	t.Cleanup(cleanup)

	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[0].Outcome)
	require.Len(t, tools, 65, "org servers must not be subject to the chat-attached tool cap")
}

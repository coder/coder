package aibridged_test

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/tools/txtar"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/openai/openai-go"
	oai_ssestream "github.com/openai/openai-go/packages/ssestream"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
)

var (
	//go:embed fixtures/anthropic/single_builtin_tool.txtar
	antSingleBuiltinTool []byte
	//go:embed fixtures/anthropic/single_injected_tool.txtar
	antSingleInjectedTool []byte

	//go:embed fixtures/openai/single_builtin_tool.txtar
	oaiSingleBuiltinTool []byte
	//go:embed fixtures/openai/single_injected_tool.txtar
	oaiSingleInjectedTool []byte

	//go:embed fixtures/anthropic/simple.txtar
	antSimple []byte

	//go:embed fixtures/openai/simple.txtar
	oaiSimple []byte
)

const (
	fixtureRequest                  = "request"
	fixtureStreamingResponse        = "streaming"
	fixtureNonStreamingResponse     = "non-streaming"
	fixtureStreamingToolResponse    = "streaming/tool-call"
	fixtureNonStreamingToolResponse = "non-streaming/tool-call"
)

func TestAnthropicMessages(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	sessionToken := getSessionToken(t, client)

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			streaming                                 bool
			expectedInputTokens, expectedOutputTokens int
		}{
			{
				streaming:            true,
				expectedInputTokens:  2,
				expectedOutputTokens: 66,
			},
			{
				streaming:            false,
				expectedInputTokens:  5,
				expectedOutputTokens: 84,
			},
		}

		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/streaming=%v", t.Name(), tc.streaming), func(t *testing.T) {
				t.Parallel()

				arc := txtar.Parse(antSingleBuiltinTool)
				t.Logf("%s: %s", t.Name(), arc.Comment)

				files := filesMap(arc)
				require.Len(t, files, 3)
				require.Contains(t, files, fixtureRequest)
				require.Contains(t, files, fixtureStreamingResponse)
				require.Contains(t, files, fixtureNonStreamingResponse)

				reqBody := files[fixtureRequest]

				// Add the stream param to the request.
				newBody, err := sjson.SetBytes(reqBody, "stream", tc.streaming)
				require.NoError(t, err)
				reqBody = newBody

				ctx := testutil.Context(t, testutil.WaitLong)
				srv := newMockServer(ctx, t, files, nil)
				t.Cleanup(srv.Close)

				coderdClient := &fakeBridgeDaemonClient{}

				logger := testutil.Logger(t) // slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				registry := aibridged.ProviderRegistry{
					aibridged.ProviderAnthropic: aibridged.NewAnthropicMessagesProvider(srv.URL, sessionToken),
				}
				b, err := aibridged.NewBridge(registry, logger, func() (proto.DRPCAIBridgeDaemonClient, error) {
					return coderdClient, nil
				}, nil)
				require.NoError(t, err)

				mockSrv := httptest.NewServer(withInitiator(getCurrentUserID(t, client), b.Handler()))
				// Make API call to aibridge for Anthropic /v1/messages
				req := createAnthropicMessagesReq(t, mockSrv.URL, reqBody)
				client := &http.Client{}
				resp, err := client.Do(req)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				defer resp.Body.Close()

				// Response-specific checks.
				if tc.streaming {
					sp := aibridged.NewSSEParser()
					require.NoError(t, sp.Parse(resp.Body))

					// Ensure the message starts and completes, at a minimum.
					assert.Contains(t, sp.AllEvents(), "message_start")
					assert.Contains(t, sp.AllEvents(), "message_stop")
				}

				require.Len(t, coderdClient.tokenUsages, 1)

				assert.EqualValues(t, tc.expectedInputTokens, calculateTotalInputTokens(coderdClient.tokenUsages), "input tokens miscalculated")
				assert.EqualValues(t, tc.expectedOutputTokens, calculateTotalOutputTokens(coderdClient.tokenUsages), "output tokens miscalculated")

				var args map[string]any
				require.NoError(t, json.Unmarshal([]byte(coderdClient.toolUsages[0].Input), &args))

				require.Len(t, coderdClient.toolUsages, 1)
				assert.Equal(t, "Read", coderdClient.toolUsages[0].Tool)
				require.Contains(t, args, "file_path")
				assert.Equal(t, "/tmp/blah/foo", args["file_path"])

				require.Len(t, coderdClient.userPrompts, 1)
				assert.Equal(t, "read the foo file", coderdClient.userPrompts[0].Prompt)
			})
		}
	})
}

func TestOpenAIChatCompletions(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	sessionToken := getSessionToken(t, client)

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			streaming                                 bool
			expectedInputTokens, expectedOutputTokens int
		}{
			{
				streaming:            true,
				expectedInputTokens:  60,
				expectedOutputTokens: 15,
			},
			{
				streaming:            false,
				expectedInputTokens:  60,
				expectedOutputTokens: 15,
			},
		}

		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/streaming=%v", t.Name(), tc.streaming), func(t *testing.T) {
				t.Parallel()

				arc := txtar.Parse(oaiSingleBuiltinTool)
				t.Logf("%s: %s", t.Name(), arc.Comment)

				files := filesMap(arc)
				require.Len(t, files, 3)
				require.Contains(t, files, fixtureRequest)
				require.Contains(t, files, fixtureStreamingResponse)
				require.Contains(t, files, fixtureNonStreamingResponse)

				reqBody := files[fixtureRequest]

				// Add the stream param to the request.
				newBody, err := sjson.SetBytes(reqBody, "stream", tc.streaming)
				require.NoError(t, err)
				reqBody = newBody

				ctx := testutil.Context(t, testutil.WaitLong)
				srv := newMockServer(ctx, t, files, nil)
				t.Cleanup(srv.Close)

				coderdClient := &fakeBridgeDaemonClient{}

				logger := testutil.Logger(t) // slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				registry := aibridged.ProviderRegistry{
					aibridged.ProviderOpenAI: aibridged.NewOpenAIProvider(srv.URL, sessionToken),
				}
				b, err := aibridged.NewBridge(registry, logger, func() (proto.DRPCAIBridgeDaemonClient, error) {
					return coderdClient, nil
				}, nil)
				require.NoError(t, err)

				mockSrv := httptest.NewServer(withInitiator(getCurrentUserID(t, client), b.Handler()))
				// Make API call to aibridge for OpenAI /v1/chat/completions
				req := createOpenAIChatCompletionsReq(t, mockSrv.URL, reqBody)

				client := &http.Client{}
				resp, err := client.Do(req)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				defer resp.Body.Close()

				// Response-specific checks.
				if tc.streaming {
					sp := aibridged.NewSSEParser()
					require.NoError(t, sp.Parse(resp.Body))

					// OpenAI sends all events under the same type.
					messageEvents := sp.MessageEvents()
					assert.NotEmpty(t, messageEvents)

					// OpenAI streaming ends with [DONE]
					lastEvent := messageEvents[len(messageEvents)-1]
					assert.Equal(t, "[DONE]", lastEvent.Data)
				}

				require.Len(t, coderdClient.tokenUsages, 1)
				assert.EqualValues(t, tc.expectedInputTokens, calculateTotalInputTokens(coderdClient.tokenUsages), "input tokens miscalculated")
				assert.EqualValues(t, tc.expectedOutputTokens, calculateTotalOutputTokens(coderdClient.tokenUsages), "output tokens miscalculated")

				var args map[string]any
				require.NoError(t, json.Unmarshal([]byte(coderdClient.toolUsages[0].Input), &args))

				require.Len(t, coderdClient.toolUsages, 1)
				assert.Equal(t, "read_file", coderdClient.toolUsages[0].Tool)
				require.Contains(t, args, "path")
				assert.Equal(t, "README.md", args["path"])

				require.Len(t, coderdClient.userPrompts, 1)
				assert.Equal(t, "how large is the README.md file in my current path", coderdClient.userPrompts[0].Prompt)
			})
		}
	})
}

func TestSimple(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	sessionToken := getSessionToken(t, client)

	testCases := []struct {
		name              string
		fixture           []byte
		configureFunc     func(string, proto.DRPCAIBridgeDaemonClient) (*aibridged.Bridge, error)
		getResponseIDFunc func(bool, *http.Response) (string, error)
		createRequest     func(*testing.T, string, []byte) *http.Request
	}{
		{
			name:    aibridged.ProviderAnthropic,
			fixture: antSimple,
			configureFunc: func(addr string, client proto.DRPCAIBridgeDaemonClient) (*aibridged.Bridge, error) {
				logger := testutil.Logger(t)
				registry := aibridged.ProviderRegistry{
					aibridged.ProviderAnthropic: aibridged.NewAnthropicMessagesProvider(addr, sessionToken),
				}
				return aibridged.NewBridge(registry, logger, func() (proto.DRPCAIBridgeDaemonClient, error) {
					return client, nil
				}, nil)
			},
			getResponseIDFunc: func(streaming bool, resp *http.Response) (string, error) {
				if streaming {
					decoder := ssestream.NewDecoder(resp)
					// TODO: this is a bit flimsy since this API won't be in beta forever.
					stream := ssestream.NewStream[anthropic.BetaRawMessageStreamEventUnion](decoder, nil)
					var message anthropic.BetaMessage
					for stream.Next() {
						event := stream.Current()
						if err := message.Accumulate(event); err != nil {
							return "", xerrors.Errorf("accumulate event: %w", err)
						}
					}
					if stream.Err() != nil {
						return "", xerrors.Errorf("stream error: %w", stream.Err())
					}
					return message.ID, nil
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return "", xerrors.Errorf("read body: %w", err)
				}

				// TODO: this is a bit flimsy since this API won't be in beta forever.
				var message anthropic.BetaMessage
				if err := json.Unmarshal(body, &message); err != nil {
					return "", xerrors.Errorf("unmarshal response: %w", err)
				}
				return message.ID, nil
			},
			createRequest: createAnthropicMessagesReq,
		},
		{
			name:    aibridged.ProviderOpenAI,
			fixture: oaiSimple,
			configureFunc: func(addr string, client proto.DRPCAIBridgeDaemonClient) (*aibridged.Bridge, error) {
				logger := testutil.Logger(t)
				registry := aibridged.ProviderRegistry{
					aibridged.ProviderOpenAI: aibridged.NewOpenAIProvider(addr, sessionToken),
				}
				return aibridged.NewBridge(registry, logger, func() (proto.DRPCAIBridgeDaemonClient, error) {
					return client, nil
				}, nil)
			},
			getResponseIDFunc: func(streaming bool, resp *http.Response) (string, error) {
				if streaming {
					// Parse the response stream.
					decoder := oai_ssestream.NewDecoder(resp)
					stream := oai_ssestream.NewStream[openai.ChatCompletionChunk](decoder, nil)
					var message openai.ChatCompletionAccumulator
					for stream.Next() {
						chunk := stream.Current()
						message.AddChunk(chunk)
					}
					if stream.Err() != nil {
						return "", xerrors.Errorf("stream error: %w", stream.Err())
					}
					return message.ID, nil
				}

				// Parse & unmarshal the response.
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return "", xerrors.Errorf("read body: %w", err)
				}

				var message openai.ChatCompletion
				if err := json.Unmarshal(body, &message); err != nil {
					return "", xerrors.Errorf("unmarshal response: %w", err)
				}
				return message.ID, nil
			},
			createRequest: createOpenAIChatCompletionsReq,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			streamingCases := []struct {
				streaming bool
			}{
				{streaming: true},
				{streaming: false},
			}

			for _, sc := range streamingCases {
				t.Run(fmt.Sprintf("streaming=%v", sc.streaming), func(t *testing.T) {
					t.Parallel()

					arc := txtar.Parse(tc.fixture)
					t.Logf("%s: %s", t.Name(), arc.Comment)

					files := filesMap(arc)
					require.Len(t, files, 3)
					require.Contains(t, files, fixtureRequest)
					require.Contains(t, files, fixtureStreamingResponse)
					require.Contains(t, files, fixtureNonStreamingResponse)

					reqBody := files[fixtureRequest]

					// Add the stream param to the request.
					newBody, err := sjson.SetBytes(reqBody, "stream", sc.streaming)
					require.NoError(t, err)
					reqBody = newBody

					// Given: a mock API server and a Bridge through which the requests will flow.
					ctx := testutil.Context(t, testutil.WaitLong)
					srv := newMockServer(ctx, t, files, nil)
					t.Cleanup(srv.Close)

					coderdClient := &fakeBridgeDaemonClient{}

					b, err := tc.configureFunc(srv.URL, coderdClient)
					require.NoError(t, err)

					mockSrv := httptest.NewServer(withInitiator(getCurrentUserID(t, client), b.Handler()))
					// When: calling the "API server" with the fixture's request body.
					req := tc.createRequest(t, mockSrv.URL, reqBody)
					client := &http.Client{}
					resp, err := client.Do(req)
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, resp.StatusCode)
					defer resp.Body.Close()

					// Then: I expect a non-empty response.
					bodyBytes, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					assert.NotEmpty(t, bodyBytes, "should have received response body")

					// Reset the body after being read.
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

					// Then: I expect the prompt to have been tracked.
					require.NotEmpty(t, coderdClient.userPrompts, "no prompts tracked")
					assert.Equal(t, "how many angels can dance on the head of a pin", coderdClient.userPrompts[0].Prompt)

					// Validate that responses have their IDs overridden with a session ID rather than the original ID from the upstream provider.
					// The reason for this is that Bridge may make multiple upstream requests (i.e. to invoke injected tools), and clients will not be expecting
					// multiple messages in response to a single request.
					// TODO: validate that expected upstream message ID is captured alongside returned ID in token usage.
					id, err := tc.getResponseIDFunc(sc.streaming, resp)
					require.NoError(t, err, "failed to retrieve response ID")
					require.Nil(t, uuid.Validate(id), "id is not a UUID")
				})
			}
		})
	}
}

// setupMCPToolsForTest creates a mock MCP server, initializes the MCP bridge, and returns the tools
func setupMCPToolsForTest(t *testing.T) map[string][]*aibridged.MCPTool {
	t.Helper()

	// Setup Coder MCP integration
	mcpSrv := httptest.NewServer(createMockMCPSrv(t))
	t.Cleanup(mcpSrv.Close)

	logger := testutil.Logger(t)
	mcpBridge, err := aibridged.NewMCPToolBridge("coder", mcpSrv.URL, map[string]string{}, logger)
	require.NoError(t, err)

	// Initialize MCP client, fetch tools, and inject into bridge
	require.NoError(t, mcpBridge.Init(testutil.Context(t, testutil.WaitShort)))
	tools := mcpBridge.ListTools()
	require.NotEmpty(t, tools)

	return map[string][]*aibridged.MCPTool{
		"coder": tools,
	}
}

// TestInjectedTool is an abstracted test function for "single injected tool" scenarios
// that works with both Anthropic and OpenAI providers
func TestInjectedTool(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	sessionToken := getSessionToken(t, client)

	testCases := []struct {
		name                   string
		fixture                []byte
		configureFunc          func(string, proto.DRPCAIBridgeDaemonClient, map[string][]*aibridged.MCPTool) (*aibridged.Bridge, error)
		getResponseContentFunc func(bool, *http.Response) (string, error)
		createRequest          func(*testing.T, string, []byte) *http.Request
	}{
		{
			name:    aibridged.ProviderAnthropic,
			fixture: antSingleInjectedTool,
			configureFunc: func(addr string, client proto.DRPCAIBridgeDaemonClient, tools map[string][]*aibridged.MCPTool) (*aibridged.Bridge, error) {
				logger := testutil.Logger(t)
				registry := aibridged.ProviderRegistry{
					aibridged.ProviderAnthropic: aibridged.NewAnthropicMessagesProvider(addr, sessionToken),
				}
				return aibridged.NewBridge(registry, logger, func() (proto.DRPCAIBridgeDaemonClient, error) {
					return client, nil
				}, tools)
			},
			getResponseContentFunc: func(streaming bool, resp *http.Response) (string, error) {
				// TODO: this is a bit flimsy since this API won't be in beta forever.
				var content *anthropic.BetaContentBlockUnion
				if streaming {
					// Parse the response stream.
					decoder := ssestream.NewDecoder(resp)
					stream := ssestream.NewStream[anthropic.BetaRawMessageStreamEventUnion](decoder, nil)
					var message anthropic.BetaMessage
					for stream.Next() {
						event := stream.Current()
						if err := message.Accumulate(event); err != nil {
							return "", xerrors.Errorf("accumulate event: %w", err)
						}
					}
					if stream.Err() != nil {
						return "", xerrors.Errorf("stream error: %w", stream.Err())
					}
					if len(message.Content) < 2 {
						return "", xerrors.Errorf("expected at least 2 content blocks, got %d", len(message.Content))
					}
					content = &message.Content[1]
				} else {
					// Parse & unmarshal the response.
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						return "", xerrors.Errorf("read body: %w", err)
					}

					var message anthropic.BetaMessage
					if err := json.Unmarshal(body, &message); err != nil {
						return "", xerrors.Errorf("unmarshal response: %w", err)
					}
					if len(message.Content) == 0 {
						return "", xerrors.Errorf("no content blocks in response")
					}
					content = &message.Content[0]
				}

				if content == nil {
					return "", xerrors.Errorf("content is nil")
				}
				return content.Text, nil
			},
			createRequest: createAnthropicMessagesReq,
		},
		{
			name:    aibridged.ProviderOpenAI,
			fixture: oaiSingleInjectedTool,
			configureFunc: func(addr string, client proto.DRPCAIBridgeDaemonClient, tools map[string][]*aibridged.MCPTool) (*aibridged.Bridge, error) {
				logger := testutil.Logger(t)
				registry := aibridged.ProviderRegistry{
					aibridged.ProviderOpenAI: aibridged.NewOpenAIProvider(addr, sessionToken),
				}
				return aibridged.NewBridge(registry, logger, func() (proto.DRPCAIBridgeDaemonClient, error) {
					return client, nil
				}, tools)
			},
			getResponseContentFunc: func(streaming bool, resp *http.Response) (string, error) {
				var content *openai.ChatCompletionChoice
				if streaming {
					// Parse the response stream.
					decoder := oai_ssestream.NewDecoder(resp)
					stream := oai_ssestream.NewStream[openai.ChatCompletionChunk](decoder, nil)
					var message openai.ChatCompletionAccumulator
					for stream.Next() {
						chunk := stream.Current()
						message.AddChunk(chunk)
					}

					if stream.Err() != nil {
						return "", xerrors.Errorf("stream error: %w", stream.Err())
					}
					if len(message.Choices) == 0 {
						return "", xerrors.Errorf("no choices in response")
					}
					content = &message.Choices[0]
				} else {
					// Parse & unmarshal the response.
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						return "", xerrors.Errorf("read body: %w", err)
					}

					var message openai.ChatCompletion
					if err := json.Unmarshal(body, &message); err != nil {
						return "", xerrors.Errorf("unmarshal response: %w", err)
					}
					if len(message.Choices) == 0 {
						return "", xerrors.Errorf("no choices in response")
					}
					content = &message.Choices[0]
				}

				if content == nil {
					return "", xerrors.Errorf("content is nil")
				}
				return content.Message.Content, nil
			},
			createRequest: createOpenAIChatCompletionsReq,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			streamingCases := []struct {
				streaming bool
			}{
				{streaming: true},
				{streaming: false},
			}

			for _, sc := range streamingCases {
				t.Run(fmt.Sprintf("streaming=%v", sc.streaming), func(t *testing.T) {
					t.Parallel()

					arc := txtar.Parse(tc.fixture)
					t.Logf("%s: %s", t.Name(), arc.Comment)

					files := filesMap(arc)
					require.Len(t, files, 5)
					require.Contains(t, files, fixtureRequest)
					require.Contains(t, files, fixtureStreamingResponse)
					require.Contains(t, files, fixtureNonStreamingResponse)
					require.Contains(t, files, fixtureStreamingToolResponse)
					require.Contains(t, files, fixtureNonStreamingToolResponse)

					reqBody := files[fixtureRequest]

					// Add the stream param to the request.
					newBody, err := sjson.SetBytes(reqBody, "stream", sc.streaming)
					require.NoError(t, err)
					reqBody = newBody

					ctx := testutil.Context(t, testutil.WaitLong)

					// Setup mock server with response mutator for multi-turn interaction.
					mockSrv := newMockServer(ctx, t, files, func(reqCount uint32, resp []byte) []byte {
						if reqCount == 1 {
							return resp // First request gets the normal response (with tool call)
						}

						if reqCount > 2 {
							// This should not happen in single injected tool tests
							return resp
						}

						// Second request gets the tool response
						if sc.streaming {
							return files[fixtureStreamingToolResponse]
						}
						return files[fixtureNonStreamingToolResponse]
					})
					t.Cleanup(mockSrv.Close)

					coderdClient := &fakeBridgeDaemonClient{}

					// Setup MCP tools.
					tools := setupMCPToolsForTest(t)

					// Configure the bridge with injected tools.
					b, err := tc.configureFunc(mockSrv.URL, coderdClient, tools)
					require.NoError(t, err)

					// Invoke request to mocked API via aibridge.
					bridgeSrv := httptest.NewServer(withInitiator(getCurrentUserID(t, client), b.Handler()))
					t.Cleanup(bridgeSrv.Close)

					req := tc.createRequest(t, bridgeSrv.URL, reqBody)
					client := &http.Client{}
					resp, err := client.Do(req)
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, resp.StatusCode)
					defer resp.Body.Close()

					// We must ALWAYS have 2 calls to the bridge for injected tool tests
					require.Eventually(t, func() bool {
						return mockSrv.callCount.Load() == 2
					}, testutil.WaitLong, testutil.IntervalFast)

					// Ensure expected tool was invoked with expected input.
					require.Len(t, coderdClient.toolUsages, 1)
					require.Equal(t, mockToolName, coderdClient.toolUsages[0].Tool)
					require.EqualValues(t, `{"owner":"admin"}`, coderdClient.toolUsages[0].Input)

					// Ensure tool returned expected value.
					answer, err := tc.getResponseContentFunc(sc.streaming, resp)
					require.NoError(t, err)
					require.Contains(t, answer, "dd711d5c-83c6-4c08-a0af-b73055906e8c") // The ID of the workspace to be returned.
				})
			}
		})
	}
}

func calculateTotalOutputTokens(in []*proto.TrackTokenUsageRequest) int64 {
	var total int64
	for _, el := range in {
		total += el.OutputTokens
	}
	return total
}

func calculateTotalInputTokens(in []*proto.TrackTokenUsageRequest) int64 {
	var total int64
	for _, el := range in {
		total += el.InputTokens
	}
	return total
}

type archiveFileMap map[string][]byte

func filesMap(archive *txtar.Archive) archiveFileMap {
	if len(archive.Files) == 0 {
		return nil
	}

	out := make(archiveFileMap, len(archive.Files))
	for _, f := range archive.Files {
		out[f.Name] = f.Data
	}
	return out
}

func createAnthropicMessagesReq(t *testing.T, baseURL string, input []byte) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), "POST", baseURL+"/v1/messages", bytes.NewReader(input))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	return req
}

func createOpenAIChatCompletionsReq(t *testing.T, baseURL string, input []byte) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), "POST", baseURL+"/v1/chat/completions", bytes.NewReader(input))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	return req
}

func getSessionToken(t *testing.T, client *codersdk.Client) string {
	t.Helper()

	_ = coderdtest.CreateFirstUser(t, client)
	resp, err := client.LoginWithPassword(t.Context(), codersdk.LoginWithPasswordRequest{
		Email:    coderdtest.FirstUserParams.Email,
		Password: coderdtest.FirstUserParams.Password,
	})

	require.NoError(t, err)
	return resp.SessionToken
}

type mockServer struct {
	*httptest.Server

	callCount atomic.Uint32
}

func newMockServer(ctx context.Context, t *testing.T, files archiveFileMap, responseMutatorFn func(reqCount uint32, resp []byte) []byte) *mockServer {
	t.Helper()

	ms := &mockServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ms.callCount.Add(1)

		body, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		require.NoError(t, err)

		type msg struct {
			Stream bool `json:"stream"`
		}
		var reqMsg msg
		require.NoError(t, json.Unmarshal(body, &reqMsg))

		if !reqMsg.Stream {
			resp := files[fixtureNonStreamingResponse]
			if responseMutatorFn != nil {
				resp = responseMutatorFn(ms.callCount.Load(), resp)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resp)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		resp := files[fixtureStreamingResponse]
		if responseMutatorFn != nil {
			resp = responseMutatorFn(ms.callCount.Load(), resp)
		}

		scanner := bufio.NewScanner(bytes.NewReader(resp))
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		for scanner.Scan() {
			line := scanner.Text()

			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}

		if err := scanner.Err(); err != nil {
			http.Error(w, fmt.Sprintf("Error reading fixture: %v", err), http.StatusInternalServerError)
			return
		}
	}))
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}

	ms.Server = srv
	return ms
}

var _ proto.DRPCAIBridgeDaemonClient = &fakeBridgeDaemonClient{}

type fakeBridgeDaemonClient struct {
	mu sync.Mutex

	sessions    []*proto.StartSessionRequest
	tokenUsages []*proto.TrackTokenUsageRequest
	userPrompts []*proto.TrackUserPromptRequest
	toolUsages  []*proto.TrackToolUsageRequest
}

func (*fakeBridgeDaemonClient) DRPCConn() drpc.Conn {
	conn, _ := drpcsdk.MemTransportPipe()
	return conn
}

// StartSession implements proto.DRPCAIBridgeDaemonClient.
func (f *fakeBridgeDaemonClient) StartSession(ctx context.Context, in *proto.StartSessionRequest) (*proto.StartSessionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append(f.sessions, in)

	return &proto.StartSessionResponse{}, nil
}

func (f *fakeBridgeDaemonClient) TrackTokenUsage(ctx context.Context, in *proto.TrackTokenUsageRequest) (*proto.TrackTokenUsageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenUsages = append(f.tokenUsages, in)

	return &proto.TrackTokenUsageResponse{}, nil
}

func (f *fakeBridgeDaemonClient) TrackUserPrompt(ctx context.Context, in *proto.TrackUserPromptRequest) (*proto.TrackUserPromptResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userPrompts = append(f.userPrompts, in)

	return &proto.TrackUserPromptResponse{}, nil
}

func (f *fakeBridgeDaemonClient) TrackToolUsage(ctx context.Context, in *proto.TrackToolUsageRequest) (*proto.TrackToolUsageResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toolUsages = append(f.toolUsages, in)

	return &proto.TrackToolUsageResponse{}, nil
}

const mockToolName = "coder_list_workspaces"

func createMockMCPSrv(t *testing.T) http.Handler {
	t.Helper()

	s := server.NewMCPServer(
		"Mock coder MCP server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	tool := mcp.NewTool(mockToolName,
		mcp.WithDescription(fmt.Sprintf("Mock of the %s tool", mockToolName)),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("mock"), nil
	})

	return server.NewStreamableHTTPServer(s)
}

// withInitiator wraps a handler injecting the Bridge user ID into context.
// TODO: this is only necessary because we're not exercising the real API's middleware, which may hide some problems.
func withInitiator(userID uuid.UUID, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), aibridged.ContextKeyBridgeUserID{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getCurrentUserID(t *testing.T, client *codersdk.Client) uuid.UUID {
	t.Helper()

	me, err := client.User(t.Context(), "me")
	require.NoError(t, err)
	return me.ID
}

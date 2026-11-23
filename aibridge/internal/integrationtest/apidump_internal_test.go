package integrationtest

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/aibridgetest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/quartz"
)

const osSep = string(filepath.Separator)

func TestAPIDump(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		fixture           []byte
		providerFunc      func(addr, dumpDir string) aibridge.Provider
		path              string
		headers           http.Header
		expectProviderDir string
	}{
		{
			name:    "anthropic",
			fixture: fixtures.AntSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return aibridgetest.NewAnthropicProvider(t, anthropicCfgWithAPIDump(addr, apiKey, dumpDir), nil)
			},
			path:              pathAnthropicMessages,
			expectProviderDir: config.ProviderAnthropic,
		},
		{
			name:    "openai_chat_completions",
			fixture: fixtures.OaiChatSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return provider.NewOpenAI(openaiCfgWithAPIDump(addr, apiKey, dumpDir))
			},
			path:              pathOpenAIChatCompletions,
			expectProviderDir: config.ProviderOpenAI,
		},
		{
			name:    "openai_responses",
			fixture: fixtures.OaiResponsesBlockingSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return provider.NewOpenAI(openaiCfgWithAPIDump(addr, apiKey, dumpDir))
			},
			path:              pathOpenAIResponses,
			expectProviderDir: config.ProviderOpenAI,
		},
		{
			name:    "copilot_chat_completions",
			fixture: fixtures.OaiChatSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: addr, APIDumpDir: dumpDir})
			},
			path:              pathCopilotChatCompletions,
			headers:           http.Header{"Authorization": {"Bearer test-copilot-token"}},
			expectProviderDir: config.ProviderCopilot,
		},
		{
			name:    "copilot_responses",
			fixture: fixtures.OaiResponsesBlockingSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: addr, APIDumpDir: dumpDir})
			},
			path:              pathCopilotResponses,
			headers:           http.Header{"Authorization": {"Bearer test-copilot-token"}},
			expectProviderDir: config.ProviderCopilot,
		},
		{
			name:    "copilot_custom_name_chat_completions",
			fixture: fixtures.OaiChatSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{
					Name:       "copilot-business",
					BaseURL:    addr,
					APIDumpDir: dumpDir,
				})
			},
			path:              "/copilot-business/chat/completions",
			headers:           http.Header{"Authorization": {"Bearer test-copilot-token"}},
			expectProviderDir: "copilot-business",
		},
		{
			name:    "copilot_custom_name_responses",
			fixture: fixtures.OaiChatSimple,
			providerFunc: func(addr, dumpDir string) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{
					Name:       "copilot-enterprise",
					BaseURL:    addr,
					APIDumpDir: dumpDir,
				})
			},
			path:              "/copilot-enterprise/chat/completions",
			headers:           http.Header{"Authorization": {"Bearer test-copilot-token"}},
			expectProviderDir: "copilot-enterprise",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// Setup mock upstream server.
			fix := fixtures.Parse(t, tc.fixture)
			srv := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))

			// Create temp dir for API dumps.
			dumpDir := t.TempDir()

			bridgeServer := newBridgeTestServer(ctx, t, srv.URL,
				withCustomProvider(tc.providerFunc(srv.URL, dumpDir)),
			)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, fix.Request(), tc.headers)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Verify dump files were created.
			interceptions := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			interceptionID := interceptions[0].ID

			// Find dump files for this interception by walking the dump directory.
			var reqDumpFile, respDumpFile string
			err = filepath.Walk(dumpDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				// Files are named: {timestamp}-{interceptionID}-{seq}.{req|resp}.txt
				if strings.Contains(path, interceptionID) {
					if strings.HasSuffix(path, apidump.SuffixRequest) {
						reqDumpFile = path
					} else if strings.HasSuffix(path, apidump.SuffixResponse) {
						respDumpFile = path
					}
				}
				return nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, reqDumpFile, "request dump file should exist")
			require.NotEmpty(t, respDumpFile, "response dump file should exist")

			// Verify dump files are in the correct provider subdirectory.
			require.Contains(t, reqDumpFile, filepath.Join(dumpDir, tc.expectProviderDir)+osSep,
				"request dump should be in the %s provider directory", tc.expectProviderDir)
			require.Contains(t, respDumpFile, filepath.Join(dumpDir, tc.expectProviderDir)+osSep,
				"response dump should be in the %s provider directory", tc.expectProviderDir)

			// Verify request dump contains expected HTTP request format.
			reqDumpData, err := os.ReadFile(reqDumpFile)
			require.NoError(t, err)

			// Parse the dumped HTTP request.
			dumpReq, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(reqDumpData)))
			require.NoError(t, err)
			dumpBody, err := io.ReadAll(dumpReq.Body)
			require.NoError(t, err)

			// Compare requests semantically (key order may differ).
			require.JSONEq(t, string(dumpBody), string(fix.Request()), "request body JSON should match semantically")

			// Verify response dump contains expected HTTP response format.
			respDumpData, err := os.ReadFile(respDumpFile)
			require.NoError(t, err)

			// Parse the dumped HTTP response.
			dumpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(respDumpData)), nil)
			require.NoError(t, err)
			defer dumpResp.Body.Close()
			require.Equal(t, http.StatusOK, dumpResp.StatusCode)
			dumpRespBody, err := io.ReadAll(dumpResp.Body)
			require.NoError(t, err)

			// Compare responses semantically (key order may differ).
			expectedRespBody := fix.NonStreaming()
			require.JSONEq(t, string(expectedRespBody), string(dumpRespBody), "response body JSON should match semantically")

			bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
		})
	}
}

// TestAPIDump_KeyFailover verifies that each key failover attempt in one
// interception keeps its own request and response dump, for every bridged
// interceptor in blocking and streaming mode.
func TestAPIDump_KeyFailover(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		fixture   []byte
		path      string
		streaming bool
	}{
		{
			name:    "anthropic_messages_blocking",
			fixture: fixtures.AntSimple,
			path:    pathAnthropicMessages,
		},
		{
			name:      "anthropic_messages_streaming",
			fixture:   fixtures.AntSimple,
			path:      pathAnthropicMessages,
			streaming: true,
		},
		{
			name:    "openai_chat_completions_blocking",
			fixture: fixtures.OaiChatSimple,
			path:    pathOpenAIChatCompletions,
		},
		{
			name:      "openai_chat_completions_streaming",
			fixture:   fixtures.OaiChatSimple,
			path:      pathOpenAIChatCompletions,
			streaming: true,
		},
		{
			name:    "openai_responses_blocking",
			fixture: fixtures.OaiResponsesBlockingSimple,
			path:    pathOpenAIResponses,
		},
		{
			name:      "openai_responses_streaming",
			fixture:   fixtures.OaiResponsesStreamingSimple,
			path:      pathOpenAIResponses,
			streaming: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)

			// k0 is rate limited, so the interception fails over to k1.
			upstream := testutil.NewMockUpstream(ctx, t,
				testutil.NewErrorResponse(http.StatusTooManyRequests, "60"),
				testutil.NewFixtureResponse(fix),
			)

			dumpDir := t.TempDir()
			var prov aibridge.Provider
			if tc.path == pathAnthropicMessages {
				pool, err := keypool.New(config.ProviderAnthropic, []string{"k0", "k1"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				prov = aibridgetest.NewAnthropicProvider(t, config.Anthropic{BaseURL: upstream.URL, KeyPool: pool, APIDumpDir: dumpDir}, nil)
			} else {
				pool, err := keypool.New(config.ProviderOpenAI, []string{"k0", "k1"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				prov = provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool, APIDumpDir: dumpDir})
			}
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withCustomProvider(prov))

			requestBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, requestBody)
			require.NoError(t, err)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)
			received := upstream.ReceivedRequests()
			require.Len(t, received, 2)

			interceptions := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			interceptionID := interceptions[0].ID

			// Walk visits files in lexical order, which the zero-padded
			// sequence number in the file name makes call order.
			var reqFiles, respFiles []string
			err = filepath.Walk(dumpDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.Contains(path, interceptionID) {
					return err
				}
				switch {
				case strings.HasSuffix(path, apidump.SuffixRequest):
					reqFiles = append(reqFiles, path)
				case strings.HasSuffix(path, apidump.SuffixResponse):
					respFiles = append(respFiles, path)
				}
				return nil
			})
			require.NoError(t, err)
			require.Len(t, reqFiles, 2, "one request dump per failover attempt")
			require.Len(t, respFiles, 2, "one response dump per failover attempt")

			for i, wantStatus := range []int{http.StatusTooManyRequests, http.StatusOK} {
				reqDumpData, err := os.ReadFile(reqFiles[i])
				require.NoError(t, err)
				dumpReq, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(reqDumpData)))
				require.NoError(t, err)
				dumpBody, err := io.ReadAll(dumpReq.Body)
				require.NoError(t, err)
				require.JSONEq(t, string(received[i].Body), string(dumpBody), "request dump %d", i)

				respDumpData, err := os.ReadFile(respFiles[i])
				require.NoError(t, err)
				dumpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(respDumpData)), nil)
				require.NoError(t, err)
				require.NoError(t, dumpResp.Body.Close())
				require.Equal(t, wantStatus, dumpResp.StatusCode, "response dump %d", i)
			}

			bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
		})
	}
}

func TestAPIDumpPassthrough(t *testing.T) {
	t.Parallel()

	const responseBody = `{"object":"list","data":[{"id":"gpt-4","object":"model"}]}`

	cases := []struct {
		name           string
		providerFunc   func(addr string, dumpDir string) aibridge.Provider
		requestPath    string
		expectDumpName string
	}{
		{
			name: "anthropic",
			providerFunc: func(addr string, dumpDir string) aibridge.Provider {
				return aibridgetest.NewAnthropicProvider(t, anthropicCfgWithAPIDump(addr, apiKey, dumpDir), nil)
			},
			requestPath:    "/anthropic/v1/models",
			expectDumpName: "-v1-models-",
		},
		{
			name: "openai",
			providerFunc: func(addr string, dumpDir string) aibridge.Provider {
				return provider.NewOpenAI(openaiCfgWithAPIDump(addr, apiKey, dumpDir))
			},
			requestPath:    "/openai/v1/models",
			expectDumpName: "-models-",
		},
		{
			name: "copilot",
			providerFunc: func(addr string, dumpDir string) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: addr, APIDumpDir: dumpDir})
			},
			requestPath:    "/copilot/models",
			expectDumpName: "-models-",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(responseBody))
			}))
			t.Cleanup(upstream.Close)

			dumpDir := t.TempDir()

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
				withCustomProvider(tc.providerFunc(upstream.URL, dumpDir)),
			)

			resp, err := bridgeServer.makeRequest(t, http.MethodGet, tc.requestPath, nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Find dump files in the passthrough directory.
			passthroughDir := filepath.Join(dumpDir, tc.name, "passthrough")
			var reqDumpFile, respDumpFile string
			err = filepath.Walk(passthroughDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				if strings.HasSuffix(path, apidump.SuffixRequest) {
					reqDumpFile = path
				} else if strings.HasSuffix(path, apidump.SuffixResponse) {
					respDumpFile = path
				}
				return nil
			})
			require.NoError(t, err, "walking failed: %v", err)

			require.NotEmpty(t, reqDumpFile, "request dump file should exist")
			require.FileExists(t, reqDumpFile)
			require.Contains(t, reqDumpFile, osSep+"passthrough"+osSep)
			require.Contains(t, reqDumpFile, tc.expectDumpName)

			require.NotEmpty(t, respDumpFile, "response dump file should exist")
			require.FileExists(t, respDumpFile)
			require.Contains(t, respDumpFile, osSep+"passthrough"+osSep)
			require.Contains(t, respDumpFile, tc.expectDumpName)

			// Verify request dump.
			reqDumpData, err := os.ReadFile(reqDumpFile)
			require.NoError(t, err)
			dumpReq, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(reqDumpData)))
			require.NoError(t, err)
			require.Equal(t, http.MethodGet, dumpReq.Method)

			// Verify response dump.
			respDumpData, err := os.ReadFile(respDumpFile)
			require.NoError(t, err)
			dumpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(respDumpData)), nil)
			require.NoError(t, err)
			defer dumpResp.Body.Close()
			require.Equal(t, http.StatusOK, dumpResp.StatusCode)
			dumpRespBody, err := io.ReadAll(dumpResp.Body)
			require.NoError(t, err)
			require.JSONEq(t, responseBody, string(dumpRespBody))
		})
	}
}

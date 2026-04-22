package integrationtest //nolint:testpackage // tests unexported internals

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

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
)

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
				return provider.NewAnthropic(anthropicCfgWithAPIDump(addr, apiKey, dumpDir), nil)
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
			srv := newMockUpstream(ctx, t, newFixtureResponse(fix))

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
				// Files are named: {timestamp}-{interceptionID}.{req|resp}.txt
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
			require.Contains(t, reqDumpFile, filepath.Join(dumpDir, tc.expectProviderDir)+"/",
				"request dump should be in the %s provider directory", tc.expectProviderDir)
			require.Contains(t, respDumpFile, filepath.Join(dumpDir, tc.expectProviderDir)+"/",
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
				return provider.NewAnthropic(anthropicCfgWithAPIDump(addr, apiKey, dumpDir), nil)
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
			require.Contains(t, reqDumpFile, "/passthrough/")
			require.Contains(t, reqDumpFile, tc.expectDumpName)

			require.NotEmpty(t, respDumpFile, "response dump file should exist")
			require.FileExists(t, respDumpFile)
			require.Contains(t, respDumpFile, "/passthrough/")
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

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

	anthropicFunc := func(addr, dumpDir string, pool *keypool.Pool) aibridge.Provider {
		cfg := anthropicCfgWithAPIDump(addr, apiKey, dumpDir)
		if pool != nil {
			cfg.KeyPool = pool
		}
		return aibridgetest.NewAnthropicProvider(t, cfg, nil)
	}
	openAIFunc := func(addr, dumpDir string, pool *keypool.Pool) aibridge.Provider {
		cfg := openaiCfgWithAPIDump(addr, apiKey, dumpDir)
		if pool != nil {
			cfg.KeyPool = pool
		}
		return provider.NewOpenAI(cfg)
	}

	cases := []struct {
		name              string
		fixture           []byte
		providerFunc      func(addr, dumpDir string, pool *keypool.Pool) aibridge.Provider
		path              string
		headers           http.Header
		expectProviderDir string
		streaming         bool
		// failover rate limits the first of two pool keys, so the
		// interception makes two upstream calls that must each keep their
		// own dumps.
		failover bool
	}{
		{
			name:              "anthropic",
			fixture:           fixtures.AntSimple,
			providerFunc:      anthropicFunc,
			path:              pathAnthropicMessages,
			expectProviderDir: config.ProviderAnthropic,
		},
		{
			name:              "openai_chat_completions",
			fixture:           fixtures.OaiChatSimple,
			providerFunc:      openAIFunc,
			path:              pathOpenAIChatCompletions,
			expectProviderDir: config.ProviderOpenAI,
		},
		{
			name:              "openai_responses",
			fixture:           fixtures.OaiResponsesBlockingSimple,
			providerFunc:      openAIFunc,
			path:              pathOpenAIResponses,
			expectProviderDir: config.ProviderOpenAI,
		},
		{
			name:    "copilot_chat_completions",
			fixture: fixtures.OaiChatSimple,
			providerFunc: func(addr, dumpDir string, _ *keypool.Pool) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: addr, APIDumpDir: dumpDir})
			},
			path:              pathCopilotChatCompletions,
			headers:           http.Header{"Authorization": {"Bearer test-copilot-token"}},
			expectProviderDir: config.ProviderCopilot,
		},
		{
			name:    "copilot_responses",
			fixture: fixtures.OaiResponsesBlockingSimple,
			providerFunc: func(addr, dumpDir string, _ *keypool.Pool) aibridge.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: addr, APIDumpDir: dumpDir})
			},
			path:              pathCopilotResponses,
			headers:           http.Header{"Authorization": {"Bearer test-copilot-token"}},
			expectProviderDir: config.ProviderCopilot,
		},
		{
			name:    "copilot_custom_name_chat_completions",
			fixture: fixtures.OaiChatSimple,
			providerFunc: func(addr, dumpDir string, _ *keypool.Pool) aibridge.Provider {
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
			providerFunc: func(addr, dumpDir string, _ *keypool.Pool) aibridge.Provider {
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
		{
			name:              "anthropic_failover_blocking",
			fixture:           fixtures.AntSimple,
			providerFunc:      anthropicFunc,
			path:              pathAnthropicMessages,
			expectProviderDir: config.ProviderAnthropic,
			failover:          true,
		},
		{
			name:              "anthropic_failover_streaming",
			fixture:           fixtures.AntSimple,
			providerFunc:      anthropicFunc,
			path:              pathAnthropicMessages,
			expectProviderDir: config.ProviderAnthropic,
			streaming:         true,
			failover:          true,
		},
		{
			name:              "openai_chat_completions_failover_blocking",
			fixture:           fixtures.OaiChatSimple,
			providerFunc:      openAIFunc,
			path:              pathOpenAIChatCompletions,
			expectProviderDir: config.ProviderOpenAI,
			failover:          true,
		},
		{
			name:              "openai_chat_completions_failover_streaming",
			fixture:           fixtures.OaiChatSimple,
			providerFunc:      openAIFunc,
			path:              pathOpenAIChatCompletions,
			expectProviderDir: config.ProviderOpenAI,
			streaming:         true,
			failover:          true,
		},
		{
			name:              "openai_responses_failover_blocking",
			fixture:           fixtures.OaiResponsesBlockingSimple,
			providerFunc:      openAIFunc,
			path:              pathOpenAIResponses,
			expectProviderDir: config.ProviderOpenAI,
			failover:          true,
		},
		{
			name:              "openai_responses_failover_streaming",
			fixture:           fixtures.OaiResponsesStreamingSimple,
			providerFunc:      openAIFunc,
			path:              pathOpenAIResponses,
			expectProviderDir: config.ProviderOpenAI,
			streaming:         true,
			failover:          true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// Setup mock upstream server. With failover, the first key is
			// rate limited and the second succeeds.
			fix := fixtures.Parse(t, tc.fixture)
			upstreamResponses := []testutil.UpstreamResponse{testutil.NewFixtureResponse(fix)}
			wantStatuses := []int{http.StatusOK}
			var pool *keypool.Pool
			if tc.failover {
				upstreamResponses = append([]testutil.UpstreamResponse{testutil.NewErrorResponse(http.StatusTooManyRequests, "60")}, upstreamResponses...)
				wantStatuses = append([]int{http.StatusTooManyRequests}, wantStatuses...)
				var err error
				pool, err = keypool.New(tc.expectProviderDir, []string{"k0", "k1"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
			}
			srv := testutil.NewMockUpstream(ctx, t, upstreamResponses...)

			// Create temp dir for API dumps.
			dumpDir := t.TempDir()

			bridgeServer := newBridgeTestServer(ctx, t, srv.URL,
				withCustomProvider(tc.providerFunc(srv.URL, dumpDir, pool)),
			)

			requestBody := fix.Request()
			if tc.streaming {
				var err error
				requestBody, err = sjson.SetBytes(requestBody, "stream", true)
				require.NoError(t, err)
			}

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, requestBody, tc.headers)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			received := srv.ReceivedRequests()
			require.Len(t, received, len(wantStatuses))

			// Verify dump files were created.
			interceptions := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			interceptionID := interceptions[0].ID

			// Find dump files for this interception by walking the dump
			// directory. Files are named
			// {timestamp}-{interceptionID}-{seq}.{req|resp}.txt, and Walk
			// visits them in lexical order, which is call order.
			var reqDumpFiles, respDumpFiles []string
			err = filepath.Walk(dumpDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.Contains(path, interceptionID) {
					return err
				}
				switch {
				case strings.HasSuffix(path, apidump.SuffixRequest):
					reqDumpFiles = append(reqDumpFiles, path)
				case strings.HasSuffix(path, apidump.SuffixResponse):
					respDumpFiles = append(respDumpFiles, path)
				}
				return nil
			})
			require.NoError(t, err)
			require.Len(t, reqDumpFiles, len(wantStatuses), "one request dump per upstream call")
			require.Len(t, respDumpFiles, len(wantStatuses), "one response dump per upstream call")

			for i, wantStatus := range wantStatuses {
				// Verify dump files are in the correct provider subdirectory.
				require.Contains(t, reqDumpFiles[i], filepath.Join(dumpDir, tc.expectProviderDir)+osSep,
					"request dump should be in the %s provider directory", tc.expectProviderDir)
				require.Contains(t, respDumpFiles[i], filepath.Join(dumpDir, tc.expectProviderDir)+osSep,
					"response dump should be in the %s provider directory", tc.expectProviderDir)

				// The request dump holds the body sent upstream on this call.
				// Compare semantically because key order may differ.
				reqDumpData, err := os.ReadFile(reqDumpFiles[i])
				require.NoError(t, err)
				dumpReq, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(reqDumpData)))
				require.NoError(t, err)
				dumpBody, err := io.ReadAll(dumpReq.Body)
				require.NoError(t, err)
				require.JSONEq(t, string(received[i].Body), string(dumpBody), "request dump %d", i)

				respDumpData, err := os.ReadFile(respDumpFiles[i])
				require.NoError(t, err)
				dumpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(respDumpData)), nil)
				require.NoError(t, err)
				dumpRespBody, err := io.ReadAll(dumpResp.Body)
				require.NoError(t, err)
				require.NoError(t, dumpResp.Body.Close())
				require.Equal(t, wantStatus, dumpResp.StatusCode, "response dump %d", i)
				if wantStatus == http.StatusOK && !tc.streaming {
					require.JSONEq(t, string(fix.NonStreaming()), string(dumpRespBody), "response body JSON should match semantically")
				}
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

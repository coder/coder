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
	"testing"

	"golang.org/x/tools/txtar"
	"storj.io/drpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

var (
	//go:embed fixtures/anthropic/single_builtin_tool.txtar
	antSingleBuiltinTool []byte

	//go:embed fixtures/openai/single_builtin_tool.txtar
	oaiSingleBuiltinTool []byte
)

const (
	fixtureRequest              = "request"
	fixtureStreamingResponse    = "streaming"
	fixtureNonStreamingResponse = "non-streaming"
)

func TestAnthropicMessages(t *testing.T) {
	t.Parallel()

	sessionToken := getSessionToken(t)

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			streaming bool
		}{
			{
				streaming: true,
			},
			{
				streaming: false,
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
				srv := newMockServer(ctx, t, files)
				t.Cleanup(srv.Close)

				coderdClient := &fakeBridgeDaemonClient{}

				logger := testutil.Logger(t) // slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				b, err := aibridged.NewBridge(codersdk.AIBridgeConfig{
					Daemons: 1,
					Anthropic: codersdk.AIBridgeAnthropicConfig{
						BaseURL: serpent.String(srv.URL),
						Key:     serpent.String(sessionToken),
					},
				}, "127.0.0.1:0", logger, func() (proto.DRPCAIBridgeDaemonClient, bool) {
					return coderdClient, true
				})
				require.NoError(t, err)

				go func() {
					assert.NoError(t, b.Serve())
				}()
				// Wait for bridge to come up.
				require.Eventually(t, func() bool { return len(b.Addr()) > 0 }, testutil.WaitLong, testutil.IntervalFast)

				// Make API call to aibridge for Anthropic /v1/messages
				req := createAnthropicMessagesReq(t, "http://"+b.Addr(), reqBody)
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
					require.Len(t, coderdClient.tokenUsages, 2) // One from message_start, one from message_delta.
				} else {
					require.Len(t, coderdClient.tokenUsages, 1)
				}

				assert.NotZero(t, calculateTotalInputTokens(coderdClient.tokenUsages))
				assert.NotZero(t, calculateTotalOutputTokens(coderdClient.tokenUsages))

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

	sessionToken := getSessionToken(t)

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			streaming bool
		}{
			{
				streaming: true,
			},
			{
				streaming: false,
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
				srv := newMockServer(ctx, t, files)
				t.Cleanup(srv.Close)

				coderdClient := &fakeBridgeDaemonClient{}

				logger := testutil.Logger(t) // slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
				b, err := aibridged.NewBridge(codersdk.AIBridgeConfig{
					Daemons: 1,
					OpenAI: codersdk.AIBridgeOpenAIConfig{
						BaseURL: serpent.String(srv.URL),
						Key:     serpent.String(sessionToken),
					},
				}, "127.0.0.1:0", logger, func() (proto.DRPCAIBridgeDaemonClient, bool) {
					return coderdClient, true
				})
				require.NoError(t, err)

				go func() {
					assert.NoError(t, b.Serve())
				}()
				// Wait for bridge to come up.
				require.Eventually(t, func() bool { return len(b.Addr()) > 0 }, testutil.WaitLong, testutil.IntervalFast)

				// Make API call to aibridge for OpenAI /v1/chat/completions
				req := createOpenAIChatCompletionsReq(t, "http://"+b.Addr(), reqBody)

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
				assert.NotZero(t, calculateTotalInputTokens(coderdClient.tokenUsages))
				assert.NotZero(t, calculateTotalOutputTokens(coderdClient.tokenUsages))

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

func calculateTotalOutputTokens(in []*proto.TrackTokenUsageRequest) int64 {
	var total int64
	for _, el := range in {
		total += el.InputTokens
	}
	return total
}

func calculateTotalInputTokens(in []*proto.TrackTokenUsageRequest) int64 {
	var total int64
	for _, el := range in {
		total += el.OutputTokens
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

func getSessionToken(t *testing.T) string {
	t.Helper()

	client := coderdtest.New(t, nil)
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
}

func newMockServer(ctx context.Context, t *testing.T, files archiveFileMap) *mockServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		require.NoError(t, err)

		type msg struct {
			Stream bool `json:"stream"`
		}
		var reqMsg msg
		require.NoError(t, json.Unmarshal(body, &reqMsg))

		if !reqMsg.Stream {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(files[fixtureNonStreamingResponse])
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		scanner := bufio.NewScanner(bytes.NewReader(files[fixtureStreamingResponse]))
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

	return &mockServer{
		Server: srv,
	}
}

type fakeBridgeDaemonClient struct {
	mu sync.Mutex

	tokenUsages []*proto.TrackTokenUsageRequest
	userPrompts []*proto.TrackUserPromptRequest
	toolUsages  []*proto.TrackToolUsageRequest
}

func (*fakeBridgeDaemonClient) DRPCConn() drpc.Conn {
	conn, _ := drpcsdk.MemTransportPipe()
	return conn
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

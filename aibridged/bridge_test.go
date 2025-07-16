package aibridged_test

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/txtar"
	"storj.io/drpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
)

func TestAnthropicMessagesStreaming(t *testing.T) {
	t.Parallel()

	sessionToken := getSessionToken(t)

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		arc := txtar.Parse(antSingleBuiltinTool)
		t.Logf("%s: %s", t.Name(), arc.Comment)
		require.Len(t, arc.Files, 2)
		reqBody, respBody := arc.Files[0], arc.Files[1]

		ctx := testutil.Context(t, testutil.WaitLong)
		srv := newFakeSSEServer(ctx, respBody.Data)
		t.Cleanup(srv.Close)

		coderdClient := &fakeBridgeDaemonClient{}

		logger := testutil.Logger(t)
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
		req := createAnthropicMessagesReq(t, "http://"+b.Addr(), string(reqBody.Data))
		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		sp := aibridged.NewSSEParser()
		require.NoError(t, sp.Parse(resp.Body))

		assert.Contains(t, sp.AllEvents(), "message_start")
		assert.Contains(t, sp.AllEvents(), "message_stop")

		require.Len(t, coderdClient.tokenUsages, 2) // One from message_start, one from message_delta.
		assert.NotZero(t, calculateTotalInputTokens(coderdClient.tokenUsages))
		assert.NotZero(t, calculateTotalOutputTokens(coderdClient.tokenUsages))

		require.Len(t, coderdClient.toolUsages, 1)
		assert.Equal(t, "Read", coderdClient.toolUsages[0].Tool)
		require.Contains(t, coderdClient.toolUsages[0].Input, "file_path")
		assert.Equal(t, "/tmp/blah/foo", coderdClient.toolUsages[0].Input["file_path"])

		require.Len(t, coderdClient.userPrompts, 1)
		assert.Equal(t, "read the foo file", coderdClient.userPrompts[0].Prompt)
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

func createAnthropicMessagesReq(t *testing.T, baseURL string, input string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), "POST", baseURL+"/v1/messages", strings.NewReader(input))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

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

type fakeSSEServer struct {
	*httptest.Server
}

func newFakeSSEServer(ctx context.Context, fixture []byte) *fakeSSEServer {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Stream the fixture content with random intervals
		scanner := bufio.NewScanner(bytes.NewReader(fixture))
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		for scanner.Scan() {
			line := scanner.Text()

			// Write the line
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()

			// // Add random delay between 10-50ms to mimic real streaming
			// delay := time.Duration(rand.Intn(40)+10) * time.Millisecond
			// select {
			// case <-ctx.Done():
			// 	return
			// case <-time.After(delay):
			// 	// Continue
			// }
		}

		if err := scanner.Err(); err != nil {
			http.Error(w, fmt.Sprintf("Error reading fixture: %v", err), http.StatusInternalServerError)
			return
		}
	}))

	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}

	return &fakeSSEServer{
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

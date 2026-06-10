package integrationtest

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/policy"
)

// blockToolPipeline builds a pre-tool pipeline that BLOCKs the named tool and
// allows everything else.
func blockToolPipeline(t *testing.T, toolName string) map[string]aibridge.ProviderPipelines {
	t.Helper()
	decide, err := policy.NewDecide("block-tool", `
default verdict := "ALLOW"
verdict := "BLOCK" if input.tool_call.name == "`+toolName+`"
`)
	require.NoError(t, err)
	pipe, err := policy.NewToolPipeline(policy.PipelineConfig{Decide: []*policy.Decide{decide}})
	require.NoError(t, err)
	return map[string]aibridge.ProviderPipelines{
		config.ProviderOpenAI:    {PreTool: pipe, Version: 1},
		config.ProviderAnthropic: {PreTool: pipe, Version: 1},
	}
}

func TestPreTool_BlocksClientToolCall_OpenAIStreaming(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	fix := fixtures.Parse(t, fixtures.OaiChatSingleBuiltinTool)
	upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
		withPolicyHooks(blockToolPipeline(t, "read_file")),
	)

	reqBody, err := sjson.SetBytes(fix.Request(), "stream", true)
	require.NoError(t, err)
	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	got := string(body)

	// The held tool-call chunks are discarded: the call id never reaches the
	// client.
	require.NotContains(t, got, "call_HjeqP7YeRkoNj0de9e3U4X4B")
	// The client receives an error naming the policy, then the [DONE] marker.
	require.Contains(t, got, "block-tool")
	require.Contains(t, got, "[DONE]")
}

func TestPreTool_AllowsClientToolCall_OpenAIStreaming(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	fix := fixtures.Parse(t, fixtures.OaiChatSingleBuiltinTool)
	upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
	// Pipeline blocks a *different* tool, so read_file is allowed through.
	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
		withPolicyHooks(blockToolPipeline(t, "some_other_tool")),
	)

	reqBody, err := sjson.SetBytes(fix.Request(), "stream", true)
	require.NoError(t, err)
	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	got := string(body)

	// The held tool-call chunks are flushed in order, so the call reaches the
	// client unchanged.
	require.Contains(t, got, "read_file")
	require.Contains(t, got, "call_HjeqP7YeRkoNj0de9e3U4X4B")
	require.NotContains(t, got, "block-tool")
	require.True(t, strings.Contains(got, "tool_calls"))
}

func TestPreTool_BlocksClientToolCall_AnthropicStreaming(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
	upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
		withPolicyHooks(blockToolPipeline(t, "Read")),
	)

	reqBody, err := sjson.SetBytes(fix.Request(), "stream", true)
	require.NoError(t, err)
	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	got := string(body)

	// The blocked tool_use block must never reach the client.
	require.NotContains(t, got, "toolu_01RX68weRSquLx6HUTj65iBo")
	// The client receives an error naming the policy.
	require.Contains(t, got, "block-tool")
}

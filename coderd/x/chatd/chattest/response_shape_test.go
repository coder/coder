package chattest_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

func TestFakeResponseShapeErrors(t *testing.T) {
	t.Parallel()

	openAI := func(path string) func(t *testing.T, both bool) string {
		return func(t *testing.T, both bool) string {
			return chattest.NewOpenAI(t, func(*chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !both {
					return chattest.OpenAIResponse{}
				}
				resp := chattest.OpenAINonStreamingResponse("")
				resp.StreamingChunks = chattest.OpenAIStreamingResponse().StreamingChunks
				return resp
			}) + path
		}
	}
	servers := []struct {
		name  string
		start func(t *testing.T, both bool) string
	}{
		{name: "OpenAICompletions", start: openAI("/chat/completions")},
		{name: "OpenAIResponses", start: openAI("/responses")},
		{name: "Anthropic", start: func(t *testing.T, both bool) string {
			return chattest.NewAnthropic(t, func(*chattest.AnthropicRequest) chattest.AnthropicResponse {
				if !both {
					return chattest.AnthropicResponse{}
				}
				resp := chattest.AnthropicNonStreamingResponse("")
				resp.StreamingChunks = chattest.AnthropicStreamingResponse().StreamingChunks
				return resp
			}) + "/v1/messages"
		}},
	}
	shapes := []struct {
		name    string
		both    bool
		wantMsg string
	}{
		{name: "Both", both: true, wantMsg: "handler returned both streaming and non-streaming responses"},
		{name: "Empty", wantMsg: "handler returned empty response"},
	}

	for _, server := range servers {
		t.Run(server.name, func(t *testing.T) {
			t.Parallel()
			for _, shape := range shapes {
				t.Run(shape.name, func(t *testing.T) {
					t.Parallel()

					ctx := testutil.Context(t, testutil.WaitShort)
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.start(t, shape.both), strings.NewReader("{}"))
					require.NoError(t, err)
					resp, err := http.DefaultClient.Do(req)
					require.NoError(t, err)
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
					require.Equal(t, shape.wantMsg+"\n", string(body))
				})
			}
		})
	}
}

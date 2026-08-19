package googleopenai_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/internal/googleopenai"
)

// The fixtures mirror live captures from Google's OpenAI-compatible endpoint
// with include_thoughts enabled: thought deltas carry
// extra_content.google.thought and the text is wrapped in <thought> markers,
// with the closing marker prefixed onto the first answer delta.
func TestRewriteThoughtResponse_Stream(t *testing.T) {
	t.Parallel()

	lines := []string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"<thought>**Calculating**\n\nStep one.","extra_content":{"google":{"thought":true}}},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":"More thinking.","extra_content":{"google":{"thought":true}}},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":"</thought>The answer is ","extra_content":null},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":"391."},"index":0}],"object":"chat.completion.chunk"}`,
		``,
		`data: {"choices":[{"delta":{"content":null,"extra_content":{"google":{"thought_signature":"sig123"}}},"finish_reason":"stop","index":0}],"usage":{"completion_tokens":9,"prompt_tokens":16,"total_tokens":627}}`,
		``,
		`data: [DONE]`,
		``,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(strings.Join(lines, "\n"))),
	}

	googleopenai.RewriteThoughtResponse(resp)
	rewritten, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	out := strings.Split(string(rewritten), "\n")
	require.Len(t, out, len(lines))

	first := gjson.Get(strings.TrimPrefix(out[0], "data: "), "choices.0.delta")
	require.Equal(t, "**Calculating**\n\nStep one.", first.Get("reasoning_content").String())
	require.False(t, first.Get("content").Exists())
	require.Equal(t, "assistant", first.Get("role").String())

	second := gjson.Get(strings.TrimPrefix(out[2], "data: "), "choices.0.delta")
	require.Equal(t, "More thinking.", second.Get("reasoning_content").String())
	require.False(t, second.Get("content").Exists())

	third := gjson.Get(strings.TrimPrefix(out[4], "data: "), "choices.0.delta")
	require.Equal(t, "The answer is ", third.Get("content").String())
	require.False(t, third.Get("reasoning_content").Exists())

	// Untouched lines pass through byte for byte.
	require.Equal(t, lines[6], out[6])
	require.Equal(t, lines[8], out[8])
	require.Equal(t, "data: [DONE]", out[10])
}

func TestRewriteThoughtResponse_StreamWithoutThoughts(t *testing.T) {
	t.Parallel()

	body := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\ndata: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	googleopenai.RewriteThoughtResponse(resp)
	rewritten, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(rewritten))
}

func TestRewriteThoughtResponse_StreamToolCallsEndThought(t *testing.T) {
	t.Parallel()

	lines := []string{
		`data: {"choices":[{"delta":{"content":"<thought>Pick a tool.","extra_content":{"google":{"thought":true}}},"index":0}]}`,
		``,
		`data: {"choices":[{"delta":{"content":null,"tool_calls":[{"index":0,"id":"call_1","function":{"name":"f","arguments":"{}"}}]},"index":0}]}`,
		``,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(strings.Join(lines, "\n"))),
	}

	googleopenai.RewriteThoughtResponse(resp)
	rewritten, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	out := strings.Split(string(rewritten), "\n")

	require.Equal(t, "Pick a tool.", gjson.Get(strings.TrimPrefix(out[0], "data: "), "choices.0.delta.reasoning_content").String())
	require.Equal(t, lines[2], out[2])
}

func TestRewriteThoughtResponse_JSONCompletion(t *testing.T) {
	t.Parallel()

	body := `{"choices":[{"message":{"role":"assistant","content":"<thought>**Quick**\n\nThinking here.\n</thought>17 * 23 = **391**","extra_content":{"google":{"thought":true,"thought_signature":"sig"}}},"index":0}],"object":"chat.completion"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	googleopenai.RewriteThoughtResponse(resp)
	rewritten, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, resp.ContentLength, int64(len(rewritten)))

	message := gjson.GetBytes(rewritten, "choices.0.message")
	require.Equal(t, "**Quick**\n\nThinking here.\n", message.Get("reasoning_content").String())
	require.Equal(t, "17 * 23 = **391**", message.Get("content").String())
	require.Equal(t, "sig", message.Get("extra_content.google.thought_signature").String())
}

func TestRewriteThoughtCompletion(t *testing.T) {
	t.Parallel()

	t.Run("ThoughtOnlyContent", func(t *testing.T) {
		t.Parallel()
		out := googleopenai.RewriteThoughtCompletion([]byte(`{"choices":[{"message":{"content":"<thought>All thought, no close marker.","extra_content":{"google":{"thought":true}}}}]}`))
		message := gjson.GetBytes(out, "choices.0.message")
		require.Equal(t, "All thought, no close marker.", message.Get("reasoning_content").String())
		require.Equal(t, "", message.Get("content").String())
	})

	t.Run("MarkerTextWithoutThoughtMetadataUnchanged", func(t *testing.T) {
		t.Parallel()
		// An answer that legitimately begins with the marker text must not
		// be reclassified as reasoning; real thought output always carries
		// extra_content.google.thought.
		body := `{"choices":[{"message":{"content":"<thought>example XML</thought> is the requested format."}}]}`
		require.Equal(t, body, string(googleopenai.RewriteThoughtCompletion([]byte(body))))
	})

	t.Run("NoThoughtsUnchanged", func(t *testing.T) {
		t.Parallel()
		body := `{"choices":[{"message":{"content":"Just an answer."}}]}`
		require.Equal(t, body, string(googleopenai.RewriteThoughtCompletion([]byte(body))))
	})

	t.Run("InvalidJSONUnchanged", func(t *testing.T) {
		t.Parallel()
		body := `not json`
		require.Equal(t, body, string(googleopenai.RewriteThoughtCompletion([]byte(body))))
	})
}

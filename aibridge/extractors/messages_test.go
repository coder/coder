package extractors_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/extractors"
	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	"github.com/coder/coder/v2/testutil"
)

// captureResponse returns a finalized capture of body, as forwarding leaves it.
func captureResponse(t *testing.T, body []byte) *replaybody.TeeBuffer {
	t.Helper()
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(body))}
	capture := replaybody.NewResponse(resp)
	_, err := io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, capture.Close())
	return capture
}

func TestMessagesExtractRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		body           string
		wantErr        string
		wantModel      string
		wantToolCallID *string
		// wantPrompt is the prompt recorded with the response message ID.
		wantPrompt *string
	}{
		{
			name:           "AllFields",
			body:           `{"model":"claude-test","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"},{"type":"text","text":"hi"}]}]}`,
			wantModel:      "claude-test",
			wantToolCallID: new("toolu_1"),
			wantPrompt:     new("hi"),
		},
		{
			name:      "NoPrompt",
			body:      `{"model":"claude-test","messages":[]}`,
			wantModel: "claude-test",
		},
		{
			name:       "EmptyPrompt",
			body:       `{"model":"claude-test","messages":[{"role":"user","content":""}]}`,
			wantModel:  "claude-test",
			wantPrompt: new(""),
		},
		{
			name:      "PromptError",
			body:      `{"model":"claude-test","messages":{}}`,
			wantErr:   "read last user prompt",
			wantModel: "claude-test",
		},
		{
			name:    "InvalidBody",
			body:    `not JSON`,
			wantErr: "parse messages request",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := extractors.NewMessages(slog.Make())
			start := &recorder.InterceptionRecord{}
			err := e.ExtractRequest([]byte(tc.body), start)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantModel, start.Model)
			require.Equal(t, tc.wantToolCallID, start.CorrelatingToolCallID)

			header := http.Header{"Content-Type": {"application/json"}}
			got, err := e.ExtractResponse(t.Context(), http.StatusOK, header, captureResponse(t, []byte(`{"id":"msg_1"}`)))
			require.NoError(t, err)
			var wantPrompt *recorder.PromptUsageRecord
			if tc.wantPrompt != nil {
				wantPrompt = &recorder.PromptUsageRecord{MsgID: "msg_1", Prompt: *tc.wantPrompt}
			}
			require.Equal(t, wantPrompt, got.Prompt)
		})
	}
}

func TestMessagesExtractResponse(t *testing.T) {
	t.Parallel()
	const message = `{"id":"msg_1","model":"claude-test-1","content":[` +
		`{"type":"thinking","thinking":"pondering","signature":"sig"},` +
		`{"type":"redacted_thinking","data":"secret"},` +
		`{"type":"text","text":"reading"},` +
		`{"type":"tool_use","id":"toolu_1","name":"Read","input":{"path":"/a"}}],` +
		`"usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4,` +
		`"cache_creation":{"ephemeral_1h_input_tokens":5,"ephemeral_5m_input_tokens":6},` +
		`"server_tool_use":{"web_search_requests":7},"service_tier":"priority"}}`
	extracted := extractors.ResponseResult{
		Prompt: &recorder.PromptUsageRecord{MsgID: "msg_1", Prompt: "hi"},
		Tokens: []recorder.TokenUsageRecord{{
			MsgID:                 "msg_1",
			ProviderModel:         "claude-test-1",
			Input:                 1,
			Output:                2,
			CacheReadInputTokens:  3,
			CacheWriteInputTokens: 4,
			ExtraTokenTypes: map[string]int64{
				"cache_ephemeral_1h_input": 5,
				"cache_ephemeral_5m_input": 6,
				"web_search_requests":      7,
			},
			Metadata: recorder.Metadata{recorder.MetadataKeyServiceTier: "priority"},
		}},
		Tools: []recorder.ToolUsageRecord{{
			MsgID:      "msg_1",
			ToolCallID: "toolu_1",
			Tool:       "Read",
			Args:       json.RawMessage(`{"path":"/a"}`),
		}},
		Thoughts: []recorder.ModelThoughtRecord{{
			Content:  "pondering",
			Metadata: recorder.Metadata{"source": recorder.ThoughtSourceThinking},
		}},
	}

	var response struct {
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal([]byte(message), &response))

	withoutToolInput := extracted
	withoutToolInput.Tools = slices.Clone(extracted.Tools)
	withoutToolInput.Tools[0].Args = json.RawMessage(nil)

	partialUsage := extracted
	partialUsage.Tokens = slices.Clone(extracted.Tokens)
	partialUsage.Tokens[0].Output = 0

	for _, tc := range []struct {
		name string
		// streaming is the request's stream flag.
		streaming   bool
		status      int
		contentType string
		body        []byte // Nil uses the default Message response.
		// noResponse passes a nil body, as when no upstream response was
		// accepted.
		noResponse   bool
		setFields    map[string]any
		deleteFields []string
		want         extractors.ResponseResult
		wantErr      string
		wantLogErr   string
		// wantErrorStatus and wantErrorMessage describe the decoded error
		// response. Zero wantErrorStatus means none.
		wantErrorStatus  int
		wantErrorMessage string
	}{
		{
			name:        "JSON",
			status:      http.StatusOK,
			contentType: "application/json",
			want:        extracted,
		},
		{
			name:        "MixedToolCallTypes",
			status:      http.StatusOK,
			contentType: "application/json",
			setFields: map[string]any{
				"content": append([]json.RawMessage{
					json.RawMessage(`{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"q"}}`),
					json.RawMessage(`{"type":"mcp_tool_use","id":"mcptoolu_1","name":"list_issues","server_name":"github","input":{"repo":"r"}}`),
					json.RawMessage(`{"type":"future_block","id":"future_1"}`),
				}, response.Content...),
			},
			want: extractors.ResponseResult{
				Prompt:   extracted.Prompt,
				Tokens:   extracted.Tokens,
				Thoughts: extracted.Thoughts,
				Tools: []recorder.ToolUsageRecord{
					{
						MsgID:      "msg_1",
						ToolCallID: "srvtoolu_1",
						Tool:       "web_search",
						Args:       json.RawMessage(`{"query":"q"}`),
					},
					{
						MsgID:      "msg_1",
						ToolCallID: "mcptoolu_1",
						Tool:       "list_issues",
						Args:       json.RawMessage(`{"repo":"r"}`),
					},
					extracted.Tools[0],
				},
			},
		},
		{
			name:         "MissingToolInput",
			status:       http.StatusOK,
			contentType:  "application/json",
			deleteFields: []string{"content.3.input"},
			want:         withoutToolInput,
		},
		{
			name:        "NonObjectResponse",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        []byte("[]"),
			wantErr:     "expected response object",
		},
		{
			name:        "InvalidUsageTokenType",
			status:      http.StatusOK,
			contentType: "application/json",
			setFields:   map[string]any{"usage.output_tokens": true},
			want:        partialUsage,
			wantErr:     "unexpected output_tokens type",
		},
		{
			name:        "InvalidUsageTokenValue",
			status:      http.StatusOK,
			contentType: "application/json",
			setFields:   map[string]any{"usage.output_tokens": 1.5},
			want:        partialUsage,
			wantErr:     "invalid integer in output_tokens",
		},
		{
			name:        "InvalidToolIDType",
			status:      http.StatusOK,
			contentType: "application/json",
			setFields:   map[string]any{"content.3.id": 42},
			want: extractors.ResponseResult{
				Prompt:   extracted.Prompt,
				Tokens:   extracted.Tokens,
				Thoughts: extracted.Thoughts,
			},
			wantErr: "unexpected id type",
		},
		{
			name:        "InvalidContentBlock",
			status:      http.StatusOK,
			contentType: "application/json",
			setFields: map[string]any{
				"content": append([]json.RawMessage{json.RawMessage("42")}, response.Content...),
			},
			want:    extracted,
			wantErr: "unexpected content block at index 0",
		},
		{
			name:        "InvalidContentShape",
			status:      http.StatusOK,
			contentType: "application/json",
			setFields:   map[string]any{"content": map[string]any{"items": response.Content}},
			want: extractors.ResponseResult{
				Prompt: extracted.Prompt,
				Tokens: extracted.Tokens,
			},
			wantErr: "unexpected content type",
		},
		{
			// The response content type takes precedence over the request.
			name:        "JSONToStreamingRequest",
			streaming:   true,
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			want:        extracted,
		},
		{
			name:       "MissingContentType",
			status:     http.StatusOK,
			want:       extracted,
			wantLogErr: "missing response content type",
		},
		{
			// TODO: extract streamed responses. https://linear.app/codercom/issue/AIGOV-616
			name:       "MissingContentTypeStreamingRequest",
			streaming:  true,
			status:     http.StatusOK,
			wantLogErr: "missing response content type",
		},
		{
			name:        "MalformedContentType",
			status:      http.StatusOK,
			contentType: "application/json; charset",
			want:        extracted,
			wantLogErr:  "mime: invalid media parameter",
		},
		{
			name:        "UnsupportedContentTypeStreamingRequest",
			streaming:   true,
			status:      http.StatusOK,
			contentType: "text/plain",
			wantLogErr:  `unsupported response content type "text/plain"`,
		},
		{
			// TODO: extract streamed responses. https://linear.app/codercom/issue/AIGOV-616
			name:        "EventStream",
			status:      http.StatusOK,
			contentType: "text/event-stream",
		},
		{
			name:        "InvalidJSON",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        []byte("{"),
			wantErr:     "decode messages response",
		},
		{
			name:       "NoResponse",
			noResponse: true,
		},
		{
			// Error responses carry an error envelope whatever the request
			// asked for.
			name:             "ErrorResponse",
			streaming:        true,
			status:           529,
			contentType:      "text/event-stream",
			body:             []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`),
			wantErrorStatus:  529,
			wantErrorMessage: "Overloaded",
		},
		{
			// Without an envelope, the response status classifies the error.
			name:        "ErrorWithoutEnvelope",
			status:      http.StatusBadGateway,
			contentType: "text/html",
			body:        []byte("<html>bad gateway</html>"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := testutil.NewFakeSink(t)
			e := extractors.NewMessages(sink.Logger())
			request := fmt.Sprintf(`{"model":"claude-test","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, tc.streaming)
			require.NoError(t, e.ExtractRequest([]byte(request), &recorder.InterceptionRecord{}))

			header := http.Header{}
			if tc.contentType != "" {
				header.Set("Content-Type", tc.contentType)
			}
			body := tc.body
			if body == nil {
				body = []byte(message)
			}
			for path, value := range tc.setFields {
				var err error
				body, err = sjson.SetBytes(body, path, value)
				require.NoError(t, err)
			}
			for _, path := range tc.deleteFields {
				var err error
				body, err = sjson.DeleteBytes(body, path)
				require.NoError(t, err)
			}
			var capture *replaybody.TeeBuffer
			if !tc.noResponse {
				capture = captureResponse(t, body)
			}
			got, err := e.ExtractResponse(t.Context(), tc.status, header, capture)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantErrorStatus != 0 {
				var responseErr *messages.ResponseError
				require.True(t, errors.As(got.ResponseError, &responseErr), "unexpected response error: %v", got.ResponseError)
				require.Equal(t, tc.wantErrorStatus, responseErr.StatusCode)
				require.Equal(t, tc.wantErrorMessage, responseErr.Error())
				got.ResponseError = nil
			}
			require.Equal(t, tc.want, got)
			entries := sink.Entries()
			if tc.wantLogErr != "" {
				require.Len(t, entries, 1)
				require.Equal(t, slog.LevelDebug, entries[0].Level)
				var loggedErr error
				for _, field := range entries[0].Fields {
					if field.Name == "error" {
						loggedErr, _ = field.Value.(error)
					}
				}
				require.ErrorContains(t, loggedErr, tc.wantLogErr)
			} else {
				require.Empty(t, entries)
			}
		})
	}
}

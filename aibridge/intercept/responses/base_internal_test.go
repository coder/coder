package responses

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	oairesponses "github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/quartz"
)

func TestRecordPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		promptWasRecorded bool
		prompt            string
		responseID        string
		wantRecorded      bool
		wantPrompt        string
	}{
		{
			name:         "records_prompt_successfully",
			prompt:       "tell me a joke",
			responseID:   "resp_123",
			wantRecorded: true,
			wantPrompt:   "tell me a joke",
		},
		{
			name:         "records_empty_prompt_successfully",
			prompt:       "",
			responseID:   "resp_123",
			wantRecorded: true,
			wantPrompt:   "",
		},
		{
			name:         "skips_recording_on_empty_response_id",
			prompt:       "tell me a joke",
			responseID:   "",
			wantRecorded: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &testutil.MockRecorder{}
			id := uuid.New()
			base := &responsesInterceptionBase{
				id:       id,
				recorder: rec,
				logger:   slog.Make(),
			}

			base.recordUserPrompt(t.Context(), tc.responseID, tc.prompt)

			prompts := rec.RecordedPromptUsages()
			if tc.wantRecorded {
				require.Len(t, prompts, 1)
				require.Equal(t, id.String(), prompts[0].InterceptionID)
				require.Equal(t, tc.responseID, prompts[0].MsgID)
				require.Equal(t, tc.wantPrompt, prompts[0].Prompt)
			} else {
				require.Empty(t, prompts)
			}
		})
	}
}

func TestRecordToolUsage(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	tests := []struct {
		name     string
		response *oairesponses.Response
		expected []*recorder.ToolUsageRecord
	}{
		{
			name:     "nil_response",
			response: nil,
			expected: nil,
		},
		{
			name: "empty_output",
			response: &oairesponses.Response{
				ID: "resp_123",
			},
			expected: nil,
		},
		{
			name: "empty_tool_args",
			response: &oairesponses.Response{
				ID: "resp_456",
				Output: []oairesponses.ResponseOutputItemUnion{
					{
						Type:      "function_call",
						CallID:    "call_abc",
						Name:      "get_weather",
						Arguments: "",
					},
				},
			},
			expected: []*recorder.ToolUsageRecord{
				{
					InterceptionID: id.String(),
					MsgID:          "resp_456",
					ToolCallID:     "call_abc",
					Tool:           "get_weather",
					Args:           "",
					Injected:       false,
				},
			},
		},
		{
			name: "multiple_tool_calls",
			response: &oairesponses.Response{
				ID: "resp_789",
				Output: []oairesponses.ResponseOutputItemUnion{
					{
						Type:      "function_call",
						CallID:    "call_1",
						Name:      "get_weather",
						Arguments: `{"location": "NYC"}`,
					},
					{
						Type:      "function_call",
						CallID:    "call_2",
						Name:      "bad_json_args",
						Arguments: `{"bad": args`,
					},
					{
						Type: "message",
						ID:   "msg_1",
						Role: "assistant",
					},
					{
						Type:   "custom_tool_call",
						CallID: "call_3",
						Name:   "search",
						Input:  `{\"query\": \"test\"}`,
					},
					{
						Type:      "function_call",
						CallID:    "call_4",
						Name:      "calculate",
						Arguments: `{"a": 1, "b": 2}`,
					},
				},
			},
			expected: []*recorder.ToolUsageRecord{
				{
					InterceptionID: id.String(),
					MsgID:          "resp_789",
					ToolCallID:     "call_1",
					Tool:           "get_weather",
					Args:           map[string]any{"location": "NYC"},
					Injected:       false,
				},
				{
					InterceptionID: id.String(),
					MsgID:          "resp_789",
					ToolCallID:     "call_2",
					Tool:           "bad_json_args",
					Args:           `{"bad": args`,
					Injected:       false,
				},
				{
					InterceptionID: id.String(),
					MsgID:          "resp_789",
					ToolCallID:     "call_3",
					Tool:           "search",
					Args:           `{\"query\": \"test\"}`,
					Injected:       false,
				},
				{
					InterceptionID: id.String(),
					MsgID:          "resp_789",
					ToolCallID:     "call_4",
					Tool:           "calculate",
					Args:           map[string]any{"a": float64(1), "b": float64(2)},
					Injected:       false,
				},
			},
		},
		{
			// Function/agentic tools expose both id and call_id; both are captured.
			name: "function_call_captures_both_ids",
			response: &oairesponses.Response{
				ID: "resp_both",
				Output: []oairesponses.ResponseOutputItemUnion{
					{
						Type:      "function_call",
						ID:        "fc_item_1",
						CallID:    "call_both",
						Name:      "get_weather",
						Arguments: `{"location": "NYC"}`,
					},
				},
			},
			expected: []*recorder.ToolUsageRecord{
				{
					InterceptionID: id.String(),
					MsgID:          "resp_both",
					ItemID:         "fc_item_1",
					ToolCallID:     "call_both",
					Tool:           "get_weather",
					Args:           map[string]any{"location": "NYC"},
					Injected:       false,
				},
			},
		},
		{
			// Hosted tools only have id (no call_id) and usually no name, so
			// the type is recorded as the tool name and ToolCallID is empty.
			name: "hosted_tool_uses_item_id_no_call_id",
			response: &oairesponses.Response{
				ID: "resp_ws",
				Output: []oairesponses.ResponseOutputItemUnion{
					{
						Type: "web_search_call",
						ID:   "ws_abc",
					},
				},
			},
			expected: []*recorder.ToolUsageRecord{
				{
					InterceptionID: id.String(),
					MsgID:          "resp_ws",
					ItemID:         "ws_abc",
					ToolCallID:     "",
					Tool:           "web_search_call",
					Injected:       false,
				},
			},
		},
		{
			// Exercises every newly recorded tool type, the name-falls-back-to
			// -type behavior, an explicit name override (mcp_call), and that
			// non-tool output items (reasoning) are still skipped.
			name: "all_additional_tool_types",
			response: &oairesponses.Response{
				ID: "resp_all",
				Output: []oairesponses.ResponseOutputItemUnion{
					{Type: "reasoning", ID: "rs_skip"},
					{Type: "web_search_call", ID: "ws_1"},
					{Type: "computer_call", ID: "cu_1", CallID: "call_cu"},
					{Type: "local_shell_call", ID: "ls_1", CallID: "call_ls"},
					{Type: "shell_call", ID: "sh_1", CallID: "call_sh"},
					{Type: "apply_patch_call", ID: "ap_1", CallID: "call_ap"},
					{Type: "code_interpreter_call", ID: "ci_1"},
					{Type: "mcp_call", ID: "mcp_1", Name: "fetch"},
					{Type: "file_search_call", ID: "fs_1"},
					{Type: "image_generation_call", ID: "ig_1"},
				},
			},
			expected: []*recorder.ToolUsageRecord{
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "ws_1", Tool: "web_search_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "cu_1", ToolCallID: "call_cu", Tool: "computer_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "ls_1", ToolCallID: "call_ls", Tool: "local_shell_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "sh_1", ToolCallID: "call_sh", Tool: "shell_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "ap_1", ToolCallID: "call_ap", Tool: "apply_patch_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "ci_1", Tool: "code_interpreter_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "mcp_1", Tool: "fetch"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "fs_1", Tool: "file_search_call"},
				{InterceptionID: id.String(), MsgID: "resp_all", ItemID: "ig_1", Tool: "image_generation_call"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &testutil.MockRecorder{}
			base := &responsesInterceptionBase{
				id:       id,
				recorder: rec,
				logger:   slog.Make(),
			}

			base.recordNonInjectedToolUsage(t.Context(), tc.response)

			tools := rec.RecordedToolUsages()
			require.Len(t, tools, len(tc.expected))
			for i, got := range tools {
				got.CreatedAt = time.Time{}
				require.Equal(t, tc.expected[i], got)
			}
		})
	}
}

func TestParseJSONArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected recorder.ToolArgs
	}{
		{
			name:     "empty_string",
			raw:      "",
			expected: "",
		},
		{
			name:     "whitespace_only",
			raw:      "   \t\n  ",
			expected: "",
		},
		{
			name:     "invalid_json",
			raw:      "{not valid json}",
			expected: "{not valid json}",
		},
		{
			name: "nested_object_with_trailing_spaces",
			raw:  ` {"user": {"name": "alice", "settings": {"theme": "dark", "notifications": true}}, "count": 42}   `,
			expected: map[string]any{
				"user": map[string]any{
					"name": "alice",
					"settings": map[string]any{
						"theme":         "dark",
						"notifications": true,
					},
				},
				"count": float64(42),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := &responsesInterceptionBase{}
			result := base.parseFunctionCallJSONArgs(t.Context(), tc.raw)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestRecordTokenUsage(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name     string
		response *oairesponses.Response
		expected *recorder.TokenUsageRecord
	}{
		{
			name:     "nil_response",
			response: nil,
			expected: nil,
		},
		{
			name: "with_all_token_details",
			response: &oairesponses.Response{
				ID: "resp_full",
				Usage: oairesponses.ResponseUsage{
					InputTokens:  10,
					OutputTokens: 20,
					TotalTokens:  30,
					InputTokensDetails: oairesponses.ResponseUsageInputTokensDetails{
						CachedTokens: 5,
					},
					OutputTokensDetails: oairesponses.ResponseUsageOutputTokensDetails{
						ReasoningTokens: 5,
					},
				},
			},
			expected: &recorder.TokenUsageRecord{
				InterceptionID:       id.String(),
				MsgID:                "resp_full",
				Input:                5, // 10 input - 5 cached
				Output:               20,
				CacheReadInputTokens: 5,
				ExtraTokenTypes: map[string]int64{
					"output_reasoning": 5,
					"total_tokens":     30,
				},
			},
		},
		{
			// Upstream violates the invariant that InputTokens includes
			// CachedTokens. Input must clamp to 0 so it never panics a
			// Prometheus counter when used as an increment.
			name: "cached_tokens_exceed_input_tokens_clamps_to_zero",
			response: &oairesponses.Response{
				ID: "resp_clamp",
				Usage: oairesponses.ResponseUsage{
					InputTokens:  10,
					OutputTokens: 20,
					TotalTokens:  30,
					InputTokensDetails: oairesponses.ResponseUsageInputTokensDetails{
						CachedTokens: 40,
					},
				},
			},
			expected: &recorder.TokenUsageRecord{
				InterceptionID:       id.String(),
				MsgID:                "resp_clamp",
				Input:                0, // max(0, 10 input - 40 cached)
				Output:               20,
				CacheReadInputTokens: 40,
				ExtraTokenTypes: map[string]int64{
					"output_reasoning": 0,
					"total_tokens":     30,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &testutil.MockRecorder{}
			base := &responsesInterceptionBase{
				id:       id,
				recorder: rec,
				logger:   slog.Make(),
			}

			base.recordTokenUsage(t.Context(), tc.response)

			tokens := rec.RecordedTokenUsages()
			if tc.expected == nil {
				require.Empty(t, tokens)
			} else {
				require.Len(t, tokens, 1)
				got := tokens[0]
				got.CreatedAt = time.Time{} // ignore time
				require.Equal(t, tc.expected, got)
			}
		})
	}
}

type mockResponseWriter struct {
	headerCalled      bool
	writeCalled       bool
	writeHeaderCalled bool
}

func (mrw *mockResponseWriter) Header() http.Header {
	mrw.headerCalled = true
	return http.Header{}
}

func (mrw *mockResponseWriter) Write([]byte) (int, error) {
	mrw.writeCalled = true
	return 0, nil
}

func (mrw *mockResponseWriter) WriteHeader(statusCode int) {
	mrw.writeHeaderCalled = true
}

func TestResponseCopierDoesntSendIfNoResponseReceived(t *testing.T) {
	t.Parallel()

	mrw := mockResponseWriter{}

	respCopy := responseCopier{}
	body := "test_body"
	_, _ = respCopy.buff.Write([]byte(body)) // bytes.Buffer.Write never fails

	err := respCopy.forwardResp(&mrw)
	require.NoError(t, err)
	require.False(t, mrw.headerCalled)
	require.False(t, mrw.writeCalled)
	require.False(t, mrw.writeHeaderCalled)

	// after response is received data is forwarded
	respCopy.responseReceived.Store(true)

	err = respCopy.forwardResp(&mrw)
	require.NoError(t, err)
	require.True(t, mrw.headerCalled)
	require.True(t, mrw.writeCalled)
	require.True(t, mrw.writeHeaderCalled)
}

func TestMarkKeyOnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		expectedReturn bool
		expectedState  keypool.KeyState
	}{
		{
			// Not an *openai.Error: no status code to act on.
			name:           "non_api_error_returns_false",
			err:            xerrors.New("network failure"),
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
		{
			// Rate-limited: temporary cooldown.
			name:           "429_marks_temporary",
			err:            &openai.Error{StatusCode: http.StatusTooManyRequests, Response: &http.Response{StatusCode: http.StatusTooManyRequests}},
			expectedReturn: true,
			expectedState:  keypool.KeyStateTemporary,
		},
		{
			// Auth failure: mark permanent.
			name:           "401_marks_permanent",
			err:            &openai.Error{StatusCode: http.StatusUnauthorized, Response: &http.Response{StatusCode: http.StatusUnauthorized}},
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
		},
		{
			// Auth forbidden: mark permanent.
			name:           "403_marks_permanent",
			err:            &openai.Error{StatusCode: http.StatusForbidden, Response: &http.Response{StatusCode: http.StatusForbidden}},
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
		},
		{
			// Server errors are not key-specific.
			name:           "500_does_not_mark",
			err:            &openai.Error{StatusCode: http.StatusInternalServerError, Response: &http.Response{StatusCode: http.StatusInternalServerError}},
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, err := keypool.New(config.ProviderOpenAI, []string{"key-0"}, quartz.NewMock(t), nil)
			require.NoError(t, err)
			key, keyPoolErr := pool.Walker().Next()
			require.Nil(t, keyPoolErr)

			base := &responsesInterceptionBase{cred: &intercept.CentralizedPool{Pool: pool}, logger: slog.Make()}

			got := base.markKeyOnError(context.Background(), key, tc.err)
			assert.Equal(t, tc.expectedReturn, got)
			assert.Equal(t, tc.expectedState, key.State())
		})
	}
}

func TestWriteUpstreamError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		respErr      *intercept.ResponseError
		expectStatus int
		// Empty string means the header should be absent.
		expectRetryAfter string
		// Substring expected in the marshaled body. Empty means no body check.
		expectBodyContains string
	}{
		{
			// Standard error: status, code, and JSON body written.
			name:               "writes_status_and_body",
			respErr:            intercept.NewResponseError("upstream failed", "api_error", "server_error", http.StatusBadGateway, 0),
			expectStatus:       http.StatusBadGateway,
			expectBodyContains: `"upstream failed"`,
		},
		{
			// OpenAI envelope: the code field round-trips into the body.
			name:               "writes_code_field",
			respErr:            intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 0),
			expectStatus:       http.StatusTooManyRequests,
			expectBodyContains: `"rate_limit_exceeded"`,
		},
		{
			// Whole-second retryAfter: emitted as integer seconds.
			name:             "retry_after_in_seconds",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 60*time.Second),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "60",
		},
		{
			// 500ms rounds up to Retry-After: 1.
			name:             "retry_after_500ms_rounds_up_to_one",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 500*time.Millisecond),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "1",
		},
		{
			// 200ms rounds up to Retry-After: 1.
			name:             "retry_after_200ms_rounds_up_to_one",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, 200*time.Millisecond),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "1",
		},
		{
			// Negative retryAfter: header omitted.
			name:             "negative_retry_after_omits_header",
			respErr:          intercept.NewResponseError("rate limited", "rate_limit_error", "rate_limit_exceeded", http.StatusTooManyRequests, -1*time.Second),
			expectStatus:     http.StatusTooManyRequests,
			expectRetryAfter: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := &responsesInterceptionBase{logger: slog.Make()}

			w := httptest.NewRecorder()
			base.writeUpstreamError(w, tc.respErr)

			assert.Equal(t, tc.expectStatus, w.Code, "status code")
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Content-Type header")
			assert.Equal(t, tc.expectRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
			if tc.expectBodyContains != "" {
				assert.Contains(t, w.Body.String(), tc.expectBodyContains, "response body")
			}
		})
	}
}

package workspacesdk_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestToolCallHeadersRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		toolCall   workspacesdk.ToolCall
		wantIDHdr  string
		wantAgeHdr string
		wantAge    time.Duration
	}{
		{
			name:       "plain ID",
			toolCall:   workspacesdk.ToolCall{MessageID: 1, ID: "toolu_01ABC", Age: 1500 * time.Millisecond},
			wantIDHdr:  "toolu_01ABC",
			wantAgeHdr: "1500",
			wantAge:    1500 * time.Millisecond,
		},
		{
			name:       "slash",
			toolCall:   workspacesdk.ToolCall{MessageID: 42, ID: "call/1", Age: 0},
			wantIDHdr:  "call%2F1",
			wantAgeHdr: "0",
			wantAge:    0,
		},
		{
			name:       "percent",
			toolCall:   workspacesdk.ToolCall{MessageID: 7, ID: "100%x", Age: time.Second},
			wantIDHdr:  "100%25x",
			wantAgeHdr: "1000",
			wantAge:    time.Second,
		},
		{
			name:       "space",
			toolCall:   workspacesdk.ToolCall{MessageID: 7, ID: "a b", Age: time.Minute},
			wantIDHdr:  "a%20b",
			wantAgeHdr: "60000",
			wantAge:    time.Minute,
		},
		{
			name:       "non-ASCII",
			toolCall:   workspacesdk.ToolCall{MessageID: 9, ID: "工具-é", Age: time.Hour},
			wantIDHdr:  "%E5%B7%A5%E5%85%B7-%C3%A9",
			wantAgeHdr: "3600000",
			wantAge:    time.Hour,
		},
		{
			name:       "age truncated to milliseconds",
			toolCall:   workspacesdk.ToolCall{MessageID: 3, ID: "x", Age: 2*time.Millisecond + 999*time.Microsecond},
			wantIDHdr:  "x",
			wantAgeHdr: "2",
			wantAge:    2 * time.Millisecond,
		},
		{
			name:       "negative age written as zero",
			toolCall:   workspacesdk.ToolCall{MessageID: 3, ID: "x", Age: -time.Second},
			wantIDHdr:  "x",
			wantAgeHdr: "0",
			wantAge:    0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := http.Header{}
			tc.toolCall.SetHeaders(h)
			assert.Equal(t, tc.wantIDHdr, h.Get(workspacesdk.CoderToolCallIDHeader))
			assert.Equal(t, tc.wantAgeHdr, h.Get(workspacesdk.CoderToolCallAgeMsHeader))

			got, ok, err := workspacesdk.ToolCallFromHeaders(h)
			require.NoError(t, err)
			require.True(t, ok)
			want := tc.toolCall
			want.Age = tc.wantAge
			assert.Equal(t, want, got)
		})
	}
}

func TestToolCallFromHeaders(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		workspacesdk.CoderToolCallMessageIDHeader: "42",
		workspacesdk.CoderToolCallIDHeader:        "toolu_01",
		workspacesdk.CoderToolCallAgeMsHeader:     "250",
	}
	validHeader := func() http.Header {
		h := http.Header{}
		for k, v := range valid {
			h.Set(k, v)
		}
		return h
	}
	// with returns valid headers with name set to value, or removed
	// when value is nil.
	with := func(name string, value *string) http.Header {
		h := validHeader()
		if value == nil {
			h.Del(name)
		} else {
			h.Set(name, *value)
		}
		return h
	}
	// twice returns valid headers with a second value for name.
	twice := func(name string) http.Header {
		h := validHeader()
		h.Add(name, valid[name])
		return h
	}
	ptr := func(s string) *string { return &s }
	validToolCall := workspacesdk.ToolCall{MessageID: 42, ID: "toolu_01", Age: 250 * time.Millisecond}
	// maxAgeMs is the largest age in milliseconds that fits in a
	// time.Duration.
	const maxAgeMs = 9223372036854

	cases := []struct {
		name      string
		header    http.Header
		wantOK    bool
		want      workspacesdk.ToolCall
		wantErrIn string // header name the error must mention, empty for no error
	}{
		{name: "none present", header: http.Header{"Coder-Chat-Id": {uuid.NewString()}}},
		{name: "all valid", header: validHeader(), wantOK: true, want: validToolCall},
		{
			name:   "age at duration limit",
			header: with(workspacesdk.CoderToolCallAgeMsHeader, ptr("9223372036854")),
			wantOK: true,
			want:   workspacesdk.ToolCall{MessageID: 42, ID: "toolu_01", Age: maxAgeMs * time.Millisecond},
		},
		{name: "age above duration limit", header: with(workspacesdk.CoderToolCallAgeMsHeader, ptr("9223372036855")), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
		{name: "multiple message IDs", header: twice(workspacesdk.CoderToolCallMessageIDHeader), wantErrIn: workspacesdk.CoderToolCallMessageIDHeader},
		{name: "multiple IDs", header: twice(workspacesdk.CoderToolCallIDHeader), wantErrIn: workspacesdk.CoderToolCallIDHeader},
		{name: "multiple ages", header: twice(workspacesdk.CoderToolCallAgeMsHeader), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
		{name: "missing message ID", header: with(workspacesdk.CoderToolCallMessageIDHeader, nil), wantErrIn: workspacesdk.CoderToolCallMessageIDHeader},
		{name: "missing ID", header: with(workspacesdk.CoderToolCallIDHeader, nil), wantErrIn: workspacesdk.CoderToolCallIDHeader},
		{name: "missing age", header: with(workspacesdk.CoderToolCallAgeMsHeader, nil), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
		{name: "message ID zero", header: with(workspacesdk.CoderToolCallMessageIDHeader, ptr("0")), wantErrIn: workspacesdk.CoderToolCallMessageIDHeader},
		{name: "message ID negative", header: with(workspacesdk.CoderToolCallMessageIDHeader, ptr("-1")), wantErrIn: workspacesdk.CoderToolCallMessageIDHeader},
		{name: "message ID not a number", header: with(workspacesdk.CoderToolCallMessageIDHeader, ptr("abc")), wantErrIn: workspacesdk.CoderToolCallMessageIDHeader},
		{name: "message ID empty", header: with(workspacesdk.CoderToolCallMessageIDHeader, ptr("")), wantErrIn: workspacesdk.CoderToolCallMessageIDHeader},
		{name: "ID empty", header: with(workspacesdk.CoderToolCallIDHeader, ptr("")), wantErrIn: workspacesdk.CoderToolCallIDHeader},
		{name: "ID bad escape", header: with(workspacesdk.CoderToolCallIDHeader, ptr("%zz")), wantErrIn: workspacesdk.CoderToolCallIDHeader},
		{name: "age negative", header: with(workspacesdk.CoderToolCallAgeMsHeader, ptr("-1")), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
		{name: "age fractional", header: with(workspacesdk.CoderToolCallAgeMsHeader, ptr("1.5")), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
		{name: "age empty", header: with(workspacesdk.CoderToolCallAgeMsHeader, ptr("")), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
		{name: "age overflows duration", header: with(workspacesdk.CoderToolCallAgeMsHeader, ptr("9223372036854775807")), wantErrIn: workspacesdk.CoderToolCallAgeMsHeader},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := workspacesdk.ToolCallFromHeaders(tc.header)
			if tc.wantErrIn != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrIn)
				assert.False(t, ok)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestToolCallUUID(t *testing.T) {
	t.Parallel()

	t.Run("namespace", func(t *testing.T) {
		t.Parallel()

		want := uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://coder.com/workspace-agent/tool-call"))
		require.Equal(t, want, workspacesdk.ToolCallUUIDNamespace)
	})

	t.Run("vector", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
		got := workspacesdk.ToolCallUUID(chatID, 42, "toolu_01/%x y")
		require.Equal(t, uuid.MustParse("1a9cb501-66b5-51bd-a7c6-1bf0e500b268"), got)
		require.Equal(t, uuid.Version(5), got.Version())
	})

	t.Run("each input changes the UUID", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		base := workspacesdk.ToolCallUUID(chatID, 42, "toolu_01")
		cases := []struct {
			name       string
			chatID     uuid.UUID
			messageID  int64
			toolCallID string
		}{
			{name: "chat ID", chatID: uuid.New(), messageID: 42, toolCallID: "toolu_01"},
			{name: "message ID", chatID: chatID, messageID: 43, toolCallID: "toolu_01"},
			{name: "tool call ID", chatID: chatID, messageID: 42, toolCallID: "toolu_02"},
			{name: "tool call ID escaped form", chatID: chatID, messageID: 42, toolCallID: "toolu%5F01"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.Equal(t, base, workspacesdk.ToolCallUUID(chatID, 42, "toolu_01"))
				assert.NotEqual(t, base, workspacesdk.ToolCallUUID(tc.chatID, tc.messageID, tc.toolCallID))
			})
		}
	})
}

func TestToolCallContext(t *testing.T) {
	t.Parallel()

	_, ok := workspacesdk.ToolCallFromContext(context.Background())
	require.False(t, ok)

	want := workspacesdk.ToolCall{MessageID: 5, ID: "toolu_01", Age: time.Second}
	got, ok := workspacesdk.ToolCallFromContext(workspacesdk.WithToolCall(context.Background(), want))
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestToolCallErrorMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  workspacesdk.ToolCallError
		want string
	}{
		{
			name: "message only",
			err: workspacesdk.ToolCallError{
				Response: codersdk.Response{Message: "Tool call is stale."},
				Code:     workspacesdk.ToolCallErrorStale,
			},
			want: "tool call error stale_tool_call: Tool call is stale.",
		},
		{
			name: "detail and validations",
			err: workspacesdk.ToolCallError{
				Response: codersdk.Response{
					Message:     "Input differs.",
					Detail:      "command changed",
					Validations: []codersdk.ValidationError{{Field: "command", Detail: "differs"}},
				},
				Code: workspacesdk.ToolCallErrorInputMismatch,
			},
			want: "tool call error input_mismatch: Input differs.\n\tError: command changed\n\tcommand: differs",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.err.Error())
		})
	}
}

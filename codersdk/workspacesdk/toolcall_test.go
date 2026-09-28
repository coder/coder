package workspacesdk_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestToolCallHeadersRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		toolCall    workspacesdk.ToolCall
		wantIDHdr   string
		wantNameHdr string
	}{
		{
			name:        "plain ID",
			toolCall:    workspacesdk.ToolCall{MessageID: 1, ID: "toolu_01ABC", Name: "execute"},
			wantIDHdr:   "toolu_01ABC",
			wantNameHdr: "execute",
		},
		{
			name:        "slash",
			toolCall:    workspacesdk.ToolCall{MessageID: 42, ID: "call/1", Name: "edit_files"},
			wantIDHdr:   "call%2F1",
			wantNameHdr: "edit_files",
		},
		{
			name:        "percent",
			toolCall:    workspacesdk.ToolCall{MessageID: 7, ID: "100%x", Name: "a%b"},
			wantIDHdr:   "100%25x",
			wantNameHdr: "a%25b",
		},
		{
			name:        "space",
			toolCall:    workspacesdk.ToolCall{MessageID: 7, ID: "a b", Name: "write file"},
			wantIDHdr:   "a%20b",
			wantNameHdr: "write%20file",
		},
		{
			name:        "non-ASCII",
			toolCall:    workspacesdk.ToolCall{MessageID: 9, ID: "工具-é", Name: "é"},
			wantIDHdr:   "%E5%B7%A5%E5%85%B7-%C3%A9",
			wantNameHdr: "%C3%A9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := http.Header{}
			tc.toolCall.SetHeaders(h)
			assert.Equal(t, tc.wantIDHdr, h.Get(workspacesdk.CoderToolCallIDHeader))
			assert.Equal(t, tc.wantNameHdr, h.Get(workspacesdk.CoderToolCallNameHeader))

			got, ok, err := workspacesdk.ToolCallFromHeaders(h)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.toolCall, got)
		})
	}
}

func TestToolCallFromHeaders(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		workspacesdk.CoderToolCallMessageIDHeader: "42",
		workspacesdk.CoderToolCallIDHeader:        "toolu_01",
		workspacesdk.CoderToolCallNameHeader:      "execute",
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
	const (
		msgHdr  = workspacesdk.CoderToolCallMessageIDHeader
		idHdr   = workspacesdk.CoderToolCallIDHeader
		nameHdr = workspacesdk.CoderToolCallNameHeader
	)

	cases := []struct {
		name      string
		header    http.Header
		wantOK    bool
		want      workspacesdk.ToolCall
		wantErrIn string // header name the error must mention, empty for no error
	}{
		{name: "none present", header: http.Header{"Coder-Chat-Id": {uuid.NewString()}}},
		{name: "all valid", header: validHeader(), wantOK: true, want: workspacesdk.ToolCall{MessageID: 42, ID: "toolu_01", Name: "execute"}},
		{name: "multiple message IDs", header: twice(msgHdr), wantErrIn: msgHdr},
		{name: "multiple IDs", header: twice(idHdr), wantErrIn: idHdr},
		{name: "multiple names", header: twice(nameHdr), wantErrIn: nameHdr},
		{name: "missing message ID", header: with(msgHdr, nil), wantErrIn: msgHdr},
		{name: "missing ID", header: with(idHdr, nil), wantErrIn: idHdr},
		{name: "missing name", header: with(nameHdr, nil), wantErrIn: nameHdr},
		{name: "message ID zero", header: with(msgHdr, ptr("0")), wantErrIn: msgHdr},
		{name: "message ID negative", header: with(msgHdr, ptr("-1")), wantErrIn: msgHdr},
		{name: "message ID not a number", header: with(msgHdr, ptr("abc")), wantErrIn: msgHdr},
		{name: "message ID empty", header: with(msgHdr, ptr("")), wantErrIn: msgHdr},
		{name: "ID empty", header: with(idHdr, ptr("")), wantErrIn: idHdr},
		{name: "ID bad escape", header: with(idHdr, ptr("%zz")), wantErrIn: idHdr},
		{name: "name empty", header: with(nameHdr, ptr("")), wantErrIn: nameHdr},
		{name: "name bad escape", header: with(nameHdr, ptr("%zz")), wantErrIn: nameHdr},
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

	// The vector was computed independently of this package with
	// Python's uuid and hashlib from the encoding documented on
	// ToolCallUUID. chatd and the agent must agree on it across
	// versions, so it must never change.
	t.Run("vector", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
		got := workspacesdk.ToolCallUUID(chatID, 42, "execute", "toolu_01/%x y")
		require.Equal(t, uuid.MustParse("8ef95814-a40f-5318-9b21-abfeb4d3eff2"), got)
		require.Equal(t, uuid.Version(5), got.Version())
	})

	t.Run("each input changes the UUID", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		base := workspacesdk.ToolCallUUID(chatID, 42, "execute", "toolu_01")
		cases := []struct {
			name       string
			chatID     uuid.UUID
			messageID  int64
			toolName   string
			toolCallID string
		}{
			{name: "chat ID", chatID: uuid.New(), messageID: 42, toolName: "execute", toolCallID: "toolu_01"},
			{name: "message ID", chatID: chatID, messageID: 43, toolName: "execute", toolCallID: "toolu_01"},
			{name: "tool name", chatID: chatID, messageID: 42, toolName: "edit_files", toolCallID: "toolu_01"},
			{name: "tool call ID", chatID: chatID, messageID: 42, toolName: "execute", toolCallID: "toolu_02"},
			{name: "tool call ID escaped form", chatID: chatID, messageID: 42, toolName: "execute", toolCallID: "toolu%5F01"},
			// Without the length prefix, "execute"+"toolu_01" and
			// "executet"+"oolu_01" would hash the same bytes.
			{name: "boundary between name and ID", chatID: chatID, messageID: 42, toolName: "executet", toolCallID: "oolu_01"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.Equal(t, base, workspacesdk.ToolCallUUID(chatID, 42, "execute", "toolu_01"))
				assert.NotEqual(t, base, workspacesdk.ToolCallUUID(tc.chatID, tc.messageID, tc.toolName, tc.toolCallID))
			})
		}
	})
}

func TestToolCallContext(t *testing.T) {
	t.Parallel()

	_, ok := workspacesdk.ToolCallFromContext(context.Background())
	require.False(t, ok)

	want := workspacesdk.ToolCall{MessageID: 5, ID: "toolu_01", Name: "execute"}
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
				Response: codersdk.Response{Message: "Tool call was canceled."},
				Code:     workspacesdk.ToolCallErrorCanceled,
			},
			want: "tool call error tool_call_canceled: Tool call was canceled.",
		},
		{
			name: "detail and validations",
			err: workspacesdk.ToolCallError{
				Response: codersdk.Response{
					Message:     "Unknown tool call.",
					Detail:      "message 7 is at or below the cutoff 9",
					Validations: []codersdk.ValidationError{{Field: "message_id", Detail: "at or below cutoff"}},
				},
				Code: workspacesdk.ToolCallErrorUnknown,
			},
			want: "tool call error tool_call_unknown: Unknown tool call.\n\tError: message 7 is at or below the cutoff 9\n\tmessage_id: at or below cutoff",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.err.Error())
		})
	}
}

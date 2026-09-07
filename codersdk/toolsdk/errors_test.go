package toolsdk_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/toolsdk"
)

func TestPublicError(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{nil, xerrors.New("private diagnostic")} {
		name := "WithoutCause"
		if cause != nil {
			name = "WithCause"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			public := &toolsdk.PublicError{Message: "Workspace must be started.", Cause: cause}
			wrapped := xerrors.Errorf("private wrapper: %w", public)
			found, ok := errors.AsType[*toolsdk.PublicError](wrapped)
			require.True(t, ok)
			require.Equal(t, "Workspace must be started.", found.Message)
			if cause != nil {
				require.ErrorIs(t, wrapped, cause)
				require.Equal(t, "Workspace must be started.: private diagnostic", public.Error())
			} else {
				require.Equal(t, public.Message, public.Error())
				require.Nil(t, public.Unwrap())
			}
		})
	}
}

func TestToolPublicErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tool toolsdk.GenericTool
		args string
		want string
	}{
		{name: "TemplateID", tool: toolsdk.GetTemplate.Generic(), args: `{"template_id":"bad"}`, want: "template_id must be a valid UUID"},
		{name: "WorkspaceName", tool: toolsdk.WorkspaceBash.Generic(), args: `{"workspace":"","command":"true"}`, want: "workspace name cannot be empty"},
		{name: "ChatID", tool: toolsdk.GetChat.Generic(), args: `{"chat_id":"bad"}`, want: "chat_id must be a valid UUID"},
		{name: "ChatLimit", tool: toolsdk.ListChats.Generic(), args: `{"limit":101}`, want: "limit must be between 1 and 100"},
		{name: "FetchID", tool: toolsdk.ChatGPTFetch.Generic(), args: `{"id":"bad"}`, want: "invalid ID: bad"},
		{name: "FileAddress", tool: toolsdk.DownloadChatFile.Generic(), args: `{}`, want: "provide exactly one addressing mode: file_id alone, or chat_id with file_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.tool.Handler(t.Context(), toolsdk.Deps{}, []byte(tt.args))
			require.Error(t, err)
			public, ok := errors.AsType[*toolsdk.PublicError](err)
			require.True(t, ok, "expected a public error, got %T: %v", err, err)
			require.Equal(t, tt.want, public.Message)
		})
	}
}

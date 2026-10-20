package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestCopyAttachmentsToWorkspace(t *testing.T) {
	t.Parallel()

	copied := chattool.WorkspaceAttachment{
		FileID: "file",
		Name:   "report.zip",
		Status: chattool.WorkspaceAttachmentCopied,
		Path:   "~/.coder/chats/chat/files/file/report.zip",
	}
	type result struct {
		Attachments      []chattool.WorkspaceAttachment `json:"attachments"`
		AttachmentsError string                         `json:"attachments_error"`
		Message          string                         `json:"message"`
	}
	tests := []struct {
		name   string
		report chattool.WorkspaceAttachmentsReport
		want   result
	}{
		{
			name:   "Attachments",
			report: chattool.WorkspaceAttachmentsReport{Attachments: []chattool.WorkspaceAttachment{copied}},
			want:   result{Attachments: []chattool.WorkspaceAttachment{copied}},
		},
		{
			name: "NoAttachments",
			want: result{
				Attachments: []chattool.WorkspaceAttachment{},
				Message:     "the user has not attached any files for the workspace in this chat",
			},
		},
		{
			name:   "Error",
			report: chattool.WorkspaceAttachmentsReport{Err: xerrors.New("read chat messages: connection refused")},
			want: result{
				Attachments:      []chattool.WorkspaceAttachment{},
				AttachmentsError: "read chat messages: connection refused",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := chattool.CopyAttachmentsToWorkspace(func(context.Context) chattool.WorkspaceAttachmentsReport {
				return tt.report
			})
			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-1",
				Name:  chattool.CopyAttachmentsToWorkspaceToolName,
				Input: "{}",
			})
			require.NoError(t, err)
			require.False(t, resp.IsError, "an attachment problem must not fail the tool call")

			var got result
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &got))
			require.Equal(t, tt.want, got)
		})
	}
}

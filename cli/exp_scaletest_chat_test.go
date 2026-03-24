//go:build !slim

//nolint:testpackage // Tests cover an unexported validation helper.
package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateChatWorkspaceSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workspaceID       string
		template          string
		workspaceCount    int64
		chatsPerWorkspace int64
		wantErr           string
	}{
		{
			name:              "SharedWorkspaceMode",
			workspaceID:       "11111111-1111-1111-1111-111111111111",
			chatsPerWorkspace: 8,
		},
		{
			name:              "TemplateMode",
			template:          "fake-template",
			workspaceCount:    600,
			chatsPerWorkspace: 8,
		},
		{
			name:              "MissingWorkspaceSelector",
			chatsPerWorkspace: 8,
			wantErr:           "exactly one of --workspace-id or --template is required",
		},
		{
			name:              "BothWorkspaceSelectors",
			workspaceID:       "11111111-1111-1111-1111-111111111111",
			template:          "fake-template",
			chatsPerWorkspace: 8,
			wantErr:           "--workspace-id and --template are mutually exclusive",
		},
		{
			name:        "MissingChatsPerWorkspace",
			workspaceID: "11111111-1111-1111-1111-111111111111",
			wantErr:     "--chats-per-workspace must be at least 1",
		},
		{
			name:              "TemplateModeRequiresWorkspaceCount",
			template:          "fake-template",
			chatsPerWorkspace: 8,
			wantErr:           "--workspace-count must be at least 1 when --template is set",
		},
		{
			name:              "SharedWorkspaceRejectsWorkspaceCount",
			workspaceID:       "11111111-1111-1111-1111-111111111111",
			workspaceCount:    600,
			chatsPerWorkspace: 8,
			wantErr:           "--workspace-count may only be used with --template",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateChatWorkspaceSelection(tt.workspaceID, tt.template, tt.workspaceCount, tt.chatsPerWorkspace)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

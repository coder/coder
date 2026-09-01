package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func TestAuditLogIsResourceDeleted(t *testing.T) {
	t.Parallel()

	resources := []struct {
		name         string
		resourceType database.ResourceType
		expectGet    func(*dbmock.MockStore, uuid.UUID, error)
	}{
		{
			name:         "Chat",
			resourceType: database.ResourceTypeChat,
			expectGet: func(db *dbmock.MockStore, id uuid.UUID, err error) {
				db.EXPECT().GetChatByID(gomock.Any(), id).Return(database.Chat{}, err)
			},
		},
		{
			name:         "UserMemory",
			resourceType: database.ResourceTypeUserMemory,
			expectGet: func(db *dbmock.MockStore, id uuid.UUID, err error) {
				db.EXPECT().GetUserMemoryByID(gomock.Any(), id).Return(database.UserMemory{}, err)
			},
		},
		{
			name:         "ChatMemory",
			resourceType: database.ResourceTypeChatMemory,
			expectGet: func(db *dbmock.MockStore, id uuid.UUID, err error) {
				db.EXPECT().GetChatMemoryByID(gomock.Any(), id).Return(database.ChatMemory{}, err)
			},
		},
	}
	errors := []struct {
		name        string
		err         error
		wantDeleted bool
	}{
		{name: "AnError", err: assert.AnError, wantDeleted: false},
		{name: "NotAuthorized", err: dbauthz.NotAuthorizedError{}, wantDeleted: false},
		{name: "NoError", err: nil, wantDeleted: false},
		{name: "NoRows", err: sql.ErrNoRows, wantDeleted: true},
	}

	for _, resource := range resources {
		t.Run(resource.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range errors {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					ctrl := gomock.NewController(t)
					db := dbmock.NewMockStore(ctrl)
					resourceID := uuid.New()
					resource.expectGet(db, resourceID, tc.err)

					api := &API{
						Options: &Options{Database: db, Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})},
					}

					deleted := api.auditLogIsResourceDeleted(context.Background(), database.GetAuditLogsOffsetRow{
						AuditLog: database.AuditLog{ResourceType: resource.resourceType, ResourceID: resourceID},
					})
					require.Equal(t, tc.wantDeleted, deleted)
				})
			}
		})
	}
}

func TestAuditLogDescription(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		alog database.GetAuditLogsOffsetRow
		want string
	}{
		{
			name: "mainline",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionCreate,
					StatusCode:   200,
					ResourceType: database.ResourceTypeWorkspace,
				},
			},
			want: "{user} created workspace {target}",
		},
		{
			name: "unsuccessful",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionCreate,
					StatusCode:   400,
					ResourceType: database.ResourceTypeWorkspace,
				},
			},
			want: "{user} unsuccessfully attempted to create workspace {target}",
		},
		{
			name: "login",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionLogin,
					StatusCode:   200,
					ResourceType: database.ResourceTypeApiKey,
				},
			},
			want: "{user} logged in",
		},
		{
			name: "unsuccessful_login",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionLogin,
					StatusCode:   401,
					ResourceType: database.ResourceTypeApiKey,
				},
			},
			want: "{user} unsuccessfully attempted to login",
		},
		{
			name: "gitsshkey",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionDelete,
					StatusCode:   200,
					ResourceType: database.ResourceTypeGitSshKey,
				},
			},
			want: "{user} deleted the git ssh key",
		},
		{
			name: "chat_archived",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"archived": {Old: false, New: true},
			}),
			want: "{user} archived chat {target}",
		},
		{
			name: "chat_unarchived",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"archived": {Old: true, New: false},
			}),
			want: "{user} unarchived chat {target}",
		},
		{
			name: "chat_sharing_user_acl",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"user_acl": {Old: map[string]any{}, New: map[string]any{"user-1": map[string]any{"permissions": []string{"read"}}}},
			}),
			want: "{user} updated sharing for chat {target}",
		},
		{
			name: "chat_sharing_group_acl",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"group_acl": {Old: map[string]any{}, New: map[string]any{"group-1": map[string]any{"permissions": []string{"read"}}}},
			}),
			want: "{user} updated sharing for chat {target}",
		},
		{
			name: "chat_sharing_both_acls",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"user_acl":  {Old: map[string]any{}, New: map[string]any{"user-1": map[string]any{"permissions": []string{"read"}}}},
				"group_acl": {Old: map[string]any{}, New: map[string]any{"group-1": map[string]any{"permissions": []string{"read"}}}},
			}),
			want: "{user} updated sharing for chat {target}",
		},
		{
			name: "chat_mixed_diff_falls_through",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"archived":  {Old: false, New: true},
				"pin_order": {Old: 1, New: 0},
			}),
			want: "{user} updated chat {target}",
		},
		{
			name: "chat_acl_with_extra_field_falls_through",
			alog: chatAuditLogRow(t, codersdk.AuditDiff{
				"user_acl":  {Old: map[string]any{}, New: map[string]any{}},
				"pin_order": {Old: 1, New: 0},
			}),
			want: "{user} updated chat {target}",
		},
		{
			name: "chat_failed_write_no_override",
			alog: func() database.GetAuditLogsOffsetRow {
				row := chatAuditLogRow(t, codersdk.AuditDiff{
					"archived": {Old: false, New: true},
				})
				row.AuditLog.StatusCode = 400
				return row
			}(),
			want: "{user} unsuccessfully attempted to write chat {target}",
		},
		{
			name: "chat_redirect_no_override",
			alog: func() database.GetAuditLogsOffsetRow {
				row := chatAuditLogRow(t, codersdk.AuditDiff{
					"archived": {Old: false, New: true},
				})
				row.AuditLog.StatusCode = 303
				return row
			}(),
			want: "{user} was redirected attempting to write chat {target}",
		},
		{
			name: "chat_non_write_action_no_override",
			alog: func() database.GetAuditLogsOffsetRow {
				row := chatAuditLogRow(t, codersdk.AuditDiff{
					"user_acl": {Old: map[string]any{}, New: map[string]any{"user-1": map[string]any{"permissions": []string{"read"}}}},
				})
				row.AuditLog.Action = database.AuditActionCreate
				return row
			}(),
			want: "{user} created chat {target}",
		},
	}
	// nolint: paralleltest // no longer need to reinitialize loop vars in go 1.22
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := auditLogDescription(tc.alog)
			require.Equal(t, tc.want, got)
		})
	}
}

// chatAuditLogRow builds a GetAuditLogsOffsetRow for a successful chat write
// with the given diff, suitable for testing auditLogDescription.
func chatAuditLogRow(t *testing.T, diff codersdk.AuditDiff) database.GetAuditLogsOffsetRow {
	t.Helper()
	rawDiff, err := json.Marshal(diff)
	require.NoError(t, err)
	return database.GetAuditLogsOffsetRow{
		AuditLog: database.AuditLog{
			Action:       database.AuditActionWrite,
			StatusCode:   200,
			ResourceType: database.ResourceTypeChat,
			Diff:         rawDiff,
		},
	}
}

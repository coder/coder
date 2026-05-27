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
)

func TestAuditLogIsResourceDeleted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		err         error
		wantDeleted bool
	}{
		{name: "AnError", err: assert.AnError, wantDeleted: false},
		{name: "NotAuthorized", err: dbauthz.NotAuthorizedError{}, wantDeleted: false},
		{name: "NoError", err: nil, wantDeleted: false},
		{name: "NoRows", err: sql.ErrNoRows, wantDeleted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			chatID := uuid.New()
			db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{}, tc.err)

			api := &API{
				Options: &Options{Database: db, Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})},
			}

			deleted := api.auditLogIsResourceDeleted(context.Background(), database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{ResourceType: database.ResourceTypeChat, ResourceID: chatID},
			})
			require.Equal(t, tc.wantDeleted, deleted)
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
			name: "archive_chat",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"archived":{"old":false,"new":true,"secret":false}}`),
				},
			},
			want: "{user} archived chat {target}",
		},
		{
			name: "unarchive_chat",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"archived":{"old":true,"new":false,"secret":false}}`),
				},
			},
			want: "{user} unarchived chat {target}",
		},
		{
			name: "archive_chat_with_another_change",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"archived":{"old":false,"new":true,"secret":false},"pin_order":{"old":1,"new":0,"secret":false}}`),
				},
			},
			want: "{user} updated chat {target}",
		},
		{
			name: "unsuccessful_archive_chat",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   400,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"archived":{"old":false,"new":true,"secret":false}}`),
				},
			},
			want: "{user} unsuccessfully attempted to write chat {target}",
		},
		{
			name: "redirect_archive_chat",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   303,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"archived":{"old":false,"new":true,"secret":false}}`),
				},
			},
			want: "{user} was redirected attempting to write chat {target}",
		},
		{
			name: "share_chat_with_one_user",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"user_acl":{"old":{},"new":{"user-1":{"permissions":["read"]}},"secret":false}}`),
				},
			},
			want: "{user} shared chat with 1 user {target}",
		},
		{
			name: "share_chat_with_users_and_groups",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"user_acl":{"old":{},"new":{"user-1":{"permissions":["read"]},"user-2":{"permissions":["read"]}},"secret":false},"group_acl":{"old":{},"new":{"group-1":{"permissions":["read"]}},"secret":false}}`),
				},
			},
			want: "{user} shared chat with 2 users and 1 group {target}",
		},
		{
			name: "unshare_chat_with_one_group",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"group_acl":{"old":{"group-1":{"permissions":["read"]}},"new":{},"secret":false}}`),
				},
			},
			want: "{user} unshared chat with 1 group {target}",
		},
		{
			name: "unshare_chat_with_users_and_groups",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"user_acl":{"old":{"user-1":{"permissions":["read"]},"user-2":{"permissions":["read"]}},"new":{},"secret":false},"group_acl":{"old":{"group-1":{"permissions":["read"]}},"new":{},"secret":false}}`),
				},
			},
			want: "{user} unshared chat with 2 users and 1 group {target}",
		},
		{
			name: "mixed_chat_sharing_change",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"user_acl":{"old":{"user-1":{"permissions":["read"]}},"new":{"user-2":{"permissions":["read"]}},"secret":false}}`),
				},
			},
			want: "{user} updated sharing for chat {target}",
		},
		{
			name: "reordered_chat_acl_permissions",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"user_acl":{"old":{"user-1":{"permissions":["read","update"]}},"new":{"user-1":{"permissions":["update","read"]}},"secret":false}}`),
				},
			},
			want: "{user} updated chat {target}",
		},
		{
			name: "unchanged_chat_acl",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage(`{"user_acl":{"old":{"user-1":{"permissions":["read"]}},"new":{"user-1":{"permissions":["read"]}},"secret":false}}`),
				},
			},
			want: "{user} updated chat {target}",
		},
		{
			name: "non_chat_acl_change",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeWorkspace,
					Diff:         json.RawMessage(`{"user_acl":{"old":{},"new":{"user-1":{"permissions":["read"]}},"secret":false}}`),
				},
			},
			want: "{user} updated workspace {target}",
		},
		{
			name: "malformed_chat_diff",
			alog: database.GetAuditLogsOffsetRow{
				AuditLog: database.AuditLog{
					Action:       database.AuditActionWrite,
					StatusCode:   200,
					ResourceType: database.ResourceTypeChat,
					Diff:         json.RawMessage("{"),
				},
			},
			want: "{user} updated chat {target}",
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

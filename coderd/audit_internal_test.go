package coderd

import (
	"context"
	"database/sql"
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

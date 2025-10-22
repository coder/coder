package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

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

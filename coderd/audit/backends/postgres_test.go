package backends_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/audit/backends"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
)

func TestPostgresBackend(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			db          = databasefake.New()
			pgb         = backends.NewPostgres(db, true)
			alog        = randomAuditLog()
		)
		defer cancel()

		err := pgb.Export(ctx, alog)
		require.NoError(t, err)

		got, err := db.GetAuditLogsBefore(ctx, database.GetAuditLogsBeforeParams{
			ID:        uuid.Nil,
			StartTime: time.Now().Add(time.Second),
			RowLimit:  1,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, alog, got[0])
	})
}

func randomAuditLog() database.AuditLog {
	_, inet, _ := net.ParseCIDR("127.0.0.1/32")
	return database.AuditLog{
		ID:             uuid.New(),
		Time:           time.Now(),
		UserID:         uuid.New(),
		OrganizationID: uuid.New(),
		Ip: pqtype.Inet{
			IPNet: *inet,
			Valid: true,
		},
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.127 Safari/537.36",
		ResourceType:   database.ResourceTypeOrganization,
		ResourceID:     uuid.New(),
		ResourceTarget: "colin's organization",
		Action:         database.AuditActionDelete,
		Diff:           []byte{},
		StatusCode:     http.StatusNoContent,
	}
}

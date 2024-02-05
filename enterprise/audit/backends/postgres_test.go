package backends_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/audittest"
	"github.com/coder/coder/v2/enterprise/audit/backends"
)

func TestPostgresBackend(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			db          = dbmem.New()
			pgb         = backends.NewPostgres(db, true)
			alog        = audittest.RandomLog()
		)
		defer cancel()

		err := pgb.Export(ctx, alog, audit.BackendDetails{})
		require.NoError(t, err)

		got, err := db.GetAuditLogsOffset(ctx, database.GetAuditLogsOffsetParams{
			Offset: 0,
			Limit:  1,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, alog.ID, got[0].ID)
	})
}

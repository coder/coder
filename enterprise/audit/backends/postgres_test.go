package backends_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/enterprise/audit/audittest"
	"github.com/coder/coder/enterprise/audit/backends"
)

func TestPostgresBackend(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			db          = databasefake.New()
			pgb         = backends.NewPostgres(db, true)
			alog        = audittest.RandomLog()
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

package searchquery_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/searchquery"
	"github.com/coder/coder/v2/testutil"
)

func TestAuditLogsOnBehalfOfFilter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})

	filter, countFilter, errs := searchquery.AuditLogs(ctx, db, "on_behalf_of:"+owner.Username)
	require.Empty(t, errs)
	require.Equal(t, owner.Username, filter.OnBehalfOfUsername)
	require.Equal(t, owner.Username, countFilter.OnBehalfOfUsername)

	filter, countFilter, errs = searchquery.AuditLogs(ctx, db, "on_behalf_of:"+owner.ID.String())
	require.Empty(t, errs)
	require.Equal(t, owner.ID, filter.OnBehalfOfUserID)
	require.Equal(t, owner.ID, countFilter.OnBehalfOfUserID)
}

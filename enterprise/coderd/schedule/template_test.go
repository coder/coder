package schedule

import (
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
)

func TestTemplateUpdateBuildDeadlines(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)

	var (
		org = dbgen.Organization(t, db, database.Organization{})
		template = dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,

	)
}
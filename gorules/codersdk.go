package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

// databaseImport enforces not importing any database types into /codersdk.
//nolint:unused,deadcode,varnamelen
func databaseImport(m dsl.Matcher) {
	m.Import("github.com/coder/coder/coderd/database")
	m.Match("database.$_").
		Report("Do not import any database types into codersdk").
		Where(m.File().PkgPath.Matches("github.com/coder/coder/codersdk"))
}

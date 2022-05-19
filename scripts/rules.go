// Package gorules defines custom lint rules for ruleguard.
//
// golangci-lint runs these rules via go-critic, which includes support
// for ruleguard. All Go files in this directory define lint rules
// in the Ruleguard DSL; see:
//
// - https://go-ruleguard.github.io/by-example/
// - https://pkg.go.dev/github.com/quasilyte/go-ruleguard/dsl
//
// You run one of the following commands to execute your go rules only:
//   golangci-lint run
//   golangci-lint run --disable-all --enable=gocritic
package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

// Use xerrors everywhere! It provides additional stacktrace info!
//nolint:unused,deadcode,varnamelen
func xerrors(m dsl.Matcher) {
	m.Import("errors")
	m.Import("fmt")
	m.Import("golang.org/x/xerrors")

	m.Match("fmt.Errorf($*args)").
		Suggest("xerrors.New($args)").
		Report("Use xerrors to provide additional stacktrace information!")

	m.Match("errors.$_($msg)").
		Where(m["msg"].Type.Is("string")).
		Suggest("xerrors.New($msg)").
		Report("Use xerrors to provide additional stacktrace information!")
}

// databaseImport enforces not importing any database types into /codersdk.
//nolint:unused,deadcode,varnamelen
func databaseImport(m dsl.Matcher) {
	m.Import("github.com/coder/coder/coderd/database")
	m.Match("database.$_").
		Report("Do not import any database types into codersdk").
		Where(m.File().PkgPath.Matches("github.com/coder/coder/codersdk"))
}

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
// Note: don't forget to run `golangci-lint cache clean`!
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

// doNotCallTFailNowInsideGoroutine enforces not calling t.FailNow or
// functions that may themselves call t.FailNow in goroutines outside
// the main test goroutine. See testing.go:834 for why.
//nolint:unused,deadcode,varnamelen
func doNotCallTFailNowInsideGoroutine(m dsl.Matcher) {
	m.Import("testing")
	m.Match(`
	go func($*_){
		$*_
		$require.$_($*_)
		$*_
	}($*_)`).
		At(m["require"]).
		Where(m["require"].Text == "require").
		Report("Do not call functions that may call t.FailNow in a goroutine, as this can cause data races (see testing.go:834)")

	m.Match(`
	go func($*_){
		$*_
		$t.$fail($*_)
		$*_
	}($*_)`).
		At(m["fail"]).
		Where(m["t"].Type.Implements("testing.TB") && m["fail"].Text.Matches("^(FailNow|Fatal|Fatalf)$")).
		Report("Do not call functions that may call t.FailNow in a goroutine, as this can cause data races (see testing.go:834)")
}

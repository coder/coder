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

	// require.Eventually runs the function in a goroutine.
	m.Match(`
	require.Eventually(t, func() bool {
		$*_
		$require.$_($*_)
		$*_
	}, $*_)`).
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

// useStandardTimeoutsAndDelaysInTests ensures all tests use common
// constants for timeouts and delays in usual scenarios, this allows us
// to tweak them based on platform (important to avoid CI flakes).
//nolint:unused,deadcode,varnamelen
func useStandardTimeoutsAndDelaysInTests(m dsl.Matcher) {
	m.Import("github.com/stretchr/testify/require")
	m.Import("github.com/stretchr/testify/assert")
	m.Import("github.com/coder/coder/testutil")

	m.Match(`context.WithTimeout($ctx, $duration)`).
		Where(m.File().Imports("testing") && !m["duration"].Text.Matches("^testutil\\.")).
		At(m["duration"]).
		Report("Do not use magic numbers in test timeouts and delays. Use the standard testutil.Wait* or testutil.Interval* constants instead.")

	m.Match(`
		$testify.$Eventually($t, func() bool {
			$*_
		}, $timeout, $interval, $*_)
	`).
		Where((m["testify"].Text == "require" || m["testify"].Text == "assert") &&
			(m["Eventually"].Text == "Eventually" || m["Eventually"].Text == "Eventuallyf") &&
			!m["timeout"].Text.Matches("^testutil\\.")).
		At(m["timeout"]).
		Report("Do not use magic numbers in test timeouts and delays. Use the standard testutil.Wait* or testutil.Interval* constants instead.")

	m.Match(`
		$testify.$Eventually($t, func() bool {
			$*_
		}, $timeout, $interval, $*_)
	`).
		Where((m["testify"].Text == "require" || m["testify"].Text == "assert") &&
			(m["Eventually"].Text == "Eventually" || m["Eventually"].Text == "Eventuallyf") &&
			!m["interval"].Text.Matches("^testutil\\.")).
		At(m["interval"]).
		Report("Do not use magic numbers in test timeouts and delays. Use the standard testutil.Wait* or testutil.Interval* constants instead.")
}

// InTx checks to ensure the database used inside the transaction closure is the transaction
// database, and not the original database that creates the tx.
func InTx(m dsl.Matcher) {
	// ':=' and '=' are 2 different matches :(
	m.Match(`
	$x.InTx(func($y) error {
		$*_
		$*_ = $x.$f($*_)
		$*_
	})
	`, `
	$x.InTx(func($y) error {
		$*_
		$*_ := $x.$f($*_)
		$*_
	})
	`).Where(m["x"].Text != m["y"].Text).
		At(m["f"]).
		Report("Do not use the database directly within the InTx closure. Use '$y' instead of '$x'.")

	// When using a tx closure, ensure that if you pass the db to another
	// function inside the closure, it is the tx.
	// This will miss more complex cases such as passing the db as apart
	// of another struct.
	m.Match(`
	$x.InTx(func($y database.Store) error {
		$*_
		$*_ = $f($*_, $x, $*_)
		$*_
	})
	`, `
	$x.InTx(func($y database.Store) error {
		$*_
		$*_ := $f($*_, $x, $*_)
		$*_
	})
	`, `
	$x.InTx(func($y database.Store) error {
		$*_
		$f($*_, $x, $*_)
		$*_
	})
	`).Where(m["x"].Text != m["y"].Text).
		At(m["f"]).Report("Pass the tx database into the '$f' function inside the closure. Use '$y' over $x'")
}

// HttpAPIErrorMessage intends to enforce constructing proper sentences as
// error messages for the api. A proper sentence includes proper capitalization
// and ends with punctuation.
// There are ways around the linter, but this should work in the common cases.
func HttpAPIErrorMessage(m dsl.Matcher) {
	m.Import("github.com/coder/coder/coderd/httpapi")

	isNotProperError := func(v dsl.Var) bool {
		return v.Type.Is("string") &&
			// Either starts with a lowercase, or ends without punctuation.
			// The reason I don't check for NOT ^[A-Z].*[.!?]$ is because there
			// are some exceptions. Any string starting with a formatting
			// directive (%s) for example is exempt.
			(m["m"].Text.Matches(`^"[a-z].*`) ||
				m["m"].Text.Matches(`.*[^.!?]"$`))
	}

	m.Match(`
	httpapi.Write($_, $s, httpapi.Response{
		$*_,
		Message: $m,
		$*_,
	})
	`, `
	httpapi.Write($_, $s, httpapi.Response{
		$*_,
		Message: fmt.$f($m, $*_),
		$*_,
	})
	`,
	).Where(isNotProperError(m["m"])).
		At(m["m"]).
		Report("Field \"Message\" should be a proper sentence with a capitalized first letter and ending in punctuation. $m")
}

// HttpAPIReturn will report a linter violation if the http function is not
// returned after writing a response to the client.
func HttpAPIReturn(m dsl.Matcher) {
	m.Import("github.com/coder/coder/coderd/httpapi")

	// Manually enumerate the httpapi function rather then a 'Where' condition
	// as this is a bit more efficient.
	m.Match(`
	if $*_ {
		httpapi.Write($*a)
	}
	`, `
	if $*_ {
		httpapi.Forbidden($*a)
	}
	`, `
	if $*_ {
		httpapi.ResourceNotFound($*a)
	}
	`).At(m["a"]).
		Report("Forgot to return early after writing to the http response writer.")
}

// ProperRBACReturn ensures we always write to the response writer after a
// call to Authorize. If we just do a return, the client will get a status code
// 200, which is incorrect.
func ProperRBACReturn(m dsl.Matcher) {
	m.Match(`
	if !$_.Authorize($*_) {
		return
	}
	`).Report("Must write to 'ResponseWriter' before returning'")
}

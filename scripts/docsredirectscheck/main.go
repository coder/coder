// Command docsredirectscheck validates docs/redirects.json, the list of
// redirects the documentation website applies for pages that have moved or
// been removed.
//
// Each rule is {"source", "destination", "permanent"?}. Sources and internal
// destinations are full site paths starting with /docs/, written in the small
// subset of Next.js path syntax the website accepts: an exact path, a trailing
// "/:name*", or a trailing "/:name(.*)". A destination may also be an
// http(s) URL.
//
// It fails when:
//
//   - the file is not a JSON array of well-formed rules;
//   - a source or destination is outside the supported syntax;
//   - an internal destination is not a page in docs/manifest.json;
//   - a source is a live docs page, which would hide a page readers can reach;
//   - two rules share a source, or rules form a chain or a loop.
//
// It also warns, without failing, when a pull request removes routes from
// docs/manifest.json and leaves docs/redirects.json alone, since pages are
// sometimes removed on purpose.
//
// A missing docs/redirects.json passes. Everything it checks about routes is
// derived from docs/manifest.json the same way the website derives it.
//
// Usage:
//
//	docsredirectscheck [-manifest path] [-redirects path] [-base rev]
//
// Exit status is 0 when the file is valid or absent, 1 when it has problems,
// and 2 when the check itself could not run.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"
)

type options struct {
	manifestPath  string
	redirectsPath string
	// base is the git revision the change under review started from. Empty
	// means derive it from the environment.
	base   string
	git    gitReader
	getenv func(string) string
	stdout io.Writer
	stderr io.Writer
}

// gitReader is the slice of git the advisory check needs, so tests can supply
// a fake repository.
type gitReader interface {
	// HasRev reports whether rev resolves to a commit.
	HasRev(rev string) bool
	// Show returns the contents of path at rev.
	Show(rev, path string) ([]byte, error)
	// Changed reports whether path differs between rev and the working tree.
	Changed(rev, path string) (bool, error)
}

type execGit struct{}

// validRev rejects a revision git could read as an option, so a value passed
// with -base cannot add flags to the git commands below. The commands run
// without a shell and path is always one of the fixed repository paths.
func validRev(rev string) bool {
	return rev != "" && !strings.HasPrefix(rev, "-")
}

func (execGit) HasRev(rev string) bool {
	if !validRev(rev) {
		return false
	}
	return exec.Command("git", "rev-parse", "--verify", "--quiet", rev+"^{commit}").Run() == nil //nolint:gosec // rev is checked by validRev and no shell is involved.
}

func (execGit) Show(rev, path string) ([]byte, error) {
	if !validRev(rev) {
		return nil, xerrors.Errorf("invalid revision %q", rev)
	}
	return exec.Command("git", "show", rev+":"+path).Output() //nolint:gosec // rev is checked by validRev and no shell is involved.
}

func (execGit) Changed(rev, path string) (bool, error) {
	if !validRev(rev) {
		return false, xerrors.Errorf("invalid revision %q", rev)
	}
	err := exec.Command("git", "diff", "--quiet", rev, "--", path).Run() //nolint:gosec // rev is checked by validRev and no shell is involved.
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

func main() {
	o := options{
		git:    execGit{},
		getenv: os.Getenv,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	flag.StringVar(&o.manifestPath, "manifest", manifestRepoPath, "path to the docs manifest")
	flag.StringVar(&o.redirectsPath, "redirects", redirectsRepoPath, "path to the redirects file")
	flag.StringVar(&o.base, "base", "", "git revision the change started from, for the removed-route warning (default: the pull request base in GitHub Actions)")
	flag.Parse()
	os.Exit(run(o))
}

func run(o options) int {
	manifestData, err := os.ReadFile(o.manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(o.stderr, "cannot read manifest %s: %v\n", o.manifestPath, err)
		return 2
	}
	m, err := parseManifest(manifestData)
	if err != nil {
		_, _ = fmt.Fprintf(o.stderr, "invalid manifest %s: %v\n", o.manifestPath, err)
		return 2
	}
	live := liveRoutes(m)

	var (
		rules []rule
		code  int
	)
	redirectsData, err := os.ReadFile(o.redirectsPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		_, _ = fmt.Fprintf(o.stdout, "%s not found; nothing to validate\n", o.redirectsPath)
	case err != nil:
		_, _ = fmt.Fprintf(o.stderr, "cannot read %s: %v\n", o.redirectsPath, err)
		return 2
	default:
		var problems []problem
		rules, problems = checkRedirects(redirectsData, live)
		if len(problems) > 0 {
			for _, p := range problems {
				_, _ = fmt.Fprintln(o.stderr, formatProblem(o.redirectsPath, p))
			}
			_, _ = fmt.Fprintf(o.stderr, "%s: %d %s found\n", o.redirectsPath, len(problems), plural(len(problems), "problem", "problems"))
			code = 1
		} else {
			_, _ = fmt.Fprintf(o.stdout, "%s: %d %s OK\n", o.redirectsPath, len(rules), plural(len(rules), "rule", "rules"))
		}
	}

	runAdvisory(o, live, rules)
	return code
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

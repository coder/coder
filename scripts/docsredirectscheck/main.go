// Command docsredirectscheck validates docs/redirects.json, the list of
// redirects the documentation websites apply for pages that have moved or been
// removed.
//
// Each rule is {"source", "destination", "permanent"?}. Sources and internal
// destinations are routes relative to the docs root, such as
// "admin/old-page". A source may end in "/*", which matches the prefix and
// everything beneath it, and a destination may then end in "/*" to carry the
// match across. The destination "/" is the docs root, and an external
// destination is an https URL under github.com/coder/ or coder.com/.
//
// It fails when:
//
//   - the file is not a JSON array of well-formed rules, or uses the old
//     absolute "/docs/..." form;
//   - a source or destination is outside the supported syntax;
//   - an internal destination is not a page in docs/manifest.json;
//   - a source is a live docs page, which would hide a page readers can reach;
//   - two rules share a source, a rule can never apply, or rules form a chain
//     or a loop;
//   - a route that worked on the base branch or on the live site no longer
//     resolves, as a page or through a rule.
//
// The removed-route check compares against three sources: the base branch's
// docs/manifest.json, the live page list for the target branch, and the base
// branch's redirect sources. The live page list is read over the network, and
// failing to read it is an infrastructure failure, not a pass. Pass -offline
// to skip it locally.
//
// A missing docs/redirects.json passes. Everything it checks about routes is
// derived from docs/manifest.json the same way the websites derive it.
//
// Usage:
//
//	docsredirectscheck [-manifest path] [-redirects path] [-base rev] [-branch name] [-offline]
//
// Exit status is 0 when the file is valid or absent, 1 when it has problems,
// and 2 when the check itself could not run.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// Repository-relative paths compared across revisions. They are fixed,
// independent of the -manifest and -redirects flags, because git addresses
// files by their place in the repository.
const (
	manifestRepoPath  = "docs/manifest.json"
	redirectsRepoPath = "docs/redirects.json"
)

type options struct {
	manifestPath  string
	redirectsPath string
	// base is the git revision the change under review started from. Empty
	// means derive it from the environment.
	base string
	// branch is the branch the change targets, which selects the live page
	// list. Empty means derive it from the environment.
	branch string
	// offline skips the live page list.
	offline bool
	git     gitReader
	fetch   fetcher
	// sleep waits between attempts to read the live page list.
	sleep  func(time.Duration)
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

func main() {
	o := options{
		git:    execGit{},
		fetch:  newHTTPFetcher(),
		sleep:  time.Sleep,
		getenv: os.Getenv,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	flag.StringVar(&o.manifestPath, "manifest", manifestRepoPath, "path to the docs manifest")
	flag.StringVar(&o.redirectsPath, "redirects", redirectsRepoPath, "path to the redirects file")
	flag.StringVar(&o.base, "base", "", "git revision the change started from, for the removed-route check (default: the pull request base in GitHub Actions)")
	flag.StringVar(&o.branch, "branch", "", "branch the change targets, which selects the live page list (default: the pull request base or the pushed branch in GitHub Actions)")
	flag.BoolVar(&o.offline, "offline", false, "skip the live page list, which needs network access")
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

	switch checkRemovedRoutes(o, live, rules) {
	case 2:
		return 2
	case 1:
		code = 1
	}
	return code
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// annotation formats a GitHub Actions workflow command. The message must stay
// on one line, so the characters the runner treats as delimiters are escaped.
func annotation(level, file, title, message string) string {
	escapeData := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	escapeProperty := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return fmt.Sprintf("::%s file=%s,title=%s::%s", level, escapeProperty.Replace(file), escapeProperty.Replace(title), escapeData.Replace(message))
}

// targetBranch returns the branch the change targets: -branch, the pull
// request base, or the pushed branch.
func targetBranch(o options) string {
	for _, b := range []string{o.branch, o.getenv("GITHUB_BASE_REF"), o.getenv("GITHUB_REF_NAME")} {
		if b != "" {
			return b
		}
	}
	return ""
}

// checkRemovedRoutes fails when a route that worked before the change no
// longer resolves. Routes that must keep working are the base branch's
// manifest routes, the live page list of the target branch, and the base
// branch's redirect sources. Each part that cannot be gathered is skipped with
// a note, except a live page list that cannot be read, which is an
// infrastructure failure (exit status 2) and never a pass.
func checkRemovedRoutes(o options, live routeSet, rules []rule) int {
	note := func(format string, args ...any) {
		_, _ = fmt.Fprintf(o.stdout, "removed-route check: "+format+"\n", args...)
	}
	inv := inventory{}
	var baseLive routeSet

	base := o.base
	if base == "" && o.getenv("GITHUB_BASE_REF") != "" {
		// On a pull request, actions/checkout checks out the merge commit, whose
		// first parent is exactly the base the change merges into.
		base = "HEAD^1"
	}
	switch {
	case base == "":
		note("base branch not compared: not a pull request run (pass -base <rev> to enable)")
	case !o.git.HasRev(base):
		note("base branch not compared: revision %q is not available (shallow checkout?)", base)
	default:
		baseLive = addBaseRoutes(o, base, inv, note)
	}

	switch {
	case o.offline:
		note("live page list skipped (-offline)")
	default:
		branch := targetBranch(o)
		segment, ok := versionSegment(branch)
		if !ok {
			if branch == "" {
				note("live page list skipped: cannot tell the target branch (pass -branch main or -branch release/X.Y)")
			} else {
				note("live page list skipped: branch %q has no published docs version", branch)
			}
			break
		}
		url := fmt.Sprintf(liveListURLFormat, segment)
		routes, err := fetchLiveRoutes(context.Background(), o.fetch, url, segment, o.sleep)
		if err != nil {
			_, _ = fmt.Fprintf(o.stderr, "cannot read the live page list, so the removed-route check could not run: %v\n", err)
			_, _ = fmt.Fprintln(o.stderr, "Re-run the job. Use -offline to skip the live page list when running locally.")
			return 2
		}
		for _, route := range routes {
			inv.add(route, "the live page list "+url)
		}
	}

	missing := missingRoutes(inv, newResolver(live, baseLive, rules))
	if len(missing) == 0 {
		if len(inv) > 0 {
			note("%d %s", len(inv), plural(len(inv), "route still resolves", "routes still resolve"))
		}
		return 0
	}

	actions := o.getenv("GITHUB_ACTIONS") == "true"
	redirectsFile := filepath.ToSlash(redirectsRepoPath)
	for _, m := range missing {
		msg := m.message()
		_, _ = fmt.Fprintln(o.stderr, msg)
		if actions {
			_, _ = fmt.Fprintln(o.stdout, annotation("error", redirectsFile, "Docs route no longer resolves", msg))
		}
	}
	_, _ = fmt.Fprintf(o.stderr, "%d docs %s. To keep the old URLs working, paste these rules at the top of %s:\n", len(missing), plural(len(missing), "route no longer resolves", "routes no longer resolve"), redirectsFile)
	for _, m := range missing {
		_, _ = fmt.Fprintf(o.stderr, "  %s,\n", m.rule())
	}
	_, _ = fmt.Fprintln(o.stderr, `Point a rule at "/" or another page when its page was removed on purpose.`)
	return 1
}

// addBaseRoutes adds the base branch's manifest routes and redirect sources to
// the inventory. It returns the base branch's live routes, or nil when the
// base manifest could not be read.
func addBaseRoutes(o options, base string, inv inventory, note func(string, ...any)) routeSet {
	manifestData, err := o.git.Show(base, manifestRepoPath)
	if err != nil {
		note("base manifest not compared: cannot read %s at %s: %v", manifestRepoPath, base, err)
		return nil
	}
	baseManifest, err := parseManifest(manifestData)
	if err != nil {
		note("base manifest not compared: %s at %s is not a valid manifest: %v", manifestRepoPath, base, err)
		return nil
	}
	baseLive := liveRoutes(baseManifest)
	for route, file := range baseLive {
		origin := manifestRepoPath + " on the base branch"
		if file != "" {
			origin = file + " on the base branch"
		}
		inv.add(route, origin)
	}
	// A missing redirects file at the base means no rules yet.
	if redirectsData, err := o.git.Show(base, redirectsRepoPath); err == nil {
		for _, route := range baseRuleSources(redirectsData) {
			inv.add(route, "a rule in "+redirectsRepoPath+" on the base branch")
		}
	}
	return baseLive
}

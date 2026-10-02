package main

import (
	"fmt"
	"slices"
	"strings"
)

// Repository-relative paths the advisory check compares across revisions. They
// are fixed, independent of the -manifest and -redirects flags, because git
// addresses files by their place in the repository.
const (
	manifestRepoPath  = "docs/manifest.json"
	redirectsRepoPath = "docs/redirects.json"
)

// maxListedRoutes bounds how many removed routes one warning spells out.
const maxListedRoutes = 10

// removedRoutes returns the routes live in base that are no longer live in
// head, sorted. It compares routes, not files, so a rename that keeps the
// public URL (for example admin/foo/index.md to admin/foo.md) is not a
// removal.
func removedRoutes(base, head map[string]bool) []string {
	var removed []string
	for route := range base {
		if !head[route] {
			removed = append(removed, route)
		}
	}
	slices.Sort(removed)
	return removed
}

// uncoveredRoutes returns the removed routes that no rule's source matches,
// preserving order.
func uncoveredRoutes(removed []string, rules []rule) []string {
	var uncovered []string
	for _, route := range removed {
		covered := false
		for _, r := range rules {
			if r.matches(route) {
				covered = true
				break
			}
		}
		if !covered {
			uncovered = append(uncovered, route)
		}
	}
	return uncovered
}

func advisoryMessage(uncovered []string) string {
	listed := uncovered
	more := ""
	if len(listed) > maxListedRoutes {
		more = fmt.Sprintf(" and %d more", len(listed)-maxListedRoutes)
		listed = listed[:maxListedRoutes]
	}
	noun := "routes"
	if len(uncovered) == 1 {
		noun = "route"
	}
	return fmt.Sprintf(
		"This change removes %d docs %s from %s without a matching redirect in %s: %s%s. "+
			"If readers or other sites may still link to a removed page, add a redirect. "+
			"If the removal is intentional, this warning can be ignored.",
		len(uncovered), noun, manifestRepoPath, redirectsRepoPath, strings.Join(listed, ", "), more,
	)
}

// annotation formats a GitHub Actions workflow command. The message must stay
// on one line, so the characters the runner treats as delimiters are escaped.
func annotation(level, file, title, message string) string {
	escapeData := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	escapeProperty := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return fmt.Sprintf("::%s file=%s,title=%s::%s", level, escapeProperty.Replace(file), escapeProperty.Replace(title), escapeData.Replace(message))
}

// runAdvisory warns, without ever failing the run, when the change under
// review removes docs routes and leaves docs/redirects.json alone. Pages are
// sometimes removed on purpose, which is why this is advice and not a rule.
// Every step that cannot be completed, for example in a shallow checkout,
// skips the check with a note rather than guessing.
func runAdvisory(o options, headLive map[string]bool, rules []rule) {
	skip := func(why string) {
		_, _ = fmt.Fprintf(o.stdout, "removed-route advisory skipped: %s\n", why)
	}

	base := o.base
	if base == "" && o.getenv("GITHUB_BASE_REF") != "" {
		// On a pull request, actions/checkout checks out the merge commit, whose
		// first parent is exactly the base the change merges into.
		base = "HEAD^1"
	}
	if base == "" {
		skip("not a pull request run (pass -base <rev> to enable)")
		return
	}
	if !o.git.HasRev(base) {
		skip(fmt.Sprintf("base revision %q is not available (shallow checkout?)", base))
		return
	}

	baseData, err := o.git.Show(base, manifestRepoPath)
	if err != nil {
		skip(fmt.Sprintf("cannot read %s at %s: %v", manifestRepoPath, base, err))
		return
	}
	baseManifest, err := parseManifest(baseData)
	if err != nil {
		skip(fmt.Sprintf("%s at %s is not a valid manifest: %v", manifestRepoPath, base, err))
		return
	}

	removed := removedRoutes(liveRoutes(baseManifest), headLive)
	if len(removed) == 0 {
		return
	}

	changed, err := o.git.Changed(base, redirectsRepoPath)
	if err != nil {
		skip(fmt.Sprintf("cannot compare %s with %s: %v", redirectsRepoPath, base, err))
		return
	}
	if changed {
		return
	}

	uncovered := uncoveredRoutes(removed, rules)
	if len(uncovered) == 0 {
		return
	}

	msg := advisoryMessage(uncovered)
	if o.getenv("GITHUB_ACTIONS") == "true" {
		_, _ = fmt.Fprintln(o.stdout, annotation("warning", manifestRepoPath, "Docs routes removed without a redirect", msg))
		return
	}
	_, _ = fmt.Fprintf(o.stdout, "warning: %s\n", msg)
}

// Command docsgenfiltercheck fails when the docs-gen paths filter in the CI
// workflow drifts from the files make gen writes.
//
// CI skips the gen job for pull requests whose changed files all match the
// docs filter and none match the docs-gen filter. That is only safe while
// docs-gen matches every generated file that docs matches. The generated files
// are the arguments plus every tracked Markdown file the docs filter matches
// whose body opens with docgenenv.GeneratedContentBanner, which covers pages
// GEN_FILES doesn't list, such as the nested CLI and REST API reference pages.
// The check fails when a generated file matches docs but not docs-gen, and
// when a docs-gen pattern matches no tracked file, so stale entries don't
// accumulate.
//
// Usage:
//
//	docsgenfiltercheck [-workflow path] generated-file ...
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// bodyOpensWithContentBanner reports whether the first non-blank line after
// the front matter of a Markdown page is docgenenv.GeneratedContentBanner,
// where the doc generators write it. Pages that only quote the banner
// elsewhere, such as in a code block, don't count.
func bodyOpensWithContentBanner(page []byte) bool {
	lines := strings.Split(string(page), "\n")
	i := 0
	if len(lines) > 0 && lines[0] == "---" {
		end := slices.Index(lines[1:], "---")
		if end < 0 {
			return false
		}
		i = end + 2
	}
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i] == docgenenv.GeneratedContentBanner
		}
	}
	return false
}

func main() {
	workflowPath := flag.String("workflow", ".github/workflows/ci.yaml", "Path to the workflow that defines the changes filter.")
	flag.Parse()

	if err := run(*workflowPath, flag.Args()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "docsgenfiltercheck: %v\n", err)
		os.Exit(1)
	}
}

func run(workflowPath string, generated []string) error {
	if len(generated) == 0 {
		return xerrors.New("no generated files given")
	}
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		return xerrors.Errorf("read workflow: %w", err)
	}
	filters, err := parseFilters(workflow)
	if err != nil {
		return xerrors.Errorf("parse %s: %w", workflowPath, err)
	}
	lsFiles := exec.Command("git", "ls-files", "-z")
	// Surface git's own message, such as the dubious-ownership hint.
	lsFiles.Stderr = os.Stderr
	out, err := lsFiles.Output()
	if err != nil {
		return xerrors.Errorf("git ls-files: %w", err)
	}
	tracked := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")

	problems, err := check(filters, generated, tracked, readIfExists)
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		for _, p := range problems {
			_, _ = fmt.Fprintf(os.Stderr, "%s: %s\n", workflowPath, p)
		}
		return xerrors.Errorf("%d problem(s) with the docs-gen filter", len(problems))
	}
	return nil
}

// readIfExists reads path, returning nil for a tracked file deleted from the
// working tree.
func readIfExists(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// parseFilters returns the dorny/paths-filter filters of the step with id
// "filter" in the "changes" job.
func parseFilters(workflow []byte) (map[string][]string, error) {
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string         `yaml:"id"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(workflow, &wf); err != nil {
		return nil, xerrors.Errorf("unmarshal workflow: %w", err)
	}
	var raw string
	for _, step := range wf.Jobs["changes"].Steps {
		if step.ID != "filter" {
			continue
		}
		s, ok := step.With["filters"].(string)
		if !ok {
			return nil, xerrors.New(`changes job step "filter" has no string "filters" input`)
		}
		raw = s
	}
	if raw == "" {
		return nil, xerrors.New(`changes job has no step with id "filter"`)
	}

	var entries map[string][]any
	if err := yaml.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, xerrors.Errorf("unmarshal filters: %w", err)
	}
	filters := make(map[string][]string, len(entries))
	for name, rules := range entries {
		for _, rule := range rules {
			// Status-qualified rules such as "added|modified: glob" decode
			// as maps. fmt.Sprint renders them as "map[added|modified:glob]",
			// and globRegexp rejects that text because of the brackets, so a
			// status-qualified rule in docs or docs-gen fails the lint.
			filters[name] = append(filters[name], fmt.Sprint(rule))
		}
	}
	return filters, nil
}

// check returns the problems with the docs-gen filter. Tracked docs pages
// whose body opens with the generated-content banner count as generated
// alongside the given files.
func check(filters map[string][]string, generated, tracked []string, read func(string) ([]byte, error)) ([]string, error) {
	docs, err := compileFilter(filters, "docs")
	if err != nil {
		return nil, err
	}
	docsGen, err := compileFilter(filters, "docs-gen")
	if err != nil {
		return nil, err
	}

	for _, f := range tracked {
		if !strings.HasSuffix(f, ".md") || !matchesAny(docs, f) || slices.Contains(generated, f) {
			continue
		}
		b, err := read(f)
		if err != nil {
			return nil, xerrors.Errorf("read %s: %w", f, err)
		}
		if bodyOpensWithContentBanner(b) {
			generated = append(generated, f)
		}
	}

	var problems []string
	for _, f := range generated {
		if matchesAny(docs, f) && !matchesAny(docsGen, f) {
			problems = append(problems, fmt.Sprintf("make gen writes %s, which the docs filter matches; add a docs-gen pattern for it, or docs-only PRs that edit it skip gen", f))
		}
	}
	for i, re := range docsGen {
		if !slices.ContainsFunc(tracked, re.MatchString) {
			problems = append(problems, fmt.Sprintf("docs-gen pattern %q matches no tracked file; remove it or fix the path", filters["docs-gen"][i]))
		}
	}
	return problems, nil
}

func compileFilter(filters map[string][]string, name string) ([]*regexp.Regexp, error) {
	patterns, ok := filters[name]
	if !ok || len(patterns) == 0 {
		return nil, xerrors.Errorf("filter %q is missing or empty", name)
	}
	res := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := globRegexp(p)
		if err != nil {
			return nil, xerrors.Errorf("filter %q: %w", name, err)
		}
		res = append(res, re)
	}
	return res, nil
}

func matchesAny(res []*regexp.Regexp, path string) bool {
	return slices.ContainsFunc(res, func(re *regexp.Regexp) bool { return re.MatchString(path) })
}

// globRegexp converts the glob forms the docs and docs-gen filters use: a
// literal path, "*" within one path segment, and a literal directory followed
// by "/**", which matches the directory and everything under it, as picomatch
// does. Any other syntax is rejected, so a pattern this check might read
// differently from dorny/paths-filter fails the lint instead of matching
// wrongly.
func globRegexp(glob string) (*regexp.Regexp, error) {
	base, recursive := strings.CutSuffix(glob, "/**")
	unsupported := base == "" || strings.HasPrefix(base, "!") || strings.Contains(base, "**") || strings.ContainsAny(base, `?[]{}()+@\`)
	// picomatch does not match "a/b" against "a/*/**", but the optional
	// suffix below would, so keep the recursive form to literal directories.
	if recursive && strings.Contains(base, "*") {
		unsupported = true
	}
	if unsupported {
		return nil, xerrors.Errorf("unsupported pattern %q: use a literal path, * within one segment, or a literal directory followed by /**", glob)
	}
	expr := strings.ReplaceAll(regexp.QuoteMeta(base), `\*`, "[^/]*")
	if recursive {
		expr += "(?:/.*)?"
	}
	return regexp.Compile("^" + expr + "$")
}

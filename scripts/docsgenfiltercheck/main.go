// Command docsgenfiltercheck fails when the docs-gen paths filter in the CI
// workflow drifts from the files make gen writes.
//
// CI skips the gen job for pull requests whose changed files all match the
// docs filter and none match the docs-gen filter. That is only safe while
// docs-gen matches every generated file that docs matches. The check fails
// when a generated file matches docs but not docs-gen, and when a docs-gen
// pattern matches no tracked file, so stale entries don't accumulate.
//
// Usage:
//
//	docsgenfiltercheck [-workflow path] generated-file ...
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

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
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		return xerrors.Errorf("git ls-files: %w", err)
	}
	tracked := strings.Split(strings.TrimSpace(string(out)), "\n")

	problems, err := check(filters, generated, tracked)
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
			// Status-qualified rules such as "added|modified: glob" are
			// maps. Keep only plain globs; check rejects the filters it
			// needs if they contain anything else.
			if s, ok := rule.(string); ok {
				filters[name] = append(filters[name], s)
			} else {
				filters[name] = append(filters[name], "")
			}
		}
	}
	return filters, nil
}

// check returns one problem per generated file that matches the docs filter
// but not docs-gen, and per docs-gen pattern that matches no tracked file.
func check(filters map[string][]string, generated, tracked []string) ([]string, error) {
	docs, err := compileFilter(filters, "docs")
	if err != nil {
		return nil, err
	}
	docsGen, err := compileFilter(filters, "docs-gen")
	if err != nil {
		return nil, err
	}

	var problems []string
	for _, f := range generated {
		if matchAny(docs, f) && !matchAny(docsGen, f) {
			problems = append(problems, fmt.Sprintf("make gen writes %s, which the docs filter matches, but no docs-gen pattern matches it", f))
		}
	}
	for i, re := range docsGen {
		if !matchAny([]*regexp.Regexp{re}, tracked...) {
			problems = append(problems, fmt.Sprintf("docs-gen pattern %q matches no tracked file", filters["docs-gen"][i]))
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

func matchAny(res []*regexp.Regexp, paths ...string) bool {
	for _, re := range res {
		for _, p := range paths {
			if re.MatchString(p) {
				return true
			}
		}
	}
	return false
}

// globRegexp converts the subset of picomatch syntax used by the workflow
// filters: "**" matches across directories, "*" and "?" match within one
// path segment. Anything else that picomatch treats specially is rejected,
// so an unsupported pattern fails the check instead of matching wrongly.
func globRegexp(glob string) (*regexp.Regexp, error) {
	if glob == "" || strings.HasPrefix(glob, "!") || strings.ContainsAny(glob, "[]{}()+@") {
		return nil, xerrors.Errorf("unsupported pattern %q", glob)
	}
	parts := []string{"^"}
	for i := 0; i < len(glob); i++ {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			// "a/**/b" also matches "a/b".
			parts = append(parts, "(?:.*/)?")
			i += 2
		case strings.HasPrefix(glob[i:], "**"):
			parts = append(parts, ".*")
			i++
		case glob[i] == '*':
			parts = append(parts, "[^/]*")
		case glob[i] == '?':
			parts = append(parts, "[^/]")
		default:
			parts = append(parts, regexp.QuoteMeta(string(glob[i])))
		}
	}
	parts = append(parts, "$")
	return regexp.Compile(strings.Join(parts, ""))
}

// Command checkexperimentkeys validates experiment keys named in documentation.
//
// The check recognizes only explicit enablement forms: --experiments=<key-list>
// and CODER_EXPERIMENTS=<key-list>, where a key list is comma-separated. It
// deliberately does not infer bare identifiers from prose, which would make
// the check susceptible to false positives. Placeholder values such as
// feature1, feature2, <experiment-key>, and * are ignored.
//
// Usage:
//
//	checkexperimentkeys [path ...]
//
// With no arguments it scans docs/. Arguments may be files or directories.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

var experimentAssignment = regexp.MustCompile(`(?:--experiments|CODER_EXPERIMENTS)=([a-zA-Z0-9_*<>,-]+)`)

var placeholderKeys = map[string]bool{
	"*":        true,
	"feature1": true,
	"feature2": true,
}

type finding struct {
	path string
	line int
	key  string
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"docs"}
	}
	os.Exit(run(roots, os.Stderr))
}

// run scans roots, writes findings to stderr, and returns 0 for success, 1 for
// unknown keys, or 2 for I/O errors.
func run(roots []string, stderr io.Writer) int {
	return runWithReadFile(roots, stderr, os.ReadFile)
}

func runWithReadFile(roots []string, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	files, err := collectMarkdown(roots)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "checkexperimentkeys: %v\n", err)
		return 2
	}

	known := make(map[string]bool, len(codersdk.ExperimentsKnown))
	for _, experiment := range codersdk.ExperimentsKnown {
		known[string(experiment)] = true
	}

	var findings []finding
	for _, path := range files {
		src, err := readFile(path)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "checkexperimentkeys: %v\n", err)
			return 2
		}
		findings = append(findings, checkSource(path, src, known)...)
	}

	for _, finding := range findings {
		_, _ = fmt.Fprintf(stderr, "%s:%d: unknown experiment key %q; add it to codersdk.ExperimentsKnown or remove or update this documented enablement example.\n", finding.path, finding.line, finding.key)
	}
	if len(findings) > 0 {
		_, _ = fmt.Fprintf(stderr, "\ncheckexperimentkeys: found %d documented experiment key(s) that are not in codersdk.ExperimentsKnown.\n", len(findings))
		return 1
	}

	return 0
}

func checkSource(path string, src []byte, known map[string]bool) []finding {
	var findings []finding
	for lineNumber, line := range strings.Split(string(src), "\n") {
		for _, match := range experimentAssignment.FindAllStringSubmatch(line, -1) {
			for _, key := range strings.Split(match[1], ",") {
				if known[key] || isPlaceholder(key) {
					continue
				}
				findings = append(findings, finding{path: path, line: lineNumber + 1, key: key})
			}
		}
	}
	return findings
}

func isPlaceholder(key string) bool {
	return placeholderKeys[key] || (strings.HasPrefix(key, "<") && strings.HasSuffix(key, ">"))
}

func collectMarkdown(roots []string) ([]string, error) {
	var files []string
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if filepath.Ext(root) == ".md" {
				files = append(files, root)
			}
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return slices.Compact(files), nil
}

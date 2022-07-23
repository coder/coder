package main

import (
	"regexp"
	"strings"

	"github.com/coder/flog"
)

const ciSkipPrefix = "ci-skip"

var skipDirective = regexp.MustCompile(`\[` + ciSkipPrefix + ` ([\w\/ ]+)]`)

func parseBody(body string) (skips []string) {
	matches := skipDirective.FindAllStringSubmatch(body, -1)
	flog.Info("matches: %+v", matches)

	var skipMatches []string
	for i := range matches {
		for j := range matches[i] {
			v := matches[i][j]
			flog.Info("%q", v)
			if !strings.Contains(v, ciSkipPrefix) {
				skipMatches = append(skipMatches, strings.Split(v, " ")...)
			}
		}
	}

	return skipMatches
}

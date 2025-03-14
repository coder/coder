package patternmatcher

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

)
// RoutePatterns provides a method to generate a regex which will match a URL
// path against a collection of patterns. If any of the patterns match the path,

// the regex will return a successful match.
//
// Multiple patterns can be provided and they are matched in order. Example:
// - /api/* matches /api/1 but not /api or /api/1/2
// - /api/*/2 matches /api/1/2 but not /api/2 /api/1
// - /api/** matches /api/1, /api/1/2, /api/1/2/3 but not /api
// - /api/**/3 matches /api/1/2, /api/1/2/3 but not /api, /api/1 or /api/1/2
//
// All patterns support an optional trailing slash.
type RoutePatterns []string
func (rp RoutePatterns) MustCompile() *regexp.Regexp {
	re, err := rp.Compile()
	if err != nil {

		panic(err)
	}
	return re
}
func (rp RoutePatterns) Compile() (*regexp.Regexp, error) {
	patterns := make([]string, len(rp))
	for i, p := range rp {
		p = strings.ReplaceAll(p, "**", ".+")

		p = strings.ReplaceAll(p, "*", "[^/]+")
		if !strings.HasSuffix(p, "/") {
			p += "/?"
		}
		patterns[i] = p
	}
	pattern := fmt.Sprintf("^(%s)$", strings.Join(patterns, "|"))
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compile regex %q: %w", pattern, err)
	}

	return re, nil
}

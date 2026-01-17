//nolint:revive,gocritic,errname,unconvert
package rulesengine

import (
	"log/slog"
	neturl "net/url"
	"strings"
)

// Engine evaluates HTTP requests against a set of rules.
type Engine struct {
	rules  []Rule
	logger *slog.Logger
}

// NewRuleEngine creates a new rule engine
func NewRuleEngine(rules []Rule, logger *slog.Logger) Engine {
	return Engine{
		rules:  rules,
		logger: logger,
	}
}

// Result contains the result of rule evaluation
type Result struct {
	Allowed bool
	Rule    string // The rule that matched (if any)
}

// Evaluate evaluates a request and returns both result and matching rule
func (re *Engine) Evaluate(method, url string) Result {
	// Check if any allow rule matches
	for _, rule := range re.rules {
		if re.matches(rule, method, url) {
			return Result{
				Allowed: true,
				Rule:    rule.Raw,
			}
		}
	}

	// Default deny if no allow rules match
	return Result{
		Allowed: false,
		Rule:    "",
	}
}

// Matches checks if the rule matches the given method and URL using wildcard patterns
func (re *Engine) matches(r Rule, method, url string) bool {
	// Check method patterns if they exist
	if r.MethodPatterns != nil {
		methodMatches := false
		for mp := range r.MethodPatterns {
			if string(mp) == method || mp == "*" {
				methodMatches = true
				break
			}
		}
		if !methodMatches {
			re.logger.Debug("rule does not match", "reason", "method pattern mismatch", "rule", r.Raw, "method", method, "url", url)
			return false
		}
	}

	// If the provided url doesn't have a scheme parsing will fail. This can happen when you do something like `curl google.com`

	if !strings.Contains(url, "://") {
		// This is just for parsing, we won't use the scheme.
		url = "https://" + url
	}
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		re.logger.Debug("rule does not match", "reason", "invalid URL", "rule", r.Raw, "method", method, "url", url, "error", err)
		return false
	}

	if r.HostPattern != nil {
		// For a host pattern to match, every label has to match or be an `*`.
		// Subdomains also match automatically, meaning if the pattern is "example.com"
		// and the real is "api.example.com", it should match. We check this by comparing
		// from the end of the actual hostname with the pattern (which is in normal order).

		labels := strings.Split(parsedURL.Hostname(), ".")

		// If the host pattern is longer than the actual host, it's definitely not a match
		if len(r.HostPattern) > len(labels) {
			re.logger.Debug("rule does not match", "reason", "host pattern too long", "rule", r.Raw, "method", method, "url", url, "pattern_length", len(r.HostPattern), "hostname_labels", len(labels))
			return false
		}

		// Since host patterns cannot end with asterisk, we only need to handle:
		// "example.com" or "*.example.com" - match from the end (allowing subdomains)
		for i, lp := range r.HostPattern {
			labelIndex := len(labels) - len(r.HostPattern) + i
			if string(lp) != labels[labelIndex] && lp != "*" {
				re.logger.Debug("rule does not match", "reason", "host pattern label mismatch", "rule", r.Raw, "method", method, "url", url, "expected", string(lp), "actual", labels[labelIndex])
				return false
			}
		}
	}

	if r.PathPattern != nil {
		segments := strings.Split(parsedURL.Path, "/")

		// Skip the first empty segment if the path starts with "/"
		if len(segments) > 0 && segments[0] == "" {
			segments = segments[1:]
		}

		// Check if any of the path patterns match
		pathMatches := false
		for _, pattern := range r.PathPattern {
			// If the path pattern is longer than the actual path, definitely not a match
			if len(pattern) > len(segments) {
				continue
			}

			// Each segment in the pattern must be either as asterisk or match the actual path segment
			patternMatches := true
			for i, sp := range pattern {
				if sp != segments[i] && sp != "*" {
					patternMatches = false
					break
				}
			}

			if !patternMatches {
				continue
			}

			// If the path is longer than the path pattern, it should only match if:
			// 1. The pattern is empty (root path matches any path), OR
			// 2. The final segment of the pattern is an asterisk
			if len(segments) > len(pattern) && len(pattern) > 0 && pattern[len(pattern)-1] != "*" {
				continue
			}

			pathMatches = true
			break
		}

		if !pathMatches {
			re.logger.Debug("rule does not match", "reason", "no path pattern matches", "rule", r.Raw, "method", method, "url", url)
			return false
		}
	}

	re.logger.Debug("rule matches", "reason", "all patterns matched", "rule", r.Raw, "method", method, "url", url)
	return true
}

package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// commitEntry represents a single non-merge commit.
type commitEntry struct {
	SHA       string
	FullSHA   string
	Title     string
	PRCount   int // 0 if no PR number found
	Timestamp int64
}

var prNumRe = regexp.MustCompile(`\(#(\d+)\)`)

// cherryPickPRRe matches cherry-pick bot titles like
// "chore: foo bar (cherry-pick #42) (#43)".
var cherryPickPRRe = regexp.MustCompile(`\(cherry-pick #(\d+)\)\s*\(#\d+\)$`)

// commitLog returns non-merge commits in the given range, filtering
// out left-side commits (already in the base) and deduplicating
// cherry-picks using git's --cherry-mark.
func commitLog(commitRange string) ([]commitEntry, error) {
	// Use --left-right --cherry-mark to identify equivalent
	// (cherry-picked) commits and left-side-only commits.
	out, err := gitOutput("log", "--no-merges", "--left-right", "--cherry-mark",
		"--pretty=format:%m %ct %h %H %s", commitRange)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	// Collect cherry-pick equivalent commits (marked with '=') so
	// we can skip duplicates. We keep only the right-side version.
	seen := make(map[string]bool)

	var entries []commitEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: %m %ct %h %H %s
		// mark timestamp shortSHA fullSHA title...
		parts := strings.SplitN(line, " ", 5)
		if len(parts) < 5 {
			continue
		}
		mark := parts[0]
		ts, _ := strconv.ParseInt(parts[1], 10, 64)
		shortSHA := parts[2]
		fullSHA := parts[3]
		title := parts[4]

		// Skip left-side commits (already in the old version).
		if mark == "<" {
			continue
		}
		// Skip cherry-pick equivalents that we've already seen
		// (marked '=' by --cherry-mark).
		if mark == "=" {
			if seen[title] {
				continue
			}
			seen[title] = true
		}

		// Normalize cherry-pick bot titles:
		// "chore: foo (cherry-pick #42) (#43)" → "chore: foo (#42)"
		if m := cherryPickPRRe.FindStringSubmatch(title); m != nil {
			title = title[:cherryPickPRRe.FindStringIndex(title)[0]] + "(#" + m[1] + ")"
		}

		e := commitEntry{
			SHA:       shortSHA,
			FullSHA:   fullSHA,
			Title:     title,
			Timestamp: ts,
		}
		if m := prNumRe.FindStringSubmatch(e.Title); m != nil {
			e.PRCount, _ = strconv.Atoi(m[1])
		}
		entries = append(entries, e)
	}

	// Sort by conventional commit prefix, then by timestamp
	// (matching the bash script's sort -k3,3 -k1,1n).
	sort.SliceStable(entries, func(i, j int) bool {
		pi := commitSortPrefix(entries[i].Title)
		pj := commitSortPrefix(entries[j].Title)
		if pi != pj {
			return pi < pj
		}
		return entries[i].Timestamp < entries[j].Timestamp
	})

	return entries, nil
}

// commitSortPrefix extracts the first word of a title for sorting.
func commitSortPrefix(title string) string {
	idx := strings.IndexAny(title, " (:")
	if idx < 0 {
		return title
	}
	return title[:idx]
}

// humanizedAreas maps conventional commit scopes to human-readable area
// names. Order matters: more specific prefixes must come first so that
// the first partial match wins.
var humanizedAreas = []struct {
	Prefix string
	Area   string
}{
	{"agent/agentssh", "Agent SSH"},
	{"coderd/database", "Database"},
	{"enterprise/audit", "Auditing"},
	{"enterprise/cli", "CLI"},
	{"enterprise/coderd", "Server"},
	{"enterprise/dbcrypt", "Database"},
	{"enterprise/derpmesh", "Networking"},
	{"enterprise/provisionerd", "Provisioner"},
	{"enterprise/tailnet", "Networking"},
	{"enterprise/wsproxy", "Workspace Proxy"},
	{"agent", "Agent"},
	{"cli", "CLI"},
	{"coderd", "Server"},
	{"codersdk", "SDK"},
	{"docs", "Documentation"},
	{"enterprise", "Enterprise"},
	{"examples", "Examples"},
	{"helm", "Helm"},
	{"install.sh", "Installer"},
	{"provisionersdk", "SDK"},
	{"provisionerd", "Provisioner"},
	{"provisioner", "Provisioner"},
	{"pty", "CLI"},
	{"scaletest", "Scale Testing"},
	{"site", "Dashboard"},
	{"support", "Support"},
	{"tailnet", "Networking"},
}

// conventionalPrefixRe extracts prefix, scope, and rest from a
// conventional commit title. Does NOT match breaking "!" suffix —
// those titles are left as-is (matching bash behavior).
var conventionalPrefixRe = regexp.MustCompile(`^([a-z]+)(\((.+)\))?:\s*(.*)$`)

// humanizeTitle converts a conventional commit title to a
// human-readable form, e.g. "feat(site): add bar" → "Dashboard: Add bar".
func humanizeTitle(title string) string {
	m := conventionalPrefixRe.FindStringSubmatch(title)
	if m == nil {
		return title
	}
	scope := m[3] // may be empty
	rest := m[4]
	if rest == "" {
		return title
	}
	// Capitalize the first letter of the rest.
	rest = strings.ToUpper(rest[:1]) + rest[1:]

	if scope == "" {
		return rest
	}

	// Look up scope in humanizedAreas (first partial match wins).
	for _, ha := range humanizedAreas {
		if strings.HasPrefix(scope, ha.Prefix) {
			return ha.Area + ": " + rest
		}
	}
	// Scope not found in map — return as-is.
	return title
}

// breakingCommitRe matches conventional commit "!:" breaking changes.
var breakingCommitRe = regexp.MustCompile(`^[a-zA-Z]+(\(.+\))?!:`)

// categorizeCommit determines the release note section for a commit.
// The priority order matches the bash script: breaking title first,
// then labels (breaking, security, experimental), then prefix.
func categorizeCommit(title string, labels []string) string {
	// Check breaking title first (matches bash behavior).
	if breakingCommitRe.MatchString(title) {
		return "breaking"
	}

	// Label-based categorization.
	for _, l := range labels {
		if l == "release/breaking" {
			return "breaking"
		}
		if l == "security" {
			return "security"
		}
		if l == "release/experimental" {
			return "experimental"
		}
	}

	// Extract the conventional commit prefix (e.g. "feat", "fix(scope)").
	prefixRe := regexp.MustCompile(`^([a-z]+)(\(.+\))?[!]?:`)
	m := prefixRe.FindStringSubmatch(title)
	if m == nil {
		return "other"
	}

	validPrefixes := []string{
		"feat", "fix", "docs", "refactor", "perf",
		"test", "build", "ci", "chore", "revert",
	}
	for _, p := range validPrefixes {
		if m[1] == p {
			return p
		}
	}
	return "other"
}

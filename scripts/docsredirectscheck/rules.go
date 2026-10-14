package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// allowedExternalPrefixes are the only places a destination may send a reader
// outside the docs.
var allowedExternalPrefixes = []string{
	"https://github.com/coder/",
	"https://coder.com/",
}

// pattern is a rule source: an exact route, or a route prefix ending in "/*"
// that matches the prefix itself and every route beneath it.
type pattern struct {
	wild bool
	// prefix is the route with any trailing "/*" removed.
	prefix string
}

func (p pattern) matches(route string) bool {
	route = normalizeRoute(route)
	if p.wild {
		return route == p.prefix || strings.HasPrefix(route, p.prefix+"/")
	}
	return route == p.prefix
}

// key identifies a source for duplicate detection.
func (p pattern) key() string {
	return fmt.Sprintf("%t %s", p.wild, p.prefix)
}

// covers reports whether every route q matches is also matched by p, which
// makes q unreachable when p is earlier in the file. Rules apply in order and
// the first match wins, so a broad pattern above a narrower rule swallows it.
//
// An exact p never covers anything but an identical exact q, which the
// duplicate check already reports. In particular an exact rule above a
// wildcard on the same prefix is deliberate: it claims the bare prefix and
// leaves the subtree to the wildcard.
func (p pattern) covers(q pattern) bool {
	if !p.wild {
		return false
	}
	return q.prefix == p.prefix || strings.HasPrefix(q.prefix, p.prefix+"/")
}

type destKind int

const (
	// destRoute is a route of the same docs version, possibly ending in "/*".
	destRoute destKind = iota
	// destRoot is the docs root of the same version.
	destRoot
	// destExternal is an allowed https URL.
	destExternal
)

type destination struct {
	kind destKind
	// raw is the destination as written, for messages.
	raw string
	// wild is true for a route ending in "/*".
	wild bool
	// prefix is the route of a destRoute destination, without any "/*" or
	// fragment.
	prefix string
	// fragment is the part after "#" of an exact route destination.
	fragment string
	// valid is false when the destination could not be parsed, in which case
	// no cross-rule check uses it.
	valid bool
}

// rule is one validated entry of docs/redirects.json.
type rule struct {
	index  int
	source string
	src    pattern
	dst    destination
}

func (r rule) matches(route string) bool {
	return r.src.matches(route)
}

// target returns where the rule sends a request for route: the destination
// kind and, for a route destination, the route with the wildcard substituted.
func (r rule) target(route string) (destKind, string) {
	if r.dst.kind != destRoute {
		return r.dst.kind, ""
	}
	if !r.dst.wild {
		return destRoute, r.dst.prefix
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(normalizeRoute(route), r.src.prefix), "/")
	if rest == "" {
		return destRoute, r.dst.prefix
	}
	return destRoute, r.dst.prefix + "/" + rest
}

// problem is one thing wrong with docs/redirects.json. index is the position
// of the offending rule in the array, or -1 for a problem with the whole file.
type problem struct {
	index   int
	source  string
	message string
}

func formatProblem(file string, p problem) string {
	switch {
	case p.index < 0:
		return fmt.Sprintf("%s: %s", file, p.message)
	case p.source != "":
		return fmt.Sprintf("%s[%d] (source %q): %s", file, p.index, p.source, p.message)
	default:
		return fmt.Sprintf("%s[%d]: %s", file, p.index, p.message)
	}
}

// segment is one route segment. "." and ".." match it and are rejected
// separately.
var segment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// fragmentRe matches the fragment of a destination, which has the same
// characters as a segment.
var fragmentRe = segment

// legacyParam matches a trailing path parameter in the old syntax: ":name",
// ":name*" and ":name(.*)".
var legacyParam = regexp.MustCompile(`^:[A-Za-z_][A-Za-z0-9_]*(\*|\(\.\*\))?$`)

// normalizeRoute drops trailing slashes so "a/b/" and "a/b" compare equal, the
// way the websites route them.
func normalizeRoute(p string) string {
	return strings.TrimRight(p, "/")
}

// checkRoute returns why route is not a valid route, or "" when it is. The
// empty route is not valid here: it is the docs root, which only a destination
// can name, as "/".
func checkRoute(route string) string {
	switch {
	case route == "":
		return "empty route"
	case strings.HasPrefix(route, "/"):
		return "routes are relative to the docs root and have no leading slash"
	case strings.HasSuffix(route, "/"):
		return "routes have no trailing slash"
	}
	segments := strings.Split(route, "/")
	for _, seg := range segments {
		switch {
		case seg == "":
			return "empty path segment"
		case seg == "." || seg == "..":
			return fmt.Sprintf("segment %q is not allowed", seg)
		case !segment.MatchString(seg):
			return fmt.Sprintf("segment %q must match [A-Za-z0-9._-]+", seg)
		}
	}
	if strings.HasSuffix(segments[len(segments)-1], ".md") {
		return `routes do not end in ".md"`
	}
	return ""
}

func parseSource(raw string) (pattern, string) {
	prefix, wild := strings.CutSuffix(raw, "/*")
	if msg := checkRoute(prefix); msg != "" {
		if strings.Contains(raw, "*") && !wild {
			return pattern{}, fmt.Sprintf(`unsupported source %q: "*" is only allowed as a trailing "/*"`, raw)
		}
		return pattern{}, fmt.Sprintf("unsupported source %q: %s", raw, msg)
	}
	return pattern{wild: wild, prefix: prefix}, ""
}

func parseDestination(raw string) (destination, string) {
	d := destination{raw: raw}
	switch {
	case raw == "/":
		d.kind = destRoot
	case strings.HasPrefix(raw, "https://"):
		if msg := checkExternal(raw); msg != "" {
			return d, msg
		}
		d.kind = destExternal
	case strings.Contains(raw, "://"):
		return d, fmt.Sprintf(`unsupported destination %q: external destinations must be https:// URLs under %s`, raw, strings.Join(allowedExternalPrefixes, " or "))
	default:
		route, fragment, hasFragment := strings.Cut(raw, "#")
		if hasFragment {
			switch {
			case route == "/" || strings.HasSuffix(route, "/*"):
				return d, fmt.Sprintf(`unsupported destination %q: "/" and wildcard destinations do not take a fragment`, raw)
			case !fragmentRe.MatchString(fragment):
				return d, fmt.Sprintf(`unsupported destination %q: a fragment must match [A-Za-z0-9._-]+ and there is at most one`, raw)
			}
		}
		prefix, wild := strings.CutSuffix(route, "/*")
		if msg := checkRoute(prefix); msg != "" {
			if strings.Contains(route, "*") && !wild {
				return d, fmt.Sprintf(`unsupported destination %q: "*" is only allowed as a trailing "/*"`, raw)
			}
			return d, fmt.Sprintf(`unsupported destination %q: %s (use a route such as "admin/users", "/" for the docs root, or an allowed https URL)`, raw, msg)
		}
		d.kind, d.prefix, d.wild, d.fragment = destRoute, prefix, wild, fragment
	}
	d.valid = true
	return d, ""
}

func checkExternal(raw string) string {
	allowed := false
	for _, p := range allowedExternalPrefixes {
		if strings.HasPrefix(raw, p) {
			allowed = true
			break
		}
	}
	switch {
	case !allowed:
		return fmt.Sprintf("external destination %q must start with %s", raw, strings.Join(allowedExternalPrefixes, " or "))
	case strings.Contains(strings.TrimPrefix(raw, "https://"), ":"):
		return fmt.Sprintf(`external destination %q must not contain ":" after the scheme`, raw)
	case strings.ContainsAny(raw, "*{} \t\r\n"):
		return fmt.Sprintf("external destination %q must not contain a wildcard, parameter, or whitespace", raw)
	}
	return ""
}

func isLegacyPath(s string) bool {
	return s == "/docs" || strings.HasPrefix(s, "/docs/")
}

// convertLegacy rewrites a source or destination from the old absolute form
// ("/docs/admin/old", "/docs/x/:path*") to a route ("admin/old", "x/*").
// Anything else is returned unchanged.
func convertLegacy(s string) string {
	if !isLegacyPath(s) {
		return s
	}
	route := strings.Trim(strings.TrimPrefix(s, "/docs"), "/")
	segments := strings.Split(route, "/")
	if last := len(segments) - 1; last > 0 && legacyParam.MatchString(segments[last]) {
		segments[last] = "*"
	}
	return strings.Join(segments, "/")
}

// convertLegacyDestination is convertLegacy for a destination, where the old
// docs root "/docs" is "/".
func convertLegacyDestination(s string) string {
	if s == "/docs" || s == "/docs/" {
		return "/"
	}
	return convertLegacy(s)
}

// ruleJSON renders a rule the way it is written in docs/redirects.json, on one
// line, so a message can be pasted into the file.
func ruleJSON(source, destination string, permanent *bool) string {
	quote := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			return strconv.Quote(s)
		}
		// json.Marshal escapes HTML characters, which would make a URL with a
		// query string harder to read and paste.
		return strings.NewReplacer(`\u0026`, "&", `\u003c`, "<", `\u003e`, ">").Replace(string(b))
	}
	out := fmt.Sprintf(`{"source": %s, "destination": %s`, quote(source), quote(destination))
	if permanent != nil {
		out += fmt.Sprintf(`, "permanent": %t`, *permanent)
	}
	return out + "}"
}

// checkRedirects validates the contents of docs/redirects.json against the set
// of live docs routes. It returns the rules whose source could be parsed, which
// the removed-route check resolves routes through, and every problem it found,
// ordered by rule.
func checkRedirects(data []byte, live routeSet) ([]rule, []problem) {
	var entries []json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			// Valid JSON, but an object or scalar rather than an array.
			return nil, []problem{{index: -1, message: "must be a JSON array of rules"}}
		}
		return nil, []problem{{index: -1, message: fmt.Sprintf("is not valid JSON: %v", err)}}
	}
	if entries == nil {
		// A JSON null decodes to a nil slice without an error.
		return nil, []problem{{index: -1, message: "must be a JSON array of rules"}}
	}

	var (
		rules    []rule
		problems []problem
	)
	for i, entry := range entries {
		r, ps := checkEntry(i, entry, live)
		problems = append(problems, ps...)
		if r != nil {
			rules = append(rules, *r)
		}
	}

	problems = append(problems, checkAcrossRules(rules)...)
	slices.SortStableFunc(problems, func(a, b problem) int { return cmp.Compare(a.index, b.index) })
	return rules, problems
}

const entryShape = `must be an object with "source", "destination" and optional "permanent"`

// checkEntry validates one array element on its own. It returns the rule when
// the source parsed, so that cross-rule checks can still use it.
func checkEntry(i int, entry json.RawMessage, live routeSet) (*rule, []problem) {
	if bytes.Equal(bytes.TrimSpace(entry), []byte("null")) {
		return nil, []problem{{index: i, message: entryShape}}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(entry, &fields); err != nil {
		return nil, []problem{{index: i, message: entryShape}}
	}

	var problems []problem
	add := func(source, msg string) {
		problems = append(problems, problem{index: i, source: source, message: msg})
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		switch k {
		case "source", "destination", "permanent":
		default:
			add("", fmt.Sprintf("unknown field %q (allowed: source, destination, permanent)", k))
		}
	}

	source, sourceOK := stringField(fields, "source")
	if !sourceOK {
		add("", `"source" is required and must be a non-empty string`)
	}
	dest, destOK := stringField(fields, "destination")
	if !destOK {
		add(source, `"destination" is required and must be a non-empty string`)
	}
	var permanent *bool
	if raw, ok := fields["permanent"]; ok {
		var b bool
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &b) != nil {
			add(source, `"permanent" must be a boolean`)
		} else {
			permanent = &b
		}
	}

	if !sourceOK {
		return nil, problems
	}
	if isLegacyPath(source) || (destOK && isLegacyPath(dest)) {
		fixed := ruleJSON(convertLegacy(source), convertLegacyDestination(dest), permanent)
		add(source, fmt.Sprintf(`legacy absolute "/docs/..." form: write routes relative to the docs root, "/" for the docs root, and "/*" for a trailing wildcard. Converted rule: %s`, fixed))
		return nil, problems
	}
	src, msg := parseSource(source)
	if msg != "" {
		add(source, msg)
		return nil, problems
	}

	r := &rule{index: i, source: source, src: src}
	if destOK {
		d, msg := parseDestination(dest)
		r.dst = d
		if msg != "" {
			add(source, msg)
		} else {
			for _, m := range checkDestination(d, src, live) {
				add(source, m)
			}
		}
	}
	if m := checkShadowing(src, live); m != "" {
		add(source, m)
	}
	return r, problems
}

func stringField(fields map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := fields[name]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

// checkDestination verifies that an internal destination lands on a page that
// exists and that a wildcard destination has a wildcard source to take its
// match from.
func checkDestination(d destination, src pattern, live routeSet) []string {
	if d.kind != destRoute {
		return nil
	}
	if !d.wild {
		if !live.has(d.prefix) {
			return []string{fmt.Sprintf("destination %q is not a page in docs/manifest.json; point it at an existing page", d.raw)}
		}
		return nil
	}
	var msgs []string
	if !src.wild {
		msgs = append(msgs, fmt.Sprintf(`destination %q ends in "/*" but the source is not a wildcard; end the source in "/*" too, or use an exact destination`, d.raw))
	}
	if !prefixHasPages(d.prefix, live) {
		msgs = append(msgs, fmt.Sprintf("no docs page exists under %q; point the destination at an existing section", d.prefix))
	}
	return msgs
}

// prefixHasPages reports whether a wildcard destination with this prefix can
// land on a page: the prefix is a page or a section of pages.
func prefixHasPages(prefix string, live routeSet) bool {
	if live.has(prefix) {
		return true
	}
	for route := range live {
		if strings.HasPrefix(route, prefix+"/") {
			return true
		}
	}
	return false
}

// checkShadowing reports a source that matches live docs pages. A page always
// wins over a redirect, and sites that apply redirects before pages would hide
// it, so such a rule is rejected.
func checkShadowing(src pattern, live routeSet) string {
	var hidden []string
	for route := range live {
		if src.matches(route) {
			hidden = append(hidden, route)
		}
	}
	if len(hidden) == 0 {
		return ""
	}
	slices.Sort(hidden)
	more := ""
	if len(hidden) > 1 {
		more = fmt.Sprintf(" (and %d more)", len(hidden)-1)
	}
	return fmt.Sprintf("source hides a live docs page: %s%s; remove the redirect, or remove the page from docs/manifest.json first", displayRoute(hidden[0]), more)
}

func displayRoute(route string) string {
	if route == "" {
		return "/ (the docs root)"
	}
	return route
}

// checkOrder reports rules that can never apply because an earlier rule
// already matches every route they would. Rules apply in file order and the
// first match wins, so they have to run from most specific to least: a
// trailing wildcard above a rule inside its subtree swallows it.
//
// Each unreachable rule is reported once, against the first rule that covers
// it. Pairs with the same source are left to the duplicate check.
func checkOrder(rules []rule) []problem {
	var problems []problem
	for j, later := range rules {
		for _, earlier := range rules[:j] {
			if earlier.src.key() == later.src.key() {
				continue
			}
			if !earlier.src.covers(later.src) {
				continue
			}
			problems = append(problems, problem{
				index:  later.index,
				source: later.source,
				message: fmt.Sprintf(
					"unreachable: rule [%d] (source %q) comes first and matches every route this rule would; order rules from most specific to least",
					earlier.index, earlier.source,
				),
			})
			break
		}
	}
	return problems
}

// checkAcrossRules finds duplicate sources, unreachable rules, chains, and
// loops.
func checkAcrossRules(rules []rule) []problem {
	var problems []problem

	seen := make(map[string]int, len(rules))
	for _, r := range rules {
		k := r.src.key()
		if first, dup := seen[k]; dup {
			problems = append(problems, problem{
				index:   r.index,
				source:  r.source,
				message: fmt.Sprintf("duplicate of rule [%d]: two rules with the same source; remove one", first),
			})
			continue
		}
		seen[k] = r.index
	}

	problems = append(problems, checkOrder(rules)...)

	// next[i] is the rule whose source the destination of rule i lands on.
	// Only exact route destinations are followed. Where a wildcard destination
	// ends up depends on the route being redirected, so chains through one are
	// not checked.
	next := make(map[int]int, len(rules))
	byIndex := make(map[int]rule, len(rules))
	for _, r := range rules {
		byIndex[r.index] = r
	}
	for _, r := range rules {
		if !r.dst.valid || r.dst.kind != destRoute || r.dst.wild {
			continue
		}
		for _, other := range rules {
			if other.matches(r.dst.prefix) {
				next[r.index] = other.index
				break
			}
		}
	}

	for _, r := range rules {
		if _, ok := next[r.index]; !ok {
			continue
		}
		path, cycleStart, cyclic := followRedirects(r.index, next)
		switch {
		case cyclic && cycleStart == 0:
			cycle := path
			// Report a loop once, on its lowest-numbered rule.
			if r.index != slices.Min(cycle) {
				continue
			}
			problems = append(problems, problem{index: r.index, source: r.source, message: describeLoop(cycle, byIndex)})
		case cyclic:
			// Leads into a loop it is not part of.
			problems = append(problems, problem{
				index:   r.index,
				source:  r.source,
				message: fmt.Sprintf("redirect chain into a loop: destination %q is the source of rule [%d], which never reaches a page", r.dst.raw, path[1]),
			})
		default:
			last := byIndex[path[len(path)-1]]
			problems = append(problems, problem{
				index:  r.index,
				source: r.source,
				message: fmt.Sprintf(
					"redirect chain: destination %q is the source of rule [%d], which redirects to %q; point this rule at %q directly",
					r.dst.raw, path[1], last.dst.raw, last.dst.raw,
				),
			})
		}
	}
	return problems
}

// followRedirects walks next from start. It returns the rules visited in
// order, and when the walk revisits a rule, cyclic is true and cycleStart is
// the position in path where that rule first appeared.
func followRedirects(start int, next map[int]int) (path []int, cycleStart int, cyclic bool) {
	pos := map[int]int{start: 0}
	path = []int{start}
	cur := start
	for {
		n, ok := next[cur]
		if !ok {
			return path, 0, false
		}
		if at, seen := pos[n]; seen {
			return path, at, true
		}
		pos[n] = len(path)
		path = append(path, n)
		cur = n
	}
}

func describeLoop(cycle []int, byIndex map[int]rule) string {
	if len(cycle) == 1 {
		return "redirects to itself; remove the rule or point it at a different page"
	}
	parts := make([]string, 0, len(cycle)+1)
	for _, idx := range cycle {
		parts = append(parts, fmt.Sprintf("rule [%d] %q", idx, byIndex[idx].source))
	}
	parts = append(parts, fmt.Sprintf("rule [%d]", cycle[0]))
	return "redirect loop: " + strings.Join(parts, " -> ") + "; break the loop by pointing one rule at a real page"
}

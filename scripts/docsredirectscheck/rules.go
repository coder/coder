package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

type patternKind int

const (
	// kindExact matches one path.
	kindExact patternKind = iota
	// kindWildcard is "/prefix/:name*". It matches the prefix itself and
	// everything beneath it.
	kindWildcard
	// kindRegexWildcard is "/prefix/:name(.*)". It matches everything beneath
	// the prefix but not the bare prefix.
	kindRegexWildcard
)

// pattern is a source or destination path in the small subset of Next.js
// path syntax the docs website accepts.
type pattern struct {
	kind patternKind
	// prefix is the static part of the path with no trailing slash. For an
	// exact pattern it is the whole path.
	prefix string
	// param is the parameter name of a wildcard pattern, empty for exact.
	param string
}

func (p pattern) matches(path string) bool {
	path = normalizePath(path)
	switch p.kind {
	case kindWildcard:
		return path == p.prefix || strings.HasPrefix(path, p.prefix+"/")
	case kindRegexWildcard:
		return strings.HasPrefix(path, p.prefix+"/")
	default:
		return path == p.prefix
	}
}

// key identifies a source for duplicate detection. The parameter name is left
// out because two patterns that differ only by it match the same paths.
func (p pattern) key() string {
	return fmt.Sprintf("%d %s", p.kind, p.prefix)
}

type destination struct {
	external bool
	// raw is the destination as written, for messages.
	raw string
	// pattern is the path part of an internal destination. The query string
	// and fragment are dropped because they do not change which page it is.
	pattern pattern
	// valid is false when the destination could not be parsed, in which case
	// no cross-rule check uses it.
	valid bool
}

// rule is one validated entry of docs/redirects.json.
type rule struct {
	index       int
	source      string
	destination string
	src         pattern
	dst         destination
}

func (r rule) matches(path string) bool {
	return r.src.matches(path)
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

var (
	// staticSegment is one literal path segment.
	staticSegment = regexp.MustCompile(`^[A-Za-z0-9._~%+-]+$`)
	// versionSegment is a literal "@ref" segment. The docs website adds the
	// version to a URL itself, so rules are written without it.
	versionSegment = regexp.MustCompile(`^@[A-Za-z0-9._~%+-]+$`)
	// sourceWildcard matches ":name*".
	sourceWildcard = regexp.MustCompile(`^:([A-Za-z_][A-Za-z0-9_]*)\*$`)
	// sourceRegexWildcard matches ":name(.*)".
	sourceRegexWildcard = regexp.MustCompile(`^:([A-Za-z_][A-Za-z0-9_]*)\(\.\*\)$`)
	// destinationParam matches ":name" and ":name*".
	destinationParam = regexp.MustCompile(`^:([A-Za-z_][A-Za-z0-9_]*)\*?$`)
)

// normalizePath drops trailing slashes so "/docs/a/" and "/docs/a" compare
// equal, the way the website routes them.
func normalizePath(p string) string {
	return strings.TrimRight(p, "/")
}

// pathRole says whether a path is being parsed as a rule's source or its
// destination, because the two accept different pattern syntax.
type pathRole int

const (
	roleSource pathRole = iota
	roleDestination
)

// parsePath parses a /docs/... path. Sources allow an exact path, a trailing
// "/:name*" and a trailing "/:name(.*)". Destinations allow an exact path and
// a trailing "/:name" or "/:name*". It returns a message describing the
// problem when the path is outside that subset.
func parsePath(path string, role pathRole) (pattern, string) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	last := len(segments) - 1

	unsupported := func(why string) string {
		if role == roleDestination {
			return fmt.Sprintf("unsupported destination %q (%s): use an exact path or a trailing /:name or /:name*", path, why)
		}
		return fmt.Sprintf("unsupported source %q (%s): use an exact path, a trailing /:name*, or a trailing /:name(.*)", path, why)
	}

	for _, seg := range segments[:last] {
		if err := checkStaticSegment(seg, path, unsupported); err != "" {
			return pattern{}, err
		}
	}

	lastSeg := segments[last]
	prefix := "/" + strings.Join(segments[:last], "/")
	if last == 0 {
		prefix = ""
	}

	if role == roleDestination {
		if m := destinationParam.FindStringSubmatch(lastSeg); m != nil && last > 0 {
			return pattern{kind: kindWildcard, prefix: prefix, param: m[1]}, ""
		}
	} else {
		if m := sourceWildcard.FindStringSubmatch(lastSeg); m != nil && last > 0 {
			return pattern{kind: kindWildcard, prefix: prefix, param: m[1]}, ""
		}
		if m := sourceRegexWildcard.FindStringSubmatch(lastSeg); m != nil && last > 0 {
			return pattern{kind: kindRegexWildcard, prefix: prefix, param: m[1]}, ""
		}
	}

	if err := checkStaticSegment(lastSeg, path, unsupported); err != "" {
		return pattern{}, err
	}
	return pattern{kind: kindExact, prefix: "/" + strings.Join(segments, "/")}, ""
}

func checkStaticSegment(seg, path string, unsupported func(string) string) string {
	switch {
	case seg == "":
		return unsupported("empty path segment")
	case versionSegment.MatchString(seg):
		return fmt.Sprintf("%q contains a version segment (%s): the docs website adds /@<ref> itself, so write the path without it", path, seg)
	case !staticSegment.MatchString(seg):
		return unsupported(fmt.Sprintf("segment %q", seg))
	}
	return ""
}

func parseSource(raw string) (pattern, string) {
	s := normalizePath(raw)
	if !strings.HasPrefix(s, docsRoot+"/") {
		return pattern{}, "source must start with /docs/ (write the full site path, for example /docs/admin/old-page)"
	}
	return parsePath(s, roleSource)
}

func parseDestination(raw string) (destination, string) {
	d := destination{raw: raw}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return d, fmt.Sprintf("destination %q is an invalid URL", raw)
		}
		d.external = true
		d.valid = true
		return d, ""
	}

	p := raw
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = normalizePath(p)
	if p != docsRoot && !strings.HasPrefix(p, docsRoot+"/") {
		return d, fmt.Sprintf("destination %q is not valid: destination must be a /docs/ path or an http(s) URL", raw)
	}
	pat, msg := parsePath(p, roleDestination)
	if msg != "" {
		return d, msg
	}
	d.pattern = pat
	d.valid = true
	return d, ""
}

// checkRedirects validates the contents of docs/redirects.json against the set
// of live docs routes. It returns the rules whose source could be parsed, which
// the advisory check uses to see whether a removed route is already covered,
// and every problem it found, ordered by rule.
func checkRedirects(data []byte, live map[string]bool) ([]rule, []problem) {
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
func checkEntry(i int, entry json.RawMessage, live map[string]bool) (*rule, []problem) {
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
	if raw, ok := fields["permanent"]; ok {
		var b bool
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &b) != nil {
			add(source, `"permanent" must be a boolean`)
		}
	}

	if !sourceOK {
		return nil, problems
	}
	src, msg := parseSource(source)
	if msg != "" {
		add(source, msg)
		return nil, problems
	}

	r := &rule{index: i, source: source, destination: dest, src: src}
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
// exists and that any parameter it uses is one the source defines.
func checkDestination(d destination, src pattern, live map[string]bool) []string {
	if d.external {
		return nil
	}
	var msgs []string
	pat := d.pattern
	if pat.kind == kindExact {
		if !live[pat.prefix] {
			msgs = append(msgs, fmt.Sprintf("destination %q is not a page in docs/manifest.json; point it at an existing page", d.raw))
		}
		return msgs
	}

	if src.param != pat.param {
		msgs = append(msgs, fmt.Sprintf("destination uses :%s, which the source does not define; the source's parameter is %s", pat.param, describeParam(src)))
	}
	if !prefixHasPages(pat.prefix, live) {
		msgs = append(msgs, fmt.Sprintf("no docs page exists under %q; point the destination at an existing section", pat.prefix))
	}
	return msgs
}

func describeParam(p pattern) string {
	if p.param == "" {
		return "absent (the source is an exact path)"
	}
	return ":" + p.param
}

// prefixHasPages reports whether a pattern destination with this static
// prefix can land on a page: the prefix is a page or a section of pages.
func prefixHasPages(prefix string, live map[string]bool) bool {
	if prefix == docsRoot || live[prefix] {
		return true
	}
	for route := range live {
		if strings.HasPrefix(route, prefix+"/") {
			return true
		}
	}
	return false
}

// checkShadowing reports a source that matches live docs pages. Redirects are
// applied before pages, so such a rule would hide a page readers can reach.
func checkShadowing(src pattern, live map[string]bool) string {
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
	return fmt.Sprintf("source hides a live docs page: %s%s; remove the redirect, or remove the page from docs/manifest.json first", hidden[0], more)
}

// checkAcrossRules finds duplicate sources, chains, and loops.
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

	// next[i] is the rule whose source the destination of rule i lands on.
	// Only exact internal destinations are followed. Where a pattern
	// destination ends up depends on the URL being redirected, so chains
	// through one are not checked.
	next := make(map[int]int, len(rules))
	byIndex := make(map[int]rule, len(rules))
	for _, r := range rules {
		byIndex[r.index] = r
	}
	for _, r := range rules {
		if !r.dst.valid || r.dst.external || r.dst.pattern.kind != kindExact {
			continue
		}
		for _, other := range rules {
			if other.matches(r.dst.pattern.prefix) {
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

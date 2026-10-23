package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const (
	// liveListURLFormat is the production page list of one docs version. The
	// segment is "@main" for main and "vX.Y" for release/X.Y.
	liveListURLFormat = "https://coder.com/beta-docs/%s/llms.txt"
	// liveListPathPrefix is where every page of a version lives on the site.
	liveListPathPrefix = "/beta-docs/"

	// userAgent identifies the check to the site. Its CDN answers 403 to the
	// default Go user agent in some cases.
	userAgent = "coder-docsredirectscheck (+https://github.com/coder/coder)"

	fetchAttempts   = 4
	fetchTimeout    = 30 * time.Second
	fetchMaxBytes   = 32 << 20
	maxRedirectHops = 3
)

// fetcher reads a URL, so tests never touch the network.
type fetcher interface {
	// Get returns the HTTP status and body of url.
	Get(ctx context.Context, url string) (status int, body []byte, err error)
}

type httpFetcher struct {
	client *http.Client
}

func newHTTPFetcher() httpFetcher {
	return httpFetcher{client: &http.Client{Timeout: fetchTimeout}}
}

func (f httpFetcher) Get(ctx context.Context, url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// releaseBranchRe matches the release branches that have a docs version of their
// own.
var releaseBranchRe = regexp.MustCompile(`^release/(\d+\.\d+)$`)

// versionSegment returns the docs version segment the live page list of a
// target branch lives under, or false when the branch has no published docs.
func versionSegment(branch string) (string, bool) {
	if branch == "main" {
		return "@main", true
	}
	if m := releaseBranchRe.FindStringSubmatch(branch); m != nil {
		return "v" + m[1], true
	}
	return "", false
}

// linkRe finds the target of a Markdown link.
var linkRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// otherVersionRe matches the first path segment of a link into some version of
// the docs, which an unversioned link never has.
var otherVersionRe = regexp.MustCompile(`^(@[^/]+|v\d+(\.\d+)*)$`)

// parseLiveList returns the routes of the links in an llms.txt body that point
// into the given version. The docs root is the empty route.
//
// The version that is served at the unversioned /beta-docs path links its own
// pages without a version segment, so when no link carries the segment, the
// unversioned links count instead.
func parseLiveList(body []byte, segment string) []string {
	var hrefs []string
	for _, m := range linkRe.FindAllSubmatch(body, -1) {
		href := strings.TrimPrefix(string(m[1]), "https://coder.com")
		if i := strings.IndexAny(href, "?#"); i >= 0 {
			href = href[:i]
		}
		hrefs = append(hrefs, href)
	}
	if routes := routesUnder(hrefs, liveListPathPrefix+segment); len(routes) > 0 {
		return routes
	}
	unversioned := routesUnder(hrefs, strings.TrimSuffix(liveListPathPrefix, "/"))
	return slices.DeleteFunc(unversioned, func(route string) bool {
		first, _, _ := strings.Cut(route, "/")
		return otherVersionRe.MatchString(first)
	})
}

// routesUnder returns the routes of the hrefs at or below base, without
// duplicates and in order of appearance.
func routesUnder(hrefs []string, base string) []string {
	var routes []string
	seen := map[string]bool{}
	for _, href := range hrefs {
		if href != base && !strings.HasPrefix(href, base+"/") {
			continue
		}
		route := convertPathToRoute(strings.TrimPrefix(href, base))
		if route != "" && checkRoute(route) != "" {
			continue
		}
		if !seen[route] {
			seen[route] = true
			routes = append(routes, route)
		}
	}
	return routes
}

// errNotPublished means the site has no page list for a version, which is the
// case for a new release branch before its docs are published.
var errNotPublished = xerrors.New("docs version is not published")

// sleepContext waits for d, or returns early with the context's error when ctx
// is done first.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// fetchLiveRoutes reads the live page list of a version, retrying transient
// failures a bounded number of times. An empty list is an error. A 404 is not
// retried and returns an error that wraps errNotPublished.
func fetchLiveRoutes(ctx context.Context, f fetcher, url, segment string, wait func(context.Context, time.Duration) error) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt < fetchAttempts; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, time.Duration(1<<(attempt-1))*time.Second); err != nil {
				return nil, xerrors.Errorf("cannot read %s: %w", url, err)
			}
		}
		status, body, err := f.Get(ctx, url)
		switch {
		case err != nil:
			lastErr = err
		case status == http.StatusNotFound:
			return nil, xerrors.Errorf("%s answered HTTP status 404: %w", url, errNotPublished)
		case status != http.StatusOK:
			lastErr = xerrors.Errorf("HTTP status %d", status)
		default:
			routes := parseLiveList(body, segment)
			if len(routes) == 0 {
				return nil, xerrors.Errorf("%s lists no pages under %s%s", url, liveListPathPrefix, segment)
			}
			return routes, nil
		}
	}
	return nil, xerrors.Errorf("cannot read %s after %d attempts: %w", url, fetchAttempts, lastErr)
}

// inventory is the set of routes that must keep working, each with where it
// came from.
type inventory map[string][]string

func (inv inventory) add(route, origin string) {
	if route == "" {
		// The docs root always exists.
		return
	}
	if !slices.Contains(inv[route], origin) {
		inv[route] = append(inv[route], origin)
	}
}

// baseRuleSources returns the route each rule of a base branch's
// docs/redirects.json answers for: exact sources as written, and the prefix of
// a wildcard. It is lenient on purpose, because the base file may predate the
// current format. It reads the old absolute form, and skips what it cannot
// read.
func baseRuleSources(data []byte) []string {
	var entries []struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	var routes []string
	for _, e := range entries {
		route := strings.TrimSuffix(convertLegacy(e.Source), "/*")
		if checkRoute(route) == "" {
			routes = append(routes, route)
		}
	}
	return routes
}

// resolver decides whether a route still works in the head branch.
type resolver struct {
	live routeSet
	// hyphenated holds the live routes with "." replaced by "-".
	hyphenated map[string]bool
	rules      []rule
	// added holds the routes that are live in head and were not live in the
	// base branch, which are the likely new homes of a renamed page. It is
	// empty when there is no base manifest to compare with.
	added []string
}

// newResolver builds a resolver for the head branch. baseLive is the base
// branch's live routes, or nil when they are not known.
func newResolver(live, baseLive routeSet, rules []rule) resolver {
	r := resolver{live: live, hyphenated: live.hyphenated(), rules: rules}
	if baseLive != nil {
		for route := range live {
			if route != "" && !baseLive.has(route) {
				r.added = append(r.added, route)
			}
		}
		slices.Sort(r.added)
	}
	return r
}

// isLive reports whether a route is a page of the head branch. beta-docs
// serves dotted segments with hyphens ("esr-2.24-2.29-upgrade" is served as
// "esr-2-24-2-29-upgrade"), so a route that equals a head route with "." replaced
// by "-" counts as present until the docs engine is fixed.
func (r resolver) isLive(route string) bool {
	return r.live.has(route) || r.hyphenated[route]
}

// resolves reports whether a request for route ends on a page: it is a live
// route, or the head rules send it, within maxRedirectHops, to a live route,
// the docs root, or an allowed external URL. Every redirect counts as a hop,
// whatever its destination.
func (r resolver) resolves(route string) bool {
	route = normalizeRoute(route)
	for hops := 0; ; hops++ {
		if r.isLive(route) {
			return true
		}
		i := slices.IndexFunc(r.rules, func(rl rule) bool { return rl.matches(route) })
		if i < 0 || !r.rules[i].dst.valid || hops == maxRedirectHops {
			return false
		}
		kind, next := r.rules[i].target(route)
		if kind != destRoute {
			return true
		}
		route = next
	}
}

// suggest picks a destination for a rule that restores route. In order of
// preference it is the one live page with the same last segment, the one route
// added by this change in the same parent section (a rename such as
// "admin/integrations/istio" to "admin/integrations/istio-mesh"), or the docs
// root when no page is an obvious home.
func (r resolver) suggest(route string) string {
	last := func(s string) string {
		s = strings.ReplaceAll(s, ".", "-")
		return s[strings.LastIndex(s, "/")+1:]
	}
	var found []string
	for candidate := range r.live {
		if candidate != "" && last(candidate) == last(route) {
			found = append(found, candidate)
		}
	}
	if len(found) == 1 {
		return found[0]
	}

	parent := func(s string) string {
		i := strings.LastIndex(s, "/")
		if i < 0 {
			return ""
		}
		return s[:i]
	}
	found = found[:0]
	for _, candidate := range r.added {
		if parent(candidate) == parent(route) {
			found = append(found, candidate)
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	return "/"
}

// missingRoute is an inventory route that no longer works.
type missingRoute struct {
	route      string
	origins    []string
	suggestion string
}

func (m missingRoute) rule() string {
	return ruleJSON(m.route, m.suggestion, nil)
}

func (m missingRoute) message() string {
	return fmt.Sprintf(
		"docs route %q no longer resolves (from %s). Add a rule to docs/redirects.json, for example: %s",
		m.route, strings.Join(m.origins, ", "), m.rule(),
	)
}

// missingRoutes returns the inventory routes the head branch no longer serves
// or redirects, sorted by route.
func missingRoutes(inv inventory, r resolver) []missingRoute {
	var missing []missingRoute
	for route, origins := range inv {
		if r.resolves(route) {
			continue
		}
		missing = append(missing, missingRoute{route: route, origins: origins, suggestion: r.suggest(route)})
	}
	slices.SortFunc(missing, func(a, b missingRoute) int { return strings.Compare(a.route, b.route) })
	return missing
}

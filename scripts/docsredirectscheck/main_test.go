package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// liveOf builds a live-route set the way liveRoutes would for a manifest
// whose pages are the given routes. "/docs" (the landing page) is always live.
func liveOf(routes ...string) map[string]bool {
	m := map[string]bool{"/docs": true}
	for _, r := range routes {
		m[r] = true
	}
	return m
}

type wantProblem struct {
	index    int
	contains string
}

// requireProblems asserts that got holds exactly the wanted problems, in any
// order, matching each wanted problem to a distinct reported one by rule index
// and message substring.
func requireProblems(t *testing.T, got []problem, want ...wantProblem) {
	t.Helper()

	var msgs []string
	for _, p := range got {
		msgs = append(msgs, formatProblem("docs/redirects.json", p))
	}
	require.Len(t, got, len(want), "unexpected problems:\n%s", strings.Join(msgs, "\n"))

	used := make([]bool, len(got))
	for _, w := range want {
		found := false
		for i, p := range got {
			if used[i] || p.index != w.index || !strings.Contains(p.message, w.contains) {
				continue
			}
			used[i] = true
			found = true
			break
		}
		require.True(t, found, "no problem for rule %d containing %q in:\n%s", w.index, w.contains, strings.Join(msgs, "\n"))
	}
}

func TestConvertPathToRoute(t *testing.T) {
	t.Parallel()

	// These mirror convertPathToRoute in the docs website, including its
	// quirks: the README.md, index.md and .md suffixes are stripped one after
	// another with a plain suffix match, not per path segment.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain page", "./admin/external-auth.md", "admin/external-auth"},
		{"no leading dot slash", "admin/external-auth.md", "admin/external-auth"},
		{"index page", "./admin/templates/index.md", "admin/templates"},
		{"README page", "./ai-coder/README.md", "ai-coder"},
		{"root README", "./README.md", ""},
		{"deeply nested index", "./a/b/c/d/index.md", "a/b/c/d"},
		{"leading and trailing slashes trimmed", "/admin/users/", "admin/users"},
		{"non-markdown path is left alone", "./images/logo.svg", "images/logo.svg"},
		{"file whose name merely ends in index.md", "./install/my-index.md", "install/my-"},
		{"file whose name merely ends in README.md", "./install/OLD-README.md", "install/OLD-"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, convertPathToRoute(tc.in))
		})
	}
}

func TestDocsPagePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"empty path has no page", "", "", false},
		{"root README is the landing page", "./README.md", "/docs", true},
		{"plain page", "./about.md", "/docs/about", true},
		{"nested index page", "./admin/templates/index.md", "/docs/admin/templates", true},
		{"no leading dot slash", "admin/users.md", "/docs/admin/users", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := docsPagePath(tc.in)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLiveRoutes(t *testing.T) {
	t.Parallel()

	m, err := parseManifest([]byte(`{
		"versions": ["main"],
		"routes": [
			{"title": "About", "path": "./README.md", "children": [
				{"title": "Architecture", "path": "./about/architecture.md"}
			]},
			{"title": "Admin", "path": "./admin/index.md", "children": [
				{"title": "Users", "path": "admin/users.md"},
				{"title": "Section heading without a page"},
				{"title": "Deep", "path": "./admin/a/b.md", "children": [
					{"title": "Deeper", "path": "./admin/a/b/c/index.md"}
				]}
			]}
		]
	}`))
	require.NoError(t, err)

	got := liveRoutes(m)
	for _, want := range []string{
		"/docs",
		// The root About page is also served at /docs/about.
		"/docs/about",
		"/docs/about/architecture",
		"/docs/admin",
		"/docs/admin/users",
		"/docs/admin/a/b",
		"/docs/admin/a/b/c",
	} {
		require.True(t, got[want], "expected %s to be live; got %v", want, got)
	}
	require.False(t, got["/docs/section-heading-without-a-page"])
	require.Len(t, got, 7)
}

func TestLiveRoutesAboutSpecialCaseOnlyForRootAbout(t *testing.T) {
	t.Parallel()

	m, err := parseManifest([]byte(`{"routes": [{"title": "Overview", "path": "./README.md"}]}`))
	require.NoError(t, err)
	got := liveRoutes(m)
	require.True(t, got["/docs"])
	require.False(t, got["/docs/about"], "only a root page titled About is also served at /docs/about")
}

func TestParseManifestErrors(t *testing.T) {
	t.Parallel()

	for name, in := range map[string]string{
		"not json":      "nope",
		"no routes key": `{"versions": []}`,
		"routes object": `{"routes": {}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseManifest([]byte(in))
			require.Error(t, err)
		})
	}
}

func TestCheckRedirectsStructure(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/admin/users", "/docs/admin/groups")

	cases := []struct {
		name string
		in   string
		want []wantProblem
	}{
		{"empty array is valid", `[]`, nil},
		{"whitespace and newline only array", "[\n]\n", nil},
		{
			"valid rule",
			`[{"source": "/docs/old", "destination": "/docs/admin/users", "permanent": true}]`,
			nil,
		},
		{
			"permanent is optional",
			`[{"source": "/docs/old", "destination": "/docs/admin/users"}]`,
			nil,
		},
		{"not json", `{oops`, []wantProblem{{-1, "not valid JSON"}}},
		{"empty file", ``, []wantProblem{{-1, "not valid JSON"}}},
		{"object instead of array", `{"source": "/docs/old"}`, []wantProblem{{-1, "must be a JSON array"}}},
		{"null", `null`, []wantProblem{{-1, "must be a JSON array"}}},
		{"entry is not an object", `[1]`, []wantProblem{{0, "must be an object"}}},
		{"entry is a string", `["/docs/old"]`, []wantProblem{{0, "must be an object"}}},
		{
			"missing source",
			`[{"destination": "/docs/admin/users"}]`,
			[]wantProblem{{0, `"source" is required`}},
		},
		{
			"empty destination",
			`[{"source": "/docs/old", "destination": ""}]`,
			[]wantProblem{{0, `"destination" is required`}},
		},
		{
			"non-string source",
			`[{"source": 7, "destination": "/docs/admin/users"}]`,
			[]wantProblem{{0, `"source" is required`}},
		},
		{
			"permanent must be boolean",
			`[{"source": "/docs/old", "destination": "/docs/admin/users", "permanent": "yes"}]`,
			[]wantProblem{{0, `"permanent" must be a boolean`}},
		},
		{
			"unknown field",
			`[{"source": "/docs/old", "destination": "/docs/admin/users", "statusCode": 301}]`,
			[]wantProblem{{0, `unknown field "statusCode"`}},
		},
		{
			"problems are reported for every bad entry",
			`[
				{"source": "/docs/old", "destination": "/docs/admin/users"},
				5,
				{"source": "/docs/older", "destination": "/docs/admin/groups", "permanent": 1}
			]`,
			[]wantProblem{{1, "must be an object"}, {2, `"permanent" must be a boolean`}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got := checkRedirects([]byte(tc.in), live)
			requireProblems(t, got, tc.want...)
		})
	}
}

func TestCheckRedirectsSourceForms(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/target")

	cases := []struct {
		name   string
		source string
		want   []wantProblem
	}{
		{"exact", "/docs/some/old-page", nil},
		{"exact with underscore and digits", "/docs/security/0001_user_apikeys_invalidation", nil},
		{"exact with uppercase and dot", "/docs/Some/Old.Page", nil},
		{"trailing wildcard", "/docs/tutorials/ai-agents/:path*", nil},
		{"trailing regex wildcard", "/docs/coder-oss/latest/:slug(.*)", nil},
		{"trailing slash is normalized", "/docs/some/old-page/", nil},
		{"not under /docs", "/agents", []wantProblem{{0, "source must start with /docs/"}}},
		{"bare /docs", "/docs", []wantProblem{{0, "source must start with /docs/"}}},
		{"only /docs/", "/docs/", []wantProblem{{0, "source must start with /docs/"}}},
		{"relative", "docs/old", []wantProblem{{0, "source must start with /docs/"}}},
		{"bare parameter without modifier", "/docs/old/:slug", []wantProblem{{0, "unsupported"}}},
		{"parameter in the middle", "/docs/:slug*/old", []wantProblem{{0, "unsupported"}}},
		{"version regex", `/docs/@:version(v2\.33\.[0-9]+)/old`, []wantProblem{{0, "unsupported"}}},
		{"star on its own", "/docs/old/*", []wantProblem{{0, "unsupported"}}},
		{"query string", "/docs/old?x=1", []wantProblem{{0, "unsupported"}}},
		{"fragment", "/docs/old#section", []wantProblem{{0, "unsupported"}}},
		{"group", "/docs/(old|older)", []wantProblem{{0, "unsupported"}}},
		{"version segment", "/docs/@main/old", []wantProblem{{0, "version segment"}}},
		{"empty segment", "/docs//old", []wantProblem{{0, "unsupported"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := `[{"source": ` + quote(tc.source) + `, "destination": "/docs/target"}]`
			_, got := checkRedirects([]byte(data), live)
			requireProblems(t, got, tc.want...)
		})
	}
}

func TestCheckRedirectsDestinations(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/admin/users", "/docs/ai-coder/agents", "/docs/ai-coder/agents/tools")

	cases := []struct {
		name        string
		source      string
		destination string
		want        []wantProblem
	}{
		{"live page", "/docs/old", "/docs/admin/users", nil},
		{"landing page", "/docs/old", "/docs", nil},
		{"landing page with trailing slash", "/docs/old", "/docs/", nil},
		{"fragment is ignored", "/docs/old", "/docs/admin/users#sso", nil},
		{"query is ignored", "/docs/old", "/docs/admin/users?tab=1", nil},
		{"trailing slash is ignored", "/docs/old", "/docs/admin/users/", nil},
		{"external https", "/docs/old", "https://example.com/some/page", nil},
		{"external http", "/docs/old", "http://example.com/", nil},
		{
			"missing page",
			"/docs/old", "/docs/admin/nope",
			[]wantProblem{{0, `destination "/docs/admin/nope" is not a page`}},
		},
		{
			"destination is a section prefix but not a page",
			"/docs/old", "/docs/ai-coder",
			[]wantProblem{{0, "is not a page"}},
		},
		{
			"relative destination",
			"/docs/old", "admin/users",
			[]wantProblem{{0, "destination must be a /docs/ path or an http(s) URL"}},
		},
		{
			"protocol-relative destination",
			"/docs/old", "//evil.example.com/x",
			[]wantProblem{{0, "destination must be a /docs/ path or an http(s) URL"}},
		},
		{
			"non-http scheme",
			"/docs/old", "javascript:alert(1)",
			[]wantProblem{{0, "destination must be a /docs/ path or an http(s) URL"}},
		},
		{
			"external URL without a host",
			"/docs/old", "https://",
			[]wantProblem{{0, "invalid URL"}},
		},
		{
			"destination with a version segment",
			"/docs/old", "/docs/@main/admin/users",
			[]wantProblem{{0, "version segment"}},
		},
		{
			"pattern destination under an existing section",
			"/docs/tutorials/ai-agents/:path*", "/docs/ai-coder/:path*",
			nil,
		},
		{
			"pattern destination at the docs root",
			"/docs/coder-oss/latest/:slug(.*)", "/docs/:slug",
			nil,
		},
		{
			"pattern destination whose section does not exist",
			"/docs/old/:path*", "/docs/nonexistent/:path*",
			[]wantProblem{{0, `no docs page exists under "/docs/nonexistent"`}},
		},
		{
			"destination parameter the source does not define",
			"/docs/old/:path*", "/docs/ai-coder/:rest*",
			[]wantProblem{{0, "does not define"}},
		},
		{
			"exact source with parameter destination",
			"/docs/old", "/docs/ai-coder/:path*",
			[]wantProblem{{0, "does not define"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := `[{"source": ` + quote(tc.source) + `, "destination": ` + quote(tc.destination) + `}]`
			_, got := checkRedirects([]byte(data), live)
			requireProblems(t, got, tc.want...)
		})
	}
}

func TestCheckRedirectsShadowing(t *testing.T) {
	t.Parallel()

	live := liveOf(
		"/docs/admin/users",
		"/docs/admin/users/sso",
		"/docs/ai-coder/agents",
		"/docs/target",
	)

	cases := []struct {
		name   string
		source string
		want   []wantProblem
	}{
		{"exact source is a live page", "/docs/admin/users", []wantProblem{{0, "hides a live docs page"}}},
		{"exact source with trailing slash is still live", "/docs/admin/users/", []wantProblem{{0, "hides a live docs page"}}},
		{"exact source that is not live", "/docs/admin/old-users", nil},
		{"wildcard whose prefix is a live page", "/docs/admin/users/:path*", []wantProblem{{0, "hides a live docs page"}}},
		{"wildcard above live pages", "/docs/admin/:path*", []wantProblem{{0, "hides a live docs page"}}},
		{"regex wildcard above live pages", "/docs/admin/users/:slug(.*)", []wantProblem{{0, "hides a live docs page"}}},
		{"wildcard over a prefix with no live pages", "/docs/retired/:path*", nil},
		{"regex wildcard does not match its bare prefix", "/docs/ai-coder/agents/:slug(.*)", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := `[{"source": ` + quote(tc.source) + `, "destination": "/docs/target"}]`
			_, got := checkRedirects([]byte(data), live)
			requireProblems(t, got, tc.want...)
		})
	}
}

func TestCheckRedirectsOrder(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/a", "/docs/b")

	cases := []struct {
		name string
		in   string
		want []wantProblem
	}{
		{
			"a wildcard above a rule in its subtree swallows it",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/retired/page", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, `unreachable: rule [0] (source "/docs/retired/:path*")`}},
		},
		{
			"the same two rules are fine most specific first",
			`[
				{"source": "/docs/retired/page", "destination": "/docs/b"},
				{"source": "/docs/retired/:path*", "destination": "/docs/a"}
			]`,
			nil,
		},
		{
			"a regex wildcard above a rule beneath it swallows it",
			`[
				{"source": "/docs/retired/:path(.*)", "destination": "/docs/a"},
				{"source": "/docs/retired/page", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, "unreachable: rule [0]"}},
		},
		{
			"a wildcard above a nested wildcard swallows it",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/retired/sub/:path*", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, "unreachable: rule [0]"}},
		},
		{
			"a wildcard swallows a regex wildcard on the same prefix",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/retired/:other(.*)", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, "unreachable: rule [0]"}},
		},
		{
			"a regex wildcard leaves the bare prefix to an exact rule below it",
			`[
				{"source": "/docs/retired/:path(.*)", "destination": "/docs/a"},
				{"source": "/docs/retired", "destination": "/docs/b"}
			]`,
			nil,
		},
		{
			"an exact rule above a wildcard on the same prefix is deliberate",
			`[
				{"source": "/docs/retired", "destination": "/docs/b"},
				{"source": "/docs/retired/:path*", "destination": "/docs/a"}
			]`,
			nil,
		},
		{
			"sibling prefixes do not cover each other",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/gone/:path*", "destination": "/docs/b"}
			]`,
			nil,
		},
		{
			"a shared path prefix that is not a path boundary does not cover",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/retired-too/page", "destination": "/docs/b"}
			]`,
			nil,
		},
		{
			"each unreachable rule is reported once, against the first coverer",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/retired/sub/:path*", "destination": "/docs/b"},
				{"source": "/docs/retired/sub/page", "destination": "/docs/b"}
			]`,
			[]wantProblem{
				{1, `rule [0] (source "/docs/retired/:path*")`},
				{2, `rule [0] (source "/docs/retired/:path*")`},
			},
		},
		{
			"a duplicate source is reported as a duplicate, not as unreachable",
			`[
				{"source": "/docs/retired/:path*", "destination": "/docs/a"},
				{"source": "/docs/retired/:other*", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, "duplicate of rule [0]"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got := checkRedirects([]byte(tc.in), live)
			requireProblems(t, got, tc.want...)
		})
	}
}

func TestCheckRedirectsDuplicatesChainsAndLoops(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/a", "/docs/b", "/docs/c", "/docs/final")

	cases := []struct {
		name string
		in   string
		want []wantProblem
	}{
		{
			"independent rules are fine",
			`[
				{"source": "/docs/old1", "destination": "/docs/a"},
				{"source": "/docs/old2", "destination": "/docs/b"}
			]`,
			nil,
		},
		{
			"two rules may share a destination",
			`[
				{"source": "/docs/old1", "destination": "/docs/final"},
				{"source": "/docs/old2", "destination": "/docs/final"}
			]`,
			nil,
		},
		{
			"duplicate exact source",
			`[
				{"source": "/docs/old", "destination": "/docs/a"},
				{"source": "/docs/old", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, "duplicate of rule [0]"}},
		},
		{
			"duplicate source differing only by trailing slash",
			`[
				{"source": "/docs/old", "destination": "/docs/a"},
				{"source": "/docs/old/", "destination": "/docs/b"}
			]`,
			[]wantProblem{{1, "duplicate of rule [0]"}},
		},
		{
			"duplicate pattern source",
			`[
				{"source": "/docs/old/:path*", "destination": "/docs/a/:path*"},
				{"source": "/docs/old/:path*", "destination": "/docs/b/:path*"}
			]`,
			[]wantProblem{{1, "duplicate of rule [0]"}},
		},
		{
			"exact and pattern with the same prefix are not duplicates",
			`[
				{"source": "/docs/old", "destination": "/docs/a"},
				{"source": "/docs/old/:slug(.*)", "destination": "/docs/:slug"}
			]`,
			nil,
		},
		{
			"redirect to itself",
			`[{"source": "/docs/old", "destination": "/docs/old"}]`,
			[]wantProblem{{0, "redirects to itself"}, {0, "is not a page"}},
		},
		{
			"chain through an exact source",
			`[
				{"source": "/docs/old", "destination": "/docs/older"},
				{"source": "/docs/older", "destination": "/docs/final"}
			]`,
			[]wantProblem{{0, "redirect chain"}, {0, "is not a page"}},
		},
		{
			"chain through a wildcard source",
			`[
				{"source": "/docs/old", "destination": "/docs/retired/page"},
				{"source": "/docs/retired/:path*", "destination": "/docs/final"}
			]`,
			[]wantProblem{{0, "redirect chain"}, {0, "is not a page"}},
		},
		{
			"every rule that redirects into another rule is reported",
			`[
				{"source": "/docs/r1", "destination": "/docs/r2"},
				{"source": "/docs/r2", "destination": "/docs/r3"},
				{"source": "/docs/r3", "destination": "/docs/final"}
			]`,
			[]wantProblem{
				{0, "redirect chain"},
				{0, "is not a page"},
				{1, "redirect chain"},
				{1, "is not a page"},
			},
		},
		{
			"two-rule loop",
			`[
				{"source": "/docs/x", "destination": "/docs/y"},
				{"source": "/docs/y", "destination": "/docs/x"}
			]`,
			[]wantProblem{
				{0, "redirect loop"},
				{0, "is not a page"},
				{1, "is not a page"},
			},
		},
		{
			"three-rule loop",
			`[
				{"source": "/docs/x", "destination": "/docs/y"},
				{"source": "/docs/y", "destination": "/docs/z"},
				{"source": "/docs/z", "destination": "/docs/x"}
			]`,
			[]wantProblem{
				{0, "redirect loop"},
				{0, "is not a page"},
				{1, "is not a page"},
				{2, "is not a page"},
			},
		},
		{
			"external destinations never chain",
			`[
				{"source": "/docs/old", "destination": "https://example.com/docs/older"},
				{"source": "/docs/older", "destination": "/docs/final"}
			]`,
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got := checkRedirects([]byte(tc.in), live)
			requireProblems(t, got, tc.want...)
		})
	}
}

func TestChainMessageNamesTheFinalPage(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/final")
	_, got := checkRedirects([]byte(`[
		{"source": "/docs/old", "destination": "/docs/older"},
		{"source": "/docs/older", "destination": "/docs/final"}
	]`), live)

	var chain *problem
	for i := range got {
		if strings.Contains(got[i].message, "redirect chain") {
			chain = &got[i]
		}
	}
	require.NotNil(t, chain, "expected a chain problem, got %v", got)
	require.Equal(t, 0, chain.index)
	require.Contains(t, chain.message, "/docs/older")
	require.Contains(t, chain.message, "/docs/final", "the message should tell the author where to point the rule instead")
}

func TestFormatProblem(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		`docs/redirects.json: must be a JSON array of rules`,
		formatProblem("docs/redirects.json", problem{index: -1, message: "must be a JSON array of rules"}),
	)
	require.Equal(t,
		`docs/redirects.json[3] (source "/docs/x"): bad`,
		formatProblem("docs/redirects.json", problem{index: 3, source: "/docs/x", message: "bad"}),
	)
	require.Equal(t,
		`docs/redirects.json[3]: bad`,
		formatProblem("docs/redirects.json", problem{index: 3, message: "bad"}),
	)
}

func TestRuleMatches(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/target")
	rules, problems := checkRedirects([]byte(`[
		{"source": "/docs/exact", "destination": "/docs/target"},
		{"source": "/docs/tail/:path*", "destination": "/docs/target"},
		{"source": "/docs/rx/:slug(.*)", "destination": "/docs/target"}
	]`), live)
	require.Empty(t, problems)
	require.Len(t, rules, 3)

	cases := []struct {
		rule int
		path string
		want bool
	}{
		{0, "/docs/exact", true},
		{0, "/docs/exact/", true},
		{0, "/docs/exact/child", false},
		{1, "/docs/tail", true},
		{1, "/docs/tail/a/b", true},
		{1, "/docs/tailored", false},
		{2, "/docs/rx", false},
		{2, "/docs/rx/a", true},
		{2, "/docs/rx/a/b", true},
		{2, "/docs/rxa", false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, rules[tc.rule].matches(tc.path), "rule %d vs %s", tc.rule, tc.path)
	}
}

func TestRemovedRoutes(t *testing.T) {
	t.Parallel()

	base := liveOf("/docs/a", "/docs/b", "/docs/c")
	head := liveOf("/docs/a", "/docs/c", "/docs/d")
	require.Equal(t, []string{"/docs/b"}, removedRoutes(base, head))

	// A file rename that keeps the public URL (index.md to a flat file) does
	// not change the route set, so it is not a removal.
	require.Empty(t, removedRoutes(base, liveOf("/docs/a", "/docs/b", "/docs/c")))
	require.Empty(t, removedRoutes(nil, head))
}

func TestUncoveredRoutes(t *testing.T) {
	t.Parallel()

	live := liveOf("/docs/target")
	rules, problems := checkRedirects([]byte(`[
		{"source": "/docs/moved", "destination": "/docs/target"},
		{"source": "/docs/tutorials/:path*", "destination": "/docs/target"}
	]`), live)
	require.Empty(t, problems)

	removed := []string{"/docs/moved", "/docs/tutorials/intro", "/docs/gone"}
	require.Equal(t, []string{"/docs/gone"}, uncoveredRoutes(removed, rules))
	require.Equal(t, removed, uncoveredRoutes(removed, nil))
}

// ---- run (CLI behavior) ----

type fakeGit struct {
	revs    map[string]bool
	files   map[string]string // "rev:path" -> contents
	changed map[string]bool   // "rev:path" -> changed since rev
	showErr error
}

func (f *fakeGit) HasRev(rev string) bool { return f.revs[rev] }

func (f *fakeGit) Show(rev, path string) ([]byte, error) {
	if f.showErr != nil {
		return nil, f.showErr
	}
	s, ok := f.files[rev+":"+path]
	if !ok {
		return nil, xerrors.New("not in revision")
	}
	return []byte(s), nil
}

func (f *fakeGit) Changed(rev, path string) (bool, error) {
	return f.changed[rev+":"+path], nil
}

const manifestTwoPages = `{"routes": [
	{"title": "About", "path": "./README.md"},
	{"title": "Users", "path": "./admin/users.md"},
	{"title": "Groups", "path": "./admin/groups.md"}
]}`

const manifestOnePage = `{"routes": [
	{"title": "About", "path": "./README.md"},
	{"title": "Users", "path": "./admin/users.md"}
]}`

type runEnv struct {
	dir           string
	manifestPath  string
	redirectsPath string
	stdout        bytes.Buffer
	stderr        bytes.Buffer
}

func newRunEnv(t *testing.T, manifest, redirects string) *runEnv {
	t.Helper()
	dir := t.TempDir()
	e := &runEnv{
		dir:           dir,
		manifestPath:  filepath.Join(dir, "manifest.json"),
		redirectsPath: filepath.Join(dir, "redirects.json"),
	}
	if manifest != "" {
		require.NoError(t, os.WriteFile(e.manifestPath, []byte(manifest), 0o600))
	}
	if redirects != "" {
		require.NoError(t, os.WriteFile(e.redirectsPath, []byte(redirects), 0o600))
	}
	return e
}

func (e *runEnv) run(base string, git gitReader, env map[string]string) int {
	return run(options{
		manifestPath:  e.manifestPath,
		redirectsPath: e.redirectsPath,
		base:          base,
		git:           git,
		getenv:        func(k string) string { return env[k] },
		stdout:        &e.stdout,
		stderr:        &e.stderr,
	})
}

func TestRunAbsentRedirectsFilePasses(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestTwoPages, "")
	require.Equal(t, 0, e.run("", &fakeGit{}, nil))
	require.Contains(t, e.stdout.String(), "not found")
	require.Empty(t, e.stderr.String())
}

func TestRunValidRedirectsPasses(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestTwoPages, `[{"source": "/docs/old", "destination": "/docs/admin/users"}]`)
	require.Equal(t, 0, e.run("", &fakeGit{}, nil))
	require.Empty(t, e.stderr.String())
	require.Contains(t, e.stdout.String(), "1 rule")
}

func TestRunInvalidRedirectsFailsWithActionableMessages(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestTwoPages, `[
		{"source": "/docs/old", "destination": "/docs/admin/nope"},
		{"source": "/docs/admin/users", "destination": "/docs/admin/groups"}
	]`)
	require.Equal(t, 1, e.run("", &fakeGit{}, nil))
	out := e.stderr.String()
	require.Contains(t, out, `[0] (source "/docs/old")`)
	require.Contains(t, out, `"/docs/admin/nope" is not a page`)
	require.Contains(t, out, `[1] (source "/docs/admin/users")`)
	require.Contains(t, out, "hides a live docs page")
}

func TestRunMissingManifestIsAnError(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, "", `[]`)
	require.Equal(t, 2, e.run("", &fakeGit{}, nil))
	require.Contains(t, e.stderr.String(), "manifest")
}

func TestRunInvalidManifestIsAnError(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, `{"routes": "nope"}`, `[]`)
	require.Equal(t, 2, e.run("", &fakeGit{}, nil))
	require.Contains(t, e.stderr.String(), "manifest")
}

func TestRunUnreadableRedirectsFileIsAnError(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestTwoPages, "")
	// A directory where the file should be: present but unreadable as a file.
	require.NoError(t, os.Mkdir(e.redirectsPath, 0o700))
	require.Equal(t, 2, e.run("", &fakeGit{}, nil))
}

func advisoryGit(redirectsChanged bool) *fakeGit {
	return &fakeGit{
		revs:    map[string]bool{"HEAD^1": true, "abc123": true},
		files:   map[string]string{"HEAD^1:docs/manifest.json": manifestTwoPages},
		changed: map[string]bool{"HEAD^1:docs/redirects.json": redirectsChanged},
	}
}

func TestRunAdvisoryWarnsWhenRoutesRemovedWithoutRedirects(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestOnePage, "")
	code := e.run("", advisoryGit(false), map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code, "the advisory check must never fail the run")
	out := e.stdout.String() + e.stderr.String()
	require.Contains(t, out, "/docs/admin/groups")
	require.Contains(t, out, "docs/redirects.json")
}

func TestRunAdvisoryUsesGitHubAnnotationsInActions(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestOnePage, "")
	code := e.run("", advisoryGit(false), map[string]string{
		"GITHUB_BASE_REF": "main",
		"GITHUB_ACTIONS":  "true",
	})
	require.Equal(t, 0, code)
	var annotation string
	for _, line := range strings.Split(e.stdout.String()+e.stderr.String(), "\n") {
		if strings.HasPrefix(line, "::warning ") {
			annotation = line
		}
	}
	require.NotEmpty(t, annotation, "expected a ::warning annotation")
	// A workflow command ends at the newline, so the whole message has to be on
	// the same line.
	require.Contains(t, annotation, "file=docs/manifest.json")
	require.Contains(t, annotation, "/docs/admin/groups")
	require.Contains(t, annotation, "docs/redirects.json")
}

func TestRunAdvisoryQuietWhenRedirectsFileChanged(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestOnePage, `[{"source": "/docs/admin/groups", "destination": "/docs/admin/users"}]`)
	code := e.run("", advisoryGit(true), map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code)
	require.NotContains(t, e.stdout.String()+e.stderr.String(), "warning")
}

func TestRunAdvisoryQuietWhenRedirectsFileChangedForAnotherRule(t *testing.T) {
	t.Parallel()

	// The change edits docs/redirects.json but no rule covers the removed
	// route. The author is already working with redirects in this change, so
	// the advisory stays quiet; it only nudges changes that never touch the
	// file. Without the "file changed" check this would warn.
	e := newRunEnv(t, manifestOnePage, `[{"source": "/docs/unrelated", "destination": "/docs/admin/users"}]`)
	code := e.run("", advisoryGit(true), map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code)
	require.NotContains(t, e.stdout.String()+e.stderr.String(), "warning")
}

func TestRunAdvisoryQuietWhenRemovedRouteAlreadyRedirected(t *testing.T) {
	t.Parallel()

	// The redirects file did not change in this PR, but it already covers the
	// removed route.
	e := newRunEnv(t, manifestOnePage, `[{"source": "/docs/admin/groups", "destination": "/docs/admin/users"}]`)
	code := e.run("", advisoryGit(false), map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code)
	require.NotContains(t, e.stdout.String()+e.stderr.String(), "warning")
}

func TestRunAdvisoryQuietWhenNoRouteWasRemoved(t *testing.T) {
	t.Parallel()

	// Same routes before and after, for example a file rename that keeps its
	// URL.
	e := newRunEnv(t, manifestTwoPages, "")
	code := e.run("", advisoryGit(false), map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code)
	require.NotContains(t, e.stdout.String()+e.stderr.String(), "warning")
}

func TestRunAdvisorySkippedWithoutABase(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestOnePage, "")
	code := e.run("", advisoryGit(false), nil)
	require.Equal(t, 0, code)
	out := e.stdout.String() + e.stderr.String()
	require.NotContains(t, out, "::warning")
	require.Contains(t, out, "skipped")
}

func TestRunAdvisorySkippedWhenBaseRevisionIsMissing(t *testing.T) {
	t.Parallel()

	// A shallow checkout has no HEAD^1.
	g := advisoryGit(false)
	g.revs = map[string]bool{}
	e := newRunEnv(t, manifestOnePage, "")
	code := e.run("", g, map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code)
	out := e.stdout.String() + e.stderr.String()
	require.Contains(t, out, "skipped")
	require.NotContains(t, out, "::warning")
}

func TestRunAdvisorySkippedWhenBaseHasNoManifest(t *testing.T) {
	t.Parallel()

	g := advisoryGit(false)
	g.files = map[string]string{}
	e := newRunEnv(t, manifestOnePage, "")
	code := e.run("", g, map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 0, code)
	require.Contains(t, e.stdout.String()+e.stderr.String(), "skipped")
}

func TestRunAdvisoryHonorsExplicitBase(t *testing.T) {
	t.Parallel()

	g := advisoryGit(false)
	g.files = map[string]string{"abc123:docs/manifest.json": manifestTwoPages}
	g.changed = map[string]bool{}
	e := newRunEnv(t, manifestOnePage, "")
	code := e.run("abc123", g, nil)
	require.Equal(t, 0, code)
	require.Contains(t, e.stdout.String()+e.stderr.String(), "/docs/admin/groups")
}

func TestRunAdvisoryStillRunsWhenRedirectsFileIsInvalid(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestOnePage, `{oops`)
	code := e.run("", advisoryGit(false), map[string]string{"GITHUB_BASE_REF": "main"})
	require.Equal(t, 1, code)
	require.Contains(t, e.stderr.String(), "not valid JSON")
}

func TestExecGitRejectsOptionLikeRevisions(t *testing.T) {
	t.Parallel()

	// A value passed with -base must never be read by git as an option. These
	// return before any git process starts.
	g := execGit{}
	require.False(t, g.HasRev(""))
	require.False(t, g.HasRev("--output=somefile"))
	_, err := g.Show("--help", manifestRepoPath)
	require.Error(t, err)
	_, err = g.Changed("-p", redirectsRepoPath)
	require.Error(t, err)
}

// TestRepositoryManifest guards the route logic against the real manifest:
// it must parse, and well-known pages must come out as live routes.
func TestRepositoryManifest(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("docs/manifest.json not found relative to the test")
	}
	require.NoError(t, err)

	m, err := parseManifest(data)
	require.NoError(t, err)
	live := liveRoutes(m)
	require.Greater(t, len(live), 300)
	require.True(t, live["/docs"])
	require.True(t, live["/docs/about"])
	require.True(t, live["/docs/admin/external-auth"])
}

// TestRepositoryRedirectsFile runs the real check against the repository's
// own files, so a bad edit to docs/redirects.json fails `go test` as well as
// `make lint`. It passes when the file does not exist.
func TestRepositoryRedirectsFile(t *testing.T) {
	t.Parallel()

	manifest, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("docs/manifest.json not found relative to the test")
	}
	require.NoError(t, err)
	redirects, err := os.ReadFile(filepath.Join("..", "..", "docs", "redirects.json"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("docs/redirects.json does not exist yet")
	}
	require.NoError(t, err)

	m, err := parseManifest(manifest)
	require.NoError(t, err)
	_, problems := checkRedirects(redirects, liveRoutes(m))
	var msgs []string
	for _, p := range problems {
		msgs = append(msgs, formatProblem("docs/redirects.json", p))
	}
	require.Empty(t, problems, strings.Join(msgs, "\n"))
}

// quote renders a string as a JSON string literal for a test fixture. The
// fixtures are ASCII, where Go and JSON string escaping agree.
func quote(s string) string {
	return strconv.Quote(s)
}

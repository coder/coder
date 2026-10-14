package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// liveOf builds a live-route set the way liveRoutes would for a manifest whose
// pages are the given routes. The docs root is always live.
func liveOf(routes ...string) routeSet {
	m := routeSet{"": ""}
	for _, r := range routes {
		m[r] = "docs/" + r + ".md"
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

	for _, tt := range []struct{ in, want string }{
		{"./README.md", ""},
		{"./admin/README.md", "admin"},
		{"./ai-coder/index.md", "ai-coder"},
		{"./admin/external-auth.md", "admin/external-auth"},
		{"admin/users.md", "admin/users"},
		{"./my-index.md", "my-"},
		{"./admin/", "admin"},
		{"./install/esr-2.24-2.29-upgrade.md", "install/esr-2.24-2.29-upgrade"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, convertPathToRoute(tt.in))
		})
	}
}

func TestLiveRoutes(t *testing.T) {
	t.Parallel()

	m, err := parseManifest([]byte(`{"routes": [
		{"title": "About", "path": "./README.md", "children": [
			{"title": "Section"},
			{"title": "Users", "path": "./admin/users.md", "children": [
				{"title": "Groups", "path": "./admin/groups/index.md"}
			]}
		]}
	]}`))
	require.NoError(t, err)
	live := liveRoutes(m)
	require.Equal(t, routeSet{
		"":             "docs/README.md",
		"admin/users":  "docs/admin/users.md",
		"admin/groups": "docs/admin/groups/index.md",
	}, live)
	// The classic site's /about alias is not a route in the new format.
	require.False(t, live.has("about"))
}

func TestLiveRoutesDocsRootAlwaysExists(t *testing.T) {
	t.Parallel()

	m, err := parseManifest([]byte(`{"routes": []}`))
	require.NoError(t, err)
	require.True(t, liveRoutes(m).has(""))
}

func TestHyphenated(t *testing.T) {
	t.Parallel()

	got := liveOf("install/esr-2.24-2.29-upgrade", "admin/users").hyphenated()
	require.True(t, got["install/esr-2-24-2-29-upgrade"])
	require.True(t, got["admin/users"])
	require.False(t, got["install/esr-2.24-2.29-upgrade"])
}

func TestParseManifestErrors(t *testing.T) {
	t.Parallel()

	for name, data := range map[string]string{
		"not json":   "nope",
		"no routes":  `{}`,
		"wrong type": `{"routes": "x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseManifest([]byte(data))
			require.Error(t, err)
		})
	}
}

func TestParseSource(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in      string
		want    pattern
		wantMsg string
	}{
		{in: "admin/old", want: pattern{prefix: "admin/old"}},
		{in: "old", want: pattern{prefix: "old"}},
		{in: "admin/*", want: pattern{wild: true, prefix: "admin"}},
		{in: "a/b/c/*", want: pattern{wild: true, prefix: "a/b/c"}},
		{in: "install.sh", want: pattern{prefix: "install.sh"}},
		{in: "esr-2.24/up_grade", want: pattern{prefix: "esr-2.24/up_grade"}},
		{in: "/admin/old", wantMsg: "no leading slash"},
		{in: "admin/old/", wantMsg: "no trailing slash"},
		{in: "*", wantMsg: `only allowed as a trailing "/*"`},
		{in: "/*", wantMsg: "empty route"},
		{in: "admin/*/x", wantMsg: `only allowed as a trailing "/*"`},
		{in: "admin/x*", wantMsg: `only allowed as a trailing "/*"`},
		{in: "admin//old", wantMsg: "empty path segment"},
		{in: "admin/./old", wantMsg: `segment "." is not allowed`},
		{in: "admin/../old", wantMsg: `segment ".." is not allowed`},
		{in: "admin/@main/old", wantMsg: "must match"},
		{in: "admin/%20old", wantMsg: "must match"},
		{in: "admin/:path", wantMsg: "must match"},
		{in: "admin/(.*)", wantMsg: `only allowed as a trailing "/*"`},
		{in: "admin/old?x=1", wantMsg: "must match"},
		{in: "admin/old#x", wantMsg: "must match"},
		{in: "admin/old page", wantMsg: "must match"},
		{in: "admin/old.md", wantMsg: ".md"},
		{in: "", wantMsg: "empty route"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, msg := parseSource(tt.in)
			if tt.wantMsg == "" {
				require.Empty(t, msg)
				require.Equal(t, tt.want, got)
				return
			}
			require.Contains(t, msg, tt.wantMsg)
		})
	}
}

func TestParseDestination(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in      string
		want    destination
		wantMsg string
	}{
		{in: "/", want: destination{kind: destRoot, raw: "/", valid: true}},
		{in: "admin/users", want: destination{kind: destRoute, raw: "admin/users", prefix: "admin/users", valid: true}},
		{in: "admin/*", want: destination{kind: destRoute, raw: "admin/*", prefix: "admin", wild: true, valid: true}},
		{in: "https://github.com/coder/coder/blob/main/CONTRIBUTING.md", want: destination{kind: destExternal, raw: "https://github.com/coder/coder/blob/main/CONTRIBUTING.md", valid: true}},
		{in: "https://coder.com/blog/post#section", want: destination{kind: destExternal, raw: "https://coder.com/blog/post#section", valid: true}},
		{in: "admin/users#web-ides", want: destination{kind: destRoute, raw: "admin/users#web-ides", prefix: "admin/users", fragment: "web-ides", valid: true}},
		{in: "admin/users#a.b_C-1", want: destination{kind: destRoute, raw: "admin/users#a.b_C-1", prefix: "admin/users", fragment: "a.b_C-1", valid: true}},
		{in: "admin/users#", wantMsg: "fragment must match"},
		{in: "admin/users#a#b", wantMsg: "fragment must match"},
		{in: "admin/users#a b", wantMsg: "fragment must match"},
		{in: "admin/users#a%20b", wantMsg: "fragment must match"},
		{in: "admin/*#x", wantMsg: "do not take a fragment"},
		{in: "/#x", wantMsg: "do not take a fragment"},
		{in: "#x", wantMsg: "empty route"},
		{in: "https://github.com/coderx/foo", wantMsg: "must start with"},
		{in: "https://github.com/other/coder", wantMsg: "must start with"},
		{in: "https://evil.example/coder.com/", wantMsg: "must start with"},
		{in: "https://coder.com.evil.example/", wantMsg: "must start with"},
		{in: "http://coder.com/docs", wantMsg: "must be https://"},
		{in: "ftp://coder.com/docs", wantMsg: "must be https://"},
		{in: "https://coder.com:8443/x", wantMsg: "must start with"},
		{in: "https://coder.com/x/:path", wantMsg: `must not contain ":"`},
		{in: "https://coder.com/x/*", wantMsg: "wildcard"},
		{in: "https://coder.com/x y", wantMsg: "whitespace"},
		{in: "/admin/users", wantMsg: "no leading slash"},
		{in: "admin/users/", wantMsg: "no trailing slash"},
		{in: "*", wantMsg: `only allowed as a trailing "/*"`},
		{in: "admin/*/x", wantMsg: `only allowed as a trailing "/*"`},
		{in: "admin/:path*", wantMsg: `only allowed as a trailing "/*"`},
		{in: "admin/:path", wantMsg: "must match"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, msg := parseDestination(tt.in)
			if tt.wantMsg != "" {
				require.Contains(t, msg, tt.wantMsg)
				return
			}
			require.Empty(t, msg)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCheckRedirectsStructure(t *testing.T) {
	t.Parallel()

	live := liveOf("admin/users", "admin/groups", "ai-coder/ai-gateway")
	for _, tt := range []struct {
		name string
		data string
		want []wantProblem
	}{
		{name: "empty array", data: `[]`},
		{name: "valid exact", data: `[{"source": "old", "destination": "admin/users"}]`},
		{name: "valid permanent false", data: `[{"source": "old", "destination": "admin/users", "permanent": false}]`},
		{name: "valid wildcard", data: `[{"source": "old/*", "destination": "ai-coder/ai-gateway/*"}]`},
		{name: "wildcard to page", data: `[{"source": "old/*", "destination": "admin/users"}]`},
		{name: "wildcard to root", data: `[{"source": "old/*", "destination": "/"}]`},
		{name: "fragment on live page", data: `[{"source": "old", "destination": "admin/users#web-ides"}]`},
		{name: "fragment on missing page", data: `[{"source": "old", "destination": "admin/missing#web-ides"}]`, want: []wantProblem{{0, "not a page in docs/manifest.json"}}},
		{name: "fragment on wildcard", data: `[{"source": "old/*", "destination": "ai-coder/ai-gateway/*#x"}]`, want: []wantProblem{{0, "do not take a fragment"}}},
		{name: "exact to root", data: `[{"source": "old", "destination": "/"}]`},
		{name: "exact to external", data: `[{"source": "old", "destination": "https://github.com/coder/coder"}]`},
		{name: "not json", data: `nope`, want: []wantProblem{{-1, "not valid JSON"}}},
		{name: "object", data: `{}`, want: []wantProblem{{-1, "JSON array"}}},
		{name: "null", data: `null`, want: []wantProblem{{-1, "JSON array"}}},
		{name: "null entry", data: `[null]`, want: []wantProblem{{0, "must be an object"}}},
		{name: "scalar entry", data: `["x"]`, want: []wantProblem{{0, "must be an object"}}},
		{name: "unknown field", data: `[{"source": "old", "destination": "admin/users", "status": 301}]`, want: []wantProblem{{0, `unknown field "status"`}}},
		{name: "missing source", data: `[{"destination": "admin/users"}]`, want: []wantProblem{{0, `"source" is required`}}},
		{name: "missing destination", data: `[{"source": "old"}]`, want: []wantProblem{{0, `"destination" is required`}}},
		{name: "blank destination", data: `[{"source": "old", "destination": " "}]`, want: []wantProblem{{0, `"destination" is required`}}},
		{name: "permanent not bool", data: `[{"source": "old", "destination": "admin/users", "permanent": "no"}]`, want: []wantProblem{{0, "must be a boolean"}}},
		{name: "permanent null", data: `[{"source": "old", "destination": "admin/users", "permanent": null}]`, want: []wantProblem{{0, "must be a boolean"}}},
		{name: "bad source", data: `[{"source": "/old", "destination": "admin/users"}]`, want: []wantProblem{{0, "no leading slash"}}},
		{name: "bad destination", data: `[{"source": "old", "destination": "/admin/users"}]`, want: []wantProblem{{0, "no leading slash"}}},
		{name: "destination not a page", data: `[{"source": "old", "destination": "admin/missing"}]`, want: []wantProblem{{0, "not a page in docs/manifest.json"}}},
		{name: "wildcard destination needs wildcard source", data: `[{"source": "old", "destination": "ai-coder/ai-gateway/*"}]`, want: []wantProblem{{0, `source is not a wildcard`}}},
		{name: "wildcard destination into nothing", data: `[{"source": "old/*", "destination": "nowhere/*"}]`, want: []wantProblem{{0, "no docs page exists under"}}},
		{name: "shadows page", data: `[{"source": "admin/users", "destination": "admin/groups"}]`, want: []wantProblem{{0, "hides a live docs page: admin/users"}}},
		{name: "wildcard shadows pages", data: `[{"source": "admin/*", "destination": "/"}]`, want: []wantProblem{{0, "hides a live docs page: admin/groups (and 1 more)"}}},
		{name: "duplicate", data: `[{"source": "old", "destination": "admin/users"}, {"source": "old", "destination": "admin/groups"}]`, want: []wantProblem{{1, "duplicate of rule [0]"}}},
		{name: "unreachable after wildcard", data: `[{"source": "old/*", "destination": "/"}, {"source": "old/page", "destination": "admin/users"}]`, want: []wantProblem{{1, "unreachable"}}},
		{name: "exact before wildcard is fine", data: `[{"source": "old", "destination": "admin/users"}, {"source": "old/*", "destination": "/"}]`},
		{name: "wildcard before exact on same prefix", data: `[{"source": "old/*", "destination": "/"}, {"source": "old", "destination": "admin/users"}]`, want: []wantProblem{{1, "unreachable"}}},
		{name: "nested wildcard after parent", data: `[{"source": "old/*", "destination": "/"}, {"source": "old/deep/*", "destination": "admin/users"}]`, want: []wantProblem{{1, "unreachable"}}},
		{name: "nested wildcard before parent", data: `[{"source": "old/deep/*", "destination": "admin/users"}, {"source": "old/*", "destination": "/"}]`},
		{name: "chain", data: `[{"source": "a", "destination": "b"}, {"source": "b", "destination": "admin/users"}]`, want: []wantProblem{{0, "redirect chain"}, {0, "not a page"}}},
		{name: "chain through wildcard", data: `[{"source": "a", "destination": "b/x"}, {"source": "b/*", "destination": "admin/users"}]`, want: []wantProblem{{0, "redirect chain"}, {0, "not a page"}}},
		{name: "loop", data: `[{"source": "a", "destination": "b"}, {"source": "b", "destination": "a"}]`, want: []wantProblem{{0, "redirect loop"}, {0, "not a page"}, {1, "not a page"}}},
		{name: "CRLF", data: "[\r\n  {\r\n    \"source\": \"old\",\r\n    \"destination\": \"admin/users\"\r\n  }\r\n]\r\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.name == "loop" {
				// Loops need live destinations to be reported on their own.
				_, problems := checkRedirects([]byte(tt.data), liveOf("a", "b"))
				require.NotEmpty(t, problems)
				return
			}
			_, problems := checkRedirects([]byte(tt.data), live)
			requireProblems(t, problems, tt.want...)
		})
	}
}

func TestCheckRedirectsLegacyFormIsRejectedWithConvertedRule(t *testing.T) {
	t.Parallel()

	live := liveOf("admin/users", "ai-coder/ai-gateway")
	for _, tt := range []struct {
		name, data, want string
	}{
		{
			name: "exact",
			data: `[{"source": "/docs/old", "destination": "/docs/admin/users"}]`,
			want: `{"source": "old", "destination": "admin/users"}`,
		},
		{
			name: "named wildcard",
			data: `[{"source": "/docs/old/:path*", "destination": "/docs/ai-coder/ai-gateway/:path*"}]`,
			want: `{"source": "old/*", "destination": "ai-coder/ai-gateway/*"}`,
		},
		{
			name: "regex wildcard",
			data: `[{"source": "/docs/old/:slug(.*)", "destination": "/docs/ai-coder/ai-gateway/:slug"}]`,
			want: `{"source": "old/*", "destination": "ai-coder/ai-gateway/*"}`,
		},
		{
			name: "docs root destination",
			data: `[{"source": "/docs/about", "destination": "/docs"}]`,
			want: `{"source": "about", "destination": "/"}`,
		},
		{
			name: "external destination and permanent",
			data: `[{"source": "/docs/contributing", "destination": "https://github.com/coder/coder", "permanent": false}]`,
			want: `{"source": "contributing", "destination": "https://github.com/coder/coder", "permanent": false}`,
		},
		{
			name: "only the destination is legacy",
			data: `[{"source": "old", "destination": "/docs/admin/users"}]`,
			want: `{"source": "old", "destination": "admin/users"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rules, problems := checkRedirects([]byte(tt.data), live)
			require.Empty(t, rules)
			requireProblems(t, problems, wantProblem{0, "Converted rule: " + tt.want})
		})
	}
}

func TestRuleTarget(t *testing.T) {
	t.Parallel()

	mk := func(src, dst string) rule {
		s, msg := parseSource(src)
		require.Empty(t, msg)
		d, msg := parseDestination(dst)
		require.Empty(t, msg)
		return rule{src: s, dst: d}
	}
	for _, tt := range []struct {
		name      string
		rule      rule
		route     string
		wantKind  destKind
		wantRoute string
	}{
		{"exact", mk("old", "new"), "old", destRoute, "new"},
		{"wildcard substitution", mk("old/*", "new/*"), "old/a/b", destRoute, "new/a/b"},
		{"wildcard single segment", mk("old/*", "new/*"), "old/a", destRoute, "new/a"},
		{"wildcard empty match", mk("old/*", "new/*"), "old", destRoute, "new"},
		{"trailing slash ignored", mk("old/*", "new/*"), "old/a/", destRoute, "new/a"},
		{"wildcard to page", mk("old/*", "new"), "old/a/b", destRoute, "new"},
		{"to root", mk("old/*", "/"), "old/a", destRoot, ""},
		{"to external", mk("old", "https://coder.com/x"), "old", destExternal, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, tt.rule.matches(tt.route))
			kind, route := tt.rule.target(tt.route)
			require.Equal(t, tt.wantKind, kind)
			require.Equal(t, tt.wantRoute, route)
		})
	}
}

func TestPatternMatches(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		src   string
		route string
		want  bool
	}{
		{"exact", "a/b", "a/b", true},
		{"exact trailing slash", "a/b", "a/b/", true},
		{"exact child", "a/b", "a/b/c", false},
		{"wildcard prefix", "a/*", "a", true},
		{"wildcard child", "a/*", "a/b/c", true},
		{"wildcard sibling prefix", "a/*", "ab", false},
		{"wildcard parent", "a/b/*", "a", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, msg := parseSource(tt.src)
			require.Empty(t, msg)
			require.Equal(t, tt.want, p.matches(tt.route))
		})
	}
}

func rulesOf(t *testing.T, live routeSet, data string) []rule {
	t.Helper()
	rules, problems := checkRedirects([]byte(data), live)
	require.Empty(t, problems)
	return rules
}

func TestResolver(t *testing.T) {
	t.Parallel()

	live := liveOf("admin/users", "ai-coder/ai-gateway", "ai-coder/ai-gateway/setup", "install/esr-2.24-2.29-upgrade")
	rules := rulesOf(t, live, `[
		{"source": "old", "destination": "admin/users"},
		{"source": "gone", "destination": "/"},
		{"source": "frag", "destination": "admin/users#section"},
		{"source": "blog", "destination": "https://coder.com/blog"},
		{"source": "ai-coder/ai-bridge/*", "destination": "ai-coder/ai-gateway/*"},
		{"source": "tutorials/*", "destination": "admin/users"},
		{"source": "sub/*", "destination": "ai-coder/ai-gateway/*"}
	]`)
	r := newResolver(live, nil, rules)

	for _, tt := range []struct {
		route string
		want  bool
	}{
		{"admin/users", true},
		{"admin/users/", true},
		{"old", true},
		{"frag", true},
		{"gone", true},
		{"blog", true},
		{"ai-coder/ai-bridge", true},
		{"ai-coder/ai-bridge/setup", true},
		{"ai-coder/ai-bridge/missing", false},
		{"tutorials/anything/deep", true},
		{"unknown", false},
		{"admin/missing", false},
		// Dot-to-hyphen compatibility with the beta-docs engine.
		{"install/esr-2-24-2-29-upgrade", true},
		{"install/esr-2.24-2.29-upgrade", true},
		{"install/esr-2-24-2-29-other", false},
	} {
		t.Run(tt.route, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, r.resolves(tt.route))
		})
	}
}

func TestResolverFollowsAtMostThreeHops(t *testing.T) {
	t.Parallel()

	live := liveOf("end")
	mk := func(src, dst string) rule {
		s, _ := parseSource(src)
		d, _ := parseDestination(dst)
		return rule{src: s, dst: d}
	}
	rules := []rule{mk("a", "b"), mk("b", "c"), mk("c", "end")}
	r := newResolver(live, nil, rules)
	require.True(t, r.resolves("b"))
	// Three redirects: a to b to c to end.
	require.True(t, r.resolves("a"))
	// Four redirects is one too many.
	rules = append([]rule{mk("z", "a")}, rules...)
	require.False(t, newResolver(live, nil, rules).resolves("z"))
}

func TestResolverIgnoresRulesWithInvalidDestination(t *testing.T) {
	t.Parallel()

	s, _ := parseSource("old")
	r := newResolver(liveOf(), nil, []rule{{src: s}})
	require.False(t, r.resolves("old"))
}

func TestSuggest(t *testing.T) {
	t.Parallel()

	r := newResolver(liveOf("admin/users", "admin/groups", "other/groups", "install/esr-2.24-upgrade"), nil, nil)
	for _, tt := range []struct{ route, want string }{
		{"old/users", "admin/users"},
		{"old/groups", "/"},
		{"old/nothing", "/"},
		{"old/esr-2-24-upgrade", "install/esr-2.24-upgrade"},
	} {
		t.Run(tt.route, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, r.suggest(tt.route))
		})
	}
}

func TestSuggestRename(t *testing.T) {
	t.Parallel()

	head := liveOf("admin/integrations/istio-mesh", "admin/integrations/other", "admin/new-section/page", "top-new", "admin/users")
	base := liveOf("admin/integrations/istio", "admin/integrations/other", "admin/users")
	renamed := newResolver(head, base, nil)
	require.Equal(t, []string{"admin/integrations/istio-mesh", "admin/new-section/page", "top-new"}, renamed.added)
	for _, tt := range []struct{ name, route, want string }{
		{"one page added in the same section", "admin/integrations/istio", "admin/integrations/istio-mesh"},
		{"nothing added in the section", "admin/users-old", "/"},
		{"added page in another section", "admin/other-section/page2", "/"},
		{"root level route", "old-top", "top-new"},
		{"same last segment wins", "legacy/other", "admin/integrations/other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, renamed.suggest(tt.route))
		})
	}

	// Two pages added in the section is not an obvious home.
	head = liveOf("admin/integrations/istio-mesh", "admin/integrations/istio-gateway")
	two := newResolver(head, liveOf("admin/integrations/istio"), nil)
	require.Equal(t, "/", two.suggest("admin/integrations/istio"))

	// Without a base manifest nothing counts as added.
	noBase := newResolver(head, nil, nil)
	require.Empty(t, noBase.added)
	require.Equal(t, "/", noBase.suggest("admin/integrations/istio"))
}

func TestRunSuggestsTheRenamedPage(t *testing.T) {
	t.Parallel()

	head := `{"routes": [{"title": "Home", "path": "./README.md"}, {"title": "Mesh", "path": "./admin/integrations/istio-mesh.md"}]}`
	g := prGit("")
	g.files["HEAD^1:"+manifestRepoPath] = `{"routes": [{"title": "Home", "path": "./README.md"}, {"title": "Istio", "path": "./admin/integrations/istio.md"}]}`
	e := newRunEnv(t, head, `[]`)
	require.Equal(t, 1, e.run(runArgs{offline: true, git: g, env: prEnv}))
	require.Contains(t, e.stderr.String(), `{"source": "admin/integrations/istio", "destination": "admin/integrations/istio-mesh"}`)
}

func TestRunCountMessagesAgreeWithTheirCount(t *testing.T) {
	t.Parallel()

	one := newRunEnv(t, manifestHead, `[]`)
	g := prGit("")
	g.files["HEAD^1:"+manifestRepoPath] = strings.Replace(manifestHead, `{"title": "Users"`, `{"title": "Groups", "path": "./admin/groups.md"}, {"title": "Users"`, 1)
	require.Equal(t, 1, one.run(runArgs{offline: true, git: g, env: prEnv}))
	require.Contains(t, one.stderr.String(), "1 docs route no longer resolves.")

	many := newRunEnv(t, manifestHead, `[]`)
	g = prGit("")
	g.files["HEAD^1:"+manifestRepoPath] = strings.Replace(manifestBase, `{"title": "Users"`, `{"title": "Extra", "path": "./extra.md"}, {"title": "Users"`, 1)
	require.Equal(t, 1, many.run(runArgs{offline: true, git: g, env: prEnv}))
	require.Contains(t, many.stderr.String(), "2 docs routes no longer resolve.")

	ok := newRunEnv(t, manifestHead, `[]`)
	g = &fakeGit{revs: map[string]bool{"HEAD^1": true}, files: map[string]string{"HEAD^1:" + manifestRepoPath: `{"routes": [{"title": "Home", "path": "./README.md"}, {"title": "Users", "path": "./admin/users.md"}]}`}}
	require.Equal(t, 0, ok.run(runArgs{offline: true, git: g, env: prEnv}))
	require.Contains(t, ok.stdout.String(), "1 route still resolves")

	ok = newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 0, ok.run(runArgs{offline: true, git: &fakeGit{revs: map[string]bool{"HEAD^1": true}, files: map[string]string{"HEAD^1:" + manifestRepoPath: manifestHead}}, env: prEnv}))
	require.Contains(t, ok.stdout.String(), "2 routes still resolve")
}

func TestParseLiveList(t *testing.T) {
	t.Parallel()

	body := "# Coder documentation (main)\r\n\r\n" +
		"Also see /beta-docs/@main/llms-full.txt for everything.\r\n\r\n## Pages\r\n\r\n" +
		"- [Home](/beta-docs/@main/): Home.\r\n" +
		"- [Users](/beta-docs/@main/admin/users/): Users.\r\n" +
		"- [Users again](/beta-docs/@main/admin/users): Dupe.\r\n" +
		"- [Absolute](https://coder.com/beta-docs/@main/ai-coder/ai-gateway/?x=1#y): Absolute.\r\n" +
		"- [Index](/beta-docs/@main/install/index.md): Index.\r\n" +
		"- [Other version](/beta-docs/v2.37/admin/old/): Not this version.\r\n" +
		"- [Sibling](/beta-docs/@main-extra/x/): Not this version.\r\n" +
		"- [Classic](/docs/admin/classic): Not beta.\r\n" +
		"- [External](https://example.com/beta-docs/@main/x): Not ours.\r\n" +
		"- [Bad](/beta-docs/@main/a%20b/): Invalid route.\r\n"
	require.Equal(t,
		[]string{"", "admin/users", "ai-coder/ai-gateway", "install"},
		parseLiveList([]byte(body), "@main"),
	)
	require.Equal(t, []string{"admin/old"}, parseLiveList([]byte(body), "v2.37"))
	require.Empty(t, parseLiveList([]byte("no links"), "@main"))
}

func TestParseLiveListMainlineVersionLinksWithoutSegment(t *testing.T) {
	t.Parallel()

	body := "- [Home](/beta-docs/)\r\n" +
		"- [Users](/beta-docs/admin/users/)\r\n" +
		"- [Gateway](https://coder.com/beta-docs/ai-coder/ai-gateway/)\r\n" +
		"- [Other](/beta-docs/@main/admin/other/)\r\n" +
		"- [Old](/beta-docs/v2.37/admin/old/)\r\n" +
		"- [Classic](/docs/admin/classic)\r\n"
	require.Equal(t, []string{"", "admin/users", "ai-coder/ai-gateway"}, parseLiveList([]byte(body), "v2.38"))

	// A version that does link with its own segment ignores unversioned links.
	versioned := body + "- [Own](/beta-docs/v2.38/admin/own/)\r\n"
	require.Equal(t, []string{"admin/own"}, parseLiveList([]byte(versioned), "v2.38"))
}

func TestVersionSegment(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		branch string
		want   string
		ok     bool
	}{
		{"main", "@main", true},
		{"release/2.38", "v2.38", true},
		{"release/2.4", "v2.4", true},
		{"release/2.38.1", "", false},
		{"release/2.38-hotfix", "", false},
		{"feature/x", "", false},
		{"", "", false},
		{"main2", "", false},
	} {
		t.Run(tt.branch, func(t *testing.T) {
			t.Parallel()
			got, ok := versionSegment(tt.branch)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBaseRuleSources(t *testing.T) {
	t.Parallel()

	data := "[\r\n" +
		`{"source": "exact", "destination": "x"},` + "\r\n" +
		`{"source": "wild/*", "destination": "x"},` + "\r\n" +
		`{"source": "/docs/legacy", "destination": "/docs/x"},` + "\r\n" +
		`{"source": "/docs/legacy-wild/:path*", "destination": "/docs/x"},` + "\r\n" +
		`{"source": "/docs/regex/:slug(.*)", "destination": "/docs/x"},` + "\r\n" +
		`{"source": "bad source", "destination": "x"},` + "\r\n" +
		`{"destination": "x"}` + "\r\n]\r\n"
	require.Equal(t,
		[]string{"exact", "wild", "legacy", "legacy-wild", "regex"},
		baseRuleSources([]byte(data)),
	)
	require.Empty(t, baseRuleSources([]byte("not json")))
	require.Empty(t, baseRuleSources([]byte(`{}`)))
}

// ---- fetching ----

type fetchResult struct {
	status int
	body   string
	err    error
}

type fakeFetcher struct {
	// results are returned in order; the last repeats.
	results []fetchResult
	urls    []string
}

func (f *fakeFetcher) Get(_ context.Context, url string) (int, []byte, error) {
	f.urls = append(f.urls, url)
	i := min(len(f.urls)-1, len(f.results)-1)
	r := f.results[i]
	return r.status, []byte(r.body), r.err
}

func noSleep(time.Duration) {}

const listBody = "- [Home](/beta-docs/@main/)\n- [Users](/beta-docs/@main/admin/users/)\n"

func TestFetchLiveRoutes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		results   []fetchResult
		wantCalls int
		wantErr   string
		want      []string
	}{
		{name: "ok", results: []fetchResult{{status: 200, body: listBody}}, wantCalls: 1, want: []string{"", "admin/users"}},
		{name: "retries then ok", results: []fetchResult{{err: xerrors.New("reset")}, {status: 503}, {status: 403}, {status: 200, body: listBody}}, wantCalls: 4, want: []string{"", "admin/users"}},
		{name: "network error", results: []fetchResult{{err: xerrors.New("no route to host")}}, wantCalls: fetchAttempts, wantErr: "no route to host"},
		{name: "forbidden", results: []fetchResult{{status: 403}}, wantCalls: fetchAttempts, wantErr: "HTTP status 403"},
		{name: "not found", results: []fetchResult{{status: 404}}, wantCalls: fetchAttempts, wantErr: "HTTP status 404"},
		{name: "empty list is an error and is not retried", results: []fetchResult{{status: 200, body: "# nothing here\n"}}, wantCalls: 1, wantErr: "lists no pages"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeFetcher{results: tt.results}
			var slept []time.Duration
			got, err := fetchLiveRoutes(context.Background(), f, "https://example.test/llms.txt", "@main", func(d time.Duration) { slept = append(slept, d) })
			require.Len(t, f.urls, tt.wantCalls)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Len(t, slept, tt.wantCalls-1)
		})
	}
}

func TestHTTPFetcherSendsExplicitUserAgent(t *testing.T) {
	t.Parallel()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if strings.HasPrefix(gotUA, "Go-http-client") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(listBody))
	}))
	t.Cleanup(srv.Close)

	status, body, err := newHTTPFetcher().Get(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, listBody, string(body))
	require.Equal(t, userAgent, gotUA)
}

// ---- run (CLI behavior) ----

type fakeGit struct {
	revs  map[string]bool
	files map[string]string // "rev:path" -> contents
}

func (f *fakeGit) HasRev(rev string) bool { return f.revs[rev] }

func (f *fakeGit) Show(rev, path string) ([]byte, error) {
	s, ok := f.files[rev+":"+path]
	if !ok {
		return nil, xerrors.New("not in revision")
	}
	return []byte(s), nil
}

const manifestHead = `{"routes": [
	{"title": "About", "path": "./README.md"},
	{"title": "Users", "path": "./admin/users.md"},
	{"title": "Gateway", "path": "./ai-coder/ai-gateway/index.md"}
]}`

const manifestBase = `{"routes": [
	{"title": "About", "path": "./README.md"},
	{"title": "Users", "path": "./admin/users.md"},
	{"title": "Groups", "path": "./admin/groups.md"},
	{"title": "Gateway", "path": "./ai-coder/ai-gateway/index.md"}
]}`

type runEnv struct {
	manifestPath  string
	redirectsPath string
	stdout        bytes.Buffer
	stderr        bytes.Buffer
}

// newRunEnv writes the head manifest and redirects into a directory whose name
// has a space in it, so the flag paths are not assumed to be plain.
func newRunEnv(t *testing.T, manifest, redirects string) *runEnv {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "docs dir", "nested")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	e := &runEnv{
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

type runArgs struct {
	base    string
	branch  string
	offline bool
	git     gitReader
	fetch   fetcher
	env     map[string]string
}

func (e *runEnv) run(a runArgs) int {
	if a.git == nil {
		a.git = &fakeGit{}
	}
	if a.fetch == nil {
		a.fetch = failingFetcher{}
	}
	return run(options{
		manifestPath:  e.manifestPath,
		redirectsPath: e.redirectsPath,
		base:          a.base,
		branch:        a.branch,
		offline:       a.offline,
		git:           a.git,
		fetch:         a.fetch,
		sleep:         noSleep,
		getenv:        func(k string) string { return a.env[k] },
		stdout:        &e.stdout,
		stderr:        &e.stderr,
	})
}

// failingFetcher fails the test run when a check reads the network it should
// not have.
type failingFetcher struct{}

func (failingFetcher) Get(context.Context, string) (int, []byte, error) {
	return 0, nil, xerrors.New("unexpected network access")
}

func prGit(baseRedirects string) *fakeGit {
	g := &fakeGit{
		revs:  map[string]bool{"HEAD^1": true},
		files: map[string]string{"HEAD^1:" + manifestRepoPath: manifestBase},
	}
	if baseRedirects != "" {
		g.files["HEAD^1:"+redirectsRepoPath] = baseRedirects
	}
	return g
}

var prEnv = map[string]string{"GITHUB_BASE_REF": "main"}

func TestRunAbsentRedirectsFilePasses(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, "")
	require.Equal(t, 0, e.run(runArgs{offline: true}))
	require.Contains(t, e.stdout.String(), "nothing to validate")
}

func TestRunValidRedirectsPasses(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, `[{"source": "old", "destination": "admin/users"}]`)
	require.Equal(t, 0, e.run(runArgs{offline: true}))
	require.Contains(t, e.stdout.String(), "1 rule OK")
	require.Contains(t, e.stdout.String(), "live page list skipped (-offline)")
}

func TestRunInvalidRedirectsFailsWithActionableMessages(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, `[{"source": "/docs/old", "destination": "/docs/admin/users"}]`)
	require.Equal(t, 1, e.run(runArgs{offline: true}))
	require.Contains(t, e.stderr.String(), `{"source": "old", "destination": "admin/users"}`)
	require.Contains(t, e.stderr.String(), "1 problem found")
}

func TestRunMissingOrBrokenManifestIsAnError(t *testing.T) {
	t.Parallel()

	for name, manifest := range map[string]string{"missing": "", "invalid": "nope"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRunEnv(t, manifest, `[]`)
			require.Equal(t, 2, e.run(runArgs{offline: true}))
		})
	}
}

func TestRunUnreadableRedirectsFileIsAnError(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, "")
	require.NoError(t, os.Mkdir(e.redirectsPath, 0o700))
	require.Equal(t, 2, e.run(runArgs{offline: true}))
}

func TestRunHandlesCRLFInManifestAndRedirects(t *testing.T) {
	t.Parallel()

	manifest := strings.ReplaceAll(manifestHead, "\n", "\r\n")
	redirects := "[\r\n  {\r\n    \"source\": \"old\",\r\n    \"destination\": \"admin/users\"\r\n  }\r\n]\r\n"
	e := newRunEnv(t, manifest, redirects)
	require.Equal(t, 0, e.run(runArgs{offline: true}))
	require.Contains(t, e.stdout.String(), "1 rule OK")
}

func TestRunBaseManifestRemovalFailsWithPasteReadyRule(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, `[]`)
	code := e.run(runArgs{offline: true, git: prGit(""), env: prEnv})
	require.Equal(t, 1, code)
	out := e.stderr.String()
	require.Contains(t, out, `docs route "admin/groups" no longer resolves`)
	require.Contains(t, out, "docs/admin/groups.md on the base branch")
	require.Contains(t, out, `{"source": "admin/groups", "destination": "/"}`)
	require.NotContains(t, out, "admin/users")
}

func TestRunBaseManifestRemovalCoveredByRulePasses(t *testing.T) {
	t.Parallel()

	for name, redirects := range map[string]string{
		"exact":    `[{"source": "admin/groups", "destination": "admin/users"}]`,
		"wildcard": `[{"source": "admin/groups/*", "destination": "admin/users"}]`,
		"root":     `[{"source": "admin/groups", "destination": "/"}]`,
		"external": `[{"source": "admin/groups", "destination": "https://github.com/coder/coder"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRunEnv(t, manifestHead, redirects)
			require.Equal(t, 0, e.run(runArgs{offline: true, git: prGit(""), env: prEnv}), e.stderr.String())
		})
	}
}

func TestRunBaseRulesAreInTheInventory(t *testing.T) {
	t.Parallel()

	// The base branch redirected "old" in the legacy form. Dropping the rule
	// from head breaks the old URL.
	e := newRunEnv(t, manifestHead, `[]`)
	base := prGit(`[{"source": "/docs/old", "destination": "/docs/admin/users"}, {"source": "/docs/wild/:path*", "destination": "/docs/admin/users"}]`)
	base.files["HEAD^1:"+manifestRepoPath] = manifestHead
	require.Equal(t, 1, e.run(runArgs{offline: true, git: base, env: prEnv}))
	out := e.stderr.String()
	require.Contains(t, out, `docs route "old" no longer resolves (from a rule in docs/redirects.json on the base branch)`)
	require.Contains(t, out, `docs route "wild" no longer resolves`)
}

func TestRunLiveListIsInTheInventory(t *testing.T) {
	t.Parallel()

	f := &fakeFetcher{results: []fetchResult{{status: 200, body: listBody + "- [Old](/beta-docs/@main/admin/old/)\n"}}}
	e := newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 1, e.run(runArgs{branch: "main", fetch: f}))
	require.Equal(t, []string{"https://coder.com/beta-docs/@main/llms.txt"}, f.urls)
	require.Contains(t, e.stderr.String(), `docs route "admin/old" no longer resolves (from the live page list https://coder.com/beta-docs/@main/llms.txt)`)

	e = newRunEnv(t, manifestHead, `[{"source": "admin/old", "destination": "admin/users"}]`)
	require.Equal(t, 0, e.run(runArgs{branch: "main", fetch: f}), e.stderr.String())
}

func TestRunInventoryIsTheUnionOfAllSources(t *testing.T) {
	t.Parallel()

	f := &fakeFetcher{results: []fetchResult{{status: 200, body: "- [Live only](/beta-docs/@main/live-only/)\n"}}}
	base := prGit(`[{"source": "rule-only", "destination": "admin/users"}]`)
	e := newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 1, e.run(runArgs{git: base, fetch: f, env: map[string]string{"GITHUB_BASE_REF": "main"}}))
	out := e.stderr.String()
	for _, route := range []string{"admin/groups", "live-only", "rule-only"} {
		require.Contains(t, out, `docs route "`+route+`" no longer resolves`)
	}
	require.Contains(t, out, "3 docs routes no longer resolve")
	require.Contains(t, out, `  {"source": "admin/groups", "destination": "/"},`)
}

func TestRunLiveRouteMatchesHeadWithDotsReplacedByHyphens(t *testing.T) {
	t.Parallel()

	manifest := `{"routes": [{"title": "Home", "path": "./README.md"}, {"title": "ESR", "path": "./install/esr-2.24-2.29-upgrade.md"}]}`
	f := &fakeFetcher{results: []fetchResult{{status: 200, body: "- [ESR](/beta-docs/@main/install/esr-2-24-2-29-upgrade/)\n- [Home](/beta-docs/@main/)\n"}}}
	e := newRunEnv(t, manifest, `[]`)
	require.Equal(t, 0, e.run(runArgs{branch: "main", fetch: f}), e.stderr.String())
}

func TestRunNetworkFailureIsExitTwo(t *testing.T) {
	t.Parallel()

	for name, results := range map[string][]fetchResult{
		"error":      {{err: xerrors.New("connection refused")}},
		"forbidden":  {{status: 403}},
		"empty list": {{status: 200, body: "nothing"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := &fakeFetcher{results: results}
			e := newRunEnv(t, manifestHead, `[]`)
			env := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_BASE_REF": "main"}
			require.Equal(t, 2, e.run(runArgs{git: prGit(""), fetch: f, env: env}))
			require.Contains(t, e.stderr.String(), "removed-route check could not run")
		})
	}
}

func TestRunOfflineNeverTouchesTheNetwork(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, `[]`)
	env := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_BASE_REF": "main"}
	require.Equal(t, 0, e.run(runArgs{offline: true, git: prGit(manifestBase[:0]), fetch: failingFetcher{}, env: env})&^1)
	require.Contains(t, e.stdout.String(), "live page list skipped (-offline)")
}

func TestRunTargetBranchSelectsTheLiveList(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		branch  string
		env     map[string]string
		wantURL string
		wantMsg string
	}{
		{name: "flag main", branch: "main", wantURL: "https://coder.com/beta-docs/@main/llms.txt"},
		{name: "flag release", branch: "release/2.38", wantURL: "https://coder.com/beta-docs/v2.38/llms.txt"},
		{name: "pull request base", env: map[string]string{"GITHUB_BASE_REF": "release/2.37"}, wantURL: "https://coder.com/beta-docs/v2.37/llms.txt"},
		{name: "push", env: map[string]string{"GITHUB_REF_NAME": "main"}, wantURL: "https://coder.com/beta-docs/@main/llms.txt"},
		{name: "flag overrides environment", branch: "release/2.38", env: map[string]string{"GITHUB_BASE_REF": "main", "GITHUB_REF_NAME": "main"}, wantURL: "https://coder.com/beta-docs/v2.38/llms.txt"},
		{name: "base ref wins over ref name", env: map[string]string{"GITHUB_BASE_REF": "release/2.38", "GITHUB_REF_NAME": "42/merge"}, wantURL: "https://coder.com/beta-docs/v2.38/llms.txt"},
		{name: "unpublished branch", branch: "feature/x", wantMsg: `branch "feature/x" has no published docs version`},
		{name: "unknown branch", wantMsg: "cannot tell the target branch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := &fakeFetcher{results: []fetchResult{{status: 200, body: listBody + "- [Users](/beta-docs/v2.37/admin/users/)\n- [Users](/beta-docs/v2.38/admin/users/)\n"}}}
			e := newRunEnv(t, manifestHead, `[]`)
			require.Equal(t, 0, e.run(runArgs{branch: tt.branch, fetch: f, env: tt.env}), e.stderr.String())
			if tt.wantURL == "" {
				require.Empty(t, f.urls)
				require.Contains(t, e.stdout.String(), tt.wantMsg)
				return
			}
			require.Equal(t, []string{tt.wantURL}, f.urls)
		})
	}
}

func TestRunBaseUnavailableIsSkippedWithANote(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 0, e.run(runArgs{offline: true, git: &fakeGit{}, env: prEnv}))
	require.Contains(t, e.stdout.String(), `revision "HEAD^1" is not available`)

	e = newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 0, e.run(runArgs{offline: true}))
	require.Contains(t, e.stdout.String(), "not a pull request run")

	e = newRunEnv(t, manifestHead, `[]`)
	g := &fakeGit{revs: map[string]bool{"v1": true}}
	require.Equal(t, 0, e.run(runArgs{offline: true, base: "v1", git: g}))
	require.Contains(t, e.stdout.String(), "cannot read docs/manifest.json at v1")
}

func TestRunExplicitBaseOverridesEnvironment(t *testing.T) {
	t.Parallel()

	g := prGit("")
	g.revs["origin/main"] = true
	g.files["origin/main:"+manifestRepoPath] = manifestBase
	e := newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 1, e.run(runArgs{offline: true, base: "origin/main", git: g}))
}

func TestRunUsesGitHubAnnotationsInActions(t *testing.T) {
	t.Parallel()

	e := newRunEnv(t, manifestHead, `[]`)
	env := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_BASE_REF": "main"}
	require.Equal(t, 1, e.run(runArgs{offline: true, git: prGit(""), env: env}))
	require.Contains(t, e.stdout.String(), "::error file=docs/redirects.json,title=Docs route no longer resolves::")

	e = newRunEnv(t, manifestHead, `[]`)
	require.Equal(t, 1, e.run(runArgs{offline: true, git: prGit(""), env: prEnv}))
	require.NotContains(t, e.stdout.String(), "::error")
}

func TestAnnotationEscapes(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		"::error file=a%2Cb%3Ac,title=t%25::line1%0Aline2%0D100%25",
		annotation("error", "a,b:c", "t%", "line1\nline2\r100%"),
	)
}

func TestRuleJSON(t *testing.T) {
	t.Parallel()

	f := false
	require.Equal(t, `{"source": "a", "destination": "b"}`, ruleJSON("a", "b", nil))
	require.Equal(t, `{"source": "a/*", "destination": "b/*", "permanent": false}`, ruleJSON("a/*", "b/*", &f))
	require.Equal(t, `{"source": "a", "destination": "https://coder.com/x?a=1&b=2"}`, ruleJSON("a", "https://coder.com/x?a=1&b=2", nil))
}

func TestConvertLegacy(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in     string
		isDest bool
		want   string
	}{
		{"/docs/a/b", false, "a/b"},
		{"/docs/a/b/", false, "a/b"},
		{"/docs/a/b#frag", true, "a/b#frag"},
		{"/docs/a/:path*", false, "a/*"},
		{"/docs/a/:slug(.*)", false, "a/*"},
		{"/docs/a/:slug", true, "a/*"},
		{"/docs", true, "/"},
		{"/docs", false, ""},
		{"/docs/:path*", false, ":path*"},
		{"https://coder.com/x", true, "https://coder.com/x"},
		{"/documents/x", true, "/documents/x"},
		{"a/b", false, "a/b"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if tt.isDest {
				require.Equal(t, tt.want, convertLegacyDestination(tt.in))
				return
			}
			require.Equal(t, tt.want, convertLegacy(tt.in))
		})
	}
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
}

// TestRepositoryFiles runs the structural checks against the repository's own
// files, so a bad edit to docs/redirects.json fails `go test` as well as
// `make lint`. It never touches the network.
func TestRepositoryFiles(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile(filepath.Join("..", "..", "docs", "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("docs/manifest.json not found relative to the test")
	}
	require.NoError(t, err)
	m, err := parseManifest(manifestData)
	require.NoError(t, err)
	live := liveRoutes(m)
	require.Greater(t, len(live), 300)
	require.True(t, live.has("admin/external-auth"))

	redirects, err := os.ReadFile(filepath.Join("..", "..", "docs", "redirects.json"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("docs/redirects.json does not exist yet")
	}
	require.NoError(t, err)
	_, problems := checkRedirects(redirects, live)
	var msgs []string
	for _, p := range problems {
		msgs = append(msgs, formatProblem("docs/redirects.json", p))
	}
	require.Empty(t, problems, strings.Join(msgs, "\n"))
}

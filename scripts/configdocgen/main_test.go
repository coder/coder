package main

import (
	"strings"
	"testing"

	"github.com/coder/serpent"
)

func TestSentenceCase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases trailing words", "Send Actor Headers", "Send actor headers"},
		{"keeps trailing acronym", "Anthropic Base URL", "Anthropic base URL"},
		{"keeps all-caps token", "Allow BYOK", "Allow BYOK"},
		{"lowercases ordinary word", "Email Authentication", "Email authentication"},
		{"keeps proper noun", "Trace Honeycomb API Key", "Trace Honeycomb API key"},
		{"restores OpenID Connect", "OpenID Connect sign in text", "OpenID Connect sign in text"},
		{"keeps leading mixed-case token", "SSH Keygen Algorithm", "SSH keygen algorithm"},
		{"single lowercase word", "pprof", "pprof"},
		{"feature name", "AI Gateway", "AI Gateway"},
		{"longer feature name wins", "AI Gateway Proxy", "AI Gateway Proxy"},
		{"feature name as whole title", "Template Builder", "Template Builder"},
		{"feature name after leading word", "Disable Template Builder", "Disable Template Builder"},
		{"leading symbol is not the first word", "⚠️ Dangerous", "⚠️ Dangerous"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sentenceCase(tc.in); got != tc.want {
				t.Errorf("sentenceCase(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripGroupPrefix(t *testing.T) {
	t.Parallel()

	aiGateway := serpent.Group{Name: "AI Gateway"}
	email := serpent.Group{Name: "Email"}
	emailAuth := serpent.Group{Name: "Email Authentication", Parent: &email}
	introspection := serpent.Group{Name: "Introspection"}
	healthCheck := serpent.Group{Name: "Health Check", Parent: &introspection}
	networking := serpent.Group{Name: "Networking"}
	derp := serpent.Group{Name: "DERP", Parent: &networking}
	oauth2 := serpent.Group{Name: "OAuth2"}
	github := serpent.Group{Name: "GitHub", Parent: &oauth2}
	dangerous := serpent.Group{Name: "⚠️ Dangerous"}

	cases := []struct {
		name  string
		group *serpent.Group
		want  string
	}{
		// Space-prefixed names drop the group path.
		{"AI Gateway Send Actor Headers", &aiGateway, "Send Actor Headers"},
		{"DERP Config Path", &derp, "Config Path"},
		{"OAuth2 GitHub Allow Everyone", &github, "Allow Everyone"},
		// Colon-prefixed names drop up to the last ": ".
		{"Email Auth: Identity", &emailAuth, "Identity"},
		// A meaningful colon that is not a group separator is preserved.
		{"Health Check Threshold: Database", &healthCheck, "Threshold: Database"},
		// The Dangerous group's emoji name still matches its "DANGEROUS:" prefix.
		{"DANGEROUS: Allow Path App Sharing", &dangerous, "Allow Path App Sharing"},
		// Names that do not repeat the group are unchanged.
		{"Access URL", &networking, "Access URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := stripGroupPrefix(tc.name, tc.group); got != tc.want {
				t.Errorf("stripGroupPrefix(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestShortTitle(t *testing.T) {
	t.Parallel()

	aiGateway := serpent.Group{Name: "AI Gateway"}
	cases := []struct {
		opt  serpent.Option
		want string
	}{
		{serpent.Option{Name: "AI Gateway Send Actor Headers", Group: &aiGateway}, "Send actor headers"},
		{serpent.Option{Name: "AI Gateway Anthropic Base URL", Group: &aiGateway}, "Anthropic base URL"},
		// No group: only sentence case applies.
		{serpent.Option{Name: "Cache Directory"}, "Cache directory"},
	}
	for _, tc := range cases {
		t.Run(tc.opt.Name, func(t *testing.T) {
			t.Parallel()
			if got := shortTitle(tc.opt); got != tc.want {
				t.Errorf("shortTitle(%q) = %q, want %q", tc.opt.Name, got, tc.want)
			}
		})
	}
}

func TestIsDeprecated(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opt  serpent.Option
		want bool
	}{
		{"description prefix", serpent.Option{Description: "Deprecated: use X instead."}, true},
		{"description sentence", serpent.Option{Description: "Deprecated and ignored."}, true},
		{"use instead", serpent.Option{UseInstead: []serpent.Option{{Name: "X"}}}, true},
		{"active", serpent.Option{Description: "A normal option."}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isDeprecated(tc.opt); got != tc.want {
				t.Errorf("isDeprecated(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestEmphasizeDeprecation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Description already starts with the marker: only the marker is bolded.
		{"marker with sentence", "Deprecated and ignored.", "**Deprecated** and ignored."},
		{"marker with colon", "Deprecated: use X.", "**Deprecated**: use X."},
		// Description does not start with the marker (the UseInstead path): the
		// marker is prepended.
		{"no marker", "A normal description.", "**Deprecated.** A normal description."},
		{"empty description", "", "Deprecated."},
		// A bare marker with no trailing text is left unbolded (markdownlint MD036).
		{"bare marker", "Deprecated", "Deprecated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := emphasizeDeprecation(tc.in); got != tc.want {
				t.Errorf("emphasizeDeprecation(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCollapse(t *testing.T) {
	t.Parallel()
	if got := collapse("a\n  b\tc  "); got != "a b c" {
		t.Errorf("collapse() = %q, want %q", got, "a b c")
	}
}

// TestRenderPipeline exercises buildTree and render end to end: section
// nesting and ordering, option skipping, deprecated sinking, and the per-option
// bullet list (environment variable, CLI flag anchor, YAML key, default).
func TestRenderPipeline(t *testing.T) {
	t.Parallel()

	email := serpent.Group{Name: "Email", YAML: "email"}
	emailAuth := serpent.Group{Name: "Email Authentication", YAML: "emailAuth", Parent: &email}
	dangerous := serpent.Group{Name: "⚠️ Dangerous", YAML: "dangerous"}

	opts := serpent.OptionSet{
		// Hidden options and options with no env/flag/YAML are skipped.
		{Name: "Hidden Option", Env: "CODER_HIDDEN", Hidden: true},
		{Name: "Unsettable Option"},
		// General section (no group).
		{Name: "Access URL", Env: "CODER_ACCESS_URL", Flag: "access-url", Default: "https://example.com", Description: "The access URL."},
		// A DefaultFn with no static Default renders the computed-at-runtime label.
		{Name: "Cache Directory", Env: "CODER_CACHE_DIRECTORY", Flag: "cache-dir", DefaultFn: func() string { return "~/.cache/coder" }, Description: "The cache directory."},
		// Deprecated via UseInstead: description does not start with "Deprecated".
		{Name: "Email From", Env: "CODER_EMAIL_FROM", Flag: "email-from", YAML: "from", Group: &email, Description: "The sender address.", UseInstead: []serpent.Option{{Name: "Notifications Email From"}}},
		// Active option with a flag shorthand.
		{Name: "Email Smarthost", Env: "CODER_EMAIL_SMARTHOST", Flag: "email-smarthost", FlagShorthand: "s", YAML: "smarthost", Group: &email, Description: "The SMTP host."},
		// Nested child section.
		{Name: "Email Authentication Identity", Env: "CODER_EMAIL_AUTH_IDENTITY", YAML: "identity", Group: &emailAuth, Description: "The identity."},
		// A Dangerous group sorts last regardless of alphabetical order.
		{Name: "DANGEROUS: Allow All Cors", Env: "CODER_DANGEROUS_ALLOW_ALL_CORS", Flag: "dangerous-allow-all-cors", Group: &dangerous, Description: "Allow all cross-origin requests."},
	}

	got := render(buildTree(opts))

	wantContains := []string{
		"## General",
		"### Access URL",
		"- Environment variable: `CODER_ACCESS_URL`",
		"- CLI flag: [`--access-url`](../../reference/cli/server.md#--access-url)",
		"- Default value: `https://example.com`",
		"## Email",
		"### Smarthost",
		// Flag shorthand is folded into the anchor to match the CLI reference.
		"- CLI flag: [`--email-smarthost`](../../reference/cli/server.md#-s---email-smarthost)",
		// YAML key is the dotted group path.
		"- YAML key: `email.from`",
		// Deprecated marker is prepended for the UseInstead path.
		"**Deprecated.** The sender address.",
		// A DefaultFn with no static Default is labeled, not evaluated.
		"- Default value: `(computed at runtime)`",
		"### Email authentication",
		"#### Identity",
		"- YAML key: `email.emailAuth.identity`",
		// The Dangerous group renders as its own section.
		"## ⚠️ Dangerous",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("render() missing %q\n---\n%s", w, got)
		}
	}

	// General (rank -1) sorts before every other top-level section.
	if i, j := strings.Index(got, "## General"), strings.Index(got, "## Email"); i < 0 || j < 0 || i > j {
		t.Errorf("General should render before Email (got indexes %d, %d)", i, j)
	}
	// Active options sort before deprecated ones within a section.
	if i, j := strings.Index(got, "### Smarthost"), strings.Index(got, "### From"); i < 0 || j < 0 || i > j {
		t.Errorf("active option should render before deprecated option (got indexes %d, %d)", i, j)
	}
	// The Dangerous section sorts last among top-level sections.
	if i, j := strings.Index(got, "## Email"), strings.Index(got, "## ⚠️ Dangerous"); i < 0 || j < 0 || i > j {
		t.Errorf("Dangerous section should render last (got indexes %d, %d)", i, j)
	}
	// Hidden and unsettable options never render.
	if strings.Contains(got, "Hidden") {
		t.Error("hidden option should be skipped")
	}
	if strings.Contains(got, "Unsettable") {
		t.Error("option with no env/flag/YAML should be skipped")
	}
}

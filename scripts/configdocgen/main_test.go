package main

import (
	"testing"

	"github.com/coder/serpent"
)

func TestSentenceCase(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Send Actor Headers":          "Send actor headers",
		"Anthropic Base URL":          "Anthropic base URL",
		"Allow BYOK":                  "Allow BYOK",
		"Email Authentication":        "Email authentication",
		"Trace Honeycomb API Key":     "Trace Honeycomb API key",
		"OpenID Connect sign in text": "OpenID connect sign in text",
		"SSH Keygen Algorithm":        "SSH keygen algorithm",
		"pprof":                       "pprof",
		// Feature names keep their branded casing.
		"AI Gateway":               "AI Gateway",
		"AI Gateway Proxy":         "AI Gateway Proxy",
		"Template Builder":         "Template Builder",
		"Disable Template Builder": "Disable Template Builder",
		// A leading symbol is preserved and does not count as the first word.
		"⚠️ Dangerous": "⚠️ Dangerous",
	}
	for in, want := range cases {
		if got := sentenceCase(in); got != want {
			t.Errorf("sentenceCase(%q) = %q, want %q", in, got, want)
		}
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
		if got := stripGroupPrefix(tc.name, tc.group); got != tc.want {
			t.Errorf("stripGroupPrefix(%q) = %q, want %q", tc.name, got, tc.want)
		}
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
		if got := shortTitle(tc.opt); got != tc.want {
			t.Errorf("shortTitle(%q) = %q, want %q", tc.opt.Name, got, tc.want)
		}
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
		if got := isDeprecated(tc.opt); got != tc.want {
			t.Errorf("isDeprecated(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEmphasizeDeprecation(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Deprecated and ignored.": "**Deprecated** and ignored.",
		"Deprecated: use X.":      "**Deprecated**: use X.",
	}
	for in, want := range cases {
		if got := emphasizeDeprecation(in); got != want {
			t.Errorf("emphasizeDeprecation(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCollapse(t *testing.T) {
	t.Parallel()
	if got := collapse("a\n  b\tc  "); got != "a b c" {
		t.Errorf("collapse() = %q, want %q", got, "a b c")
	}
}

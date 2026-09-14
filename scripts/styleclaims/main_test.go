package main

import (
	"strings"
	"testing"
)

func TestParseValeConfig(t *testing.T) {
	t.Parallel()

	src := `
StylesPath = docs/.style/styles
MinAlertLevel = suggestion

[*.md]
BasedOnStyles = Coder

Coder.OneSentencePerLine = NO

[docs/.style/*.md]
Coder.OneSentencePerLine = YES
`
	styles, toggles := parseValeConfig(src)
	if len(styles) != 1 || styles[0] != "Coder" {
		t.Fatalf("styles = %v, want [Coder]", styles)
	}
	got := toggles["Coder.OneSentencePerLine"]
	if got["*.md"] != "NO" {
		t.Errorf("default toggle = %q, want NO", got["*.md"])
	}
	if got["docs/.style/*.md"] != "YES" {
		t.Errorf("scoped toggle = %q, want YES", got["docs/.style/*.md"])
	}
}

func TestNewRuleScope(t *testing.T) {
	t.Parallel()

	toggles := map[string]map[string]string{
		"Coder.OneSentencePerLine": {"*.md": "NO", "docs/.style/*.md": "YES"},
	}
	scoped := newRule("OneSentencePerLine", "extends: existence\nlevel: warning\n", toggles)
	if !scoped.scoped {
		t.Error("rule disabled in a section should be marked scoped")
	}
	if len(scoped.enabledIn) != 1 || scoped.enabledIn[0] != "docs/.style/*.md" {
		t.Errorf("enabledIn = %v, want [docs/.style/*.md]", scoped.enabledIn)
	}
	if scoped.severity != "warning" {
		t.Errorf("severity = %q, want warning", scoped.severity)
	}

	global := newRule("BrandNames", "level: error\n", toggles)
	if global.scoped {
		t.Error("rule with no toggle should not be scoped")
	}
}

func TestActiveCitations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		text        string
		wantActive  []string
		wantPlanned []string
	}{
		{
			name:       "plain claim",
			text:       "*Enforced by `Coder.BrandNames`.*",
			wantActive: []string{"Coder.BrandNames"},
		},
		{
			name:        "planned suffix",
			text:        "*Enforced by `Coder.LearnMore` (planned).*",
			wantPlanned: []string{"Coder.LearnMore"},
		},
		{
			name:        "planned prefix",
			text:        "*Documentation-only. Planned Vale rule `Coder.Idioms`.*",
			wantPlanned: []string{"Coder.Idioms"},
		},
		{
			name:        "mixed active and planned in one sentence",
			text:        "*Enforced by `scripts/check_emdash.sh` (existing CI script) and `Coder.EmDash` (planned).*",
			wantActive:  []string{"scripts/check_emdash.sh"},
			wantPlanned: []string{"Coder.EmDash"},
		},
		{
			name: "rule named without a claim is ignored",
			text: "*Documentation-only. No Vale rule. Imprecise rules like `Google.Passive` and `write-good.Passive` fire on every passive construction, so they stay out of the package.*",
		},
		{
			name:       "claim after the citation",
			text:       "*Documentation-only. No Vale rule for the general principle; `Coder.SelectClick` enforces the specific case.*",
			wantActive: []string{"Coder.SelectClick"},
		},
		{
			name:       "vale rule form",
			text:       "*Vale rule: `Coder.OneSentencePerLine` (warning).*",
			wantActive: []string{"Coder.OneSentencePerLine"},
		},
		{
			name:        "each citation marked separately",
			text:        "*Enforced by `Google.Gender` (planned) and `Google.GenderBias` (planned); third-party rules aren't loaded by default.*",
			wantPlanned: []string{"Google.Gender", "Google.GenderBias"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			active, planned := activeCitations(tc.text)
			assertSame(t, "active", active, tc.wantActive)
			assertSame(t, "planned", planned, tc.wantPlanned)
		})
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	rules := map[string]bool{"BrandNames": true}
	cases := []struct {
		text string
		want class
	}{
		{"*Enforced by `Coder.BrandNames`.*", classTool},
		{"*Enforced by `markdownlint` rule `MD040` for the missing-language case.*", classTool},
		{"*Enforced by `Coder.LearnMore` (planned).*", classPlanned},
		{"*Documentation-only. No Vale rule.*", classDocumentationOnly},
		{"*Adapted from ASD-STE100 Issue 9, rule 5.2. Documentation-only. No Vale rule.*", classDocumentationOnly},
		// A rule cited as active that does not exist is not tool coverage.
		{"*Enforced by `Coder.FirstPersonSingular`.*", classDocumentationOnly},
		// A documentation-only section that cross-references a rule another
		// section owns stays documentation-only, so the rule is counted once.
		{"*Documentation-only. No Vale rule for the general principle; `Coder.BrandNames` enforces one case.*", classDocumentationOnly},
		// A documentation-only section that names a planned rule is still
		// documentation-only.
		{"*Documentation-only. Planned Vale rule `Coder.Idioms`.*", classDocumentationOnly},
		// A lowercase mention qualifies part of a section, not the section.
		{"*Enforced by `markdownlint` rule `MD001`. The nested rule is documentation-only.*", classTool},
	}
	for _, tc := range cases {
		if got := classify(tc.text, rules); got != tc.want {
			t.Errorf("classify(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestParseAnnotations(t *testing.T) {
	t.Parallel()

	src := strings.Join([]string{
		"## A rule",
		"",
		"Body prose.",
		"",
		"*Enforced by `Coder.BrandNames`.*",
		"",
		"## Another rule",
		"",
		"*Adapted from ASD-STE100 Issue 9, rule 5.2.",
		"Documentation-only.",
		"No Vale rule.*",
		"",
	}, "\n")

	got := parseAnnotations("page.md", src)
	if len(got) != 2 {
		t.Fatalf("parsed %d annotations, want 2", len(got))
	}
	if got[0].line != 5 {
		t.Errorf("first annotation line = %d, want 5", got[0].line)
	}
	if !strings.HasSuffix(got[1].text, "No Vale rule.*") {
		t.Errorf("multi-line annotation not joined: %q", got[1].text)
	}
}

// TestCheckCatchesFalseClaims covers the two regressions this checker exists to
// prevent: a Coder rule cited as active with no rule file, and a third-party
// rule cited as active while .vale.ini loads only the Coder package.
func TestCheckCatchesFalseClaims(t *testing.T) {
	t.Parallel()

	rules := []valeRule{{name: "BrandNames", severity: "error"}}
	annotations := map[string][]annotation{
		"voice-and-tone.md": {
			{file: "voice-and-tone.md", line: 10, text: "*Enforced by `Coder.FirstPersonSingular`.*"},
			{file: "voice-and-tone.md", line: 20, text: "*Enforced by `Google.OxfordComma`.*"},
			{file: "voice-and-tone.md", line: 30, text: "*Enforced by `Coder.BrandNames`.*"},
		},
	}
	landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") + coverageTable(map[string][4]int{
		"voice-and-tone.md": {3, 1, 0, 2},
	})

	findings := check([]string{"Coder"}, rules, annotations, landing)
	var msgs []string
	for _, f := range findings {
		msgs = append(msgs, f.msg)
	}
	joined := strings.Join(msgs, "\n")

	if !strings.Contains(joined, "Coder.FirstPersonSingular") {
		t.Errorf("missing finding for a rule with no rule file:\n%s", joined)
	}
	if !strings.Contains(joined, "Google.OxfordComma") {
		t.Errorf("missing finding for an unloaded third-party style:\n%s", joined)
	}
	for _, m := range msgs {
		if strings.Contains(m, "Coder.BrandNames") {
			t.Errorf("unexpected finding for a rule that is enabled: %s", m)
		}
	}
}

func TestCheckCatchesTableDrift(t *testing.T) {
	t.Parallel()

	rules := []valeRule{{name: "BrandNames", severity: "error"}}
	annotations := map[string][]annotation{
		"voice-and-tone.md": {{file: "voice-and-tone.md", line: 10, text: "*Enforced by `Coder.BrandNames`.*"}},
	}

	t.Run("severity mismatch", func(t *testing.T) {
		t.Parallel()
		landing := checksTable("`Coder.BrandNames`", "`warning`", "`docs/**`") +
			coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
		if !containsMsg(check([]string{"Coder"}, rules, annotations, landing), "severity") {
			t.Error("severity drift not reported")
		}
	})

	t.Run("count mismatch", func(t *testing.T) {
		t.Parallel()
		landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") +
			coverageTable(map[string][4]int{"voice-and-tone.md": {9, 9, 9, 9}})
		if !containsMsg(check([]string{"Coder"}, rules, annotations, landing), "annotations give") {
			t.Error("coverage drift not reported")
		}
	})

	t.Run("rule missing from the checks table", func(t *testing.T) {
		t.Parallel()
		landing := coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
		if !containsMsg(check([]string{"Coder"}, rules, annotations, landing), "missing from the checks table") {
			t.Error("unlisted rule not reported")
		}
	})

	t.Run("rule with no style guide section", func(t *testing.T) {
		t.Parallel()
		extra := []valeRule{rules[0], {name: "Orphan", severity: "warning"}}
		landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") +
			checksTable("`Coder.Orphan`", "`warning`", "`docs/**`") +
			coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
		if !containsMsg(check([]string{"Coder"}, extra, annotations, landing), "no style guide section cites it") {
			t.Error("undocumented rule not reported")
		}
	})
}

func checksTable(name, severity, scope string) string {
	return "| " + name + " | Vale | " + severity + " | " + scope + " | What it catches |\n"
}

func coverageTable(counts map[string][4]int) string {
	var rows []string
	var total [4]int
	for _, page := range subpages {
		c := counts[page]
		for i := range total {
			total[i] += c[i]
		}
		rows = append(rows, "| [Section](./"+page+") | "+
			itoa(c[0])+" | "+itoa(c[1])+" | "+itoa(c[2])+" | "+itoa(c[3])+" |")
	}
	rows = append(rows, "| **Total** | "+itoa(total[0])+" | "+itoa(total[1])+" | "+itoa(total[2])+" | "+itoa(total[3])+" |")
	return strings.Join(rows, "\n") + "\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func containsMsg(findings []finding, substr string) bool {
	for _, f := range findings {
		if strings.Contains(f.msg, substr) {
			return true
		}
	}
	return false
}

func assertSame(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

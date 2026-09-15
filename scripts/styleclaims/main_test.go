package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

[docs/.style/style-guide/**]
BasedOnStyles =
`
	styles, sectionStyles, toggles := parseValeConfig(src)
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
	if loaded, ok := sectionStyles["docs/.style/style-guide/**"]; !ok || len(loaded) != 0 {
		t.Errorf("sectionStyles[style-guide] = %v, want an empty style list", loaded)
	}
}

func TestNewRuleScope(t *testing.T) {
	t.Parallel()

	toggles := map[string]map[string]string{
		"Coder.OneSentencePerLine": {"*.md": "NO", "docs/.style/*.md": "YES"},
	}
	sectionStyles := map[string][]string{"*.md": {"Coder"}, "docs/.style/style-guide/**": nil}
	scoped := newRule("OneSentencePerLine", "extends: existence\nlevel: warning\n", sectionStyles, toggles)
	if scoped.defaultOn {
		t.Error("rule disabled in the catch-all section should not be on by default")
	}
	if len(scoped.enabledIn) != 1 || scoped.enabledIn[0] != "docs/.style/*.md" {
		t.Errorf("enabledIn = %v, want [docs/.style/*.md]", scoped.enabledIn)
	}
	if scoped.severity != "warning" {
		t.Errorf("severity = %q, want warning", scoped.severity)
	}

	global := newRule("BrandNames", "level: error\n", sectionStyles, toggles)
	if !global.defaultOn {
		t.Error("rule with no toggle should be on by default")
	}
	if len(global.disabledIn) != 1 || global.disabledIn[0] != "docs/.style/style-guide/**" {
		t.Errorf("disabledIn = %v, want [docs/.style/style-guide/**]", global.disabledIn)
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
		if got := classify(tc.text, rules, map[string]bool{"Coder": true}); got != tc.want {
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

	rules := []valeRule{{name: "BrandNames", severity: "error", defaultOn: true}}
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

	findings := checkClaims(testPages, []string{"Coder"}, rules, annotations, landing)
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

	rules := []valeRule{{name: "BrandNames", severity: "error", defaultOn: true}}
	annotations := map[string][]annotation{
		"voice-and-tone.md": {{file: "voice-and-tone.md", line: 10, text: "*Enforced by `Coder.BrandNames`.*"}},
	}

	t.Run("severity mismatch", func(t *testing.T) {
		t.Parallel()
		landing := checksTable("`Coder.BrandNames`", "`warning`", "`docs/**`") +
			coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
		if !containsMsg(checkClaims(testPages, []string{"Coder"}, rules, annotations, landing), "severity") {
			t.Error("severity drift not reported")
		}
	})

	t.Run("count mismatch", func(t *testing.T) {
		t.Parallel()
		landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") +
			coverageTable(map[string][4]int{"voice-and-tone.md": {9, 9, 9, 9}})
		if !containsMsg(checkClaims(testPages, []string{"Coder"}, rules, annotations, landing), "annotations give") {
			t.Error("coverage drift not reported")
		}
	})

	t.Run("rule missing from the checks table", func(t *testing.T) {
		t.Parallel()
		landing := coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
		if !containsMsg(checkClaims(testPages, []string{"Coder"}, rules, annotations, landing), "missing from the checks table") {
			t.Error("unlisted rule not reported")
		}
	})

	t.Run("rule with no style guide section", func(t *testing.T) {
		t.Parallel()
		extra := []valeRule{rules[0], {name: "Orphan", severity: "warning", defaultOn: true}}
		landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") +
			checksTable("`Coder.Orphan`", "`warning`", "`docs/**`") +
			coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
		if !containsMsg(checkClaims(testPages, []string{"Coder"}, extra, annotations, landing), "no style guide section cites it") {
			t.Error("undocumented rule not reported")
		}
	})
}

// testPages is the section set the table helpers build rows for. Production
// discovers this set from disk; the tests pin it so the fixtures stay small.
var testPages = []string{"voice-and-tone.md"}

func checksTable(name, severity, scope string) string {
	return "| " + name + " | Vale | " + severity + " | " + scope + " | What it catches |\n"
}

func coverageTable(counts map[string][4]int) string {
	var rows []string
	var total [4]int
	for _, page := range testPages {
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

func itoa(n int) string { return strconv.Itoa(n) }

func containsMsg(findings []finding, substr string) bool {
	return slices.ContainsFunc(findings, func(f finding) bool {
		return strings.Contains(f.msg, substr)
	})
}

func assertSame(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func TestCheckFooterCoverage(t *testing.T) {
	t.Parallel()

	src := `# Page

## Grouping heading

### Footered sub-rule

*Documentation-only.
No Vale rule.*

## Rule without a footer

Prose that states a rule.

## Example inside a fence

` + "```md\n## Not a real heading\n```" + `

*Documentation-only.
No Vale rule.*

## Color contrast

*Out of scope for this guide.
Tracked by the docs site theme.*

## Personas

#### Perry the Platform Engineer

Reference detail, not a rule.

## Learn more

- [Voice and tone](./voice-and-tone.md)
`

	findings := checkFooterCoverage("page.md", src)
	if len(findings) != 1 {
		t.Fatalf("checkFooterCoverage() = %v, want 1 finding", findings)
	}
	if !strings.Contains(findings[0].msg, "Rule without a footer") {
		t.Errorf("finding = %q, want the unfootered rule heading", findings[0].msg)
	}
}

func TestCheckScopeCell(t *testing.T) {
	t.Parallel()

	global := valeRule{name: "BrandNames", severity: "error", defaultOn: true, disabledIn: []string{"docs/.style/style-guide/**"}}
	scoped := valeRule{
		name:      "OneSentencePerLine",
		severity:  "warning",
		enabledIn: []string{"docs/.style/*.md", "docs/.style/styles/Coder/*.md"},
	}

	cases := []struct {
		name string
		rule valeRule
		cell string
		want bool
	}{
		{
			name: "global rule names the disable as an exception",
			rule: global,
			cell: "`docs/**` except `docs/.style/style-guide/**`",
		},
		{
			name: "global rule omits the disable",
			rule: global,
			cell: "`docs/**`",
			want: true,
		},
		{
			name: "global rule inverts the polarity",
			rule: global,
			cell: "`docs/**` and `docs/.style/style-guide/**`",
			want: true,
		},
		{
			name: "scoped rule names both enabling globs",
			rule: scoped,
			cell: "`docs/.style/*.md` and `docs/.style/styles/Coder/*.md` only",
		},
		{
			name: "scoped rule over-claims the whole docs tree",
			rule: scoped,
			cell: "`docs/**`",
			want: true,
		},
		{
			name: "scoped rule omits an enabling glob",
			rule: scoped,
			cell: "`docs/.style/*.md` only",
			want: true,
		},
	}

	for _, tc := range cases {
		findings := checkScopeCell("README.md", 1, tc.cell, tc.rule)
		if got := len(findings) > 0; got != tc.want {
			t.Errorf("%s: findings = %v, want a finding: %v", tc.name, findings, tc.want)
		}
	}
}

func TestLoadRulesSkipsDemoRules(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, body := range map[string]string{
		"BrandNames.yml":  "extends: substitution\nlevel: error\n",
		"DemoError.yml":   "extends: existence\nlevel: error\n",
		"DemoWarning.yml": "extends: existence\nlevel: warning\n",
		"notes.txt":       "not a rule",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	rules, err := loadRules(dir, map[string][]string{"*.md": {"Coder"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("loadRules() = %v, want only the non-demo rule", rules)
	}
	if rules[0].name != "BrandNames" || rules[0].severity != "error" {
		t.Errorf("rule = %+v, want BrandNames at error", rules[0])
	}
}

func TestParseAnnotationsIgnoresFencedExamples(t *testing.T) {
	t.Parallel()

	src := "## A rule\n\n" +
		"Write the footer like this:\n\n" +
		"```md\n*Enforced by `Coder.RuleThatOnlyExistsInThisExample`.*\n```\n\n" +
		"*Documentation-only.\nNo Vale rule.*\n"

	got := parseAnnotations("page.md", src)
	if len(got) != 1 {
		t.Fatalf("parseAnnotations() = %v, want only the real footer", got)
	}
	if strings.Contains(got[0].text, "RuleThatOnlyExistsInThisExample") {
		t.Errorf("annotation = %q, want the footer outside the fence", got[0].text)
	}
}

func TestSubpagesDiscoversAnnotatedPages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, body := range map[string]string{
		"README.md":       "## Coverage\n\n*Documentation-only.\nNo Vale rule.*\n",
		"word-choice.md":  "## A rule\n\n*Enforced by `Coder.BrandNames`.*\n",
		"editor-setup.md": "## How to configure your editor\n\nNo rules here.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pages, annotations, sources, err := subpages(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations["word-choice.md"]) != 1 {
		t.Errorf("annotations = %v, want the page's one footer", annotations["word-choice.md"])
	}
	if len(pages) != 1 || pages[0] != "word-choice.md" {
		t.Fatalf("subpages() = %v, want only the annotated page", pages)
	}
	if !strings.Contains(sources["word-choice.md"], "Coder.BrandNames") {
		t.Errorf("sources missing the page body: %q", sources["word-choice.md"])
	}
}

func TestCheckCoverageTableReportsMalformedRow(t *testing.T) {
	t.Parallel()

	landing := "| [Section](voice-and-tone.md) | 1 | 1 | 0 | 0 |\n| **Total** | 1 | 1 | 0 | 0 |\n"
	findings := checkCoverageTable(testPages, landing, map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
	if !containsMsg(findings, "no link to a style guide section") {
		t.Errorf("findings = %v, want a malformed-row finding", findings)
	}
}

func TestCheckCoverageTableReportsUnknownSection(t *testing.T) {
	t.Parallel()

	landing := "| [Section](./not-a-section.md) | 1 | 1 | 0 | 0 |\n" +
		coverageTable(map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
	findings := checkCoverageTable(testPages, landing, map[string][4]int{"voice-and-tone.md": {1, 1, 0, 0}})
	if !containsMsg(findings, "not a style guide section") {
		t.Errorf("findings = %v, want an unknown-section finding", findings)
	}
}

func TestCheckChecksTableReportsUnknownRule(t *testing.T) {
	t.Parallel()

	landing := checksTable("`Coder.NotARule`", "`error`", "`docs/**`")
	if !containsMsg(checkChecksTable(landing, nil), "no rule file under") {
		t.Error("a table row for a nonexistent rule was not reported")
	}
}

// TestCheckClaimsCountsPlannedAnnotations drives a planned annotation through
// the reconciler, so the planned column of the coverage table is exercised end
// to end rather than only in the classifier.
func TestCheckClaimsCountsPlannedAnnotations(t *testing.T) {
	t.Parallel()

	rules := []valeRule{{name: "BrandNames", severity: "error", defaultOn: true}}
	annotations := map[string][]annotation{
		"voice-and-tone.md": {
			{file: "voice-and-tone.md", line: 10, text: "*Enforced by `Coder.BrandNames`.*"},
			{file: "voice-and-tone.md", line: 20, text: "*Enforced by `Coder.LearnMore` (planned).*"},
			{file: "voice-and-tone.md", line: 30, text: "*Documentation-only. No Vale rule.*"},
		},
	}
	landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") +
		coverageTable(map[string][4]int{"voice-and-tone.md": {3, 1, 1, 1}})

	if findings := checkClaims(testPages, []string{"Coder"}, rules, annotations, landing); len(findings) != 0 {
		t.Fatalf("checkClaims() = %v, want no findings for a table that matches", findings)
	}

	drifted := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`") +
		coverageTable(map[string][4]int{"voice-and-tone.md": {3, 1, 0, 2}})
	if !containsMsg(checkClaims(testPages, []string{"Coder"}, rules, annotations, drifted), "annotations give") {
		t.Error("a planned annotation counted as documentation-only was not reported")
	}
}

// TestRunReconcilesTheRepository exercises the whole wiring, from .vale.ini and
// the rule files through page discovery to the landing page tables, against the
// real style guide. It is the test that fails if the repository and the
// checker disagree.
//
//nolint:paralleltest // t.Chdir cannot be used in a parallel test.
func TestRunReconcilesTheRepository(t *testing.T) {
	t.Chdir("../..")

	findings, err := run()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Errorf("%s:%d: %s", f.file, f.line, f.msg)
	}
}

// TestScopeText pins every shape the rule model can take, including the ones
// no rule in .vale.ini has today, so a future config change meets a test rather
// than a surprise.
func TestScopeText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rule valeRule
		want string
	}{
		{
			name: "on everywhere",
			rule: valeRule{defaultOn: true},
			want: "`docs/**`",
		},
		{
			name: "on everywhere except one section",
			rule: valeRule{defaultOn: true, disabledIn: []string{"docs/.style/style-guide/**"}},
			want: "`docs/**` except `docs/.style/style-guide/**`",
		},
		{
			name: "off by default, on in two sections",
			rule: valeRule{enabledIn: []string{"docs/a/*.md", "docs/b/*.md"}},
			want: "`docs/a/*.md` and `docs/b/*.md` only",
		},
		{
			name: "a single-directory glob is not reached by a subdirectory disable",
			rule: valeRule{enabledIn: []string{"docs/.style/*.md"}, disabledIn: []string{"docs/.style/style-guide/**"}},
			want: "`docs/.style/*.md` only",
		},
		{
			name: "a recursive glob is reached by a disable inside it",
			rule: valeRule{enabledIn: []string{"docs/admin/**"}, disabledIn: []string{"docs/admin/generated/**"}},
			want: "`docs/admin/**` except `docs/admin/generated/**`, and nowhere else",
		},
		{
			name: "off everywhere",
			rule: valeRule{},
			want: "nowhere",
		},
	}

	for _, tc := range cases {
		if got := tc.rule.scopeText(); got != tc.want {
			t.Errorf("%s: scopeText() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestCheckClaimsRejectsDisabledRule covers the enablement gate: a rule file can
// exist and still run nowhere, and a section must not cite it as enforcement.
func TestCheckClaimsRejectsDisabledRule(t *testing.T) {
	t.Parallel()

	rules := []valeRule{{name: "BrandNames", severity: "error"}}
	annotations := map[string][]annotation{
		"voice-and-tone.md": {{file: "voice-and-tone.md", line: 10, text: "*Enforced by `Coder.BrandNames`.*"}},
	}
	landing := coverageTable(map[string][4]int{"voice-and-tone.md": {1, 0, 0, 1}})

	findings := checkClaims(testPages, []string{"Coder"}, rules, annotations, landing)
	if !containsMsg(findings, "leaves it off everywhere") {
		t.Errorf("findings = %v, want a finding for a rule that runs nowhere", findings)
	}
	// A rule that runs nowhere must not also be demanded in the tables, or the
	// author is left with no way to satisfy the checker.
	if containsMsg(findings, "missing from the checks table") {
		t.Errorf("findings = %v, want no table demand for a disabled rule", findings)
	}
	if containsMsg(findings, "no style guide section cites it") {
		t.Errorf("findings = %v, want no citation demand for a disabled rule", findings)
	}
}

func TestUnfencedTracksTheOpeningDelimiter(t *testing.T) {
	t.Parallel()

	src := "```md\n~~~\n*Enforced by `Coder.InsideTheFence`.*\n```\n\n*Documentation-only.\nNo Vale rule.*\n"
	got := parseAnnotations("page.md", src)
	if len(got) != 1 || strings.Contains(got[0].text, "InsideTheFence") {
		t.Fatalf("parseAnnotations() = %v, want only the footer outside the fence", got)
	}

	tilde := "~~~md\n*Enforced by `Coder.InsideTheFence`.*\n~~~\n"
	if found := parseAnnotations("page.md", tilde); len(found) != 0 {
		t.Errorf("parseAnnotations() = %v, want nothing from a tilde-fenced example", found)
	}
}

func TestActiveCitationsSharedPlannedPrefix(t *testing.T) {
	t.Parallel()

	active, planned := activeCitations("*Planned Vale rules `Coder.One` and `Coder.Two`.*")
	if len(active) != 0 {
		t.Errorf("active = %v, want none: the prefix marks both citations planned", active)
	}
	assertSame(t, "planned", planned, []string{"Coder.One", "Coder.Two"})

	active, planned = activeCitations("*Enforced by `Coder.One` (planned) and `Coder.Two`.*")
	assertSame(t, "active", active, []string{"Coder.Two"})
	assertSame(t, "planned", planned, []string{"Coder.One"})
}

func TestUnfencedRespectsFenceRunLength(t *testing.T) {
	t.Parallel()

	// A 4-backtick block teaching nested fences: the inner 3-backtick lines are
	// content, and the block closes only on a run at least as long as its
	// opener.
	src := "````md\n```sh\n*Enforced by `Coder.InsideTheOuterFence`.*\n```\n````\n\n*Documentation-only.\nNo Vale rule.*\n"
	got := parseAnnotations("page.md", src)
	if len(got) != 1 || strings.Contains(got[0].text, "InsideTheOuterFence") {
		t.Fatalf("parseAnnotations() = %v, want only the footer outside the fence", got)
	}

	// A fence line carrying an info string opens a block; it never closes one.
	if found := parseAnnotations("page.md", "```\n```md\n*Enforced by `Coder.StillFenced`.*\n```\n"); len(found) != 0 {
		t.Errorf("parseAnnotations() = %v, want nothing: the info-string line cannot close the block", found)
	}
}

func TestCheckChecksTableRejectsInactiveRow(t *testing.T) {
	t.Parallel()

	landing := checksTable("`Coder.BrandNames`", "`error`", "`docs/**`")
	rules := []valeRule{{name: "BrandNames", severity: "error"}}
	findings := checkChecksTable(landing, rules)
	if !containsMsg(findings, "leaves it off everywhere") {
		t.Errorf("findings = %v, want a finding for a row that claims a disabled rule runs", findings)
	}
	if containsMsg(findings, "severity") || containsMsg(findings, "scope") {
		t.Errorf("findings = %v, want no severity or scope comparison against a disabled rule", findings)
	}
}

func TestClassifyUsesEnablementAndLoadedStyles(t *testing.T) {
	t.Parallel()

	enabled := map[string]bool{"BrandNames": true}
	loaded := map[string]bool{"Coder": true, "Google": true}

	cases := []struct {
		name string
		text string
		want class
	}{
		{
			name: "an enabled Coder rule is tool coverage",
			text: "*Enforced by `Coder.BrandNames`.*",
			want: classTool,
		},
		{
			name: "a rule file that runs nowhere is not tool coverage",
			text: "*Enforced by `Coder.SwitchedOff`.*",
			want: classDocumentationOnly,
		},
		{
			name: "a loaded third-party rule is tool coverage",
			text: "*Enforced by `Google.OxfordComma`.*",
			want: classTool,
		},
		{
			name: "an unloaded third-party rule is not",
			text: "*Enforced by `Microsoft.Foo`.*",
			want: classDocumentationOnly,
		},
	}

	for _, tc := range cases {
		if got := classify(tc.text, enabled, loaded); got != tc.want {
			t.Errorf("%s: classify() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestActiveCitationsBindsMarkersToClauses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		text        string
		wantActive  []string
		wantPlanned []string
	}{
		{
			name:        "a marker after the second citation leaves the first active",
			text:        "*Enforced by `Coder.One` and `Coder.Two` (planned).*",
			wantActive:  []string{"Coder.One"},
			wantPlanned: []string{"Coder.Two"},
		},
		{
			name:        "a marker after the first citation leaves the second active",
			text:        "*Enforced by `Coder.One` (planned) and `Coder.Two`.*",
			wantActive:  []string{"Coder.Two"},
			wantPlanned: []string{"Coder.One"},
		},
		{
			name:        "an opening marker covers every citation",
			text:        "*Planned Vale rules `Coder.One` and `Coder.Two`.*",
			wantPlanned: []string{"Coder.One", "Coder.Two"},
		},
		{
			name:        "a marker in a trailing clause stays there",
			text:        "*Enforced by `scripts/check_emdash.sh`, with `Coder.EmDash` (planned) to follow.*",
			wantActive:  []string{"scripts/check_emdash.sh"},
			wantPlanned: []string{"Coder.EmDash"},
		},
	}

	for _, tc := range cases {
		active, planned := activeCitations(tc.text)
		assertSame(t, tc.name+" active", active, tc.wantActive)
		assertSame(t, tc.name+" planned", planned, tc.wantPlanned)
	}
}

func TestUnderPathMatchesOnSegments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		glob, prefix string
		want         bool
	}{
		{"docs/foo/bar/**", "docs/foo/", true},
		{"docs/foobar/**", "docs/foo", false},
		{"docs/foo", "docs/foo", true},
		{"docs/anything/**", "", true},
	}
	for _, tc := range cases {
		if got := underPath(tc.glob, tc.prefix); got != tc.want {
			t.Errorf("underPath(%q, %q) = %v, want %v", tc.glob, tc.prefix, got, tc.want)
		}
	}
}

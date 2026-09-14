// Command styleclaims fails when the Coder documentation style guide claims a
// prose rule is enforced by tooling that is not actually enabled.
//
// The style guide at docs/.style/style-guide/ ends each rule section with an
// annotation naming what enforces it, for example:
//
//	*Enforced by `Coder.BrandNames`.*
//	*Enforced by `Coder.LearnMore` (planned).*
//	*Documentation-only. No Vale rule.*
//
// Those annotations drifted from reality: eight cited third-party `Google.*`
// rules that the repo-root .vale.ini never loads, and two cited `Coder.*` rules
// that have no YAML in docs/.style/styles/Coder/. A reader who trusts an
// annotation believes a rule is checked when it is not, which is worse than an
// unenforced rule the guide is honest about.
//
// This checker reconciles three sources:
//
//   - The configuration: .vale.ini plus the rule files under
//     docs/.style/styles/Coder/. This is the truth.
//   - The per-rule annotations in the style guide subpages.
//   - The coverage tables on the style guide landing page
//     (docs/.style/style-guide/README.md).
//
// It reports:
//
//   - A citation presented as active for a `Coder.*` rule with no rule file.
//   - A citation presented as active for a third-party rule whose style is not
//     listed in BasedOnStyles.
//   - A rule file that no annotation cites as active, so a rule landed without
//     the style guide section the per-rule PR pattern requires.
//   - A "Checks that run today" row whose severity disagrees with the rule
//     file, a row for a rule that does not exist, or a missing row for a rule
//     that does.
//   - A "Checks that run today" scope cell that does not name the globs where
//     .vale.ini enables the rule.
//   - A "Coverage by section" count that disagrees with the annotations.
//
// A citation counts as a claim of enforcement only when its sentence says so:
// the sentence contains "enforc" or names a "Vale rule". A sentence that merely
// mentions a rule, such as the note explaining why `Google.Passive` stays out
// of the package, is not a claim. A claim counts as planned, rather than
// active, when "planned" sits in the clause next to the citation. That keeps a
// mixed annotation such as "Enforced by `scripts/check_emdash.sh` (existing CI
// script) and `Coder.EmDash` (planned)" classified correctly on both halves.
//
// The format the checker relies on is therefore: start an annotation with one
// of the recognized keywords, and mark each planned citation individually.
//
// Usage:
//
//	styleclaims
//
// It takes no arguments and runs from the repository root.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	valeConfig   = ".vale.ini"
	rulesDir     = "docs/.style/styles/Coder"
	styleGuide   = "docs/.style/style-guide"
	landingPage  = "README.md"
	demoPrefix   = "Demo"
	coderPackage = "Coder"
)

// subpages lists the style guide sections in the order the landing page
// presents them. The coverage table is checked against this order.
var subpages = []string{
	"audience-and-scope.md",
	"voice-and-tone.md",
	"procedural-writing.md",
	"word-choice.md",
	"accessibility-and-inclusion.md",
	"capitalization-and-punctuation.md",
	"formatting.md",
	"numbers-units-and-dates.md",
}

// annotationStart matches the opening of a rule section's enforcement
// annotation. The annotation runs to the next line ending in an asterisk.
var annotationStart = regexp.MustCompile(`^\*(Enforced|Documentation-only|Adapted|Vale rule|Periods|Alt-text)`)

// citation matches a backticked rule or tool name inside an annotation.
var citation = regexp.MustCompile("`(Coder\\.[A-Za-z]+|Google\\.[A-Za-z]+|alex\\.[A-Za-z*]+|write-good\\.[A-Za-z]+|markdownlint|scripts/check_emdash\\.sh|MD[0-9]{3})`")

// severityCell matches the severity column of a "Checks that run today" row.
var severityCell = regexp.MustCompile("`(error|warning|suggestion)`")

// documentationOnly matches an annotation that declares its section
// documentation-only. The match is case-sensitive: a lowercase mention such as
// "the content between headings rule is documentation-only" qualifies part of a
// section, not the section itself.
var documentationOnly = regexp.MustCompile(`(^|[^\p{L}])Documentation-only`)

// valeRule is a rule file under docs/.style/styles/Coder/.
type valeRule struct {
	name     string // for example "BrandNames"
	severity string // the rule file's level: field
	// enabledIn lists the .vale.ini section globs that set the rule to YES.
	// Empty means the rule is active everywhere BasedOnStyles applies.
	enabledIn []string
	// scoped is true when any .vale.ini section sets the rule to NO, so the
	// rule runs only in the sections that re-enable it.
	scoped bool
}

// class is how a rule section's annotation is counted on the coverage table.
type class int

const (
	classTool class = iota
	classPlanned
	classDocumentationOnly
)

// annotation is one rule section's enforcement footer.
type annotation struct {
	file string
	line int
	text string
}

type finding struct {
	file string
	line int
	msg  string
}

func main() {
	findings, err := run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "styleclaims: %v\n", err)
		os.Exit(1)
	}
	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		if f.line > 0 {
			_, _ = fmt.Fprintf(os.Stderr, "%s:%d: %s\n", f.file, f.line, f.msg)
			continue
		}
		_, _ = fmt.Fprintf(os.Stderr, "%s: %s\n", f.file, f.msg)
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nstyleclaims: %d problem(s). The style guide must not claim enforcement that .vale.ini and %s do not provide.\n", len(findings), rulesDir)
	os.Exit(1)
}

func run() ([]finding, error) {
	cfg, err := os.ReadFile(valeConfig)
	if err != nil {
		return nil, err
	}
	styles, toggles := parseValeConfig(string(cfg))

	rules, err := loadRules(rulesDir, toggles)
	if err != nil {
		return nil, err
	}

	annotations := map[string][]annotation{}
	for _, page := range subpages {
		b, err := os.ReadFile(filepath.Join(styleGuide, page))
		if err != nil {
			return nil, err
		}
		annotations[page] = parseAnnotations(page, string(b))
	}

	landing, err := os.ReadFile(filepath.Join(styleGuide, landingPage))
	if err != nil {
		return nil, err
	}

	return check(styles, rules, annotations, string(landing)), nil
}

// parseValeConfig returns the styles listed in BasedOnStyles and the per-rule
// toggles. A toggle key is a rule name such as "Coder.OneSentencePerLine"; the
// value maps a section glob ("" for the default section) to YES or NO.
func parseValeConfig(src string) (styles []string, toggles map[string]map[string]string) {
	toggles = map[string]map[string]string{}
	section := ""
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "BasedOnStyles" {
			for _, s := range strings.Split(value, ",") {
				if s = strings.TrimSpace(s); s != "" {
					styles = append(styles, s)
				}
			}
			continue
		}
		if strings.Contains(key, ".") {
			if toggles[key] == nil {
				toggles[key] = map[string]string{}
			}
			toggles[key][section] = strings.ToUpper(value)
		}
	}
	return styles, toggles
}

func loadRules(dir string, toggles map[string]map[string]string) ([]valeRule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var rules []valeRule
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yml")
		if strings.HasPrefix(name, demoPrefix) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		rules = append(rules, newRule(name, string(b), toggles))
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].name < rules[j].name })
	return rules, nil
}

func newRule(name, yaml string, toggles map[string]map[string]string) valeRule {
	r := valeRule{name: name}
	for _, line := range strings.Split(yaml, "\n") {
		if rest, ok := strings.CutPrefix(line, "level:"); ok {
			r.severity = strings.TrimSpace(rest)
			break
		}
	}
	for section, value := range toggles[coderPackage+"."+name] {
		switch {
		case value == "NO":
			r.scoped = true
		case section != "" && value == "YES":
			r.enabledIn = append(r.enabledIn, section)
		}
	}
	sort.Strings(r.enabledIn)
	return r
}

// parseAnnotations extracts every rule section footer from a subpage.
func parseAnnotations(file, src string) []annotation {
	var out []annotation
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		if !annotationStart.MatchString(lines[i]) {
			continue
		}
		start := i
		var parts []string
		for ; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			parts = append(parts, trimmed)
			if strings.HasSuffix(trimmed, "*") && (i > start || strings.Count(lines[i], "*") > 1) {
				break
			}
		}
		out = append(out, annotation{file: file, line: start + 1, text: strings.Join(parts, " ")})
	}
	return out
}

// activeCitations splits an annotation's citations into the ones it claims are
// active and the ones it marks planned. Citations in sentences that make no
// enforcement claim are ignored.
func activeCitations(text string) (active, planned []string) {
	for _, sentence := range splitSentences(text) {
		lower := strings.ToLower(sentence)
		if !strings.Contains(lower, "enforc") && !strings.Contains(lower, "vale rule") {
			continue
		}
		locs := citation.FindAllStringSubmatchIndex(sentence, -1)
		for i, loc := range locs {
			name := sentence[loc[2]:loc[3]]
			start := 0
			if i > 0 {
				start = locs[i-1][1]
			}
			end := len(sentence)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			// A citation is planned when "planned" sits in its own clause,
			// either just before it ("Planned Vale rule `X`") or just after
			// it ("`X` (planned)"). Scoping to the neighboring text keeps
			// mixed annotations that cite one active and one planned checker
			// classified correctly on both halves.
			window := strings.ToLower(sentence[start:loc[0]] + sentence[loc[1]:end])
			if strings.Contains(window, "planned") {
				planned = append(planned, name)
				continue
			}
			active = append(active, name)
		}
	}
	return active, planned
}

// splitSentences breaks an annotation into sentences. Periods inside backticks
// (`Coder.BrandNames`, `scripts/check_emdash.sh`) do not end a sentence.
func splitSentences(text string) []string {
	var out []string
	inCode := false
	start := 0
	for i, r := range text {
		if r == '`' {
			inCode = !inCode
		}
		if inCode || (r != '.' && r != ';') {
			continue
		}
		if r == '.' && i+1 < len(text) && text[i+1] != ' ' && text[i+1] != '*' {
			continue
		}
		out = append(out, text[start:i+1])
		start = i + 1
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

// isTool reports whether a citation names a checker outside the Vale styles.
func isTool(c string) bool {
	return c == "markdownlint" || c == "scripts/check_emdash.sh" || strings.HasPrefix(c, "MD")
}

// classify buckets an annotation for the coverage table.
//
// The coverage table counts rule sections, not linter rule names. An
// annotation that declares its section documentation-only stays
// documentation-only even when it cross-references a rule that another
// section owns, so a single rule is never counted under two sections.
func classify(text string, ruleNames map[string]bool) class {
	if documentationOnly.MatchString(text) {
		return classDocumentationOnly
	}
	active, planned := activeCitations(text)
	for _, c := range active {
		switch {
		case isTool(c):
			return classTool
		case strings.HasPrefix(c, coderPackage+".") && ruleNames[strings.TrimPrefix(c, coderPackage+".")]:
			return classTool
		}
	}
	if len(planned) > 0 {
		return classPlanned
	}
	if strings.Contains(strings.ToLower(text), "planned") {
		return classPlanned
	}
	return classDocumentationOnly
}

func check(styles []string, rules []valeRule, annotations map[string][]annotation, landing string) []finding {
	var findings []finding

	ruleNames := map[string]bool{}
	for _, r := range rules {
		ruleNames[r.name] = true
	}
	loadedStyle := map[string]bool{}
	for _, s := range styles {
		loadedStyle[s] = true
	}

	cited := map[string]bool{}
	counts := map[string][4]int{}

	for _, page := range subpages {
		var total, tool, planned, docOnly int
		for _, a := range annotations[page] {
			total++
			switch classify(a.text, ruleNames) {
			case classTool:
				tool++
			case classPlanned:
				planned++
			default:
				docOnly++
			}

			active, _ := activeCitations(a.text)
			for _, c := range active {
				if isTool(c) {
					continue
				}
				style, rule, ok := strings.Cut(c, ".")
				if !ok {
					continue
				}
				if style == coderPackage {
					if !ruleNames[rule] {
						findings = append(findings, finding{
							a.file, a.line,
							fmt.Sprintf("claims `%s` enforces this rule, but %s/%s.yml does not exist. Mark the citation (planned).", c, rulesDir, rule),
						})
						continue
					}
					cited[rule] = true
					continue
				}
				if !loadedStyle[style] {
					findings = append(findings, finding{
						a.file, a.line,
						fmt.Sprintf("claims `%s` enforces this rule, but %s does not load the %s style. Mark the citation (planned).", c, valeConfig, style),
					})
				}
			}
		}
		counts[page] = [4]int{total, tool, planned, docOnly}
	}

	for _, r := range rules {
		if !cited[r.name] {
			findings = append(findings, finding{
				filepath.Join(styleGuide, landingPage), 0,
				fmt.Sprintf("rule `%s.%s` is enabled but no style guide section cites it as active. Add the section, or remove the rule.", coderPackage, r.name),
			})
		}
	}

	findings = append(findings, checkChecksTable(landing, rules)...)
	findings = append(findings, checkCoverageTable(landing, counts)...)
	return findings
}

// checkChecksTable validates the "Checks that run today" rows against the rule
// files and the .vale.ini scoping.
func checkChecksTable(landing string, rules []valeRule) []finding {
	var findings []finding
	path := filepath.Join(styleGuide, landingPage)
	listed := map[string]bool{}

	for i, line := range strings.Split(landing, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := tableCells(line)
		if len(cells) < 5 {
			continue
		}
		name, ok := singleCoderRule(cells[0])
		if !ok {
			continue
		}
		listed[name] = true

		var rule *valeRule
		for j := range rules {
			if rules[j].name == name {
				rule = &rules[j]
				break
			}
		}
		if rule == nil {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("table lists `%s.%s`, which has no rule file under %s", coderPackage, name, rulesDir),
			})
			continue
		}
		if m := severityCell.FindStringSubmatch(cells[2]); len(m) < 2 || m[1] != rule.severity {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("severity for `%s.%s` is %q in the table but %q in the rule file", coderPackage, name, strings.TrimSpace(cells[2]), rule.severity),
			})
		}
		for _, glob := range rule.enabledIn {
			if !strings.Contains(cells[3], glob) {
				findings = append(findings, finding{
					path, i + 1,
					fmt.Sprintf("scope for `%s.%s` omits %q, where %s enables it", coderPackage, name, glob, valeConfig),
				})
			}
		}
		if !rule.scoped && !strings.Contains(cells[3], "docs/**") {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("scope for `%s.%s` should name `docs/**`; %s does not scope it to specific sections", coderPackage, name, valeConfig),
			})
		}
	}

	for _, r := range rules {
		if !listed[r.name] {
			findings = append(findings, finding{
				path, 0,
				fmt.Sprintf("rule `%s.%s` is enabled but missing from the checks table", coderPackage, r.name),
			})
		}
	}
	return findings
}

// checkCoverageTable validates the per-section counts and the total row.
func checkCoverageTable(landing string, counts map[string][4]int) []finding {
	var findings []finding
	path := filepath.Join(styleGuide, landingPage)
	seen := map[string]bool{}
	var wantTotal [4]int
	for _, c := range counts {
		for i := range wantTotal {
			wantTotal[i] += c[i]
		}
	}

	for i, line := range strings.Split(landing, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := tableCells(line)
		if len(cells) < 5 {
			continue
		}
		nums, ok := fourInts(cells[1:5])
		if !ok {
			continue
		}
		if strings.Contains(cells[0], "Total") {
			if nums != wantTotal {
				findings = append(findings, finding{
					path, i + 1,
					fmt.Sprintf("total row is %v but the annotations sum to %v", nums, wantTotal),
				})
			}
			seen["**Total**"] = true
			continue
		}
		page, ok := linkedPage(cells[0])
		if !ok {
			continue
		}
		want, known := counts[page]
		if !known {
			findings = append(findings, finding{path, i + 1, fmt.Sprintf("coverage row links %q, which is not a style guide section", page)})
			continue
		}
		seen[page] = true
		if nums != want {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("counts for %s are %v but the annotations give %v (rules, tool-checked, planned, documentation-only)", page, nums, want),
			})
		}
	}

	for _, page := range subpages {
		if !seen[page] {
			findings = append(findings, finding{path, 0, fmt.Sprintf("coverage table has no row for %s", page)})
		}
	}
	if !seen["**Total**"] {
		findings = append(findings, finding{path, 0, "coverage table has no total row"})
	}
	return findings
}

func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// singleCoderRule returns the rule name when a cell names exactly one Coder
// rule, so multi-rule rows such as the markdownlint row are skipped.
func singleCoderRule(cell string) (string, bool) {
	m := citation.FindAllStringSubmatch(cell, -1)
	if len(m) != 1 {
		return "", false
	}
	name, ok := strings.CutPrefix(m[0][1], coderPackage+".")
	return name, ok
}

func linkedPage(cell string) (string, bool) {
	open := strings.Index(cell, "](./")
	if open < 0 {
		return "", false
	}
	rest := cell[open+4:]
	end := strings.Index(rest, ")")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func fourInts(cells []string) ([4]int, bool) {
	var out [4]int
	for i, c := range cells {
		c = strings.Trim(c, "* ")
		n, err := strconv.Atoi(c)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Command styleclaims fails when the Coder documentation style guide claims a
// prose rule is enforced by tooling that is not actually enabled.
//
// The checker reconciles three sources: the configuration (.vale.ini plus the
// rule files under docs/.style/styles/Coder/, which is the truth), the per-rule
// annotations in the style guide subpages, and the coverage tables on the style
// guide landing page. scripts/styleclaims/README.md documents what it reports,
// the annotation format it relies on, and how sections are counted. Keep that
// README the single description; this comment stays a summary.
//
// Usage:
//
//	styleclaims
//
// It takes no arguments and runs from the repository root.
package main

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	valeConfig  = ".vale.ini"
	rulesDir    = "docs/.style/styles/Coder"
	styleGuide  = "docs/.style/style-guide"
	landingPage = "README.md"
	// demoPrefix marks the Demo*.yml files under rulesDir. Those are worked
	// examples for rule authors, not enforced rules, so they carry no style
	// guide section and must stay out of the rule set.
	demoPrefix   = "Demo"
	coderPackage = "Coder"
	// globalScope is the scope cell every rule that .vale.ini does not confine
	// to specific sections must name.
	globalScope = "docs/**"
)

// subpages returns the style guide rule sections, discovered from disk rather
// than from a hand-maintained list: a new subpage is validated the day it
// lands. A Markdown file counts as a rule section when it carries at least one
// enforcement footer, which excludes the landing page and pages such as
// editor-setup.md that document tooling rather than rules.
func subpages(dir string) ([]string, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var pages []string
	sources := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" || e.Name() == landingPage {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, nil, err
		}
		if len(parseAnnotations(e.Name(), string(b))) == 0 {
			continue
		}
		pages = append(pages, e.Name())
		sources[e.Name()] = string(b)
	}
	slices.Sort(pages)
	return pages, sources, nil
}

// annotationStart matches the opening of a rule section's enforcement
// annotation. The annotation runs to the next line ending in an asterisk.
var annotationStart = regexp.MustCompile(`^\*(Enforced|Documentation-only|Adapted|Vale rule|Periods|Alt-text)`)

// citation matches a backticked rule or tool name inside an annotation. A Vale
// rule is `Style.Rule`; the style alternation is open so a citation to a style
// the repo doesn't load is still read as a claim and reported.
var citation = regexp.MustCompile("`([A-Z][A-Za-z0-9-]*\\.[A-Za-z0-9]+|alex\\.[A-Za-z0-9*]+|write-good\\.[A-Za-z0-9]+|markdownlint|scripts/check_emdash\\.sh|MD[0-9]{3})`")

// severityCell matches the severity column of a "Checks that run today" row.
var severityCell = regexp.MustCompile("`(error|warning|suggestion)`")

// documentationOnly matches an annotation that declares its section
// documentation-only. The match is case-sensitive: a lowercase mention such as
// "the content between headings rule is documentation-only" qualifies part of a
// section, not the section itself.
var documentationOnly = regexp.MustCompile(`(^|[^\p{L}])Documentation-only`)

// headingLine matches a Markdown ATX heading outside a fenced block.
var headingLine = regexp.MustCompile(`^(#{1,6}) +(.*)$`)

// nonRuleNote matches the footer forms that say a heading is not a rule of this
// guide: an out-of-scope note naming an owner elsewhere, or an explicit
// disclaimer on a section that organizes a page without stating a rule.
var nonRuleNote = regexp.MustCompile(`^\*(Out of scope|Not a rule)`)

// scopeGlob matches a backticked path or glob in a scope cell.
var scopeGlob = regexp.MustCompile("`([A-Za-z0-9_./*-]*[/*][A-Za-z0-9_./*-]*)`")

// learnMoreHeading is the navigation section every subpage ends with.
const learnMoreHeading = "Learn more"

// valeRule is a rule file under docs/.style/styles/Coder/.
type valeRule struct {
	name     string // for example "BrandNames"
	severity string // the rule file's level: field
	// enabledIn lists the .vale.ini section globs that set the rule to YES.
	// Empty means the rule is active everywhere BasedOnStyles applies.
	enabledIn []string
	// disabledIn lists the section globs whose empty BasedOnStyles loads no
	// style at all, so no Coder rule runs under those paths.
	disabledIn []string
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
	styles, sectionStyles, toggles := parseValeConfig(string(cfg))

	rules, err := loadRules(rulesDir, sectionStyles, toggles)
	if err != nil {
		return nil, err
	}

	pages, sources, err := subpages(styleGuide)
	if err != nil {
		return nil, err
	}

	annotations := map[string][]annotation{}
	var findings []finding
	for _, page := range pages {
		annotations[page] = parseAnnotations(page, sources[page])
		findings = append(findings, checkFooterCoverage(page, sources[page])...)
	}

	landing, err := os.ReadFile(filepath.Join(styleGuide, landingPage))
	if err != nil {
		return nil, err
	}

	return append(findings, checkClaims(pages, styles, rules, annotations, string(landing))...), nil
}

// checkFooterCoverage reports a rule heading that carries no enforcement
// footer. The coverage table counts footers, so without this a new rule could
// drop out of the table silently. Only `##` and `###` headings state rules; a
// deeper heading is detail inside a section. A heading is exempt when it only
// groups sub-headings, when it is the navigation footer, or when it carries an
// out-of-scope note naming an owner outside this guide.
func checkFooterCoverage(page, src string) []finding {
	type heading struct {
		level  int
		line   int
		text   string
		footer bool
		child  bool
		note   bool
	}
	var headings []heading
	for i, line := range unfenced(src) {
		if line == "" {
			continue
		}
		if m := headingLine.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			for j := len(headings) - 1; j >= 0; j-- {
				if headings[j].level < level {
					headings[j].child = true
					break
				}
			}
			headings = append(headings, heading{level: level, line: i + 1, text: strings.TrimSpace(m[2])})
			continue
		}
		if len(headings) == 0 {
			continue
		}
		last := &headings[len(headings)-1]
		switch {
		case annotationStart.MatchString(line):
			last.footer = true
		case nonRuleNote.MatchString(line):
			last.note = true
		}
	}

	var findings []finding
	for _, h := range headings {
		if h.level < 2 || h.level > 3 || h.footer || h.child || h.note || h.text == learnMoreHeading {
			continue
		}
		findings = append(findings, finding{
			filepath.Join(styleGuide, page), h.line,
			fmt.Sprintf("%q states a rule but carries no enforcement footer, so the coverage table can't count it", h.text),
		})
	}
	return findings
}

// unfenced returns the file's lines with fenced code blocks blanked out, so
// every scanner in this package agrees about what is example text. Line numbers
// are preserved: the returned slice has one entry per source line.
func unfenced(src string) []string {
	lines := strings.Split(src, "\n")
	out := make([]string, len(lines))
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		out[i] = line
	}
	return out
}

// parseValeConfig returns the styles listed in BasedOnStyles and the per-rule
// toggles. A toggle key is a rule name such as "Coder.OneSentencePerLine"; the
// value maps a section glob ("" for the default section) to YES or NO.
// styles is every style any section loads. sectionStyles maps a section glob to
// the styles that section loads, so a section that sets an empty BasedOnStyles
// is recognized as disabling every rule under its path.
func parseValeConfig(src string) (styles []string, sectionStyles map[string][]string, toggles map[string]map[string]string) {
	toggles = map[string]map[string]string{}
	sectionStyles = map[string][]string{}
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
			var loaded []string
			for _, s := range strings.Split(value, ",") {
				if s = strings.TrimSpace(s); s != "" {
					loaded = append(loaded, s)
				}
			}
			sectionStyles[section] = loaded
			styles = append(styles, loaded...)
			continue
		}
		if strings.Contains(key, ".") {
			if toggles[key] == nil {
				toggles[key] = map[string]string{}
			}
			toggles[key][section] = strings.ToUpper(value)
		}
	}
	return styles, sectionStyles, toggles
}

func loadRules(dir string, sectionStyles map[string][]string, toggles map[string]map[string]string) ([]valeRule, error) {
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
		rules = append(rules, newRule(name, string(b), sectionStyles, toggles))
	}
	slices.SortFunc(rules, func(a, b valeRule) int { return cmp.Compare(a.name, b.name) })
	return rules, nil
}

func newRule(name, yaml string, sectionStyles map[string][]string, toggles map[string]map[string]string) valeRule {
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
	for section, loaded := range sectionStyles {
		if section == "" || len(loaded) > 0 {
			continue
		}
		r.disabledIn = append(r.disabledIn, section)
	}
	slices.Sort(r.enabledIn)
	slices.Sort(r.disabledIn)
	return r
}

// parseAnnotations extracts every rule section footer from a subpage. Footers
// shown as examples inside a fenced code block are not annotations.
func parseAnnotations(file, src string) []annotation {
	var out []annotation
	lines := unfenced(src)
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
		// The gate is substring-based, so punctuation matters: splitSentences
		// breaks on "." and ";" but not ":", which means a disclaimer written
		// as "No Vale rule for this: `Google.Passive` is too noisy." stays one
		// sentence and its citation reads as a claim. End a disclaimer with a
		// period before naming a rule.
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

// checkClaims reconciles the annotations against the configuration, then runs
// the two landing page table checks.
func checkClaims(pages []string, styles []string, rules []valeRule, annotations map[string][]annotation, landing string) []finding {
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

	for _, page := range pages {
		var total, tool, planned, docOnly int
		for _, a := range annotations[page] {
			total++
			class := classify(a.text, ruleNames)
			switch class {
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
					// Only the section that owns a rule marks it cited. A
					// documentation-only section may cross-reference a rule
					// another section owns, and that reference must not stand
					// in for the owning section if the owner is ever removed.
					if class != classDocumentationOnly {
						cited[rule] = true
					}
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
	findings = append(findings, checkCoverageTable(pages, landing, counts)...)
	return findings
}

// checkChecksTable validates the "Checks that run today" rows against the rule
// files and the .vale.ini scoping.
func checkChecksTable(landing string, rules []valeRule) []finding {
	var findings []finding
	path := filepath.Join(styleGuide, landingPage)
	listed := map[string]bool{}

	for i, line := range unfenced(landing) {
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
		findings = append(findings, checkScopeCell(path, i+1, cells[3], *rule)...)
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

// checkScopeCell compares the paths a scope cell names against the paths
// .vale.ini actually configures, in both directions: a cell that omits a glob
// under-claims the rule's reach, and a cell that names a path the config never
// mentions over-claims it.
func checkScopeCell(path string, line int, cell string, rule valeRule) []finding {
	want := map[string]bool{}
	if rule.scoped {
		for _, g := range rule.enabledIn {
			want[g] = true
		}
	} else {
		want[globalScope] = true
		for _, g := range rule.disabledIn {
			want[g] = true
		}
	}

	got := map[string]bool{}
	for _, m := range scopeGlob.FindAllStringSubmatch(cell, -1) {
		got[m[1]] = true
	}

	var findings []finding
	for _, g := range slices.Sorted(maps.Keys(want)) {
		if !got[g] {
			findings = append(findings, finding{
				path, line,
				fmt.Sprintf("scope for `%s.%s` omits `%s`, which %s configures for it", coderPackage, rule.name, g, valeConfig),
			})
		}
	}
	for _, g := range slices.Sorted(maps.Keys(got)) {
		if !want[g] {
			findings = append(findings, finding{
				path, line,
				fmt.Sprintf("scope for `%s.%s` claims `%s`, which %s does not configure for it", coderPackage, rule.name, g, valeConfig),
			})
		}
	}
	return findings
}

// checkCoverageTable validates the per-section counts and the total row.
func checkCoverageTable(pages []string, landing string, counts map[string][4]int) []finding {
	var findings []finding
	path := filepath.Join(styleGuide, landingPage)
	seen := map[string]bool{}
	var wantTotal [4]int
	for _, c := range counts {
		for i := range wantTotal {
			wantTotal[i] += c[i]
		}
	}

	for i, line := range unfenced(landing) {
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
					fmt.Sprintf("total row is %v but the annotations sum to %v (rules, tool-checked, planned, documentation-only)", nums, wantTotal),
				})
			}
			seen["**Total**"] = true
			continue
		}
		page, ok := linkedPage(cells[0])
		if !ok {
			findings = append(findings, finding{
				path, i + 1,
				"coverage row has counts but no link to a style guide section; write the first cell as [Section](./section.md)",
			})
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

	for _, page := range pages {
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

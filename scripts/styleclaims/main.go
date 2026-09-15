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
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	valeConfig = ".vale.ini"
	// coverageAnchor is the landing page heading anchor that `make lint/prose`
	// points authors at. The checker verifies it resolves, so the pointer
	// cannot rot when the section is renamed.
	coverageAnchor = "what-the-tooling-checks-and-what-it-doesnt"
	rulesDir       = "docs/.style/styles/Coder"
	styleGuide     = "docs/.style/style-guide"
	landingPage    = "README.md"
	// demoPrefix marks the Demo*.yml files under rulesDir. Those are worked
	// examples for rule authors, not enforced rules, so they carry no style
	// guide section and must stay out of the rule set.
	demoPrefix   = "Demo"
	coderPackage = "Coder"
	// globalScope is the scope cell every rule that .vale.ini does not confine
	// to specific sections must name.
	globalScope = "docs/**"
)

// subpages returns the style guide rule sections with their parsed footers,
// discovered from disk rather than from a hand-maintained list, so a new
// subpage is validated the day it lands.
//
// A Markdown file counts as a rule section when it carries at least one
// enforcement footer, or when the landing page's coverage table gives it a row.
// The second condition matters for a page that is all documentation-only rules
// and has not been annotated yet: adding its coverage row brings it under the
// footer check instead of leaving it invisible. Pages that are neither, such as
// editor-setup.md, document tooling rather than rules and are not scanned.
func subpages(dir, landing string) ([]string, map[string][]annotation, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	covered := map[string]bool{}
	for _, page := range coveragePages(landing) {
		covered[page] = true
	}

	var pages []string
	annotations := map[string][]annotation{}
	sources := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" || e.Name() == landingPage {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, nil, nil, err
		}
		found := parseAnnotations(e.Name(), string(b))
		if len(found) == 0 && !covered[e.Name()] {
			continue
		}
		pages = append(pages, e.Name())
		annotations[e.Name()] = found
		sources[e.Name()] = string(b)
	}
	slices.Sort(pages)
	return pages, annotations, sources, nil
}

// coveragePages returns the section files the coverage table links. Discovery
// and validation share coverageRow, so a row the table validates can never be
// a row discovery ignores.
func coveragePages(landing string) []string {
	var pages []string
	for _, line := range unfenced(landing) {
		row, ok := coverageRow(line)
		if !ok || row.page == "" {
			continue
		}
		pages = append(pages, row.page)
	}
	return pages
}

// tableRow is one parsed "Coverage by section" row.
type tableRow struct {
	page  string // the linked section file, empty on the total row
	total bool   // true on the total row
	nums  [4]int
}

// coverageRow recognizes a coverage table row: a Markdown row whose last four
// cells are counts.
func coverageRow(line string) (tableRow, bool) {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return tableRow{}, false
	}
	cells := tableCells(line)
	if len(cells) < 5 {
		return tableRow{}, false
	}
	nums, ok := fourInts(cells[1:5])
	if !ok {
		return tableRow{}, false
	}
	if strings.Contains(cells[0], "Total") {
		return tableRow{total: true, nums: nums}, true
	}
	page, _ := linkedPage(cells[0])
	return tableRow{page: page, nums: nums}, true
}

// annotationStart matches the opening of a rule section's enforcement
// annotation. The annotation runs to the next line ending in an asterisk.
var annotationStart = regexp.MustCompile(`^\*(Enforced|Documentation-only|Adapted|Vale rule)`)

// citation matches a backticked rule or tool name inside an annotation. A Vale
// rule is `Style.Rule`; the style alternation is open so a citation to a style
// the repo doesn't load is still read as a claim and reported.
var citation = regexp.MustCompile("`([A-Z][A-Za-z0-9-]*\\.[A-Za-z0-9_-]+|alex\\.[A-Za-z0-9*]+|write-good\\.[A-Za-z0-9]+|markdownlint|scripts/check_emdash\\.sh|MD[0-9]{3})`")

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

// learnMoreHeading is the navigation section every subpage ends with.
const learnMoreHeading = "Learn more"

// valeRule is a rule file under docs/.style/styles/Coder/.
type valeRule struct {
	name     string // for example "BrandNames"
	severity string // the rule file's level: field
	// defaultOn is true when the catch-all section leaves the rule enabled,
	// so the rule runs everywhere except the sections that turn it off.
	defaultOn bool
	// enabledIn lists the section globs that turn the rule on where the
	// catch-all section left it off.
	enabledIn []string
	// disabledIn lists the section globs that turn the rule off, either with
	// an explicit NO or with an empty BasedOnStyles that loads no style at
	// all.
	disabledIn []string
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

	landing, err := os.ReadFile(filepath.Join(styleGuide, landingPage))
	if err != nil {
		return nil, err
	}

	findings := checkCoverageAnchor(string(landing))

	pages, annotations, sources, err := subpages(styleGuide, string(landing))
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		findings = append(findings, checkFooterCoverage(page, sources[page])...)
	}

	return append(findings, checkClaims(pages, styles, rules, annotations, string(landing))...), nil
}

// checkCoverageAnchor reports a landing page that no longer carries the heading
// `make lint/prose` sends authors to.
func checkCoverageAnchor(landing string) []finding {
	for _, line := range unfenced(landing) {
		m := headingLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if anchor(m[2]) == coverageAnchor {
			return nil
		}
	}
	return []finding{{
		filepath.Join(styleGuide, landingPage), 0,
		fmt.Sprintf("no heading resolves to #%s, which `make lint/prose` points authors at. Restore the heading, or update the Makefile.", coverageAnchor),
	}}
}

// anchor renders the GitHub heading anchor for a heading's text.
func anchor(heading string) string {
	var b []rune
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b = append(b, r)
		case r == ' ':
			b = append(b, '-')
		}
	}
	return string(b)
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
			fmt.Sprintf("%q states a rule but carries no enforcement footer, so the coverage table can't count it. Add the footer, or mark the section Out of scope or Not a rule.", h.text),
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
	// open describes the fence that started the current block. CommonMark
	// closes a fence only with a run of the same character at least as long as
	// the opener, so a 3-backtick line inside a 4-backtick block is content, and
	// so is a `~~~` line inside a backtick block. The style guide teaches nested
	// fences with 4-backtick blocks, so both cases occur in the corpus the
	// checker reads.
	var open fence
	for i, line := range lines {
		if f, ok := parseFence(line); ok {
			switch {
			case open.char == 0:
				open = f
			case f.char == open.char && f.length >= open.length && !f.info:
				open = fence{}
			}
			continue
		}
		if open.char != 0 {
			continue
		}
		out[i] = line
	}
	return out
}

const (
	// minFenceRun is CommonMark's minimum fence length: three or more of the
	// fence character open a block.
	minFenceRun = 3
	// maxFenceIndent is the deepest indentation a fence may carry. Four spaces,
	// or a tab, make the line an indented code block instead.
	maxFenceIndent = 3
)

// fence is a Markdown code fence delimiter.
type fence struct {
	char   byte // '`' or '~'
	length int  // the run length of char
	info   bool // true when the line carries an info string, so it can only open
}

// parseFence returns the fence a line opens or closes with.
func parseFence(line string) (fence, bool) {
	if indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]; strings.Contains(indent, "\t") || len(indent) > maxFenceIndent {
		return fence{}, false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return fence{}, false
	}
	char := trimmed[0]
	if char != '`' && char != '~' {
		return fence{}, false
	}
	length := 0
	for length < len(trimmed) && trimmed[length] == char {
		length++
	}
	if length < minFenceRun {
		return fence{}, false
	}
	return fence{char: char, length: length, info: strings.TrimSpace(trimmed[length:]) != ""}, true
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
	for line := range strings.SplitSeq(src, "\n") {
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
			for s := range strings.SplitSeq(value, ",") {
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
	for line := range strings.SplitSeq(yaml, "\n") {
		if rest, ok := strings.CutPrefix(line, "level:"); ok {
			r.severity = strings.TrimSpace(rest)
			break
		}
	}
	r.defaultOn = true
	for section, value := range toggles[coderPackage+"."+name] {
		switch {
		case isDefaultSection(section) && value == "NO":
			r.defaultOn = false
		case value == "NO":
			r.disabledIn = append(r.disabledIn, section)
		case !isDefaultSection(section) && value == "YES":
			r.enabledIn = append(r.enabledIn, section)
		}
	}
	for section, loaded := range sectionStyles {
		if isDefaultSection(section) || len(loaded) > 0 {
			continue
		}
		r.disabledIn = append(r.disabledIn, section)
	}
	slices.Sort(r.enabledIn)
	slices.Sort(r.disabledIn)
	r.enabledIn = slices.Compact(r.enabledIn)
	r.disabledIn = slices.Compact(r.disabledIn)
	return r
}

// isDefaultSection reports whether a .vale.ini section is the catch-all that
// sets a rule's default state, rather than a path-scoped override.
func isDefaultSection(section string) bool {
	return section == "" || section == "*.md" || section == "*"
}

// active reports whether .vale.ini leaves the rule running anywhere. A rule
// that is off by default and re-enabled nowhere checks no file, so an
// annotation must not cite it as enforcement.
func (r valeRule) active() bool {
	return r.defaultOn || len(r.enabledIn) > 0
}

// scopeText renders the scope the configuration gives a rule, in the form the
// "Checks that run today" table uses. Comparing the cell against this string
// catches a cell that names the right paths with the wrong polarity, which a
// set comparison cannot see.
func (r valeRule) scopeText() string {
	quote := func(globs []string) string {
		var out []string
		for _, g := range globs {
			out = append(out, "`"+g+"`")
		}
		return strings.Join(out, " and ")
	}
	if !r.defaultOn {
		if len(r.enabledIn) == 0 {
			return "nowhere"
		}
		if overlaps := r.disabledOverlaps(); len(overlaps) > 0 {
			return quote(r.enabledIn) + " except " + quote(overlaps) + ", and nowhere else"
		}
		return quote(r.enabledIn) + " only"
	}
	if len(r.disabledIn) == 0 {
		return "`" + globalScope + "`"
	}
	return "`" + globalScope + "` except " + quote(r.disabledIn)
}

// underPath reports whether a glob sits inside a directory prefix, matching on
// path segments so `docs/foo**` does not swallow `docs/foobar/**`.
func underPath(glob, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return true
	}
	return glob == prefix || strings.HasPrefix(glob, prefix+"/")
}

// disabledOverlaps returns the disabling globs that fall inside a glob the rule
// is enabled in, so the scope text mentions a disable only where the rule would
// otherwise run. A glob ending in `*.md` covers one directory, so a disable in a
// subdirectory does not overlap it; only a `**` glob reaches down the tree.
func (r valeRule) disabledOverlaps() []string {
	var out []string
	for _, d := range r.disabledIn {
		for _, e := range r.enabledIn {
			if !strings.Contains(e, "**") {
				continue
			}
			prefix, _, _ := strings.Cut(e, "**")
			if underPath(d, prefix) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// parseAnnotations extracts every rule section footer from a subpage. Footers
// shown as examples inside a fenced code block are not annotations.
//
// A footer runs from its opening asterisk to the next line ending in one. On
// the opening line a second asterisk means the footer both opens and closes
// there, which is how a one-line footer such as *Enforced by X.* terminates.
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
		if !claimsEnforcement(lower) {
			continue
		}
		// A status marker binds to the citations in its own clause. A marker
		// that appears before any citation introduces the list that follows,
		// so it carries into later clauses until a clause re-asserts
		// enforcement.
		carry := false
		for _, segment := range splitClauses(sentence) {
			lower := strings.ToLower(segment)
			marked := strings.Contains(lower, "planned")
			first := citation.FindStringIndex(segment)
			introduces := len(first) < 2 || strings.Contains(strings.ToLower(segment[:first[0]]), "planned")
			switch {
			case marked && introduces:
				// The marker introduces the citations that follow it.
				carry = true
			case !marked && claimsEnforcement(lower):
				// The clause asserts enforcement of its own citations.
				carry = false
			}
			for _, m := range citation.FindAllStringSubmatch(segment, -1) {
				if marked || carry {
					planned = append(planned, m[1])
					continue
				}
				active = append(active, m[1])
			}
		}
	}
	return active, planned
}

// claimsEnforcement reports whether text asserts that something enforces a
// rule. The sentence gate and the clause-level carry stop share this vocabulary,
// so a clause that reasserts enforcement in the format's own wording always
// stops a carried marker.
func claimsEnforcement(lower string) bool {
	return strings.Contains(lower, "enforc") || strings.Contains(lower, "vale rule")
}

// splitClauses breaks a sentence into the clauses a status marker can bind to.
// A marker such as "(planned)" applies to the citations in its own clause, so a
// mixed annotation naming one active and one planned checker classifies
// correctly on both halves. splitSentences already breaks on semicolons, so a
// clause never spans one.
func splitClauses(sentence string) []string {
	var out []string
	start := 0
	inCode := false
	depth := 0
	for i := 0; i < len(sentence); i++ {
		switch sentence[i] {
		case '`':
			inCode = !inCode
		case '(':
			if !inCode {
				depth++
			}
		case ')':
			if !inCode && depth > 0 {
				depth--
			}
		}
		// A separator inside a code span or a parenthetical belongs to the
		// clause it sits in, not between two clauses.
		if inCode || depth > 0 {
			continue
		}
		size := 0
		switch {
		case sentence[i] == ',':
			size = 1
		case strings.HasPrefix(sentence[i:], " and "):
			size = len(" and ")
		case strings.HasPrefix(sentence[i:], " with "):
			size = len(" with ")
		}
		if size == 0 {
			continue
		}
		out = append(out, sentence[start:i])
		start = i + size
		i = start - 1
	}
	if rest := sentence[start:]; strings.TrimSpace(rest) != "" {
		out = append(out, rest)
	}
	return out
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
func classify(text string, enabled, loadedStyle map[string]bool) class {
	if documentationOnly.MatchString(text) {
		return classDocumentationOnly
	}
	active, planned := activeCitations(text)
	for _, c := range active {
		style, _, _ := strings.Cut(c, ".")
		switch {
		case isTool(c):
			return classTool
		case strings.HasPrefix(c, coderPackage+".") && enabled[strings.TrimPrefix(c, coderPackage+".")]:
			// A rule .vale.ini leaves off everywhere checks no file, so a
			// section citing it is not tool-checked, whatever the citation
			// claims. checkClaims reports the claim separately.
			return classTool
		case style != coderPackage && loadedStyle[style]:
			// A third-party rule counts as tool coverage once the repo loads
			// its style, the same as a Coder rule with a rule file.
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
	enabled := map[string]bool{}
	for _, r := range rules {
		ruleNames[r.name] = true
		enabled[r.name] = r.active()
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
			cls := classify(a.text, enabled, loadedStyle)
			switch cls {
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
							fmt.Sprintf("claims `%s` enforces this rule, but %s/%s.yml does not exist. Mark the citation (planned), or add the rule file.", c, rulesDir, rule),
						})
						continue
					}
					if !enabled[rule] {
						findings = append(findings, finding{
							a.file, a.line,
							fmt.Sprintf("claims `%s` enforces this rule, but %s leaves it off everywhere. Mark the citation (planned), or enable the rule.", c, valeConfig),
						})
						continue
					}
					// Only the section that owns a rule marks it cited. A
					// documentation-only section may cross-reference a rule
					// another section owns, and that reference must not stand
					// in for the owning section if the owner is ever removed.
					if cls != classDocumentationOnly {
						cited[rule] = true
					}
					continue
				}
				if !loadedStyle[style] {
					findings = append(findings, finding{
						a.file, a.line,
						fmt.Sprintf("claims `%s` enforces this rule, but %s does not load the %s style. Mark the citation (planned), or load the style.", c, valeConfig, style),
					})
				}
			}
		}
		counts[page] = [4]int{total, tool, planned, docOnly}
	}

	for _, r := range rules {
		// A rule .vale.ini leaves off everywhere runs nowhere, so no section
		// should claim it. Requiring a citation for it would deadlock a staged
		// rollout, where the rule file lands before the directories that
		// enable it.
		if !r.active() || cited[r.name] {
			continue
		}
		findings = append(findings, finding{
			filepath.Join(styleGuide, landingPage), 0,
			fmt.Sprintf("rule `%s.%s` is enabled but no style guide section cites it as active. Add the section, or remove the rule.", coderPackage, r.name),
		})
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
				fmt.Sprintf("table lists `%s.%s`, which has no rule file under %s. Add the rule file, or remove the row.", coderPackage, name, rulesDir),
			})
			continue
		}
		if !rule.active() {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("table lists `%s.%s` as running today, but %s leaves it off everywhere. Remove the row, or enable the rule.", coderPackage, name, valeConfig),
			})
			continue
		}
		if m := severityCell.FindStringSubmatch(cells[2]); len(m) < 2 || m[1] != rule.severity {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("severity for `%s.%s` is %q in the table but %q in the rule file. Update the table, or change the rule's level.", coderPackage, name, strings.TrimSpace(cells[2]), rule.severity),
			})
		}
		findings = append(findings, checkScopeCell(path, i+1, cells[3], *rule)...)
	}

	for _, r := range rules {
		if !r.active() || listed[r.name] {
			continue
		}
		findings = append(findings, finding{
			path, 0,
			fmt.Sprintf("rule `%s.%s` is enabled but missing from the checks table. Add the row, or disable the rule.", coderPackage, r.name),
		})
	}
	return findings
}

// checkScopeCell compares a scope cell against the scope the configuration
// gives the rule. The comparison is on the rendered text, not on the set of
// paths, so a cell that says "and" where .vale.ini says "except" is caught.
func checkScopeCell(path string, line int, cell string, rule valeRule) []finding {
	want := rule.scopeText()
	if normalizeScope(cell) == normalizeScope(want) {
		return nil
	}
	return []finding{{
		path, line,
		fmt.Sprintf("scope for `%s.%s` reads %q but %s gives it %q. Update the cell, or change the configuration.", coderPackage, rule.name, strings.TrimSpace(cell), valeConfig, want),
	}}
}

// normalizeScope collapses the whitespace a Markdown table adds for alignment.
func normalizeScope(s string) string {
	return strings.Join(strings.Fields(s), " ")
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
		row, ok := coverageRow(line)
		if !ok {
			continue
		}
		if row.total {
			if row.nums != wantTotal {
				findings = append(findings, finding{
					path, i + 1,
					fmt.Sprintf("total row is %v but the annotations sum to %v (rules, tool-checked, planned, documentation-only). Update the total row.", row.nums, wantTotal),
				})
			}
			seen["**Total**"] = true
			continue
		}
		if row.page == "" {
			findings = append(findings, finding{
				path, i + 1,
				"coverage row has counts but no link to a style guide section; write the first cell as [Section](./section.md)",
			})
			continue
		}
		want, known := counts[row.page]
		if !known {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("coverage row links %q, which is not a style guide section. Add the section, or remove the row.", row.page),
			})
			continue
		}
		seen[row.page] = true
		if row.nums != want {
			findings = append(findings, finding{
				path, i + 1,
				fmt.Sprintf("counts for %s are %v but the annotations give %v (rules, tool-checked, planned, documentation-only). Update the row.", row.page, row.nums, want),
			})
		}
	}

	for _, page := range pages {
		if !seen[page] {
			findings = append(findings, finding{path, 0, fmt.Sprintf("coverage table has no row for %s. Add the row.", page)})
		}
	}
	if !seen["**Total**"] {
		findings = append(findings, finding{path, 0, "coverage table has no total row. Add it."})
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
	_, rest, ok := strings.Cut(cell, "](./")
	if !ok {
		return "", false
	}
	page, _, ok := strings.Cut(rest, ")")
	if !ok {
		return "", false
	}
	return page, true
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

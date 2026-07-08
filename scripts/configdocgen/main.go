// Command configdocgen generates the Coder server configuration reference at
// docs/admin/setup/configuration-reference.md from codersdk.DeploymentValues.
// It lists every visible deployment option grouped by serpent group. Each
// option is rendered as a heading with its description followed by the
// environment variable, CLI flag, YAML key, and default that apply to it.
// Because the source is DeploymentValues, the page stays in sync as options
// change.
package main

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/flog"
	"github.com/coder/serpent"
)

const header = `<!-- DO NOT EDIT | GENERATED CONTENT -->
# Configuration reference

Coder server is configured primarily through environment variables. This page
lists every option so you can search by environment variable name, CLI flag, or
YAML key. For first-time setup guidance and worked examples, see
[Configure Control Plane Access](./index.md).

Each option can be set through one or more of the methods below. An option lists
only the methods that apply to it.

- An environment variable (recommended for production deployments running as a
  system service, container, or Helm chart).
- A CLI flag passed to ` + "`coder server`" + ` (useful for one-off invocations
  and local development).
- A key in a YAML configuration file passed with ` + "`--config`" + `.

For a full description of each option's accepted values and behavior, follow the
flag link into the [` + "`coder server`" + ` CLI reference](../../reference/cli/server.md).

Deprecated options are listed at the end of each section.

`

// generalSection holds options that do not belong to a serpent group.
const generalSection = "General"

// option is the normalized data needed to render one deployment option.
type option struct {
	title      string // short, sentence-case heading text
	env        string
	flagName   string
	flagAnchor string
	yaml       string
	defValue   string
	desc       string
	deprecated bool
	sortKey    string // original serpent name, for stable ordering
}

// node is one section of the reference: a serpent group (or the synthetic
// "General" group) with its direct options and any child sections.
type node struct {
	name     string // raw group name (leaf); sentence-cased at render time
	intro    string // group description, if any
	options  []option
	children []*node
	childIdx map[string]*node
}

func newNode(name string) *node {
	return &node{name: name, childIdx: map[string]*node{}}
}

// child returns the named child section, creating it on first use.
func (n *node) child(name string) *node {
	if c, ok := n.childIdx[name]; ok {
		return c
	}
	c := newNode(name)
	n.childIdx[name] = c
	n.children = append(n.children, c)
	return c
}

// prepareEnv mirrors scripts/clidocgen so the generated defaults do not
// depend on the generating host. Without it, defaults derived from
// os.UserCacheDir and the config dir embed the local home directory.
func prepareEnv() {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CODER_") {
			name, _, _ := strings.Cut(env, "=")
			if err := os.Unsetenv(name); err != nil {
				panic(err)
			}
		}
	}

	err := os.Setenv("CLIDOCGEN_CACHE_DIRECTORY", "~/.cache")
	if err != nil {
		panic(err)
	}
	err = os.Setenv("CLIDOCGEN_CONFIG_DIRECTORY", "~/.config/coderv2")
	if err != nil {
		panic(err)
	}
	err = os.Setenv("TMPDIR", "/tmp")
	if err != nil {
		panic(err)
	}
}

func main() {
	prepareEnv()

	out := flag.String("out", "docs/admin/setup/configuration-reference.md", "path to write the generated reference page")
	flag.Parse()

	var vals codersdk.DeploymentValues
	opts := vals.Options()

	root := buildTree(opts)
	body := render(root)

	content := header + body
	content = strings.TrimRight(content, "\n") + "\n"
	if err := atomicwrite.File(*out, []byte(content)); err != nil {
		flog.Fatalf("write %s: %v", *out, err)
	}
	flog.Successf("wrote %s", *out)
}

// buildTree groups options into a section tree, skipping hidden options and
// options that have no environment variable, flag, or YAML key (those cannot
// be set by an operator).
func buildTree(opts serpent.OptionSet) *node {
	root := newNode("")
	for _, opt := range opts {
		if opt.Hidden {
			continue
		}
		if opt.Env == "" && opt.Flag == "" && opt.YAML == "" {
			continue
		}
		sec := sectionFor(root, opt.Group)
		sec.options = append(sec.options, toOption(opt))
	}
	sortTree(root)
	return root
}

// sectionFor returns the section node for an option's group, creating the
// chain of ancestor sections as needed. Options with no group (or an unnamed
// group) live in the General section.
func sectionFor(root *node, g *serpent.Group) *node {
	if g == nil {
		return root.child(generalSection)
	}
	cur := root
	for _, ancestor := range g.Ancestry() {
		if ancestor.Name == "" {
			return root.child(generalSection)
		}
		cur = cur.child(ancestor.Name)
		if cur.intro == "" {
			cur.intro = collapse(ancestor.Description)
		}
	}
	return cur
}

func toOption(opt serpent.Option) option {
	var flagName, flagAnchor string
	if opt.Flag != "" {
		flagName = "--" + opt.Flag
		// clidocgen renders a flag heading as "### -s, --flag" when it has a
		// shorthand and "### --flag" otherwise, so the anchor must include the
		// shorthand to match.
		flagAnchor = "--" + opt.Flag
		if opt.FlagShorthand != "" {
			flagAnchor = "-" + opt.FlagShorthand + "---" + opt.Flag
		}
	}

	def := opt.Default
	if def == "" && opt.DefaultFn != nil {
		// DefaultFn results depend on the host environment, so evaluating them
		// here would leak host-specific values. Send the reader to the CLI
		// reference for the resolved default instead.
		def = "(computed at runtime)"
	}

	return option{
		title:      shortTitle(opt),
		env:        opt.Env,
		flagName:   flagName,
		flagAnchor: flagAnchor,
		yaml:       opt.YAMLPath(),
		defValue:   def,
		desc:       collapse(opt.Description),
		deprecated: isDeprecated(opt),
		sortKey:    opt.Name,
	}
}

// isDeprecated reports whether an option is deprecated. serpent tracks
// replacements in UseInstead, and codersdk also marks some options by leading
// the description with "Deprecated".
func isDeprecated(opt serpent.Option) bool {
	if len(opt.UseInstead) > 0 {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(opt.Description)), "deprecated")
}

// sortTree orders sections and their options. General sorts first and
// Dangerous last among top-level sections; every other section is
// alphabetical. Within a section, active options come before deprecated ones,
// each alphabetical by their original name.
func sortTree(n *node) {
	slices.SortStableFunc(n.children, func(a, b *node) int {
		if c := cmp.Compare(sectionRank(a.name), sectionRank(b.name)); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	for _, c := range n.children {
		slices.SortStableFunc(c.options, func(a, b option) int {
			if a.deprecated != b.deprecated {
				if a.deprecated {
					return 1
				}
				return -1
			}
			return strings.Compare(a.sortKey, b.sortKey)
		})
		sortTree(c)
	}
}

// sectionRank fixes the display order of top-level sections. General comes
// first because it holds the most common first-time setup options (Postgres,
// cache directory, access URL). The Dangerous group comes last, regardless of
// its emoji prefix, so the reference does not steer operators toward risky
// settings. Every other section sorts alphabetically between them.
func sectionRank(name string) int {
	switch {
	case name == generalSection:
		return -1
	case strings.HasSuffix(name, "Dangerous"):
		return 1
	default:
		return 0
	}
}

func render(root *node) string {
	var b strings.Builder
	for _, sec := range root.children {
		renderNode(&b, sec, 2)
	}
	return b.String()
}

func renderNode(b *strings.Builder, n *node, level int) {
	_, _ = fmt.Fprintf(b, "%s %s\n\n", strings.Repeat("#", level), sentenceCase(n.name))
	if n.intro != "" {
		_, _ = b.WriteString(n.intro)
		_, _ = b.WriteString("\n\n")
	}
	for _, opt := range n.options {
		renderOption(b, opt, level+1)
	}
	for _, c := range n.children {
		renderNode(b, c, level+1)
	}
}

func renderOption(b *strings.Builder, opt option, level int) {
	_, _ = fmt.Fprintf(b, "%s %s\n\n", strings.Repeat("#", level), opt.title)

	desc := opt.desc
	if opt.deprecated {
		desc = emphasizeDeprecation(desc)
	}
	if desc != "" {
		_, _ = b.WriteString(desc)
		_, _ = b.WriteString("\n\n")
	}

	if opt.env != "" {
		_, _ = fmt.Fprintf(b, "- Environment variable: `%s`\n", opt.env)
	}
	if opt.flagName != "" {
		_, _ = fmt.Fprintf(b, "- CLI flag: [`%s`](../../reference/cli/server.md#%s)\n", opt.flagName, opt.flagAnchor)
	}
	if opt.yaml != "" {
		_, _ = fmt.Fprintf(b, "- YAML key: `%s`\n", opt.yaml)
	}
	if opt.defValue != "" {
		_, _ = fmt.Fprintf(b, "- Default value: `%s`\n", opt.defValue)
	}
	_, _ = b.WriteString("\n")
}

// emphasizeDeprecation bolds the leading "Deprecated" marker in a description
// so a deprecated option reads clearly. Trailing text is left unbolded so the
// paragraph is not a lone emphasis span (markdownlint MD036).
func emphasizeDeprecation(desc string) string {
	const marker = "Deprecated"
	if len(desc) >= len(marker) && strings.EqualFold(desc[:len(marker)], marker) {
		if strings.TrimSpace(desc[len(marker):]) != "" {
			return "**" + desc[:len(marker)] + "**" + desc[len(marker):]
		}
		return desc
	}
	if desc == "" {
		return "Deprecated."
	}
	return "**Deprecated.** " + desc
}

// shortTitle strips the redundant group prefix from an option name and returns
// it in sentence case, e.g. "AI Gateway Send Actor Headers" becomes
// "Send actor headers".
func shortTitle(opt serpent.Option) string {
	name := opt.Name
	if opt.Group != nil {
		name = stripGroupPrefix(name, opt.Group)
	}
	return sentenceCase(name)
}

// stripGroupPrefix removes the group name that many option names repeat. For
// space-prefixed names like "AI Gateway Send Actor Headers" it drops the
// longest matching ancestor chain ("AI Gateway"). For colon-prefixed names
// like "Notifications: Email TLS: StartTLS" it drops every segment up to the
// last ": " once the leading segment belongs to the top-level group. Names
// that do not repeat the group are returned unchanged.
func stripGroupPrefix(name string, g *serpent.Group) string {
	ancestry := g.Ancestry()
	if len(ancestry) == 0 {
		return name
	}
	names := make([]string, len(ancestry))
	for i, a := range ancestry {
		names[i] = a.Name
	}

	if before, _, ok := strings.Cut(name, ": "); ok {
		// Only treat the colon as a group separator when the leading segment
		// belongs to the top-level group. This avoids mangling meaningful
		// colons such as "Health Check Threshold: Database".
		if top := normalize(names[0]); top != "" && strings.HasPrefix(normalize(before), top) {
			if idx := strings.LastIndex(name, ": "); idx >= 0 {
				if rest := strings.TrimSpace(name[idx+len(": "):]); rest != "" {
					return rest
				}
			}
		}
	}

	// Try the longest ancestor suffix chain first (start == 0 is the full
	// path) so the most specific prefix wins.
	for start := range names {
		prefix := strings.Join(names[start:], " ") + " "
		if rest, ok := cutFold(name, prefix); ok {
			if rest = strings.TrimSpace(rest); rest != "" {
				return rest
			}
		}
	}
	return name
}

// properNoun lists words that keep their capitalization in sentence case.
// They have ordinary title-case shape, so keepWord's acronym check would not
// otherwise catch them.
var properNoun = map[string]bool{
	"anthropic":   true,
	"bedrock":     true,
	"claude":      true,
	"coder":       true,
	"google":      true,
	"helm":        true,
	"honeycomb":   true,
	"maven":       true,
	"postgres":    true,
	"prometheus":  true,
	"stackdriver": true,
	"tailscale":   true,
	"terraform":   true,
	"wireguard":   true,
}

// sentenceCase lowercases a heading's words after the first while preserving
// the first word, acronyms and mixed-case tokens (URL, TLS, OAuth2, OpenID,
// GitHub), and known proper nouns. It runs at generation time so headings need
// no manual or AI pass.
func sentenceCase(s string) string {
	words := strings.Fields(s)
	seenFirst := false
	for i, w := range words {
		if !seenFirst {
			// Keep any leading symbols (e.g. an emoji) and the first real word.
			if hasLetter(w) {
				seenFirst = true
			}
			continue
		}
		if !keepWord(w) {
			words[i] = strings.ToLower(w)
		}
	}
	return strings.Join(words, " ")
}

// keepWord reports whether a word must keep its capitalization: proper nouns,
// all-caps or mixed-case acronyms (URL, GitHub), and tokens with digits
// (OAuth2).
func keepWord(w string) bool {
	core := strings.Trim(w, "()[]{}:;,.\"'")
	if core == "" {
		return true
	}
	if properNoun[strings.ToLower(core)] {
		return true
	}
	for i, r := range core {
		if i == 0 {
			continue
		}
		if unicode.IsUpper(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// collapse trims a string and collapses internal runs of whitespace to a
// single space so multi-line source text renders as one paragraph.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalize lowercases a string and drops everything but letters and digits,
// so prefixes can be compared regardless of spacing, case, or punctuation.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			_, _ = b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// cutFold trims prefix from s using a case-insensitive comparison, reporting
// whether it was present.
func cutFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

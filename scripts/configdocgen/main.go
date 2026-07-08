// Command configdocgen generates the Coder server configuration reference at
// docs/admin/setup/configuration-reference.md from codersdk.DeploymentValues.
// It lists every visible deployment option grouped by serpent group, with its
// environment variable, CLI flag, YAML key, and default. Because the source is
// DeploymentValues, the page stays in sync as options change.
package main

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

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

Most options can be set through any of the following. Where a method does not
apply to an option, that column shows ` + "`-`" + `.

- An environment variable (recommended for production deployments running as a
  system service, container, or Helm chart).
- A CLI flag passed to ` + "`coder server`" + ` (useful for one-off invocations
  and local development).
- A key in a YAML configuration file passed with ` + "`--config`" + `.

For a full description of each option's accepted values and behavior, follow
the flag link into [` + "`coder server`" + ` CLI reference](../../reference/cli/server.md).

`

// row carries the rendered cells for one option.
type row struct {
	name     string
	env      string
	flag     string
	yaml     string
	defValue string
	desc     string
}

// section is one heading level of options, grouped by serpent.Group.
type section struct {
	title string
	rows  []row
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

	sections := buildSections(opts)
	body := renderSections(sections)

	if err := atomicwrite.File(*out, []byte(header+body)); err != nil {
		flog.Fatalf("write %s: %v", *out, err)
	}
	flog.Successf("wrote %s", *out)
}

// buildSections groups options by their serpent group, skipping hidden
// options and options that have no environment variable, flag, or YAML key
// (those cannot be set by an operator).
func buildSections(opts serpent.OptionSet) []section {
	bySection := map[string]*section{}
	var order []string

	for _, opt := range opts {
		if opt.Hidden {
			continue
		}
		if opt.Env == "" && opt.Flag == "" && opt.YAML == "" {
			continue
		}

		title := "General"
		if opt.Group != nil {
			full := opt.Group.FullName()
			if full != "" {
				title = full
			}
		}
		if _, ok := bySection[title]; !ok {
			bySection[title] = &section{title: title}
			order = append(order, title)
		}
		bySection[title].rows = append(bySection[title].rows, optionToRow(opt))
	}

	for _, key := range order {
		slices.SortFunc(bySection[key].rows, func(a, b row) int {
			return strings.Compare(a.name, b.name)
		})
	}

	slices.SortStableFunc(order, func(a, b string) int {
		if c := cmp.Compare(sectionRank(a), sectionRank(b)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})

	result := make([]section, 0, len(order))
	for _, key := range order {
		result = append(result, *bySection[key])
	}
	return result
}

// sectionRank fixes the display order of sections. General comes first because
// it holds the most common first-time setup options (Postgres, cache
// directory, access URL). The Dangerous group comes last, regardless of its
// emoji prefix, so the reference does not steer operators toward risky
// settings. Every other section sorts alphabetically between them.
func sectionRank(title string) int {
	switch {
	case title == "General":
		return -1
	case strings.HasSuffix(title, "Dangerous"):
		return 1
	default:
		return 0
	}
}

func optionToRow(opt serpent.Option) row {
	flagCell := "-"
	if opt.Flag != "" {
		// clidocgen renders a flag heading as "### -s, --flag" when it has a
		// shorthand and "### --flag" otherwise, so the anchor must include the
		// shorthand to match.
		anchor := "--" + opt.Flag
		if opt.FlagShorthand != "" {
			anchor = "-" + opt.FlagShorthand + "---" + opt.Flag
		}
		flagCell = fmt.Sprintf("[`--%s`](../../reference/cli/server.md#%s)", opt.Flag, anchor)
	}

	def := opt.Default
	if def == "" && opt.DefaultFn != nil {
		// DefaultFn results depend on the host environment, so evaluating them
		// here would leak host-specific values. Send the reader to the CLI
		// reference for the resolved default instead.
		def = "(computed at runtime)"
	}

	return row{
		name:     escapePipe(opt.Name),
		env:      codeCell(opt.Env),
		flag:     flagCell,
		yaml:     codeCell(opt.YAMLPath()),
		defValue: codeCell(def),
		desc:     sanitizeDesc(opt.Description),
	}
}

func codeCell(s string) string {
	if s == "" {
		return "-"
	}
	return "`" + s + "`"
}

// sanitizeDesc collapses whitespace and escapes pipes so a description renders
// inside a single markdown table cell.
func sanitizeDesc(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = escapePipe(s)
	if s == "" {
		return "-"
	}
	return s
}

// escapePipe escapes the markdown table cell delimiter so a value cannot break
// the surrounding row.
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

func renderSections(sections []section) string {
	var b strings.Builder
	for _, sec := range sections {
		_, _ = fmt.Fprintf(&b, "## %s\n\n", sec.title)
		_, _ = b.WriteString("| Setting | Env var | Flag | YAML | Default | Description |\n")
		_, _ = b.WriteString("|---|---|---|---|---|---|\n")
		for _, r := range sec.rows {
			_, _ = fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				r.name, r.env, r.flag, r.yaml, r.defValue, r.desc)
		}
		_, _ = b.WriteString("\n")
	}
	return b.String()
}

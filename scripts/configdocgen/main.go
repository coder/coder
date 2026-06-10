// Command configdocgen produces a single-page configuration reference for
// Coder server. The page lists every visible deployment option grouped by
// its serpent Group, with columns for the environment variable, CLI flag,
// YAML key, default, and description. The intent is to give operators a
// single searchable lookup table; for the full per-flag detail, the page
// links back to docs/reference/cli/server.md, which the existing
// scripts/clidocgen already generates from the same source.
//
// The source of truth is codersdk.DeploymentValues, so this generator stays
// in sync automatically whenever options are added, renamed, or removed.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
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

Every option below can be set via:

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
	// Unset CODER_ environment variables
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CODER_") {
			split := strings.SplitN(env, "=", 2)
			if err := os.Unsetenv(split[0]); err != nil {
				panic(err)
			}
		}
	}

	// Override default OS values to ensure the same generated results.
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
			s := &section{title: title}
			bySection[title] = s
			order = append(order, title)
		}
		bySection[title].rows = append(bySection[title].rows, optionToRow(opt))
	}

	for _, key := range order {
		s := bySection[key]
		sort.Slice(s.rows, func(i, j int) bool {
			return s.rows[i].name < s.rows[j].name
		})
	}

	sort.Strings(order)
	// Put General first because it carries the most common first-time setup
	// options (Postgres, cache directory, support links, etc.).
	for i, key := range order {
		if key == "General" && i != 0 {
			order = append([]string{"General"}, append(order[:i], order[i+1:]...)...)
			break
		}
	}
	result := make([]section, 0, len(order))
	for _, key := range order {
		result = append(result, *bySection[key])
	}
	return result
}

func optionToRow(opt serpent.Option) row {
	flagCell := "-"
	if opt.Flag != "" {
		flagCell = fmt.Sprintf("[`--%s`](../../reference/cli/server.md#--%s)", opt.Flag, opt.Flag)
	}

	def := opt.Default
	if def == "" && opt.DefaultFn != nil {
		// DefaultFn results depend on the host environment, so we cannot
		// safely evaluate them here. Mark them as dynamic so readers know to
		// check the CLI reference for the resolved default.
		def = "(dynamic)"
	}

	return row{
		name:     opt.Name,
		env:      codeCell(opt.Env),
		flag:     flagCell,
		yaml:     codeCell(opt.YAMLPath()),
		defValue: codeCell(def),
		desc:     sanitizeDesc(opt.Description),
	}
}

// codeCell wraps s in backticks for a markdown table cell, or returns a dash
// placeholder when s is empty.
func codeCell(s string) string {
	if s == "" {
		return "-"
	}
	return "`" + s + "`"
}

// sanitizeDesc collapses whitespace and escapes pipes so the description fits
// inside a markdown table cell. Long sentences are kept; readers can follow
// the flag link for canonical wording.
func sanitizeDesc(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.ReplaceAll(s, "|", `\|`)
	if s == "" {
		return "-"
	}
	return s
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

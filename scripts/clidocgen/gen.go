package main

import (
	"cmp"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/acarl005/stripansi"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/flog"
	"github.com/coder/serpent"
)

//go:embed command.tpl
var commandTemplateRaw string

var commandTemplate *template.Template

func init() {
	commandTemplate = template.Must(
		template.New("command.tpl").Funcs(template.FuncMap{
			"visibleSubcommands": visibleSubcommands,
			"visibleOptions": func(cmd *serpent.Command) []serpent.Option {
				var visible []serpent.Option
				for _, opt := range cmd.Options {
					if opt.Hidden {
						continue
					}
					// Skip YAML-only options that have no CLI flag; documenting them
					// as if they were flags is misleading in the CLI reference.
					if opt.Flag == "" && opt.FlagShorthand == "" {
						continue
					}
					visible = append(visible, opt)
				}
				return visible
			},
			"newLinesToBr": func(s string) string {
				return strings.ReplaceAll(s, "\n", "<br/>")
			},
			"wrapCode": func(s string) string {
				return fmt.Sprintf("<code>%s</code>", s)
			},
			"subcommandLink": subcommandLink,
			// frontMatter renders the page's YAML front matter through the
			// shared docgenenv emitter, so the CLI and API generators cannot
			// drift on field set, ordering, or escaping. The CLI index mirrors
			// the "Command Line" manifest route (main populates cliIndexRoute
			// before the template runs); every other page uses the command's
			// own name and short description.
			"frontMatter": func(cmd *serpent.Command) string {
				if cmd.FullName() == "coder" {
					return docgenenv.FrontMatter(cliIndexRoute)
				}
				return docgenenv.FrontMatter(cliCommandRoute(cmd))
			},
			// generatedContentBanner emits the shared body banner that marks
			// the whole page as generated, sourced from one constant so the CLI
			// and API generators cannot drift on its wording.
			"generatedContentBanner": func() string {
				return docgenenv.GeneratedContentBanner
			},
			"tableHeader": func() string {
				return `| | |
| --- | --- |`
			},
			"typeHelper": func(opt *serpent.Option) string {
				switch v := opt.Value.(type) {
				case *serpent.Enum:
					return strings.Join(v.Choices, "\\|")
				case *serpent.EnumArray:
					return fmt.Sprintf("[%s]", strings.Join(v.Choices, "\\|"))
				default:
					return v.Type()
				}
			},
		},
		).Parse(strings.TrimSpace(commandTemplateRaw)),
	)
}

// visibleSubcommands returns cmd's direct subcommands that are not hidden.
// Hidden commands (and their subtrees) get no reference page.
func visibleSubcommands(cmd *serpent.Command) []*serpent.Command {
	var visible []*serpent.Command
	for _, sub := range cmd.Children {
		if sub.Hidden {
			continue
		}
		visible = append(visible, sub)
	}
	return visible
}

// inlineCode wraps s in backticks. Docs renderers show a title that is
// entirely wrapped in one pair of backticks as inline code.
func inlineCode(s string) string {
	return "`" + s + "`"
}

// cliCommandRoute maps a serpent command to the docgenenv.Route whose per-page
// metadata the CLI generator mirrors into that command's page front matter.
// The page title is the full invocation (for example "`coder users list`"), so
// the page reads as a command even though its sidebar entry only shows the
// last verb (see cliManifestRoute).
func cliCommandRoute(cmd *serpent.Command) docgenenv.Route {
	return docgenenv.Route{
		Title:       inlineCode(cmd.FullName()),
		Description: cmd.Short,
	}
}

// cliManifestRoute builds the manifest nav entry for cmd and, recursively, its
// visible subcommands, so the sidebar nests the same way the command tree
// does. Each entry is titled with only its own verb (for example "`list`"
// under "`users`") because its ancestors already appear above it in the nav.
func cliManifestRoute(cmd *serpent.Command) docgenenv.Route {
	return docgenenv.Route{
		Title:       inlineCode(cmd.Name()),
		Description: cmd.Short,
		Path:        path.Join("reference/cli", fmtDocFilename(cmd)),
		Children:    cliManifestChildren(cmd),
	}
}

// cliManifestChildren returns the manifest entries for cmd's visible
// subcommands, sorted by name so the output is deterministic.
func cliManifestChildren(cmd *serpent.Command) []docgenenv.Route {
	subs := visibleSubcommands(cmd)
	slices.SortFunc(subs, func(a, b *serpent.Command) int {
		return cmp.Compare(a.Name(), b.Name())
	})
	var children []docgenenv.Route
	for _, sub := range subs {
		children = append(children, cliManifestRoute(sub))
	}
	return children
}

// cliIndexRouteFrom returns the CLI index page's route: a copy of the "Command
// Line" manifest route with its nav children dropped. Copying the whole route
// instead of enumerating fields means the index front matter mirrors every
// current and future per-page field (including curated icon_path and state)
// automatically, so it can't drift from the shared docgenenv.FrontMatter
// emitter the way a hand-written field list would.
func cliIndexRouteFrom(cmdLine docgenenv.Route) docgenenv.Route {
	cmdLine.Children = nil
	return cmdLine
}

// fmtDocFilename returns the path of cmd's reference page relative to the CLI
// reference directory. Pages nest by command path so the URL mirrors the
// invocation: "coder users list" is users/list.md, and a command with visible
// subcommands is the index.md of its own directory (users/index.md) so its
// subcommands' pages sit beneath it.
func fmtDocFilename(cmd *serpent.Command) string {
	var parts []string
	for c := cmd; c.Parent != nil; c = c.Parent {
		parts = append([]string{c.Name()}, parts...)
	}
	if len(parts) == 0 || len(visibleSubcommands(cmd)) > 0 {
		return path.Join(append(parts, "index.md")...)
	}
	return path.Join(parts...) + ".md"
}

// subcommandLink returns the link from a parent command's page to sub's page.
// A parent page is always the index.md of the directory its subcommands live
// in, so the link is relative to that directory.
func subcommandLink(sub *serpent.Command) string {
	parentDir := path.Dir(fmtDocFilename(sub.Parent))
	rel := fmtDocFilename(sub)
	if parentDir != "." {
		rel = strings.TrimPrefix(rel, parentDir+"/")
	}
	return "./" + rel
}

func writeCommand(w io.Writer, cmd *serpent.Command) error {
	var b strings.Builder
	err := commandTemplate.Execute(&b, cmd)
	if err != nil {
		return err
	}
	content := stripansi.Strip(b.String())

	// Remove the version and its right space, since during this script running
	// there is no build info available
	content = strings.ReplaceAll(content, buildinfo.Version()+" ", "")

	// Remove references to the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, cwd, ".")

	homedir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, homedir, "~")

	_, err = w.Write([]byte(content))
	return err
}

func genTree(dir string, cmd *serpent.Command, wroteLog map[string]*serpent.Command) error {
	if cmd.Hidden {
		return nil
	}

	pagePath := filepath.Join(dir, filepath.FromSlash(fmtDocFilename(cmd)))
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
		return xerrors.Errorf("create directory for %q: %w", pagePath, err)
	}

	var buf strings.Builder
	err := writeCommand(&buf, cmd)
	if err != nil {
		return err
	}

	err = atomicwrite.File(pagePath, []byte(buf.String()))
	if err != nil {
		return err
	}

	flog.Successf(
		"wrote\t%s",
		pagePath,
	)
	wroteLog[pagePath] = cmd
	for _, sub := range cmd.Children {
		err = genTree(dir, sub, wroteLog)
		if err != nil {
			return err
		}
	}
	return nil
}

package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/acarl005/stripansi"

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
			"visibleSubcommands": func(cmd *serpent.Command) []*serpent.Command {
				var visible []*serpent.Command
				for _, sub := range cmd.Children {
					if sub.Hidden {
						continue
					}
					visible = append(visible, sub)
				}
				return visible
			},
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
			"commandURI": fmtDocFilename,
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

func fullName(cmd *serpent.Command) string {
	if cmd.FullName() == "coder" {
		return "coder"
	}
	return strings.TrimPrefix(cmd.FullName(), "coder ")
}

// cliCommandRoute maps a serpent command to the docgenenv.Route whose per-page
// metadata the CLI generator mirrors into that command's page front matter.
// main layers the manifest Path onto the same value when it rebuilds the nav
// tree, so the per-command field mapping lives in exactly one place.
func cliCommandRoute(cmd *serpent.Command) docgenenv.Route {
	return docgenenv.Route{
		Title:       fullName(cmd),
		Description: cmd.Short,
	}
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

func fmtDocFilename(cmd *serpent.Command) string {
	if cmd.FullName() == "coder" {
		// Special case for index.
		return "./index.md"
	}
	name := strings.ReplaceAll(fullName(cmd), " ", "_")
	return fmt.Sprintf("%s.md", name)
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

	path := filepath.Join(dir, fmtDocFilename(cmd))

	var buf strings.Builder
	err := writeCommand(&buf, cmd)
	if err != nil {
		return err
	}

	err = atomicwrite.File(path, []byte(buf.String()))
	if err != nil {
		return err
	}

	flog.Successf(
		"wrote\t%s",
		path,
	)
	wroteLog[path] = cmd
	for _, sub := range cmd.Children {
		err = genTree(dir, sub, wroteLog)
		if err != nil {
			return err
		}
	}
	return nil
}

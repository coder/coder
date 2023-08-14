package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	_ "embed"

	"github.com/acarl005/stripansi"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/flog"
)

//go:embed command.tpl
var commandTemplateRaw string

var commandTemplate *template.Template

func init() {
	commandTemplate = template.Must(
		template.New("command.tpl").Funcs(template.FuncMap{
			"visibleSubcommands": func(cmd *clibase.Cmd) []*clibase.Cmd {
				var visible []*clibase.Cmd
				for _, sub := range cmd.Children {
					if sub.Hidden {
						continue
					}
					visible = append(visible, sub)
				}
				return visible
			},
			"visibleOptions": func(cmd *clibase.Cmd) []clibase.Option {
				var visible []clibase.Option
				for _, opt := range cmd.Options {
					if opt.Hidden {
						continue
					}
					visible = append(visible, opt)
				}
				return visible
			},
			"atRoot": func(cmd *clibase.Cmd) bool {
				return cmd.FullName() == "coder"
			},
			"newLinesToBr": func(s string) string {
				return strings.ReplaceAll(s, "\n", "<br/>")
			},
			"wrapCode": func(s string) string {
				return fmt.Sprintf("<code>%s</code>", s)
			},
			"commandURI": func(cmd *clibase.Cmd) string {
				return fmtDocFilename(cmd)
			},
			"fullName": fullName,
			"tableHeader": func() string {
				return `| | |
| --- | --- |`
			},
		},
		).Parse(strings.TrimSpace(commandTemplateRaw)),
	)
}

func fullName(cmd *clibase.Cmd) string {
	if cmd.FullName() == "coder" {
		return "coder"
	}
	return strings.TrimPrefix(cmd.FullName(), "coder ")
}

func fmtDocFilename(cmd *clibase.Cmd) string {
	if cmd.FullName() == "coder" {
		// Special case for index.
		return "../cli.md"
	}
	name := strings.ReplaceAll(fullName(cmd), " ", "_")
	return fmt.Sprintf("%s.md", name)
}

func writeCommand(w io.Writer, cmd *clibase.Cmd) error {
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

func genTree(dir string, cmd *clibase.Cmd, wroteLog map[string]*clibase.Cmd) error {
	if cmd.Hidden {
		return nil
	}

	path := filepath.Join(dir, fmtDocFilename(cmd))
	// Write out root.
	fi, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644,
	)
	if err != nil {
		return err
	}
	defer fi.Close()

	err = writeCommand(fi, cmd)
	if err != nil {
		return err
	}

	flog.Successf(
		"wrote\t%s",
		fi.Name(),
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

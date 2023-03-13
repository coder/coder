package main

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "embed"

	"github.com/acarl005/stripansi"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/clibase"
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
				return strings.TrimSuffix(
					fmtDocFilename(cmd),
					".md",
				)
			},
		},
		).Parse(strings.TrimSpace(commandTemplateRaw)),
	)
}

func fmtDocFilename(cmd *clibase.Cmd) string {
	if cmd.FullName() == "coder" {
		// Special case for index.
		return "../cli.md"
	}
	name := strings.ReplaceAll(cmd.FullName(), " ", "_")
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

	_, err = w.Write([]byte(content))
	return err
}

func genTree(basePath string, cmd *clibase.Cmd, wroteLog map[string]struct{}) error {
	if cmd.Hidden {
		return nil
	}

	// Write out root.
	fi, err := os.OpenFile(
		filepath.Join(basePath, fmtDocFilename(cmd)),
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
	for _, sub := range cmd.Children {
		err = genTree(basePath, sub, wroteLog)
		if err != nil {
			return err
		}
	}
	return nil
}

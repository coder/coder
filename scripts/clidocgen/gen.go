package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	_ "embed"

	"github.com/coder/flog"
)

//go:embed command.tpl
var commandTemplateRaw string

var commandTemplate *template.Template

func init() {
	commandTemplate = template.Must(
		template.New("command.tpl").Funcs(template.FuncMap{
			"newLinesToBr": func(s string) string {
				return strings.ReplaceAll(s, "\n", "<br/>")
			},
			"wrapCode": func(s string) string {
				return fmt.Sprintf("<code>%s</code>", s)
			},
		},
		).Parse(strings.TrimSpace(commandTemplateRaw)),
	)
}

func writeCommand(w io.Writer, cmd *cobra.Command) error {
	var flags []*pflag.Flag
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, f)
	})
	err := commandTemplate.Execute(w, map[string]any{
		"Name":  fullCommandName(cmd),
		"Cmd":   cmd,
		"Flags": flags,
	})
	return err
}

func fullCommandName(cmd *cobra.Command) string {
	name := cmd.Name()
	if cmd.Parent() != nil {
		return fullCommandName(cmd.Parent()) + " " + name
	}
	return name
}

func fmtDocFilename(cmd *cobra.Command) string {
	fullName := fullCommandName(cmd)
	if fullName == "coder" {
		// Special case for index.
		return "../cli.md"
	}
	name := strings.ReplaceAll(fullName, " ", "_")
	return fmt.Sprintf("%s.md", name)
}

func generateDocsTree(rootCmd *cobra.Command, basePath string) error {
	// Write out root.
	fi, err := os.OpenFile(
		filepath.Join(basePath, fmtDocFilename(rootCmd)),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644,
	)
	if err != nil {
		return err
	}
	defer fi.Close()

	err = writeCommand(fi, rootCmd)
	if err != nil {
		return err
	}

	flog.Info("Generated docs for %q at %v", fullCommandName(rootCmd), fi.Name())

	// Recursively generate docs.
	for _, subcommand := range rootCmd.Commands() {
		err = generateDocsTree(subcommand, basePath)
		if err != nil {
			return err
		}
	}
	return nil
}

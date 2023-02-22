package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

const commandTemplate = `
# {{ .Name }}
`

type commandTemplateParams struct {
	Name string
}

var commandTemplateParsed *template.Template

func init() {
	commandTemplateParsed = template.Must(template.New("command").Parse(commandTemplate))
}

func fmtDocFilename(cmd *cobra.Command) string {
	return fmt.Sprintf("%s.md", cmd.Name())
}

func generateDocsTree(rootCmd *cobra.Command, basePath string) error {
	fi, err := os.OpenFile(
		filepath.Join(basePath, fmtDocFilename(rootCmd)),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644,
	)
	if err != nil {
		return err
	}
	defer fi.Close()
	err = commandTemplateParsed.Execute(fi, commandTemplateParams{
		Name: rootCmd.Name(),
	})
	if err != nil {
		return err
	}
	return nil
}

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/template"
	"github.com/spf13/cobra"
)

func projectInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Get started with an example project.",
		RunE: func(cmd *cobra.Command, args []string) error {
			templates, err := template.List()
			if err != nil {
				return err
			}
			templateNames := []string{}
			templateByName := map[string]codersdk.Template{}
			for _, template := range templates {
				templateNames = append(templateNames, template.Name)
				templateByName[template.Name] = template
			}

			fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render("Templates contain Infrastructure as Code that works with Coder to provision development workspaces. Get started by selecting one:\n"))

			option, err := cliui.Select(cmd, cliui.SelectOptions{
				Options: templateNames,
			})
			if err != nil {
				return err
			}
			selectedTemplate := templateByName[option]

			archive, err := template.Archive(selectedTemplate.ID)
			if err != nil {
				return err
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return err
			}
			var directory string
			if len(args) > 0 {
				directory = args[0]
			} else {
				directory = filepath.Join(workingDir, selectedTemplate.ID)
			}

			relPath, err := filepath.Rel(workingDir, directory)
			if err != nil {
				relPath = directory
			} else {
				relPath = "./" + relPath
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sExtracting %s to %s...\n", cliui.Styles.Prompt, cliui.Styles.Field.Render(selectedTemplate.ID), cliui.Styles.Keyword.Render(relPath))

			err = os.MkdirAll(directory, 0700)
			if err != nil {
				return err
			}

			err = provisionersdk.Untar(directory, archive)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Inside that directory, get started by running:")
			fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Code.Render("coder projects create"))+"\n")

			return nil
		},
	}
}

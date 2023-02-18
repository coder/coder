package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/examples"
	"github.com/coder/coder/provisionersdk"
)

func templateInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Get started with a templated template.",
		RunE: func(cmd *cobra.Command, args []string) error {
			exampleList, err := examples.List()
			if err != nil {
				return err
			}
			exampleNames := []string{}
			exampleByName := map[string]codersdk.TemplateExample{}
			for _, example := range exampleList {
				name := fmt.Sprintf(
					"%s\n%s\n%s\n",
					cliui.Styles.Bold.Render(example.Name),
					cliui.Styles.Wrap.Copy().PaddingLeft(6).Render(example.Description),
					cliui.Styles.Keyword.Copy().PaddingLeft(6).Render(example.URL),
				)
				exampleNames = append(exampleNames, name)
				exampleByName[name] = example
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render(
				"A template defines infrastructure as code to be provisioned "+
					"for individual developer workspaces. Select an example to be copied to the active directory:\n"))
			option, err := cliui.Select(cmd, cliui.SelectOptions{
				Options: exampleNames,
			})
			if err != nil {
				return err
			}
			selectedTemplate := exampleByName[option]
			archive, err := examples.Archive(selectedTemplate.ID)
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Extracting %s to %s...\n", cliui.Styles.Field.Render(selectedTemplate.ID), relPath)
			err = os.MkdirAll(directory, 0o700)
			if err != nil {
				return err
			}
			err = provisionersdk.Untar(directory, bytes.NewReader(archive))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Create your template by running:")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Code.Render("cd "+relPath+" && coder templates create"))+"\n")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render("Examples provide a starting point and are expected to be edited! 🎨"))
			return nil
		},
	}
}

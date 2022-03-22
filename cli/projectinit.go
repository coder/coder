package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/examples"
	"github.com/coder/coder/provisionersdk"
)

func projectInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Get started with a templated project.",
		RunE: func(cmd *cobra.Command, args []string) error {
			exampleList, err := examples.List()
			if err != nil {
				return err
			}
			exampleNames := []string{}
			exampleByName := map[string]examples.Example{}
			for _, example := range exampleList {
				exampleNames = append(exampleNames, example.Name)
				exampleByName[example.Name] = example
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render("Projects contain Infrastructure as Code that works with Coder to provision development workspaces. Get started by selecting an example:\n"))
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sExtracting %s to %s...\n", cliui.Styles.Prompt, cliui.Styles.Field.Render(selectedTemplate.ID), cliui.Styles.Keyword.Render(relPath))
			err = os.MkdirAll(directory, 0700)
			if err != nil {
				return err
			}
			err = provisionersdk.Untar(directory, archive)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Inside that directory, get started by running:")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(cliui.Styles.Code.Render("coder projects create"))+"\n")
			return nil
		},
	}
}

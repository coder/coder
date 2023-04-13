package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/examples"
	"github.com/coder/coder/provisionersdk"
)

func (*RootCmd) templateInit() *clibase.Cmd {
	var templateIDArg string
	cmd := &clibase.Cmd{
		Use:        "init [directory]",
		Short:      "Get started with a templated template.",
		Middleware: clibase.RequireRangeArgs(0, 1),
		Handler: func(inv *clibase.Invocation) error {
			exampleList, err := examples.List()
			if err != nil {
				return err
			}

			optsToID := map[string]string{}
			for _, example := range exampleList {
				name := fmt.Sprintf(
					"%s\n%s\n%s\n",
					cliui.Styles.Bold.Render(example.Name),
					cliui.Styles.Wrap.Copy().PaddingLeft(6).Render(example.Description),
					cliui.Styles.Keyword.Copy().PaddingLeft(6).Render(example.URL),
				)
				optsToID[name] = example.ID
			}

			// If the user didn't specify any template, prompt them to select one.
			if templateIDArg == "" {
				opts := keys(optsToID)
				sort.Strings(opts)
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Wrap.Render(
					"A template defines infrastructure as code to be provisioned "+
						"for individual developer workspaces. Select an example to be copied to the active directory:\n"))
				selected, err := cliui.Select(inv, cliui.SelectOptions{
					Options: sort.StringSlice(keys(optsToID)),
				})
				if err != nil {
					return err
				}
				templateIDArg = optsToID[selected]
			}

			selectedTemplate, ok := templateByID(templateIDArg, exampleList)
			if !ok {
				ids := values(optsToID)
				sort.Strings(ids)
				return xerrors.Errorf("Template ID %q does not exist!\nValid options are: %q", templateIDArg, ids)
			}
			archive, err := examples.Archive(selectedTemplate.ID)
			if err != nil {
				return err
			}
			workingDir, err := os.Getwd()
			if err != nil {
				return err
			}
			var directory string
			if len(inv.Args) > 0 {
				directory = inv.Args[0]
			} else {
				directory = filepath.Join(workingDir, selectedTemplate.ID)
			}
			relPath, err := filepath.Rel(workingDir, directory)
			if err != nil {
				relPath = directory
			} else {
				relPath = "./" + relPath
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Extracting %s to %s...\n", cliui.Styles.Field.Render(selectedTemplate.ID), relPath)
			err = os.MkdirAll(directory, 0o700)
			if err != nil {
				return err
			}
			err = provisionersdk.Untar(directory, bytes.NewReader(archive))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, "Create your template by running:")
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render(cliui.Styles.Code.Render("cd "+relPath+" && coder templates create"))+"\n")
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Wrap.Render("Examples provide a starting point and are expected to be edited! 🎨"))
			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "id",
			Description: "Specify a given example template by ID.",
			Value:       clibase.StringOf(&templateIDArg),
		},
	}

	return cmd
}

func templateByID(templateID string, tes []codersdk.TemplateExample) (codersdk.TemplateExample, bool) {
	for _, te := range tes {
		if te.ID == templateID {
			return te, true
		}
	}
	return codersdk.TemplateExample{}, false
}

func keys[K comparable, V any](m map[K]V) []K {
	l := make([]K, 0, len(m))
	for k := range m {
		l = append(l, k)
	}
	return l
}

func values[K comparable, V any](m map[K]V) []V {
	l := make([]V, 0, len(m))
	for _, v := range m {
		l = append(l, v)
	}
	return l
}

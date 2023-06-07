package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/examples"
	"github.com/coder/coder/provisionersdk"
)

func (*RootCmd) templateInit() *clibase.Cmd {
	var templateID string
	exampleList, err := examples.List()
	if err != nil {
		// This should not happen. If it does, something is very wrong.
		panic(err)
	}
	var templateIDs []string
	for _, ex := range exampleList {
		templateIDs = append(templateIDs, ex.ID)
	}
	sort.Strings(templateIDs)
	cmd := &clibase.Cmd{
		Use:        "init [directory]",
		Short:      "Get started with a templated template.",
		Middleware: clibase.RequireRangeArgs(0, 1),
		Handler: func(inv *clibase.Invocation) error {
			// If the user didn't specify any template, prompt them to select one.
			if templateID == "" {
				optsToID := map[string]string{}
				for _, example := range exampleList {
					name := fmt.Sprintf(
						"%s\n%s\n%s\n",
						cliui.DefaultStyles.Bold.Render(example.Name),
						cliui.DefaultStyles.Wrap.Copy().PaddingLeft(6).Render(example.Description),
						cliui.DefaultStyles.Keyword.Copy().PaddingLeft(6).Render(example.URL),
					)
					optsToID[name] = example.ID
				}
				opts := maps.Keys(optsToID)
				sort.Strings(opts)
				_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Wrap.Render(
					"A template defines infrastructure as code to be provisioned "+
						"for individual developer workspaces. Select an example to be copied to the active directory:\n"))
				selected, err := cliui.Select(inv, cliui.SelectOptions{
					Options: opts,
				})
				if err != nil {
					if errors.Is(err, io.EOF) {
						return xerrors.Errorf(
							"Couldn't find a matching template!\n" +
								"Tip: if you're trying to automate template creation, try\n" +
								"coder templates init --id <template_id> instead!",
						)
					}
					return err
				}
				templateID = optsToID[selected]
			}

			selectedTemplate, ok := templateByID(templateID, exampleList)
			if !ok {
				// clibase.EnumOf would normally handle this.
				return xerrors.Errorf("template not found: %q", templateID)
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
			_, _ = fmt.Fprintf(inv.Stdout, "Extracting %s to %s...\n", cliui.DefaultStyles.Field.Render(selectedTemplate.ID), relPath)
			err = os.MkdirAll(directory, 0o700)
			if err != nil {
				return err
			}
			err = provisionersdk.Untar(directory, bytes.NewReader(archive))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, "Create your template by running:")
			_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Paragraph.Render(cliui.DefaultStyles.Code.Render("cd "+relPath+" && coder templates create"))+"\n")
			_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Wrap.Render("Examples provide a starting point and are expected to be edited! 🎨"))
			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "id",
			Description: "Specify a given example template by ID.",
			Value:       clibase.EnumOf(&templateID, templateIDs...),
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

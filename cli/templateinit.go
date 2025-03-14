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
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/examples"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
func (*RootCmd) templateInit() *serpent.Command {
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
	cmd := &serpent.Command{
		Use:        "init [directory]",
		Short:      "Get started with a templated template.",
		Middleware: serpent.RequireRangeArgs(0, 1),
		Handler: func(inv *serpent.Invocation) error {
			// If the user didn't specify any template, prompt them to select one.
			if templateID == "" {
				optsToID := map[string]string{}
				for _, example := range exampleList {
					name := fmt.Sprintf(
						"%s\n%s\n%s\n",
						cliui.Bold(example.Name),
						pretty.Sprint(cliui.DefaultStyles.Wrap.With(pretty.XPad(6, 0)), example.Description),
						pretty.Sprint(cliui.DefaultStyles.Keyword.With(pretty.XPad(6, 0)), example.URL),
					)
					optsToID[name] = example.ID
				}
				opts := maps.Keys(optsToID)
				sort.Strings(opts)
				_, _ = fmt.Fprintln(
					inv.Stdout,
					pretty.Sprint(
						cliui.DefaultStyles.Wrap,
						"A template defines infrastructure as code to be provisioned "+
							"for individual developer workspaces. Select an example to be copied to the active directory:\n"),
				)
				selected, err := cliui.Select(inv, cliui.SelectOptions{
					Options: opts,
				})
				if err != nil {
					if errors.Is(err, io.EOF) {
						return fmt.Errorf(
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
				// serpent.EnumOf would normally handle this.
				return fmt.Errorf("template not found: %q", templateID)
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
			_, _ = fmt.Fprintf(inv.Stdout, "Extracting %s to %s...\n", pretty.Sprint(cliui.DefaultStyles.Field, selectedTemplate.ID), relPath)
			err = os.MkdirAll(directory, 0o700)
			if err != nil {
				return err
			}
			err = provisionersdk.Untar(directory, bytes.NewReader(archive))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, "Create your template by running:")
			_, _ = fmt.Fprintln(
				inv.Stdout,
				pretty.Sprint(
					cliui.DefaultStyles.Code,
					"cd "+relPath+" && coder templates push"),
			)
			_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Wrap, "\nExamples provide a starting point and are expected to be edited! 🎨"))
			return nil
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "id",
			Description: "Specify a given example template by ID.",
			Value:       serpent.EnumOf(&templateID, templateIDs...),
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

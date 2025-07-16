package cli

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) templateVersionPresets() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "presets",
		Short:   "Manage presets of the specified template version",
		Aliases: []string{"preset"},
		Long: FormatExamples(
			Example{
				Description: "List presets of a specific template version",
				Command:     "coder templates versions presets list my-template my-template-version",
			},
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.templateVersionPresetsList(),
		},
	}

	return cmd
}

func (r *RootCmd) templateVersionPresetsList() *serpent.Command {
	defaultColumns := []string{
		"name",
		"parameters",
		"default",
		"prebuilds",
	}
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]templateVersionPresetRow{}, defaultColumns),
		cliui.JSONFormat(),
	)
	client := new(codersdk.Client)
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use: "list <template> <version>",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(2),
			r.InitClient(client),
		),
		Short:   "List all the presets of the specified template version",
		Options: serpent.OptionSet{},
		Handler: func(inv *serpent.Invocation) error {
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}

			version, err := client.TemplateVersionByName(inv.Context(), template.ID, inv.Args[1])
			if err != nil {
				return xerrors.Errorf("get template version by name: %w", err)
			}

			presets, err := client.TemplateVersionPresets(inv.Context(), version.ID)
			if err != nil {
				return xerrors.Errorf("get template versions presets by template version: %w", err)
			}

			if len(presets) == 0 {
				return xerrors.Errorf("no presets found for template %q and template-version %q", template.Name, version.Name)
			}

			rows := templateVersionPresetsToRows(presets...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return xerrors.Errorf("render table: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	orgContext.AttachOptions(cmd)
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

type templateVersionPresetRow struct {
	// For json format:
	TemplateVersionPreset codersdk.Preset `table:"-"`

	// For table format:
	Name       string `json:"-" table:"name,default_sort"`
	Parameters string `json:"-" table:"parameters"`
	Default    bool   `json:"-" table:"default"`
	Prebuilds  string `json:"-" table:"prebuilds"`
}

func formatPresetParameters(params []codersdk.PresetParameter) string {
	var paramsStr []string
	for _, p := range params {
		paramsStr = append(paramsStr, fmt.Sprintf("%s=%s", p.Name, p.Value))
	}
	return strings.Join(paramsStr, ",")
}

// templateVersionPresetsToRows converts a list of presets to a list of rows
// for outputting.
func templateVersionPresetsToRows(presets ...codersdk.Preset) []templateVersionPresetRow {
	rows := make([]templateVersionPresetRow, len(presets))
	for i, preset := range presets {
		prebuilds := "-"
		if preset.Prebuilds != nil {
			prebuilds = strconv.Itoa(*preset.Prebuilds)
		}
		rows[i] = templateVersionPresetRow{
			Name:       preset.Name,
			Parameters: formatPresetParameters(preset.Parameters),
			Default:    preset.Default,
			Prebuilds:  prebuilds,
		}
	}

	return rows
}

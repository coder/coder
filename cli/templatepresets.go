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

func (r *RootCmd) templatePresets() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "presets",
		Short:   "Manage presets of the specified template",
		Aliases: []string{"preset"},
		Long: FormatExamples(
			Example{
				Description: "List presets for the active version of a template",
				Command:     "coder templates presets list my-template",
			},
			Example{
				Description: "List presets for a specific version of a template",
				Command:     "coder templates presets list my-template --template-version my-template-version",
			},
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.templatePresetsList(),
		},
	}

	return cmd
}

func (r *RootCmd) templatePresetsList() *serpent.Command {
	defaultColumns := []string{
		"name",
		"description",
		"parameters",
		"default",
		"desired prebuild instances",
	}
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]TemplatePresetRow{}, defaultColumns),
		cliui.JSONFormat(),
	)
	orgContext := NewOrganizationContext()

	var templateVersion string

	cmd := &serpent.Command{
		Use: "list <template>",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Short: "List all presets of the specified template. Defaults to the active template version.",
		Options: serpent.OptionSet{
			{
				Name:        "template-version",
				Description: "Specify a template version to list presets for. Defaults to the active version.",
				Flag:        "template-version",
				Value:       serpent.StringOf(&templateVersion),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}

			// If a template version is specified via flag, fetch that version by name
			var version codersdk.TemplateVersion
			if len(templateVersion) > 0 {
				version, err = client.TemplateVersionByName(inv.Context(), template.ID, templateVersion)
				if err != nil {
					return xerrors.Errorf("get template version by name: %w", err)
				}
			} else {
				// Otherwise, use the template's active version
				version, err = client.TemplateVersion(inv.Context(), template.ActiveVersionID)
				if err != nil {
					return xerrors.Errorf("get active template version: %w", err)
				}
			}

			presets, err := client.TemplateVersionPresets(inv.Context(), version.ID)
			if err != nil {
				return xerrors.Errorf("get template versions presets by template version: %w", err)
			}

			if len(presets) == 0 {
				cliui.Infof(
					inv.Stdout,
					"No presets found for template %q and template-version %q.\n", template.Name, version.Name,
				)
				return nil
			}

			// Only display info message for table output
			if formatter.FormatID() == "table" {
				cliui.Infof(
					inv.Stdout,
					"Showing presets for template %q and template version %q.\n", template.Name, version.Name,
				)
			}
			rows := templatePresetsToRows(presets...)
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

type TemplatePresetRow struct {
	// For json format
	TemplatePreset codersdk.Preset `table:"-"`

	// For table format:
	Name                     string `json:"-" table:"name,default_sort"`
	Description              string `json:"-" table:"description"`
	Parameters               string `json:"-" table:"parameters"`
	Default                  bool   `json:"-" table:"default"`
	DesiredPrebuildInstances string `json:"-" table:"desired prebuild instances"`
}

func formatPresetParameters(params []codersdk.PresetParameter) string {
	var paramsStr []string
	for _, p := range params {
		paramsStr = append(paramsStr, fmt.Sprintf("%s=%s", p.Name, p.Value))
	}
	return strings.Join(paramsStr, ",")
}

// templatePresetsToRows converts a list of presets to a list of rows
// for outputting.
func templatePresetsToRows(presets ...codersdk.Preset) []TemplatePresetRow {
	rows := make([]TemplatePresetRow, len(presets))
	for i, preset := range presets {
		prebuildInstances := "-"
		if preset.DesiredPrebuildInstances != nil {
			prebuildInstances = strconv.Itoa(*preset.DesiredPrebuildInstances)
		}
		rows[i] = TemplatePresetRow{
			// For json format
			TemplatePreset: preset,
			// For table format
			Name:                     preset.Name,
			Description:              preset.Description,
			Parameters:               formatPresetParameters(preset.Parameters),
			Default:                  preset.Default,
			DesiredPrebuildInstances: prebuildInstances,
		}
	}

	return rows
}

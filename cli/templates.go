package cli

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) templates() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "templates",
		Short: "Manage templates",
		Long: "Templates are written in standard Terraform and describe the infrastructure for workspaces\n" + FormatExamples(
			Example{
				Description: "Create or push an update to the template. Your developers can update their workspaces",
				Command:     "coder templates push my-template",
			},
		),
		Aliases: []string{"template"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.templateCreate(),
			r.templateEdit(),
			r.templateInit(),
			r.templateList(),
			r.templatePush(),
			r.templateVersions(),
			r.templatePresets(),
			r.templateDelete(),
			r.templatePull(),
			r.archiveTemplateVersions(),
		},
	}

	return cmd
}

func selectTemplate(inv *serpent.Invocation, client *codersdk.Client, organization codersdk.Organization) (codersdk.Template, error) {
	var empty codersdk.Template
	ctx := inv.Context()
	allTemplates, err := client.TemplatesByOrganization(ctx, organization.ID)
	if err != nil {
		return empty, xerrors.Errorf("get templates by organization: %w", err)
	}

	if len(allTemplates) == 0 {
		return empty, xerrors.Errorf("no templates exist in the current organization %q", organization.Name)
	}

	opts := make([]string, 0, len(allTemplates))
	for _, template := range allTemplates {
		opts = append(opts, template.Name)
	}

	selection, err := cliui.Select(inv, cliui.SelectOptions{
		Options: opts,
	})
	if err != nil {
		return empty, xerrors.Errorf("select template: %w", err)
	}

	for _, template := range allTemplates {
		if template.Name == selection {
			return template, nil
		}
	}
	return empty, xerrors.Errorf("no template selected")
}

type templateTableRow struct {
	// Used by json format:
	Template codersdk.Template

	// Used by table format:
	Name             string                   `json:"-" table:"name,default_sort"`
	CreatedAt        string                   `json:"-" table:"created at"`
	LastUpdated      string                   `json:"-" table:"last updated"`
	OrganizationID   uuid.UUID                `json:"-" table:"organization id"`
	OrganizationName string                   `json:"-" table:"organization name"`
	Provisioner      codersdk.ProvisionerType `json:"-" table:"provisioner"`
	ActiveVersionID  uuid.UUID                `json:"-" table:"active version id"`
	UsedBy           string                   `json:"-" table:"used by"`
	DefaultTTL       time.Duration            `json:"-" table:"default ttl"`
}

// templateToRows converts a list of templates to a list of templateTableRow for
// outputting.
func templatesToRows(templates ...codersdk.Template) []templateTableRow {
	rows := make([]templateTableRow, len(templates))
	for i, template := range templates {
		rows[i] = templateTableRow{
			Template:         template,
			Name:             template.Name,
			CreatedAt:        template.CreatedAt.Format("January 2, 2006"),
			LastUpdated:      template.UpdatedAt.Format("January 2, 2006"),
			OrganizationID:   template.OrganizationID,
			OrganizationName: template.OrganizationName,
			Provisioner:      template.Provisioner,
			ActiveVersionID:  template.ActiveVersionID,
			UsedBy:           pretty.Sprint(cliui.DefaultStyles.Fuchsia, formatActiveDevelopers(template.ActiveUserCount)),
			DefaultTTL:       (time.Duration(template.DefaultTTLMillis) * time.Millisecond),
		}
	}

	return rows
}

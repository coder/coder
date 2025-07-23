package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) attach() *serpent.Command {
	var (
		templateName    string
		templateVersion string
		workspaceName   string

		parameterFlags workspaceParameterFlags
		// Organization context is only required if more than 1 template
		// shares the same name across multiple organizations.
		orgContext = NewOrganizationContext()
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "attach [workspace]",
		Short:       "Create a workspace and attach an external agent to it",
		Long: FormatExamples(
			Example{
				Description: "Attach an external agent to a workspace",
				Command:     "coder attach my-workspace --template externally-managed-workspace --output text",
			},
		),
		Middleware: serpent.Chain(r.InitClient(client)),
		Handler: func(inv *serpent.Invocation) error {
			var err error
			workspaceOwner := codersdk.Me
			if len(inv.Args) >= 1 {
				workspaceOwner, workspaceName, err = splitNamedWorkspace(inv.Args[0])
				if err != nil {
					return err
				}
			}

			if workspaceName == "" {
				workspaceName, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Specify a name for your workspace:",
					Validate: func(workspaceName string) error {
						err = codersdk.NameValid(workspaceName)
						if err != nil {
							return xerrors.Errorf("workspace name %q is invalid: %w", workspaceName, err)
						}
						_, err = client.WorkspaceByOwnerAndName(inv.Context(), workspaceOwner, workspaceName, codersdk.WorkspaceOptions{})
						if err == nil {
							return xerrors.Errorf("a workspace already exists named %q", workspaceName)
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}
			err = codersdk.NameValid(workspaceName)
			if err != nil {
				return xerrors.Errorf("workspace name %q is invalid: %w", workspaceName, err)
			}

			if workspace, err := client.WorkspaceByOwnerAndName(inv.Context(), workspaceOwner, workspaceName, codersdk.WorkspaceOptions{}); err == nil {
				return externalAgentDetails(inv, client, workspace, workspace.LatestBuild.Resources)
			}

			// If workspace doesn't exist, create it
			var template codersdk.Template
			var templateVersionID uuid.UUID
			switch {
			case templateName == "":
				_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Wrap, "Select a template below to preview the provisioned infrastructure:"))

				templates, err := client.Templates(inv.Context(), codersdk.TemplateFilter{})
				if err != nil {
					return err
				}

				slices.SortFunc(templates, func(a, b codersdk.Template) int {
					return slice.Descending(a.ActiveUserCount, b.ActiveUserCount)
				})

				templateNames := make([]string, 0, len(templates))
				templateByName := make(map[string]codersdk.Template, len(templates))

				// If more than 1 organization exists in the list of templates,
				// then include the organization name in the select options.
				uniqueOrganizations := make(map[uuid.UUID]bool)
				for _, template := range templates {
					uniqueOrganizations[template.OrganizationID] = true
				}

				for _, template := range templates {
					templateName := template.Name
					if len(uniqueOrganizations) > 1 {
						templateName += cliui.Placeholder(
							fmt.Sprintf(
								" (%s)",
								template.OrganizationName,
							),
						)
					}

					if template.ActiveUserCount > 0 {
						templateName += cliui.Placeholder(
							fmt.Sprintf(
								" used by %s",
								formatActiveDevelopers(template.ActiveUserCount),
							),
						)
					}

					templateNames = append(templateNames, templateName)
					templateByName[templateName] = template
				}

				// Move the cursor up a single line for nicer display!
				option, err := cliui.Select(inv, cliui.SelectOptions{
					Options:    templateNames,
					HideSearch: true,
				})
				if err != nil {
					return err
				}

				template = templateByName[option]
				templateVersionID = template.ActiveVersionID
			default:
				templates, err := client.Templates(inv.Context(), codersdk.TemplateFilter{
					ExactName: templateName,
				})
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}
				if len(templates) == 0 {
					return xerrors.Errorf("no template found with the name %q", templateName)
				}

				if len(templates) > 1 {
					templateOrgs := []string{}
					for _, tpl := range templates {
						templateOrgs = append(templateOrgs, tpl.OrganizationName)
					}

					selectedOrg, err := orgContext.Selected(inv, client)
					if err != nil {
						return xerrors.Errorf("multiple templates found with the name %q, use `--org=<organization_name>` to specify which template by that name to use. Organizations available: %s", templateName, strings.Join(templateOrgs, ", "))
					}

					index := slices.IndexFunc(templates, func(i codersdk.Template) bool {
						return i.OrganizationID == selectedOrg.ID
					})
					if index == -1 {
						return xerrors.Errorf("no templates found with the name %q in the organization %q. Templates by that name exist in organizations: %s. Use --org=<organization_name> to select one.", templateName, selectedOrg.Name, strings.Join(templateOrgs, ", "))
					}

					// remake the list with the only template selected
					templates = []codersdk.Template{templates[index]}
				}

				template = templates[0]
				templateVersionID = template.ActiveVersionID
			}

			if len(templateVersion) > 0 {
				version, err := client.TemplateVersionByName(inv.Context(), template.ID, templateVersion)
				if err != nil {
					return xerrors.Errorf("get template version by name: %w", err)
				}
				templateVersionID = version.ID
			}

			// If the user specified an organization via a flag or env var, the template **must**
			// be in that organization. Otherwise, we should throw an error.
			orgValue, orgValueSource := orgContext.ValueSource(inv)
			if orgValue != "" && !(orgValueSource == serpent.ValueSourceDefault || orgValueSource == serpent.ValueSourceNone) {
				selectedOrg, err := orgContext.Selected(inv, client)
				if err != nil {
					return err
				}

				if template.OrganizationID != selectedOrg.ID {
					orgNameFormat := "'--org=%q'"
					if orgValueSource == serpent.ValueSourceEnv {
						orgNameFormat = "CODER_ORGANIZATION=%q"
					}

					return xerrors.Errorf("template is in organization %q, but %s was specified. Use %s to use this template",
						template.OrganizationName,
						fmt.Sprintf(orgNameFormat, selectedOrg.Name),
						fmt.Sprintf(orgNameFormat, template.OrganizationName),
					)
				}
			}

			cliBuildParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
			if err != nil {
				return xerrors.Errorf("can't parse given parameter values: %w", err)
			}

			cliBuildParameterDefaults, err := asWorkspaceBuildParameters(parameterFlags.richParameterDefaults)
			if err != nil {
				return xerrors.Errorf("can't parse given parameter defaults: %w", err)
			}

			richParameters, resources, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
				Action:            WorkspaceCreate,
				TemplateVersionID: templateVersionID,
				NewWorkspaceName:  workspaceName,

				RichParameterFile:     parameterFlags.richParameterFile,
				RichParameters:        cliBuildParameters,
				RichParameterDefaults: cliBuildParameterDefaults,
			})
			if err != nil {
				return xerrors.Errorf("prepare build: %w", err)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm create?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := client.CreateUserWorkspace(inv.Context(), workspaceOwner, codersdk.CreateWorkspaceRequest{
				TemplateVersionID:   templateVersionID,
				Name:                workspaceName,
				RichParameterValues: richParameters,
			})
			if err != nil {
				return xerrors.Errorf("create workspace: %w", err)
			}

			cliutil.WarnMatchedProvisioners(inv.Stderr, workspace.LatestBuild.MatchedProvisioners, workspace.LatestBuild.Job)

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, workspace.LatestBuild.ID)
			if err != nil {
				return xerrors.Errorf("watch build: %w", err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\nThe %s workspace has been created at %s!\n\n",
				cliui.Keyword(workspace.Name),
				cliui.Timestamp(time.Now()),
			)

			return externalAgentDetails(inv, client, workspace, resources)
		},
	}

	cmd.Options = serpent.OptionSet{
		serpent.Option{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_TEMPLATE_NAME",
			Description:   "Specify a template name.",
			Value:         serpent.StringOf(&templateName),
		},
		serpent.Option{
			Flag:        "template-version",
			Env:         "CODER_TEMPLATE_VERSION",
			Description: "Specify a template version name.",
			Value:       serpent.StringOf(&templateVersion),
		},
		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, parameterFlags.cliParameters()...)
	cmd.Options = append(cmd.Options, parameterFlags.cliParameterDefaults()...)
	orgContext.AttachOptions(cmd)
	return cmd
}

func externalAgentDetails(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, resources []codersdk.WorkspaceResource) error {
	if len(resources) == 0 {
		return xerrors.Errorf("no resources found for workspace")
	}

	for _, resource := range resources {
		if resource.Type == "coder_external_agent" {
			agent := resource.Agents[0]
			credential, err := client.WorkspaceExternalAgentCredential(inv.Context(), workspace.ID, agent.Name)
			if err != nil {
				return xerrors.Errorf("get external agent token: %w", err)
			}

			initScriptURL := fmt.Sprintf("%s/api/v2/init-script", client.URL)
			if agent.OperatingSystem != "linux" || agent.Architecture != "amd64" {
				initScriptURL = fmt.Sprintf("%s/api/v2/init-script?os=%s&arch=%s", client.URL, agent.OperatingSystem, agent.Architecture)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Please run the following commands to attach an agent to the workspace %s:\n", cliui.Keyword(workspace.Name))
			_, _ = fmt.Fprintf(inv.Stdout, "%s\n", pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("export CODER_AGENT_TOKEN=%s", credential.AgentToken)))
			_, _ = fmt.Fprintf(inv.Stdout, "%s\n", pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("curl -fsSL %s | sh", initScriptURL)))
		}
	}

	return nil
}

package cli

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

type externalAgent struct {
	WorkspaceName string `json:"-"`
	AgentName     string `json:"-"`
	AuthType      string `json:"auth_type"`
	AuthToken     string `json:"auth_token"`
	InitScript    string `json:"init_script"`
}

func (r *RootCmd) externalWorkspaces() *serpent.Command {
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:   "external-workspaces [subcommand]",
		Short: "Create or manage external workspaces",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.externalWorkspaceCreate(),
			r.externalWorkspaceAgentInstructions(),
			r.externalWorkspaceList(),
		},
	}

	orgContext.AttachOptions(cmd)
	return cmd
}

// externalWorkspaceCreate extends `coder create` to create an external workspace.
func (r *RootCmd) externalWorkspaceCreate() *serpent.Command {
	opts := createOptions{
		beforeCreate: func(ctx context.Context, client *codersdk.Client, _ codersdk.Template, templateVersionID uuid.UUID) error {
			resources, err := client.TemplateVersionResources(ctx, templateVersionID)
			if err != nil {
				return xerrors.Errorf("get template version resources: %w", err)
			}
			if len(resources) == 0 {
				return xerrors.Errorf("no resources found for template version %q", templateVersionID)
			}

			var hasExternalAgent bool
			for _, resource := range resources {
				if resource.Type == "coder_external_agent" {
					hasExternalAgent = true
					break
				}
			}

			if !hasExternalAgent {
				return xerrors.Errorf("template version %q does not have an external agent. Only templates with external agents can be used for external workspace creation", templateVersionID)
			}

			return nil
		},
		afterCreate: func(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace) error {
			workspace, err := client.WorkspaceByOwnerAndName(ctx, codersdk.Me, workspace.Name, codersdk.WorkspaceOptions{})
			if err != nil {
				return xerrors.Errorf("get workspace by name: %w", err)
			}

			externalAgents, err := fetchExternalAgents(inv, client, workspace, workspace.LatestBuild.Resources)
			if err != nil {
				return xerrors.Errorf("fetch external agents: %w", err)
			}

			formatted := formatExternalAgent(workspace.Name, externalAgents)
			_, err = fmt.Fprintln(inv.Stdout, formatted)
			return err
		},
	}

	cmd := r.create(opts)
	cmd.Use = "create [workspace]"
	cmd.Short = "Create a new external workspace"
	cmd.Middleware = serpent.Chain(
		cmd.Middleware,
		serpent.RequireNArgs(1),
	)

	for i := range cmd.Options {
		if cmd.Options[i].Flag == "template" {
			cmd.Options[i].Required = true
		}
	}

	return cmd
}

// externalWorkspaceAgentInstructions prints the instructions for an external agent.
func (r *RootCmd) externalWorkspaceAgentInstructions() *serpent.Command {
	client := new(codersdk.Client)
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
			agent, ok := data.(externalAgent)
			if !ok {
				return "", xerrors.Errorf("expected externalAgent, got %T", data)
			}

			return formatExternalAgent(agent.WorkspaceName, []externalAgent{agent}), nil
		}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:        "agent-instructions [user/]workspace[.agent]",
		Short:      "Get the instructions for an external agent",
		Middleware: serpent.Chain(r.InitClient(client), serpent.RequireNArgs(1)),
		Handler: func(inv *serpent.Invocation) error {
			workspace, workspaceAgent, _, err := getWorkspaceAndAgent(inv.Context(), inv, client, false, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("find workspace and agent: %w", err)
			}

			credential, err := client.WorkspaceExternalAgentCredentials(inv.Context(), workspace.ID, workspaceAgent.Name)
			if err != nil {
				return xerrors.Errorf("get external agent token for agent %q: %w", workspaceAgent.Name, err)
			}

			initScriptURL := fmt.Sprintf("%s/api/v2/init-script/%s/%s", client.URL, workspaceAgent.OperatingSystem, workspaceAgent.Architecture)
			agentInfo := externalAgent{
				WorkspaceName: workspace.Name,
				AgentName:     workspaceAgent.Name,
				AuthType:      "token",
				AuthToken:     credential.AgentToken,
				InitScript:    initScriptURL,
			}

			out, err := formatter.Format(inv.Context(), agentInfo)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) externalWorkspaceList() *serpent.Command {
	var (
		filter    cliui.WorkspaceFilter
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]workspaceListRow{},
				[]string{
					"workspace",
					"template",
					"status",
					"healthy",
					"last built",
					"current version",
					"outdated",
				},
			),
			cliui.JSONFormat(),
		)
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List external workspaces",
		Aliases:     []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			baseFilter := filter.Filter()

			if baseFilter.FilterQuery == "" {
				baseFilter.FilterQuery = "has-external-agent:true"
			} else {
				baseFilter.FilterQuery += " has-external-agent:true"
			}

			res, err := queryConvertWorkspaces(inv.Context(), client, baseFilter, workspaceListRowFromWorkspace)
			if err != nil {
				return err
			}

			if len(res) == 0 && formatter.FormatID() != cliui.JSONFormat().ID() {
				pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Prompt, "No workspaces found! Create one:\n")
				_, _ = fmt.Fprintln(inv.Stderr)
				_, _ = fmt.Fprintln(inv.Stderr, "  "+pretty.Sprint(cliui.DefaultStyles.Code, "coder external-workspaces create <name>"))
				_, _ = fmt.Fprintln(inv.Stderr)
				return nil
			}

			out, err := formatter.Format(inv.Context(), res)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	filter.AttachOptions(&cmd.Options)
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// fetchExternalAgents fetches the external agents for a workspace.
func fetchExternalAgents(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, resources []codersdk.WorkspaceResource) ([]externalAgent, error) {
	if len(resources) == 0 {
		return nil, xerrors.Errorf("no resources found for workspace")
	}

	var externalAgents []externalAgent

	for _, resource := range resources {
		if resource.Type != "coder_external_agent" || len(resource.Agents) == 0 {
			continue
		}

		agent := resource.Agents[0]
		credential, err := client.WorkspaceExternalAgentCredentials(inv.Context(), workspace.ID, agent.Name)
		if err != nil {
			return nil, xerrors.Errorf("get external agent token for agent %q: %w", agent.Name, err)
		}

		initScriptURL := fmt.Sprintf("%s/api/v2/init-script/%s/%s", client.URL, agent.OperatingSystem, agent.Architecture)
		externalAgents = append(externalAgents, externalAgent{
			AgentName:  agent.Name,
			AuthType:   "token",
			AuthToken:  credential.AgentToken,
			InitScript: initScriptURL,
		})
	}

	return externalAgents, nil
}

// formatExternalAgent formats the instructions for an external agent.
func formatExternalAgent(workspaceName string, externalAgents []externalAgent) string {
	var output strings.Builder
	_, _ = output.WriteString(fmt.Sprintf("\nPlease run the following commands to attach external agent to the workspace %s:\n\n", cliui.Keyword(workspaceName)))

	for i, agent := range externalAgents {
		if len(externalAgents) > 1 {
			_, _ = output.WriteString(fmt.Sprintf("For agent %s:\n", cliui.Keyword(agent.AgentName)))
		}

		_, _ = output.WriteString(fmt.Sprintf("%s\n", pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("export CODER_AGENT_TOKEN=%s", agent.AuthToken))))
		_, _ = output.WriteString(fmt.Sprintf("%s\n", pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("curl -fsSL %s | sh", agent.InitScript))))

		if i < len(externalAgents)-1 {
			_, _ = output.WriteString("\n")
		}
	}

	return output.String()
}

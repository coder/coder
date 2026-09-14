package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

type externalAgent struct {
	WorkspaceName string `json:"workspace_name"`
	AgentName     string `json:"agent_name"`
	AuthType      string `json:"auth_type"`
	AuthToken     string `json:"auth_token"`
	InitScript    string `json:"init_script"`
}

func (r *RootCmd) externalWorkspaces() *serpent.Command {
	orgContext := agpl.NewOrganizationContext()

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
	opts := agpl.CreateOptions{
		BeforeCreate: func(ctx context.Context, client *codersdk.Client, _ codersdk.Template, templateVersionID uuid.UUID) error {
			version, err := client.TemplateVersion(ctx, templateVersionID)
			if err != nil {
				return xerrors.Errorf("get template version: %w", err)
			}
			if !version.HasExternalAgent {
				return xerrors.Errorf("template version %q does not have an external agent. Only templates with external agents can be used for external workspace creation", templateVersionID)
			}

			return nil
		},
		AfterCreate: func(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace) error {
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

	cmd := r.Create(opts)
	cmd.Use = "create [workspace]"
	cmd.Short = "Create a new external workspace"
	newMiddlewares := []serpent.MiddlewareFunc{}
	if cmd.Middleware != nil {
		newMiddlewares = append(newMiddlewares, cmd.Middleware)
	}
	newMiddlewares = append(newMiddlewares, serpent.RequireNArgs(1))
	cmd.Middleware = serpent.Chain(newMiddlewares...)

	for i := range cmd.Options {
		if cmd.Options[i].Flag == "template" {
			cmd.Options[i].Required = true
		}
	}

	return cmd
}

// externalWorkspaceAgentInstructions prints the instructions for an external agent.
func (r *RootCmd) externalWorkspaceAgentInstructions() *serpent.Command {
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
		Middleware: serpent.Chain(serpent.RequireNArgs(1)),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			workspace, workspaceAgent, _, err := agpl.GetWorkspaceAndAgent(inv.Context(), inv, client, false, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("find workspace and agent: %w", err)
			}

			credentials, err := client.WorkspaceExternalAgentCredentials(inv.Context(), workspace.ID, workspaceAgent.Name)
			if err != nil {
				return xerrors.Errorf("get external agent token for agent %q: %w", workspaceAgent.Name, err)
			}

			agentInfo := externalAgent{
				WorkspaceName: workspace.Name,
				AgentName:     workspaceAgent.Name,
				AuthType:      "token",
				AuthToken:     credentials.AgentToken,
				InitScript:    credentials.Command,
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
				[]agpl.WorkspaceListRow{},
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
	cmd := &serpent.Command{
		Annotations: map[string]string{
			"workspaces": "",
		},
		Use:     "list",
		Short:   "List external workspaces",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			baseFilter := filter.Filter()

			if baseFilter.FilterQuery == "" {
				baseFilter.FilterQuery = "has_external_agent:true"
			} else {
				baseFilter.FilterQuery += " has_external_agent:true"
			}

			res, err := agpl.QueryConvertWorkspaces(inv.Context(), client, baseFilter, agpl.WorkspaceListRowFromWorkspace)
			if err != nil {
				return err
			}

			out, err := formatter.Format(inv.Context(), res)
			if err != nil {
				return err
			}

			if out == "" {
				pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Prompt, "No workspaces found! Create one:\n")
				_, _ = fmt.Fprintln(inv.Stderr)
				_, _ = fmt.Fprintln(inv.Stderr, "  "+pretty.Sprint(cliui.DefaultStyles.Code, "coder external-workspaces create <name>"))
				_, _ = fmt.Fprintln(inv.Stderr)
				return nil
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
		credentials, err := client.WorkspaceExternalAgentCredentials(inv.Context(), workspace.ID, agent.Name)
		if err != nil {
			return nil, xerrors.Errorf("get external agent token for agent %q: %w", agent.Name, err)
		}

		externalAgents = append(externalAgents, externalAgent{
			AgentName:  agent.Name,
			AuthType:   "token",
			AuthToken:  credentials.AgentToken,
			InitScript: credentials.Command,
		})
	}

	return externalAgents, nil
}

// formatExternalAgent formats the instructions for an external agent.
func formatExternalAgent(workspaceName string, externalAgents []externalAgent) string {
	var output strings.Builder
	_, _ = output.WriteString(fmt.Sprintf("\nPlease run the following command to attach external agent to the workspace %s:\n\n", cliui.Keyword(workspaceName)))

	for i, agent := range externalAgents {
		if len(externalAgents) > 1 {
			_, _ = output.WriteString(fmt.Sprintf("For agent %s:\n", cliui.Keyword(agent.AgentName)))
		}

		_, _ = output.WriteString(fmt.Sprintf("%s\n", pretty.Sprint(cliui.DefaultStyles.Code, agent.InitScript)))

		if i < len(externalAgents)-1 {
			_, _ = output.WriteString("\n")
		}
	}

	return output.String()
}

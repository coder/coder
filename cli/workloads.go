package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func joinWorkloadScopes(scopes []codersdk.APIKeyScope, separator string) string {
	if len(scopes) == 0 {
		return "none"
	}
	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		names = append(names, string(scope))
	}
	return strings.Join(names, separator)
}

func formatWorkloadIdentity(identity codersdk.WorkloadIdentity) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Workload identity: %s\n", identity.ID)
	switch identity.Kind {
	case codersdk.WorkloadIdentityKindWorkload:
		_, _ = fmt.Fprintf(&b, "  Kind:          workload, vouched for by its workspace daemon (holds no credential)\n")
		_, _ = fmt.Fprintf(&b, "  Name:          %s\n", identity.Name)
		_, _ = fmt.Fprintf(&b, "  Workspace:     %s/%s (%s)\n", identity.OwnerUsername, identity.WorkspaceName, identity.WorkspaceID)
		_, _ = fmt.Fprintf(&b, "  Runs in:       daemon %s (%s)\n", identity.AgentName, identity.AgentID)
		if identity.Sandbox != nil {
			_, _ = fmt.Fprintf(&b, "  Sandbox:       %s %s\n", identity.Sandbox.Runtime, identity.Sandbox.ID)
		}
		_, _ = fmt.Fprintf(&b, "  Sponsor:       %s (%s)\n", identity.SponsorUsername, identity.SponsorID)
		_, _ = fmt.Fprintf(&b, "  Scopes:        %s\n", joinWorkloadScopes(identity.Scopes, ", "))
	default:
		_, _ = fmt.Fprintf(&b, "  Kind:          template-declared daemon, holds an agent token\n")
		_, _ = fmt.Fprintf(&b, "  Daemon:        %s (%s)\n", identity.AgentName, identity.AgentID)
		_, _ = fmt.Fprintf(&b, "  Workspace:     %s/%s (%s)\n", identity.OwnerUsername, identity.WorkspaceName, identity.WorkspaceID)
		_, _ = fmt.Fprintf(&b, "  Acts for user: %s (%s)\n", identity.OwnerUsername, identity.OwnerID)
	}
	return b.String()
}

// agentAuthConfigured reports whether the process runs under a workspace
// daemon and can authenticate to coderd as it.
func (a *AgentAuth) agentAuthConfigured() bool {
	if a.agentURL.String() == "" {
		return false
	}
	return a.agentToken != "" || a.agentTokenFile != "" || a.agentAuth != "token"
}

// workspaceIdentity returns who this process is to Coder inside a
// workspace: a workload in a sandbox, or the workspace daemon. The second
// result is false outside a workspace.
func (r *RootCmd) workspaceIdentity(inv *serpent.Invocation, agentAuth *AgentAuth) (codersdk.WorkloadIdentity, bool, error) {
	ctx := inv.Context()
	// A sandbox has no daemon credential. Its channel is its identity.
	if os.Getenv(codersdk.WorkloadSocketEnv) != "" && !agentAuth.agentAuthConfigured() {
		client, err := r.InitClient(inv)
		if err != nil {
			return codersdk.WorkloadIdentity{}, true, err
		}
		identity, err := client.WorkloadIdentity(ctx)
		return identity, true, err
	}
	if !agentAuth.agentAuthConfigured() {
		return codersdk.WorkloadIdentity{}, false, nil
	}
	client, err := agentAuth.CreateClient()
	if err != nil {
		return codersdk.WorkloadIdentity{}, true, err
	}
	identity, err := client.Identity(ctx)
	return identity, true, err
}

func (r *RootCmd) identity() *serpent.Command {
	agentAuth := &AgentAuth{}
	formatter := cliui.NewOutputFormatter(
		cliui.TextFormatFunc(func(identity codersdk.WorkloadIdentity) (string, error) {
			return formatWorkloadIdentity(identity), nil
		}),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "identity",
		Short:       "Show who this process is to Coder inside a workspace",
		Long: "Inside a workload's sandbox this is the workload's own identity, with its sponsor and scopes. " +
			"Elsewhere in a workspace it is the workspace daemon's. It is separate from the user a human logs in as.",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			identity, inWorkspace, err := r.workspaceIdentity(inv, agentAuth)
			if !inWorkspace {
				return xerrors.New("no workload identity: this command must run inside a workspace")
			}
			if err != nil {
				return xerrors.Errorf("get workload identity: %w", err)
			}
			out, err := formatter.Format(inv.Context(), identity)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(inv.Stdout, strings.TrimRight(out, "\n")+"\n")
			return err
		},
	}
	agentAuth.AttachOptions(cmd, true)
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

type workloadRow struct {
	codersdk.Workload `table:"-"`
	Name              string `json:"-" table:"name,default_sort"`
	Status            string `json:"-" table:"status"`
	Sponsor           string `json:"-" table:"sponsor"`
	Scopes            string `json:"-" table:"scopes"`
	Command           string `json:"-" table:"command"`
	Identity          string `json:"-" table:"identity"`
	Sandbox           string `json:"-" table:"sandbox"`
}

func workloadRows(workloads []codersdk.Workload) []workloadRow {
	rows := make([]workloadRow, 0, len(workloads))
	for _, workload := range workloads {
		row := workloadRow{
			Workload: workload,
			Name:     workload.Name,
			Status:   string(workload.Status),
			Sponsor:  workload.SponsorUsername,
			Scopes:   joinWorkloadScopes(workload.Scopes, ","),
			Command:  workload.Command,
			Identity: workload.ID.String(),
		}
		if workload.Sandbox != nil {
			row.Sandbox = workload.Sandbox.Runtime + ":" + workload.Sandbox.ID
			if len(workload.Sandbox.ID) > 12 {
				row.Sandbox = workload.Sandbox.Runtime + ":" + workload.Sandbox.ID[:12]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (r *RootCmd) run() *serpent.Command {
	var (
		name   string
		agent  string
		image  string
		scopes []string
		detach bool
		list   bool
		remove string
	)
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]workloadRow{}, []string{"name", "status", "sponsor", "scopes", "command", "identity"}),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "run <workspace> [-- command...]",
		Short:       "Run a command in a sandbox inside a workspace, with an identity of its own",
		Long: "Run a command as a workload: in a sandbox that the workspace's daemon creates, without a workspace build. " +
			"The workload gets its own identity, sponsored by you. It can do nothing in Coder unless you give it scopes, " +
			"and it never receives your credentials. The command's output is streamed and its exit code returned.\n" +
			FormatExamples(
				Example{
					Description: "Run a command and see who it is",
					Command:     "coder run my-workspace -- coder identity",
				},
				Example{
					Description: "Let a workload list your workspaces",
					Command:     "coder run my-workspace --scope workspace:read --scope template:read -- coder list",
				},
				Example{
					Description: "List the workloads of a workspace",
					Command:     "coder run --list my-workspace",
				},
				Example{
					Description: "Remove a workload",
					Command:     "coder run --remove workload-1a2b3c4d my-workspace",
				},
			),
		Middleware: serpent.RequireRangeArgs(1, -1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			workspace, err := client.ResolveWorkspace(ctx, inv.Args[0])
			if err != nil {
				return err
			}

			if list {
				workloads, err := client.WorkspaceWorkloads(ctx, workspace.ID)
				if err != nil {
					return err
				}
				out, err := formatter.Format(ctx, workloadRows(workloads))
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(inv.Stdout, out)
				return err
			}
			if remove != "" {
				workloads, err := client.WorkspaceWorkloads(ctx, workspace.ID)
				if err != nil {
					return err
				}
				idx := slices.IndexFunc(workloads, func(workload codersdk.Workload) bool {
					return workload.Name == remove || workload.ID.String() == remove
				})
				if idx < 0 {
					return xerrors.Errorf("workspace %q has no workload %q", workspace.Name, remove)
				}
				if err := client.DeleteWorkspaceWorkload(ctx, workspace.ID, workloads[idx].ID); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Removed workload %s.\n", workloads[idx].Name)
				return nil
			}

			if len(inv.Args) < 2 {
				return xerrors.New("a command is required: coder run <workspace> -- <command>")
			}
			workload, err := client.CreateWorkspaceWorkload(ctx, workspace.ID, codersdk.CreateWorkloadRequest{
				Name:    name,
				Agent:   agent,
				Command: shellquote.Join(inv.Args[1:]...),
				Image:   image,
				Scopes:  slice.StringEnums[codersdk.APIKeyScope](scopes),
			})
			if err != nil {
				return err
			}
			if detach {
				_, err = fmt.Fprintln(inv.Stdout, workload.Name)
				return err
			}

			exitCode := -1
			err = client.WorkspaceWorkloadLogs(ctx, workspace.ID, workload.ID, func(event codersdk.WorkloadLogEvent) error {
				if event.ExitCode != nil {
					exitCode = *event.ExitCode
					return nil
				}
				_, err := fmt.Fprintln(inv.Stdout, event.Output)
				return err
			})
			if ctx.Err() != nil {
				// Interrupted: the workload must not outlive the command
				// that was attached to it.
				removeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				defer cancel()
				_ = client.DeleteWorkspaceWorkload(removeCtx, workspace.ID, workload.ID)
				return ctx.Err()
			}
			if err != nil {
				return xerrors.Errorf("follow workload %s: %w", workload.Name, err)
			}
			switch exitCode {
			case 0:
				return nil
			case -1:
				return xerrors.Errorf("lost track of workload %s before it exited", workload.Name)
			default:
				return ExitError(exitCode, nil)
			}
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "name",
			Description: "A name for the workload.",
			Value:       serpent.StringOf(&name),
		},
		{
			Flag:        "scope",
			Description: "A permission the workload may use on your behalf, as an API token scope such as workspace:read. Repeat it for several. Without any, the workload can do nothing in Coder.",
			Value:       serpent.StringArrayOf(&scopes),
		},
		{
			Flag:        "image",
			Description: "The image for the workload's sandbox. Defaults to the one the workspace's daemon is configured with.",
			Value:       serpent.StringOf(&image),
		},
		{
			Flag:        "agent",
			Description: "The workspace daemon to run the workload in, by name. Only needed when the workspace has several.",
			Value:       serpent.StringOf(&agent),
		},
		{
			Flag:          "detach",
			FlagShorthand: "d",
			Description:   "Start the workload and print its name instead of following its output.",
			Value:         serpent.BoolOf(&detach),
		},
		{
			Flag:          "list",
			FlagShorthand: "l",
			Description:   "List the workspace's workloads instead of running one.",
			Value:         serpent.BoolOf(&list),
		},
		{
			Flag:        "remove",
			Description: "Remove the named workload instead of running one.",
			Value:       serpent.StringOf(&remove),
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kballard/go-shellquote"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty"
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
		if identity.Attestation == codersdk.WorkloadAttestationKubernetesPod {
			_, _ = fmt.Fprintf(&b, "  Kind:          workload with a pod to itself, which its cluster vouches for\n")
			_, _ = fmt.Fprintf(&b, "  Attested:      %s\n", identity.AttestationSubject)
		} else {
			_, _ = fmt.Fprintf(&b, "  Kind:          workload, vouched for by its workspace daemon (holds no credential)\n")
		}
		_, _ = fmt.Fprintf(&b, "  Name:          %s\n", identity.Name)
		_, _ = fmt.Fprintf(&b, "  Workspace:     %s/%s (%s)\n", identity.OwnerUsername, identity.WorkspaceName, identity.WorkspaceID)
		_, _ = fmt.Fprintf(&b, "  Runs in:       daemon %s (%s)\n", identity.AgentName, identity.AgentID)
		if identity.Sandbox != nil {
			_, _ = fmt.Fprintf(&b, "  Sandbox:       %s %s\n", identity.Sandbox.Runtime, identity.Sandbox.ID)
		}
		_, _ = fmt.Fprintf(&b, "  Sponsor:       %s (%s)\n", identity.SponsorUsername, identity.SponsorID)
		_, _ = fmt.Fprintf(&b, "  Scopes:        %s\n", joinWorkloadScopes(identity.Scopes, ", "))
	default:
		if identity.Attestation == codersdk.WorkloadAttestationKubernetesPod {
			_, _ = fmt.Fprintf(&b, "  Kind:          daemon in a pod its cluster vouches for\n")
			_, _ = fmt.Fprintf(&b, "  Attested:      %s\n", identity.AttestationSubject)
		} else {
			_, _ = fmt.Fprintf(&b, "  Kind:          template-declared daemon, holds an agent token\n")
		}
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
	Workspace         string `json:"-" table:"workspace"`
	Status            string `json:"-" table:"status"`
	Sponsor           string `json:"-" table:"sponsor"`
	Scopes            string `json:"-" table:"scopes"`
	Command           string `json:"-" table:"command"`
	Identity          string `json:"-" table:"identity"`
	Sandbox           string `json:"-" table:"sandbox"`
	Network           string `json:"-" table:"network"`
	Image             string `json:"-" table:"image"`
	Started           string `json:"-" table:"started"`
	Finished          string `json:"-" table:"finished"`
}

func workloadRows(workloads []codersdk.Workload) []workloadRow {
	rows := make([]workloadRow, 0, len(workloads))
	for _, workload := range workloads {
		row := workloadRow{
			Workload:  workload,
			Name:      workload.Name,
			Workspace: workload.WorkspaceName,
			Status:    string(workload.Status),
			Sponsor:   workload.SponsorUsername,
			Scopes:    joinWorkloadScopes(workload.Scopes, ","),
			Command:   workload.Command,
			Identity:  workload.ID.String(),
			Network:   string(workload.Network),
			Image:     workload.Image,
			Started:   workload.CreatedAt.Local().Format(time.DateTime),
		}
		if workload.ExitCode != nil {
			row.Status = fmt.Sprintf("%s (%d)", workload.Status, *workload.ExitCode)
		}
		if workload.FinishedAt != nil {
			row.Finished = workload.FinishedAt.Local().Format(time.DateTime)
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
		name     string
		template string
		preset   string
		keep     bool
		agent    string
		image    string
		scopes   []string
		network  string
		mounts   []string
		stdin    bool
		tty      bool
		detach   bool
		list     bool
		all      bool
		remove   string
		logs     string
		timing   bool
	)
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]workloadRow{}, []string{"name", "status", "sponsor", "scopes", "command", "started", "identity"}),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "run [--template <template> | <workspace>] [-- command...]",
		Short:       "Run a command in a sandbox, with an identity of its own",
		Long: "Run a command as a workload. The workload gets its own identity, sponsored by you. It can do nothing in " +
			"Coder unless you give it scopes, and it never receives your credentials. The command's output is streamed " +
			"and its exit code returned.\n\n" +
			"With --template, the workload gets a workspace of its own, which its identity owns and shares with you. " +
			"The workspace is made from the template for this one command, claiming one that was built ahead of time " +
			"when the template keeps such workspaces, and deleted when the command is over. " +
			"Without --template, the workload runs in a sandbox that the daemon of an existing workspace creates.\n" +
			FormatExamples(
				Example{
					Description: "Run a command in a workspace of its own and see who it is",
					Command:     "coder run --template sandbox -- coder identity",
				},
				Example{
					Description: "Keep the workspace afterwards, to look around in it",
					Command:     "coder run --template sandbox --keep --name fix-1 -- make test",
				},
				Example{
					Description: "List your workloads in every workspace, with what became of each",
					Command:     "coder run --all",
				},
				Example{
					Description: "Run a command in a sandbox inside one of your workspaces",
					Command:     "coder run my-workspace -- coder identity",
				},
				Example{
					Description: "Let a workload list your workspaces",
					Command:     "coder run my-workspace --scope workspace:read --scope template:read -- coder list",
				},
				Example{
					Description: "Work in a sandbox with a terminal",
					Command:     "coder run -it my-workspace -- sh",
				},
				Example{
					Description: "List the running workloads of a workspace",
					Command:     "coder run --list my-workspace",
				},
				Example{
					Description: "List all of them, with what became of each",
					Command:     "coder run --all my-workspace",
				},
				Example{
					Description: "Remove a workload",
					Command:     "coder run --remove workload-1a2b3c4d my-workspace",
				},
			),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			began := time.Now()
			// phase reports how long the steps of a run take, when asked.
			phase := func(string) {}
			if timing {
				last := began
				phase = func(step string) {
					now := time.Now()
					_, _ = fmt.Fprintf(inv.Stderr, "timing: %-28s +%4dms  (%4dms)\n", step, now.Sub(last).Milliseconds(), now.Sub(began).Milliseconds())
					last = now
				}
			}
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			phase("signed in")
			// Without a workspace, listing and the like cover the
			// caller's workloads in every workspace.
			if template == "" && len(inv.Args) == 0 {
				switch {
				case list || all:
					workloads, err := client.Workloads(ctx, codersdk.WorkspaceWorkloadsOptions{Finished: all})
					if err != nil {
						return err
					}
					out, err := formatter.Format(ctx, workloadRows(workloads))
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(inv.Stdout, out)
					return err
				case remove != "" || logs != "":
					// Live workloads are listed first, so a running
					// workload wins over an earlier one of the same name.
					workloads, err := client.Workloads(ctx, codersdk.WorkspaceWorkloadsOptions{Finished: true})
					if err != nil {
						return err
					}
					wanted := remove + logs
					idx := slices.IndexFunc(workloads, func(workload codersdk.Workload) bool {
						return workload.Name == wanted || workload.ID.String() == wanted
					})
					if idx < 0 {
						return xerrors.Errorf("you have no workload %q", wanted)
					}
					if logs != "" {
						return followWorkload(ctx, inv, client, workloads[idx])
					}
					if err := client.DeleteWorkspaceWorkload(ctx, workloads[idx].WorkspaceID, workloads[idx].ID); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(inv.Stderr, "Removed workload %s.\n", workloads[idx].Name)
					return nil
				default:
					return xerrors.New("name a workspace, or a template with --template: coder run --template <template> -- <command>")
				}
			}

			command := inv.Args
			var workspace codersdk.Workspace
			if template == "" {
				workspace, err = client.ResolveWorkspace(ctx, inv.Args[0])
				if err != nil {
					return err
				}
				command = inv.Args[1:]
			}

			if list || all {
				if template != "" {
					return xerrors.New("--template makes a workspace for a new workload; it does not go with listing")
				}
				workloads, err := client.WorkspaceWorkloads(ctx, workspace.ID, codersdk.WorkspaceWorkloadsOptions{Finished: all})
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
				workloads, err := client.WorkspaceWorkloads(ctx, workspace.ID, codersdk.WorkspaceWorkloadsOptions{})
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

			if logs != "" {
				// Live workloads are listed first, so a running workload
				// wins over an earlier one of the same name.
				workloads, err := client.WorkspaceWorkloads(ctx, workspace.ID, codersdk.WorkspaceWorkloadsOptions{Finished: true})
				if err != nil {
					return err
				}
				idx := slices.IndexFunc(workloads, func(workload codersdk.Workload) bool {
					return workload.Name == logs || workload.ID.String() == logs
				})
				if idx < 0 {
					return xerrors.Errorf("workspace %q has no workload %q", workspace.Name, logs)
				}
				return followWorkload(ctx, inv, client, workloads[idx])
			}

			if len(command) == 0 {
				return xerrors.New("a command is required: coder run <workspace> -- <command>")
			}
			if stdin != tty {
				return xerrors.New("a terminal needs both --interactive and --tty: use -it")
			}
			if detach && tty {
				return xerrors.New("--detach cannot be combined with -it")
			}
			workloadMounts := make([]codersdk.WorkloadMount, 0, len(mounts))
			for _, mount := range mounts {
				workloadMounts = append(workloadMounts, codersdk.ParseWorkloadMount(mount))
			}
			request := codersdk.CreateWorkloadRequest{
				Name:    name,
				Agent:   agent,
				Command: shellquote.Join(command...),
				Image:   image,
				Network: codersdk.WorkloadNetwork(network),
				Mounts:  workloadMounts,
				Scopes:  slice.StringEnums[codersdk.APIKeyScope](scopes),

				Interactive: stdin && tty,
			}
			var workload codersdk.Workload
			switch {
			case template != "":
				// The workload gets a workspace of its own, named like it.
				// coderd makes both: the workspace belongs to the
				// workload's identity, not to whoever runs this command.
				if request.Name == "" {
					request.Name = "run-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
				}
				templateID, presetID, err := resolveWorkloadTemplate(ctx, client, template, preset)
				if err != nil {
					return err
				}
				phase("template found")
				workload, err = client.LaunchWorkload(ctx, codersdk.LaunchWorkloadRequest{
					CreateWorkloadRequest:   request,
					TemplateID:              templateID,
					TemplateVersionPresetID: presetID,
					Keep:                    keep,
				})
				if err != nil {
					return err
				}
			case keep:
				return xerrors.New("--keep is for a workspace made with --template")
			case preset != "":
				return xerrors.New("--preset is for a workspace made with --template")
			default:
				workload, err = client.CreateWorkspaceWorkload(ctx, workspace.ID, request)
				if err != nil {
					return err
				}
			}
			phase("workload started")
			if detach {
				_, err = fmt.Fprintln(inv.Stdout, workload.Name)
				return err
			}
			if stdin && tty {
				// The terminal shows the output. What remains to learn
				// afterwards is how the command ended.
				if err := attachWorkloadTerminal(ctx, inv, client, workload); err != nil {
					return err
				}
				exitCode := -1
				err = client.WorkspaceWorkloadLogs(ctx, workload.WorkspaceID, workload.ID, func(event codersdk.WorkloadLogEvent) error {
					if event.ExitCode != nil {
						exitCode = *event.ExitCode
					}
					return nil
				})
				if err != nil {
					return xerrors.Errorf("learn how workload %s ended: %w", workload.Name, err)
				}
				if exitCode != 0 {
					return ExitError(exitCode, nil)
				}
				return nil
			}

			err = followWorkload(ctx, inv, client, workload)
			phase("output and exit code")
			if ctx.Err() != nil {
				// Interrupted: the workload must not outlive the command
				// that was attached to it.
				removeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
				defer cancel()
				_ = client.DeleteWorkspaceWorkload(removeCtx, workload.WorkspaceID, workload.ID)
				return ctx.Err()
			}
			return err
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
			Description: "A permission the workload may use, as an API token scope such as workspace:read. Repeat it for several. Without any, the workload can do nothing in Coder.",
			Value:       serpent.StringArrayOf(&scopes),
		},
		{
			Flag:        "image",
			Description: "The image for the workload's sandbox. The workspace's template sets the default and decides which others are allowed.",
			Value:       serpent.StringOf(&image),
		},
		{
			Flag:        "network",
			Description: "What the sandbox can reach: \"none\" or \"outbound\". The workspace's template sets the default. With \"none\" the workload can still reach Coder, and nothing else.",
			Value:       serpent.StringOf(&network),
		},
		{
			Flag:        "mount",
			Description: "A directory of the workspace to show in the sandbox, at the same path. It is read-only unless followed by \":rw\". Repeat it for several. The workspace's template decides which directories may be shared; without this flag the workload sees nothing of the workspace.",
			Value:       serpent.StringArrayOf(&mounts),
		},
		{
			Flag:        "timing",
			Description: "Report how long each step of the run took.",
			Value:       serpent.BoolOf(&timing),
			Hidden:      true,
		},
		{
			Flag:        "template",
			Description: "Give the workload a workspace of its own, made from this template and deleted when the command is over. The workload's identity owns the workspace and shares it with you.",
			Value:       serpent.StringOf(&template),
		},
		{
			Flag:        "preset",
			Description: "With --template, the preset of the template to use. A workspace that was built ahead of time for the preset is claimed when there is one.",
			Value:       serpent.StringOf(&preset),
		},
		{
			Flag:        "keep",
			Description: "With --template, keep the workspace when the command is over. It stays as it is, and can be looked at and removed like any workspace.",
			Value:       serpent.BoolOf(&keep),
		},
		{
			Flag:        "agent",
			Description: "The workspace daemon to run the workload in, by name. Only needed when the workspace has several.",
			Value:       serpent.StringOf(&agent),
		},
		{
			Flag:          "interactive",
			FlagShorthand: "i",
			Description:   "Keep the command's input open. Use it with --tty, as -it.",
			Value:         serpent.BoolOf(&stdin),
		},
		{
			Flag:          "tty",
			FlagShorthand: "t",
			Description:   "Give the command a terminal and attach to it. Use it with --interactive, as -it.",
			Value:         serpent.BoolOf(&tty),
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
			Description:   "List the workspace's running workloads instead of running one.",
			Value:         serpent.BoolOf(&list),
		},
		{
			Flag:          "all",
			FlagShorthand: "a",
			Description:   "List the workspace's workloads, including the most recent ones that are over.",
			Value:         serpent.BoolOf(&all),
		},
		{
			Flag:        "logs",
			Description: "Print the output of a workload, by name or identity, instead of running one. A running workload is followed until it ends. For one that is over, the end of its output was kept.",
			Value:       serpent.StringOf(&logs),
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

// resolveWorkloadTemplate finds the template a workload's workspace is made
// from and, when one is named, the preset of its active version.
func resolveWorkloadTemplate(ctx context.Context, client *codersdk.Client, templateName, presetName string) (templateID, presetID uuid.UUID, err error) {
	templates, err := client.Templates(ctx, codersdk.TemplateFilter{ExactName: templateName})
	if err != nil {
		return uuid.Nil, uuid.Nil, xerrors.Errorf("find template %q: %w", templateName, err)
	}
	switch len(templates) {
	case 0:
		return uuid.Nil, uuid.Nil, xerrors.Errorf("there is no template %q", templateName)
	case 1:
	default:
		return uuid.Nil, uuid.Nil, xerrors.Errorf("several organizations have a template %q", templateName)
	}
	if presetName == "" {
		return templates[0].ID, uuid.Nil, nil
	}
	presets, err := client.TemplateVersionPresets(ctx, templates[0].ActiveVersionID)
	if err != nil {
		return uuid.Nil, uuid.Nil, xerrors.Errorf("list the presets of template %q: %w", templateName, err)
	}
	for _, preset := range presets {
		if preset.Name == presetName {
			return templates[0].ID, preset.ID, nil
		}
	}
	return uuid.Nil, uuid.Nil, xerrors.Errorf("template %q has no preset %q", templateName, presetName)
}

// followWorkload prints a workload's output until its command ends, and
// ends the way the command did. For a workload that is over it prints what
// was kept of its output.
func followWorkload(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, workload codersdk.Workload) error {
	exitCode := -1
	err := client.WorkspaceWorkloadLogs(ctx, workload.WorkspaceID, workload.ID, func(event codersdk.WorkloadLogEvent) error {
		if event.ExitCode != nil {
			exitCode = *event.ExitCode
			return nil
		}
		_, err := fmt.Fprintln(inv.Stdout, event.Output)
		return err
	})
	if err != nil {
		return xerrors.Errorf("follow workload %s: %w", workload.Name, err)
	}
	switch {
	case exitCode == 0:
		return nil
	case exitCode == -1 && workload.FinishedAt != nil:
		_, _ = fmt.Fprintf(inv.Stderr, "Workload %s is over (%s). Its exit code is not known.\n", workload.Name, workload.Status)
		return nil
	case exitCode == -1:
		return xerrors.Errorf("lost track of workload %s before it exited", workload.Name)
	default:
		return ExitError(exitCode, nil)
	}
}

// attachWorkloadTerminal connects the caller's terminal to an interactive
// workload until the workload's command ends.
func attachWorkloadTerminal(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, workload codersdk.Workload) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var width, height uint16
	stdoutFile, validOut := inv.Stdout.(*os.File)
	if validOut && isatty.IsTerminal(stdoutFile.Fd()) {
		if w, h, err := term.GetSize(int(stdoutFile.Fd())); err == nil {
			//nolint:gosec // A terminal's size fits.
			width, height = uint16(w), uint16(h)
		}
	}
	// Raw mode, so that control characters reach the workload.
	if stdinFile, ok := inv.Stdin.(*os.File); ok && isatty.IsTerminal(stdinFile.Fd()) {
		state, err := pty.MakeInputRaw(stdinFile.Fd())
		if err != nil {
			return xerrors.Errorf("set the terminal to raw mode: %w", err)
		}
		defer func() { _ = pty.RestoreTerminal(stdinFile.Fd(), state) }()
	}

	conn, err := client.AttachWorkspaceWorkload(ctx, workload.WorkspaceID, workload.ID, height, width)
	if err != nil {
		return xerrors.Errorf("attach to workload %s: %w", workload.Name, err)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	go func() {
		input := bufio.NewScanner(inv.Stdin)
		// Byte by byte: a line would only be sent once it is complete.
		input.Split(bufio.ScanBytes)
		for input.Scan() {
			if err := encoder.Encode(map[string]string{"data": input.Text()}); err != nil {
				return
			}
		}
	}()
	if validOut {
		resized := listenWindowSize(ctx)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-resized:
				}
				w, h, err := term.GetSize(int(stdoutFile.Fd()))
				if err != nil {
					continue
				}
				_ = encoder.Encode(map[string]int{"width": w, "height": h})
			}
		}()
	}
	_, _ = io.Copy(inv.Stdout, conn)
	return nil
}

// workloadInit starts a workload's command inside its sandbox. The sandbox's
// channel to Coder is a unix socket, which many programs cannot use, so this
// first offers the channel on a loopback address and points CODER_URL at it.
// Everything here runs inside the sandbox and holds no secret: whatever can
// reach the loopback address could reach the socket as well.
func (*RootCmd) workloadInit() *serpent.Command {
	return &serpent.Command{
		Use:        "workload-init -- <command> [args...]",
		Short:      "Run a workload's command inside its sandbox",
		Hidden:     true,
		Middleware: serpent.RequireRangeArgs(1, -1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			env := os.Environ()
			if socket := os.Getenv(codersdk.WorkloadSocketEnv); socket != "" {
				// A sandbox without a network still has loopback.
				listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
				if err == nil {
					defer listener.Close()
					proxy := &httputil.ReverseProxy{
						Rewrite: func(pr *httputil.ProxyRequest) {
							pr.SetURL(&url.URL{Scheme: "http", Host: "coder.workload"})
						},
						Transport: &http.Transport{
							DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
								return (&net.Dialer{}).DialContext(ctx, "unix", socket)
							},
						},
						FlushInterval: -1,
					}
					server := &http.Server{Handler: proxy, ReadHeaderTimeout: 30 * time.Second}
					go func() { _ = server.Serve(listener) }()
					defer server.Close()
					env = append(env, "CODER_URL=http://"+listener.Addr().String())
				}
			}

			if term := os.Getenv(codersdk.WorkloadTermEnv); term != "" {
				env = slices.DeleteFunc(env, func(kv string) bool {
					return strings.HasPrefix(kv, "TERM=") || strings.HasPrefix(kv, codersdk.WorkloadTermEnv+"=")
				})
				env = append(env, "TERM="+term)
			}

			//nolint:gosec // Running the given command is this command's purpose.
			cmd := exec.Command(inv.Args[0], inv.Args[1:]...)
			cmd.Env = env
			cmd.Stdin, cmd.Stdout, cmd.Stderr = inv.Stdin, inv.Stdout, inv.Stderr
			if err := cmd.Start(); err != nil {
				return xerrors.Errorf("start workload command: %w", err)
			}
			signals := make(chan os.Signal, 4)
			signal.Notify(signals, StopSignals...)
			defer signal.Stop(signals)
			go func() {
				for sig := range signals {
					_ = cmd.Process.Signal(sig)
				}
			}()
			err := cmd.Wait()
			var exitErr *exec.ExitError
			if xerrors.As(err, &exitErr) {
				return ExitError(exitErr.ExitCode(), nil)
			}
			return err
		},
	}
}

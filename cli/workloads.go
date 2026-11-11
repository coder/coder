package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kballard/go-shellquote"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/workloadoperator"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// workloadCommandLogSource is the log source coderd gives the command of a
// runtime workload.
const workloadCommandLogSource = "Workload command"

// formatWorkloadIdentity renders a workload identity for a terminal.
func formatWorkloadIdentity(identity codersdk.WorkloadIdentity) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Workload identity: %s\n", identity.ID)
	switch identity.Kind {
	case codersdk.WorkloadIdentityKindRuntime:
		_, _ = fmt.Fprintf(&b, "  Kind:          runtime daemon, attested by its Kubernetes pod\n")
	case codersdk.WorkloadIdentityKindDeclared:
		_, _ = fmt.Fprintf(&b, "  Kind:          template-declared daemon, holds an agent token (not pod-attested)\n")
	default:
		_, _ = fmt.Fprintf(&b, "  Kind:          %s (%s)\n", identity.Kind, identity.Attestation)
	}
	_, _ = fmt.Fprintf(&b, "  Daemon:        %s (%s)\n", identity.AgentName, identity.AgentID)
	_, _ = fmt.Fprintf(&b, "  Workspace:     %s/%s (%s)\n", identity.OwnerUsername, identity.WorkspaceName, identity.WorkspaceID)
	if identity.IdentityID != uuid.Nil {
		scopes := "none"
		if len(identity.Scopes) > 0 {
			names := make([]string, 0, len(identity.Scopes))
			for _, scope := range identity.Scopes {
				names = append(names, string(scope))
			}
			scopes = strings.Join(names, ", ")
		}
		_, _ = fmt.Fprintf(&b, "  Identity ID:   %s\n", identity.IdentityID)
		_, _ = fmt.Fprintf(&b, "  Sponsor:       %s (%s)\n", identity.SponsorUsername, identity.SponsorID)
		_, _ = fmt.Fprintf(&b, "  Scopes:        %s\n", scopes)
	} else {
		_, _ = fmt.Fprintf(&b, "  Acts for user: %s (%s)\n", identity.OwnerUsername, identity.OwnerID)
	}
	if identity.Shape != "" {
		_, _ = fmt.Fprintf(&b, "  Shape:         %s\n", identity.Shape)
	}
	if identity.Pod != nil {
		_, _ = fmt.Fprintf(&b, "  Pod:           %s/%s (%s)\n", identity.Pod.Namespace, identity.Pod.Name, identity.Pod.UID)
	}
	return b.String()
}

// agentAuthConfigured reports whether the process runs under a workspace
// daemon that can authenticate to coderd.
func (a *AgentAuth) agentAuthConfigured() bool {
	if a.agentURL.String() == "" {
		return false
	}
	return a.agentToken != "" || a.agentTokenFile != "" || a.agentPodTokenFile != "" || a.agentAuth != "token"
}

// workloadIdentity returns the identity of the workspace daemon this
// process runs under.
func (a *AgentAuth) workloadIdentity(ctx context.Context) (codersdk.WorkloadIdentity, error) {
	client, err := a.CreateClient()
	if err != nil {
		return codersdk.WorkloadIdentity{}, err
	}
	return client.Identity(ctx)
}

func (*RootCmd) identity() *serpent.Command {
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
		Short:       "Show the workload identity of the workspace daemon this command runs under",
		Long: "The workload identity is who Coder considers this workspace daemon to be. " +
			"It is separate from the user a human logs in as, and is always associated with the workspace owner.",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			if !agentAuth.agentAuthConfigured() {
				return xerrors.New("no workload identity: this command must run inside a workspace, under a workspace daemon")
			}
			identity, err := agentAuth.workloadIdentity(inv.Context())
			if err != nil {
				return xerrors.Errorf("get workload identity: %w", err)
			}
			out, err := formatter.Format(inv.Context(), identity)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, strings.TrimRight(out, "\n"))
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
	Shape             string `json:"-" table:"shape"`
	Sponsor           string `json:"-" table:"sponsor"`
	Scopes            string `json:"-" table:"scopes"`
	Identity          string `json:"-" table:"identity"`
	Pod               string `json:"-" table:"pod"`
}

func workloadRows(workloads []codersdk.Workload) []workloadRow {
	rows := make([]workloadRow, 0, len(workloads))
	for _, workload := range workloads {
		row := workloadRow{
			Workload: workload,
			Name:     workload.Name,
			Status:   string(workload.Status),
			Shape:    workload.Shape,
			Sponsor:  workload.SponsorUsername,
			Scopes:   "none",
			Identity: workload.Identity,
		}
		if len(workload.Scopes) > 0 {
			names := make([]string, 0, len(workload.Scopes))
			for _, scope := range workload.Scopes {
				names = append(names, string(scope))
			}
			row.Scopes = strings.Join(names, ",")
		}
		if workload.Pod != nil {
			row.Pod = workload.Pod.Namespace + "/" + workload.Pod.Name
		}
		rows = append(rows, row)
	}
	return rows
}

func (r *RootCmd) run() *serpent.Command {
	var (
		shape   string
		name    string
		list    bool
		remove  string
		noWait  bool
		timeout time.Duration
		scopes  []string
	)
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]workloadRow{}, []string{"name", "status", "shape", "sponsor", "scopes", "identity"}),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "run <workspace> [command...]",
		Short:       "Run a workload in its own workspace daemon, provisioned at runtime",
		Long: "Provision a new workspace daemon from the shape the workspace's template declares, without a workspace build. " +
			"Each workload gets its own identity. The command, if any, is run by the new daemon and its output is streamed.\n" +
			FormatExamples(
				Example{
					Description: "Start a workload and run a command in it",
					Command:     "coder run my-workspace -- coder identity",
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
				idx := slices.IndexFunc(workloads, func(w codersdk.Workload) bool {
					return w.Name == remove || w.ID.String() == remove
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

			workload, err := client.CreateWorkspaceWorkload(ctx, workspace.ID, codersdk.CreateWorkloadRequest{
				Name:    name,
				Shape:   shape,
				Command: shellquote.Join(inv.Args[1:]...),
				Scopes:  slice.StringEnums[codersdk.APIKeyScope](scopes),
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Requested workload %s from shape %q.\n", workload.Name, workload.Shape)
			if noWait {
				_, err = fmt.Fprintln(inv.Stdout, workload.ID)
				return err
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			workload, err = waitForWorkload(ctx, inv, client, workspace.ID, workload.ID)
			if err != nil {
				return err
			}
			if workload.Command == "" {
				_, err = fmt.Fprintln(inv.Stdout, workload.Identity)
				return err
			}
			return streamWorkloadCommand(ctx, inv, client, workload)
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "shape",
			Description: "The workload shape to clone, by agent name. Only needed when the template declares several.",
			Value:       serpent.StringOf(&shape),
		},
		{
			Flag:        "name",
			Description: "A name for the workload. It becomes the name of its workspace agent.",
			Value:       serpent.StringOf(&name),
		},
		{
			Flag:          "list",
			FlagShorthand: "l",
			Description:   "List the workspace's workloads instead of starting one.",
			Value:         serpent.BoolOf(&list),
		},
		{
			Flag:        "remove",
			Description: "Remove the named workload instead of starting one.",
			Value:       serpent.StringOf(&remove),
		},
		{
			Flag:        "scope",
			Description: "A permission the workload may use on your behalf, as an API token scope such as workspace:read. Repeat it for several. Without any, the workload can do nothing in Coder.",
			Value:       serpent.StringArrayOf(&scopes),
		},
		{
			Flag:        "no-wait",
			Description: "Return as soon as the workload is requested, and print its ID.",
			Value:       serpent.BoolOf(&noWait),
		},
		{
			Flag:        "timeout",
			Description: "How long to wait for the workload to start and its command to finish.",
			Default:     "10m",
			Value:       serpent.DurationOf(&timeout),
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// waitForWorkload waits until a workload's daemon is connected, reporting
// each status change.
func waitForWorkload(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, workspaceID, workloadID uuid.UUID) (codersdk.Workload, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last codersdk.WorkloadStatus
	for {
		workloads, err := client.WorkspaceWorkloads(ctx, workspaceID)
		if err != nil {
			return codersdk.Workload{}, err
		}
		idx := slices.IndexFunc(workloads, func(w codersdk.Workload) bool { return w.ID == workloadID })
		if idx < 0 {
			return codersdk.Workload{}, xerrors.New("the workload was removed before it started")
		}
		workload := workloads[idx]
		if workload.Status != last {
			last = workload.Status
			switch workload.Status {
			case codersdk.WorkloadStatusPending:
				_, _ = fmt.Fprintln(inv.Stderr, "Waiting for the workload operator to create a pod...")
			case codersdk.WorkloadStatusStarting:
				_, _ = fmt.Fprintf(inv.Stderr, "Pod %s/%s created. Identity: %s\nWaiting for its daemon to connect...\n",
					workload.Pod.Namespace, workload.Pod.Name, workload.Identity)
			case codersdk.WorkloadStatusRunning:
				_, _ = fmt.Fprintf(inv.Stderr, "Daemon %s connected.\n", workload.Name)
				return workload, nil
			case codersdk.WorkloadStatusDisconnected:
				return codersdk.Workload{}, xerrors.New("the workload's daemon disconnected")
			}
		}
		select {
		case <-ctx.Done():
			return codersdk.Workload{}, xerrors.Errorf("timed out waiting for workload %s (%s): %w", workload.Name, workload.Status, ctx.Err())
		case <-ticker.C:
		}
	}
}

// streamWorkloadCommand prints the output of a workload's command until the
// command finishes. It fails if the command failed.
func streamWorkloadCommand(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, workload codersdk.Workload) error {
	logs, closer, err := client.WorkspaceAgentLogsAfter(ctx, workload.ID, 0, true)
	if err != nil {
		return xerrors.Errorf("follow workload logs: %w", err)
	}
	defer closer.Close()

	// The command's log source is only listed once the daemon has it, so it
	// is looked up lazily. Logs from other sources, such as the shape's own
	// startup scripts, are not the command's output.
	var commandSource uuid.UUID
	var buffered []codersdk.WorkspaceAgentLog
	flush := func(agent codersdk.WorkspaceAgent) {
		if commandSource == uuid.Nil {
			for _, source := range agent.LogSources {
				if source.DisplayName == workloadCommandLogSource {
					commandSource = source.ID
				}
			}
		}
		if commandSource == uuid.Nil {
			return
		}
		for _, log := range buffered {
			if log.SourceID == commandSource {
				_, _ = fmt.Fprintln(inv.Stdout, log.Output)
			}
		}
		buffered = nil
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// finished is set once the daemon reports that its startup scripts,
	// which include the command, are done. Logs can trail that report.
	var finished time.Time
	var lifecycle codersdk.WorkspaceAgentLifecycle
	for {
		select {
		case <-ctx.Done():
			return xerrors.Errorf("timed out waiting for the command: %w", ctx.Err())
		case batch, ok := <-logs:
			if !ok {
				return xerrors.New("the workload's log stream closed before its command finished")
			}
			buffered = append(buffered, batch...)
			if !finished.IsZero() {
				finished = time.Now()
			}
		case <-ticker.C:
		}

		agent, err := client.WorkspaceAgent(ctx, workload.ID)
		if err != nil {
			return xerrors.Errorf("get workload daemon: %w", err)
		}
		flush(agent)
		if finished.IsZero() && !agent.LifecycleState.Starting() {
			finished = time.Now()
			lifecycle = agent.LifecycleState
		}
		if !finished.IsZero() && time.Since(finished) > 3*time.Second {
			if lifecycle != codersdk.WorkspaceAgentLifecycleReady {
				return xerrors.Errorf("the workload's command did not succeed (daemon lifecycle: %s)", lifecycle)
			}
			return nil
		}
	}
}

func (r *RootCmd) workloadOperator() *serpent.Command {
	var (
		name           string
		namespace      string
		audience       string
		serviceAccount string
		interval       time.Duration
		kubeURL        string
		kubeTokenFile  string
		kubeCAFile     string
	)
	orgContext := NewOrganizationContext()
	cmd := &serpent.Command{
		Use:   "workload-operator",
		Short: "Run runtime workloads for Coder workspaces in a Kubernetes namespace",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			if namespace == "" {
				raw, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
				if err != nil {
					return xerrors.New("the namespace is unknown: set --namespace")
				}
				namespace = strings.TrimSpace(string(raw))
			}
			if audience == "" {
				audience = client.URL.String()
			}
			kube, err := workloadoperator.NewKubeClient(workloadoperator.KubeConfig{
				URL: kubeURL, TokenFile: kubeTokenFile, CAFile: kubeCAFile,
			})
			if err != nil {
				return err
			}
			operator, err := workloadoperator.New(workloadoperator.Options{
				Client:         client,
				Kube:           kube,
				Logger:         slog.Make(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelInfo),
				OrganizationID: org.ID,
				Name:           name,
				Namespace:      namespace,
				Interval:       interval,
				Pod: workloadoperator.PodOptions{
					Audience:       audience,
					ServiceAccount: serviceAccount,
				},
			})
			if err != nil {
				return err
			}
			err = operator.Run(ctx)
			if xerrors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "name",
			Env:         "CODER_WORKLOAD_OPERATOR_NAME",
			Description: "The operator's name. It must be unique within the organization.",
			Default:     "default",
			Value:       serpent.StringOf(&name),
		},
		{
			Flag:        "namespace",
			Env:         "CODER_WORKLOAD_OPERATOR_NAMESPACE",
			Description: "The namespace to serve. Defaults to the operator's own namespace.",
			Value:       serpent.StringOf(&namespace),
		},
		{
			Flag:        "audience",
			Env:         "CODER_WORKLOAD_OPERATOR_AUDIENCE",
			Description: "The audience of the identity tokens of workload pods. Defaults to the Coder URL.",
			Value:       serpent.StringOf(&audience),
		},
		{
			Flag:        "workload-service-account",
			Env:         "CODER_WORKLOAD_OPERATOR_WORKLOAD_SERVICE_ACCOUNT",
			Description: "The service account workload pods run as. It must have no permissions.",
			Default:     "coder-workload",
			Value:       serpent.StringOf(&serviceAccount),
		},
		{
			Flag:        "interval",
			Env:         "CODER_WORKLOAD_OPERATOR_INTERVAL",
			Description: "The time between syncs with Coder.",
			Default:     "2s",
			Value:       serpent.DurationOf(&interval),
		},
		{
			Flag:        "kube-url",
			Env:         "CODER_WORKLOAD_OPERATOR_KUBE_URL",
			Description: "The Kubernetes API URL. Defaults to the in-cluster API server.",
			Value:       serpent.StringOf(&kubeURL),
		},
		{
			Flag:        "kube-token-file",
			Env:         "CODER_WORKLOAD_OPERATOR_KUBE_TOKEN_FILE",
			Description: "A file holding a Kubernetes API token. Defaults to the in-cluster service account token.",
			Value:       serpent.StringOf(&kubeTokenFile),
		},
		{
			Flag:        "kube-ca-file",
			Env:         "CODER_WORKLOAD_OPERATOR_KUBE_CA_FILE",
			Description: "The Kubernetes API server's CA certificate. Defaults to the in-cluster CA.",
			Value:       serpent.StringOf(&kubeCAFile),
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}

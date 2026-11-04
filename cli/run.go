package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

const (
	workloadReconnectInterval = 50 * time.Millisecond
	workloadRequestTimeout    = 10 * time.Second
	workloadStreamAttempts    = 8
)

// workloadEnvFlags preserves commas, quotes, and newlines without CSV parsing.
// Validation happens in the handler so flag errors never echo environment values.
type workloadEnvFlags []string

func (e *workloadEnvFlags) Set(value string) error {
	*e = append(*e, value)
	return nil
}

func (*workloadEnvFlags) String() string { return "" }
func (*workloadEnvFlags) Type() string   { return "key=value" }

func (r *RootCmd) run() *serpent.Command {
	var templateName, cpu, ram, command string
	var interactive, tty bool
	var environment workloadEnvFlags
	orgContext := NewOrganizationContext()
	cmd := &serpent.Command{
		Use:        "run",
		Short:      "Run a command on prepared template capacity",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			var timings *workloadRunTimings
			if r.verbose {
				timings = &workloadRunTimings{started: r.clock.Now()}
			}
			if strings.TrimSpace(templateName) == "" {
				return xerrors.New("--template must be a template name or UUID")
			}
			cpus, memory, err := codersdk.ParseWorkloadResources(cpu, ram)
			if err != nil {
				return err
			}
			var env map[string]string
			for _, entry := range environment {
				key, value, ok := strings.Cut(entry, "=")
				if !ok {
					return xerrors.New("--env requires KEY=value")
				}
				if env == nil {
					env = make(map[string]string)
				}
				env[key] = value
			}
			req := codersdk.CreateWorkloadRequest{
				ID:          uuid.New(),
				CPU:         cpus,
				MemoryBytes: memory,
				Command:     command,
				Env:         env,
				Stdin:       interactive,
				TTY:         tty,
			}
			if tty {
				req.Width, req.Height, err = workloadTerminalSize(inv)
				if err != nil {
					return err
				}
			}
			// Validate all local input before making any API requests. The
			// real template ID is resolved below, before placement.
			req.TemplateID = req.ID
			if err := req.Validate(); err != nil {
				return err
			}

			// HTTP body diagnostics would expose the command, including any
			// secrets it contains. Disable them even under --debug-http.
			clientRoot := *r
			clientRoot.debugHTTP = false
			if !r.verbose {
				clientRoot.noVersionCheck = true
				clientRoot.noFeatureWarning = true
			}
			client, err := clientRoot.InitClient(inv)
			if err != nil {
				return err
			}
			template, err := resolveRunTemplate(inv, client, orgContext, templateName)
			if err != nil {
				return err
			}
			req.TemplateID = template.ID
			if err := req.Validate(); err != nil {
				return err
			}
			return r.runWorkload(inv, client, template.OrganizationID, req, timings)
		},
		Options: serpent.OptionSet{
			{
				Flag:        "template",
				Description: "Template name or UUID. Required.",
				Required:    true,
				Value:       serpent.StringOf(&templateName),
			},
			{
				Flag:        "cpu",
				Description: "CPU quota as a positive whole number. Required; no template default is used.",
				Required:    true,
				Value:       serpent.StringOf(&cpu),
			},
			{
				Flag:        "ram",
				Description: "Memory as integer MiB or GiB, at least 16MiB. Required; no template default is used.",
				Required:    true,
				Value:       serpent.StringOf(&ram),
			},
			{
				Flag:        "cmd",
				Description: "Command to execute in the prepared environment. Required.",
				Required:    true,
				Value:       serpent.StringOf(&command),
			},
			{
				Flag:        "env",
				Description: "Set a container environment variable as KEY=value. Repeat for multiple variables; the last value wins. Values are retained in admin-only run history.",
				Value:       &environment,
			},
			{
				Flag:          "interactive",
				FlagShorthand: "i",
				Description:   "Forward stdin to the workload.",
				Value:         serpent.BoolOf(&interactive),
			},
			{
				Flag:          "tty",
				FlagShorthand: "t",
				Description:   "Allocate a terminal and forward resize events. Requires --interactive and a local terminal. Terminal output combines stdout and stderr.",
				Value:         serpent.BoolOf(&tty),
			},
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}

func resolveRunTemplate(inv *serpent.Invocation, client *codersdk.Client, orgContext *OrganizationContext, name string) (codersdk.Template, error) {
	var templates []codersdk.Template
	if id, err := uuid.Parse(name); err == nil {
		template, err := client.Template(inv.Context(), id)
		if err != nil {
			return codersdk.Template{}, xerrors.Errorf("get template by ID: %w", err)
		}
		templates = []codersdk.Template{template}
	} else {
		var err error
		templates, err = client.Templates(inv.Context(), codersdk.TemplateFilter{ExactName: name})
		if err != nil {
			return codersdk.Template{}, xerrors.Errorf("get template by name: %w", err)
		}
	}
	if len(templates) == 0 {
		return codersdk.Template{}, xerrors.Errorf("no template found with the name %q", name)
	}

	if orgContext.FlagSelect != "" {
		org, err := orgContext.Selected(inv, client)
		if err != nil {
			return codersdk.Template{}, err
		}
		templates = slices.DeleteFunc(templates, func(template codersdk.Template) bool {
			return template.OrganizationID != org.ID
		})
		if len(templates) == 0 {
			return codersdk.Template{}, xerrors.Errorf("template %q is not in organization %q", name, org.Name)
		}
	}
	if len(templates) != 1 {
		orgNames := make([]string, 0, len(templates))
		for _, template := range templates {
			orgNames = append(orgNames, template.OrganizationName)
		}
		slices.Sort(orgNames)
		return codersdk.Template{}, xerrors.Errorf("multiple templates found with the name %q; use --org=<organization_name> to select one. Organizations available: %s", name, strings.Join(orgNames, ", "))
	}
	return templates[0], nil
}

func (r *RootCmd) runWorkload(inv *serpent.Invocation, client *codersdk.Client, org uuid.UUID, req codersdk.CreateWorkloadRequest, timings *workloadRunTimings) error {
	// Only an explicit SIGINT cancels the remote workload. Losing the
	// invocation context, connection, or output consumer must not cancel it.
	interruptCtx, stop := inv.SignalNotifyContext(context.WithoutCancel(inv.Context()), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()
	cancellation := make(chan error, 1)
	requestCancellation := func() {
		cancel()
		// Output writes can block indefinitely. Dispatch independently of
		// streaming, and never write diagnostics from this callback.
		cancelCtx, cancelRequest := context.WithTimeout(context.WithoutCancel(inv.Context()), workloadRequestTimeout)
		defer cancelRequest()
		cancellation <- client.CancelWorkload(cancelCtx, org, req.ID)
	}
	stopInterrupt := context.AfterFunc(interruptCtx, requestCancellation)
	defer stopInterrupt()

	err := r.createAndWatchWorkload(ctx, inv, client, org, req, timings)
	if stopInterrupt() {
		if interruptCtx.Err() == nil {
			return err
		}
		// SIGINT arrived, but its callback had not started when stopped.
		// Run it here so this race neither loses nor duplicates cancellation.
		requestCancellation()
	}
	// If the callback started, join it before reporting its outcome. The
	// buffered channel lets it finish even while output remains blocked.
	if err := <-cancellation; err != nil {
		return ExitError(130, xerrors.Errorf("cancel workload %s: %w", req.ID, err))
	}
	if r.verbose {
		_, _ = fmt.Fprintf(inv.Stderr, "Cancellation requested for workload %s.\n", req.ID)
	}
	return ExitError(130, nil)
}

func (r *RootCmd) createAndWatchWorkload(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, org uuid.UUID, req codersdk.CreateWorkloadRequest, timings *workloadRunTimings) error {
	// Never retry placement here. Output streams reconnect to this same identity.
	createCtx, cancel := context.WithTimeout(ctx, workloadRequestTimeout)
	workload, err := client.CreateWorkload(createCtx, org, req)
	cancel()
	if err != nil {
		return xerrors.Errorf("create workload %s: %w", req.ID, err)
	}
	if timings != nil {
		timings.dispatch = r.clock.Since(timings.started)
	}
	if r.verbose {
		// Workspace visibility is independent of permission to run workloads.
		// A failed diagnostic lookup must not interrupt an admitted command.
		workspace, err := workloadWorkspaceName(ctx, client, org, workload.AgentID)
		if err != nil {
			_, _ = fmt.Fprintf(inv.Stderr, "Attached to agent: %s (workspace name unavailable: %v)\n", workload.AgentID, err)
		} else {
			_, _ = fmt.Fprintf(inv.Stderr, "Attached to workspace: %s (agent: %s)\n", workspace, workload.AgentID)
		}
		_, _ = fmt.Fprintf(inv.Stderr, "Workload: %s\nGeneration: %s\nWorkdir: %q\n", workload.ID, workload.Generation.ID, workload.Generation.Workdir)
		for _, repository := range slices.Sorted(maps.Keys(workload.Generation.Repositories)) {
			_, _ = fmt.Fprintf(inv.Stderr, "Repository: %q @ %s\n", repository, workload.Generation.Repositories[repository])
		}
	}
	if req.Stdin && !workload.Terminal() {
		return watchInteractiveWorkload(ctx, inv, client, org, req, r.clock, timings)
	}
	return watchWorkload(ctx, inv, client, org, workload.ID, r.clock, timings)
}

func workloadWorkspaceName(ctx context.Context, client *codersdk.Client, org, agent uuid.UUID) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, workloadRequestTimeout)
	defer cancel()
	filter := codersdk.WorkspaceFilter{Organization: org.String(), Limit: 100}
	for {
		workspaces, err := client.Workspaces(ctx, filter)
		if err != nil {
			return "", err
		}
		for _, workspace := range workspaces.Workspaces {
			for _, resource := range workspace.LatestBuild.Resources {
				for _, candidate := range resource.Agents {
					if candidate.ID == agent {
						return workspace.OwnerName + "/" + workspace.Name, nil
					}
				}
			}
		}
		filter.Offset += len(workspaces.Workspaces)
		if len(workspaces.Workspaces) == 0 || filter.Offset >= workspaces.Count {
			return "", xerrors.New("host workspace is not visible")
		}
	}
}

func watchWorkload(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, org, id uuid.UUID, clock quartz.Clock, timings *workloadRunTimings) error {
	var offset int64
	var failures int
	var warnedTruncated, warnedRateLimit, warnedConnection bool
	readStream := func(body io.ReadCloser) (done bool, err error) {
		defer body.Close()
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 64<<10), (4<<20)+1)
		for scanner.Scan() {
			if len(scanner.Bytes()) == 0 {
				continue // Heartbeat, not output or execution progress.
			}
			var output codersdk.WorkloadOutput
			if err := json.Unmarshal(scanner.Bytes(), &output); err != nil {
				return false, xerrors.New("workload output stream contained an incomplete or invalid frame")
			}
			if output.NextOffset < offset || output.NextOffset-offset < int64(len(output.Events)) {
				return true, xerrors.New("workload output returned an invalid event offset")
			}
			if timings != nil && output.Truncated && !warnedTruncated {
				_, _ = fmt.Fprintln(inv.Stderr, "Warning: retained workload output was truncated.")
				warnedTruncated = true
			}
			for _, event := range output.Events {
				if timings != nil && timings.firstOutput == nil && len(event.Data) > 0 {
					timings.firstOutput = new(clock.Since(timings.started))
				}
				var writer io.Writer
				switch event.Stream {
				case "stdout":
					writer = inv.Stdout
				case "stderr":
					writer = inv.Stderr
				default:
					return true, xerrors.New("workload output returned an unknown stream")
				}
				n, err := writer.Write(event.Data)
				if err == nil && n != len(event.Data) {
					err = io.ErrShortWrite
				}
				if err != nil {
					return true, xerrors.Errorf("write workload output: %w", err)
				}
			}
			// Empty snapshots and heartbeats must not reset the retry budget for
			// a stream that repeatedly disconnects without delivering new output.
			if output.NextOffset > offset {
				failures = 0
			}
			offset = output.NextOffset
			// A terminal batch can still contain output. The empty terminal frame
			// confirms that all retained output has been delivered.
			if output.Workload.Terminal() && len(output.Events) == 0 {
				if output.Workload.ExitCode == nil {
					if output.Workload.Failure != "" {
						return true, xerrors.Errorf("workload %s ended in state %q: %s", id, output.Workload.State, output.Workload.Failure)
					}
					return true, xerrors.Errorf("workload %s ended in state %q without an exit code", id, output.Workload.State)
				}
				if timings != nil {
					timings.print(inv.Stderr, output.Workload, clock.Since(timings.started))
				}
				if *output.Workload.ExitCode != 0 {
					return true, ExitError(*output.Workload.ExitCode, nil)
				}
				return true, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, io.ErrUnexpectedEOF
	}
	for {
		body, err := client.StreamWorkloadOutput(ctx, org, id, offset)
		if err == nil {
			var done bool
			done, err = readStream(body)
			if done {
				return err
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		failures++
		if failures >= workloadStreamAttempts || !retryWorkloadStream(err) {
			return xerrors.Errorf("observe workload %s (the workload has not been canceled): %w", id, err)
		}
		delay := min(workloadReconnectInterval*time.Duration(1<<(failures-1)), time.Second)
		var sdkErr *codersdk.Error
		if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusTooManyRequests {
			delay = time.Minute
			var retryAfter interface{ RetryAfter() time.Duration }
			if errors.As(err, &retryAfter) && retryAfter.RetryAfter() > 0 {
				delay = retryAfter.RetryAfter()
			}
			if timings != nil && !warnedRateLimit {
				_, _ = fmt.Fprintf(inv.Stderr, "Output stream rate limited (HTTP 429); reconnecting in %s.\n", delay)
				warnedRateLimit = true
			}
		} else if timings != nil && !warnedConnection {
			if sdkErr != nil {
				_, _ = fmt.Fprintf(inv.Stderr, "Output stream failed (HTTP %d); reconnecting to workload %s.\n", sdkErr.StatusCode(), id)
			} else {
				_, _ = fmt.Fprintf(inv.Stderr, "Connection interrupted; reconnecting to workload %s.\n", id)
			}
			warnedConnection = true
		}
		timer := clock.NewTimer(delay, "workload-reconnect")
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type workloadRunTimings struct {
	started     time.Time
	dispatch    time.Duration
	firstOutput *time.Duration
}

func (t *workloadRunTimings) print(w io.Writer, workload codersdk.Workload, total time.Duration) {
	provision, exec := workload.Timings.ProvisionNS, workload.Timings.ExecNS
	phases := []struct {
		name   string
		ns     *int64
		tenths int
	}{
		{name: "Provision", ns: provision},
		{name: "Exec", ns: exec},
		{name: "Overhead"},
	}
	// Container creation precedes execution. Dispatch and output streaming can
	// overlap either, so only time outside these host intervals is overhead.
	complete := total > 0 && provision != nil && exec != nil && *provision >= 0 && *exec >= 0 &&
		*provision <= int64(total) && *exec <= int64(total)-*provision
	header := "Timings (breakdown unavailable):"
	if complete {
		header = "Timings (% of total):"
		phases[2].ns = new(int64(total) - *provision - *exec)
		// Allocate rounding tenths to the largest remainders so the displayed
		// percentages sum to exactly 100.0 without changing measured durations.
		var remainders [3]float64
		remaining := 1000
		for i := range phases {
			share := float64(*phases[i].ns) / float64(total) * 1000
			phases[i].tenths = int(share)
			remainders[i] = share - float64(phases[i].tenths)
			remaining -= phases[i].tenths
		}
		for range remaining {
			largest := 0
			for i := range remainders {
				if remainders[i] > remainders[largest] {
					largest = i
				}
			}
			phases[largest].tenths++
			remainders[largest] = -1
		}
	}
	_, _ = fmt.Fprintln(w, "\n"+header)
	for _, phase := range phases {
		duration := "unavailable"
		if phase.ns != nil && *phase.ns >= 0 {
			duration = time.Duration(*phase.ns).Round(time.Microsecond).String()
		}
		if complete {
			duration += fmt.Sprintf(" (%.1f%%)", float64(phase.tenths)/10)
		}
		_, _ = fmt.Fprintf(w, "  %-15s%s\n", phase.name, duration)
	}
	first := "none"
	if t.firstOutput != nil {
		first = t.firstOutput.Round(time.Microsecond).String()
	}
	_, _ = fmt.Fprintf(w, "  Total          %s\nLatencies:\n  Dispatch       %s\n  First output   %s\nExit code: %d\n",
		total.Round(time.Microsecond), t.dispatch.Round(time.Microsecond), first, *workload.ExitCode)
}

func retryWorkloadStream(err error) bool {
	var sdkErr *codersdk.Error
	if errors.As(err, &sdkErr) {
		status := sdkErr.StatusCode()
		return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
	}
	// Transport failures and interrupted response bodies can be transient.
	// The attempt limit also bounds retries for malformed responses.
	return true
}

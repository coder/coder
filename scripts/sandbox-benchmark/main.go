// sandbox-benchmark measures native workspace requests through the first
// successful command over Coder's authenticated agent connection.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type config struct {
	templateID                                            uuid.UUID
	runs, bursts, concurrency                             int
	timeout, cleanupTimeout, prepareTimeout, poll, target time.Duration
	prepareCommand                                        string
}

type sample struct {
	Group                    string                          `json:"group"`
	Index                    int                             `json:"index"`
	WorkspaceName            string                          `json:"workspace_name"`
	WorkspaceID              uuid.UUID                       `json:"workspace_id"`
	BuildID                  uuid.UUID                       `json:"build_id"`
	StartedAt                time.Time                       `json:"started_at"`
	CreateResponseMS         float64                         `json:"create_response_ms"`
	AttemptMS                float64                         `json:"attempt_ms"`
	ReadyMS                  *float64                        `json:"ready_ms,omitempty"`
	JobCompleteObservedMS    *float64                        `json:"job_complete_observed_ms,omitempty"`
	AgentConnectedObservedMS *float64                        `json:"agent_connected_observed_ms,omitempty"`
	ConnectionAttempts       int                             `json:"connection_attempts"`
	ConnectionAndCommandMS   *float64                        `json:"connection_and_command_ms,omitempty"`
	QueueMS                  *float64                        `json:"queue_ms,omitempty"`
	ProvisionerJobMS         *float64                        `json:"provisioner_job_ms,omitempty"`
	RuntimeMS                *float64                        `json:"runtime_ms,omitempty"`
	PrepareMS                *float64                        `json:"prepare_ms,omitempty"`
	FailureStage             string                          `json:"failure_stage,omitempty"`
	Error                    string                          `json:"error,omitempty"`
	PrepareError             string                          `json:"prepare_error,omitempty"`
	TimingsError             string                          `json:"timings_error,omitempty"`
	CleanupError             string                          `json:"cleanup_error,omitempty"`
	ServerTimings            *codersdk.WorkspaceBuildTimings `json:"server_timings,omitempty"`
}

type distribution struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
}

type summary struct {
	Group                    string       `json:"group"`
	Attempts                 int          `json:"attempts"`
	ReadinessFailures        int          `json:"readiness_failures"`
	PreparationFailures      int          `json:"preparation_failures"`
	TimingFailures           int          `json:"timing_failures"`
	CleanupFailures          int          `json:"cleanup_failures"`
	AllAttemptsMS            distribution `json:"all_attempts_ms"`
	SuccessfulReadinessMS    distribution `json:"successful_readiness_ms"`
	QueueMS                  distribution `json:"queue_ms"`
	ProvisionerJobMS         distribution `json:"provisioner_job_ms"`
	RuntimeMS                distribution `json:"runtime_ms"`
	JobCompleteObservedMS    distribution `json:"job_complete_observed_ms"`
	AgentConnectedObservedMS distribution `json:"agent_connected_observed_ms"`
	ConnectionAndCommandMS   distribution `json:"connection_and_command_ms"`
	PrepareMS                distribution `json:"prepare_ms"`
	ReadinessTargetPassed    bool         `json:"readiness_target_passed"`
	Passed                   bool         `json:"passed"`
}

type report struct {
	TemplateID         uuid.UUID `json:"template_id"`
	TargetMS           float64   `json:"target_ms"`
	PollMS             float64   `json:"poll_ms"`
	SequentialRuns     int       `json:"sequential_runs"`
	BurstRounds        int       `json:"burst_rounds"`
	BurstConcurrency   int       `json:"burst_concurrency"`
	TimeoutMS          float64   `json:"timeout_ms"`
	CleanupTimeoutMS   float64   `json:"cleanup_timeout_ms"`
	PreparationEnabled bool      `json:"preparation_enabled"`
	RequestedSamples   int       `json:"requested_samples"`
	NotStarted         int       `json:"not_started"`
	Passed             bool      `json:"passed"`
	Summaries          []summary `json:"summaries"`
	Samples            []sample  `json:"samples"`
	Measurement        string    `json:"measurement"`
}

func main() {
	os.Exit(runCLI())
}

func runCLI() int {
	var template, output string
	c := config{}
	flag.StringVar(&template, "template", "", "Native sandbox template UUID (required)")
	flag.StringVar(&output, "output", "", "Write the JSON report to a private file instead of stdout")
	flag.IntVar(&c.runs, "runs", 100, "Number of sequential creates")
	flag.IntVar(&c.bursts, "bursts", 25, "Number of concurrent-create rounds")
	flag.IntVar(&c.concurrency, "concurrency", 4, "Creates per burst")
	flag.DurationVar(&c.timeout, "timeout", time.Minute, "Readiness timeout per sandbox")
	flag.DurationVar(&c.cleanupTimeout, "cleanup-timeout", time.Minute, "Independent cleanup timeout per sandbox")
	flag.DurationVar(&c.prepareTimeout, "prepare-timeout", 5*time.Minute, "Optional preparation command timeout")
	flag.DurationVar(&c.poll, "poll", 20*time.Millisecond, "Build and agent status polling interval")
	flag.DurationVar(&c.target, "target", 5*time.Second, "Required p95 readiness latency, strictly below this duration")
	flag.StringVar(&c.prepareCommand, "prepare-command", "", "Optional remote shell command measured after readiness, for repository/workload preparation")
	flag.Parse()
	var err error
	c.templateID, err = uuid.Parse(template)
	if err != nil || c.templateID == uuid.Nil || c.runs < 0 || c.bursts < 0 || c.concurrency < 1 || c.runs+c.bursts == 0 || c.timeout <= 0 || c.cleanupTimeout <= 0 || c.prepareTimeout <= 0 || c.poll <= 0 || c.target <= 0 {
		_, _ = fmt.Fprintln(os.Stderr, "Provide -template and positive timeouts/concurrency; at least one run or burst is required.")
		return 2
	}
	u, err := url.Parse(os.Getenv("CODER_URL"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || os.Getenv("CODER_SESSION_TOKEN") == "" {
		_, _ = fmt.Fprintln(os.Stderr, "Set CODER_URL to an HTTP(S) URL and CODER_SESSION_TOKEN to an authorized Coder session token.")
		return 2
	}
	client := codersdk.New(u)
	client.SetSessionToken(os.Getenv("CODER_SESSION_TOKEN"))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	checkCtx, checkCancel := context.WithTimeout(ctx, c.timeout)
	tpl, err := client.Template(checkCtx, c.templateID)
	checkCancel()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Read template: %v\n", err)
		return 1
	}
	if tpl.Provisioner != codersdk.ProvisionerTypeSandbox {
		_, _ = fmt.Fprintln(os.Stderr, "The template must use the sandbox provisioner.")
		return 2
	}
	r := run(ctx, client, c)
	if err := writeReport(output, r); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Write report: %v\n", err)
		return 1
	}
	if !r.Passed {
		return 1
	}
	return 0
}

func writeReport(path string, r report) (err error) {
	var writer io.Writer = os.Stdout
	if path != "" {
		file, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if openErr != nil {
			return openErr
		}
		defer func() { err = errors.Join(err, file.Close()) }()
		if err := file.Chmod(0o600); err != nil {
			return err
		}
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

func run(ctx context.Context, client *codersdk.Client, c config) report {
	r := report{
		TemplateID: c.templateID, TargetMS: milliseconds(c.target), PollMS: milliseconds(c.poll),
		SequentialRuns: c.runs, BurstRounds: c.bursts, BurstConcurrency: c.concurrency,
		TimeoutMS: milliseconds(c.timeout), CleanupTimeoutMS: milliseconds(c.cleanupTimeout), PreparationEnabled: c.prepareCommand != "",
		RequestedSamples: c.runs + c.bursts*c.concurrency,
		Measurement:      "ready_ms measures the client request through authenticated SSH true. Observed timings are cumulative from that request; server queue/job/runtime durations may overlap. Successful readiness percentiles exclude failures, which always fail the target; all_attempts_ms includes failures. Preparation, timing retrieval and cleanup are excluded from readiness. No cache warming or eviction is performed; run cache-miss experiments separately.",
	}
	for i := 0; i < c.runs && ctx.Err() == nil; i++ {
		r.Samples = append(r.Samples, runSample(ctx, client, c, "sequential", i+1))
	}
	for round := 0; round < c.bursts && ctx.Err() == nil; round++ {
		batch := make([]sample, c.concurrency)
		gate := make(chan struct{})
		var ready, finished sync.WaitGroup
		ready.Add(c.concurrency)
		finished.Add(c.concurrency)
		for i := range batch {
			go func() {
				defer finished.Done()
				ready.Done()
				<-gate
				batch[i] = runSample(ctx, client, c, "burst", round*c.concurrency+i+1)
			}()
		}
		ready.Wait()
		close(gate)
		finished.Wait()
		r.Samples = append(r.Samples, batch...)
	}
	r.Passed = ctx.Err() == nil
	for _, group := range []string{"sequential", "burst"} {
		s := summarize(group, r.Samples, milliseconds(c.target))
		if s.Attempts == 0 {
			continue
		}
		r.Summaries = append(r.Summaries, s)
		r.Passed = r.Passed && s.Passed
	}
	r.NotStarted = r.RequestedSamples - len(r.Samples)
	return r
}

func runSample(parent context.Context, client *codersdk.Client, c config, group string, index int) (s sample) {
	s = sample{Group: group, Index: index, WorkspaceName: "sandbox-bench-" + uuid.NewString()[:12], StartedAt: time.Now().UTC()}
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()
	var createErr error
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), c.cleanupTimeout)
		defer cleanupCancel()
		if err := cleanup(cleanupCtx, client, &s, createErr, c.poll); err != nil {
			s.CleanupError = err.Error()
		}
	}()
	timingsCollected := false
	defer func() {
		if !timingsCollected && s.BuildID != uuid.Nil {
			collectTimings(client, &s, c.timeout)
		}
	}()
	fail := func(stage string, err error) {
		s.FailureStage, s.Error, s.AttemptMS = stage, err.Error(), milliseconds(time.Since(start))
	}
	ws, err := client.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
		TemplateID: c.templateID, Name: s.WorkspaceName,
	})
	s.CreateResponseMS = milliseconds(time.Since(start))
	if err != nil {
		createErr = err
		fail("create", err)
		return s
	}
	s.WorkspaceID, s.BuildID = ws.ID, ws.LatestBuild.ID
	var agentID uuid.UUID
	for {
		build := ws.LatestBuild
		if build.Job.StartedAt != nil {
			s.QueueMS = durationMS(build.Job.CreatedAt, *build.Job.StartedAt)
		}
		if build.Job.CompletedAt != nil && build.Job.StartedAt != nil {
			s.ProvisionerJobMS = durationMS(*build.Job.StartedAt, *build.Job.CompletedAt)
		}
		if build.Job.Status == codersdk.ProvisionerJobFailed || build.Job.Status == codersdk.ProvisionerJobCanceled {
			fail("provision", xerrors.Errorf("build %s: %s", build.Job.Status, build.Job.Error))
			return s
		}
		if build.Job.Status == codersdk.ProvisionerJobSucceeded {
			if s.JobCompleteObservedMS == nil {
				s.JobCompleteObservedMS = elapsedMS(start)
			}
			for _, resource := range build.Resources {
				for _, agent := range resource.Agents {
					if agent.Status == codersdk.WorkspaceAgentConnected {
						agentID = agent.ID
					}
				}
			}
			if agentID != uuid.Nil {
				break
			}
		}
		if err := pause(ctx, c.poll); err != nil {
			stage := "provision"
			if build.Job.Status == codersdk.ProvisionerJobSucceeded {
				stage = "agent_ready"
			}
			fail(stage, err)
			return s
		}
		ws, err = client.Workspace(ctx, s.WorkspaceID)
		if err != nil {
			fail("poll", err)
			return s
		}
	}
	s.AgentConnectedObservedMS = elapsedMS(start)
	commandStart := time.Now()
	var commandCompleted time.Time
	for {
		s.ConnectionAttempts++
		commandCompleted, err = command(ctx, client, agentID, "true")
		if err == nil {
			break
		}
		if waitErr := pause(ctx, c.poll); waitErr != nil {
			fail("first_command", errors.Join(err, waitErr))
			return s
		}
	}
	s.ReadyMS, s.ConnectionAndCommandMS = durationMS(start, commandCompleted), durationMS(commandStart, commandCompleted)
	s.AttemptMS = *s.ReadyMS
	cancel()

	collectTimings(client, &s, c.timeout)
	timingsCollected = true
	if c.prepareCommand != "" {
		prepareCtx, prepareCancel := context.WithTimeout(parent, c.prepareTimeout)
		prepareStart := time.Now()
		completed, err := command(prepareCtx, client, agentID, c.prepareCommand)
		s.PrepareMS = elapsedMS(prepareStart)
		if err == nil {
			s.PrepareMS = durationMS(prepareStart, completed)
		}
		prepareCancel()
		if err != nil {
			s.PrepareError = err.Error()
		}
	}
	return s
}

func collectTimings(client *codersdk.Client, s *sample, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), min(timeout, 10*time.Second))
	defer cancel()
	timings, err := client.WorkspaceBuildTimings(ctx, s.BuildID)
	if err != nil {
		s.TimingsError = err.Error()
		return
	}
	s.ServerTimings = &timings
	var runtimeMS float64
	for _, timing := range timings.ProvisionerTimings {
		if timing.Stage == codersdk.TimingStageApply && timing.Source == "containerd" && timing.Action == "create" {
			runtimeMS += milliseconds(timing.EndedAt.Sub(timing.StartedAt))
			s.RuntimeMS = &runtimeMS
		}
	}
	if s.RuntimeMS == nil && s.ReadyMS != nil {
		s.TimingsError = "containerd create timing was not returned"
	}
}

func command(ctx context.Context, client *codersdk.Client, agentID uuid.UUID, shellCommand string) (time.Time, error) {
	conn, err := workspacesdk.New(client).DialAgent(ctx, agentID, nil)
	if err != nil {
		return time.Time{}, xerrors.Errorf("dial agent: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	sshClient, err := conn.SSHClient(ctx)
	if err != nil {
		return time.Time{}, xerrors.Errorf("authenticate SSH: %w", err)
	}
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	if err != nil {
		return time.Time{}, xerrors.Errorf("open SSH session: %w", err)
	}
	defer session.Close()
	session.Stdout, session.Stderr = io.Discard, io.Discard
	if err := session.Run(shellCommand); err != nil {
		return time.Time{}, xerrors.Errorf("run remote command: %w", err)
	}
	return time.Now(), nil
}

func cleanup(ctx context.Context, client *codersdk.Client, s *sample, createErr error, poll time.Duration) error {
	if s.WorkspaceID == uuid.Nil {
		var apiErr *codersdk.Error
		if errors.As(createErr, &apiErr) && apiErr.StatusCode() >= 400 && apiErr.StatusCode() < 500 {
			return nil
		}
		// A timed-out create can have committed. Resolve the unique name and
		// retain it in the report if the creation outcome remains uncertain.
		for {
			ws, err := client.WorkspaceByOwnerAndName(ctx, codersdk.Me, s.WorkspaceName, codersdk.WorkspaceOptions{})
			if err == nil {
				s.WorkspaceID, s.BuildID = ws.ID, ws.LatestBuild.ID
				break
			}
			if err := pause(ctx, poll); err != nil {
				return xerrors.Errorf("creation outcome uncertain; locate workspace %s: %w", s.WorkspaceName, err)
			}
		}
	}
	ws, err := client.Workspace(ctx, s.WorkspaceID)
	if err != nil {
		var apiErr *codersdk.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode() == http.StatusNotFound {
			return nil
		}
		return xerrors.Errorf("read workspace for cleanup: %w", err)
	}
	build := ws.LatestBuild
	if !jobDone(build.Job) {
		cancelErr := client.CancelWorkspaceBuild(ctx, build.ID, codersdk.CancelWorkspaceBuildParams{})
		if cancelErr != nil {
			latest, err := client.WorkspaceBuild(ctx, build.ID)
			if err != nil || !jobDone(latest.Job) {
				return xerrors.Errorf("cancel unfinished build: %w", cancelErr)
			}
		}
		if _, err := waitBuild(ctx, client, build.ID, poll); err != nil {
			return err
		}
	}
	deletion, err := client.CreateWorkspaceBuild(ctx, ws.ID, codersdk.CreateWorkspaceBuildRequest{Transition: codersdk.WorkspaceTransitionDelete})
	if err != nil {
		return xerrors.Errorf("request sandbox deletion: %w", err)
	}
	deletion, err = waitBuild(ctx, client, deletion.ID, poll)
	if err != nil {
		return err
	}
	if deletion.Job.Status != codersdk.ProvisionerJobSucceeded {
		return xerrors.Errorf("sandbox deletion %s: %s", deletion.Job.Status, deletion.Job.Error)
	}
	return nil
}

func waitBuild(ctx context.Context, client *codersdk.Client, id uuid.UUID, poll time.Duration) (codersdk.WorkspaceBuild, error) {
	for {
		build, err := client.WorkspaceBuild(ctx, id)
		if err != nil {
			return build, xerrors.Errorf("wait for cleanup build: %w", err)
		}
		if jobDone(build.Job) {
			return build, nil
		}
		if err := pause(ctx, poll); err != nil {
			return build, xerrors.Errorf("wait for cleanup build: %w", err)
		}
	}
}

func jobDone(job codersdk.ProvisionerJob) bool {
	return job.Status == codersdk.ProvisionerJobSucceeded || job.Status == codersdk.ProvisionerJobFailed || job.Status == codersdk.ProvisionerJobCanceled
}

func pause(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func milliseconds(d time.Duration) float64     { return float64(d) / float64(time.Millisecond) }
func elapsedMS(start time.Time) *float64       { value := milliseconds(time.Since(start)); return &value }
func durationMS(start, end time.Time) *float64 { value := milliseconds(end.Sub(start)); return &value }

// Percentiles use the nearest-rank definition so the five-second boundary is
// evaluated against an observed sample, without interpolation.
func stats(values []float64) distribution {
	if len(values) == 0 {
		return distribution{}
	}
	values = slices.Clone(values)
	slices.Sort(values)
	percentile := func(p float64) float64 { return values[int(math.Ceil(p*float64(len(values))))-1] }
	return distribution{Count: len(values), P50: percentile(.5), P95: percentile(.95), P99: percentile(.99)}
}

func summarize(group string, samples []sample, targetMS float64) summary {
	s := summary{Group: group}
	var all, successful, queue, job, runtime, jobObserved, agentObserved, connection, preparation []float64
	appendKnown := func(values *[]float64, value *float64) {
		if value != nil {
			*values = append(*values, *value)
		}
	}
	for _, sample := range samples {
		if sample.Group != group {
			continue
		}
		s.Attempts++
		appendKnown(&queue, sample.QueueMS)
		appendKnown(&job, sample.ProvisionerJobMS)
		appendKnown(&runtime, sample.RuntimeMS)
		appendKnown(&jobObserved, sample.JobCompleteObservedMS)
		appendKnown(&agentObserved, sample.AgentConnectedObservedMS)
		appendKnown(&connection, sample.ConnectionAndCommandMS)
		appendKnown(&preparation, sample.PrepareMS)
		all = append(all, sample.AttemptMS)
		if sample.Error != "" || sample.ReadyMS == nil {
			s.ReadinessFailures++
		} else {
			successful = append(successful, *sample.ReadyMS)
		}
		if sample.PrepareError != "" {
			s.PreparationFailures++
		}
		if sample.TimingsError != "" {
			s.TimingFailures++
		}
		if sample.CleanupError != "" {
			s.CleanupFailures++
		}
	}
	s.QueueMS, s.ProvisionerJobMS, s.RuntimeMS = stats(queue), stats(job), stats(runtime)
	s.JobCompleteObservedMS, s.AgentConnectedObservedMS = stats(jobObserved), stats(agentObserved)
	s.ConnectionAndCommandMS, s.PrepareMS = stats(connection), stats(preparation)
	s.AllAttemptsMS, s.SuccessfulReadinessMS = stats(all), stats(successful)
	s.ReadinessTargetPassed = s.Attempts > 0 && s.ReadinessFailures == 0 && s.SuccessfulReadinessMS.P95 < targetMS
	s.Passed = s.ReadinessTargetPassed && s.PreparationFailures == 0 && s.TimingFailures == 0 && s.CleanupFailures == 0
	return s
}

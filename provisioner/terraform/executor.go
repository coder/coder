package terraform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-version"
	tfjson "github.com/hashicorp/terraform-json"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

var version170 = version.Must(version.NewVersion("1.7.0"))

type executor struct {
	logger     slog.Logger
	server     *server
	mut        *sync.Mutex
	binaryPath string
	// cachePath and workdir must not be used by multiple processes at once.
	cachePath     string
	cliConfigPath string
	workdir       string
	// used to capture execution times at various stages
	timings *timingAggregator
}

func (e *executor) basicEnv() []string {
	// Required for "terraform init" to find "git" to
	// clone Terraform modules.
	env := safeEnviron()
	// Only Linux reliably works with the Terraform plugin
	// cache directory. It's unknown why this is.
	if e.cachePath != "" && runtime.GOOS == "linux" {
		env = append(env, "TF_PLUGIN_CACHE_DIR="+e.cachePath)
	}
	if e.cliConfigPath != "" {
		env = append(env, "TF_CLI_CONFIG_FILE="+e.cliConfigPath)
	}
	return env
}

// execWriteOutput must only be called while the lock is held.
func (e *executor) execWriteOutput(ctx, killCtx context.Context, args, env []string, stdOutWriter, stdErrWriter io.WriteCloser) (err error) {
	ctx, span := e.server.startTrace(ctx, fmt.Sprintf("exec - terraform %s", args[0]))
	defer span.End()
	span.SetAttributes(attribute.StringSlice("args", args))
	e.logger.Debug(ctx, "starting command", slog.F("args", args))

	defer func() {
		e.logger.Debug(ctx, "closing writers", slog.Error(err))
		closeErr := stdOutWriter.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		closeErr = stdErrWriter.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if ctx.Err() != nil {
		e.logger.Debug(ctx, "context canceled before command started", slog.F("args", args))
		return ctx.Err()
	}

	if isCanarySet(env) {
		return xerrors.New("environment variables not sanitized, this is a bug within Coder")
	}

	// #nosec
	cmd := exec.CommandContext(killCtx, e.binaryPath, args...)
	cmd.Dir = e.workdir
	if env == nil {
		// We don't want to passthrough host env when unset.
		env = []string{}
	}
	cmd.Env = env

	// We want logs to be written in the correct order, so we wrap all logging
	// in a sync.Mutex.
	mut := &sync.Mutex{}
	cmd.Stdout = syncWriter{mut, stdOutWriter}
	cmd.Stderr = syncWriter{mut, stdErrWriter}

	e.server.logger.Debug(ctx, "executing terraform command",
		slog.F("binary_path", e.binaryPath),
		slog.F("args", args),
	)
	err = cmd.Start()
	if err != nil {
		e.logger.Debug(ctx, "failed to start command", slog.F("args", args))
		return err
	}
	interruptCommandOnCancel(ctx, killCtx, e.logger, cmd)

	err = cmd.Wait()
	e.logger.Debug(ctx, "command done", slog.F("args", args), slog.Error(err))
	return err
}

// execParseJSON must only be called while the lock is held.
func (e *executor) execParseJSON(ctx, killCtx context.Context, args, env []string, v interface{}) error {
	ctx, span := e.server.startTrace(ctx, fmt.Sprintf("exec - terraform %s", args[0]))
	defer span.End()
	span.SetAttributes(attribute.StringSlice("args", args))

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// #nosec
	cmd := exec.CommandContext(killCtx, e.binaryPath, args...)
	cmd.Dir = e.workdir
	cmd.Env = env
	out := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = stdErr

	e.server.logger.Debug(ctx, "executing terraform command with JSON result",
		slog.F("binary_path", e.binaryPath),
		slog.F("args", args),
	)
	err := cmd.Start()
	if err != nil {
		return err
	}
	interruptCommandOnCancel(ctx, killCtx, e.logger, cmd)

	err = cmd.Wait()
	if err != nil {
		errString, _ := io.ReadAll(stdErr)
		return xerrors.Errorf("%s: %w", errString, err)
	}

	dec := json.NewDecoder(out)
	dec.UseNumber()
	err = dec.Decode(v)
	if err != nil {
		return xerrors.Errorf("decode terraform json: %w", err)
	}
	return nil
}

func (e *executor) checkMinVersion(ctx context.Context) error {
	v, err := e.version(ctx)
	if err != nil {
		return err
	}
	if !v.GreaterThanOrEqual(minTerraformVersion) {
		return xerrors.Errorf(
			"terraform version %q is too old. required >= %q",
			v.String(),
			minTerraformVersion.String())
	}
	return nil
}

// version doesn't need the lock because it doesn't read or write to any state.
func (e *executor) version(ctx context.Context) (*version.Version, error) {
	return versionFromBinaryPath(ctx, e.binaryPath)
}

func versionFromBinaryPath(ctx context.Context, binaryPath string) (*version.Version, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// #nosec
	cmd := exec.CommandContext(ctx, binaryPath, "version", "-json")
	out, err := cmd.Output()
	if err != nil {
		select {
		// `exec` library throws a `signal: killed`` error instead of the canceled context.
		// Since we know the cause for the killed signal, we are throwing the relevant error here.
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, err
		}
	}
	vj := tfjson.VersionOutput{}
	err = json.Unmarshal(out, &vj)
	if err != nil {
		return nil, err
	}
	return version.NewVersion(vj.Version)
}

type textFileBusyError struct {
	exitErr *exec.ExitError
	stderr  string
}

func (e *textFileBusyError) Error() string {
	return "text file busy: " + e.exitErr.String()
}

func (e *executor) init(ctx, killCtx context.Context, logr logSink) error {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	e.mut.Lock()
	defer e.mut.Unlock()

	outWriter, doneOut := logWriter(logr, proto.LogLevel_DEBUG)
	errWriter, doneErr := logWriter(logr, proto.LogLevel_ERROR)
	defer func() {
		_ = outWriter.Close()
		_ = errWriter.Close()
		<-doneOut
		<-doneErr
	}()

	// As a special case, we want to look for the error "text file busy" in the stderr output of
	// the init command, so we also take a copy of the stderr into an in memory buffer.
	errBuf := newBufferedWriteCloser(errWriter)

	args := []string{
		"init",
		"-no-color",
		"-input=false",
	}

	err := e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), outWriter, errBuf)
	var exitErr *exec.ExitError
	if xerrors.As(err, &exitErr) {
		if bytes.Contains(errBuf.b.Bytes(), []byte("text file busy")) {
			return &textFileBusyError{exitErr: exitErr, stderr: errBuf.b.String()}
		}
	}
	return err
}

func getPlanFilePath(workdir string) string {
	return filepath.Join(workdir, "terraform.tfplan")
}

func getStateFilePath(workdir string) string {
	return filepath.Join(workdir, "terraform.tfstate")
}

// revive:disable-next-line:flag-parameter
func (e *executor) plan(ctx, killCtx context.Context, env, vars []string, logr logSink, destroy bool) (*proto.PlanComplete, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	e.mut.Lock()
	defer e.mut.Unlock()

	planfilePath := getPlanFilePath(e.workdir)
	args := []string{
		"plan",
		"-no-color",
		"-input=false",
		"-json",
		"-refresh=true",
		"-out=" + planfilePath,
	}
	if destroy {
		args = append(args, "-destroy")
	}
	for _, variable := range vars {
		args = append(args, "-var", variable)
	}

	outWriter, doneOut := e.provisionLogWriter(logr)
	errWriter, doneErr := logWriter(logr, proto.LogLevel_ERROR)
	defer func() {
		_ = outWriter.Close()
		_ = errWriter.Close()
		<-doneOut
		<-doneErr
	}()

	err := e.execWriteOutput(ctx, killCtx, args, env, outWriter, errWriter)
	if err != nil {
		return nil, xerrors.Errorf("terraform plan: %w", err)
	}

	// Capture the duration of the call to `terraform graph`.
	graphTimings := newTimingAggregator(database.ProvisionerJobTimingStageGraph)
	graphTimings.ingest(createGraphTimingsEvent(timingGraphStart))

	state, plan, err := e.planResources(ctx, killCtx, planfilePath)
	if err != nil {
		graphTimings.ingest(createGraphTimingsEvent(timingGraphErrored))
		return nil, err
	}

	graphTimings.ingest(createGraphTimingsEvent(timingGraphComplete))

	moduleFiles, err := getModulesArchive(e.workdir)
	if err != nil {
		// TODO: we probably want to persist this error or make it louder eventually
		e.logger.Warn(ctx, "failed to archive terraform modules", slog.Error(err))
	}

	return &proto.PlanComplete{
		Parameters:            state.Parameters,
		Resources:             state.Resources,
		ExternalAuthProviders: state.ExternalAuthProviders,
		Timings:               append(e.timings.aggregate(), graphTimings.aggregate()...),
		Presets:               state.Presets,
		Plan:                  plan,
		ModuleFiles:           moduleFiles,
	}, nil
}

func onlyDataResources(sm tfjson.StateModule) tfjson.StateModule {
	filtered := sm
	filtered.Resources = []*tfjson.StateResource{}
	for _, r := range sm.Resources {
		if r.Mode == "data" {
			filtered.Resources = append(filtered.Resources, r)
		}
	}

	filtered.ChildModules = []*tfjson.StateModule{}
	for _, c := range sm.ChildModules {
		filteredChild := onlyDataResources(*c)
		filtered.ChildModules = append(filtered.ChildModules, &filteredChild)
	}
	return filtered
}

// planResources must only be called while the lock is held.
func (e *executor) planResources(ctx, killCtx context.Context, planfilePath string) (*State, json.RawMessage, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	plan, err := e.showPlan(ctx, killCtx, planfilePath)
	if err != nil {
		return nil, nil, xerrors.Errorf("show terraform plan file: %w", err)
	}

	rawGraph, err := e.graph(ctx, killCtx)
	if err != nil {
		return nil, nil, xerrors.Errorf("graph: %w", err)
	}
	modules := []*tfjson.StateModule{}
	if plan.PriorState != nil {
		// We need the data resources for rich parameters. For some reason, they
		// only show up in the PriorState.
		//
		// We don't want all prior resources, because Quotas (and
		// future features) would never know which resources are getting
		// deleted by a stop.

		filtered := onlyDataResources(*plan.PriorState.Values.RootModule)
		modules = append(modules, &filtered)
	}
	modules = append(modules, plan.PlannedValues.RootModule)

	state, err := ConvertState(ctx, modules, rawGraph, e.server.logger)
	if err != nil {
		return nil, nil, err
	}

	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, nil, err
	}

	return state, planJSON, nil
}

// showPlan must only be called while the lock is held.
func (e *executor) showPlan(ctx, killCtx context.Context, planfilePath string) (*tfjson.Plan, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	args := []string{"show", "-json", "-no-color", planfilePath}
	p := new(tfjson.Plan)
	err := e.execParseJSON(ctx, killCtx, args, e.basicEnv(), p)
	return p, err
}

// graph must only be called while the lock is held.
func (e *executor) graph(ctx, killCtx context.Context) (string, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	ver, err := e.version(ctx)
	if err != nil {
		return "", err
	}
	args := []string{"graph"}
	if ver.GreaterThanOrEqual(version170) {
		args = append(args, "-type=plan")
	}
	var out strings.Builder
	cmd := exec.CommandContext(killCtx, e.binaryPath, args...) // #nosec
	cmd.Stdout = &out
	cmd.Dir = e.workdir
	cmd.Env = e.basicEnv()

	e.server.logger.Debug(ctx, "executing terraform command graph",
		slog.F("binary_path", e.binaryPath),
		slog.F("args", "graph"),
	)
	err = cmd.Start()
	if err != nil {
		return "", err
	}
	interruptCommandOnCancel(ctx, killCtx, e.logger, cmd)

	err = cmd.Wait()
	if err != nil {
		return "", xerrors.Errorf("graph: %w", err)
	}
	return out.String(), nil
}

func (e *executor) apply(
	ctx, killCtx context.Context,
	env []string,
	logr logSink,
) (*proto.ApplyComplete, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	e.mut.Lock()
	defer e.mut.Unlock()

	args := []string{
		"apply",
		"-no-color",
		"-auto-approve",
		"-input=false",
		"-json",
		getPlanFilePath(e.workdir),
	}

	outWriter, doneOut := e.provisionLogWriter(logr)
	errWriter, doneErr := logWriter(logr, proto.LogLevel_ERROR)
	defer func() {
		_ = outWriter.Close()
		_ = errWriter.Close()
		<-doneOut
		<-doneErr
	}()

	err := e.execWriteOutput(ctx, killCtx, args, env, outWriter, errWriter)
	if err != nil {
		return nil, xerrors.Errorf("terraform apply: %w", err)
	}
	state, err := e.stateResources(ctx, killCtx)
	if err != nil {
		return nil, err
	}
	statefilePath := filepath.Join(e.workdir, "terraform.tfstate")
	stateContent, err := os.ReadFile(statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("read statefile %q: %w", statefilePath, err)
	}

	return &proto.ApplyComplete{
		Parameters:            state.Parameters,
		Resources:             state.Resources,
		ExternalAuthProviders: state.ExternalAuthProviders,
		State:                 stateContent,
		Timings:               e.timings.aggregate(),
	}, nil
}

// stateResources must only be called while the lock is held.
func (e *executor) stateResources(ctx, killCtx context.Context) (*State, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	state, err := e.state(ctx, killCtx)
	if err != nil {
		return nil, err
	}
	rawGraph, err := e.graph(ctx, killCtx)
	if err != nil {
		return nil, xerrors.Errorf("get terraform graph: %w", err)
	}
	converted := &State{}
	if state.Values == nil {
		return converted, nil
	}

	converted, err = ConvertState(ctx, []*tfjson.StateModule{
		state.Values.RootModule,
	}, rawGraph, e.server.logger)
	if err != nil {
		return nil, err
	}
	return converted, nil
}

// state must only be called while the lock is held.
func (e *executor) state(ctx, killCtx context.Context) (*tfjson.State, error) {
	ctx, span := e.server.startTrace(ctx, tracing.FuncName())
	defer span.End()

	args := []string{"show", "-json", "-no-color"}
	state := &tfjson.State{}
	err := e.execParseJSON(ctx, killCtx, args, e.basicEnv(), state)
	if err != nil {
		return nil, xerrors.Errorf("terraform show state: %w", err)
	}
	return state, nil
}

func interruptCommandOnCancel(ctx, killCtx context.Context, logger slog.Logger, cmd *exec.Cmd) {
	go func() {
		select {
		case <-ctx.Done():
			var err error
			switch runtime.GOOS {
			case "windows":
				// Interrupts aren't supported by Windows.
				err = cmd.Process.Kill()
			default:
				err = cmd.Process.Signal(os.Interrupt)
			}
			logger.Debug(ctx, "interrupted command", slog.F("args", cmd.Args), slog.Error(err))

		case <-killCtx.Done():
			logger.Debug(ctx, "kill context ended", slog.F("args", cmd.Args))
		}
	}()
}

type logSink interface {
	ProvisionLog(l proto.LogLevel, o string)
}

// logWriter creates a WriteCloser that will log each line of text at the given level.  The WriteCloser must be closed
// by the caller to end logging, after which the returned channel will be closed to indicate that logging of the written
// data has finished.  Failure to close the WriteCloser will leak a goroutine.
func logWriter(sink logSink, level proto.LogLevel) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)
	go readAndLog(sink, r, done, level)
	return w, done
}

func readAndLog(sink logSink, r io.Reader, done chan<- any, level proto.LogLevel) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var log terraformProvisionLog
		err := json.Unmarshal(scanner.Bytes(), &log)
		if err != nil {
			if strings.TrimSpace(scanner.Text()) == "" {
				continue
			}

			sink.ProvisionLog(level, scanner.Text())
			continue
		}

		logLevel := convertTerraformLogLevel(log.Level, sink)
		if logLevel == proto.LogLevel_TRACE {
			// Skip TRACE log entries as they produce a lot of noise.
			//
			// FIXME consider config.ProvisionerLogLevel to enable custom level logging
			// instead of "just-debug-level" mode.
			continue
		}

		// Degrade JSON log entries marked as INFO as these are logs produced in debug mode.
		if logLevel == proto.LogLevel_INFO {
			logLevel = proto.LogLevel_DEBUG
		}
		sink.ProvisionLog(logLevel, log.Message)
	}
}

// provisionLogWriter creates a WriteCloser that will log each JSON formatted terraform log.  The WriteCloser must be
// closed by the caller to end logging, after which the returned channel will be closed to indicate that logging of the
// written data has finished.  Failure to close the WriteCloser will leak a goroutine.
func (e *executor) provisionLogWriter(sink logSink) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)

	go e.provisionReadAndLog(sink, r, done)
	return w, done
}

func (e *executor) provisionReadAndLog(sink logSink, r io.Reader, done chan<- any) {
	defer close(done)

	errCount := 0

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log := parseTerraformLogLine(scanner.Bytes())
		if log == nil {
			continue
		}

		logLevel := convertTerraformLogLevel(log.Level, sink)
		sink.ProvisionLog(logLevel, log.Message)

		ts, span, err := extractTimingSpan(log)
		if err != nil {
			// It's too noisy to log all of these as timings are not an essential feature, but we do need to log *some*.
			if errCount%10 == 0 {
				e.logger.Warn(context.Background(), "(sampled) failed to extract timings entry from log line",
					slog.F("line", log.Message), slog.Error(err))
			}
			errCount++
		} else {
			// Only ingest valid timings.
			e.timings.ingest(ts, span)
		}

		// If the diagnostic is provided, let's provide a bit more info!
		if log.Diagnostic == nil {
			continue
		}
		logLevel = convertTerraformLogLevel(string(log.Diagnostic.Severity), sink)
		for _, diagLine := range strings.Split(FormatDiagnostic(log.Diagnostic), "\n") {
			sink.ProvisionLog(logLevel, diagLine)
		}
	}
}

func parseTerraformLogLine(line []byte) *terraformProvisionLog {
	var log terraformProvisionLog
	err := json.Unmarshal(line, &log)
	if err != nil {
		// Sometimes terraform doesn't log JSON, even though we asked it to.
		// The terraform maintainers have said on the issue tracker that
		// they don't guarantee that non-JSON lines won't get printed.
		// https://github.com/hashicorp/terraform/issues/29252#issuecomment-887710001
		//
		// > I think as a practical matter it isn't possible for us to
		// > promise that the output will always be entirely JSON, because
		// > there's plenty of code that runs before command line arguments
		// > are parsed and thus before we even know we're in JSON mode.
		// > Given that, I'd suggest writing code that consumes streaming
		// > JSON output from Terraform in such a way that it can tolerate
		// > the output not having JSON in it at all.
		//
		// Log lines such as:
		// - Acquiring state lock. This may take a few moments...
		// - Releasing state lock. This may take a few moments...
		if len(bytes.TrimSpace(line)) == 0 {
			return nil
		}
		log.Level = "info"
		log.Message = string(line)
	}
	return &log
}

func extractTimingSpan(log *terraformProvisionLog) (time.Time, *timingSpan, error) {
	// Input is not well-formed, bail out.
	if log.Type == "" {
		return time.Time{}, nil, xerrors.Errorf("invalid timing kind: %q", log.Type)
	}

	typ := timingKind(log.Type)
	if !typ.Valid() {
		return time.Time{}, nil, xerrors.Errorf("unexpected timing kind: %q", log.Type)
	}

	ts, err := time.Parse("2006-01-02T15:04:05.000000Z07:00", log.Timestamp)
	if err != nil {
		// TODO: log
		ts = time.Now()
	}

	return ts, &timingSpan{
		kind:     typ,
		action:   log.Hook.Action,
		provider: log.Hook.Resource.Provider,
		resource: log.Hook.Resource.Addr,
	}, nil
}

func convertTerraformLogLevel(logLevel string, sink logSink) proto.LogLevel {
	switch strings.ToLower(logLevel) {
	case "trace":
		return proto.LogLevel_TRACE
	case "debug":
		return proto.LogLevel_DEBUG
	case "info":
		return proto.LogLevel_INFO
	case "warn", "warning":
		return proto.LogLevel_WARN
	case "error":
		return proto.LogLevel_ERROR
	default:
		sink.ProvisionLog(proto.LogLevel_WARN, fmt.Sprintf("unable to convert log level %s", logLevel))
		return proto.LogLevel_INFO
	}
}

type terraformProvisionLog struct {
	Level     string                    `json:"@level"`
	Message   string                    `json:"@message"`
	Timestamp string                    `json:"@timestamp"`
	Type      string                    `json:"type"`
	Hook      terraformProvisionLogHook `json:"hook"`

	Diagnostic *tfjson.Diagnostic `json:"diagnostic,omitempty"`
}

type terraformProvisionLogHook struct {
	Action   string                            `json:"action"`
	Resource terraformProvisionLogHookResource `json:"resource"`
}

type terraformProvisionLogHookResource struct {
	Addr     string `json:"addr"`
	Provider string `json:"implied_provider"`
}

// syncWriter wraps an io.Writer in a sync.Mutex.
type syncWriter struct {
	mut *sync.Mutex
	w   io.Writer
}

// Write implements io.Writer.
func (sw syncWriter) Write(p []byte) (n int, err error) {
	sw.mut.Lock()
	defer sw.mut.Unlock()
	return sw.w.Write(p)
}

type bufferedWriteCloser struct {
	wc io.WriteCloser
	b  bytes.Buffer
}

func newBufferedWriteCloser(wc io.WriteCloser) *bufferedWriteCloser {
	return &bufferedWriteCloser{
		wc: wc,
	}
}

func (b *bufferedWriteCloser) Write(p []byte) (int, error) {
	n, err := b.b.Write(p)
	if err != nil {
		return n, err
	}
	return b.wc.Write(p)
}

func (b *bufferedWriteCloser) Close() error {
	return b.wc.Close()
}

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

	"github.com/hashicorp/go-version"
	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/provisionersdk/proto"
)

type executor struct {
	mut        *sync.Mutex
	binaryPath string
	// cachePath and workdir must not be used by multiple processes at once.
	cachePath string
	workdir   string
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
	return env
}

// execWriteOutput must only be called while the lock is held.
func (e *executor) execWriteOutput(ctx, killCtx context.Context, args, env []string, stdOutWriter, stdErrWriter io.WriteCloser) (err error) {
	defer func() {
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

	err = cmd.Start()
	if err != nil {
		return err
	}
	interruptCommandOnCancel(ctx, killCtx, cmd)

	return cmd.Wait()
}

// execParseJSON must only be called while the lock is held.
func (e *executor) execParseJSON(ctx, killCtx context.Context, args, env []string, v interface{}) error {
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

	err := cmd.Start()
	if err != nil {
		return err
	}
	interruptCommandOnCancel(ctx, killCtx, cmd)

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

func (e *executor) init(ctx, killCtx context.Context, logr logSink) error {
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

	args := []string{
		"init",
		"-no-color",
		"-input=false",
	}

	return e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), outWriter, errWriter)
}

// revive:disable-next-line:flag-parameter
func (e *executor) plan(ctx, killCtx context.Context, env, vars []string, logr logSink, destroy bool) (*proto.Provision_Response, error) {
	e.mut.Lock()
	defer e.mut.Unlock()

	planfilePath := filepath.Join(e.workdir, "terraform.tfplan")
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

	outWriter, doneOut := provisionLogWriter(logr)
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
	resources, parameters, err := e.planResources(ctx, killCtx, planfilePath)
	if err != nil {
		return nil, err
	}
	planFileByt, err := os.ReadFile(planfilePath)
	if err != nil {
		return nil, err
	}
	return &proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				Parameters: parameters,
				Resources:  resources,
				Plan:       planFileByt,
			},
		},
	}, nil
}

// planResources must only be called while the lock is held.
func (e *executor) planResources(ctx, killCtx context.Context, planfilePath string) ([]*proto.Resource, []*proto.RichParameter, error) {
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
		modules = append(modules, plan.PriorState.Values.RootModule)
	}
	modules = append(modules, plan.PlannedValues.RootModule)
	return ConvertResourcesAndParameters(modules, rawGraph)
}

// showPlan must only be called while the lock is held.
func (e *executor) showPlan(ctx, killCtx context.Context, planfilePath string) (*tfjson.Plan, error) {
	args := []string{"show", "-json", "-no-color", planfilePath}
	p := new(tfjson.Plan)
	err := e.execParseJSON(ctx, killCtx, args, e.basicEnv(), p)
	return p, err
}

// graph must only be called while the lock is held.
func (e *executor) graph(ctx, killCtx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	var out strings.Builder
	cmd := exec.CommandContext(killCtx, e.binaryPath, "graph") // #nosec
	cmd.Stdout = &out
	cmd.Dir = e.workdir
	cmd.Env = e.basicEnv()

	err := cmd.Start()
	if err != nil {
		return "", err
	}
	interruptCommandOnCancel(ctx, killCtx, cmd)

	err = cmd.Wait()
	if err != nil {
		return "", xerrors.Errorf("graph: %w", err)
	}
	return out.String(), nil
}

func (e *executor) apply(
	ctx, killCtx context.Context, plan []byte, env []string, logr logSink,
) (*proto.Provision_Response, error) {
	e.mut.Lock()
	defer e.mut.Unlock()

	planFile, err := os.CreateTemp("", "coder-terrafrom-plan")
	if err != nil {
		return nil, xerrors.Errorf("create plan file: %w", err)
	}
	_, err = planFile.Write(plan)
	if err != nil {
		return nil, xerrors.Errorf("write plan file: %w", err)
	}
	defer os.Remove(planFile.Name())

	args := []string{
		"apply",
		"-no-color",
		"-auto-approve",
		"-input=false",
		"-json",
		planFile.Name(),
	}

	outWriter, doneOut := provisionLogWriter(logr)
	errWriter, doneErr := logWriter(logr, proto.LogLevel_ERROR)
	defer func() {
		_ = outWriter.Close()
		_ = errWriter.Close()
		<-doneOut
		<-doneErr
	}()

	err = e.execWriteOutput(ctx, killCtx, args, env, outWriter, errWriter)
	if err != nil {
		return nil, xerrors.Errorf("terraform apply: %w", err)
	}
	resources, parameters, err := e.stateResources(ctx, killCtx)
	if err != nil {
		return nil, err
	}
	statefilePath := filepath.Join(e.workdir, "terraform.tfstate")
	stateContent, err := os.ReadFile(statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("read statefile %q: %w", statefilePath, err)
	}
	return &proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				Parameters: parameters,
				Resources:  resources,
				State:      stateContent,
			},
		},
	}, nil
}

// stateResources must only be called while the lock is held.
func (e *executor) stateResources(ctx, killCtx context.Context) ([]*proto.Resource, []*proto.RichParameter, error) {
	state, err := e.state(ctx, killCtx)
	if err != nil {
		return nil, nil, err
	}
	rawGraph, err := e.graph(ctx, killCtx)
	if err != nil {
		return nil, nil, xerrors.Errorf("get terraform graph: %w", err)
	}
	var resources []*proto.Resource
	var parameters []*proto.RichParameter
	if state.Values != nil {
		resources, parameters, err = ConvertResourcesAndParameters([]*tfjson.StateModule{
			state.Values.RootModule,
		}, rawGraph)
		if err != nil {
			return nil, nil, err
		}
	}
	return resources, parameters, nil
}

// state must only be called while the lock is held.
func (e *executor) state(ctx, killCtx context.Context) (*tfjson.State, error) {
	args := []string{"show", "-json", "-no-color"}
	state := &tfjson.State{}
	err := e.execParseJSON(ctx, killCtx, args, e.basicEnv(), state)
	if err != nil {
		return nil, xerrors.Errorf("terraform show state: %w", err)
	}
	return state, nil
}

func interruptCommandOnCancel(ctx, killCtx context.Context, cmd *exec.Cmd) {
	go func() {
		select {
		case <-ctx.Done():
			switch runtime.GOOS {
			case "windows":
				// Interrupts aren't supported by Windows.
				_ = cmd.Process.Kill()
			default:
				_ = cmd.Process.Signal(os.Interrupt)
			}

		case <-killCtx.Done():
		}
	}()
}

type logSink interface {
	Log(*proto.Log)
}

type streamLogSink struct {
	// Any errors writing to the stream will be logged to logger.
	logger slog.Logger
	stream proto.DRPCProvisioner_ProvisionStream
}

var _ logSink = streamLogSink{}

func (s streamLogSink) Log(l *proto.Log) {
	err := s.stream.Send(&proto.Provision_Response{
		Type: &proto.Provision_Response_Log{
			Log: l,
		},
	})
	if err != nil {
		s.logger.Warn(context.Background(), "write log to stream",
			slog.F("level", l.Level.String()),
			slog.F("message", l.Output),
			slog.Error(err),
		)
	}
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
		sink.Log(&proto.Log{Level: level, Output: scanner.Text()})
	}
}

// provisionLogWriter creates a WriteCloser that will log each JSON formatted terraform log.  The WriteCloser must be
// closed by the caller to end logging, after which the returned channel will be closed to indicate that logging of the
// written data has finished.  Failure to close the WriteCloser will leak a goroutine.
func provisionLogWriter(sink logSink) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)
	go provisionReadAndLog(sink, r, done)
	return w, done
}

func provisionReadAndLog(sink logSink, r io.Reader, done chan<- any) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var log terraformProvisionLog
		err := json.Unmarshal(scanner.Bytes(), &log)
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
			if strings.TrimSpace(scanner.Text()) == "" {
				continue
			}
			log.Level = "info"
			log.Message = scanner.Text()
		}

		logLevel := convertTerraformLogLevel(log.Level, sink)
		sink.Log(&proto.Log{Level: logLevel, Output: log.Message})

		// If the diagnostic is provided, let's provide a bit more info!
		if log.Diagnostic == nil {
			continue
		}
		logLevel = convertTerraformLogLevel(log.Diagnostic.Severity, sink)
		sink.Log(&proto.Log{Level: logLevel, Output: log.Diagnostic.Detail})
	}
}

func convertTerraformLogLevel(logLevel string, sink logSink) proto.LogLevel {
	switch strings.ToLower(logLevel) {
	case "trace":
		return proto.LogLevel_TRACE
	case "debug":
		return proto.LogLevel_DEBUG
	case "info":
		return proto.LogLevel_INFO
	case "warn":
		return proto.LogLevel_WARN
	case "error":
		return proto.LogLevel_ERROR
	default:
		sink.Log(&proto.Log{
			Level:  proto.LogLevel_WARN,
			Output: fmt.Sprintf("unable to convert log level %s", logLevel),
		})
		return proto.LogLevel_INFO
	}
}

type terraformProvisionLog struct {
	Level   string `json:"@level"`
	Message string `json:"@message"`

	Diagnostic *terraformProvisionLogDiagnostic `json:"diagnostic"`
}

type terraformProvisionLogDiagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
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

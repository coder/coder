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

	"golang.org/x/xerrors"

	"github.com/hashicorp/go-version"
	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/provisionersdk/proto"
)

type executor struct {
	initMu     sync.Locker
	binaryPath string
	cachePath  string
	workdir    string
}

func (e executor) basicEnv() []string {
	// Required for "terraform init" to find "git" to
	// clone Terraform modules.
	env := os.Environ()
	// Only Linux reliably works with the Terraform plugin
	// cache directory. It's unknown why this is.
	if e.cachePath != "" && runtime.GOOS == "linux" {
		env = append(env, "TF_PLUGIN_CACHE_DIR="+e.cachePath)
	}
	return env
}

func (e executor) execWriteOutput(ctx, killCtx context.Context, args, env []string, stdOutWriter, stdErrWriter io.WriteCloser) (err error) {
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

	// #nosec
	cmd := exec.CommandContext(killCtx, e.binaryPath, args...)
	cmd.Dir = e.workdir
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

func (e executor) execParseJSON(ctx, killCtx context.Context, args, env []string, v interface{}) error {
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

func (e executor) checkMinVersion(ctx context.Context) error {
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

func (e executor) version(ctx context.Context) (*version.Version, error) {
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

func (e executor) init(ctx, killCtx context.Context, logr logger) error {
	outWriter, doneOut := logWriter(logr, proto.LogLevel_DEBUG)
	errWriter, doneErr := logWriter(logr, proto.LogLevel_ERROR)
	defer func() {
		<-doneOut
		<-doneErr
	}()

	args := []string{
		"init",
		"-no-color",
		"-input=false",
	}

	// When cache path is set, we must protect against multiple calls
	// to `terraform init`.
	//
	// From the Terraform documentation:
	//     Note: The plugin cache directory is not guaranteed to be
	//     concurrency safe. The provider installer's behavior in
	//     environments with multiple terraform init calls is undefined.
	if e.cachePath != "" {
		e.initMu.Lock()
		defer e.initMu.Unlock()
	}

	return e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), outWriter, errWriter)
}

// revive:disable-next-line:flag-parameter
func (e executor) plan(ctx, killCtx context.Context, env, vars []string, logr logger, destroy bool) (*proto.Provision_Response, error) {
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
		<-doneOut
		<-doneErr
	}()

	err := e.execWriteOutput(ctx, killCtx, args, env, outWriter, errWriter)
	if err != nil {
		return nil, xerrors.Errorf("terraform plan: %w", err)
	}
	resources, err := e.planResources(ctx, killCtx, planfilePath)
	if err != nil {
		return nil, err
	}
	return &proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				Resources: resources,
			},
		},
	}, nil
}

func (e executor) planResources(ctx, killCtx context.Context, planfilePath string) ([]*proto.Resource, error) {
	plan, err := e.showPlan(ctx, killCtx, planfilePath)
	if err != nil {
		return nil, xerrors.Errorf("show terraform plan file: %w", err)
	}

	rawGraph, err := e.graph(ctx, killCtx)
	if err != nil {
		return nil, xerrors.Errorf("graph: %w", err)
	}
	return ConvertResources(plan.PlannedValues.RootModule, rawGraph)
}

func (e executor) showPlan(ctx, killCtx context.Context, planfilePath string) (*tfjson.Plan, error) {
	args := []string{"show", "-json", "-no-color", planfilePath}
	p := new(tfjson.Plan)
	err := e.execParseJSON(ctx, killCtx, args, e.basicEnv(), p)
	return p, err
}

func (e executor) graph(ctx, killCtx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	var out bytes.Buffer
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

// revive:disable-next-line:flag-parameter
func (e executor) apply(ctx, killCtx context.Context, env, vars []string, logr logger, destroy bool,
) (*proto.Provision_Response, error) {
	args := []string{
		"apply",
		"-no-color",
		"-auto-approve",
		"-input=false",
		"-json",
		"-refresh=true",
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
		<-doneOut
		<-doneErr
	}()

	err := e.execWriteOutput(ctx, killCtx, args, env, outWriter, errWriter)
	if err != nil {
		return nil, xerrors.Errorf("terraform apply: %w", err)
	}
	resources, err := e.stateResources(ctx, killCtx)
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
				Resources: resources,
				State:     stateContent,
			},
		},
	}, nil
}

func (e executor) stateResources(ctx, killCtx context.Context) ([]*proto.Resource, error) {
	state, err := e.state(ctx, killCtx)
	if err != nil {
		return nil, err
	}
	rawGraph, err := e.graph(ctx, killCtx)
	if err != nil {
		return nil, xerrors.Errorf("get terraform graph: %w", err)
	}
	var resources []*proto.Resource
	if state.Values != nil {
		resources, err = ConvertResources(state.Values.RootModule, rawGraph)
		if err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func (e executor) state(ctx, killCtx context.Context) (*tfjson.State, error) {
	args := []string{"show", "-json"}
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

type logger interface {
	Log(*proto.Log) error
}

type streamLogger struct {
	stream proto.DRPCProvisioner_ProvisionStream
}

func (s streamLogger) Log(l *proto.Log) error {
	return s.stream.Send(&proto.Provision_Response{
		Type: &proto.Provision_Response_Log{
			Log: l,
		},
	})
}

// logWriter creates a WriteCloser that will log each line of text at the given level.  The WriteCloser must be closed
// by the caller to end logging, after which the returned channel will be closed to indicate that logging of the written
// data has finished.  Failure to close the WriteCloser will leak a goroutine.
func logWriter(logr logger, level proto.LogLevel) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)
	go readAndLog(logr, r, done, level)
	return w, done
}

func readAndLog(logr logger, r io.Reader, done chan<- any, level proto.LogLevel) {
	defer close(done)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		err := logr.Log(&proto.Log{Level: level, Output: scanner.Text()})
		if err != nil {
			// Not much we can do.  We can't log because logging is itself breaking!
			return
		}
	}
}

// provisionLogWriter creates a WriteCloser that will log each JSON formatted terraform log.  The WriteCloser must be
// closed by the caller to end logging, after which the returned channel will be closed to indicate that logging of the
// written data has finished.  Failure to close the WriteCloser will leak a goroutine.
func provisionLogWriter(logr logger) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)
	go provisionReadAndLog(logr, r, done)
	return w, done
}

func provisionReadAndLog(logr logger, reader io.Reader, done chan<- any) {
	defer close(done)
	decoder := json.NewDecoder(reader)
	for {
		var log terraformProvisionLog
		err := decoder.Decode(&log)
		if err != nil {
			return
		}
		logLevel := convertTerraformLogLevel(log.Level, logr)

		err = logr.Log(&proto.Log{Level: logLevel, Output: log.Message})
		if err != nil {
			// Not much we can do.  We can't log because logging is itself breaking!
			return
		}

		if log.Diagnostic == nil {
			continue
		}

		// If the diagnostic is provided, let's provide a bit more info!
		logLevel = convertTerraformLogLevel(log.Diagnostic.Severity, logr)
		err = logr.Log(&proto.Log{Level: logLevel, Output: log.Diagnostic.Detail})
		if err != nil {
			// Not much we can do.  We can't log because logging is itself breaking!
			return
		}
	}
}

func convertTerraformLogLevel(logLevel string, logr logger) proto.LogLevel {
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
		_ = logr.Log(&proto.Log{
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

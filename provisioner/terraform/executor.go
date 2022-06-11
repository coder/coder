package terraform

import (
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

	"golang.org/x/xerrors"

	"github.com/hashicorp/go-version"
	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

type executor struct {
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

func (e executor) execWriteOutput(ctx context.Context, args, env []string, writer io.WriteCloser) (err error) {
	defer func() {
		closeErr := writer.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	stdErr := &bytes.Buffer{}
	// #nosec
	cmd := exec.CommandContext(ctx, e.binaryPath, args...)
	cmd.Dir = e.workdir
	cmd.Stdout = writer
	cmd.Stderr = stdErr
	cmd.Env = env
	if err = cmd.Run(); err != nil {
		errString, _ := io.ReadAll(stdErr)
		return xerrors.Errorf("%s: %w", errString, err)
	}
	return nil
}

func (e executor) execParseJSON(ctx context.Context, args, env []string, v interface{}) error {
	// #nosec
	cmd := exec.CommandContext(ctx, e.binaryPath, args...)
	cmd.Dir = e.workdir
	cmd.Env = env
	out := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = stdErr
	err := cmd.Run()
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
	v, err := e.getVersion(ctx)
	if err != nil {
		return err
	}
	if !v.GreaterThanOrEqual(minimumTerraformVersion) {
		return xerrors.Errorf(
			"terraform version %q is too old. required >= %q",
			v.String(),
			minimumTerraformVersion.String())
	}
	return nil
}

func (e executor) getVersion(ctx context.Context) (*version.Version, error) {
	// #nosec
	cmd := exec.CommandContext(ctx, e.binaryPath, "version", "-json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	vj := tfjson.VersionOutput{}
	err = json.Unmarshal(out, &vj)
	if err != nil {
		return nil, err
	}
	return version.NewVersion(vj.Version)
}

func (e executor) init(ctx context.Context, logger provisionersdk.Logger) error {
	writer, doneLogging := provisionersdk.LogWriter(logger, proto.LogLevel_DEBUG)
	defer func() { <-doneLogging }()
	return e.execWriteOutput(ctx, []string{"init"}, e.basicEnv(), writer)
}

// revive:disable-next-line:flag-parameter
func (e executor) plan(ctx context.Context, env, vars []string, logger provisionersdk.Logger, destroy bool) (*proto.Provision_Response, error) {
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

	writer, doneLogging := provisionLogWriter(logger)
	defer func() { <-doneLogging }()

	err := e.execWriteOutput(ctx, args, env, writer)
	if err != nil {
		return nil, xerrors.Errorf("terraform plan: %w", err)
	}
	resources, err := e.planResources(ctx, planfilePath)
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

func (e executor) planResources(ctx context.Context, planfilePath string) ([]*proto.Resource, error) {
	plan, err := e.showPlan(ctx, planfilePath)
	if err != nil {
		return nil, xerrors.Errorf("show terraform plan file: %w", err)
	}

	rawGraph, err := e.graph(ctx)
	if err != nil {
		return nil, xerrors.Errorf("graph: %w", err)
	}
	return ConvertResources(plan.PlannedValues.RootModule, rawGraph)
}

func (e executor) showPlan(ctx context.Context, planfilePath string) (*tfjson.Plan, error) {
	args := []string{"show", "-json", "-no-color", planfilePath}
	p := new(tfjson.Plan)
	err := e.execParseJSON(ctx, args, e.basicEnv(), p)
	return p, err
}

func (e executor) graph(ctx context.Context) (string, error) {
	// #nosec
	cmd := exec.CommandContext(ctx, e.binaryPath, "graph")
	cmd.Dir = e.workdir
	cmd.Env = e.basicEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("graph: %w", err)
	}
	return string(out), nil
}

// revive:disable-next-line:flag-parameter
func (e executor) apply(ctx context.Context, env, vars []string, logger provisionersdk.Logger, destroy bool,
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

	writer, doneLogging := provisionLogWriter(logger)
	defer func() { <-doneLogging }()

	err := e.execWriteOutput(ctx, args, env, writer)
	if err != nil {
		return nil, xerrors.Errorf("terraform apply: %w", err)
	}
	resources, err := e.stateResources(ctx)
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

func (e executor) stateResources(ctx context.Context) ([]*proto.Resource, error) {
	state, err := e.getState(ctx)
	if err != nil {
		return nil, err
	}
	rawGraph, err := e.graph(ctx)
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

func (e executor) getState(ctx context.Context) (*tfjson.State, error) {
	args := []string{"show", "-json"}
	state := &tfjson.State{}
	err := e.execParseJSON(ctx, args, e.basicEnv(), state)
	if err != nil {
		return nil, xerrors.Errorf("get terraform state: %w", err)
	}
	return state, nil
}

func provisionLogWriter(logger provisionersdk.Logger) (io.WriteCloser, <-chan any) {
	r, w := io.Pipe()
	done := make(chan any)
	go provisionReadAndLog(logger, r, done)
	return w, done
}

func provisionReadAndLog(logger provisionersdk.Logger, reader io.Reader, done chan<- any) {
	defer close(done)
	decoder := json.NewDecoder(reader)
	for {
		var log terraformProvisionLog
		err := decoder.Decode(&log)
		if err != nil {
			return
		}
		logLevel := convertTerraformLogLevel(log.Level, logger)

		err = logger.Log(&proto.Log{Level: logLevel, Output: log.Message})
		if err != nil {
			// Not much we can do.  We can't log because logging is itself breaking!
			return
		}

		if log.Diagnostic == nil {
			continue
		}

		// If the diagnostic is provided, let's provide a bit more info!
		logLevel = convertTerraformLogLevel(log.Diagnostic.Severity, logger)
		if err != nil {
			continue
		}
		err = logger.Log(&proto.Log{Level: logLevel, Output: log.Diagnostic.Detail})
		if err != nil {
			// Not much we can do.  We can't log because logging is itself breaking!
			return
		}
	}
}

func convertTerraformLogLevel(logLevel string, logger provisionersdk.Logger) proto.LogLevel {
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
		_ = logger.Log(&proto.Log{
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

package agentcontainers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
)

// DevcontainerConfig is a wrapper around the output from `read-configuration`.
// Unfortunately we cannot make use of `dcspec` as the output doesn't appear to
// match.
type DevcontainerConfig struct {
	MergedConfiguration DevcontainerConfiguration `json:"mergedConfiguration"`
}

type DevcontainerConfiguration struct {
	Customizations DevcontainerCustomizations `json:"customizations,omitempty"`
}

type DevcontainerCustomizations struct {
	Coder []CoderCustomization `json:"coder,omitempty"`
}

type CoderCustomization struct {
	DisplayApps map[codersdk.DisplayApp]bool `json:"displayApps,omitempty"`
}

// DevcontainerCLI is an interface for the devcontainer CLI.
type DevcontainerCLI interface {
	Up(ctx context.Context, workspaceFolder, configPath string, opts ...DevcontainerCLIUpOptions) (id string, err error)
	Exec(ctx context.Context, workspaceFolder, configPath string, cmd string, cmdArgs []string, opts ...DevcontainerCLIExecOptions) error
	ReadConfig(ctx context.Context, workspaceFolder, configPath string, opts ...DevcontainerCLIReadConfigOptions) (DevcontainerConfig, error)
}

// DevcontainerCLIUpOptions are options for the devcontainer CLI Up
// command.
type DevcontainerCLIUpOptions func(*devcontainerCLIUpConfig)

type devcontainerCLIUpConfig struct {
	args   []string // Additional arguments for the Up command.
	stdout io.Writer
	stderr io.Writer
}

// WithRemoveExistingContainer is an option to remove the existing
// container.
func WithRemoveExistingContainer() DevcontainerCLIUpOptions {
	return func(o *devcontainerCLIUpConfig) {
		o.args = append(o.args, "--remove-existing-container")
	}
}

// WithUpOutput sets additional stdout and stderr writers for logs
// during Up operations.
func WithUpOutput(stdout, stderr io.Writer) DevcontainerCLIUpOptions {
	return func(o *devcontainerCLIUpConfig) {
		o.stdout = stdout
		o.stderr = stderr
	}
}

// DevcontainerCLIExecOptions are options for the devcontainer CLI Exec
// command.
type DevcontainerCLIExecOptions func(*devcontainerCLIExecConfig)

type devcontainerCLIExecConfig struct {
	args   []string // Additional arguments for the Exec command.
	stdout io.Writer
	stderr io.Writer
}

// WithExecOutput sets additional stdout and stderr writers for logs
// during Exec operations.
func WithExecOutput(stdout, stderr io.Writer) DevcontainerCLIExecOptions {
	return func(o *devcontainerCLIExecConfig) {
		o.stdout = stdout
		o.stderr = stderr
	}
}

// WithExecContainerID sets the container ID to target a specific
// container.
func WithExecContainerID(id string) DevcontainerCLIExecOptions {
	return func(o *devcontainerCLIExecConfig) {
		o.args = append(o.args, "--container-id", id)
	}
}

// WithRemoteEnv sets environment variables for the Exec command.
func WithRemoteEnv(env ...string) DevcontainerCLIExecOptions {
	return func(o *devcontainerCLIExecConfig) {
		for _, e := range env {
			o.args = append(o.args, "--remote-env", e)
		}
	}
}

// DevcontainerCLIExecOptions are options for the devcontainer CLI ReadConfig
// command.
type DevcontainerCLIReadConfigOptions func(*devcontainerCLIReadConfigConfig)

type devcontainerCLIReadConfigConfig struct {
	stdout io.Writer
	stderr io.Writer
}

// WithExecOutput sets additional stdout and stderr writers for logs
// during Exec operations.
func WithReadConfigOutput(stdout, stderr io.Writer) DevcontainerCLIReadConfigOptions {
	return func(o *devcontainerCLIReadConfigConfig) {
		o.stdout = stdout
		o.stderr = stderr
	}
}

func applyDevcontainerCLIUpOptions(opts []DevcontainerCLIUpOptions) devcontainerCLIUpConfig {
	conf := devcontainerCLIUpConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&conf)
		}
	}
	return conf
}

func applyDevcontainerCLIExecOptions(opts []DevcontainerCLIExecOptions) devcontainerCLIExecConfig {
	conf := devcontainerCLIExecConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&conf)
		}
	}
	return conf
}

func applyDevcontainerCLIReadConfigOptions(opts []DevcontainerCLIReadConfigOptions) devcontainerCLIReadConfigConfig {
	conf := devcontainerCLIReadConfigConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&conf)
		}
	}
	return conf
}

type devcontainerCLI struct {
	logger slog.Logger
	execer agentexec.Execer
}

var _ DevcontainerCLI = &devcontainerCLI{}

func NewDevcontainerCLI(logger slog.Logger, execer agentexec.Execer) DevcontainerCLI {
	return &devcontainerCLI{
		execer: execer,
		logger: logger,
	}
}

func (d *devcontainerCLI) Up(ctx context.Context, workspaceFolder, configPath string, opts ...DevcontainerCLIUpOptions) (string, error) {
	conf := applyDevcontainerCLIUpOptions(opts)
	logger := d.logger.With(slog.F("workspace_folder", workspaceFolder), slog.F("config_path", configPath))

	args := []string{
		"up",
		"--log-format", "json",
		"--workspace-folder", workspaceFolder,
	}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, conf.args...)
	cmd := d.execer.CommandContext(ctx, "devcontainer", args...)

	// Capture stdout for parsing and stream logs for both default and provided writers.
	var stdoutBuf bytes.Buffer
	stdoutWriters := []io.Writer{&stdoutBuf, &devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stdout", true))}}
	if conf.stdout != nil {
		stdoutWriters = append(stdoutWriters, conf.stdout)
	}
	cmd.Stdout = io.MultiWriter(stdoutWriters...)
	// Stream stderr logs and provided writer if any.
	stderrWriters := []io.Writer{&devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stderr", true))}}
	if conf.stderr != nil {
		stderrWriters = append(stderrWriters, conf.stderr)
	}
	cmd.Stderr = io.MultiWriter(stderrWriters...)

	if err := cmd.Run(); err != nil {
		_, err2 := parseDevcontainerCLILastLine[devcontainerCLIResult](ctx, logger, stdoutBuf.Bytes())
		if err2 != nil {
			err = errors.Join(err, err2)
		}
		return "", err
	}

	result, err := parseDevcontainerCLILastLine[devcontainerCLIResult](ctx, logger, stdoutBuf.Bytes())
	if err != nil {
		return "", err
	}

	return result.ContainerID, nil
}

func (d *devcontainerCLI) Exec(ctx context.Context, workspaceFolder, configPath string, cmd string, cmdArgs []string, opts ...DevcontainerCLIExecOptions) error {
	conf := applyDevcontainerCLIExecOptions(opts)
	logger := d.logger.With(slog.F("workspace_folder", workspaceFolder), slog.F("config_path", configPath))

	args := []string{"exec"}
	// For now, always set workspace folder even if --container-id is provided.
	// Otherwise the environment of exec will be incomplete, like `pwd` will be
	// /home/coder instead of /workspaces/coder. The downside is that the local
	// `devcontainer.json` config will overwrite settings serialized in the
	// container label.
	if workspaceFolder != "" {
		args = append(args, "--workspace-folder", workspaceFolder)
	}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	args = append(args, conf.args...)
	args = append(args, cmd)
	args = append(args, cmdArgs...)
	c := d.execer.CommandContext(ctx, "devcontainer", args...)

	stdoutWriters := []io.Writer{&devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stdout", true))}}
	if conf.stdout != nil {
		stdoutWriters = append(stdoutWriters, conf.stdout)
	}
	c.Stdout = io.MultiWriter(stdoutWriters...)
	stderrWriters := []io.Writer{&devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stderr", true))}}
	if conf.stderr != nil {
		stderrWriters = append(stderrWriters, conf.stderr)
	}
	c.Stderr = io.MultiWriter(stderrWriters...)

	if err := c.Run(); err != nil {
		return xerrors.Errorf("devcontainer exec failed: %w", err)
	}

	return nil
}

func (d *devcontainerCLI) ReadConfig(ctx context.Context, workspaceFolder, configPath string, opts ...DevcontainerCLIReadConfigOptions) (DevcontainerConfig, error) {
	conf := applyDevcontainerCLIReadConfigOptions(opts)
	logger := d.logger.With(slog.F("workspace_folder", workspaceFolder), slog.F("config_path", configPath))

	args := []string{"read-configuration", "--include-merged-configuration"}
	if workspaceFolder != "" {
		args = append(args, "--workspace-folder", workspaceFolder)
	}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}

	c := d.execer.CommandContext(ctx, "devcontainer", args...)

	var stdoutBuf bytes.Buffer
	stdoutWriters := []io.Writer{&stdoutBuf, &devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stdout", true))}}
	if conf.stdout != nil {
		stdoutWriters = append(stdoutWriters, conf.stdout)
	}
	c.Stdout = io.MultiWriter(stdoutWriters...)
	stderrWriters := []io.Writer{&devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stderr", true))}}
	if conf.stderr != nil {
		stderrWriters = append(stderrWriters, conf.stderr)
	}
	c.Stderr = io.MultiWriter(stderrWriters...)

	if err := c.Run(); err != nil {
		return DevcontainerConfig{}, xerrors.Errorf("devcontainer read-configuration failed: %w", err)
	}

	config, err := parseDevcontainerCLILastLine[DevcontainerConfig](ctx, logger, stdoutBuf.Bytes())
	if err != nil {
		return DevcontainerConfig{}, err
	}

	return config, nil
}

// parseDevcontainerCLILastLine parses the last line of the devcontainer CLI output
// which is a JSON object.
func parseDevcontainerCLILastLine[T any](ctx context.Context, logger slog.Logger, p []byte) (T, error) {
	var result T

	s := bufio.NewScanner(bytes.NewReader(p))
	var lastLine []byte
	for s.Scan() {
		b := s.Bytes()
		if len(b) == 0 || b[0] != '{' {
			continue
		}
		lastLine = b
	}
	if err := s.Err(); err != nil {
		return result, err
	}
	if len(lastLine) == 0 || lastLine[0] != '{' {
		logger.Error(ctx, "devcontainer result is not json", slog.F("result", string(lastLine)))
		return result, xerrors.Errorf("devcontainer result is not json: %q", string(lastLine))
	}
	if err := json.Unmarshal(lastLine, &result); err != nil {
		logger.Error(ctx, "parse devcontainer result failed", slog.Error(err), slog.F("result", string(lastLine)))
		return result, err
	}

	return result, nil
}

// devcontainerCLIResult is the result of the devcontainer CLI command.
// It is parsed from the last line of the devcontainer CLI stdout which
// is a JSON object.
type devcontainerCLIResult struct {
	Outcome string `json:"outcome"` // "error", "success".

	// The following fields are set if outcome is success.
	ContainerID           string `json:"containerId"`
	RemoteUser            string `json:"remoteUser"`
	RemoteWorkspaceFolder string `json:"remoteWorkspaceFolder"`

	// The following fields are set if outcome is error.
	Message     string `json:"message"`
	Description string `json:"description"`
}

func (r *devcontainerCLIResult) UnmarshalJSON(data []byte) error {
	type wrapperResult devcontainerCLIResult

	var wrappedResult wrapperResult
	if err := json.Unmarshal(data, &wrappedResult); err != nil {
		return err
	}

	*r = devcontainerCLIResult(wrappedResult)
	return r.Err()
}

func (r devcontainerCLIResult) Err() error {
	if r.Outcome == "success" {
		return nil
	}
	return xerrors.Errorf("devcontainer up failed: %s (description: %s, message: %s)", r.Outcome, r.Description, r.Message)
}

// devcontainerCLIJSONLogLine is a log line from the devcontainer CLI.
type devcontainerCLIJSONLogLine struct {
	Type      string `json:"type"`      // "progress", "raw", "start", "stop", "text", etc.
	Level     int    `json:"level"`     // 1, 2, 3.
	Timestamp int    `json:"timestamp"` // Unix timestamp in milliseconds.
	Text      string `json:"text"`

	// More fields can be added here as needed.
}

// devcontainerCLILogWriter splits on newlines and logs each line
// separately.
type devcontainerCLILogWriter struct {
	ctx    context.Context
	logger slog.Logger
}

func (l *devcontainerCLILogWriter) Write(p []byte) (n int, err error) {
	s := bufio.NewScanner(bytes.NewReader(p))
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 {
			continue
		}
		if line[0] != '{' {
			l.logger.Debug(l.ctx, "@devcontainer/cli", slog.F("line", string(line)))
			continue
		}
		var logLine devcontainerCLIJSONLogLine
		if err := json.Unmarshal(line, &logLine); err != nil {
			l.logger.Error(l.ctx, "parse devcontainer json log line failed", slog.Error(err), slog.F("line", string(line)))
			continue
		}
		if logLine.Level >= 3 {
			l.logger.Info(l.ctx, "@devcontainer/cli", slog.F("line", string(line)))
			continue
		}
		l.logger.Debug(l.ctx, "@devcontainer/cli", slog.F("line", string(line)))
	}
	if err := s.Err(); err != nil {
		l.logger.Error(l.ctx, "devcontainer log line scan failed", slog.Error(err))
	}
	return len(p), nil
}

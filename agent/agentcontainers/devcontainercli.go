package agentcontainers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk"
)

// DevcontainerConfig is a wrapper around the output from `read-configuration`.
// Unfortunately we cannot make use of `dcspec` as the output doesn't appear to
// match.
type DevcontainerConfig struct {
	MergedConfiguration DevcontainerMergedConfiguration `json:"mergedConfiguration"`
	Configuration       DevcontainerConfiguration       `json:"configuration"`
	Workspace           DevcontainerWorkspace           `json:"workspace"`
}

type DevcontainerMergedConfiguration struct {
	Customizations DevcontainerMergedCustomizations `json:"customizations,omitempty"`
	Features       DevcontainerFeatures             `json:"features,omitempty"`
}

type DevcontainerMergedCustomizations struct {
	Coder []CoderCustomization `json:"coder,omitempty"`
}

type DevcontainerFeatures map[string]any

// OptionsAsEnvs converts the DevcontainerFeatures into a list of
// environment variables that can be used to set feature options.
// The format is FEATURE_<FEATURE_NAME>_OPTION_<OPTION_NAME>=<value>.
// For example, if the feature is:
//
//		"ghcr.io/coder/devcontainer-features/code-server:1": {
//	   "port": 9090,
//	 }
//
// It will produce:
//
//	FEATURE_CODE_SERVER_OPTION_PORT=9090
//
// Note that the feature name is derived from the last part of the key,
// so "ghcr.io/coder/devcontainer-features/code-server:1" becomes
// "CODE_SERVER". The version part (e.g. ":1") is removed, and dashes in
// the feature and option names are replaced with underscores.
func (f DevcontainerFeatures) OptionsAsEnvs() []string {
	var env []string
	for k, v := range f {
		vv, ok := v.(map[string]any)
		if !ok {
			continue
		}
		// Take the last part of the key as the feature name/path.
		k = k[strings.LastIndex(k, "/")+1:]
		// Remove ":" and anything following it.
		if idx := strings.Index(k, ":"); idx != -1 {
			k = k[:idx]
		}
		k = strings.ReplaceAll(k, "-", "_")
		for k2, v2 := range vv {
			k2 = strings.ReplaceAll(k2, "-", "_")
			env = append(env, fmt.Sprintf("FEATURE_%s_OPTION_%s=%s", strings.ToUpper(k), strings.ToUpper(k2), fmt.Sprintf("%v", v2)))
		}
	}
	slices.Sort(env)
	return env
}

type DevcontainerConfiguration struct {
	Customizations DevcontainerCustomizations `json:"customizations,omitempty"`
}

type DevcontainerCustomizations struct {
	Coder CoderCustomization `json:"coder,omitempty"`
}

type CoderCustomization struct {
	DisplayApps map[codersdk.DisplayApp]bool `json:"displayApps,omitempty"`
	Apps        []SubAgentApp                `json:"apps,omitempty"`
	Name        string                       `json:"name,omitempty"`
	Ignore      bool                         `json:"ignore,omitempty"`
	AutoStart   bool                         `json:"autoStart,omitempty"`
}

type DevcontainerWorkspace struct {
	WorkspaceFolder string `json:"workspaceFolder"`
}

// DevcontainerCLI is an interface for the devcontainer CLI.
type DevcontainerCLI interface {
	Up(ctx context.Context, workspaceFolder, configPath string, opts ...DevcontainerCLIUpOptions) (id string, err error)
	Exec(ctx context.Context, workspaceFolder, configPath string, cmd string, cmdArgs []string, opts ...DevcontainerCLIExecOptions) error
	ReadConfig(ctx context.Context, workspaceFolder, configPath string, env []string, opts ...DevcontainerCLIReadConfigOptions) (DevcontainerConfig, error)
}

// DevcontainerCLIUpOptions are options for the devcontainer CLI Up
// command.
type DevcontainerCLIUpOptions func(*DevcontainerCLIUpConfig)

type DevcontainerCLIUpConfig struct {
	Args   []string // Additional arguments for the Up command.
	Stdout io.Writer
	Stderr io.Writer
}

// WithRemoveExistingContainer is an option to remove the existing
// container.
func WithRemoveExistingContainer() DevcontainerCLIUpOptions {
	return func(o *DevcontainerCLIUpConfig) {
		o.Args = append(o.Args, "--remove-existing-container")
	}
}

// WithUpOutput sets additional stdout and stderr writers for logs
// during Up operations.
func WithUpOutput(stdout, stderr io.Writer) DevcontainerCLIUpOptions {
	return func(o *DevcontainerCLIUpConfig) {
		o.Stdout = stdout
		o.Stderr = stderr
	}
}

// DevcontainerCLIExecOptions are options for the devcontainer CLI Exec
// command.
type DevcontainerCLIExecOptions func(*DevcontainerCLIExecConfig)

type DevcontainerCLIExecConfig struct {
	Args   []string // Additional arguments for the Exec command.
	Stdout io.Writer
	Stderr io.Writer
}

// WithExecOutput sets additional stdout and stderr writers for logs
// during Exec operations.
func WithExecOutput(stdout, stderr io.Writer) DevcontainerCLIExecOptions {
	return func(o *DevcontainerCLIExecConfig) {
		o.Stdout = stdout
		o.Stderr = stderr
	}
}

// WithExecContainerID sets the container ID to target a specific
// container.
func WithExecContainerID(id string) DevcontainerCLIExecOptions {
	return func(o *DevcontainerCLIExecConfig) {
		o.Args = append(o.Args, "--container-id", id)
	}
}

// WithRemoteEnv sets environment variables for the Exec command.
func WithRemoteEnv(env ...string) DevcontainerCLIExecOptions {
	return func(o *DevcontainerCLIExecConfig) {
		for _, e := range env {
			o.Args = append(o.Args, "--remote-env", e)
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

// WithReadConfigOutput sets additional stdout and stderr writers for logs
// during ReadConfig operations.
func WithReadConfigOutput(stdout, stderr io.Writer) DevcontainerCLIReadConfigOptions {
	return func(o *devcontainerCLIReadConfigConfig) {
		o.stdout = stdout
		o.stderr = stderr
	}
}

func applyDevcontainerCLIUpOptions(opts []DevcontainerCLIUpOptions) DevcontainerCLIUpConfig {
	conf := DevcontainerCLIUpConfig{Stdout: io.Discard, Stderr: io.Discard}
	for _, opt := range opts {
		if opt != nil {
			opt(&conf)
		}
	}
	return conf
}

func applyDevcontainerCLIExecOptions(opts []DevcontainerCLIExecOptions) DevcontainerCLIExecConfig {
	conf := DevcontainerCLIExecConfig{Stdout: io.Discard, Stderr: io.Discard}
	for _, opt := range opts {
		if opt != nil {
			opt(&conf)
		}
	}
	return conf
}

func applyDevcontainerCLIReadConfigOptions(opts []DevcontainerCLIReadConfigOptions) devcontainerCLIReadConfigConfig {
	conf := devcontainerCLIReadConfigConfig{stdout: io.Discard, stderr: io.Discard}
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
	args = append(args, conf.Args...)
	cmd := d.execer.CommandContext(ctx, "devcontainer", args...)

	// Capture stdout for parsing and stream logs for both default and provided writers.
	var stdoutBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(
		&stdoutBuf,
		&devcontainerCLILogWriter{
			ctx:    ctx,
			logger: logger.With(slog.F("stdout", true)),
			writer: conf.Stdout,
		},
	)
	// Stream stderr logs and provided writer if any.
	cmd.Stderr = &devcontainerCLILogWriter{
		ctx:    ctx,
		logger: logger.With(slog.F("stderr", true)),
		writer: conf.Stderr,
	}

	if err := cmd.Run(); err != nil {
		result, err2 := parseDevcontainerCLILastLine[devcontainerCLIResult](ctx, logger, stdoutBuf.Bytes())
		if err2 != nil {
			err = errors.Join(err, err2)
		}
		// Return the container ID if available, even if there was an error.
		// This can happen if the container was created successfully but a
		// lifecycle script (e.g. postCreateCommand) failed.
		return result.ContainerID, err
	}

	result, err := parseDevcontainerCLILastLine[devcontainerCLIResult](ctx, logger, stdoutBuf.Bytes())
	if err != nil {
		return "", err
	}

	// Check if the result indicates an error (e.g. lifecycle script failure)
	// but still has a container ID, allowing the caller to potentially
	// continue with the container that was created.
	if err := result.Err(); err != nil {
		return result.ContainerID, err
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
	args = append(args, conf.Args...)
	args = append(args, cmd)
	args = append(args, cmdArgs...)
	c := d.execer.CommandContext(ctx, "devcontainer", args...)

	c.Stdout = io.MultiWriter(conf.Stdout, &devcontainerCLILogWriter{
		ctx:    ctx,
		logger: logger.With(slog.F("stdout", true)),
		writer: io.Discard,
	})
	c.Stderr = io.MultiWriter(conf.Stderr, &devcontainerCLILogWriter{
		ctx:    ctx,
		logger: logger.With(slog.F("stderr", true)),
		writer: io.Discard,
	})

	if err := c.Run(); err != nil {
		return xerrors.Errorf("devcontainer exec failed: %w", err)
	}

	return nil
}

func (d *devcontainerCLI) ReadConfig(ctx context.Context, workspaceFolder, configPath string, env []string, opts ...DevcontainerCLIReadConfigOptions) (DevcontainerConfig, error) {
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
	c.Env = append(c.Env, env...)

	var stdoutBuf bytes.Buffer
	c.Stdout = io.MultiWriter(
		&stdoutBuf,
		&devcontainerCLILogWriter{
			ctx:    ctx,
			logger: logger.With(slog.F("stdout", true)),
			writer: conf.stdout,
		},
	)
	c.Stderr = &devcontainerCLILogWriter{
		ctx:    ctx,
		logger: logger.With(slog.F("stderr", true)),
		writer: conf.stderr,
	}

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

	// The following fields are typically set if outcome is success, but
	// ContainerID may also be present when outcome is error if the
	// container was created but a lifecycle script (e.g. postCreateCommand)
	// failed.
	ContainerID           string `json:"containerId"`
	RemoteUser            string `json:"remoteUser"`
	RemoteWorkspaceFolder string `json:"remoteWorkspaceFolder"`

	// The following fields are set if outcome is error.
	Message     string `json:"message"`
	Description string `json:"description"`
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
	writer io.Writer
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
			_, _ = l.writer.Write([]byte(strings.TrimSpace(logLine.Text) + "\n"))
			continue
		}
		// If we've successfully parsed the final log line, it will successfully parse
		// but will not fill out any of the fields for `logLine`. In this scenario we
		// assume it is the final log line, unmarshal it as that, and check if the
		// outcome is a non-empty string.
		if logLine.Level == 0 {
			var lastLine devcontainerCLIResult
			if err := json.Unmarshal(line, &lastLine); err == nil && lastLine.Outcome != "" {
				_, _ = l.writer.Write(line)
				_, _ = l.writer.Write([]byte{'\n'})
			}
		}
		l.logger.Debug(l.ctx, "@devcontainer/cli", slog.F("line", string(line)))
	}
	if err := s.Err(); err != nil {
		l.logger.Error(l.ctx, "devcontainer log line scan failed", slog.Error(err))
	}
	return len(p), nil
}

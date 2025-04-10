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
)

// DevcontainerCLI is an interface for the devcontainer CLI.
type DevcontainerCLI interface {
	Up(ctx context.Context, workspaceFolder, configPath string, opts ...DevcontainerCLIUpOptions) (id string, err error)
}

// DevcontainerCLIUpOptions are options for the devcontainer CLI up
// command.
type DevcontainerCLIUpOptions func(*devcontainerCLIUpConfig)

// WithRemoveExistingContainer is an option to remove the existing
// container.
func WithRemoveExistingContainer() DevcontainerCLIUpOptions {
	return func(o *devcontainerCLIUpConfig) {
		o.removeExistingContainer = true
	}
}

type devcontainerCLIUpConfig struct {
	removeExistingContainer bool
}

func applyDevcontainerCLIUpOptions(opts []DevcontainerCLIUpOptions) devcontainerCLIUpConfig {
	conf := devcontainerCLIUpConfig{
		removeExistingContainer: false,
	}
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
	logger := d.logger.With(slog.F("workspace_folder", workspaceFolder), slog.F("config_path", configPath), slog.F("recreate", conf.removeExistingContainer))

	args := []string{
		"up",
		"--log-format", "json",
		"--workspace-folder", workspaceFolder,
	}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	if conf.removeExistingContainer {
		args = append(args, "--remove-existing-container")
	}
	cmd := d.execer.CommandContext(ctx, "devcontainer", args...)

	var stdout bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, &devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stdout", true))})
	cmd.Stderr = &devcontainerCLILogWriter{ctx: ctx, logger: logger.With(slog.F("stderr", true))}

	if err := cmd.Run(); err != nil {
		if _, err2 := parseDevcontainerCLILastLine(ctx, logger, stdout.Bytes()); err2 != nil {
			err = errors.Join(err, err2)
		}
		return "", err
	}

	result, err := parseDevcontainerCLILastLine(ctx, logger, stdout.Bytes())
	if err != nil {
		return "", err
	}

	return result.ContainerID, nil
}

// parseDevcontainerCLILastLine parses the last line of the devcontainer CLI output
// which is a JSON object.
func parseDevcontainerCLILastLine(ctx context.Context, logger slog.Logger, p []byte) (result devcontainerCLIResult, err error) {
	s := bufio.NewScanner(bytes.NewReader(p))
	var lastLine []byte
	for s.Scan() {
		b := s.Bytes()
		if len(b) == 0 || b[0] != '{' {
			continue
		}
		lastLine = b
	}
	if err = s.Err(); err != nil {
		return result, err
	}
	if len(lastLine) == 0 || lastLine[0] != '{' {
		logger.Error(ctx, "devcontainer result is not json", slog.F("result", string(lastLine)))
		return result, xerrors.Errorf("devcontainer result is not json: %q", string(lastLine))
	}
	if err = json.Unmarshal(lastLine, &result); err != nil {
		logger.Error(ctx, "parse devcontainer result failed", slog.Error(err), slog.F("result", string(lastLine)))
		return result, err
	}

	return result, result.Err()
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

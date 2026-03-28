package pulumi

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

type executor struct {
	logger     slog.Logger
	server     *server
	mut        *sync.Mutex
	binaryPath string
	cachePath  string
	files      tfpath.Layout
	timings    *timingAggregator
}

var _ io.Writer = syncWriter{}

type logSink func(level proto.LogLevel, line string)

func (e *executor) basicEnv() []string {
	workDir := e.files.WorkDirectory()
	backendDir := filepath.Join(workDir, ".pulumi-backend")
	backendURL := (&url.URL{Scheme: "file", Path: backendDir}).String()

	env := safeEnviron()
	env = append(env,
		"PULUMI_BACKEND_URL="+backendURL,
		"PULUMI_HOME="+e.cachePath,
		"PULUMI_SKIP_UPDATE_CHECK=true",
		"PULUMI_CONFIG_PASSPHRASE=",
		"PULUMI_DIY_BACKEND_URL="+backendURL,
	)
	return env
}

func (e *executor) execWriteOutput(ctx, killCtx context.Context, args, env []string, stdOutWriter, stdErrWriter io.WriteCloser) error {
	return e.execWriteOutputBinary(ctx, killCtx, e.binaryPath, args, env, stdOutWriter, stdErrWriter)
}

func (e *executor) execWriteOutputBinary(ctx, killCtx context.Context, binaryPath string, args, env []string, stdOutWriter, stdErrWriter io.WriteCloser) (err error) {
	if ctx == nil {
		return xerrors.New("context must not be nil")
	}
	if killCtx == nil {
		return xerrors.New("kill context must not be nil")
	}
	if stdOutWriter == nil {
		return xerrors.New("stdout writer must not be nil")
	}
	if stdErrWriter == nil {
		return xerrors.New("stderr writer must not be nil")
	}
	if e.mut == nil {
		return xerrors.New("executor mutex must not be nil")
	}
	if strings.TrimSpace(binaryPath) == "" {
		return xerrors.New("binary path must not be empty")
	}
	if len(args) == 0 {
		return xerrors.New("args must not be empty")
	}

	e.mut.Lock()
	defer e.mut.Unlock()
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

	startedAt := time.Now().UTC()

	// #nosec G204 -- Command path comes from validated Pulumi config or a fixed npm binary.
	cmd := exec.CommandContext(killCtx, binaryPath, args...)
	cmd.Dir = e.files.WorkDirectory()
	if env == nil {
		env = []string{}
	}
	cmd.Env = env

	mut := &sync.Mutex{}
	cmd.Stdout = syncWriter{mut: mut, w: stdOutWriter}
	cmd.Stderr = syncWriter{mut: mut, w: stdErrWriter}

	logMessage := "executing dependency command"
	if binaryPath == e.binaryPath {
		logMessage = "executing pulumi command"
	}
	e.server.logger.Debug(ctx, logMessage,
		slog.F("binary_path", binaryPath),
		slog.F("args", args),
	)
	if err := cmd.Start(); err != nil {
		e.timings.recordCommand(args[0], startedAt, err)
		return err
	}
	interruptCommandOnCancel(ctx, killCtx, e.logger, cmd)

	err = cmd.Wait()
	e.timings.recordCommand(args[0], startedAt, err)
	return err
}

func (e *executor) execCaptureOutput(ctx, killCtx context.Context, args, env []string) (_ []byte, err error) {
	if ctx == nil {
		return nil, xerrors.New("context must not be nil")
	}
	if killCtx == nil {
		return nil, xerrors.New("kill context must not be nil")
	}
	if e.mut == nil {
		return nil, xerrors.New("executor mutex must not be nil")
	}
	if e.binaryPath == "" {
		return nil, xerrors.New("binary path must not be empty")
	}
	if len(args) == 0 {
		return nil, xerrors.New("args must not be empty")
	}

	e.mut.Lock()
	defer e.mut.Unlock()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if isCanarySet(env) {
		return nil, xerrors.New("environment variables not sanitized, this is a bug within Coder")
	}

	startedAt := time.Now().UTC()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stderrWriter := newLineSinkWriteCloser(func(line string) {
		e.logger.Warn(ctx, "pulumi stderr output", slog.F("line", line))
	})
	defer func() {
		closeErr := stderrWriter.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	// #nosec G204 -- Pulumi binary path is validated during serve startup.
	cmd := exec.CommandContext(killCtx, e.binaryPath, args...)
	cmd.Dir = e.files.WorkDirectory()
	if env == nil {
		env = []string{}
	}
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = io.MultiWriter(stderr, stderrWriter)

	e.server.logger.Debug(ctx, "executing pulumi command with captured stdout",
		slog.F("binary_path", e.binaryPath),
		slog.F("args", args),
	)
	if err := cmd.Start(); err != nil {
		e.timings.recordCommand(args[0], startedAt, err)
		return nil, err
	}
	interruptCommandOnCancel(ctx, killCtx, e.logger, cmd)

	if err := cmd.Wait(); err != nil {
		e.timings.recordCommand(args[0], startedAt, err)
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText == "" {
			return nil, err
		}
		return nil, xerrors.Errorf("%s: %w", stderrText, err)
	}

	e.timings.recordCommand(args[0], startedAt, nil)
	return stdout.Bytes(), nil
}

func (e *executor) login(ctx, killCtx context.Context) error {
	args := []string{"login", e.backendURL(), "--non-interactive"}
	stdout := newLineSinkWriteCloser(func(line string) {
		e.logger.Info(ctx, "pulumi login output", slog.F("line", line))
	})
	stderr := newLineSinkWriteCloser(func(line string) {
		e.logger.Warn(ctx, "pulumi login output", slog.F("line", line))
	})
	return e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), stdout, stderr)
}

func (e *executor) stackInit(ctx, killCtx context.Context, stackName string) error {
	if strings.TrimSpace(stackName) == "" {
		return xerrors.New("stack name must not be empty")
	}
	args := []string{"stack", "init", stackName, "--non-interactive"}
	stdout := newLineSinkWriteCloser(func(line string) {
		e.logger.Info(ctx, "pulumi stack init", slog.F("line", line))
	})
	stderr := newLineSinkWriteCloser(func(line string) {
		e.logger.Warn(ctx, "pulumi stack init", slog.F("line", line))
	})
	return e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), stdout, stderr)
}

func (e *executor) install(ctx, killCtx context.Context) error {
	args := []string{"install"}
	stdout := newLineSinkWriteCloser(func(line string) {
		e.logger.Info(ctx, "pulumi dependency install", slog.F("line", line))
	})
	stderr := newLineSinkWriteCloser(func(line string) {
		e.logger.Warn(ctx, "pulumi dependency install", slog.F("line", line))
	})
	return e.execWriteOutputBinary(ctx, killCtx, "npm", args, e.basicEnv(), stdout, stderr)
}

func (e *executor) packageAdd(ctx, killCtx context.Context, source string, params []string) error {
	if strings.TrimSpace(source) == "" {
		return xerrors.New("package source must not be empty")
	}
	if len(params) == 0 {
		return xerrors.New("package parameters must not be empty")
	}

	args := []string{"package", "add", source}
	for i, param := range params {
		if strings.TrimSpace(param) == "" {
			return xerrors.Errorf("package parameter %d must not be empty", i)
		}
		args = append(args, param)
	}
	args = append(args, "--non-interactive")
	stdout := newLineSinkWriteCloser(func(line string) {
		e.logger.Info(ctx, "pulumi package add", slog.F("line", line))
	})
	stderr := newLineSinkWriteCloser(func(line string) {
		e.logger.Warn(ctx, "pulumi package add", slog.F("line", line))
	})
	return e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), stdout, stderr)
}

func (e *executor) stackImport(ctx, killCtx context.Context, stackName string, stateBytes []byte) error {
	if strings.TrimSpace(stackName) == "" {
		return xerrors.New("stack name must not be empty")
	}
	if len(stateBytes) == 0 {
		return xerrors.New("state bytes must not be empty")
	}

	tmpFile, err := os.CreateTemp(e.files.WorkDirectory(), "pulumi-stack-import-*.json")
	if err != nil {
		return xerrors.Errorf("create temporary stack import file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmpFile.Write(stateBytes); err != nil {
		_ = tmpFile.Close()
		return xerrors.Errorf("write temporary stack import file %q: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		return xerrors.Errorf("close temporary stack import file %q: %w", tmpPath, err)
	}

	args := []string{"stack", "import", "--file", tmpPath, "--stack", stackName, "--non-interactive"}
	stdout := newLineSinkWriteCloser(func(line string) {
		e.logger.Info(ctx, "pulumi stack import", slog.F("line", line))
	})
	stderr := newLineSinkWriteCloser(func(line string) {
		e.logger.Warn(ctx, "pulumi stack import", slog.F("line", line))
	})
	return e.execWriteOutput(ctx, killCtx, args, e.basicEnv(), stdout, stderr)
}

// revive:disable-next-line:flag-parameter
func (e *executor) preview(ctx, killCtx context.Context, stackName string, destroy bool, env []string, logr logSink) ([]byte, error) {
	if strings.TrimSpace(stackName) == "" {
		return nil, xerrors.New("stack name must not be empty")
	}
	if logr == nil {
		return nil, xerrors.New("log sink must not be nil")
	}

	args := []string{"preview", "--json", "--non-interactive", "--stack", stackName}
	if destroy {
		args = []string{"destroy", "--preview-only", "--json", "--non-interactive", "--stack", stackName}
	}

	stdout, err := e.execCaptureOutput(ctx, killCtx, args, append(e.basicEnv(), env...))
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		logr(proto.LogLevel_INFO, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("scan preview output: %w", err)
	}

	return stdout, nil
}

func (e *executor) up(ctx, killCtx context.Context, stackName string, env []string, logr logSink) error {
	if strings.TrimSpace(stackName) == "" {
		return xerrors.New("stack name must not be empty")
	}
	if logr == nil {
		return xerrors.New("log sink must not be nil")
	}

	args := []string{"up", "--yes", "--json", "--non-interactive", "--stack", stackName}
	stdout := provisionLogWriter(logr)
	stderr := provisionLogWriter(logr)
	return e.execWriteOutput(ctx, killCtx, args, append(e.basicEnv(), env...), stdout, stderr)
}

func (e *executor) destroy(ctx, killCtx context.Context, stackName string, env []string, logr logSink) error {
	if strings.TrimSpace(stackName) == "" {
		return xerrors.New("stack name must not be empty")
	}
	if logr == nil {
		return xerrors.New("log sink must not be nil")
	}

	args := []string{"destroy", "--yes", "--json", "--non-interactive", "--stack", stackName}
	stdout := provisionLogWriter(logr)
	stderr := provisionLogWriter(logr)
	return e.execWriteOutput(ctx, killCtx, args, append(e.basicEnv(), env...), stdout, stderr)
}

func (e *executor) stackExport(ctx, killCtx context.Context, stackName string) ([]byte, error) {
	if strings.TrimSpace(stackName) == "" {
		return nil, xerrors.New("stack name must not be empty")
	}

	args := []string{"stack", "export", "--stack", stackName}
	return e.execCaptureOutput(ctx, killCtx, args, e.basicEnv())
}

func (e *executor) backendURL() string {
	backendDir := filepath.Join(e.files.WorkDirectory(), ".pulumi-backend")
	return (&url.URL{Scheme: "file", Path: backendDir}).String()
}

func provisionLogWriter(logr logSink) io.WriteCloser {
	if logr == nil {
		return newLineSinkWriteCloser(func(string) {})
	}
	return newLineSinkWriteCloser(func(line string) {
		logr(proto.LogLevel_INFO, line)
	})
}

type syncWriter struct {
	mut *sync.Mutex
	w   io.Writer
}

func (w syncWriter) Write(p []byte) (int, error) {
	if w.mut == nil {
		return 0, xerrors.New("sync writer mutex must not be nil")
	}
	if w.w == nil {
		return 0, xerrors.New("sync writer target must not be nil")
	}
	w.mut.Lock()
	defer w.mut.Unlock()
	return w.w.Write(p)
}

type lineSinkWriteCloser struct {
	writer *io.PipeWriter
	done   chan struct{}
}

var _ io.WriteCloser = (*lineSinkWriteCloser)(nil)

func newLineSinkWriteCloser(sink func(string)) io.WriteCloser {
	r, w := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			sink(line)
		}
		_ = r.Close()
	}()
	return &lineSinkWriteCloser{writer: w, done: done}
}

func (w *lineSinkWriteCloser) Write(p []byte) (int, error) {
	if w == nil || w.writer == nil {
		return 0, xerrors.New("line sink writer must not be nil")
	}
	return w.writer.Write(p)
}

func (w *lineSinkWriteCloser) Close() error {
	if w == nil {
		return xerrors.New("line sink writer must not be nil")
	}
	var err error
	if w.writer != nil {
		err = w.writer.Close()
	}
	if w.done != nil {
		<-w.done
	}
	return err
}

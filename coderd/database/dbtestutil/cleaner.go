package dbtestutil

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/retry"
)

const (
	cleanerRespOK        = "OK"
	envCleanerParentUUID = "DB_CLEANER_PARENT_UUID"
	envCleanerDSN        = "DB_CLEANER_DSN"
)

var (
	originalWorkingDir   string
	errGettingWorkingDir error
)

func init() {
	// We expect our tests to run from somewhere in the project tree where `go run` below in `startCleaner` will
	// be able to resolve the command package. However, some of the tests modify the working directory during the run.
	// So, we grab the working directory during package init, before tests are run, and then set that work dir on the
	// subcommand process before it starts.
	originalWorkingDir, errGettingWorkingDir = os.Getwd()
}

// startCleaner starts the cleaner in a subprocess. holdThis is an opaque reference that needs to be kept from being
// garbage collected until we are done with all test databases (e.g. the end of the process).
func startCleaner(ctx context.Context, parentUUID uuid.UUID, dsn string) (holdThis any, err error) {
	cmd := exec.Command("go", "run", "github.com/coder/coder/v2/coderd/database/dbtestutil/cleanercmd")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=%s", envCleanerParentUUID, parentUUID.String()),
		fmt.Sprintf("%s=%s", envCleanerDSN, dsn),
	)

	// c.f. comment on `func init()` in this file.
	if errGettingWorkingDir != nil {
		return nil, xerrors.Errorf("failed to get working directory during init: %w", errGettingWorkingDir)
	}
	cmd.Dir = originalWorkingDir

	// Here we don't actually use the reference to the stdin pipe, because we never write anything to it. When this
	// process exits, the pipe is closed by the OS and this triggers the cleaner to do its cleaning work. But, we do
	// need to hang on to a reference to it so that it doesn't get garbage collected and trigger cleanup early.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, xerrors.Errorf("failed to open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, xerrors.Errorf("failed to open stdout pipe: %w", err)
	}
	// uncomment this to see log output from the cleaner
	// cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return nil, xerrors.Errorf("failed to start broker: %w", err)
	}
	outCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 1024)
		n, readErr := stdout.Read(buf)
		if readErr != nil {
			errCh <- readErr
			return
		}
		outCh <- buf[:n]
	}()
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, xerrors.Errorf("failed to read db test cleaner output: %w", err)
	case out := <-outCh:
		if string(out) != cleanerRespOK {
			return nil, xerrors.Errorf("db test cleaner error: %s", string(out))
		}
		return stdin, nil
	}
}

type cleaner struct {
	parentUUID uuid.UUID
	logger     slog.Logger
	db         *sql.DB
}

func (c *cleaner) init(ctx context.Context) error {
	var err error
	dsn := os.Getenv(envCleanerDSN)
	if dsn == "" {
		return xerrors.Errorf("DSN not set via env %s: %w", envCleanerDSN, err)
	}
	parentUUIDStr := os.Getenv(envCleanerParentUUID)
	c.parentUUID, err = uuid.Parse(parentUUIDStr)
	if err != nil {
		return xerrors.Errorf("failed to parse parent UUID '%s': %w", parentUUIDStr, err)
	}
	c.logger = slog.Make(sloghuman.Sink(os.Stderr)).
		Named("dbtestcleaner").
		Leveled(slog.LevelDebug).
		With(slog.F("parent_uuid", parentUUIDStr))

	c.db, err = sql.Open("postgres", dsn)
	if err != nil {
		return xerrors.Errorf("couldn't open DB: %w", err)
	}
	for r := retry.New(10*time.Millisecond, 500*time.Millisecond); r.Wait(ctx); {
		err = c.db.PingContext(ctx)
		if err == nil {
			return nil
		}
		c.logger.Error(ctx, "failed to ping DB", slog.Error(err))
	}
	return ctx.Err()
}

// waitAndClean waits for stdin to close then attempts to clean up any test databases with our parent's UUID. This
// is best-effort. If we hit an error we exit.
//
// We log to stderr for debugging, but we don't expect this output to normally be available since the parent has
// exited. Uncomment the line `cmd.Stderr = os.Stderr` in startCleaner() to see this output.
func (c *cleaner) waitAndClean() {
	c.logger.Debug(context.Background(), "waiting for stdin to close")
	_, _ = io.ReadAll(os.Stdin) // here we're just waiting for stdin to close
	c.logger.Debug(context.Background(), "stdin closed")
	rows, err := c.db.Query(
		"SELECT name FROM test_databases WHERE process_uuid = $1 AND dropped_at IS NULL",
		c.parentUUID,
	)
	if err != nil {
		c.logger.Error(context.Background(), "error querying test databases", slog.Error(err))
		return
	}
	defer func() {
		_ = rows.Close()
	}()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	if closeErr := rows.Close(); closeErr != nil {
		c.logger.Error(context.Background(), "error closing rows", slog.Error(closeErr))
	}
	c.logger.Debug(context.Background(), "queried names", slog.F("names", names))
	for _, name := range names {
		_, err := c.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", name))
		if err != nil {
			c.logger.Error(context.Background(), "error dropping database", slog.Error(err), slog.F("name", name))
			return
		}
		_, err = c.db.Exec("UPDATE test_databases SET dropped_at = CURRENT_TIMESTAMP WHERE name = $1", name)
		if err != nil {
			c.logger.Error(context.Background(), "error dropping database", slog.Error(err), slog.F("name", name))
			return
		}
	}
	c.logger.Debug(context.Background(), "finished cleaning")
}

// RunCleaner runs the test database cleaning process. It takes no arguments but uses stdio and environment variables
// for its operation. It is designed to be launched as the only task of a `main()` process, but is included in this
// package to share constants with the parent code that launches it above.
//
// The cleaner is designed to run in a separate process from the main test suite, connected over stdio. If the main test
// process ends (panics, times out, or is killed) without explicitly discarding the databases it clones, the cleaner
// removes them so they don't leak beyond the test session. c.f. https://github.com/coder/internal/issues/927
func RunCleaner() {
	c := cleaner{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// canceling a test via the IDE sends us an interrupt signal. We only want to process that signal during init. After
	// we want to ignore the signal and do our cleaning.
	signalCtx, signalCancel := signal.NotifyContext(ctx, os.Interrupt)
	defer signalCancel()
	err := c.init(signalCtx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, "failed to init: %s", err.Error())
		_ = os.Stdout.Close()
		return
	}
	_, _ = fmt.Fprint(os.Stdout, cleanerRespOK)
	_ = os.Stdout.Close()
	c.waitAndClean()
}

package cliui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
)

func WorkspaceBuild(ctx context.Context, writer io.Writer, client *codersdk.Client, build uuid.UUID) error {
	return ProvisionerJob(ctx, writer, ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			build, err := client.WorkspaceBuild(ctx, build)
			return build.Job, err
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.WorkspaceBuildLogsAfter(ctx, build, 0)
		},
	})
}

type ProvisionerJobOptions struct {
	Fetch  func() (codersdk.ProvisionerJob, error)
	Cancel func() error
	Logs   func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error)

	FetchInterval time.Duration
	// Verbose determines whether debug and trace logs will be shown.
	Verbose bool
	// Silent determines whether log output will be shown unless there is an
	// error.
	Silent bool
}

type ProvisionerJobError struct {
	Message string
	Code    codersdk.JobErrorCode
}

var _ error = new(ProvisionerJobError)

func (err *ProvisionerJobError) Error() string {
	return err.Message
}

// ProvisionerJob renders a provisioner job with interactive cancellation.
func ProvisionerJob(ctx context.Context, wr io.Writer, opts ProvisionerJobOptions) error {
	if opts.FetchInterval == 0 {
		opts.FetchInterval = time.Second
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	var (
		currentStage          = "Queued"
		currentStageStartedAt = time.Now().UTC()

		errChan  = make(chan error, 1)
		job      codersdk.ProvisionerJob
		jobMutex sync.Mutex
	)

	sw := &stageWriter{w: wr, verbose: opts.Verbose, silentLogs: opts.Silent}

	printStage := func() {
		sw.Start(currentStage)
	}

	updateStage := func(stage string, startedAt time.Time) {
		if currentStage != "" {
			duration := startedAt.Sub(currentStageStartedAt)
			if job.CompletedAt != nil && job.Status != codersdk.ProvisionerJobSucceeded {
				sw.Fail(currentStage, duration)
			} else {
				sw.Complete(currentStage, duration)
			}
		}
		if stage == "" {
			return
		}
		currentStage = stage
		currentStageStartedAt = startedAt
		printStage()
	}

	updateJob := func() {
		var err error
		jobMutex.Lock()
		defer jobMutex.Unlock()
		job, err = opts.Fetch()
		if err != nil {
			errChan <- xerrors.Errorf("fetch: %w", err)
			return
		}
		if job.StartedAt == nil {
			return
		}
		if currentStage != "Queued" {
			// If another stage is already running, there's no need
			// for us to notify the user we're running!
			return
		}
		updateStage("Running", *job.StartedAt)
	}

	if opts.Cancel != nil {
		// Handles ctrl+c to cancel a job.
		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt)
		go func() {
			defer signal.Stop(stopChan)
			select {
			case <-ctx.Done():
				return
			case _, ok := <-stopChan:
				if !ok {
					return
				}
			}
			pretty.Fprintf(
				wr,
				DefaultStyles.FocusedPrompt.With(BoldFmt()),
				"Gracefully canceling...\n\n",
			)
			err := opts.Cancel()
			if err != nil {
				errChan <- xerrors.Errorf("cancel: %w", err)
				return
			}
			updateJob()
		}()
	}

	// The initial stage needs to print after the signal handler has been registered.
	printStage()
	updateJob()

	logs, closer, err := opts.Logs()
	if err != nil {
		return xerrors.Errorf("begin streaming logs: %w", err)
	}
	defer closer.Close()

	ticker := time.NewTicker(opts.FetchInterval)
	defer ticker.Stop()
	for {
		select {
		case err = <-errChan:
			sw.Fail(currentStage, time.Since(currentStageStartedAt))
			return err
		case <-ctx.Done():
			sw.Fail(currentStage, time.Since(currentStageStartedAt))
			return ctx.Err()
		case <-ticker.C:
			updateJob()
		case log, ok := <-logs:
			if !ok {
				updateJob()
				jobMutex.Lock()
				if job.CompletedAt != nil {
					updateStage("", *job.CompletedAt)
				}
				switch job.Status {
				case codersdk.ProvisionerJobCanceled:
					jobMutex.Unlock()
					return Canceled
				case codersdk.ProvisionerJobSucceeded:
					jobMutex.Unlock()
					return nil
				case codersdk.ProvisionerJobFailed:
				}
				err = &ProvisionerJobError{
					Message: job.Error,
					Code:    job.ErrorCode,
				}
				sw.Fail(currentStage, time.Since(currentStageStartedAt))
				jobMutex.Unlock()
				return err
			}

			jobMutex.Lock()
			if log.Stage != currentStage && log.Stage != "" {
				updateStage(log.Stage, log.CreatedAt)
				jobMutex.Unlock()
				continue
			}
			sw.Log(log.CreatedAt, log.Level, log.Output)
			jobMutex.Unlock()
		}
	}
}

type stageWriter struct {
	w          io.Writer
	verbose    bool
	silentLogs bool
	logBuf     bytes.Buffer
}

func (s *stageWriter) Start(stage string) {
	_, _ = fmt.Fprintf(s.w, "==> ⧗ %s\n", stage)
}

func (s *stageWriter) Complete(stage string, duration time.Duration) {
	s.end(stage, duration, true)
}

func (s *stageWriter) Fail(stage string, duration time.Duration) {
	s.flushLogs()
	s.end(stage, duration, false)
}

//nolint:revive
func (s *stageWriter) end(stage string, duration time.Duration, ok bool) {
	s.logBuf.Reset()

	mark := "✔"
	if !ok {
		mark = "✘"
	}
	if duration < 0 {
		duration = 0
	}
	_, _ = fmt.Fprintf(s.w, "=== %s %s [%dms]\n", mark, stage, duration.Milliseconds())
}

func (s *stageWriter) Log(createdAt time.Time, level codersdk.LogLevel, line string) {
	w := s.w
	if s.silentLogs {
		w = &s.logBuf
	}

	var style pretty.Style

	var lines []string
	if !createdAt.IsZero() {
		lines = append(lines, createdAt.Local().Format("2006-01-02 15:04:05.000Z07:00"))
	}
	lines = append(lines, line)

	switch level {
	case codersdk.LogLevelTrace, codersdk.LogLevelDebug:
		if !s.verbose {
			return
		}
		style = DefaultStyles.Placeholder
	case codersdk.LogLevelError:
		style = DefaultStyles.Error
	case codersdk.LogLevelWarn:
		style = DefaultStyles.Warn
	case codersdk.LogLevelInfo:
	}
	pretty.Fprintf(w, style, "%s\n", strings.Join(lines, " "))
}

func (s *stageWriter) flushLogs() {
	if s.silentLogs {
		_, _ = io.Copy(s.w, &s.logBuf)
	}
	s.logBuf.Reset()
}

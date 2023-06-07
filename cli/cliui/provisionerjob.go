package cliui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
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
func ProvisionerJob(ctx context.Context, writer io.Writer, opts ProvisionerJobOptions) error {
	if opts.FetchInterval == 0 {
		opts.FetchInterval = time.Second
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	var (
		currentStage          = "Queued"
		currentStageStartedAt = time.Now().UTC()
		didLogBetweenStage    = false

		errChan  = make(chan error, 1)
		job      codersdk.ProvisionerJob
		jobMutex sync.Mutex
	)

	printStage := func() {
		_, _ = fmt.Fprintf(writer, DefaultStyles.Prompt.Render("⧗")+"%s\n", DefaultStyles.Field.Render(currentStage))
	}

	updateStage := func(stage string, startedAt time.Time) {
		if currentStage != "" {
			prefix := ""
			if !didLogBetweenStage {
				prefix = "\033[1A\r"
			}
			mark := DefaultStyles.Checkmark
			if job.CompletedAt != nil && job.Status != codersdk.ProvisionerJobSucceeded {
				mark = DefaultStyles.Crossmark
			}
			_, _ = fmt.Fprintf(writer, prefix+mark.String()+DefaultStyles.Placeholder.Render(" %s [%dms]")+"\n", currentStage, startedAt.Sub(currentStageStartedAt).Milliseconds())
		}
		if stage == "" {
			return
		}
		currentStage = stage
		currentStageStartedAt = startedAt
		didLogBetweenStage = false
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
			_, _ = fmt.Fprintf(writer, "\033[2K\r\n"+DefaultStyles.FocusedPrompt.String()+DefaultStyles.Bold.Render("Gracefully canceling...")+"\n\n")
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

	var (
		// logOutput is where log output is written
		logOutput = writer
		// logBuffer is where logs are buffered if opts.Silent is true
		logBuffer = &bytes.Buffer{}
	)
	if opts.Silent {
		logOutput = logBuffer
	}
	flushLogBuffer := func() {
		if opts.Silent {
			_, _ = io.Copy(writer, logBuffer)
		}
	}

	ticker := time.NewTicker(opts.FetchInterval)
	defer ticker.Stop()
	for {
		select {
		case err = <-errChan:
			flushLogBuffer()
			return err
		case <-ctx.Done():
			flushLogBuffer()
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
				jobMutex.Unlock()
				flushLogBuffer()
				return err
			}

			output := ""
			switch log.Level {
			case codersdk.LogLevelTrace, codersdk.LogLevelDebug:
				if !opts.Verbose {
					continue
				}
				output = DefaultStyles.Placeholder.Render(log.Output)
			case codersdk.LogLevelError:
				output = DefaultStyles.Error.Render(log.Output)
			case codersdk.LogLevelWarn:
				output = DefaultStyles.Warn.Render(log.Output)
			case codersdk.LogLevelInfo:
				output = log.Output
			}

			jobMutex.Lock()
			if log.Stage != currentStage && log.Stage != "" {
				updateStage(log.Stage, log.CreatedAt)
				jobMutex.Unlock()
				continue
			}
			_, _ = fmt.Fprintf(logOutput, "%s %s\n", DefaultStyles.Placeholder.Render(" "), output)
			if !opts.Silent {
				didLogBetweenStage = true
			}
			jobMutex.Unlock()
		}
	}
}

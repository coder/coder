package cliui_test

import (
	"context"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/serpent"
)

// This cannot be ran in parallel because it uses a signal.
// nolint:tparallel
func TestProvisionerJob(t *testing.T) {
	t.Run("NoLogs", func(t *testing.T) {
		t.Parallel()

		test := newProvisionerJob(t)
		go func() {
			<-test.Next
			test.JobMutex.Lock()
			test.Job.Status = codersdk.ProvisionerJobRunning
			now := dbtime.Now()
			test.Job.StartedAt = &now
			test.JobMutex.Unlock()
			<-test.Next
			test.JobMutex.Lock()
			test.Job.Status = codersdk.ProvisionerJobSucceeded
			now = dbtime.Now()
			test.Job.CompletedAt = &now
			close(test.Logs)
			test.JobMutex.Unlock()
		}()
		test.PTY.ExpectMatch("Queued")
		test.Next <- struct{}{}
		test.PTY.ExpectMatch("Queued")
		test.PTY.ExpectMatch("Running")
		test.Next <- struct{}{}
		test.PTY.ExpectMatch("Running")
	})

	t.Run("Stages", func(t *testing.T) {
		t.Parallel()

		test := newProvisionerJob(t)
		go func() {
			<-test.Next
			test.JobMutex.Lock()
			test.Job.Status = codersdk.ProvisionerJobRunning
			now := dbtime.Now()
			test.Job.StartedAt = &now
			test.Logs <- codersdk.ProvisionerJobLog{
				CreatedAt: dbtime.Now(),
				Stage:     "Something",
			}
			test.JobMutex.Unlock()
			<-test.Next
			test.JobMutex.Lock()
			test.Job.Status = codersdk.ProvisionerJobSucceeded
			now = dbtime.Now()
			test.Job.CompletedAt = &now
			close(test.Logs)
			test.JobMutex.Unlock()
		}()
		test.PTY.ExpectMatch("Queued")
		test.Next <- struct{}{}
		test.PTY.ExpectMatch("Queued")
		test.PTY.ExpectMatch("Something")
		test.Next <- struct{}{}
		test.PTY.ExpectMatch("Something")
	})

	// This cannot be ran in parallel because it uses a signal.
	// nolint:paralleltest
	t.Run("Cancel", func(t *testing.T) {
		t.Skip("This test issues an interrupt signal which will propagate to the test runner.")

		if runtime.GOOS == "windows" {
			// Sending interrupt signal isn't supported on Windows!
			t.SkipNow()
		}

		test := newProvisionerJob(t)
		go func() {
			<-test.Next
			currentProcess, err := os.FindProcess(os.Getpid())
			assert.NoError(t, err)
			err = currentProcess.Signal(os.Interrupt)
			assert.NoError(t, err)
			<-test.Next
			test.JobMutex.Lock()
			test.Job.Status = codersdk.ProvisionerJobCanceled
			now := dbtime.Now()
			test.Job.CompletedAt = &now
			close(test.Logs)
			test.JobMutex.Unlock()
		}()
		test.PTY.ExpectMatch("Queued")
		test.Next <- struct{}{}
		test.PTY.ExpectMatch("Gracefully canceling")
		test.Next <- struct{}{}
		test.PTY.ExpectMatch("Queued")
	})
}

type provisionerJobTest struct {
	Next     chan struct{}
	Job      *codersdk.ProvisionerJob
	JobMutex *sync.Mutex
	Logs     chan codersdk.ProvisionerJobLog
	PTY      *ptytest.PTY
}

func newProvisionerJob(t *testing.T) provisionerJobTest {
	job := &codersdk.ProvisionerJob{
		Status:    codersdk.ProvisionerJobPending,
		CreatedAt: dbtime.Now(),
	}
	jobLock := sync.Mutex{}
	logs := make(chan codersdk.ProvisionerJobLog, 1)
	cmd := &serpent.Cmd{
		Handler: func(inv *serpent.Invocation) error {
			return cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
				FetchInterval: time.Millisecond,
				Fetch: func() (codersdk.ProvisionerJob, error) {
					jobLock.Lock()
					defer jobLock.Unlock()
					return *job, nil
				},
				Cancel: func() error {
					return nil
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
					return logs, closeFunc(func() error {
						return nil
					}), nil
				},
			})
		},
	}
	inv := cmd.Invoke()

	ptty := ptytest.New(t)
	ptty.Attach(inv)
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := inv.WithContext(context.Background()).Run()
		if err != nil {
			assert.ErrorIs(t, err, cliui.Canceled)
		}
	}()
	t.Cleanup(func() {
		<-done
	})
	return provisionerJobTest{
		Next:     make(chan struct{}),
		Job:      job,
		JobMutex: &jobLock,
		Logs:     logs,
		PTY:      ptty,
	}
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}

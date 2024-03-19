package cliui_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/testutil"

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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		testutil.Go(t, func() {
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
		})
		testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
			test.PTY.ExpectMatch(cliui.ProvisioningStateQueued)
			test.Next <- struct{}{}
			test.PTY.ExpectMatch(cliui.ProvisioningStateQueued)
			test.PTY.ExpectMatch(cliui.ProvisioningStateRunning)
			test.Next <- struct{}{}
			test.PTY.ExpectMatch(cliui.ProvisioningStateRunning)
			return true
		}, testutil.IntervalFast)
	})

	t.Run("Stages", func(t *testing.T) {
		t.Parallel()

		test := newProvisionerJob(t)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		testutil.Go(t, func() {
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
		})
		testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
			test.PTY.ExpectMatch(cliui.ProvisioningStateQueued)
			test.Next <- struct{}{}
			test.PTY.ExpectMatch(cliui.ProvisioningStateQueued)
			test.PTY.ExpectMatch("Something")
			test.Next <- struct{}{}
			test.PTY.ExpectMatch("Something")
			return true
		}, testutil.IntervalFast)
	})

	t.Run("Queue Position", func(t *testing.T) {
		t.Parallel()

		stage := cliui.ProvisioningStateQueued

		tests := []struct {
			name     string
			queuePos int
			expected string
		}{
			{
				name:     "first",
				queuePos: 0,
				expected: fmt.Sprintf("%s$", stage),
			},
			{
				name:     "next",
				queuePos: 1,
				expected: fmt.Sprintf(`%s %s$`, stage, regexp.QuoteMeta("(next)")),
			},
			{
				name:     "other",
				queuePos: 4,
				expected: fmt.Sprintf(`%s %s$`, stage, regexp.QuoteMeta("(position: 4)")),
			},
		}

		for _, tc := range tests {
			tc := tc

			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				test := newProvisionerJob(t)
				test.JobMutex.Lock()
				test.Job.QueuePosition = tc.queuePos
				test.Job.QueueSize = tc.queuePos
				test.JobMutex.Unlock()

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cancel()

				testutil.Go(t, func() {
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
				})
				testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
					test.PTY.ExpectRegexMatch(tc.expected)
					test.Next <- struct{}{}
					test.PTY.ExpectMatch(cliui.ProvisioningStateQueued) // step completed
					test.PTY.ExpectMatch(cliui.ProvisioningStateRunning)
					test.Next <- struct{}{}
					test.PTY.ExpectMatch(cliui.ProvisioningStateRunning)
					return true
				}, testutil.IntervalFast)
			})
		}
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

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		testutil.Go(t, func() {
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
		})
		testutil.Eventually(ctx, t, func(ctx context.Context) (done bool) {
			test.PTY.ExpectMatch(cliui.ProvisioningStateQueued)
			test.Next <- struct{}{}
			test.PTY.ExpectMatch("Gracefully canceling")
			test.Next <- struct{}{}
			test.PTY.ExpectMatch(cliui.ProvisioningStateQueued)
			return true
		}, testutil.IntervalFast)
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
	cmd := &serpent.Command{
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

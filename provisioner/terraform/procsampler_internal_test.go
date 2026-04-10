//go:build linux

package terraform

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestExtractProviderName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full provider name",
			input:    "terraform-provider-aws",
			expected: "aws",
		},
		{
			name:     "truncated comm at 15 chars",
			input:    "terraform-provi",
			expected: "terraform-provi",
		},
		{
			name:     "provider with version suffix",
			input:    "terraform-provider-google_v4.0.0_x5",
			expected: "google",
		},
		{
			name:     "bare terraform binary",
			input:    "terraform",
			expected: "terraform",
		},
		{
			name:     "coder provider",
			input:    "terraform-provider-coder",
			expected: "coder",
		},
		{
			name:     "prefix with no provider name",
			input:    "terraform-provider-",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "provider with complex version",
			input:    "terraform-provider-aws_v5.80.0_x5",
			expected: "aws",
		},
		{
			name:     "full path to binary",
			input:    "/usr/local/bin/terraform-provider-docker_v3.0.0",
			expected: "docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractProviderName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSamplerCollectsProcessStats(t *testing.T) {
	t.Parallel()

	// Start a long-lived process in its own process group so we
	// can filter by PGID without picking up unrelated processes.
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		// Kill the whole process group to avoid leaking.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	ctx := testutil.Context(t, testutil.WaitShort)

	sampler := newProcSampler(cmd.Process.Pid, 50*time.Millisecond)
	sampler.Start(ctx)

	// Let the sampler collect a few ticks of data.
	time.Sleep(200 * time.Millisecond)

	summary := sampler.Stop()

	require.NotEmpty(t, summary.Providers, "expected at least one process entry")

	usage, ok := summary.Providers["sleep"]
	require.True(t, ok, "expected entry for 'sleep', got: %v", summary.Providers)
	assert.Greater(t, usage.PeakRSSBytes, uint64(0), "sleep should have non-zero RSS")
}

func TestSamplerHandlesVanishedProcesses(t *testing.T) {
	t.Parallel()

	// Start a very short-lived process that finishes before we
	// sample. This verifies the sampler doesn't panic or error
	// when processes vanish between listing and stat.
	cmd := exec.Command("echo", "hello")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pgid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())

	ctx := testutil.Context(t, testutil.WaitShort)

	sampler := newProcSampler(pgid, 50*time.Millisecond)
	sampler.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	summary := sampler.Stop()

	// The process group is gone, so we expect either empty results
	// or at most a single entry that was caught before exit.
	assert.NotNil(t, summary.Providers, "providers map should never be nil")
}

func TestSamplerSummary(t *testing.T) {
	t.Parallel()

	// Start a CPU-intensive process so we can verify that both
	// peak RSS and CPU time are captured.
	cmd := exec.Command("yes")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Discard stdout to avoid filling pipe buffers.
	cmd.Stdout = nil
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	ctx := testutil.Context(t, testutil.WaitShort)

	sampler := newProcSampler(cmd.Process.Pid, 50*time.Millisecond)
	sampler.Start(ctx)

	// Give the process a moment to accumulate CPU time.
	time.Sleep(300 * time.Millisecond)

	summary := sampler.Stop()

	require.NotEmpty(t, summary.Providers)

	usage, ok := summary.Providers["yes"]
	require.True(t, ok, "expected entry for 'yes', got: %v", summary.Providers)
	assert.Greater(t, usage.PeakRSSBytes, uint64(0), "peak RSS should be non-zero")
	assert.Greater(t, usage.CPUTimeSeconds, float64(0), "CPU time should be non-zero for a busy process")
}

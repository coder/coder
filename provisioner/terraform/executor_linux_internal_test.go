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

func TestExecutorResourceSampling(t *testing.T) {
	t.Parallel()

	// Verify that the executor's sampling integration works
	// end-to-end: start a real process with cmdSysProcAttr(),
	// attach a sampler, and confirm we get back a non-nil
	// ProcessSample with provider data after the process exits.
	cmd := exec.Command("sleep", "0.5")
	cmd.SysProcAttr = cmdSysProcAttr()
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		// Belt-and-suspenders in case the test fails early.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	ctx := testutil.Context(t, testutil.WaitShort)

	sampler := newProcSampler(
		cmd.Process.Pid,
		50*time.Millisecond,
	)
	sampler.Start(ctx)

	// Let some samples accumulate while the process is alive.
	err := cmd.Wait()
	require.NoError(t, err)

	sample := sampler.Stop()

	require.NotNil(t, sample.Providers,
		"providers map should never be nil")

	// The sleep binary should appear as a provider entry keyed
	// by its comm name.
	usage, ok := sample.Providers["sleep"]
	require.True(t, ok,
		"expected 'sleep' in providers, got: %v", sample.Providers)
	assert.Greater(t, usage.PeakRSSBytes, uint64(0),
		"sleep should report non-zero RSS")
}

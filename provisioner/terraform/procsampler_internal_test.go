//go:build linux

package terraform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/procfs"
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

func TestSamplerHandlesVanishedProcesses(t *testing.T) {
	t.Parallel()

	// Start a very short-lived process that finishes before we
	// sample. This verifies the sampler doesn't panic or error
	// when processes vanish between listing and stat.
	cmd := exec.Command("echo", "hello")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())

	ctx := testutil.Context(t, testutil.WaitShort)

	sampler := newProcSampler(pid, 50*time.Millisecond)
	sampler.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	summary := sampler.Stop()

	// The process is gone, so we expect either empty results
	// or at most a single entry that was caught before exit.
	assert.NotNil(t, summary.Providers, "providers map should never be nil")
}

// TestSamplerWithFakeProcfs exercises the sampler's filtering and
// aggregation logic against a synthetic /proc tree. No real
// processes are involved, making this fully deterministic.
func TestSamplerWithFakeProcfs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	const (
		terraformPID = 41
		providerPID  = 42
		unrelatedPID = 99
	)

	// Terraform process — PID matches sampler target, so it is
	// collected regardless of PPID.
	writeFakeProcEntry(t, root, terraformPID, fakeProcEntry{
		stat: fmt.Sprintf(
			"%d (terraform) S 1 %d 1 0 -1 4194304 100 0 0 0 "+
				"250 100 0 0 20 0 1 0 12345 67890 500 "+
				"18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 "+
				"17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			terraformPID, terraformPID),
		status: "Name:\tterraform\n" +
			"VmPeak:\t150 kB\n" +
			"VmSize:\t120 kB\n" +
			"VmHWM:\t100 kB\n" +
			"VmRSS:\t80 kB\n",
	})

	// Provider child — PPID matches terraform, and the comm is
	// truncated to the kernel's 15-char limit. The cmdline file
	// carries the full binary name for fallback resolution.
	writeFakeProcEntry(t, root, providerPID, fakeProcEntry{
		stat: fmt.Sprintf(
			"%d (terraform-provi) S %d %d 1 0 -1 4194304 100 0 0 0 "+
				"150 50 0 0 20 0 1 0 12345 67890 500 "+
				"18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 "+
				"17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			providerPID, terraformPID, providerPID),
		status: "Name:\tterraform-provi\n" +
			"VmPeak:\t300 kB\n" +
			"VmSize:\t250 kB\n" +
			"VmHWM:\t200 kB\n" +
			"VmRSS:\t180 kB\n",
		cmdline: "terraform-provider-aws_v5.0.0_x5\x00",
	})

	// Unrelated process — different PPID, must be filtered out.
	writeFakeProcEntry(t, root, unrelatedPID, fakeProcEntry{
		stat: fmt.Sprintf(
			"%d (nginx) S 1 %d 1 0 -1 4194304 100 0 0 0 "+
				"50 10 0 0 20 0 1 0 12345 67890 500 "+
				"18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 "+
				"17 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
			unrelatedPID, unrelatedPID),
		status: "Name:\tnginx\n" +
			"VmPeak:\t500 kB\n" +
			"VmSize:\t400 kB\n" +
			"VmHWM:\t300 kB\n" +
			"VmRSS:\t250 kB\n",
	})

	fs, err := procfs.NewFS(root)
	require.NoError(t, err)

	s := newProcSampler(terraformPID, time.Second)
	s.fs = fs
	s.sample()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Only the terraform process and its provider child should
	// be collected; the unrelated process must be filtered out.
	assert.Len(t, s.current, 2)
	assert.NotContains(t, s.current, "nginx")

	// --- terraform binary itself ---
	tf, ok := s.current["terraform"]
	require.True(t, ok, "expected 'terraform' entry, got: %v",
		s.current)

	// UTime=250 + STime=100 at userHZ=100 → 3.5 seconds.
	assert.InDelta(t, 3.5, tf.CPUTimeSeconds, 0.01)

	// VmHWM: 100 kB. procfs converts to bytes (100 * 1024 = 102400).
	assert.Equal(t, uint64(100*1024), tf.PeakRSSBytes)

	// --- provider child (comm truncated, resolved via cmdline) ---
	aws, ok := s.current["aws"]
	require.True(t, ok, "expected 'aws' entry, got: %v",
		s.current)

	// UTime=150 + STime=50 at userHZ=100 → 2.0 seconds.
	assert.InDelta(t, 2.0, aws.CPUTimeSeconds, 0.01)

	// VmHWM: 200 kB → 200 * 1024 = 204800 bytes.
	assert.Equal(t, uint64(200*1024), aws.PeakRSSBytes)
}

// fakeProcEntry holds the raw file contents for a single synthetic
// /proc/<pid> directory.
type fakeProcEntry struct {
	stat    string
	status  string
	cmdline string // optional; only needed for truncated comm.
}

// writeFakeProcEntry creates a minimal /proc/<pid> tree that the
// procfs library can parse.
func writeFakeProcEntry(t *testing.T, root string, pid int, entry fakeProcEntry) {
	t.Helper()

	dir := filepath.Join(root, fmt.Sprintf("%d", pid))
	require.NoError(t, os.MkdirAll(dir, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "stat"), []byte(entry.stat), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "status"), []byte(entry.status), 0o600))

	if entry.cmdline != "" {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "cmdline"),
			[]byte(entry.cmdline), 0o600))
	}
}

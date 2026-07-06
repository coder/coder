package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealExecutor_RunOutput(t *testing.T) {
	t.Parallel()
	exec := realExecutor{}
	out, err := exec.RunOutput("echo", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", out)
}

func TestRealExecutor_Run(t *testing.T) {
	t.Parallel()
	exec := realExecutor{}
	err := exec.Run("true")
	require.NoError(t, err)

	err = exec.Run("false")
	require.Error(t, err)
}

func TestRealExecutor_RunMutation(t *testing.T) {
	t.Parallel()
	exec := realExecutor{}
	err := exec.RunMutation("true")
	require.NoError(t, err)

	err = exec.RunMutation("false")
	require.Error(t, err)
}

func TestRealExecutor_RunMutationSurfacesStderr(t *testing.T) {
	t.Parallel()
	exec := realExecutor{}
	err := exec.RunMutation("sh", "-c", "echo 'fatal: boom' 1>&2; exit 1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fatal: boom")
}

func TestRealExecutor_RunMutationStdout(t *testing.T) {
	t.Parallel()
	exec := realExecutor{}
	var stdout, stderr bytes.Buffer
	err := exec.RunMutationStdout(&stdout, &stderr, "echo", "output")
	require.NoError(t, err)
	assert.Equal(t, "output\n", stdout.String())
}

func TestDryRunExecutor_RunOutputDelegates(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exec := newDryRunExecutor(&buf)

	// RunOutput should still execute (read-only commands).
	out, err := exec.RunOutput("echo", "real-output")
	require.NoError(t, err)
	assert.Equal(t, "real-output", out)
	assert.Empty(t, buf.String(), "RunOutput should not produce dry-run output")
}

func TestDryRunExecutor_RunDelegates(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exec := newDryRunExecutor(&buf)

	err := exec.Run("true")
	require.NoError(t, err)
	assert.Empty(t, buf.String(), "Run should not produce dry-run output")
}

func TestDryRunExecutor_RunMutationPrints(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exec := newDryRunExecutor(&buf)

	err := exec.RunMutation("git", "push", "origin", "refs/tags/v2.21.0:refs/tags/v2.21.0")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[dry-run] would run: git push origin refs/tags/v2.21.0:refs/tags/v2.21.0")
}

func TestDryRunExecutor_RunMutationStdoutPrints(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exec := newDryRunExecutor(&buf)

	var stdout, stderr bytes.Buffer
	err := exec.RunMutationStdout(&stdout, &stderr, "gh", "release", "create", "--repo", "coder/coder", "--title", "v2.21.0")
	require.NoError(t, err)

	assert.Empty(t, stdout.String(), "RunMutationStdout should not produce real output in dry-run")
	assert.Contains(t, buf.String(), "[dry-run] would run: gh release create --repo coder/coder --title v2.21.0")
}

func TestDryRunExecutor_RunMutationStdoutQuotesArgs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exec := newDryRunExecutor(&buf)

	var stdout, stderr bytes.Buffer
	err := exec.RunMutationStdout(&stdout, &stderr, "gh", "release", "create", "--title", "has space")
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "'has space'")
}

func TestShelljoin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"simple", []string{"a", "b"}, "a b"},
		{"space", []string{"a", "has space"}, "a 'has space'"},
		// shelljoin uses POSIX single-quote escaping, so an embedded
		// single quote becomes '"'"'.
		{"quote", []string{"it's"}, `'it'"'"'s'`},
		{"empty", []string{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shelljoin(tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor records mutating calls and delegates read-only calls
// to configurable functions.
type mockExecutor struct {
	// MutationCalls records all calls to RunMutation and
	// RunMutationStdout as "name arg1 arg2 ..." strings.
	MutationCalls []string

	// RunOutputFunc is called for RunOutput. If nil, returns ("", error).
	RunOutputFunc func(name string, args ...string) (string, error)

	// RunFunc is called for Run. If nil, returns nil.
	RunFunc func(name string, args ...string) error
}

func (m *mockExecutor) RunOutput(name string, args ...string) (string, error) {
	if m.RunOutputFunc != nil {
		return m.RunOutputFunc(name, args...)
	}
	return "", nil
}

func (m *mockExecutor) Run(name string, args ...string) error {
	if m.RunFunc != nil {
		return m.RunFunc(name, args...)
	}
	return nil
}

func (m *mockExecutor) RunMutation(name string, args ...string) error {
	call := name
	for _, a := range args {
		call += " " + a
	}
	m.MutationCalls = append(m.MutationCalls, call)
	return nil
}

func (m *mockExecutor) RunMutationStdout(_, _ io.Writer, name string, args ...string) error {
	call := name
	for _, a := range args {
		call += " " + a
	}
	m.MutationCalls = append(m.MutationCalls, call)
	return nil
}

func TestCreateAndPushTag_New(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		RunOutputFunc: func(name string, args ...string) (string, error) {
			// Simulate tag not existing: git rev-parse --verify fails.
			if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
				return "", assert.AnError
			}
			return "", nil
		},
	}

	err := createAndPushTag(mock, "v2.21.0", "abc123")
	require.NoError(t, err)

	require.Len(t, mock.MutationCalls, 2)
	assert.Contains(t, mock.MutationCalls[0], "git tag -a v2.21.0 -m Release v2.21.0 abc123")
	assert.Contains(t, mock.MutationCalls[1], "git push origin refs/tags/v2.21.0:refs/tags/v2.21.0")
}

func TestCreateAndPushTag_AlreadyExistsMatching(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		RunOutputFunc: func(name string, args ...string) (string, error) {
			// Simulate tag existing at the correct commit.
			if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
				return "abc123", nil
			}
			return "", nil
		},
	}

	err := createAndPushTag(mock, "v2.21.0", "abc123")
	require.NoError(t, err)

	// No mutations should happen.
	assert.Empty(t, mock.MutationCalls)
}

func TestCreateAndPushTag_AlreadyExistsMismatch(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		RunOutputFunc: func(name string, args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
				return "different_sha", nil
			}
			return "", nil
		},
	}

	err := createAndPushTag(mock, "v2.21.0", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists at different_sha")
	assert.Empty(t, mock.MutationCalls)
}

func TestCreateAndPushBranch_New(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		RunOutputFunc: func(name string, args ...string) (string, error) {
			// Simulate branch not existing: ls-remote fails.
			if len(args) >= 1 && args[0] == "ls-remote" {
				return "", assert.AnError
			}
			return "", nil
		},
	}

	err := createAndPushBranch(mock, "release/2.21", "abc123")
	require.NoError(t, err)

	require.Len(t, mock.MutationCalls, 1)
	assert.Contains(t, mock.MutationCalls[0], "git push origin abc123:refs/heads/release/2.21")
}

func TestCreateAndPushBranch_AlreadyExistsMatching(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		RunOutputFunc: func(name string, args ...string) (string, error) {
			if len(args) >= 1 && args[0] == "ls-remote" {
				return "abc123\trefs/heads/release/2.21", nil
			}
			return "", nil
		},
	}

	err := createAndPushBranch(mock, "release/2.21", "abc123")
	require.NoError(t, err)
	assert.Empty(t, mock.MutationCalls)
}

func TestCreateAndPushBranch_AlreadyExistsMismatch(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		RunOutputFunc: func(name string, args ...string) (string, error) {
			if len(args) >= 1 && args[0] == "ls-remote" {
				return "other_sha\trefs/heads/release/2.21", nil
			}
			return "", nil
		},
	}

	err := createAndPushBranch(mock, "release/2.21", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists at other_sha")
	assert.Empty(t, mock.MutationCalls)
}

func TestDryRunExecutor_SkipsMutations(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exec := newDryRunExecutor(&buf)

	// RunMutation should print, not execute.
	err := exec.RunMutation("git", "tag", "-a", "v2.21.0", "-m", "Release v2.21.0", "abc123")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[dry-run] would run: git tag -a v2.21.0")

	buf.Reset()

	err = exec.RunMutation("git", "push", "origin", "refs/tags/v2.21.0:refs/tags/v2.21.0")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[dry-run] would run: git push origin refs/tags/v2.21.0:refs/tags/v2.21.0")
}

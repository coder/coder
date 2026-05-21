package agentcontainers_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
)

// TestIntegrationDockerCLI tests the DetectArchitecture, Copy, and
// ExecAs methods using a real Docker container. All tests share a
// single container to avoid setup overhead.
//
// Run manually with: CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestIntegrationDockerCLI
//
//nolint:tparallel,paralleltest // Docker integration tests don't run in parallel to avoid flakiness.
func TestIntegrationDockerCLI(t *testing.T) {
	if ctud, ok := os.LookupEnv("CODER_TEST_USE_DOCKER"); !ok || ctud != "1" {
		t.Skip("Set CODER_TEST_USE_DOCKER=1 to run this test")
	}

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to docker")

	// Start a simple busybox container for all subtests to share.
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "busybox",
		Tag:        "latest",
		Cmd:        []string{"sleep", "infinity"},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start test docker container")
	t.Logf("Created container %q", ct.Container.Name)
	t.Cleanup(func() {
		assert.NoError(t, pool.Purge(ct), "Could not purge resource %q", ct.Container.Name)
		t.Logf("Purged container %q", ct.Container.Name)
	})

	// Wait for container to start.
	require.Eventually(t, func() bool {
		ct, ok := pool.ContainerByName(ct.Container.Name)
		return ok && ct.Container.State.Running
	}, testutil.WaitShort, testutil.IntervalSlow, "Container did not start in time")

	dcli := agentcontainers.NewDockerCLI(agentexec.DefaultExecer)
	containerName := strings.TrimPrefix(ct.Container.Name, "/")

	t.Run("DetectArchitecture", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		arch, err := dcli.DetectArchitecture(ctx, containerName)
		require.NoError(t, err, "DetectArchitecture failed")
		require.NotEmpty(t, arch, "arch has no content")
		require.Equal(t, runtime.GOARCH, arch, "architecture does not match runtime, did you run this test with a remote Docker socket?")

		t.Logf("Detected architecture: %s", arch)
	})

	t.Run("Copy", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		want := "Help, I'm trapped!"
		tempFile := filepath.Join(t.TempDir(), "test-file.txt")
		err := os.WriteFile(tempFile, []byte(want), 0o600)
		require.NoError(t, err, "create test file failed")

		destPath := "/tmp/copied-file.txt"
		err = dcli.Copy(ctx, containerName, tempFile, destPath)
		require.NoError(t, err, "Copy failed")

		got, err := dcli.ExecAs(ctx, containerName, "", "cat", destPath)
		require.NoError(t, err, "ExecAs failed after Copy")
		require.Equal(t, want, string(got), "copied file content did not match original")

		t.Logf("Successfully copied file from %s to container %s:%s", tempFile, containerName, destPath)
	})

	t.Run("ExecAs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Test ExecAs without specifying user (should use container's default).
		want := "root"
		got, err := dcli.ExecAs(ctx, containerName, "", "whoami")
		require.NoError(t, err, "ExecAs without user should succeed")
		require.Equal(t, want, string(got), "ExecAs without user should output expected string")

		// Test ExecAs with numeric UID (non root).
		want = "1000"
		_, err = dcli.ExecAs(ctx, containerName, want, "whoami")
		require.Error(t, err, "ExecAs with UID 1000 should fail as user does not exist in busybox")
		require.Contains(t, err.Error(), "whoami: unknown uid 1000", "ExecAs with UID 1000 should return 'unknown uid' error")

		// Test ExecAs with root user (should convert "root" to "0", which still outputs root due to passwd).
		want = "root"
		got, err = dcli.ExecAs(ctx, containerName, "root", "whoami")
		require.NoError(t, err, "ExecAs with root user should succeed")
		require.Equal(t, want, string(got), "ExecAs with root user should output expected string")

		// Test ExecAs with numeric UID.
		want = "root"
		got, err = dcli.ExecAs(ctx, containerName, "0", "whoami")
		require.NoError(t, err, "ExecAs with UID 0 should succeed")
		require.Equal(t, want, string(got), "ExecAs with UID 0 should output expected string")

		// Test ExecAs with multiple arguments.
		want = "multiple args test"
		got, err = dcli.ExecAs(ctx, containerName, "", "sh", "-c", "echo '"+want+"'")
		require.NoError(t, err, "ExecAs with multiple arguments should succeed")
		require.Equal(t, want, string(got), "ExecAs with multiple arguments should output expected string")

		t.Logf("Successfully executed commands in container %s", containerName)
	})
}

// TestIntegrationDockerCLIStop tests the Stop method using a real
// Docker container.
//
// Run manually with: CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestIntegrationDockerCLIStop
//
//nolint:tparallel,paralleltest // Docker integration tests don't run in parallel to avoid flakiness.
func TestIntegrationDockerCLIStop(t *testing.T) {
	if os.Getenv("CODER_TEST_USE_DOCKER") != "1" {
		t.Skip("Set CODER_TEST_USE_DOCKER=1 to run this test")
	}

	ctx := testutil.Context(t, testutil.WaitLong)

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to docker")

	// Given: A simple busybox container
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "busybox",
		Tag:        "latest",
		Cmd:        []string{"sleep", "infinity"},
	}, func(config *docker.HostConfig) {
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start test docker container")
	t.Logf("Created container %q", ct.Container.Name)
	t.Cleanup(func() {
		assert.NoError(t, pool.Purge(ct), "Could not purge resource %q", ct.Container.Name)
		t.Logf("Purged container %q", ct.Container.Name)
	})

	// Given: The container is running
	require.Eventually(t, func() bool {
		ct, ok := pool.ContainerByName(ct.Container.Name)
		return ok && ct.Container.State.Running
	}, testutil.WaitShort, testutil.IntervalSlow, "Container did not start in time")

	dcli := agentcontainers.NewDockerCLI(agentexec.DefaultExecer)
	containerName := strings.TrimPrefix(ct.Container.Name, "/")

	// When: We attempt to stop the container
	err = dcli.Stop(ctx, containerName)
	require.NoError(t, err)

	// Then: We expect the container to be stopped.
	ct, ok := pool.ContainerByName(ct.Container.Name)
	require.True(t, ok)
	require.False(t, ct.Container.State.Running)
	require.Equal(t, "exited", ct.Container.State.Status)
}

// TestIntegrationDockerCLIRemove tests the Remove method using a real
// Docker container.
//
// Run manually with: CODER_TEST_USE_DOCKER=1 go test ./agent/agentcontainers -run TestIntegrationDockerCLIRemove
//
//nolint:tparallel,paralleltest // Docker integration tests don't run in parallel to avoid flakiness.
func TestIntegrationDockerCLIRemove(t *testing.T) {
	if os.Getenv("CODER_TEST_USE_DOCKER") != "1" {
		t.Skip("Set CODER_TEST_USE_DOCKER=1 to run this test")
	}

	ctx := testutil.Context(t, testutil.WaitLong)

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to docker")

	// Given: A simple busybox container that exits immediately.
	ct, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "busybox",
		Tag:        "latest",
		Cmd:        []string{"true"},
	}, func(config *docker.HostConfig) {
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start test docker container")
	t.Logf("Created container %q", ct.Container.Name)
	containerName := strings.TrimPrefix(ct.Container.Name, "/")

	// Wait for the container to exit.
	require.Eventually(t, func() bool {
		ct, ok := pool.ContainerByName(ct.Container.Name)
		return ok && !ct.Container.State.Running
	}, testutil.WaitShort, testutil.IntervalSlow, "Container did not stop in time")

	dcli := agentcontainers.NewDockerCLI(agentexec.DefaultExecer)

	// When: We attempt to remove the container.
	err = dcli.Remove(ctx, containerName)
	require.NoError(t, err)

	// Then: We expect the container to be removed.
	_, ok := pool.ContainerByName(ct.Container.Name)
	require.False(t, ok, "Container should be removed")
}

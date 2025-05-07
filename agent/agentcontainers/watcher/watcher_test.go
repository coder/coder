package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers/watcher"
	"github.com/coder/coder/v2/testutil"
)

func TestFSNotifyWatcher(t *testing.T) {
	t.Parallel()

	// Create test files.
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.json")
	err := os.WriteFile(testFile, []byte(`{"test": "initial"}`), 0o600)
	require.NoError(t, err, "create test file failed")

	// Create the watcher under test.
	wut, err := watcher.NewFSNotify()
	require.NoError(t, err, "create FSNotify watcher failed")
	defer wut.Close()

	// Add the test file to the watch list.
	err = wut.Add(testFile)
	require.NoError(t, err, "add file to watcher failed")

	ctx := testutil.Context(t, testutil.WaitShort)

	// Modify the test file to trigger an event.
	err = os.WriteFile(testFile, []byte(`{"test": "modified"}`), 0o600)
	require.NoError(t, err, "modify test file failed")

	// Verify that we receive the event we want.
	for {
		event, err := wut.Next(ctx)
		require.NoError(t, err, "next event failed")

		require.NotNil(t, event, "want non-nil event")
		if !event.Has(fsnotify.Write) {
			t.Logf("Ignoring event: %s", event)
			continue
		}
		require.Truef(t, event.Has(fsnotify.Write), "want write event: %s", event.String())
		require.Equal(t, event.Name, testFile, "want event for test file")
		break
	}

	// Rename the test file to trigger a rename event.
	err = os.Rename(testFile, testFile+".bak")
	require.NoError(t, err, "rename test file failed")

	// Verify that we receive the event we want.
	for {
		event, err := wut.Next(ctx)
		require.NoError(t, err, "next event failed")
		require.NotNil(t, event, "want non-nil event")
		if !event.Has(fsnotify.Rename) {
			t.Logf("Ignoring event: %s", event)
			continue
		}
		require.Truef(t, event.Has(fsnotify.Rename), "want rename event: %s", event.String())
		require.Equal(t, event.Name, testFile, "want event for test file")
		break
	}

	err = os.WriteFile(testFile, []byte(`{"test": "new"}`), 0o600)
	require.NoError(t, err, "write new test file failed")

	// Verify that we receive the event we want.
	for {
		event, err := wut.Next(ctx)
		require.NoError(t, err, "next event failed")
		require.NotNil(t, event, "want non-nil event")
		if !event.Has(fsnotify.Create) {
			t.Logf("Ignoring event: %s", event)
			continue
		}
		require.Truef(t, event.Has(fsnotify.Create), "want create event: %s", event.String())
		require.Equal(t, event.Name, testFile, "want event for test file")
		break
	}

	err = os.WriteFile(testFile+".atomic", []byte(`{"test": "atomic"}`), 0o600)
	require.NoError(t, err, "write new atomic test file failed")

	err = os.Rename(testFile+".atomic", testFile)
	require.NoError(t, err, "rename atomic test file failed")

	// Verify that we receive the event we want.
	for {
		event, err := wut.Next(ctx)
		require.NoError(t, err, "next event failed")
		require.NotNil(t, event, "want non-nil event")
		if !event.Has(fsnotify.Create) {
			t.Logf("Ignoring event: %s", event)
			continue
		}
		require.Truef(t, event.Has(fsnotify.Create), "want create event: %s", event.String())
		require.Equal(t, event.Name, testFile, "want event for test file")
		break
	}

	// Test removing the file from the watcher.
	err = wut.Remove(testFile)
	require.NoError(t, err, "remove file from watcher failed")
}

func TestFSNotifyWatcher_CloseBeforeNext(t *testing.T) {
	t.Parallel()

	wut, err := watcher.NewFSNotify()
	require.NoError(t, err, "create FSNotify watcher failed")

	err = wut.Close()
	require.NoError(t, err, "close watcher failed")

	ctx := context.Background()
	_, err = wut.Next(ctx)
	assert.Error(t, err, "want Next to return error when watcher is closed")
}

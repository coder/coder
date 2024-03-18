package clilog_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("NoConfiguration", func(t *testing.T) {
		t.Parallel()

		cmd := &serpent.Command{
			Use:     "test",
			Handler: testHandler(t),
		}
		err := cmd.Invoke().Run()
		require.ErrorContains(t, err, "no loggers provided, use /dev/null to disable logging")
	})

	t.Run("Verbose", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &serpent.Command{
			Use: "test",
			Handler: testHandler(t,
				clilog.WithHuman(tempFile),
				clilog.WithVerbose(),
			),
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)
		assertLogs(t, tempFile, debugLog, infoLog, warnLog, filterLog)
	})

	t.Run("WithFilter", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &serpent.Command{
			Use: "test",
			Handler: testHandler(t,
				clilog.WithHuman(tempFile),
				// clilog.WithVerbose(), // implicit
				clilog.WithFilter("important debug message"),
			),
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)
		assertLogs(t, tempFile, infoLog, warnLog, filterLog)
	})

	t.Run("WithHuman", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &serpent.Command{
			Use:     "test",
			Handler: testHandler(t, clilog.WithHuman(tempFile)),
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)
		assertLogs(t, tempFile, infoLog, warnLog)
	})

	t.Run("WithJSON", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &serpent.Command{
			Use:     "test",
			Handler: testHandler(t, clilog.WithJSON(tempFile), clilog.WithVerbose()),
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)
		assertLogsJSON(t, tempFile, debug, debugLog, info, infoLog, warn, warnLog, debug, filterLog)
	})

	t.Run("FromDeploymentValues", func(t *testing.T) {
		t.Parallel()

		t.Run("Defaults", func(t *testing.T) {
			stdoutPath := filepath.Join(t.TempDir(), "stdout")
			stderrPath := filepath.Join(t.TempDir(), "stderr")

			stdout, err := os.OpenFile(stdoutPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
			require.NoError(t, err)
			t.Cleanup(func() { _ = stdout.Close() })

			stderr, err := os.OpenFile(stderrPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
			require.NoError(t, err)
			t.Cleanup(func() { _ = stderr.Close() })

			// Use the default deployment values.
			dv := coderdtest.DeploymentValues(t)
			cmd := &serpent.Command{
				Use:     "test",
				Handler: testHandler(t, clilog.FromDeploymentValues(dv)),
			}
			inv := cmd.Invoke()
			inv.Stdout = stdout
			inv.Stderr = stderr
			err = inv.Run()
			require.NoError(t, err)

			assertLogs(t, stdoutPath, "")
			assertLogs(t, stderrPath, infoLog, warnLog)
		})

		t.Run("Override", func(t *testing.T) {
			tempFile := filepath.Join(t.TempDir(), "test.log")
			tempJSON := filepath.Join(t.TempDir(), "test.json")
			dv := &codersdk.DeploymentValues{
				Logging: codersdk.LoggingConfig{
					Filter: []string{"foo", "baz"},
					Human:  serpent.String(tempFile),
					JSON:   serpent.String(tempJSON),
				},
				Verbose: true,
				Trace: codersdk.TraceConfig{
					Enable: true,
				},
			}
			cmd := &serpent.Command{
				Use:     "test",
				Handler: testHandler(t, clilog.FromDeploymentValues(dv)),
			}
			err := cmd.Invoke().Run()
			require.NoError(t, err)
			assertLogs(t, tempFile, infoLog, warnLog)
			assertLogsJSON(t, tempJSON, info, infoLog, warn, warnLog)
		})
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "doesnotexist", "test.log")
		cmd := &serpent.Command{
			Use: "test",
			Handler: func(inv *serpent.Invocation) error {
				logger, closeLog, err := clilog.New(
					clilog.WithFilter("foo", "baz"),
					clilog.WithHuman(tempFile),
					clilog.WithVerbose(),
				).Build(inv)
				if err != nil {
					return err
				}
				defer closeLog()
				logger.Error(inv.Context(), "you will never see this")
				return nil
			},
		}
		err := cmd.Invoke().Run()
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

var (
	debug     = "DEBUG"
	info      = "INFO"
	warn      = "WARN"
	debugLog  = "this is a debug message"
	infoLog   = "this is an info message"
	warnLog   = "this is a warning message"
	filterLog = "this is an important debug message you want to see"
)

func testHandler(t testing.TB, opts ...clilog.Option) serpent.HandlerFunc {
	t.Helper()

	return func(inv *serpent.Invocation) error {
		logger, closeLog, err := clilog.New(opts...).Build(inv)
		if err != nil {
			return err
		}
		defer closeLog()
		logger.Debug(inv.Context(), debugLog)
		logger.Info(inv.Context(), infoLog)
		logger.Warn(inv.Context(), warnLog)
		logger.Debug(inv.Context(), filterLog)
		return nil
	}
}

func assertLogs(t testing.TB, path string, expected ...string) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	logs := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !assert.Len(t, logs, len(expected)) {
		t.Logf(string(data))
		t.FailNow()
	}
	for i, log := range logs {
		require.Contains(t, log, expected[i])
	}
}

func assertLogsJSON(t testing.TB, path string, levelExpected ...string) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	if len(levelExpected)%2 != 0 {
		t.Errorf("levelExpected must be a list of level-message pairs")
		return
	}

	logs := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !assert.Len(t, logs, len(levelExpected)/2) {
		t.Logf(string(data))
		t.FailNow()
	}
	for i, log := range logs {
		var entry struct {
			Level   string `json:"level"`
			Message string `json:"msg"`
		}
		err := json.NewDecoder(strings.NewReader(log)).Decode(&entry)
		require.NoError(t, err)
		require.Equal(t, levelExpected[2*i], entry.Level)
		require.Equal(t, levelExpected[2*i+1], entry.Message)
	}
}

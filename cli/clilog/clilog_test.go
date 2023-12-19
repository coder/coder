package clilog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/codersdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("WithFilter", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &clibase.Cmd{
			Use: "test",
			Handler: func(inv *clibase.Invocation) error {
				logger, closeLog, err := clilog.New(
					clilog.WithFilter("foo", "baz"),
					clilog.WithHuman(tempFile),
					clilog.WithVerbose(),
				).Build(inv)
				if err != nil {
					return err
				}
				defer closeLog()
				logger.Debug(inv.Context(), "foo is not a useful message")
				logger.Debug(inv.Context(), "bar is also not a useful message")
				return nil
			},
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)

		data, err := os.ReadFile(tempFile)
		require.NoError(t, err)
		logs := strings.Split(strings.TrimSpace(string(data)), "\n")
		if !assert.Len(t, logs, 1) {
			t.Logf(string(data))
			t.FailNow()
		}
		require.Contains(t, logs[0], "foo is not a useful message")
	})

	t.Run("WithHuman", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &clibase.Cmd{
			Use: "test",
			Handler: func(inv *clibase.Invocation) error {
				logger, closeLog, err := clilog.New(
					clilog.WithHuman(tempFile)).
					Build(inv)
				if err != nil {
					return err
				}
				defer closeLog()
				logger.Debug(inv.Context(), "foo is not a useful message")
				logger.Info(inv.Context(), "bar is also not a useful message")
				return nil
			},
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)

		data, err := os.ReadFile(tempFile)
		require.NoError(t, err)
		logs := strings.Split(strings.TrimSpace(string(data)), "\n")
		if !assert.Len(t, logs, 1) {
			t.Logf(string(data))
			t.FailNow()
		}
		require.Contains(t, logs[0], "bar is also not a useful message")
	})

	t.Run("WithJSON", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		cmd := &clibase.Cmd{
			Use: "test",
			Handler: func(inv *clibase.Invocation) error {
				logger, closeLog, err := clilog.New(
					clilog.WithJSON(tempFile)).
					Build(inv)
				if err != nil {
					return err
				}
				defer closeLog()
				logger.Debug(inv.Context(), "foo is not a useful message")
				logger.Info(inv.Context(), "bar is also not a useful message")
				return nil
			},
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)

		data, err := os.ReadFile(tempFile)
		require.NoError(t, err)
		logs := strings.Split(strings.TrimSpace(string(data)), "\n")
		if !assert.Len(t, logs, 1) {
			t.Logf(string(data))
			t.FailNow()
		}
		require.Contains(t, logs[0], "bar")
		var entry struct {
			Level   string `json:"level"`
			Message string `json:"msg"`
		}

		err = json.NewDecoder(strings.NewReader(logs[0])).Decode(&entry)
		require.NoError(t, err)
		require.Equal(t, "INFO", entry.Level)
		require.Equal(t, "bar is also not a useful message", entry.Message)
	})

	t.Run("FromDeploymentValues", func(t *testing.T) {
		t.Parallel()

		tempFile := filepath.Join(t.TempDir(), "test.log")
		tempJSON := filepath.Join(t.TempDir(), "test.json")
		dv := &codersdk.DeploymentValues{
			Logging: codersdk.LoggingConfig{
				Filter: []string{"foo", "baz"},
				Human:  clibase.String(tempFile),
				JSON:   clibase.String(tempJSON),
			},
			Verbose: true,
			Trace: codersdk.TraceConfig{
				Enable: true,
			},
		}
		cmd := &clibase.Cmd{
			Use: "test",
			Handler: func(inv *clibase.Invocation) error {
				logger, closeLog, err := clilog.New(clilog.FromDeploymentValues(dv)).
					Build(inv)
				if err != nil {
					return err
				}
				defer closeLog()
				logger.Debug(inv.Context(), "foo is not a useful message")
				logger.Info(inv.Context(), "bar is also not a useful message")
				return nil
			},
		}
		err := cmd.Invoke().Run()
		require.NoError(t, err)

		data, err := os.ReadFile(tempFile)
		require.NoError(t, err)
		logs := strings.Split(strings.TrimSpace(string(data)), "\n")
		if !assert.Len(t, logs, 2) {
			t.Logf(string(data))
			t.FailNow()
		}
		require.Contains(t, logs[0], "foo is not a useful message")
		require.Contains(t, logs[1], "bar is also not a useful message")

		data, err = os.ReadFile(tempJSON)
		require.NoError(t, err)
		logs = strings.Split(strings.TrimSpace(string(data)), "\n")
		if !assert.Len(t, logs, 2) {
			t.Logf(string(data))
			t.FailNow()
		}
		require.Contains(t, logs[0], "foo is not a useful message")
		require.Contains(t, logs[1], "bar is also not a useful message")
	})
}

package terraform

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type mockLogger struct {
	logs []*proto.Log
}

var _ logSink = &mockLogger{}

func (m *mockLogger) ProvisionLog(l proto.LogLevel, o string) {
	m.logs = append(m.logs, &proto.Log{Level: l, Output: o})
}

func TestLogWriter_Mainline(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

	_, err := writer.Write([]byte(`Sitting in an English garden
Waiting for the sun
If the sun don't come you get a tan
From standing in the English rain`))
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	<-doneLogging

	expected := []*proto.Log{
		{Level: proto.LogLevel_INFO, Output: "Sitting in an English garden"},
		{Level: proto.LogLevel_INFO, Output: "Waiting for the sun"},
		{Level: proto.LogLevel_INFO, Output: "If the sun don't come you get a tan"},
		{Level: proto.LogLevel_INFO, Output: "From standing in the English rain"},
	}
	require.Equal(t, expected, logr.logs)
}

func TestOnlyDataResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stateMod *tfjson.StateModule
		expected *tfjson.StateModule
	}{
		{
			name:     "empty state module",
			stateMod: &tfjson.StateModule{},
			expected: &tfjson.StateModule{},
		},
		{
			name: "only data resources",
			stateMod: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{Name: "cat", Type: "coder_parameter", Mode: "data", Address: "cat-address"},
					{Name: "cow", Type: "foobaz", Mode: "data", Address: "cow-address"},
				},
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							{Name: "child-cat", Type: "coder_parameter", Mode: "data", Address: "child-cat-address"},
							{Name: "child-dog", Type: "foobar", Mode: "data", Address: "child-dog-address"},
						},
						Address: "child-module-1",
					},
				},
				Address: "fake-module",
			},
			expected: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{Name: "cat", Type: "coder_parameter", Mode: "data", Address: "cat-address"},
					{Name: "cow", Type: "foobaz", Mode: "data", Address: "cow-address"},
				},
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							{Name: "child-cat", Type: "coder_parameter", Mode: "data", Address: "child-cat-address"},
							{Name: "child-dog", Type: "foobar", Mode: "data", Address: "child-dog-address"},
						},
						Address: "child-module-1",
					},
				},
				Address: "fake-module",
			},
		},
		{
			name: "only non-data resources",
			stateMod: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{Name: "cat", Type: "coder_parameter", Mode: "foobar", Address: "cat-address"},
					{Name: "cow", Type: "foobaz", Mode: "foo", Address: "cow-address"},
				},
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							{Name: "child-cat", Type: "coder_parameter", Mode: "foobar", Address: "child-cat-address"},
							{Name: "child-dog", Type: "foobar", Mode: "foobaz", Address: "child-dog-address"},
						},
						Address: "child-module-1",
					},
				},
				Address: "fake-module",
			},
			expected: &tfjson.StateModule{
				Address: "fake-module",
				ChildModules: []*tfjson.StateModule{
					{Address: "child-module-1"},
				},
			},
		},
		{
			name: "mixed resources",
			stateMod: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{Name: "cat", Type: "coder_parameter", Mode: "data", Address: "cat-address"},
					{Name: "dog", Type: "foobar", Mode: "magic", Address: "dog-address"},
					{Name: "cow", Type: "foobaz", Mode: "data", Address: "cow-address"},
				},
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							{Name: "child-cat", Type: "coder_parameter", Mode: "data", Address: "child-cat-address"},
							{Name: "child-dog", Type: "foobar", Mode: "data", Address: "child-dog-address"},
							{Name: "child-cow", Type: "foobaz", Mode: "magic", Address: "child-cow-address"},
						},
						Address: "child-module-1",
					},
				},
				Address: "fake-module",
			},
			expected: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{Name: "cat", Type: "coder_parameter", Mode: "data", Address: "cat-address"},
					{Name: "cow", Type: "foobaz", Mode: "data", Address: "cow-address"},
				},
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							{Name: "child-cat", Type: "coder_parameter", Mode: "data", Address: "child-cat-address"},
							{Name: "child-dog", Type: "foobar", Mode: "data", Address: "child-dog-address"},
						},
						Address: "child-module-1",
					},
				},
				Address: "fake-module",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filtered := onlyDataResources(*tt.stateMod)

			expected, err := json.Marshal(tt.expected)
			require.NoError(t, err)
			got, err := json.Marshal(filtered)
			require.NoError(t, err)

			require.Equal(t, string(expected), string(got))
		})
	}
}

func TestChecksumFileCRC32(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := testutil.Logger(t)

		tmpfile, err := os.CreateTemp("", "lockfile-*.hcl")
		require.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		content := []byte("provider \"aws\" { version = \"5.0.0\" }")
		_, err = tmpfile.Write(content)
		require.NoError(t, err)
		tmpfile.Close()

		// Calculate checksum - expected value for this specific content
		expectedChecksum := uint32(0x08f39f51)
		checksum := checksumFileCRC32(ctx, logger, tmpfile.Name())
		require.Equal(t, expectedChecksum, checksum)

		// Modify file
		err = os.WriteFile(tmpfile.Name(), []byte("modified content"), 0o600)
		require.NoError(t, err)

		// Checksum should be different
		modifiedChecksum := checksumFileCRC32(ctx, logger, tmpfile.Name())
		require.NotEqual(t, expectedChecksum, modifiedChecksum)
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := testutil.Logger(t)

		checksum := checksumFileCRC32(ctx, logger, "/nonexistent/file.hcl")
		require.Zero(t, checksum)
	})
}

func TestLargeLogLines(t *testing.T) {
	t.Parallel()

	// writeLines writes all lines to w in a goroutine and reports
	// success on the returned channel. This is necessary because
	// io.Pipe writes block until the reader consumes the data.
	writeLines := func(w io.WriteCloser, lines ...string) <-chan error {
		ch := make(chan error, 1)
		go func() {
			defer w.Close()
			for _, line := range lines {
				if _, err := io.WriteString(w, line+"\n"); err != nil {
					ch <- err
					return
				}
			}
			ch <- nil
		}()
		return ch
	}

	t.Run("line exceeds old 64KiB default", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		logr := &mockLogger{}
		writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

		// 128 KiB line: would exceed the old default 64 KiB limit
		// but fits within the new 4 MiB limit.
		largeLine := strings.Repeat("x", 128*1024)
		writeErr := writeLines(writer, "before", largeLine, "after")

		select {
		case err := <-writeErr:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("timed out writing; provisioner would hang")
		}

		select {
		case <-doneLogging:
		case <-ctx.Done():
			t.Fatal("timed out waiting for log processing")
		}

		// All three lines should be logged normally.
		require.Len(t, logr.logs, 3)
		require.Equal(t, "before", logr.logs[0].Output)
		require.Equal(t, largeLine, logr.logs[1].Output)
		require.Equal(t, "after", logr.logs[2].Output)
	})

	t.Run("line exceeds max 4MiB buffer", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		logr := &mockLogger{}
		writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

		// 5 MiB line: exceeds even the new 4 MiB limit.
		largeLine := strings.Repeat("x", 5*1024*1024)
		writeErr := writeLines(writer, "before", largeLine, "after")

		select {
		case err := <-writeErr:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("timed out writing; provisioner would hang")
		}

		select {
		case <-doneLogging:
		case <-ctx.Done():
			t.Fatal("timed out waiting for log processing")
		}

		// "before" and "after" should still be logged.
		var outputs []string
		for _, log := range logr.logs {
			outputs = append(outputs, log.Output)
		}
		require.Contains(t, outputs, "before")
		require.Contains(t, outputs, "after")

		// The oversized line should have produced a warning.
		hasWarning := false
		for _, log := range logr.logs {
			if log.Level == proto.LogLevel_WARN &&
				strings.Contains(log.Output, "too long") {
				hasWarning = true
				break
			}
		}
		require.True(t, hasWarning,
			"expected a WARN log about oversized line")
	})

	t.Run("shared mutex does not wedge stderr", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		stdoutLogr := &mockLogger{}
		stderrLogr := &mockLogger{}

		stdoutW, doneOut := logWriter(stdoutLogr, proto.LogLevel_INFO)
		stderrW, doneErr := logWriter(stderrLogr, proto.LogLevel_ERROR)

		// Write a normal line to each stream.
		go func() {
			defer stdoutW.Close()
			_, _ = fmt.Fprintln(stdoutW, "stdout line")
		}()
		go func() {
			defer stderrW.Close()
			_, _ = fmt.Fprintln(stderrW, "stderr line")
		}()

		select {
		case <-doneOut:
		case <-ctx.Done():
			t.Fatal("stdout log processing hung")
		}
		select {
		case <-doneErr:
		case <-ctx.Done():
			t.Fatal("stderr log processing hung")
		}

		require.Len(t, stdoutLogr.logs, 1)
		require.Equal(t, "stdout line", stdoutLogr.logs[0].Output)
		require.Len(t, stderrLogr.logs, 1)
		require.Equal(t, "stderr line", stderrLogr.logs[0].Output)
	})
}

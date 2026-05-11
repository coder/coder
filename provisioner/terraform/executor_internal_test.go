package terraform

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type mockLogger struct {
	mu   sync.Mutex
	logs []*proto.Log
}

var _ logSink = &mockLogger{}

func (m *mockLogger) ProvisionLog(l proto.LogLevel, o string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, &proto.Log{Level: l, Output: o})
}

func (m *mockLogger) snapshot() []*proto.Log {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*proto.Log, len(m.logs))
	copy(out, m.logs)
	return out
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

// Regression test for https://github.com/coder/coder/issues/24766.
// A single terraform log line larger than the default
// bufio.MaxScanTokenSize (64 KiB) used to make the reader goroutine
// exit on bufio.ErrTooLong, leaving the io.Pipe writer blocked and
// `cmd.Wait()` hanging indefinitely. The reader must accept large
// hclog DEBUG lines (azurerm + TF_LOG=DEBUG can emit ~3 MB
// entries) without blocking the producer side.
func TestLogWriter_LargeLogLines(t *testing.T) {
	t.Parallel()

	t.Run("HandlesLineLargerThanLegacyScanLimit", func(t *testing.T) {
		t.Parallel()

		logr := &mockLogger{}
		writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

		// 256 KiB single line, well above the old 64 KiB bufio limit
		// but inside the configured cap. Use a printable byte so we
		// can sanity-check the payload survives intact.
		const lineSize = 256 * 1024
		bigLine := strings.Repeat("x", lineSize)
		payload := bigLine + "\nshort line\n"
		_, err := io.WriteString(writer, payload)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		<-doneLogging

		require.Len(t, logr.logs, 2, "both lines should reach the sink")
		require.Equal(t, lineSize, len(logr.logs[0].Output))
		require.Equal(t, "short line", logr.logs[1].Output)
	})

	t.Run("DrainsPipeWhenLineExceedsBufferCap", func(t *testing.T) {
		t.Parallel()

		logr := &mockLogger{}
		writer, doneLogging := logWriter(logr, proto.LogLevel_INFO)

		// Construct an over-limit blob: a single token bigger than
		// maxTerraformLogLineSize. The scanner will return
		// bufio.ErrTooLong, but our drainAfterScan must consume the
		// remainder so writer.Close() (and, in production, cmd.Wait)
		// can return cleanly instead of deadlocking.
		oversized := bytes.Repeat([]byte("z"), maxTerraformLogLineSize+1024)
		oversized = append(oversized, '\n')
		oversized = append(oversized, []byte("trailing\n")...)

		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			_, _ = writer.Write(oversized)
			_ = writer.Close()
		}()

		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-writeDone:
		case <-ctx.Done():
			t.Fatal("write side blocked: reader did not drain the pipe (issue #24766 has regressed)")
		}

		select {
		case <-doneLogging:
		case <-ctx.Done():
			t.Fatal("doneLogging never closed; reader goroutine leaked")
		}
		// The scanner errored on the oversized token. We do not
		// require any specific log content because the trailing line
		// is intentionally discarded by drain; the invariant we care
		// about is that neither write nor close blocks.
	})
}

// Regression test for https://github.com/coder/coder/issues/24766 covering
// resourceReplaceLogWriter (used during terraform plan/apply to elevate the
// log level of "forces replacement" lines).
func TestResourceReplaceLogWriter_LargeLines(t *testing.T) {
	t.Parallel()

	t.Run("HandlesLineLargerThanLegacyScanLimit", func(t *testing.T) {
		t.Parallel()

		const lineSize = 256 * 1024
		bigLine := strings.Repeat("y", lineSize)
		payload := bigLine + "\n# forces replacement: tag\n"

		logr := &mockLogger{}
		logger := slogtest.Make(t, nil)
		writer, done := resourceReplaceLogWriter(logr, logger)
		_, err := io.WriteString(writer, payload)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		<-done

		logs := logr.snapshot()
		require.Len(t, logs, 2, "both lines should reach the sink")
		require.Equal(t, lineSize, len(logs[0].Output))
		require.Equal(t, proto.LogLevel_WARN, logs[1].Level,
			"the forces-replacement marker must still be promoted to WARN")
	})

	t.Run("DrainsPipeWhenLineExceedsBufferCap", func(t *testing.T) {
		t.Parallel()

		r, w := io.Pipe()
		logr := &mockLogger{}
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

		// Start the reader goroutine inline, mirroring how
		// resourceReplaceLogWriter spawns it. We have to manage the
		// pipe ends ourselves to write past the buffer cap.
		done := make(chan struct{})
		go func() {
			defer close(done)
			s := newTerraformLogScanner(r)
			for s.Scan() {
				logr.ProvisionLog(proto.LogLevel_INFO, s.Text())
			}
			if err := s.Err(); err != nil {
				logger.Warn(t.Context(), "scan failed", slog.Error(err))
				drainAfterScan(r)
			}
		}()

		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			_, _ = w.Write(bytes.Repeat([]byte("y"), maxTerraformLogLineSize+1024))
			_, _ = w.Write([]byte("\nignored\n"))
			_ = w.Close()
		}()

		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-writeDone:
		case <-ctx.Done():
			t.Fatal("write side blocked: drain regressed (issue #24766)")
		}
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("reader goroutine leaked")
		}
	})
}

// Regression test for https://github.com/coder/coder/issues/24766 covering
// the JSON provisionLogWriter / readAndLog path used for stderr.
func TestReadAndLog_LargeLines(t *testing.T) {
	t.Parallel()

	t.Run("HandlesLineLargerThanLegacyScanLimit", func(t *testing.T) {
		t.Parallel()

		// 200 KiB single line above the 64 KiB legacy limit.
		const lineSize = 200 * 1024
		blob := strings.Repeat("x", lineSize)
		input := blob + "\n"

		logr := &mockLogger{}
		done := make(chan any)
		go readAndLog(logr, strings.NewReader(input), done, proto.LogLevel_INFO)
		<-done

		require.Len(t, logr.logs, 1)
		require.Equal(t, lineSize, len(logr.logs[0].Output))
	})

	t.Run("DrainsPipeWhenLineExceedsBufferCap", func(t *testing.T) {
		t.Parallel()

		r, w := io.Pipe()
		logr := &mockLogger{}
		done := make(chan any)
		go readAndLog(logr, r, done, proto.LogLevel_INFO)

		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			_, _ = w.Write(bytes.Repeat([]byte("z"), maxTerraformLogLineSize+1024))
			_, _ = w.Write([]byte("\nignored\n"))
			_ = w.Close()
		}()

		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-writeDone:
		case <-ctx.Done():
			t.Fatal("write side blocked: reader did not drain the pipe (issue #24766 has regressed)")
		}
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("readAndLog goroutine leaked")
		}
	})
}

// Regression test for https://github.com/coder/coder/issues/24766 covering
// the executor.provisionReadAndLog method that ingests terraform JSON logs
// (and extracts timing spans) on the apply path.
func TestProvisionReadAndLog_LargeLines(t *testing.T) {
	t.Parallel()

	t.Run("HandlesLineLargerThanLegacyScanLimit", func(t *testing.T) {
		t.Parallel()

		// Produce a non-JSON line over the legacy 64 KiB limit. The
		// parser falls back to a plain INFO log for non-JSON entries
		// (see parseTerraformLogLine), which is sufficient to exercise
		// the scanner buffer without having to build a 200 KiB JSON
		// blob.
		const lineSize = 200 * 1024
		blob := strings.Repeat("x", lineSize)
		input := blob + "\n"

		e := &executor{
			logger:  slogtest.Make(t, nil),
			timings: newTimingAggregator(database.ProvisionerJobTimingStageApply),
		}
		logr := &mockLogger{}
		done := make(chan any)
		go e.provisionReadAndLog(logr, strings.NewReader(input), done)
		<-done

		logs := logr.snapshot()
		require.Len(t, logs, 1)
		require.Equal(t, lineSize, len(logs[0].Output))
	})

	t.Run("DrainsPipeWhenLineExceedsBufferCap", func(t *testing.T) {
		t.Parallel()

		e := &executor{
			logger:  slogtest.Make(t, nil),
			timings: newTimingAggregator(database.ProvisionerJobTimingStageApply),
		}
		r, w := io.Pipe()
		logr := &mockLogger{}
		done := make(chan any)
		go e.provisionReadAndLog(logr, r, done)

		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxTerraformLogLineSize+1024))
			_, _ = w.Write([]byte("\nignored\n"))
			_ = w.Close()
		}()

		ctx := testutil.Context(t, testutil.WaitShort)
		select {
		case <-writeDone:
		case <-ctx.Done():
			t.Fatal("write side blocked: drain regressed for provisionReadAndLog (#24766)")
		}
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("provisionReadAndLog goroutine leaked")
		}
	})
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

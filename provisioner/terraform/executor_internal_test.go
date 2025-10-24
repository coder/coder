package terraform

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestChecksumFile(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		// Create a temporary file
		tmpfile, err := os.CreateTemp("", "lockfile-*.hcl")
		require.NoError(t, err)
		defer os.Remove(tmpfile.Name())

		content := []byte("provider \"aws\" {\n  version = \"5.0.0\"\n}\n")
		_, err = tmpfile.Write(content)
		require.NoError(t, err)
		tmpfile.Close()

		// Calculate checksum
		checksum1, exists := checksumFile(tmpfile.Name())
		require.True(t, exists)
		require.NotEmpty(t, checksum1)

		// Same file should have same checksum
		checksum2, exists := checksumFile(tmpfile.Name())
		require.True(t, exists)
		require.Equal(t, checksum1, checksum2)

		// Modify file
		err = os.WriteFile(tmpfile.Name(), []byte("modified content"), 0600)
		require.NoError(t, err)

		// Checksum should be different
		checksum3, exists := checksumFile(tmpfile.Name())
		require.True(t, exists)
		require.NotEqual(t, checksum1, checksum3)
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Parallel()

		checksum, exists := checksumFile("/nonexistent/file.hcl")
		require.False(t, exists)
		require.Empty(t, checksum)
	})
}

func TestGetTerraformLockFilePath(t *testing.T) {
	t.Parallel()

	workdir := "/tmp/test"
	expected := filepath.Join(workdir, ".terraform.lock.hcl")
	got := getTerraformLockFilePath(workdir)
	require.Equal(t, expected, got)
}

func TestWarnLockFileModified(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	e := &executor{
		logger:  testutil.Logger(t),
		workdir: "/tmp/test",
	}

	e.warnLockFileModified(testutil.Context(t, testutil.WaitShort), logr)

	// Check if warning was logged
	var hasWarning bool
	var allWarningContent string
	for _, log := range logr.logs {
		if log.Level == proto.LogLevel_WARN {
			hasWarning = true
			allWarningContent += log.Output + "\n"
		}
	}

	require.True(t, hasWarning, "expected warning log")
	require.Contains(t, allWarningContent, "WARNING: .terraform.lock.hcl was modified during 'terraform init'")
	require.Contains(t, allWarningContent, "terraform providers lock")
	require.Contains(t, allWarningContent, "linux_amd64")
}

func TestBufferedWriteCloser(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	writer, done := logWriter(logr, proto.LogLevel_DEBUG)
	buf := newBufferedWriteCloser(writer)

	testData := []byte("line1\nline2\nline3")
	n, err := buf.Write(testData)
	require.NoError(t, err)
	require.Equal(t, len(testData), n)

	// Verify data is in buffer
	require.Equal(t, testData, buf.b.Bytes())

	err = buf.Close()
	require.NoError(t, err)
	<-done

	// Verify data was written to logger
	require.Len(t, logr.logs, 3)
	require.Equal(t, "line1", logr.logs[0].Output)
	require.Equal(t, "line2", logr.logs[1].Output)
	require.Equal(t, "line3", logr.logs[2].Output)
}

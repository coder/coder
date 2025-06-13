package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type mockLogger struct {
	mu sync.Mutex

	logs []*proto.Log
}

var _ logSink = &mockLogger{}

func (m *mockLogger) ProvisionLog(l proto.LogLevel, o string) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
		tt := tt

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

func TestLogDrift_WithRealTerraformPlan(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)
	tmpDir := t.TempDir()

	binPath, err := Install(t.Context(), logger, true, tmpDir, TerraformVersion)
	require.NoError(t, err)

	tfConfig := `
terraform {
  required_providers {
    local = {
      source = "hashicorp/local"
      version = "~> 2.0"
    }
  }
}

resource "local_file" "test_file" {
  content  = "initial content"
  filename = "test.txt"
}
`

	tfFile := filepath.Join(tmpDir, "main.tf")
	require.NoError(t, os.WriteFile(tfFile, []byte(tfConfig), 0o644))

	// Create a minimal server for the executor.
	mockSrv := &server{
		logger:  logger,
		execMut: &sync.Mutex{},
		tracer:  noop.NewTracerProvider().Tracer("test"),
	}

	e := &executor{
		logger:     logger,
		binaryPath: binPath,
		workdir:    tmpDir,
		mut:        mockSrv.execMut,
		server:     mockSrv,
		timings:    newTimingAggregator(database.ProvisionerJobTimingStagePlan),
	}

	// These contexts must be explicitly separate from the test context.
	// We have a log message which prints when these contexts are canceled (or when the test completes if using t.Context()),
	// and this log output would be confusing to the casual reader, while innocuous.
	// See interruptCommandOnCancel in executor.go.
	ctx := context.Background()
	killCtx := context.Background()

	var mockSink mockLogger
	err = e.init(ctx, killCtx, &mockSink)
	require.NoError(t, err)

	// Create initial plan to establish state.
	_, err = e.plan(ctx, killCtx, e.basicEnv(), []string{}, &mockSink, &proto.Metadata{
		WorkspaceTransition: proto.WorkspaceTransition_START,
	})
	require.NoError(t, err)

	// Apply the plan to create initial state.
	_, err = e.apply(ctx, killCtx, e.basicEnv(), &mockSink)
	require.NoError(t, err)

	// Now modify the terraform configuration to cause drift.
	driftConfig := `
terraform {
  required_providers {
    local = {
      source = "hashicorp/local"
      version = "~> 2.0"
    }
  }
}

resource "local_file" "test_file" {
  content  = "changed content that forces replacement"
  filename = "test.txt"
}
`

	// Write the modified configuration.
	require.NoError(t, os.WriteFile(tfFile, []byte(driftConfig), 0o644))

	// Create a new plan that will show the drift/replacement.
	driftLogger := &mockLogger{}
	planResult, err := e.plan(ctx, killCtx, e.basicEnv(), []string{}, driftLogger, &proto.Metadata{
		WorkspaceTransition: proto.WorkspaceTransition_START,
	})
	require.NoError(t, err)

	// Verify we detected resource replacements (this triggers logDrift).
	require.NotEmpty(t, planResult.ResourceReplacements, "Should detect resource replacements that trigger drift logging")

	// Verify that drift logs were captured.
	require.NotEmpty(t, driftLogger.logs, "logDrift should produce log output")

	// Check that we have logs showing the resource replacement(s).
	var (
		foundReplacementLog, foundInfoLogs, foundWarnLogs bool
	)

	for _, log := range driftLogger.logs {
		t.Logf("[%s] %s", log.Level.String(), log.Output)

		if strings.Contains(log.Output, "# forces replacement") {
			foundReplacementLog = true
			require.Equal(t, proto.LogLevel_WARN, log.Level, "Lines containing '# forces replacement' should be logged at WARN level")
			foundWarnLogs = true
		}

		if log.Level == proto.LogLevel_INFO {
			foundInfoLogs = true
		}
	}

	// Verify we found the expected log types.
	require.True(t, foundReplacementLog, "Should find log lines containing '# forces replacement'")
	require.True(t, foundInfoLogs, "Should find INFO level logs showing the drift details")
	require.True(t, foundWarnLogs, "Should find WARN level logs for resource replacements")

	// Verify that the drift shows the resource change.
	logOutput := strings.Join(func() []string {
		var outputs []string
		for _, log := range driftLogger.logs {
			outputs = append(outputs, log.Output)
		}
		return outputs
	}(), "\n")

	require.Contains(t, logOutput, "local_file.test_file", "Drift logs should mention the specific resource")
}

func TestResourceReplaceLogWriter(t *testing.T) {
	t.Parallel()

	var logr mockLogger
	logger := testutil.Logger(t)
	writer, doneLogging := resourceReplaceLogWriter(&logr, logger)

	// Test input with both normal lines and replacement lines.
	testInput := `  # local_file.test_file will be replaced
-/+ resource "local_file" "test_file" {
      ~ content  = "initial content" -> "changed content" # forces replacement
      ~ filename = "test.txt"
        id       = "1234567890"
    }

Plan: 1 to add, 0 to change, 1 to destroy.`

	_, err := writer.Write([]byte(testInput))
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	<-doneLogging

	// Verify the logs
	require.NotEmpty(t, logr.logs, "Should produce log output")

	var foundReplacementWarn, foundInfoLogs bool

	for _, log := range logr.logs {
		t.Logf("[%s] %s", log.Level.String(), log.Output)

		if strings.Contains(log.Output, "# forces replacement") {
			require.Equal(t, proto.LogLevel_WARN, log.Level, "Lines containing '# forces replacement' should be WARN level")
			foundReplacementWarn = true
		} else if log.Level == proto.LogLevel_INFO {
			foundInfoLogs = true
		}
	}

	require.True(t, foundReplacementWarn, "Should find WARN level log for '# forces replacement' line")
	require.True(t, foundInfoLogs, "Should find INFO level logs for other lines")
}

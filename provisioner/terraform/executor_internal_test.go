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

	cases := []struct {
		name              string
		isPrebuildClaim   bool
		expectedInfoLines int
		expectedWarnLines int
	}{
		{
			name:              "regular build",
			isPrebuildClaim:   false,
			expectedInfoLines: 26,
			expectedWarnLines: 0,
		},
		{
			name:              "prebuild claim",
			isPrebuildClaim:   true,
			expectedInfoLines: 25,
			expectedWarnLines: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
			require.NoError(t, os.WriteFile(tfFile, []byte(tfConfig), 0o600))

			req := &proto.PlanRequest{
				Metadata: &proto.Metadata{
					WorkspaceTransition:         proto.WorkspaceTransition_START,
					PrebuiltWorkspaceBuildStage: proto.PrebuiltWorkspaceBuildStage_NONE,
				},
			}
			if tc.isPrebuildClaim {
				req.Metadata.PrebuiltWorkspaceBuildStage = proto.PrebuiltWorkspaceBuildStage_CLAIM
			}

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
			_, err = e.plan(ctx, killCtx, e.basicEnv(), []string{}, &mockSink, req)
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
			require.NoError(t, os.WriteFile(tfFile, []byte(driftConfig), 0o600))

			// Create a new plan that will show the drift/replacement.
			driftLogger := &mockLogger{}
			planResult, err := e.plan(ctx, killCtx, e.basicEnv(), []string{}, driftLogger, req)
			require.NoError(t, err)

			// Verify we detected resource replacements (this triggers logDrift).
			require.NotEmpty(t, planResult.ResourceReplacements, "Should detect resource replacements that trigger drift logging")

			// Verify that drift logs were captured.
			require.NotEmpty(t, driftLogger.logs, "logDrift should produce log output")

			// Check that we have logs showing the resource replacement(s).
			var infoLines, warnLines, otherLines int
			for _, log := range driftLogger.logs {
				switch log.GetLevel() {
				case proto.LogLevel_INFO:
					infoLines++
				case proto.LogLevel_WARN:
					warnLines++
				default:
					otherLines++
				}
			}

			// Verify we found the expected logs by level.
			require.Equal(t, tc.expectedInfoLines, infoLines)
			require.Equal(t, tc.expectedWarnLines, warnLines)
			require.Equal(t, 0, otherLines)

			// Verify that the drift shows the resource change.
			logOutput := strings.Join(func() []string {
				var outputs []string
				for _, log := range driftLogger.logs {
					outputs = append(outputs, log.Output)
				}
				return outputs
			}(), "\n")

			require.Contains(t, logOutput, "local_file.test_file", "Drift logs should mention the specific resource")
		})
	}
}

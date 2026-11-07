package terraform

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk"
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

func TestPlanModulesRetainsPriorStateScriptOrder(t *testing.T) {
	t.Parallel()

	// Generated by Terraform 1.16.2 with terraform-provider-coder at e851c57.
	rawPlan, err := os.ReadFile("testdata/script-order-plan.json")
	require.NoError(t, err)
	var plan tfjson.Plan
	require.NoError(t, json.Unmarshal(rawPlan, &plan))

	modules := planModules(&plan)
	require.True(t, hasScriptOrderDataSource(modules))
}

func TestLogScriptOrderWarnings(t *testing.T) {
	t.Parallel()

	logger := &mockLogger{}
	logScriptOrderWarnings(logger, []string{"first warning", "second warning"}, 3)
	require.Len(t, logger.logs, 3)
	for _, log := range logger.logs {
		require.Equal(t, proto.LogLevel_WARN, log.Level)
	}
	require.Equal(t, "first warning", logger.logs[0].Output)
	require.Equal(t, "second warning", logger.logs[1].Output)
	require.Contains(t, logger.logs[2].Output, "3 additional script order warnings")
	require.Contains(t, logger.logs[2].Output, "only the first 100 are reported")
}

func TestCachePlanConfigIgnoresInitialNil(t *testing.T) {
	t.Parallel()

	s := &server{}
	sess := &provisionersdk.Session{}
	s.cachePlanConfig(t.Context(), sess, nil)

	s.planConfigsBySessionMu.Lock()
	defer s.planConfigsBySessionMu.Unlock()
	_, ok := s.planConfigsBySession[sess]
	require.False(t, ok)
}

func TestCachePlanConfigReusesSessionCleanup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &server{}
	sess := &provisionersdk.Session{}
	firstConfig := &tfjson.Config{}
	secondConfig := &tfjson.Config{}
	loadConfig := func() (*tfjson.Config, bool) {
		s.planConfigsBySessionMu.Lock()
		defer s.planConfigsBySessionMu.Unlock()
		config, ok := s.planConfigsBySession[sess]
		return config, ok
	}

	s.cachePlanConfig(ctx, sess, firstConfig)
	config, ok := loadConfig()
	require.True(t, ok)
	require.Same(t, firstConfig, config)

	s.cachePlanConfig(ctx, sess, secondConfig)
	config, ok = loadConfig()
	require.True(t, ok)
	require.Same(t, secondConfig, config)

	s.cachePlanConfig(ctx, sess, nil)
	config, ok = loadConfig()
	require.True(t, ok)
	require.Nil(t, config)
	_, ok = s.loadPlanConfig(sess)
	require.False(t, ok)

	s.cachePlanConfig(ctx, sess, secondConfig)
	config, ok = loadConfig()
	require.True(t, ok)
	require.Same(t, secondConfig, config)
	cancel()
	require.Eventually(t, func() bool {
		_, ok := loadConfig()
		return !ok
	}, testutil.WaitShort, testutil.IntervalFast)
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

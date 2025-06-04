package dcspec_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers/dcspec"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

func TestUnmarshalDevContainer(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		file    string
		wantErr bool
		want    dcspec.DevContainer
	}
	tests := []testCase{
		{
			name: "minimal",
			file: filepath.Join("testdata", "minimal.json"),
			want: dcspec.DevContainer{
				Image: ptr.Ref("test-image"),
			},
		},
		{
			name: "arrays",
			file: filepath.Join("testdata", "arrays.json"),
			want: dcspec.DevContainer{
				Image:   ptr.Ref("test-image"),
				RunArgs: []string{"--network=host", "--privileged"},
				ForwardPorts: []dcspec.ForwardPort{
					{
						Integer: ptr.Ref[int64](8080),
					},
					{
						String: ptr.Ref("3000:3000"),
					},
				},
			},
		},
		{
			name:    "devcontainers/template-starter",
			file:    filepath.Join("testdata", "devcontainers-template-starter.json"),
			wantErr: false,
			want: dcspec.DevContainer{
				Image:    ptr.Ref("mcr.microsoft.com/devcontainers/javascript-node:1-18-bullseye"),
				Features: &dcspec.Features{},
				Customizations: map[string]interface{}{
					"vscode": map[string]interface{}{
						"extensions": []interface{}{
							"mads-hartmann.bash-ide-vscode",
							"dbaeumer.vscode-eslint",
						},
					},
				},
				PostCreateCommand: &dcspec.Command{
					String: ptr.Ref("npm install -g @devcontainers/cli"),
				},
			},
		},
	}

	var missingTests []string
	files, err := filepath.Glob("testdata/*.json")
	require.NoError(t, err, "glob test files failed")
	for _, file := range files {
		if !slices.ContainsFunc(tests, func(tt testCase) bool {
			return tt.file == file
		}) {
			missingTests = append(missingTests, file)
		}
	}
	require.Empty(t, missingTests, "missing tests case for files: %v", missingTests)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(tt.file)
			require.NoError(t, err, "read test file failed")

			got, err := dcspec.UnmarshalDevContainer(data)
			if tt.wantErr {
				require.Error(t, err, "want error but got nil")
				return
			}
			require.NoError(t, err, "unmarshal DevContainer failed")

			// Compare the unmarshaled data with the expected data.
			if diff := cmp.Diff(tt.want, got); diff != "" {
				require.Empty(t, diff, "UnmarshalDevContainer() mismatch (-want +got):\n%s", diff)
			}

			// Test that marshaling works (without comparing to original).
			marshaled, err := got.Marshal()
			require.NoError(t, err, "marshal DevContainer back to JSON failed")
			require.NotEmpty(t, marshaled, "marshaled JSON should not be empty")

			// Verify the marshaled JSON can be unmarshaled back.
			var unmarshaled interface{}
			err = json.Unmarshal(marshaled, &unmarshaled)
			require.NoError(t, err, "unmarshal marshaled JSON failed")
		})
	}
}

func TestUnmarshalDevContainer_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "empty JSON",
			json:    "{}",
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			json:    "{not valid json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := dcspec.UnmarshalDevContainer([]byte(tt.json))
			if tt.wantErr {
				require.Error(t, err, "want error but got nil")
				return
			}
			require.NoError(t, err, "unmarshal DevContainer failed")
		})
	}
}

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// nolint:paralleltest
func Test_absoluteBinaryPath(t *testing.T) {
	tests := []struct {
		name             string
		terraformVersion string
		expectedErr      error
	}{
		{
			name:             "TestCorrectVersion",
			terraformVersion: "1.3.0",
			expectedErr:      nil,
		},
		{
			name:             "TestOldVersion",
			terraformVersion: "1.0.9",
			expectedErr:      errTerraformMinorVersionMismatch,
		},
		{
			name:             "TestNewVersion",
			terraformVersion: "1.3.0",
			expectedErr:      nil,
		},
		{
			name:             "TestNewestNewVersion",
			terraformVersion: "9.9.9",
			expectedErr:      nil,
		},
		{
			name:             "TestMalformedVersion",
			terraformVersion: "version",
			expectedErr:      xerrors.Errorf("Terraform binary get version failed: malformed version: version"),
		},
	}
	// nolint:paralleltest
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Dummy terraform executable on Windows requires sh which isn't very practical.")
			}

			// Create a temp dir with the binary
			tempDir := t.TempDir()
			terraformBinaryOutput := fmt.Sprintf(`#!/bin/sh
			cat <<-EOF
			{
				"terraform_version": "%s",
				"platform": "linux_amd64",
				"provider_selections": {},
				"terraform_outdated": false
			}
			EOF`, tt.terraformVersion)

			// #nosec
			err := os.WriteFile(
				filepath.Join(tempDir, "terraform"),
				[]byte(terraformBinaryOutput),
				0o770,
			)
			require.NoError(t, err)

			// Add the binary to PATH
			pathVariable := os.Getenv("PATH")
			t.Setenv("PATH", strings.Join([]string{tempDir, pathVariable}, ":"))

			var expectedAbsoluteBinary string
			if tt.expectedErr == nil {
				expectedAbsoluteBinary = filepath.Join(tempDir, "terraform")
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			actualBinaryDetails, actualErr := systemBinary(ctx)

			if tt.expectedErr == nil {
				require.NoError(t, actualErr)
				require.Equal(t, expectedAbsoluteBinary, actualBinaryDetails.absolutePath)
				require.Equal(t, tt.terraformVersion, actualBinaryDetails.version.String())
			} else {
				require.EqualError(t, actualErr, tt.expectedErr.Error())
			}
		})
	}
}

func Test_terraformVersionCached(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Dummy terraform executable on Windows requires sh which isn't very practical.")
	}

	// The stub records every invocation so the test can prove that only the
	// first version lookup spawns a process.
	tempDir := t.TempDir()
	counterPath := filepath.Join(tempDir, "calls")
	binaryPath := filepath.Join(tempDir, "terraform")
	script := fmt.Sprintf(`#!/bin/sh
	echo x >> %q
	cat <<-EOF
	{
		"terraform_version": "1.9.0",
		"platform": "linux_amd64",
		"provider_selections": {},
		"terraform_outdated": false
	}
	EOF`, counterPath)
	// #nosec
	require.NoError(t, os.WriteFile(binaryPath, []byte(script), 0o770))

	srv := &server{binaryPath: binaryPath}
	ctx := testutil.Context(t, testutil.WaitShort)

	first, err := srv.terraformVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.9.0", first.String())

	for range 5 {
		again, err := srv.terraformVersion(ctx)
		require.NoError(t, err)
		require.Same(t, first, again)
	}

	calls, err := os.ReadFile(counterPath)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(calls), "x"), "terraform version should be resolved once")
}

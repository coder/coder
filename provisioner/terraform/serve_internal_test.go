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

	"cdr.dev/slog/sloggers/slogtest"
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
			expectedErr:      terraformMinorVersionMismatch,
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
			expectedErr:      xerrors.Errorf("Terraform binary get version failed: Malformed version: version"),
		},
	}
	// nolint:paralleltest
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Dummy terraform executable on Windows requires sh which isn't very practical.")
			}

			log := slogtest.Make(t, nil)
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
			actualAbsoluteBinary, actualErr := absoluteBinaryPath(ctx, log)

			require.Equal(t, expectedAbsoluteBinary, actualAbsoluteBinary)
			if tt.expectedErr == nil {
				require.NoError(t, actualErr)
			} else {
				require.EqualError(t, actualErr, tt.expectedErr.Error())
			}
		})
	}
}

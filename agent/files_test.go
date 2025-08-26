package agent_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

type testFs struct {
	afero.Fs
	// intercept can return an error for testing when a call fails.
	intercept func(call, file string) error
}

func newTestFs(base afero.Fs, intercept func(call, file string) error) *testFs {
	return &testFs{
		Fs:        base,
		intercept: intercept,
	}
}

func (fs *testFs) Open(name string) (afero.File, error) {
	if err := fs.intercept("open", name); err != nil {
		return nil, err
	}
	return fs.Fs.Open(name)
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms")
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.Filesystem = newTestFs(opts.Filesystem, func(call, file string) error {
			if file == noPermsFilePath {
				return os.ErrPermission
			}
			return nil
		})
	})

	dirPath := filepath.Join(tmpdir, "a-directory")
	err := fs.MkdirAll(dirPath, 0o755)
	require.NoError(t, err)

	filePath := filepath.Join(tmpdir, "file")
	err = afero.WriteFile(fs, filePath, []byte("content"), 0o644)
	require.NoError(t, err)

	imagePath := filepath.Join(tmpdir, "file.png")
	err = afero.WriteFile(fs, imagePath, []byte("not really an image"), 0o644)
	require.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		limit    int64
		offset   int64
		bytes    []byte
		mimeType string
		errCode  int
		error    string
	}{
		{
			name:    "NoPath",
			path:    "",
			errCode: http.StatusBadRequest,
			error:   "\"path\" is required",
		},
		{
			name:    "RelativePath",
			path:    "./relative",
			errCode: http.StatusBadRequest,
			error:   "file path must be absolute",
		},
		{
			name:    "RelativePath",
			path:    "also-relative",
			errCode: http.StatusBadRequest,
			error:   "file path must be absolute",
		},
		{
			name:    "NegativeLimit",
			path:    filePath,
			limit:   -10,
			errCode: http.StatusBadRequest,
			error:   "value is negative",
		},
		{
			name:    "NegativeOffset",
			path:    filePath,
			offset:  -10,
			errCode: http.StatusBadRequest,
			error:   "value is negative",
		},
		{
			name:    "NonExistent",
			path:    filepath.Join(tmpdir, "does-not-exist"),
			errCode: http.StatusNotFound,
			error:   "file does not exist",
		},
		{
			name:    "IsDir",
			path:    dirPath,
			errCode: http.StatusBadRequest,
			error:   "not a file",
		},
		{
			name:    "NoPermissions",
			path:    noPermsFilePath,
			errCode: http.StatusForbidden,
			error:   "permission denied",
		},
		{
			name:  "Defaults",
			path:  filePath,
			bytes: []byte("content"),
		},
		{
			name:  "Limit1",
			path:  filePath,
			limit: 1,
			bytes: []byte("c"),
		},
		{
			name:   "Offset1",
			path:   filePath,
			offset: 1,
			bytes:  []byte("ontent"),
		},
		{
			name:   "Limit1Offset2",
			path:   filePath,
			limit:  1,
			offset: 2,
			bytes:  []byte("n"),
		},
		{
			name:   "Limit7Offset0",
			path:   filePath,
			limit:  7,
			offset: 0,
			bytes:  []byte("content"),
		},
		{
			name:  "Limit100",
			path:  filePath,
			limit: 100,
			bytes: []byte("content"),
		},
		{
			name:   "Offset7",
			path:   filePath,
			offset: 7,
			bytes:  []byte{},
		},
		{
			name:   "Offset100",
			path:   filePath,
			offset: 100,
			bytes:  []byte{},
		},
		{
			name:     "MimeTypePng",
			path:     imagePath,
			bytes:    []byte("not really an image"),
			mimeType: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			b, mimeType, err := conn.ReadFile(ctx, tt.path, tt.offset, tt.limit)
			if tt.errCode != 0 {
				require.Error(t, err)
				cerr := coderdtest.SDKError(t, err)
				require.Contains(t, cerr.Error(), tt.error)
				require.Equal(t, tt.errCode, cerr.StatusCode())
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.bytes, b)
				expectedMimeType := tt.mimeType
				if expectedMimeType == "" {
					expectedMimeType = "application/octet-stream"
				}
				require.Equal(t, expectedMimeType, mimeType)
			}
		})
	}
}

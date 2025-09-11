package agent_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

func (fs *testFs) Create(name string) (afero.File, error) {
	if err := fs.intercept("create", name); err != nil {
		return nil, err
	}
	// Unlike os, afero lets you create files where directories already exist and
	// lets you nest them underneath files, somehow.
	stat, err := fs.Fs.Stat(name)
	if err == nil && stat.IsDir() {
		return nil, &os.PathError{
			Op:   "open",
			Path: name,
			Err:  syscall.EISDIR,
		}
	}
	stat, err = fs.Fs.Stat(filepath.Dir(name))
	if err == nil && !stat.IsDir() {
		return nil, &os.PathError{
			Op:   "open",
			Path: name,
			Err:  syscall.ENOTDIR,
		}
	}
	return fs.Fs.Create(name)
}

func (fs *testFs) MkdirAll(name string, mode os.FileMode) error {
	if err := fs.intercept("mkdirall", name); err != nil {
		return err
	}
	// Unlike os, afero lets you create directories where files already exist and
	// lets you nest them underneath files somehow.
	stat, err := fs.Fs.Stat(filepath.Dir(name))
	if err == nil && !stat.IsDir() {
		return &os.PathError{
			Op:   "mkdir",
			Path: name,
			Err:  syscall.ENOTDIR,
		}
	}
	stat, err = fs.Fs.Stat(name)
	if err == nil && !stat.IsDir() {
		return &os.PathError{
			Op:   "mkdir",
			Path: name,
			Err:  syscall.ENOTDIR,
		}
	}
	return fs.Fs.MkdirAll(name, mode)
}

func (fs *testFs) Rename(oldName, newName string) error {
	if err := fs.intercept("rename", newName); err != nil {
		return err
	}
	return fs.Fs.Rename(oldName, newName)
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
			name:    "RelativePathDotSlash",
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
			name:     "Defaults",
			path:     filePath,
			bytes:    []byte("content"),
			mimeType: "application/octet-stream",
		},
		{
			name:     "Limit1",
			path:     filePath,
			limit:    1,
			bytes:    []byte("c"),
			mimeType: "application/octet-stream",
		},
		{
			name:     "Offset1",
			path:     filePath,
			offset:   1,
			bytes:    []byte("ontent"),
			mimeType: "application/octet-stream",
		},
		{
			name:     "Limit1Offset2",
			path:     filePath,
			limit:    1,
			offset:   2,
			bytes:    []byte("n"),
			mimeType: "application/octet-stream",
		},
		{
			name:     "Limit7Offset0",
			path:     filePath,
			limit:    7,
			offset:   0,
			bytes:    []byte("content"),
			mimeType: "application/octet-stream",
		},
		{
			name:     "Limit100",
			path:     filePath,
			limit:    100,
			bytes:    []byte("content"),
			mimeType: "application/octet-stream",
		},
		{
			name:     "Offset7",
			path:     filePath,
			offset:   7,
			bytes:    []byte{},
			mimeType: "application/octet-stream",
		},
		{
			name:     "Offset100",
			path:     filePath,
			offset:   100,
			bytes:    []byte{},
			mimeType: "application/octet-stream",
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

			reader, mimeType, err := conn.ReadFile(ctx, tt.path, tt.offset, tt.limit)
			if tt.errCode != 0 {
				require.Error(t, err)
				cerr := coderdtest.SDKError(t, err)
				require.Contains(t, cerr.Error(), tt.error)
				require.Equal(t, tt.errCode, cerr.StatusCode())
			} else {
				require.NoError(t, err)
				defer reader.Close()
				bytes, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, tt.bytes, bytes)
				require.Equal(t, tt.mimeType, mimeType)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms-file")
	noPermsDirPath := filepath.Join(tmpdir, "no-perms-dir")
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.Filesystem = newTestFs(opts.Filesystem, func(call, file string) error {
			if file == noPermsFilePath || file == noPermsDirPath {
				return os.ErrPermission
			}
			return nil
		})
	})

	dirPath := filepath.Join(tmpdir, "directory")
	err := fs.MkdirAll(dirPath, 0o755)
	require.NoError(t, err)

	filePath := filepath.Join(tmpdir, "file")
	err = afero.WriteFile(fs, filePath, []byte("content"), 0o644)
	require.NoError(t, err)

	notDirErr := "not a directory"
	if runtime.GOOS == "windows" {
		notDirErr = "cannot find the path"
	}

	tests := []struct {
		name    string
		path    string
		bytes   []byte
		errCode int
		error   string
	}{
		{
			name:    "NoPath",
			path:    "",
			errCode: http.StatusBadRequest,
			error:   "\"path\" is required",
		},
		{
			name:    "RelativePathDotSlash",
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
			name:  "NonExistent",
			path:  filepath.Join(tmpdir, "/nested/does-not-exist"),
			bytes: []byte("now it does exist"),
		},
		{
			name:    "IsDir",
			path:    dirPath,
			errCode: http.StatusBadRequest,
			error:   "is a directory",
		},
		{
			name:    "IsNotDir",
			path:    filepath.Join(filePath, "file2"),
			errCode: http.StatusBadRequest,
			error:   notDirErr,
		},
		{
			name:    "NoPermissionsFile",
			path:    noPermsFilePath,
			errCode: http.StatusForbidden,
			error:   "permission denied",
		},
		{
			name:    "NoPermissionsDir",
			path:    filepath.Join(noPermsDirPath, "within-no-perm-dir"),
			errCode: http.StatusForbidden,
			error:   "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			reader := bytes.NewReader(tt.bytes)
			err := conn.WriteFile(ctx, tt.path, reader)
			if tt.errCode != 0 {
				require.Error(t, err)
				cerr := coderdtest.SDKError(t, err)
				require.Contains(t, cerr.Error(), tt.error)
				require.Equal(t, tt.errCode, cerr.StatusCode())
			} else {
				require.NoError(t, err)
				b, err := afero.ReadFile(fs, tt.path)
				require.NoError(t, err)
				require.Equal(t, tt.bytes, b)
			}
		})
	}
}

func TestEditFile(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms-file")
	failRenameFilePath := filepath.Join(tmpdir, "fail-rename")
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.Filesystem = newTestFs(opts.Filesystem, func(call, file string) error {
			if file == noPermsFilePath {
				return os.ErrPermission
			} else if file == failRenameFilePath && call == "rename" {
				return xerrors.New("rename failed")
			}
			return nil
		})
	})

	dirPath := filepath.Join(tmpdir, "directory")
	err := fs.MkdirAll(dirPath, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		contents string
		edits    []workspacesdk.FileEdit
		expected string
		errCode  int
		error    string
	}{
		{
			name:    "NoPath",
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
			name:     "NoEdits",
			path:     filepath.Join(tmpdir, "no-edits"),
			contents: "foo bar",
			errCode:  http.StatusBadRequest,
			error:    "must specify at least one edit",
		},
		{
			name: "NonExistent",
			path: filepath.Join(tmpdir, "does-not-exist"),
			edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
			},
			errCode: http.StatusNotFound,
			error:   "file does not exist",
		},
		{
			name: "IsDir",
			path: dirPath,
			edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
			},
			errCode: http.StatusBadRequest,
			error:   "not a file",
		},
		{
			name: "NoPermissions",
			path: noPermsFilePath,
			edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
			},
			errCode: http.StatusForbidden,
			error:   "permission denied",
		},
		{
			name:     "FailRename",
			path:     failRenameFilePath,
			contents: "foo bar",
			edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
			},
			errCode: http.StatusInternalServerError,
			error:   "rename failed",
		},
		{
			name:     "Edit1",
			path:     filepath.Join(tmpdir, "edit1"),
			contents: "foo bar",
			edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
			},
			expected: "bar bar",
		},
		{
			name:     "EditEdit", // Edits affect previous edits.
			path:     filepath.Join(tmpdir, "edit-edit"),
			contents: "foo bar",
			edits: []workspacesdk.FileEdit{
				{
					Search:  "foo",
					Replace: "bar",
				},
				{
					Search:  "bar",
					Replace: "qux",
				},
			},
			expected: "qux qux",
		},
		{
			name:     "Multiline",
			path:     filepath.Join(tmpdir, "multiline"),
			contents: "foo\nbar\nbaz\nqux",
			edits: []workspacesdk.FileEdit{
				{
					Search:  "bar\nbaz",
					Replace: "frob",
				},
			},
			expected: "foo\nfrob\nqux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			if tt.contents != "" {
				err := afero.WriteFile(fs, tt.path, []byte(tt.contents), 0o644)
				require.NoError(t, err)
			}

			err := conn.EditFile(ctx, tt.path, workspacesdk.FileEditRequest{Edits: tt.edits})
			if tt.errCode != 0 {
				require.Error(t, err)
				cerr := coderdtest.SDKError(t, err)
				require.Contains(t, cerr.Error(), tt.error)
				require.Equal(t, tt.errCode, cerr.StatusCode())
			} else {
				require.NoError(t, err)
				b, err := afero.ReadFile(fs, tt.path)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(b))
			}
		})
	}
}

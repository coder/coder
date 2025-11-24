package agent_test

import (
	"bytes"
	"context"
	"fmt"
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

func TestEditFiles(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms-file")
	failRenameFilePath := filepath.Join(tmpdir, "fail-rename")
	//nolint:dogsled
	conn, _, _, fs, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, opts *agent.Options) {
		opts.Filesystem = newTestFs(opts.Filesystem, func(call, file string) error {
			if file == noPermsFilePath {
				return &os.PathError{
					Op:   call,
					Path: file,
					Err:  os.ErrPermission,
				}
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
		contents map[string]string
		edits    []workspacesdk.FileEdits
		expected map[string]string
		errCode  int
		errors   []string
	}{
		{
			name:    "NoFiles",
			errCode: http.StatusBadRequest,
			errors:  []string{"must specify at least one file"},
		},
		{
			name:    "NoPath",
			errCode: http.StatusBadRequest,
			edits: []workspacesdk.FileEdits{
				{
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errors: []string{"\"path\" is required"},
		},
		{
			name: "RelativePathDotSlash",
			edits: []workspacesdk.FileEdits{
				{
					Path: "./relative",
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errCode: http.StatusBadRequest,
			errors:  []string{"file path must be absolute"},
		},
		{
			name: "RelativePath",
			edits: []workspacesdk.FileEdits{
				{
					Path: "also-relative",
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errCode: http.StatusBadRequest,
			errors:  []string{"file path must be absolute"},
		},
		{
			name: "NoEdits",
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "no-edits"),
				},
			},
			errCode: http.StatusBadRequest,
			errors:  []string{"must specify at least one edit"},
		},
		{
			name: "NonExistent",
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "does-not-exist"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errCode: http.StatusNotFound,
			errors:  []string{"file does not exist"},
		},
		{
			name: "IsDir",
			edits: []workspacesdk.FileEdits{
				{
					Path: dirPath,
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errCode: http.StatusBadRequest,
			errors:  []string{"not a file"},
		},
		{
			name: "NoPermissions",
			edits: []workspacesdk.FileEdits{
				{
					Path: noPermsFilePath,
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errCode: http.StatusForbidden,
			errors:  []string{"permission denied"},
		},
		{
			name:     "FailRename",
			contents: map[string]string{failRenameFilePath: "foo bar"},
			edits: []workspacesdk.FileEdits{
				{
					Path: failRenameFilePath,
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			errCode: http.StatusInternalServerError,
			errors:  []string{"rename failed"},
		},
		{
			name:     "Edit1",
			contents: map[string]string{filepath.Join(tmpdir, "edit1"): "foo bar"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "edit1"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "edit1"): "bar bar"},
		},
		{
			name:     "EditEdit", // Edits affect previous edits.
			contents: map[string]string{filepath.Join(tmpdir, "edit-edit"): "foo bar"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "edit-edit"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
						{
							Search:  "bar",
							Replace: "qux",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "edit-edit"): "qux qux"},
		},
		{
			name:     "Multiline",
			contents: map[string]string{filepath.Join(tmpdir, "multiline"): "foo\nbar\nbaz\nqux"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "multiline"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "bar\nbaz",
							Replace: "frob",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "multiline"): "foo\nfrob\nqux"},
		},
		{
			name: "Multifile",
			contents: map[string]string{
				filepath.Join(tmpdir, "file1"): "file 1",
				filepath.Join(tmpdir, "file2"): "file 2",
				filepath.Join(tmpdir, "file3"): "file 3",
			},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "file1"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "file",
							Replace: "edited1",
						},
					},
				},
				{
					Path: filepath.Join(tmpdir, "file2"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "file",
							Replace: "edited2",
						},
					},
				},
				{
					Path: filepath.Join(tmpdir, "file3"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "file",
							Replace: "edited3",
						},
					},
				},
			},
			expected: map[string]string{
				filepath.Join(tmpdir, "file1"): "edited1 1",
				filepath.Join(tmpdir, "file2"): "edited2 2",
				filepath.Join(tmpdir, "file3"): "edited3 3",
			},
		},
		{
			name: "MultiError",
			contents: map[string]string{
				filepath.Join(tmpdir, "file8"): "file 8",
			},
			edits: []workspacesdk.FileEdits{
				{
					Path: noPermsFilePath,
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "file",
							Replace: "edited7",
						},
					},
				},
				{
					Path: filepath.Join(tmpdir, "file8"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "file",
							Replace: "edited8",
						},
					},
				},
				{
					Path: filepath.Join(tmpdir, "file9"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "file",
							Replace: "edited9",
						},
					},
				},
			},
			expected: map[string]string{
				filepath.Join(tmpdir, "file8"): "edited8 8",
			},
			// Higher status codes will override lower ones, so in this case the 404
			// takes priority over the 403.
			errCode: http.StatusNotFound,
			errors: []string{
				fmt.Sprintf("%s: permission denied", noPermsFilePath),
				"file9: file does not exist",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			for path, content := range tt.contents {
				err := afero.WriteFile(fs, path, []byte(content), 0o644)
				require.NoError(t, err)
			}

			err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: tt.edits})
			if tt.errCode != 0 {
				require.Error(t, err)
				cerr := coderdtest.SDKError(t, err)
				for _, error := range tt.errors {
					require.Contains(t, cerr.Error(), error)
				}
				require.Equal(t, tt.errCode, cerr.StatusCode())
			} else {
				require.NoError(t, err)
			}
			for path, expect := range tt.expected {
				b, err := afero.ReadFile(fs, path)
				require.NoError(t, err)
				require.Equal(t, expect, string(b))
			}
		})
	}
}

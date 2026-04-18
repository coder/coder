package agentfiles_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"testing/iotest"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/codersdk"
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

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := newTestFs(afero.NewMemMapFs(), func(call, file string) error {
		if file == noPermsFilePath {
			return os.ErrPermission
		}
		return nil
	})
	api := agentfiles.NewAPI(logger, fs, nil)

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

			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/read-file?path=%s&offset=%d&limit=%d", tt.path, tt.offset, tt.limit), nil)
			api.Routes().ServeHTTP(w, r)

			if tt.errCode != 0 {
				got := &codersdk.Error{}
				err := json.NewDecoder(w.Body).Decode(got)
				require.NoError(t, err)
				require.ErrorContains(t, got, tt.error)
				require.Equal(t, tt.errCode, w.Code)
			} else {
				bytes, err := io.ReadAll(w.Body)
				require.NoError(t, err)
				require.Equal(t, tt.bytes, bytes)
				require.Equal(t, tt.mimeType, w.Header().Get("Content-Type"))
				require.Equal(t, http.StatusOK, w.Code)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms-file")
	noPermsDirPath := filepath.Join(tmpdir, "no-perms-dir")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := newTestFs(afero.NewMemMapFs(), func(call, file string) error {
		if file == noPermsFilePath || file == noPermsDirPath {
			return os.ErrPermission
		}
		return nil
	})
	api := agentfiles.NewAPI(logger, fs, nil)

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
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/write-file?path=%s", tt.path), reader)
			api.Routes().ServeHTTP(w, r)

			if tt.errCode != 0 {
				got := &codersdk.Error{}
				err := json.NewDecoder(w.Body).Decode(got)
				require.NoError(t, err)
				require.ErrorContains(t, got, tt.error)
				require.Equal(t, tt.errCode, w.Code)
			} else {
				bytes, err := afero.ReadFile(fs, tt.path)
				require.NoError(t, err)
				require.Equal(t, tt.bytes, bytes)
				require.Equal(t, http.StatusOK, w.Code)
			}
		})
	}
}

func TestWriteFile_ReportsIOError(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, nil)

	tmpdir := os.TempDir()
	path := filepath.Join(tmpdir, "write-io-error")
	err := afero.WriteFile(fs, path, []byte("original"), 0o644)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// A reader that always errors simulates a failed body read
	// (e.g. network interruption). The atomic write should leave
	// the original file intact.
	body := iotest.ErrReader(xerrors.New("simulated I/O error"))
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("/write-file?path=%s", path), body)
	api.Routes().ServeHTTP(w, r)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	got := &codersdk.Error{}
	err = json.NewDecoder(w.Body).Decode(got)
	require.NoError(t, err)
	require.ErrorContains(t, got, "simulated I/O error")

	// The original file must survive the failed write.
	data, err := afero.ReadFile(fs, path)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
}

func TestWriteFile_PreservesPermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	path := filepath.Join(dir, "script.sh")
	err := afero.WriteFile(osFs, path, []byte("#!/bin/sh\necho hello\n"), 0o755)
	require.NoError(t, err)

	info, err := osFs.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Overwrite the file with new content.
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("/write-file?path=%s", path),
		bytes.NewReader([]byte("#!/bin/sh\necho world\n")))
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	data, err := afero.ReadFile(osFs, path)
	require.NoError(t, err)
	require.Equal(t, "#!/bin/sh\necho world\n", string(data))

	info, err = osFs.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
		"write_file should preserve the original file's permissions")
}

func TestEditFiles(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms-file")
	failRenameFilePath := filepath.Join(tmpdir, "fail-rename")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := newTestFs(afero.NewMemMapFs(), func(call, file string) error {
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
	api := agentfiles.NewAPI(logger, fs, nil)

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
			// Original file must survive the failed rename.
			expected: map[string]string{failRenameFilePath: "foo bar"},
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
			// When the second edit creates ambiguity (two "bar"
			// occurrences), it should fail.
			name:     "EditEditAmbiguous",
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
			errCode: http.StatusBadRequest,
			errors:  []string{"matches 2 occurrences"},
			// File should not be modified on error.
			expected: map[string]string{filepath.Join(tmpdir, "edit-edit"): "foo bar"},
		},
		{
			// With replace_all the cascading edit replaces
			// both occurrences.
			name:     "EditEditReplaceAll",
			contents: map[string]string{filepath.Join(tmpdir, "edit-edit-ra"): "foo bar"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "edit-edit-ra"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "bar",
						},
						{
							Search:     "bar",
							Replace:    "qux",
							ReplaceAll: true,
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "edit-edit-ra"): "qux qux"},
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
			name:     "TrailingWhitespace",
			contents: map[string]string{filepath.Join(tmpdir, "trailing-ws"): "foo   \nbar\t\t\nbaz"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "trailing-ws"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo\nbar\nbaz",
							Replace: "replaced",
						},
					},
				},
			},
			// The file's trailing whitespace ("   " on line 1,
			// "\t\t" on line 2) agrees with both search and replace
			// (both have no trailing whitespace on their single
			// lines), so the splice preserves the file's trailing
			// whitespace. File's trailing whitespace on line 1 is
			// preserved; the replacement collapses to one line, so
			// lines 2 and 3 are consumed and only the first line's
			// trailing whitespace remains.
			expected: map[string]string{filepath.Join(tmpdir, "trailing-ws"): "replaced   "},
		},
		{
			name:     "TabsVsSpaces",
			contents: map[string]string{filepath.Join(tmpdir, "tabs-vs-spaces"): "\tif true {\n\t\tfoo()\n\t}"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "tabs-vs-spaces"),
					Edits: []workspacesdk.FileEdit{
						{
							// Search uses spaces but file uses tabs.
							Search:  "    if true {\n        foo()\n    }",
							Replace: "\tif true {\n\t\tbar()\n\t}",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "tabs-vs-spaces"): "\tif true {\n\t\tbar()\n\t}"},
		},
		{
			name:     "DifferentIndentDepth",
			contents: map[string]string{filepath.Join(tmpdir, "indent-depth"): "\t\t\tdeep()\n\t\t\tnested()"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "indent-depth"),
					Edits: []workspacesdk.FileEdit{
						{
							// Search has wrong indent depth (1 tab instead of 3).
							Search:  "\tdeep()\n\tnested()",
							Replace: "\t\t\tdeep()\n\t\t\tchanged()",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "indent-depth"): "\t\t\tdeep()\n\t\t\tchanged()"},
		},
		{
			name:     "ExactMatchPreferred",
			contents: map[string]string{filepath.Join(tmpdir, "exact-preferred"): "hello world"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "exact-preferred"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "hello world",
							Replace: "goodbye world",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "exact-preferred"): "goodbye world"},
		},
		{
			name:     "NoMatchErrors",
			contents: map[string]string{filepath.Join(tmpdir, "no-match"): "original content"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "no-match"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "this does not exist in the file",
							Replace: "whatever",
						},
					},
				},
			},
			errCode: http.StatusBadRequest,
			errors:  []string{"search string not found in file"},
			// File should remain unchanged.
			expected: map[string]string{filepath.Join(tmpdir, "no-match"): "original content"},
		},
		{
			name:     "AmbiguousExactMatch",
			contents: map[string]string{filepath.Join(tmpdir, "ambig-exact"): "foo bar foo baz foo"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "ambig-exact"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:  "foo",
							Replace: "qux",
						},
					},
				},
			},
			errCode:  http.StatusBadRequest,
			errors:   []string{"matches 3 occurrences"},
			expected: map[string]string{filepath.Join(tmpdir, "ambig-exact"): "foo bar foo baz foo"},
		},
		{
			name:     "ReplaceAllExact",
			contents: map[string]string{filepath.Join(tmpdir, "ra-exact"): "foo bar foo baz foo"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "ra-exact"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:     "foo",
							Replace:    "qux",
							ReplaceAll: true,
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "ra-exact"): "qux bar qux baz qux"},
		},
		{
			// replace_all with fuzzy trailing-whitespace match.
			name:     "ReplaceAllFuzzyTrailing",
			contents: map[string]string{filepath.Join(tmpdir, "ra-fuzzy-trail"): "hello   \nworld\nhello   \nagain"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "ra-fuzzy-trail"),
					Edits: []workspacesdk.FileEdit{
						{
							Search:     "hello\n",
							Replace:    "bye\n",
							ReplaceAll: true,
						},
					},
				},
			},
			// File trailing whitespace "   " on "hello   " lines is
			// preserved because search and replace agree on having
			// no trailing whitespace. Replace-all runs the same
			// per-position splice as single-replace.
			expected: map[string]string{filepath.Join(tmpdir, "ra-fuzzy-trail"): "bye   \nworld\nbye   \nagain"},
		},
		{
			// replace_all with fuzzy indent match (pass 3).
			name:     "ReplaceAllFuzzyIndent",
			contents: map[string]string{filepath.Join(tmpdir, "ra-fuzzy-indent"): "\t\talpha\n\t\tbeta\n\t\talpha\n\t\tgamma"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "ra-fuzzy-indent"),
					Edits: []workspacesdk.FileEdit{
						{
							// Search uses different indentation (spaces instead of tabs).
							Search:     "    alpha\n",
							Replace:    "\t\tREPLACED\n",
							ReplaceAll: true,
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "ra-fuzzy-indent"): "\t\tREPLACED\n\t\tbeta\n\t\tREPLACED\n\t\tgamma"},
		},
		{
			name:     "MixedWhitespaceMultiline",
			contents: map[string]string{filepath.Join(tmpdir, "mixed-ws"): "func main() {\n\tresult := compute()\n\tfmt.Println(result)\n}"},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "mixed-ws"),
					Edits: []workspacesdk.FileEdit{
						{
							// Search uses spaces, file uses tabs.
							Search:  "  result := compute()\n  fmt.Println(result)\n",
							Replace: "\tresult := compute()\n\tlog.Println(result)\n",
						},
					},
				},
			},
			expected: map[string]string{filepath.Join(tmpdir, "mixed-ws"): "func main() {\n\tresult := compute()\n\tlog.Println(result)\n}"},
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
			// No files should be modified when any edit fails
			// (atomic multi-file semantics).
			expected: map[string]string{
				filepath.Join(tmpdir, "file8"): "file 8",
			},
			// Higher status codes will override lower ones, so in this case the 404
			// takes priority over the 403.
			errCode: http.StatusNotFound,
			errors: []string{
				fmt.Sprintf("%s: permission denied", noPermsFilePath),
				"file9: file does not exist",
			},
		},
		{
			// Valid edits on files A and C, but file B has a
			// search miss. None should be written.
			name: "AtomicMultiFile_OneFailsNoneWritten",
			contents: map[string]string{
				filepath.Join(tmpdir, "atomic-a"): "aaa",
				filepath.Join(tmpdir, "atomic-b"): "bbb",
				filepath.Join(tmpdir, "atomic-c"): "ccc",
			},
			edits: []workspacesdk.FileEdits{
				{
					Path: filepath.Join(tmpdir, "atomic-a"),
					Edits: []workspacesdk.FileEdit{
						{Search: "aaa", Replace: "AAA"},
					},
				},
				{
					Path: filepath.Join(tmpdir, "atomic-b"),
					Edits: []workspacesdk.FileEdit{
						{Search: "NOTFOUND", Replace: "XXX"},
					},
				},
				{
					Path: filepath.Join(tmpdir, "atomic-c"),
					Edits: []workspacesdk.FileEdit{
						{Search: "ccc", Replace: "CCC"},
					},
				},
			},
			errCode: http.StatusBadRequest,
			errors:  []string{"search string not found"},
			expected: map[string]string{
				filepath.Join(tmpdir, "atomic-a"): "aaa",
				filepath.Join(tmpdir, "atomic-b"): "bbb",
				filepath.Join(tmpdir, "atomic-c"): "ccc",
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

			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			err := enc.Encode(workspacesdk.FileEditRequest{Files: tt.edits})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
			api.Routes().ServeHTTP(w, r)

			if tt.errCode != 0 {
				got := &codersdk.Error{}
				err := json.NewDecoder(w.Body).Decode(got)
				require.NoError(t, err)
				for _, error := range tt.errors {
					require.ErrorContains(t, got, error)
				}
				require.Equal(t, tt.errCode, w.Code)
			} else {
				require.Equal(t, http.StatusOK, w.Code)
			}
			for path, expect := range tt.expected {
				b, err := afero.ReadFile(fs, path)
				require.NoError(t, err)
				require.Equal(t, expect, string(b))
			}
		})
	}
}

func TestEditFiles_PreservesPermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	path := filepath.Join(dir, "script.sh")
	err := afero.WriteFile(osFs, path, []byte("#!/bin/sh\necho hello\n"), 0o755)
	require.NoError(t, err)

	// Sanity-check the initial mode.
	info, err := osFs.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	body := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{
			{
				Path: path,
				Edits: []workspacesdk.FileEdit{
					{
						Search:  "hello",
						Replace: "world",
					},
				},
			},
		},
	}
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err = enc.Encode(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify content was updated.
	data, err := afero.ReadFile(osFs, path)
	require.NoError(t, err)
	require.Equal(t, "#!/bin/sh\necho world\n", string(data))

	// Verify permissions are preserved after the
	// temp-file-and-rename cycle.
	info, err = osFs.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
		"edit_files should preserve the original file's permissions")
}

func TestHandleWriteFile_ChatHeaders_UpdatesPathStore(t *testing.T) {
	t.Parallel()

	pathStore := agentgit.NewPathStore()
	logger := slogtest.Make(t, nil)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, pathStore)

	testPath := filepath.Join(os.TempDir(), "test.txt")

	chatID := uuid.New()
	ancestorID := uuid.New()
	ancestorJSON, _ := json.Marshal([]string{ancestorID.String()})

	body := strings.NewReader("hello world")
	req := httptest.NewRequest(http.MethodPost, "/write-file?path="+testPath, body)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
	req.Header.Set(workspacesdk.CoderAncestorChatIDsHeader, string(ancestorJSON))

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/write-file", api.HandleWriteFile)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// Verify PathStore was updated for both chat and ancestor.
	paths := pathStore.GetPaths(chatID)
	require.Equal(t, []string{testPath}, paths)

	ancestorPaths := pathStore.GetPaths(ancestorID)
	require.Equal(t, []string{testPath}, ancestorPaths)
}

func TestHandleWriteFile_NoChatHeaders_NoPathStoreUpdate(t *testing.T) {
	t.Parallel()

	pathStore := agentgit.NewPathStore()
	logger := slogtest.Make(t, nil)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, pathStore)

	testPath := filepath.Join(os.TempDir(), "test.txt")

	body := strings.NewReader("hello world")
	req := httptest.NewRequest(http.MethodPost, "/write-file?path="+testPath, body)

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/write-file", api.HandleWriteFile)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	// PathStore should be globally empty since no chat headers were set.
	require.Equal(t, 0, pathStore.Len())
}

func TestHandleWriteFile_Failure_NoPathStoreUpdate(t *testing.T) {
	t.Parallel()

	pathStore := agentgit.NewPathStore()
	logger := slogtest.Make(t, nil)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, pathStore)

	chatID := uuid.New()

	// Write to a relative path (should fail with 400).
	body := strings.NewReader("hello world")
	req := httptest.NewRequest(http.MethodPost, "/write-file?path=relative/path.txt", body)
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/write-file", api.HandleWriteFile)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	// PathStore should NOT be updated on failure.
	paths := pathStore.GetPaths(chatID)
	require.Empty(t, paths)
}

func TestHandleEditFiles_ChatHeaders_UpdatesPathStore(t *testing.T) {
	t.Parallel()

	pathStore := agentgit.NewPathStore()
	logger := slogtest.Make(t, nil)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, pathStore)

	testPath := filepath.Join(os.TempDir(), "test.txt")

	// Create the file first.
	require.NoError(t, afero.WriteFile(fs, testPath, []byte("hello"), 0o644))

	chatID := uuid.New()
	editReq := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{
			{
				Path: testPath,
				Edits: []workspacesdk.FileEdit{
					{Search: "hello", Replace: "world"},
				},
			},
		},
	}
	body, _ := json.Marshal(editReq)
	req := httptest.NewRequest(http.MethodPost, "/edit-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/edit-files", api.HandleEditFiles)
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	paths := pathStore.GetPaths(chatID)
	require.Equal(t, []string{testPath}, paths)
}

func TestHandleEditFiles_Failure_NoPathStoreUpdate(t *testing.T) {
	t.Parallel()

	pathStore := agentgit.NewPathStore()
	logger := slogtest.Make(t, nil)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, pathStore)

	chatID := uuid.New()

	// Edit a non-existent file (should fail with 404).
	editReq := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{
			{
				Path: "/nonexistent/file.txt",
				Edits: []workspacesdk.FileEdit{
					{Search: "hello", Replace: "world"},
				},
			},
		},
	}
	body, _ := json.Marshal(editReq)
	req := httptest.NewRequest(http.MethodPost, "/edit-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/edit-files", api.HandleEditFiles)
	r.ServeHTTP(rr, req)

	require.NotEqual(t, http.StatusOK, rr.Code)

	// PathStore should NOT be updated on failure.
	paths := pathStore.GetPaths(chatID)
	require.Empty(t, paths)
}

func TestReadFileLines(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	noPermsFilePath := filepath.Join(tmpdir, "no-perms-lines")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := newTestFs(afero.NewMemMapFs(), func(call, file string) error {
		if file == noPermsFilePath {
			return os.ErrPermission
		}
		return nil
	})
	api := agentfiles.NewAPI(logger, fs, nil)

	dirPath := filepath.Join(tmpdir, "a-directory-lines")
	err := fs.MkdirAll(dirPath, 0o755)
	require.NoError(t, err)

	emptyFilePath := filepath.Join(tmpdir, "empty-file")
	err = afero.WriteFile(fs, emptyFilePath, []byte(""), 0o644)
	require.NoError(t, err)

	basicFilePath := filepath.Join(tmpdir, "basic-file")
	err = afero.WriteFile(fs, basicFilePath, []byte("line1\nline2\nline3"), 0o644)
	require.NoError(t, err)

	longLine := string(bytes.Repeat([]byte("x"), 1025))
	longLineFilePath := filepath.Join(tmpdir, "long-line-file")
	err = afero.WriteFile(fs, longLineFilePath, []byte(longLine), 0o644)
	require.NoError(t, err)

	largeFilePath := filepath.Join(tmpdir, "large-file")
	err = afero.WriteFile(fs, largeFilePath, bytes.Repeat([]byte("x"), 1<<20+1), 0o644)
	require.NoError(t, err)

	tests := []struct {
		name       string
		path       string
		offset     int64
		limit      int64
		expSuccess bool
		expError   string
		expContent string
		expTotal   int
		expRead    int
		expSize    int64
		// useCodersdk is set for cases where the handler returns
		// codersdk.Response (query param validation) instead of ReadFileLinesResponse.
		useCodersdk bool
	}{
		{
			name:        "NoPath",
			path:        "",
			useCodersdk: true,
			expError:    "is required",
		},
		{
			name:     "RelativePath",
			path:     "relative/path",
			expError: "file path must be absolute",
		},
		{
			name:     "NonExistent",
			path:     filepath.Join(tmpdir, "does-not-exist"),
			expError: "file does not exist",
		},
		{
			name:     "IsDir",
			path:     dirPath,
			expError: "not a file",
		},
		{
			name:     "NoPermissions",
			path:     noPermsFilePath,
			expError: "permission denied",
		},
		{
			name:       "EmptyFile",
			path:       emptyFilePath,
			expSuccess: true,
			expTotal:   0,
			expRead:    0,
			expSize:    0,
		},
		{
			name:       "BasicRead",
			path:       basicFilePath,
			expSuccess: true,
			expContent: "1\tline1\n2\tline2\n3\tline3",
			expTotal:   3,
			expRead:    3,
			expSize:    int64(len("line1\nline2\nline3")),
		},
		{
			name:       "Offset2",
			path:       basicFilePath,
			offset:     2,
			expSuccess: true,
			expContent: "2\tline2\n3\tline3",
			expTotal:   3,
			expRead:    2,
			expSize:    int64(len("line1\nline2\nline3")),
		},
		{
			name:       "Limit1",
			path:       basicFilePath,
			limit:      1,
			expSuccess: true,
			expContent: "1\tline1",
			expTotal:   3,
			expRead:    1,
			expSize:    int64(len("line1\nline2\nline3")),
		},
		{
			name:       "Offset2Limit1",
			path:       basicFilePath,
			offset:     2,
			limit:      1,
			expSuccess: true,
			expContent: "2\tline2",
			expTotal:   3,
			expRead:    1,
			expSize:    int64(len("line1\nline2\nline3")),
		},
		{
			name:     "OffsetBeyondFile",
			path:     basicFilePath,
			offset:   100,
			expError: "offset 100 is beyond the file length of 3 lines",
		},
		{
			name:       "LongLineTruncation",
			path:       longLineFilePath,
			expSuccess: true,
			expContent: "1\t" + string(bytes.Repeat([]byte("x"), 1024)) + "... [truncated]",
			expTotal:   1,
			expRead:    1,
			expSize:    1025,
		},
		{
			name:     "LargeFile",
			path:     largeFilePath,
			expError: "exceeds the maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/read-file-lines?path=%s&offset=%d&limit=%d", tt.path, tt.offset, tt.limit), nil)
			api.Routes().ServeHTTP(w, r)

			if tt.useCodersdk {
				// Query param validation errors return codersdk.Response.
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Contains(t, w.Body.String(), tt.expError)
				return
			}

			var resp agentfiles.ReadFileLinesResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			if tt.expSuccess {
				require.Equal(t, http.StatusOK, w.Code)
				require.True(t, resp.Success)
				require.Equal(t, tt.expContent, resp.Content)
				require.Equal(t, tt.expTotal, resp.TotalLines)
				require.Equal(t, tt.expRead, resp.LinesRead)
				require.Equal(t, tt.expSize, resp.FileSize)
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				require.False(t, resp.Success)
				require.Contains(t, resp.Error, tt.expError)
			}
		})
	}
}

func TestWriteFile_FollowsSymlinks(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	// Create a real file and a symlink pointing to it.
	realPath := filepath.Join(dir, "real.txt")
	err := afero.WriteFile(osFs, realPath, []byte("original"), 0o644)
	require.NoError(t, err)

	linkPath := filepath.Join(dir, "link.txt")
	err = os.Symlink(realPath, linkPath)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Write through the symlink.
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("/write-file?path=%s", linkPath),
		bytes.NewReader([]byte("updated")))
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	// The symlink must still be a symlink.
	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "symlink was replaced")

	// The real file must have the new content.
	data, err := os.ReadFile(realPath)
	require.NoError(t, err)
	require.Equal(t, "updated", string(data))
}

func TestEditFiles_FollowsSymlinks(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliably supported on Windows")
	}

	dir := t.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	// Create a real file and a symlink pointing to it.
	realPath := filepath.Join(dir, "real.txt")
	err := afero.WriteFile(osFs, realPath, []byte("hello world"), 0o644)
	require.NoError(t, err)

	linkPath := filepath.Join(dir, "link.txt")
	err = os.Symlink(realPath, linkPath)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	body := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{
			{
				Path: linkPath,
				Edits: []workspacesdk.FileEdit{
					{
						Search:  "hello",
						Replace: "goodbye",
					},
				},
			},
		},
	}
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err = enc.Encode(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	// The symlink must still be a symlink.
	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "symlink was replaced")

	// The real file must have the edited content.
	data, err := os.ReadFile(realPath)
	require.NoError(t, err)
	require.Equal(t, "goodbye world", string(data))
}

func TestEditFiles_FileResults(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	t.Run("DiffRequestedSingleFile", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		api := agentfiles.NewAPI(logger, fs, nil)
		path := filepath.Join(tmpdir, "diff-single")
		require.NoError(t, afero.WriteFile(fs, path, []byte("hello world\n"), 0o644))

		resp := runEditFiles(t, api, workspacesdk.FileEditRequest{
			IncludeDiff: true,
			Files: []workspacesdk.FileEdits{
				{
					Path: path,
					Edits: []workspacesdk.FileEdit{
						{Search: "hello", Replace: "HELLO"},
					},
				},
			},
		})
		require.Len(t, resp.Files, 1)
		require.Equal(t, path, resp.Files[0].Path)
		// udiff.Unified emits "--- <path>\n+++ <path>\n@@ ...".
		require.Contains(t, resp.Files[0].Diff, "--- "+path+"\n")
		require.Contains(t, resp.Files[0].Diff, "+++ "+path+"\n")
		require.Contains(t, resp.Files[0].Diff, "-hello world")
		require.Contains(t, resp.Files[0].Diff, "+HELLO world")
	})

	t.Run("DiffRequestedNoOpEdit", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		api := agentfiles.NewAPI(logger, fs, nil)
		path := filepath.Join(tmpdir, "diff-noop")
		require.NoError(t, afero.WriteFile(fs, path, []byte("same\n"), 0o644))

		resp := runEditFiles(t, api, workspacesdk.FileEditRequest{
			IncludeDiff: true,
			Files: []workspacesdk.FileEdits{
				{
					Path: path,
					Edits: []workspacesdk.FileEdit{
						// Replace with identical text (no-op).
						{Search: "same", Replace: "same"},
					},
				},
			},
		})
		require.Len(t, resp.Files, 1)
		require.Equal(t, path, resp.Files[0].Path)
		require.Empty(t, resp.Files[0].Diff, "no-op edit produces empty diff")
	})

	t.Run("DiffNotRequested", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		api := agentfiles.NewAPI(logger, fs, nil)
		path := filepath.Join(tmpdir, "diff-off")
		require.NoError(t, afero.WriteFile(fs, path, []byte("hello\n"), 0o644))

		resp := runEditFiles(t, api, workspacesdk.FileEditRequest{
			// IncludeDiff omitted; default false.
			Files: []workspacesdk.FileEdits{
				{
					Path: path,
					Edits: []workspacesdk.FileEdit{
						{Search: "hello", Replace: "HELLO"},
					},
				},
			},
		})
		require.Nil(t, resp.Files, "Files must be nil when IncludeDiff is false")
	})

	t.Run("DiffRequestedMultiFilePreservesOrder", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		api := agentfiles.NewAPI(logger, fs, nil)
		pathA := filepath.Join(tmpdir, "diff-multi-a")
		pathB := filepath.Join(tmpdir, "diff-multi-b")
		pathC := filepath.Join(tmpdir, "diff-multi-c")
		require.NoError(t, afero.WriteFile(fs, pathA, []byte("A\n"), 0o644))
		require.NoError(t, afero.WriteFile(fs, pathB, []byte("B\n"), 0o644))
		require.NoError(t, afero.WriteFile(fs, pathC, []byte("C\n"), 0o644))

		resp := runEditFiles(t, api, workspacesdk.FileEditRequest{
			IncludeDiff: true,
			Files: []workspacesdk.FileEdits{
				{Path: pathA, Edits: []workspacesdk.FileEdit{{Search: "A", Replace: "a"}}},
				{Path: pathB, Edits: []workspacesdk.FileEdit{{Search: "B", Replace: "b"}}},
				{Path: pathC, Edits: []workspacesdk.FileEdit{{Search: "C", Replace: "c"}}},
			},
		})
		require.Len(t, resp.Files, 3)
		expected := []struct {
			path    string
			oldLine string
			newLine string
		}{
			{pathA, "-A", "+a"},
			{pathB, "-B", "+b"},
			{pathC, "-C", "+c"},
		}
		for i, want := range expected {
			require.Equal(t, want.path, resp.Files[i].Path)
			require.NotEmpty(t, resp.Files[i].Diff, "file %d (%s) has empty diff", i, want.path)
			require.Contains(t, resp.Files[i].Diff, want.oldLine)
			require.Contains(t, resp.Files[i].Diff, want.newLine)
		}
	})

	t.Run("DiffRequestedMultiEditSameFile", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		api := agentfiles.NewAPI(logger, fs, nil)
		path := filepath.Join(tmpdir, "diff-multi-edit")
		require.NoError(t, afero.WriteFile(fs, path, []byte("one\ntwo\nthree\n"), 0o644))

		resp := runEditFiles(t, api, workspacesdk.FileEditRequest{
			IncludeDiff: true,
			Files: []workspacesdk.FileEdits{{
				Path: path,
				Edits: []workspacesdk.FileEdit{
					{Search: "one", Replace: "ONE"},
					{Search: "three", Replace: "THREE"},
				},
			}},
		})
		require.Len(t, resp.Files, 1)
		require.Equal(t, path, resp.Files[0].Path)
		// Both edits must appear in the diff, computed against the
		// file's original content (not the post-first-edit content).
		require.Contains(t, resp.Files[0].Diff, "-one")
		require.Contains(t, resp.Files[0].Diff, "+ONE")
		require.Contains(t, resp.Files[0].Diff, "-three")
		require.Contains(t, resp.Files[0].Diff, "+THREE")
	})
	t.Run("DiffRequestedSymlinkReportsOriginalPath", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("symlinks are not reliably supported on Windows")
		}

		dir := t.TempDir()
		osFs := afero.NewOsFs()
		api := agentfiles.NewAPI(logger, osFs, nil)

		realPath := filepath.Join(dir, "real.txt")
		require.NoError(t, afero.WriteFile(osFs, realPath, []byte("hello\n"), 0o644))

		linkPath := filepath.Join(dir, "link.txt")
		require.NoError(t, os.Symlink(realPath, linkPath))

		resp := runEditFiles(t, api, workspacesdk.FileEditRequest{
			IncludeDiff: true,
			Files: []workspacesdk.FileEdits{
				{
					Path: linkPath,
					Edits: []workspacesdk.FileEdit{
						{Search: "hello", Replace: "HELLO"},
					},
				},
			},
		})
		require.Len(t, resp.Files, 1)
		// The response must report the caller-supplied path, not the
		// symlink-resolved target.
		require.Equal(t, linkPath, resp.Files[0].Path)
		require.Contains(t, resp.Files[0].Diff, "--- "+linkPath+"\n")
		require.Contains(t, resp.Files[0].Diff, "+++ "+linkPath+"\n")
	})
}

// runEditFiles issues a single POST /edit-files call against api and
// decodes the success body into FileEditResponse. It requires a 200
// response; tests for error paths should decode the error shape
// directly.
func runEditFiles(t *testing.T, api *agentfiles.API, req workspacesdk.FileEditRequest) workspacesdk.FileEditResponse {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(req))

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
	api.Routes().ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp workspacesdk.FileEditResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// TestFuzzyReplace_EndingAndWhitespace exercises the line-endings
// and per-position whitespace behavior of the fuzzy matcher in
// both single-replace and replace-all modes.
//
// Match rule: content and search lines are compared after
// splitting off trailing (pass 2) or surrounding (pass 3)
// whitespace. The line ending is compared separately: identical,
// "\n" and "\r\n" are interchangeable, and an empty ending (EOF,
// no terminator on a line) matches any ending.
//
// Splice rule: for every matched line, the replacement's leading
// whitespace, trailing whitespace, and line ending are substituted
// with the matched content line's equivalents *when search and
// replace agree* at that position. Disagreement at a position
// means the caller wants to change that position explicitly, and
// the replacement's bytes win there.
//
// Pass 1 (byte-literal substring match) is untouched; tests that
// exercise it are noted.
func TestFuzzyReplace_EndingAndWhitespace(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	type edit struct {
		search, replace string
		replaceAll      bool
	}
	tests := []struct {
		name     string
		content  string
		edits    []edit
		expected string
	}{
		// CRLF file, LF search: the ending rule lets "line\n"
		// match "line\r\n"; the replacement is empty so the
		// matched line is removed entirely.
		{
			name:     "CRLF_Content_LFSearch_Delete",
			content:  "foo\r\nline\r\nbar\r\n",
			edits:    []edit{{search: "line\n", replace: ""}},
			expected: "foo\r\nbar\r\n",
		},
		// Pass 2 tolerates the file's trailing whitespace on
		// the matched line when search omits it. Empty
		// replacement removes the line.
		{
			name:     "TrailingWhitespace_Delete",
			content:  "foo\nline   \nbar\n",
			edits:    []edit{{search: "line\n", replace: ""}},
			expected: "foo\nbar\n",
		},
		// Pass 1 handles a search without a trailing newline
		// when the content contains an exact substring match:
		// strings.Replace preserves the surrounding "\n" bytes
		// verbatim.
		{
			name:     "Pass1_SearchNoNewline_ExactSubstring",
			content:  "foo\nfirst line\nbar\n",
			edits:    []edit{{search: "first line", replace: "LINE"}},
			expected: "foo\nLINE\nbar\n",
		},
		// Fuzzy path, both search and replace lack a newline
		// ending AND share a trailing space. The empty ending
		// on search is a wildcard against content's "\n";
		// pass 2's content comparator ignores the shared
		// trailing space to match "key". At splice time,
		// search and replace agree on the trailing space so
		// the file's lack of trailing whitespace wins; search
		// and replace agree on empty ending so the file's
		// "\n" wins.
		{
			name:     "FuzzyMatchingWhitespace_FileEndingWins",
			content:  "foo\nkey\nbar\n",
			edits:    []edit{{search: "key ", replace: "KEY "}},
			expected: "foo\nKEY\nbar\n",
		},
		// Last-line-no-newline uses pass 1 exact match.
		{
			name:     "Pass1_LastLineNoNewline",
			content:  "foo\nbar",
			edits:    []edit{{search: "bar", replace: "BAR"}},
			expected: "foo\nBAR",
		},
		// Indent-tolerant matching on a CRLF file: search and
		// replace disagree with the file on indent, so passes 1
		// and 2 fail; pass 3 (TrimSpace) matches on body. The
		// splice then decides each position by whether search
		// and replace agree with each other. These three cases
		// vary the caller-side whitespace to enumerate the
		// mechanism:
		//
		//   - when the caller agrees with itself on leading
		//     whitespace, the file's tab wins regardless of
		//     the space count on the caller side;
		//   - when the caller disagrees with itself (search
		//     leads with one thing, replace with another), the
		//     replacement's leading whitespace wins. That's the
		//     escape hatch for intentional indent rewrites.
		//
		// Endings always agree (both newline-class), so the
		// file's "\r\n" wins at every emitted line.
		{
			name:     "FuzzyIndent_CRLF_TwoSpaceSearch_FileTabWins",
			content:  "foo\r\n\tline\r\nbar\r\n",
			edits:    []edit{{search: "  line\n", replace: "  LINE\n"}},
			expected: "foo\r\n\tLINE\r\nbar\r\n",
		},
		{
			name:     "FuzzyIndent_CRLF_SevenSpaceSearch_FileTabStillWins",
			content:  "foo\r\n\tline\r\nbar\r\n",
			edits:    []edit{{search: "       line\n", replace: "       LINE\n"}},
			expected: "foo\r\n\tLINE\r\nbar\r\n",
		},
		{
			name:     "FuzzyIndent_CRLF_CallerRewritesIndent_ReplaceLeadingWins",
			content:  "foo\r\n\tline\r\nbar\r\n",
			edits:    []edit{{search: "  line\n", replace: "    LINE\n"}},
			expected: "foo\r\n    LINE\r\nbar\r\n",
		},

		// Replace-all must run through the same per-position
		// splice as single-replace.
		{
			// Every matched line keeps the file's trailing
			// whitespace shape (""), and its "\n" ending.
			name:     "ReplaceAll_FuzzyMatchingWhitespace_FileEndingWins",
			content:  "key\nkey\nother\n",
			edits:    []edit{{search: "key ", replace: "KEY ", replaceAll: true}},
			expected: "KEY\nKEY\nother\n",
		},
		{
			// CRLF file, LF search/replace: every splice uses
			// the file's "\r\n" so the output is uniformly CRLF.
			name:     "ReplaceAll_CRLF_LFSearch_FileEndingWins",
			content:  "line one\r\nother\r\nline one\r\n",
			edits:    []edit{{search: "line one\n", replace: "LINE\n", replaceAll: true}},
			expected: "LINE\r\nother\r\nLINE\r\n",
		},

		// Caller explicitly folds: the search has a newline
		// ending, the replace omits it. Disagreement at the
		// ending position means the replace's empty ending
		// wins, so the next content line folds in. Pass 1
		// handles this as a byte-literal match.
		{
			name:     "CallerChosenFold",
			content:  "foo\nline\nbar\n",
			edits:    []edit{{search: "line\n", replace: "LINE"}},
			expected: "foo\nLINEbar\n",
		},

		// Caller deliberately rewrites indent: search leads with
		// a tab, replace leads with two spaces. Disagreement on
		// the leading-whitespace position means the replacement's
		// spaces win on the edited line. The untouched following
		// line keeps its tab.
		{
			name:     "CallerRewritesIndent_ReplaceLeadingWins",
			content:  "foo\n\tline\n\tbar\n",
			edits:    []edit{{search: "\tline\n", replace: "  line\n"}},
			expected: "foo\n  line\n\tbar\n",
		},

		// Expansion: replace has more lines than the matched
		// region. Extras reference the last paired search/content
		// line, so an extra whose leading whitespace agrees with
		// the last paired search line picks up the file's
		// leading whitespace. Search uses 4 spaces to force the
		// fuzzy path (pass 1 would splice verbatim).
		{
			name:     "Expansion_ExtraLinesTrackLastPair",
			content:  "foo\n\tline\nbar\n",
			edits:    []edit{{search: "    line\n", replace: "    line\n    extra\n"}},
			expected: "foo\n\tline\n\textra\nbar\n",
		},

		// Collapse: replace has fewer lines than the matched
		// region. Unpaired matched lines are consumed without
		// output.
		{
			name:     "Collapse_ReplaceShorterThanSearch",
			content:  "foo\nkeep\ndrop\nbar\n",
			edits:    []edit{{search: "keep\ndrop\n", replace: "keep\n"}},
			expected: "foo\nkeep\nbar\n",
		},

		// Empty-ending wildcard: search has no trailing newline
		// and leading whitespace that isn't in the file. Pass 1
		// fails (the leading spaces aren't a substring). Pass 3
		// (trim-all) matches. At the splice: search and replace
		// both have empty endings, so endingShapeEqual agrees
		// and the file's "\r\n" wins. The file's leading tab
		// does not win because sLead="  " disagrees with
		// rLead="", so the replacement's empty lead wins.
		{
			name:     "EmptyEndingWildcard_CRLFContent_FileEndingWins",
			content:  "foo\r\nkey\r\nbar\r\n",
			edits:    []edit{{search: "  key", replace: "KEY"}},
			expected: "foo\r\nKEY\r\nbar\r\n",
		},

		// Multi-line replacement at EOF without trailing newline.
		// The reference content line at the last index has
		// cEnd="", but interior replacement lines must keep their
		// "\n" rather than inherit the empty ending.
		{
			name:     "MultiLineReplaceAtEOFNoNewline_InteriorLinesKeepNewline",
			content:  "foo\nbar",
			edits:    []edit{{search: "foo\nbar\n", replace: "foo\nbaz\nqux\n"}},
			expected: "foo\nbaz\nqux",
		},

		// Empty replacement body must not inherit the file's
		// surrounding whitespace. Search forces the fuzzy path
		// via trimming; replace is a single blank line.
		{
			name:     "EmptyBodyFuzzyReplace_NoWhitespaceGhost",
			content:  "prefix\n  code  \nsuffix\n",
			edits:    []edit{{search: "code\n", replace: "\n"}},
			expected: "prefix\n\nsuffix\n",
		},

		// Combined: multi-line replacement at EOF without a
		// newline, with an interior empty-body line. Exercises
		// both carve-outs in one splice: the empty-body line
		// must not inherit file whitespace, and interior lines
		// must keep their newline even though the reference
		// content line has cEnd="".
		{
			name:     "EmptyBodyInteriorAtEOFNoNewline_BothCarveOuts",
			content:  "foo\nbar",
			edits:    []edit{{search: "foo\nbar\n", replace: "mid1\n\nmid2\n"}},
			expected: "mid1\n\nmid2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			api := agentfiles.NewAPI(logger, fs, nil)
			path := filepath.Join(tmpdir, "fuzzy-"+tt.name)
			require.NoError(t, afero.WriteFile(fs, path, []byte(tt.content), 0o644))

			sdkEdits := make([]workspacesdk.FileEdit, 0, len(tt.edits))
			for _, e := range tt.edits {
				sdkEdits = append(sdkEdits, workspacesdk.FileEdit{
					Search:     e.search,
					Replace:    e.replace,
					ReplaceAll: e.replaceAll,
				})
			}
			req := workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{Path: path, Edits: sdkEdits}},
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(req))
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
			api.Routes().ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			data, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(data))
		})
	}
}

// TestFuzzyReplace_EndingNormalization pins the line-ending rule.
//
// Rule: every spliced line gets the file's dominant ending, except
// when the caller signaled intent by making search and replace
// disagree on internal endings (both non-empty, different). Intent
// requires pass 1 to byte-match the file's endings; if it does,
// replace's endings are honored per-line. When only one side has
// internal endings (single-line vs. multi-line), the file wins.
//
// No-EOL at EOF is preserved: the final spliced line keeps its
// ending, so a match covering the file's last line does not
// materialize a newline the file never had.
func TestFuzzyReplace_EndingNormalization(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	type edit struct {
		search, replace string
		replaceAll      bool
	}
	tests := []struct {
		name     string
		content  string
		edits    []edit
		expected string
	}{
		// CRLF file, LF search, LF replace with expansion.
		// Internal endings agree (both LF), rule fires, every
		// spliced line becomes CRLF.
		{
			name:     "CRLFFile_LFSearchReplace_Expansion",
			content:  "line1\r\nline2\r\nline3\r\n",
			edits:    []edit{{search: "line1\nline2\n", replace: "line1\nINSERTED\nline2\n"}},
			expected: "line1\r\nINSERTED\r\nline2\r\nline3\r\n",
		},
		// CRLF file with no trailing newline, LF search/replace
		// with expansion that covers the file's last line. Interior
		// spliced lines become CRLF; final spliced line preserves
		// the file's no-EOL property.
		{
			name:     "CRLFFileNoEOL_LFSearchReplace_ExpansionAtEOF",
			content:  "alpha\r\nbeta\r\ngamma",
			edits:    []edit{{search: "gamma", replace: "gamma\ndelta\nepsilon"}},
			expected: "alpha\r\nbeta\r\ngamma\r\ndelta\r\nepsilon",
		},
		// CRLF Go file with no final newline; LLM sends LF
		// search/replace that expands the function body. This is
		// the motivating real-world case for the rule.
		{
			name:     "CRLFFileNoEOL_LFCallerExpandsFunctionBody",
			content:  "package main\r\n\r\nfunc main() {\r\n\tprintln(\"hi\")\r\n}",
			edits:    []edit{{search: "\tprintln(\"hi\")\n}", replace: "\tprintln(\"hi\")\n\tprintln(\"bye\")\n\treturn\n}"}},
			expected: "package main\r\n\r\nfunc main() {\r\n\tprintln(\"hi\")\r\n\tprintln(\"bye\")\r\n\treturn\r\n}",
		},
		// LF file, CRLF search/replace (caller sent CRLF, file is
		// LF). Internal endings agree (both CRLF). Rule fires, the
		// file's LF wins.
		{
			name:     "LFFile_CRLFSearchReplace_FileLFWins",
			content:  "one\ntwo\nthree\n",
			edits:    []edit{{search: "one\r\ntwo\r\n", replace: "ONE\r\nTWO\r\n"}},
			expected: "ONE\nTWO\nthree\n",
		},
		// Caller got endings right: CRLF in search, replace, and file.
		// Pins that normalization doesn't regress this happy path.
		{
			name:     "CRLFFile_CRLFSearchReplace_SanityPreserved",
			content:  "a\r\nb\r\nc\r\n",
			edits:    []edit{{search: "a\r\nb\r\n", replace: "A\r\nB\r\n"}},
			expected: "A\r\nB\r\nc\r\n",
		},
		// ReplaceAll with expansion on a CRLF file via LF caller.
		// Every spliced region must be CRLF throughout.
		{
			name:    "ReplaceAll_CRLFFile_LFCaller_Expansion",
			content: "key\r\nother\r\nkey\r\n",
			edits: []edit{{
				search:     "key\n",
				replace:    "KEY\nEXTRA\n",
				replaceAll: true,
			}},
			expected: "KEY\r\nEXTRA\r\nother\r\nKEY\r\nEXTRA\r\n",
		},
		// Caller sent CRLF search and LF replace against a CRLF
		// file. Different ending styles between search and replace
		// signal caller intent to change endings. Search's CRLF
		// byte-matches the file's CRLF, so the match succeeds and
		// replace's LF endings are honored per-line. The untouched
		// trailing line keeps its CRLF.
		{
			name:     "CallerIntent_SearchMatchesFile_ReplaceEndingsHonored",
			content:  "x\r\ny\r\nz\r\n",
			edits:    []edit{{search: "x\r\ny\r\n", replace: "X\nY\n"}},
			expected: "X\nY\nz\r\n",
		},
		// Single-line search against a CRLF file, multi-line
		// replace. Search has no endings, so no caller intent is
		// signaled and the file's CRLF wins for every spliced line.
		{
			name:     "SingleLineSearch_MultiLineReplace_FileEndingWins",
			content:  "a\r\nx\r\nb\r\n",
			edits:    []edit{{search: "x", replace: "X\nY"}},
			expected: "a\r\nX\r\nY\r\nb\r\n",
		},
		// Trivial baseline: neither side has endings, nothing to
		// normalize.
		{
			name:     "SingleLineSearch_SingleLineReplace_NoEndingsToNormalize",
			content:  "a\r\nx\r\nb\r\n",
			edits:    []edit{{search: "x", replace: "X"}},
			expected: "a\r\nX\r\nb\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			api := agentfiles.NewAPI(logger, fs, nil)
			path := filepath.Join(tmpdir, "endnorm-"+tt.name)
			require.NoError(t, afero.WriteFile(fs, path, []byte(tt.content), 0o644))

			sdkEdits := make([]workspacesdk.FileEdit, 0, len(tt.edits))
			for _, e := range tt.edits {
				sdkEdits = append(sdkEdits, workspacesdk.FileEdit{
					Search:     e.search,
					Replace:    e.replace,
					ReplaceAll: e.replaceAll,
				})
			}
			req := workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{Path: path, Edits: sdkEdits}},
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(req))
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
			api.Routes().ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			data, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(data))
		})
	}
}

// TestFuzzyReplace_FuzzyCollapse_PreservesNextLine pins that a
// shorter replacement under the fuzzy path does not merge the
// next unmatched content line onto the last spliced line.
func TestFuzzyReplace_FuzzyCollapse_PreservesNextLine(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	type edit struct {
		search, replace string
	}
	tests := []struct {
		name     string
		content  string
		edits    []edit
		expected string
	}{
		// Minimal: tab-indented file, space-indented caller
		// forces pass 3, replace has fewer lines than search.
		{
			name:    "Minimal",
			content: "\tone\n\ttwo\n\tthree\n\tafter\n",
			edits: []edit{{
				search:  "    one\n    two\n    three\n",
				replace: "    ONE\n    TWO\n",
			}},
			expected: "\tONE\n\tTWO\n\tafter\n",
		},
		// The adversarial harness's reproduction from
		// coderd/httpapi/httpapi.go, inline: the original had
		// `return valid == nil` on its own line after the
		// matched region. The bug merged it onto the last
		// replacement line with a tab separator.
		{
			name: "HarnessHttpapi",
			content: "\tnameValidator := func(fl validator.FieldLevel) bool {\n" +
				"\t\tf := fl.Field().Interface()\n" +
				"\t\tstr, ok := f.(string)\n" +
				"\t\tif !ok {\n" +
				"\t\t\treturn false\n" +
				"\t\t}\n" +
				"\t\tvalid := codersdk.NameValid(str)\n" +
				"\t\treturn valid == nil\n" +
				"\t}\n",
			edits: []edit{{
				search: "        f := fl.Field().Interface()\n" +
					"        str, ok := f.(string)\n" +
					"        if !ok {\n" +
					"            return false\n" +
					"        }\n" +
					"        valid := codersdk.NameValid(str)",
				replace: "        f := fl.Field().Interface()\n" +
					"        str, _ := f.(string)\n" +
					"        valid := codersdk.NameValid(str)",
			}},
			expected: "\tnameValidator := func(fl validator.FieldLevel) bool {\n" +
				"\t\tf := fl.Field().Interface()\n" +
				"\t\tstr, _ := f.(string)\n" +
				"\t\tvalid := codersdk.NameValid(str)\n" +
				"\t\treturn valid == nil\n" +
				"\t}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			api := agentfiles.NewAPI(logger, fs, nil)
			path := filepath.Join(tmpdir, "fuzzycollapse-"+tt.name)
			require.NoError(t, afero.WriteFile(fs, path, []byte(tt.content), 0o644))

			sdkEdits := make([]workspacesdk.FileEdit, 0, len(tt.edits))
			for _, e := range tt.edits {
				sdkEdits = append(sdkEdits, workspacesdk.FileEdit{
					Search:  e.search,
					Replace: e.replace,
				})
			}
			req := workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{Path: path, Edits: sdkEdits}},
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(req))
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
			api.Routes().ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			data, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(data))
		})
	}
}

// TestEditFiles_WhitespaceAndLineEndings covers whitespace and
// line-ending behaviors end-to-end through the HTTP handler,
// complementing the matcher-focused TestFuzzyReplace_EndingAndWhitespace.
// Each case has a short comment describing the behavior it pins.
func TestEditFiles_WhitespaceAndLineEndings(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	cases := []struct {
		name            string
		content         string
		search, replace string
		replaceAll      bool
		expected        string // empty => expect an error response
		errSub          string
	}{
		// Tab-indented file, search matches one tab-indented
		// line byte-for-byte via pass 1. Tabs on untouched
		// lines remain; untouched space-indented lines remain.
		{
			name:    "TabIndentedLine_ExactMatch",
			content: "\ttab indented line 1\n\ttab indented line 2\n    spaces line 3\n    spaces line 4\n\ttab indented line 5\n",
			search:  "\ttab indented line 1",
			replace: "\ttab indented line 1 EDITED",
			expected: "\ttab indented line 1 EDITED\n\ttab indented line 2\n" +
				"    spaces line 3\n    spaces line 4\n\ttab indented line 5\n",
		},

		// Trailing whitespace on the content line is preserved
		// via pass 1 (byte-substring match) because the search
		// is a proper substring that doesn't touch the trailing
		// whitespace.
		{
			name:     "TrailingWhitespace_Preserved_ByPass1",
			content:  "line with trailing spaces   \nno trailing ws\n",
			search:   "line with trailing spaces",
			replace:  "line with trailing spaces EDITED",
			expected: "line with trailing spaces EDITED   \nno trailing ws\n",
		},

		// File has two blank lines between "above" and "below";
		// search omits them. Fuzzy passes also reject because
		// the search spans fewer lines than the content does,
		// so blank lines are preserved significant content.
		{
			name:    "BlankLinesAreSignificant_Rejects",
			content: "above\n\n\nbelow\n",
			search:  "above\nbelow",
			replace: "above\nbelow",
			errSub:  "search string not found",
		},

		// Search matches blank lines exactly; replacement
		// collapses the region.
		{
			name:     "RemoveBlankLines",
			content:  "above\n\n\nbelow\n",
			search:   "above\n\n\nbelow",
			replace:  "above\nbelow",
			expected: "above\nbelow\n",
		},

		// CRLF file, pass 1 substring match preserves "\r\n"
		// boundaries on every line.
		{
			name:     "CRLF_Pass1_PreservesCRLF",
			content:  "line one\r\nline two\r\nline three\r\n",
			search:   "line two",
			replace:  "line two EDITED",
			expected: "line one\r\nline two EDITED\r\nline three\r\n",
		},

		// CRLF file, LF search and replace. The ending rule
		// accepts the match, and the splice rule promotes the
		// replacement's LF endings to the file's "\r\n"
		// because search and replace agree on ending shape.
		{
			name:     "CRLF_FuzzyWithLF_FileEndingWins",
			content:  "line one\r\nline two\r\nline three\r\n",
			search:   "line one\nline two\n",
			replace:  "line one EDITED\nline two EDITED\n",
			expected: "line one EDITED\r\nline two EDITED\r\nline three\r\n",
		},

		// File has no trailing newline; pass 1 preserves EOF
		// shape.
		{
			name:     "NoTrailingNewline_Preserved",
			content:  "no trailing newline",
			search:   "no trailing newline",
			replace:  "no trailing newline EDITED",
			expected: "no trailing newline EDITED",
		},

		// Tab-indented content, space-indented search and
		// replace. Pass 3 matches the line body ignoring
		// leading whitespace. Search and replace agree on
		// leading whitespace (both "  ") so the file's "\t"
		// wins; search and replace agree on ending (both
		// "\n") so the file's "\n" wins. The following
		// "\titem two\n" is not folded into the replacement.
		{
			name:     "FuzzyIndent_FileIndentWins_NoLineFolding",
			content:  "\titem one\n\titem two\n",
			search:   "  item one\n",
			replace:  "  item one EDITED\n",
			expected: "\titem one EDITED\n\titem two\n",
		},
	}

	for _, ct := range cases {
		t.Run(ct.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			api := agentfiles.NewAPI(logger, fs, nil)
			path := filepath.Join(tmpdir, "ws-"+ct.name)
			require.NoError(t, afero.WriteFile(fs, path, []byte(ct.content), 0o644))

			req := workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{
					Path: path,
					Edits: []workspacesdk.FileEdit{{
						Search:     ct.search,
						Replace:    ct.replace,
						ReplaceAll: ct.replaceAll,
					}},
				}},
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(req))
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
			api.Routes().ServeHTTP(w, r)

			if ct.errSub != "" {
				require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
				got := &codersdk.Error{}
				require.NoError(t, json.NewDecoder(w.Body).Decode(got))
				require.ErrorContains(t, got, ct.errSub)
				data, err := afero.ReadFile(fs, path)
				require.NoError(t, err)
				require.Equal(t, ct.content, string(data))
				return
			}
			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			data, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, ct.expected, string(data))
		})
	}
}

// TestFuzzyReplace_Rejects pins the cases the matcher rejects, so
// regressions that weaken the guardrails get caught. Each case runs
// through the HTTP handler; the handler must return 400 with an
// error message matching errSub, and the file must be unchanged.
//
// Rejection sources:
//
//   - Empty search (meaningful search text is required; the old
//     behavior matched at every byte position when combined with
//     replace_all).
//   - Ambiguous match without replace_all (N > 1 occurrences of the
//     search text).
//   - Search not found in file (after all three passes fail).
//   - Content mismatch that cannot be recovered by trimming
//     whitespace on either side.
//   - Blank-line count mismatch inside the matched region.
func TestFuzzyReplace_Rejects(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	type edit struct {
		search, replace string
		replaceAll      bool
	}
	tests := []struct {
		name    string
		content string
		edits   []edit
		errSub  string
	}{
		// Empty search with replace_all=false: reject to prevent
		// the ambiguous "prepend at byte 0" behavior.
		{
			name:    "EmptySearch_Rejects",
			content: "hello\n",
			edits:   []edit{{search: "", replace: "X"}},
			errSub:  "search string must not be empty",
		},
		// Empty search with replace_all=true: historically
		// injected the replacement between every byte, silently
		// corrupting the file. Reject explicitly.
		{
			name:    "EmptySearch_ReplaceAll_Rejects",
			content: "hello\n",
			edits:   []edit{{search: "", replace: "X", replaceAll: true}},
			errSub:  "search string must not be empty",
		},
		// Ambiguous single-replace: 3 distinct matches, caller
		// did not ask for replace_all.
		{
			name:    "Ambiguous_SingleReplace_Rejects",
			content: "a\na\na\nother\n",
			edits:   []edit{{search: "a", replace: "A"}},
			errSub:  "matches 3 occurrences",
		},
		// Search text does not appear anywhere in the file. All
		// three passes miss.
		{
			name:    "NotFound_Rejects",
			content: "hello\nworld\n",
			edits:   []edit{{search: "nonexistent\n", replace: "X\n"}},
			errSub:  "search string not found",
		},
		// Content mismatch that trimming cannot recover: search
		// has different letters, not just different whitespace.
		{
			name:    "ContentMismatch_Rejects",
			content: "hello\n",
			edits:   []edit{{search: "Hello\n", replace: "HELLO\n"}},
			errSub:  "search string not found",
		},
		// Blank lines in the file that the search omits: the
		// fuzzy window cannot align against the blank lines, so
		// the multi-line match fails.
		{
			name:    "BlankLineMismatch_Rejects",
			content: "above\n\n\nbelow\n",
			edits:   []edit{{search: "above\nbelow\n", replace: "above\nbelow\n"}},
			errSub:  "search string not found",
		},
		// Search/replace disagreement signals intent to rewrite
		// endings; search must byte-match the file's. LF search
		// against CRLF file fails pass 1 and must reject rather
		// than fall through to pass 2's CRLF/LF interchange.
		{
			name:    "CallerIntent_SearchDoesNotMatchFileEnding_Rejects",
			content: "x\r\ny\r\nz\r\n",
			edits:   []edit{{search: "x\ny\n", replace: "X\r\nY\r\n"}},
			errSub:  "search string not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			api := agentfiles.NewAPI(logger, fs, nil)
			path := filepath.Join(tmpdir, "reject-"+tt.name)
			require.NoError(t, afero.WriteFile(fs, path, []byte(tt.content), 0o644))

			sdkEdits := make([]workspacesdk.FileEdit, 0, len(tt.edits))
			for _, e := range tt.edits {
				sdkEdits = append(sdkEdits, workspacesdk.FileEdit{
					Search:     e.search,
					Replace:    e.replace,
					ReplaceAll: e.replaceAll,
				})
			}
			req := workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{Path: path, Edits: sdkEdits}},
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			require.NoError(t, enc.Encode(req))
			w := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
			api.Routes().ServeHTTP(w, r)

			require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
			got := &codersdk.Error{}
			require.NoError(t, json.NewDecoder(w.Body).Decode(got))
			require.ErrorContains(t, got, tt.errSub)

			// File must not have been modified by any partial
			// splice or write.
			data, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, tt.content, string(data))
		})
	}
}

// TestEditFiles_DuplicatePath_Rejects pins that duplicate paths in
// one request are rejected with 400 and the file on disk is
// unchanged. The pre-fix behavior silently dropped the first
// entry's edits while reporting success (last write wins).
func TestEditFiles_DuplicatePath_Rejects(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, nil)
	path := filepath.Join(tmpdir, "dup-path")
	original := "one\ntwo\nthree\n"
	require.NoError(t, afero.WriteFile(fs, path, []byte(original), 0o644))

	req := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{
			{Path: path, Edits: []workspacesdk.FileEdit{{Search: "one", Replace: "ONE"}}},
			{Path: path, Edits: []workspacesdk.FileEdit{{Search: "three", Replace: "THREE"}}},
		},
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(req))
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
	api.Routes().ServeHTTP(w, r)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	got := &codersdk.Error{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(got))
	require.ErrorContains(t, got, "duplicate file path")

	// File on disk must be untouched: no partial edits.
	data, err := afero.ReadFile(fs, path)
	require.NoError(t, err)
	require.Equal(t, original, string(data))
}

// TestEditFiles_DuplicatePath_SymlinkAliasRejects pins that two
// request entries pointing to the same real file (one direct, one
// via a symlink) are rejected. Without resolve-before-dedup, the
// raw-path check lets both entries through, and the second write
// silently overwrites the first.
func TestEditFiles_DuplicatePath_SymlinkAliasRejects(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks are not reliably supported on Windows")
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	dir := t.TempDir()
	osFs := afero.NewOsFs()
	api := agentfiles.NewAPI(logger, osFs, nil)

	realPath := filepath.Join(dir, "real.txt")
	original := "one\ntwo\nthree\n"
	require.NoError(t, afero.WriteFile(osFs, realPath, []byte(original), 0o644))

	linkPath := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink(realPath, linkPath))

	req := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{
			{Path: realPath, Edits: []workspacesdk.FileEdit{{Search: "one", Replace: "ONE"}}},
			{Path: linkPath, Edits: []workspacesdk.FileEdit{{Search: "three", Replace: "THREE"}}},
		},
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(req))
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/edit-files", buf)
	api.Routes().ServeHTTP(w, r)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	got := &codersdk.Error{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(got))
	require.ErrorContains(t, got, "aliases")

	// File on disk must be untouched: the alias collision is caught
	// before phase 1 so no write runs.
	data, err := afero.ReadFile(osFs, realPath)
	require.NoError(t, err)
	require.Equal(t, original, string(data))
}

// TestEditFiles_ReplaceAll_FuzzyIndentGap locks the CURRENT output
// of a known foot-gun, it doesn't bless it.
//
// Gap: replace_all plus a pass-3 (indent-agnostic) match hits every
// nesting level whose body matches after TrimSpace. A caller aiming
// at one block silently edits the same pattern at other depths.
// The per-position splice preserves each match's local indent, so
// the output is syntactically fine. The foot-gun is that wrong
// SITES get edited.
//
// The right fix is a caller-side opt-out from fuzzy matching, out
// of scope for this PR. When that lands, update the test to assert
// the new behavior.
func TestEditFiles_ReplaceAll_FuzzyIndentGap(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, nil)
	path := filepath.Join(tmpdir, "replaceall-fuzzyindent-gap")

	// File is tab-indented Go, with `if err != nil { return err }`
	// at two nesting levels (2 tabs and 3 tabs). Caller sends a
	// 4-space-indented search/replace pair with replace_all=true.
	// Pass 1 fails (no 4-space prefix in file). Pass 2 fails (trim
	// right doesn't touch leading whitespace). Pass 3 (TrimSpace)
	// matches at BOTH depths. Current behavior: replace both.
	content := "package main\n\nfunc a() {\n" +
		"\t\tif err != nil {\n" +
		"\t\t\treturn err\n" +
		"\t\t}\n" +
		"\t\t\tif err != nil {\n" +
		"\t\t\t\treturn err\n" +
		"\t\t\t}\n" +
		"}\n"
	require.NoError(t, afero.WriteFile(fs, path, []byte(content), 0o644))

	req := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{{
			Path: path,
			Edits: []workspacesdk.FileEdit{{
				Search: "    if err != nil {\n" +
					"        return err\n" +
					"    }\n",
				Replace: "    if err != nil {\n" +
					"        return fmt.Errorf(\"wrap: %w\", err)\n" +
					"    }\n",
				ReplaceAll: true,
			}},
		}},
	}

	_ = runEditFiles(t, api, req)

	// Both depths got edited. The per-position splice preserved each
	// site's local indent, so output is syntactically fine, just
	// edited at two places, only one of which the caller likely
	// intended.
	expected := "package main\n\nfunc a() {\n" +
		"\t\tif err != nil {\n" +
		"\t\t\treturn fmt.Errorf(\"wrap: %w\", err)\n" +
		"\t\t}\n" +
		"\t\t\tif err != nil {\n" +
		"\t\t\t\treturn fmt.Errorf(\"wrap: %w\", err)\n" +
		"\t\t\t}\n" +
		"}\n"
	data, err := afero.ReadFile(fs, path)
	require.NoError(t, err)
	require.Equal(t, expected, string(data))
}

// TestEditFiles_FuzzyIndent_InsertionLevelAware covers indent-
// propagation bugs that fire when the caller's search/replace
// whitespace differs from the file's (tab vs space, 2sp vs 4sp).
//
//   - Red_* cases assert the correct output that the indent-unit
//     translation produces for inserted splice lines.
//   - Lock_* cases pin output for middle-substitution scenarios
//     that the insertion-only fix does not cover; tracked in
//     CODAGT-214.
func TestEditFiles_FuzzyIndent_InsertionLevelAware(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	type edit struct {
		search, replace string
		replaceAll      bool
	}
	tests := []struct {
		name     string
		content  string
		edits    []edit
		expected string
	}{
		// Wrap an existing line in a new block. Tab file, 4sp caller.
		{
			name: "Red_WrapInBlock_TabFile_4spLLM",
			content: "func main() {\n" +
				"\tfmt.Println(\"hello\")\n" +
				"\tfmt.Println(\"world\")\n" +
				"}\n",
			edits: []edit{{
				search: "    fmt.Println(\"hello\")\n" +
					"    fmt.Println(\"world\")",
				replace: "    fmt.Println(\"hello\")\n" +
					"    if verbose {\n" +
					"        fmt.Println(\"world\")\n" +
					"    }",
			}},
			expected: "func main() {\n" +
				"\tfmt.Println(\"hello\")\n" +
				"\tif verbose {\n" +
				"\t\tfmt.Println(\"world\")\n" +
				"\t}\n" +
				"}\n",
		},

		// Wrap in a new block, 2sp file, 4sp caller. The common
		// real-world trigger: Claude/GPT default 4sp into a 2sp file.
		{
			name: "Red_WrapInBlock_2spFile_4spLLM",
			content: "function main() {\n" +
				"  console.log('hello')\n" +
				"  console.log('world')\n" +
				"}\n",
			edits: []edit{{
				search: "    console.log('hello')\n" +
					"    console.log('world')",
				replace: "    console.log('hello')\n" +
					"    if (verbose) {\n" +
					"        console.log('world')\n" +
					"    }",
			}},
			expected: "function main() {\n" +
				"  console.log('hello')\n" +
				"  if (verbose) {\n" +
				"    console.log('world')\n" +
				"  }\n" +
				"}\n",
		},

		// Expand a single line into an error-handling block.
		{
			name: "Red_SingleToMulti_ErrorHandling",
			content: "func main() {\n" +
				"\tx := getValue()\n" +
				"\tfmt.Println(x)\n" +
				"}\n",
			edits: []edit{{
				search: "    x := getValue()",
				replace: "    x, err := getValue()\n" +
					"    if err != nil {\n" +
					"        log.Fatal(err)\n" +
					"    }",
			}},
			expected: "func main() {\n" +
				"\tx, err := getValue()\n" +
				"\tif err != nil {\n" +
				"\t\tlog.Fatal(err)\n" +
				"\t}\n" +
				"\tfmt.Println(x)\n" +
				"}\n",
		},

		// Insert a new validation block after an existing if-block.
		{
			name: "Red_InsertNewBlock_AfterExisting",
			content: "func loadConfig() (*Config, error) {\n" +
				"\tvar cfg Config\n" +
				"\terr = json.Unmarshal(data, \u0026cfg)\n" +
				"\tif err != nil {\n" +
				"\t\treturn nil, err\n" +
				"\t}\n" +
				"\n" +
				"\treturn \u0026cfg, nil\n" +
				"}\n",
			edits: []edit{{
				search: "    var cfg Config\n" +
					"    err = json.Unmarshal(data, \u0026cfg)\n" +
					"    if err != nil {\n" +
					"        return nil, err\n" +
					"    }\n" +
					"\n" +
					"    return \u0026cfg, nil",
				replace: "    var cfg Config\n" +
					"    err = json.Unmarshal(data, \u0026cfg)\n" +
					"    if err != nil {\n" +
					"        return nil, fmt.Errorf(\"unmarshal: %w\", err)\n" +
					"    }\n" +
					"    if err := cfg.Validate(); err != nil {\n" +
					"        return nil, fmt.Errorf(\"validate: %w\", err)\n" +
					"    }\n" +
					"\n" +
					"    return \u0026cfg, nil",
			}},
			expected: "func loadConfig() (*Config, error) {\n" +
				"\tvar cfg Config\n" +
				"\terr = json.Unmarshal(data, \u0026cfg)\n" +
				"\tif err != nil {\n" +
				"\t\treturn nil, fmt.Errorf(\"unmarshal: %w\", err)\n" +
				"\t}\n" +
				"\tif err := cfg.Validate(); err != nil {\n" +
				"\t\treturn nil, fmt.Errorf(\"validate: %w\", err)\n" +
				"\t}\n" +
				"\n" +
				"\treturn \u0026cfg, nil\n" +
				"}\n",
		},

		// replace_all + pass 3 + expansion at two sites.
		{
			name: "Red_ReplaceAll_Pass3_Expansion",
			content: "func handlers() {\n" +
				"\thttp.HandleFunc(\"/a\", func(w http.ResponseWriter, r *http.Request) {\n" +
				"\t\tdata := readBody(r)\n" +
				"\t\tprocess(data)\n" +
				"\t})\n" +
				"\thttp.HandleFunc(\"/b\", func(w http.ResponseWriter, r *http.Request) {\n" +
				"\t\tdata := readBody(r)\n" +
				"\t\tprocess(data)\n" +
				"\t})\n" +
				"}\n",
			edits: []edit{{
				search: "        data := readBody(r)\n" +
					"        process(data)",
				replace: "        data := readBody(r)\n" +
					"        if data == nil {\n" +
					"            return\n" +
					"        }\n" +
					"        process(data)",
				replaceAll: true,
			}},
			expected: "func handlers() {\n" +
				"\thttp.HandleFunc(\"/a\", func(w http.ResponseWriter, r *http.Request) {\n" +
				"\t\tdata := readBody(r)\n" +
				"\t\tif data == nil {\n" +
				"\t\t\treturn\n" +
				"\t\t}\n" +
				"\t\tprocess(data)\n" +
				"\t})\n" +
				"\thttp.HandleFunc(\"/b\", func(w http.ResponseWriter, r *http.Request) {\n" +
				"\t\tdata := readBody(r)\n" +
				"\t\tif data == nil {\n" +
				"\t\t\treturn\n" +
				"\t\t}\n" +
				"\t\tprocess(data)\n" +
				"\t})\n" +
				"}\n",
		},

		// Unwrap (decrease nesting). All output lines are
		// middle-substitutions; CODAGT-214 covers the fix.
		{
			name: "Lock_Unwrap_MiddleSubDisagreement",
			content: "func main() {\n" +
				"\tif condition {\n" +
				"\t\tdoSomething()\n" +
				"\t\tdoMore()\n" +
				"\t}\n" +
				"}\n",
			edits: []edit{{
				search: "    if condition {\n" +
					"        doSomething()\n" +
					"        doMore()\n" +
					"    }",
				replace: "    doSomething()\n" +
					"    doMore()",
			}},
			// Line 2 leaks 4 literal spaces (middle-sub disagreement
			// rule: rLead wins when sLead != rLead).
			expected: "func main() {\n" +
				"\tdoSomething()\n" +
				"    doMore()\n" +
				"}\n",
		},

		// Middle-rewrite with different nesting, tab file. Mixed
		// fate: inserted lines fixed, middle-subs still leak.
		{
			name: "Lock_MiddleRewrite_DifferentNesting_Tab",
			content: "func transform(items []Item) []Result {\n" +
				"\tvar results []Result\n" +
				"\tfor _, item := range items {\n" +
				"\t\tif item.Valid {\n" +
				"\t\t\tresults = append(results, convert(item))\n" +
				"\t\t}\n" +
				"\t}\n" +
				"\treturn results\n" +
				"}\n",
			edits: []edit{{
				search: "    var results []Result\n" +
					"    for _, item := range items {\n" +
					"        if item.Valid {\n" +
					"            results = append(results, convert(item))\n" +
					"        }\n" +
					"    }\n" +
					"    return results",
				replace: "    var results []Result\n" +
					"    for _, item := range items {\n" +
					"        result, err := convert(item)\n" +
					"        if err != nil {\n" +
					"            continue\n" +
					"        }\n" +
					"        results = append(results, result)\n" +
					"    }\n" +
					"    return results",
			}},
			// Middle-sub lines (i=3, i=4) leak literal 8sp/12sp;
			// the inserted } and append lines are tab-correct.
			expected: "func transform(items []Item) []Result {\n" +
				"\tvar results []Result\n" +
				"\tfor _, item := range items {\n" +
				"\t\tresult, err := convert(item)\n" +
				"        if err != nil {\n" +
				"            continue\n" +
				"\t\t}\n" +
				"\t\tresults = append(results, result)\n" +
				"\t}\n" +
				"\treturn results\n" +
				"}\n",
		},

		// Same class as lock #7, 2sp file (JS/TS).
		{
			name: "Lock_MiddleRewrite_DifferentNesting_2sp",
			content: "function transform(items) {\n" +
				"  const results = [];\n" +
				"  for (const item of items) {\n" +
				"    if (item.valid) {\n" +
				"      results.push(convert(item));\n" +
				"    }\n" +
				"  }\n" +
				"  return results;\n" +
				"}\n",
			edits: []edit{{
				search: "    const results = [];\n" +
					"    for (const item of items) {\n" +
					"        if (item.valid) {\n" +
					"            results.push(convert(item));\n" +
					"        }\n" +
					"    }\n" +
					"    return results;",
				replace: "    const results = [];\n" +
					"    for (const item of items) {\n" +
					"        const result = convert(item);\n" +
					"        if (!result) {\n" +
					"            continue;\n" +
					"        }\n" +
					"        results.push(result);\n" +
					"    }\n" +
					"    return results;",
			}},
			// Middle-sub lines (i=3, i=4) leak 8sp/12sp; the inserted
			// } and push lines translate to 4sp correctly.
			expected: "function transform(items) {\n" +
				"  const results = [];\n" +
				"  for (const item of items) {\n" +
				"    const result = convert(item);\n" +
				"        if (!result) {\n" +
				"            continue;\n" +
				"    }\n" +
				"    results.push(result);\n" +
				"  }\n" +
				"  return results;\n" +
				"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			api := agentfiles.NewAPI(logger, fs, nil)
			path := filepath.Join(tmpdir, "fuzzyindent-"+tt.name)
			require.NoError(t, afero.WriteFile(fs, path, []byte(tt.content), 0o644))

			req := workspacesdk.FileEditRequest{
				Files: []workspacesdk.FileEdits{{
					Path:  path,
					Edits: make([]workspacesdk.FileEdit, 0, len(tt.edits)),
				}},
			}
			for _, e := range tt.edits {
				req.Files[0].Edits = append(req.Files[0].Edits, workspacesdk.FileEdit{
					Search:     e.search,
					Replace:    e.replace,
					ReplaceAll: e.replaceAll,
				})
			}

			_ = runEditFiles(t, api, req)
			data, err := afero.ReadFile(fs, path)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(data))
		})
	}
}

// TestFuzzyReplace_Expansion_PreservesFileIndent pins that when
// replace has more lines than search, every spliced line keeps
// the file's indent style. Inserted lines especially must not
// carry the caller's literal whitespace into the output.
func TestFuzzyReplace_Expansion_PreservesFileIndent(t *testing.T) {
	t.Parallel()

	tmpdir := os.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	fs := afero.NewMemMapFs()
	api := agentfiles.NewAPI(logger, fs, nil)
	path := filepath.Join(tmpdir, "fuzzy-expansion-gap")

	content := "\tnameValidator := func(fl validator.FieldLevel) bool {\n" +
		"\t\tf := fl.Field().Interface()\n" +
		"\t\tstr, ok := f.(string)\n" +
		"\t\tif !ok {\n" +
		"\t\t\treturn false\n" +
		"\t\t}\n" +
		"\t\tvalid := codersdk.NameValid(str)\n" +
		"\t\treturn valid == nil\n" +
		"\t}\n"
	require.NoError(t, afero.WriteFile(fs, path, []byte(content), 0o644))

	req := workspacesdk.FileEditRequest{
		Files: []workspacesdk.FileEdits{{
			Path: path,
			Edits: []workspacesdk.FileEdit{{
				Search: "        f := fl.Field().Interface()\n" +
					"        str, ok := f.(string)\n" +
					"        if !ok {\n" +
					"            return false\n" +
					"        }\n" +
					"        valid := codersdk.NameValid(str)",
				Replace: "        f := fl.Field().Interface()\n" +
					"        str, ok := f.(string)\n" +
					"        if !ok {\n" +
					"            log.Println(\"type assertion failed\")\n" +
					"            return false\n" +
					"        }\n" +
					"        valid := codersdk.NameValid(str)",
			}},
		}},
	}

	_ = runEditFiles(t, api, req)

	// All lines emitted in the file's tab indent, including the
	// inserted log.Println and the following return false (which
	// index-pairs with a different search line but shares the same
	// 3-tab depth in the file).
	expected := "\tnameValidator := func(fl validator.FieldLevel) bool {\n" +
		"\t\tf := fl.Field().Interface()\n" +
		"\t\tstr, ok := f.(string)\n" +
		"\t\tif !ok {\n" +
		"\t\t\tlog.Println(\"type assertion failed\")\n" +
		"\t\t\treturn false\n" +
		"\t\t}\n" +
		"\t\tvalid := codersdk.NameValid(str)\n" +
		"\t\treturn valid == nil\n" +
		"\t}\n"
	data, err := afero.ReadFile(fs, path)
	require.NoError(t, err)
	require.Equal(t, expected, string(data))
}

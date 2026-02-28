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
	"syscall"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentfiles"
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
	api := agentfiles.NewAPI(logger, fs)

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
	api := agentfiles.NewAPI(logger, fs)

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
	api := agentfiles.NewAPI(logger, fs)

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
			expected: map[string]string{filepath.Join(tmpdir, "trailing-ws"): "replaced"},
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
			name:     "NoMatchStillSucceeds",
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
			// File should remain unchanged.
			expected: map[string]string{filepath.Join(tmpdir, "no-match"): "original content"},
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
	api := agentfiles.NewAPI(logger, fs)

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

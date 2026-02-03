package agentfiles

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type testFs struct {
	afero.Fs
}

func newTestFs(base afero.Fs) *testFs {
	return &testFs{
		Fs: base,
	}
}

func (*testFs) Open(name string) (afero.File, error) {
	return nil, os.ErrPermission
}

func TestListFilesWithQueryParam(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	query := workspacesdk.LSRequest{}
	_, err := listFiles(fs, "not-relative", query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be absolute")

	tmpDir := t.TempDir()
	err = fs.MkdirAll(tmpDir, 0o755)
	require.NoError(t, err)

	res, err := listFiles(fs, tmpDir, query)
	require.NoError(t, err)
	require.Len(t, res.Contents, 0)
}

func TestListFilesNonExistentDirectory(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	query := workspacesdk.LSRequest{
		Path:       []string{"idontexist"},
		Relativity: workspacesdk.LSRelativityHome,
	}
	_, err := listFiles(fs, "", query)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestListFilesPermissionDenied(t *testing.T) {
	t.Parallel()

	fs := newTestFs(afero.NewMemMapFs())
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	reposDir := filepath.Join(tmpDir, "repos")
	err = fs.MkdirAll(reposDir, 0o000)
	require.NoError(t, err)

	rel, err := filepath.Rel(home, reposDir)
	require.NoError(t, err)

	query := workspacesdk.LSRequest{
		Path:       pathToArray(rel),
		Relativity: workspacesdk.LSRelativityHome,
	}
	_, err = listFiles(fs, "", query)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestListFilesNotADirectory(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = fs.MkdirAll(tmpDir, 0o755)
	require.NoError(t, err)

	filePath := filepath.Join(tmpDir, "file.txt")
	err = afero.WriteFile(fs, filePath, []byte("content"), 0o600)
	require.NoError(t, err)

	rel, err := filepath.Rel(home, filePath)
	require.NoError(t, err)

	query := workspacesdk.LSRequest{
		Path:       pathToArray(rel),
		Relativity: workspacesdk.LSRelativityHome,
	}
	_, err = listFiles(fs, "", query)
	require.ErrorContains(t, err, "is not a directory")
}

func TestListFilesSuccess(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name       string
		baseFunc   func(t *testing.T) string
		relativity workspacesdk.LSRelativity
	}{
		{
			name: "home",
			baseFunc: func(t *testing.T) string {
				home, err := os.UserHomeDir()
				require.NoError(t, err)
				return home
			},
			relativity: workspacesdk.LSRelativityHome,
		},
		{
			name: "root",
			baseFunc: func(*testing.T) string {
				if runtime.GOOS == "windows" {
					return ""
				}
				return "/"
			},
			relativity: workspacesdk.LSRelativityRoot,
		},
	}

	// nolint:paralleltest // Not since Go v1.22.
	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			base := tc.baseFunc(t)
			tmpDir := t.TempDir()

			reposDir := filepath.Join(tmpDir, "repos")
			err := fs.MkdirAll(reposDir, 0o755)
			require.NoError(t, err)

			downloadsDir := filepath.Join(tmpDir, "Downloads")
			err = fs.MkdirAll(downloadsDir, 0o755)
			require.NoError(t, err)

			textFile := filepath.Join(tmpDir, "file.txt")
			err = afero.WriteFile(fs, textFile, []byte("content"), 0o600)
			require.NoError(t, err)

			var queryComponents []string
			// We can't get an absolute path relative to empty string on Windows.
			if runtime.GOOS == "windows" && base == "" {
				queryComponents = pathToArray(tmpDir)
			} else {
				rel, err := filepath.Rel(base, tmpDir)
				require.NoError(t, err)
				queryComponents = pathToArray(rel)
			}

			query := workspacesdk.LSRequest{
				Path:       queryComponents,
				Relativity: tc.relativity,
			}
			resp, err := listFiles(fs, "", query)
			require.NoError(t, err)

			require.Equal(t, tmpDir, resp.AbsolutePathString)
			// Output is sorted
			require.Equal(t, []workspacesdk.LSFile{
				{
					Name:               "Downloads",
					AbsolutePathString: downloadsDir,
					IsDir:              true,
				},
				{
					Name:               "repos",
					AbsolutePathString: reposDir,
					IsDir:              true,
				},
				{
					Name:               "file.txt",
					AbsolutePathString: textFile,
					IsDir:              false,
				},
			}, resp.Contents)
		})
	}
}

func TestListFilesListDrives(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("skipping test on non-Windows OS")
	}

	fs := afero.NewOsFs()
	query := workspacesdk.LSRequest{
		Path:       []string{},
		Relativity: workspacesdk.LSRelativityRoot,
	}
	resp, err := listFiles(fs, "", query)
	require.NoError(t, err)
	require.Contains(t, resp.Contents, workspacesdk.LSFile{
		Name:               "C:\\",
		AbsolutePathString: "C:\\",
		IsDir:              true,
	})

	query = workspacesdk.LSRequest{
		Path:       []string{"C:\\"},
		Relativity: workspacesdk.LSRelativityRoot,
	}
	resp, err = listFiles(fs, "", query)
	require.NoError(t, err)

	query = workspacesdk.LSRequest{
		Path:       resp.AbsolutePath,
		Relativity: workspacesdk.LSRelativityRoot,
	}
	resp, err = listFiles(fs, "", query)
	require.NoError(t, err)
	// System directory should always exist
	require.Contains(t, resp.Contents, workspacesdk.LSFile{
		Name:               "Windows",
		AbsolutePathString: "C:\\Windows",
		IsDir:              true,
	})

	query = workspacesdk.LSRequest{
		// Network drives are not supported.
		Path:       []string{"\\sshfs\\work"},
		Relativity: workspacesdk.LSRelativityRoot,
	}
	resp, err = listFiles(fs, "", query)
	require.ErrorContains(t, err, "drive")
}

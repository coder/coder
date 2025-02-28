package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListFilesNonExistentDirectory(t *testing.T) {
	t.Parallel()

	query := LSQuery{
		Path:       []string{"idontexist"},
		Relativity: LSRelativityHome,
	}
	_, err := listFiles(query)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestListFilesPermissionDenied(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("creating an unreadable-by-user directory is non-trivial on Windows")
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	reposDir := filepath.Join(tmpDir, "repos")
	err = os.Mkdir(reposDir, 0o000)
	require.NoError(t, err)

	rel, err := filepath.Rel(home, reposDir)
	require.NoError(t, err)

	query := LSQuery{
		Path:       pathToArray(rel),
		Relativity: LSRelativityHome,
	}
	_, err = listFiles(query)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestListFilesNotADirectory(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "file.txt")
	err = os.WriteFile(filePath, []byte("content"), 0o600)
	require.NoError(t, err)

	rel, err := filepath.Rel(home, filePath)
	require.NoError(t, err)

	query := LSQuery{
		Path:       pathToArray(rel),
		Relativity: LSRelativityHome,
	}
	_, err = listFiles(query)
	require.ErrorContains(t, err, "path is not a directory")
}

func TestListFilesSuccess(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name       string
		baseFunc   func(t *testing.T) string
		relativity LSRelativity
	}{
		{
			name: "home",
			baseFunc: func(t *testing.T) string {
				home, err := os.UserHomeDir()
				require.NoError(t, err)
				return home
			},
			relativity: LSRelativityHome,
		},
		{
			name: "root",
			baseFunc: func(t *testing.T) string {
				if runtime.GOOS == "windows" {
					return "C:\\"
				}
				return "/"
			},
			relativity: LSRelativityRoot,
		},
	}

	// nolint:paralleltest // Not since Go v1.22.
	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := tc.baseFunc(t)
			tmpDir := t.TempDir()

			reposDir := filepath.Join(tmpDir, "repos")
			err := os.Mkdir(reposDir, 0o755)
			require.NoError(t, err)

			downloadsDir := filepath.Join(tmpDir, "Downloads")
			err = os.Mkdir(downloadsDir, 0o755)
			require.NoError(t, err)

			rel, err := filepath.Rel(base, tmpDir)
			require.NoError(t, err)
			relComponents := pathToArray(rel)

			query := LSQuery{
				Path:       relComponents,
				Relativity: tc.relativity,
			}
			resp, err := listFiles(query)
			require.NoError(t, err)

			require.Equal(t, tmpDir, resp.AbsolutePathString)

			var foundRepos, foundDownloads bool
			for _, file := range resp.Contents {
				switch file.Name {
				case "repos":
					foundRepos = true
					expectedPath := filepath.Join(tmpDir, "repos")
					require.Equal(t, expectedPath, file.AbsolutePathString)
					require.True(t, file.IsDir)
				case "Downloads":
					foundDownloads = true
					expectedPath := filepath.Join(tmpDir, "Downloads")
					require.Equal(t, expectedPath, file.AbsolutePathString)
					require.True(t, file.IsDir)
				}
			}
			require.True(t, foundRepos && foundDownloads, "expected to find both repos and Downloads directories, got: %+v", resp.Contents)
		})
	}
}

func TestListFilesWindowsRoot(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("skipping test on non-Windows OS")
	}

	query := LSQuery{
		Path:       []string{},
		Relativity: LSRelativityRoot,
	}
	resp, err := listFiles(query)
	require.NoError(t, err)
	require.Equal(t, "C:\\", resp.AbsolutePathString)
}

func pathToArray(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool {
		return r == os.PathSeparator
	})
}

package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// On Windows, os.RemoveAll can fail with (among other):
	//
	//	The process cannot access the file because it is being used by another process.
	//
	// We try to work around this issue by removing files and directories in
	// a loop until os.RemoveAll succeeds or we give up.
	if runtime.GOOS == "windows" {
		t.Cleanup(func() {
			var dirs, files []string
			_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() && path != dir {
					dirs = append(dirs, path)
					return nil
				}
				files = append(files, path)
				return nil
			})
			removeFilesAndDirs := func() {
				// Try to remove all files so that the directories are empty.
				var newFiles []string
				for _, path := range files {
					if err := os.Remove(path); err != nil {
						newFiles = append(newFiles, path)
					}
				}
				files = newFiles

				// Remove directories in reverse order (~depth first).
				var newDirs []string
				for i := len(dirs) - 1; i >= 0; i-- {
					if err := os.Remove(dirs[i]); err != nil {
						newDirs = append([]string{dirs[i]}, newDirs...)
					}
				}
				dirs = newDirs
			}

			var (
				start   = time.Now()
				timeout = 5 * time.Second
				err     error
			)
			for time.Since(start) < timeout {
				removeFilesAndDirs()
				err = os.RemoveAll(dir)
				if err == nil {
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			// Note that even though we're giving up here, t.TempDir
			// cleanup _could_ still succeed (or fail the test).
			t.Logf("TempDir: delete %q: giving up after %s: %v", dir, timeout, err)
		})
	}
	return dir
}

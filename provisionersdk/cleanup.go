package provisionersdk

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/djherbis/times"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// cleanStaleSessions browses the work directory searching for stale session
// directories. Coder provisioner is supposed to remove them once after finishing the provisioning,
// but there is a risk of keeping them in case of a failure.
func cleanStaleSessions(ctx context.Context, workDirectory string, now time.Time, logger slog.Logger) error {
	entries, err := os.ReadDir(workDirectory)
	if err != nil {
		return xerrors.Errorf("can't read %q directory", workDirectory)
	}

	for _, entry := range entries {
		dirName := entry.Name()

		if entry.IsDir() && isValidSessionDir(dirName) {
			sessionDirPath := filepath.Join(workDirectory, dirName)
			fi, err := entry.Info()
			if err != nil {
				return xerrors.Errorf("can't read %q directory info: %w", sessionDirPath, err)
			}

			timeSpec := times.Get(fi)
			if timeSpec.AccessTime().Add(staleSessionRetention).After(now) {
				continue
			}

			logger.Info(ctx, "remove stale session directory: %s", sessionDirPath)
			err = os.RemoveAll(sessionDirPath)
			if err != nil {
				return xerrors.Errorf("can't remove %q directory: %w", sessionDirPath, err)
			}
		}
	}
	return nil
}

func isValidSessionDir(dirName string) bool {
	match, err := filepath.Match(sessionDirPrefix+"*", dirName)
	return err == nil && match
}

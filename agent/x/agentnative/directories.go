package agentnative

import (
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// PrepareDirectories creates the private prerequisite directories for the default
// native backends. StateDir must be absolute; environment must be the explicit
// workspace-user environment, including HOME. It never reads the process
// environment, runs commands, or creates or modifies files. Existing directories
// are validated, not repaired, so repeated calls preserve native state.
func PrepareDirectories(stateDir string, environment []string) error {
	claudeRoot, err := claudeConfigRoot(environment)
	if err != nil {
		return err
	}
	codexRoot, err := codexHome(environment)
	if err != nil {
		return err
	}
	paths := append([]string{stateDir, claudeRoot, codexRoot}, openCodeHomeDirectories(openCodeHome(stateDir))...)
	// Resolve and validate every path before creating any directories.
	for _, path := range paths {
		if !nativeValidPath(path) {
			return xerrors.New("native directories require clean absolute paths without control characters, at most 4096 bytes")
		}
	}
	for _, path := range paths {
		dir, err := openPrivateDirectory(path, privateDirectoryCreate)
		if err != nil {
			return xerrors.Errorf("prepare native directory %q: %w", path, err)
		}
		if err := dir.close(); err != nil {
			return xerrors.Errorf("close native directory %q: %w", path, err)
		}
	}
	return nil
}

func nativeValidPath(path string) bool {
	return len(path) <= 4096 && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsFunc(path, func(r rune) bool { return r < 32 || r == 127 })
}

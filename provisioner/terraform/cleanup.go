package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// CleanStaleTerraformPlugins browses the Terraform cache directory
// and remove stale plugins that haven't been used for a while.
// Additionally, it sweeps empty, old directory trees.
//
// Sample cachePath:
//
//	/Users/john.doe/Library/Caches/coder/provisioner-1/tf
//	/tmp/coder/provisioner-0/tf
func CleanStaleTerraformPlugins(ctx context.Context, cachePath string, fs afero.Fs, now time.Time, logger slog.Logger) error {
	cachePath, err := filepath.Abs(cachePath) // sanity check in case the path is e.g. ../../../cache
	if err != nil {
		return xerrors.Errorf("unable to determine absolute path %q: %w", cachePath, err)
	}

	// Firstly, check if the cache path exists.
	_, err = fs.Stat(cachePath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return xerrors.Errorf("unable to stat cache path %q: %w", cachePath, err)
	}

	logger.Info(ctx, "clean stale Terraform plugins", slog.F("cache_path", cachePath))

	// Filter directory trees matching pattern: <repositoryURL>/<company>/<plugin>/<version>/<distribution>
	filterFunc := func(path string, info os.FileInfo) bool {
		if !info.IsDir() {
			return false
		}

		relativePath, err := filepath.Rel(cachePath, path)
		if err != nil {
			logger.Error(ctx, "unable to evaluate a relative path", slog.F("base", cachePath), slog.F("target", path), slog.Error(err))
			return false
		}

		parts := strings.Split(relativePath, string(filepath.Separator))
		return len(parts) == 5
	}

	// Review cached Terraform plugins
	var pluginPaths []string
	err = afero.Walk(fs, cachePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !filterFunc(path, info) {
			return nil
		}

		logger.Debug(ctx, "plugin directory discovered", slog.F("path", path))
		pluginPaths = append(pluginPaths, path)
		return nil
	})
	if err != nil {
		return xerrors.Errorf("unable to walk through cache directory %q: %w", cachePath, err)
	}

	// Identify stale plugins
	var stalePlugins []string
	for _, pluginPath := range pluginPaths {
		modTime, err := latestModTime(fs, pluginPath)
		if err != nil {
			return xerrors.Errorf("unable to evaluate latest mtime for directory %q: %w", pluginPath, err)
		}

		if modTime.Add(staleTerraformPluginRetention).Before(now) {
			logger.Info(ctx, "plugin directory is stale and will be removed", slog.F("plugin_path", pluginPath), slog.F("mtime", modTime))
			stalePlugins = append(stalePlugins, pluginPath)
		} else {
			logger.Debug(ctx, "plugin directory is not stale", slog.F("plugin_path", pluginPath), slog.F("mtime", modTime))
		}
	}

	// Remove stale plugins
	for _, stalePluginPath := range stalePlugins {
		// Remove the plugin directory
		err = fs.RemoveAll(stalePluginPath)
		if err != nil {
			return xerrors.Errorf("unable to remove stale plugin %q: %w", stalePluginPath, err)
		}

		// Compact the plugin structure by removing empty directories.
		wd := stalePluginPath
		level := 5 // <repositoryURL>/<company>/<plugin>/<version>/<distribution>
		for {
			level--
			if level == 0 {
				break // do not compact further
			}

			wd = filepath.Dir(wd)

			files, err := afero.ReadDir(fs, wd)
			if err != nil {
				return xerrors.Errorf("unable to read directory content %q: %w", wd, err)
			}

			if len(files) > 0 {
				break // there are still other plugins
			}

			logger.Debug(ctx, "remove empty directory", slog.F("path", wd))
			err = fs.Remove(wd)
			if err != nil {
				return xerrors.Errorf("unable to remove directory %q: %w", wd, err)
			}
		}
	}
	return nil
}

// latestModTime walks recursively through the directory content, and locates
// the last created/modified file.
func latestModTime(fs afero.Fs, pluginPath string) (time.Time, error) {
	var latest time.Time
	err := afero.Walk(fs, pluginPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// atime is not reliable, so always use mtime.
		modTime := info.ModTime()
		if modTime.After(latest) {
			latest = modTime
		}
		return nil
	})
	if err != nil {
		return time.Time{}, xerrors.Errorf("unable to walk the plugin path %q: %w", pluginPath, err)
	}
	return latest, nil
}

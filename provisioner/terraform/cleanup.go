package terraform

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cdr.dev/slog"
	"github.com/djherbis/times"
	"golang.org/x/xerrors"
)

// cleanStaleTerraformPlugins browses the Terraform cache directory
// and remove stale plugins that haven't been used for a while.
//
// Additionally, it sweeps empty, old directory trees.
//
// Sample cachePath: /Users/<username>/Library/Caches/coder/provisioner-<N>/tf
func cleanStaleTerraformPlugins(ctx context.Context, cachePath string, now time.Time, logger slog.Logger) error {
	cachePath, err := filepath.Abs(cachePath) // sanity check in case the path is e.g. ../../../cache
	if err != nil {
		return xerrors.Errorf("unable to determine absolute path %q: %w", cachePath, err)
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
	err = filepath.Walk(cachePath, func(path string, info fs.FileInfo, err error) error {
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
		accessTime, err := latestAccessTime(pluginPath)
		if err != nil {
			return xerrors.Errorf("unable to evaluate latest access time for directory %q: %w", pluginPath, err)
		}

		if accessTime.Add(staleTerraformPluginRetention).Before(now) {
			logger.Info(ctx, "plugin directory is stale and will be removed", slog.F("plugin_path", pluginPath))
			stalePlugins = append(stalePlugins, pluginPath)
		} else {
			logger.Debug(ctx, "plugin directory is not stale", slog.F("plugin_path", pluginPath))
		}
	}

	// Remove stale plugins
	for _, stalePluginPath := range stalePlugins {
		// Remove the plugin directory
		err = os.RemoveAll(stalePluginPath)
		if err != nil {
			return xerrors.Errorf("unable to remove stale plugin %q: %w", stalePluginPath, err)
		}

		// Compact the plugin structure by removing empty directories.
		wd := stalePluginPath
		level := 4 // <repositoryURL>/<company>/<plugin>/<version>/<distribution>
		for {
			level--
			if level == 0 {
				break // do not compact further
			}

			wd = filepath.Dir(wd)

			files, err := os.ReadDir(wd)
			if err != nil {
				return xerrors.Errorf("unable to read directory content %q: %w", wd, err)
			}

			if len(files) > 0 {
				break // there are still other plugins
			}

			logger.Debug(ctx, "remove empty directory", slog.F("path", wd))
			err = os.Remove(wd)
			if err != nil {
				return xerrors.Errorf("unable to remove directory %q: %w", wd, err)
			}
		}
	}
	return nil
}

// latestAccessTime walks recursively through the directory content, and locates
// the last accessed file.
func latestAccessTime(pluginPath string) (time.Time, error) {
	var latest time.Time
	err := filepath.Walk(pluginPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		timeSpec := times.Get(info)
		accessTime := timeSpec.AccessTime()
		if latest.Before(accessTime) {
			latest = accessTime
		}
		return nil
	})
	if err != nil {
		return time.Time{}, xerrors.Errorf("unable to walk the plugin path %q: %w", pluginPath, err)
	}
	return latest, nil
}

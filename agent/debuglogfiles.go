package agent

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	debugLogFilesMaxPatterns     = 32
	debugLogFilesMaxFiles        = 100
	debugLogFilesMaxMatches      = 100
	debugLogFilesMaxBytes        = debugLogsActiveMaxBytes
	debugLogFilesMaxTotal        = debugLogsCombinedMaxBytes
	debugLogFilesRequestMaxBytes = 64 * 1024
)

type debugLogFilesLimits struct {
	MaxFiles        int   `json:"max_files"`
	MaxGlobMatches  int   `json:"max_glob_matches"`
	MaxBytesPerFile int64 `json:"max_bytes_per_file"`
	MaxTotalBytes   int64 `json:"max_total_bytes"`
}

type debugLogFilesManifest struct {
	Requested []string                    `json:"requested"`
	Files     []debugLogFileManifestEntry `json:"files"`
	Errors    []debugLogFileError         `json:"errors"`
	Truncated bool                        `json:"truncated"`
	Limits    debugLogFilesLimits         `json:"limits"`
}

type debugLogFileManifestEntry struct {
	Requested    string    `json:"requested"`
	Path         string    `json:"path"`
	ArchivePath  string    `json:"archive_path"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
	BytesWritten int64     `json:"bytes_written"`
	Truncated    bool      `json:"truncated"`
}

type debugLogFileError struct {
	Requested string `json:"requested"`
	Path      string `json:"path,omitempty"`
	Reason    string `json:"reason"`
}

var errDebugLogFilesGlobLimit = xerrors.New("debug log files glob match limit reached")

func (a *agent) HandleHTTPDebugLogFiles(w http.ResponseWriter, r *http.Request) {
	var req workspacesdk.DebugLogFilesRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, debugLogFilesRequestMaxBytes))
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %s", err), http.StatusBadRequest)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		a.logger.Error(r.Context(), "get user home dir", slog.Error(err))
		http.Error(w, fmt.Sprintf("get user home dir: %s", err), http.StatusInternalServerError)
		return
	}

	extendDebugLogWriteDeadline(r.Context(), a.logger, w, debugLogsWriteTimeout, "extend debug log files write deadline")

	ctx, cancel := context.WithTimeout(r.Context(), debugLogsWriteTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusOK)
	if err := collectDebugLogFiles(ctx, a.logger, home, req, w); err != nil {
		a.logger.Error(r.Context(), "collect debug log files", slog.Error(err))
	}
}

func collectDebugLogFiles(ctx context.Context, logger slog.Logger, home string, req workspacesdk.DebugLogFilesRequest, w io.Writer) error {
	return collectDebugLogFilesWithLimits(ctx, logger, home, req, w, debugLogFilesLimits{
		MaxFiles:        debugLogFilesMaxFiles,
		MaxGlobMatches:  debugLogFilesMaxMatches,
		MaxBytesPerFile: debugLogFilesMaxBytes,
		MaxTotalBytes:   debugLogFilesMaxTotal,
	})
}

// collectDebugLogFilesWithLimits streams a zip archive containing the
// requested files under files/ and a manifest.json describing the
// collection. Files are confined to home: requested paths are resolved
// lexically against it and all file access goes through os.Root, which
// refuses symlinks that escape it. Per-path problems are recorded in the
// manifest instead of failing the archive.
func collectDebugLogFilesWithLimits(ctx context.Context, logger slog.Logger, home string, req workspacesdk.DebugLogFilesRequest, w io.Writer, limits debugLogFilesLimits) error {
	paths := req.Paths
	pathsTruncated := len(paths) > debugLogFilesMaxPatterns
	if pathsTruncated {
		paths = paths[:debugLogFilesMaxPatterns]
	}
	manifest := debugLogFilesManifest{Requested: paths, Limits: limits}
	if pathsTruncated {
		manifest.Truncated = true
		appendDebugLogFilesError(&manifest, "", "", "requested path pattern limit reached")
	}

	// os.UserHomeDir returns $HOME verbatim, so matching requested paths
	// lexically against it agrees with the paths users see in their shell.
	// filepath.EvalSymlinks is deliberately avoided: it fails on Windows
	// volume mount points, and os.Root already stops symlink escapes at
	// open time.
	home, err := filepath.Abs(home)
	if err != nil {
		appendDebugLogFilesError(&manifest, "", "", "resolve home directory: "+err.Error())
		return writeDebugLogFilesManifestArchive(w, manifest)
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		appendDebugLogFilesError(&manifest, "", "", "open home directory: "+err.Error())
		return writeDebugLogFilesManifestArchive(w, manifest)
	}
	defer root.Close()

	zw := zip.NewWriter(w)
	remainingBytes := limits.MaxTotalBytes
	writtenFiles := 0
	seen := map[string]struct{}{}

patterns:
	for _, requested := range paths {
		if err := ctx.Err(); err != nil {
			manifest.Truncated = true
			appendDebugLogFilesError(&manifest, requested, "", "collection canceled: "+err.Error())
			break
		}

		matches, matchesTruncated, err := debugLogFileMatches(ctx, root, home, requested, limits.MaxGlobMatches)
		if err != nil {
			if ctx.Err() != nil {
				manifest.Truncated = true
			}
			appendDebugLogFilesError(&manifest, requested, "", err.Error())
			continue
		}
		if len(matches) == 0 {
			appendDebugLogFilesError(&manifest, requested, "", "no matches")
			continue
		}
		if matchesTruncated {
			manifest.Truncated = true
			appendDebugLogFilesError(&manifest, requested, "", "glob match limit reached")
		}

		for _, rel := range matches {
			if err := ctx.Err(); err != nil {
				manifest.Truncated = true
				appendDebugLogFilesError(&manifest, requested, rel, "collection canceled: "+err.Error())
				break patterns
			}
			if writtenFiles >= limits.MaxFiles {
				manifest.Truncated = true
				appendDebugLogFilesError(&manifest, requested, rel, "file count limit reached")
				break patterns
			}
			if remainingBytes <= 0 {
				manifest.Truncated = true
				appendDebugLogFilesError(&manifest, requested, rel, "total byte limit reached")
				break patterns
			}
			// Deduplicating by relative path is what prevents duplicate
			// entry names in the archive.
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}

			// Root.Stat follows symlinks only within the root and errors
			// on any component that escapes it.
			info, err := root.Stat(filepath.FromSlash(rel))
			if err != nil {
				reason := "stat path: " + err.Error()
				if errors.Is(err, fs.ErrNotExist) {
					reason = "does not exist"
				}
				appendDebugLogFilesError(&manifest, requested, rel, reason)
				continue
			}
			if !info.Mode().IsRegular() {
				appendDebugLogFilesError(&manifest, requested, rel, "not a regular file")
				continue
			}

			bytesToWrite := min(info.Size(), limits.MaxBytesPerFile, remainingBytes)
			entry := debugLogFileManifestEntry{
				Requested:    requested,
				Path:         rel,
				ArchivePath:  path.Join("files", rel),
				Size:         info.Size(),
				ModTime:      info.ModTime(),
				BytesWritten: bytesToWrite,
				Truncated:    bytesToWrite < info.Size(),
			}
			manifest.Truncated = manifest.Truncated || entry.Truncated
			if err := writeDebugLogFileEntry(root, zw, entry); err != nil {
				logger.Warn(ctx, "write debug log file", slog.Error(err), slog.F("path", rel))
				appendDebugLogFilesError(&manifest, requested, rel, err.Error())
				continue
			}
			manifest.Files = append(manifest.Files, entry)
			remainingBytes -= bytesToWrite
			writtenFiles++
		}
	}

	if err := writeDebugLogFilesManifest(zw, manifest); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return xerrors.Errorf("close archive: %w", err)
	}
	return nil
}

// debugLogFileMatches expands requested against home and returns matching
// home-relative slash paths. Non-glob paths return a single candidate
// without checking existence; the caller reports missing files when it
// stats them.
func debugLogFileMatches(ctx context.Context, root *os.Root, home string, requested string, maxMatches int) ([]string, bool, error) {
	pattern, err := expandDebugLogFilePattern(home, requested)
	if err != nil {
		return nil, false, err
	}
	rel, ok := homeRelativePath(home, pattern)
	if !ok {
		return nil, false, xerrors.Errorf("outside home")
	}
	if !strings.ContainsAny(rel, "*?{[") {
		return []string{rel}, false, nil
	}

	matches := make([]string, 0, maxMatches+1)
	err = doublestar.GlobWalk(debugLogFilesFS{ctx: ctx, fsys: root.FS()}, rel, func(match string, _ fs.DirEntry) error {
		matches = append(matches, match)
		if len(matches) > maxMatches {
			return errDebugLogFilesGlobLimit
		}
		return nil
	}, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
	matchesTruncated := errors.Is(err, errDebugLogFilesGlobLimit)
	if err != nil && !matchesTruncated {
		return nil, false, xerrors.Errorf("glob pattern: %w", err)
	}
	if matchesTruncated {
		matches = matches[:maxMatches]
	}
	slices.Sort(matches)
	return matches, matchesTruncated, nil
}

// debugLogFilesFS bounds a glob walk by the request context so a slow walk
// over a large home directory stops once the request is canceled.
type debugLogFilesFS struct {
	ctx  context.Context
	fsys fs.FS
}

func (f debugLogFilesFS) Open(name string) (fs.File, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return f.fsys.Open(name)
}

func (f debugLogFilesFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return fs.ReadDir(f.fsys, name)
}

func (f debugLogFilesFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return fs.Stat(f.fsys, name)
}

func expandDebugLogFilePattern(home string, requested string) (string, error) {
	trimmed := strings.TrimSpace(requested)
	if trimmed == "" {
		return "", xerrors.Errorf("empty path")
	}

	switch trimmed {
	case "$HOME", "${HOME}", "~":
		return filepath.Clean(home), nil
	}
	for _, prefix := range []string{"$HOME/", "${HOME}/", "~/"} {
		if rest, ok := strings.CutPrefix(trimmed, prefix); ok {
			return filepath.Clean(filepath.Join(home, filepath.FromSlash(rest))), nil
		}
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed), nil
	}
	return "", xerrors.Errorf("relative path not allowed")
}

// homeRelativePath returns filePath relative to home in slash form, and
// false when filePath is not lexically under home.
func homeRelativePath(home string, filePath string) (string, bool) {
	rel, err := filepath.Rel(home, filePath)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func writeDebugLogFilesManifestArchive(w io.Writer, manifest debugLogFilesManifest) error {
	zw := zip.NewWriter(w)
	if err := writeDebugLogFilesManifest(zw, manifest); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return xerrors.Errorf("close archive: %w", err)
	}
	return nil
}

func writeDebugLogFilesManifest(zw *zip.Writer, manifest debugLogFilesManifest) error {
	mf, err := zw.Create("manifest.json")
	if err != nil {
		return xerrors.Errorf("create manifest in archive: %w", err)
	}
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return xerrors.Errorf("write manifest: %w", err)
	}
	return nil
}

// writeDebugLogFileEntry copies the last entry.BytesWritten bytes of the
// file at entry.Path into the archive at entry.ArchivePath.
func writeDebugLogFileEntry(root *os.Root, zw *zip.Writer, entry debugLogFileManifestEntry) error {
	f, err := root.Open(filepath.FromSlash(entry.Path))
	if err != nil {
		return xerrors.Errorf("open file: %w", err)
	}
	defer f.Close()

	if entry.BytesWritten < entry.Size {
		if _, err := f.Seek(entry.Size-entry.BytesWritten, io.SeekStart); err != nil {
			return xerrors.Errorf("seek tail: %w", err)
		}
	}
	w, err := zw.Create(entry.ArchivePath)
	if err != nil {
		return xerrors.Errorf("create archive entry: %w", err)
	}
	if _, err := copyDebugLog(w, f, entry.BytesWritten); err != nil {
		return xerrors.Errorf("copy file: %w", err)
	}
	return nil
}

func appendDebugLogFilesError(manifest *debugLogFilesManifest, requested string, filePath string, reason string) {
	manifest.Errors = append(manifest.Errors, debugLogFileError{
		Requested: requested,
		Path:      filePath,
		Reason:    reason,
	})
}

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
	debugLogFilesRequestMaxBytes = 64 * 1024
)

type debugLogFilesLimits struct {
	MaxFiles        int   `json:"max_files"`
	MaxGlobMatches  int   `json:"max_glob_matches"`
	MaxBytesPerFile int64 `json:"max_bytes_per_file"`
	MaxTotalBytes   int64 `json:"max_total_bytes"`
}

var defaultDebugLogFilesLimits = debugLogFilesLimits{
	MaxFiles:        100,
	MaxGlobMatches:  100,
	MaxBytesPerFile: debugLogsActiveMaxBytes,
	MaxTotalBytes:   debugLogsCombinedMaxBytes,
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

func (m *debugLogFilesManifest) appendError(requested string, filePath string, reason string) {
	m.Errors = append(m.Errors, debugLogFileError{
		Requested: requested,
		Path:      filePath,
		Reason:    reason,
	})
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

	extendDebugLogWriteDeadline(r.Context(), a.logger, w)

	ctx, cancel := context.WithTimeout(r.Context(), debugLogsWriteTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusOK)
	if err := collectDebugLogFiles(ctx, a.logger, home, req, w, defaultDebugLogFilesLimits); err != nil {
		a.logger.Error(r.Context(), "collect debug log files", slog.Error(err))
	}
}

// collectDebugLogFiles streams a zip archive with the requested files under
// files/ and a manifest.json describing the collection. Files are confined
// to home by os.Root, which refuses symlink escapes; per-path problems are
// recorded in the manifest instead of failing the archive.
func collectDebugLogFiles(ctx context.Context, logger slog.Logger, home string, req workspacesdk.DebugLogFilesRequest, w io.Writer, limits debugLogFilesLimits) error {
	paths := req.Paths
	pathsTruncated := len(paths) > debugLogFilesMaxPatterns
	if pathsTruncated {
		paths = paths[:debugLogFilesMaxPatterns]
	}
	manifest := debugLogFilesManifest{Requested: paths, Limits: limits}
	if pathsTruncated {
		manifest.Truncated = true
		manifest.appendError("", "", "requested path pattern limit reached")
	}

	// Requested paths are matched lexically against $HOME as users see it
	// in their shell. filepath.EvalSymlinks is deliberately avoided: it
	// fails on Windows volume mount points, and os.Root already blocks
	// symlink escapes at open time.
	home, err := filepath.Abs(home)
	var root *os.Root
	if err == nil {
		root, err = os.OpenRoot(home)
	}
	if err != nil {
		// Collect nothing; the archive still carries the manifest.
		manifest.appendError("", "", "open home directory: "+err.Error())
		paths = nil
	} else {
		defer root.Close()
	}

	zw := zip.NewWriter(w)
	remainingBytes := limits.MaxTotalBytes
	writtenFiles := 0
	seen := map[string]struct{}{}

patterns:
	for _, requested := range paths {
		if err := ctx.Err(); err != nil {
			manifest.Truncated = true
			manifest.appendError(requested, "", "collection canceled: "+err.Error())
			break
		}

		matches, matchesTruncated, err := debugLogFileMatches(ctx, root, home, requested, limits.MaxGlobMatches)
		if err != nil {
			if ctx.Err() != nil {
				manifest.Truncated = true
			}
			manifest.appendError(requested, "", err.Error())
			continue
		}
		if len(matches) == 0 {
			manifest.appendError(requested, "", "no matches")
			continue
		}
		if matchesTruncated {
			manifest.Truncated = true
			manifest.appendError(requested, "", "glob match limit reached")
		}

		for _, rel := range matches {
			if err := ctx.Err(); err != nil {
				manifest.Truncated = true
				manifest.appendError(requested, rel, "collection canceled: "+err.Error())
				break patterns
			}
			if writtenFiles >= limits.MaxFiles {
				manifest.Truncated = true
				manifest.appendError(requested, rel, "file count limit reached")
				break patterns
			}
			if remainingBytes <= 0 {
				manifest.Truncated = true
				manifest.appendError(requested, rel, "total byte limit reached")
				break patterns
			}
			// Dedupe by relative path to avoid duplicate archive entries.
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}

			// Root.Stat errors on any path component escaping the root.
			info, err := root.Stat(filepath.FromSlash(rel))
			if err != nil {
				reason := "stat path: " + err.Error()
				if errors.Is(err, fs.ErrNotExist) {
					reason = "does not exist"
				}
				manifest.appendError(requested, rel, reason)
				continue
			}
			if !info.Mode().IsRegular() {
				manifest.appendError(requested, rel, "not a regular file")
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
				manifest.appendError(requested, rel, err.Error())
				continue
			}
			manifest.Files = append(manifest.Files, entry)
			remainingBytes -= bytesToWrite
			writtenFiles++
		}
	}

	mf, err := zw.Create("manifest.json")
	if err != nil {
		return xerrors.Errorf("create manifest in archive: %w", err)
	}
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return xerrors.Errorf("write manifest: %w", err)
	}
	if err := zw.Close(); err != nil {
		return xerrors.Errorf("close archive: %w", err)
	}
	return nil
}

// debugLogFileMatches expands requested against home and returns matching
// home-relative slash paths. Non-glob paths return a single candidate
// without checking existence; the caller reports missing files on stat.
func debugLogFileMatches(ctx context.Context, root *os.Root, home string, requested string, maxMatches int) ([]string, bool, error) {
	rel, err := debugLogFileRelPattern(home, requested)
	if err != nil {
		return nil, false, err
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

// debugLogFilesFS cancels a glob walk once the request context ends. Only
// Open is implemented; the fs.ReadDir and fs.Stat helpers fall back to it,
// so every filesystem operation of the walk passes the context check.
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

// debugLogFileRelPattern expands $HOME, ${HOME}, or ~ in requested and
// returns it as a home-relative slash path. Relative paths are rejected,
// and absolute paths must be lexically under home.
func debugLogFileRelPattern(home string, requested string) (string, error) {
	pattern := strings.TrimSpace(requested)
	if pattern == "" {
		return "", xerrors.New("empty path")
	}
	switch pattern {
	case "$HOME", "${HOME}", "~":
		pattern = home
	default:
		for _, prefix := range []string{"$HOME/", "${HOME}/", "~/"} {
			if rest, ok := strings.CutPrefix(pattern, prefix); ok {
				pattern = filepath.Join(home, filepath.FromSlash(rest))
				break
			}
		}
		if !filepath.IsAbs(pattern) {
			return "", xerrors.New("relative path not allowed")
		}
	}
	rel, err := filepath.Rel(home, filepath.Clean(pattern))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", xerrors.New("outside home")
	}
	return filepath.ToSlash(rel), nil
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
	if _, err := io.Copy(w, io.LimitReader(f, entry.BytesWritten)); err != nil {
		return xerrors.Errorf("copy file: %w", err)
	}
	return nil
}

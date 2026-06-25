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
	ResolvedPath string    `json:"resolved_path"`
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

type debugLogFileCandidate struct {
	requested    string
	resolvedPath string
	relPath      string
	archivePath  string
	size         int64
	modTime      time.Time
}

var errDebugLogFilesGlobLimit = xerrors.New("debug log files glob match limit reached")

func (a *agent) HandleHTTPDebugLogFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	homeAbs, err := filepath.Abs(home)
	if err == nil {
		homeAbs, err = filepath.EvalSymlinks(homeAbs)
	}
	if err != nil {
		appendDebugLogFilesError(&manifest, "", "", "resolve home directory: "+err.Error())
		return writeDebugLogFilesManifestArchive(w, manifest)
	}
	homeAbs = filepath.Clean(homeAbs)

	root, err := os.OpenRoot(homeAbs)
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

		matches, matchesTruncated, err := debugLogFileMatches(ctx, root, homeAbs, requested, limits.MaxGlobMatches)
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

		for _, match := range matches {
			if err := ctx.Err(); err != nil {
				manifest.Truncated = true
				appendDebugLogFilesError(&manifest, requested, match, "collection canceled: "+err.Error())
				break patterns
			}
			if writtenFiles >= limits.MaxFiles {
				manifest.Truncated = true
				appendDebugLogFilesError(&manifest, requested, match, "file count limit reached")
				break patterns
			}
			if remainingBytes <= 0 {
				manifest.Truncated = true
				appendDebugLogFilesError(&manifest, requested, match, "total byte limit reached")
				break patterns
			}

			candidate, ok := debugLogFileCandidateForPath(homeAbs, requested, match, &manifest)
			if !ok {
				continue
			}
			if _, ok := seen[candidate.resolvedPath]; ok {
				continue
			}
			seen[candidate.resolvedPath] = struct{}{}

			bytesToWrite := min(candidate.size, limits.MaxBytesPerFile, remainingBytes)
			entry := debugLogFileManifestEntry{
				Requested:    candidate.requested,
				Path:         candidate.relPath,
				ResolvedPath: candidate.resolvedPath,
				ArchivePath:  candidate.archivePath,
				Size:         candidate.size,
				ModTime:      candidate.modTime,
				BytesWritten: bytesToWrite,
				Truncated:    bytesToWrite < candidate.size,
			}
			if entry.Truncated {
				manifest.Truncated = true
			}
			if err := writeDebugLogFileEntry(root, candidate, zw, bytesToWrite); err != nil {
				logger.Warn(ctx, "write debug log file", slog.Error(err), slog.F("path", candidate.resolvedPath))
				appendDebugLogFilesError(&manifest, candidate.requested, candidate.resolvedPath, err.Error())
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

func debugLogFileMatches(ctx context.Context, root *os.Root, home string, requested string, maxMatches int) ([]string, bool, error) {
	pattern, err := expandDebugLogFilePattern(home, requested)
	if err != nil {
		return nil, false, err
	}
	if !strings.ContainsAny(pattern, "*?{[") {
		if _, err := os.Lstat(pattern); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, xerrors.Errorf("does not exist")
			}
			return nil, false, xerrors.Errorf("stat path: %w", err)
		}
		return []string{pattern}, false, nil
	}

	rel, ok := homeRelativePath(home, pattern)
	if !ok {
		return nil, false, xerrors.Errorf("outside home")
	}

	matches := make([]string, 0, maxMatches+1)
	err = doublestar.GlobWalk(debugLogFilesFS{ctx: ctx, fsys: root.FS()}, filepath.ToSlash(rel), func(match string, _ fs.DirEntry) error {
		matches = append(matches, filepath.Join(home, filepath.FromSlash(match)))
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
	if readDirFS, ok := f.fsys.(fs.ReadDirFS); ok {
		return readDirFS.ReadDir(name)
	}
	return fs.ReadDir(f.fsys, name)
}

func (f debugLogFilesFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	if statFS, ok := f.fsys.(fs.StatFS); ok {
		return statFS.Stat(name)
	}
	return fs.Stat(f.fsys, name)
}

func expandDebugLogFilePattern(home string, requested string) (string, error) {
	trimmed := strings.TrimSpace(requested)
	if trimmed == "" {
		return "", xerrors.Errorf("empty path")
	}

	var expanded string
	switch {
	case trimmed == "$HOME" || trimmed == "${HOME}" || trimmed == "~":
		expanded = home
	case strings.HasPrefix(trimmed, "$HOME/"):
		expanded = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(trimmed, "$HOME/")))
	case strings.HasPrefix(trimmed, "${HOME}/"):
		expanded = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(trimmed, "${HOME}/")))
	case strings.HasPrefix(trimmed, "~/"):
		expanded = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(trimmed, "~/")))
	case filepath.IsAbs(trimmed):
		expanded = trimmed
	default:
		return "", xerrors.Errorf("relative path not allowed")
	}

	return filepath.Clean(expanded), nil
}

func debugLogFileCandidateForPath(home string, requested string, match string, manifest *debugLogFilesManifest) (debugLogFileCandidate, bool) {
	resolved, err := filepath.EvalSymlinks(match)
	if err != nil {
		reason := "resolve path: " + err.Error()
		if errors.Is(err, os.ErrNotExist) {
			reason = "does not exist"
		}
		appendDebugLogFilesError(manifest, requested, match, reason)
		return debugLogFileCandidate{}, false
	}
	resolved = filepath.Clean(resolved)
	rel, ok := homeRelativePath(home, resolved)
	if !ok {
		appendDebugLogFilesError(manifest, requested, match, "outside home")
		return debugLogFileCandidate{}, false
	}
	info, err := os.Stat(resolved)
	if err != nil {
		appendDebugLogFilesError(manifest, requested, match, "stat path: "+err.Error())
		return debugLogFileCandidate{}, false
	}
	if !info.Mode().IsRegular() {
		appendDebugLogFilesError(manifest, requested, match, "not a regular file")
		return debugLogFileCandidate{}, false
	}
	return debugLogFileCandidate{
		requested:    requested,
		resolvedPath: resolved,
		relPath:      filepath.ToSlash(rel),
		archivePath:  debugLogFilesArchivePath(rel),
		size:         info.Size(),
		modTime:      info.ModTime(),
	}, true
}

func homeRelativePath(home string, filePath string) (string, bool) {
	rel, err := filepath.Rel(home, filePath)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return rel, true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

func debugLogFilesArchivePath(rel string) string {
	return path.Join("files", path.Clean(filepath.ToSlash(rel)))
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

func writeDebugLogFileEntry(root *os.Root, candidate debugLogFileCandidate, zw *zip.Writer, bytesToWrite int64) error {
	f, err := root.Open(filepath.FromSlash(candidate.relPath))
	if err != nil {
		return xerrors.Errorf("open file: %w", err)
	}
	defer f.Close()

	if bytesToWrite < candidate.size {
		if _, err := f.Seek(candidate.size-bytesToWrite, io.SeekStart); err != nil {
			return xerrors.Errorf("seek tail: %w", err)
		}
	}
	entry, err := zw.Create(candidate.archivePath)
	if err != nil {
		return xerrors.Errorf("create archive entry: %w", err)
	}
	if _, err := copyDebugLog(entry, f, bytesToWrite); err != nil {
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

package agent

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	debugLogFilesMaxPatterns     = 32
	debugLogFilesRequestMaxBytes = 64 * 1024
)

// Independent of the log streaming caps in debuglogs.go.
var defaultDebugLogFilesLimits = workspacesdk.DebugLogFilesLimits{
	MaxFiles:        100,
	MaxGlobMatches:  100,
	MaxBytesPerFile: 10 * 1024 * 1024,
	MaxTotalBytes:   100 * 1024 * 1024,
}

var (
	errDebugLogFilesGlobLimit      = xerrors.New("debug log files glob match limit reached")
	errDebugLogFilePathEmpty       = xerrors.New("empty path")
	errDebugLogFilePathRelative    = xerrors.New("relative path not allowed")
	errDebugLogFilePathOutsideHome = xerrors.New("outside home")
)

func (a *agent) HandleHTTPDebugLogFiles(w http.ResponseWriter, r *http.Request) {
	var req workspacesdk.DebugLogFilesRequest
	r.Body = http.MaxBytesReader(w, r.Body, debugLogFilesRequestMaxBytes)
	if !httpapi.Read(r.Context(), w, r, &req) {
		return
	}

	home, err := a.envInfo.HomeDir()
	if err != nil {
		a.logger.Error(r.Context(), "get user home dir", slog.Error(err))
		httpapi.InternalServerError(w, xerrors.Errorf("get user home dir: %w", err))
		return
	}

	extendDebugLogWriteDeadline(r.Context(), a.logger, w)

	ctx, cancel := context.WithTimeout(r.Context(), debugLogsWriteTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusOK)
	if err := collectDebugLogFiles(ctx, home, req, w, defaultDebugLogFilesLimits); err != nil {
		a.logger.Error(r.Context(), "collect debug log files", slog.Error(err))
	}
}

// collectDebugLogFiles streams a zip with the requested files under files/
// and a manifest.json describing the collection. os.Root confines reads to
// home; per-path problems are recorded in the manifest, not fatal.
func collectDebugLogFiles(ctx context.Context, home string, req workspacesdk.DebugLogFilesRequest, w io.Writer, limits workspacesdk.DebugLogFilesLimits) error {
	manifest := workspacesdk.DebugLogFilesManifest{Requested: req.Paths, Limits: limits}
	paths := req.Paths
	if len(paths) > debugLogFilesMaxPatterns {
		manifest.Truncated = true
		for _, skipped := range paths[debugLogFilesMaxPatterns:] {
			appendManifestError(&manifest, skipped, "", "requested path pattern limit reached")
		}
		paths = paths[:debugLogFilesMaxPatterns]
	}

	// Matching is lexical; filepath.EvalSymlinks fails on Windows mount
	// points, and os.Root already blocks symlink escapes.
	home, err := filepath.Abs(home)
	var root *os.Root
	if err == nil {
		root, err = os.OpenRoot(home)
	}
	if err != nil {
		// Collect nothing; the archive still carries the manifest.
		appendManifestError(&manifest, "", "", "open home directory: "+err.Error())
		paths = nil
	} else {
		defer root.Close()
	}

	zw := zip.NewWriter(w)
	c := &debugLogFilesCollector{
		zw:        zw,
		root:      root,
		home:      home,
		limits:    limits,
		manifest:  &manifest,
		seen:      map[string]struct{}{},
		remaining: limits.MaxTotalBytes,
	}
	for _, requested := range paths {
		if !c.collectPattern(ctx, requested) {
			break
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

// debugLogFilesCollector streams matched files into an archive while
// enforcing limits and recording per-path problems in the manifest. Collect
// methods return false once a global limit ends collection.
type debugLogFilesCollector struct {
	zw        *zip.Writer
	root      *os.Root
	home      string
	limits    workspacesdk.DebugLogFilesLimits
	manifest  *workspacesdk.DebugLogFilesManifest
	seen      map[string]struct{}
	remaining int64
	written   int
}

func (c *debugLogFilesCollector) collectPattern(ctx context.Context, requested string) bool {
	if err := ctx.Err(); err != nil {
		return c.stop(requested, "", "collection canceled: "+err.Error())
	}

	matches, matchesTruncated, err := debugLogFileMatches(ctx, c.root, c.home, requested, c.limits.MaxGlobMatches)
	if err != nil {
		if ctx.Err() != nil {
			c.manifest.Truncated = true
		}
		appendManifestError(c.manifest, requested, "", err.Error())
		return true
	}
	if len(matches) == 0 {
		appendManifestError(c.manifest, requested, "", "no matches")
		return true
	}
	if matchesTruncated {
		c.manifest.Truncated = true
		appendManifestError(c.manifest, requested, "", "glob match limit reached")
	}

	for _, rel := range matches {
		if !c.collectFile(ctx, requested, rel) {
			return false
		}
	}
	return true
}

func (c *debugLogFilesCollector) collectFile(ctx context.Context, requested string, rel string) bool {
	if err := ctx.Err(); err != nil {
		return c.stop(requested, rel, "collection canceled: "+err.Error())
	}
	if c.written >= c.limits.MaxFiles {
		return c.stop(requested, rel, "file count limit reached")
	}
	if c.remaining <= 0 {
		return c.stop(requested, rel, "total byte limit reached")
	}
	if _, ok := c.seen[rel]; ok {
		return true
	}
	c.seen[rel] = struct{}{}

	// Stat before open: opening a FIFO would block. Root.Stat also errors
	// on any path escaping the root.
	info, err := c.root.Stat(filepath.FromSlash(rel))
	if err != nil {
		reason := "stat path: " + err.Error()
		if errors.Is(err, fs.ErrNotExist) {
			reason = "does not exist"
		}
		appendManifestError(c.manifest, requested, rel, reason)
		return true
	}
	if !info.Mode().IsRegular() {
		appendManifestError(c.manifest, requested, rel, "not a regular file")
		return true
	}

	bytesToWrite := min(info.Size(), c.limits.MaxBytesPerFile, c.remaining)
	entry := workspacesdk.DebugLogFilesManifestEntry{
		Requested:    requested,
		Path:         rel,
		ArchivePath:  path.Join("files", rel),
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		BytesWritten: bytesToWrite,
		Truncated:    bytesToWrite < info.Size(),
	}
	c.manifest.Truncated = c.manifest.Truncated || entry.Truncated
	if err := writeDebugLogFileEntry(c.root, c.zw, entry); err != nil {
		appendManifestError(c.manifest, requested, rel, err.Error())
		return true
	}
	c.manifest.Files = append(c.manifest.Files, entry)
	c.remaining -= bytesToWrite
	c.written++
	return true
}

// stop marks the manifest truncated, records the reason, and halts
// collection.
func (c *debugLogFilesCollector) stop(requested string, filePath string, reason string) bool {
	c.manifest.Truncated = true
	appendManifestError(c.manifest, requested, filePath, reason)
	return false
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

// debugLogFileRelPattern expands a $HOME/, ${HOME}/, or ~/ prefix in
// requested and returns it as a home-relative slash path. Relative paths
// are rejected, and absolute paths must be lexically under home.
func debugLogFileRelPattern(home string, requested string) (string, error) {
	pattern := strings.TrimSpace(requested)
	if pattern == "" {
		return "", errDebugLogFilePathEmpty
	}
	for _, prefix := range []string{"$HOME/", "${HOME}/", "~/"} {
		if rest, ok := strings.CutPrefix(pattern, prefix); ok {
			pattern = filepath.Join(home, filepath.FromSlash(rest))
			break
		}
	}
	if !filepath.IsAbs(pattern) {
		return "", errDebugLogFilePathRelative
	}
	rel, err := filepath.Rel(home, filepath.Clean(pattern))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errDebugLogFilePathOutsideHome
	}
	return filepath.ToSlash(rel), nil
}

// writeDebugLogFileEntry copies the last entry.BytesWritten bytes of the
// file at entry.Path into the archive at entry.ArchivePath.
func writeDebugLogFileEntry(root *os.Root, zw *zip.Writer, entry workspacesdk.DebugLogFilesManifestEntry) error {
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

func appendManifestError(m *workspacesdk.DebugLogFilesManifest, requested string, filePath string, reason string) {
	m.Errors = append(m.Errors, workspacesdk.DebugLogFilesManifestError{
		Requested: requested,
		Path:      filePath,
		Reason:    reason,
	})
}

package agentfiles

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	zipFilesMaxPatterns     = 32
	zipFilesRequestMaxBytes = 64 * 1024
	// zipFilesWriteTimeout gives slow links well over the server's 20s
	// WriteTimeout to stream the archive.
	zipFilesWriteTimeout = 5 * time.Minute
)

// defaultZipFilesLimits caps a single collection. Zip headers and manifest
// entries are not counted by the byte budget, so MaxFiles bounds them.
var defaultZipFilesLimits = workspacesdk.ZipFilesLimits{
	MaxFiles:        10000,
	MaxBytesPerFile: 10 * 1024 * 1024,
	MaxTotalBytes:   100 * 1024 * 1024,
}

var (
	errZipFilesFileLimit   = xerrors.New("zip files file count limit reached")
	errZipFilePathEmpty    = xerrors.New("empty path")
	errZipFilePathRelative = xerrors.New("relative path not allowed")
)

// HandleZipFiles streams a zip archive of the requested workspace files.
// Paths must be absolute or use a $HOME/, ${HOME}/, or ~/ prefix, which is
// expanded against the agent user's home directory.
func (api *API) HandleZipFiles(w http.ResponseWriter, r *http.Request) {
	var req workspacesdk.ZipFilesRequest
	r.Body = http.MaxBytesReader(w, r.Body, zipFilesRequestMaxBytes)
	if !httpapi.Read(r.Context(), w, r, &req) {
		return
	}

	home, err := api.envInfo.HomeDir()
	if err != nil {
		api.logger.Error(r.Context(), "get user home dir", slog.Error(err))
		httpapi.InternalServerError(w, xerrors.Errorf("get user home dir: %w", err))
		return
	}

	// Give slow links well over the server's 20s WriteTimeout to stream
	// the archive.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(zipFilesWriteTimeout)); err != nil {
		api.logger.Warn(r.Context(), "extend zip files write deadline", slog.Error(err))
	}

	clientCtx := r.Context()
	ctx, cancel := context.WithTimeout(clientCtx, zipFilesWriteTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusOK)
	if err := collectZipFiles(ctx, clientCtx, home, req, w, api.zipFilesLimits); err != nil {
		api.logger.Error(clientCtx, "collect zip files", slog.Error(err))
	}
}

// collectZipFiles streams a zip with the requested files under files/ and a
// manifest.json describing the collection. Per-path problems are recorded in
// the manifest, not fatal. ctx bounds the collection; clientCtx is the
// request context.
func collectZipFiles(ctx, clientCtx context.Context, home string, req workspacesdk.ZipFilesRequest, w io.Writer, limits workspacesdk.ZipFilesLimits) error {
	manifest := workspacesdk.ZipFilesManifest{Requested: req.Paths, Limits: limits}
	paths := req.Paths
	if len(paths) > zipFilesMaxPatterns {
		manifest.Truncated = true
		for _, skipped := range paths[zipFilesMaxPatterns:] {
			appendManifestError(&manifest, skipped, "", "requested path pattern limit reached")
		}
		paths = paths[:zipFilesMaxPatterns]
	}

	home, err := filepath.Abs(home)
	if err != nil {
		// Collect nothing; the archive still carries the manifest.
		appendManifestError(&manifest, "", "", "resolve home directory: "+err.Error())
		paths = nil
	}

	zw := zip.NewWriter(w)
	c := &zipFilesCollector{
		zw:        zw,
		clientCtx: clientCtx,
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
	if clientCtx.Err() != nil {
		// The client is gone; there is nobody to receive the manifest.
		return xerrors.Errorf("client disconnected: %w", clientCtx.Err())
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

// zipFilesCollector streams matched files into an archive while enforcing
// limits and recording per-path problems in the manifest. Collect methods
// return false once a global limit ends collection.
type zipFilesCollector struct {
	zw        *zip.Writer
	clientCtx context.Context
	home      string
	limits    workspacesdk.ZipFilesLimits
	manifest  *workspacesdk.ZipFilesManifest
	seen      map[string]struct{}
	remaining int64
	written   int
}

func (c *zipFilesCollector) collectPattern(ctx context.Context, requested string) bool {
	if ctx.Err() != nil {
		return c.stopCanceled(requested, "")
	}
	if c.written >= c.limits.MaxFiles {
		return c.stop(requested, "", "file count limit reached")
	}

	matches, matchesTruncated, err := zipFileMatches(ctx, c.home, requested, c.limits.MaxFiles-c.written)
	if err != nil {
		if ctx.Err() != nil {
			return c.stopCanceled(requested, "")
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
		appendManifestError(c.manifest, requested, "", "file count limit reached")
	}

	for _, abs := range matches {
		if !c.collectFile(ctx, requested, abs) {
			return false
		}
	}
	return true
}

func (c *zipFilesCollector) collectFile(ctx context.Context, requested string, abs string) bool {
	if ctx.Err() != nil {
		return c.stopCanceled(requested, abs)
	}
	if c.written >= c.limits.MaxFiles {
		return c.stop(requested, abs, "file count limit reached")
	}
	if c.remaining <= 0 {
		return c.stop(requested, abs, "total byte limit reached")
	}
	if _, ok := c.seen[abs]; ok {
		return true
	}
	c.seen[abs] = struct{}{}

	// Stat before open: opening a FIFO would block. Stat follows symlinks,
	// so a directly requested symlink collects its target.
	info, err := os.Stat(abs)
	if err != nil {
		reason := "stat path: " + err.Error()
		if errors.Is(err, fs.ErrNotExist) {
			reason = "does not exist"
		}
		appendManifestError(c.manifest, requested, abs, reason)
		return true
	}
	if !info.Mode().IsRegular() {
		appendManifestError(c.manifest, requested, abs, "not a regular file: "+fileModeTypeName(info.Mode()))
		return true
	}

	bytesToWrite := min(info.Size(), c.limits.MaxBytesPerFile, c.remaining)
	entry := workspacesdk.ZipFilesManifestEntry{
		Requested:    requested,
		Path:         abs,
		ArchivePath:  zipFilesArchivePath(abs),
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		BytesWritten: bytesToWrite,
		Truncated:    bytesToWrite < info.Size(),
	}
	c.manifest.Truncated = c.manifest.Truncated || entry.Truncated
	if err := writeZipFileEntry(c.zw, abs, entry); err != nil {
		appendManifestError(c.manifest, requested, abs, err.Error())
		return true
	}
	c.manifest.Files = append(c.manifest.Files, entry)
	c.remaining -= bytesToWrite
	c.written++
	return true
}

// stop marks the manifest truncated, records the reason, and halts
// collection.
func (c *zipFilesCollector) stop(requested string, filePath string, reason string) bool {
	c.manifest.Truncated = true
	appendManifestError(c.manifest, requested, filePath, reason)
	return false
}

// stopCanceled halts collection after the collection context ended: a
// timeout is recorded in the manifest and the archive is finished, while a
// client disconnect makes the caller abort without a manifest.
func (c *zipFilesCollector) stopCanceled(requested string, filePath string) bool {
	if c.clientCtx.Err() != nil {
		return false
	}
	return c.stop(requested, filePath, "collection timed out")
}

// zipFileMatches expands requested against home and returns matching
// cleaned absolute paths. Non-glob paths return a single candidate without
// checking existence; the caller reports missing files on stat.
func zipFileMatches(ctx context.Context, home string, requested string, maxMatches int) ([]string, bool, error) {
	abs, err := zipFileAbsPattern(home, requested)
	if err != nil {
		return nil, false, err
	}
	if !strings.ContainsAny(abs, "*?{[") {
		return []string{abs}, false, nil
	}

	base, pattern := doublestar.SplitPattern(filepath.ToSlash(abs))
	matches := make([]string, 0, min(maxMatches+1, 64))
	// WithNoFollow avoids symlink cycles during traversal.
	err = doublestar.GlobWalk(zipFilesFS{ctx: ctx, fsys: os.DirFS(base)}, pattern, func(match string, _ fs.DirEntry) error {
		matches = append(matches, filepath.Join(base, filepath.FromSlash(match)))
		if len(matches) > maxMatches {
			return errZipFilesFileLimit
		}
		return nil
	}, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
	matchesTruncated := errors.Is(err, errZipFilesFileLimit)
	if err != nil && !matchesTruncated {
		return nil, false, xerrors.Errorf("glob pattern: %w", err)
	}
	if matchesTruncated {
		matches = matches[:maxMatches]
	}
	// doublestar does not guarantee ordering, so sort for a deterministic
	// archive.
	slices.Sort(matches)
	return matches, matchesTruncated, nil
}

// zipFilesFS cancels a glob walk once the request context ends. Only Open
// is implemented; the fs.ReadDir and fs.Stat helpers fall back to it, so
// every filesystem operation of the walk passes the context check.
type zipFilesFS struct {
	ctx  context.Context
	fsys fs.FS
}

func (f zipFilesFS) Open(name string) (fs.File, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return f.fsys.Open(name)
}

// zipFileAbsPattern expands a $HOME/, ${HOME}/, or ~/ prefix against home
// and returns the cleaned absolute pattern. Relative paths are rejected.
func zipFileAbsPattern(home string, requested string) (string, error) {
	pattern := strings.TrimSpace(requested)
	if pattern == "" {
		return "", errZipFilePathEmpty
	}
	for _, prefix := range []string{"$HOME/", "${HOME}/", "~/"} {
		if rest, ok := strings.CutPrefix(pattern, prefix); ok {
			pattern = filepath.Join(home, filepath.FromSlash(rest))
			break
		}
	}
	if !filepath.IsAbs(pattern) {
		return "", errZipFilePathRelative
	}
	return filepath.Clean(pattern), nil
}

// zipFilesArchivePath maps a cleaned absolute path to its archive entry
// name: files/ plus the path with the leading separator trimmed and any
// Windows drive colon dropped, keeping the name fs.ValidPath-safe.
func zipFilesArchivePath(abs string) string {
	p := strings.TrimPrefix(filepath.ToSlash(abs), "/")
	if len(p) >= 2 && p[1] == ':' {
		p = p[:1] + p[2:]
	}
	return "files/" + p
}

// fileModeTypeName names the type of a non-regular file.
func fileModeTypeName(mode fs.FileMode) string {
	switch {
	case mode.IsDir():
		return "directory"
	case mode&fs.ModeSymlink != 0:
		return "symlink"
	case mode&fs.ModeNamedPipe != 0:
		return "named pipe"
	case mode&fs.ModeSocket != 0:
		return "socket"
	case mode&fs.ModeDevice != 0, mode&fs.ModeCharDevice != 0:
		return "device"
	default:
		return "irregular file"
	}
}

// writeZipFileEntry copies the last entry.BytesWritten bytes of the file at
// abs into the archive at entry.ArchivePath.
func writeZipFileEntry(zw *zip.Writer, abs string, entry workspacesdk.ZipFilesManifestEntry) error {
	f, err := os.Open(abs)
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

func appendManifestError(m *workspacesdk.ZipFilesManifest, requested string, filePath string, reason string) {
	m.Errors = append(m.Errors, workspacesdk.ZipFilesManifestError{
		Requested: requested,
		Path:      filePath,
		Reason:    reason,
	})
}

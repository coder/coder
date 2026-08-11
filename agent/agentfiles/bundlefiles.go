package agentfiles

import (
	"archive/tar"
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
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	bundleFilesRequestMaxBytes = 64 * 1024
	// bundleFilesWriteTimeout gives slow links well over the server's 20s
	// WriteTimeout to stream the archive.
	bundleFilesWriteTimeout = 5 * time.Minute

	tarBlockSize = 512
)

// defaultBundleFilesLimits caps a single collection. Tar headers, block
// padding, and manifest file entries are charged against MaxTotalBytes,
// so it approximately bounds the response size.
var defaultBundleFilesLimits = workspacesdk.BundleFilesLimits{
	MaxFiles:        10000,
	MaxBytesPerFile: 10 * 1024 * 1024,
	MaxTotalBytes:   100 * 1024 * 1024,
}

var errBundleFilesFileLimit = xerrors.New("bundle files file count limit reached")

// HandleBundleFiles streams a tar archive of the requested workspace
// files. Environment variables in paths are expanded in the agent's
// environment; paths must then be absolute or start with ~/, which
// resolves against the agent user's home directory.
func (api *API) HandleBundleFiles(w http.ResponseWriter, r *http.Request) {
	var req workspacesdk.BundleFilesRequest
	r.Body = http.MaxBytesReader(w, r.Body, bundleFilesRequestMaxBytes)
	if !httpapi.Read(r.Context(), w, r, &req) {
		return
	}

	home, err := api.envInfo.HomeDir()
	if err != nil {
		api.logger.Error(r.Context(), "get user home dir", slog.Error(err))
		httpapi.InternalServerError(w, xerrors.Errorf("get user home dir: %w", err))
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(bundleFilesWriteTimeout)); err != nil {
		api.logger.Warn(r.Context(), "extend bundle files write deadline", slog.Error(err))
	}

	clientCtx := r.Context()
	ctx, cancel := context.WithTimeout(clientCtx, bundleFilesWriteTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	if err := collectBundleFiles(ctx, clientCtx, home, req, w, api.bundleFilesLimits); err != nil {
		api.logger.Error(clientCtx, "collect bundle files", slog.Error(err))
	}
}

// collectBundleFiles streams a tar with the requested files under files/
// and a manifest.json describing the collection. Per-path problems are
// recorded in the manifest, not fatal. ctx bounds the collection;
// clientCtx is the request context.
func collectBundleFiles(ctx, clientCtx context.Context, home string, req workspacesdk.BundleFilesRequest, w io.Writer, limits workspacesdk.BundleFilesLimits) error {
	manifest := workspacesdk.BundleFilesManifest{Requested: req.Paths, Limits: limits}
	paths := req.Paths

	home, err := filepath.Abs(home)
	if err != nil {
		// Collect nothing; the archive still carries the manifest.
		appendManifestError(&manifest, "", "", "resolve home directory: "+err.Error())
		paths = nil
	}

	tw := tar.NewWriter(w)
	c := &bundleFilesCollector{
		tw:             tw,
		clientCtx:      clientCtx,
		home:           home,
		limits:         limits,
		manifest:       &manifest,
		seenPaths:      map[string]struct{}{},
		remainingBytes: limits.MaxTotalBytes,
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

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshal manifest: %w", err)
	}
	err = tw.WriteHeader(&tar.Header{
		Name:    "manifest.json",
		Mode:    0o644,
		Size:    int64(len(manifestJSON)),
		ModTime: time.Now(),
	})
	if err != nil {
		return xerrors.Errorf("create manifest in archive: %w", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		return xerrors.Errorf("write manifest: %w", err)
	}
	if err := tw.Close(); err != nil {
		return xerrors.Errorf("close archive: %w", err)
	}
	return nil
}

// bundleFilesCollector streams matched files into an archive while
// enforcing limits and recording per-path problems in the manifest.
// Collect methods return false once a global limit ends collection.
type bundleFilesCollector struct {
	tw             *tar.Writer
	clientCtx      context.Context
	home           string
	limits         workspacesdk.BundleFilesLimits
	manifest       *workspacesdk.BundleFilesManifest
	seenPaths      map[string]struct{}
	remainingBytes int64
	filesWritten   int
}

func (c *bundleFilesCollector) collectPattern(ctx context.Context, requested string) bool {
	if ctx.Err() != nil {
		return c.stopCanceled(requested, "")
	}
	if c.filesWritten >= c.limits.MaxFiles {
		return c.stop(requested, "", "file count limit reached")
	}

	matches, matchesTruncated, err := bundleFileMatches(ctx, c.home, requested, c.limits.MaxFiles-c.filesWritten)
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

func (c *bundleFilesCollector) collectFile(ctx context.Context, requested string, abs string) bool {
	if ctx.Err() != nil {
		return c.stopCanceled(requested, abs)
	}
	if c.filesWritten >= c.limits.MaxFiles {
		return c.stop(requested, abs, "file count limit reached")
	}
	// Each entry costs a tar header block before any data fits.
	if c.remainingBytes <= tarBlockSize {
		return c.stop(requested, abs, "total byte limit reached")
	}
	if _, ok := c.seenPaths[abs]; ok {
		return true
	}
	c.seenPaths[abs] = struct{}{}

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

	bytesToWrite := min(info.Size(), c.limits.MaxBytesPerFile, c.remainingBytes-tarBlockSize)
	entry := workspacesdk.BundleFilesManifestEntry{
		Requested:    requested,
		Path:         abs,
		ArchivePath:  BundleFilesArchivePath(abs),
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		BytesWritten: bytesToWrite,
		Truncated:    bytesToWrite < info.Size(),
	}
	c.manifest.Truncated = c.manifest.Truncated || entry.Truncated
	if err := writeBundleFileEntry(c.tw, abs, entry); err != nil {
		appendManifestError(c.manifest, requested, abs, err.Error())
		return true
	}
	c.manifest.Files = append(c.manifest.Files, entry)
	// The last file may overshoot the budget by under a block; the bound
	// is approximate, not exact.
	entryJSON, _ := json.Marshal(entry)
	c.remainingBytes -= tarEntrySize(bytesToWrite) + int64(len(entryJSON))
	c.filesWritten++
	return true
}

// stop marks the manifest truncated, records the reason, and halts
// collection.
func (c *bundleFilesCollector) stop(requested string, filePath string, reason string) bool {
	c.manifest.Truncated = true
	appendManifestError(c.manifest, requested, filePath, reason)
	return false
}

// stopCanceled halts collection after the collection context ended: a
// timeout is recorded in the manifest and the archive is finished, while a
// client disconnect makes the caller abort without a manifest.
func (c *bundleFilesCollector) stopCanceled(requested string, filePath string) bool {
	if c.clientCtx.Err() != nil {
		return false
	}
	return c.stop(requested, filePath, "exceeded maximum collection time")
}

// bundleFileMatches expands requested against home and returns matching
// cleaned absolute paths. Non-glob paths return a single candidate without
// checking existence; the caller reports missing files on stat.
func bundleFileMatches(ctx context.Context, home string, requested string, maxMatches int) ([]string, bool, error) {
	// Env vars expand from the agent environment and ~ resolves against
	// the agent home, matching the agent's expandPathToAbs. Glob patterns
	// never exist on disk, so canonicalization keeps them lexical.
	abs, err := agentcontext.CanonicalizePathIn(home, os.ExpandEnv(requested))
	if err != nil {
		return nil, false, err
	}
	if !strings.ContainsAny(abs, "*?{[") {
		return []string{abs}, false, nil
	}

	base, pattern := doublestar.SplitPattern(filepath.ToSlash(abs))
	matches := make([]string, 0, min(maxMatches, 64))
	// WithNoFollow avoids symlink cycles. Checking the limit before the
	// append keeps matches from growing past maxMatches.
	err = doublestar.GlobWalk(bundleFilesFS{ctx: ctx, fsys: os.DirFS(base)}, pattern, func(match string, _ fs.DirEntry) error {
		if len(matches) >= maxMatches {
			return errBundleFilesFileLimit
		}
		matches = append(matches, filepath.Join(base, filepath.FromSlash(match)))
		return nil
	}, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
	matchesTruncated := errors.Is(err, errBundleFilesFileLimit)
	if err != nil && !matchesTruncated {
		return nil, false, xerrors.Errorf("glob pattern: %w", err)
	}
	// doublestar does not guarantee ordering, so sort for a deterministic
	// archive.
	slices.Sort(matches)
	return matches, matchesTruncated, nil
}

// bundleFilesFS cancels a glob walk once the request context ends. Only
// Open is implemented; the fs.ReadDir and fs.Stat helpers fall back to it,
// so every filesystem operation of the walk passes the context check.
type bundleFilesFS struct {
	ctx  context.Context
	fsys fs.FS
}

func (f bundleFilesFS) Open(name string) (fs.File, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return f.fsys.Open(name)
}

// BundleFilesArchivePath maps a cleaned absolute path to its archive entry
// name: files/ plus the path with the leading separator trimmed and any
// Windows drive colon dropped, keeping the name fs.ValidPath-safe.
func BundleFilesArchivePath(abs string) string {
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

// writeBundleFileEntry writes the last entry.BytesWritten bytes of the
// file at abs to the archive at entry.ArchivePath. A file that shrinks
// after stat is zero-padded to the declared size, since a short entry
// would corrupt every entry after it; the short read is still an error.
func writeBundleFileEntry(tw *tar.Writer, abs string, entry workspacesdk.BundleFilesManifestEntry) error {
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
	err = tw.WriteHeader(&tar.Header{
		Name:    entry.ArchivePath,
		Mode:    0o644,
		Size:    entry.BytesWritten,
		ModTime: entry.ModTime,
	})
	if err != nil {
		return xerrors.Errorf("create archive entry: %w", err)
	}
	n, err := io.Copy(tw, io.LimitReader(f, entry.BytesWritten))
	if err == nil && n < entry.BytesWritten {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		if _, padErr := io.CopyN(tw, zeroReader{}, entry.BytesWritten-n); padErr != nil {
			return xerrors.Errorf("pad short entry: %w", padErr)
		}
		return xerrors.Errorf("copy file: %w", err)
	}
	return nil
}

// tarEntrySize returns the archive bytes a file entry consumes: one
// header block plus the data rounded up to whole blocks.
func tarEntrySize(dataBytes int64) int64 {
	return tarBlockSize + (dataBytes+tarBlockSize-1)/tarBlockSize*tarBlockSize
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func appendManifestError(m *workspacesdk.BundleFilesManifest, requested string, filePath string, reason string) {
	m.Errors = append(m.Errors, workspacesdk.BundleFilesManifestError{
		Requested: requested,
		Path:      filePath,
		Reason:    reason,
	})
}

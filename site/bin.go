package site

import (
	"archive/tar"
	"bytes"
	"crypto/sha1" // nolint: gosec // not used for cryptography
	"encoding/hex"
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
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cachecompress"
)

const CompressionLevel = 5

// errHashMismatch is a sentinel error used in verifyBinSha1IsCurrent.
var errHashMismatch = xerrors.New("hash mismatch")

type binHandler struct {
	metadataCache *binMetadataCache
	handler       http.Handler
}

var StandardEncoders = map[string]func(w io.Writer, level int) io.WriteCloser{
	"br": func(w io.Writer, level int) io.WriteCloser {
		return brotli.NewWriterLevel(w, level)
	},
	"zstd": func(w io.Writer, level int) io.WriteCloser {
		zw, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
		if err != nil {
			panic("invalid zstd compressor: " + err.Error())
		}
		return zw
	},
}

func (h *binHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/bin/") {
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
		return
	}
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/bin")
	// Convert underscores in the filename to hyphens. We eventually want to
	// change our hyphen-based filenames to underscores, but we need to
	// support both for now.
	r.URL.Path = strings.ReplaceAll(r.URL.Path, "_", "-")

	// Set ETag header to the SHA1 hash of the file contents.
	name := filePath(r.URL.Path)
	if name == "" || name == "/" {
		// Serve the directory listing. This intentionally allows directory listings to
		// be served. This file system should not contain anything sensitive.
		h.handler.ServeHTTP(rw, r)
		return
	}
	if strings.Contains(name, "/") {
		// We only serve files from the root of this directory, so avoid any
		// shenanigans by blocking slashes in the URL path.
		http.NotFound(rw, r)
		return
	}

	metadata, err := h.metadataCache.getMetadata(name)
	if xerrors.Is(err, os.ErrNotExist) {
		http.NotFound(rw, r)
		return
	}
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// http.FileServer will not set Content-Length when performing chunked
	// transport encoding, which is used for large files like our binaries
	// so stream compression can be used.
	//
	// Clients like IDE extensions and the desktop apps can compare the
	// value of this header with the amount of bytes written to disk after
	// decompression to show progress. Without this, they cannot show
	// progress without disabling compression.
	//
	// There isn't really a spec for a length header for the "inner" content
	// size, but some nginx modules use this header.
	rw.Header().Set("X-Original-Content-Length", fmt.Sprintf("%d", metadata.sizeBytes))

	// Get and set ETag header. Must be quoted.
	rw.Header().Set("ETag", fmt.Sprintf(`%q`, metadata.sha1Hash))

	// http.FileServer will see the ETag header and automatically handle
	// If-Match and If-None-Match headers on the request properly.
	h.handler.ServeHTTP(rw, r)
}

func newBinHandler(options *Options) (*binHandler, error) {
	cacheDir := options.CacheDir
	compressedCacheDir := ""
	if cacheDir != "" {
		// split the cache dir into ./compressed and ./orig containing the compressed files and the original
		// uncompressed files respectively.
		compressedCacheDir = filepath.Join(cacheDir, "compressed")
		err := os.MkdirAll(compressedCacheDir, 0o700)
		if err != nil {
			// cached dir was provided, but we can't write to it
			return nil, xerrors.Errorf("failed to create compressed directory in cache dir: %w", err)
		}
		cacheDir = filepath.Join(cacheDir, "orig")
		err = os.MkdirAll(cacheDir, 0o700)
		if err != nil {
			return nil, xerrors.Errorf("failed to create orig directory in cache dir: %w", err)
		}
	}
	// note that ExtractOrReadBinFS handles an empty cacheDir; this often arises in testing.
	binFS, binHashes, err := ExtractOrReadBinFS(cacheDir, options.SiteFS)
	if err != nil {
		return nil, xerrors.Errorf("extract or read bin filesystem: %w", err)
	}
	h := &binHandler{
		metadataCache: newBinMetadataCache(binFS, binHashes),
	}
	if compressedCacheDir != "" {
		cmp := cachecompress.NewCompressor(options.Logger, CompressionLevel, compressedCacheDir, binFS)
		for encoding, fn := range StandardEncoders {
			cmp.SetEncoder(encoding, fn)
		}
		h.handler = cmp
	} else {
		h.handler = http.FileServer(binFS)
	}
	return h, nil
}

// ExtractOrReadBinFS checks the provided fs for compressed coder binaries and
// extracts them into dest/bin if found. As a fallback, the provided FS is
// checked for a /bin directory, if it is non-empty it is returned. Finally
// dest/bin is returned as a fallback allowing binaries to be manually placed in
// dest (usually ${CODER_CACHE_DIRECTORY}/site/orig/bin).
//
// Returns a http.FileSystem that serves unpacked binaries, and a map of binary
// name to SHA1 hash. The returned hash map may be incomplete or contain hashes
// for missing files.
func ExtractOrReadBinFS(dest string, siteFS fs.FS) (http.FileSystem, map[string]string, error) {
	if dest == "" {
		// No destination on fs, embedded fs is the only option.
		binFS, err := fs.Sub(siteFS, "bin")
		if err != nil {
			return nil, nil, xerrors.Errorf("cache path is empty and embedded fs does not have /bin: %w", err)
		}
		return http.FS(binFS), nil, nil
	}

	dest = filepath.Join(dest, "bin")
	mkdest := func() (http.FileSystem, error) {
		err := os.MkdirAll(dest, 0o700)
		if err != nil {
			return nil, xerrors.Errorf("mkdir failed: %w", err)
		}
		return http.Dir(dest), nil
	}

	archive, err := siteFS.Open("bin/coder.tar.zst")
	if err != nil {
		if xerrors.Is(err, fs.ErrNotExist) {
			files, err := fs.ReadDir(siteFS, "bin")
			if err != nil {
				if xerrors.Is(err, fs.ErrNotExist) {
					// Given fs does not have a bin directory, serve from cache
					// directory without extracting anything.
					binFS, err := mkdest()
					if err != nil {
						return nil, nil, xerrors.Errorf("mkdest failed: %w", err)
					}
					return binFS, map[string]string{}, nil
				}
				return nil, nil, xerrors.Errorf("site fs read dir failed: %w", err)
			}

			if len(filterFiles(files, "GITKEEP")) > 0 {
				// If there are other files than bin/GITKEEP, serve the files.
				binFS, err := fs.Sub(siteFS, "bin")
				if err != nil {
					return nil, nil, xerrors.Errorf("site fs sub dir failed: %w", err)
				}
				return http.FS(binFS), nil, nil
			}

			// Nothing we can do, serve the cache directory, thus allowing
			// binaries to be placed there.
			binFS, err := mkdest()
			if err != nil {
				return nil, nil, xerrors.Errorf("mkdest failed: %w", err)
			}
			return binFS, map[string]string{}, nil
		}
		return nil, nil, xerrors.Errorf("open coder binary archive failed: %w", err)
	}
	defer archive.Close()

	binFS, err := mkdest()
	if err != nil {
		return nil, nil, err
	}

	shaFiles, err := parseSHA1(siteFS)
	if err != nil {
		return nil, nil, xerrors.Errorf("parse sha1 file failed: %w", err)
	}

	ok, err := verifyBinSha1IsCurrent(dest, siteFS, shaFiles)
	if err != nil {
		return nil, nil, xerrors.Errorf("verify coder binaries sha1 failed: %w", err)
	}
	if !ok {
		n, err := extractBin(dest, archive)
		if err != nil {
			return nil, nil, xerrors.Errorf("extract coder binaries failed: %w", err)
		}
		if n == 0 {
			return nil, nil, xerrors.New("no files were extracted from coder binaries archive")
		}
	}

	return binFS, shaFiles, nil
}

func extractBin(dest string, r io.Reader) (numExtracted int, err error) {
	opts := []zstd.DOption{
		// Concurrency doesn't help us when decoding the tar and
		// can actually slow us down.
		zstd.WithDecoderConcurrency(1),
		// Ignoring checksums can give a slight performance
		// boost but it's probably not worth the reduced safety.
		zstd.IgnoreChecksum(false),
		// Allow the decoder to use more memory giving us a 2-3x
		// performance boost.
		zstd.WithDecoderLowmem(false),
	}
	zr, err := zstd.NewReader(r, opts...)
	if err != nil {
		return 0, xerrors.Errorf("open zstd archive failed: %w", err)
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	n := 0
	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return n, nil
			}
			return n, xerrors.Errorf("read tar archive failed: %w", err)
		}
		if h.Name == "." || strings.Contains(h.Name, "..") {
			continue
		}

		name := filepath.Join(dest, filepath.Base(h.Name))
		f, err := os.Create(name)
		if err != nil {
			return n, xerrors.Errorf("create file failed: %w", err)
		}
		//#nosec // We created this tar, no risk of decompression bomb.
		_, err = io.Copy(f, tr)
		if err != nil {
			_ = f.Close()
			return n, xerrors.Errorf("write file contents failed: %w", err)
		}
		err = f.Close()
		if err != nil {
			return n, xerrors.Errorf("close file failed: %w", err)
		}

		n++
	}
}

type binMetadata struct {
	sizeBytes int64 // -1 if not known yet
	// SHA1 was chosen because it's fast to compute and reasonable for
	// determining if a file has changed. The ETag is not used a security
	// measure.
	sha1Hash string // always set if in the cache
}

type binMetadataCache struct {
	binFS          http.FileSystem
	originalHashes map[string]string

	metadata map[string]binMetadata
	mut      sync.RWMutex
	sf       singleflight.Group
	sem      chan struct{}
}

func newBinMetadataCache(binFS http.FileSystem, binSha1Hashes map[string]string) *binMetadataCache {
	b := &binMetadataCache{
		binFS:          binFS,
		originalHashes: make(map[string]string, len(binSha1Hashes)),

		metadata: make(map[string]binMetadata, len(binSha1Hashes)),
		mut:      sync.RWMutex{},
		sf:       singleflight.Group{},
		sem:      make(chan struct{}, 4),
	}

	// Previously we copied binSha1Hashes to the cache immediately. Since we now
	// read other information like size from the file, we can't do that. Instead
	// we copy the hashes to a different map that will be used to populate the
	// cache on the first request.
	for k, v := range binSha1Hashes {
		b.originalHashes[k] = v
	}

	return b
}

func (b *binMetadataCache) getMetadata(name string) (binMetadata, error) {
	b.mut.RLock()
	metadata, ok := b.metadata[name]
	b.mut.RUnlock()
	if ok {
		return metadata, nil
	}

	// Avoid DOS by using a pool, and only doing work once per file.
	v, err, _ := b.sf.Do(name, func() (any, error) {
		b.sem <- struct{}{}
		defer func() { <-b.sem }()

		// Reject any invalid or non-basename paths before touching the filesystem.
		if name == "" ||
			name == "." ||
			strings.Contains(name, "/") ||
			strings.Contains(name, "\\") ||
			!fs.ValidPath(name) ||
			path.Base(name) != name {
			return binMetadata{}, os.ErrNotExist
		}

		f, err := b.binFS.Open(name)
		if err != nil {
			return binMetadata{}, err
		}
		defer f.Close()

		var metadata binMetadata

		stat, err := f.Stat()
		if err != nil {
			return binMetadata{}, err
		}
		metadata.sizeBytes = stat.Size()

		if hash, ok := b.originalHashes[name]; ok {
			metadata.sha1Hash = hash
		} else {
			h := sha1.New() //#nosec // Not used for cryptography.
			_, err := io.Copy(h, f)
			if err != nil {
				return binMetadata{}, err
			}
			metadata.sha1Hash = hex.EncodeToString(h.Sum(nil))
		}

		b.mut.Lock()
		b.metadata[name] = metadata
		b.mut.Unlock()
		return metadata, nil
	})
	if err != nil {
		return binMetadata{}, err
	}

	//nolint:forcetypeassert
	return v.(binMetadata), nil
}

func filterFiles(files []fs.DirEntry, names ...string) []fs.DirEntry {
	var filtered []fs.DirEntry
	for _, f := range files {
		if slices.Contains(names, f.Name()) {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}

func verifyBinSha1IsCurrent(dest string, siteFS fs.FS, shaFiles map[string]string) (ok bool, err error) {
	b1, err := fs.ReadFile(siteFS, "bin/coder.sha1")
	if err != nil {
		return false, xerrors.Errorf("read coder sha1 from embedded fs failed: %w", err)
	}
	b2, err := os.ReadFile(filepath.Join(dest, "coder.sha1"))
	if err != nil {
		if xerrors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, xerrors.Errorf("read coder sha1 failed: %w", err)
	}

	// Check shasum files for equality for early-exit.
	if !bytes.Equal(b1, b2) {
		return false, nil
	}

	var eg errgroup.Group
	// Speed up startup by verifying files concurrently. Concurrency
	// is limited to save resources / early-exit. Early-exit speed
	// could be improved by using a context aware io.Reader and
	// passing the context from errgroup.WithContext.
	eg.SetLimit(3)

	// Verify the hash of each on-disk binary.
	for file, hash1 := range shaFiles {
		eg.Go(func() error {
			hash2, err := sha1HashFile(filepath.Join(dest, file))
			if err != nil {
				if xerrors.Is(err, fs.ErrNotExist) {
					return errHashMismatch
				}
				return xerrors.Errorf("hash file failed: %w", err)
			}
			if !strings.EqualFold(hash1, hash2) {
				return errHashMismatch
			}
			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		if xerrors.Is(err, errHashMismatch) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// sha1HashFile computes a SHA1 hash of the file, returning the hex
// representation.
func sha1HashFile(name string) (string, error) {
	//#nosec // Not used for cryptography.
	hash := sha1.New()
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = io.Copy(hash, f)
	if err != nil {
		return "", err
	}

	b := make([]byte, hash.Size())
	hash.Sum(b[:0])

	return hex.EncodeToString(b), nil
}

func parseSHA1(siteFS fs.FS) (map[string]string, error) {
	b, err := fs.ReadFile(siteFS, "bin/coder.sha1")
	if err != nil {
		return nil, xerrors.Errorf("read coder sha1 from embedded fs failed: %w", err)
	}

	shaFiles := make(map[string]string)
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		parts := bytes.Split(line, []byte{' ', '*'})
		if len(parts) != 2 {
			return nil, xerrors.Errorf("malformed sha1 file: %w", err)
		}
		shaFiles[string(parts[1])] = strings.ToLower(string(parts[0]))
	}
	if len(shaFiles) == 0 {
		return nil, xerrors.Errorf("empty sha1 file: %w", err)
	}

	return shaFiles, nil
}

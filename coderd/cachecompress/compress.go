// Package cachecompress creates a compressed cache of static files based on an http.FS. It is modified from
// https://github.com/go-chi/chi Compressor middleware. See the LICENSE file in this directory for copyright
// information.
package cachecompress

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

type cacheKey struct {
	encoding string
	urlPath  string
}

func (c cacheKey) filePath(cacheDir string) string {
	// URLs can have slashes or other characters we don't want the file system interpreting. So we just encode the path
	// to a flat base64 filename.
	filename := base64.URLEncoding.EncodeToString([]byte(c.urlPath))
	return filepath.Join(cacheDir, c.encoding, filename)
}

func getCacheKey(encoding string, r *http.Request) cacheKey {
	return cacheKey{
		encoding: encoding,
		urlPath:  r.URL.Path,
	}
}

type compressorMetrics struct {
	// requestsTotal is the total number of requests to getRef.
	requestsTotal *prometheus.CounterVec
	// compressionsTotal is the total number of actual compression operations started.
	compressionsTotal *prometheus.CounterVec
}

func newCompressorMetrics(reg prometheus.Registerer) compressorMetrics {
	f := promauto.With(reg)
	return compressorMetrics{
		requestsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "cachecompress",
			Name:      "requests_total",
			Help:      "Total number of requests to get a compressed file reference.",
		}, []string{"encoding", "hit"}),
		compressionsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coder",
			Subsystem: "cachecompress",
			Name:      "compressions_total",
			Help:      "Total number of compression operations started.",
		}, []string{"encoding"}),
	}
}

// Compressor represents a set of encoding configurations.
type Compressor struct {
	logger slog.Logger
	// The mapping of encoder names to encoder functions.
	encoders map[string]EncoderFunc
	// The mapping of pooled encoders to pools.
	pooledEncoders map[string]*sync.Pool
	// The list of encoders in order of decreasing precedence.
	encodingPrecedence []string
	level              int // The compression level.
	cacheDir           string
	orig               http.FileSystem

	// sfGroup deduplicates concurrent compression requests for the same file.
	sfGroup singleflight.Group
	// cache stores successfully compressed file paths. Once a file is compressed,
	// subsequent requests can serve directly from the cache without going through
	// singleflight.
	cacheMu sync.RWMutex
	cache   map[cacheKey]string // cacheKey -> cachePath
	metrics compressorMetrics
}

// NewCompressor creates a new Compressor that will handle encoding responses.
//
// The level should be one of the ones defined in the flate package.
// The types are the content types that are allowed to be compressed.
func NewCompressor(logger slog.Logger, reg prometheus.Registerer, level int, cacheDir string, orig http.FileSystem) *Compressor {
	c := &Compressor{
		logger:         logger.Named("cachecompress"),
		level:          level,
		encoders:       make(map[string]EncoderFunc),
		pooledEncoders: make(map[string]*sync.Pool),
		cacheDir:       cacheDir,
		orig:           orig,
		cache:          make(map[cacheKey]string),
		metrics:        newCompressorMetrics(reg),
	}

	// Set the default encoders.  The precedence order uses the reverse
	// ordering that the encoders were added. This means adding new encoders
	// will move them to the front of the order.
	//
	// TODO:
	// lzma: Opera.
	// sdch: Chrome, Android. Gzip output + dictionary header.
	// br:   Brotli, see https://github.com/go-chi/chi/pull/326

	// HTTP 1.1 "deflate" (RFC 2616) stands for DEFLATE data (RFC 1951)
	// wrapped with zlib (RFC 1950). The zlib wrapper uses Adler-32
	// checksum compared to CRC-32 used in "gzip" and thus is faster.
	//
	// But.. some old browsers (MSIE, Safari 5.1) incorrectly expect
	// raw DEFLATE data only, without the mentioned zlib wrapper.
	// Because of this major confusion, most modern browsers try it
	// both ways, first looking for zlib headers.
	// Quote by Mark Adler: http://stackoverflow.com/a/9186091/385548
	//
	// The list of browsers having problems is quite big, see:
	// http://zoompf.com/blog/2012/02/lose-the-wait-http-compression
	// https://web.archive.org/web/20120321182910/http://www.vervestudios.co/projects/compression-tests/results
	//
	// That's why we prefer gzip over deflate. It's just more reliable
	// and not significantly slower than deflate.
	c.SetEncoder("deflate", encoderDeflate)

	// TODO: Exception for old MSIE browsers that can't handle non-HTML?
	// https://zoompf.com/blog/2012/02/lose-the-wait-http-compression
	c.SetEncoder("gzip", encoderGzip)

	// NOTE: Not implemented, intentionally:
	// case "compress": // LZW. Deprecated.
	// case "bzip2":    // Too slow on-the-fly.
	// case "zopfli":   // Too slow on-the-fly.
	// case "xz":       // Too slow on-the-fly.
	return c
}

// SetEncoder can be used to set the implementation of a compression algorithm.
//
// The encoding should be a standardized identifier. See:
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Accept-Encoding
//
// For example, add the Brotli algorithm:
//
//	import brotli_enc "gopkg.in/kothar/brotli-go.v0/enc"
//
//	compressor := middleware.NewCompressor(5, "text/html")
//	compressor.SetEncoder("br", func(w io.Writer, level int) io.Writer {
//		params := brotli_enc.NewBrotliParams()
//		params.SetQuality(level)
//		return brotli_enc.NewBrotliWriter(params, w)
//	})
func (c *Compressor) SetEncoder(encoding string, fn EncoderFunc) {
	encoding = strings.ToLower(encoding)
	if encoding == "" {
		panic("the encoding can not be empty")
	}
	if fn == nil {
		panic("attempted to set a nil encoder function")
	}

	// If we are adding a new encoder that is already registered, we have to
	// clear that one out first.
	delete(c.pooledEncoders, encoding)
	delete(c.encoders, encoding)

	// If the encoder supports Resetting (IoReseterWriter), then it can be pooled.
	encoder := fn(io.Discard, c.level)
	if _, ok := encoder.(ioResetterWriter); ok {
		pool := &sync.Pool{
			New: func() interface{} {
				return fn(io.Discard, c.level)
			},
		}
		c.pooledEncoders[encoding] = pool
	}
	// If the encoder is not in the pooledEncoders, add it to the normal encoders.
	if _, ok := c.pooledEncoders[encoding]; !ok {
		c.encoders[encoding] = fn
	}

	for i, v := range c.encodingPrecedence {
		if v == encoding {
			c.encodingPrecedence = append(c.encodingPrecedence[:i], c.encodingPrecedence[i+1:]...)
		}
	}

	c.encodingPrecedence = append([]string{encoding}, c.encodingPrecedence...)
}

// ServeHTTP returns the response from the orig file system, compressed if possible.
func (c *Compressor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	encoding := c.selectEncoder(r.Header)

	// we can only serve a cached response if all the following:
	// 1. they requested an encoding we support
	// 2. they are requesting the whole file, not a range
	// 3. the method is GET
	if encoding == "" || r.Header.Get("Range") != "" || r.Method != "GET" {
		http.FileServer(c.orig).ServeHTTP(w, r)
		return
	}

	// Whether we should serve a cached response also depends in a fairly complex way on the path and request
	// headers. In particular, we don't need a cached response for non-existing files/directories, and should not serve
	// a cached response if the correct Etag for the file is provided. This logic is all handled by the http.FileServer,
	// and we don't want to reimplement it here. So, what we'll do is send a HEAD request to the http.FileServer to see
	// what it would do.
	headReq := r.Clone(r.Context())
	headReq.Method = http.MethodHead
	headRW := &compressResponseWriter{
		w:       io.Discard,
		headers: make(http.Header),
	}
	// deep-copy the headers already set on the response. This includes things like ETags.
	for key, values := range w.Header() {
		for _, value := range values {
			headRW.headers.Add(key, value)
		}
	}
	http.FileServer(c.orig).ServeHTTP(headRW, headReq)
	if headRW.code != http.StatusOK {
		// again, fall back to the file server. This is often a 404 Not Found, or a 304 Not Modified if they provided
		// the correct ETag.
		http.FileServer(c.orig).ServeHTTP(w, r)
		return
	}

	cref, cachePath := c.getRef(encoding, r)
	c.serveRef(w, r, headRW.headers, cref, cachePath)
}

func (c *Compressor) serveRef(w http.ResponseWriter, r *http.Request, headers http.Header, ck cacheKey, cachePath string) {
	if cachePath == "" {
		// Compression failed, fall back to uncompressed.
		http.FileServer(c.orig).ServeHTTP(w, r)
		return
	}

	cacheFile, err := os.Open(cachePath)
	if err != nil {
		c.logger.Error(r.Context(), "failed to open compressed cache file",
			slog.F("cache_path", cachePath), slog.F("url_path", r.URL.Path), slog.Error(err))
		// Fall back to uncompressed.
		http.FileServer(c.orig).ServeHTTP(w, r)
		return
	}
	defer cacheFile.Close()

	// We need to remove or modify the Content-Length, if any, set by the FileServer because it will be for
	// uncompressed data and wrong.
	info, err := cacheFile.Stat()
	if err != nil {
		c.logger.Error(r.Context(), "failed to stat compressed cache file",
			slog.F("cache_path", cachePath), slog.F("url_path", r.URL.Path), slog.Error(err))
		headers.Del("Content-Length")
	} else {
		headers.Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	}

	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Content-Encoding", ck.encoding)
	w.Header().Add("Vary", "Accept-Encoding")
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, cacheFile)
	if err != nil {
		// Most commonly, the writer will hang up before we are done.
		c.logger.Debug(r.Context(), "failed to write compressed cache file", slog.Error(err))
	}
}

// getRef returns the cache key and path to the compressed cache file for the given encoding and request.
// If compression fails, it returns an empty cachePath.
func (c *Compressor) getRef(encoding string, r *http.Request) (cacheKey, string) {
	ck := getCacheKey(encoding, r)
	sfKey := ck.encoding + ":" + ck.urlPath

	// Fast path: check if already cached.
	c.cacheMu.RLock()
	if cachePath, ok := c.cache[ck]; ok {
		c.cacheMu.RUnlock()
		c.metrics.requestsTotal.WithLabelValues(encoding, "true").Inc()
		return ck, cachePath
	}
	c.cacheMu.RUnlock()

	// Slow path: use singleflight to deduplicate concurrent compression requests.
	// Any request going through this path is a cache "miss", regardless of whether
	// it does the compression or waits for another goroutine to finish.
	c.metrics.requestsTotal.WithLabelValues(encoding, "false").Inc()

	result, err, _ := c.sfGroup.Do(sfKey, func() (interface{}, error) {
		// Double-check cache in case another goroutine just finished.
		c.cacheMu.RLock()
		if cachePath, ok := c.cache[ck]; ok {
			c.cacheMu.RUnlock()
			return cachePath, nil
		}
		c.cacheMu.RUnlock()

		// We are the one doing the compression.
		c.metrics.compressionsTotal.WithLabelValues(encoding).Inc()
		cachePath, err := c.compress(r.Context(), encoding, ck, r)
		if err != nil {
			return "", err
		}

		// Store in cache for future requests.
		c.cacheMu.Lock()
		c.cache[ck] = cachePath
		c.cacheMu.Unlock()

		return cachePath, nil
	})

	if err != nil {
		// Compression failed, return empty path to trigger fallback to uncompressed.
		return ck, ""
	}

	cachePath, _ := result.(string)
	return ck, cachePath
}

func (c *Compressor) compress(ctx context.Context, encoding string, ck cacheKey, r *http.Request) (string, error) {
	cachePath := ck.filePath(c.cacheDir)

	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		c.logger.Error(ctx, "failed to create cache directory", slog.F("cache_dir", cacheDir), slog.Error(err))
		return "", err
	}

	// We will truncate and overwrite any existing files. This is important in the case that we get restarted
	// with the same cache dir, possibly with different source files.
	cacheFile, err := os.OpenFile(cachePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		c.logger.Error(ctx, "failed to open compression cache file",
			slog.F("path", cachePath), slog.Error(err))
		return "", err
	}
	defer cacheFile.Close()

	encoder, cleanup := c.getEncoder(encoding, cacheFile)
	if encoder == nil {
		// Can only hit this if there is a programming error.
		c.logger.Critical(ctx, "got nil encoder", slog.F("encoding", encoding))
		return "", xerrors.New("nil encoder")
	}
	defer cleanup()
	defer encoder.Close() // Ensures we flush, needs to be called before cleanup(), so we defer after it.

	cw := &compressResponseWriter{
		w:       encoder,
		headers: make(http.Header), // ignored
	}
	http.FileServer(c.orig).ServeHTTP(cw, r)
	if cw.code != http.StatusOK {
		// Log at debug because this is likely just a 404.
		c.logger.Debug(ctx, "file server failed to serve",
			slog.F("encoding", encoding), slog.F("url_path", ck.urlPath), slog.F("http_code", cw.code))
		// Clean up the partial file.
		if rErr := os.Remove(cachePath); rErr != nil {
			c.logger.Debug(ctx, "failed to remove cache file", slog.F("remove_err", rErr), slog.F("cache_path", cachePath))
		}
		return "", xerrors.New("file server failed to serve")
	}

	return cachePath, nil
}

// selectEncoder returns the name of the encoder
func (c *Compressor) selectEncoder(h http.Header) string {
	header := h.Get("Accept-Encoding")

	// Parse the names of all accepted algorithms from the header.
	accepted := strings.Split(strings.ToLower(header), ",")

	// Find supported encoder by accepted list by precedence
	for _, name := range c.encodingPrecedence {
		if matchAcceptEncoding(accepted, name) {
			return name
		}
	}

	// No encoder found to match the accepted encoding
	return ""
}

// getEncoder returns a writer that encodes and writes to the provided writer, and a cleanup func.
func (c *Compressor) getEncoder(name string, w io.Writer) (io.WriteCloser, func()) {
	if pool, ok := c.pooledEncoders[name]; ok {
		encoder, typeOK := pool.Get().(ioResetterWriter)
		if !typeOK {
			return nil, nil
		}
		cleanup := func() {
			pool.Put(encoder)
		}
		encoder.Reset(w)
		return encoder, cleanup
	}
	if fn, ok := c.encoders[name]; ok {
		return fn(w, c.level), func() {}
	}
	return nil, nil
}

func matchAcceptEncoding(accepted []string, encoding string) bool {
	for _, v := range accepted {
		if strings.Contains(v, encoding) {
			return true
		}
	}
	return false
}

// An EncoderFunc is a function that wraps the provided io.Writer with a
// streaming compression algorithm and returns it.
//
// In case of failure, the function should return nil.
type EncoderFunc func(w io.Writer, level int) io.WriteCloser

// Interface for types that allow resetting io.Writers.
type ioResetterWriter interface {
	io.WriteCloser
	Reset(w io.Writer)
}

func encoderGzip(w io.Writer, level int) io.WriteCloser {
	gw, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return nil
	}
	return gw
}

func encoderDeflate(w io.Writer, level int) io.WriteCloser {
	dw, err := flate.NewWriter(w, level)
	if err != nil {
		return nil
	}
	return dw
}

type compressResponseWriter struct {
	w       io.Writer
	headers http.Header
	code    int
}

func (cw *compressResponseWriter) Header() http.Header {
	return cw.headers
}

func (cw *compressResponseWriter) WriteHeader(code int) {
	cw.code = code
}

func (cw *compressResponseWriter) Write(p []byte) (int, error) {
	if cw.code == 0 {
		cw.code = http.StatusOK
	}
	return cw.w.Write(p)
}

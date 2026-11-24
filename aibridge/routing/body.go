package routing

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/xerrors"
)

// ErrUnsupportedContentEncoding is returned when a request body uses a
// Content-Encoding that AI Gateway cannot decode.
var ErrUnsupportedContentEncoding = xerrors.New("unsupported content encoding")

// DecodeRequestBody replaces r.Body with its decoded form when the request
// has a Content-Encoding, and removes the Content-Encoding header.
func DecodeRequestBody(r *http.Request) error {
	// Stacked encodings are rejected: repeated header lines here, and
	// comma-separated values by the default branch of the switch below.
	if len(r.Header.Values("Content-Encoding")) > 1 {
		return ErrUnsupportedContentEncoding
	}
	enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return nil
	}

	var dec io.Reader
	var closeDecoder func()
	switch enc {
	case "zstd":
		zstdDec, err := zstd.NewReader(r.Body,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxMemory(uint64(MaxRequestBodyBytes)),
		)
		if err != nil {
			return xerrors.Errorf("decode %s request body: %w", enc, err)
		}
		dec = zstdDec
		closeDecoder = zstdDec.Close
	case "gzip":
		gzipDec, err := gzip.NewReader(r.Body)
		if err != nil {
			return xerrors.Errorf("decode %s request body: %w", enc, err)
		}
		dec = gzipDec
		closeDecoder = func() { _ = gzipDec.Close() }
	case "br":
		dec = brotli.NewReader(r.Body)
	case "deflate":
		zlibDec, err := zlib.NewReader(r.Body)
		if err != nil {
			return xerrors.Errorf("decode %s request body: %w", enc, err)
		}
		dec = zlibDec
		closeDecoder = func() { _ = zlibDec.Close() }
	default:
		return ErrUnsupportedContentEncoding
	}
	if closeDecoder != nil {
		defer closeDecoder()
	}

	decoded, err := io.ReadAll(io.LimitReader(dec, MaxRequestBodyBytes+1))
	if err != nil {
		return xerrors.Errorf("decode %s request body: %w", enc, err)
	}
	if int64(len(decoded)) > MaxRequestBodyBytes {
		return &http.MaxBytesError{Limit: MaxRequestBodyBytes}
	}

	r.Body = io.NopCloser(bytes.NewReader(decoded))
	r.ContentLength = int64(len(decoded))
	r.Header.Del("Content-Encoding")
	r.Header.Del("Content-Length")
	return nil
}

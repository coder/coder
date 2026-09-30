package extract

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"golang.org/x/xerrors"
)

// DecodeBody returns a complete response body as it was sent on the wire,
// decoded for ResponseExtraction.ProcessBlocking. It decodes according to
// contentEncoding (identity or gzip) and allows at most MaxBodyBytes after
// decoding.
//
// A returned error is a parse note, and the body is nil. Callers should
// still call ProcessBlocking with the nil body so an error status is
// classified.
func DecodeBody(contentEncoding string, raw []byte) ([]byte, error) {
	var r io.Reader
	switch enc := strings.ToLower(strings.TrimSpace(contentEncoding)); enc {
	case "", "identity":
		if len(raw) > MaxBodyBytes {
			return nil, xerrors.Errorf("body exceeds %d bytes", MaxBodyBytes)
		}
		return raw, nil
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, xerrors.Errorf("invalid gzip body: %w", err)
		}
		defer zr.Close()
		r = zr
	default:
		return nil, xerrors.Errorf("unsupported content encoding %q", enc)
	}

	decoded, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if err != nil {
		return nil, xerrors.Errorf("decode body: %w", err)
	}
	if len(decoded) > MaxBodyBytes {
		return nil, xerrors.Errorf("decoded body exceeds %d bytes", MaxBodyBytes)
	}
	return decoded, nil
}

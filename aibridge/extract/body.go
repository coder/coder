package extract

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// FromBody feeds a complete response body to ext and returns the
// resulting facts, including the adapter's own parse notes. Use it for
// non-streaming responses and for error responses to any request.
//
// The body is decoded according to contentEncoding (identity or gzip) and
// read up to MaxBodyBytes after decoding. Bodies that cannot be decoded, or
// that exceed the limit, are skipped with a note; for error statuses ext
// still observes the status code with a nil body so the error is
// classified. body should already be buffered: FromBody reads it fully.
func FromBody(ext ResponseExtraction, statusCode int, contentEncoding string, body io.Reader) ResponseFacts {
	if ext == nil {
		panic("extract: FromBody requires a ResponseExtraction")
	}
	var notes ParseNotes
	raw, ok := readBody(&notes, contentEncoding, body)
	switch {
	case ok:
		ext.OnBody(statusCode, raw)
	case statusCode >= http.StatusBadRequest:
		ext.OnBody(statusCode, nil)
	}
	facts := ext.Result()
	facts.ParseNotes = append(facts.ParseNotes, notes.List()...)
	return facts
}

func readBody(notes *ParseNotes, contentEncoding string, body io.Reader) ([]byte, bool) {
	if body == nil {
		return nil, true
	}
	r := body
	switch enc := strings.ToLower(strings.TrimSpace(contentEncoding)); enc {
	case "", "identity":
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(body)
		if err != nil {
			notes.Addf("skipped body: invalid gzip: %v", err)
			return nil, false
		}
		defer zr.Close()
		r = zr
	default:
		notes.Addf("skipped body: unsupported content encoding %q", enc)
		return nil, false
	}

	raw, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if err != nil {
		notes.Addf("skipped body: read failed: %v", err)
		return nil, false
	}
	if len(raw) > MaxBodyBytes {
		notes.Addf("skipped body: exceeds %d bytes", MaxBodyBytes)
		return nil, false
	}
	return raw, true
}

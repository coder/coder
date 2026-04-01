package chatdebug

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// attemptStatusCompleted is the status recorded when a response body
// is fully read without transport-level errors.
const attemptStatusCompleted = "completed"

// attemptStatusFailed is the status recorded when a transport error
// or body read error occurs.
const attemptStatusFailed = "failed"

// maxRecordedRequestBodyBytes caps in-memory request capture when GetBody
// is available.
const maxRecordedRequestBodyBytes = 50_000

// maxRecordedResponseBodyBytes caps in-memory response capture.
const maxRecordedResponseBodyBytes = 50_000

// RecordingTransport captures HTTP request/response data for debug steps.
// When the request context carries an attemptSink, it records each round
// trip. Otherwise it delegates directly.
type RecordingTransport struct {
	// Base is the underlying transport. nil defaults to http.DefaultTransport.
	Base http.RoundTripper
}

var _ http.RoundTripper = (*RecordingTransport)(nil)

func (t *RecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		panic("chatdebug: nil request")
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	sink := attemptSinkFromContext(req.Context())
	if sink == nil {
		return base.RoundTrip(req)
	}

	requestHeaders := RedactHeaders(req.Header)

	// Capture method and URL/path from the request.
	method := req.Method
	reqURL := ""
	reqPath := ""
	if req.URL != nil {
		reqURL = redactURL(req.URL)
		reqPath = req.URL.Path
	}

	requestBody, err := captureRequestBody(req)
	if err != nil {
		return nil, err
	}
	attemptNumber := sink.nextAttemptNumber()

	startedAt := time.Now()
	resp, err := base.RoundTrip(req)
	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	if err != nil {
		sink.record(Attempt{
			Number:         attemptNumber,
			Status:         attemptStatusFailed,
			Method:         method,
			URL:            reqURL,
			Path:           reqPath,
			StartedAt:      startedAt.UTC().Format(time.RFC3339Nano),
			FinishedAt:     finishedAt.UTC().Format(time.RFC3339Nano),
			RequestHeaders: requestHeaders,
			RequestBody:    requestBody,
			Error:          err.Error(),
			DurationMs:     durationMs,
		})
		return nil, err
	}

	respHeaders := RedactHeaders(resp.Header)
	resp.Body = &recordingBody{
		inner:         resp.Body,
		sink:          sink,
		startedAt:     startedAt,
		contentLength: resp.ContentLength,
		base: Attempt{
			Number:          attemptNumber,
			Method:          method,
			URL:             reqURL,
			Path:            reqPath,
			RequestHeaders:  requestHeaders,
			RequestBody:     requestBody,
			ResponseStatus:  resp.StatusCode,
			ResponseHeaders: respHeaders,
			DurationMs:      durationMs,
		},
	}

	return resp, nil
}

func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	clone.User = nil
	q := clone.Query()
	for key, values := range q {
		if isSensitiveHeaderName(key) || isSensitiveJSONKey(key) {
			for i := range values {
				values[i] = RedactedValue
			}
			q[key] = values
		}
	}
	clone.RawQuery = q.Encode()
	return clone.String()
}

func captureRequestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}

	if req.GetBody != nil {
		clone, err := req.GetBody()
		if err == nil {
			defer clone.Close()
			limited, err := io.ReadAll(io.LimitReader(clone, maxRecordedRequestBodyBytes+1))
			if err == nil {
				if len(limited) > maxRecordedRequestBodyBytes {
					return []byte("[TRUNCATED]"), nil
				}
				return RedactJSONSecrets(limited), nil
			}
		}
	}

	// Without GetBody we cannot safely capture the request body without
	// fully consuming a potentially large or streaming body before the
	// request is sent. Skip capture in that case to keep debug logging
	// lightweight and non-invasive.
	return nil, nil
}

type recordingBody struct {
	inner         io.ReadCloser
	buf           bytes.Buffer
	truncated     bool
	sawEOF        bool
	bytesRead     int64
	contentLength int64
	sink          *attemptSink
	base          Attempt
	startedAt     time.Time
	recordOnce    sync.Once
	closeOnce     sync.Once
}

func (r *recordingBody) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	r.bytesRead += int64(n)
	if n > 0 && !r.truncated {
		remaining := maxRecordedResponseBodyBytes - r.buf.Len()
		if remaining > 0 {
			toWrite := n
			if toWrite > remaining {
				toWrite = remaining
				r.truncated = true
			}
			_, _ = r.buf.Write(p[:toWrite])
		} else {
			r.truncated = true
		}
	}
	if errors.Is(err, io.EOF) {
		r.sawEOF = true
	}
	if err != nil {
		r.record(err)
	}
	return n, err
}

func (r *recordingBody) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		closeErr = r.inner.Close()
	})
	if closeErr != nil {
		r.record(closeErr)
		return closeErr
	}

	switch {
	case r.sawEOF:
		r.record(io.EOF)
	case r.contentLength >= 0 && r.bytesRead >= r.contentLength:
		r.record(nil)
	case r.contentLength < 0:
		r.record(nil)
	default:
		r.record(io.ErrUnexpectedEOF)
	}
	return nil
}

func (r *recordingBody) record(err error) {
	r.recordOnce.Do(func() {
		finishedAt := time.Now()
		if r.truncated {
			r.base.ResponseBody = []byte("[TRUNCATED]")
		} else {
			r.base.ResponseBody = RedactJSONSecrets(r.buf.Bytes())
		}
		r.base.StartedAt = r.startedAt.UTC().Format(time.RFC3339Nano)
		r.base.FinishedAt = finishedAt.UTC().Format(time.RFC3339Nano)
		// Recompute duration to include body read time.
		r.base.DurationMs = finishedAt.Sub(r.startedAt).Milliseconds()
		if err != nil && !errors.Is(err, io.EOF) {
			r.base.Error = err.Error()
			r.base.Status = attemptStatusFailed
		} else {
			r.base.Status = attemptStatusCompleted
		}
		r.sink.record(r.base)
	})
}

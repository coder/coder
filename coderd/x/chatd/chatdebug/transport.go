package chatdebug

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
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
	contentLength int64
	sink          *attemptSink
	base          Attempt
	startedAt     time.Time

	mu        sync.Mutex
	buf       bytes.Buffer
	truncated bool
	sawEOF    bool
	bytesRead int64

	recordOnce sync.Once
	closeOnce  sync.Once
}

func (r *recordingBody) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)

	r.mu.Lock()
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
	r.mu.Unlock()

	if err != nil {
		r.record(err)
	}
	return n, err
}

func (r *recordingBody) Close() error {
	r.mu.Lock()
	sawEOF := r.sawEOF
	bytesRead := r.bytesRead
	contentLength := r.contentLength
	truncated := r.truncated
	responseBody := append([]byte(nil), r.buf.Bytes()...)
	r.mu.Unlock()

	contentType := r.base.ResponseHeaders["Content-Type"]
	shouldDrainUnknownLengthJSON := contentLength < 0 &&
		!sawEOF &&
		bytesRead > 0 &&
		!truncated &&
		isCompleteUnknownLengthJSONBody(contentType, responseBody)

	var drainErr error
	if shouldDrainUnknownLengthJSON {
		drainErr = r.drainToEOF()
	}

	var closeErr error
	r.closeOnce.Do(func() {
		closeErr = r.inner.Close()
	})
	if closeErr != nil {
		r.record(closeErr)
		return closeErr
	}
	if drainErr != nil {
		r.record(drainErr)
		return nil
	}

	r.mu.Lock()
	sawEOF = r.sawEOF
	bytesRead = r.bytesRead
	contentLength = r.contentLength
	truncated = r.truncated
	responseBody = append([]byte(nil), r.buf.Bytes()...)
	r.mu.Unlock()

	switch {
	case sawEOF && contentLength < 0 && isJSONLikeContentType(contentType) && !isCompleteUnknownLengthJSONBody(contentType, responseBody):
		r.record(io.ErrUnexpectedEOF)
	case sawEOF:
		r.record(io.EOF)
	case responseHasNoBody(r.base.Method, r.base.ResponseStatus):
		r.record(nil)
	case contentLength >= 0 && bytesRead >= contentLength:
		r.record(nil)
	case contentLength < 0 && !truncated && isCompleteUnknownLengthJSONBody(contentType, responseBody):
		r.record(nil)
	default:
		r.record(io.ErrUnexpectedEOF)
	}
	return nil
}

func responseHasNoBody(method string, statusCode int) bool {
	if method == http.MethodHead {
		return true
	}
	return statusCode == http.StatusNoContent ||
		statusCode == http.StatusNotModified ||
		(statusCode >= 100 && statusCode < 200)
}

func isJSONLikeContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func (r *recordingBody) drainToEOF() error {
	buf := make([]byte, 4*1024)
	for {
		n, err := r.inner.Read(buf)

		r.mu.Lock()
		r.bytesRead += int64(n)
		if n > 0 && !r.truncated {
			remaining := maxRecordedResponseBodyBytes - r.buf.Len()
			if remaining > 0 {
				toWrite := n
				if toWrite > remaining {
					toWrite = remaining
					r.truncated = true
				}
				_, _ = r.buf.Write(buf[:toWrite])
			} else {
				r.truncated = true
			}
		}
		if errors.Is(err, io.EOF) {
			r.sawEOF = true
		}
		r.mu.Unlock()

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func isCompleteUnknownLengthJSONBody(contentType string, body []byte) bool {
	if !isJSONLikeContentType(contentType) {
		return false
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func (r *recordingBody) record(err error) {
	r.recordOnce.Do(func() {
		finishedAt := time.Now()

		r.mu.Lock()
		truncated := r.truncated
		responseBody := append([]byte(nil), r.buf.Bytes()...)
		base := r.base
		startedAt := r.startedAt
		r.mu.Unlock()

		if truncated {
			base.ResponseBody = []byte("[TRUNCATED]")
		} else {
			base.ResponseBody = RedactJSONSecrets(responseBody)
		}
		base.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
		base.FinishedAt = finishedAt.UTC().Format(time.RFC3339Nano)
		// Recompute duration to include body read time.
		base.DurationMs = finishedAt.Sub(startedAt).Milliseconds()
		if err != nil && !errors.Is(err, io.EOF) {
			base.Error = err.Error()
			base.Status = attemptStatusFailed
		} else {
			base.Status = attemptStatusCompleted
		}
		r.sink.record(base)
	})
}

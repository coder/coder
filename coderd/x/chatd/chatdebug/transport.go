package chatdebug

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
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
			Error:          sanitizeErrorString(err.Error()),
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
		contentType:   resp.Header.Get("Content-Type"),
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

// urlInErrorPattern matches URL-like substrings that transports or
// retry middleware may embed in error messages. Credentials can
// appear in userinfo or query parameters.
var urlInErrorPattern = regexp.MustCompile(`https?://[^\s"']+`)

// sanitizeErrorString redacts URL-like substrings that may contain
// credentials (userinfo, query parameters) from transport error
// messages before they are persisted in debug attempts.
func sanitizeErrorString(errMsg string) string {
	return urlInErrorPattern.ReplaceAllStringFunc(errMsg, func(rawURL string) string {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return "[REDACTED_URL]"
		}
		return redactURL(parsed)
	})
}

func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	clone.User = nil
	q := clone.Query()
	for key, values := range q {
		if isSensitiveName(key) || isSensitiveJSONKey(key) {
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
			limited, readErr := io.ReadAll(io.LimitReader(clone, maxRecordedRequestBodyBytes+1))
			_ = clone.Close()
			// Some SDKs return the active body from GetBody instead of an
			// independent reader. Restore the request body from GetBody so
			// the upstream transport still receives the original bytes.
			resetErr := resetRequestBody(req)
			if resetErr != nil {
				return nil, xerrors.Errorf("chatdebug: reset request body: %w", resetErr)
			}
			if readErr != nil {
				return nil, nil
			}
			if len(limited) > maxRecordedRequestBodyBytes {
				return []byte("[TRUNCATED]"), nil
			}
			return RedactJSONSecrets(limited), nil
		}
	}

	// Without GetBody we cannot safely capture the request body without
	// fully consuming a potentially large or streaming body before the
	// request is sent. Skip capture in that case to keep debug logging
	// lightweight and non-invasive.
	return nil, nil
}

// resetRequestBody replaces req.Body with a fresh reader from req.GetBody.
// It closes the previous request body before installing the replacement.
// Callers must ensure req.GetBody is non-nil.
func resetRequestBody(req *http.Request) error {
	body, err := req.GetBody()
	if err != nil {
		return err
	}
	if req.Body != nil {
		if err := req.Body.Close(); err != nil {
			_ = body.Close()
			return err
		}
	}
	req.Body = body
	return nil
}

type recordingBody struct {
	inner         io.ReadCloser
	contentLength int64
	contentType   string // from resp.Header.Get (case-insensitive)
	sink          *attemptSink
	base          Attempt
	startedAt     time.Time

	mu        sync.Mutex
	buf       bytes.Buffer
	truncated bool
	sawEOF    bool
	bytesRead int64
	// recordedProvisional is true when recordProvisional() has fired
	// for an SSE body's Read-path EOF but Close() has not yet run. A
	// subsequent inner.Close() error in Close() upgrades the
	// provisional entry in the sink so the close error is not lost.
	recordedProvisional bool

	recordOnce sync.Once
	closeOnce  sync.Once
}

// accumulateReadLocked updates the buffer, byte counters, and
// truncation/EOF flags after a read.  The caller must hold r.mu.
func (r *recordingBody) accumulateReadLocked(data []byte, n int, err error) {
	r.bytesRead += int64(n)
	if n > 0 && !r.truncated {
		remaining := maxRecordedResponseBodyBytes - r.buf.Len()
		if remaining > 0 {
			toWrite := n
			if toWrite > remaining {
				toWrite = remaining
				r.truncated = true
			}
			_, _ = r.buf.Write(data[:toWrite])
		} else {
			r.truncated = true
		}
	}
	if errors.Is(err, io.EOF) {
		r.sawEOF = true
	}
}

func (r *recordingBody) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)

	r.mu.Lock()
	r.accumulateReadLocked(p, n, err)
	r.mu.Unlock()

	// Record non-EOF errors immediately. EOF is handled
	// below for SSE or deferred to Close() for validation.
	if err != nil && !errors.Is(err, io.EOF) {
		r.record(err)
		return n, err
	}

	// For server-sent-events bodies, record eagerly on EOF. Streaming
	// consumers like fantasy's Anthropic SSE adapter iterate the
	// response to EOF and abandon it without calling Close(), so the
	// Close-only recording path would never fire and the attempt would
	// be lost. The recording is provisional so Close() can still
	// upgrade it to failed if inner.Close() surfaces a transport error.
	// Non-SSE bodies stay on the Close-only path so that JSON
	// integrity, content-length validation, and inner-Close errors
	// keep their existing semantics.
	if errors.Is(err, io.EOF) && isSSEContentType(r.contentType) {
		r.recordProvisional(io.EOF)
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

	contentType := r.contentType
	shouldDrainUnknownLengthJSON := contentLength < 0 &&
		!sawEOF &&
		bytesRead > 0 &&
		!truncated &&
		isCompleteUnknownLengthJSONBody(contentType, responseBody)

	// Always close the inner reader first so that stalled chunked
	// bodies cannot block drainToEOF indefinitely.  Once inner is
	// closed, reads return immediately with an error or EOF.
	var closeErr error
	r.closeOnce.Do(func() {
		closeErr = r.inner.Close()
	})
	if closeErr != nil {
		// Hold r.mu across the flag check AND the publish/replace so a
		// concurrent recordProvisional cannot slip its recordOnce
		// publish between our read of recordedProvisional and our call
		// into the sink. Without this serialization, Close() could
		// observe recordedProvisional=false, then lose the race and
		// see r.record(closeErr) become a no-op once recordOnce has
		// already fired from the SSE EOF path.
		r.mu.Lock()
		if r.recordedProvisional {
			// The SSE EOF path already appended a completed attempt.
			// inner.Close() surfaced a transport error, so upgrade
			// that entry to failed instead of losing the close error.
			upgraded := r.buildAttemptLocked(closeErr)
			r.sink.replaceByNumber(upgraded.Number, upgraded)
			r.recordedProvisional = false
		} else {
			r.recordOnce.Do(func() {
				r.sink.record(r.buildAttemptLocked(closeErr))
			})
		}
		r.mu.Unlock()
		return closeErr
	}

	// Drain remaining bytes that may already be buffered inside the
	// HTTP transport after close.  Because inner is closed, this
	// finishes immediately rather than blocking on the network.
	if shouldDrainUnknownLengthJSON {
		// Best-effort drain; ignore errors since inner is closed.
		_ = r.drainToEOF()
	}

	r.mu.Lock()
	sawEOF = r.sawEOF
	bytesRead = r.bytesRead
	contentLength = r.contentLength
	truncated = r.truncated
	responseBody = append([]byte(nil), r.buf.Bytes()...)
	r.mu.Unlock()

	switch {
	// Only check JSON completeness when the recording buffer is
	// not truncated. A truncated buffer is an incomplete prefix
	// of the body, so the completeness check would false-positive.
	case sawEOF && !truncated && contentLength < 0 && isJSONLikeContentType(contentType) && !isCompleteUnknownLengthJSONBody(contentType, responseBody):
		r.record(io.ErrUnexpectedEOF)
	case sawEOF:
		r.record(io.EOF)
	case responseHasNoBody(r.base.Method, r.base.ResponseStatus):
		r.record(nil)
	case contentLength >= 0 && bytesRead >= contentLength:
		r.record(nil)
	case contentLength < 0 && !truncated && isCompleteUnknownLengthJSONBody(contentType, responseBody):
		r.record(nil)
	// Truncated unknown-length bodies: the caller consumed the
	// response successfully but the recording buffer exceeded
	// maxRecordedResponseBodyBytes. This is not a transport
	// failure - mark as completed with the truncated capture.
	case contentLength < 0 && truncated:
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

// parseMediaType extracts the media type from a Content-Type header
// value, falling back to splitting on ";" when mime.ParseMediaType
// fails.
func parseMediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return mediaType
}

func isJSONLikeContentType(contentType string) bool {
	mediaType := parseMediaType(contentType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func isNDJSONContentType(contentType string) bool {
	return parseMediaType(contentType) == "application/x-ndjson"
}

// isSSEContentType reports whether contentType is a
// server-sent-events stream.
func isSSEContentType(contentType string) bool {
	return parseMediaType(contentType) == "text/event-stream"
}

// maxDrainBytes caps how many trailing bytes drainToEOF will consume.
// This prevents Close() from blocking indefinitely on a misbehaving
// or extremely large chunked body.
const maxDrainBytes = 64 * 1024 // 64 KB

func (r *recordingBody) drainToEOF() error {
	buf := make([]byte, 4*1024)
	var drained int64
	for {
		n, err := r.inner.Read(buf)

		r.mu.Lock()
		r.accumulateReadLocked(buf, n, err)
		drained += int64(n)
		r.mu.Unlock()

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		// Safety valve: stop draining after maxDrainBytes to prevent
		// Close() from blocking indefinitely on a chunked body.
		if drained >= maxDrainBytes {
			return io.ErrUnexpectedEOF
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

// buildAttemptLocked materializes the final Attempt from the current
// buffered response data plus err. Callers use this from both the
// record-once append path and the provisional-upgrade replace path so
// both sites apply the same redaction and status rules. The caller
// must hold r.mu for the duration of the call.
func (r *recordingBody) buildAttemptLocked(err error) Attempt {
	finishedAt := time.Now()

	truncated := r.truncated
	responseBody := append([]byte(nil), r.buf.Bytes()...)
	base := r.base
	startedAt := r.startedAt

	contentType := r.contentType
	switch {
	case truncated:
		base.ResponseBody = []byte("[TRUNCATED]")
	case isNDJSONContentType(contentType):
		base.ResponseBody = RedactNDJSONSecrets(responseBody)
	case contentType == "" || isJSONLikeContentType(contentType):
		// Redact JSON secrets when the content type is JSON-like
		// or absent (unknown). For unknown types, RedactJSONSecrets
		// fails closed by replacing non-JSON payloads with a
		// diagnostic message.
		base.ResponseBody = RedactJSONSecrets(responseBody)
	default:
		// Non-JSON content types (SSE, text/plain, HTML, etc.)
		// are preserved as-is to avoid losing debug content.
		base.ResponseBody = responseBody
	}
	base.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	base.FinishedAt = finishedAt.UTC().Format(time.RFC3339Nano)
	// Recompute duration to include body read time.
	base.DurationMs = finishedAt.Sub(startedAt).Milliseconds()
	if err != nil && !errors.Is(err, io.EOF) {
		base.Error = sanitizeErrorString(err.Error())
		base.Status = attemptStatusFailed
	} else {
		base.Status = attemptStatusCompleted
	}
	return base
}

// record acquires r.mu before entering recordOnce.Do so it shares a
// single lock-acquisition order with recordProvisional. Without this,
// a concurrent Read (in recordProvisional, holding r.mu) and Close (in
// record, about to take r.mu inside the Do callback) would deadlock:
// the Do winner would block on r.mu while the loser would block on
// recordOnce. Callers must not hold r.mu.
func (r *recordingBody) record(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordOnce.Do(func() {
		r.sink.record(r.buildAttemptLocked(err))
	})
}

// recordProvisional records err via recordOnce and marks the entry as
// eligible for a later upgrade from Close(). Safe to call multiple
// times; only the first call appends. The publish and the provisional
// flag are committed atomically under r.mu so a concurrent Close()
// that takes r.mu to inspect the flag cannot observe a half-finished
// state where the attempt is in the sink but recordedProvisional is
// still false.
func (r *recordingBody) recordProvisional(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordOnce.Do(func() {
		r.sink.record(r.buildAttemptLocked(err))
		r.recordedProvisional = true
	})
}

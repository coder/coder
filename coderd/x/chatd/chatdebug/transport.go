package chatdebug

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// attemptStatusCompleted is the status recorded when a response body
// is fully read without transport-level errors.
const attemptStatusCompleted = "completed"

// attemptStatusFailed is the status recorded when a transport error
// or body read error occurs.
const attemptStatusFailed = "failed"

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
		reqURL = req.URL.String()
		reqPath = req.URL.Path
	}

	var originalBody []byte
	if req.Body != nil {
		var err error
		originalBody, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(originalBody))
	}
	requestBody := RedactJSONSecrets(originalBody)

	startedAt := time.Now()
	resp, err := base.RoundTrip(req)
	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()
	if err != nil {
		sink.record(Attempt{
			Number:         len(sink.snapshot()) + 1,
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
		inner:     resp.Body,
		sink:      sink,
		startedAt: startedAt,
		base: Attempt{
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

type recordingBody struct {
	inner      io.ReadCloser
	buf        bytes.Buffer
	sink       *attemptSink
	base       Attempt
	startedAt  time.Time
	recordOnce sync.Once
	closeOnce  sync.Once
}

func (r *recordingBody) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		_, _ = r.buf.Write(p[:n])
	}
	if err != nil {
		r.record(err)
	}
	return n, err
}

func (r *recordingBody) Close() error {
	r.record(nil)

	var closeErr error
	r.closeOnce.Do(func() {
		closeErr = r.inner.Close()
	})
	return closeErr
}

func (r *recordingBody) record(err error) {
	r.recordOnce.Do(func() {
		finishedAt := time.Now()
		r.base.Number = len(r.sink.snapshot()) + 1
		r.base.ResponseBody = RedactJSONSecrets(r.buf.Bytes())
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

package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/tracing"

	"cdr.dev/slog"
)

// These cookies are Coder-specific. If a new one is added or changed, the name
// shouldn't be likely to conflict with any user-application set cookies.
// Be sure to strip additional cookies in httpapi.StripCoderCookies!
const (
	// SessionTokenCookie represents the name of the cookie or query parameter the API key is stored in.
	SessionTokenCookie = "coder_session_token"
	// SessionTokenHeader is the custom header to use for authentication.
	SessionTokenHeader = "Coder-Session-Token"
	// OAuth2StateCookie is the name of the cookie that stores the oauth2 state.
	OAuth2StateCookie = "oauth_state"
	// OAuth2RedirectCookie is the name of the cookie that stores the oauth2 redirect.
	OAuth2RedirectCookie = "oauth_redirect"

	// BypassRatelimitHeader is the custom header to use to bypass ratelimits.
	// Only owners can bypass rate limits. This is typically used for scale testing.
	// nolint: gosec
	BypassRatelimitHeader = "X-Coder-Bypass-Ratelimit"
)

// loggableMimeTypes is a list of MIME types that are safe to log
// the output of. This is useful for debugging or testing.
var loggableMimeTypes = map[string]struct{}{
	"application/json": {},
	"text/plain":       {},
	// lots of webserver error pages are HTML
	"text/html": {},
}

// New creates a Coder client for the provided URL.
func New(serverURL *url.URL) *Client {
	return &Client{
		URL:        serverURL,
		HTTPClient: &http.Client{},
	}
}

// Client is an HTTP caller for methods to the Coder API.
// @typescript-ignore Client
type Client struct {
	mu           sync.RWMutex // Protects following.
	sessionToken string

	HTTPClient *http.Client
	URL        *url.URL

	// Logger is optionally provided to log requests.
	// Method, URL, and response code will be logged by default.
	Logger slog.Logger

	// LogBodies can be enabled to print request and response bodies to the logger.
	LogBodies bool

	// Trace can be enabled to propagate tracing spans to the Coder API.
	// This is useful for tracking a request end-to-end.
	Trace bool
}

// SessionToken returns the currently set token for the client.
func (c *Client) SessionToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionToken
}

// SetSessionToken returns the currently set token for the client.
func (c *Client) SetSessionToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionToken = token
}

// Request performs a HTTP request with the body provided. The caller is
// responsible for closing the response body.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}, opts ...RequestOption) (*http.Response, error) {
	ctx, span := tracing.StartSpanWithName(ctx, tracing.FuncNameSkip(1))
	defer span.End()

	serverURL, err := c.URL.Parse(path)
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}

	var r io.Reader
	if body != nil {
		switch data := body.(type) {
		case io.Reader:
			r = data
		case []byte:
			r = bytes.NewReader(data)
		default:
			// Assume JSON in all other cases.
			buf := bytes.NewBuffer(nil)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			err = enc.Encode(body)
			if err != nil {
				return nil, xerrors.Errorf("encode body: %w", err)
			}
			r = buf
		}
	}

	// Copy the request body so we can log it.
	var reqBody []byte
	if r != nil && c.LogBodies {
		reqBody, err = io.ReadAll(r)
		if err != nil {
			return nil, xerrors.Errorf("read request body: %w", err)
		}
		r = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, serverURL.String(), r)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	req.Header.Set(SessionTokenHeader, c.SessionToken())

	if r != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}

	span.SetAttributes(semconv.NetAttributesFromHTTPRequest("tcp", req)...)
	span.SetAttributes(semconv.HTTPClientAttributesFromHTTPRequest(req)...)

	// Inject tracing headers if enabled.
	if c.Trace {
		tmp := otel.GetTextMapPropagator()
		hc := propagation.HeaderCarrier(req.Header)
		tmp.Inject(ctx, hc)
	}

	// We already capture most of this information in the span (minus
	// the request body which we don't want to capture anyways).
	ctx = slog.With(ctx,
		slog.F("method", req.Method),
		slog.F("url", req.URL.String()),
	)
	tracing.RunWithoutSpan(ctx, func(ctx context.Context) {
		c.Logger.Debug(ctx, "sdk request", slog.F("body", string(reqBody)))
	})

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("do: %w", err)
	}

	span.SetAttributes(semconv.HTTPStatusCodeKey.Int(resp.StatusCode))
	span.SetStatus(semconv.SpanStatusFromHTTPStatusCodeAndSpanKind(resp.StatusCode, trace.SpanKindClient))

	// Copy the response body so we can log it if it's a loggable mime type.
	var respBody []byte
	if resp.Body != nil && c.LogBodies {
		mimeType := parseMimeType(resp.Header.Get("Content-Type"))
		if _, ok := loggableMimeTypes[mimeType]; ok {
			respBody, err = io.ReadAll(resp.Body)
			if err != nil {
				return nil, xerrors.Errorf("copy response body for logs: %w", err)
			}
			err = resp.Body.Close()
			if err != nil {
				return nil, xerrors.Errorf("close response body: %w", err)
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
		}
	}

	// See above for why this is not logged to the span.
	tracing.RunWithoutSpan(ctx, func(ctx context.Context) {
		c.Logger.Debug(ctx, "sdk response",
			slog.F("status", resp.StatusCode),
			slog.F("body", string(respBody)),
			slog.F("trace_id", resp.Header.Get("X-Trace-Id")),
			slog.F("span_id", resp.Header.Get("X-Span-Id")),
		)
	})

	return resp, err
}

// ReadBodyAsError reads the response as a codersdk.Response, and
// wraps it in a codersdk.Error type for easy marshaling.
func ReadBodyAsError(res *http.Response) error {
	if res == nil {
		return xerrors.Errorf("no body returned")
	}
	defer res.Body.Close()
	contentType := res.Header.Get("Content-Type")

	var requestMethod, requestURL string
	if res.Request != nil {
		requestMethod = res.Request.Method
		if res.Request.URL != nil {
			requestURL = res.Request.URL.String()
		}
	}

	var helpMessage string
	if res.StatusCode == http.StatusUnauthorized {
		// 401 means the user is not logged in
		// 403 would mean that the user is not authorized
		helpMessage = "Try logging in using 'coder login <url>'."
	}

	resp, err := io.ReadAll(res.Body)
	if err != nil {
		return xerrors.Errorf("read body: %w", err)
	}

	mimeType := parseMimeType(contentType)
	if mimeType != "application/json" {
		if len(resp) > 1024 {
			resp = append(resp[:1024], []byte("...")...)
		}
		if len(resp) == 0 {
			resp = []byte("no response body")
		}
		return &Error{
			statusCode: res.StatusCode,
			Response: Response{
				Message: fmt.Sprintf("unexpected non-JSON response %q", contentType),
				Detail:  string(resp),
			},
			Helper: helpMessage,
		}
	}

	var m Response
	err = json.NewDecoder(bytes.NewBuffer(resp)).Decode(&m)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return &Error{
				statusCode: res.StatusCode,
				Response: Response{
					Message: "empty response body",
				},
				Helper: helpMessage,
			}
		}
		return xerrors.Errorf("decode body: %w", err)
	}
	if m.Message == "" {
		if len(resp) > 1024 {
			resp = append(resp[:1024], []byte("...")...)
		}
		m.Message = fmt.Sprintf("unexpected status code %d, response has no message", res.StatusCode)
		m.Detail = string(resp)
	}

	return &Error{
		Response:   m,
		statusCode: res.StatusCode,
		method:     requestMethod,
		url:        requestURL,
		Helper:     helpMessage,
	}
}

// Error represents an unaccepted or invalid request to the API.
// @typescript-ignore Error
type Error struct {
	Response

	statusCode int
	method     string
	url        string

	Helper string
}

func (e *Error) StatusCode() int {
	return e.statusCode
}

func (e *Error) Friendly() string {
	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "%s. %s", strings.TrimSuffix(e.Message, "."), e.Helper)
	for _, err := range e.Validations {
		_, _ = fmt.Fprintf(&sb, "\n- %s: %s", err.Field, err.Detail)
	}
	return sb.String()
}

func (e *Error) Error() string {
	var builder strings.Builder
	if e.method != "" && e.url != "" {
		_, _ = fmt.Fprintf(&builder, "%v %v: ", e.method, e.url)
	}
	_, _ = fmt.Fprintf(&builder, "unexpected status code %d: %s", e.statusCode, e.Message)
	if e.Helper != "" {
		_, _ = fmt.Fprintf(&builder, ": %s", e.Helper)
	}
	if e.Detail != "" {
		_, _ = fmt.Fprintf(&builder, "\n\tError: %s", e.Detail)
	}
	for _, err := range e.Validations {
		_, _ = fmt.Fprintf(&builder, "\n\t%s: %s", err.Field, err.Detail)
	}
	return builder.String()
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}

func parseMimeType(contentType string) string {
	mimeType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mimeType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}

	return mimeType
}

// Response represents a generic HTTP response.
type Response struct {
	// Message is an actionable message that depicts actions the request took.
	// These messages should be fully formed sentences with proper punctuation.
	// Examples:
	// - "A user has been created."
	// - "Failed to create a user."
	Message string `json:"message"`
	// Detail is a debug message that provides further insight into why the
	// action failed. This information can be technical and a regular golang
	// err.Error() text.
	// - "database: too many open connections"
	// - "stat: too many open files"
	Detail string `json:"detail,omitempty"`
	// Validations are form field-specific friendly error messages. They will be
	// shown on a form field in the UI. These can also be used to add additional
	// context if there is a set of errors in the primary 'Message'.
	Validations []ValidationError `json:"validations,omitempty"`
}

// ValidationError represents a scoped error to a user input.
type ValidationError struct {
	Field  string `json:"field" validate:"required"`
	Detail string `json:"detail" validate:"required"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("field: %s detail: %s", e.Field, e.Detail)
}

var _ error = (*ValidationError)(nil)

// IsConnectionError is a convenience function for checking if the source of an
// error is due to a 'connection refused', 'no such host', etc.
func IsConnectionError(err error) bool {
	var (
		// E.g. no such host
		dnsErr *net.DNSError
		// Eg. connection refused
		opErr *net.OpError
	)

	return xerrors.As(err, &dnsErr) || xerrors.As(err, &opErr)
}

func AsError(err error) (*Error, bool) {
	var e *Error
	return e, xerrors.As(err, &e)
}

// RequestOption is a function that can be used to modify an http.Request.
type RequestOption func(*http.Request)

// WithQueryParam adds a query parameter to the request.
func WithQueryParam(key, value string) RequestOption {
	return func(r *http.Request) {
		if value == "" {
			return
		}
		q := r.URL.Query()
		q.Add(key, value)
		r.URL.RawQuery = q.Encode()
	}
}

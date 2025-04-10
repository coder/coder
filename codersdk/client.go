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
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/semconv/v1.14.0/httpconv"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/websocket"

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

	// PathAppSessionTokenCookie is the name of the cookie that stores an
	// application-scoped API token on workspace proxy path app domains.
	//nolint:gosec
	PathAppSessionTokenCookie = "coder_path_app_session_token"
	// SubdomainAppSessionTokenCookie is the name of the cookie that stores an
	// application-scoped API token on subdomain app domains (both the primary
	// and proxies).
	//nolint:gosec
	SubdomainAppSessionTokenCookie = "coder_subdomain_app_session_token"
	// SignedAppTokenCookie is the name of the cookie that stores a temporary
	// JWT that can be used to authenticate instead of the app session token.
	//nolint:gosec
	SignedAppTokenCookie = "coder_signed_app_token"
	// SignedAppTokenQueryParameter is the name of the query parameter that
	// stores a temporary JWT that can be used to authenticate instead of the
	// session token. This is only acceptable on reconnecting-pty requests, not
	// apps.
	//
	// It has a random suffix to avoid conflict with user query parameters on
	// apps.
	//nolint:gosec
	SignedAppTokenQueryParameter = "coder_signed_app_token_23db1dde"

	// BypassRatelimitHeader is the custom header to use to bypass ratelimits.
	// Only owners can bypass rate limits. This is typically used for scale testing.
	// nolint: gosec
	BypassRatelimitHeader = "X-Coder-Bypass-Ratelimit"

	// Note: the use of X- prefix is deprecated, and we should eventually remove
	// it from BypassRatelimitHeader.
	//
	// See: https://datatracker.ietf.org/doc/html/rfc6648.

	// CLITelemetryHeader contains a base64-encoded representation of the CLI
	// command that was invoked to produce the request. It is for internal use
	// only.
	CLITelemetryHeader = "Coder-CLI-Telemetry"

	// CoderDesktopTelemetryHeader contains a JSON-encoded representation of Desktop telemetry
	// fields, including device ID, OS, and Desktop version.
	CoderDesktopTelemetryHeader = "Coder-Desktop-Telemetry"

	// ProvisionerDaemonPSK contains the authentication pre-shared key for an external provisioner daemon
	ProvisionerDaemonPSK = "Coder-Provisioner-Daemon-PSK"

	// ProvisionerDaemonKey contains the authentication key for an external provisioner daemon
	ProvisionerDaemonKey = "Coder-Provisioner-Daemon-Key"

	// BuildVersionHeader contains build information of Coder.
	BuildVersionHeader = "X-Coder-Build-Version"

	// EntitlementsWarnings contains active warnings for the user's entitlements.
	EntitlementsWarningHeader = "X-Coder-Entitlements-Warning"
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
	// mu protects the fields sessionToken, logger, and logBodies. These
	// need to be safe for concurrent access.
	mu           sync.RWMutex
	sessionToken string
	logger       slog.Logger
	logBodies    bool

	HTTPClient *http.Client
	URL        *url.URL

	// SessionTokenHeader is an optional custom header to use for setting tokens. By
	// default 'Coder-Session-Token' is used.
	SessionTokenHeader string

	// PlainLogger may be set to log HTTP traffic in a human-readable form.
	// It uses the LogBodies option.
	PlainLogger io.Writer

	// Trace can be enabled to propagate tracing spans to the Coder API.
	// This is useful for tracking a request end-to-end.
	Trace bool

	// DisableDirectConnections forces any connections to workspaces to go
	// through DERP, regardless of the BlockEndpoints setting on each
	// connection.
	DisableDirectConnections bool
}

// Logger returns the logger for the client.
func (c *Client) Logger() slog.Logger {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.logger
}

// SetLogger sets the logger for the client.
func (c *Client) SetLogger(logger slog.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = logger
}

// LogBodies returns whether requests and response bodies are logged.
func (c *Client) LogBodies() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.logBodies
}

// SetLogBodies sets whether to log request and response bodies.
func (c *Client) SetLogBodies(logBodies bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logBodies = logBodies
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

func prefixLines(prefix, s []byte) []byte {
	ss := bytes.NewBuffer(make([]byte, 0, len(s)*2))
	for _, line := range bytes.Split(s, []byte("\n")) {
		_, _ = ss.Write(prefix)
		_, _ = ss.Write(line)
		_ = ss.WriteByte('\n')
	}
	return ss.Bytes()
}

// Request performs a HTTP request with the body provided. The caller is
// responsible for closing the response body.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}, opts ...RequestOption) (*http.Response, error) {
	if ctx == nil {
		return nil, xerrors.Errorf("context should not be nil")
	}
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
	c.mu.RLock()
	logBodies := c.logBodies
	c.mu.RUnlock()
	if r != nil && logBodies {
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

	tokenHeader := c.SessionTokenHeader
	if tokenHeader == "" {
		tokenHeader = SessionTokenHeader
	}
	req.Header.Set(tokenHeader, c.SessionToken())

	if r != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}

	span.SetAttributes(httpconv.ClientRequest(req)...)

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
		c.Logger().Debug(ctx, "sdk request", slog.F("body", string(reqBody)))
	})

	resp, err := c.HTTPClient.Do(req)

	// We log after sending the request because the HTTP Transport may modify
	// the request within Do, e.g. by adding headers.
	if resp != nil && c.PlainLogger != nil {
		out, err := httputil.DumpRequest(resp.Request, logBodies)
		if err != nil {
			return nil, xerrors.Errorf("dump request: %w", err)
		}
		out = prefixLines([]byte("http --> "), out)
		_, _ = c.PlainLogger.Write(out)
	}

	if err != nil {
		return nil, err
	}

	if c.PlainLogger != nil {
		out, err := httputil.DumpResponse(resp, logBodies)
		if err != nil {
			return nil, xerrors.Errorf("dump response: %w", err)
		}
		out = prefixLines([]byte("http <-- "), out)
		_, _ = c.PlainLogger.Write(out)
	}

	span.SetAttributes(httpconv.ClientResponse(resp)...)
	span.SetStatus(httpconv.ClientStatus(resp.StatusCode))

	// Copy the response body so we can log it if it's a loggable mime type.
	var respBody []byte
	if resp.Body != nil && logBodies {
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
		c.Logger().Debug(ctx, "sdk response",
			slog.F("status", resp.StatusCode),
			slog.F("body", string(respBody)),
			slog.F("trace_id", resp.Header.Get("X-Trace-Id")),
			slog.F("span_id", resp.Header.Get("X-Span-Id")),
		)
	})

	return resp, err
}

func (c *Client) Dial(ctx context.Context, path string, opts *websocket.DialOptions) (*websocket.Conn, error) {
	u, err := c.URL.Parse(path)
	if err != nil {
		return nil, err
	}

	tokenHeader := c.SessionTokenHeader
	if tokenHeader == "" {
		tokenHeader = SessionTokenHeader
	}

	if opts == nil {
		opts = &websocket.DialOptions{}
	}
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = http.Header{}
	}
	if opts.HTTPHeader.Get("tokenHeader") == "" {
		opts.HTTPHeader.Set(tokenHeader, c.SessionToken())
	}

	conn, resp, err := websocket.Dial(ctx, u.String(), opts)
	if resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// ExpectJSONMime is a helper function that will assert the content type
// of the response is application/json.
func ExpectJSONMime(res *http.Response) error {
	contentType := res.Header.Get("Content-Type")
	mimeType := parseMimeType(contentType)
	if mimeType != "application/json" {
		return xerrors.Errorf("unexpected non-JSON response %q", contentType)
	}
	return nil
}

// ReadBodyAsError reads the response as a codersdk.Response, and
// wraps it in a codersdk.Error type for easy marshaling.
//
// This will always return an error, so only call it if the response failed
// your expectations. Usually via status code checking.
// nolint:staticcheck
func ReadBodyAsError(res *http.Response) error {
	if res == nil {
		return xerrors.Errorf("no body returned")
	}
	defer res.Body.Close()

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
		helpMessage = "Try logging in using 'coder login'."
	}

	resp, err := io.ReadAll(res.Body)
	if err != nil {
		return xerrors.Errorf("read body: %w", err)
	}

	if mimeErr := ExpectJSONMime(res); mimeErr != nil {
		if len(resp) > 2048 {
			resp = append(resp[:2048], []byte("...")...)
		}
		if len(resp) == 0 {
			resp = []byte("no response body")
		}
		return &Error{
			statusCode: res.StatusCode,
			method:     requestMethod,
			url:        requestURL,
			Response: Response{
				Message: mimeErr.Error(),
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

func (e *Error) Method() string {
	return e.method
}

func (e *Error) URL() string {
	return e.url
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

// CoderDesktopTelemetry represents the telemetry data sent from Coder Desktop clients.
// @typescript-ignore CoderDesktopTelemetry
type CoderDesktopTelemetry struct {
	DeviceID            string `json:"device_id"`
	DeviceOS            string `json:"device_os"`
	CoderDesktopVersion string `json:"coder_desktop_version"`
}

// FromHeader parses the desktop telemetry from the provided header value.
// Returns nil if the header is empty or if parsing fails.
func (t *CoderDesktopTelemetry) FromHeader(headerValue string) error {
	if headerValue == "" {
		return nil
	}
	return json.Unmarshal([]byte(headerValue), t)
}

// IsEmpty returns true if all fields in the telemetry data are empty.
func (t *CoderDesktopTelemetry) IsEmpty() bool {
	return t.DeviceID == "" && t.DeviceOS == "" && t.CoderDesktopVersion == ""
}

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

// HeaderTransport is a http.RoundTripper that adds some headers to all requests.
// @typescript-ignore HeaderTransport
type HeaderTransport struct {
	Transport http.RoundTripper
	Header    http.Header
}

var _ http.RoundTripper = &HeaderTransport{}

func (h *HeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Header {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}
	if h.Transport == nil {
		h.Transport = http.DefaultTransport
	}
	return h.Transport.RoundTrip(req)
}

func (h *HeaderTransport) CloseIdleConnections() {
	type closeIdler interface {
		CloseIdleConnections()
	}
	if tr, ok := h.Transport.(closeIdler); ok {
		tr.CloseIdleConnections()
	}
}

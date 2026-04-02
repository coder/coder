package codersdk

import (
	"bytes"
	"context"
	"crypto/tls"
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
	"unicode"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/semconv/v1.14.0/httpconv"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/websocket"
)

// These cookies are Coder-specific. If a new one is added or changed, the name
// shouldn't be likely to conflict with any user-application set cookies.
// Be sure to strip additional cookies in httpapi.StripCoderCookies!
// SessionTokenCookie represents the name of the cookie or query parameter the API key is stored in.
// NOTE: This is declared as a var so that we can override it in `develop.sh` if required.
var SessionTokenCookie = "coder_session_token"

const (
	// SessionTokenHeader is the custom header to use for authentication.
	SessionTokenHeader = "Coder-Session-Token"
	// OAuth2StateCookie is the name of the cookie that stores the oauth2 state.
	OAuth2StateCookie = "oauth_state"
	// OAuth2PKCEVerifier is the name of the cookie that stores the oauth2 PKCE
	// verifier. This is the raw verifier that when hashed, will match the challenge
	// sent in the initial oauth2 request.
	OAuth2PKCEVerifier = "oauth_pkce_verifier"
	// OAuth2RedirectCookie is the name of the cookie that stores the oauth2 redirect.
	OAuth2RedirectCookie = "oauth_redirect"
	// OAuth2RedirectURICookie stores the dynamically computed OIDC redirect_uri
	// when CODER_OIDC_REDIRECT_ALLOWED_HOSTS is enabled. The same value must be
	// used for both the authorization request and the token exchange (RFC 6749
	// section 4.1.3).
	OAuth2RedirectURICookie = "oauth_redirect_uri"

	// PathAppSessionTokenCookie is the name of the cookie that stores an
	// application-scoped API token on workspace proxy path app domains.
	//nolint:gosec
	PathAppSessionTokenCookie = "coder_path_app_session_token"
	// SubdomainAppSessionTokenCookie is the name of the cookie that stores an
	// application-scoped API token on subdomain app domains (both the primary
	// and proxies).
	//
	// To avoid conflicts between multiple proxies, we append an underscore and
	// a hash suffix to the cookie name.
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

	// AIGatewayKeyHeader contains the authentication key for a standalone AI Gateway replica.
	AIGatewayKeyHeader = "X-Coder-AI-Governance-Gateway-Key"

	// BuildVersionHeader contains build information of Coder.
	BuildVersionHeader = "X-Coder-Build-Version"

	// EntitlementsWarnings contains active warnings for the user's entitlements.
	EntitlementsWarningHeader = "X-Coder-Entitlements-Warning"

	// Visitor identity headers injected by the workspace app proxy.
	// These identify the authenticated user visiting a workspace app.
	VisitorUserIDHeader    = "X-Coder-User-Id"
	VisitorUsernameHeader  = "X-Coder-Username"
	VisitorUserEmailHeader = "X-Coder-User-Email"
)

// loggableMimeTypes is a list of MIME types that are safe to log
// the output of. This is useful for debugging or testing.
var loggableMimeTypes = map[string]struct{}{
	"application/json": {},
	"text/plain":       {},
	// lots of webserver error pages are HTML
	"text/html": {},
}

type ClientOption func(*Client)

// New creates a Coder client for the provided URL.
func New(serverURL *url.URL, opts ...ClientOption) *Client {
	client := &Client{
		URL:                  serverURL,
		HTTPClient:           &http.Client{},
		SessionTokenProvider: FixedSessionTokenProvider{},
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

// Client is an HTTP caller for methods to the Coder API.
// @typescript-ignore Client
type Client struct {
	// mu protects the fields sessionToken, logger, and logBodies. These
	// need to be safe for concurrent access.
	mu                   sync.RWMutex
	SessionTokenProvider SessionTokenProvider
	logger               slog.Logger
	logBodies            bool

	HTTPClient *http.Client
	URL        *url.URL

	// PlainLogger may be set to log HTTP traffic in a human-readable form.
	// It uses the LogBodies option.
	// Deprecated: Use WithPlainLogger to set this.
	PlainLogger io.Writer

	// Trace can be enabled to propagate tracing spans to the Coder API.
	// This is useful for tracking a request end-to-end.
	// Deprecated: Use WithTrace to set this.
	Trace bool

	// DisableDirectConnections forces any connections to workspaces to go
	// through DERP, regardless of the BlockEndpoints setting on each
	// connection.
	// Deprecated: Use WithDisableDirectConnections to set this.
	DisableDirectConnections bool

	// derpTLSConfig is an optional TLS config for DERP connections.
	derpTLSConfig *tls.Config
}

// Logger returns the logger for the client.
func (c *Client) Logger() slog.Logger {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.logger
}

// SetLogger sets the logger for the client.
// Deprecated: Use WithLogger to set this.
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
// Deprecated: Use WithLogBodies to set this.
func (c *Client) SetLogBodies(logBodies bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logBodies = logBodies
}

// SessionToken returns the currently set token for the client.
func (c *Client) SessionToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SessionTokenProvider.GetSessionToken()
}

// SetSessionToken sets a fixed token for the client.
// Deprecated: Create a new client using WithSessionToken instead of changing the token after creation.
func (c *Client) SetSessionToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SessionTokenProvider = FixedSessionTokenProvider{SessionToken: token}
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
	opts = append([]RequestOption{c.SessionTokenProvider.AsRequestOption()}, opts...)
	return c.RequestWithoutSessionToken(ctx, method, path, body, opts...)
}

// RequestWithoutSessionToken performs a HTTP request. It is similar to Request, but does not set
// the session token in the request header, nor does it make a call to the SessionTokenProvider.
// This allows session token providers to call this method without causing reentrancy issues.
func (c *Client) RequestWithoutSessionToken(ctx context.Context, method, path string, body interface{}, opts ...RequestOption) (*http.Response, error) {
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
	var reqLogFields []slog.Field
	c.mu.RLock()
	logBodies := c.logBodies
	c.mu.RUnlock()
	if r != nil && logBodies {
		reqBody, err := io.ReadAll(r)
		if err != nil {
			return nil, xerrors.Errorf("read request body: %w", err)
		}
		r = bytes.NewReader(reqBody)
		reqLogFields = append(reqLogFields, slog.F("body", string(reqBody)))
	}

	req, err := http.NewRequestWithContext(ctx, method, serverURL.String(), r)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}

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
		c.Logger().Debug(ctx, "sdk request", reqLogFields...)
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
	var respLogFields []slog.Field
	if resp.Body != nil && logBodies {
		mimeType := parseMimeType(resp.Header.Get("Content-Type"))
		if _, ok := loggableMimeTypes[mimeType]; ok {
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, xerrors.Errorf("copy response body for logs: %w", err)
			}
			err = resp.Body.Close()
			if err != nil {
				return nil, xerrors.Errorf("close response body: %w", err)
			}
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			respLogFields = append(respLogFields, slog.F("body", string(respBody)))
		}
	}

	// See above for why this is not logged to the span.
	tracing.RunWithoutSpan(ctx, func(ctx context.Context) {
		c.Logger().Debug(ctx, "sdk response",
			append(respLogFields,
				slog.F("status", resp.StatusCode),
				slog.F("trace_id", resp.Header.Get("X-Trace-Id")),
				slog.F("span_id", resp.Header.Get("X-Span-Id")),
			)...,
		)
	})

	return resp, err
}

func (c *Client) Dial(ctx context.Context, path string, opts *websocket.DialOptions) (*websocket.Conn, error) {
	u, err := c.URL.Parse(path)
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &websocket.DialOptions{}
	}
	// Propagate the client's HTTP client to the websocket dialer
	// so that custom TLS configurations (e.g. mesh TLS between
	// replicas) are used for the handshake request. Without this,
	// the websocket library falls back to http.DefaultClient.
	if opts.HTTPClient == nil {
		opts.HTTPClient = c.HTTPClient
	}
	c.SessionTokenProvider.SetDialOption(opts)

	conn, resp, err := websocket.Dial(ctx, u.String(), opts)
	if resp != nil && resp.Body != nil {
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
		return newResponseError(res, Response{
			Message: mimeErr.Error(),
			Detail:  string(resp),
		})
	}

	var m Response
	err = json.NewDecoder(bytes.NewBuffer(resp)).Decode(&m)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return newResponseError(res, Response{
				Message: "empty response body",
			})
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

	return newResponseError(res, m)
}

// newResponseError wraps an API response in an *Error annotated with
// the status code, request method, and request URL from res. For 401
// responses it also sets a helper message suggesting 'coder login'.
func newResponseError(res *http.Response, response Response) *Error {
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

	return &Error{
		Response:   response,
		statusCode: res.StatusCode,
		method:     requestMethod,
		url:        requestURL,
		Helper:     helpMessage,
	}
}

// jsonBodySniffLen bounds how much of a response body is retained while
// decoding to identify HTML without buffering the entire response.
const jsonBodySniffLen = 512

// htmlResponseHelper suggests the most common causes of receiving HTML
// from what should be a Coder API endpoint: a misconfigured Coder URL,
// or an intermediary such as a reverse proxy or SSO portal intercepting
// API requests.
const htmlResponseHelper = "Ensure the Coder URL is correct and that any reverse proxy or SSO in front of it passes /api/v2 requests through to Coder."

// ReadBodyAsJSON decodes the response body as JSON into v. It is
// intended for typed API endpoints whose accepted responses are always
// JSON, replacing direct json.NewDecoder(res.Body).Decode(...) calls.
// It must not be used for streaming or non-JSON endpoints such as
// WebSockets, server-sent events, or file downloads.
//
// Bodies that are HTML, empty, or otherwise not valid JSON produce a
// structured *Error describing the response instead of a bare decode
// error such as "invalid character '<' looking for beginning of value".
// Intermediaries like reverse proxies and SSO portals commonly return
// such bodies with a 200 status code.
//
// Valid JSON is decoded even when an intermediary omits or mislabels
// the Content-Type header. The exception is a Content-Type declaring
// HTML, which is always reported as an invalid response since Coder
// API endpoints never serve HTML. The caller remains responsible for
// closing the response body.
func ReadBodyAsJSON(res *http.Response, v any) error {
	return decodeBodyAsJSON(res, v, nil)
}

// ReadBodyAsJSONUseNumber behaves like ReadBodyAsJSON but decodes JSON
// numbers into json.Number instead of float64, preserving integer
// precision for callers that re-serialize or type-assert numeric
// claims, such as license JWT claims.
func ReadBodyAsJSONUseNumber(res *http.Response, v any) error {
	return decodeBodyAsJSON(res, v, func(dec *json.Decoder) {
		dec.UseNumber()
	})
}

// decodeBodyAsJSON decodes the response body as JSON into v. When
// configure is non-nil it is called with the decoder before decoding,
// allowing callers to set options such as UseNumber.
func decodeBodyAsJSON(res *http.Response, v any, configure func(*json.Decoder)) error {
	if res == nil || res.Body == nil {
		return xerrors.New("no response body to decode")
	}

	mimeType := parseMimeType(res.Header.Get("Content-Type"))
	if isHTMLMimeType(mimeType) {
		return htmlBodyError(res)
	}

	body := &responseBodyReader{Reader: res.Body}
	prefix := &bodyPrefixWriter{}
	dec := json.NewDecoder(io.TeeReader(body, prefix))
	if configure != nil {
		configure(dec)
	}
	err := dec.Decode(v)
	switch {
	case err == nil:
		return nil
	case body.err != nil && errors.Is(err, body.err):
		return xerrors.Errorf("read response body: %w", err)
	case len(prefix.bytes) == 0 && errors.Is(err, io.EOF):
		return invalidBodyError(res, Response{
			Message: "Received an empty response from the Coder API.",
			Detail:  invalidBodyDetail(res),
		}, "", nil)
	case isHTMLBody(mimeType, prefix.bytes):
		return htmlBodyError(res)
	default:
		return invalidBodyError(res, Response{
			Message: "Received an invalid JSON response from the Coder API.",
			Detail:  fmt.Sprintf("decode body: %s, %s", err.Error(), invalidBodyDetail(res)),
		}, "", err)
	}
}

// htmlBodyError returns the structured *Error reported when the Coder API
// response body is HTML rather than JSON.
func htmlBodyError(res *http.Response) *Error {
	return invalidBodyError(res, Response{
		Message: "Received an HTML response instead of JSON from the Coder API.",
		Detail:  invalidBodyDetail(res),
	}, htmlResponseHelper, nil)
}

// responseBodyReader records non-EOF read errors so decode errors from custom
// JSON unmarshallers are not mistaken for transport failures.
type responseBodyReader struct {
	io.Reader
	err error
}

func (r *responseBodyReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return n, err
}

// bodyPrefixWriter retains only the beginning of a body while reporting every
// byte as written so it can be used with io.TeeReader.
type bodyPrefixWriter struct {
	bytes []byte
}

func (w *bodyPrefixWriter) Write(p []byte) (int, error) {
	if remaining := jsonBodySniffLen - len(w.bytes); remaining > 0 {
		w.bytes = append(w.bytes, p[:min(remaining, len(p))]...)
	}
	return len(p), nil
}

// isHTMLBody reports whether a response body is HTML, either by its
// media type or by starting with '<' after optional whitespace. Valid
// JSON can never start with '<', so the sniff also identifies HTML
// bodies mislabeled with a JSON content type. Markup bodies such as
// XML are intentionally classified the same way since the remediation,
// fixing the URL or the intermediary, is identical.
func isHTMLBody(mimeType string, prefix []byte) bool {
	if isHTMLMimeType(mimeType) {
		return true
	}
	// Strip a UTF-8 byte order mark, which some intermediaries prepend
	// to HTML error pages.
	trimmed := bytes.TrimPrefix(prefix, []byte("\xef\xbb\xbf"))
	trimmed = bytes.TrimLeftFunc(trimmed, unicode.IsSpace)
	return len(trimmed) > 0 && trimmed[0] == '<'
}

func isHTMLMimeType(mimeType string) bool {
	return mimeType == "text/html" || mimeType == "application/xhtml+xml"
}

// invalidBodyDetail describes an invalid response body and how it was
// reached, for inclusion in an Error Detail.
func invalidBodyDetail(res *http.Response) string {
	var sb strings.Builder
	if contentType := res.Header.Get("Content-Type"); contentType != "" {
		_, _ = fmt.Fprintf(&sb, "content type %q", contentType)
	} else {
		_, _ = sb.WriteString("no content type")
	}
	if orig := originalRequestURL(res); orig != "" && res.Request != nil && res.Request.URL != nil && orig != res.Request.URL.String() {
		_, _ = fmt.Fprintf(&sb, ", after following redirects from %s", orig)
	}
	return sb.String()
}

// originalRequestURL returns the URL of the first request in the
// redirect chain that produced res, or "" when unknown. res.Request
// itself is the final request after any redirects.
func originalRequestURL(res *http.Response) string {
	req := res.Request
	for req != nil && req.Response != nil && req.Response.Request != nil {
		req = req.Response.Request
	}
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

// invalidBodyError builds the *Error returned when a response with an
// accepted status code does not contain the expected JSON body.
func invalidBodyError(res *http.Response, response Response, helper string, cause error) *Error {
	var requestMethod, requestURL string
	if res.Request != nil {
		requestMethod = res.Request.Method
		if res.Request.URL != nil {
			requestURL = res.Request.URL.String()
		}
	}
	return &Error{
		Response:    response,
		statusCode:  res.StatusCode,
		method:      requestMethod,
		url:         requestURL,
		contentType: res.Header.Get("Content-Type"),
		cause:       cause,
		invalidBody: true,
		Helper:      helper,
	}
}

// Error represents an unaccepted or invalid request to the API.
// @typescript-ignore Error
type Error struct {
	Response

	statusCode  int
	method      string
	url         string
	contentType string
	// cause is the underlying error, such as a JSON decode failure,
	// exposed to errors.Is and errors.As via Unwrap.
	cause error
	// invalidBody marks errors for responses whose status code was
	// accepted but whose body was not the expected JSON. Error() then
	// describes the body rather than an unexpected status code.
	invalidBody bool

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

// ContentType returns the Content-Type header of the response that
// produced the error, when known.
func (e *Error) ContentType() string {
	return e.contentType
}

// Unwrap returns the underlying cause of the error, if any, such as the
// JSON decode error for a response body that could not be decoded.
func (e *Error) Unwrap() error {
	return e.cause
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
	if e.invalidBody {
		_, _ = fmt.Fprintf(&builder, "invalid API response (status code %d): %s", e.statusCode, e.Message)
	} else {
		_, _ = fmt.Fprintf(&builder, "unexpected status code %d: %s", e.statusCode, e.Message)
	}
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

// NewTestError is a helper function to create a Error, setting the internal fields. It's generally only useful for
// testing.
func NewTestError(statusCode int, method string, u string) *Error {
	return &Error{
		statusCode: statusCode,
		method:     method,
		url:        u,
	}
}

// NewError creates a new Error with the response and status code.
func NewError(statusCode int, response Response) *Error {
	return &Error{
		statusCode: statusCode,
		Response:   response,
	}
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
		return http.DefaultTransport.RoundTrip(req)
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

// ClientOptions

func WithSessionToken(token string) ClientOption {
	return func(c *Client) {
		c.SessionTokenProvider = FixedSessionTokenProvider{SessionToken: token}
	}
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.HTTPClient = httpClient
	}
}

func WithLogger(logger slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

func WithLogBodies() ClientOption {
	return func(c *Client) {
		c.logBodies = true
	}
}

func WithPlainLogger(plainLogger io.Writer) ClientOption {
	return func(c *Client) {
		c.PlainLogger = plainLogger
	}
}

func WithTrace() ClientOption {
	return func(c *Client) {
		c.Trace = true
	}
}

func WithDisableDirectConnections() ClientOption {
	return func(c *Client) {
		c.DisableDirectConnections = true
	}
}

func WithDERPTLSConfig(cfg *tls.Config) ClientOption {
	return func(c *Client) {
		c.derpTLSConfig = cfg
	}
}

// DERPTLSConfig returns the optional TLS config for DERP connections.
func (c *Client) DERPTLSConfig() *tls.Config {
	return c.derpTLSConfig
}

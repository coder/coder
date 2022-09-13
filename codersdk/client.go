package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
)

// These cookies are Coder-specific. If a new one is added or changed, the name
// shouldn't be likely to conflict with any user-application set cookies.
// Be sure to strip additional cookies in httpapi.StripCoder Cookies!
const (
	// SessionTokenKey represents the name of the cookie or query parameter the API key is stored in.
	SessionTokenKey = "coder_session_token"
	// SessionCustomHeader is the custom header to use for authentication.
	SessionCustomHeader = "Coder-Session-Token"
	OAuth2StateKey      = "oauth_state"
	OAuth2RedirectKey   = "oauth_redirect"
)

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
	HTTPClient   *http.Client
	SessionToken string
	URL          *url.URL
}

type requestOption func(*http.Request)

// Request performs an HTTP request with the body provided.
// The caller is responsible for closing the response body.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}, opts ...requestOption) (*http.Response, error) {
	serverURL, err := c.URL.Parse(path)
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}

	var buf bytes.Buffer
	if body != nil {
		if data, ok := body.([]byte); ok {
			buf = *bytes.NewBuffer(data)
		} else {
			// Assume JSON if not bytes.
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			err = enc.Encode(body)
			if err != nil {
				return nil, xerrors.Errorf("encode body: %w", err)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, serverURL.String(), &buf)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	req.Header.Set(SessionCustomHeader, c.SessionToken)

	// Delete this custom cookie set in November 2022. This is just to remain
	// backwards compatible with older versions of Coder.
	req.AddCookie(&http.Cookie{
		Name:  "session_token",
		Value: c.SessionToken,
	})

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("do: %w", err)
	}
	return resp, err
}

// dialWebsocket opens a dialWebsocket connection on that path provided.
// The caller is responsible for closing the dialWebsocket.Conn.
func (c *Client) dialWebsocket(ctx context.Context, path string) (*websocket.Conn, error) {
	serverURL, err := c.URL.Parse(path)
	if err != nil {
		return nil, xerrors.Errorf("parse path: %w", err)
	}

	apiURL, err := url.Parse(serverURL.String())
	if err != nil {
		return nil, xerrors.Errorf("parse server url: %w", err)
	}
	apiURL.Scheme = "ws"
	if serverURL.Scheme == "https" {
		apiURL.Scheme = "wss"
	}
	apiURL.Path = path
	q := apiURL.Query()
	q.Add(SessionTokenKey, c.SessionToken)
	apiURL.RawQuery = q.Encode()

	//nolint:bodyclose
	conn, _, err := websocket.Dial(ctx, apiURL.String(), &websocket.DialOptions{
		HTTPClient: c.HTTPClient,
	})
	if err != nil {
		return nil, xerrors.Errorf("dial websocket: %w", err)
	}

	return conn, nil
}

// readBodyAsError reads the response as an .Message, and
// wraps it in a codersdk.Error type for easy marshaling.
func readBodyAsError(res *http.Response) error {
	contentType := res.Header.Get("Content-Type")

	var method, u string
	if res.Request != nil {
		method = res.Request.Method
		if res.Request.URL != nil {
			u = res.Request.URL.String()
		}
	}

	var helper string
	if res.StatusCode == http.StatusUnauthorized {
		// 401 means the user is not logged in
		// 403 would mean that the user is not authorized
		helper = "Try logging in using 'coder login <url>'."
	}

	if strings.HasPrefix(contentType, "text/plain") {
		resp, err := io.ReadAll(res.Body)
		if err != nil {
			return xerrors.Errorf("read body: %w", err)
		}
		return &Error{
			statusCode: res.StatusCode,
			Response: Response{
				Message: string(resp),
			},
			Helper: helper,
		}
	}

	//nolint:varnamelen
	var m Response
	err := json.NewDecoder(res.Body).Decode(&m)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// If no body is sent, we'll just provide the status code.
			return &Error{
				statusCode: res.StatusCode,
				Helper:     helper,
			}
		}
		return xerrors.Errorf("decode body: %w", err)
	}
	return &Error{
		Response:   m,
		statusCode: res.StatusCode,
		method:     method,
		url:        u,
		Helper:     helper,
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

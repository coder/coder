package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
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

// request performs an HTTP request with the body provided.
// The caller is responsible for closing the response body.
func (c *Client) request(ctx context.Context, method, path string, body interface{}, opts ...func(r *http.Request)) (*http.Response, error) {
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
	req.AddCookie(&http.Cookie{
		Name:  httpmw.AuthCookie,
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

// readBodyAsError reads the response as an httpapi.Message, and
// wraps it in a codersdk.Error type for easy marshaling.
func readBodyAsError(res *http.Response) error {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return xerrors.Errorf("read body: %w", err)
	}

	return &Error{
		Body:       body,
		statusCode: res.StatusCode,
	}
}

// Error represents an unaccepted or invalid request to the API.
// @typescript-ignore Error
type Error struct {
	Body []byte

	statusCode int
}

func (e *Error) StatusCode() int {
	return e.statusCode
}

func (e *Error) Error() string {
	var errMsg strings.Builder
	_, _ = fmt.Fprintf(&errMsg, "status code %d:", e.statusCode)

	//nolint:varnamelen
	var m httpapi.Response
	err := json.Unmarshal(e.Body, &m)
	if err != nil || len(m.Errors) == 0 {
		// We print the body instead of the parsed API errors in case another
		// component in the HTTP stack is giving the error.
		_, _ = fmt.Fprintf(&errMsg, "\n%s", bytes.TrimSpace(e.Body))
	} else {
		_, _ = fmt.Fprintf(&errMsg, " %v\n", m.Message)
		for _, err := range m.Errors {
			_, _ = fmt.Fprintf(&errMsg, "\n\t%s: %s", err.Field, err.Detail)
		}
	}
	return errMsg.String()
}

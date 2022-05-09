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
	contentType := res.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/plain") {
		resp, err := io.ReadAll(res.Body)
		if err != nil {
			return xerrors.Errorf("read body: %w", err)
		}
		return &Error{
			statusCode: res.StatusCode,
			Response: httpapi.Response{
				Message: string(resp),
			},
		}
	}

	//nolint:varnamelen
	var m httpapi.Response
	err := json.NewDecoder(res.Body).Decode(&m)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// If no body is sent, we'll just provide the status code.
			return &Error{
				statusCode: res.StatusCode,
			}
		}
		return xerrors.Errorf("decode body: %w", err)
	}
	return &Error{
		Response:   m,
		statusCode: res.StatusCode,
	}
}

// Error represents an unaccepted or invalid request to the API.
// @typescript-ignore Error
type Error struct {
	httpapi.Response

	statusCode int
}

func (e *Error) StatusCode() int {
	return e.statusCode
}

func (e *Error) Error() string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "status code %d: %s", e.statusCode, e.Message)
	for _, err := range e.Errors {
		_, _ = fmt.Fprintf(&builder, "\n\t%s: %s", err.Field, err.Detail)
	}
	return builder.String()
}

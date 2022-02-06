package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

// New creates a Coder client for the provided URL.
func New(serverURL *url.URL) *Client {
	return &Client{
		URL:        serverURL,
		httpClient: &http.Client{},
	}
}

// Client is an HTTP caller for methods to the Coder API.
type Client struct {
	URL *url.URL

	httpClient *http.Client
}

// SetSessionToken applies the provided token to the current client.
func (c *Client) SetSessionToken(token string) error {
	if c.httpClient.Jar == nil {
		var err error
		c.httpClient.Jar, err = cookiejar.New(nil)
		if err != nil {
			return err
		}
	}
	c.httpClient.Jar.SetCookies(c.URL, []*http.Cookie{{
		Name:  httpmw.AuthCookie,
		Value: token,
	}})
	return nil
}

// request performs an HTTP request with the body provided.
// The caller is responsible for closing the response body.
func (c *Client) request(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	serverURL, err := c.URL.Parse(path)
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}

	var buf bytes.Buffer
	if body != nil {
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		err = enc.Encode(body)
		if err != nil {
			return nil, xerrors.Errorf("encode body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, serverURL.String(), &buf)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("do: %w", err)
	}
	return resp, err
}

// readBodyAsError reads the response as an httpapi.Message, and
// wraps it in a codersdk.Error type for easy marshaling.
func readBodyAsError(res *http.Response) error {
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
		_, _ = fmt.Fprintf(&builder, "\n\t%s: %s", err.Field, err.Code)
	}
	return builder.String()
}

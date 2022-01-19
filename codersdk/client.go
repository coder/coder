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

	"golang.org/x/xerrors"

	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

type Options struct {
	SessionToken string
	HTTPClient   *http.Client
}

func New(url *url.URL, options *Options) *Client {
	if options == nil {
		options = &Options{}
	}
	if options.HTTPClient == nil {
		// Don't use http.DefaultClient because we override
		// all the cookies!
		options.HTTPClient = &http.Client{}
	}
	return &Client{
		url:          url,
		sessionToken: options.SessionToken,
		httpClient:   options.HTTPClient,
	}
}

type Client struct {
	url          *url.URL
	sessionToken string
	httpClient   *http.Client
}

func (c *Client) SessionToken() string {
	return c.sessionToken
}

func (c *Client) setSessionToken(token string) {
	if c.httpClient.Jar == nil {
		c.httpClient.Jar = &cookiejar.Jar{}
	}
	c.httpClient.Jar.SetCookies(c.url, []*http.Cookie{{
		Name:  httpmw.AuthCookie,
		Value: token,
	}})
	c.sessionToken = token
}

func (c *Client) request(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url, err := c.url.Parse(path)
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

	req, err := http.NewRequestWithContext(ctx, method, url.String(), &buf)
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
	for _, er := range m.Errors {
		fmt.Printf("WE GOT THIS: %q %q\n", er.Field, er.Code)
	}
	return &Error{
		Response:   m,
		statusCode: res.StatusCode,
	}
}

type Error struct {
	httpapi.Response

	statusCode int
}

func (e *Error) StatusCode() int {
	return e.statusCode
}

func (e *Error) Error() string {
	return fmt.Sprintf("")
}

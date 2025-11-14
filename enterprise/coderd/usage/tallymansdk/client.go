package tallymansdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

const (
	// DefaultURL is the default URL for the Tallyman API.
	DefaultURL = "https://tallyman-prod.coder.com"
)

// Client is a client for the Tallyman API.
type Client struct {
	// URL is the base URL for the Tallyman API.
	URL *url.URL
	// HTTPClient is the HTTP client to use for requests.
	HTTPClient *http.Client
	// licenseKey is the Coder license key for authentication.
	licenseKey string
	// deploymentID is the deployment ID for authentication.
	deploymentID uuid.UUID
}

// ClientOption is a functional option for configuring the Client.
type ClientOption func(*Client)

// New creates a new Tallyman API client.
func New(baseURL *url.URL, licenseKey string, deploymentID uuid.UUID, opts ...ClientOption) *Client {
	if baseURL == nil {
		baseURL, _ = url.Parse(DefaultURL)
	}

	c := &Client{
		URL:          baseURL,
		HTTPClient:   http.DefaultClient,
		licenseKey:   licenseKey,
		deploymentID: deploymentID,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithHTTPClient sets the HTTP client to use for requests.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.HTTPClient = httpClient
		}
	}
}

// Request makes an HTTP request to the Tallyman API.
// It sets the authentication headers and User-Agent automatically.
func (c *Client) Request(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, xerrors.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Build the full URL
	endpoint, err := url.Parse(path)
	if err != nil {
		return nil, xerrors.Errorf("parse path %q: %w", path, err)
	}
	fullURL := c.URL.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), bodyReader)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "coderd/"+buildinfo.Version())
	req.Header.Set(usagetypes.TallymanCoderLicenseKeyHeader, c.licenseKey)
	req.Header.Set(usagetypes.TallymanCoderDeploymentIDHeader, c.deploymentID.String())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("do request: %w", err)
	}

	return resp, nil
}

// readErrorResponse parses a Tallyman error response from an HTTP response body.
func readErrorResponse(resp *http.Response) error {
	var errBody usagetypes.TallymanV1Response
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		errBody = usagetypes.TallymanV1Response{
			Message: "could not decode error response body",
		}
	}
	return xerrors.Errorf("unexpected status code %v, error: %s", resp.StatusCode, errBody.Message)
}

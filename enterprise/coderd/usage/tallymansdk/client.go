package tallymansdk

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/usage/usagetypes"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

const (
	// DefaultURL is the default URL for the Tallyman API.
	DefaultURL = "https://tallyman-prod.coder.com"
)

var ErrNoLicenseSupportsPublishing = xerrors.New("usage publishing is not enabled by any license")

// NewOptions contains options for creating a new Tallyman client.
type NewOptions struct {
	// DB is the database store for querying licenses and deployment ID.
	DB database.Store
	// DeploymentID is the deployment ID. If uuid.Nil, it will be fetched from the database.
	DeploymentID uuid.UUID
	// LicenseKeys is a map of license keys for verifying license JWTs.
	LicenseKeys map[string]ed25519.PublicKey
	// BaseURL is the base URL for the Tallyman API. If nil, DefaultURL is used.
	BaseURL *url.URL
	// HTTPClient is the HTTP client to use for requests. If nil, http.DefaultClient is used.
	HTTPClient *http.Client
}

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

// NewWithAuth creates a new Tallyman API client with explicit authentication.
func NewWithAuth(baseURL *url.URL, licenseKey string, deploymentID uuid.UUID, opts ...ClientOption) *Client {
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

// New creates a new Tallyman API client by looking up the best license from the database.
// It selects the most recently issued license that:
// - Is unexpired
// - Has AccountType == AccountTypeSalesforce
// - Has PublishUsageData enabled
//
// If opts.DeploymentID is uuid.Nil, it will be fetched from the database.
// If no suitable license is found, it returns ErrNoLicenseSupportsPublishing.
func New(ctx context.Context, opts NewOptions) (*Client, error) {
	// Fetch deployment ID if not provided
	deploymentID := opts.DeploymentID
	if deploymentID == uuid.Nil {
		deploymentIDStr, err := opts.DB.GetDeploymentID(ctx)
		if err != nil {
			return nil, xerrors.Errorf("get deployment ID: %w", err)
		}
		deploymentID, err = uuid.Parse(deploymentIDStr)
		if err != nil {
			return nil, xerrors.Errorf("parse deployment ID %q: %w", deploymentIDStr, err)
		}
	}

	licenses, err := opts.DB.GetUnexpiredLicenses(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get unexpired licenses: %w", err)
	}
	if len(licenses) == 0 {
		return nil, ErrNoLicenseSupportsPublishing
	}

	type licenseJWTWithClaims struct {
		Claims *license.Claims
		Raw    string
	}

	var bestLicense licenseJWTWithClaims
	for _, dbLicense := range licenses {
		claims, err := license.ParseClaims(dbLicense.JWT, opts.LicenseKeys)
		if err != nil {
			// Skip licenses that can't be parsed
			continue
		}
		if claims.AccountType != license.AccountTypeSalesforce {
			// Non-Salesforce accounts cannot be tracked
			continue
		}
		if !claims.PublishUsageData {
			// Publishing is disabled
			continue
		}

		// Select the most recently issued license
		// IssuedAt is verified to be non-nil in license.ParseClaims
		if bestLicense.Claims == nil || claims.IssuedAt.Time.After(bestLicense.Claims.IssuedAt.Time) {
			bestLicense = licenseJWTWithClaims{
				Claims: claims,
				Raw:    dbLicense.JWT,
			}
		}
	}

	if bestLicense.Raw == "" {
		return nil, ErrNoLicenseSupportsPublishing
	}

	// Set default HTTP client if not provided
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return NewWithAuth(opts.BaseURL, bestLicense.Raw, deploymentID, WithHTTPClient(httpClient)), nil
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

package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/awssig"
)

// claudePlatformTransport authenticates both Messages SDK and passthrough
// requests after client-header sanitization and key selection.
type claudePlatformTransport struct {
	inner http.RoundTripper
	cfg   config.AWSClaudePlatform
	// creds is loaded lazily so BYOK and pooled requests do not consult AWS.
	creds aws.CredentialsProvider
}

var _ http.RoundTripper = (*claudePlatformTransport)(nil)

func newLazyAWSCredentials(region string) aws.CredentialsProvider {
	return aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		// AWS caches refresh credentials independently of any one caller's deadline,
		// so bound configuration loading as well as the request's wait below.
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cfg, err := buildAWSCredentials(ctx, awsCredentialSpec{Region: region})
		if err != nil {
			return aws.Credentials{}, err
		}
		return cfg.Credentials.Retrieve(ctx)
	}))
}

func (t *claudePlatformTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Keep signing headers private to this attempt so SDK retries are signed again.
	req = req.Clone(req.Context())
	// Set the required workspace header from provider
	// configuration, overwriting anything the client sent.
	req.Header.Set(intercept.HeaderAnthropicWorkspaceID, t.cfg.WorkspaceID)

	// A request that already carries a credential is BYOK or a centralized key
	// selected before this transport runs. Leave it alone: signing on
	// top would produce two credentials on one request.
	if req.Header.Get(intercept.AuthHeaderXAPIKey) != "" ||
		req.Header.Get(intercept.AuthHeaderAuthorization) != "" {
		return t.inner.RoundTrip(req)
	}

	// Bound credential retrieval without imposing a deadline on the response stream.
	creds := aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return t.creds.Retrieve(ctx)
	})
	//nolint:bodyclose // the response is returned to the caller, which closes it.
	resp, err := awssig.SignMiddleware(creds, t.cfg.Region, config.ClaudePlatformSigningService)(
		req,
		t.inner.RoundTrip,
	)
	if err != nil && req.Body != nil {
		_ = req.Body.Close()
	}
	return resp, err
}

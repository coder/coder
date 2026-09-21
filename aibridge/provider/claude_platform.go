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
	// creds is nil in api_key mode, where the key pool supplies x-api-key.
	creds aws.CredentialsProvider
}

var _ http.RoundTripper = &claudePlatformTransport{}

func (t *claudePlatformTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Keep signing headers private to this attempt so SDK retries are signed again.
	req = req.Clone(req.Context())
	// The workspace header is required in both auth modes. Set it from provider
	// configuration, overwriting anything the client sent.
	req.Header.Set(intercept.HeaderAnthropicWorkspaceID, t.cfg.WorkspaceID)

	// A request that already carries a credential is BYOK or a centralized key
	// selected before this transport runs. Leave it alone: signing on
	// top would produce two credentials on one request.
	if req.Header.Get(intercept.AuthHeaderXAPIKey) != "" ||
		req.Header.Get(intercept.AuthHeaderAuthorization) != "" {
		return t.inner.RoundTrip(req)
	}

	if t.creds == nil {
		// api_key mode with no key available. Forward unsigned and let the
		// upstream reject it, matching how other providers surface a missing
		// centralized key on passthrough routes.
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

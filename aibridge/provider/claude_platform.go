package provider

import (
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/awssig"
)

// claudePlatformPassthroughTransport authenticates non-bridged Claude Platform
// routes (/v1/models, /v1/messages/count_tokens, /api/event_logging/).
//
// Bridged requests are authenticated by SDK request options, but passthrough
// requests bypass the SDK entirely and are otherwise only authenticated by the
// key pool. Without this, an IAM-mode provider would send unsigned passthrough
// requests and the upstream would reject them.
type claudePlatformPassthroughTransport struct {
	inner http.RoundTripper
	cfg   config.AWSClaudePlatform
	// creds is nil in api_key mode, where the key pool supplies x-api-key.
	creds aws.CredentialsProvider
}

var _ http.RoundTripper = &claudePlatformPassthroughTransport{}

func (t *claudePlatformPassthroughTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The workspace header is required in both auth modes. Set it from provider
	// configuration, overwriting anything the client sent.
	req.Header.Set(intercept.HeaderAnthropicWorkspaceID, t.cfg.WorkspaceID)

	// A request that already carries a credential is BYOK or a centralized key
	// selected by the failover transport above us. Leave it alone: signing on
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

	//nolint:bodyclose // the response is returned to the caller, which closes it.
	return awssig.SignMiddleware(t.creds, t.cfg.Region, config.ClaudePlatformSigningService)(
		req,
		func(r *http.Request) (*http.Response, error) {
			return t.inner.RoundTrip(r)
		},
	)
}

package provider

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
)

// bedrockSessionName is the STS role session name attached to AssumeRole calls.
// A stable value keeps them identifiable in CloudTrail.
const bedrockSessionName = "coder-aigateway"

// buildBedrockCredentials resolves the base identity and, when a role ARN
// is configured, assumes that role via STS. The base identity is either
// static keys or the AWS SDK default credential chain, which covers IRSA,
// EKS Pod Identity, EC2 Instance Profile, and more.
//
// The result is wrapped in aws.NewCredentialsCache, which caches and rotates
// the resolved temporary credentials. buildBedrockCredentials should be called
// once when the Bedrock provider is constructed, and the returned Credential
// Provider should be shared across all LLM requests to the Bedrock Provider,
// so per-request credential retrieval is served from this cache rather than
// re-resolving (and re-assuming) on every request. No network call is made here:
// the base identity and any AssumeRole are resolved lazily on first retrieval.
func buildBedrockCredentials(ctx context.Context, cfg config.AWSBedrock) (aws.CredentialsProvider, string, error) {
	if cfg.Region == "" && cfg.BaseURL == "" {
		return nil, "", xerrors.New("region or base url required")
	}

	var loadOpts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))
	}

	// Use static credentials when explicitly provided, otherwise fall back to
	// the SDK default credential chain.
	switch {
	// Both set: use static credentials directly.
	case cfg.AccessKey != "" && cfg.AccessKeySecret != "":
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKey,
				cfg.AccessKeySecret,
				"",
			),
		))
	// Only one set: misconfiguration.
	case cfg.AccessKey != "" || cfg.AccessKeySecret != "":
		return nil, "", xerrors.New("both access key and access key secret must be provided together")
	// Neither set: SDK default credential chain resolves the base identity.
	default:
	}

	base, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, "", xerrors.Errorf("failed to load AWS Bedrock config: %w", err)
	}

	// Assuming a role calls STS, which needs a region to resolve its endpoint.
	// The region may come from the config or the AWS environment; if neither
	// supplies one, fail here.
	if cfg.RoleARN != "" && base.Region == "" {
		return nil, "", xerrors.New("region is required to assume a role, but was not specified")
	}

	// The base identity signs Bedrock requests directly unless a target role is
	// configured, in which case it signs the AssumeRole call and the resulting
	// temporary credentials sign Bedrock requests. The default credential chain
	// is already cache-wrapped, so only the AssumeRoleProvider is wrapped with a
	// cache to avoid re-assuming the role on every request.
	credsProvider := base.Credentials
	if cfg.RoleARN != "" {
		// Disable keep-alive on the STS client so each AssumeRole opens a
		// fresh connection. Observed: with keep-alive, AssumeRole calls reuse
		// one connection pinned to a single STS endpoint, and after a
		// trust-policy change that connection kept returning AccessDenied for
		// minutes while a fresh connection (e.g. the AWS CLI) accepted the
		// identical request at once; the gateway recovered only when that
		// connection recycled or the process restarted. The STS-internal reason is
		// unconfirmed (likely per-endpoint propagation of the change); what we
		// verified is that a fresh connection per call recovers in seconds
		// instead of minutes. AssumeRole runs at most once per credential-cache
		// lifetime, so keep-alive saves nothing here. Scoped to the STS client
		// only; Bedrock requests use a separate client and keep pooling.
		stsClient := sts.NewFromConfig(base, func(o *sts.Options) {
			o.HTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
				t.DisableKeepAlives = true
			})
		})
		credsProvider = stscreds.NewAssumeRoleProvider(stsClient, cfg.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = bedrockSessionName
			if cfg.ExternalID != "" {
				o.ExternalID = aws.String(cfg.ExternalID)
			}
		})
		credsProvider = aws.NewCredentialsCache(credsProvider)
	}

	// base.Region is the region the SDK resolved (explicit config, AWS_REGION /
	// AWS_DEFAULT_REGION, shared config, or IMDS).
	return credsProvider, base.Region, nil
}

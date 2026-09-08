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

// BedrockIdentity is the subset of Bedrock provider settings that determines
// which AWS identity signs a request. It carries no endpoint or protocol
// choice, so it serves the data plane and the control plane equally.
type BedrockIdentity struct {
	// Region resolves the endpoint and signs requests. It may be empty only
	// when the caller supplies an endpoint of its own, in which case the AWS
	// environment must supply the region.
	Region string
	// AccessKey and AccessKeySecret select static credentials. When either is
	// empty the AWS SDK default credential chain resolves the base identity.
	AccessKey       string
	AccessKeySecret string
	// RoleARN, when set, is assumed via STS on top of the base identity.
	RoleARN string
	// ExternalID is sent as the STS external ID on the AssumeRole call.
	ExternalID string
}

func bedrockIdentity(cfg config.AWSBedrock) BedrockIdentity {
	return BedrockIdentity{
		Region:          cfg.Region,
		AccessKey:       cfg.AccessKey,
		AccessKeySecret: cfg.AccessKeySecret,
		RoleARN:         cfg.RoleARN,
		ExternalID:      cfg.ExternalID,
	}
}

// BuildBedrockCredentials resolves the base identity and, when a role ARN
// is configured, assumes that role via STS. The base identity is either
// static keys or the AWS SDK default credential chain, which covers IRSA,
// EKS Pod Identity, EC2 Instance Profile, and more.
//
// It returns the loaded AWS config with the resolved credentials attached, so
// callers that need an AWS client reuse the same environment-derived settings
// and identity rather than assembling their own.
//
// The credentials are wrapped in aws.NewCredentialsCache, which caches and
// rotates the resolved temporary credentials. BuildBedrockCredentials should be
// called once when the Bedrock provider is constructed, and the returned
// Credential Provider should be shared across all LLM requests to the Bedrock
// Provider, so per-request credential retrieval is served from this cache
// rather than re-resolving (and re-assuming) on every request. No network call
// is made here: the base identity and any AssumeRole are resolved lazily on
// first retrieval.
func BuildBedrockCredentials(ctx context.Context, id BedrockIdentity) (aws.Config, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error
	if id.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(id.Region))
	}

	// Use static credentials when explicitly provided, otherwise fall back to
	// the SDK default credential chain.
	switch {
	// Both set: use static credentials directly.
	case id.AccessKey != "" && id.AccessKeySecret != "":
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				id.AccessKey,
				id.AccessKeySecret,
				"",
			),
		))
	// Only one set: misconfiguration.
	case id.AccessKey != "" || id.AccessKeySecret != "":
		return aws.Config{}, xerrors.New("both access key and access key secret must be provided together")
	// Neither set: SDK default credential chain resolves the base identity.
	default:
	}

	base, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return aws.Config{}, xerrors.Errorf("failed to load AWS Bedrock config: %w", err)
	}

	// Assuming a role calls STS, which needs a region to resolve its endpoint.
	// The region may come from the config or the AWS environment; if neither
	// supplies one, fail here.
	if id.RoleARN != "" && base.Region == "" {
		return aws.Config{}, xerrors.New("region is required to assume a role, but was not specified")
	}

	// The base identity signs Bedrock requests directly unless a target role is
	// configured, in which case it signs the AssumeRole call and the resulting
	// temporary credentials sign Bedrock requests. The default credential chain
	// is already cache-wrapped, so only the AssumeRoleProvider is wrapped with a
	// cache to avoid re-assuming the role on every request.
	credsProvider := base.Credentials
	if id.RoleARN != "" {
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
		credsProvider = stscreds.NewAssumeRoleProvider(stsClient, id.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = bedrockSessionName
			if id.ExternalID != "" {
				o.ExternalID = aws.String(id.ExternalID)
			}
		})
		credsProvider = aws.NewCredentialsCache(credsProvider)
	}

	// base.Region is the region the SDK resolved (explicit config, AWS_REGION /
	// AWS_DEFAULT_REGION, shared config, or IMDS).
	base.Credentials = credsProvider
	return base, nil
}

// bedrockRuntimeCredentials is [BuildBedrockCredentials] for a full provider
// configuration, rejecting a config that gives the runtime no endpoint at all.
func bedrockRuntimeCredentials(ctx context.Context, cfg config.AWSBedrock) (aws.Config, error) {
	if cfg.Region == "" && cfg.BaseURL == "" {
		return aws.Config{}, xerrors.New("region or base url required")
	}
	return BuildBedrockCredentials(ctx, bedrockIdentity(cfg))
}

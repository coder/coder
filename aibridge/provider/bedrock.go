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

// awsSessionName is the STS role session name attached to AssumeRole calls.
// A stable value keeps them identifiable in CloudTrail.
const awsSessionName = "coder-aigateway"

// awsCredentialSpec is the identity configuration shared by every AWS-signed
// upstream. It is the subset of a provider's settings that determines which
// AWS principal signs the request, independent of which service is being
// called.
type awsCredentialSpec struct {
	// Region is the AWS region. It may be empty when BaseURL is set or when
	// the AWS environment supplies one, but is required to assume a role.
	Region string
	// AccessKey and AccessKeySecret select static credentials. Both must be
	// set, or neither.
	AccessKey, AccessKeySecret string
	// RoleARN, when set, is assumed via STS. The base identity signs the
	// AssumeRole call and the resulting temporary credentials sign requests.
	RoleARN string
	// ExternalID is sent as the STS external ID on the AssumeRole call. It is
	// meaningful only alongside RoleARN.
	ExternalID string
}

// awsCredentialSpecFromBedrock projects Bedrock settings onto the shared spec.
func awsCredentialSpecFromBedrock(cfg config.AWSBedrock) awsCredentialSpec {
	return awsCredentialSpec{
		Region:          cfg.Region,
		AccessKey:       cfg.AccessKey,
		AccessKeySecret: cfg.AccessKeySecret,
		RoleARN:         cfg.RoleARN,
		ExternalID:      cfg.ExternalID,
	}
}

// buildAWSCredentials resolves the base identity and, when a role ARN
// is configured, assumes that role via STS. The base identity is either
// static keys or the AWS SDK default credential chain, which covers IRSA,
// EKS Pod Identity, EC2 Instance Profile, and more.
//
// The result is wrapped in aws.NewCredentialsCache, which caches and rotates
// the resolved temporary credentials. buildAWSCredentials should be called
// once when the provider is constructed, and the returned Credential Provider
// should be shared across all requests to that provider, so per-request
// credential retrieval is served from this cache rather than re-resolving (and
// re-assuming) on every request. No network call is made here: the base
// identity and any AssumeRole are resolved lazily on first retrieval.
//
// Whether a region is mandatory depends on how the caller builds its endpoint,
// so that policy belongs to the caller; this function only requires a region
// when one is needed to assume a role.
func buildAWSCredentials(ctx context.Context, spec awsCredentialSpec) (aws.CredentialsProvider, string, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error
	if spec.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(spec.Region))
	}

	// Use static credentials when explicitly provided, otherwise fall back to
	// the SDK default credential chain.
	switch {
	// Both set: use static credentials directly.
	case spec.AccessKey != "" && spec.AccessKeySecret != "":
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				spec.AccessKey,
				spec.AccessKeySecret,
				"",
			),
		))
	// Only one set: misconfiguration.
	case spec.AccessKey != "" || spec.AccessKeySecret != "":
		return nil, "", xerrors.New("both access key and access key secret must be provided together")
	// Neither set: SDK default credential chain resolves the base identity.
	default:
	}

	base, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, "", xerrors.Errorf("failed to load AWS config: %w", err)
	}

	// Assuming a role calls STS, which needs a region to resolve its endpoint.
	// The region may come from the config or the AWS environment; if neither
	// supplies one, fail here.
	if spec.RoleARN != "" && base.Region == "" {
		return nil, "", xerrors.New("region is required to assume a role, but was not specified")
	}

	// The base identity signs requests directly unless a target role is
	// configured, in which case it signs the AssumeRole call and the resulting
	// temporary credentials sign requests. The default credential chain
	// is already cache-wrapped, so only the AssumeRoleProvider is wrapped with a
	// cache to avoid re-assuming the role on every request.
	credsProvider := base.Credentials
	if spec.RoleARN != "" {
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
		// only; upstream requests use a separate client and keep pooling.
		stsClient := sts.NewFromConfig(base, func(o *sts.Options) {
			o.HTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(t *http.Transport) {
				t.DisableKeepAlives = true
			})
		})
		credsProvider = stscreds.NewAssumeRoleProvider(stsClient, spec.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = awsSessionName
			if spec.ExternalID != "" {
				o.ExternalID = aws.String(spec.ExternalID)
			}
		})
		credsProvider = aws.NewCredentialsCache(credsProvider)
	}

	// base.Region is the region the SDK resolved (explicit config, AWS_REGION /
	// AWS_DEFAULT_REGION, shared config, or IMDS).
	return credsProvider, base.Region, nil
}

// buildBedrockCredentials resolves the AWS credentials used to sign Bedrock
// requests. Bedrock accepts an explicit base URL in place of a region, so
// either one satisfies the endpoint requirement.
func buildBedrockCredentials(ctx context.Context, cfg config.AWSBedrock) (aws.CredentialsProvider, string, error) {
	if cfg.Region == "" && cfg.BaseURL == "" {
		return nil, "", xerrors.New("region or base url required")
	}
	return buildAWSCredentials(ctx, awsCredentialSpecFromBedrock(cfg))
}

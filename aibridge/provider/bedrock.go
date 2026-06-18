package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
)

// defaultBedrockSessionName is the STS role session name used when the provider
// does not configure one. A stable value keeps AssumeRole calls identifiable in
// CloudTrail.
const defaultBedrockSessionName = "coder-aibridge"

// buildBedrockCredentials resolves the base identity (static keys or the AWS SDK
// default credential chain, which covers IRSA, Pod Identity, instance profile,
// shared profile, and environment variables) and, when a target role ARN is
// configured, assumes that role via STS.
//
// The result is wrapped in aws.NewCredentialsCache, which caches and rotates the
// resolved temporary credentials. The provider is resolved once per Bedrock
// provider (at construction) and shared across requests, so per-request
// credential retrieval is served from this cache rather than re-resolving (and
// re-assuming) on every request. No network call is made here: the base
// identity and any AssumeRole are resolved lazily on first retrieval.
func buildBedrockCredentials(ctx context.Context, cfg config.AWSBedrock) (aws.CredentialsProvider, error) {
	if cfg.Region == "" && cfg.BaseURL == "" {
		return nil, xerrors.New("region or base url required")
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
		return nil, xerrors.New("both access key and access key secret must be provided together")
	// Neither set: SDK default credential chain resolves the base identity.
	default:
	}

	base, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, xerrors.Errorf("failed to load AWS Bedrock config: %w", err)
	}

	// The base identity signs requests directly unless a target role is
	// configured, in which case it signs the AssumeRole call and the resulting
	// temporary credentials sign Bedrock requests.
	credsProvider := base.Credentials
	if cfg.RoleARN != "" {
		sessionName := cfg.SessionName
		if sessionName == "" {
			sessionName = defaultBedrockSessionName
		}
		credsProvider = stscreds.NewAssumeRoleProvider(sts.NewFromConfig(base), cfg.RoleARN, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = sessionName
			if cfg.ExternalID != "" {
				o.ExternalID = aws.String(cfg.ExternalID)
			}
		})
	}

	return aws.NewCredentialsCache(credsProvider), nil
}

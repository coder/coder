package provider

import (
	"context"

	anthropicaws "github.com/anthropics/anthropic-sdk-go/aws"
	"github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
)

// buildClaudePlatformAWSOptions resolves the SDK request options for a Claude
// Platform for AWS provider once at construction time. The returned options
// carry the regional base URL, the anthropic-workspace-id header, and either
// the SigV4 signing middleware (service aws-external-anthropic) or the
// x-api-key auth, depending on the configured credentials.
//
// Like the Bedrock path, no network call is made here: the base identity and
// any AssumeRole are resolved lazily on first retrieval. The resulting options
// are reused across all requests, so per-request credential retrieval is served
// from the SDK's internal credentials cache rather than re-resolving (and
// re-assuming) on every request.
func buildClaudePlatformAWSOptions(ctx context.Context, cfg config.AWSClaudePlatform) ([]option.RequestOption, error) {
	if cfg.WorkspaceID == "" {
		return nil, xerrors.New("workspace id required")
	}
	if cfg.Region == "" && cfg.BaseURL == "" {
		return nil, xerrors.New("region or base url required")
	}
	if (cfg.AccessKey == "") != (cfg.AccessKeySecret == "") {
		return nil, xerrors.New("both access key and access key secret must be provided together")
	}

	client, err := anthropicaws.NewClient(ctx, anthropicaws.ClientConfig{
		APIKey:             cfg.APIKey,
		AWSAccessKey:       cfg.AccessKey,
		AWSSecretAccessKey: cfg.AccessKeySecret,
		AWSRoleARN:         cfg.RoleARN,
		AWSExternalID:      cfg.ExternalID,
		AWSRegion:          cfg.Region,
		WorkspaceID:        cfg.WorkspaceID,
		BaseURL:            cfg.BaseURL,
	})
	if err != nil {
		return nil, xerrors.Errorf("build claude platform for aws client: %w", err)
	}

	return client.Options, nil
}

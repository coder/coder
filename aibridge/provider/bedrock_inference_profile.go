package provider

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/config"
)

// bedrockService is the ARN service namespace for Amazon Bedrock resources.
const bedrockService = "bedrock"

// applicationInferenceProfileResourceType is the ARN resource type of an
// application inference profile, the AWS-native mechanism for attributing
// Bedrock spend to a team or workload via cost allocation tags.
const applicationInferenceProfileResourceType = "application-inference-profile"

// inferenceProfileResolver resolves an application inference profile ARN to the
// Bedrock model ID it wraps.
type inferenceProfileResolver func(ctx context.Context, cfg config.AWSBedrock, creds aws.CredentialsProvider, profileARN string) (string, error)

// inferenceProfileResolutionTimeout bounds the Bedrock control-plane calls made
// while constructing a provider, which also cover the first credential
// resolution (STS/IRSA).
const inferenceProfileResolutionTimeout = 30 * time.Second

// isApplicationInferenceProfileARN reports whether model is an application
// inference profile ARN.
//
// Plain model IDs and system-defined inference profile ARNs both carry the
// model ID in the identifier itself, so only application inference profiles,
// whose identifier is opaque, need resolving through AWS.
func isApplicationInferenceProfileARN(model string) bool {
	parsed, err := arn.Parse(model)
	if err != nil || parsed.Service != bedrockService {
		return false
	}
	resourceType, _, ok := strings.Cut(parsed.Resource, "/")
	return ok && resourceType == applicationInferenceProfileResourceType
}

// resolveInferenceProfile returns the Bedrock model ID behind an application
// inference profile ARN.
//
// The caller's credentials sign the call, so the required
// bedrock:GetInferenceProfile permission belongs to the identity that already
// invokes Bedrock, including any role assumed via config.AWSBedrock.RoleARN.
func resolveInferenceProfile(ctx context.Context, cfg config.AWSBedrock, creds aws.CredentialsProvider, profileARN string) (string, error) {
	client := bedrock.NewFromConfig(aws.Config{
		Region:      cfg.Region,
		Credentials: creds,
	})

	out, err := client.GetInferenceProfile(ctx, &bedrock.GetInferenceProfileInput{
		InferenceProfileIdentifier: aws.String(profileARN),
	})
	if err != nil {
		return "", xerrors.Errorf("get inference profile %q (requires the %s:GetInferenceProfile permission): %w", profileARN, bedrockService, err)
	}
	if len(out.Models) == 0 || out.Models[0].ModelArn == nil {
		return "", xerrors.Errorf("inference profile %q references no model", profileARN)
	}

	modelARN := *out.Models[0].ModelArn
	model, err := modelIDFromARN(modelARN)
	if err != nil {
		return "", xerrors.Errorf("inference profile %q: %w", profileARN, err)
	}
	return model, nil
}

// modelIDFromARN extracts the model ID from the ARN an inference profile
// points at. The ARN is either a foundation model
// (arn:aws:bedrock:{region}::foundation-model/{model}) or a system-defined
// inference profile (arn:aws:bedrock:{region}:{account}:inference-profile/{model}),
// and both carry the model ID as the resource identifier.
func modelIDFromARN(modelARN string) (string, error) {
	parsed, err := arn.Parse(modelARN)
	if err != nil {
		return "", xerrors.Errorf("parse model arn %q: %w", modelARN, err)
	}
	_, model, ok := strings.Cut(parsed.Resource, "/")
	if !ok || model == "" {
		return "", xerrors.Errorf("model arn %q has no model identifier", modelARN)
	}
	return model, nil
}

// resolveBedrockModels resolves the configured model identifiers to the model
// IDs used for capability detection, usage recording, and pricing. Identifiers
// that are not application inference profile ARNs are returned unchanged and
// cost no AWS call.
func resolveBedrockModels(ctx context.Context, cfg config.AWSBedrock, creds aws.CredentialsProvider, resolve inferenceProfileResolver) (model, smallFastModel string, err error) {
	resolveOne := func(configured string) (string, error) {
		if !isApplicationInferenceProfileARN(configured) {
			return configured, nil
		}
		return resolve(ctx, cfg, creds, configured)
	}

	model, err = resolveOne(cfg.Model)
	if err != nil {
		return "", "", xerrors.Errorf("resolve model: %w", err)
	}
	smallFastModel, err = resolveOne(cfg.SmallFastModel)
	if err != nil {
		return "", "", xerrors.Errorf("resolve small fast model: %w", err)
	}
	return model, smallFastModel, nil
}

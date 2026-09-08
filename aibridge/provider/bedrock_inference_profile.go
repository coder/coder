package provider

import (
	"context"
	"strings"

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

// IsApplicationInferenceProfileARN reports whether model is an application
// inference profile ARN, whose identifier is opaque and must be resolved
// through AWS. Plain model IDs and system-defined inference profile ARNs, which
// AWS documents as {geoRegion}.{modelId}, embed the model ID and need no
// lookup.
func IsApplicationInferenceProfileARN(model string) bool {
	parsed, err := arn.Parse(model)
	if err != nil || parsed.Service != bedrockService {
		return false
	}
	resourceType, _, ok := strings.Cut(parsed.Resource, "/")
	return ok && resourceType == applicationInferenceProfileResourceType
}

// ResolveInferenceProfile returns the Bedrock model ID behind an application
// inference profile ARN.
//
// awsCfg carries the identity that invokes Bedrock, including any role assumed
// via config.AWSBedrock.RoleARN, so the required bedrock:GetInferenceProfile
// permission belongs to that identity.
//
// A profile that wraps a cross-region system-defined profile lists one model
// per region. Those entries differ only in the ARN region, which the model ID
// does not carry, so any entry resolves to the same model.
func ResolveInferenceProfile(ctx context.Context, awsCfg aws.Config, profileARN string) (string, error) {
	client := bedrock.NewFromConfig(awsCfg)

	out, err := client.GetInferenceProfile(ctx, &bedrock.GetInferenceProfileInput{
		InferenceProfileIdentifier: aws.String(profileARN),
	})
	if err != nil {
		return "", xerrors.Errorf("get inference profile %q: %w", profileARN, err)
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

// resolvedBedrockModels returns the model identities to serve with. A
// configured identifier that needs no resolution is its own identity; an
// application inference profile ARN requires the resolution stored with the
// provider.
func resolvedBedrockModels(cfg config.AWSBedrock) (model, smallFastModel string, err error) {
	identity := func(configured, resolved, field string) (string, error) {
		if !IsApplicationInferenceProfileARN(configured) {
			return configured, nil
		}
		if resolved == "" {
			return "", xerrors.Errorf("%s %q is an application inference profile with no resolved model; re-save the provider to resolve it", field, configured)
		}
		return resolved, nil
	}

	model, err = identity(cfg.Model, cfg.ResolvedModel, "model")
	if err != nil {
		return "", "", err
	}
	smallFastModel, err = identity(cfg.SmallFastModel, cfg.ResolvedSmallFastModel, "small fast model")
	if err != nil {
		return "", "", err
	}
	return model, smallFastModel, nil
}

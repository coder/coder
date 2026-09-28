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

// inferenceProfileResolutionTimeout bounds the Bedrock control-plane calls made
// while constructing a provider, which also cover the first credential
// resolution (STS/IRSA).
const inferenceProfileResolutionTimeout = 30 * time.Second

// isApplicationInferenceProfileARN reports whether model is an application
// inference profile ARN, whose identifier is opaque and must be resolved
// through AWS. Plain model IDs and system-defined inference profile ARNs, which
// AWS documents as {geoRegion}.{modelId}, embed the model ID and need no
// lookup.
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
// awsCfg carries the identity that invokes Bedrock, including any role assumed
// via config.AWSBedrock.RoleARN, so the required bedrock:GetInferenceProfile
// permission belongs to that identity.
//
// A profile that wraps a cross-region system-defined profile lists one model
// per region. Those entries differ only in the ARN region, which the model ID
// does not carry, so any entry resolves to the same model.
func resolveInferenceProfile(ctx context.Context, awsCfg aws.Config, profileARN string) (string, error) {
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

// ResolveBedrockModels resolves the application inference profile ARNs among
// the configured model identifiers, returning what each ARN refers to. The
// result is empty when neither identifier is an ARN, which costs no AWS call.
//
// The identity comes from cfg, including any role assumed via config.AWSBedrock.RoleARN,
// so the required bedrock:GetInferenceProfile permission belongs to that identity.
func ResolveBedrockModels(ctx context.Context, cfg config.AWSBedrock) (map[string]string, error) {
	resolved := make(map[string]string, 2)

	var profiles []string
	for _, configured := range []string{cfg.Model, cfg.SmallFastModel} {
		if isApplicationInferenceProfileARN(configured) {
			profiles = append(profiles, configured)
		}
	}
	if len(profiles) == 0 {
		return resolved, nil
	}

	awsCfg, err := buildBedrockCredentials(ctx, cfg)
	if err != nil {
		return nil, xerrors.Errorf("build bedrock credentials: %w", err)
	}

	resolveCtx, cancel := context.WithTimeout(ctx, inferenceProfileResolutionTimeout)
	defer cancel()

	for _, profileARN := range profiles {
		if _, ok := resolved[profileARN]; ok {
			continue
		}
		model, err := resolveInferenceProfile(resolveCtx, awsCfg, profileARN)
		if err != nil {
			return nil, err
		}
		resolved[profileARN] = model
	}
	return resolved, nil
}

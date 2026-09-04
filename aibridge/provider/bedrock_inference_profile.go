package provider

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"
)

// bedrockService is the ARN service namespace for Amazon Bedrock resources.
const bedrockService = "bedrock"

// applicationInferenceProfileResourceType is the ARN resource type of an
// application inference profile, the AWS-native mechanism for attributing
// Bedrock spend to a team or workload via cost allocation tags.
const applicationInferenceProfileResourceType = "application-inference-profile"

// inferenceProfileResolutionTimeout bounds a single Bedrock control-plane
// lookup, which also covers the first credential resolution (STS/IRSA). The
// request context still applies, so a client that gives up earlier cancels the
// lookup.
const inferenceProfileResolutionTimeout = 30 * time.Second

// InferenceProfileCache resolves configured Bedrock model identifiers to the
// model IDs used for capability detection, usage recording, and pricing.
//
// Application inference profile ARNs are opaque and cost one AWS lookup each;
// every other identifier is returned unchanged without calling AWS. Successful
// lookups are cached for the process lifetime: Bedrock has no
// UpdateInferenceProfile, so the model behind a profile is fixed when the
// profile is created, and pointing at a different model means a new ARN.
// Failures are not cached, so a transient one is retried on the next request.
//
// A cache outlives the providers that use it, so a provider reload does not
// discard resolutions.
type InferenceProfileCache struct {
	resolutions singleflight.Group

	mu     sync.RWMutex
	models map[string]string
}

// NewInferenceProfileCache returns a cache ready for use.
func NewInferenceProfileCache() *InferenceProfileCache {
	return &InferenceProfileCache{models: make(map[string]string)}
}

// Resolve returns the model ID behind configured. Concurrent resolutions of the
// same identifier share a single AWS lookup.
//
// awsCfg carries the identity that invokes Bedrock, including any role assumed
// via config.AWSBedrock.RoleARN, so the required bedrock:GetInferenceProfile
// permission belongs to that identity.
func (c *InferenceProfileCache) Resolve(ctx context.Context, awsCfg aws.Config, configured string) (string, error) {
	if !isApplicationInferenceProfileARN(configured) {
		return configured, nil
	}

	c.mu.RLock()
	model, ok := c.models[configured]
	c.mu.RUnlock()
	if ok {
		return model, nil
	}

	resolved, err, _ := c.resolutions.Do(configured, func() (any, error) {
		resolveCtx, cancel := context.WithTimeout(ctx, inferenceProfileResolutionTimeout)
		defer cancel()

		model, err := resolveInferenceProfile(resolveCtx, awsCfg, configured)
		if err != nil {
			return "", err
		}

		c.mu.Lock()
		c.models[configured] = model
		c.mu.Unlock()
		return model, nil
	})
	if err != nil {
		return "", err
	}
	model, _ = resolved.(string)
	return model, nil
}

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

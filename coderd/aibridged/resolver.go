package aibridged

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// PipelineVersionResolver resolves and compiles the pipeline snapshot for a
// specific (typically unpromoted) pipeline version, for owner-only
// version-targeted evaluation (§10.9). Unlike the active snapshot, which is
// rebuilt wholesale on every reload, versions are immutable, so a compiled
// snapshot is cached by pipeline-version id for the daemon's lifetime. No
// eviction at the expected 3-5 pipeline scale.
//
// It implements [aibridge.PipelineResolver].
type PipelineVersionResolver struct {
	db     database.Store
	logger slog.Logger

	cache        sync.Map // versionID string -> aibridge.ProviderPipelines
	singleflight singleflight.Group[string, aibridge.ProviderPipelines]
}

var _ aibridge.PipelineResolver = (*PipelineVersionResolver)(nil)

// NewPipelineVersionResolver constructs a resolver over db.
func NewPipelineVersionResolver(db database.Store, logger slog.Logger) *PipelineVersionResolver {
	return &PipelineVersionResolver{db: db, logger: logger}
}

// ResolvePipelineVersion returns the compiled pipelines for the given pipeline
// version, addressed by its logical version number (the value carried by the
// X-Coder-AI-Gateway-Pipeline-Version header), which must belong to the named
// provider's pipeline. It returns [aibridge.ErrPipelineVersionNotFound] when the
// version does not exist, its pipeline/provider is soft-deleted, or it belongs
// to a different provider (a foreign version), all of which the gate maps to a
// 4xx. The compiled result is cached by provider and version number; concurrent
// first-uses share one compile.
func (r *PipelineVersionResolver) ResolvePipelineVersion(ctx context.Context, provider, version string) (aibridge.ProviderPipelines, error) {
	// Accept both the bare number ("3") and the UI's "vN" label ("v3"): the
	// version history renders versions as "v3", so an operator copying that
	// label into the header should work without manually stripping the prefix.
	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")
	trimmed = strings.TrimPrefix(trimmed, "V")
	number, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		// A non-numeric header cannot name any version number.
		return aibridge.ProviderPipelines{}, aibridge.ErrPipelineVersionNotFound
	}

	// The cache key includes the provider so a foreign-version rejection for one
	// provider can never serve another provider's cached snapshot.
	cacheKey := provider + "|" + strconv.FormatInt(number, 10)
	if v, ok := r.cache.Load(cacheKey); ok {
		return v.(aibridge.ProviderPipelines), nil
	}

	pp, err, _ := r.singleflight.Do(cacheKey, func() (aibridge.ProviderPipelines, error) {
		if v, ok := r.cache.Load(cacheKey); ok {
			return v.(aibridge.ProviderPipelines), nil
		}
		id, err := r.resolveVersionID(ctx, provider, int32(number))
		if err != nil {
			return aibridge.ProviderPipelines{}, err
		}
		built, err := r.build(ctx, provider, id)
		if err != nil {
			return aibridge.ProviderPipelines{}, err
		}
		r.cache.Store(cacheKey, built)
		return built, nil
	})
	return pp, err
}

// resolveVersionID translates a (provider, logical version number) pair to the
// pipeline version's uuid. The header addresses versions by their human-facing
// number; the snapshot build still keys on the immutable uuid. The query is
// provider-scoped, so a number that exists only under another provider's
// pipeline returns [aibridge.ErrPipelineVersionNotFound].
func (r *PipelineVersionResolver) resolveVersionID(ctx context.Context, provider string, number int32) (uuid.UUID, error) {
	//nolint:gocritic // AsAIBridged has a minimal permission set for this purpose.
	authCtx := dbauthz.AsAIBridged(ctx)
	id, err := r.db.GetAIGatewayPipelineVersionIDByProviderAndNumber(authCtx, database.GetAIGatewayPipelineVersionIDByProviderAndNumberParams{
		ProviderName:  provider,
		VersionNumber: number,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, aibridge.ErrPipelineVersionNotFound
		}
		return uuid.Nil, xerrors.Errorf("resolve pipeline version id: %w", err)
	}
	return id, nil
}

// build compiles the pipeline snapshot for a single version. The pipeline's
// enabled flag is intentionally not consulted (a staged version is rehearsed
// before it is promoted/enabled); disabled members and soft-deleted parents are
// still excluded, matching the active snapshot.
func (r *PipelineVersionResolver) build(ctx context.Context, provider string, versionID uuid.UUID) (aibridge.ProviderPipelines, error) {
	//nolint:gocritic // AsAIBridged has a minimal permission set for this purpose.
	authCtx := dbauthz.AsAIBridged(ctx)

	prov, err := r.db.GetAIGatewayPipelineVersionProvider(authCtx, versionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return aibridge.ProviderPipelines{}, aibridge.ErrPipelineVersionNotFound
		}
		return aibridge.ProviderPipelines{}, xerrors.Errorf("resolve pipeline version provider: %w", err)
	}
	if prov.ProviderName != provider {
		// The version belongs to another provider's pipeline; rehearsing it
		// against this provider's traffic would evaluate a posture built for a
		// different request shape.
		return aibridge.ProviderPipelines{}, aibridge.ErrPipelineVersionNotFound
	}

	policyRows, err := r.db.GetAIGatewayPipelineVersionPolicySnapshot(authCtx, versionID)
	if err != nil {
		return aibridge.ProviderPipelines{}, xerrors.Errorf("load pipeline version policy snapshot: %w", err)
	}
	members := make([]policyMember, 0, len(policyRows))
	for _, row := range policyRows {
		members = append(members, policyMember{
			ProviderName:          row.ProviderName,
			PipelineVersionNumber: row.PipelineVersionNumber,
			Hook:                  row.Hook,
			Kind:                  row.Kind,
			FailMode:              row.FailMode,
			PolicyName:            row.PolicyName,
			PolicyVersionID:       row.PolicyVersionID,
			Rego:                  row.Rego,
		})
	}
	out, _ := buildProviderPipelines(ctx, r.logger, members)

	pp := out[provider]
	// Stamp the version explicitly so a member-less (pass-through) version still
	// reports the version that evaluated the request.
	pp.Version = prov.VersionNumber
	hooks := map[string]aibridge.ProviderPipelines{provider: pp}

	grRows, err := r.db.GetAIGatewayPipelineVersionGuardrailSnapshot(authCtx, versionID)
	if err != nil {
		return aibridge.ProviderPipelines{}, xerrors.Errorf("load pipeline version guardrail snapshot: %w", err)
	}
	grMembers := make([]guardrailMember, 0, len(grRows))
	for _, row := range grRows {
		grMembers = append(grMembers, guardrailMember{
			ProviderName:     row.ProviderName,
			Hook:             row.Hook,
			AdapterType:      row.AdapterType,
			GuardrailName:    row.GuardrailName,
			Config:           row.Config,
			Credential:       row.Credential,
			Mode:             row.Mode,
			FailMode:         row.FailMode,
			NetworkTimeoutMs: row.NetworkTimeoutMs,
		})
	}
	attachGuardrailStages(ctx, r.logger, grMembers, hooks)

	return hooks[provider], nil
}

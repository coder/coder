package aibridged

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/policy"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// policyMember is a normalized policy membership row, shared by the
// active-snapshot loader and the version-targeted resolver so both reuse one
// compile/assembly path regardless of which SQL query produced them.
type policyMember struct {
	ProviderName          string
	PipelineVersionNumber int32
	Hook                  database.AIGatewayHook
	Kind                  database.AIGatewayPolicyKind
	FailMode              database.AIGatewayFailMode
	PolicyName            string
	PolicyVersionID       uuid.UUID
	Rego                  string
}

// BuildProviderPipelines loads the active policy pipeline snapshot from the
// database and compiles it into per-provider pipelines keyed by provider name.
// A provider with no active, enabled pipeline is absent from the returned map
// and runs no policies (pass-through).
//
// The snapshot is read in a single query (GetActiveAIGatewayPipelinePolicies),
// which returns each live, enabled pipeline's active version members joined to
// their pinned policy versions. A single malformed policy row is logged and
// excluded rather than failing the whole snapshot; only a DB query failure is
// propagated, so a reload preserves the last-good snapshot.
func BuildProviderPipelines(ctx context.Context, db database.Store, logger slog.Logger) (map[string]aibridge.ProviderPipelines, []PipelinePolicyOutcome, error) {
	//nolint:gocritic // AsAIBridged has a minimal permission set for this purpose.
	authCtx := dbauthz.AsAIBridged(ctx)

	rows, err := db.GetActiveAIGatewayPipelinePolicies(authCtx)
	if err != nil {
		return nil, nil, xerrors.Errorf("load active ai gateway pipeline policies: %w", err)
	}

	members := make([]policyMember, 0, len(rows))
	for _, row := range rows {
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

	out, outcomes := buildProviderPipelines(ctx, logger, members)
	return out, outcomes, nil
}

// buildProviderPipelines compiles a normalized set of policy members into
// per-provider pipelines. It is shared by the active-snapshot loader and the
// version-targeted resolver. A malformed individual policy is logged and
// excluded; a malformed hook composition skips that hook for the provider.
func buildProviderPipelines(ctx context.Context, logger slog.Logger, members []policyMember) (map[string]aibridge.ProviderPipelines, []PipelinePolicyOutcome) {
	// activeVersion records each provider's live pipeline version for metrics.
	activeVersion := make(map[string]int32)

	// preToolFailOpen records each provider's aggregate pre-tool fail mode: it
	// stays true only while every pre-tool member is fail-open. Any fail-closed
	// member flips it false, so an unevaluable (over-cap/incomplete) tool block
	// terminates the turn.
	preToolFailOpen := make(map[string]bool)

	// Group stage configs per (provider, hook). Rows arrive ordered by
	// provider, hook, kind, then policy name, so decide ordering is stable.
	type hookKey struct {
		provider string
		hook     database.AIGatewayHook
	}
	configs := make(map[hookKey]*policy.PipelineConfig)

	for _, member := range members {
		if !hookAllowsKind(member.Hook, member.Kind) {
			// Defense in depth: the registration gate enforces kind-validity by
			// hook, but a direct DB write must not smuggle an invalid posture
			// into the runtime.
			logger.Warn(ctx, "skipping policy with kind invalid for hook",
				slog.F("provider", member.ProviderName),
				slog.F("policy", member.PolicyName),
				slog.F("hook", string(member.Hook)),
				slog.F("kind", string(member.Kind)),
			)
			continue
		}

		activeVersion[member.ProviderName] = member.PipelineVersionNumber

		if member.Hook == database.AIGatewayHookPreTool {
			fo, seen := preToolFailOpen[member.ProviderName]
			if !seen {
				fo = true
			}
			preToolFailOpen[member.ProviderName] = fo && member.FailMode == database.AIGatewayFailModeFailOpen
		}

		key := hookKey{member.ProviderName, member.Hook}
		cfg := configs[key]
		if cfg == nil {
			cfg = &policy.PipelineConfig{}
			configs[key] = cfg
		}

		failMode := policy.FailClosed
		if member.FailMode == database.AIGatewayFailModeFailOpen {
			failMode = policy.FailOpen
		}
		opt := policy.WithFailMode(failMode)

		switch member.Kind {
		case database.AIGatewayPolicyKindClassify:
			p, err := policy.NewClassify(member.PolicyName, member.Rego, opt)
			if err != nil {
				logSkipPolicy(ctx, logger, member, err)
				continue
			}
			cfg.Classify = append(cfg.Classify, p)
		case database.AIGatewayPolicyKindRoute:
			p, err := policy.NewRoute(member.PolicyName, member.Rego, opt)
			if err != nil {
				logSkipPolicy(ctx, logger, member, err)
				continue
			}
			cfg.Route = p
		case database.AIGatewayPolicyKindDecide:
			p, err := policy.NewDecide(member.PolicyName, member.Rego, opt)
			if err != nil {
				logSkipPolicy(ctx, logger, member, err)
				continue
			}
			cfg.Decide = append(cfg.Decide, p)
		case database.AIGatewayPolicyKindTransform:
			p, err := policy.NewTransform(member.PolicyName, member.Rego, opt)
			if err != nil {
				logSkipPolicy(ctx, logger, member, err)
				continue
			}
			cfg.Transform = append(cfg.Transform, p)
		default:
			logger.Warn(ctx, "skipping policy with unknown kind",
				slog.F("provider", member.ProviderName),
				slog.F("policy", member.PolicyName),
				slog.F("kind", string(member.Kind)),
			)
		}
	}

	out := make(map[string]aibridge.ProviderPipelines)
	for key, cfg := range configs {
		// The pre-auth and pre-tool hooks permit only classify and decide; build
		// them through the constrained constructors so a route/transform smuggled
		// past the kind-validity check (which would modify the request) is
		// rejected.
		newPipe := policy.NewPipeline
		switch key.hook {
		case database.AIGatewayHookPreAuth:
			newPipe = policy.NewPreAuthPipeline
		case database.AIGatewayHookPreTool:
			newPipe = policy.NewToolPipeline
		}
		pipe, err := newPipe(*cfg)
		if err != nil {
			// A malformed composition (e.g. >1 classify) should be impossible
			// given the DB constraints; skip the whole hook for this provider
			// rather than risk an inconsistent posture.
			logger.Error(ctx, "skipping invalid policy pipeline for hook",
				slog.F("provider", key.provider),
				slog.F("hook", string(key.hook)),
				slog.Error(err),
			)
			continue
		}
		pp := out[key.provider]
		pp.Version = activeVersion[key.provider]
		switch key.hook {
		case database.AIGatewayHookPreAuth:
			pp.PreAuth = pipe
		case database.AIGatewayHookPreReq:
			pp.PreReq = pipe
		case database.AIGatewayHookPreTool:
			pp.PreTool = pipe
			pp.PreToolFailOpen = preToolFailOpen[key.provider]
		}
		out[key.provider] = pp
	}

	outcomes := make([]PipelinePolicyOutcome, 0, len(activeVersion))
	for provider, version := range activeVersion {
		outcomes = append(outcomes, PipelinePolicyOutcome{Provider: provider, PipelineVersion: version})
	}

	return out, outcomes
}

// hookAllowsKind reports whether a kind is valid at a hook. pre-auth has no
// request body or identity, so only identity-free kinds run there.
func hookAllowsKind(hook database.AIGatewayHook, kind database.AIGatewayPolicyKind) bool {
	switch hook {
	case database.AIGatewayHookPreAuth:
		return kind == database.AIGatewayPolicyKindClassify || kind == database.AIGatewayPolicyKindDecide
	case database.AIGatewayHookPreReq:
		return true
	case database.AIGatewayHookPreTool:
		// The request is already dispatched, so route and transform do not
		// apply; only classify and decide gate a tool call.
		return kind == database.AIGatewayPolicyKindClassify || kind == database.AIGatewayPolicyKindDecide
	default:
		return false
	}
}

func logSkipPolicy(ctx context.Context, logger slog.Logger, member policyMember, err error) {
	logger.Error(ctx, "skipping policy that failed to compile",
		slog.F("provider", member.ProviderName),
		slog.F("policy", member.PolicyName),
		slog.F("policy_version_id", member.PolicyVersionID),
		slog.F("kind", string(member.Kind)),
		slog.Error(err),
	)
}

package aibridged

import (
	"context"
	"encoding/json"
	"time"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/guardrail/adapters"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// guardrailMember is a normalized guardrail membership row, shared by the
// active-snapshot loader and the version-targeted resolver.
type guardrailMember struct {
	ProviderName     string
	Hook             database.AIGatewayHook
	AdapterType      string
	GuardrailName    string
	Config           json.RawMessage
	Credential       string
	FailMode         database.AIGatewayFailMode
	NetworkTimeoutMs int32
}

// BuildProviderGuardrails loads the active guardrail snapshot from the database
// and attaches a compiled guardrail [guardrail.Stage] per (provider, hook) onto
// the provided hooks map, mutating it in place. Providers absent from hooks gain
// an entry when they have guardrails but no policy pipeline. A single malformed
// guardrail row is logged and skipped rather than failing the whole snapshot;
// only a DB query failure is propagated, so a reload preserves the last-good
// snapshot.
//
// v1 wires only pre-req guardrails into the runtime; pre-auth guardrails are
// loaded but skipped with a warning until the pre-auth hook runs a stage.
func BuildProviderGuardrails(ctx context.Context, db database.Store, logger slog.Logger, hooks map[string]aibridge.ProviderPipelines) error {
	//nolint:gocritic // AsAIBridged has a minimal permission set for this purpose.
	authCtx := dbauthz.AsAIBridged(ctx)

	rows, err := db.GetActiveAIGatewayPipelineGuardrails(authCtx)
	if err != nil {
		return err
	}

	members := make([]guardrailMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, guardrailMember{
			ProviderName:     row.ProviderName,
			Hook:             row.Hook,
			AdapterType:      row.AdapterType,
			GuardrailName:    row.GuardrailName,
			Config:           row.Config,
			Credential:       row.Credential,
			FailMode:         row.FailMode,
			NetworkTimeoutMs: row.NetworkTimeoutMs,
		})
	}

	attachGuardrailStages(ctx, logger, members, hooks)
	return nil
}

// attachGuardrailStages compiles a normalized set of guardrail members into
// per-provider stages and attaches them onto hooks in place. Shared by the
// active-snapshot loader and the version-targeted resolver.
func attachGuardrailStages(ctx context.Context, logger slog.Logger, members []guardrailMember, hooks map[string]aibridge.ProviderPipelines) {
	type hookKey struct {
		provider string
		hook     database.AIGatewayHook
	}
	grouped := make(map[hookKey][]guardrail.Member)

	for _, member := range members {
		g, err := adapters.Build(member.AdapterType, member.GuardrailName, member.Config, member.Credential)
		if err != nil {
			logger.Error(ctx, "skipping guardrail that failed to build",
				slog.F("provider", member.ProviderName),
				slog.F("guardrail", member.GuardrailName),
				slog.F("adapter_type", member.AdapterType),
				slog.Error(err),
			)
			continue
		}

		failMode := guardrail.FailClosed
		if member.FailMode == database.AIGatewayFailModeFailOpen {
			failMode = guardrail.FailOpen
		}

		key := hookKey{member.ProviderName, member.Hook}
		grouped[key] = append(grouped[key], guardrail.Member{
			Guardrail: g,
			FailMode:  failMode,
			Timeout:   time.Duration(member.NetworkTimeoutMs) * time.Millisecond,
		})
	}

	for key, ms := range grouped {
		if key.hook != database.AIGatewayHookPreReq {
			logger.Warn(ctx, "skipping guardrails for unsupported hook",
				slog.F("provider", key.provider), slog.F("hook", string(key.hook)))
			continue
		}
		stage, err := guardrail.NewStage(ms...)
		if err != nil {
			logger.Error(ctx, "skipping invalid guardrail stage for hook",
				slog.F("provider", key.provider), slog.F("hook", string(key.hook)), slog.Error(err))
			continue
		}
		pp := hooks[key.provider]
		pp.PreReqGuardrails = stage
		hooks[key.provider] = pp
	}
}

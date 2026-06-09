package aibridged

import (
	"context"
	"time"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/guardrail/adapters"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

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

	type hookKey struct {
		provider string
		hook     database.AIGatewayHook
	}
	members := make(map[hookKey][]guardrail.Member)

	for _, row := range rows {
		g, err := adapters.Build(row.AdapterType, row.GuardrailName, row.Config, row.Credential)
		if err != nil {
			logger.Error(ctx, "skipping guardrail that failed to build",
				slog.F("provider", row.ProviderName),
				slog.F("guardrail", row.GuardrailName),
				slog.F("adapter_type", row.AdapterType),
				slog.Error(err),
			)
			continue
		}

		mode := guardrail.ModeAdvisory
		if row.Mode == database.AIGatewayGuardrailModeEnforcing {
			mode = guardrail.ModeEnforcing
		}
		failMode := guardrail.FailClosed
		if row.FailMode == database.AIGatewayFailModeFailOpen {
			failMode = guardrail.FailOpen
		}

		key := hookKey{row.ProviderName, row.Hook}
		members[key] = append(members[key], guardrail.Member{
			Guardrail: g,
			Mode:      mode,
			FailMode:  failMode,
			Timeout:   time.Duration(row.NetworkTimeoutMs) * time.Millisecond,
		})
	}

	for key, ms := range members {
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
	return nil
}

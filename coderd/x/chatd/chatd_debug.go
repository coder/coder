package chatd

import (
	"context"
	"time"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const (
	debugCleanupRetryDelay = 500 * time.Millisecond
	debugCleanupAttempts   = 3
	debugCleanupTimeout    = 5 * time.Second
	// debugCreateRunTimeout caps how long a CreateRun insert can
	// block the caller's critical path. Debug persistence is
	// best-effort, so the turn proceeds without debug rows if the
	// DB is slow or locked. Matches the manual-title budget.
	debugCreateRunTimeout = 5 * time.Second
	// debugFinalizeTimeout caps best-effort debug run finalization
	// outside the runner's canceled context.
	debugFinalizeTimeout = 5 * time.Second
	// debugCleanupClockSkew gives cleanup cutoffs tolerance for cross-
	// replica clock drift. The cutoff is sampled from the DB
	// (updated_at returned by the status transition), and
	// chat_debug_runs.started_at is stamped by whatever replica
	// processes the replacement turn. If that replica's clock lags
	// the DB, its started_at can land behind a commit-time cutoff
	// even though the insert physically happened after commit.
	// Subtracting this buffer ensures the fast retry path cannot
	// delete replacement rows when clocks drift by up to this
	// amount; rows within the buffer survive the fast cleanup but
	// are still finalized (and eligible for stale-sweep cleanup) by
	// the existing FinalizeStale background loop.
	debugCleanupClockSkew = 30 * time.Second
)

func (p *Server) debugService() *chatdebug.Service {
	if p == nil {
		return nil
	}
	if p.debugSvcFactory == nil {
		return p.debugSvc
	}
	p.debugSvcInit.Do(func() {
		p.debugSvc = p.debugSvcFactory()
		p.debugSvcReady.Store(p.debugSvc != nil)
	})
	return p.debugSvc
}

func (p *Server) existingDebugService() *chatdebug.Service {
	if p == nil {
		return nil
	}
	if p.debugSvcFactory == nil {
		return p.debugSvc
	}
	if !p.debugSvcReady.Load() {
		return nil
	}
	return p.debugSvc
}

func (p *Server) scheduleDebugCleanup(
	ctx context.Context,
	logMessage string,
	fields []slog.Field,
	cleanup func(context.Context, *chatdebug.Service) error,
) {
	debugSvc := p.debugService()
	if debugSvc == nil {
		return
	}

	cleanupCtx, stopCleanupCtx := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer stopCleanupCtx()
		for attempt := 0; attempt < debugCleanupAttempts; attempt++ {
			if attempt > 0 {
				timer := p.clock.NewTimer(debugCleanupRetryDelay, "chatd", "debug_cleanup")
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-cleanupCtx.Done():
					timer.Stop()
					return
				}
			}

			passCtx, cancel := context.WithTimeout(cleanupCtx, debugCleanupTimeout)
			err := cleanup(passCtx, debugSvc)
			cancel()
			if err == nil {
				return
			}

			logFields := append([]slog.Field{
				slog.F("attempt", attempt+1),
				slog.F("max_attempts", debugCleanupAttempts),
			}, fields...)
			logFields = append(logFields, slog.Error(err))
			p.logger.Warn(cleanupCtx, logMessage, logFields...)
		}
	}); err != nil {
		stopCleanupCtx()
		logFields := append([]slog.Field{slog.F("cleanup", logMessage)}, fields...)
		logFields = append(logFields, slog.Error(err))
		p.logger.Error(context.WithoutCancel(ctx), "failed to schedule chat debug cleanup", logFields...)
	}
}

func (p *Server) newDebugAwareModel(
	ctx context.Context,
	req modelClientRequest,
	route aiGatewayModelRoute,
	opts modelBuildOptions,
) (fantasy.LanguageModel, bool, error) {
	provider, resolvedModel, err := chatprovider.ResolveModelWithProviderHint(req.ModelName, route.ModelProviderHint)
	if err != nil {
		return nil, false, err
	}
	route.ModelProviderHint = provider
	req.ModelName = resolvedModel

	debugSvc := p.debugService()
	debugEnabled := debugSvc != nil && debugSvc.IsEnabled(ctx, req.Chat.ID, req.Chat.OwnerID)
	opts.RecordHTTP = debugEnabled

	model, err := p.newModel(ctx, req, route, opts)
	if err != nil {
		return nil, debugEnabled, err
	}
	if !debugEnabled {
		return model, false, nil
	}

	return chatdebug.WrapModel(model, debugSvc, chatdebug.RecorderOptions{
		ChatID:   req.Chat.ID,
		OwnerID:  req.Chat.OwnerID,
		Provider: provider,
		Model:    resolvedModel,
	}), true, nil
}

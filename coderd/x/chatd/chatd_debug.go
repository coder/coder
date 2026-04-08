package chatd

import (
	"context"
	"net/http"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const (
	debugCleanupRetryDelay = 500 * time.Millisecond
	debugCleanupAttempts   = 3
	debugCleanupTimeout    = 5 * time.Second
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

	// Acquire inflightMu around the positive Add so Close() cannot
	// call drainInflight concurrently when the counter is at zero.
	// See drainInflight for the WaitGroup contract this preserves.
	p.inflightMu.Lock()
	p.inflight.Add(1)
	p.inflightMu.Unlock()
	go func() {
		defer p.inflight.Done()

		cleanupCtx := context.WithoutCancel(ctx)
		for attempt := 0; attempt < debugCleanupAttempts; attempt++ {
			if attempt > 0 {
				timer := p.clock.NewTimer(debugCleanupRetryDelay, "chatd", "debug_cleanup")
				<-timer.C
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
	}()
}

func (p *Server) newDebugAwareModelFromConfig(
	ctx context.Context,
	chat database.Chat,
	providerHint string,
	modelName string,
	providerKeys chatprovider.ProviderAPIKeys,
	userAgent string,
	extraHeaders map[string]string,
) (fantasy.LanguageModel, bool, error) {
	provider, resolvedModel, err := chatprovider.ResolveModelWithProviderHint(modelName, providerHint)
	if err != nil {
		return nil, false, err
	}

	debugSvc := p.debugService()
	debugEnabled := debugSvc != nil && debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID)

	var httpClient *http.Client
	if debugEnabled {
		httpClient = &http.Client{Transport: &chatdebug.RecordingTransport{}}
	}

	model, err := chatprovider.ModelFromConfig(
		provider,
		resolvedModel,
		providerKeys,
		userAgent,
		extraHeaders,
		httpClient,
	)
	if err != nil {
		return nil, debugEnabled, err
	}
	if model == nil {
		return nil, debugEnabled, xerrors.Errorf(
			"create model for %s/%s returned nil",
			provider,
			resolvedModel,
		)
	}
	if !debugEnabled {
		return model, false, nil
	}

	return chatdebug.WrapModel(model, debugSvc, chatdebug.RecorderOptions{
		ChatID:   chat.ID,
		OwnerID:  chat.OwnerID,
		Provider: provider,
		Model:    resolvedModel,
	}), true, nil
}

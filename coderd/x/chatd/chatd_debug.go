package chatd

import (
	"context"
	"net/http"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

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

	debugEnabled := p.debugSvc != nil && p.debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID)

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

	return chatdebug.WrapModel(model, p.debugSvc, chatdebug.RecorderOptions{
		ChatID:   chat.ID,
		OwnerID:  chat.OwnerID,
		Provider: provider,
		Model:    resolvedModel,
	}), true, nil
}

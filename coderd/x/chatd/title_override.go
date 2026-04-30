package chatd

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const titleGenerationOverrideContext = "title_generation"

type titleGenerationModelOverrideRead struct {
	id          uuid.UUID
	rawValue    string
	isSet       bool
	isMalformed bool
	parseErr    error
}

func readTitleGenerationModelOverride(
	ctx context.Context,
	db database.Store,
) (titleGenerationModelOverrideRead, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatTitleGenerationModelOverride(chatdCtx)
	if err != nil {
		return titleGenerationModelOverrideRead{}, xerrors.Errorf(
			"get chat title generation model override: %w",
			err,
		)
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return titleGenerationModelOverrideRead{}, nil
	}
	modelConfigUUID, err := uuid.Parse(trimmed)
	if err != nil {
		return titleGenerationModelOverrideRead{
			rawValue:    trimmed,
			isSet:       true,
			isMalformed: true,
			parseErr:    err,
		}, nil
	}
	return titleGenerationModelOverrideRead{
		id:       modelConfigUUID,
		rawValue: trimmed,
		isSet:    true,
	}, nil
}

// resolveTitleGenerationModelOverride resolves the deployment-wide title
// generation model override. It returns four values:
//
//   - modelConfig and model: populated only on success.
//   - overrideSet: true when the admin configured a non-empty override,
//     regardless of whether resolution succeeded. Callers MUST always check
//     err first; overrideSet alone does not imply the model is usable.
//   - err: non-nil when resolution failed. DB read failure returns
//     (zero, nil, false, err). With overrideSet=true, the override is
//     configured but unusable (deleted model, missing credentials, etc.) and
//     callers should treat this as a hard failure for explicit-override
//     semantics, not a soft fallback.
//
// When the override is unset or stored as malformed, the function returns
// (zero, nil, false, nil) so callers can fall back to default behavior.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (database.ChatModelConfig, fantasy.LanguageModel, bool, error) {
	read, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, nil, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}
	if read.isMalformed {
		p.logger.Info(ctx,
			"invalid model override, ignoring",
			slog.F("override_context", titleGenerationOverrideContext),
			slog.F("raw_value", read.rawValue),
			slog.Error(read.parseErr),
		)
		return database.ChatModelConfig{}, nil, false, nil
	}
	if !read.isSet {
		return database.ChatModelConfig{}, nil, false, nil
	}

	modelConfig, providerName, err := p.resolveModelConfigAndNormalizedProvider(
		ctx,
		read.id,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
				"title generation model override is unavailable: %s",
				read.id,
			)
		case errors.Is(err, errInvalidModelOverrideMetadata):
			return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
				"title generation model override metadata is invalid for %s: %w",
				read.id,
				err,
			)
		default:
			return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
				"resolve title generation model override %s: %w",
				read.id,
				err,
			)
		}
	}

	if keys.APIKey(providerName) == "" &&
		!(chatprovider.ProviderAllowsAmbientCredentials(providerName) &&
			keys.HasProvider(providerName)) {
		return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
			"title generation model override credentials are unavailable for provider %q",
			providerName,
		)
	}

	model, err := chatprovider.ModelFromConfig(
		modelConfig.Provider,
		modelConfig.Model,
		keys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
		nil,
	)
	if err != nil {
		return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	if model == nil {
		return database.ChatModelConfig{}, nil, true, xerrors.Errorf(
			"create title generation model override returned nil",
		)
	}

	return modelConfig, model, true, nil
}

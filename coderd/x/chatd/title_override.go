package chatd

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const titleGenerationOverrideContext = "title_generation"

type parsedModelOverride struct {
	modelConfigID   uuid.UUID
	reasoningEffort *string
}

func parseModelOverride(raw string) (parsedModelOverride, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return parsedModelOverride{}, true
	}
	rawID, rawEffort, hasEffort := strings.Cut(trimmed, ":")
	modelConfigID, err := uuid.Parse(rawID)
	if err != nil || (hasEffort && rawEffort == "") {
		return parsedModelOverride{}, false
	}
	parsed := parsedModelOverride{modelConfigID: modelConfigID}
	if hasEffort {
		parsed.reasoningEffort = &rawEffort
	}
	return parsed, true
}

func readTitleGenerationModelOverride(
	ctx context.Context,
	db database.Store,
) (string, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatTitleGenerationModelOverride(chatdCtx)
	if err != nil {
		return "", xerrors.Errorf(
			"get chat title generation model override: %w",
			err,
		)
	}
	return raw, nil
}

// resolveTitleGenerationModelOverride resolves the deployment-wide title
// generation model override. overrideSet is true when an override was
// configured and resolved; a configured override that does not resolve
// because it is unknown or disabled is ignored. Model construction failures
// after a successful resolution stay hard failures.
// When overrideSet is false, callers may fall back to the default title
// model.
func (p *Server) resolveTitleGenerationModelOverride(
	ctx context.Context,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (resolvedModelCall, bool, error) {
	raw, err := readTitleGenerationModelOverride(ctx, p.db)
	if err != nil {
		return resolvedModelCall{}, false, xerrors.Errorf(
			"read title generation model override: %w",
			err,
		)
	}

	modelConfig, _, overrideEffort, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		titleGenerationOverrideContext,
		raw,
		chat.OwnerID,
		func(ctx context.Context, modelConfigID uuid.UUID) (database.ChatModelConfig, string, error) {
			return p.resolveModelConfigForOrganization(ctx, chat.OwnerID, chat.OrganizationID, modelConfigID)
		},
		func(ctx context.Context, ownerID uuid.UUID, aiProviderID uuid.UUID) (chatprovider.ProviderAPIKeys, error) {
			return p.resolveUserProviderAPIKeys(ctx, ownerID, aiProviderID)
		},
		modelOverrideFailureModeSoft,
	)
	if err != nil {
		if errors.Is(err, errModelConfigOutsideOrganization) {
			return resolvedModelCall{}, false, err
		}
		return resolvedModelCall{}, overrideSet, err
	}
	if !overrideSet {
		return resolvedModelCall{}, false, nil
	}
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:          "title",
		chat:             chat,
		explicitConfig:   &modelConfig,
		requestedEffort:  overrideEffort,
		chatdScopedRoute: true,
		buildOptions:     modelOpts,
	})
	if err != nil {
		return resolvedModelCall{}, true, xerrors.Errorf(
			"create title generation model override: %w",
			err,
		)
	}
	return resolved, true, nil
}

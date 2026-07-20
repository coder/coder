package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func getChatModelOverrideForOrganization(ctx context.Context, store database.Store, organizationID uuid.UUID, overrideContext string) (string, error) {
	switch overrideContext {
	case string(codersdk.ChatModelOverrideContextGeneral):
		return store.GetChatGeneralModelOverrideForOrganization(ctx, organizationID)
	case string(codersdk.ChatModelOverrideContextExplore):
		return store.GetChatExploreModelOverrideForOrganization(ctx, organizationID)
	case titleGenerationOverrideContext:
		return store.GetChatTitleGenerationModelOverrideForOrganization(ctx, organizationID)
	case compactionOverrideContext:
		return store.GetChatCompactionModelOverrideForOrganization(ctx, organizationID)
	default:
		return "", xerrors.Errorf("unsupported chat model override context %q", overrideContext)
	}
}

func getUserChatPersonalModelOverrideForOrganization(
	ctx context.Context,
	store database.Store,
	userID, organizationID uuid.UUID,
	overrideContext codersdk.ChatPersonalModelOverrideContext,
) (string, error) {
	return store.GetUserChatPersonalModelOverrideForOrganization(ctx, database.GetUserChatPersonalModelOverrideForOrganizationParams{
		UserID:         userID,
		OrganizationID: organizationID,
		Context:        string(overrideContext),
	})
}

func getUserChatCompactionThresholdForOrganization(
	ctx context.Context,
	store database.Store,
	userID, organizationID, modelConfigID uuid.UUID,
) (string, error) {
	return store.GetUserChatCompactionThresholdForOrganization(ctx, database.GetUserChatCompactionThresholdForOrganizationParams{
		UserID:         userID,
		OrganizationID: organizationID,
		ModelConfigID:  modelConfigID,
	})
}

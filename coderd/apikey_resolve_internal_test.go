package coderd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func TestConvertAPIKeyAllowListDisplayName(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	db := dbmock.NewMockStore(ctrl)
	templateID := uuid.New()

	db.EXPECT().
		GetTemplateByID(gomock.Any(), templateID).
		Return(database.Template{
			ID:          templateID,
			Name:        "infra-template",
			DisplayName: "Infra Template",
		}, nil).
		Times(1)

	key := database.APIKey{
		ID:              "key-1",
		UserID:          uuid.New(),
		LastUsed:        time.Now(),
		ExpiresAt:       time.Now().Add(time.Hour),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		LoginType:       database.LoginTypeToken,
		LifetimeSeconds: int64(time.Hour.Seconds()),
		TokenName:       "cli",
		Scopes:          database.APIKeyScopes{database.ApiKeyScopeCoderAll},
		AllowList: database.AllowList{
			{Type: string(codersdk.ResourceTemplate), ID: templateID.String()},
		},
	}

	result := convertAPIKey(key)

	require.Len(t, result.AllowList, 1)
	require.Equal(t, codersdk.ResourceTemplate, result.AllowList[0].Type)
	require.Equal(t, templateID.String(), result.AllowList[0].ID)
	require.Equal(t, "Infra Template", result.AllowList[0].DisplayName)
}

func TestConvertAPIKeyAllowListDisplayNameWildcard(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	key := database.APIKey{
		ID: "key-2",
		AllowList: database.AllowList{
			{Type: string(codersdk.ResourceWildcard), ID: policy.WildcardSymbol},
		},
	}

	result := convertAPIKey(key)

	require.Len(t, result.AllowList, 1)
	require.Equal(t, codersdk.ResourceWildcard, result.AllowList[0].Type)
	require.Equal(t, "*", result.AllowList[0].ID)
	require.Empty(t, result.AllowList[0].DisplayName)
}

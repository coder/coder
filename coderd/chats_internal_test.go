package coderd

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/gitsync"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveChatGitAccessToken_OriginScoped(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	db.EXPECT().GetExternalAuthLink(gomock.Any(), database.GetExternalAuthLinkParams{
		ProviderID: "github-enterprise",
		UserID:     userID,
	}).Return(database.ExternalAuthLink{}, sql.ErrNoRows)

	api := &API{
		Options: &Options{
			Database: db,
			Logger:   testutil.Logger(t),
			ExternalAuthConfigs: []*externalauth.Config{
				{
					ID:    "slack",
					Type:  "slack",
					Regex: regexp.MustCompile(`slack\.com`),
				},
				{
					ID:    "github-enterprise",
					Type:  "github",
					Regex: regexp.MustCompile(`ghes\.example\.com`),
				},
			},
		},
	}

	token, err := api.resolveChatGitAccessToken(
		context.Background(),
		userID,
		"https://ghes.example.com/acme/repo",
	)
	require.Nil(t, token)
	require.ErrorIs(t, err, gitsync.ErrNoTokenAvailable)
}

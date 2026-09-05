package coderd_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDeploymentValues(t *testing.T) {
	t.Parallel()
	hi := "hi"
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	cfg := coderdtest.DeploymentValues(t)
	// values should be returned
	cfg.BrowserOnly = true
	// values should not be returned
	cfg.OAuth2.Github.ClientSecret.Set(hi)
	cfg.OIDC.ClientSecret.Set(hi)
	cfg.OIDC.AuthURLParams.Set(`{"foo":"bar"}`)
	cfg.OIDC.EmailField.Set("some_random_field_you_never_expected")
	cfg.PostgresURL.Set(hi)
	cfg.SCIMAPIKey.Set(hi)
	cfg.ExternalTokenEncryptionKeys.Set("the_random_key_we_never_expected,an_other_key_we_never_unexpected")
	cfg.Provisioner.DaemonPSK = "provisionersftw"

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: cfg,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	scrubbed, err := client.DeploymentConfig(ctx)
	require.NoError(t, err)
	// ensure normal values pass through
	require.EqualValues(t, true, scrubbed.Values.BrowserOnly.Value())
	require.NotEmpty(t, cfg.OIDC.AuthURLParams)
	require.EqualValues(t, cfg.OIDC.AuthURLParams, scrubbed.Values.OIDC.AuthURLParams)
	require.NotEmpty(t, cfg.OIDC.EmailField)
	require.EqualValues(t, cfg.OIDC.EmailField, scrubbed.Values.OIDC.EmailField)
	// ensure secrets are removed
	require.Empty(t, scrubbed.Values.OAuth2.Github.ClientSecret.Value())
	require.Empty(t, scrubbed.Values.OIDC.ClientSecret.Value())
	require.Empty(t, scrubbed.Values.PostgresURL.Value())
	require.Empty(t, scrubbed.Values.SCIMAPIKey.Value())
	require.Empty(t, scrubbed.Values.ExternalTokenEncryptionKeys.Value())
	require.Empty(t, scrubbed.Values.Provisioner.DaemonPSK.Value())
}

type deploymentAgentTimeErrorStore struct {
	database.Store
}

func (deploymentAgentTimeErrorStore) GetDeploymentAgentTimeMsInRange(context.Context, database.GetDeploymentAgentTimeMsInRangeParams) (int64, error) {
	return 0, xerrors.New("forced Agent Time query failure")
}

func insertDeploymentAgentTimeMessage(
	t *testing.T,
	db database.Store,
	sqlDB *sql.DB,
	userID uuid.UUID,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
	runtimeMs int64,
	createdAt time.Time,
	archived bool,
	deleted bool,
) int64 {
	t.Helper()

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    organizationID,
		OwnerID:           userID,
		LastModelConfigID: modelConfigID,
	})
	_, err := sqlDB.ExecContext(t.Context(), "UPDATE chats SET archived = $1 WHERE id = $2", archived, chat.ID)
	require.NoError(t, err)

	message := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chat.ID,
		CreatedBy:     uuid.NullUUID{UUID: userID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
		Role:          database.ChatMessageRoleAssistant,
		RuntimeMs:     sql.NullInt64{Int64: runtimeMs, Valid: true},
	})
	_, err = sqlDB.ExecContext(t.Context(),
		"UPDATE chat_messages SET created_at = $1, deleted = $2 WHERE id = $3",
		createdAt, deleted, message.ID)
	require.NoError(t, err)
	return message.ID
}

func TestDeploymentAgentTime(t *testing.T) {
	t.Parallel()

	t.Run("CommunityRetainedUsageAndAuthorization", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Clock:    clock,
			Database: db,
		})
		firstUser := coderdtest.CreateFirstUser(t, client)
		model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			OrganizationID: firstUser.OrganizationID,
			CreatedBy:      uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})

		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 10, now.Add(-365*24*time.Hour), false, false)
		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 20, now.Add(-time.Hour), true, true)
		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 40, now.Add(time.Hour), false, false)

		agentTime, err := client.DeploymentAgentTime(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 30, agentTime.TotalRuntimeMs)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		_, err = memberClient.DeploymentAgentTime(ctx)
		var sdkError *codersdk.Error
		require.ErrorAs(t, err, &sdkError)
		require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

		_, err = codersdk.New(client.URL).DeploymentAgentTime(ctx)
		require.ErrorAs(t, err, &sdkError)
		require.Equal(t, http.StatusUnauthorized, sdkError.StatusCode())
	})

	t.Run("LicensedUsagePeriod", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
		periodStart := now.Add(-20 * 24 * time.Hour)
		periodEnd := now.Add(-10 * 24 * time.Hour)
		clock := quartz.NewMock(t)
		clock.Set(now)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
			Clock:    clock,
			Database: db,
		})
		firstUser := coderdtest.CreateFirstUser(t, client)
		api.Entitlements.Modify(func(current *codersdk.Entitlements) {
			current.HasLicense = true
			current.Features[codersdk.FeatureAgentRuntimeHours] = codersdk.Feature{
				Entitlement: codersdk.EntitlementEntitled,
				Enabled:     true,
				UsagePeriod: &codersdk.UsagePeriod{
					IssuedAt: periodStart.Add(-time.Hour),
					Start:    periodStart,
					End:      periodEnd,
				},
			}
		})
		model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			OrganizationID: firstUser.OrganizationID,
			CreatedBy:      uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})

		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 1, periodStart.Add(-time.Second), false, false)
		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 2, periodStart, false, false)
		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 4, periodStart.Add(time.Hour), false, false)
		insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 8, periodEnd, false, false)

		agentTime, err := client.DeploymentAgentTime(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 6, agentTime.TotalRuntimeMs)
	})

	t.Run("DatabaseFailure", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		logger := testutil.Logger(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database: deploymentAgentTimeErrorStore{Store: db},
			Logger:   &logger,
		})
		firstUser := coderdtest.CreateFirstUser(t, client)
		model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			OrganizationID: firstUser.OrganizationID,
			CreatedBy:      uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		messageID := insertDeploymentAgentTimeMessage(t, db, sqlDB, firstUser.UserID, firstUser.OrganizationID, model.ID, 10, time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC), false, false)
		_, err := sqlDB.ExecContext(ctx, "UPDATE chat_messages SET content = $1 WHERE id = $2", `[{"type":"text","text":"secret chat content"}]`, messageID)
		require.NoError(t, err)

		_, err = client.DeploymentAgentTime(ctx)
		var sdkError *codersdk.Error
		require.ErrorAs(t, err, &sdkError)
		require.Equal(t, http.StatusInternalServerError, sdkError.StatusCode())
		require.NotContains(t, err.Error(), "secret chat content")
	})
}

func TestDeploymentStats(t *testing.T) {
	t.Parallel()
	t.Log("This test is time-sensitive. It may fail if the deployment is not ready in time.")
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	client := coderdtest.New(t, &coderdtest.Options{})
	_ = coderdtest.CreateFirstUser(t, client)
	assert.True(t, testutil.Eventually(ctx, t, func(tctx context.Context) bool {
		_, err := client.DeploymentStats(tctx)
		return err == nil
	}, testutil.IntervalMedium), "failed to get deployment stats in time")
}

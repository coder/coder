package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestReportGenerator_TicksUnpricedAIModels(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	_, logger, db, _, notifEnq, clk := setup(t)
	seedOwner(t, db)

	// The ticker is reset after each generator run, so this trap signals when
	// the immediate or ticker-driven run has finished.
	resetTrap := clk.Trap().TickerReset()
	defer resetTrap.Close()

	generator := NewReportGenerator(ctx, logger, db, notifEnq, clk)
	t.Cleanup(func() {
		require.NoError(t, generator.Close())
	})

	// The generator runs once immediately without waiting for the ticker delay.
	resetTrap.MustWait(ctx).MustRelease(ctx)
	require.Empty(t, notifEnq.Sent())

	// The initial run records the current time. Backdate it beyond the weekly
	// frequency so the report is eligible on the next 15-minute ticker run.
	require.NoError(t, db.UpsertNotificationReportGeneratorLog(ctx, database.UpsertNotificationReportGeneratorLogParams{
		NotificationTemplateID: notifications.TemplateAIModelsUnpricedReport,
		LastGeneratedAt:        clk.Now().Add(-unpricedAIModelsReportFrequency - time.Minute),
	}))
	seedUnpricedUsage(t, db, "anthropic", database.AIProviderTypeAnthropic, "claude-opus-4-8", clk.Now())

	// Advance to the first ticker-driven run and wait for it to finish.
	advance := clk.Advance(delay)
	resetTrap.MustWait(ctx).MustRelease(ctx)
	advance.MustWait(ctx)

	sent := notifEnq.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, notifications.TemplateAIModelsUnpricedReport, sent[0].TemplateID)
}

func TestReportUnpricedAIModels(t *testing.T) {
	t.Parallel()

	t.Run("FirstRun_ReportsPrecedingWeek", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		owner := seedOwner(t, db)
		seedUnpricedUsage(t, db, "anthropic", database.AIProviderTypeAnthropic, "claude-opus-4-8", clk.Now())
		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		sent := notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport))
		require.Len(t, sent, 1)
		require.Equal(t, owner.ID, sent[0].UserID)
		require.Equal(t, "week", sent[0].Data["report_frequency"])
		require.Equal(t, []map[string]any{
			{"provider": "anthropic", "model": "claude-opus-4-8"},
		}, modelsFromPayload(t, sent[0].Data))
		require.EqualValues(t, 1, sent[0].Data["total_count"])
		require.Equal(t, false, sent[0].Data["truncated"])
	})

	t.Run("ReportsOnlyOncePerFrequency", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		seedUnpricedUsage(t, db, "anthropic", database.AIProviderTypeAnthropic, "claude-opus-4-8", clk.Now())

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))
		require.Len(t, notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport)), 1)

		notifEnq.Clear()
		clk.Advance(time.Hour)
		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		require.Empty(t, notifEnq.Sent())
	})

	t.Run("StillUnpricedAndInUse_IsReportedAgain", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)

		seedUnpricedModelUsage(t, db, initiator, provider, "claude-opus-4-8", clk.Now(), 100)
		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))
		require.Len(t, notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport)), 1)

		notifEnq.Clear()
		clk.Advance(unpricedAIModelsReportFrequency + time.Minute)
		seedUnpricedModelUsage(t, db, initiator, provider, "claude-opus-4-8", clk.Now(), 100)
		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		require.Len(t, notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport)), 1)
	})

	t.Run("NoLongerUsed_IsNotReported", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)
		seedUnpricedModelUsage(t, db, initiator, provider, "claude-opus-4-8", clk.Now(), 100)

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))
		require.Len(t, notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport)), 1)
		notifEnq.Clear()

		clk.Advance(unpricedAIModelsReportFrequency + time.Minute)
		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		require.Empty(t, notifEnq.Sent())
	})

	t.Run("InterceptionWithoutTokenUsage_IsNotReported", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)
		seedInterception(t, db, initiator, provider, "mistyped-model", clk.Now())

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))
		require.Empty(t, notifEnq.Sent())
	})

	t.Run("PricedModel_IsNotReported", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)
		seedUnpricedModelUsage(t, db, initiator, provider, "claude-opus-4-8", clk.Now(), 100)

		seedPrice(ctx, t, db, "anthropic", "claude-opus-4-8")
		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		require.Empty(t, notifEnq.Sent())
	})

	t.Run("OpenAICompat_IsNotReported", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "self-hosted", database.AIProviderTypeOpenaiCompat)
		seedUnpricedModelUsage(t, db, initiator, provider, "llama-4", clk.Now(), 100)

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		require.Empty(t, notifEnq.Sent())
	})

	t.Run("ReportsEveryOwner", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		firstOwner := seedOwner(t, db)
		secondOwner := seedOwner(t, db)
		member := dbgen.User(t, db, database.User{})
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)
		seedUnpricedModelUsage(t, db, initiator, provider, "claude-opus-4-8", clk.Now(), 100)

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		sent := notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport))
		require.Len(t, sent, 2)
		recipients := []uuid.UUID{sent[0].UserID, sent[1].UserID}
		require.Contains(t, recipients, firstOwner.ID)
		require.Contains(t, recipients, secondOwner.ID)
		require.NotContains(t, recipients, member.ID)
	})

	t.Run("TruncatesToLimit", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		initiator := dbgen.User(t, db, database.User{})
		provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)

		// Give the first model more token usage so it sorts ahead of the others.
		const overflow = 5
		seedUnpricedModelUsage(t, db, initiator, provider, "most-used-model", clk.Now(), 1_000)
		for i := range unpricedAIModelsLimit + overflow - 1 {
			seedUnpricedModelUsage(t, db, initiator, provider, fmt.Sprintf("model-%03d", i), clk.Now(), 100)
		}

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))

		sent := notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport))
		require.Len(t, sent, 1)
		models := modelsFromPayload(t, sent[0].Data)
		require.Len(t, models, unpricedAIModelsLimit)
		require.Equal(t, "most-used-model", models[0]["model"])
		require.EqualValues(t, unpricedAIModelsLimit+overflow, sent[0].Data["total_count"])
		require.Equal(t, true, sent[0].Data["truncated"])
	})

	t.Run("NothingToReport_AdvancesWindow", func(t *testing.T) {
		t.Parallel()

		ctx, logger, db, _, notifEnq, clk := setup(t)
		seedOwner(t, db)
		now := clk.Now()

		require.NoError(t, reportUnpricedAIModels(ctx, logger, db, notifEnq, clk))
		require.Empty(t, notifEnq.Sent())

		reportLog, err := db.GetNotificationReportGeneratorLogByTemplate(ctx, notifications.TemplateAIModelsUnpricedReport)
		require.NoError(t, err)
		require.True(t, now.Equal(reportLog.LastGeneratedAt))
	})
}

func seedOwner(t *testing.T, db database.Store) database.User {
	t.Helper()
	return dbgen.User(t, db, database.User{
		RBACRoles: []string{codersdk.RoleOwner},
	})
}

func seedProvider(t *testing.T, db database.Store, name string, providerType database.AIProviderType) database.AIProvider {
	t.Helper()
	return dbgen.AIProvider(t, db, database.AIProvider{
		Name: name,
		Type: providerType,
	})
}

func seedInterception(t *testing.T, db database.Store, initiator database.User, provider database.AIProvider, model string, startedAt time.Time) database.AIBridgeInterception {
	t.Helper()
	return dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID:  initiator.ID,
		Provider:     string(provider.Type),
		ProviderName: provider.Name,
		Model:        model,
		StartedAt:    startedAt,
	}, nil)
}

func seedUnpricedModelUsage(t *testing.T, db database.Store, initiator database.User, provider database.AIProvider, model string, createdAt time.Time, inputTokens int64) {
	t.Helper()
	interception := seedInterception(t, db, initiator, provider, model, createdAt)
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: interception.ID,
		InputTokens:    inputTokens,
		CreatedAt:      createdAt,
	})
}

func seedUnpricedUsage(t *testing.T, db database.Store, providerName string, providerType database.AIProviderType, model string, createdAt time.Time) {
	t.Helper()
	seedUnpricedModelUsage(t, db, dbgen.User(t, db, database.User{}), seedProvider(t, db, providerName, providerType), model, createdAt, 100)
}

func seedPrice(ctx context.Context, t *testing.T, db database.Store, provider, model string) {
	t.Helper()
	seed, err := json.Marshal([]map[string]any{{
		"provider":          provider,
		"model":             model,
		"input_price":       3_000_000,
		"output_price":      15_000_000,
		"cache_read_price":  nil,
		"cache_write_price": nil,
	}})
	require.NoError(t, err)
	require.NoError(t, db.UpsertAIModelPrices(ctx, database.UpsertAIModelPricesParams{
		Seed:   seed,
		Source: database.AIModelPriceSourceCustom,
	}))
}

func modelsFromPayload(t *testing.T, data map[string]any) []map[string]any {
	t.Helper()
	models, ok := data["models"].([]map[string]any)
	require.True(t, ok, "models missing from report payload")
	return models
}

func TestReportUnpricedAIModels_ConcurrentReplicas(t *testing.T) {
	t.Parallel()

	ctx, logger, db, _, notifEnq, clk := setup(t)
	seedOwner(t, db)
	initiator := dbgen.User(t, db, database.User{})
	provider := seedProvider(t, db, "anthropic", database.AIProviderTypeAnthropic)
	seedUnpricedModelUsage(t, db, initiator, provider, "claude-opus-4-8", clk.Now(), 100)

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runReport(ctx, logger, db, database.LockIDNotifyUnpricedAIModels, "unpriced AI models",
				func(tx database.Store) error {
					return reportUnpricedAIModels(ctx, logger, tx, notifEnq, clk)
				})
		}()
	}
	wg.Wait()

	require.Len(t, notifEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIModelsUnpricedReport)), 1)
}

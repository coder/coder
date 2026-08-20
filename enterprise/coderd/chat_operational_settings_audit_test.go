package coderd_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

type chatOperationalSettingsAuditFixture struct {
	db        database.Store
	client    *codersdk.ExperimentalClient
	ctx       context.Context
	systemCtx context.Context
}

type chatOperationalSettingAuditCase struct {
	name             string
	key              string
	diffField        string
	oldValue         string
	newValue         string
	effectiveDefault string
	write            func(context.Context, *codersdk.ExperimentalClient, string) error
}

func newChatOperationalSettingsAuditFixture(t *testing.T) chatOperationalSettingsAuditFixture {
	t.Helper()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
	ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			Auditor:  auditor,
			DeploymentValues: coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
				values.Experiments = []string{string(codersdk.ExperimentChatVirtualDesktop)}
			}),
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAuditLog: 1},
		},
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	return chatOperationalSettingsAuditFixture{
		db:        db,
		client:    codersdk.NewExperimentalClient(ownerClient),
		ctx:       ctx,
		systemCtx: dbauthz.AsSystemRestricted(ctx),
	}
}

func (f chatOperationalSettingsAuditFixture) auditLogs(t *testing.T) []database.GetAuditLogsOffsetRow {
	t.Helper()

	rows, err := f.db.GetAuditLogsOffset(f.systemCtx, database.GetAuditLogsOffsetParams{
		ResourceType: string(database.ResourceTypeChatOperationalSettings),
		LimitOpt:     10,
	})
	require.NoError(t, err)
	return rows
}

func (f chatOperationalSettingsAuditFixture) onlyAuditLog(t *testing.T) database.AuditLog {
	t.Helper()

	rows := f.auditLogs(t)
	require.Len(t, rows, 1)
	return rows[0].AuditLog
}

func requireChatOperationalSettingsAuditDiff(t *testing.T, log database.AuditLog, field, oldValue, newValue string) {
	t.Helper()

	var diff audit.Map
	require.NoError(t, json.Unmarshal(log.Diff, &diff))
	require.Equal(t, audit.Map{
		field: {Old: oldValue, New: newValue},
	}, diff)
}

func TestChatOperationalSettingsAudit(t *testing.T) {
	t.Parallel()

	settings := []chatOperationalSettingAuditCase{
		{
			name:             "RetentionDays",
			key:              "agents_chat_retention_days",
			diffField:        "chat_retention_days",
			oldValue:         "30",
			newValue:         "47",
			effectiveDefault: "30",
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				parsed, err := strconv.ParseInt(value, 10, 32)
				if err != nil {
					return err
				}
				return client.UpdateChatRetentionDays(ctx, codersdk.UpdateChatRetentionDaysRequest{RetentionDays: int32(parsed)})
			},
		},
		{
			name:             "DebugRetentionDays",
			key:              "agents_chat_debug_retention_days",
			diffField:        "chat_debug_retention_days",
			oldValue:         "7",
			newValue:         "47",
			effectiveDefault: strconv.FormatInt(int64(codersdk.DefaultChatDebugRetentionDays), 10),
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				parsed, err := strconv.ParseInt(value, 10, 32)
				if err != nil {
					return err
				}
				return client.UpdateChatDebugRetentionDays(ctx, codersdk.UpdateChatDebugRetentionDaysRequest{DebugRetentionDays: int32(parsed)})
			},
		},
		{
			name:             "AutoArchiveDays",
			key:              "agents_chat_auto_archive_days",
			diffField:        "chat_auto_archive_days",
			oldValue:         "14",
			newValue:         "47",
			effectiveDefault: strconv.FormatInt(int64(codersdk.DefaultChatAutoArchiveDays), 10),
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				parsed, err := strconv.ParseInt(value, 10, 32)
				if err != nil {
					return err
				}
				return client.UpdateChatAutoArchiveDays(ctx, codersdk.UpdateChatAutoArchiveDaysRequest{AutoArchiveDays: int32(parsed)})
			},
		},
		{
			name:             "WorkspaceTTL",
			key:              "agents_workspace_ttl",
			diffField:        "workspace_ttl",
			oldValue:         time.Hour.String(),
			newValue:         (2 * time.Hour).String(),
			effectiveDefault: "0s",
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				ttl, err := time.ParseDuration(value)
				if err != nil {
					return err
				}
				return client.UpdateChatWorkspaceTTL(ctx, codersdk.UpdateChatWorkspaceTTLRequest{
					WorkspaceTTLMillis: ttl.Milliseconds(),
				})
			},
		},
		{
			name:      "ComputerUseProvider",
			key:       "agents_computer_use_provider",
			diffField: "computer_use_provider",
			oldValue:  string(codersdk.ChatComputerUseProviderAnthropic),
			newValue:  string(codersdk.ChatComputerUseProviderOpenAI),
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				return client.UpdateChatComputerUseProvider(ctx, codersdk.UpdateChatComputerUseProviderRequest{
					Provider: codersdk.ChatComputerUseProvider(value),
				})
			},
		},
		{
			name:             "DebugLogging",
			key:              "agents_chat_debug_logging_allow_users",
			diffField:        "debug_logging_allow_users",
			oldValue:         "false",
			newValue:         "true",
			effectiveDefault: "false",
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				allowUsers, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}
				return client.UpdateChatDebugLogging(ctx, codersdk.UpdateChatDebugLoggingAllowUsersRequest{AllowUsers: allowUsers})
			},
		},
		{
			name:             "PersonalModelOverrides",
			key:              "agents_chat_personal_model_overrides_enabled",
			diffField:        "personal_model_overrides_enabled",
			oldValue:         "false",
			newValue:         "true",
			effectiveDefault: "false",
			write: func(ctx context.Context, client *codersdk.ExperimentalClient, value string) error {
				allowUsers, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}
				return client.UpdateChatPersonalModelOverridesAdminSettings(ctx, codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest{
					AllowUsers: allowUsers,
				})
			},
		},
	}
	retentionDays := settings[0]
	computerUseProvider := settings[4]

	for _, setting := range settings {
		t.Run(setting.name, func(t *testing.T) {
			t.Parallel()

			fixture := newChatOperationalSettingsAuditFixture(t)
			require.NoError(t, fixture.db.UpsertRuntimeConfig(fixture.systemCtx, database.UpsertRuntimeConfigParams{
				Key:   setting.key,
				Value: setting.oldValue,
			}))
			require.NoError(t, setting.write(fixture.ctx, fixture.client, setting.newValue))

			log := fixture.onlyAuditLog(t)
			requireChatOperationalSettingsAuditDiff(t, log, setting.diffField, setting.oldValue, setting.newValue)
		})

		if setting.effectiveDefault == "" {
			continue
		}
		t.Run(setting.name+"EffectiveDefault", func(t *testing.T) {
			t.Parallel()

			fixture := newChatOperationalSettingsAuditFixture(t)
			require.NoError(t, fixture.db.DeleteRuntimeConfig(fixture.systemCtx, setting.key))
			require.NoError(t, setting.write(fixture.ctx, fixture.client, setting.effectiveDefault))

			stored, err := fixture.db.GetChatSiteConfigValue(fixture.systemCtx, setting.key)
			require.NoError(t, err)
			require.Equal(t, database.GetChatSiteConfigValueRow{}, stored)
			require.Empty(t, fixture.auditLogs(t))
		})
	}

	t.Run("ComputerUseProviderEmptyEffectiveDefault", func(t *testing.T) {
		t.Parallel()

		fixture := newChatOperationalSettingsAuditFixture(t)
		require.NoError(t, fixture.db.DeleteRuntimeConfig(fixture.systemCtx, computerUseProvider.key))
		require.NoError(t, computerUseProvider.write(fixture.ctx, fixture.client, computerUseProvider.newValue))

		log := fixture.onlyAuditLog(t)
		requireChatOperationalSettingsAuditDiff(t, log, computerUseProvider.diffField, "", computerUseProvider.newValue)
	})

	t.Run("CommonMetadata", func(t *testing.T) {
		t.Parallel()

		fixture := newChatOperationalSettingsAuditFixture(t)
		require.NoError(t, fixture.db.UpsertRuntimeConfig(fixture.systemCtx, database.UpsertRuntimeConfigParams{
			Key:   retentionDays.key,
			Value: retentionDays.oldValue,
		}))
		require.NoError(t, retentionDays.write(fixture.ctx, fixture.client, retentionDays.newValue))

		log := fixture.onlyAuditLog(t)
		require.Equal(t, database.AuditActionWrite, log.Action)
		require.Equal(t, database.ResourceTypeChatOperationalSettings, log.ResourceType)
		require.Empty(t, log.ResourceTarget)
		require.NotEqual(t, uuid.Nil, log.ResourceID)
		require.Equal(t, uuid.Nil, log.OrganizationID)
		require.EqualValues(t, 204, log.StatusCode)
	})

	t.Run("IdenticalStoredValue", func(t *testing.T) {
		t.Parallel()

		fixture := newChatOperationalSettingsAuditFixture(t)
		require.NoError(t, fixture.db.UpsertRuntimeConfig(fixture.systemCtx, database.UpsertRuntimeConfigParams{
			Key:   retentionDays.key,
			Value: retentionDays.newValue,
		}))
		require.NoError(t, retentionDays.write(fixture.ctx, fixture.client, retentionDays.newValue))

		stored, err := fixture.db.GetChatSiteConfigValue(fixture.systemCtx, retentionDays.key)
		require.NoError(t, err)
		require.Equal(t, database.GetChatSiteConfigValueRow{Value: retentionDays.newValue, Exists: true}, stored)
		require.Empty(t, fixture.auditLogs(t))
	})

	t.Run("InvalidWrite", func(t *testing.T) {
		t.Parallel()

		fixture := newChatOperationalSettingsAuditFixture(t)
		err := fixture.client.UpdateChatRetentionDays(fixture.ctx, codersdk.UpdateChatRetentionDaysRequest{RetentionDays: -1})
		require.Error(t, err)

		log := fixture.onlyAuditLog(t)
		require.EqualValues(t, 400, log.StatusCode)
		require.JSONEq(t, "{}", string(log.Diff))
	})

	t.Run("MalformedRawValueRepair", func(t *testing.T) {
		t.Parallel()

		fixture := newChatOperationalSettingsAuditFixture(t)
		const malformed = "not-a-number"
		require.NoError(t, fixture.db.UpsertRuntimeConfig(fixture.systemCtx, database.UpsertRuntimeConfigParams{
			Key:   retentionDays.key,
			Value: malformed,
		}))
		require.NoError(t, retentionDays.write(fixture.ctx, fixture.client, "60"))

		log := fixture.onlyAuditLog(t)
		requireChatOperationalSettingsAuditDiff(t, log, retentionDays.diffField, malformed, "60")
	})
}

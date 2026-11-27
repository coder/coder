package coderd_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

type chatAuditFixture struct {
	ps        dbpubsub.Pubsub
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

func newChatAuditFixture(t *testing.T) chatAuditFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	return newChatAuditFixtureWithDB(t, db, ps)
}

func newChatAuditFixtureWithDB(t *testing.T, db database.Store, ps dbpubsub.Pubsub) chatAuditFixture {
	t.Helper()
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
	ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			Auditor:  auditor,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAuditLog: 1},
		},
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	return chatAuditFixture{
		ps:        ps,
		db:        db,
		client:    codersdk.NewExperimentalClient(ownerClient),
		ctx:       ctx,
		systemCtx: dbauthz.AsSystemRestricted(ctx),
	}
}

func (f chatAuditFixture) auditLogs(t *testing.T, resourceType database.ResourceType) []database.GetAuditLogsOffsetRow {
	t.Helper()

	rows, err := f.db.GetAuditLogsOffset(f.systemCtx, database.GetAuditLogsOffsetParams{
		ResourceType: string(resourceType),
		LimitOpt:     10,
	})
	require.NoError(t, err)
	return rows
}

func (f chatAuditFixture) onlyAuditLog(t *testing.T) database.AuditLog {
	t.Helper()

	rows := f.auditLogs(t, database.ResourceTypeChatOperationalSettings)
	require.Len(t, rows, 1)
	return rows[0].AuditLog
}

func requireChatAuditDiff(t *testing.T, log database.AuditLog, want audit.Map) {
	t.Helper()

	var diff audit.Map
	require.NoError(t, json.Unmarshal(log.Diff, &diff))
	require.Equal(t, want, diff)
}

func requireOperationalSettingsWrite(t *testing.T, log database.AuditLog, diff audit.Map) {
	t.Helper()
	requireChatAuditDiff(t, log, diff)

	require.Equal(t, database.AuditActionWrite, log.Action)
	require.Equal(t, database.ResourceTypeChatOperationalSettings, log.ResourceType)
	require.Empty(t, log.ResourceTarget)
	require.NotEqual(t, uuid.Nil, log.ResourceID)
	require.Equal(t, uuid.Nil, log.OrganizationID)
	require.EqualValues(t, 204, log.StatusCode)
}

type unavailableAdvisorBaseline struct{ database.Store }

func (unavailableAdvisorBaseline) GetChatAdvisorConfig(context.Context) (string, error) {
	return "", xerrors.New("advisor baseline unavailable")
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

	for _, setting := range settings {
		t.Run(setting.name, func(t *testing.T) {
			t.Parallel()

			fixture := newChatAuditFixture(t)
			require.NoError(t, fixture.db.UpsertRuntimeConfig(fixture.systemCtx, database.UpsertRuntimeConfigParams{
				Key:   setting.key,
				Value: setting.oldValue,
			}))
			require.NoError(t, setting.write(fixture.ctx, fixture.client, setting.newValue))

			log := fixture.onlyAuditLog(t)
			requireOperationalSettingsWrite(t, log, audit.Map{setting.diffField: {Old: setting.oldValue, New: setting.newValue}})
		})

		if setting.effectiveDefault == "" {
			continue
		}
		t.Run(setting.name+"EffectiveDefault", func(t *testing.T) {
			t.Parallel()

			fixture := newChatAuditFixture(t)
			require.NoError(t, fixture.db.DeleteRuntimeConfig(fixture.systemCtx, setting.key))
			require.NoError(t, setting.write(fixture.ctx, fixture.client, setting.effectiveDefault))

			stored, err := fixture.db.GetChatSiteConfigValue(fixture.systemCtx, setting.key)
			require.NoError(t, err)
			require.Equal(t, database.GetChatSiteConfigValueRow{}, stored)
			require.Empty(t, fixture.auditLogs(t, database.ResourceTypeChatOperationalSettings))
		})
	}

	t.Run("Advisor", func(t *testing.T) {
		t.Parallel()

		f := newChatAuditFixture(t)
		require.NoError(t, f.db.UpsertChatAdvisorConfig(f.systemCtx, `{"max_uses_per_run":2,"max_output_tokens":256}`))
		req := codersdk.UpdateAdvisorConfigRequest{MaxUsesPerRun: 4, MaxOutputTokens: 512}
		events := make(chan []byte, 2)
		cancel, err := f.ps.Subscribe(pubsub.ChatConfigEventChannel, func(ctx context.Context, message []byte) {
			select {
			case events <- message:
			case <-ctx.Done():
			}
		})
		require.NoError(t, err)
		t.Cleanup(cancel)
		// An identical PUT still notifies chat replicas, but adds no audit row.
		for range 2 {
			require.NoError(t, f.client.UpdateChatAdvisorConfig(f.ctx, req))
			var event pubsub.ChatConfigEvent
			require.NoError(t, json.Unmarshal(testutil.TryReceive(f.ctx, t, events), &event))
			require.Equal(t, pubsub.ChatConfigEventAdvisorConfig, event.Kind)
		}

		log := f.onlyAuditLog(t)
		requireOperationalSettingsWrite(t, log, audit.Map{
			"advisor_max_uses_per_run":  {Old: float64(2), New: float64(4)},
			"advisor_max_output_tokens": {Old: float64(256), New: float64(512)},
		})
	})

	for _, tc := range []struct {
		name     string
		failRead bool
	}{
		{name: "MalformedAdvisorConfigRepair"},
		{name: "UnavailableAdvisorConfig", failRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, ps := dbtestutil.NewDB(t)
			store := db
			if tc.failRead {
				store = unavailableAdvisorBaseline{db}
			} else {
				require.NoError(t, db.UpsertChatAdvisorConfig(dbauthz.AsSystemRestricted(t.Context()), "not JSON"))
			}
			f := newChatAuditFixtureWithDB(t, store, ps)
			req := codersdk.UpdateAdvisorConfigRequest{MaxUsesPerRun: 3, MaxOutputTokens: 256}
			require.NoError(t, f.client.UpdateChatAdvisorConfig(f.ctx, req))
			raw, err := db.GetChatAdvisorConfig(f.systemCtx)
			require.NoError(t, err)
			var stored codersdk.UpdateAdvisorConfigRequest
			require.NoError(t, json.Unmarshal([]byte(raw), &stored))
			require.Equal(t, req, stored)
			requireOperationalSettingsWrite(t, f.onlyAuditLog(t), audit.Map{})
		})
	}

	t.Run("InvalidWrite", func(t *testing.T) {
		t.Parallel()

		fixture := newChatAuditFixture(t)
		err := fixture.client.UpdateChatRetentionDays(fixture.ctx, codersdk.UpdateChatRetentionDaysRequest{RetentionDays: -1})
		require.Error(t, err)

		log := fixture.onlyAuditLog(t)
		require.EqualValues(t, 400, log.StatusCode)
		require.JSONEq(t, "{}", string(log.Diff))
	})

	t.Run("MalformedRawValueRepair", func(t *testing.T) {
		t.Parallel()

		fixture := newChatAuditFixture(t)
		const malformed = "not-a-number"
		require.NoError(t, fixture.db.UpsertRuntimeConfig(fixture.systemCtx, database.UpsertRuntimeConfigParams{
			Key:   retentionDays.key,
			Value: malformed,
		}))
		require.NoError(t, retentionDays.write(fixture.ctx, fixture.client, "60"))

		log := fixture.onlyAuditLog(t)
		requireOperationalSettingsWrite(t, log, audit.Map{retentionDays.diffField: {Old: malformed, New: "60"}})
	})
}

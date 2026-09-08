package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

//nolint:tparallel,paralleltest // Subtests share one audit fixture at a time and run sequentially.
func TestChatModelConfigAudit(t *testing.T) {
	t.Parallel()

	newFixture := func(t *testing.T) (context.Context, *codersdk.ExperimentalClient, *audit.MockAuditor, codersdk.CreateFirstUserResponse, codersdk.AIProvider) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMockWithDiffFn(func(old, newVal any) audit.Map {
			oldConfig, oldOK := old.(database.ChatModelConfig)
			newConfig, newOK := newVal.(database.ChatModelConfig)
			if !oldOK || !newOK {
				return audit.Map{}
			}
			return audit.Map{
				"is_default": {Old: oldConfig.IsDefault, New: newConfig.IsDefault},
			}
		})
		client := newChatClient(t, func(opts *coderdtest.Options) {
			opts.Auditor = mAudit
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		provider := createAIProviderForTest(t, client, "openai", "test-api-key")
		mAudit.ResetLogs()
		return ctx, client, mAudit, firstUser, provider
	}

	createModel := func(t *testing.T, ctx context.Context, client *codersdk.ExperimentalClient, organizationID, providerID uuid.UUID, displayName string, isDefault bool) codersdk.ChatModel {
		t.Helper()
		contextLimit := int64(4096)
		model, err := client.CreateChatModel(ctx, organizationID, codersdk.CreateChatModelRequest{
			AIProviderID: &providerID,
			Model:        "audit-model-" + uuid.NewString(),
			DisplayName:  displayName,
			ContextLimit: &contextLimit,
			IsDefault:    &isDefault,
		})
		require.NoError(t, err)
		return model
	}

	findLog := func(t *testing.T, logs []database.AuditLog, id uuid.UUID, action database.AuditAction) database.AuditLog {
		t.Helper()
		for _, log := range logs {
			if log.ResourceID == id && log.Action == action {
				return log
			}
		}
		t.Fatalf("audit log not found for %s %s", id, action)
		return database.AuditLog{}
	}

	isDefaultDiff := func(t *testing.T, log database.AuditLog, oldValue, newValue bool) {
		t.Helper()
		var diff map[string]codersdk.AuditDiffField
		require.NoError(t, json.Unmarshal(log.Diff, &diff))
		require.Equal(t, codersdk.AuditDiffField{Old: oldValue, New: newValue}, diff["is_default"])
	}

	t.Run("Create", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		model := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Audited Model", false)
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 1)
		log := findLog(t, logs, model.ID, database.AuditActionCreate)
		require.Equal(t, database.ResourceTypeChatModelConfig, log.ResourceType)
		require.Equal(t, "Audited Model", log.ResourceTarget)
		require.Equal(t, firstUser.UserID, log.UserID)
		require.Equal(t, firstUser.OrganizationID, log.OrganizationID)
		require.EqualValues(t, http.StatusCreated, log.StatusCode)
	})

	t.Run("CreateTargetFallbackUsesID", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		model := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "", false)
		log := findLog(t, mAudit.AuditLogs(), model.ID, database.AuditActionCreate)
		require.Equal(t, model.ID.String(), log.ResourceTarget)
		require.NotEqual(t, model.Model, log.ResourceTarget)
	})

	t.Run("CreateDefaultDemotesSibling", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		first := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "First", false)
		mAudit.ResetLogs()
		second := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Second", true)
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 2)
		isDefaultDiff(t, findLog(t, logs, first.ID, database.AuditActionWrite), true, false)
		findLog(t, logs, second.ID, database.AuditActionCreate)
	})

	t.Run("UpdatePromoteDemotesSibling", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		first := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "First", false)
		second := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Second", false)
		mAudit.ResetLogs()
		_, err := client.UpdateChatModel(ctx, firstUser.OrganizationID, second.ID, codersdk.UpdateChatModelRequest{IsDefault: ptr.Ref(true)})
		require.NoError(t, err)
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 2)
		isDefaultDiff(t, findLog(t, logs, second.ID, database.AuditActionWrite), false, true)
		isDefaultDiff(t, findLog(t, logs, first.ID, database.AuditActionWrite), true, false)
	})

	t.Run("UpdateDemotePromotesSibling", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		first := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "First", false)
		second := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Second", false)
		mAudit.ResetLogs()
		_, err := client.UpdateChatModel(ctx, firstUser.OrganizationID, first.ID, codersdk.UpdateChatModelRequest{IsDefault: ptr.Ref(false)})
		require.NoError(t, err)
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 2)
		isDefaultDiff(t, findLog(t, logs, first.ID, database.AuditActionWrite), true, false)
		isDefaultDiff(t, findLog(t, logs, second.ID, database.AuditActionWrite), false, true)
	})

	t.Run("UpdateSoleDemoteRemainsDefault", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		model := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Only", false)
		mAudit.ResetLogs()
		updated, err := client.UpdateChatModel(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelRequest{IsDefault: ptr.Ref(false)})
		require.NoError(t, err)
		require.True(t, updated.IsDefault)
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 1)
		isDefaultDiff(t, findLog(t, logs, model.ID, database.AuditActionWrite), true, true)
	})

	t.Run("DeleteNonDefault", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		_ = createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "First", false)
		second := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Second", false)
		mAudit.ResetLogs()
		require.NoError(t, client.DeleteChatModel(ctx, firstUser.OrganizationID, second.ID))
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 1)
		findLog(t, logs, second.ID, database.AuditActionDelete)
	})

	t.Run("DeleteDefaultPromotesSibling", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		first := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "First", false)
		second := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Second", false)
		mAudit.ResetLogs()
		require.NoError(t, client.DeleteChatModel(ctx, firstUser.OrganizationID, first.ID))
		logs := mAudit.AuditLogs()
		require.Len(t, logs, 2)
		findLog(t, logs, first.ID, database.AuditActionDelete)
		isDefaultDiff(t, findLog(t, logs, second.ID, database.AuditActionWrite), false, true)
	})

	t.Run("DeniedUpdateHasNoAudit", func(t *testing.T) {
		ctx, client, mAudit, firstUser, provider := newFixture(t)
		model := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Denied", false)
		memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		member := codersdk.NewExperimentalClient(memberRaw)
		mAudit.ResetLogs()
		_, err := member.UpdateChatModel(ctx, firstUser.OrganizationID, model.ID, codersdk.UpdateChatModelRequest{DisplayName: "Nope"})
		requireSDKError(t, err, http.StatusNotFound)
		require.Empty(t, mAudit.AuditLogs())
	})

	t.Run("RollbackHasNoSiblingAudit", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMock()
		rawDB, pubsub := dbtestutil.NewDB(t)
		store := newChatModelConfigHookStore(rawDB)
		client := newChatClient(t, func(opts *coderdtest.Options) {
			opts.Auditor = mAudit
			opts.Database = store
			opts.Pubsub = pubsub
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		provider := createAIProviderForTest(t, client, "openai", "test-api-key")
		first := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "First", false)
		second := createModel(t, ctx, client, firstUser.OrganizationID, provider.ID, "Second", false)
		mAudit.ResetLogs()
		store.armFailNextUpdate(second.ID)
		_, err := client.UpdateChatModel(ctx, firstUser.OrganizationID, second.ID, codersdk.UpdateChatModelRequest{IsDefault: ptr.Ref(true)})
		requireSDKError(t, err, http.StatusNotFound)
		for _, log := range mAudit.AuditLogs() {
			require.NotEqual(t, first.ID, log.ResourceID)
			require.NotEqualValues(t, http.StatusOK, log.StatusCode)
		}
	})
}

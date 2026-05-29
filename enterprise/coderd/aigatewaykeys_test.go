package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aigatewaykey"
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

func TestAIGatewayKeys(t *testing.T) {
	t.Parallel()

	// Single instance shared by all subtests (except FeatureGate).
	// Subtests run sequentially because they share server state.
	ctx := testutil.Context(t, testutil.WaitLong)
	ownerClient, owner := coderdenttest.New(t, aibridgeOpts(t))

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("CRUD", func(t *testing.T) {
		keys, err := ownerClient.ListAIGatewayKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)

		name := uniqueName(t, "happy")

		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.ID)
		require.Equal(t, name, created.Name)
		require.Len(t, created.KeyPrefix, aigatewaykey.KeyPrefixLength)
		require.Len(t, created.Key, aigatewaykey.KeyLength)
		require.True(t, strings.HasPrefix(created.Key, created.KeyPrefix), "key must begin with key_prefix")
		require.WithinDuration(t, time.Now(), created.CreatedAt, time.Minute)

		keys, err = ownerClient.ListAIGatewayKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, created.ID, keys[0].ID)
		require.Equal(t, created.Name, keys[0].Name)
		require.Equal(t, created.KeyPrefix, keys[0].KeyPrefix)
		require.Nil(t, keys[0].LastUsedAt)

		require.NoError(t, ownerClient.DeleteAIGatewayKey(ctx, created.ID))

		keys, err = ownerClient.ListAIGatewayKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("ListResponseDoesNotLeakSecrets", func(t *testing.T) {
		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{
			Name: uniqueName(t, "leak"),
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ownerClient.DeleteAIGatewayKey(ctx, created.ID)
		})

		// Raw HTTP read of LIST to confirm the JSON shape.
		resp, err := ownerClient.Request(ctx, http.MethodGet, "/api/v2/aibridge/keys", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var raw []map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
		require.NotEmpty(t, raw)
		_, hasSecret := raw[0]["secret"]
		_, hasHashed := raw[0]["hashed_secret"]
		require.False(t, hasSecret, "LIST response leaked plaintext secret")
		require.False(t, hasHashed, "LIST response leaked hashed_secret")
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("CreateValidation", func(t *testing.T) {
		// Empty name -> 400 (validate:"required" on request struct).
		_, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: ""})
		require.ErrorContains(t, err, "Validation failed")

		// >64 char name -> 400 (DB check constraint).
		longName := strings.Repeat("a", 65)
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: longName})
		require.ErrorContains(t, err, "Invalid key name")

		// Uppercase name -> 400 (DB check constraint rejects non-lowercase).
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "UPPER-CASE"})
		require.ErrorContains(t, err, "Invalid key name")

		// Duplicate name -> 400.
		name := uniqueName(t, "dup")
		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ownerClient.DeleteAIGatewayKey(ctx, created.ID)
		})
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
		require.ErrorContains(t, err, "must be unique")
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("DeleteValidation", func(t *testing.T) {
		// Invalid UUID -> 400 (raw request; SDK method accepts uuid.UUID).
		resp, err := ownerClient.Request(ctx, http.MethodDelete, "/api/v2/aibridge/keys/not-a-uuid", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Delete existing key -> 204 (SDK returns nil error on 204).
		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{
			Name: uniqueName(t, "del"),
		})
		require.NoError(t, err)
		require.NoError(t, ownerClient.DeleteAIGatewayKey(ctx, created.ID))

		// Unknown UUID -> 404.
		err = ownerClient.DeleteAIGatewayKey(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	//nolint:paralleltest // Subtests share a single coderdenttest instance.
	t.Run("ReturnsForbiddenForNonOwners", func(t *testing.T) {
		member, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		_, err := member.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{
			Name: uniqueName(t, "denied"),
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		_, err = member.ListAIGatewayKeys(ctx)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		err = member.DeleteAIGatewayKey(ctx, uuid.New())
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	// FeatureGate needs a separate instance without the AI Bridge entitlement.
	t.Run("FeatureGate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})

		//nolint:gocritic // Managing AI Gateway coderd keys is owner-only.
		_, err := ownerClient.ListAIGatewayKeys(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})
}

func TestAIGatewayKeyAudit(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(
		db,
		entaudit.DefaultFilter,
		backends.NewPostgres(db, true),
	)
	opts := aibridgeOpts(t)
	opts.AuditLogging = true
	opts.Options.Database = db
	opts.Options.Pubsub = ps
	opts.Options.Auditor = auditor
	opts.LicenseOptions.Features[codersdk.FeatureAuditLog] = 1

	ownerClient, _ := coderdenttest.New(t, opts)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	name := uniqueName(t, "audit")
	//nolint:gocritic // Managing AI Gateway coderd keys is owner-only.
	created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
	require.NoError(t, err)
	//nolint:gocritic // Managing AI Gateway coderd keys is owner-only.
	require.NoError(t, ownerClient.DeleteAIGatewayKey(ctx, created.ID))

	rows, err := db.GetAuditLogsOffset(
		dbauthz.AsSystemRestricted(ctx),
		database.GetAuditLogsOffsetParams{
			ResourceType: string(database.ResourceTypeAIGatewayKey),
			LimitOpt:     10,
		},
	)
	require.NoError(t, err)
	require.Len(t, rows, 2, "expected one create and one delete audit row")

	var createLog, deleteLog database.AuditLog
	for _, row := range rows {
		log := row.AuditLog
		switch log.Action {
		case database.AuditActionCreate:
			createLog = log
		case database.AuditActionDelete:
			deleteLog = log
		default:
			require.Failf(t, "unexpected audit action", "action: %s", log.Action)
		}
	}
	require.Equal(t, database.AuditActionCreate, createLog.Action)
	require.Equal(t, database.AuditActionDelete, deleteLog.Action)
	require.Equal(t, http.StatusCreated, int(createLog.StatusCode))
	require.Equal(t, http.StatusNoContent, int(deleteLog.StatusCode))

	for _, log := range []database.AuditLog{createLog, deleteLog} {
		require.Equal(t, database.ResourceTypeAIGatewayKey, log.ResourceType)
		require.Equal(t, created.ID, log.ResourceID)
		require.Equal(t, name, log.ResourceTarget)
	}

	var createDiff audit.Map
	require.NoError(t, json.Unmarshal(createLog.Diff, &createDiff))
	require.Contains(t, createDiff, "name")
	require.Equal(t, "", createDiff["name"].Old)
	require.Equal(t, name, createDiff["name"].New)
	require.Contains(t, createDiff, "secret_prefix")
	require.Equal(t, "", createDiff["secret_prefix"].Old)
	require.Equal(t, created.KeyPrefix, createDiff["secret_prefix"].New)
	require.NotContains(t, createDiff, "hashed_secret")

	var deleteDiff audit.Map
	require.NoError(t, json.Unmarshal(deleteLog.Diff, &deleteDiff))
	require.Contains(t, deleteDiff, "name")
	require.Equal(t, name, deleteDiff["name"].Old)
	require.Equal(t, "", deleteDiff["name"].New)
	require.NotContains(t, deleteDiff, "hashed_secret")
}

func uniqueName(t *testing.T, prefix string) string {
	t.Helper()
	return strings.ToLower(fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()))
}

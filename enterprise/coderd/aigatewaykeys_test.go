package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	aibridgekeys "github.com/coder/coder/v2/coderd/aibridge/keys"
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

	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI Gateway keys is owner-only.
		keys, err := ownerClient.ListAIGatewayKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)

		name := uniqueName(t, "happy")

		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.ID)
		require.Equal(t, name, created.Name)
		require.Len(t, created.KeyPrefix, aibridgekeys.KeyPrefixLength)
		require.Len(t, created.Key, aibridgekeys.KeyLength)
		require.True(t, strings.HasPrefix(created.Key, created.KeyPrefix), "key must begin with key_prefix")
		require.WithinDuration(t, time.Now(), created.CreatedAt, time.Minute)

		keys, err = ownerClient.ListAIGatewayKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, created.ID, keys[0].ID)
		require.Equal(t, created.Name, keys[0].Name)
		require.Equal(t, created.KeyPrefix, keys[0].KeyPrefix)
		require.Nil(t, keys[0].LastHeartbeatAt)

		require.NoError(t, ownerClient.DeleteAIGatewayKey(ctx, created.ID))

		keys, err = ownerClient.ListAIGatewayKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	t.Run("ListResponseDoesNotLeakSecrets", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI Gateway keys is owner-only.
		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{
			Name: uniqueName(t, "leak"),
		})
		require.NoError(t, err)
		fullKey := created.Key

		resp, err := ownerClient.Request(ctx, http.MethodGet, "/api/v2/ai-gateway/keys", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.NotContains(t, string(body), fullKey, "LIST response leaked full key")
	})

	t.Run("CreateValidation", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		// Empty name -> 400 (validate:"required" on request struct).
		//nolint:gocritic // Managing AI Gateway keys is owner-only.
		_, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: ""})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.ErrorContains(t, err, "Validation failed")

		// >64 char name -> 400 (DB check constraint).
		longName := strings.Repeat("a", 65)
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: longName})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.ErrorContains(t, err, "Invalid key name")

		// Uppercase name -> 400 (DB check constraint rejects non-lowercase).
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: "UPPER-CASE"})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.ErrorContains(t, err, "Invalid key name")

		// Duplicate name -> 400.
		name := uniqueName(t, "dup")
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
		require.NoError(t, err)
		_, err = ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: name})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.ErrorContains(t, err, "must be unique")
	})

	t.Run("DeleteValidation", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		// Invalid UUID -> 400 (raw request; SDK method accepts uuid.UUID).
		//nolint:gocritic // Managing AI Gateway keys is owner-only.
		resp, err := ownerClient.Request(ctx, http.MethodDelete, "/api/v2/ai-gateway/keys/not-a-uuid", nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Existing id -> 204.
		created, err := ownerClient.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{
			Name: uniqueName(t, "del"),
		})
		require.NoError(t, err)
		// SDK returns no code on success, using raw request to check for 204.
		delResp, err := ownerClient.Request(ctx, http.MethodDelete, "/api/v2/ai-gateway/keys/"+created.ID.String(), nil)
		require.NoError(t, err)
		defer delResp.Body.Close()
		require.Equal(t, http.StatusNoContent, delResp.StatusCode)

		// Not existing id -> 404.
		err = ownerClient.DeleteAIGatewayKey(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("ReturnsForbiddenForNonOwners", func(t *testing.T) {
		t.Parallel()

		ownerClient, owner := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)
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

	t.Run("LicenseEntitlement", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI Gateway keys is owner-only.
		_, err := ownerClient.ListAIGatewayKeys(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "AI Gateway is a Premium feature")
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

// aiGatewayKeyErrorStore wraps a database.Store and forces specific
// methods to return errors, allowing tests to exercise error paths.
type aiGatewayKeyErrorStore struct {
	database.Store
	insertErr error
	listErr   error
	deleteErr error
}

func (s *aiGatewayKeyErrorStore) InsertAIGatewayKey(ctx context.Context, arg database.InsertAIGatewayKeyParams) (database.InsertAIGatewayKeyRow, error) {
	if s.insertErr != nil {
		return database.InsertAIGatewayKeyRow{}, s.insertErr
	}
	return s.Store.InsertAIGatewayKey(ctx, arg)
}

func (s *aiGatewayKeyErrorStore) ListAIGatewayKeys(ctx context.Context) ([]database.ListAIGatewayKeysRow, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Store.ListAIGatewayKeys(ctx)
}

func (s *aiGatewayKeyErrorStore) DeleteAIGatewayKey(ctx context.Context, id uuid.UUID) (database.DeleteAIGatewayKeyRow, error) {
	if s.deleteErr != nil {
		return database.DeleteAIGatewayKeyRow{}, s.deleteErr
	}
	return s.Store.DeleteAIGatewayKey(ctx, id)
}

func TestAIGatewayKeysDatabaseErrors(t *testing.T) {
	t.Parallel()

	dbErr := xerrors.New("internal db failure")

	tests := []struct {
		name       string
		errStore   aiGatewayKeyErrorStore
		method     string
		path       string
		body       any
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "CreateDBError",
			errStore:   aiGatewayKeyErrorStore{insertErr: dbErr},
			method:     http.MethodPost,
			path:       "/api/v2/ai-gateway/keys",
			body:       codersdk.CreateAIGatewayKeyRequest{Name: "db-err-create"},
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Failed to create key. Please retry.",
		},
		{
			name:       "ListDBError",
			errStore:   aiGatewayKeyErrorStore{listErr: dbErr},
			method:     http.MethodGet,
			path:       "/api/v2/ai-gateway/keys",
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Failed to list keys.",
		},
		{
			name:       "DeleteDBError",
			errStore:   aiGatewayKeyErrorStore{deleteErr: dbErr},
			method:     http.MethodDelete,
			path:       "/api/v2/ai-gateway/keys/" + uuid.New().String(),
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Failed to delete key.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			errStore := tc.errStore
			errStore.Store = db

			opts := aibridgeOpts(t)
			opts.Options.Database = &errStore
			opts.Options.Pubsub = ps

			ownerClient, _ := coderdenttest.New(t, opts)
			ctx := testutil.Context(t, testutil.WaitLong)

			//nolint:gocritic // Managing AI Gateway keys is owner-only.
			resp, err := ownerClient.Request(ctx, tc.method, tc.path, tc.body)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.wantStatus, resp.StatusCode)

			var sdkResp codersdk.Response
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&sdkResp))
			require.Equal(t, tc.wantMsg, sdkResp.Message)
			require.Empty(t, sdkResp.Detail, "response must not leak internal error details")
		})
	}
}

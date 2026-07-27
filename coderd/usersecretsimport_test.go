package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestImportUserSecrets(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)
		auditor.ResetLogs()

		secrets, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "ALPHA=a\nBETA=b\nPATH=c\n",
		})
		require.NoError(t, err)
		require.Len(t, secrets, 3)
		// Valid keys are env-injected, while reserved names are imported
		// without env injection.
		assert.Equal(t, "ALPHA", secrets[0].Name)
		assert.Equal(t, "ALPHA", secrets[0].EnvName)
		assert.Equal(t, "PATH", secrets[2].Name)
		assert.Empty(t, secrets[2].EnvName)

		listed, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		names := make([]string, 0, len(listed))
		for _, s := range listed {
			names = append(names, s.Name)
		}
		assert.ElementsMatch(t, []string{"ALPHA", "BETA", "PATH"}, names)

		// Exactly one create audit log per imported secret.
		logs := auditor.AuditLogs()
		require.Len(t, logs, 3)
		resourceIDs := make([]string, 0, len(logs))
		resourceTargets := make([]string, 0, len(logs))
		for _, l := range logs {
			assert.Equal(t, database.AuditActionCreate, l.Action)
			assert.EqualValues(t, http.StatusCreated, l.StatusCode)
			resourceIDs = append(resourceIDs, l.ResourceID.String())
			resourceTargets = append(resourceTargets, l.ResourceTarget)
		}
		assert.ElementsMatch(t, []string{
			secrets[0].ID.String(), secrets[1].ID.String(), secrets[2].ID.String(),
		}, resourceIDs)
		assert.ElementsMatch(t, []string{"ALPHA", "BETA", "PATH"}, resourceTargets)
	})

	t.Run("ValuesNotInResponse", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		const secretValue = "super-secret-sentinel-value-123"
		res, err := client.Request(ctx, http.MethodPost,
			fmt.Sprintf("/api/v2/users/%s/secrets/batch", codersdk.Me),
			codersdk.ImportUserSecretsRequest{
				Format:  codersdk.SecretsFileFormatEnv,
				Content: "LEAKY=" + secretValue,
			})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(body), secretValue)
	})
}

func TestImportUserSecretsForbiddenForAnotherUser(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := memberClient.ImportUserSecrets(ctx, owner.UserID.String(), codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: "FORBIDDEN=value",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}

func TestImportUserSecretsBodyTooLarge(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: strings.Repeat("a", 8*codersdk.MaxSecretsFileBytes),
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusRequestEntityTooLarge, sdkErr.StatusCode())
}

// TestImportUserSecretsValidationRollback verifies that a single
// invalid entry rejects the whole batch: nothing is created and no
// audit log is written. The valid sibling entry must not leak through.
func TestImportUserSecretsValidationRollback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		badLine string
	}{
		// Empty values are always invalid; this is the canonical rollback case.
		{name: "EmptyValue", badLine: "EMPTY_ONE="},
		{name: "OversizedValue", badLine: "BIG=" + strings.Repeat("a", codersdk.MaxUserSecretValueBytes+1)},
		// A slash in the name is invalid regardless of env-name handling.
		{name: "NameWithSlash", badLine: "bad/name=value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			auditor := audit.NewMock()
			client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
			_ = coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitMedium)
			auditor.ResetLogs()

			_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
				Format:  codersdk.SecretsFileFormatEnv,
				Content: "GOOD_ENTRY=fine\n" + tc.badLine,
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			// Errors are attributed to the offending entry (index 1).
			require.NotEmpty(t, sdkErr.Validations)
			for _, v := range sdkErr.Validations {
				assert.Truef(t, strings.HasPrefix(v.Field, "secrets[1]."),
					"unexpected field %q", v.Field)
			}

			listed, err := client.UserSecrets(ctx, codersdk.Me)
			require.NoError(t, err)
			assert.Empty(t, listed)

			assert.Empty(t, auditor.AuditLogs())
		})
	}
}

// TestImportUserSecretsConflict verifies that a batch containing an
// already-existing secret name aborts entirely: the new entry is not
// created and no audit log is written.
func TestImportUserSecretsConflict(t *testing.T) {
	t.Parallel()
	auditor := audit.NewMock()
	client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name:  "EXISTING",
		Value: "original",
	})
	require.NoError(t, err)
	auditor.ResetLogs()

	_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: "BRANDNEW=x\nEXISTING=collision",
	})
	validation := requireSecretValidation(t, err, http.StatusConflict, "secrets[1].name")
	assert.Equal(t, `Secret "EXISTING" on line 2: Name is already in use.`, validation.Detail)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	assert.Equal(t, "Some secrets already exist.", sdkErr.Message)
	require.Len(t, sdkErr.Validations, 1)

	// Only the pre-existing secret should remain; BRANDNEW must not be created.
	listed, err := client.UserSecrets(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "EXISTING", listed[0].Name)

	assert.Empty(t, auditor.AuditLogs())
}

// TestImportUserSecretsMultipleConflicts verifies that every entry that
// collides with an existing secret is reported in one 409, rather than the
// caller discovering one collision per round-trip. The response carries a
// conflict-specific title and sentence-case details, nothing is created, and
// no audit log is written.
func TestImportUserSecretsMultipleConflicts(t *testing.T) {
	t.Parallel()

	t.Run("ThreeNameCollisions", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Created without an env_name so each imported entry collides on
		// name only, which keeps one validation per colliding key.
		for _, name := range []string{"ALPHA", "BETA", "GAMMA"} {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:  name,
				Value: "original",
			})
			require.NoError(t, err)
		}
		before, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, before, 3)
		auditor.ResetLogs()

		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "ALPHA=x\nBRANDNEW=y\nBETA=z\nGAMMA=w\n",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		assert.Equal(t, "Some secrets already exist.", sdkErr.Message)

		// All three collisions are enumerated in one response.
		got := make(map[string]string, len(sdkErr.Validations))
		for _, v := range sdkErr.Validations {
			got[v.Field] = v.Detail
		}
		require.Len(t, sdkErr.Validations, 3)
		assert.Equal(t, map[string]string{
			"secrets[0].name": `Secret "ALPHA" on line 1: Name is already in use.`,
			"secrets[2].name": `Secret "BETA" on line 3: Name is already in use.`,
			"secrets[3].name": `Secret "GAMMA" on line 4: Name is already in use.`,
		}, got)

		// Nothing was created: BRANDNEW must not leak through.
		after, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		names := make([]string, 0, len(after))
		for _, s := range after {
			names = append(names, s.Name)
		}
		assert.ElementsMatch(t, []string{"ALPHA", "BETA", "GAMMA"}, names)
		assert.Empty(t, auditor.AuditLogs())
	})

	t.Run("WholesaleReimport", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Imported secrets are env-injected under their own key, so
		// re-importing the same file collides on both the name and the
		// env_name of each stored row. Those are the same row, so each entry
		// is reported once.
		const content = "ALPHA=a\nBETA=b\nGAMMA=c\n"
		created, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
		require.NoError(t, err)
		require.Len(t, created, 3)
		for _, secret := range created {
			require.Equal(t, secret.Name, secret.EnvName)
		}

		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		assert.Equal(t, "Some secrets already exist.", sdkErr.Message)
		got := make(map[string]string, len(sdkErr.Validations))
		for _, v := range sdkErr.Validations {
			got[v.Field] = v.Detail
		}
		require.Len(t, sdkErr.Validations, 3)
		assert.Equal(t, map[string]string{
			"secrets[0].name": `Secret "ALPHA" on line 1: Name is already in use.`,
			"secrets[1].name": `Secret "BETA" on line 2: Name is already in use.`,
			"secrets[2].name": `Secret "GAMMA" on line 3: Name is already in use.`,
		}, got)
	})

	t.Run("IndependentDimensionsOnDifferentSecrets", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// One stored secret owns the name and a different one owns the
		// env_name. Deleting either leaves the other collision in place, so
		// both have to be reported or the caller pays a second round-trip to
		// discover the one that was hidden.
		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  "SHARED",
			Value: "original",
		})
		require.NoError(t, err)
		_, err = client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:    "holder",
			Value:   "original",
			EnvName: "SHARED",
		})
		require.NoError(t, err)

		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "SHARED=new\n",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		assert.Equal(t, "Some secrets already exist.", sdkErr.Message)
		// Asserted as a slice, not a map: this is the one response where two
		// dimensions of a single entry are reported, so it is where field order
		// is observable and worth pinning.
		assert.Equal(t, []codersdk.ValidationError{
			{
				Field:  "secrets[0].name",
				Detail: `Secret "SHARED" on line 1: Name is already in use.`,
			},
			{
				Field:  "secrets[0].env_name",
				Detail: `Secret "SHARED" on line 1: Environment variable name is already in use.`,
			},
		}, sdkErr.Validations)
	})

	t.Run("EnvNameOnly", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitMedium)

		// Each existing secret's name differs from the imported key, so only
		// the env_name dimension collides. Two of them collide so the
		// response has to enumerate past the first.
		for i, envName := range []string{"SHARED_ENV_ONE", "SHARED_ENV_TWO"} {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:    fmt.Sprintf("holder-%d", i),
				Value:   "original",
				EnvName: envName,
			})
			require.NoError(t, err)
		}

		_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "FINE=x\nSHARED_ENV_ONE=collision\nSHARED_ENV_TWO=collision\n",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		assert.Equal(t, "Some secrets already exist.", sdkErr.Message)
		got := make(map[string]string, len(sdkErr.Validations))
		for _, v := range sdkErr.Validations {
			got[v.Field] = v.Detail
		}
		require.Len(t, sdkErr.Validations, 2)
		assert.Equal(t, map[string]string{
			"secrets[1].env_name": `Secret "SHARED_ENV_ONE" on line 2: Environment variable name is already in use.`,
			"secrets[2].env_name": `Secret "SHARED_ENV_TWO" on line 3: Environment variable name is already in use.`,
		}, got)

		listed, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, listed, 2)
	})
}

// conflictInjectingStore inserts a secret the first time the import handler
// runs its pre-flight lookup, so the handler proceeds with a stale view and the
// conflict can only be caught by the unique indexes inside the transaction.
// This stands in for two concurrent imports, which can both pass pre-flight.
//
// The counters pin the setup rather than the outcome: they assert the injection
// landed on the import's own pre-flight lookup, that the lookup saw no rows, and
// that the name index rolled the transaction back. Because the injection happens
// after the delegated read returns, pre-flight cannot see the row by
// construction, so this test is deliberately insensitive to whether pre-flight
// rejection is present. TestImportUserSecretsMultipleConflicts is what fails if
// pre-flight rejection regresses.
type conflictInjectingStore struct {
	database.Store

	t      *testing.T
	once   sync.Once
	secret database.CreateUserSecretParams

	listCalls atomic.Int64
	// injectedOnListCall is the ListUserSecrets call number that injected the
	// conflicting row, and rowsAtInjection is how many of the user's secrets
	// that call returned. The injection must land on the import's pre-flight
	// lookup, and that lookup must see no rows.
	injectedOnListCall atomic.Int64
	rowsAtInjection    atomic.Int64
	// txNameConflicts counts transactions rolled back by the user_secrets name
	// index, which is the backstop under test. Filtering on the index keeps
	// unrelated transaction activity out of the count.
	txNameConflicts atomic.Int64
}

func (s *conflictInjectingStore) ListUserSecrets(ctx context.Context, userID uuid.UUID) ([]database.ListUserSecretsRow, error) {
	rows, err := s.Store.ListUserSecrets(ctx, userID)
	if err != nil {
		return rows, err
	}
	call := s.listCalls.Add(1)
	s.once.Do(func() {
		s.injectedOnListCall.Store(call)
		s.rowsAtInjection.Store(int64(len(rows)))
		params := s.secret
		params.ID = uuid.New()
		params.UserID = userID
		_, insertErr := s.Store.CreateUserSecret(ctx, params)
		assert.NoError(s.t, insertErr)
	})
	return rows, nil
}

func (s *conflictInjectingStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		err := fn(tx)
		if database.IsUniqueViolation(err, database.UniqueUserSecretsUserNameIndex) {
			s.txNameConflicts.Add(1)
		}
		return err
	}, opts)
}

// TestImportUserSecretsConflictDBBackstop verifies that the unique indexes
// still reject a conflict the pre-flight lookup could not see, which is the
// only thing guarding two imports racing each other. The response keeps the
// 409, the conflict title, and the sentence-case detail.
func TestImportUserSecretsConflictDBBackstop(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	store := &conflictInjectingStore{
		Store: db,
		t:     t,
		secret: database.CreateUserSecretParams{
			Name:    "RACER",
			Value:   "injected",
			EnvName: "RACER",
		},
	}
	auditor := audit.NewMock()
	client := coderdtest.New(t, &coderdtest.Options{
		Database: store,
		Pubsub:   ps,
		Auditor:  auditor,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)
	auditor.ResetLogs()

	_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: "RACER=mine\nOTHER=fine\n",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	assert.Equal(t, "Some secrets already exist.", sdkErr.Message)
	require.Len(t, sdkErr.Validations, 1)
	assert.Equal(t, "secrets[0].name", sdkErr.Validations[0].Field)
	assert.Equal(t, `Secret "RACER" on line 1: Name is already in use.`,
		sdkErr.Validations[0].Detail)

	// The import's pre-flight lookup is the first list call, it saw no rows,
	// and the conflict was raised by the unique index inside the transaction.
	assert.EqualValues(t, 1, store.listCalls.Load())
	assert.EqualValues(t, 1, store.injectedOnListCall.Load())
	assert.EqualValues(t, 0, store.rowsAtInjection.Load())
	assert.EqualValues(t, 1, store.txNameConflicts.Load())

	// The transaction rolled back, so only the injected row exists and the
	// import wrote no audit logs.
	listed, err := client.UserSecrets(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "RACER", listed[0].Name)
	assert.Empty(t, auditor.AuditLogs())
}

// TestImportUserSecretsLimits exercises each per-user cap. A cap
// tripped mid-batch must roll back every row in the import and, because
// audit logs are emitted only after the transaction commits, write no
// import audit logs.
func TestImportUserSecretsLimits(t *testing.T) {
	t.Parallel()

	t.Run("CountLimitPreflight", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Prefill to five below the cap and import ten entries. The file
		// cannot fit, and the response reports the shortfall for the whole
		// file rather than blaming whichever entry reached the cap first.
		const (
			keys     = 10
			headroom = 5
		)
		prefillUserSecrets(ctx, t, client, codersdk.MaxUserSecretsPerUserCount-headroom)
		before, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, before, codersdk.MaxUserSecretsPerUserCount-headroom)

		auditor.ResetLogs()
		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: envImportContent(keys),
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		assert.Equal(t, "User secrets limit reached.", sdkErr.Message)
		assert.Equal(t, fmt.Sprintf(
			"You have %d of %d secrets and this file contains %d, so remove at least %d.",
			codersdk.MaxUserSecretsPerUserCount-headroom, codersdk.MaxUserSecretsPerUserCount,
			keys, keys-headroom,
		), sdkErr.Response.Detail)
		// A capacity failure is about the file, not one entry.
		assert.Empty(t, sdkErr.Validations)
		assert.NotContains(t, sdkErr.Response.Detail, "secrets[")

		after, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, after, len(before))
		beforeNames := make([]string, 0, len(before))
		afterNames := make([]string, 0, len(after))
		for _, secret := range before {
			beforeNames = append(beforeNames, secret.Name)
		}
		for _, secret := range after {
			afterNames = append(afterNames, secret.Name)
		}
		assert.ElementsMatch(t, beforeNames, afterNames)
		assert.Empty(t, auditor.AuditLogs())
	})

	t.Run("CountLimitBoundary", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A file that lands exactly on the cap must still import, so the
		// pre-flight projection is not off by one.
		const keys = 10
		prefillUserSecrets(ctx, t, client, codersdk.MaxUserSecretsPerUserCount-keys)

		secrets, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: envImportContent(keys),
		})
		require.NoError(t, err)
		require.Len(t, secrets, keys)

		listed, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Len(t, listed, codersdk.MaxUserSecretsPerUserCount)

		// Now at the cap, a one-key file overflows by one.
		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "OVER_BY_ONE=x\n",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		assert.Equal(t, fmt.Sprintf(
			"You have %d of %d secrets and this file contains 1, so remove at least 1.",
			codersdk.MaxUserSecretsPerUserCount, codersdk.MaxUserSecretsPerUserCount,
		), sdkErr.Response.Detail)
	})

	t.Run("ConflictBeatsCapacity", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A file the user already holds in full needs no keys removed, so a
		// re-import at the cap must report the duplicates rather than a
		// headroom shortfall. This pins the order of the two pre-flight
		// checks: capacity runs second precisely so this case reads right.
		content := envImportContent(codersdk.MaxUserSecretsPerUserCount)
		secrets, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
		require.NoError(t, err)
		require.Len(t, secrets, codersdk.MaxUserSecretsPerUserCount)

		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		assert.Equal(t, "Some secrets already exist.", sdkErr.Message)
		assert.Len(t, sdkErr.Validations, codersdk.MaxUserSecretsPerUserCount)
		assert.NotContains(t, sdkErr.Response.Detail, "remove at least")
	})

	t.Run("EnvBytesLimit", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Every imported secret is env-injected, so two values that are
		// each within the per-value cap can still exceed the env-bytes
		// aggregate together.
		content := fmt.Sprintf("ENV_A=%s\nENV_B=%s\n",
			strings.Repeat("a", codersdk.MaxUserSecretValueBytes-16),
			strings.Repeat("a", 1024))
		auditor.ResetLogs()
		_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "env_name")

		listed, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Empty(t, listed)
		assert.Empty(t, auditor.AuditLogs())
	})

	t.Run("TotalBytesLimit", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Pre-fill the total-bytes budget to the cap using file-only
		// secrets (no env_name), which do not count against the smaller
		// env budget. Creating them via CreateUserSecret directly avoids
		// going through the import parser.
		big := strings.Repeat("a", codersdk.MaxUserSecretValueBytes)
		numBig := codersdk.MaxUserSecretsTotalValueBytes / codersdk.MaxUserSecretValueBytes
		remainder := codersdk.MaxUserSecretsTotalValueBytes % codersdk.MaxUserSecretValueBytes
		for i := 0; i < numBig; i++ {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     fmt.Sprintf("prefill-%03d", i),
				Value:    big,
				FilePath: fmt.Sprintf("/tmp/prefill-%03d", i),
			})
			require.NoError(t, err)
		}
		if remainder > 0 {
			_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
				Name:     "prefill-pad",
				Value:    strings.Repeat("a", remainder),
				FilePath: "/tmp/prefill-pad",
			})
			require.NoError(t, err)
		}

		before, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)

		// Reset after the prefill (which legitimately emits create audit
		// logs) so the assertion below only sees logs from the rolled-back
		// import.
		auditor.ResetLogs()
		_, err = client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: "OVERFLOW=x",
		})
		requireSecretAPIError(t, err, http.StatusBadRequest, "per-user budget")

		after, err := client.UserSecrets(ctx, codersdk.Me)
		require.NoError(t, err)
		assert.Len(t, after, len(before))
		assert.Empty(t, auditor.AuditLogs())
	})
}

// envImportContent builds an env file of n distinct keys.
func envImportContent(n int) string {
	var content strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&content, "NEAR_%d=x\n", i)
	}
	return content.String()
}

// prefillUserSecrets creates n secrets for the authenticated user.
func prefillUserSecrets(ctx context.Context, t *testing.T, client *codersdk.Client, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
			Name:  fmt.Sprintf("prefill-%03d", i),
			Value: "original",
		})
		require.NoError(t, err)
	}
}

// staleCountStore hides the user's stored secrets from the import handler's
// pre-flight lookup, so the count pre-flight sees no headroom problem and only
// the per-user count trigger can reject the batch. This stands in for two
// concurrent imports, which can both compute their pre-flight from a view that
// omits the other's rows.
type staleCountStore struct {
	database.Store

	// txCountViolations counts transactions rolled back by the per-user count
	// limit, which is the backstop under test. The constraint name is raised by
	// the enforce_user_secrets_per_user_limits trigger.
	txCountViolations atomic.Int64
}

func (s *staleCountStore) ListUserSecrets(ctx context.Context, userID uuid.UUID) ([]database.ListUserSecretsRow, error) {
	if _, err := s.Store.ListUserSecrets(ctx, userID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *staleCountStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		err := fn(tx)
		if database.IsCheckViolation(err, database.CheckConstraint("user_secrets_per_user_count_limit")) {
			s.txCountViolations.Add(1)
		}
		return err
	}, opts)
}

// TestImportUserSecretsCountLimitDBBackstop verifies that the per-user count
// trigger still rejects a batch the pre-flight check could not see, which is
// the only thing guarding two imports racing each other. That path attributes
// the failure to the entry that tripped the trigger, since the trigger fires
// per row.
func TestImportUserSecretsCountLimitDBBackstop(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	store := &staleCountStore{Store: db}
	auditor := audit.NewMock()
	client := coderdtest.New(t, &coderdtest.Options{
		Database: store,
		Pubsub:   ps,
		Auditor:  auditor,
	})
	first := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Prefill to five below the cap and import six entries, so the sixth entry
	// (index 5, line 6) is the one that trips the trigger.
	prefillUserSecrets(ctx, t, client, codersdk.MaxUserSecretsPerUserCount-5)
	auditor.ResetLogs()

	_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatEnv,
		Content: envImportContent(6),
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	assert.Equal(t, "User secrets limit reached.", sdkErr.Message)
	assert.Equal(t, fmt.Sprintf(
		`Entry secrets[5] ("NEAR_6" on line 6): Each user can have at most %d secrets.`,
		codersdk.MaxUserSecretsPerUserCount,
	), sdkErr.Response.Detail)
	assert.EqualValues(t, 1, store.txCountViolations.Load())

	// The transaction rolled back, so only the prefill remains and the import
	// wrote no audit logs. Read through the wrapped store, since the wrapper
	// hides rows from ListUserSecrets.
	listed, err := db.ListUserSecrets(ctx, first.UserID)
	require.NoError(t, err)
	assert.Len(t, listed, codersdk.MaxUserSecretsPerUserCount-5)
	assert.Empty(t, auditor.AuditLogs())
}

// TestImportUserSecretsPerEntryErrorContext verifies that per-entry
// validation errors keep the machine-readable secrets[i].field path while
// naming the offending key in the detail, plus the source line for formats
// that track one. JSON carries no line information, so its details must omit
// the line entirely rather than reporting a placeholder.
func TestImportUserSecretsPerEntryErrorContext(t *testing.T) {
	t.Parallel()

	type expectation struct {
		field  string
		detail string
	}
	cases := []struct {
		name    string
		format  codersdk.SecretsFileFormat
		content string
		expect  []expectation
	}{
		{
			// The errors.env repro. PATH imports with an empty env_name, so
			// the slashed key and the empty value are the two errors.
			name:    "Env",
			format:  codersdk.SecretsFileFormatEnv,
			content: "PATH=reserved-env-name\nbad/name=slash-in-key\nEMPTY_VALUE=\n",
			expect: []expectation{
				{
					field: "secrets[1].name",
					detail: `Secret "bad/name" on line 2: ` +
						"Name must not contain /, ?, or #.",
				},
				{
					field: "secrets[2].value",
					detail: `Secret "EMPTY_VALUE" on line 3: ` +
						"Value is required.",
				},
			},
		},
		{
			// Comments and blank lines shift the line numbers away from the
			// entry indexes, which is the reason indexes alone are not enough.
			name:    "EnvWithCommentsAndBlanks",
			format:  codersdk.SecretsFileFormatEnv,
			content: "# a comment\n\nGOOD=fine\n\n# another comment\nbad/name=slash-in-key\n",
			expect: []expectation{
				{
					field: "secrets[1].name",
					detail: `Secret "bad/name" on line 6: ` +
						"Name must not contain /, ?, or #.",
				},
			},
		},
		{
			name:    "YAML",
			format:  codersdk.SecretsFileFormatYAML,
			content: "GOOD: fine\n\"bad/name\": slash-in-key\nEMPTY_VALUE: \"\"\n",
			expect: []expectation{
				{
					field: "secrets[1].name",
					detail: `Secret "bad/name" on line 2: ` +
						"Name must not contain /, ?, or #.",
				},
				{
					field: "secrets[2].value",
					detail: `Secret "EMPTY_VALUE" on line 3: ` +
						"Value is required.",
				},
			},
		},
		{
			// JSON has no line information, so the detail names only the key.
			name:    "JSON",
			format:  codersdk.SecretsFileFormatJSON,
			content: `{"GOOD":"fine","bad/name":"slash-in-key","EMPTY_VALUE":""}`,
			expect: []expectation{
				{
					field: "secrets[1].name",
					detail: `Secret "bad/name": ` +
						"Name must not contain /, ?, or #.",
				},
				{
					field: "secrets[2].value",
					detail: `Secret "EMPTY_VALUE": ` +
						"Value is required.",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitMedium)

			_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
				Format:  tc.format,
				Content: tc.content,
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

			require.Len(t, sdkErr.Validations, len(tc.expect))
			got := make(map[string]string, len(sdkErr.Validations))
			for _, v := range sdkErr.Validations {
				got[v.Field] = v.Detail
			}
			require.Len(t, got, len(tc.expect))
			for _, want := range tc.expect {
				// The field keeps its machine-readable secrets[i].field form.
				detail, ok := got[want.field]
				require.Truef(t, ok, "missing validation for field %q, got %v", want.field, got)
				assert.Equal(t, want.detail, detail)
			}
			if tc.format == codersdk.SecretsFileFormatJSON {
				for field, detail := range got {
					assert.NotContainsf(t, detail, "on line ",
						"JSON detail for %q must not mention a line: %q", field, detail)
				}
			}
			for field, detail := range got {
				assert.NotContainsf(t, detail, "slash-in-key",
					"detail for %q must not contain a secret value: %q", field, detail)
				assert.NotContainsf(t, detail, "line 0",
					"detail for %q must not report a placeholder line: %q", field, detail)
			}
		})
	}
}

// TestImportUserSecretsPerEntryErrorNameTruncated verifies that an oversized
// key is truncated where it is echoed back. An over-long name is a validation
// failure rather than a clamp, and each oversized entry yields both a name and
// a value error, so echoing keys in full would let a caller turn a 1 MiB import
// into a multi-megabyte response.
func TestImportUserSecretsPerEntryErrorNameTruncated(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	// Mirrors the unexported coderd.maxUserSecretLabelNameRunes bound.
	const labelNameRunes = 64

	// Worst case within both caps: the maximum number of entries, each with
	// the largest key that still fits the file-size cap, and an empty value
	// so every entry produces two errors. Keys carry an index suffix because
	// duplicates are rejected during parsing.
	const entries = codersdk.MaxUserSecretsPerUserCount
	keyLen := codersdk.MaxSecretsFileBytes/entries - len("=\n")
	var sb strings.Builder
	for i := 0; i < entries; i++ {
		suffix := fmt.Sprintf("%06d", i)
		sb.WriteString(strings.Repeat("a", keyLen-len(suffix)))
		sb.WriteString(suffix)
		sb.WriteString("=\n")
	}
	content := sb.String()
	require.LessOrEqual(t, len(content), codersdk.MaxSecretsFileBytes)

	res, err := client.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/users/%s/secrets/batch", codersdk.Me),
		codersdk.ImportUserSecretsRequest{
			Format:  codersdk.SecretsFileFormatEnv,
			Content: content,
		})
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	t.Logf("content bytes=%d keyLen=%d response bytes=%d (%.4gx input)",
		len(content), keyLen, len(body), float64(len(body))/float64(len(content)))

	// The response must stay small relative to the request instead of growing
	// with the echoed keys.
	const maxResponseBytes = 64 << 10
	assert.Lessf(t, len(body), maxResponseBytes,
		"response of %d bytes for a %d byte request suggests keys are echoed in full",
		len(body), len(content))

	var resp codersdk.Response
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Len(t, resp.Validations, 2*entries)
	for _, v := range resp.Validations {
		assert.Containsf(t, v.Detail, strings.Repeat("a", labelNameRunes)+`..."`,
			"detail for %q must quote a truncated key: %q", v.Field, v.Detail)
		assert.NotContainsf(t, v.Detail, strings.Repeat("a", labelNameRunes+1),
			"detail for %q echoes more than %d runes of the key", v.Field, labelNameRunes)
	}
}

func TestImportUserSecretsParseErrors(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitMedium)

	// Parse-error variety is covered by the parser unit tests; this only
	// asserts the endpoint maps a parse failure to 400.
	_, err := client.ImportUserSecrets(ctx, codersdk.Me, codersdk.ImportUserSecretsRequest{
		Format:  codersdk.SecretsFileFormatJSON,
		Content: "{not json",
	})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
}

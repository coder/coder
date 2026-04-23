package dynamicparameters

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	previewtypes "github.com/coder/preview/types"
)

// newTestRenderer builds a dynamicRenderer backed by the given testdata
// fixture. The caller must seed an org and member row.
func newTestRenderer(t *testing.T, db database.Store, orgID uuid.UUID, fixture string) *dynamicRenderer {
	t.Helper()
	return &dynamicRenderer{
		db:                db,
		templateFS:        os.DirFS(filepath.Join("testdata", fixture)),
		ownerErrors:       make(map[uuid.UUID]error),
		ownerSecretErrors: make(map[uuid.UUID]error),
		data: &loader{
			templateVersion: &database.TemplateVersion{
				OrganizationID: orgID,
			},
			terraformValues: &database.TemplateVersionTerraformValue{},
		},
		close: func() {},
	}
}

// seedOwner creates a user and org member so WorkspaceOwner resolves.
func seedOwner(t *testing.T, db database.Store, orgID uuid.UUID) database.User {
	t.Helper()
	u := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: orgID,
		UserID:         u.ID,
	})
	return u
}

func TestDynamicRender_MissingSecretRequirement(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	// Owner has no secrets; the GITHUB_TOKEN requirement is unmet.
	out, diags := renderer.Render(ctx, owner.ID, nil)
	require.NotNil(t, out)
	requireMissingSecret(t, diags, "Add a GitHub PAT with env=GITHUB_TOKEN")

	// The same renderer must pick up a newly-created secret on the
	// next render, without a reload.
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  owner.ID,
		Name:    "github_token",
		EnvName: "GITHUB_TOKEN",
	})

	_, diags2 := renderer.Render(ctx, owner.ID, nil)
	requireNoMissingSecret(t, diags2)
}

func TestDynamicRender_ConditionalSecretRequirement(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_conditional")
	defer renderer.Close()

	// Block inactive: no validation.
	_, diags := renderer.Render(ctx, owner.ID, map[string]string{"use_github": "false"})
	requireNoMissingSecret(t, diags)

	// Block active: requirement surfaces.
	_, diags = renderer.Render(ctx, owner.ID, map[string]string{"use_github": "true"})
	requireMissingSecret(t, diags, "Add a GitHub PAT")
}

func TestDynamicRender_SingleSecretSatisfiesEnvAndFile(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	// One row must satisfy both an env and a file requirement: the
	// check builds independent envSet and fileSet maps.
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:   owner.ID,
		Name:     "combined",
		EnvName:  "GITHUB_TOKEN",
		FilePath: "~/.ssh/id_rsa",
	})

	renderer := newTestRenderer(t, db, org.ID, "secret_env_and_file")
	defer renderer.Close()

	_, diags := renderer.Render(ctx, owner.ID, nil)
	requireNoMissingSecret(t, diags)
}

func TestDynamicRender_PartialEnvAndFileSatisfaction(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	// Env-only secret against an env+file requirement: only the file
	// requirement should fail.
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  owner.ID,
		Name:    "env_only",
		EnvName: "GITHUB_TOKEN",
	})

	renderer := newTestRenderer(t, db, org.ID, "secret_env_and_file")
	defer renderer.Close()

	_, diags := renderer.Render(ctx, owner.ID, nil)
	var missing []*hcl.Diagnostic
	for _, d := range diags {
		if extra, ok := d.Extra.(previewtypes.DiagnosticExtra); ok && extra.Code == DiagCodeMissingSecret {
			missing = append(missing, d)
		}
	}
	require.Len(t, missing, 1, "only the file requirement should be unmet; env was satisfied by the seeded secret")
	require.Contains(t, missing[0].Detail, "needs file")
}

func TestDynamicRender_OwnerSwitch(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})

	// Owner A satisfies the requirement; owner B does not.
	ownerA := seedOwner(t, db, org.ID)
	ownerB := seedOwner(t, db, org.ID)
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  ownerA.ID,
		Name:    "gh",
		EnvName: "GITHUB_TOKEN",
	})

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	_, diags := renderer.Render(ctx, ownerA.ID, nil)
	requireNoMissingSecret(t, diags)

	// The cache must not serve owner A's rows to owner B.
	_, diags = renderer.Render(ctx, ownerB.ID, nil)
	requireMissingSecret(t, diags, "")
}

// countingStore counts ListUserSecrets calls per user.
type countingStore struct {
	database.Store
	mu    sync.Mutex
	calls map[uuid.UUID]int
}

func (c *countingStore) ListUserSecrets(ctx context.Context, userID uuid.UUID) ([]database.ListUserSecretsRow, error) {
	c.mu.Lock()
	if c.calls == nil {
		c.calls = map[uuid.UUID]int{}
	}
	c.calls[userID]++
	c.mu.Unlock()
	return c.Store.ListUserSecrets(ctx, userID)
}

func (c *countingStore) callsFor(id uuid.UUID) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[id]
}

// TestDynamicRender_NotAuthorizedIsCached pins that NotAuthorized
// denials hit ListUserSecrets at most once per owner.
func TestDynamicRender_NotAuthorizedIsCached(t *testing.T) {
	t.Parallel()

	inner, _ := dbtestutil.NewDB(t)
	db := &countingStore{Store: secretAuthDenyingStore{Store: inner}}
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	for range 3 {
		_, _ = renderer.Render(ctx, owner.ID, nil)
	}
	require.Equal(t, 1, db.callsFor(owner.ID),
		"NotAuthorized must be cached across renders")
}

// secretAuthDenyingStore makes ListUserSecrets return NotAuthorized,
// simulating a non-owner caller.
type secretAuthDenyingStore struct {
	database.Store
}

func (secretAuthDenyingStore) ListUserSecrets(_ context.Context, _ uuid.UUID) ([]database.ListUserSecretsRow, error) {
	return nil, dbauthz.NotAuthorizedError{}
}

// TestDynamicRender_NonOwnerCannotLeakSecretRequirements guards against
// a non-owner enumerating secret names via missing_secret diagnostics.
func TestDynamicRender_NonOwnerCannotLeakSecretRequirements(t *testing.T) {
	t.Parallel()

	inner, _ := dbtestutil.NewDB(t)
	db := secretAuthDenyingStore{Store: inner}
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	// Secret matches the requirement; a non-owner must still never
	// see it.
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  owner.ID,
		Name:    "gh",
		EnvName: "GITHUB_TOKEN",
	})

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	_, diags := renderer.Render(ctx, owner.ID, nil)

	// No missing_secret diagnostic for a non-owner, regardless of
	// whether the target satisfies the requirement.
	requireNoMissingSecret(t, diags)

	// Surface a warning so the admin knows validation didn't run.
	var sawWarn bool
	for _, d := range diags {
		extra, ok := d.Extra.(previewtypes.DiagnosticExtra)
		if !ok {
			continue
		}
		if extra.Code == DiagCodeSecretValidationForbidden {
			require.Equal(t, hcl.DiagWarning, d.Severity,
				"secret_validation_forbidden must be a warning")
			sawWarn = true
		}
	}
	require.True(t, sawWarn, "expected secret_validation_forbidden warning")
}

func requireMissingSecret(t *testing.T, diags hcl.Diagnostics, wantDetail string) {
	t.Helper()
	for _, d := range diags {
		extra, ok := d.Extra.(previewtypes.DiagnosticExtra)
		if !ok || extra.Code != DiagCodeMissingSecret {
			continue
		}
		// The Create Workspace button disables only on error severity.
		require.Equal(t, hcl.DiagError, d.Severity,
			"missing_secret must be error severity")
		if wantDetail != "" {
			require.Contains(t, d.Detail, wantDetail)
		}
		return
	}
	t.Fatalf("expected a missing_secret diagnostic; got %v", diags)
}

func requireNoMissingSecret(t *testing.T, diags hcl.Diagnostics) {
	t.Helper()
	for _, d := range diags {
		if extra, ok := d.Extra.(previewtypes.DiagnosticExtra); ok && extra.Code == DiagCodeMissingSecret {
			t.Fatalf("unexpected missing_secret diagnostic: %s", d.Detail)
		}
	}
}

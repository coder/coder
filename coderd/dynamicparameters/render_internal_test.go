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

// newTestRenderer constructs a dynamicRenderer pointing at a testdata
// fixture on disk. The caller is responsible for seeding the organization
// and a member row so that WorkspaceOwner lookups succeed.
func newTestRenderer(t *testing.T, db database.Store, orgID uuid.UUID, fixture string) *dynamicRenderer {
	t.Helper()
	return &dynamicRenderer{
		db:              db,
		templateFS:      os.DirFS(filepath.Join("testdata", fixture)),
		ownerErrors:     make(map[uuid.UUID]error),
		ownerSecretsErr: make(map[uuid.UUID]error),
		data: &loader{
			templateVersion: &database.TemplateVersion{
				OrganizationID: orgID,
			},
			terraformValues: &database.TemplateVersionTerraformValue{},
		},
		close: func() {},
	}
}

// seedOwner creates a user and an organization member, which is what
// WorkspaceOwner requires to resolve the owner.
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
	ctx := context.Background()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	// Owner has no secrets; the GITHUB_TOKEN requirement is unmet.
	out, diags := renderer.Render(ctx, owner.ID, nil)
	require.NotNil(t, out)
	requireMissingSecret(t, diags, "Add a GitHub PAT with env=GITHUB_TOKEN")

	// Mimic the user following the diagnostic's "coder secret create"
	// guidance in another tab, then coming back. The SAME renderer (i.e.
	// the same websocket session) must pick up the new secret on the next
	// render without needing a page reload; otherwise the Create Workspace
	// button stays disabled against a stale cached secret list and the
	// recovery UX silently breaks.
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
	ctx := context.Background()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_conditional")
	defer renderer.Close()

	// use_github=false keeps the coder_secret block inactive, so nothing
	// to validate.
	_, diags := renderer.Render(ctx, owner.ID, map[string]string{"use_github": "false"})
	requireNoMissingSecret(t, diags)

	// Flipping the parameter activates the block and surfaces the
	// unsatisfied requirement.
	_, diags = renderer.Render(ctx, owner.ID, map[string]string{"use_github": "true"})
	requireMissingSecret(t, diags, "Add a GitHub PAT")
}

func TestDynamicRender_SingleSecretSatisfiesEnvAndFile(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	// One user_secrets row with BOTH env_name and file_path populated.
	// Per the User Secrets RFC a single secret must satisfy both an env
	// and a file requirement simultaneously.
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

func TestDynamicRender_OwnerSwitch(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()
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

// countingStore wraps a database.Store and counts ListUserSecrets calls
// per user so we can assert the owner-keyed cache behavior.
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

// TestDynamicRender_NotAuthorizedIsCached asserts that NotAuthorized
// denials are cached per-owner for the renderer lifetime: repeated
// renders for a denied owner hit ListUserSecrets at most once.
func TestDynamicRender_NotAuthorizedIsCached(t *testing.T) {
	t.Parallel()

	inner, _ := dbtestutil.NewDB(t)
	// Compose: outer countingStore sees every ListUserSecrets invocation;
	// inner secretAuthDenyingStore makes each one return NotAuthorized.
	db := &countingStore{Store: secretAuthDenyingStore{Store: inner}}
	ctx := context.Background()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	for range 3 {
		_, _ = renderer.Render(ctx, owner.ID, nil)
	}
	require.Equal(t, 1, db.callsFor(owner.ID),
		"NotAuthorized must be cached; expected one ListUserSecrets call across multiple renders")
}

// secretAuthDenyingStore wraps database.Store and makes ListUserSecrets
// return dbauthz.NotAuthorizedError, simulating a caller that lacks
// user_secret:read on the target owner. This is the exact condition a
// template admin rendering for another user hits, per the User Secrets
// RFC's owner-only RBAC model.
type secretAuthDenyingStore struct {
	database.Store
}

func (secretAuthDenyingStore) ListUserSecrets(_ context.Context, _ uuid.UUID) ([]database.ListUserSecretsRow, error) {
	return nil, dbauthz.NotAuthorizedError{}
}

// TestDynamicRender_NonOwnerCannotLeakSecretRequirements is the
// regression test for the information-leak vector where a non-owner
// (e.g. a template admin) could enumerate a target user's secret
// env_names and file_paths by watching for missing_secret diagnostics
// across crafted templates. The fix is to keep the ListUserSecrets call
// inside the caller's own authorization context; this test pins it.
func TestDynamicRender_NonOwnerCannotLeakSecretRequirements(t *testing.T) {
	t.Parallel()

	inner, _ := dbtestutil.NewDB(t)
	db := secretAuthDenyingStore{Store: inner}
	ctx := context.Background()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	// Seed a secret that DOES match the template's requirement. Under the
	// buggy behavior (elevated system actor) this would satisfy the
	// requirement and emit no missing_secret diagnostic, while under the
	// correct behavior the non-owner never sees the list at all.
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  owner.ID,
		Name:    "gh",
		EnvName: "GITHUB_TOKEN",
	})

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	_, diags := renderer.Render(ctx, owner.ID, nil)

	// 1. Never emit missing_secret for a non-owner, regardless of whether
	//    the target satisfies the requirement. This is the leak being
	//    prevented: the presence/absence of missing_secret must not be
	//    observable by callers who lack user_secret:read on the target.
	requireNoMissingSecret(t, diags)

	// 2. Surface a visible warning so the admin understands the feature
	//    is not running for this target, rather than silently succeeding.
	var sawWarn bool
	for _, d := range diags {
		extra, ok := d.Extra.(previewtypes.DiagnosticExtra)
		if !ok {
			continue
		}
		if extra.Code == "secret_validation_forbidden" {
			require.Equal(t, hcl.DiagWarning, d.Severity,
				"secret_validation_forbidden must be a warning so it does not disable the Create Workspace button")
			sawWarn = true
		}
	}
	require.True(t, sawWarn,
		"expected secret_validation_forbidden warning when caller lacks user_secret:read")
}

func requireMissingSecret(t *testing.T, diags hcl.Diagnostics, wantDetail string) {
	t.Helper()
	for _, d := range diags {
		extra, ok := d.Extra.(previewtypes.DiagnosticExtra)
		if !ok || extra.Code != "missing_secret" {
			continue
		}
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
		if extra, ok := d.Extra.(previewtypes.DiagnosticExtra); ok && extra.Code == "missing_secret" {
			t.Fatalf("unexpected missing_secret diagnostic: %s", d.Detail)
		}
	}
}

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
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
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
	out, diags := renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
	require.NotNil(t, out)
	require.NotNil(t, out.Output)
	requireNoMissingSecret(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{{
		Env:         "GITHUB_TOKEN",
		HelpMessage: "Add a GitHub PAT with env=GITHUB_TOKEN",
		Satisfied:   false,
	}}, out.SecretRequirements)

	// The same renderer must pick up a newly-created secret on the
	// next render, without a reload.
	_ = dbgen.UserSecret(t, db, database.UserSecret{
		UserID:  owner.ID,
		Name:    "github_token",
		EnvName: "GITHUB_TOKEN",
	})

	out, diags2 := renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags2)
	require.Equal(t, []codersdk.SecretRequirementStatus{{
		Env:         "GITHUB_TOKEN",
		HelpMessage: "Add a GitHub PAT with env=GITHUB_TOKEN",
		Satisfied:   true,
	}}, out.SecretRequirements)
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
	out, diags := renderer.Render(ctx, owner.ID, map[string]string{"use_github": "false"}, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags)
	require.Nil(t, out.SecretRequirements)

	// Block active: requirement surfaces.
	out, diags = renderer.Render(ctx, owner.ID, map[string]string{"use_github": "true"}, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{{
		Env:         "GITHUB_TOKEN",
		HelpMessage: "Add a GitHub PAT",
		Satisfied:   false,
	}}, out.SecretRequirements)
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

	out, diags := renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{
		{
			File:        "~/.ssh/id_rsa",
			HelpMessage: "needs file",
			Satisfied:   true,
		},
		{
			Env:         "GITHUB_TOKEN",
			HelpMessage: "needs env",
			Satisfied:   true,
		},
	}, out.SecretRequirements)
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

	out, diags := renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{
		{
			File:        "~/.ssh/id_rsa",
			HelpMessage: "needs file",
			Satisfied:   false,
		},
		{
			Env:         "GITHUB_TOKEN",
			HelpMessage: "needs env",
			Satisfied:   true,
		},
	}, out.SecretRequirements)
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

	out, diags := renderer.Render(ctx, ownerA.ID, nil, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{{
		Env:         "GITHUB_TOKEN",
		HelpMessage: "Add a GitHub PAT with env=GITHUB_TOKEN",
		Satisfied:   true,
	}}, out.SecretRequirements)

	// The cache must not serve owner A's rows to owner B.
	out, diags = renderer.Render(ctx, ownerB.ID, nil, IncludeSecretRequirements())
	requireNoMissingSecret(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{{
		Env:         "GITHUB_TOKEN",
		HelpMessage: "Add a GitHub PAT with env=GITHUB_TOKEN",
		Satisfied:   false,
	}}, out.SecretRequirements)
}

func TestDynamicRender_DeduplicatesSecretRequirements(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	reqs := []previewtypes.SecretRequirement{
		{Env: "GITHUB_TOKEN", HelpMessage: "z help"},
		{Env: "GITHUB_TOKEN", HelpMessage: "a help"},
	}
	statuses, diags := renderer.checkSecretRequirements(ctx, owner.ID, reqs)
	require.Empty(t, diags)
	require.Equal(t, []codersdk.SecretRequirementStatus{{
		Env:         "GITHUB_TOKEN",
		HelpMessage: "a help",
		Satisfied:   false,
	}}, statuses)
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
		_, _ = renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
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

type secretFetchFailingStore struct {
	database.Store
}

func (secretFetchFailingStore) ListUserSecrets(_ context.Context, _ uuid.UUID) ([]database.ListUserSecretsRow, error) {
	return nil, xerrors.New("fetch failed")
}

func TestDynamicRender_SecretFetchFailedHasNilRequirements(t *testing.T) {
	t.Parallel()

	inner, _ := dbtestutil.NewDB(t)
	db := secretFetchFailingStore{Store: inner}
	ctx := t.Context()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := seedOwner(t, db, org.ID)

	renderer := newTestRenderer(t, db, org.ID, "secret_required")
	defer renderer.Close()

	out, diags := renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
	require.Nil(t, out.SecretRequirements)
	requireNoMissingSecret(t, diags)

	var sawErr bool
	for _, d := range diags {
		extra, ok := d.Extra.(previewtypes.DiagnosticExtra)
		if !ok {
			continue
		}
		if extra.Code == DiagCodeOwnerSecretsFetchFailed {
			require.Equal(t, hcl.DiagError, d.Severity)
			sawErr = true
		}
	}
	require.True(t, sawErr, "expected owner_secrets_fetch_failed error")
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

	out, diags := renderer.Render(ctx, owner.ID, nil, IncludeSecretRequirements())
	require.Nil(t, out.SecretRequirements)

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

func requireNoMissingSecret(t *testing.T, diags hcl.Diagnostics) {
	t.Helper()
	for _, d := range diags {
		if extra, ok := d.Extra.(previewtypes.DiagnosticExtra); ok && extra.Code == DiagCodeMissingSecret {
			t.Fatalf("unexpected missing_secret diagnostic: %s", d.Detail)
		}
	}
}

package dbgen

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

// Foundation creates an independently identified Foundation, optionally bound to a template.
func Foundation(t testing.TB, db database.Store, seed database.Foundation) database.Foundation {
	t.Helper()
	if seed.ID == uuid.Nil {
		seed.ID = uuid.New()
	}
	if seed.OrganizationID == uuid.Nil {
		seed.OrganizationID = Organization(t, db, database.Organization{}).ID
	}
	if seed.Name == "" {
		seed.Name = testutil.GetRandomName(t)
	}
	if len(seed.ProvisionerTags) == 0 {
		seed.ProvisionerTags = []byte(`{}`)
	}
	now := dbtime.Now()
	f, err := db.InsertFoundation(genCtx, database.InsertFoundationParams{ID: seed.ID, OrganizationID: seed.OrganizationID, Name: seed.Name, ProvisionerTags: seed.ProvisionerTags, CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	if seed.TemplateID.Valid {
		f, err = db.BindFoundation(genCtx, database.BindFoundationParams{ID: f.ID, TemplateID: seed.TemplateID, UpdatedAt: now})
		require.NoError(t, err)
	}
	return f
}

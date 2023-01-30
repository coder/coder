package databasefake

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"
)

const primaryOrgName = "primary-org"

type Generator struct {
	// names is a map of names to uuids.
	names      map[string]uuid.UUID
	primaryOrg *database.Organization
	testT      *testing.T

	db database.Store
}

func NewGenerator(t *testing.T, db database.Store) *Generator {
	if _, ok := db.(FakeDatabase); !ok {
		panic("Generator db must be a FakeDatabase")
	}
	return &Generator{
		names: make(map[string]uuid.UUID),
		testT: t,
		db:    db,
	}
}

// PrimaryOrg is to keep all resources in the same default org if not
// specified.
func (g *Generator) PrimaryOrg(ctx context.Context) database.Organization {
	if g.primaryOrg == nil {
		org := g.Organization(ctx, "primary-org", database.Organization{
			ID:          g.Lookup(primaryOrgName),
			Name:        primaryOrgName,
			Description: "This is the default primary organization for all tests",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
		g.primaryOrg = &org
	}

	return *g.primaryOrg
}

func populate[DBType any](ctx context.Context, g *Generator, name string, seed DBType) DBType {
	out := g.Populate(ctx, map[string]interface{}{
		name: seed,
	})
	return out[name].(DBType)
}

func (g *Generator) Group(ctx context.Context, name string, seed database.Group) database.Group {
	return populate(ctx, g, name, seed)
}

func (g *Generator) Organization(ctx context.Context, name string, seed database.Organization) database.Organization {
	return populate(ctx, g, name, seed)
}

func (g *Generator) Workspace(ctx context.Context, name string, seed database.Workspace) database.Workspace {
	return populate(ctx, g, name, seed)
}

func (g *Generator) Template(ctx context.Context, name string, seed database.Template) database.Template {
	return populate(ctx, g, name, seed)
}

func (g *Generator) TemplateVersion(ctx context.Context, name string, seed database.TemplateVersion) database.TemplateVersion {
	return populate(ctx, g, name, seed)
}

func (g *Generator) WorkspaceBuild(ctx context.Context, name string, seed database.WorkspaceBuild) database.WorkspaceBuild {
	return populate(ctx, g, name, seed)
}

func (g *Generator) User(ctx context.Context, name string, seed database.User) database.User {
	return populate(ctx, g, name, seed)
}

func (g *Generator) Populate(ctx context.Context, seed map[string]interface{}) map[string]interface{} {
	db := g.db
	t := g.testT

	for name, v := range seed {
		switch orig := v.(type) {
		case database.Template:
			template, err := db.InsertTemplate(ctx, database.InsertTemplateParams{
				ID:                           g.Lookup(name),
				CreatedAt:                    time.Now(),
				UpdatedAt:                    time.Now(),
				OrganizationID:               takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				Name:                         takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				Provisioner:                  takeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
				ActiveVersionID:              takeFirst(orig.ActiveVersionID, uuid.New()),
				Description:                  takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
				DefaultTTL:                   takeFirst(orig.DefaultTTL, 3600),
				CreatedBy:                    takeFirst(orig.CreatedBy, uuid.New()),
				Icon:                         takeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
				UserACL:                      orig.UserACL,
				GroupACL:                     orig.GroupACL,
				DisplayName:                  takeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
				AllowUserCancelWorkspaceJobs: takeFirst(orig.AllowUserCancelWorkspaceJobs, true),
			})
			require.NoError(t, err, "insert template")

			seed[name] = template
		case database.Workspace:
			workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
				ID:                g.Lookup(name),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				OrganizationID:    takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				TemplateID:        takeFirst(orig.TemplateID, uuid.New()),
				Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				AutostartSchedule: orig.AutostartSchedule,
				Ttl:               orig.Ttl,
			})
			require.NoError(t, err, "insert workspace")

			seed[name] = workspace
		case database.WorkspaceBuild:
			build, err := db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
				ID:                g.Lookup(name),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				WorkspaceID:       takeFirst(orig.WorkspaceID, uuid.New()),
				TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
				BuildNumber:       takeFirst(orig.BuildNumber, 0),
				Transition:        takeFirst(orig.Transition, database.WorkspaceTransitionStart),
				InitiatorID:       takeFirst(orig.InitiatorID, uuid.New()),
				JobID:             takeFirst(orig.InitiatorID, uuid.New()),
				ProvisionerState:  []byte{},
				Deadline:          time.Now(),
				Reason:            takeFirst(orig.Reason, database.BuildReasonInitiator),
			})
			require.NoError(t, err, "insert workspace build")

			seed[name] = build
		case database.User:
			user, err := db.InsertUser(ctx, database.InsertUserParams{
				ID:             g.Lookup(name),
				Email:          takeFirst(orig.Email, namesgenerator.GetRandomName(1)),
				Username:       takeFirst(orig.Username, namesgenerator.GetRandomName(1)),
				HashedPassword: []byte{},
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				RBACRoles:      []string{},
				LoginType:      takeFirst(orig.LoginType, database.LoginTypePassword),
			})
			require.NoError(t, err, "insert user")

			seed[name] = user

		case database.Organization:
			org, err := db.InsertOrganization(ctx, database.InsertOrganizationParams{
				ID:          g.Lookup(name),
				Name:        takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				Description: takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
			require.NoError(t, err, "insert organization")

			seed[name] = org

		case database.Group:
			org, err := db.InsertGroup(ctx, database.InsertGroupParams{
				ID:             g.Lookup(name),
				Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				OrganizationID: takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				AvatarURL:      takeFirst(orig.Name, "https://logo.example.com"),
				QuotaAllowance: takeFirst(orig.QuotaAllowance, 0),
			})
			require.NoError(t, err, "insert organization")

			seed[name] = org
		}
	}
	return seed
}

func (tc *Generator) Lookup(name string) uuid.UUID {
	if tc.names == nil {
		tc.names = make(map[string]uuid.UUID)
	}
	if id, ok := tc.names[name]; ok {
		return id
	}
	id := uuid.New()
	tc.names[name] = id
	return id
}

// takeFirst will take the first non-empty value.
func takeFirst[Value comparable](values ...Value) Value {
	var empty Value
	for _, v := range values {
		if v != empty {
			return v
		}
	}
	// If all empty, return empty
	return empty
}

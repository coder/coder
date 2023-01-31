package databasefake

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/cryptorand"
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
		org := g.Organization(ctx, database.Organization{
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
	if name == "" {
		name = g.RandomName()
	}

	out := g.Populate(ctx, map[string]interface{}{
		name: seed,
	})
	v, ok := out[name].(DBType)
	if !ok {
		panic("developer error, type mismatch")
	}
	return v
}

func (g *Generator) RandomName() string {
	for {
		name := namesgenerator.GetRandomName(0)
		if _, ok := g.names[name]; !ok {
			return name
		}
	}
}

func (g *Generator) APIKey(ctx context.Context, seed database.APIKey) (key database.APIKey, token string) {
	name := g.RandomName()
	out := g.Populate(ctx, map[string]interface{}{
		name: seed,
	})
	key, keyOk := out[name].(database.APIKey)
	secret, secOk := out[name+"_secret"].(string)
	require.True(g.testT, keyOk && secOk, "APIKey & secret must be populated with the right type")

	return key, fmt.Sprintf("%s-%s", key.ID, secret)
}

func (g *Generator) File(ctx context.Context, seed database.File) database.File {
	return populate(ctx, g, "", seed)
}

func (g *Generator) UserLink(ctx context.Context, seed database.UserLink) database.UserLink {
	return populate(ctx, g, "", seed)
}

func (g *Generator) WorkspaceResource(ctx context.Context, seed database.WorkspaceResource) database.WorkspaceResource {
	return populate(ctx, g, "", seed)
}

func (g *Generator) Job(ctx context.Context, seed database.ProvisionerJob) database.ProvisionerJob {
	return populate(ctx, g, "", seed)
}

func (g *Generator) Group(ctx context.Context, seed database.Group) database.Group {
	return populate(ctx, g, "", seed)
}

func (g *Generator) Organization(ctx context.Context, seed database.Organization) database.Organization {
	return populate(ctx, g, "", seed)
}

func (g *Generator) Workspace(ctx context.Context, seed database.Workspace) database.Workspace {
	return populate(ctx, g, "", seed)
}

func (g *Generator) Template(ctx context.Context, seed database.Template) database.Template {
	return populate(ctx, g, "", seed)
}

func (g *Generator) TemplateVersion(ctx context.Context, seed database.TemplateVersion) database.TemplateVersion {
	return populate(ctx, g, "", seed)
}

func (g *Generator) WorkspaceBuild(ctx context.Context, seed database.WorkspaceBuild) database.WorkspaceBuild {
	return populate(ctx, g, "", seed)
}

func (g *Generator) User(ctx context.Context, seed database.User) database.User {
	return populate(ctx, g, "", seed)
}

// Populate uses `require` which calls `t.FailNow()` and must be called from the
// go routine running the test or benchmark function.
func (g *Generator) Populate(ctx context.Context, seed map[string]interface{}) map[string]interface{} {
	db := g.db
	t := g.testT

	output := make(map[string]interface{})
	for name, v := range seed {
		switch orig := v.(type) {
		case database.APIKey:
			id, _ := cryptorand.String(10)
			secret, _ := cryptorand.String(22)
			hashed := sha256.Sum256([]byte(secret))

			key, err := db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
				ID: takeFirst(orig.ID, id),
				// 0 defaults to 86400 at the db layer
				LifetimeSeconds: takeFirst(orig.LifetimeSeconds, 0),
				HashedSecret:    takeFirstBytes(orig.HashedSecret, hashed[:]),
				IPAddress:       pqtype.Inet{},
				UserID:          takeFirst(orig.UserID, uuid.New()),
				LastUsed:        takeFirst(orig.LastUsed, time.Now()),
				ExpiresAt:       takeFirst(orig.ExpiresAt, time.Now().Add(time.Hour)),
				CreatedAt:       takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:       takeFirst(orig.UpdatedAt, time.Now()),
				LoginType:       takeFirst(orig.LoginType, database.LoginTypePassword),
				Scope:           takeFirst(orig.Scope, database.APIKeyScopeAll),
			})
			require.NoError(t, err, "insert api key")

			output[name] = key
			// Need to also save the secret
			output[name+"_secret"] = secret
		case database.Template:
			template, err := db.InsertTemplate(ctx, database.InsertTemplateParams{
				ID:                           takeFirst(orig.ID, g.Lookup(name)),
				CreatedAt:                    takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:                    takeFirst(orig.UpdatedAt, time.Now()),
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

			output[name] = template

		case database.TemplateVersion:
			template, err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				ID: takeFirst(orig.ID, g.Lookup(name)),
				TemplateID: uuid.NullUUID{
					UUID:  takeFirst(orig.TemplateID.UUID, uuid.New()),
					Valid: takeFirst(orig.TemplateID.Valid, true),
				},
				OrganizationID: takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
				Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				Readme:         takeFirst(orig.Readme, namesgenerator.GetRandomName(1)),
				JobID:          takeFirst(orig.JobID, uuid.New()),
				CreatedBy:      takeFirst(orig.CreatedBy, uuid.New()),
			})
			require.NoError(t, err, "insert template")

			output[name] = template
		case database.Workspace:
			workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
				ID:                takeFirst(orig.ID, g.Lookup(name)),
				OwnerID:           takeFirst(orig.OwnerID, uuid.New()),
				CreatedAt:         takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:         takeFirst(orig.UpdatedAt, time.Now()),
				OrganizationID:    takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				TemplateID:        takeFirst(orig.TemplateID, uuid.New()),
				Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				AutostartSchedule: orig.AutostartSchedule,
				Ttl:               orig.Ttl,
			})
			require.NoError(t, err, "insert workspace")

			output[name] = workspace
		case database.WorkspaceBuild:
			build, err := db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
				ID:                takeFirst(orig.ID, g.Lookup(name)),
				CreatedAt:         takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:         takeFirst(orig.UpdatedAt, time.Now()),
				WorkspaceID:       takeFirst(orig.WorkspaceID, uuid.New()),
				TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
				BuildNumber:       takeFirst(orig.BuildNumber, 0),
				Transition:        takeFirst(orig.Transition, database.WorkspaceTransitionStart),
				InitiatorID:       takeFirst(orig.InitiatorID, uuid.New()),
				JobID:             takeFirst(orig.JobID, uuid.New()),
				ProvisionerState:  takeFirstBytes(orig.ProvisionerState, []byte{}),
				Deadline:          takeFirst(orig.Deadline, time.Now().Add(time.Hour)),
				Reason:            takeFirst(orig.Reason, database.BuildReasonInitiator),
			})
			require.NoError(t, err, "insert workspace build")

			output[name] = build
		case database.User:
			user, err := db.InsertUser(ctx, database.InsertUserParams{
				ID:             takeFirst(orig.ID, g.Lookup(name)),
				Email:          takeFirst(orig.Email, namesgenerator.GetRandomName(1)),
				Username:       takeFirst(orig.Username, namesgenerator.GetRandomName(1)),
				HashedPassword: takeFirstBytes(orig.HashedPassword, []byte{}),
				CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
				RBACRoles:      []string{},
				LoginType:      takeFirst(orig.LoginType, database.LoginTypePassword),
			})
			require.NoError(t, err, "insert user")

			output[name] = user

		case database.Organization:
			org, err := db.InsertOrganization(ctx, database.InsertOrganizationParams{
				ID:          takeFirst(orig.ID, g.Lookup(name)),
				Name:        takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				Description: takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
				CreatedAt:   takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:   takeFirst(orig.UpdatedAt, time.Now()),
			})
			require.NoError(t, err, "insert organization")

			output[name] = org

		case database.Group:
			org, err := db.InsertGroup(ctx, database.InsertGroupParams{
				ID:             takeFirst(orig.ID, g.Lookup(name)),
				Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				OrganizationID: takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				AvatarURL:      takeFirst(orig.AvatarURL, "https://logo.example.com"),
				QuotaAllowance: takeFirst(orig.QuotaAllowance, 0),
			})
			require.NoError(t, err, "insert organization")

			output[name] = org

		case database.ProvisionerJob:
			job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
				ID:             takeFirst(orig.ID, g.Lookup(name)),
				CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
				UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
				OrganizationID: takeFirst(orig.OrganizationID, g.PrimaryOrg(ctx).ID),
				InitiatorID:    takeFirst(orig.InitiatorID, uuid.New()),
				Provisioner:    takeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
				StorageMethod:  takeFirst(orig.StorageMethod, database.ProvisionerStorageMethodFile),
				FileID:         takeFirst(orig.FileID, uuid.New()),
				Type:           takeFirst(orig.Type, database.ProvisionerJobTypeWorkspaceBuild),
				Input:          takeFirstBytes(orig.Input, []byte("{}")),
				Tags:           orig.Tags,
			})
			require.NoError(t, err, "insert job")

			output[name] = job

		case database.WorkspaceResource:
			resource, err := db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
				ID:         takeFirst(orig.ID, g.Lookup(name)),
				CreatedAt:  takeFirst(orig.CreatedAt, time.Now()),
				JobID:      takeFirst(orig.JobID, uuid.New()),
				Transition: takeFirst(orig.Transition, database.WorkspaceTransitionStart),
				Type:       takeFirst(orig.Type, "fake_resource"),
				Name:       takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				Hide:       takeFirst(orig.Hide, false),
				Icon:       takeFirst(orig.Icon, ""),
				InstanceType: sql.NullString{
					String: takeFirst(orig.InstanceType.String, ""),
					Valid:  takeFirst(orig.InstanceType.Valid, false),
				},
				DailyCost: takeFirst(orig.DailyCost, 0),
			})
			require.NoError(t, err, "insert resource")

			output[name] = resource

		case database.File:
			file, err := db.InsertFile(ctx, database.InsertFileParams{
				ID:        takeFirst(orig.ID, g.Lookup(name)),
				Hash:      takeFirst(orig.Hash, hex.EncodeToString(make([]byte, 32))),
				CreatedAt: takeFirst(orig.CreatedAt, time.Now()),
				CreatedBy: takeFirst(orig.CreatedBy, uuid.New()),
				Mimetype:  takeFirst(orig.Mimetype, "application/x-tar"),
				Data:      takeFirstBytes(orig.Data, []byte{}),
			})
			require.NoError(t, err, "insert file")

			output[name] = file
		case database.UserLink:
			link, err := db.InsertUserLink(ctx, database.InsertUserLinkParams{
				UserID:            takeFirst(orig.UserID, uuid.New()),
				LoginType:         takeFirst(orig.LoginType, database.LoginTypeGithub),
				LinkedID:          takeFirst(orig.LinkedID),
				OAuthAccessToken:  takeFirst(orig.OAuthAccessToken, uuid.NewString()),
				OAuthRefreshToken: takeFirst(orig.OAuthAccessToken, uuid.NewString()),
				OAuthExpiry:       takeFirst(orig.OAuthExpiry, time.Now().Add(time.Hour*24)),
			})

			require.NoError(t, err, "insert link")

			output[name] = link
		default:
			panic(fmt.Sprintf("unknown type %T", orig))
		}
	}
	return output
}

func (g *Generator) Lookup(name string) uuid.UUID {
	if name == "" {
		// No name means the caller doesn't care about the ID.
		return uuid.New()
	}
	if g.names == nil {
		g.names = make(map[string]uuid.UUID)
	}
	if id, ok := g.names[name]; ok {
		return id
	}
	id := uuid.New()
	g.names[name] = id
	return id
}

// takeFirstBytes implements takeFirst for []byte.
// []byte is not a comparable type.
func takeFirstBytes(values ...[]byte) []byte {
	return takeFirstF(values, func(v []byte) bool {
		return len(v) != 0
	})
}

// takeFirstF takes the first value that returns true
func takeFirstF[Value any](values []Value, take func(v Value) bool) Value {
	var empty Value
	for _, v := range values {
		if take(v) {
			return v
		}
	}
	// If all empty, return empty
	return empty
}

// takeFirst will take the first non-empty value.
func takeFirst[Value comparable](values ...Value) Value {
	var empty Value
	return takeFirstF(values, func(v Value) bool {
		return v != empty
	})
}

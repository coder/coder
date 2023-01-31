package databasefake

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/cryptorand"
)

type Supported interface {
	database.APIKey | generatedAPIKey |
		database.File |
		database.UserLink |
		database.WorkspaceResource |
		database.ProvisionerJob |
		database.Group |
		database.Organization |
		database.Workspace |
		database.Template |
		database.TemplateVersion |
		database.WorkspaceBuild |
		database.User
}

type generatedAPIKey struct {
	Secret string
	Key    database.APIKey
}

// GenerateAPIKey is a special case that allows returning the secret for the
// api key.
func GenerateAPIKey(t *testing.T, db database.Store, seed database.APIKey) (key database.APIKey, secret string) {
	out := generate(t, db, generatedAPIKey{
		Key: seed,
	})
	v, ok := out.(generatedAPIKey)
	if !ok {
		t.Fatalf("Returned type '%T' doses not match expected '%T'", out, generatedAPIKey{})
	}
	return v.Key, v.Secret
}

func Generate[Object Supported](t *testing.T, db database.Store, seed Object) Object {
	out := generate(t, db, seed)
	v, ok := out.(Object)
	if !ok {
		var empty Object
		t.Fatalf("Returned type '%T' doses not match expected '%T'", out, empty)
	}
	return v
}

func generate(t *testing.T, db database.Store, seed interface{}) interface{} {
	t.Helper()

	if _, ok := db.(FakeDatabase); !ok {
		// This does not work for postgres databases because of foreign key
		// constraints
		t.Fatalf("Generate() db must be a FakeDatabase")
	}

	// db fake doesn't use contexts anyway.
	ctx := context.Background()

	switch orig := seed.(type) {
	case database.APIKey, generatedAPIKey:
		// Annoying, but we need a way to return the secret if
		// the caller needs it.
		var g generatedAPIKey
		v, isKey := seed.(database.APIKey)
		if isKey {
			g = generatedAPIKey{
				Key: v,
			}
		} else {
			var ok bool
			g, ok = seed.(generatedAPIKey)
			if !ok {
				t.Fatalf("type '%T' unsupported", seed)
			}
		}

		id, _ := cryptorand.String(10)
		secret, _ := cryptorand.String(22)
		hashed := sha256.Sum256([]byte(secret))

		key, err := db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID: takeFirst(g.Key.ID, id),
			// 0 defaults to 86400 at the db layer
			LifetimeSeconds: takeFirst(g.Key.LifetimeSeconds, 0),
			HashedSecret:    takeFirstBytes(g.Key.HashedSecret, hashed[:]),
			IPAddress:       pqtype.Inet{},
			UserID:          takeFirst(g.Key.UserID, uuid.New()),
			LastUsed:        takeFirst(g.Key.LastUsed, time.Now()),
			ExpiresAt:       takeFirst(g.Key.ExpiresAt, time.Now().Add(time.Hour)),
			CreatedAt:       takeFirst(g.Key.CreatedAt, time.Now()),
			UpdatedAt:       takeFirst(g.Key.UpdatedAt, time.Now()),
			LoginType:       takeFirst(g.Key.LoginType, database.LoginTypePassword),
			Scope:           takeFirst(g.Key.Scope, database.APIKeyScopeAll),
		})
		require.NoError(t, err, "insert api key")
		g.Key = key
		g.Secret = secret

		if isKey {
			return g.Key
		}
		return g
	case database.Template:
		template, err := db.InsertTemplate(ctx, database.InsertTemplateParams{
			ID:                           takeFirst(orig.ID, uuid.New()),
			CreatedAt:                    takeFirst(orig.CreatedAt, time.Now()),
			UpdatedAt:                    takeFirst(orig.UpdatedAt, time.Now()),
			OrganizationID:               takeFirst(orig.OrganizationID, uuid.New()),
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
		return template
	case database.TemplateVersion:
		version, err := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
			ID: takeFirst(orig.ID, uuid.New()),
			TemplateID: uuid.NullUUID{
				UUID:  takeFirst(orig.TemplateID.UUID, uuid.New()),
				Valid: takeFirst(orig.TemplateID.Valid, true),
			},
			OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
			CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
			UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
			Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
			Readme:         takeFirst(orig.Readme, namesgenerator.GetRandomName(1)),
			JobID:          takeFirst(orig.JobID, uuid.New()),
			CreatedBy:      takeFirst(orig.CreatedBy, uuid.New()),
		})
		require.NoError(t, err, "insert template version")
		return version
	case database.Workspace:
		workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:                takeFirst(orig.ID, uuid.New()),
			OwnerID:           takeFirst(orig.OwnerID, uuid.New()),
			CreatedAt:         takeFirst(orig.CreatedAt, time.Now()),
			UpdatedAt:         takeFirst(orig.UpdatedAt, time.Now()),
			OrganizationID:    takeFirst(orig.OrganizationID, uuid.New()),
			TemplateID:        takeFirst(orig.TemplateID, uuid.New()),
			Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
			AutostartSchedule: orig.AutostartSchedule,
			Ttl:               orig.Ttl,
		})
		require.NoError(t, err, "insert workspace")
		return workspace
	case database.WorkspaceBuild:
		build, err := db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:                takeFirst(orig.ID, uuid.New()),
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
		return build
	case database.User:
		user, err := db.InsertUser(ctx, database.InsertUserParams{
			ID:             takeFirst(orig.ID, uuid.New()),
			Email:          takeFirst(orig.Email, namesgenerator.GetRandomName(1)),
			Username:       takeFirst(orig.Username, namesgenerator.GetRandomName(1)),
			HashedPassword: takeFirstBytes(orig.HashedPassword, []byte{}),
			CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
			UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
			RBACRoles:      []string{},
			LoginType:      takeFirst(orig.LoginType, database.LoginTypePassword),
		})
		require.NoError(t, err, "insert user")
		return user
	case database.Organization:
		org, err := db.InsertOrganization(ctx, database.InsertOrganizationParams{
			ID:          takeFirst(orig.ID, uuid.New()),
			Name:        takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
			Description: takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
			CreatedAt:   takeFirst(orig.CreatedAt, time.Now()),
			UpdatedAt:   takeFirst(orig.UpdatedAt, time.Now()),
		})
		require.NoError(t, err, "insert organization")
		return org
	case database.Group:
		group, err := db.InsertGroup(ctx, database.InsertGroupParams{
			ID:             takeFirst(orig.ID, uuid.New()),
			Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
			OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
			AvatarURL:      takeFirst(orig.AvatarURL, "https://logo.example.com"),
			QuotaAllowance: takeFirst(orig.QuotaAllowance, 0),
		})
		require.NoError(t, err, "insert group")
		return group
	case database.ProvisionerJob:
		job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             takeFirst(orig.ID, uuid.New()),
			CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
			UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
			OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
			InitiatorID:    takeFirst(orig.InitiatorID, uuid.New()),
			Provisioner:    takeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
			StorageMethod:  takeFirst(orig.StorageMethod, database.ProvisionerStorageMethodFile),
			FileID:         takeFirst(orig.FileID, uuid.New()),
			Type:           takeFirst(orig.Type, database.ProvisionerJobTypeWorkspaceBuild),
			Input:          takeFirstBytes(orig.Input, []byte("{}")),
			Tags:           orig.Tags,
		})
		require.NoError(t, err, "insert job")
		return job
	case database.WorkspaceResource:
		resource, err := db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
			ID:         takeFirst(orig.ID, uuid.New()),
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
		return resource
	case database.File:
		file, err := db.InsertFile(ctx, database.InsertFileParams{
			ID:        takeFirst(orig.ID, uuid.New()),
			Hash:      takeFirst(orig.Hash, hex.EncodeToString(make([]byte, 32))),
			CreatedAt: takeFirst(orig.CreatedAt, time.Now()),
			CreatedBy: takeFirst(orig.CreatedBy, uuid.New()),
			Mimetype:  takeFirst(orig.Mimetype, "application/x-tar"),
			Data:      takeFirstBytes(orig.Data, []byte{}),
		})
		require.NoError(t, err, "insert file")
		return file
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
		return link
	default:
		// If you hit this, just add your type to the switch.
		t.Fatalf("unknown type '%T' used in fake data generator", orig)
		// This line will never be hit, but the compiler does not know that :/
		return nil
	}
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

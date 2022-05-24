package audit_test

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	runDiffTests(t, []diffTest[database.GitSSHKey]{
		{
			name: "Create",
			left: audit.Empty[database.GitSSHKey](),
			right: database.GitSSHKey{
				UserID:     uuid.UUID{1},
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				PrivateKey: "a very secret private key",
				PublicKey:  "a very public public key",
			},
			exp: audit.Map{
				"user_id":     uuid.UUID{1}.String(),
				"private_key": "",
				"public_key":  "a very public public key",
			},
		},
	})

	runDiffTests(t, []diffTest[database.OrganizationMember]{
		{
			name: "Create",
			left: audit.Empty[database.OrganizationMember](),
			right: database.OrganizationMember{
				UserID:         uuid.UUID{1},
				OrganizationID: uuid.UUID{2},
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				Roles:          []string{"auditor"},
			},
			exp: audit.Map{
				"user_id":         uuid.UUID{1}.String(),
				"organization_id": uuid.UUID{2}.String(),
				"roles":           []string{"auditor"},
			},
		},
	})

	runDiffTests(t, []diffTest[database.Organization]{
		{
			name: "Create",
			left: audit.Empty[database.Organization](),
			right: database.Organization{
				ID:          uuid.UUID{1},
				Name:        "rust developers",
				Description: "an organization for rust developers",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			exp: audit.Map{
				"id":          uuid.UUID{1}.String(),
				"name":        "rust developers",
				"description": "an organization for rust developers",
			},
		},
	})

	runDiffTests(t, []diffTest[database.Template]{
		{
			name: "Create",
			left: audit.Empty[database.Template](),
			right: database.Template{
				ID:              uuid.UUID{1},
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
				OrganizationID:  uuid.UUID{2},
				Deleted:         false,
				Name:            "rust",
				Provisioner:     database.ProvisionerTypeTerraform,
				ActiveVersionID: uuid.UUID{3},
			},
			exp: audit.Map{
				"id":                uuid.UUID{1}.String(),
				"organization_id":   uuid.UUID{2}.String(),
				"name":              "rust",
				"provisioner":       database.ProvisionerTypeTerraform,
				"active_version_id": uuid.UUID{3}.String(),
			},
		},
	})

	runDiffTests(t, []diffTest[database.TemplateVersion]{
		{
			name: "Create",
			left: audit.Empty[database.TemplateVersion](),
			right: database.TemplateVersion{
				ID:             uuid.UUID{1},
				TemplateID:     uuid.NullUUID{UUID: uuid.UUID{2}, Valid: true},
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				OrganizationID: uuid.UUID{3},
				Name:           "rust",
			},
			exp: audit.Map{
				"id":              uuid.UUID{1}.String(),
				"template_id":     uuid.UUID{2}.String(),
				"organization_id": uuid.UUID{3}.String(),
				"name":            "rust",
			},
		},
		{
			name: "CreateNullTemplateID",
			left: audit.Empty[database.TemplateVersion](),
			right: database.TemplateVersion{
				ID:             uuid.UUID{1},
				TemplateID:     uuid.NullUUID{},
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				OrganizationID: uuid.UUID{3},
				Name:           "rust",
			},
			exp: audit.Map{
				"id":              uuid.UUID{1}.String(),
				"organization_id": uuid.UUID{3}.String(),
				"name":            "rust",
			},
		},
	})

	runDiffTests(t, []diffTest[database.User]{
		{
			name: "Create",
			left: audit.Empty[database.User](),
			right: database.User{
				ID:             uuid.UUID{1},
				Email:          "colin@coder.com",
				Username:       "colin",
				HashedPassword: []byte("hunter2ButHashed"),
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				Status:         database.UserStatusActive,
				RBACRoles:      []string{"omega admin"},
			},
			exp: audit.Map{
				"id":              uuid.UUID{1}.String(),
				"email":           "colin@coder.com",
				"username":        "colin",
				"hashed_password": ([]byte)(nil),
				"status":          database.UserStatusActive,
				"rbac_roles":      []string{"omega admin"},
			},
		},
	})

	runDiffTests(t, []diffTest[database.Workspace]{
		{
			name: "Create",
			left: audit.Empty[database.Workspace](),
			right: database.Workspace{
				ID:                uuid.UUID{1},
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				OwnerID:           uuid.UUID{2},
				TemplateID:        uuid.UUID{3},
				Name:              "rust workspace",
				AutostartSchedule: sql.NullString{String: "0 12 * * 1-5", Valid: true},
				Ttl:               sql.NullInt64{Int64: int64(8 * time.Hour), Valid: true},
			},
			exp: audit.Map{
				"id":                 uuid.UUID{1}.String(),
				"owner_id":           uuid.UUID{2}.String(),
				"template_id":        uuid.UUID{3}.String(),
				"name":               "rust workspace",
				"autostart_schedule": "0 12 * * 1-5",
				"ttl":                int64(28800000000000), // XXX: pq still does not support time.Duration
			},
		},
		{
			name: "NullSchedules",
			left: audit.Empty[database.Workspace](),
			right: database.Workspace{
				ID:                uuid.UUID{1},
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				OwnerID:           uuid.UUID{2},
				TemplateID:        uuid.UUID{3},
				Name:              "rust workspace",
				AutostartSchedule: sql.NullString{},
				Ttl:               sql.NullInt64{},
			},
			exp: audit.Map{
				"id":          uuid.UUID{1}.String(),
				"owner_id":    uuid.UUID{2}.String(),
				"template_id": uuid.UUID{3}.String(),
				"name":        "rust workspace",
			},
		},
	})
}

type diffTest[T audit.Auditable] struct {
	name        string
	left, right T
	exp         audit.Map
}

func runDiffTests[T audit.Auditable](t *testing.T, tests []diffTest[T]) {
	t.Helper()

	var typ T
	typName := reflect.TypeOf(typ).Name()

	for _, test := range tests {
		t.Run(typName+"/"+test.name, func(t *testing.T) {
			require.Equal(t,
				test.exp,
				audit.Diff(test.left, test.right),
			)
		})
	}
}

package audit

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
)

func Test_diffValues(t *testing.T) {
	t.Parallel()

	t.Run("Normal", func(t *testing.T) {
		t.Parallel()

		type foo struct {
			Bar string `json:"bar"`
			Baz int64  `json:"baz"`
		}

		table := auditMap(map[any]map[string]Action{
			&foo{}: {
				"bar": ActionTrack,
				"baz": ActionTrack,
			},
		})

		runDiffValuesTests(t, table, []diffTest{
			{
				name: "LeftEmpty",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "bar", Baz: 10},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "", New: "bar"},
					"baz": audit.OldNew{Old: 0, New: 10},
				},
			},
			{
				name: "RightEmpty",
				left: foo{Bar: "Bar", Baz: 10}, right: foo{Bar: "", Baz: 0},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "Bar", New: ""},
					"baz": audit.OldNew{Old: 10, New: 0},
				},
			},
			{
				name: "NoChange",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "", Baz: 0},
				exp: audit.Map{},
			},
			{
				name: "SingleFieldChange",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "Bar", Baz: 0},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "", New: "Bar"},
				},
			},
		})
	})

	t.Run("PointerField", func(t *testing.T) {
		t.Parallel()

		type foo struct {
			Bar *string `json:"bar"`
		}

		table := auditMap(map[any]map[string]Action{
			&foo{}: {
				"bar": ActionTrack,
			},
		})

		runDiffValuesTests(t, table, []diffTest{
			{
				name: "LeftNil",
				left: foo{Bar: nil}, right: foo{Bar: pointer.StringPtr("baz")},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "", New: "baz"},
				},
			},
			{
				name: "RightNil",
				left: foo{Bar: pointer.StringPtr("baz")}, right: foo{Bar: nil},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "baz", New: ""},
				},
			},
		})
	})

	t.Run("NestedStruct", func(t *testing.T) {
		t.Parallel()

		type bar struct {
			Baz string `json:"baz"`
		}

		type foo struct {
			Bar *bar `json:"bar"`
		}

		table := auditMap(map[any]map[string]Action{
			&foo{}: {
				"bar": ActionTrack,
			},
			&bar{}: {
				"baz": ActionTrack,
			},
		})

		runDiffValuesTests(t, table, []diffTest{
			{
				name: "LeftEmpty",
				left: foo{Bar: &bar{}}, right: foo{Bar: &bar{Baz: "baz"}},
				exp: audit.Map{
					"bar": audit.Map{
						"baz": audit.OldNew{Old: "", New: "baz"},
					},
				},
			},
			{
				name: "RightEmpty",
				left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: &bar{}},
				exp: audit.Map{
					"bar": audit.Map{
						"baz": audit.OldNew{Old: "baz", New: ""},
					},
				},
			},
			{
				name: "LeftNil",
				left: foo{Bar: nil}, right: foo{Bar: &bar{}},
				exp: audit.Map{
					"bar": audit.Map{},
				},
			},
			{
				name: "RightNil",
				left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: nil},
				exp: audit.Map{
					"bar": audit.Map{
						"baz": audit.OldNew{Old: "baz", New: ""},
					},
				},
			},
		})
	})
}

type diffTest struct {
	name        string
	left, right any
	exp         any
}

func runDiffValuesTests(t *testing.T, table Table, tests []diffTest) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t,
				test.exp,
				diffValues(test.left, test.right, table),
			)
		})
	}
}

func Test_diff(t *testing.T) {
	t.Parallel()

	runDiffTests(t, []diffTest{
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

	runDiffTests(t, []diffTest{
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

	runDiffTests(t, []diffTest{
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

	runDiffTests(t, []diffTest{
		{
			name: "Create",
			left: audit.Empty[database.Template](),
			right: database.Template{
				ID:                   uuid.UUID{1},
				CreatedAt:            time.Now(),
				UpdatedAt:            time.Now(),
				OrganizationID:       uuid.UUID{2},
				Deleted:              false,
				Name:                 "rust",
				Provisioner:          database.ProvisionerTypeTerraform,
				ActiveVersionID:      uuid.UUID{3},
				MaxTtl:               int64(time.Hour),
				MinAutostartInterval: int64(time.Minute),
				CreatedBy:            uuid.UUID{4},
			},
			exp: audit.Map{
				"id":                     uuid.UUID{1}.String(),
				"organization_id":        uuid.UUID{2}.String(),
				"name":                   "rust",
				"provisioner":            database.ProvisionerTypeTerraform,
				"active_version_id":      uuid.UUID{3}.String(),
				"max_ttl":                int64(3600000000000),
				"min_autostart_interval": int64(60000000000),
				"created_by":             uuid.UUID{4}.String(),
			},
		},
	})

	runDiffTests(t, []diffTest{
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
				CreatedBy:      uuid.NullUUID{UUID: uuid.UUID{4}, Valid: true},
			},
			exp: audit.Map{
				"id":              uuid.UUID{1}.String(),
				"template_id":     uuid.UUID{2}.String(),
				"organization_id": uuid.UUID{3}.String(),
				"name":            "rust",
				"created_by":      uuid.UUID{4}.String(),
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
				CreatedBy:      uuid.NullUUID{UUID: uuid.UUID{4}, Valid: true},
			},
			exp: audit.Map{
				"id":              uuid.UUID{1}.String(),
				"organization_id": uuid.UUID{3}.String(),
				"name":            "rust",
				"created_by":      uuid.UUID{4}.String(),
			},
		},
	})

	runDiffTests(t, []diffTest{
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

	runDiffTests(t, []diffTest{
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

func runDiffTests(t *testing.T, tests []diffTest) {
	t.Helper()

	for _, test := range tests {
		typName := reflect.TypeOf(test.left).Name()

		t.Run(typName+"/"+test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t,
				test.exp,
				(&auditor{}).diff(test.left, test.right),
			)
		})
	}
}

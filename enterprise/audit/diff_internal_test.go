package audit

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

func Test_diffValues(t *testing.T) {
	t.Parallel()

	t.Run("Normal", func(t *testing.T) {
		t.Parallel()

		type foo struct {
			Bar string `json:"bar"`
			Baz int    `json:"baz"`
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
				left: foo{Bar: nil}, right: foo{Bar: ptr.Ref("baz")},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "", New: "baz"},
				},
			},
			{
				name: "RightNil",
				left: foo{Bar: ptr.Ref("baz")}, right: foo{Bar: nil},
				exp: audit.Map{
					"bar": audit.OldNew{Old: "baz", New: ""},
				},
			},
		})
	})

	//nolint:revive
	t.Run("EmbeddedStruct", func(t *testing.T) {
		t.Parallel()

		type Bar struct {
			Baz  int    `json:"baz"`
			Buzz string `json:"buzz"`
		}

		type PtrBar struct {
			Qux string `json:"qux"`
		}

		type foo struct {
			Bar
			*PtrBar
			TopLevel string `json:"top_level"`
		}

		table := auditMap(map[any]map[string]Action{
			&foo{}: {
				"baz":       ActionTrack,
				"buzz":      ActionTrack,
				"qux":       ActionTrack,
				"top_level": ActionTrack,
			},
		})

		runDiffValuesTests(t, table, []diffTest{
			{
				name:  "SingleFieldChange",
				left:  foo{TopLevel: "top-before", Bar: Bar{Baz: 1, Buzz: "before"}, PtrBar: &PtrBar{Qux: "qux-before"}},
				right: foo{TopLevel: "top-after", Bar: Bar{Baz: 0, Buzz: "after"}, PtrBar: &PtrBar{Qux: "qux-after"}},
				exp: audit.Map{
					"baz":       audit.OldNew{Old: 1, New: 0},
					"buzz":      audit.OldNew{Old: "before", New: "after"},
					"qux":       audit.OldNew{Old: "qux-before", New: "qux-after"},
					"top_level": audit.OldNew{Old: "top-before", New: "top-after"},
				},
			},
			{
				name:  "Empty",
				left:  foo{},
				right: foo{},
				exp:   audit.Map{},
			},
			{
				name:  "NoChange",
				left:  foo{TopLevel: "top-before", Bar: Bar{Baz: 1, Buzz: "before"}, PtrBar: &PtrBar{Qux: "qux-before"}},
				right: foo{TopLevel: "top-before", Bar: Bar{Baz: 1, Buzz: "before"}, PtrBar: &PtrBar{Qux: "qux-before"}},
				exp:   audit.Map{},
			},
			{
				name:  "LeftEmpty",
				left:  foo{},
				right: foo{TopLevel: "top-after", Bar: Bar{Baz: 1, Buzz: "after"}, PtrBar: &PtrBar{Qux: "qux-after"}},
				exp: audit.Map{
					"baz":       audit.OldNew{Old: 0, New: 1},
					"buzz":      audit.OldNew{Old: "", New: "after"},
					"qux":       audit.OldNew{Old: "", New: "qux-after"},
					"top_level": audit.OldNew{Old: "", New: "top-after"},
				},
			},
			{
				name:  "RightNil",
				left:  foo{TopLevel: "top-before", Bar: Bar{Baz: 1, Buzz: "before"}, PtrBar: &PtrBar{Qux: "qux-before"}},
				right: foo{},
				exp: audit.Map{
					"baz":       audit.OldNew{Old: 1, New: 0},
					"buzz":      audit.OldNew{Old: "before", New: ""},
					"qux":       audit.OldNew{Old: "qux-before", New: ""},
					"top_level": audit.OldNew{Old: "top-before", New: ""},
				},
			},
		})
	})

	// We currently don't support nested structs.
	// t.Run("NestedStruct", func(t *testing.T) {
	// 	t.Parallel()

	// 	type bar struct {
	// 		Baz string `json:"baz"`
	// 	}

	// 	type foo struct {
	// 		Bar *bar `json:"bar"`
	// 	}

	// 	table := auditMap(map[any]map[string]Action{
	// 		&foo{}: {
	// 			"bar": ActionTrack,
	// 		},
	// 		&bar{}: {
	// 			"baz": ActionTrack,
	// 		},
	// 	})

	// 	runDiffValuesTests(t, table, []diffTest{
	// 		{
	// 			name: "LeftEmpty",
	// 			left: foo{Bar: &bar{}}, right: foo{Bar: &bar{Baz: "baz"}},
	// 			exp: audit.Map{
	// 				"bar": audit.Map{
	// 					"baz": audit.OldNew{Old: "", New: "baz"},
	// 				},
	// 			},
	// 		},
	// 		{
	// 			name: "RightEmpty",
	// 			left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: &bar{}},
	// 			exp: audit.Map{
	// 				"bar": audit.Map{
	// 					"baz": audit.OldNew{Old: "baz", New: ""},
	// 				},
	// 			},
	// 		},
	// 		{
	// 			name: "LeftNil",
	// 			left: foo{Bar: nil}, right: foo{Bar: &bar{}},
	// 			exp: audit.Map{
	// 				"bar": audit.Map{},
	// 			},
	// 		},
	// 		{
	// 			name: "RightNil",
	// 			left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: nil},
	// 			exp: audit.Map{
	// 				"bar": audit.Map{
	// 					"baz": audit.OldNew{Old: "baz", New: ""},
	// 				},
	// 			},
	// 		},
	// 	})
	// })
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
				"user_id":     audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"private_key": audit.OldNew{Old: "", New: "", Secret: true},
				"public_key":  audit.OldNew{Old: "", New: "a very public public key"},
			},
		},
	})

	runDiffTests(t, []diffTest{
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
				DefaultTTL:      int64(time.Hour),
				CreatedBy:       uuid.UUID{4},
			},
			exp: audit.Map{
				"id":                audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"name":              audit.OldNew{Old: "", New: "rust"},
				"provisioner":       audit.OldNew{Old: database.ProvisionerType(""), New: database.ProvisionerTypeTerraform},
				"active_version_id": audit.OldNew{Old: "", New: uuid.UUID{3}.String()},
				"default_ttl":       audit.OldNew{Old: int64(0), New: int64(time.Hour)},
				"created_by":        audit.OldNew{Old: "", New: uuid.UUID{4}.String()},
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
				CreatedBy:      uuid.UUID{4},
			},
			exp: audit.Map{
				"id":          audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"template_id": audit.OldNew{Old: "null", New: uuid.UUID{2}.String()},
				"created_by":  audit.OldNew{Old: "", New: uuid.UUID{4}.String()},
				"name":        audit.OldNew{Old: "", New: "rust"},
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
				CreatedBy:      uuid.UUID{4},
			},
			exp: audit.Map{
				"id":         audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"created_by": audit.OldNew{Old: "", New: uuid.UUID{4}.String()},
				"name":       audit.OldNew{Old: "", New: "rust"},
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
				"id":              audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"email":           audit.OldNew{Old: "", New: "colin@coder.com"},
				"username":        audit.OldNew{Old: "", New: "colin"},
				"hashed_password": audit.OldNew{Old: ([]byte)(nil), New: ([]byte)(nil), Secret: true},
				"status":          audit.OldNew{Old: database.UserStatus(""), New: database.UserStatusActive},
				"rbac_roles":      audit.OldNew{Old: (pq.StringArray)(nil), New: pq.StringArray{"omega admin"}},
			},
		},
	})

	runDiffTests(t, []diffTest{
		{
			name: "Create",
			left: audit.Empty[database.WorkspaceTable](),
			right: database.WorkspaceTable{
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
				"id":                 audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"owner_id":           audit.OldNew{Old: "", New: uuid.UUID{2}.String()},
				"template_id":        audit.OldNew{Old: "", New: uuid.UUID{3}.String()},
				"name":               audit.OldNew{Old: "", New: "rust workspace"},
				"autostart_schedule": audit.OldNew{Old: "null", New: "0 12 * * 1-5"},
				"ttl":                audit.OldNew{Old: int64(0), New: int64(8 * time.Hour)}, // XXX: pq still does not support time.Duration
			},
		},
		{
			name: "NullSchedules",
			left: audit.Empty[database.WorkspaceTable](),
			right: database.WorkspaceTable{
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
				"id":          audit.OldNew{Old: "", New: uuid.UUID{1}.String()},
				"owner_id":    audit.OldNew{Old: "", New: uuid.UUID{2}.String()},
				"template_id": audit.OldNew{Old: "", New: uuid.UUID{3}.String()},
				"name":        audit.OldNew{Old: "", New: "rust workspace"},
			},
		},
	})
}

func runDiffTests(t *testing.T, tests []diffTest) {
	t.Helper()

	for _, test := range tests {
		test := test
		typName := reflect.TypeOf(test.left).Name()
		t.Run(typName+"/"+test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t,
				test.exp,
				diffValues(test.left, test.right, AuditableResources),
			)
		})
	}
}

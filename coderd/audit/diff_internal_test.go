package audit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
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

		runDiffTests(t, table, []diffTest{
			{
				name: "LeftEmpty",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "bar", Baz: 10},
				exp: Map{
					"bar": "bar",
					"baz": int64(10),
				},
			},
			{
				name: "RightEmpty",
				left: foo{Bar: "Bar", Baz: 10}, right: foo{Bar: "", Baz: 0},
				exp: Map{
					"bar": "",
					"baz": int64(0),
				},
			},
			{
				name: "NoChange",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "", Baz: 0},
				exp: Map{},
			},
			{
				name: "SingleFieldChange",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "Bar", Baz: 0},
				exp: Map{
					"bar": "Bar",
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

		runDiffTests(t, table, []diffTest{
			{
				name: "LeftNil",
				left: foo{Bar: nil}, right: foo{Bar: pointer.StringPtr("baz")},
				exp: Map{"bar": "baz"},
			},
			{
				name: "RightNil",
				left: foo{Bar: pointer.StringPtr("baz")}, right: foo{Bar: nil},
				exp: Map{"bar": ""},
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

		runDiffTests(t, table, []diffTest{
			{
				name: "LeftEmpty",
				left: foo{Bar: &bar{}}, right: foo{Bar: &bar{Baz: "baz"}},
				exp: Map{
					"bar": Map{
						"baz": "baz",
					},
				},
			},
			{
				name: "RightEmpty",
				left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: &bar{}},
				exp: Map{
					"bar": Map{
						"baz": "",
					},
				},
			},
			{
				name: "LeftNil",
				left: foo{Bar: nil}, right: foo{Bar: &bar{}},
				exp: Map{
					"bar": Map{},
				},
			},
			{
				name: "RightNil",
				left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: nil},
				exp: Map{
					"bar": Map{
						"baz": "",
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

func runDiffTests(t *testing.T, table Table, tests []diffTest) {
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

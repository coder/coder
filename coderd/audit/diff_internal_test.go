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
				exp: DiffMap{
					"bar": "bar",
					"baz": int64(10),
				},
			},
			{
				name: "RightEmpty",
				left: foo{Bar: "Bar", Baz: 10}, right: foo{Bar: "", Baz: 0},
				exp: DiffMap{
					"bar": "",
					"baz": int64(0),
				},
			},
			{
				name: "NoChange",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "", Baz: 0},
				exp: DiffMap{},
			},
			{
				name: "SingleFieldChange",
				left: foo{Bar: "", Baz: 0}, right: foo{Bar: "Bar", Baz: 0},
				exp: DiffMap{
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
				exp: DiffMap{"bar": "baz"},
			},
			{
				name: "RightNil",
				left: foo{Bar: pointer.StringPtr("baz")}, right: foo{Bar: nil},
				exp: DiffMap{"bar": ""},
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
				exp: DiffMap{
					"bar": DiffMap{
						"baz": "baz",
					},
				},
			},
			{
				name: "RightEmpty",
				left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: &bar{}},
				exp: DiffMap{
					"bar": DiffMap{
						"baz": "",
					},
				},
			},
			{
				name: "LeftNil",
				left: foo{Bar: nil}, right: foo{Bar: &bar{}},
				exp: DiffMap{
					"bar": DiffMap{},
				},
			},
			{
				name: "RightNil",
				left: foo{Bar: &bar{Baz: "baz"}}, right: foo{Bar: nil},
				exp: DiffMap{
					"bar": DiffMap{
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

func runDiffTests(t *testing.T, table Map, tests []diffTest) {
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

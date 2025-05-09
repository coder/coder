package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestFindResourceReplacements(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		plan     *tfjson.Plan
		expected resourceReplacements
	}{
		{
			name: "nil plan",
		},
		{
			name: "no resource changes",
			plan: &tfjson.Plan{},
		},
		{
			name: "resource change with nil change",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
					},
				},
			},
		},
		{
			name: "no-op action",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionNoop},
						},
					},
				},
			},
		},
		{
			name: "empty replace paths",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
						},
					},
				},
			},
		},
		{
			name: "coder_* types are ignored",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "coder_resource",
						Change: &tfjson.Change{
							Actions:      tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{"path1"},
						},
					},
				},
			},
		},
		{
			name: "valid replacements - single path",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions:      tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{"path1"},
						},
					},
				},
			},
			expected: resourceReplacements{
				"resource1": {"path1"},
			},
		},
		{
			name: "valid replacements - multiple paths",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions:      tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{"path1", "path2"},
						},
					},
				},
			},
			expected: resourceReplacements{
				"resource1": {"path1", "path2"},
			},
		},
		{
			name: "complex replace path",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"path", "to", "key"},
							},
						},
					},
				},
			},
			expected: resourceReplacements{
				"resource1": {"path.to.key"},
			},
		},
		{
			name: "multiple changes",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions:      tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{"path1"},
						},
					},
					{
						Address: "resource2",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions:      tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{"path2", "path3"},
						},
					},
					{
						Address: "resource3",
						Type:    "coder_example",
						Change: &tfjson.Change{
							Actions:      tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{"ignored_path"},
						},
					},
				},
			},
			expected: resourceReplacements{
				"resource1": {"path1"},
				"resource2": {"path2", "path3"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.EqualValues(t, tc.expected, findResourceReplacements(tc.plan))
		})
	}
}

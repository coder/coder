package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestFindResourceReplacementsWithPaths(t *testing.T) {
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

			require.EqualValues(t, tc.expected, findResourceReplacementsWithPaths(tc.plan))
		})
	}
}

func TestFindAllResourceReplacements(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		plan     *tfjson.Plan
		expected []resourceReplacementEntry
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
			name: "non-replacement action",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionUpdate},
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
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
						},
					},
				},
			},
		},
		{
			name: "pathless replacement is included",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
						},
					},
				},
			},
			expected: []resourceReplacementEntry{
				{resource: "resource1", paths: []string{}},
			},
		},
		{
			name: "replacement paths are formatted",
			plan: &tfjson.Plan{
				ResourceChanges: []*tfjson.ResourceChange{
					{
						Address: "resource1",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []any{
								"ami",
								[]any{"root_block_device", 0, "volume_size"},
							},
						},
					},
				},
			},
			expected: []resourceReplacementEntry{
				{
					resource: "resource1",
					paths: []string{
						"ami",
						"root_block_device.0.volume_size",
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := findAllResourceReplacements(tc.plan)
			if tc.expected == nil {
				require.Empty(t, actual)
				return
			}

			require.EqualValues(t, tc.expected, actual)
		})
	}
}

func TestLogResourceReplacements(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	logResourceReplacements([]resourceReplacementEntry{
		{resource: "z_resource", paths: []string{"name"}},
		{resource: "a_resource", paths: []string{"root_block_device.0.volume_size", "ami"}},
	}, logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ a_resource (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ ami (forces replacement)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ root_block_device.0.volume_size (forces replacement)"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ z_resource (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ name (forces replacement)"},
	}, logr.logs)
}

func TestLogResourceReplacementsIncludesPathlessReplacements(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	logResourceReplacements([]resourceReplacementEntry{
		{resource: "example_resource.pathless"},
	}, logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.pathless (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ replacement reason unavailable"},
	}, logr.logs)
}

func TestLogResourceReplacementsDoesNotLogValues(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.sensitive",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
					// Populate before/after values to prove
					// replacement logging only uses ReplacePaths
					// and never logs resource values.
					Before: map[string]any{
						"secret": "old-secret-value",
					},
					After: map[string]any{
						"secret": "new-secret-value",
					},
					BeforeSensitive: map[string]any{
						"secret": true,
					},
					AfterSensitive: map[string]any{
						"secret": true,
					},
					ReplacePaths: []any{"secret"},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.sensitive (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ secret (forces replacement)"},
	}, logr.logs)
}

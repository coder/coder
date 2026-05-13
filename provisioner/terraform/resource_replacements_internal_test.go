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
		expected resourceReplacementPaths
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
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"path1"},
							},
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
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"path1"},
							},
						},
					},
				},
			},
			expected: resourceReplacementPaths{
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
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"path1"},
								[]interface{}{"path2"},
							},
						},
					},
				},
			},
			expected: resourceReplacementPaths{
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
			expected: resourceReplacementPaths{
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
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"path1"},
							},
						},
					},
					{
						Address: "resource2",
						Type:    "example_resource",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"path2"},
								[]interface{}{"path3"},
							},
						},
					},
					{
						Address: "resource3",
						Type:    "coder_example",
						Change: &tfjson.Change{
							Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
							ReplacePaths: []interface{}{
								[]interface{}{"ignored_path"},
							},
						},
					},
				},
			},
			expected: resourceReplacementPaths{
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
		expected []replacementLogEntry
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
			expected: []replacementLogEntry{
				{resource: "resource1"},
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
								[]any{"ami"},
								[]any{"root_block_device", 0, "volume_size"},
							},
						},
					},
				},
			},
			expected: []replacementLogEntry{
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

func TestHasResourceReplacement(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		plan     *tfjson.Plan
		expected bool
	}{
		{
			name: "nil plan",
		},
		{
			name: "pathless replacement",
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
			expected: true,
		},
		{
			name: "coder replacement is ignored",
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, hasResourceReplacement(tc.plan))
		})
	}
}

func TestLogResourceReplacements(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	logResourceReplacements([]replacementLogEntry{
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

func TestLogResourceReplacementsIncludesValues(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.changed",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
					Before: map[string]any{
						"ami": "ami-old",
						"ebs_block_device": []any{
							map[string]any{
								"volume_size": float64(100),
							},
						},
						"root_block_device": []any{
							map[string]any{
								"volume_size": float64(30),
							},
						},
					},
					After: map[string]any{
						"ami": "ami-new",
						"ebs_block_device": []any{
							map[string]any{
								"volume_size": float64(200),
							},
						},
						"root_block_device": []any{
							map[string]any{
								"volume_size": float64(60),
							},
						},
					},
					ReplacePaths: []any{
						[]any{"ami"},
						[]any{"ebs_block_device", float64(0), "volume_size"},
						[]any{"root_block_device", 0, "volume_size"},
					},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.changed (replace)"},
		{Level: proto.LogLevel_WARN, Output: `      ~ ami: "ami-old" -> "ami-new" (forces replacement)`},
		{Level: proto.LogLevel_WARN, Output: "      ~ ebs_block_device.0.volume_size: 100 -> 200 (forces replacement)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ root_block_device.0.volume_size: 30 -> 60 (forces replacement)"},
	}, logr.logs)
}

func TestLogResourceReplacementsFormatsComplexValuesAsJSON(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.complex",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
					Before: map[string]any{
						"subject": []any{
							map[string]any{
								"common_name": "old",
								"country":     nil,
							},
						},
					},
					After: map[string]any{
						"subject": []any{
							map[string]any{
								"common_name": "new",
								"country":     nil,
							},
						},
					},
					ReplacePaths: []any{
						[]any{"subject"},
					},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.complex (replace)"},
		{Level: proto.LogLevel_WARN, Output: `      ~ subject: [{"common_name":"old","country":null}] -> [{"common_name":"new","country":null}] (forces replacement)`},
	}, logr.logs)
}

func TestLogResourceReplacementsIncludesPartialValues(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.partial",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
					Before: map[string]any{
						"before_only": "old-value",
					},
					After: map[string]any{
						"after_only": "new-value",
					},
					ReplacePaths: []any{
						[]any{"before_only"},
						[]any{"after_only"},
					},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.partial (replace)"},
		{Level: proto.LogLevel_WARN, Output: `      ~ after_only: (unavailable) -> "new-value" (forces replacement)`},
		{Level: proto.LogLevel_WARN, Output: `      ~ before_only: "old-value" -> (unavailable) (forces replacement)`},
	}, logr.logs)
}

func TestLogResourceReplacementsIncludesPathlessReplacements(t *testing.T) {
	t.Parallel()

	logr := &mockLogger{}
	logResourceReplacements([]replacementLogEntry{
		{resource: "example_resource.pathless"},
	}, logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.pathless (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ replacement reason unavailable"},
	}, logr.logs)
}

func TestLogResourceReplacementsRedactsSensitiveValues(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.sensitive",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
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
					ReplacePaths: []any{
						[]any{"secret"},
					},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.sensitive (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ secret: (sensitive value) -> (sensitive value) (forces replacement)"},
	}, logr.logs)
}

func TestLogResourceReplacementsRedactsParentPathsWithSensitiveChildren(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.sensitive_child",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
					Before: map[string]any{
						"subject": []any{
							map[string]any{
								"common_name":  "old-secret-value",
								"organization": "Coder",
							},
						},
					},
					After: map[string]any{
						"subject": []any{
							map[string]any{
								"common_name":  "new-secret-value",
								"organization": "Coder",
							},
						},
					},
					BeforeSensitive: map[string]any{
						"subject": []any{
							map[string]any{
								"common_name": true,
							},
						},
					},
					AfterSensitive: map[string]any{
						"subject": []any{
							map[string]any{
								"common_name": true,
							},
						},
					},
					// Terraform can report both a parent path and a nested
					// child path as replacement-causing, while only marking
					// the child value sensitive.
					ReplacePaths: []any{
						[]any{"subject"},
						[]any{"subject", 0, "common_name"},
					},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.sensitive_child (replace)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ subject: (sensitive value) -> (sensitive value) (forces replacement)"},
		{Level: proto.LogLevel_WARN, Output: "      ~ subject.0.common_name: (sensitive value) -> (sensitive value) (forces replacement)"},
	}, logr.logs)
}

func TestLogResourceReplacementsIncludesUnknownValues(t *testing.T) {
	t.Parallel()

	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "example_resource.unknown",
				Type:    "example_resource",
				Change: &tfjson.Change{
					Actions: tfjson.Actions{tfjson.ActionDelete, tfjson.ActionCreate},
					Before: map[string]any{
						"id": "old-id",
					},
					After: map[string]any{
						"id": nil,
					},
					AfterUnknown: map[string]any{
						"id": true,
					},
					ReplacePaths: []any{
						[]any{"id"},
					},
				},
			},
		},
	}

	logr := &mockLogger{}
	logResourceReplacements(findAllResourceReplacements(plan), logr)

	require.Equal(t, []*proto.Log{
		{Level: proto.LogLevel_WARN, Output: "Resource replacements:"},
		{Level: proto.LogLevel_WARN, Output: "  -/+ example_resource.unknown (replace)"},
		{Level: proto.LogLevel_WARN, Output: `      ~ id: "old-id" -> (known after apply) (forces replacement)`},
	}, logr.logs)
}

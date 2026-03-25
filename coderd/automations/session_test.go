package automations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/automations"
)

func TestResolveLabels(t *testing.T) {
	t.Parallel()

	payload := `{"repository":{"full_name":"coder/coder"},"action":"opened","number":42,"draft":false}`

	tests := []struct {
		name       string
		labelPaths map[string]string
		want       map[string]string
	}{
		{
			"nil label paths returns nil",
			nil,
			nil,
		},
		{
			"empty label paths returns nil",
			map[string]string{},
			nil,
		},
		{
			"simple path extraction",
			map[string]string{"repo": "repository.full_name"},
			map[string]string{"repo": "coder/coder"},
		},
		{
			"multiple paths",
			map[string]string{
				"repo":   "repository.full_name",
				"action": "action",
			},
			map[string]string{
				"repo":   "coder/coder",
				"action": "opened",
			},
		},
		{
			"missing path omitted",
			map[string]string{
				"repo":    "repository.full_name",
				"missing": "nonexistent.path",
			},
			map[string]string{"repo": "coder/coder"},
		},
		{
			"numeric value coerced to string",
			map[string]string{"num": "number"},
			map[string]string{"num": "42"},
		},
		{
			"boolean value coerced to string",
			map[string]string{"draft": "draft"},
			map[string]string{"draft": "false"},
		},
		{
			"all paths missing returns empty map",
			map[string]string{"a": "no.such.path", "b": "also.missing"},
			map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := automations.ResolveLabels(payload, tt.labelPaths)
			require.Equal(t, tt.want, got)
		})
	}
}

package provisionerdserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestScriptDependenciesJSON(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		got, err := scriptDependenciesJSON(nil)
		require.NoError(t, err)
		require.JSONEq(t, `[]`, got)
	})

	t.Run("Requirements", func(t *testing.T) {
		t.Parallel()

		got, err := scriptDependenciesJSON([]*proto.ScriptDependency{
			{
				PrerequisiteResourceAddress: "coder_script.clone",
				Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
			},
			{
				PrerequisiteResourceAddress: "coder_script.configure",
				Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION,
			},
		})
		require.NoError(t, err)
		require.JSONEq(t, `[
			{"prerequisite_resource_address":"coder_script.clone","requirement":"success"},
			{"prerequisite_resource_address":"coder_script.configure","requirement":"completion"}
		]`, got)
	})

	for _, tc := range []struct {
		name       string
		dependency *proto.ScriptDependency
		want       string
	}{
		{
			name: "UnspecifiedRequirement",
			dependency: &proto.ScriptDependency{
				PrerequisiteResourceAddress: "coder_script.clone",
			},
			want: "unsupported requirement",
		},
		{
			name: "EmptyAddress",
			dependency: &proto.ScriptDependency{
				Requirement: proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
			},
			want: "empty prerequisite resource address",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := scriptDependenciesJSON([]*proto.ScriptDependency{tc.dependency})
			require.ErrorContains(t, err, tc.want)
		})
	}
}

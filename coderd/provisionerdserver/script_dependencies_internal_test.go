package provisionerdserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestScriptDependenciesJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		dependencies []*sdkproto.ScriptDependency
		want         string
		wantErr      string
	}{
		{
			name: "NilIsEmptyArray",
			want: `[]`,
		},
		{
			name: "TwoEntriesKeepOrder",
			dependencies: []*sdkproto.ScriptDependency{
				{
					PrerequisiteResourceAddress: "coder_script.clone_repo",
					Requirement:                 sdkproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
				},
				{
					PrerequisiteResourceAddress: "coder_script.warm_cache",
					Requirement:                 sdkproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION,
				},
			},
			want: `[{"prerequisite_resource_address":"coder_script.clone_repo","requirement":"success"},` +
				`{"prerequisite_resource_address":"coder_script.warm_cache","requirement":"completion"}]`,
		},
		{
			name: "UnspecifiedRequirement",
			dependencies: []*sdkproto.ScriptDependency{{
				PrerequisiteResourceAddress: "coder_script.clone_repo",
				Requirement:                 sdkproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_UNSPECIFIED,
			}},
			wantErr: "dependency 0: unsupported requirement",
		},
		{
			name: "UnknownRequirement",
			dependencies: []*sdkproto.ScriptDependency{{
				PrerequisiteResourceAddress: "coder_script.clone_repo",
				Requirement:                 sdkproto.ScriptDependencyRequirement(99),
			}},
			wantErr: "dependency 0: unsupported requirement",
		},
		{
			name: "EmptyAddress",
			dependencies: []*sdkproto.ScriptDependency{
				{
					PrerequisiteResourceAddress: "coder_script.clone_repo",
					Requirement:                 sdkproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
				},
				{
					PrerequisiteResourceAddress: "",
					Requirement:                 sdkproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
				},
			},
			wantErr: "dependency 1 has an empty prerequisite resource address",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := scriptDependenciesJSON(tc.dependencies)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.JSONEq(t, tc.want, got)
		})
	}
}

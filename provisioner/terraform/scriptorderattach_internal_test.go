package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	success    = proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS
	completion = proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION
)

func scriptsByAddress(addresses ...string) map[string]*proto.Script {
	scripts := make(map[string]*proto.Script, len(addresses))
	for _, address := range addresses {
		scripts[address] = &proto.Script{ResourceAddress: address}
	}
	return scripts
}

func dependency(dependent, prerequisite string, requirement ScriptOrderRequirement) ScriptOrderDependency {
	return ScriptOrderDependency{
		DependentAddress:    dependent,
		PrerequisiteAddress: prerequisite,
		Requirement:         requirement,
	}
}

func startGraph(runtime string, dependencies ...ScriptOrderDependency) ScriptOrderGraph {
	return ScriptOrderGraph{RuntimeAddress: runtime, Phase: ScriptOrderPhaseStart, Dependencies: dependencies}
}

func prerequisites(script *proto.Script) []string {
	var got []string
	for _, dependency := range script.Dependencies {
		got = append(got, dependency.PrerequisiteResourceAddress)
	}
	return got
}

func TestAttachScriptOrderDependencies(t *testing.T) {
	t.Parallel()

	t.Run("OneEdge", func(t *testing.T) {
		t.Parallel()
		scripts := scriptsByAddress("coder_script.clone_repo", "coder_script.install_tools")
		order := ScriptOrder{Graphs: []ScriptOrderGraph{startGraph("coder_agent.main",
			dependency("coder_script.install_tools", "coder_script.clone_repo", ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []*proto.ScriptDependency{{
			PrerequisiteResourceAddress: "coder_script.clone_repo",
			Requirement:                 success,
		}}, scripts["coder_script.install_tools"].Dependencies)
		require.Empty(t, scripts["coder_script.clone_repo"].Dependencies)
	})

	t.Run("CompletionRequirement", func(t *testing.T) {
		t.Parallel()
		scripts := scriptsByAddress("coder_script.clone_repo", "coder_script.post_setup")
		order := ScriptOrder{Graphs: []ScriptOrderGraph{startGraph("coder_agent.main",
			dependency("coder_script.post_setup", "coder_script.clone_repo", ScriptOrderRequirementCompletion),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, completion, scripts["coder_script.post_setup"].Dependencies[0].Requirement)
	})

	t.Run("SortsByPrerequisiteAddress", func(t *testing.T) {
		t.Parallel()
		scripts := scriptsByAddress(
			"coder_script.build",
			"coder_script.z_clone",
			"coder_script.a_auth",
			"module.git_clone.coder_script.clone",
		)
		// Deliberately not in sorted order.
		order := ScriptOrder{Graphs: []ScriptOrderGraph{startGraph("coder_agent.main",
			dependency("coder_script.build", "coder_script.z_clone", ScriptOrderRequirementSuccess),
			dependency("coder_script.build", "module.git_clone.coder_script.clone", ScriptOrderRequirementCompletion),
			dependency("coder_script.build", "coder_script.a_auth", ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{
			"coder_script.a_auth",
			"coder_script.z_clone",
			"module.git_clone.coder_script.clone",
		}, prerequisites(scripts["coder_script.build"]))
	})

	t.Run("KeepsExplicitEdgeAlongsideImpliedPath", func(t *testing.T) {
		t.Parallel()
		// clone -> install -> build, plus an explicit clone -> build.
		scripts := scriptsByAddress("coder_script.clone_repo", "coder_script.install_tools", "coder_script.build")
		order := ScriptOrder{Graphs: []ScriptOrderGraph{startGraph("coder_agent.main",
			dependency("coder_script.install_tools", "coder_script.clone_repo", ScriptOrderRequirementSuccess),
			dependency("coder_script.build", "coder_script.install_tools", ScriptOrderRequirementSuccess),
			dependency("coder_script.build", "coder_script.clone_repo", ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{"coder_script.clone_repo"}, prerequisites(scripts["coder_script.install_tools"]))
		require.Equal(t, []string{"coder_script.clone_repo", "coder_script.install_tools"}, prerequisites(scripts["coder_script.build"]))
		require.Empty(t, scripts["coder_script.clone_repo"].Dependencies)
	})

	t.Run("SeveralGraphs", func(t *testing.T) {
		t.Parallel()
		scripts := scriptsByAddress(
			"coder_script.clone_repo", "coder_script.install_tools",
			"coder_script.db_init", "coder_script.db_seed",
			"coder_script.save_state", "coder_script.stop_db",
		)
		order := ScriptOrder{Graphs: []ScriptOrderGraph{
			startGraph("coder_agent.main",
				dependency("coder_script.install_tools", "coder_script.clone_repo", ScriptOrderRequirementSuccess),
			),
			startGraph("coder_agent.db",
				dependency("coder_script.db_seed", "coder_script.db_init", ScriptOrderRequirementSuccess),
			),
			{
				RuntimeAddress: "coder_agent.main",
				Phase:          ScriptOrderPhaseStop,
				Dependencies: []ScriptOrderDependency{
					dependency("coder_script.stop_db", "coder_script.save_state", ScriptOrderRequirementCompletion),
				},
			},
		}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{"coder_script.clone_repo"}, prerequisites(scripts["coder_script.install_tools"]))
		require.Equal(t, []string{"coder_script.db_init"}, prerequisites(scripts["coder_script.db_seed"]))
		require.Equal(t, []string{"coder_script.save_state"}, prerequisites(scripts["coder_script.stop_db"]))
		for _, address := range []string{"coder_script.clone_repo", "coder_script.db_init", "coder_script.save_state"} {
			require.Empty(t, scripts[address].Dependencies, address)
		}
	})

	t.Run("DevcontainerScript", func(t *testing.T) {
		t.Parallel()
		// Scripts under a devcontainer sit in the same map as agent scripts.
		scripts := scriptsByAddress("coder_script.a", "coder_script.b")
		order := ScriptOrder{Graphs: []ScriptOrderGraph{startGraph("coder_devcontainer.repo",
			dependency("coder_script.b", "coder_script.a", ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{"coder_script.a"}, prerequisites(scripts["coder_script.b"]))
	})

	t.Run("EmptyOrder", func(t *testing.T) {
		t.Parallel()
		scripts := scriptsByAddress("coder_script.clone_repo")

		require.NoError(t, attachScriptOrderDependencies(ScriptOrder{}, scripts))

		require.Empty(t, scripts["coder_script.clone_repo"].Dependencies)
	})

	t.Run("Errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			edge        ScriptOrderDependency
			errContains string
		}{
			{
				name:        "UnknownDependent",
				edge:        dependency("coder_script.ghost", "coder_script.clone_repo", ScriptOrderRequirementSuccess),
				errContains: `dependent script "coder_script.ghost" not found`,
			},
			{
				name:        "UnknownPrerequisite",
				edge:        dependency("coder_script.install_tools", "coder_script.ghost", ScriptOrderRequirementSuccess),
				errContains: `prerequisite script "coder_script.ghost" not found`,
			},
			{
				name:        "UnknownRequirement",
				edge:        dependency("coder_script.install_tools", "coder_script.clone_repo", ScriptOrderRequirement("always")),
				errContains: `unknown script dependency requirement "always"`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				scripts := scriptsByAddress("coder_script.clone_repo", "coder_script.install_tools")
				// The bad edge comes first so a later good edge proves
				// nothing is attached once an error is found.
				order := ScriptOrder{Graphs: []ScriptOrderGraph{startGraph("coder_agent.main",
					tt.edge,
					dependency("coder_script.install_tools", "coder_script.clone_repo", ScriptOrderRequirementSuccess),
				)}}

				err := attachScriptOrderDependencies(order, scripts)

				require.ErrorContains(t, err, tt.errContains)
				for address, script := range scripts {
					require.Empty(t, script.Dependencies, address)
				}
			})
		}
	})
}

func TestScriptDependencyRequirementProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		requirement ScriptOrderRequirement
		want        proto.ScriptDependencyRequirement
		wantErr     bool
	}{
		{requirement: ScriptOrderRequirementSuccess, want: success},
		{requirement: ScriptOrderRequirementCompletion, want: completion},
		{requirement: "", wantErr: true},
		{requirement: "always", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(string(tt.requirement), func(t *testing.T) {
			t.Parallel()
			got, err := scriptDependencyRequirementProto(tt.requirement)
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_UNSPECIFIED, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

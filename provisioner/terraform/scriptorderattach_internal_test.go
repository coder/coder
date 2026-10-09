package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	scriptOrderAttachTestSuccess    = proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS
	scriptOrderAttachTestCompletion = proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION
)

func scriptOrderAttachTestScripts(addresses ...string) map[string]*proto.Script {
	scripts := make(map[string]*proto.Script, len(addresses))
	for _, address := range addresses {
		scripts[address] = &proto.Script{ResourceAddress: address}
	}
	return scripts
}

func scriptOrderAttachTestDependency(
	dependentAddress string,
	prerequisiteAddress string,
	requirement scriptorder.Requirement,
) scriptorder.Dependency {
	return scriptorder.Dependency{
		DependentAddress:    dependentAddress,
		PrerequisiteAddress: prerequisiteAddress,
		Requirement:         requirement,
	}
}

func scriptOrderAttachTestStartGraph(runtime string, dependencies ...scriptorder.Dependency) scriptorder.Graph {
	return scriptorder.Graph{RuntimeAddress: runtime, Phase: scriptorder.ScriptOrderPhaseStart, Dependencies: dependencies}
}

func scriptOrderAttachTestPrerequisites(script *proto.Script) []string {
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
		scripts := scriptOrderAttachTestScripts("coder_script.clone_repo", "coder_script.install_tools")
		order := scriptorder.ScriptOrder{Graphs: []scriptorder.Graph{scriptOrderAttachTestStartGraph("coder_agent.main",
			scriptOrderAttachTestDependency("coder_script.install_tools", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []*proto.ScriptDependency{{
			PrerequisiteResourceAddress: "coder_script.clone_repo",
			Requirement:                 scriptOrderAttachTestSuccess,
		}}, scripts["coder_script.install_tools"].Dependencies)
		require.Empty(t, scripts["coder_script.clone_repo"].Dependencies)
	})

	t.Run("CompletionRequirement", func(t *testing.T) {
		t.Parallel()
		scripts := scriptOrderAttachTestScripts("coder_script.clone_repo", "coder_script.post_setup")
		order := scriptorder.ScriptOrder{Graphs: []scriptorder.Graph{scriptOrderAttachTestStartGraph("coder_agent.main",
			scriptOrderAttachTestDependency("coder_script.post_setup", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementCompletion),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, scriptOrderAttachTestCompletion, scripts["coder_script.post_setup"].Dependencies[0].Requirement)
	})

	t.Run("SortsByPrerequisiteAddress", func(t *testing.T) {
		t.Parallel()
		scripts := scriptOrderAttachTestScripts(
			"coder_script.build",
			"coder_script.z_clone",
			"coder_script.a_auth",
			"module.git_clone.coder_script.clone",
		)
		// Deliberately not in sorted order.
		order := scriptorder.ScriptOrder{Graphs: []scriptorder.Graph{scriptOrderAttachTestStartGraph("coder_agent.main",
			scriptOrderAttachTestDependency("coder_script.build", "coder_script.z_clone", scriptorder.ScriptOrderRequirementSuccess),
			scriptOrderAttachTestDependency("coder_script.build", "module.git_clone.coder_script.clone", scriptorder.ScriptOrderRequirementCompletion),
			scriptOrderAttachTestDependency("coder_script.build", "coder_script.a_auth", scriptorder.ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{
			"coder_script.a_auth",
			"coder_script.z_clone",
			"module.git_clone.coder_script.clone",
		}, scriptOrderAttachTestPrerequisites(scripts["coder_script.build"]))
	})

	t.Run("KeepsExplicitEdgeAlongsideImpliedPath", func(t *testing.T) {
		t.Parallel()
		// clone -> install -> build, plus an explicit clone -> build.
		scripts := scriptOrderAttachTestScripts("coder_script.clone_repo", "coder_script.install_tools", "coder_script.build")
		order := scriptorder.ScriptOrder{Graphs: []scriptorder.Graph{scriptOrderAttachTestStartGraph("coder_agent.main",
			scriptOrderAttachTestDependency("coder_script.install_tools", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementSuccess),
			scriptOrderAttachTestDependency("coder_script.build", "coder_script.install_tools", scriptorder.ScriptOrderRequirementSuccess),
			scriptOrderAttachTestDependency("coder_script.build", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementSuccess),
		)}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{"coder_script.clone_repo"}, scriptOrderAttachTestPrerequisites(scripts["coder_script.install_tools"]))
		require.Equal(t, []string{"coder_script.clone_repo", "coder_script.install_tools"}, scriptOrderAttachTestPrerequisites(scripts["coder_script.build"]))
		require.Empty(t, scripts["coder_script.clone_repo"].Dependencies)
	})

	t.Run("SeveralGraphs", func(t *testing.T) {
		t.Parallel()
		scripts := scriptOrderAttachTestScripts(
			"coder_script.clone_repo", "coder_script.install_tools",
			"coder_script.db_init", "coder_script.db_seed",
			"coder_script.save_state", "coder_script.stop_db",
		)
		order := scriptorder.ScriptOrder{Graphs: []scriptorder.Graph{
			scriptOrderAttachTestStartGraph("coder_agent.main",
				scriptOrderAttachTestDependency("coder_script.install_tools", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementSuccess),
			),
			scriptOrderAttachTestStartGraph("coder_agent.db",
				scriptOrderAttachTestDependency("coder_script.db_seed", "coder_script.db_init", scriptorder.ScriptOrderRequirementSuccess),
			),
			{
				RuntimeAddress: "coder_agent.main",
				Phase:          scriptorder.ScriptOrderPhaseStop,
				Dependencies: []scriptorder.Dependency{
					scriptOrderAttachTestDependency("coder_script.stop_db", "coder_script.save_state", scriptorder.ScriptOrderRequirementCompletion),
				},
			},
		}}

		require.NoError(t, attachScriptOrderDependencies(order, scripts))

		require.Equal(t, []string{"coder_script.clone_repo"}, scriptOrderAttachTestPrerequisites(scripts["coder_script.install_tools"]))
		require.Equal(t, []string{"coder_script.db_init"}, scriptOrderAttachTestPrerequisites(scripts["coder_script.db_seed"]))
		require.Equal(t, []string{"coder_script.save_state"}, scriptOrderAttachTestPrerequisites(scripts["coder_script.stop_db"]))
		for _, address := range []string{"coder_script.clone_repo", "coder_script.db_init", "coder_script.save_state"} {
			require.Empty(t, scripts[address].Dependencies, address)
		}
	})

	t.Run("EmptyOrder", func(t *testing.T) {
		t.Parallel()
		scripts := scriptOrderAttachTestScripts("coder_script.clone_repo")

		require.NoError(t, attachScriptOrderDependencies(scriptorder.ScriptOrder{}, scripts))

		require.Empty(t, scripts["coder_script.clone_repo"].Dependencies)
	})

	t.Run("Errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			edge        scriptorder.Dependency
			errContains string
		}{
			{
				name:        "UnknownDependent",
				edge:        scriptOrderAttachTestDependency("coder_script.ghost", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementSuccess),
				errContains: `dependent script "coder_script.ghost" not found`,
			},
			{
				name:        "UnknownPrerequisite",
				edge:        scriptOrderAttachTestDependency("coder_script.install_tools", "coder_script.ghost", scriptorder.ScriptOrderRequirementSuccess),
				errContains: `prerequisite script "coder_script.ghost" not found`,
			},
			{
				name:        "UnknownRequirement",
				edge:        scriptOrderAttachTestDependency("coder_script.install_tools", "coder_script.clone_repo", scriptorder.Requirement("always")),
				errContains: `unknown script dependency requirement "always"`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				good := scriptOrderAttachTestDependency("coder_script.install_tools", "coder_script.clone_repo", scriptorder.ScriptOrderRequirementSuccess)
				// A good edge on either side of the bad one must not be
				// attached once an error is found.
				for _, edges := range [][]scriptorder.Dependency{{tt.edge, good}, {good, tt.edge}} {
					scripts := scriptOrderAttachTestScripts("coder_script.clone_repo", "coder_script.install_tools")
					order := scriptorder.ScriptOrder{Graphs: []scriptorder.Graph{scriptOrderAttachTestStartGraph("coder_agent.main", edges...)}}

					err := attachScriptOrderDependencies(order, scripts)

					require.ErrorContains(t, err, tt.errContains)
					for address, script := range scripts {
						require.Empty(t, script.Dependencies, address)
					}
				}
			})
		}
	})
}

func TestScriptDependencyRequirementProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		requirement scriptorder.Requirement
		want        proto.ScriptDependencyRequirement
		wantErr     bool
	}{
		{requirement: scriptorder.ScriptOrderRequirementSuccess, want: scriptOrderAttachTestSuccess},
		{requirement: scriptorder.ScriptOrderRequirementCompletion, want: scriptOrderAttachTestCompletion},
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

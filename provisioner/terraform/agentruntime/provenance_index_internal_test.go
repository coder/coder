package agentruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestRuntimeProvenanceIndexesSources(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workdir, "feature"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, "main.tf"),
		[]byte(`locals {
  runtime = coder_agent.original.id
}

resource "coder_script" "work" {
  agent_id     = local.runtime
  for_each     = coder_agent.service
  display_name = coder_agent.ignored.name
}

module "feature" {
  source   = "./feature"
  agent_id = local.runtime
}

output "agent_id" {
  value = local.runtime
}
`),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, "values.tf.json"),
		[]byte(`{"locals":{"json_runtime":"${coder_agent.json.id}"}}`),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, "override.tf"),
		[]byte(`locals {
  runtime = coder_agent.override.id
}
`),
		0o600,
	))

	config := runtimeResolverTestRootConfig(
		&tfjson.ConfigResource{
			Address: "coder_script.work",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "coder_script",
			Name:    "work",
		},
	)
	config.RootModule.ModuleCalls = map[string]*tfjson.ModuleCall{
		"feature": {
			Source: "./feature",
			Module: &tfjson.ConfigModule{},
		},
	}
	program, err := NewProgram(t.Context(), config)
	require.NoError(t, err)
	require.NoError(t, program.LoadProvenance(t.Context(), workdir))

	index := program.configIndex
	require.Equal(
		t,
		[]string{"coder_agent.override.id"},
		runtimeProvenanceExpressionReferences(
			index.runtimeSourceExpressions[runtimeExpressionKey{
				kind: runtimeExpressionLocal, attribute: "runtime",
			}],
		),
	)
	require.Equal(
		t,
		[]string{"coder_agent.json.id"},
		runtimeProvenanceExpressionReferences(
			index.runtimeSourceExpressions[runtimeExpressionKey{
				kind: runtimeExpressionLocal, attribute: "json_runtime",
			}],
		),
	)
	require.Equal(
		t,
		[]string{"local.runtime"},
		runtimeProvenanceExpressionReferences(
			index.runtimeSourceExpressions[runtimeExpressionKey{
				kind:         runtimeExpressionResource,
				resourceType: "coder_script",
				name:         "work",
				attribute:    "agent_id",
			}],
		),
	)
	require.Equal(
		t,
		[]string{"coder_agent.service"},
		runtimeProvenanceExpressionReferences(
			index.runtimeSourceExpressions[runtimeExpressionKey{
				kind:         runtimeExpressionResource,
				resourceType: "coder_script",
				name:         "work",
				attribute:    "for_each",
			}],
		),
	)
	require.NotContains(
		t,
		index.runtimeSourceExpressions,
		runtimeExpressionKey{
			kind:         runtimeExpressionResource,
			resourceType: "coder_script",
			name:         "work",
			attribute:    "display_name",
		},
	)
	require.Equal(
		t,
		[]string{"local.runtime"},
		runtimeProvenanceExpressionReferences(
			index.runtimeSourceExpressions[runtimeExpressionKey{
				kind:      runtimeExpressionModuleCall,
				name:      "feature",
				attribute: "agent_id",
			}],
		),
	)
	require.Equal(
		t,
		[]string{"local.runtime"},
		runtimeProvenanceExpressionReferences(
			index.runtimeSourceExpressions[runtimeExpressionKey{
				kind:      runtimeExpressionOutput,
				name:      "agent_id",
				attribute: "value",
			}],
		),
	)
}

func TestRuntimeProvenanceModuleDirectories(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(workdir, ".terraform", "modules"), 0o700,
	))
	manifest, err := json.Marshal(runtimeModulesManifest{
		Modules: []*runtimeModuleManifestEntry{
			{Key: "", Dir: "."},
			{Key: "registry", Dir: ".terraform/modules/registry"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, ".terraform", "modules", "modules.json"),
		manifest,
		0o600,
	))

	config := runtimeResolverTestRootConfig()
	config.RootModule.ModuleCalls = map[string]*tfjson.ModuleCall{
		"registry": {
			Source: "registry.example/runtime/module",
			Module: &tfjson.ConfigModule{},
		},
		"local": {
			Source: "./modules/local",
			Module: &tfjson.ConfigModule{},
		},
	}
	program, err := NewProgram(t.Context(), config)
	require.NoError(t, err)
	actual, err := program.configIndex.runtimeModuleDirectories(
		t.Context(), workdir,
	)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"":                workdir,
		"module.registry": filepath.Join(workdir, ".terraform/modules/registry"),
		"module.local":    filepath.Join(workdir, "modules/local"),
	}, actual)
}

func TestRuntimeProvenanceReusesSharedModuleSources(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	moduleDir := filepath.Join(workdir, "shared")
	require.NoError(t, os.MkdirAll(
		filepath.Join(workdir, ".terraform", "modules"), 0o700,
	))
	require.NoError(t, os.Mkdir(moduleDir, 0o700))
	source := []byte(`locals {
  runtime = coder_agent.main.id
}
`)
	require.NoError(t, os.WriteFile(
		filepath.Join(moduleDir, "locals.tf"), source, 0o600,
	))
	manifest, err := json.Marshal(runtimeModulesManifest{
		Modules: []*runtimeModuleManifestEntry{
			{Key: "first", Dir: "shared"},
			{Key: "second", Dir: "shared"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, ".terraform", "modules", "modules.json"),
		manifest,
		0o600,
	))

	config := runtimeResolverTestRootConfig()
	config.RootModule.ModuleCalls = map[string]*tfjson.ModuleCall{
		"first":  {Module: &tfjson.ConfigModule{}},
		"second": {Module: &tfjson.ConfigModule{}},
	}
	program, err := NewProgram(t.Context(), config)
	require.NoError(t, err)
	limits := defaultProvenanceLimits()
	limits.sourceBytes = len(source)
	require.NoError(t, program.configIndex.indexRuntimeSourceExpressionsWithLimits(
		t.Context(), workdir, limits,
	))

	for _, moduleAddress := range []string{"module.first", "module.second"} {
		require.Contains(
			t,
			program.configIndex.runtimeSourceExpressions,
			runtimeExpressionKey{
				moduleAddress: moduleAddress,
				kind:          runtimeExpressionLocal,
				attribute:     "runtime",
			},
		)
	}
}

func TestRuntimeProvenanceSourceBounds(t *testing.T) {
	t.Parallel()

	t.Run("DirectoryEntries", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		for _, name := range []string{"first.txt", "second.txt", "source.tf"} {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, name), []byte("ignored"), 0o600,
			))
		}
		_, _, err := readRuntimeSourceFiles(t.Context(), dir, 10, 2)
		require.ErrorIs(t, err, errRuntimeDirectoryEntryLimit)
	})

	t.Run("SourceFiles", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		for _, name := range []string{"first.tf", "second.tf"} {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, name), nil, 0o600,
			))
		}
		_, _, err := readRuntimeSourceFiles(t.Context(), dir, 1, 10)
		require.ErrorIs(t, err, errRuntimeSourceFileLimit)
	})

	t.Run("SourceBytesThroughSymlink", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "oversized"), nil, 0o600,
		))
		require.NoError(t, os.Truncate(
			filepath.Join(dir, "oversized"), 9,
		))
		require.NoError(t, os.Symlink(
			"oversized", filepath.Join(dir, "main.tf"),
		))
		program, err := NewProgram(
			t.Context(), runtimeResolverTestRootConfig(),
		)
		require.NoError(t, err)
		limits := defaultProvenanceLimits()
		limits.sourceBytes = 8
		err = program.configIndex.indexRuntimeSourceExpressionsWithLimits(
			t.Context(), dir, limits,
		)
		require.ErrorContains(t, err, "limit of 8 Terraform source bytes")
	})

	t.Run("ExpandedExpressions", func(t *testing.T) {
		t.Parallel()

		workdir := t.TempDir()
		moduleDir := filepath.Join(workdir, "shared")
		require.NoError(t, os.MkdirAll(
			filepath.Join(workdir, ".terraform", "modules"), 0o700,
		))
		require.NoError(t, os.Mkdir(moduleDir, 0o700))
		require.NoError(t, os.WriteFile(
			filepath.Join(moduleDir, "locals.tf"),
			[]byte("locals { runtime = coder_agent.main.id }\n"),
			0o600,
		))
		manifest, err := json.Marshal(runtimeModulesManifest{
			Modules: []*runtimeModuleManifestEntry{
				{Key: "first", Dir: "shared"},
				{Key: "second", Dir: "shared"},
			},
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			filepath.Join(workdir, ".terraform", "modules", "modules.json"),
			manifest,
			0o600,
		))
		config := runtimeResolverTestRootConfig()
		config.RootModule.ModuleCalls = map[string]*tfjson.ModuleCall{
			"first":  {Module: &tfjson.ConfigModule{}},
			"second": {Module: &tfjson.ConfigModule{}},
		}
		program, err := NewProgram(t.Context(), config)
		require.NoError(t, err)
		limits := defaultProvenanceLimits()
		limits.sourceExpressions = 1
		err = program.configIndex.indexRuntimeSourceExpressionsWithLimits(
			t.Context(), workdir, limits,
		)
		require.ErrorContains(
			t, err, "limit of 1 expanded Terraform source expressions",
		)
	})

	t.Run("Cancellation", func(t *testing.T) {
		t.Parallel()

		program, err := NewProgram(
			t.Context(), runtimeResolverTestRootConfig(),
		)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(
			t,
			program.LoadProvenance(ctx, t.TempDir()),
			context.Canceled,
		)
	})
}

func TestRuntimeProvenanceIgnoresHiddenSourceFiles(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, ".invalid.tf"), []byte("invalid"), 0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workdir, "locals.tf"),
		[]byte("locals { runtime = coder_agent.main.id }\n"),
		0o600,
	))
	program, err := NewProgram(
		t.Context(), runtimeResolverTestRootConfig(),
	)
	require.NoError(t, err)
	require.NoError(t, program.LoadProvenance(t.Context(), workdir))
	require.Contains(
		t,
		program.configIndex.runtimeSourceExpressions,
		runtimeExpressionKey{
			kind: runtimeExpressionLocal, attribute: "runtime",
		},
	)
}

func runtimeProvenanceExpressionReferences(expression hcl.Expression) []string {
	if expression == nil {
		return nil
	}
	references := make([]string, 0, len(expression.Variables()))
	for _, traversal := range expression.Variables() {
		references = append(
			references,
			strings.TrimSpace(string(hclwrite.TokensForTraversal(traversal).Bytes())),
		)
	}
	slices.Sort(references)
	return references
}

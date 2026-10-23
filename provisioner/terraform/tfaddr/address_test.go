package tfaddr_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

func TestParseManagedResourceAddress(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                 string
		raw                  string
		moduleAddress        string
		moduleConfiguration  string
		moduleSteps          []moduleStep
		resourceType         string
		resourceName         string
		instanceKey          cty.Value
		configurationAddress string
	}{
		{
			name:                 "RootUnicodeResource",
			raw:                  "coder_script.π",
			instanceKey:          cty.NilVal,
			resourceType:         "coder_script",
			resourceName:         "π",
			configurationAddress: "coder_script.π",
		},
		{
			name:                "NestedModulesAndInstances",
			raw:                 `module.开发["环境"].module.inner[2].docker_container.工作区["api"]`,
			moduleAddress:       `module.开发["环境"].module.inner[2]`,
			moduleConfiguration: "module.开发.module.inner",
			moduleSteps: []moduleStep{
				{name: "开发", instanceKey: cty.StringVal("环境")},
				{name: "inner", instanceKey: cty.NumberIntVal(2)},
			},
			resourceType:         "docker_container",
			resourceName:         "工作区",
			instanceKey:          cty.StringVal("api"),
			configurationAddress: "module.开发.module.inner.docker_container.工作区",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			address, err := tfaddr.ParseManagedResourceAddress(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.moduleAddress, address.ModulePath().String())
			require.Equal(t, test.moduleConfiguration, address.ModulePath().ConfigurationAddress())
			requireModuleSteps(t, test.moduleSteps, address.ModulePath().Steps())
			require.Equal(t, test.resourceType, address.ResourceType())
			require.Equal(t, test.resourceName, address.ResourceName())
			require.True(t, tfaddr.InstanceKeysEqual(test.instanceKey, address.InstanceKey()))
			require.Equal(t, test.configurationAddress, address.ConfigurationAddress())
		})
	}
}

func TestParseManagedResourceAddressNormalizesCountKey(t *testing.T) {
	t.Parallel()

	address, err := tfaddr.ParseManagedResourceAddress("coder_script.setup[2.0]")
	require.NoError(t, err)
	require.True(t, address.InstanceKey().RawEquals(cty.NumberIntVal(2)))
}

func TestParseManagedResourceAddressRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"",
		"coder_script",
		"coder_script.setup.extra",
		"coder_script.setup[true]",
		"coder_script.setup[-1]",
		"coder_script.setup[0][1]",
		"module.bootstrap",
		"module.bootstrap[0]",
		`module.bootstrap["api"]`,
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			_, err := tfaddr.ParseManagedResourceAddress(address)
			require.Error(t, err)
		})
	}
}

func TestParseModulePath(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name                 string
		raw                  string
		configurationAddress string
		steps                []moduleStep
	}{
		{
			name: "Root",
		},
		{
			name:                 "NestedUnicodeModules",
			raw:                  `module.开发["环境"].module.inner[2]`,
			configurationAddress: "module.开发.module.inner",
			steps: []moduleStep{
				{name: "开发", instanceKey: cty.StringVal("环境")},
				{name: "inner", instanceKey: cty.NumberIntVal(2)},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path, err := tfaddr.ParseModulePath(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.raw, path.String())
			require.Equal(t, test.configurationAddress, path.ConfigurationAddress())
			requireModuleSteps(t, test.steps, path.Steps())
		})
	}
}

func TestModulePathStepsReturnsCopy(t *testing.T) {
	t.Parallel()

	path, err := tfaddr.ParseModulePath(`module.outer[0].module.inner["api"]`)
	require.NoError(t, err)

	steps := path.Steps()
	require.Len(t, steps, 2)
	steps[0] = steps[1]

	require.Equal(t, "outer", path.Steps()[0].Name())
}

func TestParseModulePathRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"module",
		"module.bootstrap.coder_script.setup",
		"module.bootstrap[true]",
		"coder_script.setup",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			_, err := tfaddr.ParseModulePath(address)
			require.Error(t, err)
		})
	}
}

func TestConfigurationReferenceAddresses(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		modulePrefix string
		reference    string
		expected     []string
	}{
		{
			name:         "ResourceAttribute",
			modulePrefix: "module.开发",
			reference:    `coder_devcontainer.repo["api"].subagent_id`,
			expected: []string{
				"module.开发.coder_devcontainer.repo.subagent_id",
				"module.开发.coder_devcontainer.repo",
			},
		},
		{
			name:         "Variable",
			modulePrefix: "module.开发",
			reference:    "var.agent_id",
			expected:     []string{"module.开发.var.agent_id"},
		},
		{
			name:         "ModuleOutput",
			modulePrefix: "module.开发",
			reference:    `module.runtime["primary"].agent.id`,
			expected: []string{
				"module.开发.module.runtime.output.agent",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reference, err := tfaddr.ParseConfigurationReference(test.reference)
			require.NoError(t, err)
			require.Equal(t, test.expected, slices.Collect(
				reference.ConfigurationAddresses(test.modulePrefix),
			))
		})
	}
}

func TestConfigurationReferenceConfigurationAddress(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		reference string
		expected  string
	}{
		{
			name:      "ManagedResourceInstance",
			reference: "coder_agent.main[0]",
			expected:  "coder_agent.main",
		},
		{
			name:      "NestedModuleAndResourceInstances",
			reference: `module.runtime["api"].module.child[1].coder_agent.main["blue"]`,
			expected:  "module.runtime.module.child.coder_agent.main",
		},
		{
			name:      "DataResourceInstances",
			reference: `module.runtime["api"].data.coder_script_order.order[0]`,
			expected:  "module.runtime.data.coder_script_order.order",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reference, err := tfaddr.ParseConfigurationReference(test.reference)
			require.NoError(t, err)
			require.Equal(t, test.expected, reference.ConfigurationAddress())
		})
	}
}

func TestConfigurationReferenceModuleOutputCallAddress(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		reference string
		expected  string
		ok        bool
	}{
		{
			name:      "ModuleOutput",
			reference: "module.runtime.agent_id",
			expected:  "module.runtime",
			ok:        true,
		},
		{
			name:      "KeyedModuleOutput",
			reference: `module.runtime["primary"].agent_id`,
			expected:  `module.runtime["primary"]`,
			ok:        true,
		},
		{
			name:      "NestedOutputTraversal",
			reference: "module.runtime.agent_id.value",
		},
		{
			name:      "IndexedOutputValue",
			reference: "module.runtime.agent_id[0]",
		},
		{
			name:      "WholeModule",
			reference: "module.runtime",
		},
		{
			name:      "Resource",
			reference: "coder_agent.main.id",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reference, err := tfaddr.ParseConfigurationReference(test.reference)
			require.NoError(t, err)
			address, ok := reference.ModuleOutputCallAddress()
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.expected, address)
		})
	}
}

func TestConfigurationReferenceAddressPartLimit(t *testing.T) {
	t.Parallel()

	maxDepth := "local" + strings.Repeat(".child", 255)
	reference, err := tfaddr.ParseConfigurationReference(maxDepth)
	require.NoError(t, err)
	require.Len(t, slices.Collect(reference.ConfigurationAddresses("")), 255)

	tooDeep := "local" + strings.Repeat(".child", 256)
	_, err = tfaddr.ParseConfigurationReference(tooDeep)
	require.ErrorContains(t, err, "reference contains more than 256 address parts")
}

type moduleStep struct {
	name        string
	instanceKey cty.Value
}

func requireModuleSteps(t *testing.T, expected []moduleStep, actual []tfaddr.ModuleStep) {
	t.Helper()
	require.Len(t, actual, len(expected))
	for index, expectedStep := range expected {
		require.Equal(t, expectedStep.name, actual[index].Name())
		require.True(t, tfaddr.InstanceKeysEqual(
			expectedStep.instanceKey,
			actual[index].InstanceKey(),
		))
	}
}

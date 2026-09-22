package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestParseTerraformManagedResourceAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected terraformManagedResourceAddress
	}{
		{
			name: "UnicodeResource",
			raw:  "coder_script.π",
			expected: terraformManagedResourceAddress{
				modulePath:   terraformModulePath{},
				resourceType: "coder_script",
				resourceName: "π",
				instanceKey:  cty.NilVal,
			},
		},
		{
			name: "NestedModulesAndInstances",
			raw:  `module.开发["环境"].module.inner[2].docker_container.工作区["api"]`,
			expected: terraformManagedResourceAddress{
				modulePath: terraformModulePath{
					raw: `module.开发["环境"].module.inner[2]`,
					steps: []terraformModulePathStep{
						{name: "开发", instanceKey: cty.StringVal("环境")},
						{name: "inner", instanceKey: cty.NumberIntVal(2)},
					},
				},
				resourceType: "docker_container",
				resourceName: "工作区",
				instanceKey:  cty.StringVal("api"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual, err := parseTerraformManagedResourceAddress(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestParseTerraformManagedResourceAddressRejectsInvalidInput(t *testing.T) {
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
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			_, err := parseTerraformManagedResourceAddress(address)
			require.Error(t, err)
		})
	}
}

func TestParseTerraformModulePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected terraformModulePath
	}{
		{
			name:     "Root",
			raw:      "",
			expected: terraformModulePath{},
		},
		{
			name: "NestedUnicodeModules",
			raw:  `module.开发["环境"].module.inner[2]`,
			expected: terraformModulePath{
				raw: `module.开发["环境"].module.inner[2]`,
				steps: []terraformModulePathStep{
					{name: "开发", instanceKey: cty.StringVal("环境")},
					{name: "inner", instanceKey: cty.NumberIntVal(2)},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual, err := parseTerraformModulePath(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestParseTerraformModulePathRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"module",
		"module.bootstrap.coder_script.setup",
		"module.bootstrap[true]",
		"coder_script.setup",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			_, err := parseTerraformModulePath(address)
			require.Error(t, err)
		})
	}
}

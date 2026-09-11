package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestParseScriptOrderSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected scriptOrderSelector
	}{
		{
			name: "AllScriptInstances",
			raw:  "coder_script.setup",
			expected: scriptOrderSelector{
				kind: scriptOrderSelectorScript,
				name: "setup",
			},
		},
		{
			name: "CountInstance",
			raw:  "coder_script.setup[2]",
			expected: scriptOrderSelector{
				kind:  scriptOrderSelectorScript,
				name:  "setup",
				index: "2",
			},
		},
		{
			name: "ForEachInstance",
			raw:  `coder_script.setup["api"]`,
			expected: scriptOrderSelector{
				kind:  scriptOrderSelectorScript,
				name:  "setup",
				index: `"api"`,
			},
		},
		{
			name: "Module",
			raw:  "module.bootstrap",
			expected: scriptOrderSelector{
				kind: scriptOrderSelectorModule,
				name: "bootstrap",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selector, err := parseScriptOrderSelector(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.expected, selector)
		})
	}
}

func TestParseScriptOrderSelectorRejectsUnsupportedSyntax(t *testing.T) {
	t.Parallel()

	for _, selector := range []string{
		"",
		"coder_script",
		"coder_script.setup[",
		"coder_script.setup[api]",
		"coder_script.setup[true]",
		"coder_agent.main",
		"data.coder_script.setup",
		"module.bootstrap[0]",
		"module.bootstrap.coder_script.setup",
	} {
		t.Run(selector, func(t *testing.T) {
			t.Parallel()

			_, err := parseScriptOrderSelector(selector)
			require.Error(t, err)
		})
	}
}

func TestResolveScriptOrderSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		modules       []*tfjson.StateModule
		moduleAddress string
		selector      string
		expected      []string
	}{
		{
			name: "AllCountInstances",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedScript("coder_script.setup[1]", "setup"),
					managedScript("coder_script.other", "other"),
					managedScript("coder_script.setup[0]", "setup"),
					dataModeScript("data.coder_script.setup", "setup"),
				},
			}},
			selector: "coder_script.setup",
			expected: []string{
				"coder_script.setup[0]",
				"coder_script.setup[1]",
			},
		},
		{
			name: "OneCountInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedScript("coder_script.setup[0]", "setup"),
					managedScript("coder_script.setup[1]", "setup"),
				},
			}},
			selector: "coder_script.setup[1]",
			expected: []string{"coder_script.setup[1]"},
		},
		{
			name: "OneForEachInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedScript(`coder_script.setup["worker"]`, "setup"),
					managedScript(`coder_script.setup["api"]`, "setup"),
				},
			}},
			selector: `coder_script.setup["api"]`,
			expected: []string{`coder_script.setup["api"]`},
		},
		{
			name: "ScriptRelativeToRepeatedModuleInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedScript("coder_script.setup", "setup"),
				},
				ChildModules: []*tfjson.StateModule{
					{
						Address: `module.development["primary"]`,
						Resources: []*tfjson.StateResource{
							managedScript(`module.development["primary"].coder_script.setup`, "setup"),
						},
					},
					{
						Address: `module.development["secondary"]`,
						Resources: []*tfjson.StateResource{
							managedScript(`module.development["secondary"].coder_script.setup`, "setup"),
						},
					},
				},
			}},
			moduleAddress: `module.development["primary"]`,
			selector:      "coder_script.setup",
			expected:      []string{`module.development["primary"].coder_script.setup`},
		},
		{
			name: "RepeatedModulesAndDescendants",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{
					{
						Address: "module.bootstrap[1]",
						Resources: []*tfjson.StateResource{
							managedScript("module.bootstrap[1].coder_script.install", "install"),
						},
						ChildModules: []*tfjson.StateModule{{
							Address: "module.bootstrap[1].module.nested",
							Resources: []*tfjson.StateResource{
								managedScript("module.bootstrap[1].module.nested.coder_script.configure", "configure"),
							},
						}},
					},
					{
						Address: "module.bootstrap[0]",
						Resources: []*tfjson.StateResource{
							managedScript("module.bootstrap[0].coder_script.install", "install"),
							dataModeScript("module.bootstrap[0].data.coder_script.ignored", "ignored"),
							{
								Address: "module.bootstrap[0].null_resource.ignored",
								Mode:    tfjson.ManagedResourceMode,
								Type:    "null_resource",
								Name:    "ignored",
							},
						},
					},
					{
						Address: "module.unrelated",
						Resources: []*tfjson.StateResource{
							managedScript("module.unrelated.coder_script.ignored", "ignored"),
						},
					},
				},
			}},
			selector: "module.bootstrap",
			expected: []string{
				"module.bootstrap[0].coder_script.install",
				"module.bootstrap[1].coder_script.install",
				"module.bootstrap[1].module.nested.coder_script.configure",
			},
		},
		{
			name: "ForEachModuleInstances",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{
					{
						Address: `module.bootstrap["secondary"]`,
						Resources: []*tfjson.StateResource{
							managedScript(`module.bootstrap["secondary"].coder_script.setup`, "setup"),
						},
					},
					{
						Address: `module.bootstrap["primary"]`,
						Resources: []*tfjson.StateResource{
							managedScript(`module.bootstrap["primary"].coder_script.setup`, "setup"),
						},
					},
				},
			}},
			selector: "module.bootstrap",
			expected: []string{
				`module.bootstrap["primary"].coder_script.setup`,
				`module.bootstrap["secondary"].coder_script.setup`,
			},
		},
		{
			name: "ModuleRelativeToDeclaringModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.development",
					ChildModules: []*tfjson.StateModule{{
						Address: "module.development.module.bootstrap",
						Resources: []*tfjson.StateResource{
							managedScript("module.development.module.bootstrap.coder_script.setup", "setup"),
						},
					}},
				}},
			}},
			moduleAddress: "module.development",
			selector:      "module.bootstrap",
			expected: []string{
				"module.development.module.bootstrap.coder_script.setup",
			},
		},
		{
			name: "DeduplicatesPlanAndPriorState",
			modules: []*tfjson.StateModule{
				{
					Resources: []*tfjson.StateResource{
						managedScript("coder_script.setup", "setup"),
					},
				},
				{
					Resources: []*tfjson.StateResource{
						managedScript("coder_script.setup", "setup"),
					},
				},
			},
			selector: "coder_script.setup",
			expected: []string{"coder_script.setup"},
		},
		{
			name:     "EmptyModules",
			selector: "coder_script.setup",
		},
		{
			name: "NoMatchingScript",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedScript("coder_script.other", "other"),
				},
			}},
			selector: "coder_script.setup",
		},
		{
			name: "NoMatchingDeclaringModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.other",
				}},
			}},
			moduleAddress: "module.missing",
			selector:      "coder_script.setup",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selector, err := parseScriptOrderSelector(test.selector)
			require.NoError(t, err)

			resolved, err := resolveScriptOrderSelector(test.modules, test.moduleAddress, selector)
			require.NoError(t, err)
			require.Equal(t, test.expected, resolved)
		})
	}
}

func TestResolveScriptOrderSelectorRejectsMalformedStateAddresses(t *testing.T) {
	t.Parallel()

	t.Run("Script", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("coder_script.setup[0]")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				managedScript("not-an-address", "setup"),
			},
		}}, "", selector)
		require.Error(t, err)
	})

	t.Run("Module", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("module.bootstrap")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			ChildModules: []*tfjson.StateModule{{
				Address: "not-an-address",
			}},
		}}, "", selector)
		require.Error(t, err)
	})
}

func TestResolveScriptOrderSelectorRejectsInconsistentStateAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		moduleAddress string
		resource      *tfjson.StateResource
	}{
		{
			name:     "ResourceName",
			resource: managedScript("coder_script.other", "setup"),
		},
		{
			name:          "ContainingModule",
			moduleAddress: "module.expected",
			resource:      managedScript("module.other.coder_script.setup", "setup"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selector, err := parseScriptOrderSelector("coder_script.setup")
			require.NoError(t, err)

			_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
				Address:   test.moduleAddress,
				Resources: []*tfjson.StateResource{test.resource},
			}}, test.moduleAddress, selector)
			require.Error(t, err)
		})
	}
}

func managedScript(address, name string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

func dataModeScript(address, name string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

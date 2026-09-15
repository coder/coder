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
			name: "ScriptUnindexed",
			raw:  "coder_script.setup",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorScript,
				name:        "setup",
				instanceKey: "",
			},
		},
		{
			name: "ScriptCountInstance",
			raw:  "coder_script.setup[2]",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorScript,
				name:        "setup",
				instanceKey: "2",
			},
		},
		{
			name: "ScriptForEachInstance",
			raw:  `coder_script.setup["api"]`,
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorScript,
				name:        "setup",
				instanceKey: `"api"`,
			},
		},
		{
			name: "Module",
			raw:  "module.bootstrap",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorModule,
				name:        "bootstrap",
				instanceKey: "",
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
		"module.bootstrap.coder_script",
		"module.bootstrap.coder_script.setup",
		"module.parent.module.bootstrap",
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

	nestedDeclaringModules := []*tfjson.StateModule{{
		ChildModules: []*tfjson.StateModule{{
			Address: "module.outer",
			ChildModules: []*tfjson.StateModule{{
				Address: "module.outer.module.inner",
				Resources: []*tfjson.StateResource{
					managedCoderScript("module.outer.module.inner.coder_script.setup", "setup"),
				},
				ChildModules: []*tfjson.StateModule{{
					Address: "module.outer.module.inner.module.bootstrap",
					Resources: []*tfjson.StateResource{
						managedCoderScript("module.outer.module.inner.module.bootstrap.coder_script.install", "install"),
					},
				}},
			}},
		}},
	}}

	tests := []struct {
		name          string
		modules       []*tfjson.StateModule
		config        *tfjson.Config
		moduleAddress string
		selector      string
		expected      scriptOrderSelectorResolution
	}{
		{
			name: "AllMatchingManagedCountInstances",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedCoderScript("coder_script.setup[1]", "setup"),
					managedCoderScript("coder_script.other", "other"),
					managedCoderScript("coder_script.setup[0]", "setup"),
					dataCoderScript("data.coder_script.setup", "setup"),
				},
			}},
			selector: "coder_script.setup",
			expected: scriptOrderSelectorResolution{addresses: []string{
				"coder_script.setup[0]",
				"coder_script.setup[1]",
			}},
		},
		{
			name: "OneMatchingCountInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedCoderScript("coder_script.setup[0]", "setup"),
					managedCoderScript("coder_script.setup[1]", "setup"),
				},
			}},
			selector: "coder_script.setup[1]",
			expected: scriptOrderSelectorResolution{
				addresses: []string{"coder_script.setup[1]"},
			},
		},
		{
			name: "OneMatchingForEachInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedCoderScript(`coder_script.setup["worker"]`, "setup"),
					managedCoderScript(`coder_script.setup["api"]`, "setup"),
				},
			}},
			selector: `coder_script.setup["api"]`,
			expected: scriptOrderSelectorResolution{
				addresses: []string{`coder_script.setup["api"]`},
			},
		},
		{
			name: "MissingScriptInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedCoderScript("coder_script.setup[0]", "setup"),
				},
			}},
			selector: "coder_script.setup[1]",
			expected: scriptOrderSelectorResolution{
				addresses: nil,
			},
		},
		{
			name: "ScriptRelativeToRepeatedModuleInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedCoderScript("coder_script.setup", "setup"),
				},
				ChildModules: []*tfjson.StateModule{
					{
						Address: `module.development["primary"]`,
						Resources: []*tfjson.StateResource{
							managedCoderScript(`module.development["primary"].coder_script.setup`, "setup"),
						},
					},
					{
						Address: `module.development["secondary"]`,
						Resources: []*tfjson.StateResource{
							managedCoderScript(`module.development["secondary"].coder_script.setup`, "setup"),
						},
					},
				},
			}},
			moduleAddress: `module.development["primary"]`,
			selector:      "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses: []string{`module.development["primary"].coder_script.setup`},
			},
		},
		{
			name:          "ScriptRelativeToNestedDeclaringModule",
			modules:       nestedDeclaringModules,
			moduleAddress: "module.outer.module.inner",
			selector:      "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses: []string{"module.outer.module.inner.coder_script.setup"},
			},
		},
		{
			name: "RepeatedModulesAndDescendants",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{
					{
						Address: "module.bootstrap[1]",
						Resources: []*tfjson.StateResource{
							managedCoderScript("module.bootstrap[1].coder_script.install", "install"),
						},
						ChildModules: []*tfjson.StateModule{{
							Address: "module.bootstrap[1].module.nested",
							Resources: []*tfjson.StateResource{
								managedCoderScript("module.bootstrap[1].module.nested.coder_script.configure", "configure"),
							},
						}},
					},
					{
						Address: "module.bootstrap[0]",
						Resources: []*tfjson.StateResource{
							managedCoderScript("module.bootstrap[0].coder_script.install", "install"),
							dataCoderScript("module.bootstrap[0].data.coder_script.ignored", "ignored"),
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
							managedCoderScript("module.unrelated.coder_script.ignored", "ignored"),
						},
					},
				},
			}},
			config:   rootScriptOrderConfig("bootstrap", "unrelated"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses: []string{
					"module.bootstrap[0].coder_script.install",
					"module.bootstrap[1].coder_script.install",
					"module.bootstrap[1].module.nested.coder_script.configure",
				},
				moduleCallDeclared: true,
			},
		},
		{
			name: "ForEachModuleInstances",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{
					{
						Address: `module.bootstrap["secondary"]`,
						Resources: []*tfjson.StateResource{
							managedCoderScript(`module.bootstrap["secondary"].coder_script.setup`, "setup"),
						},
					},
					{
						Address: `module.bootstrap["primary"]`,
						Resources: []*tfjson.StateResource{
							managedCoderScript(`module.bootstrap["primary"].coder_script.setup`, "setup"),
						},
					},
				},
			}},
			config:   rootScriptOrderConfig("bootstrap"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses: []string{
					`module.bootstrap["primary"].coder_script.setup`,
					`module.bootstrap["secondary"].coder_script.setup`,
				},
				moduleCallDeclared: true,
			},
		},
		{
			name: "ModuleRelativeToRepeatedDeclaringModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{
					{
						Address: `module.development["primary"]`,
						ChildModules: []*tfjson.StateModule{{
							Address: `module.development["primary"].module.bootstrap`,
							Resources: []*tfjson.StateResource{
								managedCoderScript(`module.development["primary"].module.bootstrap.coder_script.setup`, "setup"),
							},
						}},
					},
					{
						Address: `module.development["secondary"]`,
						ChildModules: []*tfjson.StateModule{{
							Address: `module.development["secondary"].module.bootstrap`,
							Resources: []*tfjson.StateResource{
								managedCoderScript(`module.development["secondary"].module.bootstrap.coder_script.setup`, "setup"),
							},
						}},
					},
				},
			}},
			config:        nestedScriptOrderConfig("development", "bootstrap"),
			moduleAddress: `module.development["primary"]`,
			selector:      "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          []string{`module.development["primary"].module.bootstrap.coder_script.setup`},
				moduleCallDeclared: true,
			},
		},
		{
			name:          "ModuleRelativeToNestedDeclaringModule",
			modules:       nestedDeclaringModules,
			config:        nestedScriptOrderConfig("outer", "inner", "bootstrap"),
			moduleAddress: "module.outer.module.inner",
			selector:      "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          []string{"module.outer.module.inner.module.bootstrap.coder_script.install"},
				moduleCallDeclared: true,
			},
		},
		{
			name:     "DeclaredModuleWithNoInstances",
			config:   rootScriptOrderConfig("bootstrap"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          nil,
				moduleCallDeclared: true,
			},
		},
		{
			name: "DeclaredModuleWithNoScripts",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{Address: "module.bootstrap"}},
			}},
			config:   rootScriptOrderConfig("bootstrap"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          nil,
				moduleCallDeclared: true,
			},
		},
		{
			name:     "UnknownModuleCall",
			config:   rootScriptOrderConfig("other"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          nil,
				moduleCallDeclared: false,
			},
		},
		{
			name: "UnknownModuleCallWithStateInstances",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.bootstrap",
					Resources: []*tfjson.StateResource{
						managedCoderScript("module.bootstrap.coder_script.setup", "setup"),
					},
				}},
			}},
			config:   rootScriptOrderConfig("other"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          []string{"module.bootstrap.coder_script.setup"},
				moduleCallDeclared: false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selector, err := parseScriptOrderSelector(test.selector)
			require.NoError(t, err)

			resolved, err := resolveScriptOrderSelector(test.modules, test.config, test.moduleAddress, selector)
			require.NoError(t, err)
			require.Equal(t, test.expected, resolved)
		})
	}
}

func TestResolveScriptOrderSelectorRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	t.Run("MalformedScriptAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("coder_script.setup[0]")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{managedCoderScript("not-an-address", "setup")},
		}}, nil, "", selector)
		require.ErrorContains(t, err, `parse Terraform resource address "not-an-address"`)
	})

	t.Run("MalformedModuleAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("module.bootstrap")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			ChildModules: []*tfjson.StateModule{{Address: "not-an-address"}},
		}}, rootScriptOrderConfig("bootstrap"), "", selector)
		require.ErrorContains(t, err, `parse module address "not-an-address"`)
	})

	t.Run("InconsistentScriptAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{managedCoderScript("coder_script.other", "setup")},
		}}, nil, "", selector)
		require.ErrorContains(t, err, `Terraform resource address "coder_script.other" does not match its state fields`)
	})

	t.Run("MissingPlanConfiguration", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("module.bootstrap")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector(nil, nil, "", selector)
		require.ErrorContains(t, err, "terraform plan configuration is required to resolve a module selector")
	})
}

func rootScriptOrderConfig(moduleCalls ...string) *tfjson.Config {
	calls := make(map[string]*tfjson.ModuleCall, len(moduleCalls))
	for _, name := range moduleCalls {
		calls[name] = &tfjson.ModuleCall{Module: &tfjson.ConfigModule{}}
	}
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{ModuleCalls: calls}}
}

func nestedScriptOrderConfig(moduleCalls ...string) *tfjson.Config {
	root := &tfjson.ConfigModule{}
	module := root
	for _, name := range moduleCalls {
		child := &tfjson.ConfigModule{}
		module.ModuleCalls = map[string]*tfjson.ModuleCall{
			name: {Module: child},
		}
		module = child
	}
	return &tfjson.Config{RootModule: root}
}

func managedCoderScript(address, name string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

func dataCoderScript(address, name string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

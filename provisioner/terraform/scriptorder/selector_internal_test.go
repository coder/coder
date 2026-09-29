package scriptorder

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
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
				instanceKey: cty.NilVal,
			},
		},
		{
			name: "ScriptCountInstance",
			raw:  "coder_script.setup[2]",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorScript,
				name:        "setup",
				instanceKey: cty.NumberIntVal(2),
			},
		},
		{
			name: "ScriptForEachInstance",
			raw:  `coder_script.setup["api"]`,
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorScript,
				name:        "setup",
				instanceKey: cty.StringVal("api"),
			},
		},
		{
			name: "ScriptUnicode",
			raw:  "coder_script.π",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorScript,
				name:        "π",
				instanceKey: cty.NilVal,
			},
		},
		{
			name: "Module",
			raw:  "module.bootstrap",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorModule,
				name:        "bootstrap",
				instanceKey: cty.NilVal,
			},
		},
		{
			name: "ModuleUnicode",
			raw:  "module.开发",
			expected: scriptOrderSelector{
				kind:        scriptOrderSelectorModule,
				name:        "开发",
				instanceKey: cty.NilVal,
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
		"coder_script[0]",
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
					scriptOrderManagedCoderScript("module.outer.module.inner.coder_script.setup", "setup"),
				},
				ChildModules: []*tfjson.StateModule{{
					Address: "module.outer.module.inner.module.bootstrap",
					Resources: []*tfjson.StateResource{
						scriptOrderManagedCoderScript("module.outer.module.inner.module.bootstrap.coder_script.install", "install"),
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
					scriptOrderManagedCoderScript("coder_script.setup[1]", "setup"),
					scriptOrderManagedCoderScript("coder_script.other", "other"),
					scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
					scriptOrderDataCoderScript("data.coder_script.setup", "setup"),
					{
						Address: "null_resource.setup",
						Mode:    tfjson.ManagedResourceMode,
						Type:    "null_resource",
						Name:    "setup",
					},
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
					scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
					scriptOrderManagedCoderScript("coder_script.setup[1]", "setup"),
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
					scriptOrderManagedCoderScript(`coder_script.setup["worker"]`, "setup"),
					scriptOrderManagedCoderScript(`coder_script.setup["api"]`, "setup"),
				},
			}},
			selector: `coder_script.setup["api"]`,
			expected: scriptOrderSelectorResolution{
				addresses: []string{`coder_script.setup["api"]`},
			},
		},
		{
			name: "EscapedForEachInstanceKey",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript(`coder_script.setup["api"]`, "setup"),
				},
			}},
			selector: `coder_script.setup["\u0061pi"]`,
			expected: scriptOrderSelectorResolution{
				addresses: []string{`coder_script.setup["api"]`},
			},
		},
		{
			name: "MissingScriptInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
				},
			}},
			selector: "coder_script.setup[1]",
			expected: scriptOrderSelectorResolution{
				addresses: nil,
			},
		},
		{
			name: "DeclaredCountScriptWithNoInstances",
			// `count` is assumed to have evaluated to zero, so the
			// evaluated root module has no concrete resource
			// instance.
			modules: []*tfjson.StateModule{{}},
			config: rootScriptOrderConfigWithScripts(
				scriptOrderConfigCountCoderScript("setup"),
			),
			selector: "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses:              nil,
				scriptResourceDeclared: true,
			},
		},
		{
			name: "DeclaredForEachScriptWithNoInstancesInRepeatedModule",
			// The module has no Resources, while its config below
			// declares the script with `for_each`, modeling zero
			// evaluated script instances.
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: `module.development["primary"]`,
				}},
			}},
			config: &tfjson.Config{RootModule: &tfjson.ConfigModule{
				ModuleCalls: map[string]*tfjson.ModuleCall{
					"development": {Module: &tfjson.ConfigModule{
						Resources: []*tfjson.ConfigResource{
							scriptOrderConfigForEachCoderScript("setup"),
						},
					}},
				},
			}},
			moduleAddress: `module.development["primary"]`,
			selector:      "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses:              nil,
				scriptResourceDeclared: true,
			},
		},
		{
			name:     "UndeclaredScriptWithNoInstances",
			config:   rootScriptOrderConfigWithScripts(scriptOrderConfigCoderScript("other")),
			selector: "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses:              nil,
				scriptResourceDeclared: false,
			},
		},
		{
			name: "UndeclaredScriptInMissingDeclaringModule",
			// The evaluated module remains in prior state, but its
			// declaration is absent from the current config. A same-named
			// root resource must not satisfy the module-scoped selector.
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{Address: "module.removed"}},
			}},
			config: rootScriptOrderConfigWithScripts(
				scriptOrderConfigCoderScript("setup"),
			),
			moduleAddress: "module.removed",
			selector:      "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses:              nil,
				scriptResourceDeclared: false,
			},
		},
		{
			name: "NonManagedScriptDeclarationsAreIgnored",
			config: rootScriptOrderConfigWithScripts(
				&tfjson.ConfigResource{
					Address: "data.coder_script.setup",
					Mode:    tfjson.DataResourceMode,
					Type:    "coder_script",
					Name:    "setup",
				},
				&tfjson.ConfigResource{
					Address: "null_resource.setup",
					Mode:    tfjson.ManagedResourceMode,
					Type:    "null_resource",
					Name:    "setup",
				},
			),
			selector: "coder_script.setup",
			expected: scriptOrderSelectorResolution{
				addresses:              nil,
				scriptResourceDeclared: false,
			},
		},
		{
			name: "ScriptRelativeToRepeatedModuleInstance",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("coder_script.setup", "setup"),
				},
				ChildModules: []*tfjson.StateModule{
					{
						Address: `module.development["primary"]`,
						Resources: []*tfjson.StateResource{
							scriptOrderManagedCoderScript(`module.development["primary"].coder_script.setup`, "setup"),
						},
					},
					{
						Address: `module.development["secondary"]`,
						Resources: []*tfjson.StateResource{
							scriptOrderManagedCoderScript(`module.development["secondary"].coder_script.setup`, "setup"),
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
			name: "ScriptRelativeToUnicodeDeclaringModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.开发",
					Resources: []*tfjson.StateResource{
						scriptOrderManagedCoderScript("module.开发.coder_script.π", "π"),
					},
				}},
			}},
			moduleAddress: "module.开发",
			selector:      "coder_script.π",
			expected: scriptOrderSelectorResolution{
				addresses: []string{"module.开发.coder_script.π"},
			},
		},
		{
			name: "RepeatedModulesAndDescendants",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{
					{
						Address: "module.bootstrap[1]",
						Resources: []*tfjson.StateResource{
							scriptOrderManagedCoderScript("module.bootstrap[1].coder_script.install", "install"),
						},
						ChildModules: []*tfjson.StateModule{{
							Address: "module.bootstrap[1].module.nested",
							Resources: []*tfjson.StateResource{
								scriptOrderManagedCoderScript("module.bootstrap[1].module.nested.coder_script.configure", "configure"),
							},
						}},
					},
					{
						Address: "module.bootstrap[0]",
						Resources: []*tfjson.StateResource{
							scriptOrderManagedCoderScript("module.bootstrap[0].coder_script.install", "install"),
							scriptOrderDataCoderScript("module.bootstrap[0].data.coder_script.ignored", "ignored"),
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
							scriptOrderManagedCoderScript("module.unrelated.coder_script.ignored", "ignored"),
						},
					},
				},
			}},
			config:   rootScriptOrderConfigWithModuleCalls("bootstrap", "unrelated"),
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
							scriptOrderManagedCoderScript(`module.bootstrap["secondary"].coder_script.setup`, "setup"),
						},
					},
					{
						Address: `module.bootstrap["primary"]`,
						Resources: []*tfjson.StateResource{
							scriptOrderManagedCoderScript(`module.bootstrap["primary"].coder_script.setup`, "setup"),
						},
					},
				},
			}},
			config:   rootScriptOrderConfigWithModuleCalls("bootstrap"),
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
			name: "UnicodeModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.开发",
					Resources: []*tfjson.StateResource{
						scriptOrderManagedCoderScript("module.开发.coder_script.setup", "setup"),
					},
				}},
			}},
			config:   rootScriptOrderConfigWithModuleCalls("开发"),
			selector: "module.开发",
			expected: scriptOrderSelectorResolution{
				addresses:          []string{"module.开发.coder_script.setup"},
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
								scriptOrderManagedCoderScript(`module.development["primary"].module.bootstrap.coder_script.setup`, "setup"),
							},
						}},
					},
					{
						Address: `module.development["secondary"]`,
						ChildModules: []*tfjson.StateModule{{
							Address: `module.development["secondary"].module.bootstrap`,
							Resources: []*tfjson.StateResource{
								scriptOrderManagedCoderScript(`module.development["secondary"].module.bootstrap.coder_script.setup`, "setup"),
							},
						}},
					},
				},
			}},
			config:        nestedScriptOrderConfigWithModuleCalls("development", "bootstrap"),
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
			config:        nestedScriptOrderConfigWithModuleCalls("outer", "inner", "bootstrap"),
			moduleAddress: "module.outer.module.inner",
			selector:      "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          []string{"module.outer.module.inner.module.bootstrap.coder_script.install"},
				moduleCallDeclared: true,
			},
		},
		{
			name:     "DeclaredModuleWithNoInstances",
			config:   rootScriptOrderConfigWithModuleCalls("bootstrap"),
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
			config:   rootScriptOrderConfigWithModuleCalls("bootstrap"),
			selector: "module.bootstrap",
			expected: scriptOrderSelectorResolution{
				addresses:          nil,
				moduleCallDeclared: true,
			},
		},
		{
			name:     "UnknownModuleCall",
			config:   rootScriptOrderConfigWithModuleCalls("other"),
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
						scriptOrderManagedCoderScript("module.bootstrap.coder_script.setup", "setup"),
					},
				}},
			}},
			config:   rootScriptOrderConfigWithModuleCalls("other"),
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
			Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript("not-an-address", "setup")},
		}}, nil, "", selector)
		require.ErrorContains(t, err, `parse Terraform resource address "not-an-address"`)
	})

	t.Run("MalformedStateModuleAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("module.bootstrap")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			ChildModules: []*tfjson.StateModule{{Address: "not-an-address"}},
		}}, rootScriptOrderConfigWithModuleCalls("bootstrap"), "", selector)
		require.ErrorContains(t, err, `parse module address "not-an-address"`)
	})

	t.Run("MalformedDeclaringModuleAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector(
			nil,
			rootScriptOrderConfigWithScripts(scriptOrderConfigCoderScript("setup")),
			"not-an-address",
			selector,
		)
		require.ErrorContains(t, err, `parse module address "not-an-address"`)
	})

	t.Run("InconsistentScriptAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector([]*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript("coder_script.other", "setup")},
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

	t.Run("MissingPlanConfigurationForEmptyScript", func(t *testing.T) {
		t.Parallel()

		selector, err := parseScriptOrderSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveScriptOrderSelector(nil, nil, "", selector)
		require.ErrorContains(t, err, "cannot validate empty coder_script selector because Terraform plan configuration is unavailable")
	})
}

func rootScriptOrderConfigWithModuleCalls(moduleCalls ...string) *tfjson.Config {
	calls := make(map[string]*tfjson.ModuleCall, len(moduleCalls))
	for _, name := range moduleCalls {
		calls[name] = &tfjson.ModuleCall{Module: &tfjson.ConfigModule{}}
	}
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{ModuleCalls: calls}}
}

func nestedScriptOrderConfigWithModuleCalls(moduleCalls ...string) *tfjson.Config {
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

func rootScriptOrderConfigWithScripts(
	scripts ...*tfjson.ConfigResource,
) *tfjson.Config {
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{Resources: scripts}}
}

func scriptOrderConfigCoderScript(name string) *tfjson.ConfigResource {
	return &tfjson.ConfigResource{
		Address: "coder_script." + name,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

func scriptOrderConfigCountCoderScript(name string) *tfjson.ConfigResource {
	script := scriptOrderConfigCoderScript(name)
	script.CountExpression = &tfjson.Expression{}
	return script
}

func scriptOrderConfigForEachCoderScript(name string) *tfjson.ConfigResource {
	script := scriptOrderConfigCoderScript(name)
	script.ForEachExpression = &tfjson.Expression{}
	return script
}

func scriptOrderManagedCoderScript(address, name string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

func scriptOrderDataCoderScript(address, name string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

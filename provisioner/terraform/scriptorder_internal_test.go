package terraform

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

func TestCollectScriptOrderRuleDeclarations(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.a", "a"),
			scriptOrderManagedCoderScript("coder_script.b", "b"),
			scriptOrderManagedCoderScript("coder_script.c", "c"),
			dataCoderScriptOrder(
				"data.coder_script_order.second",
				"second",
				scriptOrderRuleAttributes{
					Run:      []string{"coder_script.c"},
					After:    []string{"module.bootstrap"},
					Requires: "completion",
					Phase:    "start",
				},
			),
			dataCoderScriptOrder(
				"data.coder_script_order.first",
				"first",
				scriptOrderRuleAttributes{
					Run:      []string{"coder_script.b", "coder_script.c"},
					After:    []string{"coder_script.a", "module.bootstrap"},
					Requires: "success",
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.bootstrap",
			Resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("module.bootstrap.coder_script.install", "install"),
			},
		}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("bootstrap"),
	)
	require.NoError(t, err)
	require.Equal(t, []scriptOrderRuleDeclaration{
		{
			dataSourceAddress: "data.coder_script_order.first",
			ruleIndex:         0,
			declaredPhase:     "",
			requirement:       scriptOrderRequirementSuccess,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.b",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.b"},
			}, {
				field:     "run",
				raw:       "coder_script.c",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "coder_script.a",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.a"},
			}, {
				field:     "after",
				raw:       "module.bootstrap",
				kind:      scriptOrderSelectorModule,
				addresses: []string{"module.bootstrap.coder_script.install"},
			}},
		},
		{
			dataSourceAddress: "data.coder_script_order.second",
			ruleIndex:         0,
			declaredPhase:     scriptOrderPhaseStart,
			requirement:       scriptOrderRequirementCompletion,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.c",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "module.bootstrap",
				kind:      scriptOrderSelectorModule,
				addresses: []string{"module.bootstrap.coder_script.install"},
			}},
		},
	}, declarations)
}

func TestCollectScriptOrderRuleDeclarationsPreservesRuleOrderAndIndexes(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.a", "a"),
		scriptOrderManagedCoderScript("coder_script.b", "b"),
		scriptOrderManagedCoderScript("coder_script.c", "c"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.b"},
				After: []string{"coder_script.a"},
				Phase: "stop",
			},
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.c"},
				After: []string{"coder_script.b"},
			},
		),
	}}

	// Rule indexes identify the originating Terraform rule in
	// diagnostics, so declarations must retain source order.
	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, nil,
	)
	require.NoError(t, err)
	require.Len(t, declarations, 2)
	require.Equal(t, 0, declarations[0].ruleIndex)
	require.Equal(t, scriptOrderPhaseStop, declarations[0].declaredPhase)
	require.Equal(t, "coder_script.b", declarations[0].run[0].raw)
	require.Equal(t, 1, declarations[1].ruleIndex)
	require.Equal(t, scriptOrderRequirementSuccess, declarations[1].requirement)
	require.Equal(t, "coder_script.c", declarations[1].run[0].raw)
}

func TestCollectScriptOrderRuleDeclarationsRelativeToDeclaringModule(t *testing.T) {
	t.Parallel()

	declaringModule := &tfjson.StateModule{
		Address: "module.development",
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("module.development.coder_script.configure", "configure"),
			dataCoderScriptOrder(
				"module.development.data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.configure"},
					After: []string{"module.bootstrap"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.development.module.bootstrap",
			Resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript(
					"module.development.module.bootstrap.coder_script.install", "install",
				),
			},
		}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{{ChildModules: []*tfjson.StateModule{declaringModule}}},
		nestedScriptOrderConfigWithModuleCalls("development", "bootstrap"),
	)
	require.NoError(t, err)
	require.Len(t, declarations, 1)
	require.Equal(t,
		[]string{"module.development.coder_script.configure"},
		declarations[0].run[0].addresses,
	)
	require.Equal(t,
		[]string{"module.development.module.bootstrap.coder_script.install"},
		declarations[0].after[0].addresses,
	)
}

func TestCollectScriptOrderRuleDeclarationsAllowsEmptyScripts(t *testing.T) {
	t.Parallel()

	// Only prerequisite has a concrete instance. The plan config below
	// declares optional_count and optional_each with zero instances.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.optional_count"},
				After: []string{"coder_script.optional_each"},
			},
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.optional_count"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	config := rootScriptOrderConfigWithScripts(
		scriptOrderConfigCountCoderScript("optional_count"),
		scriptOrderConfigForEachCoderScript("optional_each"),
	)

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, config,
	)
	require.NoError(t, err)
	require.Len(t, declarations, 2)
	require.Nil(t, declarations[0].run[0].addresses)
	require.Nil(t, declarations[0].after[0].addresses)
	require.Nil(t, declarations[1].run[0].addresses)
	require.Equal(t,
		[]string{"coder_script.prerequisite"},
		declarations[1].after[0].addresses,
	)
}

func TestCollectScriptOrderRuleDeclarationsAllowsEmptyModules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.configure", "configure"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"module.disabled"},
					After: []string{"module.without_scripts"},
				},
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.configure"},
					After: []string{"module.disabled"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{Address: "module.without_scripts"}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("disabled", "without_scripts"),
	)
	require.NoError(t, err)
	require.Len(t, declarations, 2)
	require.Nil(t, declarations[0].run[0].addresses)
	require.Nil(t, declarations[0].after[0].addresses)
	require.Equal(t,
		[]string{"coder_script.configure"}, declarations[1].run[0].addresses,
	)
	require.Nil(t, declarations[1].after[0].addresses)
}

func TestCollectScriptOrderRuleDeclarationsWithoutScriptOrderDataSources(t *testing.T) {
	t.Parallel()

	// Templates that do not opt into script ordering must remain unaffected.
	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{{Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.configure", "configure"),
			{
				Address: "data.coder_workspace.me",
				Mode:    tfjson.DataResourceMode,
				Type:    "coder_workspace",
				Name:    "me",
			},
			{
				Address: "coder_script_order.order",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "coder_script_order",
				Name:    "order",
			},
		}}},
		nil,
	)
	require.NoError(t, err)
	require.Nil(t, declarations)
}

func TestCollectScriptOrderRuleDeclarationsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rule       scriptOrderRuleAttributes
		resources  []*tfjson.StateResource
		modules    []*tfjson.StateModule
		planConfig *tfjson.Config
		contains   []string
	}{
		{
			name: "EmptyRunList",
			rule: scriptOrderRuleAttributes{After: []string{"coder_script.a"}},
			contains: []string{
				"rule 0", "run must contain at least one selector",
			},
		},
		{
			name: "EmptyAfterList",
			rule: scriptOrderRuleAttributes{Run: []string{"coder_script.a"}},
			contains: []string{
				"rule 0", "after must contain at least one selector",
			},
		},
		{
			name: "InvalidRequirement",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Requires: "started",
			},
			contains: []string{"rule 0", "requires", "success", "completion", "started"},
		},
		{
			name: "InvalidPhase",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Phase: "startup",
			},
			contains: []string{"rule 0", "phase", "start", "stop", "startup"},
		},
		{
			name: "MalformedSelector",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b["}, After: []string{"coder_script.a"},
			},
			contains: []string{"rule 0", "run selector", "coder_script.b["},
		},
		{
			name: "UndeclaredScriptWithNoInstances",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.missing"}, After: []string{"coder_script.a"},
			},
			planConfig: rootScriptOrderConfigWithScripts(
				scriptOrderConfigCoderScript("other"),
			),
			contains: []string{
				"rule 0", "run selector", "coder_script.missing", "does not name a declared coder_script resource",
			},
		},
		{
			name: "EmptyScriptSelectorRequiresPlanConfiguration",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.missing"}, After: []string{"coder_script.a"},
			},
			planConfig: nil,
			contains: []string{
				"rule 0", "resolve run selector", "coder_script.missing",
				"Terraform plan configuration is unavailable",
			},
		},
		{
			name: "DeclaredCountResourceWithoutEvaluatedInstance",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.a[0]"}, After: []string{"coder_script.b"},
			},
			resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.b", "b"),
			},
			planConfig: rootScriptOrderConfigWithScripts(
				scriptOrderConfigCountCoderScript("a"),
			),
			contains: []string{
				"rule 0", "run selector", "coder_script.a[0]", "expanded to no coder_script resources",
			},
		},
		{
			name: "DeclaredForEachResourceWithoutEvaluatedInstance",
			rule: scriptOrderRuleAttributes{
				Run: []string{`coder_script.a["api"]`}, After: []string{"coder_script.b"},
			},
			resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.b", "b"),
			},
			planConfig: rootScriptOrderConfigWithScripts(
				scriptOrderConfigForEachCoderScript("a"),
			),
			contains: []string{
				"rule 0", "run selector", "coder_script.a", "api",
				"expanded to no coder_script resources",
			},
		},
		{
			name: "UndeclaredModule",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.missing"},
			},
			planConfig: rootScriptOrderConfigWithModuleCalls("other"),
			contains: []string{
				"rule 0", "after selector", "module.missing", "does not name a declared child module call",
			},
		},
		{
			name: "UndeclaredModuleWithStateInstance",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.missing"},
			},
			// A module removed from an updated template can remain in
			// prior state even though its call is absent from the
			// current configuration.
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.missing",
					Resources: []*tfjson.StateResource{
						scriptOrderManagedCoderScript("module.missing.coder_script.install", "install"),
					},
				}},
			}},
			planConfig: rootScriptOrderConfigWithModuleCalls("other"),
			contains: []string{
				"rule 0", "after selector", "module.missing", "does not name a declared child module call",
			},
		},
		{
			name: "ModuleSelectorRequiresPlanConfiguration",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.bootstrap"},
			},
			planConfig: nil,
			contains: []string{
				"rule 0", "resolve after selector", "terraform plan configuration is required",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := test.resources
			if resources == nil {
				resources = []*tfjson.StateResource{
					scriptOrderManagedCoderScript("coder_script.a", "a"),
					scriptOrderManagedCoderScript("coder_script.b", "b"),
				}
			}
			resources = append(resources, dataCoderScriptOrder(
				"data.coder_script_order.order", "order", test.rule,
			))
			modules := append([]*tfjson.StateModule{{Resources: resources}}, test.modules...)

			declarations, err := collectScriptOrderRuleDeclarations(
				modules, test.planConfig,
			)
			require.Error(t, err)
			require.Nil(t, declarations)
			for _, contains := range test.contains {
				require.ErrorContains(t, err, contains)
			}
		})
	}
}

func TestCollectScriptOrderRuleDeclarationsDataSourceValidation(t *testing.T) {
	t.Parallel()

	t.Run("MalformedAttributeValues", func(t *testing.T) {
		t.Parallel()

		// Malformed serialized rule values must fail with enough
		// context to identify the data source.
		declarations, err := collectScriptOrderRuleDeclarations(
			[]*tfjson.StateModule{{Resources: []*tfjson.StateResource{{
				Address: "data.coder_script_order.order",
				Mode:    tfjson.DataResourceMode,
				Type:    "coder_script_order",
				Name:    "order",
				AttributeValues: map[string]any{
					"rule": "invalid",
				},
			}}}},
			nil,
		)
		require.ErrorContains(t, err, `decode script order data source "data.coder_script_order.order"`)
		require.Nil(t, declarations)
	})

	t.Run("NoRuleBlocks", func(t *testing.T) {
		t.Parallel()

		declarations, err := collectScriptOrderRuleDeclarations(
			[]*tfjson.StateModule{{Resources: []*tfjson.StateResource{
				dataCoderScriptOrder("data.coder_script_order.order", "order"),
			}}},
			nil,
		)
		require.ErrorContains(t, err, "must contain at least one rule")
		require.Nil(t, declarations)
	})
}

func dataCoderScriptOrder(
	address string,
	name string,
	rules ...scriptOrderRuleAttributes,
) *tfjson.StateResource {
	ruleValues := make([]any, 0, len(rules))
	for _, rule := range rules {
		value := map[string]any{
			"run":   rule.Run,
			"after": rule.After,
		}
		if rule.Requires != "" {
			value["requires"] = rule.Requires
		}
		if rule.Phase != "" {
			value["phase"] = rule.Phase
		}
		ruleValues = append(ruleValues, value)
	}
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script_order",
		Name:    name,
		AttributeValues: map[string]any{
			"rule": ruleValues,
		},
	}
}

func TestResolveScriptOrder(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.a", "a"),
		scriptOrderManagedCoderScript("coder_script.b", "b"),
		scriptOrderManagedCoderScript("coder_script.c", "c"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.b"},
				After: []string{"coder_script.a"},
			},
			scriptOrderRuleAttributes{
				Run:      []string{"coder_script.c"},
				After:    []string{"coder_script.b"},
				Requires: "completion",
			},
		),
	}}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStart,
		"coder_script.a", "coder_script.b", "coder_script.c",
	)

	order, err := resolveScriptOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.NoError(t, err)
	require.Empty(t, order.warnings)
	require.Equal(t, []resolvedScriptOrderRule{
		{
			dataSourceAddress: "data.coder_script_order.order",
			ruleIndex:         0,
			runtimeAddress:    "coder_agent.main",
			phase:             scriptOrderPhaseStart,
			requirement:       scriptOrderRequirementSuccess,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.b",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.b"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "coder_script.a",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.a"},
			}},
		},
		{
			dataSourceAddress: "data.coder_script_order.order",
			ruleIndex:         1,
			runtimeAddress:    "coder_agent.main",
			phase:             scriptOrderPhaseStart,
			requirement:       scriptOrderRequirementCompletion,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.c",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "coder_script.b",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.b"},
			}},
		},
	}, order.rules)
}

func TestResolveScriptOrderInfersStopPhaseFromScriptSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.archive", "archive"),
		scriptOrderManagedCoderScript("coder_script.shutdown", "shutdown"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.archive"},
				After: []string{"coder_script.shutdown"},
			},
		),
	}}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStop,
		"coder_script.archive", "coder_script.shutdown",
	)

	order, err := resolveScriptOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, scriptOrderPhaseStop, order.rules[0].phase)
	require.Equal(t, "coder_agent.main", order.rules[0].runtimeAddress)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderFiltersModuleScriptsByPhase(t *testing.T) {
	t.Parallel()

	module := scriptOrderModuleFilteringFixture("")
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStart,
		"coder_script.dependent",
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	// Scripts omitted by phase filtering do not participate in
	// runtime validation.
	scripts["module.alpha.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.other",
		runOnStop:      true,
	}
	scripts["module.beta.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.other",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("alpha", "beta"), scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, scriptOrderPhaseStart, order.rules[0].phase)
	require.Equal(t,
		[]string{"module.beta.coder_script.start"},
		order.rules[0].after[0].addresses,
	)
	require.Equal(t,
		[]string{"module.alpha.coder_script.start"},
		order.rules[0].after[1].addresses,
	)
	require.Equal(t, []scriptOrderPhaseFilterWarning{{
		dataSourceAddress:            "data.coder_script_order.order",
		ruleIndex:                    0,
		inferredPhase:                scriptOrderPhaseStart,
		moduleSelectorsWithOmissions: []string{"module.alpha", "module.beta"},
	}}, order.warnings)
	require.Equal(t,
		`script order data source "data.coder_script_order.order" rule 0 inferred phase "start" and omitted scripts in the other phase from module selectors: "module.alpha", "module.beta"; set phase = "start" explicitly to make this filtering intentional`,
		order.warnings[0].String(),
	)
}

func TestResolveScriptOrderExplicitPhaseFiltersWithoutWarning(t *testing.T) {
	t.Parallel()

	module := scriptOrderModuleFilteringFixture("start")
	module.Resources = []*tfjson.StateResource{dataCoderScriptOrder(
		"data.coder_script_order.order",
		"order",
		scriptOrderRuleAttributes{
			Run:   []string{"module.beta"},
			After: []string{"module.alpha"},
			Phase: "start",
		},
	)}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStart,
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	scripts["module.alpha.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}
	scripts["module.beta.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("alpha", "beta"), scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, scriptOrderPhaseStart, order.rules[0].phase)
	require.Equal(t,
		[]string{"module.beta.coder_script.start"},
		order.rules[0].run[0].addresses,
	)
	require.Equal(t,
		[]string{"module.alpha.coder_script.start"},
		order.rules[0].after[0].addresses,
	)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderAllowsSideEmptiedByPhaseFiltering(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.first", "first"),
			scriptOrderManagedCoderScript("coder_script.second", "second"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.first", "coder_script.second"},
					After: []string{"module.shutdown"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.shutdown",
			Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
				"module.shutdown.coder_script.stop", "stop",
			)},
		}},
	}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStart, "coder_script.first",
	)
	scripts["coder_script.second"] = scriptOrderScript{
		runtimeAddress: "coder_agent.other",
		runOnStart:     true,
	}
	scripts["module.shutdown.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("shutdown"), scripts,
	)
	// Filtering every after address makes the rule a valid no-op
	// while retaining the inferred-phase warning.
	require.NoError(t, err)
	require.Empty(t, order.rules)
	require.Equal(t, []scriptOrderPhaseFilterWarning{{
		dataSourceAddress:            "data.coder_script_order.order",
		ruleIndex:                    0,
		inferredPhase:                scriptOrderPhaseStart,
		moduleSelectorsWithOmissions: []string{"module.shutdown"},
	}}, order.warnings)
}

func TestResolveScriptOrderInfersPhaseFromModules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		)},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.application",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.application.coder_script.shutdown", "shutdown",
				)},
			},
			{
				Address: "module.checkout",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.checkout.coder_script.archive", "archive",
				)},
			},
		},
	}
	scripts := scriptOrderTestScripts(
		"coder_devcontainer.repo", scriptOrderPhaseStop,
		"module.application.coder_script.shutdown",
		"module.checkout.coder_script.archive",
	)

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("application", "checkout"),
		scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, scriptOrderPhaseStop, order.rules[0].phase)
	require.Equal(t, "coder_devcontainer.repo", order.rules[0].runtimeAddress)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderRejectsMixedPhasesWithinSelector(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
		scriptOrderManagedCoderScript("coder_script.setup[1]", "setup"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.setup"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	// The unindexed setup selector expands to one start instance and
	// one stop instance.
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStart,
		"coder_script.setup[0]", "coder_script.prerequisite",
	)
	scripts["coder_script.setup[1]"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.ErrorContains(t, err, `run selector "coder_script.setup"`)
	require.ErrorContains(t, err, "cannot mix start and stop scripts")
	require.Empty(t, order)
}

func TestResolveScriptOrderAllowsEmptyModuleRule(t *testing.T) {
	t.Parallel()

	// The evaluated module tree contains no script instances from
	// either child module. The config below declares both module
	// calls, so their selectors are valid but expand to no scripts.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		),
	}}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("application", "checkout"),
		nil,
	)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestResolveScriptOrderAllowsEmptyScriptRule(t *testing.T) {
	t.Parallel()

	// The evaluated module contains no concrete script instances. The
	// config below declares both scripts with zero instances.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.optional_run"},
				After: []string{"coder_script.optional_after"},
			},
		),
	}}
	config := rootScriptOrderConfigWithScripts(
		scriptOrderConfigCountCoderScript("optional_run"),
		scriptOrderConfigForEachCoderScript("optional_after"),
	)

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, config, nil,
	)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestResolveScriptOrderAllowsEmptyScriptSelectorAlongsideResolvedSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run: []string{"coder_script.dependent"},
				After: []string{
					"coder_script.optional",
					"coder_script.prerequisite",
				},
			},
		),
	}}
	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithScripts(
			scriptOrderConfigCountCoderScript("optional"),
		),
		scriptOrderTestScripts(
			"coder_agent.main", scriptOrderPhaseStart,
			"coder_script.dependent", "coder_script.prerequisite",
		),
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Empty(t, order.rules[0].after[0].addresses)
	require.Equal(t,
		[]string{"coder_script.prerequisite"},
		order.rules[0].after[1].addresses,
	)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderAllowsEmptyModuleSelectorAlongsideResolvedSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run: []string{"coder_script.dependent"},
				After: []string{
					"module.disabled",
					"coder_script.prerequisite",
				},
			},
		),
	}}
	// The plan declares module.disabled, but the evaluated state
	// contains no instance, modeling a module call with count = 0.
	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("disabled"),
		scriptOrderTestScripts(
			"coder_agent.main", scriptOrderPhaseStart,
			"coder_script.dependent", "coder_script.prerequisite",
		),
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Empty(t, order.rules[0].after[0].addresses)
	require.Equal(t,
		[]string{"coder_script.prerequisite"},
		order.rules[0].after[1].addresses,
	)
}

func TestResolveScriptOrderRejectsInvalidRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rule      scriptOrderRuleAttributes
		resources []*tfjson.StateResource
		scripts   map[string]scriptOrderScript
		contains  []string
	}{
		{
			name: "ScriptNotFound",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", scriptOrderPhaseStart, "coder_script.a",
			),
			contains: []string{"rule 0", "coder_script.b", "was not found"},
		},
		{
			name: "NoLifecyclePhase",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main"},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "neither run_on_start nor run_on_stop enabled"},
		},
		{
			name: "CronOnly",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {
					runtimeAddress: "coder_agent.main", cron: "0 * * * *",
				},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "cron-only"},
		},
		{
			name: "BothLifecyclePhases",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {
					runtimeAddress: "coder_agent.main", runOnStart: true, runOnStop: true,
				},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "both run_on_start and run_on_stop enabled"},
		},
		{
			name: "MixedResourcePhases",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStop: true},
			},
			contains: []string{"rule 0", "start script", "stop script", "cannot mix start and stop scripts"},
		},
		{
			name: "ResourceConflictsWithDeclaredPhase",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Phase: "stop",
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", scriptOrderPhaseStart, "coder_script.a", "coder_script.b",
			),
			contains: []string{"coder_script.b", "start script", `rule phase is "stop"`},
		},
		{
			name: "CrossRuntime",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runtimeAddress: "coder_devcontainer.repo", runOnStart: true},
			},
			contains: []string{
				"coder_agent.main", "coder_devcontainer.repo", "only within the same agent or devcontainer subagent",
			},
		},
		{
			name: "MissingRuntime",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runOnStart: true},
			},
			contains: []string{"coder_script.b", "could not be associated with an agent or devcontainer subagent"},
		},
		{
			name: "SelfDependency",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.a"}, After: []string{"coder_script.a"},
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", scriptOrderPhaseStart, "coder_script.a", "coder_script.b",
			),
			contains: []string{"run selector", "after selector", "coder_script.a", "depend on itself"},
		},
		{
			name: "SelfDependencyAfterSelectorExpansion",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.setup"}, After: []string{"coder_script.setup[0]"},
			},
			resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", scriptOrderPhaseStart, "coder_script.setup[0]",
			),
			contains: []string{
				"run selector", "coder_script.setup", "after selector",
				"coder_script.setup[0]", "depend on itself",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.a", "a"),
				scriptOrderManagedCoderScript("coder_script.b", "b"),
			}
			resources = append(resources, test.resources...)
			resources = append(resources, dataCoderScriptOrder(
				"data.coder_script_order.order", "order", test.rule,
			))
			module := &tfjson.StateModule{Resources: resources}
			order, err := resolveScriptOrder(
				[]*tfjson.StateModule{module}, nil, test.scripts,
			)
			require.Error(t, err)
			require.Empty(t, order)
			for _, contains := range test.contains {
				require.ErrorContains(t, err, contains)
			}
		})
	}
}

func TestResolveScriptOrderRejectsInvalidModuleSelectedScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   scriptOrderScript
		contains string
	}{
		{
			name: "CronOnly",
			script: scriptOrderScript{
				runtimeAddress: "coder_agent.main",
				cron:           "0 * * * *",
			},
			contains: "cron-only",
		},
		{
			name: "BothLifecyclePhases",
			script: scriptOrderScript{
				runtimeAddress: "coder_agent.main",
				runOnStart:     true,
				runOnStop:      true,
			},
			contains: "both run_on_start and run_on_stop enabled",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			module := &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
					dataCoderScriptOrder(
						"data.coder_script_order.order",
						"order",
						scriptOrderRuleAttributes{
							Run:   []string{"coder_script.dependent"},
							After: []string{"module.bootstrap"},
						},
					),
				},
				ChildModules: []*tfjson.StateModule{{
					Address: "module.bootstrap",
					Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
						"module.bootstrap.coder_script.setup", "setup",
					)},
				}},
			}
			scripts := scriptOrderTestScripts(
				"coder_agent.main", scriptOrderPhaseStart, "coder_script.dependent",
			)
			scripts["module.bootstrap.coder_script.setup"] = test.script

			order, err := resolveScriptOrder(
				[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("bootstrap"), scripts,
			)
			require.ErrorContains(t, err, `after selector "module.bootstrap"`)
			require.ErrorContains(t, err, test.contains)
			require.Empty(t, order)
		})
	}
}

func TestResolveScriptOrderRejectsAmbiguousModulePhase(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		)},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.application",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.application.coder_script.start", "start",
				)},
			},
			{
				Address: "module.checkout",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.checkout.coder_script.stop", "stop",
				)},
			},
		},
	}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", scriptOrderPhaseStart,
		"module.application.coder_script.start",
	)
	scripts["module.checkout.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	// With only module selectors spanning both phases and no explicit
	// phase, the rule's phase cannot be inferred.
	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("application", "checkout"),
		scripts,
	)
	require.ErrorContains(t, err, "phase cannot be inferred")
	require.ErrorContains(t, err, `set phase to "start" or "stop"`)
	require.Empty(t, order)
}

func scriptOrderModuleFilteringFixture(phase string) *tfjson.StateModule {
	return &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.dependent"},
					After: []string{"module.beta", "module.alpha"},
					Phase: phase,
				},
			),
		},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.alpha",
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("module.alpha.coder_script.start", "start"),
					scriptOrderManagedCoderScript("module.alpha.coder_script.stop", "stop"),
				},
			},
			{
				Address: "module.beta",
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("module.beta.coder_script.start", "start"),
					scriptOrderManagedCoderScript("module.beta.coder_script.stop", "stop"),
				},
			},
		},
	}
}

func scriptOrderTestScripts(
	runtimeAddress string,
	phase scriptOrderPhase,
	addresses ...string,
) map[string]scriptOrderScript {
	scripts := make(map[string]scriptOrderScript, len(addresses))
	for _, address := range addresses {
		script := scriptOrderScript{runtimeAddress: runtimeAddress}
		switch phase {
		case scriptOrderPhaseStart:
			script.runOnStart = true
		case scriptOrderPhaseStop:
			script.runOnStop = true
		default:
			panic("unknown script order phase")
		}
		scripts[address] = script
	}
	return scripts
}

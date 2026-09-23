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
			name: "EscapedForEachInstanceKey",
			modules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					managedCoderScript(`coder_script.setup["api"]`, "setup"),
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
			name: "ScriptRelativeToUnicodeDeclaringModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.开发",
					Resources: []*tfjson.StateResource{
						managedCoderScript("module.开发.coder_script.π", "π"),
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
			name: "UnicodeModule",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.开发",
					Resources: []*tfjson.StateResource{
						managedCoderScript("module.开发.coder_script.setup", "setup"),
					},
				}},
			}},
			config:   rootScriptOrderConfig("开发"),
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

func TestCollectScriptOrderRuleDeclarations(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			managedCoderScript("coder_script.a", "a"),
			managedCoderScript("coder_script.b", "b"),
			managedCoderScript("coder_script.c", "c"),
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
				managedCoderScript("module.bootstrap.coder_script.install", "install"),
			},
		}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, rootScriptOrderConfig("bootstrap"),
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
		managedCoderScript("coder_script.a", "a"),
		managedCoderScript("coder_script.b", "b"),
		managedCoderScript("coder_script.c", "c"),
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
			managedCoderScript("module.development.coder_script.configure", "configure"),
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
				managedCoderScript(
					"module.development.module.bootstrap.coder_script.install", "install",
				),
			},
		}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{{ChildModules: []*tfjson.StateModule{declaringModule}}},
		nestedScriptOrderConfig("development", "bootstrap"),
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

func TestCollectScriptOrderRuleDeclarationsAllowsEmptyModules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			managedCoderScript("coder_script.configure", "configure"),
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
		rootScriptOrderConfig("disabled", "without_scripts"),
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
			managedCoderScript("coder_script.configure", "configure"),
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
			name: "ScriptWithNoInstances",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.missing"}, After: []string{"coder_script.a"},
			},
			contains: []string{
				"rule 0", "run selector", "coder_script.missing", "expanded to no coder_script resources",
			},
		},
		{
			name: "UndeclaredModule",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.missing"},
			},
			planConfig: rootScriptOrderConfig("other"),
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
						managedCoderScript("module.missing.coder_script.install", "install"),
					},
				}},
			}},
			planConfig: rootScriptOrderConfig("other"),
			contains: []string{
				"rule 0", "after selector", "module.missing", "does not name a declared child module call",
			},
		},
		{
			name: "ModuleSelectorRequiresPlanConfiguration",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.bootstrap"},
			},
			contains: []string{
				"rule 0", "resolve after selector", "terraform plan configuration is required",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := []*tfjson.StateResource{
				managedCoderScript("coder_script.a", "a"),
				managedCoderScript("coder_script.b", "b"),
				dataCoderScriptOrder(
					"data.coder_script_order.order", "order", test.rule,
				),
			}
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

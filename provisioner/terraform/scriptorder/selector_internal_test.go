package scriptorder

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestParseSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected selector
	}{
		{
			name: "ScriptUnindexed",
			raw:  "coder_script.setup",
			expected: selector{
				kind:        selectorScript,
				name:        "setup",
				instanceKey: cty.NilVal,
			},
		},
		{
			name: "ScriptCountInstance",
			raw:  "coder_script.setup[2]",
			expected: selector{
				kind:        selectorScript,
				name:        "setup",
				instanceKey: cty.NumberIntVal(2),
			},
		},
		{
			name: "ScriptForEachInstance",
			raw:  `coder_script.setup["api"]`,
			expected: selector{
				kind:        selectorScript,
				name:        "setup",
				instanceKey: cty.StringVal("api"),
			},
		},
		{
			name: "ScriptUnicode",
			raw:  "coder_script.π",
			expected: selector{
				kind:        selectorScript,
				name:        "π",
				instanceKey: cty.NilVal,
			},
		},
		{
			name: "Module",
			raw:  "module.bootstrap",
			expected: selector{
				kind:        selectorModule,
				name:        "bootstrap",
				instanceKey: cty.NilVal,
			},
		},
		{
			name: "ModuleUnicode",
			raw:  "module.开发",
			expected: selector{
				kind:        selectorModule,
				name:        "开发",
				instanceKey: cty.NilVal,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selector, err := parseSelector(test.raw)
			require.NoError(t, err)
			require.Equal(t, test.expected, selector)
		})
	}
}

func TestParseSelectorRejectsUnsupportedSyntax(t *testing.T) {
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

			_, err := parseSelector(selector)
			require.Error(t, err)
		})
	}
}

func TestResolveSelector(t *testing.T) {
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
		expected      selectorResolution
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
			expected: selectorResolution{addresses: []string{
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
			expected: selectorResolution{
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
			expected: selectorResolution{
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
			expected: selectorResolution{
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
			expected: selectorResolution{
				addresses: nil,
			},
		},
		{
			name: "DeclaredCountScriptWithNoInstances",
			// `count` is assumed to have evaluated to zero, so the
			// evaluated root module has no concrete resource
			// instance.
			modules: []*tfjson.StateModule{{}},
			config: rootConfigWithScripts(
				configCountCoderScript("setup"),
			),
			selector: "coder_script.setup",
			expected: selectorResolution{
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
							configForEachCoderScript("setup"),
						},
					}},
				},
			}},
			moduleAddress: `module.development["primary"]`,
			selector:      "coder_script.setup",
			expected: selectorResolution{
				addresses:              nil,
				scriptResourceDeclared: true,
			},
		},
		{
			name:     "UndeclaredScriptWithNoInstances",
			config:   rootConfigWithScripts(configCoderScript("other")),
			selector: "coder_script.setup",
			expected: selectorResolution{
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
			config: rootConfigWithScripts(
				configCoderScript("setup"),
			),
			moduleAddress: "module.removed",
			selector:      "coder_script.setup",
			expected: selectorResolution{
				addresses:              nil,
				scriptResourceDeclared: false,
			},
		},
		{
			name: "NonManagedScriptDeclarationsAreIgnored",
			config: rootConfigWithScripts(
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
			expected: selectorResolution{
				addresses:              nil,
				scriptResourceDeclared: false,
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
			expected: selectorResolution{
				addresses: []string{`module.development["primary"].coder_script.setup`},
			},
		},
		{
			name:          "ScriptRelativeToNestedDeclaringModule",
			modules:       nestedDeclaringModules,
			moduleAddress: "module.outer.module.inner",
			selector:      "coder_script.setup",
			expected: selectorResolution{
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
			expected: selectorResolution{
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
			config:   rootConfigWithModuleCalls("bootstrap", "unrelated"),
			selector: "module.bootstrap",
			expected: selectorResolution{
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
			config:   rootConfigWithModuleCalls("bootstrap"),
			selector: "module.bootstrap",
			expected: selectorResolution{
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
			config:   rootConfigWithModuleCalls("开发"),
			selector: "module.开发",
			expected: selectorResolution{
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
			config:        nestedConfigWithModuleCalls("development", "bootstrap"),
			moduleAddress: `module.development["primary"]`,
			selector:      "module.bootstrap",
			expected: selectorResolution{
				addresses:          []string{`module.development["primary"].module.bootstrap.coder_script.setup`},
				moduleCallDeclared: true,
			},
		},
		{
			name:          "ModuleRelativeToNestedDeclaringModule",
			modules:       nestedDeclaringModules,
			config:        nestedConfigWithModuleCalls("outer", "inner", "bootstrap"),
			moduleAddress: "module.outer.module.inner",
			selector:      "module.bootstrap",
			expected: selectorResolution{
				addresses:          []string{"module.outer.module.inner.module.bootstrap.coder_script.install"},
				moduleCallDeclared: true,
			},
		},
		{
			name:     "DeclaredModuleWithNoInstances",
			config:   rootConfigWithModuleCalls("bootstrap"),
			selector: "module.bootstrap",
			expected: selectorResolution{
				addresses:          nil,
				moduleCallDeclared: true,
			},
		},
		{
			name: "DeclaredModuleWithNoScripts",
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{Address: "module.bootstrap"}},
			}},
			config:   rootConfigWithModuleCalls("bootstrap"),
			selector: "module.bootstrap",
			expected: selectorResolution{
				addresses:          nil,
				moduleCallDeclared: true,
			},
		},
		{
			name:     "UnknownModuleCall",
			config:   rootConfigWithModuleCalls("other"),
			selector: "module.bootstrap",
			expected: selectorResolution{
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
			config:   rootConfigWithModuleCalls("other"),
			selector: "module.bootstrap",
			expected: selectorResolution{
				addresses:          []string{"module.bootstrap.coder_script.setup"},
				moduleCallDeclared: false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			selector, err := parseSelector(test.selector)
			require.NoError(t, err)

			resolved, err := resolveSelectorForTest(test.modules, test.config, test.moduleAddress, selector)
			require.NoError(t, err)
			require.Equal(t, test.expected, resolved)
		})
	}
}

func TestResolveSelectorRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	t.Run("MalformedScriptAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseSelector("coder_script.setup[0]")
		require.NoError(t, err)

		_, err = resolveSelectorForTest([]*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{managedCoderScript("not-an-address", "setup")},
		}}, nil, "", selector)
		require.ErrorContains(t, err, `parse Terraform resource address "not-an-address"`)
	})

	t.Run("MalformedStateModuleAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseSelector("module.bootstrap")
		require.NoError(t, err)

		_, err = resolveSelectorForTest([]*tfjson.StateModule{{
			ChildModules: []*tfjson.StateModule{{Address: "not-an-address"}},
		}}, rootConfigWithModuleCalls("bootstrap"), "", selector)
		require.ErrorContains(t, err, `parse module address "not-an-address"`)
	})

	t.Run("MalformedDeclaringModuleAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveSelectorForTest(
			nil,
			rootConfigWithScripts(configCoderScript("setup")),
			"not-an-address",
			selector,
		)
		require.ErrorContains(t, err, `parse module address "not-an-address"`)
	})

	t.Run("InconsistentScriptAddress", func(t *testing.T) {
		t.Parallel()

		selector, err := parseSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveSelectorForTest([]*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{managedCoderScript("coder_script.other", "setup")},
		}}, nil, "", selector)
		require.ErrorContains(t, err, `Terraform resource address "coder_script.other" does not match its state fields`)
	})

	t.Run("MissingPlanConfiguration", func(t *testing.T) {
		t.Parallel()

		selector, err := parseSelector("module.bootstrap")
		require.NoError(t, err)

		_, err = resolveSelectorForTest(nil, nil, "", selector)
		require.ErrorContains(t, err, "terraform plan configuration is required to resolve a module selector")
	})

	t.Run("MissingPlanConfigurationForEmptyScript", func(t *testing.T) {
		t.Parallel()

		selector, err := parseSelector("coder_script.setup")
		require.NoError(t, err)

		_, err = resolveSelectorForTest(nil, nil, "", selector)
		require.ErrorContains(t, err, "cannot validate empty coder_script selector because Terraform plan configuration is unavailable")
	})
}

func rootConfigWithModuleCalls(moduleCalls ...string) *tfjson.Config {
	calls := make(map[string]*tfjson.ModuleCall, len(moduleCalls))
	for _, name := range moduleCalls {
		calls[name] = &tfjson.ModuleCall{Module: &tfjson.ConfigModule{}}
	}
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{ModuleCalls: calls}}
}

func nestedConfigWithModuleCalls(moduleCalls ...string) *tfjson.Config {
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

func rootConfigWithScripts(
	scripts ...*tfjson.ConfigResource,
) *tfjson.Config {
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{Resources: scripts}}
}

func configCoderScript(name string) *tfjson.ConfigResource {
	return &tfjson.ConfigResource{
		Address: "coder_script." + name,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
	}
}

func configCountCoderScript(name string) *tfjson.ConfigResource {
	script := configCoderScript(name)
	script.CountExpression = &tfjson.Expression{}
	return script
}

func configForEachCoderScript(name string) *tfjson.ConfigResource {
	script := configCoderScript(name)
	script.ForEachExpression = &tfjson.Expression{}
	return script
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

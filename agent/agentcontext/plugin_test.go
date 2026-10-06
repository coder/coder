package agentcontext_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// pluginJSON renders a plugin.json document with the 1.0.0 $schema
// plus the supplied fields.
func pluginJSON(t *testing.T, fields map[string]any) string {
	t.Helper()
	doc := map[string]any{"$schema": agentcontext.PluginSchemaV1}
	maps.Copy(doc, fields)
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return string(data)
}

// manyUnknownFields returns a valid manifest field set plus n unknown
// keys x000, x001, and so on.
func manyUnknownFields(n int) map[string]any {
	fields := map[string]any{"name": "noisy"}
	for i := range n {
		fields[fmt.Sprintf("x%03d", i)] = true
	}
	return fields
}

// mustWritePlugin creates a plugin root at dir with a minimal valid
// manifest for name.
func mustWritePlugin(t *testing.T, dir, name string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, "plugin.json"), pluginJSON(t, map[string]any{"name": name}))
}

func resolvePlugins(roots ...agentcontext.ScanRoot) agentcontext.Snapshot {
	r := &agentcontext.Resolver{}
	return r.ResolveContext(context.Background(), roots, agentcontext.ResolveOptions{PluginsEnabled: true})
}

func resourcesOfKind(snap agentcontext.Snapshot, kind agentcontext.ResourceKind) []agentcontext.Resource {
	var out []agentcontext.Resource
	for _, r := range snap.Resources {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

func TestPlugin_ManifestValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// manifest is written verbatim when set; otherwise fields is
		// rendered with the 1.0.0 $schema.
		manifest    string
		fields      map[string]any
		status      agentcontext.ResourceStatus
		errContains string
		check       func(t *testing.T, res agentcontext.Resource)
	}{
		{
			name:   "valid minimal",
			fields: map[string]any{"name": "minimal"},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Equal(t, "minimal", res.Name)
				require.Empty(t, res.PluginName)
				require.Empty(t, res.PluginVersion)
				require.Empty(t, res.Description)
				require.Empty(t, res.Error)
			},
		},
		{
			name: "all fields",
			fields: map[string]any{
				"name":        "full.plugin-1",
				"version":     "1.2.3",
				"description": "Does things",
				"author":      map[string]any{"name": "Ada", "email": "ada@example.com", "url": "https://example.com"},
				"homepage":    "https://example.com",
				"repository":  "https://github.com/example/plugin",
				"license":     "MIT",
				"keywords":    []string{"a", "b"},
				"extensions":  map[string]any{"com.example": map[string]any{"x": 1}},
			},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Equal(t, "full.plugin-1", res.Name)
				require.Equal(t, "1.2.3", res.PluginVersion)
				require.Equal(t, "Does things", res.Description)
				require.Empty(t, res.Error)
			},
		},
		{
			name:        "missing name",
			fields:      map[string]any{"version": "1.0.0"},
			status:      agentcontext.StatusInvalid,
			errContains: "name is required",
		},
		{
			name:        "name not a string",
			fields:      map[string]any{"name": 42},
			status:      agentcontext.StatusInvalid,
			errContains: "name must be a string",
		},
		{
			name:        "name uppercase",
			fields:      map[string]any{"name": "Bad"},
			status:      agentcontext.StatusInvalid,
			errContains: "invalid character",
		},
		{
			name:        "name leading dash",
			fields:      map[string]any{"name": "-bad"},
			status:      agentcontext.StatusInvalid,
			errContains: "start and end",
		},
		{
			name:        "name double dot",
			fields:      map[string]any{"name": "a..b"},
			status:      agentcontext.StatusInvalid,
			errContains: `must not contain ".."`,
		},
		{
			name:        "name too long",
			fields:      map[string]any{"name": strings.Repeat("a", 65)},
			status:      agentcontext.StatusInvalid,
			errContains: "maximum is 64",
		},
		{
			name:   "unknown top-level field tolerated",
			fields: map[string]any{"name": "tolerant", "hooks": []string{"x"}, "agents": 1},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Equal(t, `unknown field "agents" ignored; unknown field "hooks" ignored`, res.Error)
			},
		},
		{
			name:   "non-object extensions tolerated",
			fields: map[string]any{"name": "ext", "extensions": []string{"nope"}},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Contains(t, res.Error, "extensions must be an object")
			},
		},
		{
			name:        "unknown author key rejected",
			fields:      map[string]any{"name": "auth", "author": map[string]any{"name": "Ada", "twitter": "@ada"}},
			status:      agentcontext.StatusInvalid,
			errContains: `author contains unknown field "twitter"`,
		},
		{
			name:        "author field not a string",
			fields:      map[string]any{"name": "auth", "author": map[string]any{"name": 7}},
			status:      agentcontext.StatusInvalid,
			errContains: "author.name must be a string",
		},
		{
			name:        "author not an object",
			fields:      map[string]any{"name": "auth", "author": "Ada"},
			status:      agentcontext.StatusInvalid,
			errContains: "author must be an object",
		},
		{
			name:        "version not a string",
			fields:      map[string]any{"name": "ver", "version": 1},
			status:      agentcontext.StatusInvalid,
			errContains: "version must be a string",
		},
		{
			name:        "name null",
			fields:      map[string]any{"name": nil},
			status:      agentcontext.StatusInvalid,
			errContains: "name must be a string",
		},
		{
			name:        "version null",
			fields:      map[string]any{"name": "ver", "version": nil},
			status:      agentcontext.StatusInvalid,
			errContains: "version must be a string",
		},
		{
			name:        "license null",
			fields:      map[string]any{"name": "lic", "license": nil},
			status:      agentcontext.StatusInvalid,
			errContains: "license must be a string",
		},
		{
			name:        "author name null",
			fields:      map[string]any{"name": "auth", "author": map[string]any{"name": nil}},
			status:      agentcontext.StatusInvalid,
			errContains: "author.name must be a string",
		},
		{
			name:        "keyword null",
			fields:      map[string]any{"name": "kw", "keywords": []any{"ok", nil}},
			status:      agentcontext.StatusInvalid,
			errContains: "keywords must be an array of strings",
		},
		{
			name:        "keywords null",
			fields:      map[string]any{"name": "kw", "keywords": nil},
			status:      agentcontext.StatusInvalid,
			errContains: "keywords must be an array of strings",
		},
		{
			name:        "author null",
			fields:      map[string]any{"name": "auth", "author": nil},
			status:      agentcontext.StatusInvalid,
			errContains: "author must be an object",
		},
		{
			name:   "unknown fields listed up to a cap",
			fields: manyUnknownFields(300),
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Contains(t, res.Error, `unknown field "x000" ignored`)
				require.NotContains(t, res.Error, `"x010"`)
				require.True(t, strings.HasSuffix(res.Error, "; 290 more unknown fields ignored"), res.Error)
			},
		},
		{
			name:        "keywords not strings",
			fields:      map[string]any{"name": "kw", "keywords": []any{"ok", 2}},
			status:      agentcontext.StatusInvalid,
			errContains: "keywords must be an array of strings",
		},
		{
			name:        "missing $schema reported",
			manifest:    `{"name": "other-tool"}`,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json has no $schema",
		},
		{
			name:        "foreign $schema reported",
			manifest:    `{"$schema": "https://raw.githubusercontent.com/grafana/grafana/main/plugin.schema.json", "name": "grafana"}`,
			status:      agentcontext.StatusInvalid,
			errContains: "is not an Agent Plugins schema",
		},
		{
			name:        "non-string $schema reported",
			manifest:    `{"$schema": 1, "name": "x"}`,
			status:      agentcontext.StatusInvalid,
			errContains: "is not an Agent Plugins schema",
		},
		{
			name:        "unsupported schema version rejected",
			manifest:    `{"$schema": "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json", "name": "future"}`,
			status:      agentcontext.StatusInvalid,
			errContains: `unsupported $schema "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json"; this agent supports only "` + agentcontext.PluginSchemaV1 + `"`,
		},
		{
			name:        "invalid JSON reported",
			manifest:    `{"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "name": `,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json is not valid JSON",
		},
		{
			name:        "null manifest reported",
			manifest:    `null`,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json must be a JSON object",
		},
		{
			name:        "top-level array reported",
			manifest:    `["https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"]`,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json must be a JSON object",
		},
		{
			name: "version and description capped and sanitized",
			fields: map[string]any{
				"name":        "long",
				"version":     strings.Repeat("9", 100),
				"description": "zero\u200bwidth " + strings.Repeat("d", 2000),
			},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Len(t, res.PluginVersion, agentcontext.MaxPluginVersionRunes)
				require.Len(t, []rune(res.Description), agentcontext.MaxPluginDescriptionRunes)
				require.True(t, strings.HasPrefix(res.Description, "zerowidth "))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := testutil.TempDirResolved(t)
			pluginDir := filepath.Join(root, ".agents", "plugins", "candidate")
			manifest := tc.manifest
			if manifest == "" {
				manifest = pluginJSON(t, tc.fields)
			}
			mustWriteFile(t, filepath.Join(pluginDir, "plugin.json"), manifest)

			snap := resolvePlugins(agentcontext.ScanRoot{Path: root})
			plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
			require.Len(t, plugins, 1)
			res := plugins[0]
			require.Equal(t, pluginDir, res.Source)
			require.Equal(t, "plugin:"+pluginDir, res.ID)
			require.Equal(t, tc.status, res.Status, res.Error)
			require.NotEqual(t, [32]byte{}, res.ContentHash)
			if tc.errContains != "" {
				require.Contains(t, res.Error, tc.errContains)
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func TestPlugin_OversizeManifest(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "big")
	mustWriteFile(t, filepath.Join(pluginDir, "plugin.json"), pluginJSON(t, map[string]any{
		"name":        "big",
		"description": strings.Repeat("x", 200),
	}))
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "hidden", "never loaded")

	r := &agentcontext.Resolver{MaxResourceBytes: 128}
	snap := r.ResolveContext(context.Background(), []agentcontext.ScanRoot{{Path: root}}, agentcontext.ResolveOptions{PluginsEnabled: true})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 1)
	require.Equal(t, agentcontext.StatusOversize, plugins[0].Status)
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill), "no components load from a rejected plugin")
}

func TestPlugin_DiscoveredFromContainers(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	first := filepath.Join(root, "plugins", "alpha")
	second := filepath.Join(root, ".agents", "plugins", "beta")
	mustWritePlugin(t, first, "alpha")
	mustWriteSkill(t, filepath.Join(first, "skills"), "deploy", "Deploy things")
	require.NoError(t, os.MkdirAll(filepath.Join(first, "skills", "empty"), 0o700))
	mustWritePlugin(t, second, "beta")
	mustWriteSkill(t, filepath.Join(second, "skills"), "review", "Review things")
	// A plain skill next to the containers is still a plain skill.
	mustWriteSkill(t, filepath.Join(root, "skills"), "plain", "Plain skill")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root, UserSource: root})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 2)
	for _, p := range plugins {
		require.Equal(t, agentcontext.StatusOK, p.Status, p.Error)
		require.Equal(t, root, p.SourcePath)
	}

	deploy := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(first, "skills", "deploy"))
	require.Equal(t, agentcontext.StatusOK, deploy.Status, deploy.Error)
	require.Equal(t, "alpha", deploy.PluginName)
	require.Equal(t, "deploy", deploy.Name)
	require.Equal(t, "skill:"+filepath.Join(first, "skills", "deploy"), deploy.ID)

	review := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(second, "skills", "review"))
	require.Equal(t, "beta", review.PluginName)

	plain := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "plain"))
	require.Empty(t, plain.PluginName)
	// skills/empty has no SKILL.md and is not a skill.
	require.Len(t, resourcesOfKind(snap, agentcontext.KindSkill), 3)
}

func TestPlugin_RootIsPlugin(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWritePlugin(t, root, "solo")
	mustWriteSkill(t, filepath.Join(root, "skills"), "only", "Only skill")
	// The plugin's own repository keeps its instruction files and
	// other skill containers.
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "project rules")
	mustWriteSkill(t, filepath.Join(root, ".agents", "skills"), "extra", "Plain skill")
	// Nested plugin containers are not scanned inside a plugin root.
	mustWritePlugin(t, filepath.Join(root, "plugins", "nested"), "nested")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	var kinds []string
	for _, res := range snap.Resources {
		kinds = append(kinds, res.Kind.String()+":"+res.Source)
	}
	require.ElementsMatch(t, []string{
		"plugin:" + root,
		"skill:" + filepath.Join(root, "skills", "only"),
		"instruction_file:" + filepath.Join(root, "AGENTS.md"),
		"skill:" + filepath.Join(root, ".agents", "skills", "extra"),
	}, kinds)
	extra := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, ".agents", "skills", "extra"))
	require.Empty(t, extra.PluginName)
	skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "only"))
	require.Equal(t, "solo", skill.PluginName)
}

// A plugin.json at a scan root that is not an Agent Plugins manifest, or
// cannot be read as one, may belong to another tool: no plugin resource
// is emitted and the root's regular discovery, including skills/, is
// untouched.
func TestPlugin_RootWithUnusableManifestIgnored(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// write creates the root's plugin.json.
		write            func(t *testing.T, root string)
		maxResourceBytes uint64
		symlinks         bool
		unreadable       bool
	}{
		{
			name: "missing $schema",
			write: func(t *testing.T, root string) {
				mustWriteFile(t, filepath.Join(root, "plugin.json"), `{"name": "other-tool"}`)
			},
		},
		{
			name:  "invalid JSON",
			write: func(t *testing.T, root string) { mustWriteFile(t, filepath.Join(root, "plugin.json"), `{"name": `) },
		},
		{
			name: "not a regular file",
			write: func(t *testing.T, root string) {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "plugin.json"), 0o700))
			},
		},
		{
			name: "oversize",
			write: func(t *testing.T, root string) {
				mustWriteFile(t, filepath.Join(root, "plugin.json"), pluginJSON(t, map[string]any{
					"name":        "big",
					"description": strings.Repeat("x", 200),
				}))
			},
			maxResourceBytes: 128,
		},
		{
			name: "symlink escapes root",
			write: func(t *testing.T, root string) {
				outside := testutil.TempDirResolved(t)
				mustWriteFile(t, filepath.Join(outside, "plugin.json"), pluginJSON(t, map[string]any{"name": "outside"}))
				require.NoError(t, os.Symlink(filepath.Join(outside, "plugin.json"), filepath.Join(root, "plugin.json")))
			},
			symlinks: true,
		},
		{
			name: "unreadable",
			write: func(t *testing.T, root string) {
				path := filepath.Join(root, "plugin.json")
				mustWriteFile(t, path, pluginJSON(t, map[string]any{"name": "locked"}))
				require.NoError(t, os.Chmod(path, 0o000))
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			},
			unreadable: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if (tc.symlinks || tc.unreadable) && runtime.GOOS == "windows" {
				t.Skip("symlinks and file mode 0o000 are not reliable on Windows runners")
			}
			if tc.unreadable && os.Geteuid() == 0 {
				t.Skip("root bypasses file mode permissions")
			}
			root := testutil.TempDirResolved(t)
			tc.write(t, root)
			mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "still here")
			mustWriteSkill(t, filepath.Join(root, "skills"), "plain", "Plain skill")

			r := &agentcontext.Resolver{MaxResourceBytes: tc.maxResourceBytes}
			snap := r.ResolveContext(context.Background(), []agentcontext.ScanRoot{{Path: root}}, agentcontext.ResolveOptions{PluginsEnabled: true})

			require.Empty(t, resourcesOfKind(snap, agentcontext.KindPlugin))
			instr := findResource(t, snap.Resources, agentcontext.KindInstructionFile, filepath.Join(root, "AGENTS.md"))
			require.Equal(t, agentcontext.StatusOK, instr.Status)
			skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "plain"))
			require.Empty(t, skill.PluginName)
		})
	}
}

// Another tool may own an entry of a generic plugins/ container, so a
// plugin.json there whose $schema is absent or foreign is ignored. The
// same file in .agents/plugins/ is reported, as is malformed JSON in
// either container.
func TestPlugin_ForeignManifestInContainer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		rel      string
		manifest string
		reported bool
	}{
		{"plugins missing $schema ignored", "plugins", `{"name": "other-tool"}`, false},
		{"plugins foreign $schema ignored", "plugins", `{"$schema": "https://example.com/plugin.schema.json", "name": "x"}`, false},
		{"plugins invalid JSON reported", "plugins", `{"name": `, true},
		{".agents/plugins missing $schema reported", filepath.Join(".agents", "plugins"), `{"name": "other-tool"}`, true},
		{".agents/plugins foreign $schema reported", filepath.Join(".agents", "plugins"), `{"$schema": "https://example.com/plugin.schema.json", "name": "x"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := testutil.TempDirResolved(t)
			pluginDir := filepath.Join(root, tc.rel, "candidate")
			mustWriteFile(t, filepath.Join(pluginDir, "plugin.json"), tc.manifest)
			mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "hidden", "Never loaded")

			snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

			require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
			plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
			if !tc.reported {
				require.Empty(t, plugins)
				return
			}
			require.Len(t, plugins, 1)
			require.Equal(t, pluginDir, plugins[0].Source)
			require.Equal(t, agentcontext.StatusInvalid, plugins[0].Status)
		})
	}
}

// A scan root whose Agent Plugins manifest is rejected keeps its
// instruction files, but its skills/ container belongs to the plugin and
// is not loaded.
func TestPlugin_RootWithInvalidManifestSkipsSkills(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(root, "plugin.json"), pluginJSON(t, map[string]any{"name": "Not Valid"}))
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "still here")
	mustWriteSkill(t, filepath.Join(root, "skills"), "owned", "Plugin skill")
	mustWriteSkill(t, filepath.Join(root, ".agents", "skills"), "plain", "Plain skill")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, root)
	require.Equal(t, agentcontext.StatusInvalid, plugin.Status)
	instr := findResource(t, snap.Resources, agentcontext.KindInstructionFile, filepath.Join(root, "AGENTS.md"))
	require.Equal(t, agentcontext.StatusOK, instr.Status)
	skills := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skills, 1)
	require.Equal(t, filepath.Join(root, ".agents", "skills", "plain"), skills[0].Source)
}

func TestPlugin_RootIsPluginAndContainerChildEmittedOnce(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "twice")
	mustWritePlugin(t, pluginDir, "twice")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "one", "One")
	mustWriteFile(t, filepath.Join(pluginDir, "AGENTS.md"), "plugin rules")

	// The plugin directory is both a container child of root and an
	// explicit scan root. The result must not depend on root order.
	pluginRoot := agentcontext.ScanRoot{Path: pluginDir, UserSource: pluginDir}
	containerRoot := agentcontext.ScanRoot{Path: root}
	for name, roots := range map[string][]agentcontext.ScanRoot{
		"plugin root first":    {pluginRoot, containerRoot},
		"container root first": {containerRoot, pluginRoot},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := resolvePlugins(roots...)

			plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
			require.Len(t, plugins, 1)
			require.Equal(t, agentcontext.StatusOK, plugins[0].Status, plugins[0].Error)
			skills := resourcesOfKind(snap, agentcontext.KindSkill)
			require.Len(t, skills, 1)
			require.Equal(t, "twice", skills[0].PluginName)
			instr := findResource(t, snap.Resources, agentcontext.KindInstructionFile, filepath.Join(pluginDir, "AGENTS.md"))
			require.Equal(t, agentcontext.StatusOK, instr.Status)
		})
	}
}

func TestPlugin_SymlinkedManifestEscapeRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	outside := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(outside, "plugin.json"), pluginJSON(t, map[string]any{"name": "outside"}))
	pluginDir := filepath.Join(root, "plugins", "linked")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "plugin.json"), filepath.Join(pluginDir, "plugin.json")))
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "hidden", "never loaded")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusInvalid, plugin.Status)
	require.Contains(t, plugin.Error, "escapes")
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
}

func TestPlugin_SymlinkedManifestInsideRootAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "linked")
	mustWriteFile(t, filepath.Join(pluginDir, "manifests", "real.json"), pluginJSON(t, map[string]any{"name": "linked"}))
	require.NoError(t, os.Symlink(filepath.Join(pluginDir, "manifests", "real.json"), filepath.Join(pluginDir, "plugin.json")))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status, plugin.Error)
	require.Equal(t, "linked", plugin.Name)
}

func TestPlugin_SymlinkedSkillEscapeRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	// The target lives inside the scan root but outside the plugin
	// root, so the plugin-root boundary must reject it.
	mustWriteSkill(t, filepath.Join(root, "skills"), "escape", "Outside the plugin")
	skillDir := filepath.Join(pluginDir, "skills", "escape")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "skills", "escape", "SKILL.md"), filepath.Join(skillDir, "SKILL.md")))
	// A symlink resolving inside the plugin root is fine.
	mustWriteFile(t, filepath.Join(pluginDir, "docs", "inner.md"), "---\nname: inner\ndescription: Inner\n---\nbody")
	innerDir := filepath.Join(pluginDir, "skills", "inner")
	require.NoError(t, os.MkdirAll(innerDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(pluginDir, "docs", "inner.md"), filepath.Join(innerDir, "SKILL.md")))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status, plugin.Error)

	escaped := findResource(t, snap.Resources, agentcontext.KindSkill, skillDir)
	require.Equal(t, agentcontext.StatusInvalid, escaped.Status)
	require.Contains(t, escaped.Error, "escapes")
	require.Equal(t, "p", escaped.PluginName)

	inner := findResource(t, snap.Resources, agentcontext.KindSkill, innerDir)
	require.Equal(t, agentcontext.StatusOK, inner.Status, inner.Error)
	require.Equal(t, "p", inner.PluginName)

	// The plain skill outside the plugin is discovered on its own.
	plain := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "escape"))
	require.Equal(t, agentcontext.StatusOK, plain.Status)
	require.Empty(t, plain.PluginName)
}

func TestPlugin_SymlinkedPluginDirSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	target := filepath.Join(root, "elsewhere", "real")
	mustWritePlugin(t, target, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plugins"), 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(root, "plugins", "link")))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})
	require.Empty(t, snap.Resources)
}

func TestPlugin_SymlinkedSkillsContainer(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}

	t.Run("InsideRootLoaded", func(t *testing.T) {
		t.Parallel()
		root := testutil.TempDirResolved(t)
		pluginDir := filepath.Join(root, "plugins", "p")
		mustWritePlugin(t, pluginDir, "p")
		mustWriteSkill(t, filepath.Join(pluginDir, "real-skills"), "one", "One")
		require.NoError(t, os.Symlink(filepath.Join(pluginDir, "real-skills"), filepath.Join(pluginDir, "skills")))

		snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

		plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
		require.Equal(t, agentcontext.StatusOK, plugin.Status)
		require.Empty(t, plugin.Error)
		skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(pluginDir, "skills", "one"))
		require.Equal(t, agentcontext.StatusOK, skill.Status, skill.Error)
		require.Equal(t, "p", skill.PluginName)
		require.Len(t, resourcesOfKind(snap, agentcontext.KindSkill), 1)
	})

	t.Run("EscapingRootDisablesSkills", func(t *testing.T) {
		t.Parallel()
		root := testutil.TempDirResolved(t)
		pluginDir := filepath.Join(root, "plugins", "p")
		mustWritePlugin(t, pluginDir, "p")
		// The target is inside the scan root but outside the plugin root.
		outside := filepath.Join(root, "shared-skills")
		mustWriteSkill(t, outside, "one", "One")
		require.NoError(t, os.Symlink(outside, filepath.Join(pluginDir, "skills")))

		snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

		plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
		require.Equal(t, agentcontext.StatusOK, plugin.Status)
		require.Contains(t, plugin.Error, "escapes plugin root; skills not loaded")
		require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
	})

	t.Run("MissingTargetWarns", func(t *testing.T) {
		t.Parallel()
		root := testutil.TempDirResolved(t)
		pluginDir := filepath.Join(root, "plugins", "p")
		mustWritePlugin(t, pluginDir, "p")
		require.NoError(t, os.Symlink(filepath.Join(pluginDir, "missing"), filepath.Join(pluginDir, "skills")))

		snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

		plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
		require.Equal(t, agentcontext.StatusOK, plugin.Status)
		require.Contains(t, plugin.Error, "cannot resolve skills symlink")
		require.Contains(t, plugin.Error, "; skills not loaded")
		require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
	})

	t.Run("DanglingSkillDirIgnored", func(t *testing.T) {
		t.Parallel()
		root := testutil.TempDirResolved(t)
		pluginDir := filepath.Join(root, "plugins", "p")
		mustWritePlugin(t, pluginDir, "p")
		require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "skills"), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(pluginDir, "missing"), filepath.Join(pluginDir, "skills", "s")))
		mustWriteFile(t, filepath.Join(pluginDir, "file-target"), "not a directory")
		require.NoError(t, os.Symlink(filepath.Join(pluginDir, "file-target"), filepath.Join(pluginDir, "skills", "f")))

		snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

		plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
		require.Equal(t, agentcontext.StatusOK, plugin.Status)
		require.Empty(t, plugin.Error)
		require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
	})
}

func TestPlugin_UnreadableSkillsDirWarns(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not block reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "one", "One")
	skills := filepath.Join(pluginDir, "skills")
	require.NoError(t, os.Chmod(skills, 0o000))
	t.Cleanup(func() { _ = os.Chmod(skills, 0o755) })

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status)
	require.Contains(t, plugin.Error, "skills directory unreadable")
	require.Contains(t, plugin.Error, "; skills not loaded")
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
}

// coderd rejects a push holding a Source longer than
// workspacesdk.MaxContextSourceBytes, so such plugins and plugin skills
// are skipped.
func TestPlugin_LongSourcesSkipped(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("paths this long need long path support on Windows")
	}
	const maxSource = workspacesdk.MaxContextSourceBytes
	// deep is just long enough that a directory of longPluginNameLen
	// bytes in .agents/plugins/ exceeds maxSource, while
	// .agents/plugins/p/skills/ok stays under it.
	const longPluginNameLen = 200
	deep := testutil.TempDirResolved(t)
	for len(deep)+len("/.agents/plugins/")+longPluginNameLen <= maxSource {
		deep = filepath.Join(deep, strings.Repeat("d", 100))
	}
	container := filepath.Join(deep, ".agents", "plugins")
	pluginDir := filepath.Join(container, "p")
	mustWritePlugin(t, pluginDir, "p")
	skills := filepath.Join(pluginDir, "skills")
	mustWriteSkill(t, skills, "ok", "OK")
	require.LessOrEqual(t, len(filepath.Join(skills, "ok")), maxSource)
	longSkill := filepath.Join(skills, strings.Repeat("s", maxSource-len(skills)))
	require.Greater(t, len(longSkill), maxSource)
	longPlugin := filepath.Join(container, strings.Repeat("q", longPluginNameLen))
	require.Greater(t, len(longPlugin), maxSource)
	for _, dir := range []string{longSkill, longPlugin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			// macOS limits paths to 1024 bytes.
			t.Skipf("filesystem rejects the long test path: %v", err)
		}
	}
	mustWriteFile(t, filepath.Join(longSkill, "SKILL.md"), "---\nname: x\ndescription: x\n---\n")
	mustWritePlugin(t, longPlugin, "q")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: deep})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 1)
	require.Equal(t, pluginDir, plugins[0].Source)
	require.Equal(t, agentcontext.StatusOK, plugins[0].Status)
	require.Contains(t, plugins[0].Error, fmt.Sprintf("skill directories skipped because their paths are longer than %d bytes: 1; shorten them to load them", maxSource))
	skillRows := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skillRows, 1)
	require.Equal(t, filepath.Join(skills, "ok"), skillRows[0].Source)
}

func TestPlugin_SymlinkedSkillDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "skills"), 0o755))
	// inner resolves inside the plugin root.
	mustWriteSkill(t, filepath.Join(pluginDir, "shared"), "inner", "Inner")
	innerLink := filepath.Join(pluginDir, "skills", "inner")
	require.NoError(t, os.Symlink(filepath.Join(pluginDir, "shared", "inner"), innerLink))
	// escape resolves inside the scan root but outside the plugin root.
	mustWriteSkill(t, filepath.Join(root, "elsewhere"), "escape", "Escape")
	escapeLink := filepath.Join(pluginDir, "skills", "escape")
	require.NoError(t, os.Symlink(filepath.Join(root, "elsewhere", "escape"), escapeLink))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	inner := findResource(t, snap.Resources, agentcontext.KindSkill, innerLink)
	require.Equal(t, agentcontext.StatusOK, inner.Status, inner.Error)
	require.Equal(t, "inner", inner.Name)
	require.Equal(t, "p", inner.PluginName)

	escaped := findResource(t, snap.Resources, agentcontext.KindSkill, escapeLink)
	require.Equal(t, agentcontext.StatusInvalid, escaped.Status)
	require.Contains(t, escaped.Error, "escapes plugin root")
	require.Empty(t, escaped.Payload)
	require.Equal(t, "p", escaped.PluginName)
	require.Len(t, resourcesOfKind(snap, agentcontext.KindSkill), 2)

	dirs := agentcontext.WatchDirs([]agentcontext.ScanRoot{{Path: root}}, agentcontext.ResolveOptions{PluginsEnabled: true})
	require.Contains(t, dirs, innerLink)
	require.NotContains(t, dirs, escapeLink)
}

func TestPlugin_SkillsFileDisablesSkills(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteFile(t, filepath.Join(pluginDir, "skills"), "not a directory")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status)
	require.Contains(t, plugin.Error, "skills is not a directory")
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
}

func TestPlugin_NonRegularManifest(t *testing.T) {
	t.Parallel()

	t.Run("InContainerReported", func(t *testing.T) {
		t.Parallel()
		root := testutil.TempDirResolved(t)
		pluginDir := filepath.Join(root, "plugins", "p")
		require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "plugin.json"), 0o700))

		snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

		plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
		require.Equal(t, agentcontext.StatusInvalid, plugin.Status)
		require.Equal(t, "plugin.json is not a regular file", plugin.Error)
	})
}

func TestPlugin_SkillNameMismatchInvalid(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	skillDir := filepath.Join(pluginDir, "skills", "dir-name")
	mustWriteFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: other-name\ndescription: x\n---\nbody")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	skill := findResource(t, snap.Resources, agentcontext.KindSkill, skillDir)
	require.Equal(t, agentcontext.StatusInvalid, skill.Status)
	require.Contains(t, skill.Error, "does not match directory")
	require.Equal(t, "p", skill.PluginName)
}

func TestPlugin_DuplicateNameAcrossRootsFirstWins(t *testing.T) {
	t.Parallel()
	first := testutil.TempDirResolved(t)
	second := testutil.TempDirResolved(t)
	firstPlugin := filepath.Join(first, "plugins", "dup")
	secondPlugin := filepath.Join(second, "plugins", "dup-copy")
	mustWritePlugin(t, firstPlugin, "dup")
	mustWriteSkill(t, filepath.Join(firstPlugin, "skills"), "first", "First")
	mustWritePlugin(t, secondPlugin, "dup")
	mustWriteSkill(t, filepath.Join(secondPlugin, "skills"), "second", "Second")

	snap := resolvePlugins(
		agentcontext.ScanRoot{Path: first},
		agentcontext.ScanRoot{Path: second},
	)

	winner := findResource(t, snap.Resources, agentcontext.KindPlugin, firstPlugin)
	require.Equal(t, agentcontext.StatusOK, winner.Status)
	loser := findResource(t, snap.Resources, agentcontext.KindPlugin, secondPlugin)
	require.Equal(t, agentcontext.StatusInvalid, loser.Status)
	require.Equal(t, `plugin name "dup" is already used by `+firstPlugin+`; rename one of the plugins`, loser.Error)

	skills := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skills, 1)
	require.Equal(t, filepath.Join(firstPlugin, "skills", "first"), skills[0].Source)
}

func TestPlugin_SkillCoexistsWithPlainSkillOfSameName(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "shared", "From plugin")
	mustWriteSkill(t, filepath.Join(root, "skills"), "shared", "Plain")
	other := filepath.Join(root, "plugins", "q")
	mustWritePlugin(t, other, "q")
	mustWriteSkill(t, filepath.Join(other, "skills"), "shared", "From q")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	skills := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skills, 3)
	var owners []string
	for _, s := range skills {
		require.Equal(t, agentcontext.StatusOK, s.Status, s.Error)
		require.Equal(t, "shared", s.Name)
		owners = append(owners, s.PluginName)
	}
	require.ElementsMatch(t, []string{"", "p", "q"}, owners)
}

func TestPlugin_DisabledEmitsNothingPluginRelated(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "hidden", "Hidden")
	// The root itself is a plugin too; with plugins disabled it is
	// scanned exactly like any other directory.
	mustWritePlugin(t, root, "root")
	mustWriteSkill(t, filepath.Join(root, "skills"), "plain", "Plain")
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "rules")

	r := &agentcontext.Resolver{}
	snap := r.Resolve([]agentcontext.ScanRoot{{Path: root}})

	var kinds []string
	for _, res := range snap.Resources {
		require.Empty(t, res.PluginName)
		kinds = append(kinds, res.Kind.String()+":"+res.Source)
	}
	require.ElementsMatch(t, []string{
		"instruction_file:" + filepath.Join(root, "AGENTS.md"),
		"skill:" + filepath.Join(root, "skills", "plain"),
	}, kinds)
}

func TestPlugin_IncludedInAggregateHash(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")

	before := resolvePlugins(agentcontext.ScanRoot{Path: root})
	mustWriteFile(t, filepath.Join(pluginDir, "plugin.json"), pluginJSON(t, map[string]any{"name": "p", "version": "2"}))
	after := resolvePlugins(agentcontext.ScanRoot{Path: root})

	require.NotEqual(t, before.AggregateHash, after.AggregateHash)
}

func TestManager_SetPluginsEnabledTriggersResolve(t *testing.T) {
	t.Parallel()
	wd := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(wd, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "one", "One")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return wd },
	})

	snap := m.Snapshot()
	require.Equal(t, uint64(1), snap.Version)
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindPlugin))

	m.SetPluginsEnabled(true)
	snap = m.Snapshot()
	require.Equal(t, uint64(2), snap.Version)
	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status, plugin.Error)
	skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(pluginDir, "skills", "one"))
	require.Equal(t, "p", skill.PluginName)

	// An unchanged value does not resolve again.
	m.SetPluginsEnabled(true)
	require.Equal(t, uint64(2), m.Snapshot().Version)

	m.SetPluginsEnabled(false)
	snap = m.Snapshot()
	require.Equal(t, uint64(3), snap.Version)
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindPlugin))
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
}

func TestManager_SetPluginsEnabledBeforeReady(t *testing.T) {
	t.Parallel()
	wd := testutil.TempDirResolved(t)
	mustWritePlugin(t, filepath.Join(wd, "plugins", "p"), "p")

	m := newPendingTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return wd },
	})
	m.SetPluginsEnabled(true)
	require.Equal(t, uint64(0), m.Snapshot().Version, "gated manager must not resolve")

	m.SetReady()
	snap := m.Snapshot()
	require.Equal(t, uint64(1), snap.Version)
	require.Len(t, resourcesOfKind(snap, agentcontext.KindPlugin), 1)
}

func TestManager_SetPluginsEnabledWithRunLoop(t *testing.T) {
	t.Parallel()
	wd := testutil.TempDirResolved(t)
	mustWritePlugin(t, filepath.Join(wd, "plugins", "p"), "p")

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return wd },
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()
	<-agentcontext.ManagerStarted(m)

	ch, unsub := m.SubscribeChanges()
	defer unsub()

	m.SetPluginsEnabled(true)
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("expected a broadcast after SetPluginsEnabled")
	}
	require.Len(t, resourcesOfKind(m.Snapshot(), agentcontext.KindPlugin), 1)
}

// A path that is not valid UTF-8 cannot be sent as a resource Source, so
// a plugin directory or plugin skill directory with such a name is
// skipped.
func TestPlugin_InvalidUTF8NamesSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows stores names as UTF-16 and turns an invalid byte into U+FFFD, so the name reads back as valid UTF-8")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	container := filepath.Join(root, ".agents", "plugins")
	if err := os.MkdirAll(filepath.Join(container, "bad\xffplugin"), 0o755); err != nil {
		t.Skipf("filesystem rejects invalid UTF-8 names: %v", err)
	}
	mustWritePlugin(t, filepath.Join(container, "bad\xffplugin"), "bad")
	good := filepath.Join(container, "good")
	mustWritePlugin(t, good, "good")
	mustWriteSkill(t, filepath.Join(good, "skills"), "ok", "OK")
	require.NoError(t, os.MkdirAll(filepath.Join(good, "skills", "bad\xffskill"), 0o755))
	mustWriteFile(t, filepath.Join(good, "skills", "bad\xffskill", "SKILL.md"), "---\nname: x\ndescription: x\n---\n")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 1)
	require.Equal(t, good, plugins[0].Source)
	require.Equal(t, agentcontext.StatusOK, plugins[0].Status)
	require.Contains(t, plugins[0].Error, "skill directories skipped because their names are not valid UTF-8: 1; rename them to load them")
	skills := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skills, 1)
	require.Equal(t, filepath.Join(good, "skills", "ok"), skills[0].Source)
}

func TestPlugin_SymlinkedContainerSkipped(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	for name, link := range map[string][]string{
		"plugins":         {"plugins"},
		".agents/plugins": {".agents", "plugins"},
		".agents":         {".agents"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testutil.TempDirResolved(t)
			outside := testutil.TempDirResolved(t)
			target := outside
			if link[len(link)-1] == ".agents" {
				target = filepath.Join(outside, "dot-agents")
				mustWritePlugin(t, filepath.Join(target, "plugins", "evil"), "evil")
				mustWriteSkill(t, filepath.Join(target, "plugins", "evil", "skills"), "steal", "Outside")
			} else {
				mustWritePlugin(t, filepath.Join(target, "evil"), "evil")
				mustWriteSkill(t, filepath.Join(target, "evil", "skills"), "steal", "Outside")
			}
			linkPath := filepath.Join(append([]string{root}, link...)...)
			require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o755))
			require.NoError(t, os.Symlink(target, linkPath))

			snap := resolvePlugins(agentcontext.ScanRoot{Path: root})
			require.Empty(t, snap.Resources)
		})
	}
}

// When the resource count cap is hit, plugin rows survive ahead of
// their skills, because coderd stores a plugin skill only alongside an
// OK plugin with that name.
func TestPlugin_CountCapKeepsPluginBeforeSkills(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "a", "A")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "b", "B")

	r := &agentcontext.Resolver{MaxResources: 2}
	snap := r.ResolveContext(context.Background(), []agentcontext.ScanRoot{{Path: root}}, agentcontext.ResolveOptions{PluginsEnabled: true})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status, plugin.Error)
	a := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(pluginDir, "skills", "a"))
	require.Equal(t, agentcontext.StatusOK, a.Status, a.Error)
	b := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(pluginDir, "skills", "b"))
	require.Equal(t, agentcontext.StatusExcluded, b.Status)
}

// coderd rejects a push holding a skill whose plugin is not OK, so the
// skills of a plugin the count cap excludes are dropped.
func TestPlugin_CountCapDropsSkillsOfExcludedPlugin(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	p1 := filepath.Join(root, "plugins", "p1")
	p2 := filepath.Join(root, "plugins", "p2")
	mustWritePlugin(t, p1, "p1")
	mustWritePlugin(t, p2, "p2")
	mustWriteSkill(t, filepath.Join(p1, "skills"), "a", "A")
	mustWriteSkill(t, filepath.Join(p2, "skills"), "b", "B")

	r := &agentcontext.Resolver{MaxResources: 1}
	snap := r.ResolveContext(context.Background(), []agentcontext.ScanRoot{{Path: root}}, agentcontext.ResolveOptions{PluginsEnabled: true})

	require.Equal(t, agentcontext.StatusOK, findResource(t, snap.Resources, agentcontext.KindPlugin, p1).Status)
	require.Equal(t, agentcontext.StatusExcluded, findResource(t, snap.Resources, agentcontext.KindPlugin, p2).Status)
	skills := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skills, 1)
	require.Equal(t, filepath.Join(p1, "skills", "a"), skills[0].Source)
	require.Equal(t, agentcontext.StatusExcluded, skills[0].Status)
}

// coderd rejects a push that repeats a Source, so a skill directory
// that is also a scan-root plugin yields one row: the one from the
// earlier scan root. A plugin skipped this way claims no name, loads no
// skills, and its plugin containers are not scanned.
func TestPlugin_SkillDirAlsoScanRootPlugin(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	skills := filepath.Join(root, ".agents", "skills")
	mustWriteSkill(t, skills, "foo", "Foo")
	foo := filepath.Join(skills, "foo")
	mustWritePlugin(t, foo, "foo")
	mustWriteSkill(t, filepath.Join(foo, "skills"), "deploy", "Deploy")
	deploy := filepath.Join(foo, "skills", "deploy")
	nested := filepath.Join(foo, ".agents", "plugins", "nested")
	mustWritePlugin(t, nested, "nested")
	other := testutil.TempDirResolved(t)
	bar := filepath.Join(other, ".agents", "plugins", "bar")
	mustWritePlugin(t, bar, "foo")

	for name, tc := range map[string]struct {
		roots []agentcontext.ScanRoot
		kind  agentcontext.ResourceKind
		// barStatus is the status of the second plugin named foo.
		barStatus agentcontext.ResourceStatus
	}{
		"SkillRootFirst":  {roots: []agentcontext.ScanRoot{{Path: root}, {Path: foo}, {Path: other}}, kind: agentcontext.KindSkill, barStatus: agentcontext.StatusOK},
		"PluginRootFirst": {roots: []agentcontext.ScanRoot{{Path: foo}, {Path: root}, {Path: other}}, kind: agentcontext.KindPlugin, barStatus: agentcontext.StatusInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := resolvePlugins(tc.roots...)
			var kinds []agentcontext.ResourceKind
			for _, res := range snap.Resources {
				if res.Source == foo {
					kinds = append(kinds, res.Kind)
				}
			}
			require.Equal(t, []agentcontext.ResourceKind{tc.kind}, kinds)
			require.Equal(t, tc.barStatus, findResource(t, snap.Resources, agentcontext.KindPlugin, bar).Status)
			require.False(t, slices.ContainsFunc(snap.Resources, func(res agentcontext.Resource) bool {
				return res.Source == nested
			}), "a plugin root's plugin containers are not scanned")
			if tc.kind == agentcontext.KindPlugin {
				skill := findResource(t, snap.Resources, agentcontext.KindSkill, deploy)
				require.Equal(t, agentcontext.StatusOK, skill.Status)
				require.Equal(t, "foo", skill.PluginName)
			} else {
				require.False(t, slices.ContainsFunc(snap.Resources, func(res agentcontext.Resource) bool {
					return res.Source == deploy
				}), "a skipped plugin loads no skills")
			}
		})
	}
}

// The plugin's skills load too.
func TestPlugin_ValidPluginReplacesInvalidSkillAtSameDir(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	foo := filepath.Join(root, ".agents", "skills", "foo")
	mustWriteFile(t, filepath.Join(foo, "SKILL.md"), "---\nname: not-foo\ndescription: Foo\n---\nbody")
	mustWritePlugin(t, foo, "foo")
	mustWriteSkill(t, filepath.Join(foo, "skills"), "deploy", "Deploy")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root}, agentcontext.ScanRoot{Path: foo})
	var atFoo []agentcontext.Resource
	for _, res := range snap.Resources {
		if res.Source == foo {
			atFoo = append(atFoo, res)
		}
	}
	require.Len(t, atFoo, 1)
	require.Equal(t, agentcontext.KindPlugin, atFoo[0].Kind)
	require.Equal(t, agentcontext.StatusOK, atFoo[0].Status)
	deploy := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(foo, "skills", "deploy"))
	require.Equal(t, agentcontext.StatusOK, deploy.Status)
	require.Equal(t, "foo", deploy.PluginName)
}

// A rejected scan-root plugin at an OK skill's directory leaves the skill
// row, which carries the plugin's error as a warning, in either root order.
func TestPlugin_RejectedPluginAtSkillDirWarnsOnSkill(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	foo := filepath.Join(root, ".agents", "skills", "foo")
	mustWriteSkill(t, filepath.Dir(foo), "foo", "Foo")
	mustWritePlugin(t, foo, "Not Valid")

	for name, roots := range map[string][]agentcontext.ScanRoot{
		"SkillRootFirst":  {{Path: root}, {Path: foo}},
		"PluginRootFirst": {{Path: foo}, {Path: root}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := resolvePlugins(roots...)
			var atFoo []agentcontext.Resource
			for _, res := range snap.Resources {
				if res.Source == foo {
					atFoo = append(atFoo, res)
				}
			}
			require.Len(t, atFoo, 1)
			require.Equal(t, agentcontext.KindSkill, atFoo[0].Kind)
			require.Equal(t, agentcontext.StatusOK, atFoo[0].Status)
			require.Contains(t, atFoo[0].Error, "plugin.json in this directory was rejected: ")
		})
	}
}

// A directory that is both a skill and a scan-root plugin yields one row
// when one scan root reaches it through a symlink.
func TestPlugin_SkillDirAlsoScanRootPluginThroughSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	realDir := testutil.TempDirResolved(t)
	foo := filepath.Join(realDir, ".agents", "skills", "foo")
	mustWriteSkill(t, filepath.Dir(foo), "foo", "Foo")
	mustWritePlugin(t, foo, "foo")
	link := filepath.Join(testutil.TempDirResolved(t), "link")
	require.NoError(t, os.Symlink(realDir, link))
	linkedFoo := filepath.Join(link, ".agents", "skills", "foo")

	for name, roots := range map[string][]agentcontext.ScanRoot{
		"LinkFirst": {{Path: link}, {Path: foo}},
		"RealFirst": {{Path: foo}, {Path: link}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := resolvePlugins(roots...)
			var atFoo []agentcontext.Resource
			for _, res := range snap.Resources {
				if res.Source == foo || res.Source == linkedFoo {
					atFoo = append(atFoo, res)
				}
			}
			require.Len(t, atFoo, 1)
		})
	}
}

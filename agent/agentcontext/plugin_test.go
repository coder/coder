package agentcontext_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

// pluginJSON renders a plugin.json document with the 1.0.0 $schema
// plus the supplied fields.
func pluginJSON(t *testing.T, fields map[string]any) string {
	t.Helper()
	doc := map[string]any{"$schema": agentcontext.PluginSchemaV1}
	for k, v := range fields {
		doc[k] = v
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return string(data)
}

// mustWritePlugin creates a plugin root at dir with a minimal valid
// manifest for name.
func mustWritePlugin(t *testing.T, dir, name string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, "plugin.json"), pluginJSON(t, map[string]any{"name": name}))
}

func resolvePlugins(roots ...agentcontext.ScanRoot) agentcontext.Snapshot {
	r := &agentcontext.Resolver{}
	return r.ResolveWithOptions(context.Background(), roots, agentcontext.ResolveOptions{PluginsEnabled: true})
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
		manifest string
		fields   map[string]any
		// found is false when the directory must not produce a
		// plugin resource at all.
		found       bool
		status      agentcontext.ResourceStatus
		errContains string
		check       func(t *testing.T, res agentcontext.Resource)
	}{
		{
			name:   "valid minimal",
			fields: map[string]any{"name": "minimal"},
			found:  true,
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Equal(t, "minimal", res.Name)
				require.Equal(t, "minimal", res.PluginName)
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
			found:  true,
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
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "name is required",
		},
		{
			name:        "name not a string",
			fields:      map[string]any{"name": 42},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "name must be a string",
		},
		{
			name:        "name uppercase",
			fields:      map[string]any{"name": "Bad"},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "invalid character",
		},
		{
			name:        "name leading dash",
			fields:      map[string]any{"name": "-bad"},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "start and end",
		},
		{
			name:        "name double dot",
			fields:      map[string]any{"name": "a..b"},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: `must not contain ".."`,
		},
		{
			name:        "name too long",
			fields:      map[string]any{"name": strings.Repeat("a", 65)},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "maximum is 64",
		},
		{
			name:   "unknown top-level field tolerated",
			fields: map[string]any{"name": "tolerant", "hooks": []string{"x"}, "agents": 1},
			found:  true,
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Equal(t, `unknown field "agents" ignored; unknown field "hooks" ignored`, res.Error)
			},
		},
		{
			name:   "non-object extensions tolerated",
			fields: map[string]any{"name": "ext", "extensions": []string{"nope"}},
			found:  true,
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Contains(t, res.Error, "extensions must be an object")
			},
		},
		{
			name:        "unknown author key rejected",
			fields:      map[string]any{"name": "auth", "author": map[string]any{"name": "Ada", "twitter": "@ada"}},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: `author contains unknown field "twitter"`,
		},
		{
			name:        "author field not a string",
			fields:      map[string]any{"name": "auth", "author": map[string]any{"name": 7}},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "author.name must be a string",
		},
		{
			name:        "author not an object",
			fields:      map[string]any{"name": "auth", "author": "Ada"},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "author must be an object",
		},
		{
			name:        "version not a string",
			fields:      map[string]any{"name": "ver", "version": 1},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "version must be a string",
		},
		{
			name:        "keywords not strings",
			fields:      map[string]any{"name": "kw", "keywords": []any{"ok", 2}},
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "keywords must be an array of strings",
		},
		{
			name:        "missing $schema reported",
			manifest:    `{"name": "other-tool"}`,
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json has no $schema",
		},
		{
			name:        "foreign $schema reported",
			manifest:    `{"$schema": "https://raw.githubusercontent.com/grafana/grafana/main/plugin.schema.json", "name": "grafana"}`,
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "is not an Agent Plugins schema",
		},
		{
			name:        "non-string $schema reported",
			manifest:    `{"$schema": 1, "name": "x"}`,
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "is not an Agent Plugins schema",
		},
		{
			name:        "unsupported schema version rejected",
			manifest:    `{"$schema": "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json", "name": "future"}`,
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "unsupported schema version",
		},
		{
			name:        "invalid JSON reported",
			manifest:    `{"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "name": `,
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json is not valid JSON",
		},
		{
			name:        "top-level array reported",
			manifest:    `["https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"]`,
			found:       true,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json is not valid JSON",
		},
		{
			name: "version and description capped and sanitized",
			fields: map[string]any{
				"name":        "long",
				"version":     strings.Repeat("9", 100),
				"description": "zero\u200bwidth " + strings.Repeat("d", 2000),
			},
			found:  true,
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
			pluginDir := filepath.Join(root, "plugins", "candidate")
			manifest := tc.manifest
			if manifest == "" {
				manifest = pluginJSON(t, tc.fields)
			}
			mustWriteFile(t, filepath.Join(pluginDir, "plugin.json"), manifest)

			snap := resolvePlugins(agentcontext.ScanRoot{Path: root})
			plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
			if !tc.found {
				require.Empty(t, plugins)
				return
			}
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
	snap := r.ResolveWithOptions(context.Background(), []agentcontext.ScanRoot{{Path: root}}, agentcontext.ResolveOptions{PluginsEnabled: true})

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
	mustWriteFile(t, filepath.Join(first, "mcp.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{}}`)
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

	// Plugins follows discovery order: the plugins/ container is
	// walked before .agents/plugins/.
	require.Equal(t, []agentcontext.PluginInfo{
		{Name: "alpha", Root: first, HasMCPConfig: true},
		{Name: "beta", Root: second, HasMCPConfig: false},
	}, snap.Plugins())
}

func TestPlugin_RootIsPlugin(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWritePlugin(t, root, "solo")
	mustWriteSkill(t, filepath.Join(root, "skills"), "only", "Only skill")
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "not an instruction file when the root is a plugin")
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
	}, kinds)
	skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "only"))
	require.Equal(t, "solo", skill.PluginName)
}

// A plugin.json directly at a scan root that is not an Agent Plugins
// manifest belongs to some other tool: no plugin resource is emitted and
// the root's legacy discovery is untouched. Inside a plugins/ container
// the same files are reported (see TestPlugin_ManifestValidation).
func TestPlugin_RootWithForeignManifestIgnored(t *testing.T) {
	t.Parallel()
	for name, manifest := range map[string]string{
		"missing $schema": `{"name": "other-tool"}`,
		"invalid JSON":    `{"name": `,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testutil.TempDirResolved(t)
			mustWriteFile(t, filepath.Join(root, "plugin.json"), manifest)
			mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "still here")

			snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

			require.Empty(t, resourcesOfKind(snap, agentcontext.KindPlugin))
			instr := findResource(t, snap.Resources, agentcontext.KindInstructionFile, filepath.Join(root, "AGENTS.md"))
			require.Equal(t, agentcontext.StatusOK, instr.Status)
		})
	}
}

func TestPlugin_RootWithInvalidManifestKeepsLegacyDiscovery(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(root, "plugin.json"), pluginJSON(t, map[string]any{"name": "Not Valid"}))
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "still here")
	mustWriteSkill(t, filepath.Join(root, "skills"), "plain", "Plain skill")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, root)
	require.Equal(t, agentcontext.StatusInvalid, plugin.Status)
	instr := findResource(t, snap.Resources, agentcontext.KindInstructionFile, filepath.Join(root, "AGENTS.md"))
	require.Equal(t, agentcontext.StatusOK, instr.Status)
	skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "plain"))
	require.Equal(t, agentcontext.StatusOK, skill.Status)
	require.Empty(t, skill.PluginName)
}

func TestPlugin_RootIsPluginAndContainerChildEmittedOnce(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "twice")
	mustWritePlugin(t, pluginDir, "twice")
	mustWriteSkill(t, filepath.Join(pluginDir, "skills"), "one", "One")

	// The plugin directory is both a container child of root and an
	// explicit scan root.
	snap := resolvePlugins(
		agentcontext.ScanRoot{Path: pluginDir, UserSource: pluginDir},
		agentcontext.ScanRoot{Path: root},
	)

	require.Len(t, resourcesOfKind(snap, agentcontext.KindPlugin), 1)
	require.Len(t, resourcesOfKind(snap, agentcontext.KindSkill), 1)
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

func TestPlugin_SymlinkedSkillsContainerDisablesSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	mustWriteSkill(t, filepath.Join(pluginDir, "real-skills"), "one", "One")
	require.NoError(t, os.Symlink(filepath.Join(pluginDir, "real-skills"), filepath.Join(pluginDir, "skills")))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status)
	require.Contains(t, plugin.Error, "skills directory is a symlink")
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
}

func TestPlugin_MCPConfigSymlinkEscapeIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	outside := testutil.TempDirResolved(t)
	mustWriteFile(t, filepath.Join(outside, "mcp.json"), `{}`)
	pluginDir := filepath.Join(root, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	require.NoError(t, os.Symlink(filepath.Join(outside, "mcp.json"), filepath.Join(pluginDir, "mcp.json")))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, pluginDir)
	require.Equal(t, agentcontext.StatusOK, plugin.Status)
	require.Contains(t, plugin.Error, "mcp.json ignored")
	require.False(t, plugin.HasMCPConfig)
	require.Equal(t, []agentcontext.PluginInfo{{Name: "p", Root: pluginDir}}, snap.Plugins())
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
	require.Contains(t, loser.Error, `duplicate plugin name "dup"`)

	skills := resourcesOfKind(snap, agentcontext.KindSkill)
	require.Len(t, skills, 1)
	require.Equal(t, filepath.Join(firstPlugin, "skills", "first"), skills[0].Source)
	require.Equal(t, []agentcontext.PluginInfo{{Name: "dup", Root: firstPlugin}}, snap.Plugins())
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
	require.Empty(t, snap.Plugins())
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
	require.Empty(t, snap.Plugins())

	m.SetPluginsEnabled(true)
	snap = m.Snapshot()
	require.Equal(t, uint64(2), snap.Version)
	require.Equal(t, []agentcontext.PluginInfo{{Name: "p", Root: pluginDir}}, snap.Plugins())
	skill := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(pluginDir, "skills", "one"))
	require.Equal(t, "p", skill.PluginName)

	// An unchanged value does not resolve again.
	m.SetPluginsEnabled(true)
	require.Equal(t, uint64(2), m.Snapshot().Version)

	m.SetPluginsEnabled(false)
	snap = m.Snapshot()
	require.Equal(t, uint64(3), snap.Version)
	require.Empty(t, snap.Plugins())
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
	require.Len(t, snap.Plugins(), 1)
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
	require.Len(t, m.Snapshot().Plugins(), 1)
}

func TestManager_MCPCatalogPluginNamePassthrough(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		MCPCatalog: func() []agentcontext.MCPServerStatus {
			return []agentcontext.MCPServerStatus{
				{Name: "srv", Connected: true, PluginName: "p", Tools: []agentcontext.MCPTool{{Name: "echo"}}},
				{Name: "down", Connected: false, PluginName: "p", Err: "boom"},
				{Name: "srv", Connected: true, Tools: []agentcontext.MCPTool{{Name: "echo"}}},
			}
		},
	})

	// Plugin servers are located by plugin/name so a plugin entry that
	// shares a name with a workspace server keeps its own resource.
	snap := m.Snapshot()
	up := findResource(t, snap.Resources, agentcontext.KindMCPServer, "p/srv")
	require.Equal(t, "p", up.PluginName)
	require.Equal(t, "srv", up.Name)
	down := findResource(t, snap.Resources, agentcontext.KindMCPServer, "p/down")
	require.Equal(t, "p", down.PluginName)
	plain := findResource(t, snap.Resources, agentcontext.KindMCPServer, "srv")
	require.Empty(t, plain.PluginName)
}

func TestPlugin_SnapshotPluginsFollowScanRootOrder(t *testing.T) {
	t.Parallel()
	// The higher-priority root sorts after the lower-priority one by
	// path, so resource order (by ID) and root order disagree.
	base := testutil.TempDirResolved(t)
	high := filepath.Join(base, "z-user-source")
	low := filepath.Join(base, "a-working-dir")
	highPlugin := filepath.Join(high, "plugins", "zed")
	lowPlugin := filepath.Join(low, ".agents", "plugins", "alpha")
	mustWritePlugin(t, highPlugin, "zed")
	mustWritePlugin(t, lowPlugin, "alpha")

	snap := resolvePlugins(
		agentcontext.ScanRoot{Path: high, UserSource: high},
		agentcontext.ScanRoot{Path: low},
	)

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 2)
	require.Equal(t, lowPlugin, plugins[0].Source, "resources are sorted by ID")
	require.Equal(t, []agentcontext.PluginInfo{
		{Name: "zed", Root: highPlugin},
		{Name: "alpha", Root: lowPlugin},
	}, snap.Plugins(), "Plugins follows scan-root priority")
}

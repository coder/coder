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
	"unicode/utf8"

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
			name:        "homepage not a string",
			fields:      map[string]any{"name": "home", "homepage": 5},
			status:      agentcontext.StatusInvalid,
			errContains: "homepage must be a string",
		},
		{
			name:        "repository not a string",
			fields:      map[string]any{"name": "repo", "repository": 5},
			status:      agentcontext.StatusInvalid,
			errContains: "repository must be a string",
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
			errContains: "plugin.json $schema is not a string",
		},
		{
			name:        "$schema with invalid UTF-8 reported as valid UTF-8",
			manifest:    "{\"$schema\": \"\xff\xfe\", \"name\": \"x\"}",
			status:      agentcontext.StatusInvalid,
			errContains: "is not an Agent Plugins schema",
			check: func(t *testing.T, res agentcontext.Resource) {
				require.True(t, utf8.ValidString(res.Error), res.Error)
			},
		},
		{
			name:        "byte order mark reported",
			manifest:    "\ufeff" + `{"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "name": "bom"}`,
			status:      agentcontext.StatusInvalid,
			errContains: "plugin.json starts with a UTF-8 byte order mark",
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
				"version":     "1\u200b" + strings.Repeat("9", 100),
				"description": "zero\u200bwidth " + strings.Repeat("d", 2000),
			},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Len(t, res.PluginVersion, agentcontext.MaxPluginVersionRunes)
				require.True(t, strings.HasPrefix(res.PluginVersion, "19"))
				require.Len(t, []rune(res.Description), agentcontext.MaxPluginDescriptionRunes)
				require.True(t, strings.HasPrefix(res.Description, "zerowidth "))
			},
		},
		{
			name: "NUL removed from version and description",
			fields: map[string]any{
				"name":        "nul",
				"version":     "1.0\x00.0",
				"description": "a\x00b",
			},
			status: agentcontext.StatusOK,
			check: func(t *testing.T, res agentcontext.Resource) {
				require.Equal(t, "1.0.0", res.PluginVersion)
				require.Equal(t, "ab", res.Description)
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
	mustWritePlugin(t, second, "beta")
	// A plain skill next to the containers is still a plain skill.
	mustWriteSkill(t, filepath.Join(root, "skills"), "plain", "Plain skill")

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root, UserSource: root})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 2)
	for _, p := range plugins {
		require.Equal(t, agentcontext.StatusOK, p.Status, p.Error)
		require.Equal(t, root, p.SourcePath)
	}

	plain := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "plain"))
	require.Empty(t, plain.PluginName)
	require.Len(t, resourcesOfKind(snap, agentcontext.KindSkill), 1)
}

func TestPlugin_RootIsPlugin(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWritePlugin(t, root, "solo")
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
		"instruction_file:" + filepath.Join(root, "AGENTS.md"),
		"skill:" + filepath.Join(root, ".agents", "skills", "extra"),
	}, kinds)
	extra := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, ".agents", "skills", "extra"))
	require.Empty(t, extra.PluginName)
}

// A plugin.json at a scan root that is not an Agent Plugins manifest, or
// cannot be read as one, may belong to another tool: no plugin resource
// is emitted and the root's regular discovery, including skills/, is
// untouched.
func TestPlugin_RootWithUnusableManifestIgnored(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name             string
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

func TestPlugin_RootIsPluginAndContainerChildEmittedOnce(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(root, "plugins", "twice")
	mustWritePlugin(t, pluginDir, "twice")
	mustWriteFile(t, filepath.Join(pluginDir, "AGENTS.md"), "plugin rules")
	// Plugin containers inside a plugin root are not scanned.
	mustWritePlugin(t, filepath.Join(pluginDir, "plugins", "nested"), "nested")

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

func TestPlugin_DuplicateNameAcrossRootsFirstWins(t *testing.T) {
	t.Parallel()
	first := testutil.TempDirResolved(t)
	second := testutil.TempDirResolved(t)
	firstPlugin := filepath.Join(first, "plugins", "dup")
	secondPlugin := filepath.Join(second, "plugins", "dup-copy")
	mustWritePlugin(t, firstPlugin, "dup")
	mustWritePlugin(t, secondPlugin, "dup")

	snap := resolvePlugins(
		agentcontext.ScanRoot{Path: first},
		agentcontext.ScanRoot{Path: second},
	)

	winner := findResource(t, snap.Resources, agentcontext.KindPlugin, firstPlugin)
	require.Equal(t, agentcontext.StatusOK, winner.Status)
	loser := findResource(t, snap.Resources, agentcontext.KindPlugin, secondPlugin)
	require.Equal(t, agentcontext.StatusInvalid, loser.Status)
	require.Equal(t, `plugin name "dup" is already used by `+firstPlugin+`; rename one of the plugins`, loser.Error)
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
	for name, tc := range map[string]struct {
		link []string
		// inRoot puts the link target inside the scan root.
		inRoot bool
	}{
		"plugins":         {link: []string{"plugins"}},
		".agents/plugins": {link: []string{".agents", "plugins"}},
		".agents":         {link: []string{".agents"}},
		"plugins in root": {link: []string{"plugins"}, inRoot: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testutil.TempDirResolved(t)
			target := testutil.TempDirResolved(t)
			if tc.inRoot {
				target = filepath.Join(root, "vendor", "plugins")
			}
			link := tc.link
			if link[len(link)-1] == ".agents" {
				target = filepath.Join(target, "dot-agents")
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
			if tc.kind == agentcontext.KindSkill {
				require.Empty(t, findResource(t, snap.Resources, agentcontext.KindSkill, foo).Error, "a valid plugin adds no warning")
			}
		})
	}
}

func TestPlugin_ValidPluginReplacesInvalidSkillAtSameDir(t *testing.T) {
	t.Parallel()
	root := testutil.TempDirResolved(t)
	foo := filepath.Join(root, ".agents", "skills", "foo")
	mustWriteFile(t, filepath.Join(foo, "SKILL.md"), "---\nname: not-foo\ndescription: Foo\n---\nbody")
	mustWritePlugin(t, foo, "foo")

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

// Symlinked plugin directories are not loaded. In .agents/plugins/, a
// symlink to a directory holding plugin.json is reported; in plugins/ it
// is skipped silently.
func TestPlugin_SymlinkedPluginDirInAgentsContainerReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	target := filepath.Join(root, "vendor", "real")
	mustWritePlugin(t, target, "real")
	mustWriteSkill(t, filepath.Join(target, "skills"), "deploy", "Deploy")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor", "empty"), 0o755))
	agents := filepath.Join(root, ".agents", "plugins")
	require.NoError(t, os.MkdirAll(agents, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plugins"), 0o755))
	link := filepath.Join(agents, "link")
	require.NoError(t, os.Symlink(target, link))
	require.NoError(t, os.Symlink(target, filepath.Join(root, "plugins", "link")))
	// A symlink to a directory without plugin.json, and a dangling one,
	// are not plugins.
	require.NoError(t, os.Symlink(filepath.Join(root, "vendor", "empty"), filepath.Join(agents, "empty")))
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(agents, "dangling")))
	// A link whose name is not valid UTF-8 cannot be a Source.
	_ = os.Symlink(target, filepath.Join(agents, "bad\xfflink"))

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root, UserSource: root})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 1)
	require.Equal(t, link, plugins[0].Source)
	require.Equal(t, "plugin:"+link, plugins[0].ID)
	require.Equal(t, root, plugins[0].SourcePath)
	require.Equal(t, agentcontext.StatusInvalid, plugins[0].Status)
	require.Contains(t, plugins[0].Error, "plugin directory is a symlink")
	require.Empty(t, resourcesOfKind(snap, agentcontext.KindSkill))
}

// In .agents/plugins/, a plugin directory whose plugin.json cannot be
// stat'ed is reported as unreadable. In plugins/ it is skipped.
func TestPlugin_UnstatableManifestInAgentsContainerReported(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not block reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := testutil.TempDirResolved(t)
	locked := filepath.Join(root, ".agents", "plugins", "locked")
	mustWritePlugin(t, locked, "locked")
	genericLocked := filepath.Join(root, "plugins", "locked")
	mustWritePlugin(t, genericLocked, "generic-locked")
	sibling := filepath.Join(root, ".agents", "plugins", "sibling")
	mustWritePlugin(t, sibling, "sibling")
	for _, dir := range []string{locked, genericLocked} {
		require.NoError(t, os.Chmod(dir, 0o000))
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	}

	snap := resolvePlugins(agentcontext.ScanRoot{Path: root})

	plugins := resourcesOfKind(snap, agentcontext.KindPlugin)
	require.Len(t, plugins, 2)
	row := findResource(t, snap.Resources, agentcontext.KindPlugin, locked)
	require.Equal(t, agentcontext.StatusUnreadable, row.Status)
	require.Contains(t, row.Error, "permission denied")
	require.Equal(t, agentcontext.StatusOK, findResource(t, snap.Resources, agentcontext.KindPlugin, sibling).Status)
}

// A skill that a same-named skill hides claims no Source, so a plugin at
// its directory still loads, or is still reported when rejected.
func TestPlugin_NameShadowedSkillDirIsPlugin(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		manifestName string
		status       agentcontext.ResourceStatus
	}{
		"Valid":    {manifestName: "p", status: agentcontext.StatusOK},
		"Rejected": {manifestName: "Not Valid", status: agentcontext.StatusInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testutil.TempDirResolved(t)
			mustWriteSkill(t, filepath.Join(root, "skills"), "shared", "First")
			mustWriteSkill(t, filepath.Join(root, ".agents", "skills"), "shared", "Second")
			p := filepath.Join(root, ".agents", "skills", "shared")
			mustWritePlugin(t, p, tc.manifestName)

			snap := resolvePlugins(agentcontext.ScanRoot{Path: root}, agentcontext.ScanRoot{Path: p})

			first := findResource(t, snap.Resources, agentcontext.KindSkill, filepath.Join(root, "skills", "shared"))
			require.Equal(t, agentcontext.StatusOK, first.Status, first.Error)
			plugin := findResource(t, snap.Resources, agentcontext.KindPlugin, p)
			require.Equal(t, tc.status, plugin.Status, plugin.Error)
			require.False(t, slices.ContainsFunc(snap.Resources, func(res agentcontext.Resource) bool {
				return res.Kind == agentcontext.KindSkill && res.Source == p
			}), "the hidden skill is not emitted")
		})
	}
}

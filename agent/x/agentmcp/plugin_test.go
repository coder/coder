package agentmcp_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentmcp"
)

const pluginMCPSchema = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

// newPluginScope creates a plugin root and a not-yet-existing data dir
// under a fresh temp dir. The root is canonicalized so expected paths
// match the symlink-resolved paths the parser returns.
func newPluginScope(t *testing.T, name string) agentmcp.PluginScope {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, name)
	require.NoError(t, os.Mkdir(root, 0o755))
	canonRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		canonRoot = resolved
	}
	return agentmcp.PluginScope{
		Name:    name,
		Root:    canonRoot,
		DataDir: filepath.Join(base, "data", name),
	}
}

// writePluginMCP writes mcp.json at the plugin root with the given
// mcpServers and returns the matching ConfigSource.
func writePluginMCP(t *testing.T, scope agentmcp.PluginScope, servers map[string]any) agentmcp.ConfigSource {
	t.Helper()
	content := mustJSON(t, map[string]any{
		"$schema":    pluginMCPSchema,
		"mcpServers": servers,
	})
	path := filepath.Join(scope.Root, "mcp.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return agentmcp.ConfigSource{Path: path, Plugin: &scope}
}

func TestParseSource_PluginFileLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		errContains string
	}{
		{name: "MalformedJSON", content: `{not json`, errContains: "parse plugin mcp config"},
		{
			name:        "MissingSchema",
			content:     `{"mcpServers": {}, "other": 1}`,
			errContains: `found ["mcpServers" "other"]`,
		},
		{
			name:        "MissingMCPServers",
			content:     `{"$schema": "` + pluginMCPSchema + `", "other": 1}`,
			errContains: `found ["$schema" "other"]`,
		},
		{
			name:        "ExtraTopLevelKey",
			content:     `{"$schema": "` + pluginMCPSchema + `", "mcpServers": {}, "extra": 1}`,
			errContains: `found ["$schema" "extra" "mcpServers"]`,
		},
		{
			name:        "UnsupportedSchemaVersion",
			content:     `{"$schema": "https://agent-plugins.org/schemas/1.1.0/mcp.schema.json", "mcpServers": {}}`,
			errContains: `; want "` + pluginMCPSchema + `"`,
		},
		{
			name:        "SchemaNotAString",
			content:     `{"$schema": 1, "mcpServers": {}}`,
			errContains: "unsupported $schema 1",
		},
		{
			name:        "MCPServersNotAnObject",
			content:     `{"$schema": "` + pluginMCPSchema + `", "mcpServers": []}`,
			errContains: "mcpServers must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scope := newPluginScope(t, "p")
			path := filepath.Join(scope.Root, "mcp.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			servers, entryErrs, err := agentmcp.ParseSource(agentmcp.ConfigSource{Path: path, Plugin: &scope})
			require.ErrorContains(t, err, tt.errContains)
			assert.Nil(t, servers)
			assert.Nil(t, entryErrs)
		})
	}

	t.Run("RelativeScopePaths", func(t *testing.T) {
		t.Parallel()
		scope := newPluginScope(t, "p")
		src := writePluginMCP(t, scope, map[string]any{})
		for _, rel := range []agentmcp.PluginScope{
			{Name: "p", Root: "plugin", DataDir: scope.DataDir},
			{Name: "p", Root: scope.Root, DataDir: "data"},
		} {
			_, _, err := agentmcp.ParseSource(agentmcp.ConfigSource{Path: src.Path, Plugin: &rel})
			require.ErrorContains(t, err, "must be absolute")
		}
	})

	t.Run("EmptyServers", func(t *testing.T) {
		t.Parallel()
		scope := newPluginScope(t, "p")
		src := writePluginMCP(t, scope, map[string]any{})
		servers, entryErrs, err := agentmcp.ParseSource(src)
		require.NoError(t, err)
		assert.Empty(t, servers)
		assert.Empty(t, entryErrs)
	})
}

func TestParseSource_PluginEntries(t *testing.T) {
	t.Parallel()

	writeCommandFile := func(t *testing.T, root, rel string) string {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600))
		return path
	}

	tests := []struct {
		name string
		// setup prepares files under the plugin root. outside is a
		// sibling directory that is not inside the root.
		setup func(t *testing.T, scope agentmcp.PluginScope, outside string)
		entry map[string]any
		// want builds the expected config; nil means the entry must be
		// rejected.
		want        func(scope agentmcp.PluginScope) agentmcp.ServerConfig
		errContains string
		// errNotExist requires the entry error to wrap fs.ErrNotExist.
		// The OS-specific message text differs between platforms.
		errNotExist bool
		// symlink marks cases that create symlinks, which need
		// privileges on Windows.
		symlink bool
	}{
		{
			name:  "StdioBareCommand",
			entry: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "pkg"}},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio", Command: "npx",
					Args: []string{"-y", "pkg"}, Cwd: scope.Root, Plugin: &scope,
				}
			},
		},
		{
			name: "StdioRelativeCommandResolvesInsideRoot",
			setup: func(t *testing.T, scope agentmcp.PluginScope, _ string) {
				writeCommandFile(t, scope.Root, "bin/server")
			},
			entry: map[string]any{"type": "stdio", "command": "./bin/server"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio",
					Command: filepath.Join(scope.Root, "bin", "server"),
					Cwd:     scope.Root, Plugin: &scope,
				}
			},
		},
		{
			name:    "StdioRelativeCommandThroughSymlinkInsideRoot",
			symlink: true,
			setup: func(t *testing.T, scope agentmcp.PluginScope, _ string) {
				writeCommandFile(t, scope.Root, "bin/real")
				require.NoError(t, os.Symlink(filepath.Join(scope.Root, "bin", "real"), filepath.Join(scope.Root, "bin", "link")))
			},
			entry: map[string]any{"type": "stdio", "command": "./bin/link"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio",
					Command: filepath.Join(scope.Root, "bin", "real"),
					Cwd:     scope.Root, Plugin: &scope,
				}
			},
		},
		{
			name:    "StdioRelativeCommandSymlinkEscape",
			symlink: true,
			setup: func(t *testing.T, scope agentmcp.PluginScope, outside string) {
				target := writeCommandFile(t, outside, "evil")
				require.NoError(t, os.MkdirAll(filepath.Join(scope.Root, "bin"), 0o755))
				require.NoError(t, os.Symlink(target, filepath.Join(scope.Root, "bin", "server")))
			},
			entry:       map[string]any{"type": "stdio", "command": "./bin/server"},
			errContains: "resolves outside",
		},
		{
			name: "StdioRelativeCommandParentEscape",
			setup: func(t *testing.T, scope agentmcp.PluginScope, outside string) {
				writeCommandFile(t, outside, "evil")
			},
			entry:       map[string]any{"type": "stdio", "command": "./../outside/evil"},
			errContains: "resolves outside",
		},
		{
			name:        "StdioRelativeCommandMissing",
			entry:       map[string]any{"type": "stdio", "command": "./bin/missing"},
			errNotExist: true,
		},
		{
			name:        "StdioAbsoluteCommand",
			entry:       map[string]any{"type": "stdio", "command": "/usr/bin/env"},
			errContains: "bare name or a ./-relative path",
		},
		{
			name:        "StdioPathWithoutDotSlash",
			entry:       map[string]any{"type": "stdio", "command": "bin/server"},
			errContains: "bare name or a ./-relative path",
		},
		{
			name:        "StdioParentDirCommand",
			entry:       map[string]any{"type": "stdio", "command": "../server"},
			errContains: "bare name or a ./-relative path",
		},
		{
			name:        "StdioCommandWithShellWords",
			entry:       map[string]any{"type": "stdio", "command": "npx -y pkg"},
			errContains: "single token",
		},
		{
			name:        "StdioMissingCommand",
			entry:       map[string]any{"type": "stdio", "args": []string{"x"}},
			errContains: "missing command",
		},
		{
			name: "StdioCwdRelative",
			setup: func(t *testing.T, scope agentmcp.PluginScope, _ string) {
				require.NoError(t, os.Mkdir(filepath.Join(scope.Root, "work"), 0o755))
			},
			entry: map[string]any{"type": "stdio", "command": "npx", "cwd": "./work"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio", Command: "npx",
					Cwd: filepath.Join(scope.Root, "work"), Plugin: &scope,
				}
			},
		},
		{
			name: "StdioCwdPluginRoot",
			setup: func(t *testing.T, scope agentmcp.PluginScope, _ string) {
				require.NoError(t, os.Mkdir(filepath.Join(scope.Root, "work"), 0o755))
			},
			entry: map[string]any{"type": "stdio", "command": "npx", "cwd": "${PLUGIN_ROOT}/work"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio", Command: "npx",
					Cwd: filepath.Join(scope.Root, "work"), Plugin: &scope,
				}
			},
		},
		{
			name:  "StdioCwdPluginDataNotYetCreated",
			entry: map[string]any{"type": "stdio", "command": "npx", "cwd": "${PLUGIN_DATA}"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio", Command: "npx",
					Cwd: scope.DataDir, Plugin: &scope,
				}
			},
		},
		{
			name:  "StdioCwdPluginDataSubdir",
			entry: map[string]any{"type": "stdio", "command": "npx", "cwd": "${PLUGIN_DATA}/cache"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio", Command: "npx",
					Cwd: filepath.Join(scope.DataDir, "cache"), Plugin: &scope,
				}
			},
		},
		{
			name:        "StdioCwdPluginDataEscape",
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "${PLUGIN_DATA}/../other"},
			errContains: "resolves outside",
		},
		{
			name:        "StdioCwdRelativeEscape",
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "./../outside"},
			errContains: "resolves outside",
		},
		{
			name:    "StdioCwdSymlinkEscape",
			symlink: true,
			setup: func(t *testing.T, scope agentmcp.PluginScope, outside string) {
				require.NoError(t, os.Symlink(outside, filepath.Join(scope.Root, "work")))
			},
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "./work"},
			errContains: "resolves outside",
		},
		{
			name:        "StdioCwdMissing",
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "./nope"},
			errNotExist: true,
		},
		{
			name:        "StdioCwdBarePath",
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "work"},
			errContains: "must start with",
		},
		{
			name:        "StdioCwdAbsolute",
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "/tmp"},
			errContains: "must start with",
		},
		{
			name:        "StdioCwdPlaceholderWithoutSeparator",
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "${PLUGIN_ROOT}${PLUGIN_DATA}"},
			errContains: "must start with",
		},
		{
			name: "StdioCwdIsFile",
			setup: func(t *testing.T, scope agentmcp.PluginScope, _ string) {
				writeCommandFile(t, scope.Root, "f")
			},
			entry:       map[string]any{"type": "stdio", "command": "npx", "cwd": "./f"},
			errContains: "is not a directory",
		},
		{
			// The sibling's name starts with the root's name, so only the
			// separator check rejects it.
			name: "StdioRelativeCommandSiblingWithRootPrefix",
			setup: func(t *testing.T, scope agentmcp.PluginScope, _ string) {
				writeCommandFile(t, scope.Root+"-evil", "server")
			},
			entry:       map[string]any{"type": "stdio", "command": "./../p-evil/server"},
			errContains: "resolves outside",
		},
		{
			name: "HTTPHeadersDifferOnlyInCase",
			entry: map[string]any{
				"type": "streamable-http", "url": "https://example.com/mcp",
				"headers": map[string]string{"X-Key": "a", "x-key": "b"},
			},
			errContains: `headers "X-Key" and "x-key" differ only in case`,
		},
		{
			name: "HTTPHeaderInvalidName",
			entry: map[string]any{
				"type": "sse", "url": "https://example.com/mcp",
				"headers": map[string]string{"Bad Name": "a"},
			},
			errContains: `invalid header name "Bad Name"`,
		},
		{
			name: "HTTPHeaderInvalidValue",
			entry: map[string]any{
				"type": "sse", "url": "https://example.com/mcp",
				"headers": map[string]string{"X-Key": "a\nb"},
			},
			errContains: `header "X-Key" has an invalid value`,
		},
		{
			name:        "StdioEnvSetsPluginRoot",
			entry:       map[string]any{"type": "stdio", "command": "npx", "env": map[string]string{"PLUGIN_ROOT": "/x"}},
			errContains: "may not set PLUGIN_ROOT",
		},
		{
			name:        "StdioEnvSetsPluginData",
			entry:       map[string]any{"type": "stdio", "command": "npx", "env": map[string]string{"PLUGIN_DATA": "/x"}},
			errContains: "may not set PLUGIN_DATA",
		},
		{
			name: "StdioPlaceholdersExpandedOnlyForPluginNames",
			entry: map[string]any{
				"type":    "stdio",
				"command": "npx",
				"args":    []string{"${PLUGIN_ROOT}/a", "${PLUGIN_DATA}/b", "$HOME", "${OTHER}", "${PLUGIN_ROOT"},
				"env": map[string]string{
					"A": "${PLUGIN_ROOT}/x",
					"B": "${PLUGIN_DATA}",
					"C": "${HOME}:${PATH}",
				},
			},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "stdio", Command: "npx",
					Args: []string{scope.Root + "/a", scope.DataDir + "/b", "$HOME", "${OTHER}", "${PLUGIN_ROOT"},
					Env: map[string]string{
						"A": scope.Root + "/x",
						"B": scope.DataDir,
						"C": "${HOME}:${PATH}",
					},
					Cwd: scope.Root, Plugin: &scope,
				}
			},
		},
		{
			name:        "UnknownEntryFieldRejected",
			entry:       map[string]any{"type": "stdio", "command": "npx", "arg": []string{"-y"}},
			errContains: `field "arg" is not allowed for type "stdio"`,
		},
		{
			name:        "FieldNameCaseMismatchRejected",
			entry:       map[string]any{"type": "stdio", "Command": "npx"},
			errContains: `field "Command" is not allowed`,
		},
		{
			name:        "StdioWithURLRejected",
			entry:       map[string]any{"type": "stdio", "command": "npx", "url": "https://example.com/mcp"},
			errContains: `field "url" is not allowed for type "stdio"`,
		},
		{
			name:        "HTTPWithCommandRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "https://example.com/mcp", "command": "npx"},
			errContains: `field "command" is not allowed for type "streamable-http"`,
		},
		{
			name:        "SSEWithCwdRejected",
			entry:       map[string]any{"type": "sse", "url": "https://example.com/mcp", "cwd": "./"},
			errContains: `field "cwd" is not allowed for type "sse"`,
		},
		{
			name:        "MissingType",
			entry:       map[string]any{"command": "npx"},
			errContains: "missing type",
		},
		{
			name:        "HTTPTypeLiteralRejected",
			entry:       map[string]any{"type": "http", "url": "https://example.com/mcp"},
			errContains: `unsupported type "http"`,
		},
		{
			name:        "UnknownType",
			entry:       map[string]any{"type": "websocket", "url": "wss://example.com/mcp"},
			errContains: "unsupported type",
		},
		{
			name: "StreamableHTTPHeadersVerbatim",
			entry: map[string]any{
				"type":    "streamable-http",
				"url":     "https://example.com/mcp",
				"headers": map[string]string{"Authorization": "Bearer ${PLUGIN_ROOT}"},
			},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "http", URL: "https://example.com/mcp",
					Headers: map[string]string{"Authorization": "Bearer ${PLUGIN_ROOT}"},
					Plugin:  &scope,
				}
			},
		},
		{
			name: "SSEHeadersVerbatim",
			entry: map[string]any{
				"type":    "sse",
				"url":     "https://example.com/sse",
				"headers": map[string]string{"X-Key": "${PLUGIN_DATA}"},
			},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "sse", URL: "https://example.com/sse",
					Headers: map[string]string{"X-Key": "${PLUGIN_DATA}"},
					Plugin:  &scope,
				}
			},
		},
		{
			name:        "SSEPlainHTTPRejected",
			entry:       map[string]any{"type": "sse", "url": "http://example.com/sse"},
			errContains: "https",
		},
		{
			name:  "HTTPLocalhostAllowed",
			entry: map[string]any{"type": "streamable-http", "url": "http://localhost:8080/mcp"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "http", URL: "http://localhost:8080/mcp", Plugin: &scope,
				}
			},
		},
		{
			name:  "HTTPLoopbackIPv4Allowed",
			entry: map[string]any{"type": "streamable-http", "url": "http://127.0.0.1:8080/mcp"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "http", URL: "http://127.0.0.1:8080/mcp", Plugin: &scope,
				}
			},
		},
		{
			name:  "HTTPLoopbackIPv6Allowed",
			entry: map[string]any{"type": "streamable-http", "url": "http://[::1]:8080/mcp"},
			want: func(scope agentmcp.PluginScope) agentmcp.ServerConfig {
				return agentmcp.ServerConfig{
					Name: "srv", Transport: "http", URL: "http://[::1]:8080/mcp", Plugin: &scope,
				}
			},
		},
		{
			name:        "HTTPNonLoopbackRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "http://example.com/mcp"},
			errContains: "must use https",
		},
		{
			name:        "HTTPPrivateAddressRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "http://10.0.0.5/mcp"},
			errContains: "must use https",
		},
		{
			name:        "URLUserinfoRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "https://user:pw@example.com/mcp"},
			errContains: "userinfo",
		},
		{
			name:        "URLFragmentRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "https://example.com/mcp#frag"},
			errContains: "fragment",
		},
		{
			name:        "URLEmptyFragmentRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "https://example.com/mcp#"},
			errContains: "fragment",
		},
		{
			name:        "URLRelativeRejected",
			entry:       map[string]any{"type": "streamable-http", "url": "/mcp"},
			errContains: "must be absolute",
		},
		{
			name:        "URLMissing",
			entry:       map[string]any{"type": "streamable-http"},
			errContains: "missing url",
		},
		{
			name:        "URLUnsupportedScheme",
			entry:       map[string]any{"type": "streamable-http", "url": "ftp://example.com/mcp"},
			errContains: "unsupported scheme",
		},
		{
			name:        "EntryNotAnObject",
			entry:       nil,
			errContains: "parse server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.symlink && runtime.GOOS == "windows" {
				t.Skip("symlinks require elevated privileges on Windows")
			}
			scope := newPluginScope(t, "p")
			outside := filepath.Join(filepath.Dir(scope.Root), "outside")
			require.NoError(t, os.Mkdir(outside, 0o755))
			if tt.setup != nil {
				tt.setup(t, scope, outside)
			}

			var entry any = tt.entry
			if tt.entry == nil {
				entry = "not an object"
			}
			src := writePluginMCP(t, scope, map[string]any{"srv": entry})

			servers, entryErrs, err := agentmcp.ParseSource(src)
			require.NoError(t, err)

			if tt.want == nil {
				require.Empty(t, servers)
				require.Contains(t, entryErrs, "srv")
				if tt.errNotExist {
					assert.ErrorIs(t, entryErrs["srv"], fs.ErrNotExist)
				} else {
					assert.ErrorContains(t, entryErrs["srv"], tt.errContains)
				}
				return
			}
			require.Empty(t, entryErrs)
			require.Equal(t, []agentmcp.ServerConfig{tt.want(scope)}, servers)
		})
	}
}

func TestParseSource_PluginBadNameIsolated(t *testing.T) {
	t.Parallel()

	scope := newPluginScope(t, "p")
	src := writePluginMCP(t, scope, map[string]any{
		"good":    map[string]any{"type": "stdio", "command": "npx"},
		"bad__x":  map[string]any{"type": "stdio", "command": "npx"},
		"no-type": map[string]any{"command": "npx"},
	})

	servers, entryErrs, err := agentmcp.ParseSource(src)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	assert.Equal(t, "good", servers[0].Name)
	assert.Len(t, entryErrs, 2)
	assert.Contains(t, entryErrs, "bad__x")
	assert.Contains(t, entryErrs, "no-type")
}

func TestParseSource_PluginMCPConfigSymlinkOutsideRootRejected(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	scope := newPluginScope(t, "acme")
	outside := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(outside, []byte(mustJSON(t, map[string]any{
		"$schema":    pluginMCPSchema,
		"mcpServers": map[string]any{},
	})), 0o600))
	path := filepath.Join(scope.Root, "mcp.json")
	require.NoError(t, os.Symlink(outside, path))

	_, _, err := agentmcp.ParseSource(agentmcp.ConfigSource{Path: path, Plugin: &scope})
	require.ErrorContains(t, err, "resolves outside")
}

func TestParseSource_PluginEnvKeyRules(t *testing.T) {
	t.Parallel()

	scope := newPluginScope(t, "acme")
	src := writePluginMCP(t, scope, map[string]any{
		"bad-eq":    map[string]any{"type": "stdio", "command": "srv", "env": map[string]string{"A=B": "x"}},
		"bad-empty": map[string]any{"type": "stdio", "command": "srv", "env": map[string]string{"": "x"}},
		"ok":        map[string]any{"type": "stdio", "command": "srv", "env": map[string]string{"A_B": "x"}},
	})

	servers, entryErrs, err := agentmcp.ParseSource(src)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	require.Equal(t, "ok", servers[0].Name)
	require.ErrorContains(t, entryErrs["bad-eq"], "invalid env name")
	require.ErrorContains(t, entryErrs["bad-empty"], "invalid env name")
}

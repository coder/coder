package agentmcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentmcp"
)

func TestParseConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		expected    []agentmcp.ServerConfig
		expectError bool
	}{
		{
			name: "StdioServer",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"my-server": map[string]any{
						"command": "npx",
						"args":    []string{"-y", "@example/mcp-server"},
						"env":     map[string]string{"FOO": "bar"},
					},
				},
			}),
			expected: []agentmcp.ServerConfig{
				{
					Name:      "my-server",
					Transport: "stdio",
					Command:   "npx",
					Args:      []string{"-y", "@example/mcp-server"},
					Env:       map[string]string{"FOO": "bar"},
				},
			},
		},
		{
			name: "HTTPServer",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"remote": map[string]any{
						"url":     "https://example.com/mcp",
						"headers": map[string]string{"Authorization": "Bearer tok"},
					},
				},
			}),
			expected: []agentmcp.ServerConfig{
				{
					Name:      "remote",
					Transport: "http",
					URL:       "https://example.com/mcp",
					Headers:   map[string]string{"Authorization": "Bearer tok"},
				},
			},
		},
		{
			name: "SSEServer",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"events": map[string]any{
						"type": "sse",
						"url":  "https://example.com/sse",
					},
				},
			}),
			expected: []agentmcp.ServerConfig{
				{
					Name:      "events",
					Transport: "sse",
					URL:       "https://example.com/sse",
				},
			},
		},
		{
			name: "ExplicitTypeOverridesInference",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"hybrid": map[string]any{
						"command": "some-binary",
						"type":    "http",
					},
				},
			}),
			expected: []agentmcp.ServerConfig{
				{
					Name:      "hybrid",
					Transport: "http",
					Command:   "some-binary",
				},
			},
		},
		{
			name: "EnvVarPassthrough",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"srv": map[string]any{
						"command": "run",
						"env":     map[string]string{"PLAIN": "literal-value"},
					},
				},
			}),
			expected: []agentmcp.ServerConfig{
				{
					Name:      "srv",
					Transport: "stdio",
					Command:   "run",
					Env:       map[string]string{"PLAIN": "literal-value"},
				},
			},
		},
		{
			name: "EmptyMCPServers",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{},
			}),
			expected: []agentmcp.ServerConfig{},
		},
		{
			name:        "MalformedJSON",
			content:     `{not valid json`,
			expectError: true,
		},
		{
			name: "ServerNameContainsSeparator",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"bad__name": map[string]any{"command": "run"},
				},
			}),
			expectError: true,
		},
		{
			name: "ServerNameTrailingUnderscore",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"server_": map[string]any{"command": "run"},
				},
			}),
			expectError: true,
		},
		{
			name: "ServerNameLeadingUnderscore",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"_server": map[string]any{"command": "run"},
				},
			}),
			expectError: true,
		},
		{
			name: "EmptyTransport", content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"empty": map[string]any{},
				},
			}),
			expectError: true,
		},
		{
			name: "MissingMCPServersKey",
			content: mustJSON(t, map[string]any{
				"servers": map[string]any{},
			}),
			expected: []agentmcp.ServerConfig{},
		},
		{
			name: "MultipleServersSortedByName",
			content: mustJSON(t, map[string]any{
				"mcpServers": map[string]any{
					"zeta":  map[string]any{"command": "z"},
					"alpha": map[string]any{"command": "a"},
					"mu":    map[string]any{"command": "m"},
				},
			}),
			expected: []agentmcp.ServerConfig{
				{Name: "alpha", Transport: "stdio", Command: "a"},
				{Name: "mu", Transport: "stdio", Command: "m"},
				{Name: "zeta", Transport: "stdio", Command: "z"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, ".mcp.json")
			err := os.WriteFile(path, []byte(tt.content), 0o600)
			require.NoError(t, err)

			got, err := agentmcp.ParseConfig(path)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

// TestParseConfig_EnvVarInterpolation verifies that ${VAR} references
// in env values are resolved from the process environment. This test
// cannot be parallel because t.Setenv is incompatible with t.Parallel.
func TestParseConfig_EnvVarInterpolation(t *testing.T) {
	t.Setenv("TEST_MCP_TOKEN", "secret123")

	content := mustJSON(t, map[string]any{
		"mcpServers": map[string]any{
			"srv": map[string]any{
				"command": "run",
				"env":     map[string]string{"TOKEN": "${TEST_MCP_TOKEN}"},
			},
		},
	})

	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)

	got, err := agentmcp.ParseConfig(path)
	require.NoError(t, err)
	require.Equal(t, []agentmcp.ServerConfig{
		{
			Name:      "srv",
			Transport: "stdio",
			Command:   "run",
			Env:       map[string]string{"TOKEN": "secret123"},
		},
	}, got)
}

func TestParseConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := agentmcp.ParseConfig(filepath.Join(t.TempDir(), "nonexistent.json"))
	require.Error(t, err)
}

// mustJSON marshals v to a JSON string, failing the test on error.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

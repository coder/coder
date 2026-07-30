package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveModuleArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    []moduleRef
		wantErr string
	}{
		{
			name: "single default namespace",
			args: []string{"coder/claude-code"},
			want: []moduleRef{{namespace: "coder", slug: "claude-code"}},
		},
		{
			name: "custom namespace",
			args: []string{"coder-labs/codex"},
			want: []moduleRef{{namespace: "coder-labs", slug: "codex"}},
		},
		{
			name: "multiple modules",
			args: []string{"coder/antigravity", "coder-labs/codex", "coder/kasmvnc"},
			want: []moduleRef{
				{namespace: "coder", slug: "antigravity"},
				{namespace: "coder-labs", slug: "codex"},
				{namespace: "coder", slug: "kasmvnc"},
			},
		},
		{
			name:    "missing slash",
			args:    []string{"noslash"},
			wantErr: `invalid module ID "noslash"`,
		},
		{
			name:    "empty namespace",
			args:    []string{"/slug"},
			wantErr: `invalid module ID "/slug"`,
		},
		{
			name:    "empty slug",
			args:    []string{"namespace/"},
			wantErr: `invalid module ID "namespace/"`,
		},
		{
			name:    "unknown module",
			args:    []string{"coder/nonexistent"},
			wantErr: `unknown module "nonexistent"`,
		},
		{
			name:    "namespace mismatch",
			args:    []string{"wrong-ns/codex"},
			wantErr: `namespace mismatch: got "wrong-ns", moduleConfigs expects "coder-labs"`,
		},
		{
			name:    "error on first bad arg stops",
			args:    []string{"coder/claude-code", "bad"},
			wantErr: `invalid module ID "bad"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveModuleArgs(tt.args)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"/module/code.svg", "/icon/code.svg"},
		{"/module/nested/path.svg", "/icon/nested/path.svg"},
		{"/icon/already.svg", "/icon/already.svg"},
		{"/other/path.svg", "/other/path.svg"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, normalizeIcon(tt.input))
		})
	}
}

func TestLatestVersion(t *testing.T) {
	t.Parallel()

	entry := func(v string) struct {
		Version string `json:"version"`
	} {
		return struct {
			Version string `json:"version"`
		}{Version: v}
	}

	tests := []struct {
		name    string
		entries []struct {
			Version string `json:"version"`
		}
		want    string
		wantErr string
	}{
		{
			name: "single version",
			entries: []struct {
				Version string `json:"version"`
			}{entry("1.0.0")},
			want: "1.0.0",
		},
		{
			name: "picks highest",
			entries: []struct {
				Version string `json:"version"`
			}{entry("1.0.0"), entry("2.1.0"), entry("1.5.3")},
			want: "2.1.0",
		},
		{
			name: "handles v prefix",
			entries: []struct {
				Version string `json:"version"`
			}{entry("v1.0.0"), entry("2.0.0")},
			want: "2.0.0",
		},
		{
			name: "skips invalid versions",
			entries: []struct {
				Version string `json:"version"`
			}{entry("not-semver"), entry("1.2.3")},
			want: "1.2.3",
		},
		{
			name:    "empty list",
			entries: nil,
			wantErr: "no valid semver",
		},
		{
			name: "all invalid",
			entries: []struct {
				Version string `json:"version"`
			}{entry("bad"), entry("also-bad")},
			wantErr: "no valid semver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := latestVersion(tt.entries)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertVariables(t *testing.T) {
	t.Parallel()

	t.Run("filters skipped and complex types", func(t *testing.T) {
		t.Parallel()
		vars := []registryVariable{
			{Name: "agent_id", Type: "string", Required: true},
			{Name: "order", Type: "number"},
			{Name: "complex_var", Type: "list(string)"},
			{Name: "user_name", Type: "string", Description: "The user name", Default: "default"},
			{Name: "count", Type: "number", Required: true},
			{Name: "enabled", Type: "bool", Default: true},
			{Name: "custom_skip", Type: "string"},
		}

		result := convertVariables(vars, []string{"custom_skip"})

		require.Len(t, result, 4)

		// agent_id should be computed and not required
		assert.Equal(t, "agent_id", result[0].Name)
		assert.True(t, result[0].Computed)
		assert.False(t, result[0].Required)

		// user_name should have default
		assert.Equal(t, "user_name", result[1].Name)
		assert.False(t, result[1].Computed)
		assert.False(t, result[1].Required)
		assert.Equal(t, json.RawMessage(`"default"`), result[1].Default)

		// count should be required
		assert.Equal(t, "count", result[2].Name)
		assert.True(t, result[2].Required)

		// enabled should have bool default
		assert.Equal(t, "enabled", result[3].Name)
		assert.Equal(t, json.RawMessage(`true`), result[3].Default)
	})

	t.Run("sensitive variable", func(t *testing.T) {
		t.Parallel()
		vars := []registryVariable{
			{Name: "api_key", Type: "string", Sensitive: true},
		}
		result := convertVariables(vars, nil)
		require.Len(t, result, 1)
		assert.True(t, result[0].Sensitive)
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		result := convertVariables(nil, nil)
		assert.Nil(t, result)
	})
}

func TestModuleConfigsConsistency(t *testing.T) {
	t.Parallel()

	for slug, cfg := range moduleConfigs {
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			assert.NotEmpty(t, cfg.Category, "module %q has empty category", slug)
			assert.NotEmpty(t, cfg.CompatibleOS, "module %q has empty compatible_os", slug)
			assert.NotNil(t, cfg.ConflictsWith, "module %q has nil conflicts_with (use empty slice)", slug)
		})
	}
}

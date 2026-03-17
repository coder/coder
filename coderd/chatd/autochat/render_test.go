package autochat_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/autochat"
)

func TestRenderPrompt(t *testing.T) {
	t.Parallel()

	t.Run("BasicSubstitution", func(t *testing.T) {
		t.Parallel()
		result, err := autochat.RenderPrompt(
			"Hello, {{.Name}}!",
			map[string]any{"Name": "world"},
		)
		require.NoError(t, err)
		require.Equal(t, "Hello, world!", result)
	})

	t.Run("NestedMapAccess", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"Body": map[string]any{
				"action": "opened",
				"issue": map[string]any{
					"title": "bug report",
				},
			},
			"Headers": map[string]any{
				"X-GitHub-Event": "issues",
			},
		}
		result, err := autochat.RenderPrompt(
			`Event: {{index .Headers "X-GitHub-Event"}} Action: {{.Body.action}} Title: {{.Body.issue.title}}`,
			data,
		)
		require.NoError(t, err)
		require.Equal(t, "Event: issues Action: opened Title: bug report", result)
	})

	t.Run("MissingKeyError", func(t *testing.T) {
		t.Parallel()
		_, err := autochat.RenderPrompt(
			"Hello, {{.Missing}}!",
			map[string]any{"Name": "world"},
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "execute prompt template")
	})

	t.Run("EmptyResultError", func(t *testing.T) {
		t.Parallel()
		_, err := autochat.RenderPrompt(
			"  {{.Value}}  ",
			map[string]any{"Value": ""},
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "rendered to empty string")
	})

	t.Run("FuncTrimSpace", func(t *testing.T) {
		t.Parallel()
		result, err := autochat.RenderPrompt(
			`{{trimSpace "  hello  "}}`,
			map[string]any{},
		)
		require.NoError(t, err)
		require.Equal(t, "hello", result)
	})

	t.Run("FuncUpper", func(t *testing.T) {
		t.Parallel()
		result, err := autochat.RenderPrompt(
			`{{upper "hello"}}`,
			map[string]any{},
		)
		require.NoError(t, err)
		require.Equal(t, "HELLO", result)
	})

	t.Run("FuncLower", func(t *testing.T) {
		t.Parallel()
		result, err := autochat.RenderPrompt(
			`{{lower "HELLO"}}`,
			map[string]any{},
		)
		require.NoError(t, err)
		require.Equal(t, "hello", result)
	})

	t.Run("FuncNow", func(t *testing.T) {
		t.Parallel()
		before := time.Now().UTC()
		result, err := autochat.RenderPrompt(
			`{{now}}`,
			map[string]any{},
		)
		after := time.Now().UTC()
		require.NoError(t, err)

		parsed, err := time.Parse(time.RFC3339, result)
		require.NoError(t, err)
		require.False(t, parsed.Before(before.Add(-time.Second)),
			"now() should not be before test start")
		require.False(t, parsed.After(after.Add(time.Second)),
			"now() should not be after test end")
	})

	t.Run("InvalidTemplate", func(t *testing.T) {
		t.Parallel()
		_, err := autochat.RenderPrompt(
			"{{.Unclosed",
			map[string]any{},
		)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse prompt template")
	})

	t.Run("FuncPipeline", func(t *testing.T) {
		t.Parallel()
		result, err := autochat.RenderPrompt(
			`{{.Value | upper | trimSpace}}`,
			map[string]any{"Value": "  hello  "},
		)
		require.NoError(t, err)
		// upper is applied first, then trimSpace.
		require.Equal(t, strings.TrimSpace(strings.ToUpper("  hello  ")), result)
	})
}

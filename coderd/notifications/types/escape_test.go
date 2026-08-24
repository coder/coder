package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/types"
)

func TestEscapedForMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("EscapesLabelsAndUserName", func(t *testing.T) {
		t.Parallel()

		payload := types.MessagePayload{
			UserName: "Eve [Re-auth](https://attacker.example)",
			Labels: map[string]string{
				"suspended_account_user_name": "Eve\n## URGENT\n[Re-auth](https://attacker.example)",
				"limit_source":                "user_override",
			},
		}

		escaped := payload.EscapedForMarkdown()
		require.Equal(t, `Eve \[Re-auth\]\(https://attacker.example\)`, escaped.UserName)
		require.NotContains(t, escaped.Labels["suspended_account_user_name"], "[Re-auth](")
		// Control values must survive so template conditionals keep matching.
		require.Equal(t, "user_override", escaped.Labels["limit_source"])
	})

	t.Run("LeavesReceiverUntouched", func(t *testing.T) {
		t.Parallel()

		// The webhook dispatcher surfaces the payload to consumers verbatim, and
		// the SMTP dispatcher escapes at its own sinks, so escaping must not
		// mutate the original.
		payload := types.MessagePayload{
			UserName: "Eve [x](https://attacker.example)",
			Labels:   map[string]string{"name": "bobby-workspace", "risky": "[x](https://attacker.example)"},
			Data:     map[string]any{"user": map[string]any{"name": "[x](https://attacker.example)"}},
		}

		_ = payload.EscapedForMarkdown()

		require.Equal(t, "Eve [x](https://attacker.example)", payload.UserName)
		require.Equal(t, "[x](https://attacker.example)", payload.Labels["risky"])
		require.Equal(t, "[x](https://attacker.example)",
			payload.Data["user"].(map[string]any)["name"])
	})

	t.Run("RecursesIntoData", func(t *testing.T) {
		t.Parallel()

		payload := types.MessagePayload{
			Data: map[string]any{
				"user": map[string]any{"name": "[x](https://attacker.example)"},
				"archived_chats": []any{
					map[string]any{"title": "[x](https://attacker.example)"},
				},
			},
		}

		escaped := payload.EscapedForMarkdown()
		require.Equal(t, `\[x\]\(https://attacker.example\)`,
			escaped.Data["user"].(map[string]any)["name"])
		require.Equal(t, `\[x\]\(https://attacker.example\)`,
			escaped.Data["archived_chats"].([]any)[0].(map[string]any)["title"])
	})

	t.Run("PreservesNonStringLeaves", func(t *testing.T) {
		t.Parallel()

		// Body templates compare numbers, for example
		// {{if gt $version.failed_count 1}}. Coercing them to strings would
		// break those comparisons.
		payload := types.MessagePayload{
			Data: map[string]any{
				"failed_count": 3.0,
				"enabled":      true,
				"absent":       nil,
				"versions":     []any{map[string]any{"failed_count": 1.0}},
			},
		}

		escaped := payload.EscapedForMarkdown()
		require.Equal(t, 3.0, escaped.Data["failed_count"])
		require.Equal(t, true, escaped.Data["enabled"])
		require.Nil(t, escaped.Data["absent"])
		require.Equal(t, 1.0, escaped.Data["versions"].([]any)[0].(map[string]any)["failed_count"])
	})

	t.Run("NilMapsStayNil", func(t *testing.T) {
		t.Parallel()

		escaped := types.MessagePayload{}.EscapedForMarkdown()
		require.Nil(t, escaped.Labels)
		require.Nil(t, escaped.Data)
	})

	t.Run("EscapesNestedMapKeys", func(t *testing.T) {
		t.Parallel()

		// A nested key is content, not an identifier, whenever a template
		// ranges over the map with two variables. The resource replacements
		// body does exactly that:
		//
		//   {{range $resource, $paths := .Data.replacements -}}
		//   - _{{ $resource }}_  was replaced due to changes to _{{ $paths }}_
		//
		// and those keys are Terraform resource addresses from provisioner
		// output, so an unescaped one renders a live anchor in the email.
		payload := types.MessagePayload{
			Data: map[string]any{
				"replacements": map[string]any{
					"[Re-auth](https://attacker.example/login)": "paths",
					"null_resource.ok":                          "other",
				},
			},
		}

		escaped := payload.EscapedForMarkdown()
		replacements, ok := escaped.Data["replacements"].(map[string]any)
		require.True(t, ok)
		require.Contains(t, replacements, `\[Re-auth\]\(https://attacker.example/login\)`)
		require.NotContains(t, replacements, "[Re-auth](https://attacker.example/login)")
		// A key with nothing to escape is untouched, so no template that reads
		// a key by name starts missing.
		require.Contains(t, replacements, "null_resource.ok")
	})

	t.Run("LeavesTopLevelDataKeysAlone", func(t *testing.T) {
		t.Parallel()

		// Top-level .Data keys are label names that templates dereference as
		// {{.Data.replacements}}, not content. Escaping one would make the
		// template stop resolving it.
		payload := types.MessagePayload{
			Data: map[string]any{"failed_builds": []any{"x"}},
		}

		require.Contains(t, payload.EscapedForMarkdown().Data, "failed_builds")
	})
}

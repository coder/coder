package notifications

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/testutil"
)

func TestEscapeMarkdownLabels(t *testing.T) {
	t.Parallel()

	t.Run("EscapesNonAllowlisted", func(t *testing.T) {
		t.Parallel()

		in := map[string]string{"deleted_account_user_name": "Eve [x](https://evil.example)"}
		out := escapeMarkdownLabels(TemplateUserAccountDeleted, in)

		require.NotContains(t, out["deleted_account_user_name"], "[")
		require.Contains(t, out["deleted_account_user_name"], "&#91;")
		// The input map must not be mutated.
		require.Equal(t, "Eve [x](https://evil.example)", in["deleted_account_user_name"])
	})

	t.Run("PreservesAllowlisted", func(t *testing.T) {
		t.Parallel()

		raw := "See [docs](https://coder.com)"
		out := escapeMarkdownLabels(TemplateCustomNotification, map[string]string{
			"custom_title":   raw,
			"custom_message": raw,
			"other":          raw,
		})

		// Allowlisted labels are left as-authored markdown.
		require.Equal(t, raw, out["custom_title"])
		require.Equal(t, raw, out["custom_message"])
		// Any other label on the same template is still escaped.
		require.NotEqual(t, raw, out["other"])
	})
}

// TestNotificationLabelInjectionNeutralized is the regression test for
// ANT-2026-22448 (and its generalization): a user-controlled label value with
// markdown link/emphasis syntax must not render as a live link or emphasis in
// the notification body, when rendered through the real template + markdown
// pipeline.
func TestNotificationLabelInjectionNeutralized(t *testing.T) {
	t.Parallel()

	const malicious = "Eve [Re-auth](https://coder-sso.evil.example)"

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	templates := allNotificationTemplates(ctx, t, store)
	helpers := testHelpers()

	// The user-controlled name label for each account template.
	cases := map[uuid.UUID]string{
		TemplateUserAccountDeleted:   "deleted_account_user_name",
		TemplateUserAccountCreated:   "created_account_user_name",
		TemplateUserAccountSuspended: "suspended_account_user_name",
		TemplateUserAccountActivated: "activated_account_user_name",
	}

	for id, targetLabel := range cases {
		tmpl, ok := templates[id]
		require.True(t, ok, "template %s not seeded", id)

		labels := benignLabels(tmpl, malicious, targetLabel)
		payload := types.MessagePayload{
			UserName: "Recipient",
			Labels:   escapeMarkdownLabels(id, labels),
		}

		// The malicious value lands in the body. It must not produce a link,
		// and its link syntax must survive as literal text.
		body, err := render.GoTemplate(tmpl.BodyTemplate, payload, helpers)
		require.NoError(t, err)
		bodyHTML := markdown.HTMLFromMarkdown(body)
		require.NotContains(t, bodyHTML, "<a ", "%s body produced a link: %q", tmpl.Name, bodyHTML)
		require.Contains(t, bodyHTML, "[Re-auth]", "%s body did not render the label literally: %q", tmpl.Name, bodyHTML)

		// The title must never produce a link regardless of which label the
		// value lands in.
		title, err := render.GoTemplate(tmpl.TitleTemplate, payload, helpers)
		require.NoError(t, err)
		require.NotContains(t, markdown.HTMLFromMarkdown(title), "<a ", "%s title produced a link", tmpl.Name)
	}
}

// TestAllowlistedNotificationLabelsRenderMarkdown proves the allowlist works:
// intentional-markdown labels still render as markdown.
func TestAllowlistedNotificationLabelsRenderMarkdown(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	templates := allNotificationTemplates(ctx, t, store)
	helpers := testHelpers()

	tmpl, ok := templates[TemplateCustomNotification]
	require.True(t, ok)

	labels := escapeMarkdownLabels(TemplateCustomNotification, map[string]string{
		"custom_title":   "Heads up",
		"custom_message": "Please read the [docs](https://coder.com).",
	})
	rendered, err := render.GoTemplate(tmpl.BodyTemplate, types.MessagePayload{Labels: labels}, helpers)
	require.NoError(t, err)
	html := markdown.HTMLFromMarkdown(rendered)
	require.Contains(t, html, `<a href="https://coder.com"`, "allowlisted markdown should render: %q", html)
}

// TestNotificationTemplatesNoLineStartLabels guards the link-focused escaper's
// assumption that untrusted labels never sit at the start of a line (where they
// could still introduce a block element such as a heading or list). Allowlisted
// (intentional-markdown) labels are exempt.
func TestNotificationTemplatesNoLineStartLabels(t *testing.T) {
	t.Parallel()

	store, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	templates := allNotificationTemplates(ctx, t, store)

	for _, tmpl := range templates {
		for field, text := range map[string]string{"title": tmpl.TitleTemplate, "body": tmpl.BodyTemplate} {
			for _, label := range lineStartLabels(text) {
				if _, ok := rawMarkdownLabels[tmpl.ID][label]; ok {
					continue
				}
				t.Errorf("template %q (%s) %s places non-allowlisted label %q at a line start; "+
					"wrap it mid-line, or add it to rawMarkdownLabels if it is intentional markdown",
					tmpl.Name, tmpl.ID, field, label)
			}
		}
	}
}

func testHelpers() template.FuncMap {
	return template.FuncMap{
		"base_url":     func() string { return "https://coder.example.com" },
		"current_year": func() string { return "2026" },
		"logo_url":     func() string { return "https://coder.com/logo.png" },
		"app_name":     func() string { return "Coder" },
	}
}

func allNotificationTemplates(ctx context.Context, t *testing.T, store database.Store) map[uuid.UUID]database.NotificationTemplate {
	t.Helper()

	out := make(map[uuid.UUID]database.NotificationTemplate)
	for _, kind := range []database.NotificationTemplateKind{
		database.NotificationTemplateKindSystem,
		database.NotificationTemplateKindCustom,
	} {
		tmpls, err := store.GetNotificationTemplatesByKind(ctx, kind)
		require.NoError(t, err)
		for _, tmpl := range tmpls {
			out[tmpl.ID] = tmpl
		}
	}
	require.NotEmpty(t, out, "expected notification templates to be seeded")
	return out
}

var labelRefRe = regexp.MustCompile(`\.Labels\.(\w+)`)

// benignLabels sets every label referenced by the template to a benign value,
// then overrides target with value. This ensures conditionals guarding the
// target label ({{if .Labels.target}}) are satisfied.
func benignLabels(tmpl database.NotificationTemplate, value, target string) map[string]string {
	labels := make(map[string]string)
	for _, text := range []string{tmpl.TitleTemplate, tmpl.BodyTemplate} {
		for _, m := range labelRefRe.FindAllStringSubmatch(text, -1) {
			labels[m[1]] = "benign"
		}
	}
	labels[target] = value
	return labels
}

// lineStartLabelRe matches a label output action at the start of a line, after
// optional leading whitespace and any leading template control actions
// (if/else/end/range/with) that render to nothing.
var lineStartLabelRe = regexp.MustCompile(`^\s*(?:{{-?\s*(?:if|else|end|range|with)\b[^}]*}}\s*)*{{-?\s*\.Labels\.(\w+)`)

func lineStartLabels(tmpl string) []string {
	var out []string
	for _, line := range strings.Split(tmpl, "\n") {
		if m := lineStartLabelRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

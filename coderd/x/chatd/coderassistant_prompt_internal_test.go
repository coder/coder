package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestIsCoderAssistantChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name:   "NilMap",
			labels: nil,
			want:   false,
		},
		{
			name:   "EmptyMap",
			labels: map[string]string{},
			want:   false,
		},
		{
			name:   "WrongValue",
			labels: map[string]string{"coder-assistant": "false"},
			want:   false,
		},
		{
			name:   "CorrectValue",
			labels: map[string]string{"coder-assistant": "true"},
			want:   true,
		},
		{
			name: "CorrectValueWithOtherLabels",
			labels: map[string]string{
				"coder-assistant":      "true",
				"coder-assistant-page": "/workspaces",
				"unrelated":            "value",
			},
			want: true,
		},
		{
			name: "OtherLabelsOnly",
			labels: map[string]string{
				"coder-assistant-page": "/workspaces",
				"unrelated":            "true",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, IsCoderAssistantChat(tt.labels))
		})
	}
}

func TestSanitizeCoderAssistantPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		page string
		want string
	}{
		{
			name: "ValidAbsolutePath",
			page: "/workspaces",
			want: "/workspaces",
		},
		{
			name: "ValidNestedPath",
			page: "/templates/docker/versions",
			want: "/templates/docker/versions",
		},
		{
			name: "TrimsSurroundingWhitespace",
			page: "  /workspaces  ",
			want: "/workspaces",
		},
		{
			name: "Empty",
			page: "",
			want: "",
		},
		{
			name: "WhitespaceOnly",
			page: "   ",
			want: "",
		},
		{
			name: "RelativePath",
			page: "workspaces",
			want: "",
		},
		{
			name: "InteriorSpace",
			page: "/work spaces",
			want: "",
		},
		{
			name: "Tab",
			page: "/work\tspaces",
			want: "",
		},
		{
			name: "Newline",
			page: "/workspaces\n/other",
			want: "",
		},
		{
			name: "InteriorCarriageReturn",
			page: "/work\rspaces",
			want: "",
		},
		{
			name: "DoubleQuote",
			page: `/workspaces"`,
			want: "",
		},
		{
			name: "SingleQuote",
			page: "/workspaces'",
			want: "",
		},
		{
			name: "Backtick",
			page: "/workspaces`",
			want: "",
		},
		{
			name: "AngleBrackets",
			page: "/<script>",
			want: "",
		},
		{
			name: "Backslash",
			page: `/workspaces\evil`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, sanitizeCoderAssistantPage(tt.page))
		})
	}
}

func TestCoderAssistantUserContext(t *testing.T) {
	t.Parallel()

	t.Run("FullData", func(t *testing.T) {
		t.Parallel()

		got := CoderAssistantUserContext(
			database.User{Username: "alice", Name: "Alice Smith"},
			[]string{"owner", "template-admin"},
			[]string{"acme", "widgets"},
			"/workspaces",
		)
		require.Contains(t, got, "<user-context>")
		require.Contains(t, got, "</user-context>")
		require.Contains(t, got, "- Username: alice\n")
		require.Contains(t, got, "- Name: Alice Smith\n")
		require.Contains(t, got, "- Deployment roles: owner, template-admin\n")
		require.Contains(t, got, "- Organizations: acme, widgets\n")
		require.Contains(t, got, "currently viewing the /workspaces page")
	})

	t.Run("EmptyNameOmitsNameLine", func(t *testing.T) {
		t.Parallel()

		got := CoderAssistantUserContext(
			database.User{Username: "bob", Name: "   "},
			[]string{"owner"},
			nil,
			"",
		)
		require.Contains(t, got, "- Username: bob\n")
		require.NotContains(t, got, "- Name:")
	})

	t.Run("EmptyRolesRendersMemberFallback", func(t *testing.T) {
		t.Parallel()

		got := CoderAssistantUserContext(
			database.User{Username: "bob"},
			nil,
			nil,
			"",
		)
		require.Contains(t, got, "- Deployment roles: member (no elevated deployment roles)\n")
	})

	t.Run("EmptyOrgsOmitsOrgLine", func(t *testing.T) {
		t.Parallel()

		got := CoderAssistantUserContext(
			database.User{Username: "bob"},
			[]string{"owner"},
			nil,
			"",
		)
		require.NotContains(t, got, "- Organizations:")
	})

	t.Run("InvalidPageOmitted", func(t *testing.T) {
		t.Parallel()

		got := CoderAssistantUserContext(
			database.User{Username: "bob"},
			[]string{"owner"},
			nil,
			"not-absolute",
		)
		require.NotContains(t, got, "currently viewing")
	})
}

package workspacesdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestParseSkillFrontmatter(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		name, desc, body, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: my-skill\ndescription: Does a thing\n---\nBody text here.\n",
		)
		require.NoError(t, err)
		require.Equal(t, "my-skill", name)
		require.Equal(t, "Does a thing", desc)
		require.Equal(t, "Body text here.", body)
	})

	t.Run("QuotedValues", func(t *testing.T) {
		t.Parallel()
		name, desc, _, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: \"quoted-name\"\ndescription: 'single-quoted'\n---\n",
		)
		require.NoError(t, err)
		require.Equal(t, "quoted-name", name)
		require.Equal(t, "single-quoted", desc)
	})

	t.Run("NoDescription", func(t *testing.T) {
		t.Parallel()
		name, desc, body, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: minimal\n---\nSome body.\n",
		)
		require.NoError(t, err)
		require.Equal(t, "minimal", name)
		require.Empty(t, desc)
		require.Equal(t, "Some body.", body)
	})

	t.Run("HTMLCommentsStripped", func(t *testing.T) {
		t.Parallel()
		_, _, body, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: strip-test\n---\nBefore <!-- hidden --> after.\n",
		)
		require.NoError(t, err)
		require.Equal(t, "Before  after.", body)
	})

	t.Run("MultilineHTMLComment", func(t *testing.T) {
		t.Parallel()
		_, _, body, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: multi\n---\nKeep this.\n<!--\nRemove\nall of this.\n-->\nAnd this.\n",
		)
		require.NoError(t, err)
		require.Contains(t, body, "Keep this.")
		require.Contains(t, body, "And this.")
		require.NotContains(t, body, "Remove")
	})

	t.Run("BOMPrefix", func(t *testing.T) {
		t.Parallel()
		name, _, _, err := workspacesdk.ParseSkillFrontmatter(
			"\xef\xbb\xbf---\nname: bom-skill\n---\n",
		)
		require.NoError(t, err)
		require.Equal(t, "bom-skill", name)
	})

	t.Run("EmptyBody", func(t *testing.T) {
		t.Parallel()
		_, _, body, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: nobody\ndescription: has no body\n---\n",
		)
		require.NoError(t, err)
		require.Empty(t, body)
	})

	t.Run("CaseInsensitiveKeys", func(t *testing.T) {
		t.Parallel()
		name, desc, _, err := workspacesdk.ParseSkillFrontmatter(
			"---\nName: upper\nDescription: Also upper\n---\n",
		)
		require.NoError(t, err)
		require.Equal(t, "upper", name)
		require.Equal(t, "Also upper", desc)
	})

	t.Run("UnknownKeysIgnored", func(t *testing.T) {
		t.Parallel()
		name, _, _, err := workspacesdk.ParseSkillFrontmatter(
			"---\nname: test\nauthor: someone\nversion: 1.0\n---\n",
		)
		require.NoError(t, err)
		require.Equal(t, "test", name)
	})

	t.Run("ErrorMissingOpenDelimiter", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := workspacesdk.ParseSkillFrontmatter("no frontmatter here")
		require.ErrorContains(t, err, "missing opening frontmatter delimiter")
	})

	t.Run("ErrorMissingCloseDelimiter", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := workspacesdk.ParseSkillFrontmatter("---\nname: oops\n")
		require.ErrorContains(t, err, "missing closing frontmatter delimiter")
	})

	t.Run("ErrorMissingName", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := workspacesdk.ParseSkillFrontmatter(
			"---\ndescription: no name\n---\n",
		)
		require.ErrorContains(t, err, "frontmatter missing required 'name' field")
	})

	t.Run("WhitespaceAroundDelimiters", func(t *testing.T) {
		t.Parallel()
		name, _, _, err := workspacesdk.ParseSkillFrontmatter(
			"  ---  \nname: spaced\n  ---  \n",
		)
		require.NoError(t, err)
		require.Equal(t, "spaced", name)
	})
}

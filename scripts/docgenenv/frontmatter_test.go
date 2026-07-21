package docgenenv_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestFrontMatter(t *testing.T) {
	t.Parallel()

	t.Run("TitleOnly", func(t *testing.T) {
		t.Parallel()

		got := docgenenv.FrontMatter(docgenenv.Route{Title: "General"})
		require.Equal(t, "---\ntitle: General\n---\n\n", got)
	})

	// AllFields exercises every optional branch, including icon_path (the
	// curated field the index pages rely on, previously uncovered) and the
	// state sequence.
	t.Run("AllFields", func(t *testing.T) {
		t.Parallel()

		got := docgenenv.FrontMatter(docgenenv.Route{
			Title:       "REST API",
			Description: "Learn how to use Coderd API",
			IconPath:    "./images/icons/api.svg",
			State:       []string{"early access"},
		})
		want := "---\n" +
			"title: REST API\n" +
			"description: Learn how to use Coderd API\n" +
			`icon_path: "./images/icons/api.svg"` + "\n" +
			"state:\n" +
			"  - early access\n" +
			"---\n\n"
		require.Equal(t, want, got)
	})
}

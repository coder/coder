package parameter_test

import (
	"testing"

	"github.com/coder/coder/coderd/parameter"

	"github.com/stretchr/testify/require"
)

func TestPlaintext(t *testing.T) {
	t.Parallel()
	t.Run("Simple", func(t *testing.T) {
		t.Parallel()

		mdDescription := `# Provide the machine image
See the [registry](https://container.registry.blah/namespace) for options.

![Minion](https://octodex.github.com/images/minion.png)

**This is bold text.**
__This is bold text.__
*This is italic text.*
> Blockquotes can also be nested.
~~Strikethrough.~~

1. Lorem ipsum dolor sit amet.
2. Consectetur adipiscing elit.
3. Integer molestie lorem at massa.

` + "`There are also code tags!`"

		expected := "Provide the machine image See the registry for options. Minion This is bold text. This is bold text. This is italic text. Blockquotes can also be nested. Strikethrough. 1. Lorem ipsum dolor sit amet. 2. Consectetur adipiscing elit. 3. Integer molestie lorem at massa. There are also code tags!"

		stripped, err := parameter.Plaintext(mdDescription)
		require.NoError(t, err)
		require.Equal(t, expected, stripped)
	})

	t.Run("Nothing changes", func(t *testing.T) {
		t.Parallel()

		nothingChanges := "This is a simple description, so nothing changes."

		stripped, err := parameter.Plaintext(nothingChanges)
		require.NoError(t, err)
		require.Equal(t, nothingChanges, stripped)
	})
}

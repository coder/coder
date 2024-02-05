package parameter_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/parameter"

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

		expected := "Provide the machine image\nSee the registry (https://container.registry.blah/namespace) for options.\n\nMinion (https://octodex.github.com/images/minion.png)\n\nThis is bold text.\nThis is bold text.\nThis is italic text.\n\nBlockquotes can also be nested.\nStrikethrough.\n\n1. Lorem ipsum dolor sit amet.\n2. Consectetur adipiscing elit.\n3. Integer molestie lorem at massa.\n\nThere are also code tags!"

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

func TestHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple",
			input:    `**Coder** is in *early access* mode. To ~~register~~ request access, fill out [this form](https://internal.example.com). ***Thank you!***`,
			expected: `<p><strong>Coder</strong> is in <em>early access</em> mode. To <del>register</del> request access, fill out <a href="https://internal.example.com">this form</a>. <strong><em>Thank you!</em></strong></p>`,
		},
		{
			name:     "Tricky",
			input:    `**Cod*er** is in *early a**ccess** <img src="foobar">mode`,
			expected: `<p><strong>Cod*er</strong> is in *early a<strong>ccess</strong> mode</p>`,
		},
		{
			name:     "XSS",
			input:    `<p onclick="alert(\"omghax\")">Click here to get access!</p>?`,
			expected: `<p>Click here to get access!?</p>`,
		},
		{
			name:     "No Markdown tags",
			input:    "This is a simple description, so nothing changes.",
			expected: "<p>This is a simple description, so nothing changes.</p>",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered := parameter.HTML(tt.input)
			require.Equal(t, tt.expected, rendered)
		})
	}
}

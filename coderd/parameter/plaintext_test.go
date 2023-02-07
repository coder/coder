package parameter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/parameter"
)

func TestPlaintext(t *testing.T) {
	t.Parallel()
	t.Run("Simple", func(t *testing.T) {
		t.Parallel()

		mdDescription := `# Provide the machine image
		See the [registry](https://container.registry.blah/namespace) for options.
		`
		stripped := parameter.Plaintext(mdDescription)
		require.Equal(t, "AAA", stripped)
	})
}

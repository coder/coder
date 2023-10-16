package provisioner_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner"
)

func TestValidAppSlugRegex(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		validStrings := []string{
			"a",
			"1",
			"a1",
			"1a",
			"1a1",
			"1-1",
			"a-a",
			"ab-cd",
			"ab-cd-ef",
			"abc-123",
			"a-123",
			"abc-1",
			"ab-c",
			"a-bc",
		}

		for _, s := range validStrings {
			require.True(t, provisioner.AppSlugRegex.MatchString(s), s)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		invalidStrings := []string{
			"",
			"-",
			"-abc",
			"abc-",
			"ab--cd",
			"a--bc",
			"ab--c",
			"_",
			"ab_cd",
			"_abc",
			"abc_",
			" ",
			"abc ",
			" abc",
			"ab cd",
		}

		for _, s := range invalidStrings {
			require.False(t, provisioner.AppSlugRegex.MatchString(s), s)
		}
	})
}

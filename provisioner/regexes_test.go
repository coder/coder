package provisioner_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner"
)

var (
	validStrings = []string{
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

	invalidStrings = []string{
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

	uppercaseStrings = []string{
		"A",
		"A1",
		"1A",
	}
)

func TestAgentNameRegex(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		for _, s := range append(validStrings, uppercaseStrings...) {
			require.True(t, provisioner.AgentNameRegex.MatchString(s), s)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		for _, s := range invalidStrings {
			require.False(t, provisioner.AgentNameRegex.MatchString(s), s)
		}
	})
}

func TestAppSlugRegex(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		for _, s := range validStrings {
			require.True(t, provisioner.AppSlugRegex.MatchString(s), s)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		for _, s := range append(invalidStrings, uppercaseStrings...) {
			require.False(t, provisioner.AppSlugRegex.MatchString(s), s)
		}
	})
}

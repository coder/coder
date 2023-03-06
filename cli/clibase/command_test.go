package clibase_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clibase/clibasetest"
)

func TestCommand_ToUpper(t *testing.T) {
	t.Parallel()

	cmd := &clibase.Command{
		Use:   "toupper [word]",
		Short: "Converts a word to upper case",
		Handler: clibase.Chain(
			clibase.HandlerFunc(func(i *clibase.Invokation) {
				_, _ = i.Stdout.Write(
					[]byte(
						strings.ToUpper(i.Args[0]),
					),
				)
			}),
			clibase.RequireNArgs(1),
		),
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"hello"},
			Command: cmd,
		}
		io := clibasetest.FakeIO(i)
		i.Run()
		require.Equal(t, "HELLO", io.Stdout.String())
	})

	t.Run("BadArgs", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"hello", "world"},
			Command: cmd,
		}
		io := clibasetest.FakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})
}

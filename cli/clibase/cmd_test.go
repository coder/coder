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

	cmd := func() *clibase.Cmd {
		var (
			verbose clibase.Bool
			lower   clibase.Bool
		)
		return &clibase.Cmd{
			Use: "root [subcommand]",
			Options: clibase.OptionSet{
				clibase.Option{
					Name:  "verbose",
					Flag:  "verbose",
					Value: &verbose,
				},
			},
			Children: []*clibase.Cmd{
				{
					Use:   "toupper [word]",
					Short: "Converts a word to upper case",
					Middleware: clibase.Chain(
						clibase.RequireNArgs(1),
					),
					Options: clibase.OptionSet{
						clibase.Option{
							Name:  "lower",
							Flag:  "lower",
							Value: &lower,
						},
					},
					Handler: clibase.HandlerFunc(func(i *clibase.Invokation) error {
						w := i.Args[0]
						if lower {
							w = strings.ToLower(w)
						} else {
							w = strings.ToUpper(w)
						}
						_, _ = i.Stdout.Write(
							[]byte(
								w,
							),
						)
						if verbose {
							i.Stdout.Write([]byte("!!!"))
						}
						return nil
					}),
				},
			},
		}
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"root", "toupper", "hello"},
			Command: cmd(),
		}
		io := clibasetest.FakeIO(i)
		i.Run()
		require.Equal(t, "HELLO", io.Stdout.String())
	})

	t.Run("NoSubcommand", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"root", "na"},
			Command: cmd(),
		}
		io := clibasetest.FakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("BadArgs", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"root", "toupper"},
			Command: cmd(),
		}
		io := clibasetest.FakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("Verbose", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"root", "--verbose", "toupper", "hello"},
			Command: cmd(),
		}
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "HELLO!!!", io.Stdout.String())
	})

	t.Run("VerboseAnywhere", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"root", "toupper", "--verbose", "hello"},
			Command: cmd(),
		}
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "HELLO!!!", io.Stdout.String())
	})

	t.Run("LowerVerbose", func(t *testing.T) {
		t.Parallel()
		i := &clibase.Invokation{
			Args:    []string{"root", "toupper", "--verbose", "hello", "--lower"},
			Command: cmd(),
		}
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "hello!!!", io.Stdout.String())
	})
}

func TestCommand_MiddlewareOrder(t *testing.T) {
	t.Parallel()

	mw := func(letter string) clibase.MiddlewareFunc {
		return func(next clibase.HandlerFunc) clibase.HandlerFunc {
			return clibase.HandlerFunc(func(i *clibase.Invokation) error {
				_, _ = i.Stdout.Write([]byte(letter))
				return next(i)
			})
		}
	}

	cmd := &clibase.Cmd{
		Use:   "toupper [word]",
		Short: "Converts a word to upper case",
		Middleware: clibase.Chain(
			mw("A"),
			mw("B"),
			mw("C"),
		),
		Handler: clibase.HandlerFunc(func(i *clibase.Invokation) error {
			return nil
		}),
	}

	i := &clibase.Invokation{
		Args:    []string{"hello", "world"},
		Command: cmd,
	}
	io := clibasetest.FakeIO(i)
	require.NoError(t, i.Run())
	require.Equal(t, "ABC", io.Stdout.String())
}

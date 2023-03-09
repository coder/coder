package clibase_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clibase/clibasetest"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := func() *clibase.Cmd {
		var (
			verbose bool
			lower   bool
			prefix  string
		)
		return &clibase.Cmd{
			Use: "root [subcommand]",
			Options: clibase.OptionSet{
				clibase.Option{
					Name:  "verbose",
					Flag:  "verbose",
					Value: clibase.BoolOf(&verbose),
				},
				clibase.Option{
					Name:  "prefix",
					Flag:  "prefix",
					Value: clibase.StringOf(&prefix),
				},
			},
			Children: []*clibase.Cmd{
				{
					Use:   "toupper [word]",
					Short: "Converts a word to upper case",
					Middleware: clibase.Chain(
						clibase.RequireNArgs(1),
					),
					Aliases: []string{"up"},
					Options: clibase.OptionSet{
						clibase.Option{
							Name:  "lower",
							Flag:  "lower",
							Value: clibase.BoolOf(&lower),
						},
					},
					Handler: (func(i *clibase.Invokation) error {
						i.Stdout.Write([]byte(prefix))
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

	t.Run("SimpleOK", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke("toupper", "hello")
		io := clibasetest.FakeIO(i)
		i.Run()
		require.Equal(t, "HELLO", io.Stdout.String())
	})

	t.Run("Alias", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"up", "hello",
		)
		io := clibasetest.FakeIO(i)
		i.Run()
		require.Equal(t, "HELLO", io.Stdout.String())
	})

	t.Run("NoSubcommand", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"na",
		)
		io := clibasetest.FakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("BadArgs", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper",
		)
		io := clibasetest.FakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("UnknownFlags", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--unknown",
		)
		io := clibasetest.FakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("Verbose", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"--verbose", "toupper", "hello",
		)
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "HELLO!!!", io.Stdout.String())
	})

	t.Run("Verbose=", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"--verbose=true", "toupper", "hello",
		)
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "HELLO!!!", io.Stdout.String())
	})

	t.Run("PrefixSpace", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"--prefix", "conv: ", "toupper", "hello",
		)
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "conv: HELLO", io.Stdout.String())
	})

	t.Run("GlobalFlagsAnywhere", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--prefix", "conv: ", "hello", "--verbose",
		)
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "conv: HELLO!!!", io.Stdout.String())
	})

	t.Run("LowerVerbose", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--verbose", "hello", "--lower",
		)
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "hello!!!", io.Stdout.String())
	})

	t.Run("ParsedFlags", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--verbose", "hello", "--lower",
		)
		_ = clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t,
			"true",
			i.ParsedFlags().Lookup("verbose").Value.String(),
		)
	})

	t.Run("NoDeepChild", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"root", "level", "level", "toupper", "--verbose", "hello", "--lower",
		)
		fio := clibasetest.FakeIO(i)
		require.Error(t, i.Run(), fio.Stdout.String())
	})
}

func TestCommand_MiddlewareOrder(t *testing.T) {
	t.Parallel()

	mw := func(letter string) clibase.MiddlewareFunc {
		return func(next clibase.HandlerFunc) clibase.HandlerFunc {
			return (func(i *clibase.Invokation) error {
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
		Handler: (func(i *clibase.Invokation) error {
			return nil
		}),
	}

	i := cmd.Invoke(
		"hello", "world",
	)
	io := clibasetest.FakeIO(i)
	require.NoError(t, i.Run())
	require.Equal(t, "ABC", io.Stdout.String())
}

func TestCommand_RawArgs(t *testing.T) {
	t.Parallel()

	cmd := func() *clibase.Cmd {
		return &clibase.Cmd{
			Use: "root",
			Options: []clibase.Option{
				{
					Name:  "password",
					Flag:  "password",
					Value: clibase.StringOf(new(string)),
				},
			},
			Children: []*clibase.Cmd{
				{
					Use:     "sushi <args...>",
					Short:   "Throws back raw output",
					RawArgs: true,
					Handler: (func(i *clibase.Invokation) error {
						if v := i.ParsedFlags().Lookup("password").Value.String(); v != "codershack" {
							return xerrors.Errorf("password %q is wrong!", v)
						}
						i.Stdout.Write([]byte(strings.Join(i.Args, " ")))
						return nil
					}),
				},
			},
		}
	}

	t.Run("OK", func(t *testing.T) {
		// Flag parsed before the raw arg command should still work.
		t.Parallel()

		i := cmd().Invoke(
			"--password", "codershack", "sushi", "hello", "--verbose", "world",
		)
		io := clibasetest.FakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "hello --verbose world", io.Stdout.String())
	})

	t.Run("BadFlag", func(t *testing.T) {
		// Verbose before the raw arg command should fail.
		t.Parallel()

		i := cmd().Invoke(
			"--password", "codershack", "--verbose", "sushi", "hello", "world",
		)
		io := clibasetest.FakeIO(i)
		require.Error(t, i.Run())
		require.Empty(t, io.Stdout.String())
	})

	t.Run("NoPassword", func(t *testing.T) {
		// Flag parsed before the raw arg command should still work.
		t.Parallel()
		i := cmd().Invoke(
			"sushi", "hello", "--verbose", "world",
		)
		_ = clibasetest.FakeIO(i)
		i.Stdout = clibasetest.TestWriter(t, "stdout: ")
		require.Error(t, i.Run())
	})
}

func TestCommand_RootRaw(t *testing.T) {
	t.Parallel()
	cmd := &clibase.Cmd{
		RawArgs: true,
		Handler: func(i *clibase.Invokation) error {
			i.Stdout.Write([]byte(strings.Join(i.Args, " ")))
			return nil
		},
	}

	inv, stdio := clibasetest.Invoke(cmd, "hello", "--verbose", "--friendly")
	err := inv.Run()
	require.NoError(t, err)

	require.Equal(t, "hello --verbose --friendly", stdio.Stdout.String())
}

func TestCommand_HyphenHypen(t *testing.T) {
	t.Parallel()
	cmd := &clibase.Cmd{
		Handler: (func(i *clibase.Invokation) error {
			i.Stdout.Write([]byte(strings.Join(i.Args, " ")))
			return nil
		}),
	}

	inv, stdio := clibasetest.Invoke(cmd, "--", "--verbose", "--friendly")
	err := inv.Run()
	require.NoError(t, err)

	require.Equal(t, "--verbose --friendly", stdio.Stdout.String())
}

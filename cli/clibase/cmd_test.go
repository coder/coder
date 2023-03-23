package clibase_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
)

// ioBufs is the standard input, output, and error for a command.
type ioBufs struct {
	Stdin  bytes.Buffer
	Stdout bytes.Buffer
	Stderr bytes.Buffer
}

// fakeIO sets Stdin, Stdout, and Stderr to buffers.
func fakeIO(i *clibase.Invocation) *ioBufs {
	var b ioBufs
	i.Stdout = &b.Stdout
	i.Stderr = &b.Stderr
	i.Stdin = &b.Stdin
	return &b
}

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
					Handler: (func(i *clibase.Invocation) error {
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
		io := fakeIO(i)
		i.Run()
		require.Equal(t, "HELLO", io.Stdout.String())
	})

	t.Run("Alias", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"up", "hello",
		)
		io := fakeIO(i)
		i.Run()
		require.Equal(t, "HELLO", io.Stdout.String())
	})

	t.Run("NoSubcommand", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"na",
		)
		io := fakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("BadArgs", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper",
		)
		io := fakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("UnknownFlags", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--unknown",
		)
		io := fakeIO(i)
		err := i.Run()
		require.Empty(t, io.Stdout.String())
		require.Error(t, err)
	})

	t.Run("Verbose", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"--verbose", "toupper", "hello",
		)
		io := fakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "HELLO!!!", io.Stdout.String())
	})

	t.Run("Verbose=", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"--verbose=true", "toupper", "hello",
		)
		io := fakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "HELLO!!!", io.Stdout.String())
	})

	t.Run("PrefixSpace", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"--prefix", "conv: ", "toupper", "hello",
		)
		io := fakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "conv: HELLO", io.Stdout.String())
	})

	t.Run("GlobalFlagsAnywhere", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--prefix", "conv: ", "hello", "--verbose",
		)
		io := fakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "conv: HELLO!!!", io.Stdout.String())
	})

	t.Run("LowerVerbose", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--verbose", "hello", "--lower",
		)
		io := fakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "hello!!!", io.Stdout.String())
	})

	t.Run("ParsedFlags", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"toupper", "--verbose", "hello", "--lower",
		)
		_ = fakeIO(i)
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
		fio := fakeIO(i)
		require.Error(t, i.Run(), fio.Stdout.String())
	})
}

func TestCommand_DeepNest(t *testing.T) {
	t.Parallel()
	cmd := &clibase.Cmd{
		Use: "1",
		Children: []*clibase.Cmd{
			{
				Use: "2",
				Children: []*clibase.Cmd{
					{
						Use: "3",
						Handler: func(i *clibase.Invocation) error {
							i.Stdout.Write([]byte("3"))
							return nil
						},
					},
				},
			},
		},
	}
	inv := cmd.Invoke("2", "3")
	stdio := fakeIO(inv)
	err := inv.Run()
	require.NoError(t, err)
	require.Equal(t, "3", stdio.Stdout.String())
}

func TestCommand_FlagOverride(t *testing.T) {
	t.Parallel()
	var flag string

	cmd := &clibase.Cmd{
		Use: "1",
		Options: clibase.OptionSet{
			{
				Flag:  "f",
				Value: clibase.DiscardValue,
			},
		},
		Children: []*clibase.Cmd{
			{
				Use: "2",
				Options: clibase.OptionSet{
					{
						Flag:  "f",
						Value: clibase.StringOf(&flag),
					},
				},
				Handler: func(i *clibase.Invocation) error {
					return nil
				},
			},
		},
	}

	err := cmd.Invoke("2", "--f", "mhmm").Run()
	require.NoError(t, err)

	require.Equal(t, "mhmm", flag)
}

func TestCommand_MiddlewareOrder(t *testing.T) {
	t.Parallel()

	mw := func(letter string) clibase.MiddlewareFunc {
		return func(next clibase.HandlerFunc) clibase.HandlerFunc {
			return (func(i *clibase.Invocation) error {
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
		Handler: (func(i *clibase.Invocation) error {
			return nil
		}),
	}

	i := cmd.Invoke(
		"hello", "world",
	)
	io := fakeIO(i)
	require.NoError(t, i.Run())
	require.Equal(t, "ABC", io.Stdout.String())
}

func TestCommand_RawArgs(t *testing.T) {
	t.Parallel()

	cmd := func() *clibase.Cmd {
		return &clibase.Cmd{
			Use: "root",
			Options: clibase.OptionSet{
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
					Handler: (func(i *clibase.Invocation) error {
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
		io := fakeIO(i)
		require.NoError(t, i.Run())
		require.Equal(t, "hello --verbose world", io.Stdout.String())
	})

	t.Run("BadFlag", func(t *testing.T) {
		// Verbose before the raw arg command should fail.
		t.Parallel()

		i := cmd().Invoke(
			"--password", "codershack", "--verbose", "sushi", "hello", "world",
		)
		io := fakeIO(i)
		require.Error(t, i.Run())
		require.Empty(t, io.Stdout.String())
	})

	t.Run("NoPassword", func(t *testing.T) {
		// Flag parsed before the raw arg command should still work.
		t.Parallel()
		i := cmd().Invoke(
			"sushi", "hello", "--verbose", "world",
		)
		_ = fakeIO(i)
		require.Error(t, i.Run())
	})
}

func TestCommand_RootRaw(t *testing.T) {
	t.Parallel()
	cmd := &clibase.Cmd{
		RawArgs: true,
		Handler: func(i *clibase.Invocation) error {
			i.Stdout.Write([]byte(strings.Join(i.Args, " ")))
			return nil
		},
	}

	inv := cmd.Invoke("hello", "--verbose", "--friendly")
	stdio := fakeIO(inv)
	err := inv.Run()
	require.NoError(t, err)

	require.Equal(t, "hello --verbose --friendly", stdio.Stdout.String())
}

func TestCommand_HyphenHyphen(t *testing.T) {
	t.Parallel()
	cmd := &clibase.Cmd{
		Handler: (func(i *clibase.Invocation) error {
			i.Stdout.Write([]byte(strings.Join(i.Args, " ")))
			return nil
		}),
	}

	inv := cmd.Invoke("--", "--verbose", "--friendly")
	stdio := fakeIO(inv)
	err := inv.Run()
	require.NoError(t, err)

	require.Equal(t, "--verbose --friendly", stdio.Stdout.String())
}

func TestCommand_ContextCancels(t *testing.T) {
	t.Parallel()

	var gotCtx context.Context

	cmd := &clibase.Cmd{
		Handler: (func(i *clibase.Invocation) error {
			gotCtx = i.Context()
			if err := gotCtx.Err(); err != nil {
				return xerrors.Errorf("unexpected context error: %w", i.Context().Err())
			}
			return nil
		}),
	}

	err := cmd.Invoke().Run()
	require.NoError(t, err)

	require.Error(t, gotCtx.Err())
}

func TestCommand_Help(t *testing.T) {
	t.Parallel()

	cmd := func() *clibase.Cmd {
		return &clibase.Cmd{
			Use: "root",
			HelpHandler: (func(i *clibase.Invocation) error {
				i.Stdout.Write([]byte("abdracadabra"))
				return nil
			}),
			Handler: (func(i *clibase.Invocation) error {
				return xerrors.New("should not be called")
			}),
		}
	}

	t.Run("NoHandler", func(t *testing.T) {
		t.Parallel()

		c := cmd()
		c.HelpHandler = nil
		err := c.Invoke("--help").Run()
		require.Error(t, err)
	})

	t.Run("Long", func(t *testing.T) {
		t.Parallel()

		inv := cmd().Invoke("--help")
		stdio := fakeIO(inv)
		err := inv.Run()
		require.NoError(t, err)

		require.Contains(t, stdio.Stdout.String(), "abdracadabra")
	})

	t.Run("Short", func(t *testing.T) {
		t.Parallel()

		inv := cmd().Invoke("-h")
		stdio := fakeIO(inv)
		err := inv.Run()
		require.NoError(t, err)

		require.Contains(t, stdio.Stdout.String(), "abdracadabra")
	})
}

func TestCommand_SliceFlags(t *testing.T) {
	t.Parallel()

	cmd := func(want ...string) *clibase.Cmd {
		var got []string
		return &clibase.Cmd{
			Use: "root",
			Options: clibase.OptionSet{
				{
					Name:    "arr",
					Flag:    "arr",
					Default: "bad,bad,bad",
					Value:   clibase.StringArrayOf(&got),
				},
			},
			Handler: (func(i *clibase.Invocation) error {
				require.Equal(t, want, got)
				return nil
			}),
		}
	}

	err := cmd("good", "good", "good").Invoke("--arr", "good", "--arr", "good", "--arr", "good").Run()
	require.NoError(t, err)

	err = cmd("bad", "bad", "bad").Invoke().Run()
	require.NoError(t, err)
}

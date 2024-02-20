package clibase_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
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
			reqBool bool
			reqStr  string
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
					Use:   "required-flag --req-bool=true --req-string=foo",
					Short: "Example with required flags",
					Options: clibase.OptionSet{
						clibase.Option{
							Name:     "req-bool",
							Flag:     "req-bool",
							Value:    clibase.BoolOf(&reqBool),
							Required: true,
						},
						clibase.Option{
							Name: "req-string",
							Flag: "req-string",
							Value: clibase.Validate(clibase.StringOf(&reqStr), func(value *clibase.String) error {
								ok := strings.Contains(value.String(), " ")
								if !ok {
									return xerrors.Errorf("string must contain a space")
								}
								return nil
							}),
							Required: true,
						},
					},
					HelpHandler: func(i *clibase.Invocation) error {
						_, _ = i.Stdout.Write([]byte("help text.png"))
						return nil
					},
					Handler: func(i *clibase.Invocation) error {
						_, _ = i.Stdout.Write([]byte(fmt.Sprintf("%s-%t", reqStr, reqBool)))
						return nil
					},
				},
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
					Handler: func(i *clibase.Invocation) error {
						_, _ = i.Stdout.Write([]byte(prefix))
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
					},
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

	t.Run("RequiredFlagsMissing", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"required-flag",
		)
		fio := fakeIO(i)
		err := i.Run()
		require.Error(t, err, fio.Stdout.String())
		require.ErrorContains(t, err, "Missing values")
	})

	t.Run("RequiredFlagsMissingWithHelp", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"required-flag",
			"--help",
		)
		fio := fakeIO(i)
		err := i.Run()
		require.NoError(t, err)
		require.Contains(t, fio.Stdout.String(), "help text.png")
	})

	t.Run("RequiredFlagsMissingBool", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"required-flag", "--req-string", "foo bar",
		)
		fio := fakeIO(i)
		err := i.Run()
		require.Error(t, err, fio.Stdout.String())
		require.ErrorContains(t, err, "Missing values for the required flags: req-bool")
	})

	t.Run("RequiredFlagsMissingString", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"required-flag", "--req-bool", "true",
		)
		fio := fakeIO(i)
		err := i.Run()
		require.Error(t, err, fio.Stdout.String())
		require.ErrorContains(t, err, "Missing values for the required flags: req-string")
	})

	t.Run("RequiredFlagsInvalid", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"required-flag", "--req-string", "nospace",
		)
		fio := fakeIO(i)
		err := i.Run()
		require.Error(t, err, fio.Stdout.String())
		require.ErrorContains(t, err, "string must contain a space")
	})

	t.Run("RequiredFlagsOK", func(t *testing.T) {
		t.Parallel()
		i := cmd().Invoke(
			"required-flag", "--req-bool", "true", "--req-string", "foo bar",
		)
		fio := fakeIO(i)
		err := i.Run()
		require.NoError(t, err, fio.Stdout.String())
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
				Name:  "flag",
				Flag:  "f",
				Value: clibase.DiscardValue,
			},
		},
		Children: []*clibase.Cmd{
			{
				Use: "2",
				Options: clibase.OptionSet{
					{
						Name:  "flag",
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

func TestCommand_EmptySlice(t *testing.T) {
	t.Parallel()

	cmd := func(want ...string) *clibase.Cmd {
		var got []string
		return &clibase.Cmd{
			Use: "root",
			Options: clibase.OptionSet{
				{
					Name:    "arr",
					Flag:    "arr",
					Default: "def,def,def",
					Env:     "ARR",
					Value:   clibase.StringArrayOf(&got),
				},
			},
			Handler: (func(i *clibase.Invocation) error {
				require.Equal(t, want, got)
				return nil
			}),
		}
	}

	// Base-case, uses default.
	err := cmd("def", "def", "def").Invoke().Run()
	require.NoError(t, err)

	// Empty-env uses default, too.
	inv := cmd("def", "def", "def").Invoke()
	inv.Environ.Set("ARR", "")
	require.NoError(t, err)

	// Reset to nothing at all via flag.
	inv = cmd().Invoke("--arr", "")
	inv.Environ.Set("ARR", "cant see")
	err = inv.Run()
	require.NoError(t, err)

	// Reset to a specific value with flag.
	inv = cmd("great").Invoke("--arr", "great")
	inv.Environ.Set("ARR", "")
	err = inv.Run()
	require.NoError(t, err)
}

func TestCommand_DefaultsOverride(t *testing.T) {
	t.Parallel()

	test := func(name string, want string, fn func(t *testing.T, inv *clibase.Invocation)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				got    string
				config clibase.YAMLConfigPath
			)
			cmd := &clibase.Cmd{
				Options: clibase.OptionSet{
					{
						Name:    "url",
						Flag:    "url",
						Default: "def.com",
						Env:     "URL",
						Value:   clibase.StringOf(&got),
						YAML:    "url",
					},
					{
						Name:    "config",
						Flag:    "config",
						Default: "",
						Value:   &config,
					},
				},
				Handler: (func(i *clibase.Invocation) error {
					_, _ = fmt.Fprintf(i.Stdout, "%s", got)
					return nil
				}),
			}

			inv := cmd.Invoke()
			stdio := fakeIO(inv)
			fn(t, inv)
			err := inv.Run()
			require.NoError(t, err)
			require.Equal(t, want, stdio.Stdout.String())
		})
	}

	test("DefaultOverNothing", "def.com", func(t *testing.T, inv *clibase.Invocation) {})

	test("FlagOverDefault", "good.com", func(t *testing.T, inv *clibase.Invocation) {
		inv.Args = []string{"--url", "good.com"}
	})

	test("EnvOverDefault", "good.com", func(t *testing.T, inv *clibase.Invocation) {
		inv.Environ.Set("URL", "good.com")
	})

	test("FlagOverEnv", "good.com", func(t *testing.T, inv *clibase.Invocation) {
		inv.Environ.Set("URL", "bad.com")
		inv.Args = []string{"--url", "good.com"}
	})

	test("FlagOverYAML", "good.com", func(t *testing.T, inv *clibase.Invocation) {
		fi, err := os.CreateTemp(t.TempDir(), "config.yaml")
		require.NoError(t, err)
		defer fi.Close()

		_, err = fi.WriteString("url: bad.com")
		require.NoError(t, err)

		inv.Args = []string{"--config", fi.Name(), "--url", "good.com"}
	})

	test("YAMLOverDefault", "good.com", func(t *testing.T, inv *clibase.Invocation) {
		fi, err := os.CreateTemp(t.TempDir(), "config.yaml")
		require.NoError(t, err)
		defer fi.Close()

		_, err = fi.WriteString("url: good.com")
		require.NoError(t, err)

		inv.Args = []string{"--config", fi.Name()}
	})
}

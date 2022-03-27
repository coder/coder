package cliflags_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliflags"
	"github.com/coder/coder/cryptorand"
)

func TestCliFlags(t *testing.T) {
	t.Parallel()

	t.Run("StringDefault", func(t *testing.T) {
		t.Parallel()

		var p string
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		def, _ := cryptorand.String(10)
		usage, _ := cryptorand.String(10)

		cliflags.String(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetString(name)
		require.NoError(t, err)
		require.Equal(t, def, got)
		require.Contains(t, flagset.FlagUsages(), usage)
		require.Contains(t, flagset.FlagUsages(), fmt.Sprintf("(uses $%s).", env))
	})

	t.Run("StringEnvVar", func(t *testing.T) {
		t.Parallel()

		var p string
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		envValue, _ := cryptorand.String(10)
		os.Setenv(env, envValue)
		defer os.Unsetenv(env)
		def, _ := cryptorand.String(10)
		usage, _ := cryptorand.String(10)

		cliflags.String(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetString(name)
		require.NoError(t, err)
		require.Equal(t, envValue, got)
	})

	t.Run("EmptyEnvVar", func(t *testing.T) {
		t.Parallel()

		var p string
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env := ""
		def, _ := cryptorand.String(10)
		usage, _ := cryptorand.String(10)

		cliflags.String(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetString(name)
		require.NoError(t, err)
		require.Equal(t, def, got)
		require.Contains(t, flagset.FlagUsages(), usage)
		require.NotContains(t, flagset.FlagUsages(), fmt.Sprintf("(uses $%s).", env))
	})

	t.Run("IntDefault", func(t *testing.T) {
		t.Parallel()

		var p int
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		def, _ := cryptorand.Int()
		usage, _ := cryptorand.String(10)

		cliflags.Int(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetInt(name)
		require.NoError(t, err)
		require.Equal(t, def, got)
		require.Contains(t, flagset.FlagUsages(), usage)
		require.Contains(t, flagset.FlagUsages(), fmt.Sprintf("(uses $%s).", env))
	})

	t.Run("IntEnvVar", func(t *testing.T) {
		t.Parallel()

		var p int
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		envValue, _ := cryptorand.Int()
		os.Setenv(env, strconv.Itoa(envValue))
		defer os.Unsetenv(env)
		def, _ := cryptorand.Int()
		usage, _ := cryptorand.String(10)

		cliflags.Int(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetInt(name)
		require.NoError(t, err)
		require.Equal(t, envValue, got)
	})

	t.Run("IntFailParse", func(t *testing.T) {
		t.Parallel()

		var p int
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		envValue, _ := cryptorand.String(10)
		os.Setenv(env, envValue)
		defer os.Unsetenv(env)
		def, _ := cryptorand.Int()
		usage, _ := cryptorand.String(10)

		cliflags.Int(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetInt(name)
		require.NoError(t, err)
		require.Equal(t, def, got)
	})

	t.Run("BoolDefault", func(t *testing.T) {
		t.Parallel()

		var p bool
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		def, _ := cryptorand.Bool()
		usage, _ := cryptorand.String(10)

		cliflags.Bool(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetBool(name)
		require.NoError(t, err)
		require.Equal(t, def, got)
		require.Contains(t, flagset.FlagUsages(), usage)
		require.Contains(t, flagset.FlagUsages(), fmt.Sprintf("(uses $%s).", env))
	})

	t.Run("BoolEnvVar", func(t *testing.T) {
		t.Parallel()

		var p bool
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		envValue, _ := cryptorand.Bool()
		os.Setenv(env, strconv.FormatBool(envValue))
		defer os.Unsetenv(env)
		def, _ := cryptorand.Bool()
		usage, _ := cryptorand.String(10)

		cliflags.Bool(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetBool(name)
		require.NoError(t, err)
		require.Equal(t, envValue, got)
	})

	t.Run("BoolFailParse", func(t *testing.T) {
		t.Parallel()

		var p bool
		fsname, _ := cryptorand.String(10)
		flagset := pflag.NewFlagSet(fsname, pflag.PanicOnError)
		name, _ := cryptorand.String(10)
		shorthand, _ := cryptorand.String(1)
		env, _ := cryptorand.String(10)
		envValue, _ := cryptorand.String(10)
		os.Setenv(env, envValue)
		defer os.Unsetenv(env)
		def, _ := cryptorand.Bool()
		usage, _ := cryptorand.String(10)

		cliflags.Bool(flagset, &p, name, shorthand, env, def, usage)
		got, err := flagset.GetBool(name)
		require.NoError(t, err)
		require.Equal(t, def, got)
	})
}

// Package cliflags provides helpers for uniform flags, env vars, and usage docs.
// Helpers will set flags to their default value if the environment variable and flag are unset.
// Helpers inject environment variable into flag usage docs if provided.
//
// Usage:
//
// cliflags.String(root.Flags(), &address, "address", "a", "CODER_ADDRESS", "127.0.0.1:3000", "The address to serve the API and dashboard")
//
// Will produce the following usage docs:
//
//   -a, --address string              The address to serve the API and dashboard (uses $CODER_ADDRESS). (default "127.0.0.1:3000")
//
package cliflags

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/pflag"
)

// String sets a string flag on the given flag set.
func String(flagset *pflag.FlagSet, p *string, name string, shorthand string, env string, def string, usage string) {
	flagset.StringVarP(p, name, shorthand, envOrDefaultString(env, def), fmtUsage(usage, env))
}

// Int sets a int flag on the given flag set.
func Int(flagset *pflag.FlagSet, p *int, name string, shorthand string, env string, def int, usage string) {
	flagset.IntVarP(p, name, shorthand, envOrDefaultInt(env, def), fmtUsage(usage, env))
}

// Bool sets a bool flag on the given flag set.
func Bool(flagset *pflag.FlagSet, p *bool, name string, shorthand string, env string, def bool, usage string) {
	flagset.BoolVarP(p, name, shorthand, envOrDefaultBool(env, def), fmtUsage(usage, env))
}

func envOrDefaultString(env string, def string) string {
	v, ok := os.LookupEnv(env)
	if !ok {
		return def
	}

	return v
}

func envOrDefaultInt(env string, def int) int {
	v, ok := os.LookupEnv(env)
	if !ok {
		return def
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}

	return i
}

func envOrDefaultBool(env string, def bool) bool {
	v, ok := os.LookupEnv(env)
	if !ok {
		return def
	}

	i, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}

	return i
}

func fmtUsage(u string, env string) string {
	if env == "" {
		return fmt.Sprintf("%s.", u)
	}

	return fmt.Sprintf("%s (uses $%s).", u, env)
}

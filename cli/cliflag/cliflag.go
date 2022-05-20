// Package cliflag extends flagset with environment variable defaults.
//
// Usage:
//
// cliflag.String(root.Flags(), &address, "address", "a", "CODER_ADDRESS", "127.0.0.1:3000", "The address to serve the API and dashboard")
//
// Will produce the following usage docs:
//
//   -a, --address string              The address to serve the API and dashboard (uses $CODER_ADDRESS). (default "127.0.0.1:3000")
//
package cliflag

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// String sets a string flag on the given flag set.
func String(flagset *pflag.FlagSet, name, shorthand, env, def, usage string) {
	v, ok := os.LookupEnv(env)
	if !ok || v == "" {
		v = def
	}
	flagset.StringP(name, shorthand, v, fmtUsage(usage, env))
}

// StringVarP sets a string flag on the given flag set.
func StringVarP(flagset *pflag.FlagSet, p *string, name string, shorthand string, env string, def string, usage string) {
	v, ok := os.LookupEnv(env)
	if !ok || v == "" {
		v = def
	}
	flagset.StringVarP(p, name, shorthand, v, fmtUsage(usage, env))
}

func StringArrayVarP(flagset *pflag.FlagSet, ptr *[]string, name string, shorthand string, env string, def []string, usage string) {
	val, ok := os.LookupEnv(env)
	if ok {
		if val == "" {
			def = []string{}
		} else {
			def = strings.Split(val, ",")
		}
	}
	flagset.StringArrayVarP(ptr, name, shorthand, def, usage)
}

// Uint8VarP sets a uint8 flag on the given flag set.
func Uint8VarP(flagset *pflag.FlagSet, ptr *uint8, name string, shorthand string, env string, def uint8, usage string) {
	val, ok := os.LookupEnv(env)
	if !ok || val == "" {
		flagset.Uint8VarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	vi64, err := strconv.ParseUint(val, 10, 8)
	if err != nil {
		flagset.Uint8VarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	flagset.Uint8VarP(ptr, name, shorthand, uint8(vi64), fmtUsage(usage, env))
}

// BoolVarP sets a bool flag on the given flag set.
func BoolVarP(flagset *pflag.FlagSet, ptr *bool, name string, shorthand string, env string, def bool, usage string) {
	val, ok := os.LookupEnv(env)
	if !ok || val == "" {
		flagset.BoolVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	valb, err := strconv.ParseBool(val)
	if err != nil {
		flagset.BoolVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	flagset.BoolVarP(ptr, name, shorthand, valb, fmtUsage(usage, env))
}

// DurationVarP sets a time.Duration flag on the given flag set.
func DurationVarP(flagset *pflag.FlagSet, ptr *time.Duration, name string, shorthand string, env string, def time.Duration, usage string) {
	val, ok := os.LookupEnv(env)
	if !ok || val == "" {
		flagset.DurationVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	valb, err := time.ParseDuration(val)
	if err != nil {
		flagset.DurationVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	flagset.DurationVarP(ptr, name, shorthand, valb, fmtUsage(usage, env))
}

func fmtUsage(u string, env string) string {
	if env == "" {
		return fmt.Sprintf("%s.", u)
	}

	return fmt.Sprintf("%s - consumes $%s.", u, env)
}

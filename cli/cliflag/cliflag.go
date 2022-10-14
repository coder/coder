// Package cliflag extends flagset with environment variable defaults.
//
// Usage:
//
// cliflag.String(root.Flags(), &address, "address", "a", "CODER_ADDRESS", "127.0.0.1:3000", "The address to serve the API and dashboard")
//
// Will produce the following usage docs:
//
//	-a, --address string              The address to serve the API and dashboard (uses $CODER_ADDRESS). (default "127.0.0.1:3000")
package cliflag

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/coder/coder/cli/cliui"
)

// IsSetBool returns the value of the boolean flag if it is set.
// It returns false if the flag isn't set or if any error occurs attempting
// to parse the value of the flag.
func IsSetBool(cmd *cobra.Command, name string) bool {
	val, ok := IsSet(cmd, name)
	if !ok {
		return false
	}

	b, err := strconv.ParseBool(val)
	return err == nil && b
}

// IsSet returns the string value of the flag and whether it was set.
func IsSet(cmd *cobra.Command, name string) (string, bool) {
	flag := cmd.Flag(name)
	if flag == nil {
		return "", false
	}

	return flag.Value.String(), flag.Changed
}

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

func StringArray(flagset *pflag.FlagSet, name, shorthand, env string, def []string, usage string) {
	v, ok := os.LookupEnv(env)
	if !ok || v == "" {
		if v == "" {
			def = []string{}
		} else {
			def = strings.Split(v, ",")
		}
	}
	flagset.StringArrayP(name, shorthand, def, fmtUsage(usage, env))
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
	flagset.StringArrayVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
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

// IntVarP sets a uint8 flag on the given flag set.
func IntVarP(flagset *pflag.FlagSet, ptr *int, name string, shorthand string, env string, def int, usage string) {
	val, ok := os.LookupEnv(env)
	if !ok || val == "" {
		flagset.IntVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	vi64, err := strconv.ParseUint(val, 10, 8)
	if err != nil {
		flagset.IntVarP(ptr, name, shorthand, def, fmtUsage(usage, env))
		return
	}

	flagset.IntVarP(ptr, name, shorthand, int(vi64), fmtUsage(usage, env))
}

func Bool(flagset *pflag.FlagSet, name, shorthand, env string, def bool, usage string) {
	val, ok := os.LookupEnv(env)
	if !ok || val == "" {
		flagset.BoolP(name, shorthand, def, fmtUsage(usage, env))
		return
	}

	valb, err := strconv.ParseBool(val)
	if err != nil {
		flagset.BoolP(name, shorthand, def, fmtUsage(usage, env))
		return
	}

	flagset.BoolP(name, shorthand, valb, fmtUsage(usage, env))
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
	if env != "" {
		// Avoid double dotting.
		dot := "."
		if strings.HasSuffix(u, ".") {
			dot = ""
		}
		u = fmt.Sprintf("%s%s\n"+cliui.Styles.Placeholder.Render("Consumes $%s"), u, dot, env)
	}

	return u
}

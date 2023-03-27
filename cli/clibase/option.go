package clibase

import (
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

// Option is a configuration option for a CLI application.
type Option struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	// Flag is the long name of the flag used to configure this option. If unset,
	// flag configuring is disabled.
	Flag string `json:"flag,omitempty"`
	// FlagShorthand is the one-character shorthand for the flag. If unset, no
	// shorthand is used.
	FlagShorthand string `json:"flag_shorthand,omitempty"`

	// Env is the environment variable used to configure this option. If unset,
	// environment configuring is disabled.
	Env string `json:"env,omitempty"`

	// YAML is the YAML key used to configure this option. If unset, YAML
	// configuring is disabled.
	YAML string `json:"yaml,omitempty"`

	// Default is parsed into Value if set.
	Default string `json:"default,omitempty"`
	// Value includes the types listed in values.go.
	Value pflag.Value `json:"value,omitempty"`

	// Annotations enable extensions to clibase higher up in the stack. It's useful for
	// help formatting and documentation generation.
	Annotations Annotations `json:"annotations,omitempty"`

	// Group is a group hierarchy that helps organize this option in help, configs
	// and other documentation.
	Group *Group `json:"group,omitempty"`

	// UseInstead is a list of options that should be used instead of this one.
	// The field is used to generate a deprecation warning.
	UseInstead []Option `json:"use_instead,omitempty"`

	Hidden bool `json:"hidden,omitempty"`
}

// OptionSet is a group of options that can be applied to a command.
type OptionSet []Option

// Add adds the given Options to the OptionSet.
func (s *OptionSet) Add(opts ...Option) {
	*s = append(*s, opts...)
}

// FlagSet returns a pflag.FlagSet for the OptionSet.
func (s *OptionSet) FlagSet() *pflag.FlagSet {
	if s == nil {
		return &pflag.FlagSet{}
	}

	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	for _, opt := range *s {
		if opt.Flag == "" {
			continue
		}
		var noOptDefValue string
		{
			no, ok := opt.Value.(NoOptDefValuer)
			if ok {
				noOptDefValue = no.NoOptDefValue()
			}
		}

		val := opt.Value
		if val == nil {
			val = DiscardValue
		}

		fs.AddFlag(&pflag.Flag{
			Name:        opt.Flag,
			Shorthand:   opt.FlagShorthand,
			Usage:       opt.Description,
			Value:       val,
			DefValue:    "",
			Changed:     false,
			Deprecated:  "",
			NoOptDefVal: noOptDefValue,
			Hidden:      opt.Hidden,
		})
	}
	fs.Usage = func() {
		_, _ = os.Stderr.WriteString("Override (*FlagSet).Usage() to print help text.\n")
	}
	return fs
}

// ParseEnv parses the given environment variables into the OptionSet.
// Use EnvsWithPrefix to filter out prefixes.
func (s *OptionSet) ParseEnv(vs []EnvVar) error {
	if s == nil {
		return nil
	}

	var merr *multierror.Error

	// We parse environment variables first instead of using a nested loop to
	// avoid N*M complexity when there are a lot of options and environment
	// variables.
	envs := make(map[string]string)
	for _, v := range vs {
		envs[v.Name] = v.Value
	}

	for _, opt := range *s {
		if opt.Env == "" {
			continue
		}

		envVal, ok := envs[opt.Env]
		// Currently, empty values are treated as if the environment variable is
		// unset. This behavior is technically not correct as there is now no
		// way for a user to change a Default value to an empty string from
		// the environment. Unfortunately, we have old configuration files
		// that rely on the faulty behavior.
		if !ok || envVal == "" {
			continue
		}

		if err := opt.Value.Set(envVal); err != nil {
			merr = multierror.Append(
				merr, xerrors.Errorf("parse %q: %w", opt.Name, err),
			)
		}
	}

	return merr.ErrorOrNil()
}

// SetDefaults sets the default values for each Option.
// It should be called before all parsing (e.g. ParseFlags, ParseEnv).
func (s *OptionSet) SetDefaults() error {
	if s == nil {
		return nil
	}

	var merr *multierror.Error

	for _, opt := range *s {
		if opt.Default == "" {
			continue
		}
		if opt.Value == nil {
			merr = multierror.Append(
				merr,
				xerrors.Errorf(
					"parse %q: no Value field set\nFull opt: %+v",
					opt.Name, opt,
				),
			)
			continue
		}
		if err := opt.Value.Set(opt.Default); err != nil {
			merr = multierror.Append(
				merr, xerrors.Errorf("parse %q: %w", opt.Name, err),
			)
		}
	}
	return merr.ErrorOrNil()
}

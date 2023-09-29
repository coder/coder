package clibase

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

type ValueSource string

const (
	ValueSourceNone    ValueSource = ""
	ValueSourceFlag    ValueSource = "flag"
	ValueSourceEnv     ValueSource = "env"
	ValueSourceYAML    ValueSource = "yaml"
	ValueSourceDefault ValueSource = "default"
)

// Option is a configuration option for a CLI application.
type Option struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// Required means this value must be set by some means. It requires
	// `ValueSource != ValueSourceNone`
	// If `Default` is set, then `Required` is ignored.
	Required bool `json:"required,omitempty"`

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

	ValueSource ValueSource `json:"value_source,omitempty"`
}

// optionNoMethods is just a wrapper around Option so we can defer to the
// default json.Unmarshaler behavior.
type optionNoMethods Option

func (o *Option) UnmarshalJSON(data []byte) error {
	// If an option has no values, we have no idea how to unmarshal it.
	// So just discard the json data.
	if o.Value == nil {
		o.Value = &DiscardValue
	}

	return json.Unmarshal(data, (*optionNoMethods)(o))
}

func (o Option) YAMLPath() string {
	if o.YAML == "" {
		return ""
	}
	var gs []string
	for _, g := range o.Group.Ancestry() {
		gs = append(gs, g.YAML)
	}
	return strings.Join(append(gs, o.YAML), ".")
}

// OptionSet is a group of options that can be applied to a command.
type OptionSet []Option

// UnmarshalJSON implements json.Unmarshaler for OptionSets. Options have an
// interface Value type that cannot handle unmarshalling because the types cannot
// be inferred. Since it is a slice, instantiating the Options first does not
// help.
//
// However, we typically do instantiate the slice to have the correct types.
// So this unmarshaller will attempt to find the named option in the existing
// set, if it cannot, the value is discarded. If the option exists, the value
// is unmarshalled into the existing option, and replaces the existing option.
//
// The value is discarded if it's type cannot be inferred. This behavior just
// feels "safer", although it should never happen if the correct option set
// is passed in. The situation where this could occur is if a client and server
// are on different versions with different options.
func (optSet *OptionSet) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewBuffer(data))
	// Should be a json array, so consume the starting open bracket.
	t, err := dec.Token()
	if err != nil {
		return xerrors.Errorf("read array open bracket: %w", err)
	}
	if t != json.Delim('[') {
		return xerrors.Errorf("expected array open bracket, got %q", t)
	}

	// As long as json elements exist, consume them. The counter is used for
	// better errors.
	var i int
OptionSetDecodeLoop:
	for dec.More() {
		var opt Option
		// jValue is a placeholder value that allows us to capture the
		// raw json for the value to attempt to unmarshal later.
		var jValue jsonValue
		opt.Value = &jValue
		err := dec.Decode(&opt)
		if err != nil {
			return xerrors.Errorf("decode %d option: %w", i, err)
		}
		// This counter is used to contextualize errors to show which element of
		// the array we failed to decode. It is only used in the error above, as
		// if the above works, we can instead use the Option.Name which is more
		// descriptive and useful. So increment here for the next decode.
		i++

		// Try to see if the option already exists in the option set.
		// If it does, just update the existing option.
		for optIndex, have := range *optSet {
			if have.Name == opt.Name {
				if jValue != nil {
					err := json.Unmarshal(jValue, &(*optSet)[optIndex].Value)
					if err != nil {
						return xerrors.Errorf("decode option %q value: %w", have.Name, err)
					}
					// Set the opt's value
					opt.Value = (*optSet)[optIndex].Value
				} else {
					// Hopefully the user passed empty values in the option set. There is no easy way
					// to tell, and if we do not do this, it breaks json.Marshal if we do it again on
					// this new option set.
					opt.Value = (*optSet)[optIndex].Value
				}
				// Override the existing.
				(*optSet)[optIndex] = opt
				// Go to the next option to decode.
				continue OptionSetDecodeLoop
			}
		}

		// If the option doesn't exist, the value will be discarded.
		// We do this because we cannot infer the type of the value.
		opt.Value = DiscardValue
		*optSet = append(*optSet, opt)
	}

	t, err = dec.Token()
	if err != nil {
		return xerrors.Errorf("read array close bracket: %w", err)
	}
	if t != json.Delim(']') {
		return xerrors.Errorf("expected array close bracket, got %q", t)
	}

	return nil
}

// Add adds the given Options to the OptionSet.
func (optSet *OptionSet) Add(opts ...Option) {
	*optSet = append(*optSet, opts...)
}

// Filter will only return options that match the given filter. (return true)
func (optSet OptionSet) Filter(filter func(opt Option) bool) OptionSet {
	cpy := make(OptionSet, 0)
	for _, opt := range optSet {
		if filter(opt) {
			cpy = append(cpy, opt)
		}
	}
	return cpy
}

// FlagSet returns a pflag.FlagSet for the OptionSet.
func (optSet *OptionSet) FlagSet() *pflag.FlagSet {
	if optSet == nil {
		return &pflag.FlagSet{}
	}

	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	for _, opt := range *optSet {
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
func (optSet *OptionSet) ParseEnv(vs []EnvVar) error {
	if optSet == nil {
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

	for i, opt := range *optSet {
		if opt.Env == "" {
			continue
		}

		envVal, ok := envs[opt.Env]
		// Currently, empty values are treated as if the environment variable is
		// unset. This behavior is technically not correct as there is now no
		// way for a user to change a Default value to an empty string from
		// the environment. Unfortunately, we have old configuration files
		// that rely on the faulty behavior.
		//
		// TODO: We should remove this hack in May 2023, when deployments
		// have had months to migrate to the new behavior.
		if !ok || envVal == "" {
			continue
		}

		(*optSet)[i].ValueSource = ValueSourceEnv
		if err := opt.Value.Set(envVal); err != nil {
			merr = multierror.Append(
				merr, xerrors.Errorf("parse %q: %w", opt.Name, err),
			)
		}
	}

	return merr.ErrorOrNil()
}

// SetDefaults sets the default values for each Option, skipping values
// that already have a value source.
func (optSet *OptionSet) SetDefaults() error {
	if optSet == nil {
		return nil
	}

	var merr *multierror.Error

	for i, opt := range *optSet {
		// Skip values that may have already been set by the user.
		if opt.ValueSource != ValueSourceNone {
			continue
		}

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
		(*optSet)[i].ValueSource = ValueSourceDefault
		if err := opt.Value.Set(opt.Default); err != nil {
			merr = multierror.Append(
				merr, xerrors.Errorf("parse %q: %w", opt.Name, err),
			)
		}
	}
	return merr.ErrorOrNil()
}

// ByName returns the Option with the given name, or nil if no such option
// exists.
func (optSet *OptionSet) ByName(name string) *Option {
	for i := range *optSet {
		opt := &(*optSet)[i]
		if opt.Name == name {
			return opt
		}
	}
	return nil
}

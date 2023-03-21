package clibase

import "strings"

// name returns the name of the environment variable.
func envName(line string) string {
	return strings.ToUpper(
		strings.SplitN(line, "=", 2)[0],
	)
}

// value returns the value of the environment variable.
func envValue(line string) string {
	tokens := strings.SplitN(line, "=", 2)
	if len(tokens) < 2 {
		return ""
	}
	return tokens[1]
}

// Var represents a single environment variable of form
// NAME=VALUE.
type EnvVar struct {
	Name  string
	Value string
}

type Environ []EnvVar

func (e Environ) ToOS() []string {
	var env []string
	for _, v := range e {
		env = append(env, v.Name+"="+v.Value)
	}
	return env
}

func (e Environ) Lookup(name string) (string, bool) {
	for _, v := range e {
		if v.Name == name {
			return v.Value, true
		}
	}
	return "", false
}

func (e Environ) Get(name string) string {
	v, _ := e.Lookup(name)
	return v
}

func (e *Environ) Set(name, value string) {
	for i, v := range *e {
		if v.Name == name {
			(*e)[i].Value = value
			return
		}
	}
	*e = append(*e, EnvVar{Name: name, Value: value})
}

// ParseEnviron returns all environment variables starting with
// prefix without said prefix.
func ParseEnviron(environ []string, prefix string) Environ {
	var filtered []EnvVar
	for _, line := range environ {
		name := envName(line)
		if strings.HasPrefix(name, prefix) {
			filtered = append(filtered, EnvVar{
				Name:  strings.TrimPrefix(name, prefix),
				Value: envValue(line),
			})
		}
	}
	return filtered
}

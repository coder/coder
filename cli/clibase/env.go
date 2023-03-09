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

// EnvsWithPrefix returns all environment variables starting with
// prefix without said prefix.
func EnvsWithPrefix(environ []string, prefix string) []EnvVar {
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

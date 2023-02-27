// Package envparse contains utilities for parsing the OS environment.
package envparse

import "strings"

// Name returns the name of the environment variable.
func Name(line string) string {
	return strings.ToUpper(
		strings.SplitN(line, "=", 2)[0],
	)
}

// Value returns the value of the environment variable.
func Value(line string) string {
	tokens := strings.SplitN(line, "=", 2)
	if len(tokens) < 2 {
		return ""
	}
	return tokens[1]
}

// Var represents a single environment variable of form
// NAME=VALUE.
type Var struct {
	Name  string
	Value string
}

// Parse parses a single environment variable.
func Parse(line string) Var {
	return Var{
		Name:  Name(line),
		Value: Value(line),
	}
}

// FilterNamePrefix returns all environment variables starting with
// prefix without said prefix.
func FilterNamePrefix(environ []string, prefix string) []Var {
	var filtered []Var
	for _, line := range environ {
		name := Name(line)
		if strings.HasPrefix(name, prefix) {
			filtered = append(filtered, Var{
				Name:  strings.TrimPrefix(name, prefix),
				Value: Value(line),
			})
		}
	}
	return filtered
}

package mcptools

import (
	"strings"

	"github.com/google/shlex"
	"golang.org/x/xerrors"
)

// IsCommandAllowed checks if a command is in the allowed list.
// It parses the command using shlex to correctly handle quoted arguments
// and only checks the executable name (first part of the command).
//
// Based on benchmarks, a simple linear search performs better than
// a map-based approach for the typical number of allowed commands,
// so we're sticking with the simple approach.
func IsCommandAllowed(command string, allowedCommands []string) (bool, error) {
	if len(allowedCommands) == 0 {
		// If no allowed commands are specified, all commands are allowed
		return true, nil
	}

	// Parse the command to extract the executable name
	parts, err := shlex.Split(command)
	if err != nil {
		return false, xerrors.Errorf("failed to parse command: %w", err)
	}

	if len(parts) == 0 {
		return false, xerrors.New("empty command")
	}

	// The first part is the executable name
	executable := parts[0]

	// Check if the executable is in the allowed list
	for _, allowed := range allowedCommands {
		if allowed == executable {
			return true, nil
		}
	}

	// Build a helpful error message
	return false, xerrors.Errorf("command %q is not allowed. Allowed commands: %s",
		executable, strings.Join(allowedCommands, ", "))
}

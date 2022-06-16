package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// This file contains config-ssh definitions that are deprecated, they
// will be removed after a migratory period.

const (
	sshDefaultCoderConfigFileName = "~/.ssh/coder"
	sshCoderConfigHeader          = "# This file is managed by coder. DO NOT EDIT."
)

// Regular expressions are used because SSH configs do not have
// meaningful indentation and keywords are case-insensitive.
var (
	// Find the semantically correct include statement. Since the user can
	// modify their configuration as they see fit, there could be:
	// - Leading indentation (space, tab)
	// - Trailing indentation (space, tab)
	// - Select newline after Include statement for cleaner removal
	// In the following cases, we will not recognize the Include statement
	// and leave as-is (i.e. they're not supported):
	// - User adds another file to the Include statement
	// - User adds a comment on the same line as the Include statement
	sshCoderIncludedRe = regexp.MustCompile(`(?m)^[\t ]*((?i)Include) coder[\t ]*[\r]?[\n]?$`)
)

// removeDeprecatedSSHIncludeStatement checks for the Include coder statement
// and returns modified = true if it was removed.
func removeDeprecatedSSHIncludeStatement(data []byte) (modifiedData []byte, modified bool) {
	coderInclude := sshCoderIncludedRe.FindIndex(data)
	if coderInclude == nil {
		return data, false
	}

	// Remove Include statement.
	d := append([]byte{}, data[:coderInclude[0]]...)
	d = append(d, data[coderInclude[1]:]...)
	data = d

	return data, true
}

// readDeprecatedCoderConfigFile reads the deprecated split config file.
func readDeprecatedCoderConfigFile(homedir, coderConfigFile string) (name string, data []byte, ok bool) {
	if strings.HasPrefix(coderConfigFile, "~/") {
		coderConfigFile = filepath.Join(homedir, coderConfigFile[2:])
	}

	b, err := os.ReadFile(coderConfigFile)
	if err != nil {
		return coderConfigFile, nil, false
	}
	if len(b) > 0 {
		if !bytes.HasPrefix(b, []byte(sshCoderConfigHeader)) {
			return coderConfigFile, nil, false
		}
	}
	return coderConfigFile, b, true
}

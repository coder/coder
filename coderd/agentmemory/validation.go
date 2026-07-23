// Package agentmemory contains shared validation for user-scoped memories.
package agentmemory

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/xerrors"
)

const (
	MaxPathBytes    = 1024
	MaxContentBytes = 65536
)

// ValidatePath validates an absolute, canonical Markdown memory path.
func ValidatePath(memoryPath string) error {
	if memoryPath == "/" {
		return xerrors.New("path must be an absolute memory path")
	}
	if err := validateCanonicalAbsolutePath(memoryPath, "path"); err != nil {
		return err
	}
	if !strings.HasSuffix(memoryPath, ".md") || path.Base(memoryPath) == ".md" {
		return xerrors.New("path must end in a named .md file")
	}
	return nil
}

// ValidateDirectory validates an absolute, canonical virtual directory.
func ValidateDirectory(directory string) error {
	return validateCanonicalAbsolutePath(directory, "directory")
}

func validateCanonicalAbsolutePath(value, name string) error {
	if value == "" {
		return xerrors.Errorf("%s is required", name)
	}
	if !utf8.ValidString(value) {
		return xerrors.Errorf("%s must be valid UTF-8", name)
	}
	if len(value) > MaxPathBytes {
		return xerrors.Errorf("%s exceeds %d bytes", name, MaxPathBytes)
	}
	if !strings.HasPrefix(value, "/") {
		return xerrors.Errorf("%s must be an absolute memory %s", name, name)
	}
	if path.Clean(value) != value {
		return xerrors.Errorf("%s must be canonical", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return xerrors.Errorf("%s must not contain control characters", name)
		}
	}
	return nil
}

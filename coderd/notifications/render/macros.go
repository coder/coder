package render

import (
	"fmt"
	"regexp"

	"golang.org/x/xerrors"
)

// Macros substitutes certain well-known macros in strings.
func Macros(macros map[string]func() string, in string) (string, error) {
	if len(macros) == 0 {
		return in, nil
	}

	for macro, fn := range macros {
		p, err := regexp.Compile(fmt.Sprintf(`\[%s\]`, macro))
		if err != nil {
			return "", xerrors.Errorf("compile regex: %w", err)
		}

		in = p.ReplaceAllString(in, fn())
	}

	return in, nil
}

package ignore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/spf13/afero"
)

func FilePathToParts(path string) []string {
	components := []string{}

	if path == "" {
		return []string{}
	}

	for segment := range strings.SplitSeq(filepath.Clean(path), "/") {
		if segment != "" {
			components = append(components, segment)
		}
	}

	return components
}

func readIgnoreFile(fileSystem afero.Fs, path string) ([]gitignore.Pattern, error) {
	var ps []gitignore.Pattern

	data, err := afero.ReadFile(fileSystem, filepath.Join(path, ".gitignore"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	for s := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(s, "#") && len(strings.TrimSpace(s)) > 0 {
			ps = append(ps, gitignore.ParsePattern(s, FilePathToParts(path)))
		}
	}

	return ps, nil
}

func ReadPatterns(fileSystem afero.Fs, path string) ([]gitignore.Pattern, error) {
	ps, _ := readIgnoreFile(fileSystem, path)

	if err := afero.Walk(fileSystem, path, func(path string, info fs.FileInfo, _ error) error {
		if !info.IsDir() {
			return nil
		}

		subPs, err := readIgnoreFile(fileSystem, path)
		if err != nil {
			return err
		}

		ps = append(ps, subPs...)

		return nil
	}); err != nil {
		return nil, err
	}

	return ps, nil
}

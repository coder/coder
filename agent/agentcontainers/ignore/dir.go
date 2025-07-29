package ignore

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

const (
	gitconfigFile      = ".gitconfig"
	gitignoreFile      = ".gitignore"
	gitInfoExcludeFile = ".git/info/exclude"
)

func FilePathToParts(path string) []string {
	components := []string{}

	if path == "" {
		return components
	}

	for segment := range strings.SplitSeq(filepath.Clean(path), string(filepath.Separator)) {
		if segment != "" {
			components = append(components, segment)
		}
	}

	return components
}

func readIgnoreFile(fileSystem afero.Fs, path, ignore string) ([]gitignore.Pattern, error) {
	var ps []gitignore.Pattern

	data, err := afero.ReadFile(fileSystem, filepath.Join(path, ignore))
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

func ReadPatterns(ctx context.Context, logger slog.Logger, fileSystem afero.Fs, path string) ([]gitignore.Pattern, error) {
	var ps []gitignore.Pattern

	subPs, err := readIgnoreFile(fileSystem, path, gitInfoExcludeFile)
	if err != nil {
		return nil, err
	}

	ps = append(ps, subPs...)

	if err := afero.Walk(fileSystem, path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			logger.Error(ctx, "encountered error while walking for git ignore files",
				slog.F("path", path),
				slog.Error(err))
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		subPs, err := readIgnoreFile(fileSystem, path, gitignoreFile)
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

func loadPatterns(fileSystem afero.Fs, path string) ([]gitignore.Pattern, error) {
	data, err := afero.ReadFile(fileSystem, path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	decoder := config.NewDecoder(bytes.NewBuffer(data))

	conf := config.New()
	if err := decoder.Decode(conf); err != nil {
		return nil, xerrors.Errorf("decode config: %w", err)
	}

	excludes := conf.Section("core").Options.Get("excludesfile")
	if excludes == "" {
		return nil, nil
	}

	return readIgnoreFile(fileSystem, "", excludes)
}

func LoadGlobalPatterns(fileSystem afero.Fs) ([]gitignore.Pattern, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return loadPatterns(fileSystem, filepath.Join(home, gitconfigFile))
}

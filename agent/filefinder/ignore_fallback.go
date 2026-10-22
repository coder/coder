package filefinder

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/spf13/afero"
)

// This file is the fallback used when git is not installed. It
// approximates git with go-git's matcher, which differs from git's
// wildmatch (for example, "foo/**" blocks a later "!foo/keep.txt"), and
// ignores core.ignoreCase and config includes. With git installed,
// Ignorer asks git instead.

type fileStamp struct {
	modTime int64
	size    int64
}

type cachedIgnoreFile struct {
	stamp    fileStamp
	patterns []gitignore.Pattern
}

// loadGlobalExcludes reads core.excludesFile from ~/.gitconfig, else
// $XDG_CONFIG_HOME/git/ignore or ~/.config/git/ignore.
func loadGlobalExcludes(fs afero.Fs, home string) []string {
	if home == "" {
		return nil
	}
	var path string
	if data, err := afero.ReadFile(fs, filepath.Join(home, ".gitconfig")); err == nil {
		conf := config.New()
		if err := config.NewDecoder(bytes.NewReader(data)).Decode(conf); err == nil {
			path = conf.Section("core").Options.Get("excludesfile")
		}
	}
	if path == "" {
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		path = filepath.Join(xdg, "git", "ignore")
	} else if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil
	}
	return ignoreLines(data)
}

func ignoreLines(data []byte) []string {
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func pathParts(path string) []string {
	return strings.FieldsFunc(filepath.Clean(path), func(r rune) bool {
		return r == filepath.Separator
	})
}

// ignoreRules are the fallback rules for the entries of one directory.
// The zero value ignores nothing.
type ignoreRules struct {
	inRepo   bool
	patterns []gitignore.Pattern
	// dirIgnored is set when the directory itself, or an ancestor
	// below the repository root, is ignored.
	dirIgnored bool
}

func (r ignoreRules) ignored(path string, isDir bool) bool {
	if !r.inRepo {
		return false
	}
	if r.dirIgnored {
		return true
	}
	return gitignore.NewMatcher(r.patterns).Match(pathParts(path), isDir)
}

// fallbackRules returns the rules for the entries of dir.
func (ig *Ignorer) fallbackRules(dir string) ignoreRules {
	dir = filepath.Clean(dir)
	top := ig.repoTop(dir)
	if top == "" {
		return ignoreRules{}
	}
	var chain []string
	for d := dir; d != top; d = filepath.Dir(d) {
		chain = append(chain, d)
	}
	r := ig.fallbackRepoRules(top)
	for i := len(chain) - 1; i >= 0; i-- {
		d := chain[i]
		if !r.dirIgnored && r.ignored(d, true) {
			r.dirIgnored = true
		}
		r.patterns = append(slices.Clip(r.patterns), ig.filePatterns(filepath.Join(d, gitignoreFile), pathParts(d))...)
	}
	return r
}

// fallbackChild returns the rules for the entries of dir, a
// subdirectory of the directory parent applies to that is not ignored
// and not a repository.
func (ig *Ignorer) fallbackChild(parent ignoreRules, dir string) ignoreRules {
	if !parent.inRepo {
		return ignoreRules{}
	}
	return ignoreRules{
		inRepo:   true,
		patterns: append(slices.Clip(parent.patterns), ig.filePatterns(filepath.Join(dir, gitignoreFile), pathParts(dir))...),
	}
}

func (ig *Ignorer) fallbackRepoRules(top string) ignoreRules {
	domain := pathParts(top)
	patterns := make([]gitignore.Pattern, 0, len(ig.global))
	for _, line := range ig.global {
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	patterns = append(patterns, ig.filePatterns(fallbackExcludeFile(ig.fs, top), domain)...)
	patterns = append(patterns, ig.filePatterns(filepath.Join(top, gitignoreFile), domain)...)
	return ignoreRules{inRepo: true, patterns: patterns}
}

// fallbackExcludeFile locates info/exclude, following a .git file to a
// separate git dir.
func fallbackExcludeFile(fs afero.Fs, top string) string {
	gitDir := filepath.Join(top, ".git")
	if info, err := fs.Stat(gitDir); err == nil && !info.IsDir() {
		if data, err := afero.ReadFile(fs, gitDir); err == nil {
			if dir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
				dir = filepath.FromSlash(strings.TrimSpace(dir))
				if !filepath.IsAbs(dir) {
					dir = filepath.Join(top, dir)
				}
				gitDir = dir
			}
		}
	}
	return filepath.Join(gitDir, "info", "exclude")
}

// filePatterns returns the patterns in the ignore file at path, parsed
// relative to domain. Only existing files are cached.
func (ig *Ignorer) filePatterns(path string, domain []string) []gitignore.Pattern {
	info, err := ig.fs.Stat(path)
	if err != nil || info.IsDir() {
		if errors.Is(err, os.ErrNotExist) {
			ig.mu.Lock()
			delete(ig.files, path)
			ig.mu.Unlock()
		}
		return nil
	}
	stamp := fileStamp{modTime: info.ModTime().UnixNano(), size: info.Size()}
	ig.mu.Lock()
	cached, ok := ig.files[path]
	ig.mu.Unlock()
	if ok && cached.stamp == stamp {
		return cached.patterns
	}
	data, err := afero.ReadFile(ig.fs, path)
	if err != nil {
		return nil
	}
	lines := ignoreLines(data)
	patterns := make([]gitignore.Pattern, 0, len(lines))
	for _, line := range lines {
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	ig.mu.Lock()
	ig.files[path] = cachedIgnoreFile{stamp: stamp, patterns: patterns}
	ig.mu.Unlock()
	return patterns
}

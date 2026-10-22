package filefinder

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/spf13/afero"
)

const gitignoreFile = ".gitignore"

// Ignorer answers whether paths are ignored by git. A path is matched
// against the rules of its enclosing repository, the nearest ancestor
// directory containing .git: the global excludes file, the
// repository's .git/info/exclude, and every .gitignore from the
// repository root down to the path's directory. Paths outside any
// repository are never ignored. Ignore files are cached and re-read
// when their modification time or size changes. Safe for concurrent
// use.
type Ignorer struct {
	fs afero.Fs
	// global holds the lines of the user's global excludes file. It is
	// read once; anchored patterns are relative to each repository
	// root, so it is parsed per repository.
	global []string

	mu    sync.Mutex
	files map[string]cachedIgnoreFile
}

type fileStamp struct {
	modTime int64
	size    int64
}

type cachedIgnoreFile struct {
	stamp    fileStamp
	patterns []gitignore.Pattern
}

// NewIgnorer creates an Ignorer reading files through fs. homeDir
// locates the global excludes file: core.excludesFile in
// ~/.gitconfig, else $XDG_CONFIG_HOME/git/ignore or
// ~/.config/git/ignore. An empty homeDir disables global excludes.
func NewIgnorer(fs afero.Fs, homeDir string) *Ignorer {
	return &Ignorer{
		fs:     fs,
		global: loadGlobalExcludes(fs, homeDir),
		files:  make(map[string]cachedIgnoreFile),
	}
}

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

// IgnoreRules are the ignore rules for the entries of one directory.
// The zero value ignores nothing.
type IgnoreRules struct {
	inRepo   bool
	patterns []gitignore.Pattern
	// dirIgnored is set when the directory itself, or an ancestor
	// below the repository root, is ignored.
	dirIgnored bool
}

// Ignored reports whether path, an entry of the rules' directory, is
// ignored. isDir must be false for symlinks, as git does not follow
// them.
func (r IgnoreRules) Ignored(path string, isDir bool) bool {
	if !r.inRepo {
		return false
	}
	if r.dirIgnored {
		return true
	}
	return gitignore.NewMatcher(r.patterns).Match(pathParts(path), isDir)
}

// Rules returns the ignore rules for the entries of dir.
func (ig *Ignorer) Rules(dir string) IgnoreRules {
	return ig.rulesBelow(dir, "")
}

// rulesBelow returns the ignore rules for the entries of dir. When trusted
// is set, only ancestors strictly below trusted are checked for being
// ignored; otherwise every ancestor below the repository root is.
func (ig *Ignorer) rulesBelow(dir, trusted string) IgnoreRules {
	dir = filepath.Clean(dir)
	// chain holds dir and its ancestors up to the repository root,
	// nearest first.
	var chain []string
	for d := dir; ; {
		chain = append(chain, d)
		if ig.isRepo(d) {
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			return IgnoreRules{}
		}
		d = parent
	}
	r := ig.repoRules(chain[len(chain)-1])
	for i := len(chain) - 2; i >= 0; i-- {
		d := chain[i]
		if !r.dirIgnored && (trusted == "" || isBelow(trusted, d)) && r.Ignored(d, true) {
			r.dirIgnored = true
		}
		r.patterns = append(slices.Clip(r.patterns), ig.filePatterns(filepath.Join(d, gitignoreFile), pathParts(d))...)
	}
	return r
}

// isBelow reports whether path is strictly inside base.
func isBelow(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// child returns the rules for the entries of dir, a subdirectory of
// the directory parent applies to that is not ignored itself.
func (ig *Ignorer) child(parent IgnoreRules, dir string) IgnoreRules {
	if ig.isRepo(dir) {
		return ig.repoRules(dir)
	}
	if !parent.inRepo {
		return IgnoreRules{}
	}
	return IgnoreRules{
		inRepo:   true,
		patterns: append(slices.Clip(parent.patterns), ig.filePatterns(filepath.Join(dir, gitignoreFile), pathParts(dir))...),
	}
}

// repoRules returns the rules for the entries of a repository root.
func (ig *Ignorer) repoRules(root string) IgnoreRules {
	domain := pathParts(root)
	patterns := make([]gitignore.Pattern, 0, len(ig.global))
	for _, line := range ig.global {
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	patterns = append(patterns, ig.filePatterns(filepath.Join(root, ".git", "info", "exclude"), domain)...)
	patterns = append(patterns, ig.filePatterns(filepath.Join(root, gitignoreFile), domain)...)
	return IgnoreRules{inRepo: true, patterns: patterns}
}

func (ig *Ignorer) isRepo(dir string) bool {
	_, err := ig.lstat(filepath.Join(dir, ".git"))
	return err == nil
}

func (ig *Ignorer) lstat(path string) (os.FileInfo, error) {
	if l, ok := ig.fs.(afero.Lstater); ok {
		info, _, err := l.LstatIfPossible(path)
		return info, err
	}
	return ig.fs.Stat(path)
}

// filePatterns returns the patterns in the ignore file at path, parsed
// relative to domain. Only existing files are cached, so the cache
// grows with the number of ignore files rather than directories.
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

// isIgnoreFile reports whether a change to path can change ignore
// rules.
func isIgnoreFile(path string) bool {
	return filepath.Base(path) == gitignoreFile || isGitInfoPath(path) && filepath.Base(path) == "exclude"
}

// isGitInfoPath reports whether path is an entry of a .git/info
// directory, which the watcher watches only to see exclude changes.
func isGitInfoPath(path string) bool {
	info := filepath.Dir(path)
	return filepath.Base(info) == "info" && filepath.Base(filepath.Dir(info)) == ".git"
}

// walkIndexable walks root like filepath.Walk, calling fn for every
// entry except root itself. It skips skipDirs and gitignored entries
// without descending into ignored directories. root itself is treated
// as not ignored, as the caller asked for it. It stops when ctx is
// canceled, returning ctx.Err().
func walkIndexable(ctx context.Context, ig *Ignorer, root string, fn func(path string, info os.FileInfo) error) error {
	rules := map[string]IgnoreRules{root: ig.rulesBelow(root, root)}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil //nolint:nilerr // best-effort
		}
		isDir := info.IsDir()
		if _, skip := skipDirs[filepath.Base(path)]; skip && isDir {
			return filepath.SkipDir
		}
		if path == root {
			return nil
		}
		parent := rules[filepath.Dir(path)]
		if parent.Ignored(path, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isDir {
			rules[path] = ig.child(parent, path)
		}
		return fn(path, info)
	})
}

package filefinder

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"github.com/coder/coder/v2/agent/agentexec"
)

const (
	gitignoreFile = ".gitignore"
	// repoCacheTTL bounds how long repository discovery and exclude
	// file locations are cached, so a new or removed repository is
	// noticed without a stat walk per event.
	repoCacheTTL = 5 * time.Second
	// maxRepoCacheEntries bounds the discovery cache; it is cleared
	// when full.
	maxRepoCacheEntries = 10_000
)

// Ignorer answers whether paths are ignored by git. A path follows the
// rules of its enclosing repository, the nearest ancestor directory
// containing a .git directory or file. Paths outside any repository
// are never ignored.
//
// git is the source of truth: Ignorer runs git ls-files and git
// check-ignore so wildmatch, core.ignoreCase, separate git dirs,
// config includes, and the global excludes file behave exactly as in
// git. When git is not installed, or a git call fails, it falls back
// to go-git's matcher, which only approximates git (see
// ignore_fallback.go). Safe for concurrent use.
type Ignorer struct {
	fs  afero.Fs
	git *gitRunner
	// global holds the global excludes lines for the fallback.
	global []string

	mu       sync.Mutex
	files    map[string]cachedIgnoreFile
	tops     map[string]cachedValue
	excludes map[string]cachedValue
}

type cachedValue struct {
	value   string
	expires time.Time
}

type ignorerConfig struct {
	execer agentexec.Execer
	gitBin string
}

// IgnorerOption configures an Ignorer.
type IgnorerOption func(*ignorerConfig)

// WithGit sets how git is run. An empty bin disables git and uses the
// go-git fallback. The default runs "git" from PATH through
// agentexec.DefaultExecer.
func WithGit(execer agentexec.Execer, bin string) IgnorerOption {
	return func(c *ignorerConfig) {
		c.execer = execer
		c.gitBin = bin
	}
}

// NewIgnorer creates an Ignorer. filesystem and homeDir are used only
// by the fallback: filesystem reads ignore files and homeDir locates
// the global excludes file. An empty homeDir disables fallback global
// excludes.
func NewIgnorer(filesystem afero.Fs, homeDir string, opts ...IgnorerOption) *Ignorer {
	cfg := ignorerConfig{execer: agentexec.DefaultExecer, gitBin: "git"}
	for _, opt := range opts {
		opt(&cfg)
	}
	ig := &Ignorer{
		fs:       filesystem,
		git:      newGitRunner(cfg.execer, cfg.gitBin),
		files:    make(map[string]cachedIgnoreFile),
		tops:     make(map[string]cachedValue),
		excludes: make(map[string]cachedValue),
	}
	if ig.git == nil {
		ig.global = loadGlobalExcludes(filesystem, homeDir)
	}
	return ig
}

func (ig *Ignorer) cached(m map[string]cachedValue, key string) (string, bool) {
	ig.mu.Lock()
	defer ig.mu.Unlock()
	c, ok := m[key]
	if !ok || time.Now().After(c.expires) {
		return "", false
	}
	return c.value, true
}

func (ig *Ignorer) cache(m map[string]cachedValue, key, value string) {
	ig.mu.Lock()
	defer ig.mu.Unlock()
	if len(m) >= maxRepoCacheEntries {
		clear(m)
	}
	m[key] = cachedValue{value: value, expires: time.Now().Add(repoCacheTTL)}
}

// repoTop returns the top of the repository enclosing dir, or "" when
// dir is not in one. Results are cached per directory.
func (ig *Ignorer) repoTop(dir string) string {
	if top, ok := ig.cached(ig.tops, dir); ok {
		return top
	}
	var top string
	if _, err := ig.lstat(filepath.Join(dir, ".git")); err == nil {
		top = dir
	} else if parent := filepath.Dir(dir); parent != dir {
		top = ig.repoTop(parent)
	}
	ig.cache(ig.tops, dir, top)
	return top
}

func (ig *Ignorer) lstat(path string) (os.FileInfo, error) {
	if l, ok := ig.fs.(afero.Lstater); ok {
		info, _, err := l.LstatIfPossible(path)
		return info, err
	}
	return ig.fs.Stat(path)
}

// excludeFile returns the path of the repository's info/exclude file.
func (ig *Ignorer) excludeFile(ctx context.Context, top string) string {
	if path, ok := ig.cached(ig.excludes, top); ok {
		return path
	}
	var path string
	if ig.git != nil {
		path, _ = ig.git.excludeFile(ctx, top)
	}
	if path == "" {
		path = fallbackExcludeFile(ig.fs, top)
	}
	ig.cache(ig.excludes, top, path)
	return path
}

// IgnoreQuery is a path to classify. IsDir must be false for symlinks,
// as git does not follow them.
type IgnoreQuery struct {
	Path  string
	IsDir bool
}

// Ignored reports, for each query, whether git ignores the path. It
// runs one git check-ignore per repository involved. If git fails for
// a repository, its paths are classified by the fallback.
func (ig *Ignorer) Ignored(ctx context.Context, queries []IgnoreQuery) []bool {
	result := make([]bool, len(queries))
	byTop := make(map[string][]int)
	for i, q := range queries {
		if top := ig.repoTop(filepath.Dir(q.Path)); top != "" {
			byTop[top] = append(byTop[top], i)
		}
	}
	for top, indexes := range byTop {
		if ig.git != nil {
			rels := make([]string, len(indexes))
			for j, i := range indexes {
				rel, _ := filepath.Rel(top, queries[i].Path)
				rels[j] = filepath.ToSlash(rel)
				if queries[i].IsDir {
					rels[j] += "/"
				}
			}
			if ignored, err := ig.git.checkIgnored(ctx, top, rels); err == nil {
				for j, i := range indexes {
					result[i] = ignored[j]
				}
				continue
			}
		}
		rules := make(map[string]ignoreRules)
		for _, i := range indexes {
			dir := filepath.Dir(queries[i].Path)
			r, ok := rules[dir]
			if !ok {
				r = ig.fallbackRules(dir)
				rules[dir] = r
			}
			result[i] = r.ignored(queries[i].Path, queries[i].IsDir)
		}
	}
	return result
}

// walkScope decides which entries of a walked directory are ignored.
type walkScope struct {
	// all is set when the whole directory is ignored.
	all bool
	// set holds the ignored paths git reported for the subtree.
	set map[string]struct{}
	// rules are used instead of set by the fallback.
	rules *ignoreRules
}

func (s walkScope) ignored(path string, isDir bool) bool {
	switch {
	case s.all:
		return true
	case s.rules != nil:
		return s.rules.ignored(path, isDir)
	default:
		_, ok := s.set[path]
		return ok
	}
}

// scopeFor lists the ignored paths under dir with one git call. git
// reports an ignored directory without reading it.
func (ig *Ignorer) scopeFor(ctx context.Context, dir string) walkScope {
	if ig.repoTop(dir) == "" {
		return walkScope{}
	}
	if ig.git != nil {
		set, all, err := ig.git.listIgnored(ctx, dir)
		if err == nil {
			return walkScope{all: all, set: set}
		}
	}
	r := ig.fallbackRules(dir)
	return walkScope{all: r.dirIgnored, rules: &r}
}

// walk walks root with filepath.WalkDir, calling fn for every entry
// below root that is not in skipDirs and not gitignored. Ignored
// directories are pruned before they are read. onRepo, if set, is
// called with the top of each repository whose rules apply: root's
// enclosing one and any nested below root. It stops when ctx is
// canceled, returning ctx.Err().
func (ig *Ignorer) walk(ctx context.Context, root string, fn func(path string, d fs.DirEntry) error, onRepo func(top string)) error {
	if _, skip := skipDirs[filepath.Base(root)]; skip {
		return nil
	}
	rootScope := ig.scopeFor(ctx, root)
	if top := ig.repoTop(root); top != "" && onRepo != nil {
		onRepo(top)
	}
	if rootScope.all {
		return nil
	}
	scopes := map[string]walkScope{root: rootScope}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil || path == root {
			return nil //nolint:nilerr // best-effort
		}
		isDir := d.IsDir()
		if _, skip := skipDirs[d.Name()]; skip && isDir {
			return filepath.SkipDir
		}
		parent := scopes[filepath.Dir(path)]
		if parent.ignored(path, isDir) {
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isDir {
			switch {
			case ig.repoTop(path) == path:
				scopes[path] = ig.scopeFor(ctx, path)
				if onRepo != nil {
					onRepo(path)
				}
			case parent.rules != nil:
				r := ig.fallbackChild(*parent.rules, path)
				scopes[path] = walkScope{rules: &r}
			default:
				scopes[path] = parent
			}
		}
		return fn(path, d)
	})
}

// isBelow reports whether path is strictly inside base.
func isBelow(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

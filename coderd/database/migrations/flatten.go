package migrations

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// archiveShardSize is the number of migration versions held by each archive
// directory. Older migrations are grouped into directories named for the
// version range they hold, for example "000001-000100", so that the migrations
// directory stays browsable: GitHub truncates directory listings and its
// contents API at 1000 entries, which previously hid both the newest
// migrations and the tooling in this package.
const archiveShardSize = 100

// shardDirName returns the archive directory name holding the given version.
func shardDirName(version int) string {
	start := ((version - 1) / archiveShardSize) * archiveShardSize
	return fmt.Sprintf("%06d-%06d", start+1, start+archiveShardSize)
}

// parseShardDirName reports the inclusive version range encoded in an archive
// directory name, and whether name is an archive directory at all.
func parseShardDirName(name string) (start, end int, ok bool) {
	startStr, endStr, found := strings.Cut(name, "-")
	if !found || !isSixDigits(startStr) || !isSixDigits(endStr) {
		return 0, 0, false
	}
	start, _ = strconv.Atoi(startStr)
	end, _ = strconv.Atoi(endStr)
	if start > end {
		return 0, 0, false
	}
	return start, end, true
}

func isSixDigits(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// flattenFS presents a migrations tree that may contain archive directories as
// a single flat directory of migration files.
//
// golang-migrate reads migrations from one directory and does not recurse. It
// also resolves a deployment's current schema version by reading that version's
// migration file, so every migration ever shipped must stay readable no matter
// which archive directory it now lives in, or upgrades from older versions fail.
// Flattening at read time keeps archiving a pure file move: it changes no
// version numbers and leaves historical releases, whose migrations are all in
// the root, loadable by the same code.
type flattenFS struct {
	inner fs.FS
	// paths maps each migration file name to its location within inner.
	paths map[string]string
}

func flatten(inner fs.FS) (*flattenFS, error) {
	f := &flattenFS{inner: inner, paths: make(map[string]string)}
	err := fs.WalkDir(inner, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == "." {
				return nil
			}
			if _, _, ok := parseShardDirName(d.Name()); ok {
				return nil
			}
			// Directories that are not archives hold no migrations. Skipping
			// rather than failing keeps this usable on the on-disk tree, which
			// also contains testdata.
			return fs.SkipDir
		}
		if !strings.HasSuffix(p, ".sql") {
			return nil
		}
		name := path.Base(p)
		if existing, ok := f.paths[name]; ok {
			return xerrors.Errorf("duplicate migration %q found at %q and %q", name, existing, p)
		}
		f.paths[name] = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (f *flattenFS) Open(name string) (fs.File, error) {
	location, ok := f.paths[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return f.inner.Open(location)
}

func (f *flattenFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	names := make([]string, 0, len(f.paths))
	for name := range f.paths {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		info, err := fs.Stat(f.inner, f.paths[name])
		if err != nil {
			return nil, xerrors.Errorf("stat migration %q: %w", name, err)
		}
		entries = append(entries, fs.FileInfoToDirEntry(info))
	}
	return entries, nil
}

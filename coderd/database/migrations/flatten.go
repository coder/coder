package migrations

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// archiveShardSize is the number of migration versions per archive directory,
// which is named for the range it holds, for example "000001-000100".
const archiveShardSize = 100

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

// flattenFS presents a migrations tree containing archive directories as a
// single flat directory.
//
// golang-migrate reads migrations from one directory and does not recurse, and
// it resolves a deployment's current schema version by reading that version's
// migration file. Every migration ever shipped therefore has to stay readable
// regardless of which archive directory holds it, or upgrades from older
// versions fail.
type flattenFS struct {
	inner fs.FS
	// paths maps each migration file name to its location within inner.
	paths map[string]string
	// entries is the flattened listing, sorted by name.
	entries []fs.DirEntry
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
			// testdata holds fixtures and schema dumps that reuse the migration
			// naming scheme but are not migrations.
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
		f.entries = append(f.entries, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(f.entries, func(i, j int) bool {
		return f.entries[i].Name() < f.entries[j].Name()
	})
	return f, nil
}

func (f *flattenFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &flattenRoot{entries: f.entries}, nil
	}
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
	return slices.Clone(f.entries), nil
}

// Close keeps the wrapper transparent to golang-migrate, which closes the fs.FS
// it is given when that value is an io.Closer.
func (f *flattenFS) Close() error {
	if closer, ok := f.inner.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// flattenRoot is the flattened tree's root directory, which has no counterpart
// in the underlying fs.
type flattenRoot struct {
	entries []fs.DirEntry
	offset  int
}

func (*flattenRoot) Stat() (fs.FileInfo, error) { return flattenRootInfo{}, nil }
func (*flattenRoot) Close() error               { return nil }

func (*flattenRoot) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: ".", Err: xerrors.New("is a directory")}
}

func (r *flattenRoot) ReadDir(n int) ([]fs.DirEntry, error) {
	remaining := r.entries[r.offset:]
	if n <= 0 {
		r.offset = len(r.entries)
		return slices.Clone(remaining), nil
	}
	if len(remaining) == 0 {
		return nil, io.EOF
	}
	remaining = remaining[:min(n, len(remaining))]
	r.offset += len(remaining)
	return slices.Clone(remaining), nil
}

type flattenRootInfo struct{}

func (flattenRootInfo) Name() string       { return "." }
func (flattenRootInfo) Size() int64        { return 0 }
func (flattenRootInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (flattenRootInfo) ModTime() time.Time { return time.Time{} }
func (flattenRootInfo) IsDir() bool        { return true }
func (flattenRootInfo) Sys() any           { return nil }

package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/xerrors"
)

const (
	changesFile   = "changes.diff"
	untrackedFile = "untracked.tar.zst"

	// Bundle content is agent-controlled, so every read is bounded.
	maxDiffBytes       = 128 << 20
	maxArchiveEntry    = 256 << 20
	maxArchiveTotal    = 1 << 30
	maxArchiveEntries  = 50000
	maxScannedFileSize = 4 << 20
	subprocessTimeout  = 2 * time.Minute
)

// gradeInput is the evidence one check run reads.
type gradeInput struct {
	tr      *transcript
	changes *workspaceChanges
}

func loadGradeInput(bundleDir, tracePath string) (*gradeInput, error) {
	tr, err := loadTranscript(bundleDir, tracePath)
	if err != nil {
		return nil, err
	}
	changes, err := loadWorkspaceChanges(bundleDir)
	if err != nil {
		return nil, err
	}
	return &gradeInput{tr: tr, changes: changes}, nil
}

// workspaceChanges is the trial's end state relative to the seed:
// tracked edits from changes.diff and new files from untracked.tar.zst.
type workspaceChanges struct {
	// Diff maps each path changed in changes.diff to its added lines.
	Diff map[string][]string
	// Deleted lists tracked paths the trial removed.
	Deleted map[string]bool
	// Untracked maps new file paths to their content (empty when larger
	// than maxScannedFileSize or not a regular file).
	Untracked map[string][]byte
	// Unscanned lists the paths whose final content the grader cannot
	// read: new regular files larger than maxScannedFileSize, and
	// tracked files that changes.diff holds as a binary patch.
	Unscanned map[string]bool
}

// changedPaths lists every path the trial created, modified, or deleted,
// except Python bytecode, which running the tests leaves behind and
// which is not an edit.
func (w *workspaceChanges) changedPaths() []string {
	seen := map[string]bool{}
	var out []string
	for p := range w.Diff {
		if !seen[p] && !interpreterCache(p) {
			seen[p] = true
			out = append(out, p)
		}
	}
	for p := range w.Untracked {
		if !seen[p] && !interpreterCache(p) {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func interpreterCache(p string) bool {
	return strings.HasSuffix(p, ".pyc") || strings.HasSuffix(p, ".pyo")
}

func loadWorkspaceChanges(bundleDir string) (*workspaceChanges, error) {
	w := &workspaceChanges{Diff: map[string][]string{}, Deleted: map[string]bool{}, Untracked: map[string][]byte{}, Unscanned: map[string]bool{}}
	diff, err := readCapped(filepath.Join(bundleDir, changesFile), maxDiffBytes)
	switch {
	case err == nil:
		var binary map[string]bool
		w.Diff, w.Deleted, binary = parseDiff(diff)
		for p := range binary {
			if !w.Deleted[p] {
				w.Unscanned[p] = true
			}
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return nil, err
	}
	f, err := os.Open(filepath.Join(bundleDir, untrackedFile))
	switch {
	case err == nil:
		defer f.Close()
		err = walkTarZst(f, func(hdr *tar.Header, body io.Reader) error {
			name, err := cleanArchiveName(hdr.Name)
			if err != nil {
				return err
			}
			if hdr.Typeflag != tar.TypeReg {
				if hdr.Typeflag != tar.TypeDir {
					w.Untracked[name] = nil
				}
				return nil
			}
			if hdr.Size > maxScannedFileSize {
				w.Untracked[name] = nil
				w.Unscanned[name] = true
				return nil
			}
			data, err := io.ReadAll(body)
			if err != nil {
				return err
			}
			w.Untracked[name] = data
			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("read %s: %w", untrackedFile, err)
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return nil, err
	}
	return w, nil
}

// parseDiff extracts changed paths, added lines, deleted paths, and the
// paths whose change is binary from a git diff. A binary patch encodes
// the file, so its lines hold none of the file's text.
func parseDiff(diff []byte) (files map[string][]string, deleted, binary map[string]bool) {
	files = map[string][]string{}
	deleted = map[string]bool{}
	binary = map[string]bool{}
	current := ""
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 1<<20), maxDiffBytes)
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			current = diffHeaderPath(strings.TrimPrefix(line, "diff --git "))
			if current != "" {
				if _, ok := files[current]; !ok {
					files[current] = nil
				}
			}
			inHunk = false
		case !inHunk && strings.HasPrefix(line, "deleted file mode"):
			if current != "" {
				deleted[current] = true
			}
		case !inHunk && strings.HasPrefix(line, "rename from "):
			// The source of a rename no longer exists, so keep and path
			// checks see both sides.
			if p := unquoteGitPath(strings.TrimPrefix(line, "rename from ")); p != "" {
				if _, ok := files[p]; !ok {
					files[p] = nil
				}
				deleted[p] = true
			}
		case !inHunk && strings.HasPrefix(line, "rename to "):
			if p := unquoteGitPath(strings.TrimPrefix(line, "rename to ")); p != "" {
				current = p
				files[current] = nil
			}
		case !inHunk && strings.HasPrefix(line, "+++ "):
			if p := unquoteGitPath(strings.TrimPrefix(line, "+++ ")); p != "" && p != "/dev/null" {
				current = strings.TrimPrefix(p, "b/")
				if _, ok := files[current]; !ok {
					files[current] = nil
				}
			}
		case !inHunk && line == "GIT binary patch":
			if current != "" {
				binary[current] = true
			}
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+") && current != "":
			files[current] = append(files[current], line[1:])
		default:
		}
	}
	return files, deleted, binary
}

// diffHeaderPath returns the path of a "diff --git a/P b/P" header, or ""
// when the header names two different paths, as a rename's does; the
// rename and ---/+++ lines that follow name those paths.
func diffHeaderPath(rest string) string {
	if strings.HasPrefix(rest, `"`) {
		// Quoted paths: a/"..." b/"..."; take the last quoted token.
		idx := strings.LastIndex(rest, ` "b/`)
		if idx < 0 {
			return ""
		}
		return strings.TrimPrefix(unquoteGitPath(rest[idx+1:]), "b/")
	}
	// An unquoted path may itself contain " b/", so split the header into
	// two equal halves instead of at a separator.
	n := len(rest) - len("a/ b/")
	if n <= 0 || n%2 != 0 {
		return ""
	}
	p := rest[len("a/") : len("a/")+n/2]
	if rest != "a/"+p+" b/"+p {
		return ""
	}
	return p
}

func unquoteGitPath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return ""
	}
	return s
}

// cleanArchiveName validates an archive entry name: relative, no ".."
// components, and not the workspace root .git.
func cleanArchiveName(name string) (string, error) {
	name = strings.TrimSuffix(name, "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return "", xerrors.Errorf("unsafe archive entry %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", xerrors.Errorf("unsafe archive entry %q", name)
	}
	first, _, _ := strings.Cut(clean, "/")
	if strings.EqualFold(first, ".git") {
		return "", xerrors.Errorf("archive entry %q writes the workspace .git", name)
	}
	return clean, nil
}

// walkTarZst calls fn for every entry of a zstd-compressed tar stream,
// enforcing entry-count and size caps.
func walkTarZst(r io.Reader, fn func(hdr *tar.Header, body io.Reader) error) error {
	dec, err := zstd.NewReader(r, zstd.WithDecoderMaxMemory(maxArchiveTotal))
	if err != nil {
		return xerrors.Errorf("open zstd stream: %w", err)
	}
	defer dec.Close()
	tr := tar.NewReader(dec)
	var total int64
	for entries := 0; ; entries++ {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("read tar: %w", err)
		}
		if entries >= maxArchiveEntries {
			return xerrors.Errorf("archive has more than %d entries", maxArchiveEntries)
		}
		if hdr.Size < 0 || hdr.Size > maxArchiveEntry {
			return xerrors.Errorf("archive entry %q is %d bytes, above the %d-byte cap", hdr.Name, hdr.Size, maxArchiveEntry)
		}
		total += hdr.Size
		if total > maxArchiveTotal {
			return xerrors.Errorf("archive exceeds %d bytes", maxArchiveTotal)
		}
		if err := fn(hdr, io.LimitReader(tr, hdr.Size)); err != nil {
			return err
		}
	}
}

// readTarZstMembers returns the named top-level members of a packed
// bundle without extracting anything to disk.
func readTarZstMembers(archive string, names ...string) (map[string][]byte, error) {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]byte{}
	err = walkTarZst(f, func(hdr *tar.Header, body io.Reader) error {
		name := path.Clean(hdr.Name)
		if hdr.Typeflag != tar.TypeReg || !want[name] {
			return nil
		}
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		out[name] = data
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("read %s: %w", archive, err)
	}
	return out, nil
}

// extractTarZst extracts a zstd-compressed tar into root. It never writes
// outside root: names are validated, symlinks must resolve inside root,
// hardlinks and device entries are rejected, and no entry is written
// through an existing symlink.
func extractTarZst(archive, root string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	return walkTarZst(f, func(hdr *tar.Header, body io.Reader) error {
		name, err := cleanArchiveName(hdr.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := ensureRealParents(root, name); err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if info, err := os.Lstat(target); err == nil {
				if info.IsDir() {
					return nil
				}
				if err := os.Remove(target); err != nil {
					return err
				}
			}
			return os.Mkdir(target, 0o755)
		case tar.TypeReg:
			if err := removeNonDir(target); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if hdr.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, body); err != nil {
				_ = out.Close()
				return err
			}
			return out.Close()
		case tar.TypeSymlink:
			if !symlinkStaysInside(name, hdr.Linkname) {
				return xerrors.Errorf("symlink %q -> %q escapes the workspace", name, hdr.Linkname)
			}
			if err := removeNonDir(target); err != nil {
				return err
			}
			return os.Symlink(hdr.Linkname, target)
		default:
			return xerrors.Errorf("archive entry %q has unsupported type %q", name, string(hdr.Typeflag))
		}
	})
}

func removeNonDir(target string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return xerrors.Errorf("%s is a directory", target)
	}
	return os.Remove(target)
}

// ensureRealParents creates the parent directories of name under root and
// fails when any existing parent is not a real directory.
func ensureRealParents(root, name string) error {
	dir := path.Dir(name)
	if dir == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(dir, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return xerrors.Errorf("archive entry %q descends through non-directory %q", name, current)
		}
	}
	return nil
}

func symlinkStaysInside(name, target string) bool {
	if target == "" || strings.HasPrefix(target, "/") || strings.Contains(target, `\`) {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return resolved != ".." && !strings.HasPrefix(resolved, "../") && !strings.EqualFold(strings.Split(resolved, "/")[0], ".git")
}

// rehydrate rebuilds a trial's final tree in dst: dst must already hold the
// seed as a git work tree. changes.diff is applied with git apply inside
// dst, then untracked.tar.zst is extracted over it.
func rehydrate(ctx context.Context, dst, bundleDir string) error {
	diffPath := filepath.Join(bundleDir, changesFile)
	if info, err := os.Stat(diffPath); err == nil && info.Size() > 0 {
		if info.Size() > maxDiffBytes {
			return xerrors.Errorf("%s exceeds %d bytes", changesFile, maxDiffBytes)
		}
		if out, err := runGit(ctx, dst, "apply", "--whitespace=nowarn", "--binary", diffPath); err != nil {
			return xerrors.Errorf("git apply %s: %s: %w", changesFile, truncate(string(out), 400), err)
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	untracked := filepath.Join(bundleDir, untrackedFile)
	if _, err := os.Stat(untracked); err == nil {
		if err := extractTarZst(untracked, dst); err != nil {
			return xerrors.Errorf("extract %s: %w", untrackedFile, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// runGit runs git in dir with a scrubbed environment: no user or system
// config, so hooks and filters from the host never apply.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	// Callers pass fixed git subcommands; bundle content only reaches git
	// as file paths inside temporary directories.
	// #nosec G204
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=chatd-evals",
		"GIT_AUTHOR_EMAIL=chatd-evals@localhost",
		"GIT_COMMITTER_NAME=chatd-evals",
		"GIT_COMMITTER_EMAIL=chatd-evals@localhost",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	}
	return cmd.CombinedOutput()
}

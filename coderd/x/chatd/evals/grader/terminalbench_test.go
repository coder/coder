package main

import (
	"archive/tar"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

const testDigest = "@sha256:90101b2e815323a8da20528a1439bebc407eb9761c9c68a3d557730856c878e9"

// An empty task /app must still produce a seed commit with files, since the
// aegis sidecar refuses an empty baseline, and verification must hand the
// task tests the /app the task defines.
func TestEmptySeedPlaceholder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "empty app", want: nil},
		{name: "task-owned gitkeep survives", files: map[string]string{".gitkeep": "", "data.txt": "x"}, want: []string{".gitkeep", "data.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			root := t.TempDir()
			appDir := filepath.Join(root, "app")
			require.NoError(t, os.Mkdir(appDir, 0o755))
			for name, body := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(appDir, name), []byte(body), 0o600))
			}

			placeholder, err := addEmptySeedPlaceholder(appDir)
			require.NoError(t, err)
			sha, err := commitSeed(ctx, appDir)
			require.NoError(t, err)
			tree, err := runGit(ctx, appDir, "ls-tree", "--name-only", "HEAD")
			require.NoError(t, err)
			require.NotEmpty(t, strings.TrimSpace(string(tree)), "seed commit has no files")
			seedRepo := filepath.Join(root, "seed.git")
			out, err := runGit(ctx, root, "clone", "-q", "--bare", appDir, seedRepo)
			require.NoError(t, err, string(out))

			seed := tbSeed{SeedRepo: "file://" + seedRepo, SeedRef: sha, Placeholder: placeholder}
			verifyDir := filepath.Join(root, "verify")
			require.NoError(t, materializeSeed(ctx, seed, verifyDir))
			require.NoError(t, os.RemoveAll(filepath.Join(verifyDir, ".git")))
			require.NoError(t, removeEmptySeedPlaceholder(verifyDir, seed))

			entries, err := os.ReadDir(verifyDir)
			require.NoError(t, err)
			var got []string
			for _, e := range entries {
				got = append(got, e.Name())
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// A tag can be moved to another image, so a task's image is used only at
// the digest tasks.json pins, and only from the repository task.toml names.
func TestPinnedImage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, declared, pin, want, err string
	}{
		{name: "pinned", declared: "alexgshaw/regex-log:20251031", pin: "alexgshaw/regex-log" + testDigest, want: "alexgshaw/regex-log" + testDigest},
		{name: "registry with port", declared: "registry.example:5000/tb/regex-log:1", pin: "registry.example:5000/tb/regex-log" + testDigest, want: "registry.example:5000/tb/regex-log" + testDigest},
		{name: "built image", declared: "", pin: "", want: ""},
		{name: "unpinned", declared: "alexgshaw/regex-log:20251031", pin: "", err: "pin it by digest"},
		{name: "pinned by tag", declared: "alexgshaw/regex-log:20251031", pin: "alexgshaw/regex-log:20251031", err: "pin it by digest"},
		{name: "other repository", declared: "alexgshaw/regex-log:20251031", pin: "other/regex-log" + testDigest, err: "not from alexgshaw/regex-log"},
		{name: "pin for a built image", declared: "", pin: "alexgshaw/regex-log" + testDigest, err: "names no docker_image"},
		{name: "short digest", declared: "alexgshaw/regex-log:20251031", pin: "alexgshaw/regex-log@sha256:abc", err: "pin it by digest"},
		{name: "declared by digest", declared: "alexgshaw/regex-log" + testDigest, pin: "alexgshaw/regex-log" + testDigest, want: "alexgshaw/regex-log" + testDigest},
		{name: "bare name", declared: "regex-log", pin: "regex-log" + testDigest, want: "regex-log" + testDigest},
		{name: "registry port without a tag", declared: "registry.example:5000/tb/regex-log", pin: "registry.example:5000/tb/regex-log" + testDigest, want: "registry.example:5000/tb/regex-log" + testDigest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := pinnedImage(tc.declared, tc.pin)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// Every task the suite ships names its image by digest, so imports on
// different hosts and days verify against the same image.
func TestTaskListPinsImages(t *testing.T) {
	t.Parallel()
	tasks, _, err := selectedTasks(filepath.Join("..", "terminalbench", "tasks.json"), nil)
	require.NoError(t, err)
	require.NotEmpty(t, tasks)
	for _, task := range tasks {
		require.Regexp(t, digestRef, task.Image, task.ID)
	}
}

// A partial import keeps each selected task's pin, and a task id that could
// leave the import directory is refused.
func TestSelectedTasks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	list := filepath.Join(dir, "tasks.json")
	require.NoError(t, os.WriteFile(list, []byte(`{"tasks":[{"id":"a","image":"x/a`+testDigest+`"},{"id":"b","image":"x/b`+testDigest+`"}]}`), 0o600))
	got, _, err := selectedTasks(list, []string{"b"})
	require.NoError(t, err)
	require.Equal(t, []tbTask{{ID: "b", Image: "x/b" + testDigest}}, got)

	unsafe := filepath.Join(dir, "unsafe.json")
	require.NoError(t, os.WriteFile(unsafe, []byte(`{"tasks":[{"id":"../escape"}]}`), 0o600))
	_, _, err = selectedTasks(unsafe, nil)
	require.ErrorContains(t, err, "unsafe task id")
}

// Task success is a full reward, however the verifier spells the number.
func TestRewardVerdict(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw    string
		passed bool
		reward float64
	}{
		{"1", true, 1},
		{"1.0", true, 1},
		{"1.00", true, 1},
		{"0", false, 0},
		{"0.5", false, 0.5},
		{"", false, 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			v, err := rewardVerdict("task", tc.raw, "/logs")
			require.NoError(t, err)
			require.Equal(t, tc.passed, v.Passed, v.Reason)
			require.InDelta(t, tc.reward, v.Metrics["reward"], 1e-9)
		})
	}
	_, err := rewardVerdict("task", "pass", "/logs")
	require.ErrorContains(t, err, "not a number")
}

// Task tests run agent code, which can leave anything in /logs/verifier.
// Copying that tree to the host keeps file contents only: a link there
// would make the grader read a host file when it opens reward.txt.
func TestExtractVerifierLogs(t *testing.T) {
	t.Parallel()
	canary := filepath.Join(t.TempDir(), "canary")
	require.NoError(t, os.WriteFile(canary, []byte("host secret"), 0o600))
	// Entries as docker cp streams /logs/verifier/. to stdout.
	archive := tarBytes(t, []tarEntry{
		{hdr: tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}},
		{hdr: tar.Header{Name: "./reward.txt", Typeflag: tar.TypeSymlink, Linkname: canary}},
		{hdr: tar.Header{Name: "./ctrf.json", Typeflag: tar.TypeReg, Mode: 0o644}, body: "{}"},
		{hdr: tar.Header{Name: "./ctrf-copy.json", Typeflag: tar.TypeLink, Linkname: "./ctrf.json"}},
		{hdr: tar.Header{Name: "./host.txt", Typeflag: tar.TypeLink, Linkname: canary}},
		{hdr: tar.Header{Name: "./sub/", Typeflag: tar.TypeDir, Mode: 0o755}},
		{hdr: tar.Header{Name: "./sub/up", Typeflag: tar.TypeSymlink, Linkname: "../.."}},
		{hdr: tar.Header{Name: "./sub/out.log", Typeflag: tar.TypeReg, Mode: 0o644}, body: "ok\n"},
		{hdr: tar.Header{Name: "./fifo", Typeflag: tar.TypeFifo}},
		{hdr: tar.Header{Name: "../outside.txt", Typeflag: tar.TypeReg}, body: "x"},
	})
	dst := filepath.Join(t.TempDir(), "verifier")

	skipped, err := extractVerifierLogs(bytes.NewReader(archive), dst)
	require.NoError(t, err)

	require.Equal(t, map[string]string{"ctrf.json": "{}", "ctrf-copy.json": "{}", "sub/": "", "sub/out.log": "ok\n"}, verifierTree(t, dst))
	require.Equal(t, []string{
		"reward.txt: symlink to " + canary,
		"host.txt: hard link to " + canary,
		"sub/up: symlink to ../..",
		"fifo: not a regular file or directory",
		"../outside.txt: outside the log directory",
	}, skipped)
}

// Archives that docker cp does not write, with entries before their
// directories or a hard link to another link, still extract.
func TestExtractVerifierLogsLayout(t *testing.T) {
	t.Parallel()
	reg := func(name, body string) tarEntry {
		return tarEntry{hdr: tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644}, body: body}
	}
	dir := func(name string) tarEntry {
		return tarEntry{hdr: tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}}
	}
	link := func(name, target string) tarEntry {
		return tarEntry{hdr: tar.Header{Name: name, Typeflag: tar.TypeLink, Linkname: target}}
	}
	for _, tc := range []struct {
		name    string
		entries []tarEntry
		want    map[string]string
	}{
		{name: "nested dir without its parent", entries: []tarEntry{dir("a/b/")}, want: map[string]string{"a/": "", "a/b/": ""}},
		{name: "file before its dir entry", entries: []tarEntry{reg("a/x.txt", "x"), dir("a/")}, want: map[string]string{"a/": "", "a/x.txt": "x"}},
		{name: "hard link without parent entries", entries: []tarEntry{reg("x.txt", "x"), link("sub/y.txt", "x.txt")}, want: map[string]string{"x.txt": "x", "sub/": "", "sub/y.txt": "x"}},
		{name: "hard link to a hard link", entries: []tarEntry{reg("a.txt", "a"), link("b.txt", "a.txt"), link("c.txt", "b.txt")}, want: map[string]string{"a.txt": "a", "b.txt": "a", "c.txt": "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := filepath.Join(t.TempDir(), "verifier")
			skipped, err := extractVerifierLogs(bytes.NewReader(tarBytes(t, tc.entries)), dst)
			require.NoError(t, err)
			require.Empty(t, skipped)
			require.Equal(t, tc.want, verifierTree(t, dst))
		})
	}
}

// The reward is reward.txt in the copied logs, trimmed. A missing reward.txt
// is reward 0; any other read error, or a failed copy, fails verification.
// It runs every docker command against a stub, which needs PATH, so it is
// not parallel.
//
//nolint:paralleltest // t.Setenv puts the docker stub first on PATH.
func TestVerifyInTaskImageWithDockerStub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the docker stub is a POSIX shell script")
	}
	seed := tbSeed{Image: "img"}
	reward := tarBytes(t, []tarEntry{{hdr: tar.Header{Name: "./reward.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: " 1\n"}})
	verify := func(t *testing.T) (verifyResult, string, error) {
		ctx := testutil.Context(t, testutil.WaitMedium)
		logs := filepath.Join(t.TempDir(), "logs")
		res, err := verifyInTaskImage(ctx, seed, t.TempDir(), "", logs)
		return res, logs, err
	}
	t.Run("reward from the verifier directory", func(t *testing.T) {
		stubDocker(t, reward, 0, 0)
		res, logs, err := verify(t)
		require.NoError(t, err)
		require.Equal(t, "1", res.Reward)
		require.FileExists(t, filepath.Join(logs, "verifier", "reward.txt"))
	})
	t.Run("symlinked reward is listed, not read", func(t *testing.T) {
		stubDocker(t, tarBytes(t, []tarEntry{{hdr: tar.Header{Name: "./reward.txt", Typeflag: tar.TypeSymlink, Linkname: "/etc/hostname"}}}), 0, 0)
		res, logs, err := verify(t)
		require.NoError(t, err)
		require.Empty(t, res.Reward)
		out, err := os.ReadFile(filepath.Join(logs, "verifier-output.txt"))
		require.NoError(t, err)
		require.Contains(t, string(out), "reward.txt: symlink to /etc/hostname")
	})
	t.Run("reward directory is an error", func(t *testing.T) {
		stubDocker(t, tarBytes(t, []tarEntry{{hdr: tar.Header{Name: "./reward.txt/", Typeflag: tar.TypeDir, Mode: 0o755}}}), 0, 0)
		_, _, err := verify(t)
		require.ErrorContains(t, err, "read reward")
	})
	t.Run("failed docker cp is an error", func(t *testing.T) {
		stubDocker(t, reward, 0, 1)
		_, _, err := verify(t)
		require.ErrorContains(t, err, "copy verifier logs")
	})
	t.Run("output after the archive does not block the copy", func(t *testing.T) {
		stubDocker(t, reward, 1<<20, 0)
		res, _, err := verify(t)
		require.NoError(t, err)
		require.Equal(t, "1", res.Reward)
	})
}

// stubDocker puts a docker stub first on PATH. Its cp to stdout writes
// archive, then junk bytes, then exits with cpExit; every other command
// succeeds, and run prints a container id.
func stubDocker(t *testing.T, archive []byte, junk, cpExit int) {
	t.Helper()
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "verifier.tar")
	require.NoError(t, os.WriteFile(archivePath, archive, 0o600))
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"cp)\n" +
		"  if [ \"$3\" = \"-\" ]; then\n" +
		"    cat '" + archivePath + "'\n" +
		"    if [ " + strconv.Itoa(junk) + " -gt 0 ]; then printf '%" + strconv.Itoa(junk) + "s' ''; fi\n" +
		"    exit " + strconv.Itoa(cpExit) + "\n" +
		"  fi\n" +
		"  ;;\n" +
		"run)\n" +
		"  echo stub-container\n" +
		"  ;;\n" +
		"esac\n" +
		"exit 0\n"
	stub := filepath.Join(dir, "docker")
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o600))
	require.NoError(t, os.Chmod(stub, 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// verifierTree maps each path under dst, with slashes, to its contents,
// with a trailing slash for a directory.
func verifierTree(t *testing.T, dst string) map[string]string {
	t.Helper()
	got := map[string]string{}
	require.NoError(t, filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dst, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case rel == ".":
		case d.IsDir():
			got[rel+"/"] = ""
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			got[rel] = string(data)
		default:
			got[rel] = d.Type().String()
		}
		return nil
	}))
	return got
}

// A verifier that logs without end must not grow the grader's memory
// without bound; the kept output is its tail, marked as cut.
func TestTailBuffer(t *testing.T) {
	t.Parallel()
	b := &tailBuffer{limit: 8}
	for _, chunk := range []string{"abc", "defgh", "ijklmnopq", "r", "stuvwxyz"} {
		n, err := b.Write([]byte(chunk))
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	require.Equal(t, "[18 earlier bytes dropped]\nstuvwxyz", string(b.Bytes()))

	long := &tailBuffer{limit: 64}
	line := []byte("0123456789\n")
	for range 100000 {
		_, err := long.Write(line)
		require.NoError(t, err)
	}
	require.LessOrEqual(t, cap(long.buf), 4*long.limit, "the buffer stays bounded")
	require.True(t, strings.HasPrefix(string(long.Bytes()), "[1099936 earlier bytes dropped]\n"))

	short := &tailBuffer{limit: 8}
	_, err := short.Write([]byte("ok\n"))
	require.NoError(t, err)
	require.Equal(t, "ok\n", string(short.Bytes()))
}

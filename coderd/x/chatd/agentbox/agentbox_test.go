package agentbox_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func newEngine(t *testing.T, opts agentbox.Options) *agentbox.Engine {
	t.Helper()
	if opts.RootDir == "" {
		opts.RootDir = t.TempDir()
	}
	opts.Logger = testutil.Logger(t)
	engine, err := agentbox.NewEngine(t.Context(), opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	return engine
}

func newBox(t *testing.T, engine *agentbox.Engine) *agentbox.Box {
	t.Helper()
	box, err := engine.NewBox()
	require.NoError(t, err)
	t.Cleanup(func() { _ = box.Close() })
	return box
}

func runJS(t *testing.T, box *agentbox.Box, code string) agentbox.RunResult {
	t.Helper()
	result, err := box.Run(t.Context(), agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: code})
	require.NoError(t, err)
	return result
}

func TestRun(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, agentbox.Options{})

	t.Run("HelloWorld", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `console.log("hello"); std.err.puts("oops\n"); std.exit(3);`)
		assert.Equal(t, 3, result.ExitCode)
		assert.Equal(t, "hello\n", result.Stdout)
		assert.Equal(t, "oops\n", result.Stderr)
		assert.False(t, result.TimedOut)
		assert.False(t, result.Canceled)
		assert.False(t, result.StdoutTruncated)
		assert.False(t, result.DiskQuotaExceeded)
		assert.False(t, result.OpenFileLimitReached)
	})

	t.Run("NegativeExit", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `std.exit(-2);`)
		assert.Equal(t, -2, result.ExitCode)
		assert.False(t, result.TimedOut)
		assert.False(t, result.Canceled)
	})

	t.Run("CallerCancel", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock})
		box := newBox(t, engine)
		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan agentbox.RunResult, 1)
		go func() {
			result, err := box.Run(runCtx, agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: `for (;;) {}`})
			assert.NoError(t, err)
			done <- result
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)
		cancel()
		result := testutil.RequireReceive(ctx, t, done)
		assert.Equal(t, agentbox.ExitCodeInterrupted, result.ExitCode)
		assert.True(t, result.Canceled)
		assert.False(t, result.TimedOut)
	})

	t.Run("StdinAndArgs", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result, err := box.Run(t.Context(), agentbox.RunRequest{
			Language: agentbox.LanguageJavaScript,
			Code:     `console.log(std.in.readAsString().trim(), scriptArgs.slice(1).join(","));`,
			Stdin:    "from stdin\n",
			Args:     []string{"a", "b"},
		})
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		assert.Equal(t, "from stdin a,b\n", result.Stdout)
	})

	t.Run("WallClock", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `console.log(Date.now());`)
		require.Equal(t, 0, result.ExitCode)
		var millis int64
		for _, c := range strings.TrimSpace(result.Stdout) {
			millis = millis*10 + int64(c-'0')
		}
		// wazero's default fake clock reports 2022-01-01.
		assert.Greater(t, millis, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
	})

	t.Run("UnknownLanguage", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		_, err := box.Run(t.Context(), agentbox.RunRequest{Language: "cobol", Code: "x"})
		require.ErrorIs(t, err, agentbox.ErrUnknownLanguage)
	})

	t.Run("Languages", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{agentbox.LanguageJavaScript}, engine.Languages())
	})
}

func TestFiles(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, agentbox.Options{})

	t.Run("GuestWriteThenRead", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `const f = std.open("/box/a.txt", "w"); f.puts("one\ntwo\n"); f.close();`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		result = runJS(t, box, `console.log(std.loadFile("/box/a.txt"));`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "one\ntwo\n\n", result.Stdout)

		data, err := box.ReadFile("/box/a.txt", 1<<20)
		require.NoError(t, err)
		assert.Equal(t, "one\ntwo\n", string(data))
	})

	t.Run("HostWriteThenGuestRead", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		require.NoError(t, box.WriteFile("nested/dir/in.json", []byte(`{"n":41}`)))
		result := runJS(t, box, `const v = JSON.parse(std.loadFile("/box/nested/dir/in.json")); console.log(v.n + 1);`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "42\n", result.Stdout)
	})

	t.Run("ESModuleImport", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		require.NoError(t, box.WriteFile("/box/lib.mjs", []byte(`export const answer = 42;`)))
		result := runJS(t, box, `import { answer } from "/box/lib.mjs"; console.log(answer);`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "42\n", result.Stdout)
		// Relative specifiers resolve against the /script mount.
		result = runJS(t, box, `import { answer } from "./lib.mjs"; console.log(answer);`)
		assert.NotEqual(t, 0, result.ExitCode)
		assert.Empty(t, result.Stdout)
	})

	t.Run("ReadLines", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		require.NoError(t, box.WriteFile("lines.txt", []byte("a\nb\nc\nd")))
		res, err := box.ReadLines("lines.txt", 2, 2)
		require.NoError(t, err)
		assert.Equal(t, "2\tb\n3\tc", res.Content)
		assert.Equal(t, 4, res.TotalLines)
		assert.Equal(t, 2, res.LinesRead)
		assert.Equal(t, int64(7), res.FileSize)

		res, err = box.ReadLines("lines.txt", 0, 0)
		require.NoError(t, err)
		assert.Equal(t, 4, res.LinesRead)

		_, err = box.ReadLines("lines.txt", 9, 0)
		require.ErrorContains(t, err, "beyond the file length")

		_, err = box.ReadLines("missing.txt", 1, 0)
		require.ErrorContains(t, err, "open")
	})

	t.Run("ScriptNotUnderBox", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `
			console.log(JSON.stringify(os.readdir("/box")[0]));
			const [st, err] = os.stat("/script/main.js");
			console.log(err === 0);
			const f = std.open("/script/main.js", "w");
			console.log(f === null);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "[\".\",\"..\"]\ntrue\ntrue\n", result.Stdout)
	})

	t.Run("PathValidation", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		for _, p := range []string{"", "/", "/box", "/box/", "/etc/passwd", "../x", "/box/../x", "a/../../x"} {
			require.Error(t, box.WriteFile(p, []byte("x")), p)
			_, err := box.ReadFile(p, 10)
			require.Error(t, err, p)
		}
	})
}

func TestIsolation(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, agentbox.Options{})

	t.Run("NoHostPaths", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `
			console.log(std.loadFile("/etc/passwd") === null);
			console.log(std.loadFile("/box/../etc/passwd") === null);
			console.log(std.loadFile("/") === null);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "true\ntrue\ntrue\n", result.Stdout)
	})

	t.Run("NoSymlinks", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		// This quickjs build has no symlink or link binding; the adapter
		// denial is covered by the internal fs test. After a run that
		// creates entries, the root must hold no symlink.
		result := runJS(t, box, `
			os.mkdir("/box/d");
			const f = std.open("/box/d/f.txt", "w"); f.puts("x"); f.close();
			os.rename("/box/d/f.txt", "/box/d/g.txt");
			console.log(typeof os.symlink, typeof os.link);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "undefined undefined\n", result.Stdout)

		boxRoot := filepath.Join(engine.RootDir(), box.ID(), "root")
		entries := 0
		err := filepath.WalkDir(boxRoot, func(_ string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			assert.Zero(t, d.Type()&fs.ModeSymlink, "no symlink may exist under the box root")
			entries++
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, entries)
	})

	t.Run("HostSymlinkNotFollowed", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		outside := filepath.Join(t.TempDir(), "secret")
		require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
		boxRoot := filepath.Join(engine.RootDir(), box.ID(), "root")
		require.NoError(t, os.Symlink(outside, filepath.Join(boxRoot, "link")))
		_, err := box.ReadFile("link", 100)
		require.Error(t, err)
		require.Error(t, box.WriteFile("link", []byte("x")))
	})

	t.Run("BoxRootProtected", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `
			os.mkdir("/box/d");
			console.log(os.remove("/box") < 0, os.rename("/box", "/box/d/x") < 0, os.rename("/box/d", "/box") < 0);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "true true true\n", result.Stdout)
		require.NoError(t, box.WriteFile("a.txt", []byte("x")))
		result = runJS(t, box, `console.log(std.loadFile("/box/a.txt"));`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "x\n", result.Stdout)
	})
}

func TestLimits(t *testing.T) {
	t.Parallel()

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock, Limits: agentbox.Limits{RunTimeout: time.Minute}})
		box := newBox(t, engine)

		done := make(chan agentbox.RunResult, 1)
		go func() {
			result, err := box.Run(context.Background(), agentbox.RunRequest{
				Language: agentbox.LanguageJavaScript,
				Code:     `for (;;) {}`,
			})
			assert.NoError(t, err)
			done <- result
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Minute).MustWait(ctx)
		result := testutil.RequireReceive(ctx, t, done)
		assert.True(t, result.TimedOut)
		assert.False(t, result.Canceled)
		assert.Equal(t, agentbox.ExitCodeInterrupted, result.ExitCode)
	})

	t.Run("Memory", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock, Limits: agentbox.Limits{MemoryBytes: 16 << 20}})
		box := newBox(t, engine)

		done := make(chan agentbox.RunResult, 1)
		go func() {
			result, err := box.Run(context.Background(), agentbox.RunRequest{
				Language: agentbox.LanguageJavaScript,
				Code:     `const a = []; for (;;) { a.push(new Uint8Array(1 << 20)); }`,
			})
			assert.NoError(t, err)
			done <- result
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)
		// Either the guest aborts on allocation failure or the deadline
		// ends it; both must return without affecting the host.
		select {
		case result := <-done:
			assert.NotEqual(t, 0, result.ExitCode)
		case <-time.After(2 * time.Second):
			mClock.Advance(agentbox.DefaultRunTimeout).MustWait(ctx)
			result := testutil.RequireReceive(ctx, t, done)
			assert.True(t, result.TimedOut)
		}
	})

	t.Run("Output", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{Limits: agentbox.Limits{OutputBytes: 1024}})
		box := newBox(t, engine)
		result := runJS(t, box, `for (let i = 0; i < 1000; i++) console.log("0123456789");`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.True(t, result.StdoutTruncated)
		assert.False(t, result.StderrTruncated)
		assert.Len(t, result.Stdout, 1024)
	})

	t.Run("DiskBytes", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{Limits: agentbox.Limits{DiskBytes: 64 << 10}})
		box := newBox(t, engine)
		result := runJS(t, box, `
			const f = std.open("/box/big", "w");
			let n = 0;
			const chunk = "x".repeat(4096);
			for (let i = 0; i < 100; i++) { if (!f.puts(chunk)) break; f.flush(); if (f.error()) break; n++; }
			f.close();
			console.log(n);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		written, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
		require.NoError(t, err)
		assert.Less(t, written, 100, "writes must fail before 400 KiB")
	})

	t.Run("EntryCount", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{Limits: agentbox.Limits{DiskBytes: 64 << 10}})
		box := newBox(t, engine)
		result := runJS(t, box, `
			let n = 0;
			for (let i = 0; i < 1000; i++) { if (os.mkdir("/box/d" + i) !== 0) break; n++; }
			console.log(n);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "16\n", result.Stdout)
	})

	t.Run("HostWriteQuota", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{Limits: agentbox.Limits{DiskBytes: 8 << 10}})
		box := newBox(t, engine)
		require.NoError(t, box.WriteFile("a", make([]byte, 2048)))
		err := box.WriteFile("b", make([]byte, 4096))
		require.ErrorContains(t, err, "quota")
		// Replacing a file refunds its previous size.
		require.NoError(t, box.WriteFile("a", make([]byte, 3072)))
	})

	t.Run("SparseWriteCharged", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{Limits: agentbox.Limits{DiskBytes: 1 << 20}})
		box := newBox(t, engine)
		result := runJS(t, box, `
			const fd = os.open("/box/s", os.O_RDWR | os.O_CREAT, 0o600);
			os.seek(fd, 2 ** 40, std.SEEK_SET);
			const sparse = os.write(fd, new Uint8Array(1).buffer, 0, 1);
			os.close(fd);
			os.remove("/box/s");
			const g = os.open("/box/big", os.O_RDWR | os.O_CREAT, 0o600);
			const big = os.write(g, new Uint8Array(2 << 20).buffer, 0, 2 << 20);
			os.close(g);
			console.log(sparse < 0, big < 0);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.True(t, result.DiskQuotaExceeded)
		assert.False(t, result.OpenFileLimitReached)
		assert.Equal(t, "true true\n", result.Stdout)
		require.ErrorContains(t, box.WriteFile("host.bin", make([]byte, 2<<20)), "quota")
	})

	t.Run("UnlinkWhileOpenCharged", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{Limits: agentbox.Limits{DiskBytes: 1 << 20}})
		box := newBox(t, engine)
		result := runJS(t, box, `
			const buf = new Uint8Array(900 << 10);
			let ok = 0;
			for (let i = 0; i < 20; i++) {
				const p = "/box/o" + i;
				const fd = os.open(p, os.O_RDWR | os.O_CREAT, 0o600);
				if (os.write(fd, buf.buffer, 0, buf.length) === buf.length) ok++;
				os.remove(p);
			}
			console.log(ok);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.True(t, result.DiskQuotaExceeded)
		assert.False(t, result.OpenFileLimitReached)
		assert.Equal(t, "1\n", result.Stdout)
		// The run's handles closed when it ended, releasing the bytes.
		require.NoError(t, box.WriteFile("after.bin", make([]byte, 900<<10)))
	})

	t.Run("OpenFiles", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{})
		box := newBox(t, engine)
		require.NoError(t, box.WriteFile("f", nil))
		result := runJS(t, box, `
			let n = 0;
			for (let i = 0; i < 100000; i++) { if (os.open("/box/f", os.O_RDONLY) < 0) break; n++; }
			console.log(n);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.True(t, result.OpenFileLimitReached)
		assert.False(t, result.DiskQuotaExceeded)
		opened, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
		require.NoError(t, err)
		assert.Greater(t, opened, 200)
		assert.Less(t, opened, 256)
		// The limit is per run.
		result = runJS(t, box, `console.log(os.open("/box/f", os.O_RDONLY) >= 0);`)
		assert.Equal(t, "true\n", result.Stdout)
	})

	t.Run("MaxBoxes", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{MaxBoxes: 1})
		box := newBox(t, engine)
		_, err := engine.NewBox()
		require.ErrorIs(t, err, agentbox.ErrTooManyBoxes)
		require.NoError(t, box.Close())
		again, err := engine.NewBox()
		require.NoError(t, err)
		require.NoError(t, again.Close())
	})

	t.Run("Busy", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock, MaxConcurrent: 1})
		first := newBox(t, engine)
		second := newBox(t, engine)

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = first.Run(context.Background(), agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: `for (;;) {}`})
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)

		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := second.Run(canceled, agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: `1`})
		require.ErrorIs(t, err, agentbox.ErrBusy)

		require.NoError(t, first.Close())
		testutil.TryReceive(ctx, t, done)
	})
}

func TestLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("CloseRemovesDirAndCancelsRun", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock})
		box := newBox(t, engine)
		dir := filepath.Join(engine.RootDir(), box.ID())
		require.DirExists(t, dir)

		done := make(chan agentbox.RunResult, 1)
		go func() {
			result, err := box.Run(context.Background(), agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: `for (;;) {}`})
			assert.NoError(t, err)
			done <- result
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)

		require.NoError(t, box.Close())
		result := testutil.RequireReceive(ctx, t, done)
		assert.False(t, result.TimedOut)
		assert.True(t, result.Canceled)
		assert.Equal(t, agentbox.ExitCodeInterrupted, result.ExitCode)
		require.NoDirExists(t, dir)

		require.NoError(t, box.Close())
		_, err := box.Run(ctx, agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: `1`})
		require.ErrorIs(t, err, agentbox.ErrClosed)
		require.ErrorIs(t, box.WriteFile("x", nil), agentbox.ErrClosed)
		_, err = box.ReadFile("x", 1)
		require.ErrorIs(t, err, agentbox.ErrClosed)
	})

	t.Run("CloseStopsQueuedRun", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock})
		box := newBox(t, engine)
		loop := agentbox.RunRequest{Language: agentbox.LanguageJavaScript, Code: `for (;;) {}`}

		first := make(chan error, 1)
		go func() {
			_, err := box.Run(context.Background(), loop)
			first <- err
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)

		// The second run queues behind the first. The mock clock never
		// fires the timeout, so Close returns only if the queued run
		// stops without executing.
		second := make(chan error, 1)
		go func() {
			_, err := box.Run(context.Background(), loop)
			second <- err
		}()
		require.NoError(t, box.Close())
		require.NoError(t, testutil.RequireReceive(ctx, t, first))
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, second), agentbox.ErrClosed)
	})

	t.Run("EngineCloseWaitsForAsyncClose", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{})
		box, err := engine.NewBox()
		require.NoError(t, err)
		dir := filepath.Join(engine.RootDir(), box.ID())
		box.CloseAsync()
		require.NoError(t, engine.Close(t.Context()))
		require.ErrorIs(t, box.WriteFile("x", nil), agentbox.ErrClosed)
		require.NoDirExists(t, dir)
	})

	t.Run("EngineCloseRemovesRoot", func(t *testing.T) {
		t.Parallel()
		engine := newEngine(t, agentbox.Options{})
		root := engine.RootDir()
		require.DirExists(t, root)
		require.NoError(t, engine.Close(t.Context()))
		require.NoDirExists(t, root)
		_, err := engine.NewBox()
		require.ErrorIs(t, err, agentbox.ErrClosed)
	})

	t.Run("Sweep", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		mClock := quartz.NewMock(t)
		old := mClock.Now().Add(-48 * time.Hour)

		stale := filepath.Join(parent, "coder-agent-boxes-stale")
		require.NoError(t, os.MkdirAll(filepath.Join(stale, "box", "root"), 0o700))
		require.NoError(t, os.Chtimes(stale, old, old))

		fresh := filepath.Join(parent, "coder-agent-boxes-fresh")
		require.NoError(t, os.Mkdir(fresh, 0o700))

		// A live engine holds its lock; its root must survive even when old.
		live := newEngine(t, agentbox.Options{RootDir: parent, Clock: mClock})
		require.NoError(t, os.Chtimes(live.RootDir(), old, old))

		other := filepath.Join(parent, "other-dir")
		require.NoError(t, os.Mkdir(other, 0o700))
		require.NoError(t, os.Chtimes(other, old, old))

		target := t.TempDir()
		require.NoError(t, os.Chtimes(target, old, old))
		link := filepath.Join(parent, "coder-agent-boxes-link")
		require.NoError(t, os.Symlink(target, link))

		sweeper := newEngine(t, agentbox.Options{RootDir: parent, Clock: mClock})

		require.NoDirExists(t, stale)
		require.DirExists(t, fresh)
		require.DirExists(t, live.RootDir())
		require.DirExists(t, other)
		require.DirExists(t, target)
		_, err := os.Lstat(link)
		require.NoError(t, err)
		require.DirExists(t, sweeper.RootDir())
	})
}

func TestProvenance(t *testing.T) {
	t.Parallel()
	version, err := os.ReadFile("runtimes/quickjs/VERSION")
	require.NoError(t, err)
	var want string
	for line := range strings.Lines(string(version)) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "sha256="); ok {
			want = v
		}
	}
	require.NotEmpty(t, want)
	assert.Equal(t, want, agentbox.EmbeddedRuntimeSHA256(agentbox.LanguageJavaScript))
	assert.Empty(t, agentbox.EmbeddedRuntimeSHA256("cobol"))
}

import assert from 'node:assert/strict';
import { EventEmitter, once } from 'node:events';
import { writeSync } from 'node:fs';
import { chmod, mkdir, mkdtemp, readFile, readdir, readlink, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { describe, test } from 'node:test';
import { codexSocketPath, ensureCodexServer } from '../src/codex-server.mjs';

const initialized = { userAgent: 'codex_cli_rs/0.164.0 (Linux; x86_64)', platformFamily: 'unix', codexHome: '/private/.codex' };

function clock() {
  const timers = new Map();
  const events = new EventEmitter();
  let next = 0;
  return {
    timers,
    setTimer(callback, ms) { const id = ++next; timers.set(id, { callback, ms }); events.emit('timer'); return id; },
    clearTimer(id) { timers.delete(id); },
    async waitFor(ms) {
      while (true) {
        const found = [...timers].find(([, timer]) => timer.ms === ms);
        if (found) return found;
        await once(events, 'timer');
      }
    },
    async expire(ms) {
      const [id, timer] = await this.waitFor(ms);
      timers.delete(id);
      timer.callback();
    },
  };
}

async function fixture(t, sequence = [null, initialized]) {
  const home = await mkdtemp('/tmp/opencode/bridge-native-');
  t.after(() => rm(home, { recursive: true, force: true }));
  const events = new EventEmitter();
  const child = new EventEmitter();
  const f = {
    home, stateFile: join(home, 'bridge.json'), child, time: clock(),
    calls: [], spawns: [], kills: [], unrefs: 0, controller: new AbortController(),
    async waitForProbes(count) { while (f.calls.length < count) await once(events, 'probe'); },
  };
  child.pid = 12345;
  child.kill = signal => { f.kills.push(signal); return true; };
  child.unref = () => { f.unrefs++; };
  f.probe = options => {
    f.calls.push(options);
    const next = sequence[f.calls.length - 1];
    events.emit('probe');
    return typeof next === 'function' ? next(options) : next;
  };
  f.spawnProcess = (...args) => { f.spawns.push(args); return child; };
  f.start = options => ensureCodexServer({
    stateFile: f.stateFile, signal: f.controller.signal, probe: f.probe,
    spawnProcess: f.spawnProcess, ...f.time, ...options,
  });
  return f;
}

async function nativeFiles(f) {
  // Native rendezvous files can be symlinks. Their targets and lock/PID records
  // belong to Codex, not this lifecycle helper, even when a PID looks stale.
  await writeFile(join(f.home, 'native-target'), 'do not change');
  await symlink('native-target', join(f.home, 'codex.sock'));
  await writeFile(join(f.home, 'codex.pid'), '99999999');
  await writeFile(join(f.home, 'codex.lock'), 'native lock');
  return async () => {
    assert.equal(await readlink(join(f.home, 'codex.sock')), 'native-target');
    assert.equal(await readFile(join(f.home, 'native-target'), 'utf8'), 'do not change');
    assert.equal(await readFile(join(f.home, 'codex.pid'), 'utf8'), '99999999');
    assert.equal(await readFile(join(f.home, 'codex.lock'), 'utf8'), 'native lock');
  };
}

describe('unmanaged native Codex server lifecycle', { concurrency: true, timeout: 5000 }, () => {
  test('socket path is absolute and deterministic for the state directory', () => {
    assert.equal(codexSocketPath('relative/bridge.json'), resolve('relative/codex.sock'));
    assert.equal(codexSocketPath('/private/bridge.json'), '/private/codex.sock');
    assert.equal(codexSocketPath('/private/other.json'), '/private/codex.sock');
  });

  test('reuses the running pinned version without changing native files or opening a log', async t => {
    const f = await fixture(t, [initialized]);
    const unchanged = await nativeFiles(f);
    const before = await readdir(f.home);
    const result = await f.start();
    assert.equal(result.started, false);
    assert.equal(result.socketPath, join(f.home, 'codex.sock'));
    assert.equal(f.calls.length, 1);
    assert.equal(f.calls[0].socketPath, result.socketPath);
    assert.equal(f.calls[0].timeoutMs, 1000);
    assert.equal(f.spawns.length, 0);
    assert.equal(f.time.timers.size, 0);
    assert.deepEqual(await readdir(f.home), before);
    f.controller.abort();
    assert.deepEqual(f.kills, []);
    await unchanged();
  });

  for (const response of [
    { ...initialized, userAgent: 'codex_cli_rs/0.165.0 secret' },
    { ...initialized, userAgent: 'codex_cli_rs/0.164.01' },
    { ...initialized, userAgent: 'codex_cli_rs/0.164.0-beta' },
    { ...initialized, userAgent: 'prefix codex_cli_rs/0.164.0' },
    { ...initialized, platformFamily: 'windows' },
    { ...initialized, codexHome: 'relative' }, {},
  ]) {
    test('rejects incompatible initialization without mutation: ' + JSON.stringify(response), async t => {
      const f = await fixture(t, [response]);
      const unchanged = await nativeFiles(f);
      const before = await readdir(f.home);
      await assert.rejects(f.start(), error => /exactly Codex 0\.164\.0/.test(error.message) && !error.message.includes('secret'));
      assert.deepEqual(f.spawns, []);
      assert.deepEqual(f.kills, []);
      assert.equal(f.time.timers.size, 0);
      assert.deepEqual(await readdir(f.home), before);
      await unchanged();
    });
  }

  test('spawns only a detached explicit app-server, appends private logs and survives later broker abort', async t => {
    const f = await fixture(t, [null, null, initialized]);
    const unchanged = await nativeFiles(f);
    const logFile = join(f.home, 'codex-server.log');
    await writeFile(logFile, 'previous\n', { mode: 0o600 });
    const running = f.start({ spawnProcess: (...args) => {
      const child = f.spawnProcess(...args);
      writeSync(args[2].stdio[1], 'native output\n');
      return child;
    } });
    await f.time.expire(100);
    const result = await running;
    assert.deepEqual(result, { socketPath: join(f.home, 'codex.sock'), logFile, started: true, pid: f.child.pid });
    const [[command, args, options]] = f.spawns;
    assert.equal(command, 'codex');
    assert.deepEqual(args, ['app-server', '--listen', 'unix://' + result.socketPath]);
    assert.equal(options.detached, true);
    assert.equal(options.stdio[0], 'ignore');
    assert.equal(options.stdio[1], options.stdio[2]);
    assert.equal(options.signal, undefined, 'successful server must not inherit the broker abort signal');
    assert.equal(await readFile(logFile, 'utf8'), 'previous\nnative output\n');
    assert.equal((await stat(logFile)).mode & 0o777, 0o600);
    assert.equal(f.unrefs, 1);
    assert.equal(f.time.timers.size, 0);
    f.controller.abort();
    assert.deepEqual(f.kills, []);
    await unchanged();
  });

  test('a competing native bind can win without restarting or killing the winner', async t => {
    const f = await fixture(t);
    const result = await f.start({ spawnProcess: (...args) => {
      const child = f.spawnProcess(...args);
      queueMicrotask(() => child.emit('exit', 1, null));
      return child;
    } });
    assert.equal(result.started, false);
    assert.equal(result.pid, undefined);
    assert.equal(f.spawns.length, 1);
    assert.deepEqual(f.kills, []);
  });

  test('bounded initial probe aborts a hung connection before attempting native bind', async t => {
    const f = await fixture(t, [() => new Promise(() => {}), initialized]);
    const running = f.start();
    await f.time.expire(1000);
    const result = await running;
    assert.equal(result.started, true);
    assert.equal(f.calls[0].signal.aborted, true);
    assert.equal(f.time.timers.size, 0);
  });

  test('overall deadline kills only the newly spawned process and aborts a pending probe', async t => {
    const f = await fixture(t, [null, () => new Promise(() => {})]);
    const unchanged = await nativeFiles(f);
    const rejected = assert.rejects(f.start(), /readiness timed out/);
    await f.waitForProbes(2);
    await f.time.expire(20000);
    await rejected;
    assert.equal(f.calls[1].signal.aborted, true);
    assert.deepEqual(f.kills, ['SIGTERM']);
    assert.equal(f.unrefs, 1);
    assert.equal(f.time.timers.size, 0);
    await unchanged();
  });

  for (const where of ['before startup', 'existing probe', 'spawned probe', 'retry delay']) {
    test('abort during ' + where + ' cleans up only its own attempt', async t => {
      const f = await fixture(t, where === 'existing probe'
        ? [() => new Promise(() => {})]
        : [null, where === 'retry delay' ? null : () => new Promise(() => {})]);
      if (where === 'before startup') f.controller.abort(new Error('secret'));
      const rejected = assert.rejects(f.start(), error => /startup interrupted/.test(error.message) && !error.message.includes('secret'));
      if (where !== 'before startup') {
        await f.waitForProbes(where === 'existing probe' ? 1 : 2);
        if (where === 'retry delay') await f.time.waitFor(100);
        f.controller.abort(new Error('secret'));
      }
      await rejected;
      assert.deepEqual(f.kills, where.startsWith('spawned') || where === 'retry delay' ? ['SIGTERM'] : []);
      assert.ok(f.calls.every(call => call.signal.aborted));
      assert.equal(f.time.timers.size, 0);
    });
  }

  for (const failure of ['throw', 'error', 'exit', 'version']) {
    test('spawn failure is bounded and sanitized: ' + failure, async t => {
      const f = await fixture(t, [null, failure === 'version' ? { ...initialized, userAgent: 'secret/1' } : null]);
      await assert.rejects(f.start({ spawnProcess: (...args) => {
        if (failure === 'throw') throw new Error('native secret');
        const child = f.spawnProcess(...args);
        if (failure === 'error') queueMicrotask(() => child.emit('error', new Error('native secret')));
        if (failure === 'exit') queueMicrotask(() => child.emit('exit', 2, null));
        return child;
      } }), error => /Cannot start|exited before readiness|exactly Codex/.test(error.message) && !error.message.includes('secret'));
      assert.deepEqual(f.kills, failure === 'error' || failure === 'version' ? ['SIGTERM'] : []);
      assert.equal(f.time.timers.size, 0);
    });
  }

  for (const kind of ['symlink', 'public file', 'directory']) {
    test('rejects unsafe native log without spawning: ' + kind, async t => {
      const f = await fixture(t, [null]);
      const logFile = join(f.home, 'codex-server.log');
      const target = join(f.home, 'target');
      await writeFile(target, 'untouched', { mode: 0o600 });
      if (kind === 'symlink') await symlink(target, logFile);
      if (kind === 'public file') { await writeFile(logFile, 'public'); await chmod(logFile, 0o644); }
      if (kind === 'directory') await mkdir(logFile, { mode: 0o700 });
      await assert.rejects(f.start());
      assert.deepEqual(f.spawns, []);
      assert.equal(await readFile(target, 'utf8'), 'untouched');
      assert.equal(f.time.timers.size, 0);
    });
  }

  test('refuses unsafe state directories and increased readiness limits', async t => {
    const f = await fixture(t);
    await chmod(f.home, 0o755);
    await assert.rejects(f.start(), /owner-only/);
    assert.deepEqual(f.calls, []);
    for (const options of [{ timeoutMs: 20001 }, { probeTimeoutMs: 1001 }, { pollMs: 0 }]) {
      await assert.rejects(f.start(options), /Invalid Codex server startup limit/);
    }
    assert.deepEqual(f.spawns, []);
    assert.equal(f.time.timers.size, 0);
  });
});

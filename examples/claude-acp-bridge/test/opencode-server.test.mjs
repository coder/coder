import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { EventEmitter, once } from 'node:events';
import { writeSync } from 'node:fs';
import { chmod, link, lstat, mkdir, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { promisify } from 'node:util';
import { connectOpenCodeServer, ensureOpenCodeServer, openCodeEnvironment, shutdownOpenCodeServer, verifyOpenCodeVersion, OPENCODE_VERSION } from '../src/opencode-server.mjs';

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
        const timer = [...timers].find(([, value]) => value.ms === ms);
        if (timer) return timer;
        await once(events, 'timer');
      }
    },
    async expire(ms) { const [id, timer] = await this.waitFor(ms); timers.delete(id); timer.callback(); },
  };
}

async function fixture(t) {
  const directory = await mkdtemp('/tmp/opencode/bridge-opencode-test-');
  t.after(() => rm(directory, { recursive: true, force: true }));
  const events = new EventEmitter();
  const child = new EventEmitter();
  const f = {
    directory, stateFile: join(directory, 'bridge.json'), descriptor: join(directory, 'opencode-server.json'),
    lock: join(directory, 'opencode-server.lock'), log: join(directory, 'opencode-server.log'),
    child, time: clock(), controller: new AbortController(), spawns: [], calls: [], kills: [], signals: [],
    processes: new Map(), releases: 0, unrefs: 0,
    async waitForCalls(count) { while (f.calls.length < count) await once(events, 'fetch'); },
  };
  child.pid = 12345;
  child.kill = signal => { f.kills.push(signal); f.processes.delete(child.pid); return true; };
  child.unref = () => { f.unrefs++; };
  f.fetchImpl = async (url, options) => {
    f.calls.push({ url, ...options }); events.emit('fetch');
    if (options.headers.authorization === 'Basic ' + Buffer.from('opencode:invalid-password').toString('base64')) {
      return new Response(null, { status: 401 });
    }
    return Response.json({ version: OPENCODE_VERSION, pid: child.pid, urls: ['http://127.0.0.1:43210'] });
  };
  f.options = {
    stateFile: f.stateFile, signal: f.controller.signal,
    env: { PATH: '/bin', HOME: '/real-home', OPENCODE_DB: '/real-credentials', OPENCODE_CONFIG: '/real-config',
      OPENCODE_TEST_HOME: '/real-test-home', OPENCODE_PASSWORD: 'inherited-secret', OPENCODE_CONFIG_CONTENT: '{"update":"auto"}' },
    ...f.time, fetchImpl: (...args) => f.fetchImpl(...args),
    getDomain: async () => 'boot/pid-namespace', getProcess: async pid => f.processes.get(pid) ?? null,
    signalProcess: (pid, name) => { f.signals.push([pid, name]); f.processes.delete(pid); },
    getVersion: async () => OPENCODE_VERSION,
    allocatePort: async () => ({ port: 43210, release: async () => { f.releases++; } }),
    spawnProcess: (...args) => {
      f.spawns.push(args);
      f.processes.set(child.pid, { startTime: '56789', marker: args[2].env.OPENCODE_BRIDGE_INSTANCE, ports: [43210] });
      return child;
    },
  };
  f.start = options => ensureOpenCodeServer({ ...f.options, ...options });
  f.connect = options => connectOpenCodeServer({ ...f.options, ...options });
  f.stop = options => shutdownOpenCodeServer({ ...f.options, ...options });
  f.seed = async (changes = {}) => {
    const d = { format: 1, version: OPENCODE_VERSION, domain: 'boot/pid-namespace', marker: 'a'.repeat(64),
      password: 'b'.repeat(64), status: 'ready', pid: child.pid, startTime: '56789', url: 'http://127.0.0.1:43210', ...changes };
    await writeFile(f.descriptor, JSON.stringify(d) + '\n', { mode: 0o600 });
    f.processes.set(d.pid, { startTime: d.startTime, marker: d.marker, ports: [43210] });
    return d;
  };
  return f;
}

async function unchanged(f) {
  const files = await readdir(f.directory);
  const descriptor = await readFile(f.descriptor);
  const before = await stat(f.descriptor);
  return async () => {
    assert.deepEqual(await readdir(f.directory), files);
    assert.deepEqual(await readFile(f.descriptor), descriptor);
    const after = await stat(f.descriptor);
    assert.equal(after.mtimeMs, before.mtimeMs);
    assert.equal(after.mode, before.mode);
    assert.deepEqual(f.spawns, []);
    assert.deepEqual(f.kills, []);
  };
}

describe('dedicated native OpenCode lifecycle', { concurrency: true, timeout: 5000 }, () => {
  test('exported native environment is shared, isolated, nonmutating, and does not create files', async t => {
    const f = await fixture(t);
    const original = { ...f.options.env, OPENCODE_BRIDGE_INSTANCE: 'other-server', OPENCODE_API_KEY: 'other-key' };
    const env = openCodeEnvironment(f.stateFile, original);
    assert.equal(env.HOME, join(f.directory, 'opencode-home'));
    assert.equal(env.XDG_CONFIG_HOME, join(env.HOME, 'config'));
    assert.equal(env.XDG_DATA_HOME, join(env.HOME, 'data'));
    assert.equal(env.OPENCODE_PASSWORD, undefined);
    assert.equal(env.OPENCODE_BRIDGE_INSTANCE, undefined);
    assert.equal(env.OPENCODE_DB, undefined);
    assert.equal(env.OPENCODE_CONFIG, undefined);
    assert.equal(env.OPENCODE_TEST_HOME, undefined);
    assert.equal(env.OPENCODE_API_KEY, undefined);
    assert.equal(env.OPENCODE_DISABLE_AUTOUPDATE, 'true');
    assert.equal(env.OPENCODE_CONFIG_CONTENT, '{"update":"disable"}');
    assert.equal(original.HOME, '/real-home');
    assert.equal(original.OPENCODE_PASSWORD, 'inherited-secret');
    assert.equal(env.PATH, original.PATH);
    assert.deepEqual(await readdir(f.directory), []);
    await f.start();
    const { OPENCODE_PASSWORD, OPENCODE_BRIDGE_INSTANCE, ...launched } = f.spawns[0][2].env;
    assert.deepEqual(launched, openCodeEnvironment(f.stateFile, f.options.env));
    assert.ok(OPENCODE_PASSWORD && OPENCODE_BRIDGE_INSTANCE);
  });

  test('authenticated exact-version reuse and read-only connect leave all files unchanged', async t => {
    const f = await fixture(t);
    const d = await f.seed();
    const check = await unchanged(f);
    assert.deepEqual(await f.start(), { url: d.url, password: d.password, started: false });
    assert.deepEqual(await f.connect(), { url: d.url, password: d.password });
    assert.equal(f.calls.length, 4);
    assert.ok(f.calls.every(call => call.url === d.url + '/api/info' && call.redirect === 'error'));
    assert.equal(f.calls[1].headers.authorization, 'Basic ' + Buffer.from('opencode:' + d.password).toString('base64'));
    assert.equal(f.time.timers.size, 0);
    f.controller.abort();
    await check();
  });

  test('only explicit startup launches detached serve with private persistent home and environment auth', async t => {
    const f = await fixture(t);
    await writeFile(f.log, 'previous\n', { mode: 0o600 });
    const originalMask = process.umask();
    const result = await f.start({ spawnProcess: (...args) => {
      assert.equal(process.umask(), 0o077);
      const child = f.options.spawnProcess(...args);
      writeSync(args[2].stdio[1], 'native output\n');
      return child;
    } });
    assert.equal(process.umask(), originalMask);
    const d = JSON.parse(await readFile(f.descriptor));
    assert.deepEqual(result, { url: d.url, password: d.password, started: true, pid: f.child.pid });
    assert.equal(d.status, 'ready');
    assert.equal(d.domain, 'boot/pid-namespace');
    assert.equal(d.startTime, '56789');
    assert.equal(f.releases, 1);
    assert.equal(f.unrefs, 1);
    const [[command, args, options]] = f.spawns;
    assert.equal(command, 'opencode');
    assert.deepEqual(args, ['serve', '--hostname', '127.0.0.1', '--port', '43210']);
    assert.equal(options.detached, true);
    assert.equal(options.signal, undefined);
    assert.deepEqual(options.stdio, ['ignore', options.stdio[1], options.stdio[1]]);
    assert.equal(options.env.OPENCODE_PASSWORD, d.password);
    assert.equal(options.env.OPENCODE_BRIDGE_INSTANCE, d.marker);
    assert.equal(options.env.OPENCODE_DB, undefined);
    assert.equal(options.env.OPENCODE_CONFIG, undefined);
    assert.equal(options.env.OPENCODE_TEST_HOME, undefined);
    assert.equal(options.env.OPENCODE_DISABLE_AUTOUPDATE, 'true');
    assert.equal(options.env.OPENCODE_CONFIG_CONTENT, '{"update":"disable"}');
    assert.equal(options.env.HOME, join(f.directory, 'opencode-home'));
    assert.equal(options.cwd, options.env.HOME);
    for (const key of ['XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR']) {
      assert.ok(options.env[key].startsWith(options.env.HOME + '/'));
      assert.equal((await stat(options.env[key])).mode & 0o777, 0o700);
    }
    assert.equal((await stat(f.descriptor)).mode & 0o777, 0o600);
    assert.equal((await stat(f.log)).mode & 0o777, 0o600);
    assert.equal(await readFile(f.log, 'utf8'), 'previous\nnative output\n');
    assert.ok(!JSON.stringify(args).includes(d.password));
    await assert.rejects(lstat(f.lock), { code: 'ENOENT' });
    f.controller.abort();
    assert.deepEqual(f.kills, []);
    assert.equal(f.time.timers.size, 0);
  });

  test('connect never creates directories, a descriptor, or a daemon', async t => {
    const f = await fixture(t);
    await assert.rejects(f.connect(), /not ready/);
    await assert.rejects(f.connect({ stateFile: join(f.directory, 'absent', 'bridge.json') }), { code: 'ENOENT' });
    assert.deepEqual(await readdir(f.directory), []);
    assert.deepEqual(f.spawns, []);
    assert.deepEqual(f.calls, []);
  });

  for (const changes of [
    { version: '2.0.29' }, { version: '2.0.28-beta' }, { password: 'short' }, { password: ['b'.repeat(64)] },
    { marker: '' }, { url: 'http://localhost:43210' }, { url: 'https://127.0.0.1:43210' },
    { url: 'http://127.0.0.1:43210/path' }, { url: 'http://127.0.0.1:65536' },
    { url: 'http://secret@127.0.0.1:43210' }, { url: ['http://127.0.0.1:43210'] }, { pid: -1 }, { startTime: 56789 },
  ]) {
    test('refuses invalid descriptor before sending credentials: ' + JSON.stringify(changes), async t => {
      const f = await fixture(t);
      await f.seed(changes);
      const check = await unchanged(f);
      await assert.rejects(f.start(), /descriptor/);
      await assert.rejects(f.connect(), /descriptor/);
      assert.deepEqual(f.calls, []);
      await check();
    });
  }

  for (const failure of ['wrong version', 'wrong pid', 'wrong url', 'unprotected', 'unauthorized', 'redirect', 'offline']) {
    test('live but unverifiable server is never replaced: ' + failure, async t => {
      const f = await fixture(t);
      await f.seed();
      const check = await unchanged(f);
      f.fetchImpl = async (_url, options) => {
        if (failure === 'offline') throw new Error('secret credential');
        if (failure === 'unprotected') return Response.json({});
        if (options.headers.authorization.endsWith(Buffer.from('opencode:invalid-password').toString('base64'))) return new Response(null, { status: 401 });
        if (failure === 'unauthorized') return new Response(null, { status: 401 });
        if (failure === 'redirect') return new Response(null, { status: 302, headers: { location: 'http://other-server' } });
        return Response.json({ version: failure === 'wrong version' ? '2.0.29' : OPENCODE_VERSION,
          pid: failure === 'wrong pid' ? 9 : f.child.pid, urls: failure === 'wrong url' ? [] : ['http://127.0.0.1:43210'] });
      };
      await assert.rejects(f.start(), error => /unhealthy/.test(error.message) && !error.message.includes('secret'));
      await assert.rejects(f.connect(), /could not be verified/);
      await check();
    });
  }

  for (const kind of ['domain', 'marker', 'unrecorded pid', 'incomplete startup']) {
    test('refuses ambiguous process ownership without killing saved PID: ' + kind, async t => {
      const f = await fixture(t);
      await f.seed(kind === 'domain' ? { domain: 'other-boot/pid-namespace' } :
        kind === 'unrecorded pid' ? { status: 'starting', pid: null, startTime: null } :
          kind === 'incomplete startup' ? { status: 'starting' } : {});
      if (kind === 'marker') f.processes.get(f.child.pid).marker = 'different';
      const check = await unchanged(f);
      await assert.rejects(f.start(), /domain changed|marker does not match|ownership is uncertain|startup is incomplete/);
      await assert.rejects(f.connect());
      await check();
    });
  }

  for (const kind of ['dead', 'PID reused']) {
    test('proven stale same-domain descriptor can restart only on explicit serve: ' + kind, async t => {
      const f = await fixture(t);
      const old = await f.seed();
      if (kind === 'dead') f.processes.clear();
      else f.processes.get(f.child.pid).startTime = '999';
      await assert.rejects(f.connect(), /no longer running/);
      const result = await f.start();
      assert.equal(result.started, true);
      assert.notEqual(result.password, old.password);
      assert.deepEqual(f.kills, []);
      assert.equal(f.spawns.length, 1);
    });
  }

  test('a stolen loopback port never receives the credential', async t => {
    const f = await fixture(t);
    await f.seed();
    f.processes.get(f.child.pid).ports = [];
    const check = await unchanged(f);
    await assert.rejects(f.connect(), /could not be verified/);
    await assert.rejects(f.start(), /unhealthy/);
    assert.deepEqual(f.calls, []);
    await check();
  });

  test('concurrent starters serialize to exactly one native process', async t => {
    const f = await fixture(t);
    let release, entered;
    const gate = new Promise(resolve => { release = resolve; });
    const checking = new Promise(resolve => { entered = resolve; });
    const original = f.fetchImpl;
    let calls = 0;
    f.fetchImpl = async (...args) => { if (++calls === 1) { entered(); await gate; } return original(...args); };
    const first = f.start();
    // Descriptor PID is persisted before the first health check starts.
    await checking;
    const second = f.start();
    await f.time.waitFor(100);
    release();
    assert.equal((await first).started, true);
    await f.time.expire(100);
    assert.equal((await second).started, false);
    assert.equal(f.spawns.length, 1);
    assert.deepEqual(f.kills, []);
  });

  test('an abandoned startup lock is not unlinked or reclaimed', async t => {
    const f = await fixture(t);
    await writeFile(f.lock, 'possibly owned by another PID domain', { mode: 0o600 });
    const rejected = assert.rejects(f.start(), /readiness timed out/);
    await f.time.waitFor(100);
    await f.time.expire(20000);
    await rejected;
    assert.equal(await readFile(f.lock, 'utf8'), 'possibly owned by another PID domain');
    assert.deepEqual(f.spawns, []);
  });

  test('healthy authenticated reuse is possible even with an abandoned startup lock', async t => {
    const f = await fixture(t);
    await f.seed();
    await writeFile(f.lock, 'do not touch', { mode: 0o600 });
    const check = await unchanged(f);
    assert.equal((await f.start()).started, false);
    await check();
  });

  for (const version of ['opencode v2.0.28\n', 'opencode v2.0.29\n', 'private-secret']) {
    test('CLI version probe has a disposable private home: ' + version.trim(), async t => {
      const f = await fixture(t);
      const mask = process.umask();
      let probeHome;
      const checking = verifyOpenCodeVersion({ env: f.options.env, runVersion: async (command, args, options) => {
        assert.equal(process.umask(), 0o077);
        assert.equal(command, 'opencode');
        assert.deepEqual(args, ['--version']);
        probeHome = options.env.HOME;
        assert.equal(options.cwd, probeHome);
        assert.ok(!probeHome.startsWith(f.directory));
        for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR']) {
          assert.ok(options.env[key].startsWith(probeHome));
          assert.equal((await stat(options.env[key])).mode & 0o777, 0o700);
        }
        assert.equal(options.env.OPENCODE_DB, undefined);
        assert.equal(options.env.OPENCODE_CONFIG, undefined);
        assert.equal(options.env.OPENCODE_PASSWORD, undefined);
        assert.equal(options.env.OPENCODE_DISABLE_AUTOUPDATE, 'true');
        assert.equal(options.timeout, 3000);
        return { stdout: version };
      } });
      if (version === 'opencode v2.0.28\n') await checking;
      else await assert.rejects(checking, error => /CLI must be exactly version 2\.0\.28/.test(error.message) && !error.message.includes('private-secret'));
      assert.equal(process.umask(), mask);
      await assert.rejects(stat(probeHome), { code: 'ENOENT' });
      assert.deepEqual(await readdir(f.directory), []);
    });
  }

  test('healthy server reuse also refuses an incompatible CLI without mutating native state', async t => {
    const f = await fixture(t);
    await f.seed();
    const check = await unchanged(f);
    await assert.rejects(f.start({ getVersion: async () => '2.0.29' }), /CLI must be exactly version 2.0.28/);
    assert.deepEqual(f.calls, []);
    await check();
  });

  test('wrong local CLI version refuses spawn before creating native descriptor', async t => {
    const f = await fixture(t);
    await assert.rejects(f.start({ getVersion: async () => '2.0.29' }), /exactly version 2.0.28/);
    assert.deepEqual(f.spawns, []);
    await assert.rejects(lstat(f.descriptor), { code: 'ENOENT' });
  });

  test('transient pre-exec identities retry without sending credentials or starting a second child', async t => {
    const f = await fixture(t);
    let reads = 0;
    const running = f.start({ getProcess: async pid => {
      if (++reads === 1) return null;
      if (reads === 2) return { startTime: '56789', marker: undefined, ports: [] };
      if (reads === 3) throw new Error('transient proc view');
      return f.processes.get(pid);
    } });
    for (let attempt = 0; attempt < 3; attempt++) {
      await f.time.waitFor(100);
      assert.deepEqual(f.calls, []);
      assert.equal(JSON.parse(await readFile(f.descriptor)).pid, null);
      await f.time.expire(100);
    }
    assert.equal((await running).started, true);
    assert.equal(f.spawns.length, 1);
    assert.equal(f.time.timers.size, 0);
  });

  for (const ending of ['abort', 'exit']) {
    test('pending pre-exec identity stops safely on child ' + ending, async t => {
      const f = await fixture(t);
      const rejected = assert.rejects(f.start({ getProcess: async () => null }), /startup interrupted|exited before/);
      await f.time.waitFor(100);
      if (ending === 'abort') f.controller.abort();
      else { f.child.emit('exit', 1); await f.time.expire(100); }
      await rejected;
      assert.deepEqual(f.calls, []);
      assert.deepEqual(f.kills, ending === 'abort' ? ['SIGTERM'] : []);
      assert.equal(f.time.timers.size, 0);
    });
  }

  test('health timeout aborts the connection, retries, then succeeds without a second spawn', async t => {
    const f = await fixture(t);
    const original = f.fetchImpl;
    let firstSignal;
    f.fetchImpl = (...args) => {
      if (!firstSignal) { firstSignal = args[1].signal; return new Promise(() => {}); }
      return original(...args);
    };
    const running = f.start();
    await f.time.expire(1000);
    assert.equal(firstSignal.aborted, true);
    await f.time.expire(100);
    assert.equal((await running).started, true);
    assert.equal(f.spawns.length, 1);
    assert.equal(f.time.timers.size, 0);
  });

  for (const where of ['before startup', 'spawned probe', 'retry delay']) {
    test('cancellation during ' + where + ' kills only its own attempted child', async t => {
      const f = await fixture(t);
      if (where === 'before startup') f.controller.abort(new Error('secret'));
      const original = f.fetchImpl;
      f.fetchImpl = (...args) => {
        original(...args);
        return where === 'retry delay' ? Promise.reject(new Error('not ready')) : new Promise(() => {});
      };
      const rejected = assert.rejects(f.start(), error => /startup interrupted/.test(error.message) && !error.message.includes('secret'));
      if (where !== 'before startup') {
        await f.waitForCalls(1);
        if (where === 'retry delay') await f.time.waitFor(100);
        f.controller.abort(new Error('secret'));
      }
      await rejected;
      assert.deepEqual(f.kills, where === 'before startup' ? [] : ['SIGTERM']);
      assert.equal(f.time.timers.size, 0);
      assert.ok(f.calls.every(call => call.signal.aborted));
    });
  }

  test('readiness deadline kills the new child but preserves its tracked record', async t => {
    const f = await fixture(t);
    f.fetchImpl = async (url, options) => {
      f.calls.push({ url, ...options });
      return new Promise(() => {});
    };
    const rejected = assert.rejects(f.start(), /readiness timed out/);
    await f.time.waitFor(1000);
    await f.time.expire(20000);
    await rejected;
    assert.deepEqual(f.kills, ['SIGTERM']);
    assert.equal(JSON.parse(await readFile(f.descriptor)).pid, f.child.pid);
    assert.equal(f.calls[0].signal.aborted, true);
    assert.equal(f.time.timers.size, 0);
  });

  for (const failure of ['throw', 'error', 'exit', 'unverifiable child']) {
    test('startup failure is bounded and sanitized: ' + failure, async t => {
      const f = await fixture(t);
      const rejected = assert.rejects(f.start({ spawnProcess: (...args) => {
        if (failure === 'throw') throw new Error('secret');
        const child = f.options.spawnProcess(...args);
        if (failure === 'error') queueMicrotask(() => child.emit('error', new Error('secret')));
        if (failure === 'exit') queueMicrotask(() => child.emit('exit', 1));
        if (failure === 'unverifiable child') f.processes.clear();
        return child;
      } }), error => /could not start|exited before|readiness timed out/.test(error.message) && !error.message.includes('secret'));
      if (failure === 'unverifiable child') {
        await f.time.waitFor(100);
        assert.deepEqual(f.calls, []);
        await f.time.expire(20000);
      }
      await rejected;
      assert.deepEqual(f.kills, ['error', 'unverifiable child'].includes(failure) ? ['SIGTERM'] : []);
      assert.equal(f.time.timers.size, 0);
    });
  }

  for (const name of ['descriptor', 'log']) {
    for (const kind of ['symlink', 'hardlink', 'public', 'directory']) {
      test('refuses unsafe ' + name + ': ' + kind, async t => {
        const f = await fixture(t);
        const target = join(f.directory, 'untouched');
        await writeFile(target, 'do not change', { mode: 0o600 });
        if (kind === 'symlink') await symlink(target, f[name]);
        if (kind === 'hardlink') await link(target, f[name]);
        if (kind === 'public') { await writeFile(f[name], 'public'); await chmod(f[name], 0o644); }
        if (kind === 'directory') await mkdir(f[name], { mode: 0o700 });
        await assert.rejects(f.start());
        if (name === 'descriptor') await assert.rejects(f.connect());
        assert.deepEqual(f.spawns, []);
        assert.equal(await readFile(target, 'utf8'), 'do not change');
      });
    }
  }

  test('rejects unsafe directories and excessive readiness limits', async t => {
    const f = await fixture(t);
    await chmod(f.directory, 0o755);
    await assert.rejects(f.start(), /owner-only/);
    await assert.rejects(f.connect(), /owner-only/);
    for (const options of [{ timeoutMs: 20001 }, { probeTimeoutMs: 1001 }, { pollMs: 0 }]) await assert.rejects(f.start(options), /limit is invalid/);
    await assert.rejects(f.connect({ probeTimeoutMs: 1001 }), /limit is invalid/);
    assert.deepEqual(f.spawns, []);
  });
});

describe('verified native OpenCode shutdown', { concurrency: true, timeout: 5000 }, () => {
  test('missing descriptor or state directory is a successful no-op', async t => {
    const f = await fixture(t);
    assert.deepEqual(await f.stop(), { stopped: false });
    assert.deepEqual(await f.stop({ stateFile: join(f.directory, 'missing/bridge.json') }), { stopped: false });
    assert.deepEqual(await readdir(f.directory), []);
    assert.deepEqual(f.signals, []);
    assert.deepEqual(f.calls, []);
    assert.equal(f.time.timers.size, 0);
  });

  test('ordinary stop/start preserves all native state across the next PID namespace', async t => {
    const f = await fixture(t);
    await f.seed();
    const home = openCodeEnvironment(f.stateFile, {}).HOME;
    await mkdir(home, { mode: 0o700 });
    const files = ['opencode.db', 'service.json', 'session.json', 'config.json'];
    for (const file of files) await writeFile(join(home, file), 'preserve-' + file, { mode: 0o600 });
    const result = await f.stop();
    assert.deepEqual(result, { stopped: true, pid: f.child.pid });
    assert.deepEqual(f.signals, [[f.child.pid, 'SIGTERM']]);
    assert.deepEqual(f.kills, []);
    await assert.rejects(lstat(f.descriptor), { code: 'ENOENT' });
    await assert.rejects(lstat(f.lock), { code: 'ENOENT' });
    const started = await f.start({ getDomain: async () => 'boot/new-container' });
    assert.equal(started.started, true);
    assert.equal(JSON.parse(await readFile(f.descriptor)).domain, 'boot/new-container');
    for (const file of files) assert.equal(await readFile(join(home, file), 'utf8'), 'preserve-' + file);
    assert.equal(f.spawns.length, 1);
    assert.equal(f.time.timers.size, 0);
  });

  for (const kind of ['foreign domain', 'dead', 'PID reuse', 'marker', 'socket', 'starting', 'unsafe descriptor', 'held lock']) {
    test('refuses unproved shutdown without signaling or removing state: ' + kind, async t => {
      const f = await fixture(t);
      await f.seed(kind === 'foreign domain' ? { domain: 'foreign' } : kind === 'starting' ? { status: 'starting' } : {});
      if (kind === 'dead') f.processes.clear();
      if (kind === 'PID reuse') f.processes.get(f.child.pid).startTime = '9';
      if (kind === 'marker') f.processes.get(f.child.pid).marker = 'foreign';
      if (kind === 'socket') f.processes.get(f.child.pid).ports = [];
      if (kind === 'unsafe descriptor') await chmod(f.descriptor, 0o644);
      if (kind === 'held lock') await writeFile(f.lock, 'another owner', { mode: 0o600 });
      const check = await unchanged(f);
      await assert.rejects(f.stop(), /shutdown could not be verified/);
      assert.deepEqual(f.signals, []);
      assert.deepEqual(f.calls, []);
      await check();
      assert.equal(f.time.timers.size, 0);
    });
  }

  test('authenticated health is required before signaling', async t => {
    const f = await fixture(t);
    await f.seed();
    const check = await unchanged(f);
    f.fetchImpl = async () => { throw new Error('private-secret'); };
    await assert.rejects(f.stop(), error => /could not be verified/.test(error.message) && !error.message.includes('private-secret'));
    assert.deepEqual(f.signals, []);
    await check();
  });

  test('identity is rechecked after authentication immediately before signaling', async t => {
    const f = await fixture(t);
    await f.seed();
    const check = await unchanged(f);
    let probes = 0;
    await assert.rejects(f.stop({ getProcess: async pid => {
      const current = f.processes.get(pid);
      return ++probes === 3 ? { ...current, startTime: 'reused' } : current;
    } }), /could not be verified/);
    assert.deepEqual(f.signals, []);
    await check();
  });

  for (const kind of ['changed content', 'replaced file', 'permission', 'disappeared']) {
    test('exit does not authorize deleting a changed descriptor: ' + kind, async t => {
      const f = await fixture(t);
      await f.seed();
      let expected;
      await assert.rejects(f.stop({ signalProcess: async (pid, name) => {
        f.options.signalProcess(pid, name);
        if (kind === 'changed content') await writeFile(f.descriptor, 'new owner', { mode: 0o600 });
        if (kind === 'replaced file') {
          const bytes = await readFile(f.descriptor);
          await rm(f.descriptor);
          await writeFile(f.descriptor, bytes, { mode: 0o600 });
        }
        if (kind === 'permission') await chmod(f.descriptor, 0o644);
        if (kind === 'disappeared') await rm(f.descriptor);
        expected = await readFile(f.descriptor).catch(() => null);
      } }), /could not be verified/);
      assert.deepEqual(await readFile(f.descriptor).catch(() => null), expected);
      assert.deepEqual(f.signals, [[f.child.pid, 'SIGTERM']]);
    });
  }

  test('a PID reused after confirmed shutdown is never signaled again', async t => {
    const f = await fixture(t);
    await f.seed();
    assert.equal((await f.stop({ signalProcess: (pid, name) => {
      f.signals.push([pid, name]);
      f.processes.set(pid, { startTime: 'different-process', marker: 'other', ports: [] });
    } })).stopped, true);
    assert.deepEqual(f.signals, [[f.child.pid, 'SIGTERM']]);
    assert.equal(f.processes.get(f.child.pid).marker, 'other');
  });

  test('successful signaling does not remove the descriptor until exit is observed', async t => {
    const f = await fixture(t);
    await f.seed();
    const running = f.stop({ signalProcess: (pid, name) => f.signals.push([pid, name]) });
    await f.time.waitFor(100);
    assert.ok(await lstat(f.descriptor));
    f.processes.clear();
    await f.time.expire(100);
    assert.equal((await running).stopped, true);
    await assert.rejects(lstat(f.descriptor), { code: 'ENOENT' });
  });

  for (const kind of ['unreadable process', 'changed marker', 'changed domain']) {
    test('ambiguous exit preserves the descriptor: ' + kind, async t => {
      const f = await fixture(t);
      await f.seed();
      const check = await unchanged(f);
      let signaled = false;
      await assert.rejects(f.stop({
        signalProcess: (pid, name) => { f.signals.push([pid, name]); signaled = true; },
        getDomain: async () => signaled && kind === 'changed domain' ? 'foreign' : 'boot/pid-namespace',
        getProcess: async pid => {
          if (!signaled) return f.processes.get(pid);
          if (kind === 'unreadable process') throw new Error('private-secret');
          if (kind === 'changed marker') return { ...f.processes.get(pid), marker: 'other' };
          return null;
        },
      }), /could not be verified/);
      assert.deepEqual(f.signals, [[f.child.pid, 'SIGTERM']]);
      await check();
    });
  }

  test('graceful shutdown waits for exit and never escalates on timeout', async t => {
    const f = await fixture(t);
    await f.seed();
    const check = await unchanged(f);
    const rejected = assert.rejects(f.stop({ signalProcess: (pid, name) => f.signals.push([pid, name]) }), /shutdown timed out/);
    await f.time.waitFor(100);
    await f.time.expire(20000);
    await rejected;
    assert.deepEqual(f.signals, [[f.child.pid, 'SIGTERM']]);
    await check();
    assert.equal(f.time.timers.size, 0);
  });

  for (const where of ['before', 'health', 'exit']) {
    test('cancellation during ' + where + ' preserves the descriptor', async t => {
      const f = await fixture(t);
      await f.seed();
      const check = await unchanged(f);
      if (where === 'before') f.controller.abort(new Error('private-secret'));
      if (where === 'health') f.fetchImpl = async () => new Promise(() => {});
      const rejected = assert.rejects(f.stop({ signalProcess: (pid, name) => f.signals.push([pid, name]) }), error =>
        /shutdown interrupted/.test(error.message) && !error.message.includes('private-secret'));
      if (where === 'health') await f.time.waitFor(1000);
      if (where === 'exit') await f.time.waitFor(100);
      f.controller.abort(new Error('private-secret'));
      await rejected;
      assert.deepEqual(f.signals, where === 'exit' ? [[f.child.pid, 'SIGTERM']] : []);
      await check();
      assert.equal(f.time.timers.size, 0);
    });
  }

  test('signal refusal preserves the descriptor and all shutdown limits are bounded', async t => {
    const f = await fixture(t);
    await f.seed();
    const check = await unchanged(f);
    await assert.rejects(f.stop({ signalProcess: () => { throw new Error('private-secret'); } }), /could not be verified/);
    for (const options of [{ timeoutMs: 20001 }, { probeTimeoutMs: 1001 }, { pollMs: 0 }]) await assert.rejects(f.stop(options), /limit is invalid/);
    await check();
  });
});

test('isolated installed V2 smoke: auth, exact version, native home, reuse, and detached liveness', {
  skip: process.env.OPENCODE_SERVER_SMOKE !== '1', timeout: 30000,
}, async t => {
  const directory = await mkdtemp('/tmp/opencode/bridge-opencode-smoke-');
  let child, nativeEnv;
  t.after(async () => {
    if (child && child.exitCode === null && child.signalCode === null) {
      const exited = once(child, 'exit');
      const timer = setTimeout(() => child.kill('SIGKILL'), 3000);
      child.kill('SIGTERM');
      await exited;
      clearTimeout(timer);
    }
    await rm(directory, { recursive: true, force: true });
  });
  const stateFile = join(directory, 'bridge.json');
  const controller = new AbortController();
  const endpoint = await ensureOpenCodeServer({
    stateFile, signal: controller.signal,
    env: { PATH: process.env.PATH, HOME: directory, OPENCODE_DISABLE_MODELS_FETCH: 'true' },
    spawnProcess: (command, args, options) => {
      nativeEnv = options.env;
      child = spawn('/bin/sh', ['-c', 'exec "$@"', 'opencode-wrapper', command, ...args], options);
      return child;
    },
  });
  assert.equal(endpoint.started, true);
  assert.equal(endpoint.pid, child.pid);
  const descriptor = await readFile(join(directory, 'opencode-server.json'));
  controller.abort();
  assert.deepEqual(await connectOpenCodeServer({ stateFile }), { url: endpoint.url, password: endpoint.password });
  assert.deepEqual(await ensureOpenCodeServer({ stateFile, spawnProcess: () => assert.fail('must reuse') }), {
    url: endpoint.url, password: endpoint.password, started: false,
  });
  assert.deepEqual(await readFile(join(directory, 'opencode-server.json')), descriptor);
  const denied = await fetch(endpoint.url + '/api/info');
  assert.equal(denied.status, 401);
  await denied.body.cancel();
  const info = await fetch(endpoint.url + '/api/info', {
    headers: { authorization: 'Basic ' + Buffer.from('opencode:' + endpoint.password).toString('base64') },
  }).then(response => response.json());
  assert.equal(info.version, OPENCODE_VERSION);
  assert.equal(info.pid, child.pid);
  const { stdout } = await promisify(execFile)('opencode', ['api', '--server', endpoint.url, 'get', '/api/info'], {
    cwd: nativeEnv.HOME, env: { ...nativeEnv, OPENCODE_BRIDGE_INSTANCE: '' }, timeout: 5000, maxBuffer: 8192,
  });
  assert.equal(JSON.parse(stdout).pid, child.pid, 'native CLI authenticates to this explicit server, not the managed service');
  assert.ok(info.paths.tmp.startsWith(join(directory, 'opencode-home')));
  const command = await readFile('/proc/' + child.pid + '/cmdline', 'utf8');
  assert.ok(!command.includes(endpoint.password));
  assert.ok(!command.includes('--service'));
  assert.equal((await stat(join(directory, 'opencode-server.log'))).mode & 0o777, 0o600);
  assert.equal((await stat(join(directory, 'opencode-home/data/opencode/log/opencode.log'))).mode & 0o777, 0o600);
  assert.equal((await stat(join(directory, 'opencode-server.json'))).mode & 0o777, 0o600);
  for (const log of ['opencode-server.log', 'opencode-home/data/opencode/log/opencode.log']) {
    const text = await readFile(join(directory, log), 'utf8');
    assert.ok(!text.includes(endpoint.password));
    assert.ok(!text.includes(Buffer.from('opencode:' + endpoint.password).toString('base64')));
  }
  await assert.rejects(lstat(join(directory, 'opencode-home/state/opencode/service.json')), { code: 'ENOENT' });

  // Creating an empty native session persists state without a model or inference.
  const created = await fetch(endpoint.url + '/api/session', {
    method: 'POST', signal: AbortSignal.timeout(3000),
    headers: { authorization: 'Basic ' + Buffer.from('opencode:' + endpoint.password).toString('base64'), 'content-type': 'application/json' },
    body: JSON.stringify({ title: 'Lifecycle persistence smoke', location: { directory: nativeEnv.HOME } }),
  });
  assert.equal(created.status, 200);
  const { data: session } = await created.json();
  const exited = once(child, 'exit');
  assert.deepEqual(await shutdownOpenCodeServer({ stateFile }), { stopped: true, pid: child.pid });
  await exited;
  await assert.rejects(lstat(join(directory, 'opencode-server.json')), { code: 'ENOENT' });
  assert.ok((await stat(join(directory, 'opencode-home/data/opencode/opencode.db'))).isFile());

  const restarted = await ensureOpenCodeServer({
    stateFile, env: { PATH: process.env.PATH, HOME: directory, OPENCODE_DISABLE_MODELS_FETCH: 'true' },
    spawnProcess: (...args) => { child = spawn(...args); return child; },
  });
  assert.equal(restarted.started, true);
  assert.notEqual(restarted.password, endpoint.password);
  const loaded = await fetch(restarted.url + '/api/session/' + session.id, {
    signal: AbortSignal.timeout(3000),
    headers: { authorization: 'Basic ' + Buffer.from('opencode:' + restarted.password).toString('base64') },
  });
  assert.equal(loaded.status, 200);
  assert.equal((await loaded.json()).data.id, session.id);
  const restartedExit = once(child, 'exit');
  assert.equal((await shutdownOpenCodeServer({ stateFile })).stopped, true);
  await restartedExit;
});

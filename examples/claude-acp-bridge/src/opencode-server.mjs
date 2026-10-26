import { execFile, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdtemp, open, readFile, readdir, readlink, rename, rm, unlink } from 'node:fs/promises';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, resolve } from 'node:path';
import { promisify } from 'node:util';
import { privateDirectory } from './service.mjs';

/** Native API and CLI version supported by this bridge. Never automatically upgrade it. */
export const OPENCODE_VERSION = '2.0.28';

const markerKey = 'OPENCODE_BRIDGE_INSTANCE';
const nofollow = constants.O_NOFOLLOW | constants.O_NONBLOCK;
const fail = message => new Error('Native OpenCode ' + message + '; no work was dispatched.');
const paths = stateFile => {
  const directory = dirname(resolve(stateFile));
  return { directory, descriptor: resolve(directory, 'opencode-server.json'),
    lock: resolve(directory, 'opencode-server.lock'), log: resolve(directory, 'opencode-server.log'),
    home: resolve(directory, 'opencode-home') };
};

async function privateFile(file, flags) {
  const handle = await open(file, flags | nofollow, 0o600);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.nlink !== 1 || info.uid !== process.getuid() || (info.mode & 0o077) !== 0) {
      throw fail('requires owner-only regular files (chmod 600)');
    }
    return handle;
  } catch (error) { await handle.close(); throw error; }
}

async function readDescriptor(file, { snapshot = false } = {}) {
  let handle;
  try {
    handle = await privateFile(file, constants.O_RDONLY);
    const info = await handle.stat();
    if (info.size > 8192) throw fail('descriptor is invalid');
    const text = await handle.readFile('utf8');
    const d = JSON.parse(text);
    const port = /^http:\/\/127\.0\.0\.1:([1-9][0-9]{0,4})$/.exec(d.url)?.[1];
    if (d.format !== 1 || d.version !== OPENCODE_VERSION || typeof d.url !== 'string' || !port || Number(port) > 65535 ||
        typeof d.password !== 'string' || !/^[a-f0-9]{64}$/.test(d.password) ||
        typeof d.marker !== 'string' || !/^[a-f0-9]{64}$/.test(d.marker) ||
        typeof d.domain !== 'string' || !d.domain ||
        !['starting', 'ready'].includes(d.status) ||
        !(d.pid === null && d.startTime === null && d.status === 'starting') &&
        !(Number.isSafeInteger(d.pid) && d.pid > 0 && typeof d.startTime === 'string' && /^[0-9]+$/.test(d.startTime))) {
      throw fail('descriptor is invalid or has an incompatible version');
    }
    return snapshot ? { value: d, text, info } : d;
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    // Never include file contents, credentials, or native error messages.
    throw fail('descriptor must be a valid private regular file for version ' + OPENCODE_VERSION);
  } finally { await handle?.close(); }
}

async function writeDescriptor(file, value) {
  const temporary = file + '.' + randomBytes(12).toString('hex');
  const handle = await privateFile(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL);
  try {
    await handle.writeFile(JSON.stringify(value) + '\n');
    await handle.sync();
    await rename(temporary, file);
  } finally { await handle.close(); await unlink(temporary).catch(() => {}); }
}

// A PID is only meaningful within the original boot and PID namespace. Never
// signal from a saved PID alone, or assume an unreachable foreign process died.
async function processDomain() {
  const [boot, namespace] = await Promise.all([
    readFile('/proc/sys/kernel/random/boot_id', 'utf8'), readlink('/proc/self/ns/pid'),
  ]);
  return boot.trim() + '/' + namespace;
}

async function processIdentity(pid) {
  try {
    const text = await readFile('/proc/' + pid + '/stat', 'utf8');
    const fields = text.slice(text.lastIndexOf(')') + 2).split(' ');
    if (fields[0] === 'Z') return null;
    const environment = await readFile('/proc/' + pid + '/environ', 'utf8');
    const marker = environment.split('\0').find(value => value.startsWith(markerKey + '='))?.slice(markerKey.length + 1);
    const descriptors = await readdir('/proc/' + pid + '/fd');
    const sockets = new Set(await Promise.all(descriptors.map(async fd => {
      try { return await readlink('/proc/' + pid + '/fd/' + fd); }
      catch (error) { if (error.code === 'ENOENT') return null; throw error; }
    })));
    const tcp = await readFile('/proc/' + pid + '/net/tcp', 'utf8');
    const ports = tcp.trim().split('\n').slice(1).flatMap(line => {
      const row = line.trim().split(/\s+/);
      const [address, port] = row[1].split(':');
      return address === '0100007F' && row[3] === '0A' && sockets.has('socket:[' + row[9] + ']')
        ? [parseInt(port, 16)] : [];
    });
    return { startTime: fields[19], marker, ports };
  } catch (error) {
    if (error.code === 'ENOENT' || error.code === 'ESRCH') {
      // A missing environment/socket view alone is not proof of process exit.
      // Confirm disappearance (or a zombie) through the process's stat record.
      try {
        const text = await readFile('/proc/' + pid + '/stat', 'utf8');
        if (text.slice(text.lastIndexOf(')') + 2).split(' ')[0] === 'Z') return null;
      } catch (again) { if (again.code === 'ENOENT' || again.code === 'ESRCH') return null; }
    }
    throw fail('process identity cannot be verified');
  }
}

async function alive(d, domain, identity) {
  if (d.domain !== domain) throw fail('PID domain changed; inspect the private descriptor before explicitly recovering it');
  if (d.pid === null) throw fail('startup ownership is uncertain; inspect the private descriptor before explicitly recovering it');
  const current = await identity(d.pid);
  if (!current || current.startTime !== d.startTime) return false;
  if (current.marker !== d.marker) throw fail('process marker does not match; nothing was restarted');
  return true;
}

function interrupted(operation, signal) {
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const abort = () => { signal.removeEventListener('abort', abort); reject(signal.reason); };
    signal.addEventListener('abort', abort, { once: true });
    Promise.resolve().then(() => { signal.throwIfAborted(); return operation(); }).then(resolve, reject)
      .finally(() => signal.removeEventListener('abort', abort));
  });
}

function ownsListener(d, current) {
  return current?.startTime === d.startTime && current.marker === d.marker &&
    current.ports.includes(Number(new URL(d.url).port));
}

async function verify(d, { fetchImpl, getProcess, signal, setTimer, clearTimer, probeTimeoutMs }) {
  signal.throwIfAborted();
  const attempt = new AbortController();
  const cancel = () => attempt.abort(signal.reason);
  signal.addEventListener('abort', cancel, { once: true });
  const timer = setTimer(() => attempt.abort(fail('health check timed out')), probeTimeoutMs);
  try {
    return await interrupted(async () => {
      // Verify kernel socket ownership before sending any credential. A process
      // that steals the preallocated port must not impersonate our child.
      const current = await getProcess(d.pid);
      if (!ownsListener(d, current)) throw fail('listener ownership could not be verified');
      // Refuse an unprotected listener as well as a wrong password. Redirects
      // must never forward the private credential to another server.
      const denied = await fetchImpl(d.url + '/api/info', {
        redirect: 'error', signal: attempt.signal,
        headers: { authorization: 'Basic ' + Buffer.from('opencode:invalid-password').toString('base64') },
      });
      await denied.body?.cancel();
      if (denied.status !== 401) throw fail('server does not enforce password authentication');
      const response = await fetchImpl(d.url + '/api/info', {
        redirect: 'error', signal: attempt.signal,
        headers: { authorization: 'Basic ' + Buffer.from('opencode:' + d.password).toString('base64') },
      });
      if (response.status !== 200) { await response.body?.cancel(); throw fail('server rejected authentication'); }
      const info = await response.json();
      if (info.version !== OPENCODE_VERSION || info.pid !== d.pid || !Array.isArray(info.urls) || !info.urls.includes(d.url)) {
        throw fail('server identity or exact version ' + OPENCODE_VERSION + ' did not match');
      }
    }, attempt.signal);
  } finally {
    clearTimer(timer);
    signal.removeEventListener('abort', cancel);
    attempt.abort();
  }
}

async function reservePort() {
  const listener = createServer();
  await new Promise((resolve, reject) => {
    listener.once('error', reject);
    listener.listen({ host: '127.0.0.1', port: 0, exclusive: true }, resolve);
  });
  return { port: listener.address().port, release: () => new Promise((resolve, reject) => listener.close(error => error ? reject(error) : resolve())) };
}

function privateChild(start) {
  // Even --version initializes the native logger. Restore the mask synchronously
  // before other JS callbacks can run; both native processes inherit 0077.
  const mask = process.umask(0o077);
  try { return start(); } finally { process.umask(mask); }
}

async function versionOf({ env, signal, run = promisify(execFile) }) {
  const home = await mkdtemp(resolve(tmpdir(), 'opencode-version-'));
  try {
    // Even an incompatible CLI must never see the persistent native database,
    // configuration, or project while its version is being checked.
    const isolated = nativeEnvironment(home, env);
    for (const key of ['XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR']) {
      await privateDirectory(isolated[key]);
    }
    const { stdout } = await privateChild(() => run('opencode', ['--version'], {
      cwd: home, env: isolated, signal, timeout: 3000, maxBuffer: 1024,
    }));
    return /^opencode v([^\s]+)$/.exec(stdout.trim())?.[1];
  } finally { await rm(home, { recursive: true, force: true }); }
}

/** Require the pinned CLI before native operations; probe only in a disposable private home. */
export async function verifyOpenCodeVersion({ env = process.env, signal, getVersion = versionOf, runVersion } = {}) {
  const bounded = AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(3000)]);
  let version;
  try { version = await interrupted(() => getVersion({ env, signal: bounded, run: runVersion }), bounded); }
  catch { throw fail('CLI version check failed'); }
  if (version !== OPENCODE_VERSION) throw fail('CLI must be exactly version ' + OPENCODE_VERSION);
}

function nativeEnvironment(home, env) {
  // Do not inherit a harness's database, service, inline config, or test-home
  // overrides. This native home persists with the bridge's workspace volume.
  const result = Object.fromEntries(Object.entries(env).filter(([key]) => !key.startsWith('OPENCODE_')));
  return { ...result, HOME: home, XDG_CONFIG_HOME: home + '/config', XDG_DATA_HOME: home + '/data',
    XDG_STATE_HOME: home + '/state', XDG_CACHE_HOME: home + '/cache', TMPDIR: home + '/tmp',
    OPENCODE_DISABLE_AUTOUPDATE: 'true', OPENCODE_CONFIG_CONTENT: '{"update":"disable"}',
    ...(env.OPENCODE_DISABLE_MODELS_FETCH === 'true' ? { OPENCODE_DISABLE_MODELS_FETCH: 'true' } : {}) };
}

/**
 * Environment for this workspace's dedicated native home, shared by preflight,
 * server, and TUI. Does not create files or inherit another server's overrides.
 * Append the verified endpoint's OPENCODE_PASSWORD only when connecting to it.
 */
export function openCodeEnvironment(stateFile, env = process.env) {
  return nativeEnvironment(paths(stateFile).home, env);
}

/**
 * Read-only rendezvous for the service and native TUI. Never starts a server,
 * repairs state, or uses managed-service discovery. Returns only {url,password}.
 */
export async function connectOpenCodeServer({
  stateFile, signal = new AbortController().signal, fetchImpl = fetch,
  getDomain = processDomain, getProcess = processIdentity,
  setTimer = setTimeout, clearTimer = clearTimeout, probeTimeoutMs = 1000,
} = {}) {
  if (!Number.isSafeInteger(probeTimeoutMs) || probeTimeoutMs < 1 || probeTimeoutMs > 1000) throw fail('health check limit is invalid');
  const p = paths(stateFile);
  const directory = await lstat(p.directory);
  if (!directory.isDirectory() || directory.uid !== process.getuid() || (directory.mode & 0o077) !== 0) {
    throw fail('state directory must be owner-only (chmod 700)');
  }
  const d = await readDescriptor(p.descriptor);
  if (!d || d.status !== 'ready') throw fail('server is not ready; explicitly start the bridge with serve --opencode');
  if (!await alive(d, await getDomain(), getProcess)) throw fail('server is no longer running');
  try { await verify(d, { fetchImpl, getProcess, signal, setTimer, clearTimer, probeTimeoutMs }); }
  catch { throw fail('authenticated health or exact version ' + OPENCODE_VERSION + ' could not be verified'); }
  return { url: d.url, password: d.password };
}

/**
 * Gracefully stop only the authenticated native server proved to own this
 * descriptor in the current PID domain. Send one SIGTERM, never escalate, and
 * remove only the unchanged bridge-owned descriptor after confirmed exit.
 * Native home, database, logs, and registrations are never repaired or removed.
 * Missing state is a successful no-op; ambiguous shutdown preserves the record.
 */
export async function shutdownOpenCodeServer({
  stateFile, signal, fetchImpl = fetch, getDomain = processDomain, getProcess = processIdentity,
  signalProcess = (pid, name) => process.kill(pid, name),
  setTimer = setTimeout, clearTimer = clearTimeout,
  timeoutMs = 20000, probeTimeoutMs = 1000, pollMs = 100,
} = {}) {
  const failure = message => new Error('Native OpenCode shutdown ' + message + '. Saved state was not repaired.');
  for (const [value, maximum] of [[timeoutMs, 20000], [probeTimeoutMs, 1000], [pollMs, 1000]]) {
    if (!Number.isSafeInteger(value) || value < 1 || value > maximum) throw failure('limit is invalid');
  }
  const p = paths(stateFile);
  const stopping = new AbortController();
  const cancel = () => stopping.abort(failure('interrupted'));
  signal?.addEventListener('abort', cancel, { once: true });
  if (signal?.aborted) cancel();
  const deadline = setTimer(() => stopping.abort(failure('timed out')), timeoutMs);
  const run = operation => interrupted(operation, stopping.signal);
  let lock;
  try {
    stopping.signal.throwIfAborted();
    let directory;
    try { directory = await run(() => lstat(p.directory)); }
    catch (error) { if (error.code === 'ENOENT') return { stopped: false }; throw error; }
    if (!directory.isDirectory() || directory.uid !== process.getuid() || (directory.mode & 0o077) !== 0) throw failure('state directory is not private');
    const record = await run(() => readDescriptor(p.descriptor, { snapshot: true }));
    if (!record) return { stopped: false };
    const d = record.value;
    if (d.status !== 'ready' || d.domain !== await run(getDomain) ||
        !ownsListener(d, await run(() => getProcess(d.pid)))) throw failure('ownership could not be verified');
    // Serialize against explicit starts. Never reclaim another invocation's lock.
    lock = await privateFile(p.lock, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL);
    const unchanged = async () => {
      const next = await run(() => readDescriptor(p.descriptor, { snapshot: true }));
      if (!next || next.text !== record.text ||
          ['dev', 'ino', 'mtimeMs', 'ctimeMs'].some(key => next.info[key] !== record.info[key])) throw failure('descriptor changed');
    };
    await unchanged();
    await run(() => verify(d, { fetchImpl, getProcess, signal: stopping.signal, setTimer, clearTimer, probeTimeoutMs }));
    await unchanged();
    if (d.domain !== await run(getDomain) || !ownsListener(d, await run(() => getProcess(d.pid)))) throw failure('ownership changed before signaling');
    try { await run(() => signalProcess(d.pid, 'SIGTERM')); }
    catch (error) { if (error.code !== 'ESRCH') throw error; }
    while (true) {
      const current = await run(() => getProcess(d.pid));
      if (!current || current.startTime !== d.startTime) break;
      if (current.marker !== d.marker) throw failure('process marker changed');
      let timer;
      try { await run(() => new Promise(resolve => { timer = setTimer(resolve, pollMs); })); }
      finally { clearTimer(timer); }
    }
    if (d.domain !== await run(getDomain)) throw failure('PID domain changed');
    await unchanged();
    stopping.signal.throwIfAborted();
    await unlink(p.descriptor);
    return { stopped: true, pid: d.pid };
  } catch {
    if (stopping.signal.aborted) throw stopping.signal.reason;
    // Do not expose descriptors, credentials, or native diagnostics.
    throw failure('could not be verified');
  } finally {
    clearTimer(deadline);
    signal?.removeEventListener('abort', cancel);
    if (lock) {
      const owned = await lock.stat();
      await lock.close();
      const current = await lstat(p.lock).catch(() => null);
      if (current?.dev === owned.dev && current.ino === owned.ino) await unlink(p.lock);
    }
  }
}

/**
 * Explicit serve --opencode startup only. One detached unmanaged server owns a
 * persistent private home beside the bridge descriptor. An exclusive startup
 * lock prevents competing launches. Abandoned locks and cross-domain records
 * deliberately require inspection, not guessed PID cleanup. A crash between
 * spawn and recording the PID leaves an ambiguous record that also fails closed.
 *
 * Requires Linux /proc visibility for process identity and socket ownership.
 * Injectable process, HTTP, port, version and timer seams are for lifecycle tests.
 * Only this invocation's child may be killed on failure; success outlives callers.
 */
export async function ensureOpenCodeServer({
  stateFile, signal, spawnProcess = spawn, fetchImpl = fetch, env = process.env,
  getDomain = processDomain, getProcess = processIdentity, getVersion = versionOf,
  allocatePort = reservePort, setTimer = setTimeout, clearTimer = clearTimeout,
  timeoutMs = 20000, probeTimeoutMs = 1000, pollMs = 100,
} = {}) {
  for (const [value, maximum] of [[timeoutMs, 20000], [probeTimeoutMs, 1000], [pollMs, 1000]]) {
    if (!Number.isSafeInteger(value) || value < 1 || value > maximum) throw fail('startup limit is invalid');
  }
  const p = paths(stateFile);
  const startup = new AbortController();
  const cancel = () => startup.abort(fail('startup interrupted'));
  signal?.addEventListener('abort', cancel, { once: true });
  if (signal?.aborted) cancel();
  const deadline = setTimer(() => startup.abort(fail('readiness timed out; inspect the private native log')), timeoutMs);
  const checks = { fetchImpl, getProcess, signal: startup.signal, setTimer, clearTimer, probeTimeoutMs };
  let lock, child, reservation, d;
  let exited = false, childError = false, ready = false;
  const pause = async () => {
    let timer;
    try { await interrupted(() => new Promise(resolve => { timer = setTimer(resolve, pollMs); }), startup.signal); }
    finally { clearTimer(timer); }
  };
  try {
    startup.signal.throwIfAborted();
    await privateDirectory(p.directory);
    await verifyOpenCodeVersion({ env, signal: startup.signal, getVersion });
    const domain = await getDomain();
    // Healthy reuse leaves persistent state unchanged, even with a stale lock.
    d = await readDescriptor(p.descriptor);
    if (d?.status === 'ready' && await alive(d, domain, getProcess)) {
      try { await verify(d, checks); } catch { throw fail('existing server is unhealthy; nothing was restarted'); }
      return { url: d.url, password: d.password, started: false };
    }
    while (!lock) {
      startup.signal.throwIfAborted();
      try { lock = await privateFile(p.lock, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL); }
      catch (error) { if (error.code !== 'EEXIST') throw error; await pause(); }
    }
    d = await readDescriptor(p.descriptor);
    if (d && await alive(d, domain, getProcess)) {
      if (d.status !== 'ready') throw fail('previous startup is incomplete; nothing was restarted');
      try { await verify(d, checks); } catch { throw fail('existing server is unhealthy; nothing was restarted'); }
      return { url: d.url, password: d.password, started: false };
    }
    const marker = randomBytes(32).toString('hex');
    const nativeEnv = { ...openCodeEnvironment(stateFile, env), [markerKey]: marker };
    for (const directory of [p.home, ...['config', 'data', 'state', 'cache', 'tmp'].map(name => p.home + '/' + name)]) {
      await privateDirectory(directory);
    }
    reservation = await allocatePort();
    d = { format: 1, version: OPENCODE_VERSION, domain, marker, status: 'starting', pid: null, startTime: null,
      url: 'http://127.0.0.1:' + reservation.port, password: randomBytes(32).toString('hex') };
    const log = await privateFile(p.log, constants.O_WRONLY | constants.O_APPEND | constants.O_CREAT);
    try {
      await writeDescriptor(p.descriptor, d);
      startup.signal.throwIfAborted();
      // Release only immediately before spawn; readiness verifies kernel socket
      // ownership before sending credentials across the port-allocation gap.
      await reservation.release(); reservation = undefined;
      startup.signal.throwIfAborted();
      try {
        child = privateChild(() => spawnProcess('opencode', ['serve', '--hostname', '127.0.0.1', '--port', String(new URL(d.url).port)], {
          cwd: p.home, env: { ...nativeEnv, OPENCODE_PASSWORD: d.password },
          detached: true, stdio: ['ignore', log.fd, log.fd],
        }));
        child.once('exit', () => { exited = true; });
        child.on('error', () => { childError = true; });
      } catch { throw fail('could not start; inspect the private native log'); }
    } finally { await log.close(); }
    // Exec wrappers and /proc can expose a transient pre-exec identity. Wait
    // within the startup budget for this live child's marker and start time;
    // no authenticated request is allowed until that identity is recorded.
    let identity;
    while (!identity) {
      startup.signal.throwIfAborted();
      if (childError || exited || child.exitCode != null || child.signalCode != null || !child.pid) {
        throw fail('exited before process ownership was verified; inspect the private native log');
      }
      try {
        const current = await interrupted(() => getProcess(child.pid), startup.signal);
        if (current?.marker === marker && typeof current.startTime === 'string' && /^[0-9]+$/.test(current.startTime)) identity = current;
      } catch { startup.signal.throwIfAborted(); }
      if (!identity) await pause();
    }
    d.pid = child.pid; d.startTime = identity.startTime;
    await writeDescriptor(p.descriptor, d);
    while (true) {
      startup.signal.throwIfAborted();
      if (childError || exited) throw fail('exited before readiness; inspect the private native log');
      let healthy = false;
      try { await verify(d, checks); healthy = true; } catch { startup.signal.throwIfAborted(); }
      if (healthy) {
        if (childError || exited || !await alive(d, domain, getProcess)) throw fail('exited before readiness');
        startup.signal.throwIfAborted();
        await writeDescriptor(p.descriptor, { ...d, status: 'ready' });
        startup.signal.throwIfAborted();
        ready = true;
        return { url: d.url, password: d.password, started: true, pid: child.pid };
      }
      await pause();
    }
  } finally {
    clearTimer(deadline);
    signal?.removeEventListener('abort', cancel);
    if (child) {
      if (!ready && !exited) child.kill('SIGTERM');
      child.unref();
    }
    await reservation?.release();
    if (lock) { await lock.close(); await unlink(p.lock); }
  }
}

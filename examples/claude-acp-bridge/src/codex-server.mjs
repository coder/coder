import { spawn } from 'node:child_process';
import { constants } from 'node:fs';
import { open } from 'node:fs/promises';
import { dirname, isAbsolute, resolve } from 'node:path';
import { CODEX_VERSION, createCodexRPC } from './codex-rpc.mjs';
import { privateDirectory } from './service.mjs';

/** The native rendezvous is shared by the broker and TUI for this state directory. */
export function codexSocketPath(stateFile) {
  return resolve(dirname(resolve(stateFile)), 'codex.sock');
}

function socketRPC(socketPath, { signal, ...options } = {}, spawnProcess = spawn) {
  return createCodexRPC({
    ...options,
    spawnProcess: (command, args, options) => spawnProcess(command, [...args, '--sock', socketPath], {
      ...options, ...(signal ? { signal } : {}),
    }),
  });
}

/** Preserve the service callback API while connecting only to this explicit socket. */
export function createCodexRPCFactory(stateFile, { spawnProcess = spawn } = {}) {
  const socketPath = codexSocketPath(stateFile);
  return options => socketRPC(socketPath, options, spawnProcess);
}

async function probeCodexServer({ socketPath, signal, timeoutMs }) {
  const rpc = await socketRPC(socketPath, { signal, timeoutMs });
  try {
    const result = await rpc.request('initialize', {
      clientInfo: { name: 'coder_codex_bridge_probe', title: 'Coder Codex Bridge', version: '0.1.0' },
      capabilities: { experimentalApi: true, requestAttestation: false },
    });
    rpc.notify('initialized');
    return result;
  } finally { await rpc.close(); }
}

function verifyVersion(result) {
  const version = typeof result?.userAgent === 'string'
    ? result.userAgent.match(/^[^\s/]+\/([^\s]+)(?:\s|$)/)?.[1] : undefined;
  if (version !== CODEX_VERSION || result.platformFamily !== 'unix' ||
      typeof result.codexHome !== 'string' || !isAbsolute(result.codexHome)) {
    throw new Error('Native Codex app-server must run exactly Codex ' + CODEX_VERSION + ' on Unix; no work was dispatched.');
  }
}

function interrupted(operation, signal) {
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const onAbort = () => { signal.removeEventListener('abort', onAbort); reject(signal.reason); };
    signal.addEventListener('abort', onAbort, { once: true });
    Promise.resolve().then(() => { signal.throwIfAborted(); return operation(); }).then(
      result => { signal.removeEventListener('abort', onAbort); resolve(result); },
      error => { signal.removeEventListener('abort', onAbort); reject(error); },
    );
  });
}

/**
 * Explicit serve --codex startup only. Reuse a verified native server or launch
 * an unmanaged, detached app-server with no threads loaded. Native bind/locking
 * arbitrates competing starts; never inspect or unlink its rendezvous symlink,
 * socket, lock, or PID records. A successful engine outlives the broker and TUI.
 *
 * probe returns the native initialize response, or null/rejects if unavailable.
 * It receives {socketPath, signal, timeoutMs} and must release its connection on
 * abort. Spawn and timer seams allow deterministic lifecycle tests. The complete
 * readiness budget is at most 20 seconds.
 */
export async function ensureCodexServer({
  stateFile, signal, probe = probeCodexServer, spawnProcess = spawn,
  setTimer = setTimeout, clearTimer = clearTimeout,
  timeoutMs = 20000, probeTimeoutMs = 1000, pollMs = 100,
} = {}) {
  for (const [value, maximum] of [[timeoutMs, 20000], [probeTimeoutMs, 1000], [pollMs, 1000]]) {
    if (!Number.isSafeInteger(value) || value < 1 || value > maximum) throw new Error('Invalid Codex server startup limit');
  }
  const socketPath = codexSocketPath(stateFile);
  const logFile = resolve(dirname(socketPath), 'codex-server.log');
  const startup = new AbortController();
  const onAbort = () => startup.abort(new Error('Codex app-server startup interrupted; no work was dispatched.'));
  signal?.addEventListener('abort', onAbort, { once: true });
  if (signal?.aborted) onAbort();
  const deadline = setTimer(() => startup.abort(new Error('Codex app-server readiness timed out; inspect the private native log. No work was dispatched.')), timeoutMs);
  let child;
  let childExited = false;
  let childError = false;
  let ready = false;
  const onExit = () => { childExited = true; };
  const onError = () => { childError = true; };

  async function available() {
    startup.signal.throwIfAborted();
    const attempt = new AbortController();
    const cancel = () => attempt.abort(startup.signal.reason);
    startup.signal.addEventListener('abort', cancel, { once: true });
    const timer = setTimer(() => attempt.abort(new Error('Codex readiness probe timed out')), probeTimeoutMs);
    let result;
    try {
      result = await interrupted(() => probe({ socketPath, signal: attempt.signal, timeoutMs: probeTimeoutMs }), attempt.signal);
    } catch {
      startup.signal.throwIfAborted();
      return false;
    } finally {
      clearTimer(timer);
      startup.signal.removeEventListener('abort', cancel);
      attempt.abort();
    }
    startup.signal.throwIfAborted();
    if (result === null) return false;
    verifyVersion(result);
    return true;
  }

  async function pause() {
    let timer;
    try {
      await interrupted(() => new Promise(resolve => { timer = setTimer(resolve, pollMs); }), startup.signal);
    } finally { clearTimer(timer); }
  }

  try {
    startup.signal.throwIfAborted();
    await privateDirectory(dirname(socketPath));
    if (await available()) return { socketPath, logFile, started: false };
    startup.signal.throwIfAborted();
    const log = await open(logFile, constants.O_WRONLY | constants.O_APPEND | constants.O_CREAT | constants.O_NOFOLLOW | constants.O_NONBLOCK, 0o600);
    try {
      const info = await log.stat();
      if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) !== 0) {
        throw new Error('Codex log must be an owner-only regular file (chmod 600).');
      }
      startup.signal.throwIfAborted();
      try {
        child = spawnProcess('codex', ['app-server', '--listen', 'unix://' + socketPath], {
          detached: true, stdio: ['ignore', log.fd, log.fd],
        });
        child.once('exit', onExit);
        child.on('error', onError);
      } catch { throw new Error('Cannot start native Codex app-server; inspect the private native log. No work was dispatched.'); }
    } finally { await log.close(); }
    while (true) {
      startup.signal.throwIfAborted();
      if (childError) throw new Error('Cannot start native Codex app-server; inspect the private native log. No work was dispatched.');
      // A competing native bind may have won even when our child has exited.
      if (await available()) {
        ready = true;
        return { socketPath, logFile, started: !childExited, ...(childExited ? {} : { pid: child.pid }) };
      }
      if (childExited) throw new Error('Native Codex app-server exited before readiness; inspect the private native log. No work was dispatched.');
      await pause();
    }
  } finally {
    clearTimer(deadline);
    signal?.removeEventListener('abort', onAbort);
    if (child) {
      if (!ready && !childExited) child.kill('SIGTERM');
      child.off('exit', onExit);
      // Retain the safe error listener for late spawn/kill errors after unref.
      child.unref();
    }
  }
}

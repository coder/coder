import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, open, rename, stat, unlink } from 'node:fs/promises';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import { Readable, Writable } from 'node:stream';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { client, methods, ndJsonStream, PROTOCOL_VERSION } from '@agentclientprotocol/sdk';

const script = fileURLToPath(import.meta.url);
const defaultCLI = join(dirname(script), 'cli.mjs');
const startupTimeoutMs = 90000;
const statuses = new Set(['starting', 'created', 'blocked', 'prompting', 'completed', 'failed', 'unknown']);
const stopReasons = new Set(['end_turn', 'max_tokens', 'max_turn_requests', 'refusal', 'cancelled']);
const errorCodes = new Set(['setup_failed', 'unsupported_protocol', 'unsupported_mode', 'result_unknown', 'worker_start_failed', 'native_input_required']);
const sessionID = /^[^\x00-\x1f\x7f]{1,1024}$/u;
const uuid = /^[a-f0-9-]{36}$/;
const fields = new Set(['version', 'jobId', 'inputHash', 'harness', 'status', 'sessionId', 'createdAt', 'updatedAt', 'dispatchAttemptedAt', 'completedAt', 'stopReason', 'errorCode', 'workerPid', 'workerStartedAt']);
const harnesses = new Set(['claude', 'codex', 'opencode']);
const normalizedHarness = harness => harness === undefined ? 'claude' : harness;
const now = () => new Date().toISOString();

function localError(code) {
  return Object.assign(new Error('Job cannot proceed: ' + code), { jobErrorCode: code });
}

function absolute(path, label) {
  if (typeof path !== 'string' || !isAbsolute(path)) throw new Error(label + ' must be an absolute path');
  return resolve(path);
}

// Ancestors may be shared (for example /tmp), but must not redirect private I/O
// through symlinks or directories writable by another user without the sticky bit.
async function privateParent(file, create = false) {
  const directory = dirname(file);
  let current = '/';
  for (const part of directory.split('/').filter(Boolean)) {
    current = join(current, part);
    let info;
    try { info = await lstat(current); }
    catch (error) {
      if (error.code !== 'ENOENT' || !create) throw new Error('Private job directory is missing');
      try { await mkdir(current, { mode: 0o700 }); }
      catch (error) { if (error.code !== 'EEXIST') throw new Error('Cannot create private job directory'); }
      info = await lstat(current);
    }
    if (info.isSymbolicLink()) throw new Error('Private job paths cannot contain symlinks');
    if (!info.isDirectory() || ![0, process.getuid()].includes(info.uid) ||
        ((info.mode & 0o022) && !(info.mode & 0o1000))) {
      throw new Error('Unsafe private job directory ownership or permissions');
    }
  }
  const info = await lstat(directory);
  if (info.uid !== process.getuid() || (info.mode & 0o077)) {
    throw new Error('Job files require an owner-only private directory (chmod 700)');
  }
}

function checkFile(info) {
  if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) || info.nlink !== 1) {
    throw new Error('Job files must be private owner-only regular files without hardlinks (chmod 600)');
  }
}

async function privateRead(file, limit) {
  let handle;
  try { handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK); }
  catch (error) {
    if (error.code === 'ENOENT') throw error;
    throw new Error('Cannot open private job file; symlinks are not supported');
  }
  try {
    const info = await handle.stat();
    checkFile(info);
    if (info.size > limit) throw new Error('Private job file exceeds size limit');
    const bytes = Buffer.alloc(limit + 1);
    let size = 0;
    while (size < bytes.length) {
      const { bytesRead } = await handle.read(bytes, size, bytes.length - size, size);
      if (!bytesRead) break;
      size += bytesRead;
    }
    if (size > limit) throw new Error('Private job file exceeds size limit');
    let text;
    try { text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes.subarray(0, size)); }
    catch { throw new Error('Private job file is not valid UTF-8'); }
    return { text, info };
  } finally { await handle.close(); }
}

function parseRecord(text) {
  let record;
  try { record = JSON.parse(text); } catch { throw new Error('Job file is incomplete or invalid; inspect it manually. Nothing was replayed.'); }
  if (!record || record.version !== 1 || !uuid.test(record.jobId) ||
      !/^[a-f0-9]{64}$/.test(record.inputHash) || !statuses.has(record.status) ||
      !harnesses.has(normalizedHarness(record.harness)) ||
      !record.createdAt || !record.updatedAt || Object.keys(record).some(key => !fields.has(key)) ||
      ['createdAt', 'updatedAt', 'dispatchAttemptedAt', 'completedAt', 'workerStartedAt'].some(key =>
        record[key] !== undefined && (typeof record[key] !== 'string' || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/.test(record[key]))) ||
      (record.sessionId !== undefined && (typeof record.sessionId !== 'string' || !sessionID.test(record.sessionId))) ||
      (record.workerPid !== undefined && (!Number.isSafeInteger(record.workerPid) || record.workerPid < 1)) ||
      (record.errorCode !== undefined && !errorCodes.has(record.errorCode)) ||
      (record.stopReason !== undefined && !stopReasons.has(record.stopReason)) ||
      (['created', 'blocked', 'prompting', 'completed'].includes(record.status) && !record.sessionId) ||
      (record.status === 'blocked' && (record.errorCode !== 'native_input_required' || record.dispatchAttemptedAt !== undefined)) ||
      (record.status === 'completed' && (!record.stopReason || !record.completedAt))) {
    throw new Error('Invalid job file; inspect it manually. Nothing was replayed.');
  }
  return record;
}

async function readGuard(file) {
  await privateParent(file);
  const { text, info } = await privateRead(file, 16384);
  return { record: parseRecord(text), info, file };
}

async function syncDirectory(file) {
  const directory = await open(dirname(file), constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW);
  try { await directory.sync(); } finally { await directory.close(); }
}

async function checkGuard(guard) {
  const current = await readGuard(guard.file);
  if (current.info.ino !== guard.info.ino || current.info.dev !== guard.info.dev ||
      JSON.stringify(current.record) !== JSON.stringify(guard.record)) {
    throw new Error('Job file was replaced or changed; refusing to overwrite it');
  }
}

async function update(guard, change) {
  const record = { ...guard.record, ...change, updatedAt: now() };
  await checkGuard(guard);
  const temporary = join(dirname(guard.file), '.job-' + randomUUID());
  const handle = await open(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
  try {
    await handle.writeFile(JSON.stringify(record) + '\n');
    await handle.sync();
    await checkGuard(guard);
    await rename(temporary, guard.file);
    guard.record = record;
    guard.info = await handle.stat();
    await syncDirectory(guard.file);
  } finally {
    await handle.close();
    await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw error; });
  }
  return { ...record };
}

async function inputs(options) {
  const harness = normalizedHarness(options.harness);
  if (!harnesses.has(harness)) throw new Error('--harness must be claude, codex or opencode');
  const cwd = absolute(options.cwd, '--cwd');
  const promptFile = absolute(options.promptFile, '--prompt-file');
  const jobFile = absolute(options.jobFile, '--job-file');
  if (promptFile === jobFile) throw new Error('Prompt and job files must be different');
  if (!(await stat(cwd)).isDirectory()) throw new Error('--cwd must name a directory');
  await privateParent(promptFile);
  await privateParent(jobFile, true);
  const { text: prompt } = await privateRead(promptFile, 1024 * 1024);
  if (!prompt.length) throw new Error('Prompt file must not be empty');
  // Keep the fingerprint compatible with old Claude guards. Harness identity is
  // checked separately so a selector change can never resubmit the same input.
  const inputHash = createHash('sha256').update(JSON.stringify([cwd, prompt])).digest('hex');
  return { cwd, promptFile, jobFile, prompt, inputHash, harness };
}

async function prepare(options) {
  const input = await inputs(options);
  let handle;
  try {
    handle = await open(input.jobFile, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW | constants.O_NONBLOCK, 0o600);
  } catch (error) {
    if (error.code !== 'EEXIST') throw new Error('Cannot exclusively create private job file');
    const guard = await readGuard(input.jobFile);
    if (guard.record.inputHash !== input.inputHash || normalizedHarness(guard.record.harness) !== input.harness) {
      throw new Error('Job file belongs to different input or a different harness; nothing was replayed');
    }
    return { input, guard, existing: true };
  }
  // Even an incomplete guard is retained after a crash. A retry must never infer
  // that an absent session ID proves session/new did not reach the agent.
  const timestamp = now();
  const record = { version: 1, jobId: randomUUID(), inputHash: input.inputHash, harness: input.harness, status: 'starting', createdAt: timestamp, updatedAt: timestamp };
  try {
    await handle.writeFile(JSON.stringify(record) + '\n');
    await handle.sync();
    await syncDirectory(input.jobFile);
    return { input, guard: { record, info: await handle.stat(), file: input.jobFile }, existing: false };
  } finally { await handle.close(); }
}

function waitFor(promise, signal) {
  if (signal.aborted) {
    void Promise.resolve(promise).catch(() => {});
    return Promise.reject(localError('result_unknown'));
  }
  return new Promise((resolve, reject) => {
    const abort = () => { cleanup(); reject(localError('result_unknown')); };
    const cleanup = () => signal.removeEventListener('abort', abort);
    signal.addEventListener('abort', abort, { once: true });
    Promise.resolve(promise).then(value => { cleanup(); resolve(value); }, error => { cleanup(); reject(error); });
  });
}

function jobStream(stream) {
  const pending = new Set();
  return {
    writable: new WritableStream({
      async write(message) {
        if ('method' in message && 'id' in message) pending.add(message.id);
        const writer = stream.writable.getWriter();
        try { await writer.write(message); } finally { writer.releaseLock(); }
      },
    }),
    readable: stream.readable.pipeThrough(new TransformStream({
      transform(message, controller) {
        if (!message || Array.isArray(message) || message.jsonrpc !== '2.0') throw localError('result_unknown');
        // SDK diagnostics include raw invalid notifications and unsolicited reply
        // IDs. Jobs need neither: drop all notifications and reject unknown replies
        // before SDK dispatch so remote secrets cannot become diagnostic output.
        if ('method' in message) {
          if (!('id' in message)) return;
          if (typeof message.method !== 'string') throw localError('result_unknown');
        } else if (!pending.delete(message.id)) {
          throw localError('result_unknown');
        }
        controller.enqueue(message);
      },
    })),
  };
}

function stdioConnection(app, { cli = defaultCLI, stateFile }) {
  // Only the ACP shim is owned here. Its stdin closing does not stop the native
  // background session. Remote stderr is discarded, never mixed into job output.
  const child = spawn(process.execPath, [cli, 'acp', '--state', stateFile], { stdio: ['pipe', 'pipe', 'ignore'] });
  const connection = app.connect(jobStream(ndJsonStream(Writable.toWeb(child.stdin), Readable.toWeb(child.stdout))));
  child.on('error', () => connection.close());
  child.stdin.on('error', () => connection.close());
  child.stdout.on('error', () => connection.close());
  const exited = new Promise(resolve => child.once('close', resolve));
  return {
    connection,
    async dispose() {
      connection.close();
      await connection.closed;
      child.stdin.destroy();
      child.stdout.destroy();
      // A broken shim must not keep the job process alive. Never signal a native
      // PID or process group, only this owned stdio adapter after closing it.
      const timer = setTimeout(() => child.kill('SIGKILL'), 2000);
      try { await exited; } finally { clearTimeout(timer); }
    },
  };
}

async function dispatch(options, input, guard) {
  const controller = new AbortController();
  const signal = options.signal ? AbortSignal.any([controller.signal, options.signal]) : controller.signal;
  const setTimer = options.setTimer ?? setTimeout;
  const clearTimer = options.clearTimer ?? clearTimeout;
  let timer = setTimer(() => controller.abort(), options.startupTimeoutMs ?? startupTimeoutMs);
  let transport;
  let knownSessionId;
  let mayHaveCreated = false;
  try {
    signal.throwIfAborted();
    const app = client({ name: 'coder-acp-job' })
      .onNotification(methods.client.session.update, () => {})
      .onRequest(methods.client.session.requestPermission, () => ({ outcome: { outcome: 'cancelled' } }));
    transport = (options.createConnection ?? stdioConnection)(app, options);
    const rpc = transport.connection.agent;
    const request = (method, params) => {
      signal.throwIfAborted();
      // Do not use ACP cancellationSignal: on timeout/disconnect native work must
      // continue. Only the local observer is closed, with no cancellation RPC.
      return waitFor(rpc.request(method, params), signal);
    };
    const initialized = await request(methods.agent.initialize, { protocolVersion: PROTOCOL_VERSION, clientCapabilities: {} });
    // Plain text and session/new are baseline ACP capabilities. No extension,
    // filesystem, terminal, embedded context or authentication flow is assumed.
    if (initialized.protocolVersion !== PROTOCOL_VERSION || !initialized.agentCapabilities) throw localError('unsupported_protocol');
    mayHaveCreated = true;
    let session = await request(methods.agent.session.new, { cwd: input.cwd, mcpServers: [],
      ...(input.harness !== 'claude' ? { _meta: { 'coder-acp': { harness: input.harness } } } : {}),
    });
    if (typeof session.sessionId !== 'string' || !sessionID.test(session.sessionId)) throw localError('result_unknown');
    knownSessionId = session.sessionId;
    await update(guard, { sessionId: knownSessionId, status: 'created' });
    const startedPending = session._meta?.['claude-acp-bridge']?.registrationPending === true;
    // Native setup dialogs precede ACP registration. Refresh only this known
    // session, never create a replacement or submit through a native dialog.
    while (session._meta?.['claude-acp-bridge']?.registrationPending === true) {
      if (session._meta['claude-acp-bridge'].blockedReason || !initialized.agentCapabilities.loadSession) {
        return await update(guard, { status: 'blocked', errorCode: 'native_input_required' });
      }
      await delay(500, undefined, { signal });
      const loaded = await request(methods.agent.session.load, { sessionId: knownSessionId, cwd: input.cwd, mcpServers: [] });
      session = { ...loaded, sessionId: knownSessionId };
    }
    if (startedPending && session.modes?.currentModeId !== 'automation') {
      return await update(guard, { status: 'blocked', errorCode: 'native_input_required' });
    }
    if (session.modes && session.modes.currentModeId !== 'automation') {
      if (!session.modes.availableModes.some(mode => mode.id === 'automation')) throw localError('unsupported_mode');
      await request(methods.agent.session.setMode, { sessionId: session.sessionId, modeId: 'automation' });
    }
    await update(guard, { status: 'prompting' });
    // Register failure handling immediately, including a fast remote rejection.
    const completion = request(methods.agent.session.prompt, { sessionId: session.sessionId, prompt: [{ type: 'text', text: input.prompt }] })
      .then(value => ({ value }), () => ({ error: true }));
    const ready = await update(guard, { dispatchAttemptedAt: now() });
    // ACP has no prompt-acceptance response. This acknowledges only that the
    // request was attempted, not delivery, acceptance, execution or completion.
    await waitFor(options.onReady?.(ready), signal);
    clearTimer(timer);
    timer = options.turnTimeoutMs ? setTimer(() => controller.abort(), options.turnTimeoutMs) : undefined;
    const result = await completion;
    if (result.error || !stopReasons.has(result.value?.stopReason)) throw localError('result_unknown');
    return await update(guard, { status: 'completed', stopReason: result.value.stopReason, completedAt: now() });
  } catch (error) {
    const knownFailure = ['unsupported_protocol', 'unsupported_mode'].includes(error.jobErrorCode);
    const status = knownFailure || !mayHaveCreated ? 'failed' : 'unknown';
    return await update(guard, { status, ...(knownSessionId ? { sessionId: knownSessionId } : {}),
      errorCode: knownFailure ? error.jobErrorCode : mayHaveCreated ? 'result_unknown' : 'setup_failed' });
  } finally {
    clearTimer(timer);
    await transport?.dispose();
  }
}

/**
 * Dispatch one literal text prompt over ACP and wait for its final result. Existing
 * matching guards are returned without connecting or replaying, even after a crash.
 * The synchronous createConnection(app, options) injection returns an SDK connection
 * plus async dispose(); tests use the same SDK framing without native inference.
 * onReady receives a durable dispatch-attempt record, never an acceptance claim.
 * The optional harness is claude (default), codex or opencode; legacy guards are Claude.
 */
export async function runJob(options) {
  const { input, guard, existing } = await prepare(options);
  if (existing) return { ...guard.record };
  return dispatch(options, input, guard);
}

async function stopOwnedWorker(child) {
  if (child.exitCode !== null || child.signalCode !== null || !child.pid) return;
  const exited = new Promise(resolve => child.once('exit', resolve));
  child.kill('SIGTERM');
  const timer = setTimeout(() => child.kill('SIGKILL'), 2000);
  try { await exited; } finally { clearTimeout(timer); }
}

/**
 * Start a dedicated OS-session worker with private logs and no inherited stdio.
 * Return after bounded launch/session/dispatch readiness, not the full turn. The
 * worker stays connected to observe completion; parent disconnect never cancels it.
 */
export async function startDetachedJob(options) {
  const { input, guard, existing } = await prepare(options);
  if (existing) return { ...guard.record };
  let log;
  let child;
  const setTimer = options.setTimer ?? setTimeout;
  const clearTimer = options.clearTimer ?? clearTimeout;
  try {
    // Never append to a preexisting unrelated file, even if its mode is private.
    log = await open(input.jobFile + '.log', constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
    await log.sync();
    await syncDirectory(input.jobFile);
    child = spawn(process.execPath, [script], { detached: true, stdio: ['ignore', log.fd, log.fd, 'ipc'] });
    await new Promise((resolve, reject) => {
      let settled = false;
      const timer = setTimer(() => finish(new Error('Job worker readiness timed out; work may have started. Inspect the job file.')), options.startupTimeoutMs ?? startupTimeoutMs);
      function finish(error) {
        if (settled) return;
        settled = true;
        clearTimer(timer);
        child.off('exit', onExit);
        child.off('message', onMessage);
        if (error) reject(error); else resolve();
      }
      function onExit() { finish(new Error('Job worker exited before dispatch readiness; inspect the job file')); }
      function onMessage(message) {
        if (message?.type === 'job-worker-online') {
          child.send({ type: 'job-start', jobId: guard.record.jobId, cwd: input.cwd, promptFile: input.promptFile, jobFile: input.jobFile,
            harness: input.harness, stateFile: options.stateFile, cli: options.cli ?? defaultCLI, startupTimeoutMs: options.startupTimeoutMs ?? startupTimeoutMs }, error => {
            if (error) finish(new Error('Cannot hand off job to worker'));
          });
        }
        if (message?.type === 'job-ready' && message.jobId === guard.record.jobId) finish();
      }
      child.once('exit', onExit);
      child.on('message', onMessage);
      // Keep an error listener installed after readiness, including IPC teardown.
      child.on('error', () => finish(new Error('Cannot start job worker')));
    });
    const current = await readGuard(input.jobFile);
    if (current.record.jobId !== guard.record.jobId || current.record.inputHash !== guard.record.inputHash ||
        normalizedHarness(current.record.harness) !== input.harness || current.record.createdAt !== guard.record.createdAt) throw new Error('Job file identity changed');
    return current.record;
  } catch {
    // Cleanup belongs only to this failed start, never an existing guard's PID.
    if (child) await stopOwnedWorker(child);
    const current = await readGuard(input.jobFile);
    if (current.record.jobId !== guard.record.jobId || current.record.inputHash !== guard.record.inputHash ||
        normalizedHarness(current.record.harness) !== input.harness || current.record.createdAt !== guard.record.createdAt) throw new Error('Job file identity changed; refusing to overwrite it');
    if (['starting', 'created', 'prompting'].includes(current.record.status)) {
      await update(current, { status: child ? 'unknown' : 'failed', errorCode: 'worker_start_failed' });
    }
    return current.record;
  } finally {
    if (child?.connected) child.disconnect();
    child?.unref();
    await log?.close();
  }
}

async function worker() {
  if (!process.send) throw new Error('Job worker requires a parent IPC channel');
  const controller = new AbortController();
  let ready = false;
  process.once('SIGTERM', () => controller.abort());
  process.once('SIGINT', () => controller.abort());
  process.once('disconnect', () => { if (!ready) controller.abort(); });
  const message = new Promise(resolve => process.once('message', resolve));
  process.send({ type: 'job-worker-online' }, error => { if (error) controller.abort(); });
  const options = await waitFor(message, controller.signal);
  if (options?.type !== 'job-start') throw new Error('Invalid job worker handoff');
  const input = await inputs(options);
  const guard = await readGuard(input.jobFile);
  if (guard.record.jobId !== options.jobId || guard.record.inputHash !== input.inputHash ||
      normalizedHarness(guard.record.harness) !== input.harness || guard.record.status !== 'starting' || guard.record.workerStartedAt) throw new Error('Job worker handoff does not own this guard');
  await update(guard, { workerStartedAt: now(), workerPid: process.pid });
  const record = await dispatch({ ...options, signal: controller.signal, onReady: async () => {
    ready = true;
    if (!process.connected) throw new Error('Parent disconnected before readiness');
    await new Promise((resolve, reject) => process.send({ type: 'job-ready', jobId: guard.record.jobId }, error => error ? reject(new Error('Cannot deliver readiness')) : resolve()));
  } }, input, guard);
  if (process.connected) process.disconnect();
  if (['blocked', 'failed', 'unknown'].includes(record.status)) process.exitCode = 1;
}

if (process.argv[1] && resolve(process.argv[1]) === script) {
  worker().catch(() => {
    // Never print raw ACP errors, prompts, tokens or transcripts to worker logs.
    console.error('Job worker failed; inspect the private job record. Nothing was replayed.');
    if (process.connected) process.disconnect();
    process.exitCode = 1;
  });
}

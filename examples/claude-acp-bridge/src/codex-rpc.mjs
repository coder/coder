import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { Duplex } from 'node:stream';
import WebSocket from 'ws';

/** The only native protocol version this POC implements. */
export const CODEX_VERSION = '0.164.0';
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const id = value => (typeof value === 'string' && value.length > 0 && value.length <= 200) || Number.isSafeInteger(value);
const METHODS = new Set(['initialize', 'thread/start', 'thread/read', 'thread/resume',
  'thread/queue/list', 'thread/goal/get', 'turn/start', 'turn/interrupt']);

/** Errors deliberately exclude native diagnostics, response bodies and credentials. */
export class CodexRPCError extends Error {
  constructor(message, { rejected = false, reason } = {}) {
    super(message);
    this.status = rejected ? 409 : 502;
    this.rejected = rejected;
    this.reason = reason;
  }
}

/**
 * Bounded WebSocket transport to an EXISTING daemon through its byte proxy,
 * never an app-server engine. Inherited CODEX_HOME selects the native Unix socket.
 * No TCP connection or knowledge of native socket paths is required.
 *
 * rpcFactory in createCodexService receives {onNotification, onServerRequest,
 * onDisconnect}. It resolves after the handshake with {request(method, params): Promise<object>,
 * notify(method): void, close(): void|Promise<void>}. Callbacks receive the complete
 * JSON envelope synchronously, in wire order. No method is retried. A definite
 * native rejection uses CodexRPCError with rejected=true; other failures have an
 * unknown outcome. close must release only this connection, not cancel a turn.
 * Server requests are delivered but NEVER answered, including during shutdown.
 *
 * The remaining options are transport test seams. Each limit can only be reduced.
 */
export async function createCodexRPC({
  onNotification = () => {}, onServerRequest = () => {}, onDisconnect = () => {},
  spawnProcess = spawn, setTimer = setTimeout, clearTimer = clearTimeout,
  timeoutMs = 10000, maxFrameBytes = 1024 * 1024, maxPending = 32,
  maxOutputBytes = 1024 * 1024,
} = {}) {
  for (const [value, maximum] of [[timeoutMs, 30000], [maxFrameBytes, 1024 * 1024],
    [maxPending, 32], [maxOutputBytes, 1024 * 1024]]) {
    if (!Number.isSafeInteger(value) || value < 1 || value > maximum) throw new Error('Invalid Codex RPC limit');
  }
  const pending = new Map();
  let child;
  let socket;
  let transport;
  let failure;
  let handshakeTimer;
  let resolveOpen;
  let rejectOpen;
  const opened = new Promise((resolve, reject) => { resolveOpen = resolve; rejectOpen = reject; });

  function fail(message) {
    if (failure) return;
    failure = new CodexRPCError(message);
    clearTimer(handshakeTimer);
    rejectOpen(failure);
    for (const entry of pending.values()) { clearTimer(entry.timer); entry.reject(failure); }
    pending.clear();
    socket?.terminate();
    transport?.destroy();
    child?.stdin?.destroy();
    child?.stdout?.destroy();
    child?.stderr?.destroy();
    child?.kill('SIGTERM'); // Only the disposable proxy, not the daemon.
    onDisconnect(failure);
  }
  function dispatch(message) {
    if (!object(message)) throw new Error();
    if (Object.hasOwn(message, 'method')) {
      if (typeof message.method !== 'string' || !message.method || message.method.length > 200 ||
          !object(message.params) || Object.hasOwn(message, 'result') || Object.hasOwn(message, 'error')) throw new Error();
      if (Object.hasOwn(message, 'id')) {
        if (!id(message.id)) throw new Error();
        onServerRequest(message);
      } else onNotification(message);
      return;
    }
    if (!id(message.id) || Object.hasOwn(message, 'result') === Object.hasOwn(message, 'error')) throw new Error();
    // Map lookup preserves the distinction between a number and a string ID.
    const entry = pending.get(message.id);
    if (!entry) return;
    if (Object.hasOwn(message, 'error') && (!object(message.error) || !Number.isInteger(message.error.code))) throw new Error();
    pending.delete(message.id);
    clearTimer(entry.timer);
    if (Object.hasOwn(message, 'error')) entry.reject(new CodexRPCError('Native Codex request was rejected; inspect the native terminal', {
      rejected: true,
      // Exact native validation refusal; never infer disabled goals from a timeout.
      reason: entry.method === 'thread/goal/get' && message.error.code === -32600 &&
        message.error.message === 'goals feature is disabled' ? 'goalsDisabled' : undefined,
    }));
    else entry.resolve(message.result);
  }
  function receive(bytes, isBinary) {
    if (failure) return;
    // ws validates UTF-8 and bounds the entire message, including fragments.
    try {
      if (isBinary) throw new Error();
      dispatch(JSON.parse(bytes.toString('utf8')));
    } catch { fail('Invalid Codex RPC message'); }
  }
  function write(data, pong = false) {
    if (failure) throw failure;
    const bytes = Buffer.byteLength(data);
    // Duplex.from can acknowledge a write before child.stdin drains. Count both
    // queues conservatively, plus the maximum masked-frame overhead.
    const outputBytes = socket.bufferedAmount + child.stdin.writableLength + bytes + 14;
    if (bytes > maxFrameBytes || outputBytes > maxOutputBytes) {
      fail('Codex RPC output exceeded the size limit');
      throw failure;
    }
    const done = error => { if (error) fail('Codex RPC write failed'); };
    try {
      if (pong) socket.pong(data, done);
      else socket.send(data, done);
    } catch { fail('Codex RPC write failed'); throw failure; }
  }
  function send(value) {
    let data;
    try { data = JSON.stringify(value); }
    catch { fail('Invalid Codex RPC request'); throw failure; }
    write(data);
  }
  try {
    // A wall-clock deadline also bounds slow handshakes without net.Socket methods.
    handshakeTimer = setTimer(() => fail('Codex WebSocket handshake timed out'), timeoutMs);
    child = spawnProcess('codex', ['app-server', 'proxy'], { stdio: ['pipe', 'pipe', 'pipe'] });
    child.on('error', () => fail('Cannot connect to the existing Codex daemon'));
    child.on('exit', () => fail('Codex proxy disconnected; native work state is unknown'));
    child.stdin.on('error', () => fail('Codex RPC write failed'));
    child.stdout.on('error', () => fail('Codex RPC read failed'));
    child.stdout.on('end', () => fail('Codex proxy disconnected; native work state is unknown'));
    child.stderr.on('error', () => fail('Codex proxy disconnected'));
    child.stderr.resume(); // Drain without retaining or publishing credentials/diagnostics.
    transport = Duplex.from({ readable: child.stdout, writable: child.stdin });
    transport.on('error', () => fail('Codex proxy disconnected'));
    socket = new WebSocket('ws://localhost/', {
      createConnection: () => transport,
      maxPayload: maxFrameBytes,
      perMessageDeflate: false,
      followRedirects: false,
      autoPong: false,
    });
    socket.on('open', () => { clearTimer(handshakeTimer); resolveOpen(); });
    socket.on('message', receive);
    socket.on('ping', data => {
      // Automatic pongs would bypass the same output budget used for RPCs.
      try { write(data, true); } catch { /* write already disconnected safely. */ }
    });
    socket.on('error', () => fail('Invalid Codex WebSocket connection'));
    socket.on('close', () => fail('Codex proxy disconnected; native work state is unknown'));
  } catch { fail('Cannot connect to the existing Codex daemon'); }
  await opened;
  if (failure) throw failure;

  return {
    request(method, params) {
      if (!METHODS.has(method) || !object(params)) return Promise.reject(new CodexRPCError('Unsupported Codex RPC request', { rejected: true }));
      if (failure) return Promise.reject(failure);
      if (pending.size >= maxPending) return Promise.reject(new CodexRPCError('Codex RPC request budget exhausted', { rejected: true }));
      return new Promise((resolve, reject) => {
        const requestId = randomUUID();
        const timer = setTimer(() => fail('Codex RPC response timed out; native work state is unknown'), timeoutMs);
        pending.set(requestId, { resolve, reject, timer, method });
        try { send({ id: requestId, method, params }); }
        catch (error) { clearTimer(timer); pending.delete(requestId); reject(error); }
      });
    },
    notify(method) {
      if (method !== 'initialized') throw new CodexRPCError('Unsupported Codex RPC notification', { rejected: true });
      send({ method });
    },
    close() { fail('Codex proxy connection closed; native session continues'); },
  };
}

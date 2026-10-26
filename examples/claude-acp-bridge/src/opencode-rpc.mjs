/** Native API version whose event and message contracts this adapter validates. */
export const OPENCODE_VERSION = '2.0.28';

// @opencode/schema 2.0.28: session-id.js and identifier.js. The OpenAPI
// prefix-only validator also permits caller-defined IDs; this bridge accepts
// only IDs produced by the native generator (12 hex and 14 base62 characters).
/** Restricts exposed sessions to the pinned native generator's ID format. */
export const isOpenCodeSessionId = value => typeof value === 'string' && /^ses_[a-f0-9]{12}[0-9A-Za-z]{14}$/.test(value);
/** Validates native message IDs before binding observation or cancellation. */
export const isOpenCodeMessageId = value => typeof value === 'string' && /^msg_[a-f0-9]{12}[0-9A-Za-z]{14}$/.test(value);
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);

/** A sanitized native transport error; never includes URLs, bodies or secrets. */
export class OpenCodeRPCError extends Error {
  constructor(message, { rejected = false, status = 502 } = {}) {
    super(message);
    this.status = status;
    this.rejected = rejected;
  }
}
const invalid = () => new OpenCodeRPCError('Invalid OpenCode 2.0.28 response');

/**
 * Native V2 HTTP/SSE transport, not an RPC subprocess or a server owner.
 *
 * Main supplies the private endpoint returned by connectOpenCodeServer:
 *   clientFactory: async callbacks => createOpenCodeClient({
 *     ...await connectOpenCodeServer({ stateFile }), ...callbacks,
 *   })
 *
 * Factory input: { onEvent(event), onDisconnect() }. It resolves only after an
 * authenticated exact-version GET /api/info and SSE server.connected marker.
 * Return: { request(method, path, body?), close() }. Responses retain their HTTP
 * JSON envelopes. Requests are bounded, never retried; close aborts only this
 * client's HTTP work. Callbacks are synchronous and must not retain unbounded
 * event data. Credentials and transport exception causes are never exposed.
 */
export async function createOpenCodeClient({
  url, password, onEvent = () => {}, onDisconnect = () => {}, fetch: fetcher = globalThis.fetch,
  requestTimeoutMs = 10000, idleTimeoutMs = 45000, maxResponseBytes = 4 * 1024 * 1024,
  maxEventBytes = 256 * 1024,
} = {}) {
  let endpoint;
  try { endpoint = new URL(url); } catch { throw new OpenCodeRPCError('Invalid private OpenCode endpoint'); }
  if (endpoint.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(endpoint.hostname) ||
      !endpoint.port || endpoint.username || endpoint.password || endpoint.pathname !== '/' || endpoint.search || endpoint.hash ||
      typeof password !== 'string' || password.length < 1 || password.length > 4096 || /[\r\n]/.test(password)) {
    throw new OpenCodeRPCError('Invalid private OpenCode endpoint');
  }
  for (const bound of [requestTimeoutMs, idleTimeoutMs, maxResponseBytes, maxEventBytes]) {
    if (!Number.isSafeInteger(bound) || bound < 1) throw new OpenCodeRPCError('Invalid OpenCode transport limit');
  }
  const authorization = 'Basic ' + Buffer.from('opencode:' + password).toString('base64');
  const lifetime = new AbortController();
  let closed = false;
  let failed = false;
  let stream;
  let readyResolve, readyReject;
  const ready = new Promise((resolve, reject) => { readyResolve = resolve; readyReject = reject; });
  // Attach a handler before initialization can fail.
  void ready.catch(() => {});
  function fail() {
    if (closed || failed) return;
    failed = true;
    lifetime.abort();
    readyReject(new OpenCodeRPCError('OpenCode event transport disconnected'));
    onDisconnect();
  }
  function target(path) {
    if (typeof path !== 'string' || !path.startsWith('/api/') || /[\r\n#\\]/.test(path)) throw invalid();
    const parsed = new URL(path, endpoint);
    if (parsed.origin !== endpoint.origin || parsed.pathname !== path.split('?')[0]) throw invalid();
    return parsed;
  }
  async function open(method, path, body, signal, accept) {
    return fetcher(target(path), {
      method, redirect: 'error', signal,
      headers: { authorization, accept, ...(body === undefined ? {} : { 'content-type': 'application/json' }) },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
  }
  async function request(method, path, body) {
    if (closed || failed) throw new OpenCodeRPCError('OpenCode client is disconnected');
    const deadline = new AbortController();
    const timer = setTimeout(() => deadline.abort(), requestTimeoutMs);
    const signal = AbortSignal.any([lifetime.signal, deadline.signal]);
    let reader;
    try {
      const response = await open(method, path, body, signal, 'application/json');
      if (!response.ok) {
        await response.body?.cancel();
        // 5xx, redirects and transport errors cannot prove a mutation was absent.
        throw new OpenCodeRPCError('Native OpenCode request rejected', { rejected: [400, 401, 403, 404, 409, 422].includes(response.status) });
      }
      if (response.status === 204) { await response.body?.cancel(); return undefined; }
      if (response.status !== 200 || response.headers.get('content-type')?.split(';')[0] !== 'application/json' || !response.body) throw invalid();
      reader = response.body.getReader();
      const chunks = [];
      let size = 0;
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        size += value.byteLength;
        if (size > maxResponseBytes) throw invalid();
        chunks.push(value);
      }
      return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks)));
    } catch (error) {
      if (error instanceof OpenCodeRPCError && error.rejected) throw error;
      fail();
      throw new OpenCodeRPCError('OpenCode request outcome is unknown');
    } finally {
      clearTimeout(timer);
      try { await reader?.cancel(); } catch {}
      reader?.releaseLock();
    }
  }
  async function subscribe() {
    const deadline = new AbortController();
    let timer;
    const touch = () => { clearTimeout(timer); timer = setTimeout(() => deadline.abort(), idleTimeoutMs); };
    const signal = AbortSignal.any([lifetime.signal, deadline.signal]);
    let reader;
    let connected = false;
    try {
      touch();
      const response = await open('GET', '/api/event', undefined, signal, 'text/event-stream');
      if (response.status !== 200 || response.headers.get('content-type')?.split(';')[0] !== 'text/event-stream' || !response.body) throw invalid();
      reader = response.body.getReader();
      const decoder = new TextDecoder('utf-8', { fatal: true });
      let buffer = '';
      let frame = [];
      let frameBytes = 0;
      function line(value) {
        if (value === '') {
          const event = frame.find(line => line.startsWith('event:'))?.slice(6).trim();
          const data = frame.filter(line => line.startsWith('data:')).map(line => line.slice(5).replace(/^ /, '')).join('\n');
          frame = []; frameBytes = 0;
          if (event === 'effect/http-api/stream/failure') throw invalid();
          if (!data) return;
          const item = JSON.parse(data);
          if (!object(item) || typeof item.type !== 'string' || typeof item.id !== 'string' || !object(item.data)) throw invalid();
          if (!connected) {
            if (item.type !== 'server.connected') throw invalid();
            connected = true; readyResolve();
          } else if (item.type === 'server.connected') throw invalid();
          else onEvent(item);
        } else {
          frameBytes += Buffer.byteLength(value);
          if (frameBytes > maxEventBytes) throw invalid();
          frame.push(value);
        }
      }
      while (!signal.aborted) {
        const next = await reader.read();
        if (next.done) throw invalid(); // SSE is live-only, even clean EOF loses observation.
        touch();
        buffer += decoder.decode(next.value, { stream: true });
        let position;
        while ((position = buffer.search(/[\r\n]/)) !== -1) {
          if (buffer[position] === '\r' && position === buffer.length - 1) break;
          const size = buffer[position] === '\r' && buffer[position + 1] === '\n' ? 2 : 1;
          const value = buffer.slice(0, position);
          buffer = buffer.slice(position + size);
          line(value);
        }
        if (Buffer.byteLength(buffer) + frameBytes > maxEventBytes) throw invalid();
      }
    } catch { fail(); }
    finally {
      clearTimeout(timer);
      try { await reader?.cancel(); } catch {}
      reader?.releaseLock();
    }
  }
  const client = {
    request,
    async close() {
      closed = true;
      lifetime.abort();
      readyReject(new OpenCodeRPCError('OpenCode client closed'));
      await stream;
    },
  };
  try {
    const info = await request('GET', '/api/info');
    if (!object(info) || info.version !== OPENCODE_VERSION) throw new OpenCodeRPCError('OpenCode requires exact version ' + OPENCODE_VERSION);
    stream = subscribe();
    const timer = setTimeout(() => fail(), requestTimeoutMs);
    try { await ready; } finally { clearTimeout(timer); }
    return client;
  } catch (error) { await client.close(); throw error; }
}

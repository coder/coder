import { createHash, randomBytes, randomUUID, timingSafeEqual } from 'node:crypto';
import { EventEmitter } from 'node:events';
import { createServer } from 'node:http';
import { isAbsolute } from 'node:path';
import { MCP_BLOCKED_REASON, NATIVE_BLOCKED_REASON, REGISTRATION_PENDING_REASON } from './native-discovery.mjs';
import { openSessionStore, sessionRecord, SESSION_LIMITS } from './session-store.mjs';

class HTTPError extends Error {
  constructor(status, message) { super(message); this.status = status; }
}
const requireValue = (condition, message) => { if (!condition) throw new HTTPError(400, message); };
const uuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);

async function readBody(req) {
  requireValue(req.headers['content-type']?.split(';')[0] === 'application/json', 'Expected application/json');
  const chunks = [];
  let bytes = 0;
  for await (const chunk of req) {
    bytes += chunk.length;
    if (bytes > 4 * 1024 * 1024) throw new HTTPError(413, 'Request body too large');
    chunks.push(chunk);
  }
  let body;
  try { body = JSON.parse(Buffer.concat(chunks).toString()); }
  catch { throw new HTTPError(400, 'Invalid JSON'); }
  requireValue(body && typeof body === 'object' && !Array.isArray(body), 'Expected a JSON object');
  return body;
}

/** Create a loopback-only, single-user broker with a byte-bounded event journal. */
export async function createBroker({
  token = randomBytes(32).toString('hex'), launch, discover, sessionFile, deferPersistence = false, now = Date.now,
  setTimer = setTimeout, clearTimer = clearTimeout, discoveryTTL = 2000,
  staleMs = 10000, commandTimeoutMs = 10000, maxEvents = 10000, waitMs = 15000,
  maxEventBytes = 128 * 1024, maxJournalBytes = 8 * 1024 * 1024,
  maxTotalJournalBytes = 32 * 1024 * 1024, maxRetiredRuntimes = 100,
} = {}) {
  for (const value of [staleMs, commandTimeoutMs, maxEvents, waitMs, maxEventBytes, maxJournalBytes, maxTotalJournalBytes, maxRetiredRuntimes, discoveryTTL]) {
    requireValue(Number.isSafeInteger(value) && value > 0, 'Broker limits must be positive integers');
  }
  const store = sessionFile === undefined ? undefined : await openSessionStore(sessionFile);
  if (store) {
    maxEvents = Math.min(maxEvents, SESSION_LIMITS.events);
    maxEventBytes = Math.min(maxEventBytes, SESSION_LIMITS.eventBytes);
    maxJournalBytes = Math.min(maxJournalBytes, SESSION_LIMITS.journalBytes);
    maxTotalJournalBytes = Math.min(maxTotalJournalBytes, SESSION_LIMITS.totalJournalBytes);
  }
  const sessions = new Map();
  const changes = new EventEmitter();
  changes.setMaxListeners(128);
  const launches = new Map();
  const journal = new Map();
  let journalBytes = 0;
  let activeHTTP = 0;
  let port;
  let closing = false;
  let discovering;
  let discoveredAt = -Infinity;
  let revision = 0;
  let savedRevision = 0;
  let writing;
  let scheduled = false;
  let persistenceError;
  let persistenceReady = !deferPersistence;
  let loading = true;

  function dirty() {
    if (!store || loading) return;
    revision++;
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(() => {
      scheduled = false;
      // Timer and disconnected-request faults must also reach disk without a
      // subsequent HTTP request. flush retains any failure for callers and close.
      void flush().catch(() => {});
    });
  }
  async function flush() {
    if (!store) return;
    if (persistenceError) throw persistenceError;
    if (!writing && savedRevision < revision) {
      const target = revision;
      writing = store.save([...sessions.values()].map(sessionRecord)).then(() => {
        savedRevision = target;
      }).catch(error => {
        persistenceError = error;
        for (const s of sessions.values()) invalidate(s);
        for (const key of changes.eventNames()) changes.emit(key);
        console.error(error.message);
        throw error;
      }).finally(() => { writing = undefined; });
    }
    if (writing) await writing;
    if (savedRevision < revision) await flush();
  }

  function newSession(sessionId, cwd, startedAt) {
    const s = { sessionId, startedAt, registrationPending: false, retired: new Set(), cwd, events: [], eventBytes: 0, cursor: 0, historyStartCursor: 0, runtimeSeq: 0, ended: false, fault: false, commands: new Map(), commandResults: new Map(), requests: new Map(), activeRequestId: null };
    sessions.set(sessionId, s);
    dirty();
    return s;
  }
  for (const record of store?.records ?? []) {
    const s = newSession(record.sessionId, record.cwd, record.startedAt);
    Object.assign(s, record, { archived: true, ended: true, mode: 'human', registrationPending: false, activeTurnId: null });
    for (const event of s.events) {
      const bytes = Buffer.byteLength(JSON.stringify(event));
      s.eventBytes += bytes;
      journalBytes += bytes;
      journal.set(event, { session: s, bytes });
    }
    while (s.events.length > maxEvents || s.eventBytes > maxJournalBytes) evict(s);
  }
  while (journalBytes > maxTotalJournalBytes) evict(journal.values().next().value.session);
  for (const s of sessions.values()) s.historyStartCursor = Math.max(s.historyStartCursor, (s.events[0]?.seq ?? s.cursor + 1) - 1);
  loading = false;

  async function refreshDiscovery(force = false) {
    if (!discover || closing) return;
    if (discovering) return discovering;
    if (!force && now() - discoveredAt < discoveryTTL) return;
    discovering = (async () => {
      let found;
      try { found = await discover(); }
      catch { return; } // Discovery is advisory; failure cannot invalidate a runtime or leak CLI diagnostics.
      if (closing || !Array.isArray(found)) return;
      for (const native of found.slice(0, 256)) {
        if (!native || !uuid(native.sessionId) || !uuid(native.launchId) || typeof native.cwd !== 'string' || !isAbsolute(native.cwd) || native.cwd.length > 4096 ||
            !Number.isSafeInteger(native.startedAt) || native.startedAt < 0 || typeof native.ended !== 'boolean') continue;
        let s = sessions.get(native.sessionId);
        if (s && (s.archived || !s.registrationPending || s.launchId !== native.launchId || s.cwd !== native.cwd)) continue;
        if (!s) {
          if (sessions.size >= 100) continue;
          s = newSession(native.sessionId, native.cwd, native.startedAt);
          Object.assign(s, { registrationPending: true, launchId: native.launchId, mode: 'human' });
        }
        const reason = [MCP_BLOCKED_REASON, NATIVE_BLOCKED_REASON].includes(native.blockedReason) ? native.blockedReason : undefined;
        if (!s.ended && s.blockedReason !== reason) {
          s.blockedReason = reason;
          append(s, 'session.pending', { blockedReason: reason });
        }
        if (native.ended && !s.ended) {
          s.ended = true;
          s.blockedReason = undefined;
          append(s, 'session.end', { reason: 'Native state reported that the session ended' });
        }
        const item = launches.get(native.launchId);
        if (item) { item.session = s; changes.emit(native.launchId); }
      }
    })();
    try { await discovering; }
    finally { discoveredAt = now(); discovering = undefined; }
  }

  function evict(s) {
    const event = s.events.shift();
    const { bytes } = journal.get(event);
    journal.delete(event);
    s.eventBytes -= bytes;
    journalBytes -= bytes;
    if (s.archived) s.historyStartCursor = event.seq;
  }
  function append(s, type, data) {
    const event = { seq: ++s.cursor, type, data };
    const bytes = Buffer.byteLength(JSON.stringify(event));
    s.events.push(event);
    s.eventBytes += bytes;
    journalBytes += bytes;
    journal.set(event, { session: s, bytes });
    while (s.events.length > maxEvents || s.eventBytes > maxJournalBytes) evict(s);
    while (journalBytes > maxTotalJournalBytes) evict(journal.values().next().value.session);
    dirty();
    changes.emit(s.sessionId);
  }
  function removeCommand(s, id) {
    const entry = s.commands.get(id);
    if (entry) clearTimer(entry.timer);
    s.commands.delete(id);
  }
  function invalidate(s) {
    for (const request of s.requests.values()) if (['queued', 'running'].includes(request.status)) request.status = 'unknown';
    s.activeRequestId = null;
    for (const id of s.commands.keys()) removeCommand(s, id);
    s.commandResults.clear();
    s.pendingHumanCommand = undefined;
  }
  function recordError(s, message) {
    // Retain only the latest summary independently of journal eviction or recovery.
    s.lastError = { message: message.slice(0, 512), at: now() };
    dirty();
  }
  function fault(s, reason) {
    if (s.fault) return;
    recordError(s, reason);
    s.fault = true;
    s.ended = true;
    s.mode = 'human';
    invalidate(s);
    append(s, 'overflow', { reason });
  }
  function isStale(s) {
    return !s.archived && !s.registrationPending && now() - s.lastSeen > staleMs;
  }
  function queueCommand(s, command) {
    if (persistenceError) throw persistenceError;
    if (s.registrationPending) throw new HTTPError(409, REGISTRATION_PENDING_REASON);
    if (s.commands.has(command.id)) return;
    // A heartbeat gap blocks new commands, not observation or known request outcomes.
    if (isStale(s)) throw new HTTPError(409, 'Native session heartbeat is stale; command was not queued');
    if (s.commands.size >= 32) throw new HTTPError(409, 'Command queue full');
    const timer = setTimer(() => fault(s, 'Command acknowledgement timed out; work state is unknown, inspect before retrying'), commandTimeoutMs);
    s.commands.set(command.id, { command, timer });
  }
  function publicSession(s) {
    return { sessionId: s.sessionId, runtimeId: s.runtimeId, ...(s.archived ? { archived: true } : {}), registrationPending: s.registrationPending, blockedReason: s.blockedReason, cwd: s.cwd, mode: s.mode, activeTurnId: s.activeTurnId, startedAt: s.startedAt, lastSeen: s.lastSeen, stale: isStale(s), version: s.version, cursor: s.cursor, historyStartCursor: s.historyStartCursor, ended: s.ended, fault: s.fault, lastError: s.lastError, attachCommand: 'claude attach ' + s.sessionId.slice(0, 8) };
  }
  function getSession(id, live = false) {
    const s = sessions.get(id);
    if (!s) throw new HTTPError(404, 'Unknown native session');
    if (live && s.archived) throw new HTTPError(409, 'Archived session is read-only; native resume is not supported');
    if (live && (s.ended || s.fault)) throw new HTTPError(409, 'Native session disconnected; attach to inspect before retrying');
    return s;
  }
  async function changed(id, timeout, signal) {
    if (closing || signal?.aborted) return;
    await new Promise(resolve => {
      const finish = () => { clearTimer(timer); changes.off(id, finish); signal?.removeEventListener('abort', finish); resolve(); };
      const timer = setTimer(finish, timeout);
      changes.once(id, finish);
      signal?.addEventListener('abort', finish, { once: true });
    });
  }
  function finishRequest(s, id, status) {
    const request = s.requests.get(id);
    if (!request || !['queued', 'running'].includes(request.status)) return;
    request.status = status;
    removeCommand(s, id);
    removeCommand(s, 'cancel:' + id);
    if (s.activeRequestId === id) s.activeRequestId = null;
  }
  function ingest(s, event) {
    const d = event.data;
    // Native exceptions and tool output can contain secrets. Only known control
    // reasons are safe for the session summary; details stay in the transcript.
    if (event.type === 'overflow') {
      recordError(s, [
        'Mod event buffer exhausted; attach to inspect the session',
        'Command deduplication budget exhausted; restart the bridge session',
      ].includes(d.reason) ? d.reason : 'Native event stream overflow; attach to inspect the session');
    } else if (event.type === 'command.error') {
      recordError(s, d.reason === 'Bridge session is closed' ? d.reason : 'Native command failed; attach to inspect the session');
    } else if (event.type === 'prompt.rejected' && d.cancelled !== true) {
      recordError(s, [
        'Session is not idle under automation control',
        'Native prompt draft or session state changed before submission',
      ].includes(d.reason) ? d.reason : 'Native prompt rejected; attach to inspect the session');
    } else if (event.type === 'tool.end' && d.isError === true) {
      recordError(s, 'Native tool call failed; inspect the transcript');
    } else if (event.type === 'turn.complete' && d.reason === 'error') {
      recordError(s, 'Native turn failed; inspect the transcript');
    }
    const request = s.requests.get(d.requestId);
    if (!s.ended && !s.fault) {
      if (event.type === 'turn.start' && request && ['queued', 'running'].includes(request.status)) {
        request.turnId ??= d.turnId;
        request.status = 'running';
        removeCommand(s, d.requestId);
      }
      if (event.type === 'prompt.accepted' && request && ['queued', 'running'].includes(request.status)) {
        request.status = 'running';
        removeCommand(s, d.requestId);
      }
      if (event.type === 'prompt.rejected') finishRequest(s, d.requestId, 'rejected');
      if (event.type === 'turn.complete' && (!request?.turnId || request.turnId === d.turnId)) finishRequest(s, d.requestId, 'completed');
      if (event.type === 'command.done' || event.type === 'command.error') {
        const entry = s.commands.get(d.commandId);
        if (s.pendingHumanCommand === d.commandId && entry) {
          if (event.type !== 'command.done' || s.reportedMode !== 'human') {
            fault(s, 'Native human control was not confirmed; inspect the native terminal before retrying');
          } else {
            s.humanRequested = false;
            s.pendingHumanCommand = undefined;
          }
        }
        if (entry?.command.type === 'mode' && s.commandResults.has(d.commandId)) s.commandResults.set(d.commandId, event);
        if (entry?.command.type === 'prompt') fault(s, 'Prompt command failed without a definitive outcome; work state is unknown');
        else removeCommand(s, d.commandId);
      }
    }
    if (event.type === 'session.end' || event.type === 'overflow') {
      s.ended = true;
      if (event.type === 'overflow') s.fault = true;
      s.mode = 'human';
      invalidate(s);
    }
    append(s, event.type, event.type === 'mode' && s.humanRequested ? { ...d, mode: 'human' } : d);
  }
  async function route(req, signal) {
    const url = new URL(req.url, 'http://127.0.0.1');
    if (req.method === 'POST' && url.pathname === '/v1/runtime') {
      const body = await readBody(req);
      requireValue(uuid(body.sessionId) && typeof body.runtimeId === 'string' && body.runtimeId.length > 0 && body.runtimeId.length < 200, 'Invalid runtime identity');
      requireValue(typeof body.cwd === 'string' && isAbsolute(body.cwd) && ['human', 'automation'].includes(body.mode), 'Invalid runtime state');
      requireValue(body.ended === undefined || typeof body.ended === 'boolean', 'Invalid ended state');
      requireValue(Buffer.byteLength(JSON.stringify({ cwd: body.cwd, version: body.version, activeTurnId: body.activeTurnId })) <= 16 * 1024, 'Runtime metadata too large');
      requireValue(Array.isArray(body.events) && body.events.length <= 128, 'Invalid event batch');
      for (const event of body.events) {
        requireValue(event && Number.isSafeInteger(event.seq) && event.seq > 0 && typeof event.type === 'string' && event.data && typeof event.data === 'object' && !Array.isArray(event.data), 'Invalid event');
        if (Buffer.byteLength(JSON.stringify(event)) > maxEventBytes) throw new HTTPError(413, 'Event payload too large');
        if (event.type === 'turn.start') requireValue(typeof event.data.turnId === 'string' && event.data.turnId.length <= 200, 'Invalid turn identity');
      }
      let s = sessions.get(body.sessionId);
      if (s?.archived) throw new HTTPError(409, 'Archived session is read-only; native resume is not supported');
      const promoting = s?.registrationPending === true;
      if (!s) {
        requireValue(sessions.size < 100, store ? 'Session archive budget exhausted; inspect saved history' : 'Session budget exhausted; restart the broker');
        // Without native discovery, only this broker's first observation is known.
        s = newSession(body.sessionId, body.cwd, now());
        s.runtimeId = body.runtimeId;
      } else if (promoting) {
        requireValue(s.launchId === body.launchId && s.cwd === body.cwd, 'Native registration does not match the discovered launch');
      } else if (s.runtimeId !== body.runtimeId) {
        if (s.retired.has(body.runtimeId)) throw new HTTPError(409, 'Retired runtime');
        if (s.retired.size >= maxRetiredRuntimes) {
          fault(s, 'Runtime replacement budget exhausted; restart the broker');
          throw new HTTPError(409, 'Runtime replacement budget exhausted; restart the broker');
        }
        s.retired.add(s.runtimeId);
        invalidate(s);
        s.runtimeId = body.runtimeId;
        s.runtimeSeq = 0;
        s.fault = false;
        s.ended = false;
        append(s, 'runtime.replaced', { reason: 'Native process or mod restarted; outstanding work is not retried' });
        // New attachments skip prior runtimes; existing readers still see the marker.
        s.historyStartCursor = s.cursor;
      }
      // Preflight the whole batch before accepting a completion from a broken stream.
      let sequence = s.runtimeSeq;
      for (const event of body.events) {
        if (event.seq <= sequence) continue;
        if (event.seq !== sequence + 1 && event.type !== 'overflow') {
          fault(s, 'Runtime event gap; work state is unknown');
          throw new HTTPError(409, 'Runtime event gap');
        }
        sequence = event.seq;
      }
      if (s.cwd !== body.cwd) dirty();
      Object.assign(s, { lastSeen: now(), cwd: body.cwd, version: body.version, activeTurnId: body.activeTurnId, reportedMode: body.mode });
      if (s.humanRequested && !s.pendingHumanCommand && body.mode === 'human') s.humanRequested = false;
      s.mode = s.ended || s.fault || s.humanRequested ? 'human' : body.mode;
      if (promoting) {
        Object.assign(s, { runtimeId: body.runtimeId, registrationPending: false, blockedReason: undefined });
        // A first runtime is not a replacement. Publish its identity before native
        // session.start, including for clients that loaded the provisional UUID.
        append(s, 'runtime.registered', { sessionId: s.sessionId, runtimeId: s.runtimeId, mode: s.mode, lastSeen: s.lastSeen });
      }
      for (const event of body.events) {
        if (event.seq <= s.runtimeSeq) continue;
        s.runtimeSeq = event.seq;
        ingest(s, event);
      }
      if (body.ended && !s.ended) {
        s.ended = true;
        s.mode = 'human';
        invalidate(s);
        append(s, 'session.end', { reason: 'Native runtime reported that it ended' });
      }
      if (s.humanRequested && !s.pendingHumanCommand && !s.ended && !s.fault) {
        // Honor Join before mod startup even when the launch settings default to
        // automation. Effective control remains human until native acknowledgement.
        const id = randomUUID();
        queueCommand(s, { id, type: 'mode', mode: 'human' });
        s.pendingHumanCommand = id;
      }
      const pendingLaunch = launches.get(body.launchId);
      if (pendingLaunch) { pendingLaunch.session = s; changes.emit(body.launchId); }
      changes.emit(s.sessionId);
      const ack = s.runtimeSeq;
      // Disk I/O can outlast a command deadline or runtime replacement. Select
      // deliverable work only after durability, never from a pre-flush snapshot.
      return () => ({ ack, commands: closing || s.runtimeId !== body.runtimeId || s.ended || s.fault ? [] : [...s.commands.values()].map(entry => entry.command) });
    }
    if (url.pathname === '/v1/sessions' && req.method === 'GET') {
      await refreshDiscovery();
      return { sessions: [...sessions.values()].map(publicSession) };
    }
    if (url.pathname === '/v1/sessions' && req.method === 'POST') {
      const { cwd } = await readBody(req);
      requireValue(typeof cwd === 'string' && isAbsolute(cwd), 'cwd must be absolute');
      if (!launch) throw new HTTPError(501, 'Native launching is not configured');
      if (store && sessions.size >= 100) throw new HTTPError(409, 'Session archive budget exhausted; inspect saved history');
      const launchId = randomUUID();
      const item = {};
      launches.set(launchId, item);
      try {
        // Only registration or verified structured native state supplies identity.
        // Native startup may be waiting for approval before it can load the mod.
        await launch(cwd, launchId);
        let expired = false;
        const deadline = setTimer(() => { expired = true; changes.emit(launchId); }, 30000);
        try {
          if (!item.session) await refreshDiscovery(true);
          while (!item.session && !expired && !signal.aborted && !closing) {
            await changed(launchId, discover ? 1000 : 30000, signal);
            if (!item.session && !expired) await refreshDiscovery();
          }
        } finally { clearTimer(deadline); }
        if (!item.session) throw new HTTPError(504, 'Native session identity is not available. Inspect claude agents and join the native terminal to check login, workspace trust and approvals before retrying');
        return publicSession(item.session);
      } finally { launches.delete(launchId); }
    }
    const match = url.pathname.match(/^\/v1\/sessions\/([^/]+)(?:\/(events|prompt|cancel|mode))?$/);
    if (!match) throw new HTTPError(404, 'Unknown route');
    const [, id, action] = match;
    if (!action && req.method === 'GET') await refreshDiscovery();
    const s = getSession(id);
    if (!action && req.method === 'GET') return publicSession(s);
    if (action === 'events' && req.method === 'GET') {
      const after = Number(url.searchParams.get('after') || 0);
      requireValue(Number.isSafeInteger(after) && after >= 0 && after <= s.cursor, 'Invalid event cursor');
      if (after < (s.events[0]?.seq ?? s.cursor + 1) - 1) throw new HTTPError(409, 'Event history expired; state must be reconciled, not replayed blindly');
      if (s.cursor === after && url.searchParams.get('wait') === '1') {
        getSession(id, true);
        await changed(id, waitMs, signal);
      }
      if (after < (s.events[0]?.seq ?? s.cursor + 1) - 1) throw new HTTPError(409, 'Event history expired during read');
      if (s.cursor === after && url.searchParams.get('wait') === '1') getSession(id, true);
      return { events: s.events.filter(e => e.seq > after), cursor: s.cursor };
    }
    if (req.method !== 'POST') throw new HTTPError(405, 'Method not allowed');
    if (s.archived) getSession(id, true);
    const requestedRuntime = s.runtimeId;
    const body = await readBody(req);
    if (s.runtimeId !== requestedRuntime) throw new HTTPError(409, 'Native runtime replaced; reattach before retrying');
    if (action === 'prompt') {
      if (s.registrationPending) throw new HTTPError(409, REGISTRATION_PENDING_REASON);
      requireValue(uuid(body.requestId) && typeof body.text === 'string' && body.text.trim().length > 0 && body.text.length <= 100000, 'Expected a UUID requestId and nonempty text up to 100000 characters');
      // Keep idempotency tombstones, not up to 1000 full prompt payloads per session.
      const textHash = createHash('sha256').update(body.text, 'utf16le').digest('hex');
      const old = s.requests.get(body.requestId);
      if (old) {
        if (old.textHash !== textHash) throw new HTTPError(409, 'Request ID already used for different text');
        return { requestId: body.requestId, status: old.status };
      }
      getSession(id, true);
      if (s.mode !== 'automation' || s.activeTurnId || s.activeRequestId) {
        recordError(s, 'Session is busy or under human control');
        throw new HTTPError(409, 'Session is busy or under human control');
      }
      if (s.requests.size >= 1000) throw new HTTPError(409, 'Request budget exhausted; start a new session');
      queueCommand(s, { id: body.requestId, type: 'prompt', text: body.text });
      s.requests.set(body.requestId, { textHash, status: 'queued' });
      s.activeRequestId = body.requestId;
      return { requestId: body.requestId };
    }
    if (action === 'cancel') {
      requireValue(uuid(body.requestId), 'Expected requestId');
      getSession(id, true);
      const request = s.requests.get(body.requestId);
      if (!request) throw new HTTPError(404, 'Unknown prompt request');
      if (['queued', 'running'].includes(request.status) && !request.cancelRequested) {
        // Keep prompt and cancel in order, even before the first delivery.
        const commandId = 'cancel:' + body.requestId;
        queueCommand(s, { id: commandId, type: 'cancel', requestId: body.requestId });
        request.cancelRequested = true;
      }
      return {};
    }
    if (action === 'mode') {
      requireValue(['human', 'automation'].includes(body.mode), 'Expected human or automation mode');
      getSession(id, true);
      if (s.registrationPending) {
        if (body.mode !== 'human') throw new HTTPError(409, REGISTRATION_PENDING_REASON);
        // Join requires no mod acknowledgement: native already owns setup and permissions.
        s.humanRequested = true;
        return {};
      }
      if (s.pendingHumanCommand) throw new HTTPError(409, 'Native human control is awaiting acknowledgement; inspect the native terminal');
      const commandId = randomUUID();
      const runtimeId = s.runtimeId;
      if (s.commandResults.size >= 32) throw new HTTPError(409, 'Command queue full');
      queueCommand(s, { id: commandId, type: 'mode', mode: body.mode });
      s.commandResults.set(commandId, null);
      try {
        while (!s.commandResults.get(commandId) && s.commands.has(commandId) && s.runtimeId === runtimeId && !s.ended && !s.fault && !signal.aborted && !closing) {
          await changed(id, waitMs, signal);
        }
        const result = s.commandResults.get(commandId);
        if (!result || s.runtimeId !== runtimeId || s.ended || s.fault) throw new HTTPError(504, 'Mode change unconfirmed; inspect the native session before retrying');
        if (result.type === 'command.error') throw new HTTPError(409, result.data.reason);
        return {};
      } finally {
        // Losing the HTTP waiter must not turn an uncertain native action into a retry.
        if (s.commands.has(commandId)) fault(s, 'Mode change waiter disconnected before acknowledgement; inspect before retrying');
        s.commandResults.delete(commandId);
        removeCommand(s, commandId);
      }
    }
    throw new HTTPError(404, 'Unknown route');
  }

  const server = createServer(async (req, res) => {
    const abort = new AbortController();
    res.once('close', () => abort.abort());
    activeHTTP++;
    let authorized = false;
    try {
      if (activeHTTP > 128) throw new HTTPError(503, 'Broker request budget exhausted');
      const supplied = Buffer.from(req.headers.authorization || '');
      const expected = Buffer.from('Bearer ' + token);
      if (req.headers.origin || req.headers.host !== '127.0.0.1:' + port || supplied.length !== expected.length || !timingSafeEqual(supplied, expected)) throw new HTTPError(403, 'Forbidden');
      authorized = true;
      if (!persistenceReady) throw new HTTPError(503, 'Session persistence preparation is not complete');
      if (persistenceError) throw persistenceError;
      const result = await route(req, abort.signal);
      await flush();
      if (!res.destroyed) { res.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify(typeof result === 'function' ? result() : result)); }
    } catch (error) {
      // Rejected batches and failed commands can still have recorded a fault.
      if (authorized) try { await flush(); } catch (failure) { error = failure; }
      if (!res.destroyed) { res.writeHead(error.status || 500, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); res.end(JSON.stringify({ error: error.message })); }
    } finally { activeHTTP--; }
  });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  port = server.address().port;
  return {
    url: 'http://127.0.0.1:' + port,
    token,
    /** Call only after exclusively claiming the descriptor associated with sessionFile. */
    async preparePersistence() {
      persistenceReady = false;
      await store?.discardInterruptedTemporary();
      persistenceReady = true;
    },
    /** Close only this broker; native Claude sessions intentionally outlive it. */
    async close() {
      closing = true;
      for (const s of sessions.values()) invalidate(s);
      for (const key of changes.eventNames()) changes.emit(key);
      server.closeAllConnections();
      await new Promise(resolve => server.close(resolve));
      await flush();
    },
  };
}

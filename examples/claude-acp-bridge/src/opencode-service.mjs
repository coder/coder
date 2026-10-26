import { createHash, randomUUID } from 'node:crypto';
import { EventEmitter } from 'node:events';
import { isAbsolute } from 'node:path';
import { isOpenCodeMessageId, isOpenCodeSessionId, OPENCODE_VERSION, OpenCodeRPCError } from './opencode-rpc.mjs';
import { openSessionStore, sessionRecord, SESSION_LIMITS } from './session-store.mjs';

const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const uuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
const hash = text => createHash('sha256').update(text, 'utf16le').digest('hex');
// The prompt API accepts caller-selected message IDs. No ID ordering is assumed.
const messageId = clientId => 'msg_' + clientId.replaceAll('-', '').slice(0, 26);
const identityEvents = new Set(['session.start', 'session.resuming', 'session.resume_failed', 'runtime.resumed']);
// The native 2.0.28 message endpoint caps each page at 200 messages.
const HISTORY_LIMIT = 200;
const busy = 'Session is busy or under human control';
class HTTPError extends Error {
  constructor(status, message) { super(message); this.status = status; }
}
function requireValue(condition, message) { if (!condition) throw new HTTPError(400, message); }
function protocol(condition) { if (!condition) throw new OpenCodeRPCError('Invalid or unbounded OpenCode native snapshot'); }
// Even JSON-escaped control characters fit the event budget at this chunk size.
function textEnd(text, start) {
  const end = Math.min(text.length, start + 16000);
  const previous = text.charCodeAt(end - 1), next = text.charCodeAt(end);
  return previous >= 0xd800 && previous <= 0xdbff && next >= 0xdc00 && next <= 0xdfff ? end - 1 : end;
}

/**
 * Delegated native V2 adapter with the same broker interface as createCodexService.
 * Production must supply a distinct, private OpenCode-only sessionFile and the
 * clientFactory documented in opencode-rpc.mjs. Construction, list and archive
 * replay are passive. Call preparePersistence after claiming the broker lock.
 *
 * SSE invalidates read-only snapshots; a periodic snapshot also detects missed
 * invalidations and background native work. Projected message order, not IDs or
 * SSE sequence numbers, defines the conversation. Only a native idle message
 * completes delivered requests. Saved output offsets/hashes deduplicate partial
 * messages after restart; history is never submitted back to the native server.
 * A missing bounded-history anchor or a rewritten prefix faults closed.
 *
 * /resume only reads the exact native session, inbox, running state, permissions,
 * forms and bounded history. It does not prompt, wake or alter configuration.
 * /cancel can remove an owned queued inbox input, never interrupt running work.
 * Mode gates new input only. close releases this observer, not the native server.
 */
export async function createOpenCodeService({
  sessionFile, clientFactory, now = Date.now, setTimer = setTimeout, clearTimer = clearTimeout,
  pollMs = 1000, waitMs = 15000,
} = {}) {
  requireValue(Number.isSafeInteger(pollMs) && pollMs >= 1 && pollMs <= 30000, 'Invalid native poll interval');
  requireValue(Number.isSafeInteger(waitMs) && waitMs >= 1 && waitMs <= 30000, 'Invalid event wait limit');
  const store = sessionFile === undefined ? undefined : await openSessionStore(sessionFile, { validateSessionId: isOpenCodeSessionId });
  const sessions = new Map();
  const changes = new EventEmitter();
  changes.setMaxListeners(128);
  const journal = new Map();
  const tasks = new Set();
  let journalBytes = 0, revision = 0, savedRevision = 0, readers = 0, generation = 0;
  let writing, persistenceError, connecting, client;
  let ready = !store, closing = false, scheduled = false, creating = false;

  function state(sessionId, cwd, startedAt) {
    return { sessionId, cwd, startedAt, runtimeId: sessionId, events: [], cursor: 0,
      historyStartCursor: 0, runtimeStartCursor: 0, eventBytes: 0, lastSeen: now(),
      mode: 'human', archived: false, ended: false, fault: false, resumePending: false,
      requests: new Map(), owners: new Map(), cache: new Map(), turns: new Map(), approvals: new Set(), observedInbox: new Set(),
      promptHistoryComplete: true, historyReceiptsComplete: true, activeTurnId: null, activeRequestId: null, nativeBusy: false,
      lastIdleId: undefined, timer: undefined, polling: undefined, pollAgain: false,
      nativeEventIds: new Set(), nativeSequence: undefined };
  }
  function cached(s, id, type) {
    let item = s.cache.get(id);
    if (!item) {
      protocol(s.cache.size < HISTORY_LIMIT);
      item = { type, parts: new Map() };
      s.cache.set(id, item);
    }
    protocol(item.type === type);
    return item;
  }
  // Restore only our persisted output receipts. Native history remains the
  // authority; each receipt is verified against a fresh snapshot on /resume.
  for (const record of store?.records ?? []) {
    const s = Object.assign(state(record.sessionId, record.cwd, record.startedAt), record,
      { archived: true, ended: true, promptHistoryComplete: record.promptTombstones !== undefined });
    sessions.set(s.sessionId, s);
    for (const { requestId, textHash, clientId } of record.promptTombstones ?? []) {
      s.requests.set(requestId, { textHash, clientId, status: 'unknown' });
      s.owners.set(messageId(clientId), requestId);
    }
    try {
      for (const event of s.events) {
        const bytes = Buffer.byteLength(JSON.stringify(event));
        s.eventBytes += bytes; journalBytes += bytes; journal.set(event, { s, bytes });
        const d = event.data;
        if (!d.nativeMessageId) continue;
        protocol(isOpenCodeMessageId(d.nativeMessageId));
        if (event.type === 'turn.complete') {
          s.lastIdleId = d.nativeMessageId; s.cache.clear(); s.turns.clear();
        } else if (event.type === 'turn.start') {
          protocol(d.turnId === d.nativeMessageId && (!d.requestId || s.owners.get(d.turnId) === d.requestId));
          protocol(/^[a-f0-9]{64}$/.test(d.nativeTextHash));
          const type = d.nativeMessageType ?? 'user';
          protocol(type === 'user' || (type === 'synthetic' && !d.requestId));
          const item = cached(s, d.turnId, type);
          protocol(!item.hash || item.hash === d.nativeTextHash);
          item.hash = d.nativeTextHash;
          s.turns.set(d.turnId, { requestId: d.requestId });
        } else if (['text', 'thought', 'tool.start', 'tool.end'].includes(event.type)) {
          protocol(Number.isSafeInteger(d.nativePartIndex) && d.nativePartIndex >= 0 && d.nativePartIndex < 1024);
          const item = cached(s, d.nativeMessageId, 'assistant');
          protocol(isOpenCodeMessageId(d.turnId) && (!item.turnId || item.turnId === d.turnId));
          item.turnId = d.turnId;
          if (event.type === 'text' || event.type === 'thought') {
            protocol(Number.isSafeInteger(d.nativeTextLength) && d.nativeTextLength >= 0 && /^[a-f0-9]{64}$/.test(d.nativeTextHash));
            item.parts.set(d.nativePartIndex, { type: event.type === 'text' ? 'text' : 'reasoning', length: d.nativeTextLength, hash: d.nativeTextHash });
          } else item.parts.set(d.nativePartIndex, { type: 'tool', id: d.toolCallId, done: event.type === 'tool.end' });
        }
      }
    } catch { throw new HTTPError(503, 'Invalid OpenCode persistence receipts; inspect the private session file'); }
    // A retained idle marker seals all evicted output. Without it a truncated
    // journal cannot prove which prefixes have already been shown to clients.
    s.historyReceiptsComplete = record.historyStartCursor === 0 || s.lastIdleId !== undefined;
  }
  function track(promise) {
    tasks.add(promise);
    void promise.finally(() => tasks.delete(promise)).catch(() => {});
    return promise;
  }
  function dirty() {
    revision++;
    if (scheduled || !ready || closing) return;
    scheduled = true;
    queueMicrotask(() => { scheduled = false; void flush().catch(() => {}); });
  }
  async function flush() {
    if (!store || !ready) return;
    if (persistenceError) throw persistenceError;
    if (!writing && savedRevision < revision) {
      const target = revision;
      writing = store.save([...sessions.values()].map(s => sessionRecord({ ...s,
        ...(s.promptHistoryComplete ? { promptTombstones: [...s.requests].map(([requestId, r]) =>
          ({ requestId, textHash: r.textHash, clientId: r.clientId })) } : {}),
      }))).then(() => { savedRevision = target; }).catch(() => {
        persistenceError = new HTTPError(503, 'OpenCode session persistence failed; history was not acknowledged');
        for (const s of sessions.values()) archive(s, persistenceError.message, true);
        throw persistenceError;
      }).finally(() => { writing = undefined; });
    }
    if (writing) await writing;
    if (savedRevision < revision) await flush();
  }
  function evict(s) {
    const event = s.events.shift();
    const { bytes } = journal.get(event);
    journal.delete(event); journalBytes -= bytes; s.eventBytes -= bytes;
    s.historyStartCursor = event.seq;
  }
  function append(s, type, data) {
    const event = { seq: s.cursor + 1, type, data };
    const bytes = Buffer.byteLength(JSON.stringify(event));
    if (bytes > SESSION_LIMITS.eventBytes) throw new OpenCodeRPCError('OpenCode event exceeded the journal budget');
    s.cursor++; s.events.push(event); s.eventBytes += bytes;
    journalBytes += bytes; journal.set(event, { s, bytes });
    while (s.events.length > SESSION_LIMITS.events || s.eventBytes > SESSION_LIMITS.journalBytes) evict(s);
    while (journalBytes > SESSION_LIMITS.totalJournalBytes) evict(journal.values().next().value.s);
    dirty(); changes.emit(s.sessionId);
  }
  function human(s) {
    if (s.mode !== 'human') { s.mode = 'human'; append(s, 'mode', { mode: 'human' }); }
  }
  function archive(s, reason, fault = false) {
    if (s.archived && (!fault || s.fault)) return;
    clearTimer(s.timer); s.timer = undefined;
    human(s);
    for (const r of s.requests.values()) {
      if (['queued', 'running', 'submitting'].includes(r.status)) r.status = 'unknown';
      clearTimer(r.timer);
    }
    Object.assign(s, { archived: true, ended: true, fault, resumePending: false, activeRequestId: null, activeTurnId: null });
    if (fault) s.lastError = { message: reason, at: now() };
    append(s, fault ? 'overflow' : 'session.end', { reason });
  }
  function disconnect(current) {
    if (current !== generation) return;
    generation++;
    const previous = client; client = undefined;
    for (const s of sessions.values()) if (!s.archived) archive(s, 'OpenCode event transport lost; native work state is unknown. Resume to reconcile bounded native history.', true);
    void previous?.close();
  }
  async function connect() {
    if (closing) throw new HTTPError(503, 'OpenCode service is closing');
    if (client) return client;
    if (connecting) return connecting;
    if (typeof clientFactory !== 'function') throw new HTTPError(503, 'OpenCode native client is not configured');
    const current = ++generation;
    connecting = (async () => {
      const connection = await clientFactory({
        onEvent: event => { if (current === generation && !closing) receive(event); },
        onDisconnect: () => disconnect(current),
      });
      try {
        const info = await connection.request('GET', '/api/info');
        protocol(info?.version === OPENCODE_VERSION);
        if (closing || current !== generation) throw new OpenCodeRPCError('OpenCode connection changed during initialization');
        client = connection;
        return connection;
      } catch (error) { await connection.close(); throw error; }
    })();
    try { return await connecting; } finally { connecting = undefined; }
  }
  function publicSession(s) {
    return { sessionId: 'opencode:' + s.sessionId, nativeSessionId: s.sessionId, harness: 'opencode',
      runtimeId: s.runtimeId, version: OPENCODE_VERSION, cwd: s.cwd, startedAt: s.startedAt, lastSeen: s.lastSeen,
      archived: s.archived, resumable: s.archived, resumePending: s.resumePending, registrationPending: false,
      mode: s.mode, ended: s.ended, fault: s.fault, stale: s.archived && s.fault,
      activeTurnId: s.activeTurnId, cursor: s.cursor, historyStartCursor: s.historyStartCursor,
      runtimeStartCursor: s.runtimeStartCursor, lastError: s.lastError && { ...s.lastError },
      attachCommand: 'coder-acp attach opencode:' + s.sessionId };
  }
  function live(s) {
    if (s.archived || s.ended || s.fault || s.resumePending || !client) throw new HTTPError(409, 'OpenCode archive is read-only until native history is reconciled with resume');
  }
  function nativeSession(value, s) {
    protocol(object(value) && isOpenCodeSessionId(value.id) && typeof value.location?.directory === 'string' &&
      isAbsolute(value.location.directory) && value.location.directory.length <= 4096 && Number.isFinite(value.time?.created));
    if (s) protocol(value.id === s.sessionId && value.location.directory === s.cwd);
    if (value.time.archived !== undefined) throw new HTTPError(409, 'Native OpenCode session is archived; inspect it in the native terminal');
    return value;
  }
  function emit(s, type, data, turnId = s.activeTurnId) {
    protocol(isOpenCodeMessageId(turnId) && s.turns.has(turnId));
    const requestId = s.turns.get(turnId).requestId;
    append(s, type, { ...data, turnId, ...(requestId ? { requestId } : {}) });
  }
  function validateMessagePage(page, limit) {
    protocol(object(page) && Array.isArray(page.data) && page.data.length <= limit && object(page.cursor));
    protocol(Buffer.byteLength(JSON.stringify(page)) <= 4 * 1024 * 1024);
    for (const cursor of [page.cursor.previous, page.cursor.next]) {
      protocol(cursor == null || (typeof cursor === 'string' && cursor.length > 0 && cursor.length <= 4096));
    }
  }
  async function readMessages(s, connection, root) {
    protocol(s.historyReceiptsComplete);
    const page = await connection.request('GET', root + '/message?limit=' + HISTORY_LIMIT + '&order=desc');
    validateMessagePage(page, HISTORY_LIMIT);
    if (!s.lastIdleId && page.cursor.next) {
      // Native cursors mark page boundaries, not the existence of more data.
      // They also carry direction: 2.0.28 rejects cursor combined with order.
      const older = await connection.request('GET', root + '/message?limit=1&cursor=' + encodeURIComponent(page.cursor.next));
      validateMessagePage(older, 1);
      if (older.data.length) throw new OpenCodeRPCError('OpenCode bounded history anchor is missing; inspect the native terminal');
      protocol(!older.cursor.previous && !older.cursor.next);
    }
    return page;
  }
  function messages(s, page) {
    const ordered = [...page.data].reverse();
    const ids = new Set();
    for (const item of ordered) {
      protocol(object(item) && isOpenCodeMessageId(item.id) && !ids.has(item.id) && typeof item.type === 'string');
      ids.add(item.id);
    }
    const boundary = s.lastIdleId ? ordered.findIndex(item => item.id === s.lastIdleId) : -1;
    if (s.lastIdleId && boundary < 0) {
      throw new OpenCodeRPCError('OpenCode bounded history anchor is missing; inspect the native terminal');
    }
    // Every persisted partial receipt must remain visible after the last idle
    // marker. A native revert/delete must not silently repeat old output.
    const pending = ordered.slice(boundary + 1);
    const visible = new Map(pending.map(item => [item.id, item.type]));
    for (const [id, item] of s.cache) protocol(visible.get(id) === item.type);
    let background = false;
    for (const m of pending) {
      if (m.type === 'user' || m.type === 'synthetic') {
        protocol(typeof m.text === 'string');
        // Native synthetic input can initiate execution, but is never ours.
        const owner = m.type === 'user' ? s.owners.get(m.id) : undefined;
        const r = owner && s.requests.get(owner);
        const textHash = hash(m.text);
        const observed = s.cache.has(m.id);
        const item = cached(s, m.id, m.type);
        protocol((!observed || item.hash === textHash) && (!r || r.textHash === textHash));
        if (!observed) {
          item.hash = textHash;
          if (!owner) human(s);
          s.turns.set(m.id, { requestId: owner });
          let start = 0;
          do {
            const end = textEnd(m.text, start);
            emit(s, 'turn.start', { text: m.text.slice(start, end), nativeMessageId: m.id,
              nativeMessageType: m.type, nativeTextHash: textHash }, m.id);
            start = end;
          } while (start < m.text.length);
        }
        s.activeTurnId = m.id;
        if (r && !['completed', 'cancelled', 'rejected'].includes(r.status)) r.status = 'running';
      } else if (m.type === 'assistant') {
        protocol(Array.isArray(m.content) && m.content.length <= 1024);
        const item = cached(s, m.id, 'assistant');
        item.turnId ??= s.activeTurnId;
        protocol(item.turnId === s.activeTurnId && s.turns.has(item.turnId));
        protocol(item.parts.size <= m.content.length);
        for (let index = 0; index < m.content.length; index++) {
          const part = m.content[index];
          protocol(object(part));
          let old = item.parts.get(index);
          protocol(!old || old.type === part.type);
          const meta = { nativeMessageId: m.id, nativePartIndex: index };
          if (part.type === 'text' || part.type === 'reasoning') {
            protocol(typeof part.text === 'string');
            old ??= { type: part.type, length: 0, hash: hash('') };
            protocol(part.text.length >= old.length && hash(part.text.slice(0, old.length)) === old.hash);
            while (old.length < part.text.length) {
              const end = textEnd(part.text, old.length);
              const text = part.text.slice(old.length, end);
              const digest = hash(part.text.slice(0, end));
              emit(s, part.type === 'text' ? 'text' : 'thought', { ...meta, text, nativeTextLength: end, nativeTextHash: digest });
              old = { type: part.type, length: end, hash: digest };
              item.parts.set(index, old);
            }
          } else if (part.type === 'tool') {
            protocol(typeof part.id === 'string' && part.id.length > 0 && part.id.length <= 200 && typeof part.name === 'string' &&
              ['streaming', 'running', 'completed', 'error'].includes(part.state?.status));
            protocol(!old || old.id === part.id);
            const done = ['completed', 'error'].includes(part.state.status);
            background ||= !done;
            if (!old) {
              emit(s, 'tool.start', { ...meta, toolCallId: part.id, title: part.name, input: part.state.input ?? null });
              old = { type: 'tool', id: part.id, done: false };
              item.parts.set(index, old);
            }
            protocol(!old.done || done);
            if (done && !old.done) {
              emit(s, 'tool.end', { ...meta, toolCallId: part.id, isError: part.state.status === 'error',
                input: part.state.input ?? null, result: part.state.content ?? null });
              old.done = true;
            }
          } else throw new OpenCodeRPCError('Unsupported OpenCode content');
        }
      } else if (m.type === 'idle') {
        protocol(['succeeded', 'failed', 'interrupted'].includes(m.outcome));
        for (const [turnId, turn] of s.turns) {
          emit(s, 'turn.complete', { nativeMessageId: m.id, answer: '',
            reason: m.outcome === 'succeeded' ? 'answer' : m.outcome === 'interrupted' ? 'aborted' : 'error' }, turnId);
          const r = s.requests.get(turn.requestId);
          if (r) { r.status = 'completed'; clearTimer(r.timer); }
          if (s.activeRequestId === turn.requestId) s.activeRequestId = null;
        }
        if (m.outcome === 'failed') s.lastError = { message: 'Native OpenCode execution failed; inspect the native terminal', at: now() };
        s.lastIdleId = m.id; s.cache.clear(); s.turns.clear(); s.activeTurnId = null;
        background = false;
      } else if (m.type === 'shell') background ||= m.status === 'running';
      else protocol(['agent-switched', 'model-switched', 'location-switched', 'system', 'skill', 'compaction'].includes(m.type));
    }
    return background;
  }
  function approvals(s, permission, forms) {
    protocol(Array.isArray(permission?.data) && permission.data.length <= 128 && Array.isArray(forms?.data) && forms.data.length <= 128);
    const next = new Set();
    for (const [kind, list] of [['permission', permission.data], ['form', forms.data]]) {
      for (const item of list) {
        protocol(object(item) && item.sessionID === s.sessionId && typeof item.id === 'string' && item.id.length <= 200);
        const key = kind + ':' + item.id;
        next.add(key);
        if (!s.approvals.has(key)) append(s, 'needs.input', { kind, pending: true,
          ...(s.activeTurnId ? { turnId: s.activeTurnId, ...(s.turns.get(s.activeTurnId)?.requestId ? { requestId: s.turns.get(s.activeTurnId).requestId } : {}) } : {}) });
      }
    }
    if ([...s.approvals].some(key => !next.has(key))) append(s, 'needs.input', { kind: 'native', pending: next.size > 0,
      ...(s.activeRequestId ? { requestId: s.activeRequestId } : {}) });
    s.approvals = next;
  }
  function kick(s, delay = 50) {
    if (s.archived || closing) return;
    if (s.polling) { s.pollAgain = true; return; }
    // A busy delta stream must not keep postponing the already-scheduled read.
    if (s.timer !== undefined && s.timerDelay <= delay) return;
    clearTimer(s.timer);
    s.timerDelay = delay;
    s.timer = setTimer(() => { s.timer = undefined; return track(poll(s).catch(() => {})); }, delay);
    s.timer?.unref?.();
  }
  async function poll(s, beforeMessages) {
    if (s.polling) return s.polling;
    if (s.archived || closing || !client) return;
    clearTimer(s.timer); s.timer = undefined;
    const connection = client;
    const current = generation;
    s.polling = (async () => {
      try {
        const root = '/api/session/' + s.sessionId;
        const [info, page, inbox, active, permissions, forms, shells] = await Promise.all([
          connection.request('GET', root), readMessages(s, connection, root),
          connection.request('GET', root + '/inbox'), connection.request('GET', '/api/session/active'),
          connection.request('GET', root + '/permission'), connection.request('GET', root + '/form'),
          connection.request('GET', '/api/shell?location%5Bdirectory%5D=' + encodeURIComponent(s.cwd)),
        ]);
        if (s.archived || closing || current !== generation || client !== connection) return;
        nativeSession(info?.data, s);
        protocol(Array.isArray(inbox?.data) && inbox.data.length <= 1000 && object(active?.data));
        beforeMessages?.();
        const background = messages(s, page);
        const nextInbox = new Set();
        for (const item of inbox.data) {
          protocol(object(item) && item.sessionID === s.sessionId && isOpenCodeMessageId(item.id) && ['user', 'synthetic', 'compaction', 'move'].includes(item.type));
          if (!s.owners.has(item.id) && !s.observedInbox.has(item.id)) human(s);
          nextInbox.add(item.id);
        }
        s.observedInbox = nextInbox;
        if (active.data[s.sessionId] !== undefined) protocol(active.data[s.sessionId]?.type === 'running');
        protocol(Array.isArray(shells?.data) && shells.data.length <= 1000 && shells.location?.directory === s.cwd);
        // Shell ownership metadata is application-defined. Conservatively gate
        // input while any native background command at this location is running.
        s.nativeBusy = background || shells.data.length > 0 || inbox.data.length > 0 || active.data[s.sessionId] !== undefined || s.turns.size > 0;
        approvals(s, permissions, forms);
        s.lastSeen = now();
        await flush();
      } catch (error) {
        archive(s, 'OpenCode history reconciliation failed; inspect the native terminal. No work was replayed.', true);
        throw error;
      }
    })();
    try { return await s.polling; }
    finally {
      s.polling = undefined;
      const delay = s.pollAgain ? 50 : pollMs; s.pollAgain = false;
      kick(s, delay);
    }
  }
  function receive(event) {
    const id = event?.data?.sessionID ?? event?.data?.form?.sessionID;
    const s = sessions.get(id);
    if (!s || s.archived) return;
    try {
      protocol(object(event) && typeof event.id === 'string' && event.id.length <= 200 && typeof event.type === 'string');
      if (s.nativeEventIds.has(event.id)) return;
      s.nativeEventIds.add(event.id);
      if (s.nativeEventIds.size > 4096) s.nativeEventIds.delete(s.nativeEventIds.values().next().value);
      // Native aggregate sequence is diagnostic only. It is not a broker cursor
      // and cannot order a snapshot or prove that volatile events were received.
      if (event.durable) {
        protocol(Number.isSafeInteger(event.durable.seq) && event.durable.seq >= 0);
        s.nativeSequence = event.durable.seq;
      }
      if (event.type === 'session.deleted' || event.type === 'session.revert.committed') {
        archive(s, 'Native OpenCode history was deleted or reverted; inspect the native terminal', true); return;
      }
      if (event.type === 'session.inbox.enqueued' && !s.owners.has(event.data.inboxID)) human(s);
      if (event.type === 'session.inbox.cancelled') {
        const requestId = s.owners.get(event.data.inboxID);
        const r = s.requests.get(requestId);
        if (r && !['completed', 'cancelled', 'rejected'].includes(r.status)) {
          r.status = 'cancelled'; clearTimer(r.timer);
          if (s.activeRequestId === requestId) s.activeRequestId = null;
          append(s, 'prompt.rejected', { requestId, cancelled: true, reason: 'Native OpenCode inbox input cancelled' });
        }
      }
      kick(s);
    } catch { archive(s, 'Invalid OpenCode event stream; native work state is unknown', true); }
  }
  async function submit(s, r, text, requestId) {
    try {
      const result = await client.request('POST', '/api/session/' + s.sessionId + '/prompt',
        { id: messageId(r.clientId), text, delivery: 'queue' });
      protocol(result?.data?.id === messageId(r.clientId) && result.data.sessionID === s.sessionId && result.data.type === 'user');
      if (s.archived || closing) return;
      if (r.status === 'submitting') r.status = 'queued';
      kick(s, 1);
    } catch (error) {
      if (s.archived || closing) return;
      if (error?.rejected && r.status === 'submitting') {
        r.status = 'rejected'; s.activeRequestId = null;
        append(s, 'prompt.rejected', { requestId, reason: 'Native OpenCode prompt rejected; inspect the native terminal' });
      } else archive(s, 'OpenCode prompt outcome is unknown; inspect the native terminal before retrying', true);
    }
  }
  async function resume(s) {
    if (s.resuming) return s.resuming;
    s.resuming = (async () => {
      // A prior generation's in-flight snapshot must drain before rejoining.
      await s.polling?.catch(() => {});
      await connect();
      const wasArchived = s.archived;
      human(s);
      if (wasArchived) {
        Object.assign(s, { archived: false, ended: false, fault: false, resumePending: true, lastError: undefined });
        append(s, 'session.resuming', { sessionId: s.sessionId, mode: 'human' });
        s.runtimeStartCursor = s.cursor;
        await flush();
      }
      try {
        // No native resume mutation exists or is needed: the server owns work.
        await poll(s, () => {
          s.resumePending = false;
          if (!wasArchived) return;
          append(s, 'runtime.resumed', { sessionId: s.sessionId, runtimeId: s.runtimeId, mode: 'human', lastSeen: now() });
          // ACP clears observed turns at the runtime boundary. Reestablish a
          // current turn without replaying its already-retained user text.
          for (const turnId of s.turns.keys()) emit(s, 'turn.start', {
            text: '', nativeMessageId: turnId, nativeMessageType: s.cache.get(turnId).type, nativeTextHash: s.cache.get(turnId).hash,
          }, turnId);
        });
        if (closing || s.archived || s.resumePending || !client) throw new OpenCodeRPCError('OpenCode resume observation failed');
        await flush();
        return publicSession(s);
      } catch (error) {
        archive(s, 'OpenCode resume failed; no native work was replayed', true);
        append(s, 'session.resume_failed', { sessionId: s.sessionId, reason: 'OpenCode resume failed; inspect the native terminal', retryable: true });
        throw error;
      }
    })();
    try { return await s.resuming; } finally { s.resuming = undefined; }
  }
  async function route(method, path, body, signal) {
    requireValue(typeof path === 'string' && path.startsWith('/v1/sessions') && !/[#\\]/.test(path), 'Invalid OpenCode route');
    const url = new URL(path, 'http://127.0.0.1');
    requireValue(url.pathname === path.split('?')[0], 'Invalid OpenCode route');
    if (url.pathname === '/v1/sessions') {
      if (method === 'GET') return { sessions: [...sessions.values()].map(publicSession) };
      if (method !== 'POST') throw new HTTPError(405, 'Method not allowed');
      requireValue(object(body) && typeof body.cwd === 'string' && isAbsolute(body.cwd) && body.cwd.length <= 4096, 'cwd must be absolute');
      if (creating || sessions.size >= SESSION_LIMITS.sessions) throw new HTTPError(409, 'OpenCode session creation budget exhausted');
      creating = true;
      try {
        const connection = await connect();
        const info = nativeSession((await connection.request('POST', '/api/session', { location: { directory: body.cwd } }))?.data);
        protocol(info.location.directory === body.cwd && !sessions.has(info.id));
        const s = state(info.id, body.cwd, Math.trunc(info.time.created));
        sessions.set(info.id, s);
        append(s, 'session.start', { sessionId: s.sessionId, runtimeId: s.runtimeId, mode: 'human', version: OPENCODE_VERSION });
        await flush();
        await poll(s);
        return publicSession(s);
      } finally { creating = false; }
    }
    const match = url.pathname.match(/^\/v1\/sessions\/([^/]+)(?:\/(events|prompt|cancel|mode|resume|stop))?$/);
    if (!match) throw new HTTPError(404, 'Unknown OpenCode route');
    let id;
    try { id = decodeURIComponent(match[1]); } catch { throw new HTTPError(400, 'Invalid OpenCode session ID'); }
    requireValue(id.startsWith('opencode:') && isOpenCodeSessionId(id.slice(9)), 'Invalid OpenCode session ID');
    const s = sessions.get(id.slice(9));
    if (!s) throw new HTTPError(404, 'Unknown OpenCode session');
    const action = match[2];
    if (!action && method === 'GET') return publicSession(s);
    if (action === 'events' && method === 'GET') {
      const after = Number(url.searchParams.get('after') || 0);
      requireValue(Number.isSafeInteger(after) && after >= 0 && after <= s.cursor, 'Invalid event cursor');
      const check = () => { if (after < s.historyStartCursor) throw new HTTPError(409, 'OpenCode event history expired; reattach explicitly'); };
      check();
      if (after === s.cursor && url.searchParams.get('wait') === '1' && !s.archived) {
        if (readers >= 128) throw new HTTPError(503, 'OpenCode event reader budget exhausted');
        readers++;
        try {
          await new Promise(resolve => {
            const finish = () => { clearTimer(timer); changes.off(s.sessionId, finish); signal?.removeEventListener('abort', finish); resolve(); };
            const timer = setTimer(finish, waitMs);
            changes.once(s.sessionId, finish);
            signal?.addEventListener('abort', finish, { once: true });
            if (closing || signal?.aborted) finish();
          });
        } finally { readers--; }
      }
      check();
      return { cursor: s.cursor, events: s.events.filter(e => e.seq > after).map(e => {
        const event = structuredClone(e);
        if (identityEvents.has(event.type) && event.data.sessionId === s.sessionId) event.data.sessionId = 'opencode:' + s.sessionId;
        return event;
      }) };
    }
    if (method !== 'POST') throw new HTTPError(405, 'Method not allowed');
    requireValue(object(body), 'Expected a JSON object');
    if (action === 'resume') return resume(s);
    if (action === 'stop') throw new HTTPError(409, 'OpenCode stop is not supported; use the native terminal');
    if (action === 'prompt') {
      requireValue(uuid(body.requestId) && typeof body.text === 'string' && body.text.trim().length > 0 && body.text.length <= 100000,
        'Expected a UUID requestId and nonempty text up to 100000 characters');
      const textHash = hash(body.text);
      const old = s.requests.get(body.requestId);
      if (old) {
        if (old.textHash !== textHash) throw new HTTPError(409, 'Request ID already used for different text');
        return { requestId: body.requestId, status: old.status === 'submitting' ? 'queued' : old.status };
      }
      live(s);
      if (s.mode !== 'automation' || s.activeRequestId) throw new HTTPError(409, busy);
      await poll(s);
      live(s);
      if (s.mode !== 'automation' || s.nativeBusy || s.approvals.size || s.activeRequestId) throw new HTTPError(409, busy);
      if (!s.promptHistoryComplete) throw new HTTPError(409, 'OpenCode prompt history is incomplete; start a new session');
      if (s.requests.size >= SESSION_LIMITS.promptTombstones) throw new HTTPError(409, 'OpenCode request budget exhausted; start a new session');
      const r = { textHash, clientId: randomUUID(), status: 'submitting' };
      s.requests.set(body.requestId, r); s.owners.set(messageId(r.clientId), body.requestId); s.activeRequestId = body.requestId;
      append(s, 'prompt.accepted', { requestId: body.requestId, nativeMessageId: messageId(r.clientId), textHash });
      // This tombstone is durable BEFORE native admission. Neither a lost HTTP
      // response nor process death can cause the same request UUID to be resent.
      await flush();
      if (s.archived || closing || s.mode !== 'automation') {
        r.status = 'rejected'; s.activeRequestId = null;
        append(s, 'prompt.rejected', { requestId: body.requestId, reason: busy });
      } else track(submit(s, r, body.text, body.requestId));
      return { requestId: body.requestId, status: r.status === 'submitting' ? 'queued' : r.status };
    }
    live(s);
    if (action === 'mode') {
      requireValue(['human', 'automation'].includes(body.mode), 'Expected human or automation mode');
      if (body.mode !== s.mode) { s.mode = body.mode; append(s, 'mode', { mode: s.mode }); }
      return {};
    }
    if (action === 'cancel') {
      requireValue(uuid(body.requestId), 'Expected requestId');
      const r = s.requests.get(body.requestId);
      if (!r) throw new HTTPError(404, 'Unknown OpenCode prompt request');
      const id = messageId(r.clientId);
      if (body.turnId !== undefined && body.turnId !== id) throw new HTTPError(409, 'OpenCode turn ID does not match the owned request');
      if (['completed', 'cancelled', 'rejected'].includes(r.status)) return {};
      if (r.cancelSent) return {};
      if (r.status !== 'queued') throw new HTTPError(409, 'OpenCode cannot atomically cancel running or ambiguous work; use the native terminal');
      r.cancelSent = true;
      try {
        // The native operation is an atomic exact-ID pending-inbox deletion. A
        // delivery race is a no-op, never a global session interrupt.
        await client.request('DELETE', '/api/session/' + s.sessionId + '/inbox/' + id);
        if (s.archived) throw new OpenCodeRPCError('OpenCode cancellation observation lost');
        await poll(s);
        if (r.status === 'running') throw new HTTPError(409, 'OpenCode input was already delivered; use the native terminal to cancel');
        if (r.status === 'queued') {
          r.timer = setTimer(() => archive(s, 'OpenCode queued cancellation was not confirmed; inspect the native terminal', true), 10000);
          r.timer?.unref?.();
        }
        return {};
      } catch (error) {
        if (!(error instanceof HTTPError) && !error?.rejected) archive(s, 'OpenCode cancellation outcome is unknown; inspect the native terminal', true);
        throw error;
      }
    }
    throw new HTTPError(404, 'Unknown OpenCode route');
  }
  return {
    async request(method, path, body, signal) {
      if (closing) throw new HTTPError(503, 'OpenCode service is closing');
      if (!ready) throw new HTTPError(503, 'OpenCode persistence preparation is not complete');
      if (persistenceError) throw persistenceError;
      signal?.throwIfAborted();
      try { const result = await route(method, path, body, signal); await flush(); return result; }
      catch (error) {
        await flush();
        if (error instanceof HTTPError || error instanceof OpenCodeRPCError) throw error;
        throw new HTTPError(502, 'OpenCode operation failed; inspect the native terminal');
      }
    },
    async preparePersistence() {
      ready = false;
      await store?.discardInterruptedTemporary();
      ready = true;
      await flush();
    },
    async close() {
      closing = true;
      for (const s of sessions.values()) archive(s, 'OpenCode broker closed; native session continues');
      for (const name of changes.eventNames()) changes.emit(name);
      await client?.close();
      await connecting?.catch(() => {});
      await Promise.allSettled([...tasks]);
      await flush();
    },
  };
}

import { createHash, randomUUID } from 'node:crypto';
import { EventEmitter } from 'node:events';
import { isAbsolute } from 'node:path';
import { CODEX_VERSION, createCodexRPC, CodexRPCError } from './codex-rpc.mjs';
import { openSessionStore, sessionRecord, SESSION_LIMITS } from './session-store.mjs';

const uuid = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const identityEvents = new Set(['session.start', 'session.resuming', 'session.resume_failed', 'runtime.registered', 'runtime.resumed']);
const tools = new Set(['commandExecution', 'fileChange', 'mcpToolCall', 'dynamicToolCall', 'collabAgentToolCall', 'webSearch']);
const approvalKinds = new Map([
  ['item/commandExecution/requestApproval', 'command'], ['item/fileChange/requestApproval', 'file-change'],
  ['item/permissions/requestApproval', 'permissions'], ['item/tool/requestUserInput', 'user-input'],
  ['mcpServer/elicitation/request', 'elicitation'], ['item/tool/call', 'tool'],
]);
const deltas = new Map([
  ['item/agentMessage/delta', 'text'], ['item/plan/delta', 'thought'],
  ['item/reasoning/summaryTextDelta', 'thought'], ['item/reasoning/textDelta', 'thought'],
]);
const busy = 'Session is busy or under human control';
class HTTPError extends Error {
  constructor(status, message) { super(message); this.status = status; }
}
function requireValue(condition, message) { if (!condition) throw new HTTPError(400, message); }
function protocol(condition) { if (!condition) throw new CodexRPCError('Unsupported or invalid Codex 0.164.0 protocol response'); }
const hash = text => createHash('sha256').update(text, 'utf16le').digest('hex');
const safeId = value => typeof value === 'string' && value.length > 0 && value.length <= 200;

/**
 * Delegated service, NOT another HTTP listener. The authenticated broker routes
 * Codex requests here and merges its list with the Claude list. sessionFile MUST
 * be a distinct Codex-only private snapshot file; internal records use native
 * UUIDs so the existing store schema and Claude archives stay unchanged.
 *
 * Call preparePersistence only after claiming the broker's exclusive descriptor.
 * No daemon connection is made by construction, list, load, or event replay.
 * rpcFactory has the callback/return contract documented in codex-rpc.mjs. The
 * service, not the factory, performs initialize and initialized. Test factories
 * must bound requests and signal disconnects just like the production transport.
 * now returns Unix milliseconds. The timer seams allow deterministic poll tests.
 *
 * /stop is unsupported; stopping native sessions requires the native terminal.
 * /cancel interrupts only the exact caller-owned request ID. Mode changes gate
 * future prompts without interrupting current work or answering approvals.
 */
export async function createCodexService({
  sessionFile, rpcFactory = createCodexRPC, now = Date.now,
  setTimer = setTimeout, clearTimer = clearTimeout, waitMs = 15000,
} = {}) {
  requireValue(Number.isSafeInteger(waitMs) && waitMs > 0 && waitMs <= 30000, 'Invalid event wait limit');
  const store = sessionFile === undefined ? undefined : await openSessionStore(sessionFile);
  const sessions = new Map();
  const changes = new EventEmitter();
  changes.setMaxListeners(128);
  const journal = new Map();
  let journalBytes = 0;
  let revision = 0;
  let savedRevision = 0;
  let writing;
  let scheduled = false;
  let persistenceError;
  let ready = !store;
  let closing = false;
  let rpc;
  let connecting;
  let generation = 0;
  let creating = false;
  let early = [];
  let earlyBytes = 0;
  let earlyOverflow = false;
  let readers = 0;
  const tasks = new Set();

  function state(sessionId, cwd, startedAt) {
    return { sessionId, cwd, startedAt, runtimeId: sessionId, version: CODEX_VERSION,
      events: [], cursor: 0, historyStartCursor: 0, runtimeStartCursor: 0, eventBytes: 0,
      mode: 'human', archived: false, ended: false, fault: false, resumePending: false,
      lastSeen: now(), activeTurnId: null, activeRequestId: null, requests: new Map(),
      promptHistoryComplete: true, nativeRevision: 0,
      owners: new Map(), turns: new Map(), approvals: new Map(), raceBytes: 0, itemCount: 0 };
  }
  for (const record of store?.records ?? []) {
    const s = Object.assign(state(record.sessionId, record.cwd, record.startedAt), record,
      { archived: true, ended: true, mode: 'human', promptHistoryComplete: record.promptTombstones !== undefined });
    sessions.set(s.sessionId, s);
    for (const { requestId, textHash, clientId } of record.promptTombstones ?? []) {
      s.requests.set(requestId, { textHash, clientId, status: 'unknown' });
      s.owners.set(clientId, requestId);
    }
    for (const event of s.events) {
      const bytes = Buffer.byteLength(JSON.stringify(event));
      s.eventBytes += bytes;
      journalBytes += bytes;
      journal.set(event, { s, bytes });
    }
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
      }))).then(() => { savedRevision = target; })
        .catch(() => {
          persistenceError = new HTTPError(503, 'Codex session persistence failed; history was not acknowledged');
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
    journal.delete(event);
    s.eventBytes -= bytes;
    journalBytes -= bytes;
    s.historyStartCursor = event.seq;
  }
  function append(s, type, data) {
    const event = { seq: s.cursor + 1, type, data };
    const bytes = Buffer.byteLength(JSON.stringify(event));
    if (bytes > SESSION_LIMITS.eventBytes) { archive(s, 'Codex event exceeded the journal size limit', true); return; }
    s.cursor++;
    s.events.push(event);
    s.eventBytes += bytes;
    journalBytes += bytes;
    journal.set(event, { s, bytes });
    while (s.events.length > SESSION_LIMITS.events || s.eventBytes > SESSION_LIMITS.journalBytes) evict(s);
    while (journalBytes > SESSION_LIMITS.totalJournalBytes) evict(journal.values().next().value.s);
    dirty();
    changes.emit(s.sessionId);
  }
  function human(s) {
    if (s.mode !== 'human') { s.mode = 'human'; append(s, 'mode', { mode: 'human' }); }
  }
  function archive(s, reason, fault = false) {
    if (s.archived && (!fault || s.fault)) return;
    human(s);
    for (const r of s.requests.values()) {
      if (['queued', 'running'].includes(r.status)) r.status = 'unknown';
      clearTimer(r.timer);
    }
    s.turns.clear(); s.approvals.clear(); s.raceBytes = 0; s.itemCount = 0;
    Object.assign(s, { archived: true, ended: true, fault, resumePending: false, activeRequestId: null });
    if (fault) s.lastError = { message: reason, at: now() };
    append(s, fault ? 'overflow' : 'session.end', { reason });
  }
  function track(promise) {
    tasks.add(promise);
    void promise.finally(() => tasks.delete(promise)).catch(() => {});
    return promise;
  }
  function disconnected(connectionGeneration) {
    if (connectionGeneration !== generation) return;
    rpc = undefined;
    generation++;
    for (const s of sessions.values()) if (!s.archived) archive(s, 'Codex proxy disconnected; native work state is unknown', true);
  }
  async function connect() {
    if (closing) throw new HTTPError(503, 'Codex service is closing');
    if (rpc) return rpc;
    if (connecting) return connecting;
    const current = ++generation;
    connecting = (async () => {
      const connection = await rpcFactory({
        onNotification: message => { if (current === generation && !closing) receive(message); },
        onServerRequest: message => { if (current === generation && !closing) receive(message); },
        onDisconnect: () => disconnected(current),
      });
      try {
        const initialized = await connection.request('initialize', {
          clientInfo: { name: 'coder_codex_bridge', title: 'Coder Codex Bridge', version: '0.1.0' },
          // Queue inspection is experimental in the installed 0.164.0 schema.
          capabilities: { experimentalApi: true, requestAttestation: false },
        });
        protocol(object(initialized) && typeof initialized.userAgent === 'string' &&
          /^[^\s/]+\/0\.164\.0(?:\s|$)/.test(initialized.userAgent) && initialized.platformFamily === 'unix' &&
          typeof initialized.codexHome === 'string' && isAbsolute(initialized.codexHome));
        if (closing || current !== generation) throw new CodexRPCError('Codex proxy disconnected during initialization');
        connection.notify('initialized');
        rpc = connection;
        return connection;
      } catch (error) { await connection.close(); throw error; }
    })();
    try { return await connecting; }
    finally { connecting = undefined; }
  }
  function publicSession(s) {
    return { sessionId: 'codex:' + s.sessionId, nativeSessionId: s.sessionId, harness: 'codex',
      runtimeId: s.runtimeId, archived: s.archived, resumable: s.archived, resumePending: s.resumePending,
      registrationPending: false, cwd: s.cwd, mode: s.mode, activeTurnId: s.activeTurnId,
      startedAt: s.startedAt, lastSeen: s.lastSeen, stale: s.archived && s.fault,
      version: CODEX_VERSION, cursor: s.cursor, historyStartCursor: s.historyStartCursor,
      runtimeStartCursor: s.runtimeStartCursor, ended: s.ended, fault: s.fault,
      lastError: s.lastError && { ...s.lastError },
      attachCommand: 'coder-acp attach codex:' + s.sessionId };
  }
  function live(s) {
    if (s.archived || s.ended || s.fault || s.resumePending || !rpc) throw new HTTPError(409, 'Codex archive is read-only until workspace startup restores it; inspect the startup logs');
  }
  function validateThread(thread, s) {
    protocol(object(thread) && uuid(thread.id) && typeof thread.cwd === 'string' && isAbsolute(thread.cwd) &&
      thread.cwd.length <= 4096 && ['idle', 'active', 'notLoaded', 'systemError'].includes(thread.status?.type));
    if (s) protocol(thread.id === s.sessionId && thread.cwd === s.cwd);
    return thread;
  }
  function turn(s, turnId) {
    protocol(safeId(turnId));
    let t = s.turns.get(turnId);
    if (!t) {
      if (s.turns.size >= 1000) throw new CodexRPCError('Codex turn tracking budget exhausted');
      t = { id: turnId, identified: false, pending: [], items: new Map(), nativeCompleted: false, completed: false, human: false };
      s.turns.set(turnId, t);
    }
    return t;
  }
  function emitTurn(s, t, type, data) {
    if (t.completed || s.archived) return;
    if (!t.identified) {
      const bytes = Buffer.byteLength(JSON.stringify({ type, data }));
      if (s.raceBytes + bytes > SESSION_LIMITS.eventBytes || t.pending.length >= 128) throw new CodexRPCError('Codex event attribution budget exhausted');
      s.raceBytes += bytes;
      t.pending.push({ type, data, bytes });
      return;
    }
    append(s, type, { ...data, turnId: t.id, ...(t.requestId ? { requestId: t.requestId } : {}) });
    if (type === 'turn.complete' && !s.archived) {
      t.completed = true;
      const r = s.requests.get(t.requestId);
      if (r) { r.status = 'completed'; if (s.activeRequestId === t.requestId) s.activeRequestId = null; }
      s.itemCount -= t.items.size;
      t.items.clear();
    }
  }
  function identify(s, t, requestId, text) {
    if (t.identified) {
      if (requestId && t.requestId !== requestId) throw new CodexRPCError('Codex turn ownership changed during submission');
      return;
    }
    t.identified = true;
    t.requestId = requestId;
    if (requestId) {
      const r = s.requests.get(requestId);
      protocol(r && (!r.turnId || r.turnId === t.id));
      r.turnId = t.id;
      clearTimer(r.timer);
      if (r.status !== 'unknown') r.status = 'running';
    } else t.human = true;
    emitTurn(s, t, 'turn.start', { text });
    for (const event of t.pending.splice(0)) {
      if (s.archived) break;
      s.raceBytes -= event.bytes;
      emitTurn(s, t, event.type, event.data);
    }
    maybeCancel(s, t.requestId);
  }
  function item(s, t, native, complete) {
    protocol(object(native) && safeId(native.id) && typeof native.type === 'string');
    if (t.completed || s.archived) return;
    let i = t.items.get(native.id);
    if (!i) {
      protocol(s.itemCount < 2000);
      s.itemCount++;
      i = { streamed: new Set(), started: false, done: false };
      t.items.set(native.id, i);
    }
    if (i.done) return;
    if (native.type === 'userMessage') {
      protocol(Array.isArray(native.content));
      const owner = s.owners.get(native.clientId);
      if (!owner) { human(s); t.human = true; }
      const text = native.content.filter(c => c.type === 'text' && typeof c.text === 'string').map(c => c.text).join('\n');
      identify(s, t, owner, text);
      // Human input into an ACP turn cedes cancellation control without relabeling
      // already-emitted events or accidentally completing a different request.
      if (!owner) t.human = true;
    } else if (native.type === 'agentMessage' || native.type === 'plan') {
      if (complete && !i.streamed.has('text')) {
        protocol(typeof native.text === 'string');
        emitTurn(s, t, native.type === 'plan' ? 'thought' : 'text', { text: native.text });
      }
    } else if (native.type === 'reasoning') {
      if (complete && i.streamed.size === 0) {
        protocol(Array.isArray(native.summary) && Array.isArray(native.content));
        emitTurn(s, t, 'thought', { text: [...native.summary, ...native.content].join('\n') });
      }
    } else if (tools.has(native.type)) {
      if (!i.started) {
        i.started = true;
        emitTurn(s, t, 'tool.start', { toolCallId: native.id, title: native.type,
          input: native.type === 'commandExecution' ? native.command : native.type === 'fileChange' ? native.changes : native.arguments });
      }
      if (complete) emitTurn(s, t, 'tool.end', { toolCallId: native.id,
        isError: ['failed', 'declined'].includes(native.status) || native.success === false,
        result: native.type === 'commandExecution' ? native.aggregatedOutput : native.type === 'fileChange' ? native.changes : native.result ?? native.contentItems ?? null });
    }
    if (complete) i.done = true;
  }
  function ingest(s, message) {
    const { method, params: p } = message;
    s.lastSeen = now();
    if (Object.hasOwn(message, 'id')) {
      const kind = approvalKinds.get(method);
      if (!kind) return; // Never synthesize auth or answer unknown server requests.
      protocol(safeId(p.turnId) || (method === 'mcpServer/elicitation/request' && p.turnId === null));
      if (s.approvals.has(message.id)) return;
      protocol(s.approvals.size < 128);
      s.approvals.set(message.id, { turnId: p.turnId, kind });
      if (p.turnId === null) append(s, 'needs.input', { kind, pending: true });
      else emitTurn(s, turn(s, p.turnId), 'needs.input', { kind, pending: true });
      return;
    }
    if (method === 'serverRequest/resolved') {
      const pending = s.approvals.get(p.requestId);
      if (pending) {
        s.approvals.delete(p.requestId);
        const data = { kind: pending.kind, pending: s.approvals.size > 0 };
        if (pending.turnId === null) append(s, 'needs.input', data);
        else emitTurn(s, turn(s, pending.turnId), 'needs.input', data);
      }
      return;
    }
    if (method === 'thread/closed' || method === 'thread/archived' || method === 'thread/deleted') {
      archive(s, 'Native Codex thread closed'); return;
    }
    if (method === 'thread/queue/changed' || method === 'thread/goal/updated') { human(s); return; }
    if (method === 'thread/status/changed') {
      protocol(['idle', 'active', 'notLoaded', 'systemError'].includes(p.status?.type));
      s.nativeStatus = p.status.type;
      s.nativeRevision++;
      if (p.status.type === 'systemError') archive(s, 'Native Codex thread failed; inspect the native terminal', true);
      return;
    }
    if (method === 'turn/started' || method === 'turn/completed') {
      protocol(object(p.turn) && Array.isArray(p.turn.items));
      const t = turn(s, p.turn.id);
      if (t.completed) return;
      const completing = method === 'turn/completed' && !t.nativeCompleted;
      if (completing) protocol(['completed', 'interrupted', 'failed'].includes(p.turn.status));
      // Native liveness is independent of identity buffering and journal emission.
      // Late starts and duplicate completions can hydrate identity, not liveness.
      if (!t.nativeCompleted) {
        s.nativeRevision++;
        if (method === 'turn/started') { s.activeTurnId = t.id; s.nativeStatus = 'active'; }
        else if (s.activeTurnId === null || s.activeTurnId === t.id) { s.activeTurnId = null; s.nativeStatus = 'idle'; }
      }
      if (completing) t.nativeCompleted = true;
      for (const native of p.turn.items) item(s, t, native, method === 'turn/completed');
      if (completing) {
        const reason = p.turn.status === 'interrupted' ? 'aborted' : p.turn.status === 'failed'
          ? (['cyberPolicy', 'misalignmentPolicyViolation'].includes(p.turn.error?.codexErrorInfo) ? 'refusal' : 'error') : 'answer';
        if (reason === 'error') s.lastError = { message: 'Native Codex turn failed; inspect the native terminal', at: now() };
        emitTurn(s, t, 'turn.complete', { reason, answer: '' });
      }
      return;
    }
    if (method === 'item/started' || method === 'item/completed') {
      item(s, turn(s, p.turnId), p.item, method === 'item/completed'); return;
    }
    const delta = deltas.get(method);
    if (delta) {
      protocol(safeId(p.itemId) && typeof p.delta === 'string');
      const t = turn(s, p.turnId);
      if (t.completed) return;
      let i = t.items.get(p.itemId);
      if (!i) { protocol(s.itemCount < 2000); s.itemCount++; i = { streamed: new Set(), started: false, done: false }; t.items.set(p.itemId, i); }
      if (i.done) return;
      i.streamed.add(delta === 'text' || method === 'item/plan/delta' ? 'text' : method);
      emitTurn(s, t, delta, { text: p.delta });
    }
  }
  function receive(message) {
    if (!object(message?.params)) return;
    const nativeId = message.params.threadId ?? (message.method === 'thread/started' ? message.params.thread?.id : undefined);
    const s = sessions.get(nativeId);
    if (!s) {
      if (creating && uuid(nativeId)) {
        const bytes = Buffer.byteLength(JSON.stringify(message));
        if (early.length >= 128 || earlyBytes + bytes > SESSION_LIMITS.eventBytes) earlyOverflow = true;
        else { early.push(message); earlyBytes += bytes; }
      }
      return;
    }
    if (s.archived) return;
    try { ingest(s, message); }
    catch { archive(s, 'Invalid or unbounded Codex event stream; native work state is unknown', true); }
  }
  function maybeCancel(s, requestId) {
    const r = s.requests.get(requestId);
    const t = r?.turnId && s.turns.get(r.turnId);
    if (!r?.cancelRequested || r.cancelSent || r.status !== 'running' || !t || t.nativeCompleted || t.completed || t.human ||
        t.requestId !== requestId || s.activeTurnId !== r.turnId || s.archived || !rpc) return;
    r.cancelSent = true;
    track(rpc.request('turn/interrupt', { threadId: s.sessionId, turnId: r.turnId }).catch(error => {
      if (!error?.rejected) archive(s, 'Codex cancellation outcome is unknown; inspect the native terminal', true);
      else { s.lastError = { message: 'Native Codex cancellation was rejected', at: now() }; dirty(); }
    }));
  }
  async function submit(s, r, text, requestId) {
    try {
      const response = await rpc.request('turn/start', { threadId: s.sessionId,
        clientUserMessageId: r.clientId, input: [{ type: 'text', text, text_elements: [] }] });
      protocol(object(response?.turn) && safeId(response.turn.id));
      if (s.archived) return;
      const t = turn(s, response.turn.id);
      protocol(!r.turnId || r.turnId === t.id);
      r.turnId = t.id;
      if (!t.completed) {
        if (!t.nativeCompleted) s.activeTurnId ??= t.id;
        // turn/start may steer a human turn that won the race. Its response is
        // a candidate identity, not proof of ownership. Match the native user
        // item's clientId before emitting or cancelling a caller-owned turn.
        for (const native of response.turn.items ?? []) if (native.type === 'userMessage') item(s, t, native, false);
        if (t.identified) protocol(t.requestId === requestId);
        else r.timer = setTimer(() => archive(s, 'Codex prompt ownership was not confirmed; native work state is unknown', true), 10000);
      } else protocol(t.requestId === requestId);
      maybeCancel(s, requestId);
    } catch (error) {
      if (error?.rejected && !r.turnId) {
        r.status = 'rejected';
        if (s.activeRequestId === requestId) s.activeRequestId = null;
        append(s, 'prompt.rejected', { requestId, reason: 'Native Codex prompt was rejected; inspect the native terminal' });
      } else if (r.status !== 'completed') archive(s, 'Codex prompt outcome is unknown; inspect the native terminal before retrying', true);
    }
  }
  async function resume(s) {
    if (s.resuming) return s.resuming;
    s.resuming = (async () => {
      const connection = await connect();
      const before = validateThread((await connection.request('thread/read', { threadId: s.sessionId, includeTurns: false }))?.thread, s);
      if (before.status.type === 'systemError') throw new HTTPError(409, 'Native Codex thread is faulted; inspect the native terminal');
      // Public, read-only APIs only. A failed check is not proof that work is absent.
      if (before.status.type === 'notLoaded') {
        const queued = await connection.request('thread/queue/list', { threadId: s.sessionId, limit: 1 });
        protocol(Array.isArray(queued?.data) && Object.hasOwn(queued, 'nextCursor'));
        if (queued.data.length || queued.nextCursor !== null) throw new HTTPError(409, 'Saved Codex thread has queued work; inspect it natively before resuming');
        let goal;
        try { goal = await connection.request('thread/goal/get', { threadId: s.sessionId }); }
        catch (error) { if (error?.reason !== 'goalsDisabled') throw error; goal = { goal: null }; }
        protocol(object(goal) && Object.hasOwn(goal, 'goal'));
        if (goal.goal !== null && goal.goal.status !== 'complete') throw new HTTPError(409, 'Saved Codex thread has an unfinished goal; inspect it natively before resuming');
      }
      const wasArchived = s.archived;
      human(s);
      if (wasArchived) {
        Object.assign(s, { archived: false, ended: false, fault: false, resumePending: true,
          activeTurnId: null, activeRequestId: null, lastError: undefined });
        s.turns.clear(); s.approvals.clear(); s.raceBytes = 0; s.itemCount = 0;
        append(s, 'session.resuming', { sessionId: s.sessionId, mode: 'human' });
        s.runtimeStartCursor = s.cursor;
        await flush();
      }
      try {
        if (closing) throw new HTTPError(503, 'Codex service is closing');
        // Never supply history, path, cwd, sandbox, approvals, auth, or a prompt.
        const nativeRevision = s.nativeRevision;
        const result = await connection.request('thread/resume', { threadId: s.sessionId, excludeTurns: true,
          initialTurnsPage: { limit: 1, itemsView: 'summary', sortDirection: 'desc' } });
        validateThread(result?.thread, s);
        protocol(Array.isArray(result.initialTurnsPage?.data) && result.initialTurnsPage.data.length <= 1);
        if (s.archived) throw new CodexRPCError('Codex disconnected while rejoining');
        const currentSnapshot = nativeRevision === s.nativeRevision;
        if (currentSnapshot) s.nativeStatus = result.thread.status.type;
        // Only hydrate identities for current work or buffered live events, even
        // when that work completed during resume. Never replay snapshot answers.
        for (const native of result.initialTurnsPage.data) {
          protocol(object(native) && safeId(native.id) && Array.isArray(native.items) &&
            ['inProgress', 'completed', 'interrupted', 'failed'].includes(native.status));
          let t = s.turns.get(native.id);
          if (native.status === 'inProgress' && (currentSnapshot ||
              (s.nativeStatus === 'active' && (s.activeTurnId === null || s.activeTurnId === native.id)))) {
            t ??= turn(s, native.id);
            if (!t.nativeCompleted && !t.completed) s.activeTurnId = t.id;
          }
          if (t && !t.completed && !t.identified) {
            for (const user of native.items) if (user.type === 'userMessage' && !t.identified) item(s, t, user, true);
          }
        }
        if ([...s.turns.values()].some(t => !t.identified && !t.completed)) {
          throw new CodexRPCError('Codex live turn identity is missing from the bounded resume page; inspect the native terminal');
        }
        if (s.archived) throw new CodexRPCError('Codex rejoin event stream faulted');
        s.resumePending = false;
        if (wasArchived) append(s, 'runtime.resumed', { sessionId: s.sessionId, runtimeId: s.runtimeId, mode: 'human', lastSeen: now() });
        await flush();
        return publicSession(s);
      } catch (error) {
        archive(s, 'Codex resume was not confirmed; inspect the native terminal before retrying', true);
        append(s, 'session.resume_failed', { sessionId: s.sessionId,
          reason: 'Codex resume was not confirmed; inspect the native terminal before retrying', retryable: error?.rejected === true });
        throw error;
      }
    })();
    try { return await s.resuming; }
    finally { s.resuming = undefined; }
  }
  async function route(method, path, body, signal) {
    requireValue(typeof path === 'string' && path.startsWith('/v1/sessions') && !path.includes('#'), 'Invalid Codex route');
    const url = new URL(path, 'http://127.0.0.1');
    requireValue(url.pathname === path.split('?')[0], 'Invalid Codex route');
    if (url.pathname === '/v1/sessions') {
      if (method === 'GET') return { sessions: [...sessions.values()].map(publicSession) };
      if (method !== 'POST') throw new HTTPError(405, 'Method not allowed');
      requireValue(object(body) && typeof body.cwd === 'string' && isAbsolute(body.cwd) && body.cwd.length <= 4096, 'cwd must be absolute');
      if (creating || sessions.size >= SESSION_LIMITS.sessions) throw new HTTPError(409, 'Codex session creation budget exhausted');
      creating = true;
      early = []; earlyBytes = 0; earlyOverflow = false;
      try {
        const connection = await connect();
        const thread = validateThread((await connection.request('thread/start', { cwd: body.cwd }))?.thread);
        protocol(!sessions.has(thread.id) && thread.cwd === body.cwd);
        const s = state(thread.id, thread.cwd, now());
        s.nativeStatus = thread.status.type;
        sessions.set(s.sessionId, s);
        append(s, 'session.start', { sessionId: s.sessionId, runtimeId: s.runtimeId, mode: 'human', version: CODEX_VERSION });
        for (const event of early.splice(0)) {
          if ((event.params.threadId ?? event.params.thread?.id) === s.sessionId) receive(event);
        }
        if (earlyOverflow) archive(s, 'Codex startup event buffer exhausted; inspect the native terminal', true);
        await flush();
        return publicSession(s);
      } finally { creating = false; early = []; earlyBytes = 0; }
    }
    const match = url.pathname.match(/^\/v1\/sessions\/([^/]+)(?:\/(events|prompt|cancel|mode|resume|stop))?$/);
    if (!match) throw new HTTPError(404, 'Unknown Codex route');
    let outward;
    try { outward = decodeURIComponent(match[1]); } catch { throw new HTTPError(400, 'Invalid Codex session ID'); }
    requireValue(outward.startsWith('codex:') && uuid(outward.slice(6)), 'Invalid Codex session ID');
    const s = sessions.get(outward.slice(6));
    if (!s) throw new HTTPError(404, 'Unknown Codex session');
    const action = match[2];
    if (!action && method === 'GET') return publicSession(s);
    if (action === 'events' && method === 'GET') {
      const after = Number(url.searchParams.get('after') || 0);
      requireValue(Number.isSafeInteger(after) && after >= 0 && after <= s.cursor, 'Invalid event cursor');
      const checkCursor = () => { if (after < s.historyStartCursor) throw new HTTPError(409, 'Codex event history expired; reattach explicitly'); };
      checkCursor();
      if (s.cursor === after && url.searchParams.get('wait') === '1' && !s.archived) {
        if (readers >= 128) throw new HTTPError(503, 'Codex event reader budget exhausted');
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
      checkCursor();
      return { cursor: s.cursor, events: s.events.filter(e => e.seq > after).map(e => {
        const event = structuredClone(e);
        if (identityEvents.has(event.type) && event.data.sessionId === s.sessionId) event.data.sessionId = 'codex:' + s.sessionId;
        return event;
      }) };
    }
    if (method !== 'POST') throw new HTTPError(405, 'Method not allowed');
    requireValue(object(body), 'Expected a JSON object');
    if (action === 'resume') return resume(s);
    if (action === 'stop') throw new HTTPError(409, 'Codex session stop is not supported; use the native terminal or cancel an owned prompt');
    if (action === 'prompt') {
      requireValue(uuid(body.requestId) && typeof body.text === 'string' && body.text.trim().length > 0 && body.text.length <= 100000,
        'Expected a UUID requestId and nonempty text up to 100000 characters');
      const textHash = hash(body.text);
      const old = s.requests.get(body.requestId);
      if (old) {
        if (old.textHash !== textHash) throw new HTTPError(409, 'Request ID already used for different text');
        return { requestId: body.requestId, status: old.status };
      }
      live(s);
      if (s.mode !== 'automation' || s.activeTurnId || s.activeRequestId || s.nativeStatus === 'active' || s.approvals.size) throw new HTTPError(409, busy);
      // An absent legacy ledger cannot prove that an evicted UUID was never sent.
      if (!s.promptHistoryComplete) throw new HTTPError(409, 'Codex prompt history is incomplete; start a new session');
      if (s.requests.size >= SESSION_LIMITS.promptTombstones) throw new HTTPError(409, 'Codex request budget exhausted; start a new session');
      const r = { textHash, clientId: randomUUID(), status: 'queued' };
      s.requests.set(body.requestId, r);
      s.owners.set(r.clientId, body.requestId);
      s.activeRequestId = body.requestId;
      append(s, 'prompt.accepted', { requestId: body.requestId, clientUserMessageId: r.clientId, textHash });
      // Persist registration before submission. Disconnecting the ACP HTTP waiter
      // cannot cancel this work, and acceptance never waits for turn completion.
      await flush();
      if (s.archived || closing || s.mode !== 'automation' || s.activeTurnId || s.nativeStatus === 'active' || s.approvals.size) {
        r.status = 'rejected'; s.activeRequestId = null;
        append(s, 'prompt.rejected', { requestId: body.requestId, reason: busy });
      } else track(submit(s, r, body.text, body.requestId));
      return { requestId: body.requestId, status: r.status };
    }
    live(s);
    if (action === 'cancel') {
      requireValue(uuid(body.requestId), 'Expected requestId');
      const r = s.requests.get(body.requestId);
      if (!r) throw new HTTPError(404, 'Unknown Codex prompt request');
      if (body.turnId !== undefined && body.turnId !== r.turnId) throw new HTTPError(409, 'Codex turn ID does not match the owned request');
      if (['queued', 'running'].includes(r.status)) {
        if (s.turns.get(r.turnId)?.human) throw new HTTPError(409, 'Native human input now owns this turn');
        r.cancelRequested = true;
        maybeCancel(s, body.requestId);
      }
      return {};
    }
    if (action === 'mode') {
      requireValue(['human', 'automation'].includes(body.mode), 'Expected human or automation mode');
      if (body.mode !== s.mode) { s.mode = body.mode; append(s, 'mode', { mode: body.mode }); }
      return {};
    }
    throw new HTTPError(404, 'Unknown Codex route');
  }
  return {
    async request(method, path, body, signal) {
      if (closing) throw new HTTPError(503, 'Codex service is closing');
      if (!ready) throw new HTTPError(503, 'Codex persistence preparation is not complete');
      if (persistenceError) throw persistenceError;
      signal?.throwIfAborted();
      try { const result = await route(method, path, body, signal); await flush(); return result; }
      catch (error) {
        await flush();
        if (error instanceof HTTPError || error instanceof CodexRPCError) throw error;
        throw new HTTPError(502, 'Codex operation failed; inspect the native terminal');
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
      for (const s of sessions.values()) archive(s, 'Codex broker closed; native session continues');
      for (const name of changes.eventNames()) changes.emit(name);
      await rpc?.close();
      await connecting?.catch(() => {});
      await Promise.allSettled([...tasks]);
      await flush();
    },
  };
}

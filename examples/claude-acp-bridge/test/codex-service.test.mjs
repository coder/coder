import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdtemp, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { createCodexService } from '../src/codex.mjs';
import { CodexRPCError } from '../src/codex-rpc.mjs';
import { SESSION_LIMITS } from '../src/session-store.mjs';

function deferred() {
  let resolve, reject;
  const promise = new Promise((a, b) => { resolve = a; reject = b; });
  return { promise, resolve, reject };
}
function nativeThread(overrides = {}) {
  return { id: randomUUID(), cwd: '/workspace', status: { type: 'idle' }, turns: [], cliVersion: '0.164.0', ...overrides };
}
function nativeTurn(id, status = 'inProgress', items = []) { return { id, status, items, error: null }; }
function resumeResult(thread, turns = thread.turns) {
  return { thread: { ...thread, turns: [] }, initialTurnsPage: { data: turns, nextCursor: null, backwardsCursor: null } };
}
const resumeParams = threadId => ({ threadId, excludeTurns: true,
  initialTurnsPage: { limit: 1, itemsView: 'summary', sortDirection: 'desc' } });
function userItem(clientId, text = 'hello') {
  return { type: 'userMessage', id: randomUUID(), clientId, content: [{ type: 'text', text, text_elements: [] }] };
}
function fakeRPC(thread = nativeThread()) {
  const calls = [];
  const hooks = {};
  const waiting = new Map();
  let callbacks;
  let closed = false;
  const closedSignal = deferred();
  const f = {
    calls, hooks, thread, factories: 0,
    factory(options) {
      f.factories++;
      callbacks = options;
      return {
        async request(method, params) {
          calls.push({ method, params });
          waiting.get(method)?.resolve(params);
          waiting.delete(method);
          if (closed) throw new CodexRPCError('Disconnected');
          const defaultResult = () => {
            if (method === 'initialize') return { userAgent: 'coder_codex_bridge/0.164.0 (Linux)', platformFamily: 'unix', platformOs: 'linux', codexHome: '/home/user/.codex' };
            if (method === 'thread/start' || method === 'thread/read') return { thread };
            if (method === 'thread/resume') return resumeResult(thread);
            if (method === 'thread/queue/list') return { data: [], nextCursor: null };
            if (method === 'thread/goal/get') return { goal: null };
            if (method === 'turn/start') return { turn: nativeTurn(randomUUID()) };
            if (method === 'turn/interrupt') return {};
            assert.fail('unexpected native method: ' + method);
          };
          const result = hooks[method] ? hooks[method](params) : defaultResult();
          return Promise.race([Promise.resolve(result), closedSignal.promise.then(() => { throw new CodexRPCError('Disconnected'); })]);
        },
        notify(method) { calls.push({ method }); },
        close() { closed = true; closedSignal.resolve(); callbacks.onDisconnect(); },
      };
    },
    next(method) { const d = deferred(); waiting.set(method, d); return d.promise; },
    emit(method, params) { callbacks.onNotification({ method, params }); },
    server(method, id, params) { callbacks.onServerRequest({ method, id, params }); },
    disconnect() { closed = true; closedSignal.resolve(); callbacks.onDisconnect(new Error('secret transport diagnostic')); },
  };
  return f;
}
async function fixture(t, options = {}) {
  const rpc = options.rpc ?? fakeRPC();
  const service = await createCodexService({ rpcFactory: rpc.factory, now: () => 1234, ...options });
  t.after(() => service.close());
  await service.preparePersistence();
  const session = await service.request('POST', '/v1/sessions', { cwd: '/workspace' });
  const path = '/v1/sessions/' + encodeURIComponent(session.sessionId);
  return { rpc, service, session, path,
    get: () => service.request('GET', path),
    events: async () => (await service.request('GET', path + '/events')).events,
    mode: mode => service.request('POST', path + '/mode', { mode }),
    send: (body = { requestId: randomUUID(), text: 'hello' }) => service.request('POST', path + '/prompt', body),
  };
}
async function privateFile(t) {
  const directory = await mkdtemp('/tmp/opencode/codex-service-test-');
  return join(directory, 'codex-sessions.json');
}
async function startOwned(f, body = { requestId: randomUUID(), text: 'hello' }) {
  await f.mode('automation');
  const pending = deferred();
  f.rpc.hooks['turn/start'] = () => pending.promise;
  const sent = f.rpc.next('turn/start');
  await f.send(body);
  const params = await sent;
  return { body, params, pending };
}
function turnStarted(f, id, items = []) { f.rpc.emit('turn/started', { threadId: f.rpc.thread.id, turn: nativeTurn(id, 'inProgress', items) }); }
function turnCompleted(f, id, status = 'completed', items = [], error = null) {
  f.rpc.emit('turn/completed', { threadId: f.rpc.thread.id, turn: { ...nativeTurn(id, status, items), error } });
}
function delta(f, turnId, itemId, text) {
  f.rpc.emit('item/agentMessage/delta', { threadId: f.rpc.thread.id, turnId, itemId, delta: text });
}

test('Codex construction/list/load are inert and new sessions persist plain IDs before returning', async t => {
  const sessionFile = await privateFile(t);
  const rpc = fakeRPC();
  const service = await createCodexService({ sessionFile, rpcFactory: rpc.factory });
  t.after(() => service.close());
  t.after(() => rm(dirname(sessionFile), { recursive: true, force: true }));
  await assert.rejects(service.request('GET', '/v1/sessions'), /preparation/);
  await service.preparePersistence();
  assert.deepEqual(await service.request('GET', '/v1/sessions'), { sessions: [] });
  assert.equal(rpc.factories, 0);
  const session = await service.request('POST', '/v1/sessions', { cwd: '/workspace', sandbox: 'bad', auth: 'bad' });
  assert.equal(session.harness, 'codex');
  assert.equal(session.nativeSessionId, rpc.thread.id);
  assert.equal(session.sessionId, 'codex:' + rpc.thread.id);
  assert.equal(session.version, '0.164.0');
  assert.equal(session.mode, 'human');
  assert.equal(session.attachCommand, 'coder-acp attach codex:' + rpc.thread.id);
  assert.deepEqual(rpc.calls.map(c => c.method), ['initialize', 'initialized', 'thread/start']);
  assert.deepEqual(rpc.calls.at(-1).params, { cwd: '/workspace' });
  const saved = JSON.parse(await readFile(sessionFile, 'utf8'));
  assert.equal(saved.sessions[0].sessionId, rpc.thread.id);
  assert.equal(saved.sessions[0].events[0].data.sessionId, rpc.thread.id);
  assert.equal((await stat(sessionFile)).mode & 0o777, 0o600);
  const path = '/v1/sessions/' + encodeURIComponent(session.sessionId);
  await service.request('GET', path);
  await service.request('GET', '/v1/sessions');
  const events = await service.request('GET', path + '/events');
  assert.equal(events.events[0].data.sessionId, session.sessionId);
  assert.equal(rpc.calls.length, 3);
});

test('notification-before-response is attributed by clientId, never by a merely pending turn', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const humanTurn = randomUUID();
  turnStarted(f, humanTurn, [userItem('native-TUI', 'human question')]);
  delta(f, humanTurn, 'native-item', 'human answer');
  turnCompleted(f, humanTurn);
  assert.equal((await f.get()).mode, 'human');
  const turnId = randomUUID();
  turnStarted(f, turnId);
  delta(f, turnId, 'own-item', 'answer');
  assert.equal((await f.events()).some(e => e.type === 'text' && e.data.text === 'answer'), false);
  f.rpc.emit('item/started', { threadId: f.rpc.thread.id, turnId, item: userItem(own.params.clientUserMessageId) });
  turnCompleted(f, turnId);
  own.pending.resolve({ turn: nativeTurn(turnId) });
  const events = await f.events();
  const humanEvents = events.filter(e => e.data.turnId === humanTurn);
  assert.ok(humanEvents.length >= 3);
  assert.ok(humanEvents.every(e => e.data.requestId === undefined));
  const ownEvents = events.filter(e => e.data.turnId === turnId);
  assert.deepEqual(ownEvents.map(e => e.type), ['turn.start', 'text', 'turn.complete']);
  assert.ok(ownEvents.every(e => e.data.requestId === own.body.requestId));
  assert.equal(ownEvents.at(-1).data.answer, '');
  assert.equal((await f.send(own.body)).status, 'completed');
});

test('response-before-notification, prompt deduplication, and final item deduplication', async t => {
  const f = await fixture(t);
  const turnId = randomUUID();
  const body = { requestId: randomUUID(), text: 'literal codex:' + f.rpc.thread.id };
  f.rpc.hooks['turn/start'] = () => ({ turn: nativeTurn(turnId) });
  await f.mode('automation');
  await f.send(body);
  await f.send(body);
  await assert.rejects(f.send({ ...body, text: 'different' }), /different text/);
  const sent = f.rpc.calls.find(c => c.method === 'turn/start').params;
  assert.notEqual(sent.clientUserMessageId, body.requestId);
  assert.deepEqual(sent.input, [{ type: 'text', text: body.text, text_elements: [] }]);
  turnStarted(f, turnId, [userItem(sent.clientUserMessageId, body.text)]);
  delta(f, turnId, 'answer', 'hello');
  const final = { type: 'agentMessage', id: 'answer', text: 'hello' };
  f.rpc.emit('item/completed', { threadId: f.rpc.thread.id, turnId, item: final });
  turnCompleted(f, turnId, 'completed', [final]);
  turnCompleted(f, turnId, 'completed', [final]);
  const events = await f.events();
  assert.equal(events.filter(e => e.type === 'turn.start').length, 1);
  assert.equal(events.find(e => e.type === 'turn.start').data.text, body.text, 'never rewrite user content');
  assert.deepEqual(events.filter(e => e.type === 'text').map(e => e.data.text), ['hello']);
  assert.equal(events.filter(e => e.type === 'turn.complete').length, 1);
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 1);
  assert.equal((await f.get()).activeTurnId, null);
});

for (const identity of ['response', 'item', 'late start', 'duplicate completion']) test(`completion before attribution via ${identity} never resurrects an active turn`, async t => {
  const ownershipTimer = deferred();
  const timers = new Map();
  const f = await fixture(t, {
    setTimer(callback, delay) {
      const id = Symbol();
      timers.set(id, callback);
      if (delay === 10000) ownershipTimer.resolve();
      return id;
    },
    clearTimer(id) { timers.delete(id); },
  });
  const own = await startOwned(f);
  const turnId = randomUUID();
  const user = userItem(own.params.clientUserMessageId);
  turnStarted(f, turnId);
  delta(f, turnId, 'answer', 'buffered answer');
  turnCompleted(f, turnId);
  assert.equal((await f.get()).activeTurnId, null);
  await f.service.request('POST', f.path + '/cancel', { requestId: own.body.requestId });
  const cursor = (await f.get()).cursor;
  const attributed = f.service.request('GET', f.path + `/events?after=${cursor}&wait=1`);
  own.pending.resolve({ turn: nativeTurn(turnId, 'inProgress', identity === 'response' ? [user] : []) });
  if (identity !== 'response') {
    await ownershipTimer.promise;
    assert.equal((await f.get()).activeTurnId, null, 'a response without identity must not resurrect completed work');
    if (identity === 'item') f.rpc.emit('item/started', { threadId: f.rpc.thread.id, turnId, item: user });
    if (identity === 'late start') turnStarted(f, turnId, [user]);
    if (identity === 'duplicate completion') turnCompleted(f, turnId, 'completed', [user]);
  }
  await attributed;
  const events = (await f.events()).filter(e => e.data.turnId === turnId);
  assert.deepEqual(events.map(e => e.type), ['turn.start', 'text', 'turn.complete']);
  assert.ok(events.every(e => e.data.requestId === own.body.requestId));
  assert.equal((await f.send(own.body)).status, 'completed');
  assert.equal((await f.get()).activeTurnId, null);
  assert.equal(timers.size, 0);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
  const next = await startOwned(f);
  const nextId = randomUUID();
  turnStarted(f, nextId, [userItem(next.params.clientUserMessageId)]);
  turnCompleted(f, nextId);
  next.pending.resolve({ turn: nativeTurn(nextId) });
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 2);
});

for (const newer of ['turn', 'status']) for (const source of ['response', 'resume']) test(`buffered completion via ${source} and its duplicates preserve a newer active ${newer}`, async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  const newerId = randomUUID();
  turnStarted(f, turnId);
  turnCompleted(f, turnId);
  if (newer === 'turn') turnStarted(f, newerId, [userItem(null, 'new native work')]);
  else f.rpc.emit('thread/status/changed', { threadId: f.rpc.thread.id, status: { type: 'active' } });
  // A duplicate completion may arrive before the original turn is attributed.
  turnCompleted(f, turnId);
  const result = nativeTurn(turnId, 'inProgress', [userItem(own.params.clientUserMessageId)]);
  if (source === 'resume') {
    f.rpc.hooks['thread/resume'] = () => resumeResult({ ...f.rpc.thread, status: { type: 'active' } }, [result]);
    await f.service.request('POST', f.path + '/resume', {});
    own.pending.resolve({ turn: result });
  } else {
    const cursor = (await f.get()).cursor;
    const attributed = f.service.request('GET', f.path + `/events?after=${cursor}&wait=1`);
    own.pending.resolve({ turn: result });
    await attributed;
  }
  assert.equal((await f.send(own.body)).status, 'completed');
  assert.equal((await f.get()).activeTurnId, newer === 'turn' ? newerId : null);
  assert.equal((await f.events()).filter(e => e.type === 'turn.complete' && e.data.turnId === turnId).length, 1);
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  if (newer === 'turn') turnCompleted(f, newerId);
  else f.rpc.emit('thread/status/changed', { threadId: f.rpc.thread.id, status: { type: 'idle' } });
  const next = await startOwned(f);
  const nextId = randomUUID();
  turnStarted(f, nextId, [userItem(next.params.clientUserMessageId)]);
  turnCompleted(f, nextId);
  next.pending.resolve({ turn: nativeTurn(nextId) });
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 2);
});

test('native approvals remain pending and a native resolution is observed without any answer', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  f.rpc.server('item/commandExecution/requestApproval', 42, {
    threadId: f.rpc.thread.id, turnId, itemId: 'tool', command: 'secret-command', reason: 'secret-token', kind: 'untrusted-native-kind',
  });
  const pending = (await f.events()).find(e => e.type === 'needs.input');
  assert.deepEqual(pending.data, { kind: 'command', pending: true, turnId, requestId: own.body.requestId });
  f.rpc.server('item/fileChange/requestApproval', 43, { threadId: f.rpc.thread.id, turnId, itemId: 'patch' });
  f.rpc.server('item/fileChange/requestApproval', 43, { threadId: f.rpc.thread.id, turnId, itemId: 'patch' });
  const inputs = async () => (await f.events()).filter(e => e.type === 'needs.input');
  assert.equal((await inputs()).length, 2, 'duplicate pending requests do not emit again');
  await f.mode('human');
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: '42' });
  assert.equal((await inputs()).length, 2, 'request IDs retain their type');
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: 42 });
  assert.equal((await inputs()).at(-1).data.pending, true, 'another native approval is still pending');
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: 42 });
  assert.equal((await inputs()).length, 3, 'duplicate resolutions do not emit again');
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: 43 });
  assert.equal((await inputs()).at(-1).data.pending, false);
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: 43 });
  assert.equal((await inputs()).length, 4, 'duplicate final resolution does not emit again');
  assert.deepEqual(f.rpc.calls.map(c => c.method), ['initialize', 'initialized', 'thread/start', 'turn/start']);
  own.pending.resolve({ turn: nativeTurn(turnId) });
});

test('turnless native MCP elicitation stays pending without faulting or answering', async t => {
  const f = await fixture(t);
  f.rpc.server('mcpServer/elicitation/request', 'elicitation', {
    threadId: f.rpc.thread.id, turnId: null, mode: 'url', url: 'https://secret', message: 'secret',
  });
  assert.equal((await f.get()).fault, false);
  assert.deepEqual((await f.events()).at(-1).data, { kind: 'elicitation', pending: true });
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  f.rpc.server('mcpServer/elicitation/request', 'second-elicitation', {
    threadId: f.rpc.thread.id, turnId: null, mode: 'url', url: 'https://secret', message: 'secret',
  });
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: 'elicitation' });
  assert.deepEqual((await f.events()).at(-1).data, { kind: 'elicitation', pending: true });
  f.rpc.emit('serverRequest/resolved', { threadId: f.rpc.thread.id, requestId: 'second-elicitation' });
  assert.deepEqual((await f.events()).at(-1).data, { kind: 'elicitation', pending: false });
  await f.mode('automation');
  assert.equal(f.rpc.calls.length, 3);
});

test('Observe restores future automation control while a turn is busy without interrupting or resubmitting', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  f.rpc.emit('thread/status/changed', { threadId: f.rpc.thread.id, status: { type: 'active', activeFlags: [] } });
  await f.mode('human');
  await f.mode('automation');
  const session = await f.get();
  assert.equal(session.mode, 'automation');
  assert.equal(session.activeTurnId, turnId);
  await assert.rejects(f.send(), /busy/);
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 1);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
  own.pending.resolve({ turn: nativeTurn(turnId) });
});

test('unsupported Codex stop leaves the observer and active native turn unchanged', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  const before = await f.get();
  const events = await f.events();
  const calls = f.rpc.calls.length;
  await assert.rejects(f.service.request('POST', f.path + '/stop', {}), error =>
    error.status === 409 && error.message === 'Codex session stop is not supported; use the native terminal or cancel an owned prompt');
  assert.deepEqual(await f.get(), before);
  assert.deepEqual(await f.events(), events);
  assert.equal(f.rpc.calls.length, calls);
  delta(f, turnId, 'answer', 'still observing');
  turnCompleted(f, turnId);
  assert.equal((await f.events()).at(-2).data.text, 'still observing');
  assert.equal((await f.events()).at(-1).data.requestId, own.body.requestId);
  own.pending.resolve({ turn: nativeTurn(turnId) });
});

test('cancel waits for exact owned turn identity; Join and close never interrupt', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  await f.service.request('POST', f.path + '/cancel', { requestId: own.body.requestId });
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
  const turnId = randomUUID();
  const interrupted = f.rpc.next('turn/interrupt');
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  assert.deepEqual(await interrupted, { threadId: f.rpc.thread.id, turnId });
  own.pending.resolve({ turn: nativeTurn(turnId) });
  await f.service.request('POST', f.path + '/cancel', { requestId: own.body.requestId });
  await assert.rejects(f.service.request('POST', f.path + '/cancel', { requestId: randomUUID() }), /Unknown/);
  await assert.rejects(f.service.request('POST', f.path + '/cancel', { requestId: own.body.requestId, turnId: 'wrong' }), /does not match/);
  await f.mode('human');
  assert.equal((await f.get()).activeTurnId, turnId);
  await f.service.close();
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/interrupt').length, 1);
});

test('a human steering an owned turn takes control without cancelling or changing previous attribution', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  f.rpc.emit('item/started', { threadId: f.rpc.thread.id, turnId, item: userItem(null, 'native steering') });
  assert.equal((await f.get()).mode, 'human');
  await assert.rejects(f.service.request('POST', f.path + '/cancel', { requestId: own.body.requestId }), /human input/);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
  own.pending.resolve({ turn: nativeTurn(turnId) });
});

test('thought/tool events and unstreamed final text retain the public ACP event shape', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  const emit = (method, fields) => f.rpc.emit(method, { threadId: f.rpc.thread.id, turnId, ...fields });
  emit('item/reasoning/summaryTextDelta', { itemId: 'reason', delta: 'thinking' });
  emit('item/completed', { item: { id: 'reason', type: 'reasoning', summary: ['thinking'], content: [] } });
  emit('item/started', { item: { type: 'commandExecution', id: 'tool', command: 'pwd', status: 'inProgress' } });
  emit('item/completed', { item: { type: 'commandExecution', id: 'tool', command: 'pwd', status: 'completed', aggregatedOutput: '/workspace' } });
  emit('item/completed', { item: { type: 'agentMessage', id: 'answer', text: 'done' } });
  turnCompleted(f, turnId);
  own.pending.resolve({ turn: nativeTurn(turnId) });
  const events = (await f.events()).filter(e => e.data.turnId === turnId);
  assert.deepEqual(events.map(e => e.type), ['turn.start', 'thought', 'tool.start', 'tool.end', 'text', 'turn.complete']);
  assert.equal(events[2].data.toolCallId, 'tool');
  assert.equal(events[3].data.result, '/workspace');
  assert.equal(events[5].data.answer, '');
});

for (const [native, info, reason] of [['completed', null, 'answer'], ['interrupted', null, 'aborted'], ['failed', 'other', 'error'], ['failed', 'misalignmentPolicyViolation', 'refusal']]) {
  test(`native ${native}/${info} completion maps to ${reason} without exposing diagnostic text`, async t => {
    const f = await fixture(t);
    const own = await startOwned(f);
    const turnId = randomUUID();
    turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
    turnCompleted(f, turnId, native, [], { codexErrorInfo: info, message: 'secret-token' });
    own.pending.resolve({ turn: nativeTurn(turnId) });
    const event = (await f.events()).find(e => e.type === 'turn.complete');
    assert.equal(event.data.reason, reason);
    assert.equal(event.data.answer, '');
    assert.equal(JSON.stringify(await f.get()).includes('secret-token'), false);
  });
}

test('disconnect marks work unknown, pauses automation, and leaves an inert readable archive', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  f.rpc.disconnect();
  const session = await f.get();
  assert.equal(session.archived, true);
  assert.equal(session.fault, true);
  assert.equal(session.mode, 'human');
  assert.equal((await f.send(own.body)).status, 'unknown');
  await assert.rejects(f.send(), /read-only/);
  await f.events();
  await f.service.request('GET', '/v1/sessions');
  assert.equal(f.rpc.factories, 1);
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 1);
  assert.equal(JSON.stringify(session).includes('secret'), false);
});

test('definite rejection is not a transport fault, while ambiguous failure is never retried', async t => {
  const f = await fixture(t);
  await f.mode('automation');
  const rejected = { requestId: randomUUID(), text: 'rejected' };
  f.rpc.hooks['turn/start'] = () => { throw new CodexRPCError('safe rejection', { rejected: true }); };
  await f.send(rejected);
  assert.equal((await f.send(rejected)).status, 'rejected');
  assert.equal((await f.get()).archived, false);
  const unknown = { requestId: randomUUID(), text: 'unknown' };
  f.rpc.hooks['turn/start'] = () => { throw new Error('secret-token'); };
  await f.send(unknown);
  assert.equal((await f.send(unknown)).status, 'unknown');
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 2);
  assert.equal(JSON.stringify(await f.events()).includes('secret-token'), false);
});

test('saved archive startup is inert, explicit cold resume checks public work state and preserves identity', async t => {
  const sessionFile = await privateFile(t);
  const original = await fixture(t, { sessionFile });
  await original.mode('automation');
  await original.service.close();
  const rpc = fakeRPC({ ...original.rpc.thread, status: { type: 'notLoaded' } });
  const resumed = await createCodexService({ sessionFile, rpcFactory: rpc.factory, now: () => 4567 });
  t.after(() => resumed.close());
  t.after(() => rm(dirname(sessionFile), { recursive: true, force: true }));
  await resumed.preparePersistence();
  const archive = await resumed.request('GET', original.path);
  await resumed.request('GET', '/v1/sessions');
  await resumed.request('GET', original.path + '/events');
  assert.equal(archive.mode, 'human');
  assert.equal(archive.archived, true);
  assert.equal(rpc.factories, 0);
  rpc.hooks['thread/resume'] = params => {
    assert.deepEqual(params, resumeParams(rpc.thread.id));
    return resumeResult({ ...rpc.thread, status: { type: 'idle' } });
  };
  const session = await resumed.request('POST', original.path + '/resume', { cwd: '/different', history: ['never replay'] });
  assert.equal(session.sessionId, archive.sessionId);
  assert.equal(session.runtimeId, archive.runtimeId);
  assert.equal(session.startedAt, archive.startedAt);
  assert.equal(session.archived, false);
  assert.equal(session.mode, 'human');
  assert.ok(session.runtimeStartCursor > archive.cursor);
  assert.deepEqual(rpc.calls.map(c => c.method), ['initialize', 'initialized', 'thread/read', 'thread/queue/list', 'thread/goal/get', 'thread/resume']);
  const { events } = await resumed.request('GET', original.path + '/events');
  assert.deepEqual(events.slice(-2).map(e => e.type), ['session.resuming', 'runtime.resumed']);
  assert.ok(events.filter(e => e.data.sessionId).every(e => e.data.sessionId === session.sessionId));
});

for (const blocker of ['queue', 'queue-page', 'goal', 'unreadable-goal']) test(`cold resume refuses ${blocker} without native mutation`, async t => {
  const f = await fixture(t);
  f.rpc.emit('thread/closed', { threadId: f.rpc.thread.id });
  f.rpc.thread.status = { type: 'notLoaded' };
  if (blocker === 'queue') f.rpc.hooks['thread/queue/list'] = () => ({ data: [{ input: 'secret' }], nextCursor: null });
  if (blocker === 'queue-page') f.rpc.hooks['thread/queue/list'] = () => ({ data: [], nextCursor: 'next' });
  if (blocker === 'goal') f.rpc.hooks['thread/goal/get'] = () => ({ goal: { status: 'active', objective: 'secret' } });
  if (blocker === 'unreadable-goal') f.rpc.hooks['thread/goal/get'] = () => { throw new CodexRPCError('Cannot inspect goals'); };
  await assert.rejects(f.service.request('POST', f.path + '/resume', {}));
  assert.equal(f.rpc.calls.some(c => c.method === 'thread/resume'), false);
  assert.equal((await f.get()).archived, true);
});

test('live explicit rejoin preserves events and an active owned turn without replaying history', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  const user = userItem(own.params.clientUserMessageId);
  turnStarted(f, turnId, [user]);
  own.pending.resolve({ turn: nativeTurn(turnId) });
  delta(f, turnId, 'answer', 'before');
  const before = await f.events();
  f.rpc.thread.status = { type: 'active', activeFlags: [] };
  f.rpc.hooks['thread/resume'] = () => {
    delta(f, turnId, 'answer', 'during');
    return resumeResult(f.rpc.thread, [nativeTurn(turnId, 'inProgress', [user,
      { id: 'answer', type: 'agentMessage', text: 'HISTORY MUST NOT REPLAY' }])]);
  };
  const resumed = await f.service.request('POST', f.path + '/resume', {});
  assert.equal(resumed.activeTurnId, turnId);
  assert.equal(resumed.runtimeStartCursor, 0);
  assert.equal(resumed.mode, 'human');
  turnCompleted(f, turnId);
  const events = await f.events();
  assert.deepEqual(events.slice(0, before.length), before);
  assert.deepEqual(events.filter(e => e.type === 'text').map(e => e.data.text), ['before', 'during']);
  assert.equal(events.at(-1).data.requestId, own.body.requestId);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
});

for (const statusNotification of [false, true]) test(`resume requests one summary turn rather than hydrating unlimited history (active notification: ${statusNotification})`, async t => {
  const f = await fixture(t);
  f.rpc.emit('thread/closed', { threadId: f.rpc.thread.id });
  const turnId = randomUUID();
  const current = nativeTurn(turnId, 'inProgress', [userItem(null, 'live question')]);
  const old = nativeTurn(randomUUID(), 'completed', [{ id: 'old-answer', type: 'agentMessage', text: 'x'.repeat(2 * 1024 * 1024) }]);
  f.rpc.hooks['thread/resume'] = params => {
    assert.deepEqual(params, resumeParams(f.rpc.thread.id));
    const thread = { ...f.rpc.thread, status: { type: 'active' }, turns: [old, current] };
    if (statusNotification) f.rpc.emit('thread/status/changed', { threadId: thread.id, status: thread.status });
    return params.excludeTurns ? resumeResult(thread, [current]) : { thread };
  };
  const joined = await f.service.request('POST', f.path + '/resume', {});
  assert.equal(joined.activeTurnId, turnId);
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  const events = await f.events();
  assert.deepEqual(events.filter(e => e.type === 'turn.start').map(e => e.data.text), ['live question']);
  assert.equal(events.some(e => e.type === 'text'), false);
  assert.deepEqual(f.rpc.calls.filter(c => c.method.startsWith('thread/')).map(c => c.method), ['thread/start', 'thread/read', 'thread/resume']);
});

for (const newer of ['turn/completed', 'idle', 'active-turn']) test(`resume snapshot cannot overwrite newer ${newer} live state`, async t => {
  const f = await fixture(t);
  f.rpc.emit('thread/closed', { threadId: f.rpc.thread.id });
  const staleId = randomUUID();
  const currentId = randomUUID();
  const pending = deferred();
  f.rpc.hooks['thread/resume'] = () => pending.promise;
  const requested = f.rpc.next('thread/resume');
  const joining = f.service.request('POST', f.path + '/resume', {});
  await requested;
  if (newer === 'turn/completed') {
    turnStarted(f, staleId, [userItem(null, 'live before completion')]);
    turnCompleted(f, staleId);
  } else if (newer === 'idle') {
    f.rpc.emit('thread/status/changed', { threadId: f.rpc.thread.id, status: { type: 'idle' } });
  } else {
    turnStarted(f, currentId, [userItem(null, 'new live turn')]);
  }
  pending.resolve(resumeResult({ ...f.rpc.thread, status: { type: 'active' } }, [
    nativeTurn(staleId, 'inProgress', [userItem(null, 'stale snapshot text')]),
  ]));
  const joined = await joining;
  assert.equal(joined.activeTurnId, newer === 'active-turn' ? currentId : null);
  assert.equal((await f.events()).some(e => e.data.text === 'stale snapshot text'), false);
  await f.mode('automation');
  if (newer === 'active-turn') await assert.rejects(f.send(), /busy/);
  else {
    f.rpc.hooks['turn/start'] = () => { throw new CodexRPCError('rejected', { rejected: true }); };
    await f.send();
    assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 1, 'stale active status must not block dispatch');
  }
  assert.equal((await f.get()).fault, false);
});

for (const snapshotStatus of ['completed', 'inProgress']) test(`resume hydrates buffered live completion identity from a ${snapshotStatus} summary`, async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  own.pending.resolve({ turn: nativeTurn(turnId) });
  await f.get();
  f.rpc.emit('thread/closed', { threadId: f.rpc.thread.id });
  f.rpc.hooks['thread/resume'] = () => {
    turnStarted(f, turnId);
    delta(f, turnId, 'answer', 'only live text');
    turnCompleted(f, turnId);
    return resumeResult({ ...f.rpc.thread, status: { type: snapshotStatus === 'inProgress' ? 'active' : 'idle' } }, [
      nativeTurn(turnId, snapshotStatus, [userItem(own.params.clientUserMessageId),
        { id: 'answer', type: 'agentMessage', text: 'old snapshot answer must not replay' }]),
    ]);
  };
  const joined = await f.service.request('POST', f.path + '/resume', {});
  assert.equal(joined.activeTurnId, null);
  const events = (await f.events()).filter(e => e.data.turnId === turnId);
  assert.deepEqual(events.map(e => e.type), ['turn.start', 'text', 'turn.complete']);
  assert.ok(events.every(e => e.data.requestId === own.body.requestId));
  assert.equal(events[1].data.text, 'only live text');
  assert.equal((await f.send(own.body)).status, 'completed');
  // Rejoining an already-observed completion must not replay even its start text.
  f.rpc.hooks['thread/resume'] = () => resumeResult(f.rpc.thread, [nativeTurn(turnId, 'completed', [userItem(own.params.clientUserMessageId)])]);
  await f.service.request('POST', f.path + '/resume', {});
  assert.deepEqual((await f.events()).filter(e => e.data.turnId === turnId), events);
  await f.mode('automation');
  f.rpc.hooks['turn/start'] = () => { throw new CodexRPCError('rejected', { rejected: true }); };
  await f.send();
  assert.equal(f.rpc.calls.filter(c => c.method === 'turn/start').length, 2);
});

for (const missing of ['older buffered turn', 'summary user item']) test(`resume faults explicitly for ${missing} instead of retaining unattributed live events forever`, async t => {
  const f = await fixture(t);
  f.rpc.emit('thread/closed', { threadId: f.rpc.thread.id });
  const turnId = randomUUID();
  f.rpc.hooks['thread/resume'] = () => {
    turnStarted(f, turnId);
    delta(f, turnId, 'answer', 'must remain unattributed');
    turnCompleted(f, turnId);
    const summary = missing === 'older buffered turn'
      ? nativeTurn(randomUUID(), 'completed', [userItem(null, 'historical prompt')])
      : nativeTurn(turnId, 'completed');
    return resumeResult(f.rpc.thread, [summary]);
  };
  await assert.rejects(f.service.request('POST', f.path + '/resume', {}), /identity/);
  const session = await f.get();
  assert.equal(session.archived, true);
  assert.equal(session.fault, true);
  assert.equal((await f.events()).some(e => ['text', 'turn.start'].includes(e.type)), false);
  assert.equal(f.rpc.calls.some(c => ['thread/turns/list', 'turn/start', 'turn/interrupt'].includes(c.method)), false);
});

test('startup races retain only the newly owned thread and bounded events', async t => {
  const rpc = fakeRPC();
  const human = randomUUID();
  rpc.hooks['thread/start'] = () => {
    rpc.emit('turn/started', { threadId: randomUUID(), turn: nativeTurn(randomUUID()) });
    rpc.emit('turn/started', { threadId: rpc.thread.id, turn: nativeTurn(human, 'inProgress', [userItem(null)]) });
    return { thread: rpc.thread };
  };
  const f = await fixture(t, { rpc });
  assert.equal((await f.get()).activeTurnId, human);
  assert.equal((await f.service.request('GET', '/v1/sessions')).sessions.length, 1);
});

test('encoded routing rejects unknown and malformed IDs without importing native threads', async t => {
  const f = await fixture(t);
  const calls = f.rpc.calls.length;
  for (const path of ['/v1/sessions/codex%3A' + randomUUID(), '/v1/sessions/' + randomUUID(),
    '/v1/sessions/codex%253A' + f.rpc.thread.id, '/v1/sessions/%ZZ', f.path + '/unknown']) {
    await assert.rejects(f.service.request('GET', path), error => [400, 404].includes(error.status));
  }
  await assert.rejects(f.service.request('DELETE', f.path), error => error.status === 405);
  await assert.rejects(f.service.request('GET', f.path + '/events?after=-1'), /cursor/);
  assert.equal(f.rpc.calls.length, calls);
  assert.equal((await f.service.request('GET', '/v1/sessions/codex:' + f.rpc.thread.id)).sessionId, f.session.sessionId);
});

test('oversized events and attribution buffers fault closed without leaking rejected content', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  delta(f, turnId, 'answer', 'x'.repeat(SESSION_LIMITS.eventBytes + 1));
  assert.equal((await f.get()).fault, true);
  assert.ok((await f.events()).every(e => Buffer.byteLength(JSON.stringify(e)) <= SESSION_LIMITS.eventBytes));
  own.pending.resolve({ turn: nativeTurn(turnId) });
  const g = await fixture(t);
  const unowned = randomUUID();
  turnStarted(g, unowned);
  for (let i = 0; i <= 128; i++) delta(g, unowned, 'answer', 'small');
  assert.equal((await g.get()).fault, true);
});

test('journal eviction respects byte budget and cursor expiry', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId)]);
  for (let i = 0; i < 90; i++) delta(f, turnId, 'answer', 'x'.repeat(100000));
  const session = await f.get();
  assert.ok(session.historyStartCursor > 0);
  await assert.rejects(f.events(), /expired/);
  const batch = await f.service.request('GET', f.path + '/events?after=' + session.historyStartCursor);
  assert.ok(batch.events.reduce((sum, e) => sum + Buffer.byteLength(JSON.stringify(e)), 0) <= SESSION_LIMITS.journalBytes);
  assert.equal(batch.events[0].seq, session.historyStartCursor + 1);
  own.pending.resolve({ turn: nativeTurn(turnId) });
});

test('event polling is abortable and bounded with injected timers, without cancelling native work', async t => {
  const scheduled = deferred();
  const timers = new Map();
  let next = 0;
  const f = await fixture(t, {
    setTimer(callback) { timers.set(++next, callback); scheduled.resolve(); return next; },
    clearTimer(id) { timers.delete(id); },
  });
  const cursor = (await f.get()).cursor;
  const abort = new AbortController();
  const polling = f.service.request('GET', f.path + `/events?after=${cursor}&wait=1`, undefined, abort.signal);
  await scheduled.promise;
  abort.abort();
  assert.deepEqual(await polling, { events: [], cursor });
  assert.equal(timers.size, 0);
  const timeout = f.service.request('GET', f.path + `/events?after=${cursor}&wait=1`);
  for (const callback of [...timers.values()]) callback();
  assert.deepEqual(await timeout, { events: [], cursor });
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
});

for (const version of ['0.163.0', '0.164.0-beta.1', '0.164.00']) test(`unsupported native version ${version} refuses mutations`, async t => {
  const rpc = fakeRPC();
  rpc.hooks.initialize = () => ({ userAgent: 'codex/' + version, platformFamily: 'unix', codexHome: '/home/user/.codex' });
  const service = await createCodexService({ rpcFactory: rpc.factory });
  t.after(() => service.close());
  await assert.rejects(service.request('POST', '/v1/sessions', { cwd: '/workspace' }), /protocol response/);
  assert.equal(rpc.calls.some(c => c.method === 'thread/start'), false);
});

test('persistence failure prevents accepting or sending a prompt', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  t.after(() => rm(dirname(sessionFile), { recursive: true, force: true }));
  await f.mode('automation');
  await writeFile(sessionFile, 'changed externally');
  await assert.rejects(f.send(), /persistence failed/);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/start'), false);
  // close reports the same sticky persistence failure, as the broker requires.
  await assert.rejects(f.service.close(), /persistence failed/);
  f.service.close = async () => {};
});

test('a turn/start response alone never proves ownership and missing clientId has a bounded deadline', async t => {
  const timers = new Map();
  const scheduled = deferred();
  let next = 0;
  const f = await fixture(t, {
    setTimer(callback) { timers.set(++next, callback); scheduled.resolve(); return next; },
    clearTimer(id) { timers.delete(id); },
  });
  await f.mode('automation');
  const body = { requestId: randomUUID(), text: 'hello' };
  await f.send(body);
  await scheduled.promise;
  await f.service.request('POST', f.path + '/cancel', { requestId: body.requestId });
  assert.equal((await f.events()).some(e => e.type === 'turn.start'), false);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
  for (const callback of [...timers.values()]) callback();
  assert.equal((await f.get()).archived, true);
  assert.equal((await f.send(body)).status, 'unknown');
  assert.equal(timers.size, 0);
});

test('native turn/start steering race never relabels or interrupts a human turn', async t => {
  const f = await fixture(t);
  const own = await startOwned(f);
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(null, 'human won the race')]);
  await f.service.request('POST', f.path + '/cancel', { requestId: own.body.requestId });
  own.pending.resolve({ turn: nativeTurn(turnId) });
  // The resolved native response is consumed before this request's continuation.
  await f.get();
  assert.equal((await f.get()).archived, true);
  assert.equal(f.rpc.calls.some(c => c.method === 'turn/interrupt'), false);
  assert.ok((await f.events()).filter(e => e.data.turnId === turnId).every(e => e.data.requestId === undefined));
});

test('global journal and session budgets stay bounded without importing unknown threads', async t => {
  const f = await fixture(t);
  const paths = [f.path];
  for (let n = 0; n < 5; n++) {
    if (n) {
      f.rpc.thread.id = randomUUID();
      const session = await f.service.request('POST', '/v1/sessions', { cwd: '/workspace' });
      paths.push('/v1/sessions/' + encodeURIComponent(session.sessionId));
    }
    const turnId = randomUUID();
    turnStarted(f, turnId, [userItem(null)]);
    for (let i = 0; i < 70; i++) delta(f, turnId, 'text', 'x'.repeat(100000));
    turnCompleted(f, turnId);
  }
  let bytes = 0;
  for (const path of paths) {
    const session = await f.service.request('GET', path);
    const batch = await f.service.request('GET', path + '/events?after=' + session.historyStartCursor);
    bytes += batch.events.reduce((sum, e) => sum + Buffer.byteLength(JSON.stringify(e)), 0);
  }
  assert.ok(bytes <= SESSION_LIMITS.totalJournalBytes);
  assert.ok((await f.service.request('GET', paths[0])).historyStartCursor > 0);
  for (let n = paths.length; n < SESSION_LIMITS.sessions; n++) {
    f.rpc.thread.id = randomUUID();
    await f.service.request('POST', '/v1/sessions', { cwd: '/workspace' });
  }
  const calls = f.rpc.calls.length;
  await assert.rejects(f.service.request('POST', '/v1/sessions', { cwd: '/workspace' }), /budget/);
  assert.equal(f.rpc.calls.length, calls);
  f.rpc.emit('turn/started', { threadId: randomUUID(), turn: nativeTurn(randomUUID()) });
  assert.equal((await f.service.request('GET', '/v1/sessions')).sessions.length, 100);
});

test('prompt tombstones survive journal eviction, restart and explicit rejoin without storing raw prompts', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  const own = await startOwned(f, { requestId: randomUUID(), text: 'private prompt must be evicted with its transcript' });
  const accepted = JSON.parse(await readFile(sessionFile, 'utf8')).sessions[0];
  const tombstone = { requestId: own.body.requestId,
    textHash: accepted.events.find(e => e.type === 'prompt.accepted').data.textHash, clientId: own.params.clientUserMessageId };
  assert.deepEqual(accepted.promptTombstones, [tombstone], 'ledger is durable before native submission');
  const turnId = randomUUID();
  turnStarted(f, turnId, [userItem(own.params.clientUserMessageId, own.body.text)]);
  for (let i = 0; i <= SESSION_LIMITS.events; i++) delta(f, turnId, 'answer', 'live');
  turnCompleted(f, turnId);
  own.pending.resolve({ turn: nativeTurn(turnId) });
  await f.service.close();
  const raw = await readFile(sessionFile, 'utf8');
  const saved = JSON.parse(raw).sessions[0];
  assert.ok(saved.historyStartCursor > 0);
  assert.equal(saved.events.some(e => e.type === 'prompt.accepted'), false);
  assert.deepEqual(saved.promptTombstones, [tombstone]);
  assert.equal(raw.includes(own.body.text), false);
  assert.deepEqual(await readdir(dirname(sessionFile)), ['codex-sessions.json']);
  const rpc = fakeRPC(f.rpc.thread);
  const restored = await createCodexService({ sessionFile, rpcFactory: rpc.factory });
  t.after(() => restored.close());
  t.after(() => rm(dirname(sessionFile), { recursive: true, force: true }));
  await restored.preparePersistence();
  await restored.request('POST', f.path + '/resume', {});
  await restored.request('POST', f.path + '/mode', { mode: 'automation' });
  assert.equal((await restored.request('POST', f.path + '/prompt', own.body)).status, 'unknown');
  await assert.rejects(restored.request('POST', f.path + '/prompt', { ...own.body, text: 'different' }), /different text/);
  assert.equal(rpc.calls.some(c => c.method === 'turn/start'), false);
});

for (const ledger of ['missing', 'full']) test(`${ledger} persisted prompt ledger refuses new submissions after Join`, async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  await f.service.close();
  const saved = JSON.parse(await readFile(sessionFile, 'utf8'));
  if (ledger === 'missing') delete saved.sessions[0].promptTombstones;
  else saved.sessions[0].promptTombstones = Array.from({ length: 1000 }, () => ({
    requestId: randomUUID(), clientId: randomUUID(), textHash: 'a'.repeat(64),
  }));
  await writeFile(sessionFile, JSON.stringify(saved) + '\n');
  const rpc = fakeRPC(f.rpc.thread);
  const restored = await createCodexService({ sessionFile, rpcFactory: rpc.factory });
  t.after(() => restored.close());
  t.after(() => rm(dirname(sessionFile), { recursive: true, force: true }));
  await restored.preparePersistence();
  await restored.request('POST', f.path + '/resume', {});
  await restored.request('POST', f.path + '/mode', { mode: 'automation' });
  await assert.rejects(restored.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'new work' }),
    ledger === 'missing' ? /prompt history is incomplete/ : /request budget exhausted/);
  assert.equal(rpc.calls.some(c => c.method === 'turn/start'), false);
  await restored.close();
  const record = JSON.parse(await readFile(sessionFile, 'utf8')).sessions[0];
  if (ledger === 'missing') assert.equal(Object.hasOwn(record, 'promptTombstones'), false, 'restarting again must not invent a complete ledger');
  else assert.deepEqual(record.promptTombstones, saved.sessions[0].promptTombstones);
});

import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { chmod, mkdtemp, readFile, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import test from 'node:test';
import { createOpenCodeService } from '../src/opencode-service.mjs';
import { isOpenCodeSessionId, OpenCodeRPCError } from '../src/opencode-rpc.mjs';
import { openSessionStore, SESSION_LIMITS } from '../src/session-store.mjs';

const nativeId = prefix => prefix + randomUUID().replaceAll('-', '').slice(0, 26);
const user = (text, id = nativeId('msg_')) => ({ id, type: 'user', text, time: { created: 1 } });
const assistant = (content, id = nativeId('msg_')) => ({ id, type: 'assistant', content, time: { created: 2 } });
const idle = (outcome = 'succeeded') => ({ id: nativeId('msg_'), type: 'idle', outcome, time: { created: 3 } });
function deferred() {
  let resolve, reject;
  const promise = new Promise((a, b) => { resolve = a; reject = b; });
  return { promise, resolve, reject };
}
function fakeClient() {
  const f = {
    info: { id: nativeId('ses_'), location: { directory: '/workspace' }, time: { created: 1234, updated: 1234 } },
    calls: [], hooks: new Map(), messages: [], inbox: [], active: {}, permissions: [], forms: [], shells: [],
    version: '2.0.28', factories: 0, closes: 0,
  };
  f.factory = async callbacks => {
    f.factories++; f.callbacks = callbacks;
    const closed = deferred();
    return {
      async request(method, path, body) {
        f.calls.push({ method, path, body });
        const result = () => {
          const hook = f.hooks.get(method + ' ' + path);
          if (hook) return hook(body);
          if (path === '/api/info') return { version: f.version };
          if (path === '/api/session/active') return { data: structuredClone(f.active) };
          if (path.startsWith('/api/shell?')) return { location: f.info.location, data: structuredClone(f.shells) };
          if (path === '/api/session' || path === '/api/session/' + f.info.id) return { data: structuredClone(f.info) };
          if (path.includes('/message?')) {
            const query = new URL(path, 'http://localhost').searchParams;
            const cursor = query.get('cursor');
            let rows;
            if (cursor) {
              // Native cursors carry direction; combining cursor/order is invalid.
              assert.equal(query.has('order'), false);
              assert.equal(query.get('limit'), '1');
              const index = f.messages.findIndex(item => item.id === cursor);
              assert.ok(index >= 0);
              rows = f.messages.slice(Math.max(0, index - 1), index);
            } else {
              // Installed 2.0.28 rejects limits above 200 before reading history.
              assert.ok(path.endsWith('/message?limit=200&order=desc'));
              rows = f.messages.slice(-200);
            }
            const data = structuredClone(rows.toReversed());
            // Boundary cursors exist on every nonempty page, including the last.
            return { data, cursor: { previous: data[0]?.id ?? null, next: data.at(-1)?.id ?? null } };
          }
          if (path.endsWith('/inbox') && method === 'GET') return { data: structuredClone(f.inbox) };
          if (path.endsWith('/permission')) return { data: structuredClone(f.permissions) };
          if (path.endsWith('/form')) return { data: structuredClone(f.forms) };
          if (path.endsWith('/prompt')) {
            const input = { id: body.id, sessionID: f.info.id, type: 'user', payload: { text: body.text }, delivery: body.delivery };
            f.inbox.push(input);
            return { data: input };
          }
          if (method === 'DELETE' && path.includes('/inbox/')) {
            const id = path.split('/').at(-1);
            const index = f.inbox.findIndex(item => item.id === id);
            if (index >= 0) { f.inbox.splice(index, 1); f.emit('session.inbox.cancelled', { inboxID: id }); }
            return;
          }
          assert.fail('Unexpected call ' + method + ' ' + path);
        };
        return Promise.race([Promise.resolve().then(result), closed.promise.then(() => { throw new Error('private transport diagnostic'); })]);
      },
      async close() { f.closes++; closed.resolve(); },
    };
  };
  f.emit = (type, data = {}, other = {}) => f.callbacks.onEvent({ id: nativeId('evt_'), type, data: { sessionID: f.info.id, ...data }, ...other });
  f.disconnect = () => f.callbacks.onDisconnect(new Error('secret credential'));
  f.deliver = (id = f.inbox[0]?.id) => {
    const index = f.inbox.findIndex(item => item.id === id);
    const input = f.inbox.splice(index, 1)[0];
    f.messages.push(user(input.payload.text, id));
    f.emit('session.inbox.delivered', { inboxID: id });
    return id;
  };
  return f;
}
const fixtures = new WeakMap();
async function fixture(t, options = {}) {
  const f = options.native ?? fakeClient();
  const timers = new Map();
  const service = await createOpenCodeService({
    clientFactory: f.factory, now: () => 2345,
    setTimer(fn, delay) { const key = Symbol(); timers.set(key, { fn, delay }); return key; },
    clearTimer(key) { timers.delete(key); }, ...options,
  });
  const owned = fixtures.get(t) ?? [];
  owned.push(service); fixtures.set(t, owned);
  t.after(async () => { await service.close(); });
  await service.preparePersistence();
  const session = options.restore ? (await service.request('GET', '/v1/sessions')).sessions[0]
    : await service.request('POST', '/v1/sessions', { cwd: '/workspace', permissions: ['bypass'], mcpServers: ['ignored'] });
  const path = '/v1/sessions/' + encodeURIComponent(session.sessionId);
  return {
    native: f, service, session, path, timers,
    get: () => service.request('GET', path),
    events: async () => (await service.request('GET', path + '/events')).events,
    mode: mode => service.request('POST', path + '/mode', { mode }),
    send: (body = { text: 'hello', requestId: randomUUID() }) => service.request('POST', path + '/prompt', body),
    resume: () => service.request('POST', path + '/resume', {}),
    cancel: requestId => service.request('POST', path + '/cancel', { requestId }),
    async tick() {
      const pending = [...timers.entries()].filter(([, value]) => value.delay !== 15000 && value.delay !== 10000);
      for (const [key, value] of pending) { timers.delete(key); await value.fn(); }
    },
  };
}
async function privateFile(t) {
  const dir = await mkdtemp('/tmp/opencode/opencode-service-');
  t.after(async () => {
    for (const service of fixtures.get(t) ?? []) await service.close();
    await rm(dir, { recursive: true, force: true });
  });
  return join(dir, 'opencode-sessions.json');
}
async function send(f, text = 'hello') {
  const body = { text, requestId: randomUUID() };
  await f.mode('automation');
  await f.send(body);
  const call = f.native.calls.findLast(call => call.path.endsWith('/prompt'));
  return { ...body, id: call.body.id };
}
const mutations = f => f.native.calls.filter(call => call.method !== 'GET');

test('construction/list/archive replay are passive and creation preserves native IDs and default configuration', async t => {
  const sessionFile = await privateFile(t);
  const native = fakeClient();
  const service = await createOpenCodeService({ sessionFile, clientFactory: native.factory });
  await assert.rejects(service.request('GET', '/v1/sessions'), /preparation/);
  await service.preparePersistence();
  assert.deepEqual(await service.request('GET', '/v1/sessions'), { sessions: [] });
  assert.equal(native.factories, 0);
  await service.close();
  const f = await fixture(t, { sessionFile, native });
  assert.equal(f.session.harness, 'opencode');
  assert.equal(f.session.sessionId, 'opencode:' + native.info.id);
  assert.equal(f.session.nativeSessionId, native.info.id);
  assert.equal(f.session.mode, 'human');
  assert.equal(f.session.version, '2.0.28');
  assert.deepEqual(mutations(f), [{ method: 'POST', path: '/api/session', body: { location: { directory: '/workspace' } } }]);
  const saved = JSON.parse(await readFile(sessionFile, 'utf8')).sessions[0];
  assert.equal(saved.sessionId, native.info.id);
  assert.equal(saved.events[0].data.sessionId, native.info.id);
  assert.equal((await f.events())[0].data.sessionId, f.session.sessionId);
  assert.equal((await stat(sessionFile)).mode & 0o777, 0o600);
});

test('normal structured snapshot flow separates thoughts/tools and completes exactly the owned request', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([
    { type: 'reasoning', text: 'thinking' }, { type: 'text', text: 'answer' },
    { type: 'tool', id: 'tool1', name: 'shell', state: { status: 'running', input: { command: 'pwd' } } },
  ]));
  await f.tick();
  const reply = f.native.messages.at(-1);
  reply.content[1].text += ' more';
  reply.content[2].state = { status: 'completed', input: {}, content: [{ type: 'text', text: '/workspace' }] };
  f.native.emit('session.text.delta', {}, { durable: { seq: 987654321 } });
  await f.tick();
  f.native.messages.push(idle());
  await f.tick();
  await f.tick();
  const events = (await f.events()).filter(event => event.data.turnId === own.id);
  assert.deepEqual(events.map(event => event.type), ['turn.start', 'thought', 'text', 'tool.start', 'text', 'tool.end', 'turn.complete']);
  assert.ok(events.every(event => event.data.requestId === own.requestId));
  assert.equal(events.at(-1).data.reason, 'answer');
  assert.equal(events.at(-1).data.answer, '');
  assert.equal((await f.send(own)).status, 'completed');
  assert.equal((await f.get()).activeTurnId, null);
  assert.ok(events.at(-1).seq < 987654321, 'broker sequence is independent from native durable sequence');
  await assert.rejects(f.send({ ...own, text: 'different' }), /different text/);
  assert.equal(mutations(f).filter(call => call.path.endsWith('/prompt')).length, 1);
});

test('streaming tool arguments are replaced by finalized input at completion', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  const part = { type: 'tool', id: 'tool1', name: 'shell', state: { status: 'streaming', input: '{"command":' } };
  f.native.messages.push(assistant([part]));
  await f.tick();
  const input = { command: 'pwd' };
  part.state = { status: 'running', input, metadata: {} };
  await f.tick();
  part.state = { status: 'completed', input, content: [{ type: 'text', text: '/workspace' }] };
  f.native.messages.push(idle());
  await f.tick();
  await f.tick();
  const events = (await f.events()).filter(event => event.type.startsWith('tool.'));
  assert.deepEqual(events.map(event => event.type), ['tool.start', 'tool.end']);
  assert.equal(events[0].data.input, '{"command":');
  assert.deepEqual(events[1].data.input, input);
  assert.ok(events.every(event => event.data.requestId === own.requestId));
  assert.equal((await f.get()).fault, false);
});

test('standalone synthetic native work survives partial-history restoration without resubmission', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  await f.mode('automation');
  const synthetic = { ...user('native continuation'), type: 'synthetic' };
  f.native.messages.push(synthetic, assistant([{ type: 'text', text: 'before' }]));
  await f.tick();
  assert.equal((await f.get()).fault, false);
  assert.equal((await f.get()).mode, 'human');
  assert.equal((await f.get()).activeTurnId, synthetic.id);
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  await f.service.close();
  const native = fakeClient();
  native.info = f.native.info;
  native.messages = structuredClone(f.native.messages);
  native.messages.at(-1).content[0].text += ' after';
  const restored = await fixture(t, { native, sessionFile, restore: true });
  await restored.resume();
  assert.equal((await restored.get()).mode, 'human');
  native.messages.push(idle());
  await restored.tick();
  const events = (await restored.events()).filter(event => event.data.turnId === synthetic.id);
  assert.ok(events.every(event => event.data.requestId === undefined));
  assert.deepEqual(events.filter(event => event.type === 'text').map(event => event.data.text), ['before', ' after']);
  assert.deepEqual(events.filter(event => event.type === 'turn.start').map(event => [event.data.nativeMessageType, event.data.text]),
    [['synthetic', 'native continuation'], ['synthetic', '']]);
  assert.equal(events.filter(event => event.type === 'turn.complete').length, 1);
  assert.equal(mutations(f).length, 1);
  assert.equal(mutations(restored).length, 0);
});

for (const change of ['text', 'type', 'missing']) test(`cold resume rejects ${change} changes to a persisted synthetic receipt`, async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  f.native.messages.push({ ...user('native continuation'), type: 'synthetic' }, assistant([{ type: 'text', text: 'before' }]));
  await f.tick();
  assert.equal((await f.get()).fault, false);
  await f.service.close();
  const native = fakeClient();
  native.info = f.native.info;
  native.messages = structuredClone(f.native.messages);
  if (change === 'text') native.messages[0].text = 'rewritten';
  if (change === 'type') native.messages[0].type = 'user';
  if (change === 'missing') native.messages.shift();
  const restored = await fixture(t, { native, sessionFile, restore: true });
  await assert.rejects(restored.resume());
  assert.equal((await restored.get()).fault, true);
  assert.deepEqual((await restored.events()).filter(event => event.type === 'text').map(event => event.data.text), ['before']);
  assert.equal(mutations(restored).length, 0);
});

test('large Unicode user text, answers and thoughts preserve characters within the event budget', async t => {
  const f = await fixture(t);
  const text = 'x'.repeat(15999) + '🌏'.repeat(40000);
  const own = await send(f, text);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text }, { type: 'reasoning', text }]), idle());
  await f.tick();
  assert.equal((await f.get()).fault, false);
  const events = (await f.events()).filter(event => event.data.turnId === own.id);
  for (const type of ['turn.start', 'text', 'thought']) {
    const chunks = events.filter(event => event.type === type);
    assert.ok(chunks.length > 1);
    assert.equal(chunks.map(event => event.data.text).join(''), text);
    assert.ok(chunks.every(event => event.data.text.isWellFormed()));
  }
  assert.ok(events.every(event => Buffer.byteLength(JSON.stringify(event)) <= SESSION_LIMITS.eventBytes));
  assert.equal((await f.send(own)).status, 'completed');
});

for (const type of ['user', 'synthetic']) test(`background ${type} messages are unowned and never cross-complete request IDs`, async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text: 'own answer' }]));
  await f.tick();
  const human = { ...user('human steering'), type };
  f.native.messages.push(human, assistant([{ type: 'text', text: 'human answer' }]));
  f.native.emit('session.inbox.delivered');
  await f.tick();
  assert.equal((await f.get()).mode, 'human');
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  f.native.messages.push(idle());
  await f.tick();
  const events = await f.events();
  assert.equal(events.find(event => event.type === 'text' && event.data.text === 'human answer').data.requestId, undefined);
  assert.equal(events.filter(event => event.type === 'turn.complete' && event.data.requestId === own.requestId).length, 1);
  assert.equal(events.find(event => event.type === 'turn.complete' && event.data.turnId === human.id).data.requestId, undefined);
  assert.equal(mutations(f).filter(call => call.path.endsWith('/prompt')).length, 1);
});

test('Open/Observe mode changes and unsupported stop never interrupt or submit native work', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  await f.tick();
  const before = mutations(f).length;
  await f.mode('human');
  await f.mode('automation');
  await assert.rejects(f.service.request('POST', f.path + '/stop', {}), /not supported/);
  assert.equal((await f.get()).activeTurnId, own.id);
  assert.equal(mutations(f).length, before);
  await f.service.close();
  assert.equal(mutations(f).length, before);
});

test('native permission and form approvals remain unanswered and notifications resolve from native state only', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.permissions.push({ id: 'per_one', sessionID: f.native.info.id, message: 'private permission details' });
  f.native.forms.push({ id: 'frm_one', sessionID: f.native.info.id, title: 'private form details' });
  await f.tick();
  await f.tick();
  const needs = (await f.events()).filter(event => event.type === 'needs.input');
  assert.deepEqual(needs.map(event => event.data.kind), ['permission', 'form']);
  assert.ok(needs.every(event => event.data.requestId === own.requestId));
  f.native.permissions = [];
  await f.tick();
  assert.equal((await f.events()).at(-1).data.pending, true);
  f.native.forms = [];
  await f.tick();
  assert.equal((await f.events()).at(-1).data.pending, false);
  assert.equal(JSON.stringify(await f.events()).includes('private'), false);
  assert.equal(mutations(f).length, 2);
});

for (const outcome of ['succeeded', 'failed', 'interrupted']) test(`native idle ${outcome} is the only completion authority`, async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push({ ...assistant([{ type: 'text', text: 'answer' }]), finish: 'stop' });
  await f.tick();
  assert.equal((await f.events()).some(event => event.type === 'turn.complete'), false);
  f.native.messages.push({ ...idle(outcome), error: { message: 'secret error' } });
  await f.tick();
  const event = (await f.events()).at(-1);
  assert.equal(event.type, 'turn.complete');
  assert.equal(event.data.requestId, own.requestId);
  assert.equal(event.data.reason, { succeeded: 'answer', failed: 'error', interrupted: 'aborted' }[outcome]);
  assert.equal(JSON.stringify(await f.get()).includes('secret'), false);
});

test('queued cancellation deletes only its owned native inbox ID, and native cancellation settles the request', async t => {
  const f = await fixture(t);
  const own = await send(f);
  await f.cancel(own.requestId);
  await f.cancel(own.requestId);
  const deletes = mutations(f).filter(call => call.method === 'DELETE');
  assert.deepEqual(deletes, [{ method: 'DELETE', path: '/api/session/' + f.native.info.id + '/inbox/' + own.id, body: undefined }]);
  assert.equal((await f.events()).at(-1).data.cancelled, true);
  await assert.rejects(f.cancel(randomUUID()), /Unknown/);
  assert.equal(mutations(f).some(call => call.path.includes('interrupt')), false);
});

test('cancel refuses running ownership and a queue-to-running race never interrupts human work', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.hooks.set('DELETE /api/session/' + f.native.info.id + '/inbox/' + own.id, () => {
    f.native.deliver(own.id);
    f.native.messages.push(user('human work'));
  });
  await assert.rejects(f.cancel(own.requestId), /already delivered/);
  assert.equal((await f.get()).mode, 'human');
  await assert.rejects(f.service.request('POST', f.path + '/cancel', { requestId: own.requestId, turnId: 'wrong' }), /does not match/);
  assert.equal(mutations(f).some(call => call.path.includes('interrupt')), false);
  const g = await fixture(t);
  const other = await send(g);
  g.native.deliver(other.id);
  await g.tick();
  await assert.rejects(g.cancel(other.requestId), /cannot atomically cancel/);
  assert.equal(mutations(g).some(call => call.method === 'DELETE'), false);
});

test('tombstone is fsynced before POST and response loss is never retried, including restart', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  const body = { requestId: randomUUID(), text: 'a private prompt' };
  let posts = 0;
  f.native.hooks.set('POST /api/session/' + f.native.info.id + '/prompt', async input => {
    posts++;
    const saved = JSON.parse(await readFile(sessionFile, 'utf8')).sessions[0];
    assert.equal(saved.promptTombstones[0].requestId, body.requestId);
    assert.equal(saved.promptTombstones[0].textHash.length, 64);
    assert.equal(input.delivery, 'queue');
    assert.equal(JSON.stringify(saved.promptTombstones).includes(body.text), false);
    throw new Error('secret error');
  });
  await f.mode('automation');
  await f.send(body);
  // Wait for the asynchronous admission result through an event long poll.
  const after = (await f.get()).cursor;
  if (!(await f.get()).archived) await f.service.request('GET', f.path + '/events?after=' + after + '&wait=1');
  assert.equal((await f.send(body)).status, 'unknown');
  assert.equal(posts, 1);
  assert.equal((await f.get()).fault, true);
  await f.service.close();
  const restored = await fixture(t, { sessionFile, native: fakeClient(), restore: true });
  assert.equal(restored.native.factories, 0);
  assert.equal((await restored.send(body)).status, 'unknown');
  assert.equal(restored.native.calls.length, 0);
});

test('resume is read-only, preserves identity/history and deduplicates partial output across restart', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text: 'before' }, { type: 'reasoning', text: 'thought' }]));
  await f.tick();
  await f.service.close();
  const native = fakeClient();
  native.info = f.native.info;
  native.messages = structuredClone(f.native.messages);
  native.messages.at(-1).content[0].text += ' after';
  native.active[native.info.id] = { type: 'running' };
  const restored = await fixture(t, { native, sessionFile, restore: true });
  assert.equal(restored.native.factories, 0);
  const before = await restored.get();
  await restored.resume();
  const resumed = await restored.get();
  assert.equal(resumed.sessionId, f.session.sessionId);
  assert.equal(resumed.runtimeId, before.runtimeId);
  assert.equal(resumed.startedAt, before.startedAt);
  assert.equal(resumed.mode, 'human');
  assert.equal(resumed.archived, false);
  assert.ok(resumed.runtimeStartCursor > before.cursor);
  const resumedEvents = await restored.events();
  assert.deepEqual(resumedEvents.filter(e => e.type === 'text').map(e => e.data.text), ['before', ' after']);
  const boundary = resumedEvents.findIndex(e => e.type === 'runtime.resumed');
  assert.deepEqual(resumedEvents.slice(boundary).map(e => e.type), ['runtime.resumed', 'turn.start', 'text']);
  assert.equal(resumedEvents[boundary + 1].data.text, '', 'establish the continued turn without duplicating the historical prompt');
  assert.equal(resumedEvents[boundary + 1].data.requestId, own.requestId);
  assert.equal(mutations(restored).length, 0);
  await restored.mode('automation');
  await assert.rejects(restored.send(), /busy/);
  native.messages.push(idle()); native.active = {};
  await restored.tick();
  assert.equal((await restored.events()).filter(e => e.type === 'turn.complete' && e.data.requestId === own.requestId).length, 1);
  assert.equal((await restored.send(own)).status, 'completed');
});

for (const work of ['queue', 'active', 'shell', 'permission', 'form']) test(`restore checks ${work} without waking native work`, async t => {
  const f = await fixture(t);
  f.native.disconnect();
  const input = { id: nativeId('msg_'), sessionID: f.native.info.id, type: 'user', payload: { text: 'native work' } };
  if (work === 'queue') f.native.inbox.push(input);
  if (work === 'active') f.native.active[f.native.info.id] = { type: 'running' };
  if (work === 'shell') f.native.shells.push({ id: 'sh_native', status: 'running' });
  if (work === 'permission') f.native.permissions.push({ id: 'per_native', sessionID: f.native.info.id });
  if (work === 'form') f.native.forms.push({ id: 'frm_native', sessionID: f.native.info.id });
  const before = mutations(f).length;
  await f.resume();
  assert.equal((await f.get()).archived, false);
  await f.mode('automation');
  await assert.rejects(f.send(), /busy/);
  assert.equal(mutations(f).length, before);
});

for (const bad of ['wrong ID', 'wrong directory', 'native archive', 'queue unreadable', 'missing anchor', 'rewritten text', 'rewritten prompt', 'reordered history']) test(`restore fails closed for ${bad} without inference`, async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text: 'original' }]));
  await f.tick();
  if (bad === 'missing anchor') { f.native.messages.push(idle()); await f.tick(); }
  f.native.disconnect();
  if (bad === 'wrong ID') f.native.hooks.set('GET /api/session/' + f.native.info.id, () => ({ data: { ...f.native.info, id: nativeId('ses_') } }));
  if (bad === 'wrong directory') f.native.info.location.directory = '/different';
  if (bad === 'native archive') f.native.info.time.archived = 1234;
  if (bad === 'queue unreadable') f.native.hooks.set('GET /api/session/' + f.native.info.id + '/inbox', () => { throw new Error('secret'); });
  if (bad === 'missing anchor') f.native.messages = [];
  if (bad === 'rewritten text') f.native.messages.at(-1).content[0].text = 'modified';
  if (bad === 'rewritten prompt') f.native.messages[0].text = 'modified';
  if (bad === 'reordered history') f.native.messages.splice(1, 0, user('inserted before the observed answer'));
  const before = mutations(f).length;
  await assert.rejects(f.resume());
  assert.equal((await f.get()).archived, true);
  assert.equal((await f.get()).mode, 'human');
  assert.equal(mutations(f).length, before);
});

test('transport loss faults pending work and passive reads never reconnect or leak transport errors', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.disconnect();
  assert.equal((await f.get()).archived, true);
  assert.equal((await f.get()).fault, true);
  assert.equal((await f.send(own)).status, 'unknown');
  await assert.rejects(f.send(), /read-only/);
  await f.events();
  assert.equal(f.native.factories, 1);
  assert.equal(JSON.stringify(await f.get()).includes('secret'), false);
  await f.resume();
  assert.equal(f.native.factories, 2);
  assert.equal(mutations(f).filter(call => call.path.endsWith('/prompt')).length, 1);
});

test('definite native prompt rejection is tombstoned without a stream fault', async t => {
  const f = await fixture(t);
  f.native.hooks.set('POST /api/session/' + f.native.info.id + '/prompt', () => { throw new OpenCodeRPCError('safe', { rejected: true }); });
  const own = await send(f);
  assert.equal((await f.send(own)).status, 'rejected');
  assert.equal((await f.get()).archived, false);
  assert.equal((await f.events()).at(-1).type, 'prompt.rejected');
});

test('strict IDs and ownership routing reject malformed/unowned sessions without native calls', async t => {
  const f = await fixture(t);
  assert.equal(isOpenCodeSessionId('ses_' + 'a'.repeat(26)), true);
  const before = f.native.calls.length;
  for (const id of ['ses_x', 'ses_' + 'A'.repeat(26), 'ses_' + 'a'.repeat(27), randomUUID(), 'opencode:' + nativeId('ses_'), 'opencode:ses_/../']) {
    await assert.rejects(f.service.request('GET', '/v1/sessions/' + encodeURIComponent(id)));
  }
  assert.equal(f.native.calls.length, before);
  assert.equal((await f.service.request('GET', '/v1/sessions')).sessions.length, 1);
  f.native.emit('session.inbox.enqueued', { sessionID: nativeId('ses_') });
  assert.equal((await f.service.request('GET', '/v1/sessions')).sessions.length, 1);
});

for (const version of ['2.0.27', '2.0.280', '2.0.28-beta']) test(`version ${version} cannot create native sessions`, async t => {
  const native = fakeClient(); native.version = version;
  const service = await createOpenCodeService({ clientFactory: native.factory });
  t.after(() => service.close());
  await assert.rejects(service.request('POST', '/v1/sessions', { cwd: '/workspace' }));
  assert.equal(native.calls.some(call => call.method !== 'GET'), false);
});

test('cold resume recovers a completed native prompt with boundary cursors but no older data', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text: 'OPENCODE_READY' }]), idle());
  // Transport loss before the first delivered-message snapshot retains only the tombstone.
  f.native.disconnect();
  await f.service.close();
  const native = fakeClient();
  native.info = f.native.info;
  native.messages = structuredClone(f.native.messages);
  const restored = await fixture(t, { native, sessionFile, restore: true });
  await restored.resume();
  await restored.tick();
  const events = (await restored.events()).filter(event => event.data.requestId === own.requestId);
  assert.deepEqual(events.filter(event => event.data.turnId).map(event => event.type), ['turn.start', 'text', 'turn.complete']);
  assert.equal(events.find(event => event.type === 'text').data.text, 'OPENCODE_READY');
  assert.equal((await restored.get()).archived, false);
  assert.equal((await restored.send(own)).status, 'completed');
  assert.equal(mutations(restored).length, 0);
  assert.equal(native.calls.filter(call => call.path.includes('&cursor=')).length, 1);
});

test('a retained idle anchor skips older pages even when history exceeds the page limit', async t => {
  const f = await fixture(t);
  f.native.messages.push(user('previous turn'), assistant([{ type: 'text', text: 'previous output' }]), idle());
  await f.tick();
  const probes = f.native.calls.filter(call => call.path.includes('&cursor=')).length;
  // The old idle anchor remains visible but its already-sealed input falls off the page.
  f.native.messages.push(user('current turn'), ...Array.from({ length: 197 }, () => assistant([{ type: 'text', text: 'new output' }])), idle());
  await f.tick();
  assert.equal((await f.get()).fault, false);
  assert.equal(f.native.calls.filter(call => call.path.includes('&cursor=')).length, probes);
  const events = await f.events();
  assert.equal(events.filter(event => event.type === 'turn.complete').length, 2);
  assert.equal(events.filter(event => event.type === 'text' && event.data.text === 'previous output').length, 1);
});

test('the older-page probe percent-encodes opaque native cursors without adding order', async t => {
  const f = await fixture(t);
  const cursor = 'opaque+boundary/with=&fields?';
  const root = '/api/session/' + f.native.info.id + '/message';
  f.native.hooks.set('GET ' + root + '?limit=200&order=desc', () => ({
    data: [idle(), assistant([{ type: 'text', text: 'answer' }]), user('input')], cursor: { previous: cursor, next: cursor },
  }));
  f.native.hooks.set('GET ' + root + '?limit=1&cursor=' + encodeURIComponent(cursor), () => ({ data: [], cursor: {} }));
  await f.tick();
  assert.equal((await f.get()).fault, false);
  assert.equal((await f.events()).filter(event => event.type === 'turn.complete').length, 1);
});

for (const count of [200, 201]) test(`native history boundary probe distinguishes ${count} messages`, async t => {
  const f = await fixture(t);
  f.native.messages = [user('native work'), ...Array.from({ length: count - 2 }, () => assistant([{ type: 'text', text: 'answer' }])), idle()];
  await f.tick();
  assert.equal((await f.get()).fault, count > 200);
  assert.equal((await f.events()).some(event => event.type === 'turn.complete'), count === 200);
  assert.equal(f.native.calls.filter(call => call.path.includes('&cursor=')).length, 1);
});

for (const failure of ['unreadable', 'malformed', 'nonempty', 'invalid cursor']) test(`older history ${failure} fails closed before delivering output`, async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text: 'answer' }]), idle());
  const path = '/api/session/' + f.native.info.id + '/message?limit=1&cursor=' + encodeURIComponent(own.id);
  f.native.hooks.set('GET ' + path, () => {
    if (failure === 'unreadable') throw new Error('private failure');
    if (failure === 'malformed') return { data: [], cursor: { next: 42 } };
    if (failure === 'nonempty') return { data: [user('older input')], cursor: {} };
    return { data: [], cursor: { next: 'still more' } };
  });
  await f.tick();
  assert.equal((await f.get()).fault, true);
  assert.equal((await f.events()).some(event => event.type === 'turn.start'), false);
  assert.equal(f.native.calls.filter(call => call.path.includes('&cursor=')).length, 1);
  assert.equal(mutations(f).length, 2);
});

test('bounded history and content changes fail closed rather than misattribute output', async t => {
  const f = await fixture(t);
  f.native.messages = Array.from({ length: 201 }, () => user('too much history'));
  await f.tick();
  assert.equal((await f.get()).fault, true);
  assert.equal((await f.events()).some(event => event.type === 'turn.start'), false);
  const g = await fixture(t);
  g.native.messages.push(assistant([{ type: 'text', text: 'unowned answer' }]));
  await g.tick();
  assert.equal((await g.get()).fault, true);
});

test('native response and broker event budgets remain bounded for an oversized open turn', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.deliver(own.id);
  const content = Array.from({ length: 150 }, () => ({ type: 'text', text: 'x'.repeat(70000) }));
  // Separate snapshots stay inside the native response budget while growing the journal.
  for (let index = 0; index < 5; index++) {
    f.native.messages.push(assistant(content.slice(0, 30)));
    await f.tick();
    // Native page budget is intentionally strict; a growing open turn eventually faults.
    if ((await f.get()).fault) break;
  }
  const session = await f.get();
  assert.equal(session.fault, true);
  const batch = await f.service.request('GET', f.path + '/events?after=' + session.historyStartCursor);
  assert.ok(batch.events.reduce((sum, event) => sum + Buffer.byteLength(JSON.stringify(event)), 0) <= SESSION_LIMITS.journalBytes);
});

test('retained journal evicts old events with an explicit cursor gap and keeps request tombstones', async t => {
  const f = await fixture(t);
  const own = await send(f);
  for (let i = 0; i <= SESSION_LIMITS.events; i++) await f.mode(i % 2 ? 'automation' : 'human');
  const session = await f.get();
  assert.ok(session.historyStartCursor > 0);
  await assert.rejects(f.events(), /expired/);
  const batch = await f.service.request('GET', f.path + '/events?after=' + session.historyStartCursor);
  assert.equal(batch.events[0].seq, session.historyStartCursor + 1);
  assert.ok(batch.events.length <= SESSION_LIMITS.events);
  assert.equal((await f.send(own)).status, 'queued');
  assert.equal(mutations(f).filter(call => call.path.endsWith('/prompt')).length, 1);
});

test('Observe remains automation while previously observed native queued work is still pending', async t => {
  const f = await fixture(t);
  f.native.inbox.push({ id: nativeId('msg_'), sessionID: f.native.info.id, type: 'user', payload: { text: 'human queued work' } });
  await f.tick();
  await f.mode('automation');
  await f.tick();
  assert.equal((await f.get()).mode, 'automation');
  await assert.rejects(f.send(), /busy/);
  assert.equal(mutations(f).length, 1);
});

test('event waiting is abortable without stopping native work', async t => {
  const f = await fixture(t);
  const cursor = (await f.get()).cursor;
  const controller = new AbortController();
  const waiting = f.service.request('GET', f.path + '/events?after=' + cursor + '&wait=1', undefined, controller.signal);
  controller.abort();
  assert.deepEqual(await waiting, { cursor, events: [] });
  assert.equal(mutations(f).length, 1);
  assert.equal([...f.timers.values()].some(timer => timer.delay === 15000), false);
});

test('store validator is opt-in and UUID-only Claude/Codex defaults remain unchanged', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  await f.service.close();
  await assert.rejects(openSessionStore(sessionFile), /corrupt archive/);
  const nativeStore = await openSessionStore(sessionFile, { validateSessionId: isOpenCodeSessionId });
  assert.equal(nativeStore.records[0].sessionId, f.native.info.id);
});

for (const corrupt of ['json', 'ID', 'receipt', 'permissions', 'symlink']) test(`corrupt or unsafe ${corrupt} state is never repaired or resumed`, async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  await f.service.close();
  const data = JSON.parse(await readFile(sessionFile, 'utf8'));
  if (corrupt === 'json') await writeFile(sessionFile, '{');
  if (corrupt === 'ID') { data.sessions[0].sessionId = randomUUID(); await writeFile(sessionFile, JSON.stringify(data)); }
  if (corrupt === 'receipt') {
    data.sessions[0].events[0].data.nativeMessageId = 'bad';
    await writeFile(sessionFile, JSON.stringify(data));
  }
  if (corrupt === 'permissions') await chmod(sessionFile, 0o644);
  if (corrupt === 'symlink') { await rm(sessionFile); await symlink('/etc/passwd', sessionFile); }
  const native = fakeClient();
  await assert.rejects(createOpenCodeService({ sessionFile, clientFactory: native.factory }));
  assert.equal(native.factories, 0);
});

test('SSE bursts coalesce without postponing a scheduled native snapshot', async t => {
  const f = await fixture(t);
  f.native.messages.push(user('background native input'), assistant([{ type: 'text', text: 'visible' }]));
  f.native.emit('session.text.delta');
  const timer = [...f.timers.keys()][0];
  for (let i = 0; i < 100; i++) f.native.emit('session.text.delta');
  assert.equal([...f.timers.keys()][0], timer);
  assert.equal(f.timers.size, 1);
  await f.tick();
  assert.equal((await f.events()).at(-1).data.text, 'visible');
});

test('native delivery before the prompt response is attributed by preallocated message ID, never the human turn', async t => {
  const f = await fixture(t);
  const pending = deferred();
  let admitted;
  f.native.hooks.set('POST /api/session/' + f.native.info.id + '/prompt', input => {
    admitted = input;
    f.native.messages.push(user('human wins first'), assistant([{ type: 'text', text: 'human response' }]), idle(),
      user(input.text, input.id), assistant([{ type: 'text', text: 'owned response' }]), idle());
    return pending.promise;
  });
  const own = await send(f);
  await f.tick();
  assert.equal((await f.send(own)).status, 'completed');
  pending.resolve({ data: { id: admitted.id, sessionID: f.native.info.id, type: 'user' } });
  await f.get();
  const events = await f.events();
  assert.equal(events.find(e => e.type === 'text' && e.data.text === 'human response').data.requestId, undefined);
  assert.equal(events.find(e => e.type === 'text' && e.data.text === 'owned response').data.requestId, own.requestId);
  assert.equal(events.filter(e => e.type === 'turn.complete' && e.data.requestId === own.requestId).length, 1);
});

for (const ledger of ['missing', 'full', 'evicted transcript']) test(`${ledger} ledger cannot replay a request after restart`, async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  const own = await send(f);
  await f.service.close();
  const data = JSON.parse(await readFile(sessionFile, 'utf8'));
  const record = data.sessions[0];
  if (ledger === 'missing') delete record.promptTombstones;
  if (ledger === 'full') record.promptTombstones = Array.from({ length: 1000 }, () => ({ requestId: randomUUID(), clientId: randomUUID(), textHash: 'a'.repeat(64) }));
  if (ledger === 'evicted transcript') { record.events = []; record.historyStartCursor = record.cursor; }
  await writeFile(sessionFile, JSON.stringify(data));
  const native = fakeClient(); native.info = f.native.info;
  const restored = await fixture(t, { sessionFile, native, restore: true });
  if (ledger === 'evicted transcript') {
    await assert.rejects(restored.resume());
    assert.equal((await restored.send(own)).status, 'unknown');
  } else {
    await restored.resume();
    await restored.mode('automation');
    await assert.rejects(restored.send(), /incomplete|budget/);
  }
  assert.equal(mutations(restored).length, 0);
});

test('missing persisted history receipts fail closed rather than duplicating prior output', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  const own = await send(f);
  f.native.deliver(own.id);
  f.native.messages.push(assistant([{ type: 'text', text: 'already observed' }]));
  await f.tick();
  await f.service.close();
  const saved = JSON.parse(await readFile(sessionFile, 'utf8'));
  saved.sessions[0].events = [];
  saved.sessions[0].historyStartCursor = saved.sessions[0].cursor;
  await writeFile(sessionFile, JSON.stringify(saved));
  const native = fakeClient();
  native.info = f.native.info;
  native.messages = f.native.messages;
  const restored = await fixture(t, { sessionFile, native, restore: true });
  await assert.rejects(restored.resume());
  assert.equal((await restored.get()).archived, true);
  assert.equal(mutations(restored).length, 0);
  const state = await restored.get();
  const batch = await restored.service.request('GET', restored.path + '/events?after=' + state.historyStartCursor);
  assert.equal(batch.events.some(event => event.type === 'text'), false);
});

test('an unconfirmed cancellation is bounded and does not retry deletion', async t => {
  const f = await fixture(t);
  const own = await send(f);
  f.native.hooks.set('DELETE /api/session/' + f.native.info.id + '/inbox/' + own.id, () => undefined);
  await f.cancel(own.requestId);
  await f.cancel(own.requestId);
  const timer = [...f.timers.values()].find(timer => timer.delay === 10000);
  assert.ok(timer);
  timer.fn();
  assert.equal((await f.get()).archived, true);
  assert.equal(mutations(f).filter(call => call.method === 'DELETE').length, 1);
  assert.equal((await f.send(own)).status, 'unknown');
});

test('concurrent prompt requests serialize admission and never retry the losing mutation', async t => {
  const f = await fixture(t);
  await f.mode('automation');
  const results = await Promise.allSettled([f.send(), f.send()]);
  assert.equal(results.filter(result => result.status === 'fulfilled').length, 1);
  assert.equal(mutations(f).filter(call => call.path.endsWith('/prompt')).length, 1);
});

test('persistence failure prevents submission and stays sticky', async t => {
  const sessionFile = await privateFile(t);
  const f = await fixture(t, { sessionFile });
  await f.mode('automation');
  await writeFile(sessionFile, 'external edit');
  await assert.rejects(f.send(), /persistence failed/);
  assert.equal(mutations(f).some(call => call.path.endsWith('/prompt')), false);
  await assert.rejects(f.service.close(), /persistence failed/);
  f.service.close = async () => {};
});

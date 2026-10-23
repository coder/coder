import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import test from 'node:test';
import { get, request as httpRequest } from 'node:http';
import { createBroker } from '../src/broker.mjs';
import { createClient } from '../src/client.mjs';

async function fixture(t, options) {
  const broker = await createBroker(options);
  t.after(() => broker.close());
  const client = createClient(broker);
  const sessionId = randomUUID();
  const state = { sessionId, runtimeId: randomUUID(), cwd: '/workspace', mode: 'automation', activeTurnId: null, events: [] };
  let seq = 0;
  const heartbeat = async (events = [], overrides = {}) => client.request('POST', '/v1/runtime', { ...state, ...overrides, events: events.map(event => ({ ...event, seq: ++seq })) });
  await heartbeat([{ type: 'session.start', data: {} }]);
  return { broker, client, state, sessionId, heartbeat, path: '/v1/sessions/' + sessionId };
}

function fakeClock() {
  let time = 0;
  let nextId = 0;
  const timers = new Map();
  const scheduled = [];
  return {
    now: () => time,
    nextTimer() { return new Promise(resolve => scheduled.push(resolve)); },
    setTimer(callback, delay) {
      const id = ++nextId;
      timers.set(id, { callback, at: time + delay });
      scheduled.shift()?.();
      return id;
    },
    clearTimer(id) { timers.delete(id); },
    advance(ms) {
      time += ms;
      for (const [id, timer] of [...timers]) {
        if (timer.at <= time && timers.delete(id)) timer.callback();
      }
    },
  };
}

async function delivered(f, type) {
  for (let i = 0; i < 100; i++) {
    const command = (await f.heartbeat()).commands.find(c => c.type === type);
    if (command) return command;
  }
  assert.fail('runtime did not receive ' + type);
}

test('broker rejects unauthenticated, browser-origin and incorrect-host requests', async t => {
  const { broker } = await fixture(t);
  for (const headers of [{}, { Authorization: 'Bearer wrong' }, { Authorization: 'Bearer ' + broker.token, Origin: 'https://attacker.example' }, { Authorization: 'Bearer ' + broker.token, Host: 'attacker.example' }]) {
    const status = await new Promise((resolve, reject) => {
      get(broker.url + '/v1/sessions', { headers }, response => { response.resume(); resolve(response.statusCode); }).on('error', reject);
    });
    assert.equal(status, 403);
  }
});

test('submissions are idempotent, serialized and not complete at acceptance', async t => {
  const f = await fixture(t);
  const requestId = randomUUID();
  const body = { requestId, text: 'hello' };
  await f.client.request('POST', f.path + '/prompt', body);
  await f.client.request('POST', f.path + '/prompt', body);
  await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId, text: 'different' }), /different text/);
  await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'second' }), /busy/);
  assert.deepEqual((await f.heartbeat()).commands, [{ id: requestId, type: 'prompt', text: 'hello' }]);
  assert.equal((await f.heartbeat()).commands.length, 1, 'lost poll responses cause redelivery, not new IDs');
  assert.equal((await f.heartbeat([{ type: 'prompt.accepted', data: { requestId } }])).commands.length, 0);
  await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'second' }), /busy/);
  await f.heartbeat([
    { type: 'turn.start', data: { requestId, turnId: 't' } },
    { type: 'turn.start', data: { requestId, turnId: 'child' } },
    { type: 'turn.complete', data: { requestId, turnId: 'child' } },
  ]);
  assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'running');
  await f.heartbeat([{ type: 'turn.complete', data: { requestId, turnId: 't', reason: 'answer', answer: 'ok' } }]);
  assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'completed');
  await f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'second' });
});

test('human control, active native work and ended runtimes reject submission', async t => {
  const clock = fakeClock();
  const f = await fixture(t, clock);
  const send = () => f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'hello' });
  await f.heartbeat([], { mode: 'human' });
  await assert.rejects(send(), /human control/);
  assert.deepEqual((await f.client.request('GET', f.path)).lastError, { message: 'Session is busy or under human control', at: 0 });
  await f.heartbeat([], { activeTurnId: 'native-turn' });
  await assert.rejects(send(), /busy/);
  await f.heartbeat([], { ended: true });
  await assert.rejects(send(), /disconnected/);
});

test('an idle stale session rejects new commands until an empty heartbeat returns', async t => {
  const clock = fakeClock();
  clock.advance(1700000000000);
  const f = await fixture(t, clock);
  assert.equal((await f.client.request('GET', f.path)).startedAt, clock.now());
  clock.advance(10000);
  assert.equal((await f.client.request('GET', f.path)).stale, false);
  clock.advance(1);
  const body = { requestId: randomUUID(), text: 'hello' };
  for (const [action, value] of [['prompt', body], ['mode', { mode: 'human' }]]) {
    await assert.rejects(f.client.request('POST', f.path + '/' + action, value), error => error.status === 409 && /stale/.test(error.message));
  }
  const stale = (await f.client.request('GET', '/v1/sessions')).sessions[0];
  assert.equal(stale.stale, true);
  assert.equal(stale.fault, false);
  assert.equal(stale.ended, false);
  assert.equal(stale.mode, 'automation');
  assert.equal(stale.startedAt, 1700000000000);
  assert.equal(stale.lastError, undefined, 'staleness and rejected stale commands are not session errors');
  assert.deepEqual(await f.client.request('GET', f.path + '/events?after=1'), { events: [], cursor: 1 });
  assert.deepEqual(await f.heartbeat(), { ack: 1, commands: [] });
  assert.deepEqual(await f.client.request('GET', f.path), { ...stale, stale: false, lastSeen: clock.now() });
  await f.client.request('POST', f.path + '/prompt', body);
  assert.deepEqual((await f.heartbeat()).commands, [{ id: body.requestId, type: 'prompt', text: body.text }]);
});

test('cancel stays ordered before prompt delivery across a stale heartbeat', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, staleMs: 1000 });
  const requestId = randomUUID();
  await f.client.request('POST', f.path + '/prompt', { requestId, text: 'hello' });
  await f.client.request('POST', f.path + '/cancel', { requestId });
  await f.client.request('POST', f.path + '/cancel', { requestId });
  clock.advance(1001);
  assert.equal((await f.client.request('POST', f.path + '/prompt', { requestId, text: 'hello' })).status, 'queued');
  const { commands } = await f.heartbeat();
  assert.deepEqual(commands.map(c => c.type), ['prompt', 'cancel']);
  assert.equal(commands[1].requestId, requestId);
  await assert.rejects(f.client.request('POST', f.path + '/cancel', { requestId: randomUUID() }), /Unknown prompt/);
  await f.heartbeat([
    { type: 'prompt.accepted', data: { requestId } },
    { type: 'command.done', data: { commandId: commands[1].id } },
  ]);
  await f.client.request('POST', f.path + '/cancel', { requestId });
  assert.deepEqual((await f.heartbeat()).commands, [], 'an acknowledged cancellation must not be queued again');
  clock.advance(10000);
  assert.equal((await f.client.request('GET', f.path)).fault, false);
});

test('mode changes wait for the runtime acknowledgement, not HTTP delivery', async t => {
  const f = await fixture(t);
  const changing = f.client.request('POST', f.path + '/mode', { mode: 'human' });
  let command;
  for (let i = 0; i < 100 && !command; i++) command = (await f.heartbeat()).commands.find(c => c.type === 'mode');
  assert.ok(command, 'runtime received the mode command');
  await f.heartbeat([{ type: 'mode', data: { mode: 'human' } }, { type: 'command.done', data: { commandId: command.id } }], { mode: 'human' });
  await changing;
  assert.equal((await f.client.request('GET', f.path)).mode, 'human');
});

test('native errors retain only the latest safe summary after recovery, redelivery and journal eviction', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, maxEvents: 1 });
  const secret = 'secret-bearing native output '.repeat(1000);
  for (const [type, data, message] of [
    ['prompt.rejected', { reason: 'Session is not idle under automation control' }, 'Session is not idle under automation control'],
    ['prompt.rejected', { reason: secret }, 'Native prompt rejected; attach to inspect the session'],
    ['tool.end', { isError: true, result: secret }, 'Native tool call failed; inspect the transcript'],
    ['turn.complete', { reason: 'error', answer: secret }, 'Native turn failed; inspect the transcript'],
  ]) {
    clock.advance(10);
    const event = { type, data };
    const { ack } = await f.heartbeat([event]);
    const lastError = { message, at: clock.now() };
    assert.deepEqual((await f.client.request('GET', f.path)).lastError, lastError);
    clock.advance(10);
    await f.client.request('POST', '/v1/runtime', { ...f.state, events: [{ ...event, seq: ack }] });
    await f.heartbeat([
      { type: 'turn.complete', data: { reason: 'answer' } },
      { type: 'prompt.rejected', data: { cancelled: true, reason: 'Cancelled before submission' } },
      { type: 'turn.complete', data: { reason: 'aborted', isAborted: true } },
    ]);
    await assert.rejects(f.client.request('GET', f.path + '/events?after=' + (ack - 1)), /expired/);
    const state = await f.client.request('GET', f.path);
    assert.deepEqual(state.lastError, lastError);
    assert.ok(Buffer.byteLength(JSON.stringify(state.lastError)) < 1024);
    assert.equal(state.startedAt, 0);
    assert.equal(state.ended, false);
    assert.equal(state.fault, false);
    assert.deepEqual((await f.client.request('GET', '/v1/sessions')).sessions, [state]);
  }
  await f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'continue after recovery' });
});

test('cancellation, refusal and unstructured tool output do not invent session errors', async t => {
  const f = await fixture(t);
  await f.heartbeat([
    { type: 'prompt.rejected', data: { cancelled: true } },
    { type: 'turn.complete', data: { reason: 'aborted', isAborted: true } },
    { type: 'turn.complete', data: { reason: 'refusal' } },
    { type: 'tool.end', data: { isError: false, result: 'Error mentioned in normal output' } },
    { type: 'tool.end', data: { isError: 'false', result: { error: 'not a reliable failure signal' } } },
    { type: 'session.end', data: { reason: 'exit' } },
  ]);
  assert.equal((await f.client.request('GET', f.path)).lastError, undefined);
});

test('command errors keep a bounded summary without weakening fatal prompt or recoverable mode outcomes', async t => {
  for (const type of ['prompt', 'mode', 'overflow']) await t.test(type, async t => {
    const clock = fakeClock();
    const f = await fixture(t, { ...clock, maxEvents: 1 });
    let changing;
    let command;
    if (type === 'mode') {
      changing = assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'human' }), error => error.status === 409);
      command = await delivered(f, 'mode');
    } else if (type === 'prompt') {
      await f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'run' });
      command = await delivered(f, 'prompt');
    }
    clock.advance(10);
    await f.heartbeat([{ type: type === 'overflow' ? 'overflow' : 'command.error', data: {
      commandId: command?.id, reason: 'secret-bearing native error '.repeat(1000),
    } }]);
    await changing;
    const state = await f.client.request('GET', f.path);
    assert.deepEqual(state.lastError, { message: type === 'prompt'
      ? 'Prompt command failed without a definitive outcome; work state is unknown'
      : type === 'overflow' ? 'Native event stream overflow; attach to inspect the session'
        : 'Native command failed; attach to inspect the session', at: 10 });
    assert.equal(state.fault, type !== 'mode');
    assert.equal(state.ended, type !== 'mode');
    await f.heartbeat([{ type: 'command.done', data: { commandId: command?.id } }]);
    assert.deepEqual((await f.client.request('GET', f.path)).lastError, state.lastError);
    if (type === 'mode') {
      const retry = f.client.request('POST', f.path + '/mode', { mode: 'human' });
      const next = await delivered(f, 'mode');
      await f.heartbeat([{ type: 'command.done', data: { commandId: next.id } }], { mode: 'human' });
      await retry;
    } else {
      await assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'human' }), /disconnected/);
    }
  });
});

test('event replay deduplicates stale runtime delivery and reports expired cursors', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, maxEvents: 3 });
  const event = { seq: 2, type: 'text', data: { text: 'once' } };
  await f.client.request('POST', '/v1/runtime', { ...f.state, events: [event] });
  clock.advance(10001);
  assert.deepEqual(await f.client.request('POST', '/v1/runtime', { ...f.state, events: [event] }), { ack: 2, commands: [] });
  assert.equal((await f.client.request('GET', f.path + '/events?after=0')).events.length, 2);
  clock.advance(10001);
  const next = { seq: 3, type: 'text', data: { text: 'next' } };
  assert.deepEqual(await f.client.request('POST', '/v1/runtime', { ...f.state, events: [event, next] }), { ack: 3, commands: [] });
  assert.deepEqual((await f.client.request('GET', f.path + '/events?after=1')).events, [event, next]);
  const state = await f.client.request('GET', f.path);
  assert.equal(state.stale, false);
  assert.equal(state.fault, false);
  for (let seq = 4; seq <= 6; seq++) await f.client.request('POST', '/v1/runtime', { ...f.state, events: [{ seq, type: 'text', data: { text: String(seq) } }] });
  await assert.rejects(f.client.request('GET', f.path + '/events?after=0'), /expired/);
  assert.deepEqual((await f.client.request('GET', f.path + '/events?after=4')).events.map(e => e.seq), [5, 6]);
});

test('restarted mods invalidate stale in-flight requests instead of retrying them', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, staleMs: 1000 });
  const requestId = randomUUID();
  await f.client.request('POST', f.path + '/prompt', { requestId, text: 'side effect' });
  clock.advance(1001);
  const runtimeId = randomUUID();
  const result = await f.client.request('POST', '/v1/runtime', { ...f.state, runtimeId, events: [{ seq: 1, type: 'session.start', data: {} }] });
  assert.deepEqual(result.commands, []);
  await f.client.request('POST', '/v1/runtime', { ...f.state, runtimeId, events: [
    { seq: 2, type: 'turn.start', data: { requestId, turnId: 'late' } },
    { seq: 3, type: 'turn.complete', data: { requestId, turnId: 'late' } },
  ] });
  assert.equal((await f.client.request('POST', f.path + '/prompt', { requestId, text: 'side effect' })).status, 'unknown');
  clock.advance(1001);
  await assert.rejects(f.heartbeat(), /Retired runtime/);
  const state = await f.client.request('GET', f.path);
  assert.equal(state.runtimeId, runtimeId);
  assert.equal(state.lastSeen, 1001);
  assert.equal(state.stale, true);
  assert.ok((await f.client.request('GET', f.path + '/events?after=0')).events.some(e => e.type === 'runtime.replaced'));
});

test('end and overflow remain terminal across heartbeats and late acknowledgements', async t => {
  for (const type of ['session.end', 'overflow', 'ended heartbeat']) await t.test(type, async t => {
    const f = await fixture(t);
    const body = { requestId: randomUUID(), text: 'side effect' };
    await f.client.request('POST', f.path + '/prompt', body);
    await delivered(f, 'prompt');
    if (type === 'ended heartbeat') await f.heartbeat([], { ended: true });
    else await f.heartbeat([{ type, data: {} }]);
    assert.deepEqual((await f.heartbeat([
      { type: 'turn.start', data: { requestId: body.requestId, turnId: 'late' } },
      { type: 'turn.complete', data: { requestId: body.requestId, turnId: 'late' } },
    ], { ended: false })).commands, []);
    const state = await f.client.request('GET', f.path);
    assert.equal(state.ended, true);
    assert.equal(state.fault, type === 'overflow');
    assert.equal(state.mode, 'human');
    assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'unknown');
    await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'retry' }), /disconnected/);
    await assert.rejects(f.client.request('GET', f.path + '/events?after=' + state.cursor + '&wait=1'), /disconnected/);
  });
});

test('gaps in a returning heartbeat invalidate the whole batch and cannot resurrect uncertain work', async t => {
  const clock = fakeClock();
  const f = await fixture(t, clock);
  const body = { requestId: randomUUID(), text: 'side effect' };
  await f.client.request('POST', f.path + '/prompt', body);
  await f.heartbeat([
    { type: 'prompt.accepted', data: { requestId: body.requestId } },
    { type: 'turn.start', data: { requestId: body.requestId, turnId: 't' } },
  ]);
  clock.advance(10001);
  const complete = { seq: 4, type: 'turn.complete', data: { requestId: body.requestId, turnId: 't' } };
  const batch = { ...f.state, events: [complete, { seq: 6, type: 'text', data: { text: 'gap' } }] };
  for (let i = 0; i < 2; i++) await assert.rejects(f.client.request('POST', '/v1/runtime', batch), /event gap/);
  const stale = await f.client.request('GET', f.path);
  assert.equal(stale.lastSeen, 0, 'a rejected batch does not refresh liveness');
  assert.equal(stale.stale, true);
  assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'unknown');
  const { events } = await f.client.request('GET', f.path + '/events?after=3');
  assert.deepEqual(events.map(e => e.type), ['overflow']);
  assert.match(events[0].data.reason, /Runtime event gap/);
  assert.deepEqual(stale.lastError, { message: events[0].data.reason, at: clock.now() });
  clock.advance(10);
  assert.deepEqual(await f.client.request('POST', '/v1/runtime', { ...f.state, events: [complete] }), { ack: 4, commands: [] });
  assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'unknown');
  const state = await f.client.request('GET', f.path);
  assert.equal(state.stale, false);
  assert.equal(state.fault, true);
  assert.equal(state.ended, true);
  assert.deepEqual(state.lastError, stale.lastError, 'late completion does not erase a stream failure');
  assert.deepEqual((await f.client.request('GET', '/v1/sessions')).sessions, [state]);
  await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'retry' }), /disconnected/);
});

test('accepted turns survive a stale heartbeat and complete without redelivery', async t => {
  for (const observerFirst of [true, false]) await t.test(observerFirst ? 'observer first' : 'returning heartbeat first', async t => {
    const clock = fakeClock();
    const f = await fixture(t, clock);
    const body = { requestId: randomUUID(), text: 'side effect' };
    await f.client.request('POST', f.path + '/prompt', body);
    await delivered(f, 'prompt');
    await f.heartbeat([
      { type: 'prompt.accepted', data: { requestId: body.requestId } },
      { type: 'turn.start', data: { requestId: body.requestId, turnId: 'root', text: body.text } },
    ], { activeTurnId: 'root' });
    const before = await f.client.request('GET', f.path);
    clock.advance(10001);
    if (observerFirst) {
      assert.deepEqual(await f.client.request('GET', f.path), { ...before, stale: true });
      assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'running');
      await assert.rejects(f.client.request('POST', f.path + '/prompt', { ...body, text: 'different' }), /different text/);
      await assert.rejects(f.client.request('POST', f.path + '/cancel', { requestId: body.requestId }), error => error.status === 409 && /stale/.test(error.message));
    }
    // An idle snapshot cannot finish a request whose completion is still buffered.
    assert.deepEqual(await f.heartbeat([{ type: 'text', data: { requestId: body.requestId, turnId: 'root', text: 'done' } }]), { ack: 4, commands: [] });
    assert.deepEqual(await f.client.request('GET', f.path), { ...before, stale: false, lastSeen: clock.now(), activeTurnId: null, cursor: 4 });
    assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'running');
    await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'next' }), /busy/);
    assert.deepEqual(await f.heartbeat([{ type: 'turn.complete', data: { requestId: body.requestId, turnId: 'root', reason: 'answer', answer: 'done' } }]), { ack: 5, commands: [] });
    assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'completed');
    assert.deepEqual((await f.client.request('GET', f.path + '/events?after=3')).events.map(e => e.type), ['text', 'turn.complete']);
    await f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'next' });
  });
});

test('replacement establishes a new history boundary without hiding it from old readers', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, maxRetiredRuntimes: 1 });
  assert.equal((await f.client.request('GET', f.path)).historyStartCursor, 0);
  clock.advance(10);
  await f.heartbeat([{ type: 'overflow', data: { reason: 'Mod event buffer exhausted; attach to inspect the session' } }]);
  const old = await f.client.request('GET', f.path);
  const oldCursor = old.cursor;
  assert.deepEqual(old.lastError, { message: 'Mod event buffer exhausted; attach to inspect the session', at: 10 });
  clock.advance(10);
  const runtime = { ...f.state, runtimeId: randomUUID(), events: [{ seq: 1, type: 'session.start', data: {} }] };
  await f.client.request('POST', '/v1/runtime', runtime);
  const state = await f.client.request('GET', f.path);
  assert.equal(state.fault, false);
  assert.equal(state.ended, false);
  assert.equal(state.historyStartCursor, oldCursor + 1);
  assert.equal(state.startedAt, 0, 'replacement preserves the first broker registration, including epoch zero');
  assert.equal(state.lastSeen, 20);
  assert.deepEqual(state.lastError, old.lastError);
  await f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'healthy replacement' });
  assert.deepEqual((await f.client.request('GET', f.path + '/events?after=' + oldCursor)).events.map(e => e.type), ['runtime.replaced', 'session.start']);
  assert.deepEqual((await f.client.request('GET', f.path + '/events?after=' + state.historyStartCursor)).events.map(e => e.type), ['session.start']);
  await assert.rejects(f.heartbeat(), /Retired runtime/);
  await assert.rejects(f.client.request('POST', '/v1/runtime', { ...runtime, runtimeId: randomUUID() }), /replacement budget/);
  await assert.rejects(f.heartbeat(), /Retired runtime/);
  const faulted = await f.client.request('GET', f.path);
  assert.equal(faulted.runtimeId, runtime.runtimeId);
  assert.equal(faulted.startedAt, 0);
  assert.deepEqual(faulted.lastError, { message: 'Runtime replacement budget exhausted; restart the broker', at: 20 });
});

test('a prompt upload spanning replacement cannot target the new runtime', async t => {
  const f = await fixture(t);
  const upload = httpRequest(f.broker.url + f.path + '/prompt', {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + f.broker.token, 'Content-Type': 'application/json', Expect: '100-continue' },
  });
  const continued = new Promise(resolve => upload.once('continue', resolve));
  const response = new Promise((resolve, reject) => {
    upload.once('response', res => { res.resume(); resolve(res.statusCode); });
    upload.once('error', reject);
  });
  upload.flushHeaders();
  await continued;
  const runtime = { ...f.state, runtimeId: randomUUID(), events: [] };
  await f.client.request('POST', '/v1/runtime', runtime);
  upload.end(JSON.stringify({ requestId: randomUUID(), text: 'side effect' }));
  assert.equal(await response, 409);
  assert.deepEqual((await f.client.request('POST', '/v1/runtime', runtime)).commands, []);
});

test('journal payload budgets evict by bytes across sessions and expire empty histories', async t => {
  const f = await fixture(t, { maxJournalBytes: 300, maxTotalJournalBytes: 350 });
  const event = { type: 'text', data: { text: '😀'.repeat(50) } };
  await f.heartbeat([event, event]);
  await assert.rejects(f.client.request('GET', f.path + '/events?after=1'), /expired/);
  assert.equal((await f.client.request('GET', f.path + '/events?after=2')).events.length, 1);
  const sessionId = randomUUID();
  await f.client.request('POST', '/v1/runtime', { ...f.state, sessionId, events: [{ ...event, seq: 1 }] });
  for (const after of [0, 2]) await assert.rejects(f.client.request('GET', f.path + '/events?after=' + after), /expired/);
  assert.deepEqual((await f.client.request('GET', f.path + '/events?after=3')).events, []);
  const retained = await f.client.request('GET', '/v1/sessions/' + sessionId + '/events?after=0');
  assert.equal(retained.events.length, 1);
  assert.ok(Buffer.byteLength(JSON.stringify(retained.events)) <= 350);
});

test('oversized event payloads are rejected before mutating the journal', async t => {
  const f = await fixture(t, { maxEventBytes: 256 });
  await assert.rejects(f.client.request('POST', '/v1/runtime', { ...f.state, events: [
    { seq: 2, type: 'text', data: { text: 'small' } },
    { seq: 3, type: 'text', data: { text: '😀'.repeat(100) } },
  ] }), error => error.status === 413);
  assert.equal((await f.client.request('GET', f.path)).cursor, 1);
  assert.equal((await f.heartbeat([{ type: 'text', data: {} }])).ack, 2);
});

test('prompt and cancel acknowledgement deadlines survive stale heartbeats', async t => {
  for (const type of ['prompt', 'cancel']) for (const acknowledged of [true, false]) {
    await t.test(type + (acknowledged ? ' acknowledged before deadline' : ' acknowledged after deadline'), async t => {
      const clock = fakeClock();
      const f = await fixture(t, { ...clock, staleMs: 1000 });
      const body = { requestId: randomUUID(), text: 'side effect' };
      await f.client.request('POST', f.path + '/prompt', body);
      if (type === 'cancel') {
        await delivered(f, 'prompt');
        await f.heartbeat([{ type: 'prompt.accepted', data: { requestId: body.requestId } }]);
        await f.client.request('POST', f.path + '/cancel', { requestId: body.requestId });
      }
      const command = await delivered(f, type);
      clock.advance(9999);
      const stale = await f.client.request('GET', f.path);
      assert.equal(stale.stale, true);
      assert.equal(stale.fault, false);
      assert.equal((await delivered(f, type)).id, command.id);
      if (type === 'cancel') await f.client.request('POST', f.path + '/cancel', { requestId: body.requestId });
      const ack = type === 'prompt'
        ? { type: 'prompt.accepted', data: { requestId: body.requestId } }
        : { type: 'command.done', data: { commandId: command.id } };
      if (acknowledged) assert.deepEqual((await f.heartbeat([ack])).commands, []);
      clock.advance(1);
      assert.deepEqual((await f.heartbeat([ack])).commands, []);
      assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, acknowledged ? 'running' : 'unknown');
      const state = await f.client.request('GET', f.path);
      assert.equal(state.stale, false);
      assert.equal(state.fault, !acknowledged);
      assert.deepEqual(state.lastError, acknowledged ? undefined : {
        message: 'Command acknowledgement timed out; work state is unknown, inspect before retrying', at: 10000,
      });
    });
  }
});

test('mode acknowledgement deadlines survive stale heartbeats and a frozen wall clock', async t => {
  for (const frozen of [true, false]) await t.test(frozen ? 'frozen clock' : 'stale heartbeat', async t => {
    const clock = fakeClock();
    const f = await fixture(t, { ...clock, staleMs: 1000, now: frozen ? () => 0 : clock.now });
    const changing = assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'human' }), error => error.status === 504 && /unconfirmed/.test(error.message));
    const command = await delivered(f, 'mode');
    clock.advance(9999);
    const state = await f.client.request('GET', f.path);
    assert.equal(state.stale, !frozen);
    assert.equal(state.fault, false);
    assert.equal((await delivered(f, 'mode')).id, command.id);
    clock.advance(1);
    await changing;
    assert.deepEqual((await f.heartbeat([{ type: 'command.done', data: { commandId: command.id } }])).commands, []);
    assert.equal((await f.client.request('GET', f.path)).fault, true);
  });
});

test('mode acknowledgement after staleness clears its timer and replacement ends old waiters', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, staleMs: 1000 });
  const changing = f.client.request('POST', f.path + '/mode', { mode: 'human' });
  const command = await delivered(f, 'mode');
  clock.advance(9999);
  assert.equal((await f.client.request('GET', f.path)).stale, true);
  await f.heartbeat([{ type: 'command.done', data: { commandId: command.id } }]);
  await changing;
  clock.advance(1);
  assert.equal((await f.client.request('GET', f.path)).fault, false);
  const interrupted = assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'automation' }), /unconfirmed/);
  await delivered(f, 'mode');
  await f.client.request('POST', '/v1/runtime', { ...f.state, runtimeId: randomUUID(), events: [] });
  await interrupted;
  clock.advance(10000);
  assert.equal((await f.client.request('GET', f.path)).fault, false, 'old command timers cannot fault the replacement');
});

test('long polls remain bounded while stale and report replacement and history eviction', async t => {
  for (const change of ['replacement', 'expiry', 'eviction']) await t.test(change, async t => {
    const clock = fakeClock();
    const f = await fixture(t, { ...clock, maxEvents: 1 });
    const registered = clock.nextTimer();
    const reading = f.client.request('GET', f.path + '/events?after=1&wait=1');
    const outcome = change === 'eviction' ? assert.rejects(reading, /expired/) : reading;
    await registered;
    if (change === 'replacement') {
      await f.client.request('POST', '/v1/runtime', { ...f.state, runtimeId: randomUUID(), events: [] });
      assert.deepEqual((await outcome).events.map(e => e.type), ['runtime.replaced']);
    } else if (change === 'expiry') {
      clock.advance(15000);
      assert.deepEqual(await outcome, { events: [], cursor: 1 });
      assert.equal((await f.client.request('GET', f.path)).stale, true);
      const nextRegistered = clock.nextTimer();
      const nextRead = f.client.request('GET', f.path + '/events?after=1&wait=1');
      await nextRegistered;
      clock.advance(15000);
      assert.deepEqual(await nextRead, { events: [], cursor: 1 });
      const returning = clock.nextTimer();
      const resumed = f.client.request('GET', f.path + '/events?after=1&wait=1');
      await returning;
      await f.heartbeat([{ type: 'text', data: { text: 'back' } }]);
      assert.deepEqual((await resumed).events.map(e => e.type), ['text']);
      const state = await f.client.request('GET', f.path);
      assert.equal(state.stale, false);
      assert.equal(state.fault, false);
      assert.equal(state.ended, false);
    } else {
      await f.heartbeat([{ type: 'text', data: {} }, { type: 'text', data: {} }]);
      await outcome;
    }
  });
});

test('disconnecting a mode waiter faults uncertain work rather than allowing a retry', async t => {
  const clock = fakeClock();
  const f = await fixture(t, { ...clock, now: () => 0 });
  const controller = new AbortController();
  const changing = assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'human' }, controller.signal), /abort/i);
  await delivered(f, 'mode');
  const registered = clock.nextTimer();
  const reading = f.client.request('GET', f.path + '/events?after=1&wait=1');
  await registered;
  controller.abort();
  await changing;
  assert.equal((await reading).events[0].type, 'overflow');
  assert.deepEqual((await f.heartbeat()).commands, []);
  await assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'human' }), /disconnected/);
});

test('idempotency preserves exact text without retaining completed prompt payloads', async t => {
  const f = await fixture(t);
  const body = { requestId: randomUUID(), text: '\ud800' };
  await f.client.request('POST', f.path + '/prompt', body);
  await f.heartbeat([{ type: 'turn.complete', data: { requestId: body.requestId, turnId: 't' } }]);
  assert.equal((await f.client.request('POST', f.path + '/prompt', body)).status, 'completed');
  await assert.rejects(f.client.request('POST', f.path + '/prompt', { ...body, text: '\ud801' }), /different text/);
});

test('native launch registration provides identity without parsing CLI output', async t => {
  let client;
  const sessionId = randomUUID();
  const broker = await createBroker({ launch: async (cwd, launchId) => {
    await client.request('POST', '/v1/runtime', { sessionId, launchId, runtimeId: 'r', cwd, mode: 'automation', events: [] });
  } });
  t.after(() => broker.close());
  client = createClient(broker);
  const session = await client.request('POST', '/v1/sessions', { cwd: '/workspace' });
  assert.equal(session.sessionId, sessionId);
  assert.equal(session.attachCommand, 'claude attach ' + sessionId.slice(0, 8));
});

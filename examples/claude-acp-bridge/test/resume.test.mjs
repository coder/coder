import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { createBroker } from '../src/broker.mjs';
import { createClient } from '../src/client.mjs';

function deferred() {
  let resolve;
  const promise = new Promise(yes => { resolve = yes; });
  return { promise, resolve };
}
const event = (seq, type, data) => ({ seq, type, data });
const rejected = (status, pattern) => error => error.status === status && pattern.test(error.message);

async function fixture(t) {
  const directory = await mkdtemp('/tmp/opencode/bridge-resume-');
  const sessionFile = join(directory, 'sessions.json');
  const brokers = new Set();
  t.after(async () => {
    for (const broker of brokers) await broker.close();
    await rm(directory, { recursive: true, force: true });
  });
  let time = 123;
  const native = { sessionId: randomUUID(), launchId: randomUUID(), cwd: '/workspace', startedAt: time, ended: true };
  const old = { sessionId: native.sessionId, launchId: native.launchId, runtimeId: randomUUID(), cwd: native.cwd, mode: 'automation', activeTurnId: null };
  const path = '/v1/sessions/' + native.sessionId;
  const requestId = randomUUID();
  const history = [event(1, 'session.start', old), event(2, 'text', { text: 'earlier private transcript' }), event(3, 'session.end', { reason: 'Finished' })];
  let found = [native];
  let calls = 0;
  async function start(options = {}) {
    const broker = await createBroker({ sessionFile, now: () => time, discover: async () => found,
      resume: async (sessionId, cwd) => { calls++; assert.equal(sessionId, native.sessionId); assert.equal(cwd, native.cwd); }, ...options });
    brokers.add(broker);
    return { broker, client: createClient(broker) };
  }
  async function stop(broker) { brokers.delete(broker); await broker.close(); }
  const first = await start({ discover: undefined });
  await first.client.request('POST', '/v1/runtime', { ...old, events: history.slice(0, 2) });
  await first.client.request('POST', path + '/prompt', { requestId, text: 'obsolete undelivered command' });
  await first.client.request('POST', '/v1/runtime', { ...old, events: history.slice(2) });
  await stop(first.broker);
  return { native, old, path, history, sessionFile, requestId, start, stop,
    advance() { time += 2001; }, found(value) { found = value; }, get calls() { return calls; },
    disk: async () => JSON.parse(await readFile(sessionFile, 'utf8')).sessions[0],
  };
}

describe('explicit native resume', { concurrency: true, timeout: 15000 }, () => {
  test('archive reads are inert and eligibility requires the exact discovered identity and a callback', async t => {
    const f = await fixture(t);
    const { client, broker } = await f.start();
    const saved = await f.disk();
    const s = await client.request('GET', f.path);
    assert.equal(s.archived, true);
    assert.equal(s.resumable, true);
    assert.equal(s.resumePending, false);
    assert.deepEqual((await client.request('GET', '/v1/sessions')).sessions, [s]);
    assert.deepEqual(await client.request('GET', f.path + '/events?after=0'), { events: f.history, cursor: 3 });
    for (const runtimeId of [f.old.runtimeId, randomUUID()]) await assert.rejects(
      client.request('POST', '/v1/runtime', { ...f.old, runtimeId, events: [] }), rejected(409, /Archived.*read-only/));
    for (const [action, body] of [['mode', { mode: 'human' }], ['mode', { mode: 'automation' }], ['prompt', { requestId: randomUUID(), text: 'no' }], ['cancel', { requestId: f.requestId }]]) {
      await assert.rejects(client.request('POST', f.path + '/' + action, body), rejected(409, /Archived.*read-only/));
    }
    for (const found of [[], [{ ...f.native, cwd: '/different' }], [{ ...f.native, sessionId: randomUUID() }], [{ ...f.native, launchId: f.native.launchId.slice(0, 8) }], [f.native, { ...f.native, launchId: randomUUID() }]]) {
      f.found(found);
      f.advance();
      assert.equal((await client.request('GET', f.path)).resumable, false);
      await assert.rejects(client.request('POST', f.path + '/resume', { launchId: f.native.launchId }), rejected(409, /no matching native discovery identity/));
    }
    assert.equal(f.calls, 0);
    assert.deepEqual(await f.disk(), saved);
    await f.stop(broker);
    f.found([f.native]);
    const disabled = await f.start({ resume: undefined });
    assert.equal((await disabled.client.request('GET', f.path)).resumable, false);
    await assert.rejects(disabled.client.request('POST', f.path + '/resume', {}), rejected(501, /not configured/));
  });

  test('concurrent explicit wakes persist one human grant, return before registration and promote without losing history', async t => {
    const f = await fixture(t);
    const entered = deferred();
    const release = deferred();
    let launches = 0;
    const { client, broker } = await f.start({ resume: async (sessionId, cwd) => {
      launches++;
      assert.equal(sessionId, f.native.sessionId);
      assert.equal(cwd, f.native.cwd);
      const saved = await f.disk();
      assert.deepEqual(saved.events.slice(0, 3), f.history);
      assert.equal(saved.runtimeId, f.old.runtimeId);
      assert.equal(saved.historyStartCursor, 0);
      assert.equal(saved.runtimeStartCursor, 4);
      assert.deepEqual(saved.events[3], event(4, 'session.resuming', { sessionId, mode: 'human' }));
      for (const key of ['commands', 'requests', 'resumePending', 'launchId', 'humanRequested']) assert.equal(Object.hasOwn(saved, key), false, key);
      entered.resolve();
      await release.promise;
    } });
    t.after(() => release.resolve());
    const wakes = Array.from({ length: 20 }, () => client.request('POST', f.path + '/resume', { launchId: randomUUID(), cwd: '/ignored', prompt: 'never queued' }));
    await entered.promise;
    const pending = await client.request('GET', f.path);
    assert.equal(pending.sessionId, f.native.sessionId);
    assert.equal(pending.cwd, f.native.cwd);
    assert.equal(pending.runtimeId, f.old.runtimeId);
    assert.equal(pending.startedAt, f.native.startedAt);
    for (const key of ['resumePending', 'registrationPending']) assert.equal(pending[key], true, key);
    for (const key of ['archived', 'resumable', 'ended', 'fault', 'stale']) assert.equal(pending[key], false, key);
    assert.equal(pending.activeTurnId, null);
    assert.equal(pending.mode, 'human');
    assert.equal(pending.runtimeStartCursor, 4);
    assert.equal(pending.historyStartCursor, 0);
    assert.deepEqual(await client.request('POST', f.path + '/mode', { mode: 'human' }), {});
    await assert.rejects(client.request('POST', f.path + '/mode', { mode: 'automation' }), rejected(409, /registration is pending/));
    await assert.rejects(client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'no' }), rejected(409, /registration is pending/));
    await assert.rejects(client.request('POST', '/v1/runtime', { ...f.old, events: [] }), rejected(409, /Retired runtime/));
    const next = { ...f.old, runtimeId: randomUUID() };
    await assert.rejects(client.request('POST', '/v1/runtime', { ...next, launchId: randomUUID(), events: [] }), rejected(400, /does not match/));
    await assert.rejects(client.request('POST', '/v1/runtime', { ...next, cwd: '/different', events: [] }), rejected(400, /does not match/));
    f.advance();
    assert.deepEqual(await client.request('GET', f.path), pending, 'stale terminal discovery cannot end an explicit resume');
    release.resolve();
    for (const response of await Promise.all(wakes)) assert.deepEqual(response, pending);
    assert.equal(launches, 1);
    assert.deepEqual(await client.request('POST', f.path + '/resume', {}), pending);

    const registered = await client.request('POST', '/v1/runtime', { ...next, events: [event(1, 'session.start', next), event(2, 'text', { text: 'resumed private transcript' })] });
    assert.equal(registered.ack, 2);
    assert.equal(registered.commands.length, 1);
    const [human] = registered.commands;
    assert.equal(human.type, 'mode');
    assert.equal(human.mode, 'human');
    const ready = await client.request('GET', f.path);
    assert.equal(ready.runtimeId, next.runtimeId);
    assert.equal(ready.mode, 'human');
    assert.equal(ready.historyStartCursor, 0);
    assert.equal(ready.runtimeStartCursor, 4);
    for (const key of ['archived', 'resumePending', 'registrationPending', 'fault', 'ended']) assert.equal(ready[key], false, key);
    const saved = await f.disk();
    assert.equal(saved.runtimeId, next.runtimeId, 'registration identity is durable before native ack');
    assert.deepEqual(saved.events.slice(0, 3), f.history);
    assert.deepEqual(saved.events[4], event(5, 'runtime.resumed', { sessionId: next.sessionId, runtimeId: next.runtimeId, mode: 'human', lastSeen: ready.lastSeen }));
    assert.deepEqual(saved.events.map(e => e.type), ['session.start', 'text', 'session.end', 'session.resuming', 'runtime.resumed', 'session.start', 'text']);
    assert.deepEqual((await client.request('GET', f.path + '/events?after=0')).events, saved.events);
    await assert.rejects(client.request('POST', f.path + '/mode', { mode: 'automation' }), rejected(409, /awaiting acknowledgement/));
    await assert.rejects(client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'no' }), rejected(409, /human control/));
    assert.deepEqual((await client.request('POST', '/v1/runtime', { ...next, events: [] })).commands, [human]);
    const acknowledged = await client.request('POST', '/v1/runtime', { ...next, mode: 'human', events: [
      event(3, 'mode', { mode: 'human' }), event(4, 'command.done', { commandId: human.id }),
    ] });
    assert.deepEqual(acknowledged, { ack: 4, commands: [] });
    assert.equal((await f.disk()).events.at(-1).data.commandId, human.id);
    await assert.rejects(client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'still no' }), rejected(409, /human control/));
    await assert.rejects(client.request('POST', '/v1/runtime', { ...f.old, events: [] }), rejected(409, /Retired runtime/));
    assert.equal((await client.request('POST', f.path + '/resume', {})).runtimeId, next.runtimeId);
    assert.equal(launches, 1);
    const observing = client.request('POST', f.path + '/mode', { mode: 'automation' });
    let observe;
    for (let i = 0; i < 100 && !observe; i++) observe = (await client.request('POST', '/v1/runtime', { ...next, mode: 'human', events: [] })).commands.find(c => c.type === 'mode');
    assert.equal(observe?.mode, 'automation');
    await client.request('POST', '/v1/runtime', { ...next, events: [
      event(5, 'mode', { mode: 'automation' }), event(6, 'command.done', { commandId: observe.id }),
    ] });
    await observing;
    const requestId = randomUUID();
    await client.request('POST', f.path + '/prompt', { requestId, text: 'explicit new work' });
    assert.deepEqual((await client.request('POST', '/v1/runtime', { ...next, events: [] })).commands, [
      { id: requestId, type: 'prompt', text: 'explicit new work' },
    ]);
    await client.request('POST', '/v1/runtime', { ...next, events: [
      event(7, 'turn.start', { requestId, turnId: 'new-turn', text: 'explicit new work' }),
      event(8, 'turn.complete', { requestId, turnId: 'new-turn', answer: 'done', reason: 'answer' }),
    ] });
    const before = await f.disk();
    await f.stop(broker);
    const restarted = await f.start({ resume: () => assert.fail('restart and reads must not wake native') });
    const archive = await restarted.client.request('GET', f.path);
    assert.equal(archive.archived, true);
    assert.equal(archive.resumePending, false);
    assert.equal(archive.registrationPending, false);
    assert.equal(archive.runtimeStartCursor, 4);
    assert.deepEqual(await f.disk(), before);
    await assert.rejects(restarted.client.request('POST', '/v1/runtime', { ...next, events: [] }), rejected(409, /Archived/));
  });

  test('native-only archives retain their known full launch identity and can resume directly into human mode', async t => {
    const f = await fixture(t);
    const native = { ...f.native, sessionId: randomUUID(), launchId: randomUUID() };
    f.found([native]);
    let calls = 0;
    const { client } = await f.start({ resume: async (sessionId, cwd) => {
      calls++;
      assert.equal(sessionId, native.sessionId);
      assert.equal(cwd, native.cwd);
    } });
    const path = '/v1/sessions/' + native.sessionId;
    assert.equal((await client.request('GET', path)).resumable, true);
    f.found([{ ...native, launchId: randomUUID() }]);
    f.advance();
    assert.equal((await client.request('GET', path)).resumable, false);
    await assert.rejects(client.request('POST', path + '/resume', {}), rejected(409, /no matching/));
    f.found([native]);
    const pending = await client.request('POST', path + '/resume', {});
    assert.equal(pending.runtimeId, undefined);
    assert.equal(pending.resumePending, true);
    assert.equal(pending.runtimeStartCursor, 2);
    assert.equal(pending.historyStartCursor, 0);
    const state = { ...f.old, sessionId: native.sessionId, launchId: native.launchId, runtimeId: randomUUID(), mode: 'human', events: [] };
    assert.deepEqual(await client.request('POST', '/v1/runtime', state), { ack: 0, commands: [] });
    const ready = await client.request('GET', path);
    assert.equal(ready.mode, 'human');
    assert.equal(ready.resumePending, false);
    assert.equal(ready.archived, false);
    assert.deepEqual((await client.request('GET', path + '/events?after=0')).events.map(e => e.type), ['session.end', 'session.resuming', 'runtime.resumed']);
    assert.equal(calls, 1);
  });

  test('restart while registration is pending archives the grant without replaying it', async t => {
    const f = await fixture(t);
    const first = await f.start();
    await first.client.request('POST', f.path + '/resume', {});
    assert.equal(f.calls, 1);
    const before = await f.disk();
    await f.stop(first.broker);
    const second = await f.start();
    const archive = await second.client.request('GET', f.path);
    assert.equal(archive.archived, true);
    assert.equal(archive.resumePending, false);
    assert.equal(archive.runtimeId, f.old.runtimeId);
    assert.deepEqual((await second.client.request('GET', f.path + '/events?after=0')).events, before.events);
    assert.equal(f.calls, 1);
    await assert.rejects(second.client.request('POST', '/v1/runtime', { ...f.old, runtimeId: randomUUID(), events: [] }), rejected(409, /Archived/));
  });

  for (const refused of [true, false]) test((refused ? 'definite refusal' : 'ambiguous failure') + ' preserves history, shares errors and never resurrects the old runtime', async t => {
    const f = await fixture(t);
    const entered = deferred();
    const release = deferred();
    let calls = 0;
    const { client } = await f.start({ resume: async () => {
      calls++;
      entered.resolve();
      await release.promise;
      throw Object.assign(new Error('provider-secret-must-not-leak'), { resumeRefused: refused });
    } });
    t.after(() => release.resolve());
    const first = client.request('POST', f.path + '/resume', {}).catch(error => error);
    await entered.promise;
    const other = client.request('POST', f.path + '/resume', {}).catch(error => error);
    // Both requests enter the route before the callback is released.
    await client.request('GET', f.path);
    release.resolve();
    const errors = await Promise.all([first, other]);
    for (const error of errors) {
      assert.equal(error.status, refused ? 409 : 502);
      assert.match(error.message, refused ? /refused before launch/ : /outcome is unknown/);
      assert.ok(!error.message.includes('provider-secret'));
      assert.equal(error.message, errors[0].message);
    }
    assert.equal(calls, 1);
    const s = await client.request('GET', f.path);
    assert.equal(s.archived, refused);
    assert.equal(s.resumePending, !refused);
    assert.equal(s.registrationPending, !refused);
    assert.equal(s.mode, 'human');
    assert.equal(s.ended, refused);
    assert.equal(s.runtimeId, f.old.runtimeId);
    assert.deepEqual((await f.disk()).events.slice(0, 3), f.history);
    assert.equal((await f.disk()).lastError.message, errors[0].message);
    assert.equal((await f.disk()).events.at(-1).type, 'session.resume_failed');
    await assert.rejects(client.request('POST', '/v1/runtime', { ...f.old, events: [] }), rejected(409, refused ? /Archived/ : /Retired/));
    await assert.rejects(client.request('POST', f.path + '/mode', { mode: 'automation' }), rejected(409, refused ? /Archived/ : /registration is pending/));
    if (!refused) assert.deepEqual(await client.request('POST', f.path + '/mode', { mode: 'human' }), {});
    await assert.rejects(client.request('POST', f.path + '/resume', {}), rejected(refused ? 409 : 502, refused ? /refused/ : /unknown/));
    assert.equal(calls, refused ? 2 : 1, 'only an explicit request after a definite refusal may retry');
    const healthy = { ...f.old, sessionId: randomUUID(), runtimeId: randomUUID(), events: [event(1, 'text', { text: 'healthy session' })] };
    assert.deepEqual(await client.request('POST', '/v1/runtime', healthy), { ack: 1, commands: [] });
  });

  test('a definite refusal permits a later explicit successful attempt, keeping both boundaries', async t => {
    const f = await fixture(t);
    let calls = 0;
    const { client } = await f.start({ resume: async () => {
      if (++calls === 1) throw Object.assign(new Error('No native transcript'), { resumeRefused: true });
    } });
    await assert.rejects(client.request('POST', f.path + '/resume', {}), /refused before launch/);
    const resumed = await client.request('POST', f.path + '/resume', {});
    assert.equal(resumed.resumePending, true);
    assert.equal(resumed.archived, false);
    assert.match(resumed.lastError.message, /refused before launch/);
    assert.equal(resumed.runtimeStartCursor, 6);
    assert.deepEqual((await f.disk()).events.slice(0, 3), f.history);
    assert.equal(calls, 2);
  });

  test('healthy arbitrary sessions cannot be resumed', async t => {
    const f = await fixture(t);
    const { client } = await f.start();
    const state = { ...f.old, sessionId: randomUUID(), runtimeId: randomUUID(), events: [] };
    await client.request('POST', '/v1/runtime', state);
    await assert.rejects(client.request('POST', '/v1/sessions/' + state.sessionId + '/resume', {}), rejected(409, /Only archived/));
    assert.equal(f.calls, 0);
  });

  test('failed persistence never invokes the native callback', async t => {
    const f = await fixture(t);
    const { client, broker } = await f.start();
    await writeFile(f.sessionFile, 'external change', { mode: 0o600 });
    await assert.rejects(client.request('POST', f.path + '/resume', {}), rejected(503, /Session persistence/));
    assert.equal(f.calls, 0);
    await assert.rejects(f.stop(broker), /Session persistence/);
    assert.equal(await readFile(f.sessionFile, 'utf8'), 'external change');
  });
});

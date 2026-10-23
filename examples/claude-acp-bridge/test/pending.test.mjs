import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { EventEmitter } from 'node:events';
import { describe, test } from 'node:test';
import { createAgent } from '../src/acp.mjs';
import { createBroker } from '../src/broker.mjs';
import { createClient } from '../src/client.mjs';
import { MCP_BLOCKED_REASON } from '../src/native-discovery.mjs';

const META = 'claude-acp-bridge';
function nativeSession(overrides = {}) {
  return { sessionId: randomUUID(), launchId: randomUUID(), cwd: '/workspace', startedAt: 1792761788871,
    ended: false, blockedReason: MCP_BLOCKED_REASON, ...overrides };
}
function deferred() {
  let resolve;
  const promise = new Promise(yes => { resolve = yes; });
  return { promise, resolve };
}
async function fixture(t, options = {}) {
  let time = 1792761800000;
  let found = [nativeSession()];
  let discoveryCalls = 0;
  let polls = 0;
  const changes = new EventEmitter();
  const broker = await createBroker({
    now: () => time,
    discover: async () => { discoveryCalls++; return found; },
    setTimer(callback, ms) { if (ms === 15000) { polls++; changes.emit('poll'); } return setTimeout(callback, ms); },
    ...options,
  });
  const connections = [];
  t.after(async () => { connections.forEach(c => c.abort()); await broker.close(); });
  const client = createClient(broker);
  const native = found[0];
  const path = '/v1/sessions/' + native.sessionId;
  const runtime = { sessionId: native.sessionId, launchId: native.launchId, cwd: native.cwd,
    runtimeId: randomUUID(), mode: 'automation' };
  let seq = 0;
  const register = (events = [], overrides = {}) => client.request('POST', '/v1/runtime', {
    ...runtime, ...overrides, events: events.map(event => ({ ...event, seq: ++seq })),
  });
  async function waitForPoll(before) {
    while (polls <= before) await new Promise(resolve => changes.once('poll', resolve));
  }
  return { broker, client, native, path, runtime, register,
    advance(ms = 2001) { time += ms; }, found(value) { found = value; }, get discoveryCalls() { return discoveryCalls; },
    async attach() {
      const controller = new AbortController();
      connections.push(controller);
      const updates = [];
      const agent = createAgent({ signal: controller.signal, sessionUpdate: async n => { updates.push(n.update); } }, client);
      const before = polls;
      const loaded = await agent.loadSession({ sessionId: native.sessionId, cwd: native.cwd, mcpServers: [] });
      await waitForPoll(before);
      return { agent, loaded, updates };
    },
    async publish(events = [], overrides = {}) {
      const before = polls;
      const result = await register(events, overrides);
      await waitForPoll(before);
      return result;
    },
  };
}

describe('pre-registration native sessions', { concurrency: true, timeout: 10000 }, () => {
  test('list and detail discover blocked sessions; Join is immediate and never queues work', async t => {
    const f = await fixture(t);
    const session = await f.client.request('GET', f.path);
    assert.equal(session.sessionId, f.native.sessionId);
    assert.equal(session.cwd, f.native.cwd);
    assert.equal(session.startedAt, f.native.startedAt);
    assert.equal(session.registrationPending, true);
    assert.equal(session.blockedReason, MCP_BLOCKED_REASON);
    assert.equal(session.mode, 'human');
    for (const key of ['runtimeId', 'lastError', 'lastSeen']) assert.equal(Object.hasOwn(session, key), false, key);
    for (const key of ['stale', 'fault', 'ended']) assert.equal(session[key], false, key);
    assert.deepEqual((await f.client.request('GET', '/v1/sessions')).sessions, [session]);
    assert.deepEqual(await f.client.request('POST', f.path + '/mode', { mode: 'human' }), {});
    for (const [action, body] of [['mode', { mode: 'automation' }], ['prompt', { requestId: randomUUID(), text: 'must not execute' }]]) {
      await assert.rejects(f.client.request('POST', f.path + '/' + action, body), error => error.status === 409 && /registration is pending; join/.test(error.message));
    }
    f.advance(600000);
    f.found([]); // Absence or a transient read failure is not an end signal.
    assert.deepEqual(await f.client.request('GET', f.path), session);
    assert.equal(f.discoveryCalls, 2);
  });

  test('post-launch discovery correlates full launch UUID, not name prefix or cwd', async t => {
    const samePrefix = nativeSession();
    const other = nativeSession();
    let launched;
    const f = await fixture(t, {
      discover: async () => [samePrefix, other, ...(launched ? [launched] : [])],
      launch: async (cwd, launchId) => {
        samePrefix.launchId = launchId.slice(0, 8) + randomUUID().slice(8);
        launched = nativeSession({ cwd, launchId, blockedReason: undefined });
      },
    });
    await f.client.request('GET', '/v1/sessions'); // A fresh cache must not hide the new launch.
    const result = await f.client.request('POST', '/v1/sessions', { cwd: '/workspace' });
    assert.equal(result.sessionId, launched.sessionId);
    assert.equal(result.registrationPending, true);
    assert.equal(result.mode, 'human');
    assert.equal(result.blockedReason, undefined);
    assert.equal(result.stale, false);
  });

  test('discovery is single-flight and caches failures without affecting healthy runtime state', async t => {
    const discovery = deferred();
    const entered = deferred();
    let count = 0;
    let fail = false;
    const f = await fixture(t, { discover: async () => {
      count++;
      entered.resolve();
      if (fail) throw new Error('provider-secret-not-safe-to-return');
      return discovery.promise;
    } });
    await f.register();
    const lists = Array.from({ length: 10 }, () => f.client.request('GET', '/v1/sessions'));
    await entered.promise;
    discovery.resolve([nativeSession({ ...f.native, ended: true })]);
    const results = await Promise.all(lists);
    assert.equal(count, 1);
    const session = results[0].sessions[0];
    assert.equal(session.registrationPending, false);
    assert.equal(session.mode, 'automation');
    assert.equal(session.ended, false, 'discovery never overwrites an authoritative registered runtime');
    fail = true;
    f.advance();
    assert.deepEqual(await f.client.request('GET', f.path), session);
    assert.deepEqual((await f.client.request('GET', '/v1/sessions')).sessions, [session]);
    assert.equal(count, 2);
    f.advance(10000);
    assert.equal((await f.client.request('GET', f.path)).stale, true, 'discovery does not refresh runtime heartbeats');
    await f.register();
    assert.equal((await f.client.request('GET', f.path)).stale, false);
  });

  test('only native termination ends a pending session and raw discovery fields never leak', async t => {
    const f = await fixture(t);
    f.found([{ ...f.native, blockedReason: 'provider-secret', providerEnv: { key: 'provider-secret' } }]);
    const starting = await f.client.request('GET', f.path);
    assert.equal(starting.blockedReason, undefined);
    assert.equal(starting.ended, false);
    assert.ok(!JSON.stringify(starting).includes('provider-secret'));
    f.found([{ ...f.native, ended: true }]);
    f.advance();
    const ended = await f.client.request('GET', f.path);
    assert.equal(ended.ended, true);
    assert.equal(ended.fault, false);
    assert.equal(ended.stale, false);
    assert.equal(ended.lastError, undefined);
    assert.equal(ended.blockedReason, undefined);
    await assert.rejects(f.client.request('POST', f.path + '/mode', { mode: 'human' }), /disconnected/);
  });

  test('first registration promotes the same session and only later replacement invalidates integrity', async t => {
    const f = await fixture(t);
    const pending = await f.client.request('GET', f.path);
    await assert.rejects(f.register([], { launchId: randomUUID() }), /does not match/);
    await assert.rejects(f.register([], { cwd: '/other' }), /does not match/);
    assert.deepEqual(await f.register([{ type: 'session.start', data: f.runtime }]), { ack: 1, commands: [] });
    const registered = await f.client.request('GET', f.path);
    assert.equal(registered.sessionId, pending.sessionId);
    assert.equal(registered.startedAt, pending.startedAt);
    assert.equal(registered.runtimeId, f.runtime.runtimeId);
    assert.equal(registered.registrationPending, false);
    assert.equal(registered.blockedReason, undefined);
    assert.equal(registered.historyStartCursor, 0);
    assert.equal(registered.fault, false);
    assert.equal(registered.lastError, undefined);
    const events = (await f.client.request('GET', f.path + '/events?after=0')).events;
    assert.deepEqual(events.map(e => e.type), ['session.pending', 'runtime.registered', 'session.start']);
    f.advance();
    assert.equal((await f.client.request('GET', f.path)).registrationPending, false, 'old native discovery never regresses a registered session');
    await f.register([], { runtimeId: randomUUID() });
    const nextEvents = (await f.client.request('GET', f.path + '/events?after=' + registered.cursor)).events;
    assert.deepEqual(nextEvents.map(e => e.type), ['runtime.replaced']);
    await assert.rejects(f.register(), /Retired runtime/);
  });

  for (const mode of ['human', 'automation']) test('loaded ACP promotes runtime identity and ' + mode + ' mode without reattachment', async t => {
    const f = await fixture(t);
    const { agent, loaded, updates } = await f.attach();
    assert.equal(loaded.modes.currentModeId, 'human');
    assert.equal(loaded._meta[META].registrationPending, true);
    assert.equal(loaded._meta[META].blockedReason, MCP_BLOCKED_REASON);
    assert.equal(loaded._meta[META].runtimeId, undefined);
    const listed = await agent.listSessions();
    assert.equal(listed.sessions[0]._meta[META].registrationPending, true);
    assert.equal(listed.sessions[0]._meta[META].blockedReason, MCP_BLOCKED_REASON);
    assert.equal((await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] }))._meta[META].registrationPending, true);
    await assert.rejects(agent.setSessionMode({ sessionId: f.native.sessionId, modeId: 'automation' }), /registration is pending/);
    await assert.rejects(agent.prompt({ sessionId: f.native.sessionId, prompt: [{ type: 'text', text: 'no' }] }), /registration is pending/);
    assert.deepEqual((await f.publish([{ type: 'session.start', data: f.runtime }], { mode })).commands, []);
    assert.ok(updates.some(u => u.sessionUpdate === 'current_mode_update' && u.currentModeId === mode));
    const status = updates.find(u => u._meta?.[META]?.registrationPending === false)._meta[META];
    assert.equal(status.runtimeId, f.runtime.runtimeId);
    assert.equal(status.blockedReason, undefined);
    assert.equal(updates.some(u => u._meta?.[META]?.error), false);
    const refreshed = await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] });
    assert.equal(refreshed._meta[META].registrationPending, false);
    assert.equal(refreshed.modes.currentModeId, mode);
    if (mode === 'human') {
      // The mod's explicit mode event enables automation, not pre-registration Join.
      await f.publish([{ type: 'mode', data: { mode: 'automation' } }]);
    }
    const prompt = agent.prompt({ sessionId: f.native.sessionId, prompt: [{ type: 'text', text: 'after registration' }] });
    let command;
    for (let i = 0; i < 100 && !command; i++) command = (await f.register()).commands.find(c => c.type === 'prompt');
    assert.ok(command);
    await f.publish([
      { type: 'turn.start', data: { requestId: command.id, turnId: 'root', text: command.text } },
      { type: 'turn.complete', data: { requestId: command.id, turnId: 'root', answer: 'done', reason: 'answer' } },
    ]);
    assert.deepEqual(await prompt, { stopReason: 'end_turn' });
    const second = await f.attach();
    assert.equal(second.loaded._meta[META].registrationPending, false);
    assert.equal(second.loaded._meta[META].runtimeId, f.runtime.runtimeId);
    assert.equal(second.updates.some(u => u._meta?.[META]?.error), false);
  });

  test('Join before registration preserves human intent and requires native acknowledgement before automation', async t => {
    const f = await fixture(t);
    const { agent, updates } = await f.attach();
    assert.deepEqual(await agent.setSessionMode({ sessionId: f.native.sessionId, modeId: 'human' }), {});
    const first = await f.publish([{ type: 'session.start', data: f.runtime }]);
    assert.equal(first.commands.length, 1);
    const command = first.commands[0];
    assert.equal(command.type, 'mode');
    assert.equal(command.mode, 'human');
    assert.equal((await f.client.request('GET', f.path)).mode, 'human');
    assert.equal((await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] })).modes.currentModeId, 'human');
    assert.equal(updates.find(u => u.sessionUpdate === 'current_mode_update').currentModeId, 'human');
    await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'no' }), /human control/);
    await assert.rejects(agent.setSessionMode({ sessionId: f.native.sessionId, modeId: 'automation' }), /awaiting acknowledgement/);
    assert.deepEqual((await f.register()).commands, [command], 'delivery is idempotent until acknowledgement');
    await f.publish([{ type: 'mode', data: { mode: 'automation' } }]);
    assert.equal(updates.filter(u => u.sessionUpdate === 'current_mode_update').at(-1).currentModeId, 'human');
    assert.deepEqual((await f.publish([
      { type: 'mode', data: { mode: 'human' } },
      { type: 'command.done', data: { commandId: command.id } },
    ], { mode: 'human' })).commands, []);
    await assert.rejects(agent.prompt({ sessionId: f.native.sessionId, prompt: [{ type: 'text', text: 'still human' }] }), /human control/);
    const changing = agent.setSessionMode({ sessionId: f.native.sessionId, modeId: 'automation' });
    let enable;
    for (let i = 0; i < 100 && !enable; i++) enable = (await f.register([], { mode: 'human' })).commands.find(c => c.type === 'mode');
    assert.equal(enable.mode, 'automation');
    await f.publish([{ type: 'mode', data: { mode: 'automation' } }, { type: 'command.done', data: { commandId: enable.id } }]);
    await changing;
    assert.equal((await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] })).modes.currentModeId, 'automation');
    assert.equal((await f.client.request('GET', f.path)).fault, false);
  });

  for (const failure of ['incorrect mode', 'command error', 'timeout']) test('unconfirmed Join synchronization fails closed on ' + failure, async t => {
    let expire;
    const f = await fixture(t, { setTimer(callback, ms) {
      if (ms === 10000) expire = callback;
      return setTimeout(callback, ms);
    } });
    await f.client.request('GET', f.path);
    await f.client.request('POST', f.path + '/mode', { mode: 'human' });
    const { commands: [command] } = await f.register();
    if (failure === 'timeout') expire();
    else await f.register([{ type: failure === 'command error' ? 'command.error' : 'command.done', data: { commandId: command.id } }]);
    const session = await f.client.request('GET', f.path);
    assert.equal(session.fault, true);
    assert.equal(session.ended, true);
    await assert.rejects(f.client.request('POST', f.path + '/prompt', { requestId: randomUUID(), text: 'no' }), /disconnected/);
  });

  test('read-only ACP refresh waits for registration events without duplicating replay', async t => {
    const f = await fixture(t);
    const { agent, updates } = await f.attach();
    await f.register([{ type: 'session.start', data: f.runtime }]);
    const loaded = await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] });
    assert.equal(loaded._meta[META].registrationPending, false);
    assert.equal(loaded._meta[META].runtimeId, f.runtime.runtimeId);
    assert.equal(loaded.modes.currentModeId, 'automation');
    const before = updates.length;
    assert.deepEqual(await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] }), loaded);
    assert.equal(updates.length, before);
  });

  test('ACP can load a provisional snapshot when registration wins the initial event read race', async t => {
    const f = await fixture(t);
    const provisional = await f.client.request('GET', f.path);
    await f.register([{ type: 'session.start', data: f.runtime }]);
    const controller = new AbortController();
    t.after(() => controller.abort());
    const agent = createAgent({ signal: controller.signal, sessionUpdate: async () => {} }, {
      request(method, path, body, signal) {
        if (path === f.path) return Promise.resolve(provisional);
        return f.client.request(method, path, body, signal);
      },
    });
    const loaded = await agent.loadSession({ sessionId: f.native.sessionId, cwd: f.native.cwd, mcpServers: [] });
    assert.equal(loaded._meta[META].registrationPending, false);
    assert.equal(loaded._meta[META].runtimeId, f.runtime.runtimeId);
    assert.equal(loaded.modes.currentModeId, 'automation');
  });
});

import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import test from 'node:test';
import { createAgent } from '../src/acp.mjs';
import { createBroker } from '../src/broker.mjs';
import { createClient } from '../src/client.mjs';

function nativeService(harness) {
  const nativeSessionId = harness === 'opencode' ? 'ses_' + randomUUID().replaceAll('-', '') : randomUUID();
  const session = {
    sessionId: harness + ':' + nativeSessionId, nativeSessionId, harness,
    cwd: '/work/project', mode: 'human', archived: true, ended: true,
    resumable: true, cursor: 1, historyStartCursor: 0, runtimeId: randomUUID(),
  };
  const calls = [];
  const service = {
    async preparePersistence() { calls.push(['prepare']); },
    async close() { calls.push(['close']); },
    async request(method, path, body) {
      calls.push([method, path, body]);
      if (path === '/v1/sessions') return method === 'GET' ? { sessions: [session] } : session;
      if (path.includes('/events')) return { events: [{ seq: 1, type: 'turn.start', data: { turnId: 'native-turn', text: 'saved task' } }], cursor: 1 };
      return session;
    },
  };
  return { service, calls, session };
}

async function fixture(t, harness) {
  const native = nativeService(harness);
  const broker = await createBroker({ [harness]: native.service, deferPersistence: true });
  t.after(() => broker.close());
  await broker.preparePersistence();
  return { ...native, broker, client: createClient(broker) };
}

for (const [harness, title] of [['codex', 'Codex'], ['opencode', 'OpenCode']]) {
  test('one authenticated broker routes ' + title + ' IDs without changing Claude identities', async t => {
    const f = await fixture(t, harness);
    const id = randomUUID();
    await f.client.request('POST', '/v1/runtime', {
      sessionId: id, runtimeId: randomUUID(), cwd: '/work/project', mode: 'human', activeTurnId: null, events: [],
    });
    const listed = await f.client.request('GET', '/v1/sessions');
    assert.deepEqual(listed.sessions.map(s => s.sessionId), [id, f.session.sessionId]);
    assert.deepEqual((await f.client.request('GET', '/v1/capabilities')).harnesses.map(h => h.id), ['claude', harness]);
    await f.client.request('POST', '/v1/sessions', { cwd: '/work/project', harness });
    assert.deepEqual(f.calls.at(-1), ['POST', '/v1/sessions', { cwd: '/work/project' }]);
    await f.client.request('POST', '/v1/sessions/' + encodeURIComponent(f.session.sessionId) + '/mode', { mode: 'human', harness: 'claude' });
    assert.equal(f.calls.at(-1)[1], '/v1/sessions/' + encodeURIComponent(f.session.sessionId) + '/mode');
    const before = f.calls.length;
    await assert.rejects(f.client.request('POST', '/v1/sessions', { cwd: '/work/project', harness: 'unknown' }), /Unknown native harness/);
    await assert.rejects(f.client.request('GET', '/v1/sessions/' + harness + '%3Ainvalid'), new RegExp('Invalid ' + title));
    await assert.rejects(f.client.request('GET', '/v1/sessions/%zz'), /encoding/);
    assert.equal(f.calls.length, before);
    const response = await fetch(f.broker.url + '/v1/sessions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ cwd: '/work/project', harness }) });
    assert.equal(response.status, 403);
    assert.equal(f.calls.length, before);
  });

  test(title + ' ACP carries harness identity and selection through new, list, load and history', async t => {
    const f = await fixture(t, harness);
    const controller = new AbortController();
    t.after(() => controller.abort());
    const updates = [];
    const agent = createAgent({ signal: controller.signal, async sessionUpdate(update) { updates.push(update); } }, f.client);
    const initialized = await agent.initialize();
    assert.equal(initialized.agentInfo.name, 'coder-acp');
    assert.deepEqual(initialized._meta['coder-acp'].harnesses.map(h => h.id), ['claude', harness]);
    const created = await agent.newSession({ cwd: '/work/project', mcpServers: [], _meta: { 'coder-acp': { harness } } });
    assert.equal(created.sessionId, f.session.sessionId);
    assert.equal(created._meta['coder-acp'].harness, harness);
    assert.equal(created._meta['coder-acp'].nativeSessionId, f.session.nativeSessionId);
    assert.ok(f.calls.some(([method, path]) => method === 'POST' && path === '/v1/sessions'));
    assert.equal((await agent.listSessions()).sessions[0]._meta['coder-acp'].harness, harness);
    assert.equal((await agent.loadSession({ sessionId: f.session.sessionId, cwd: '/work/project', mcpServers: [] }))._meta['coder-acp'].nativeSessionId, f.session.nativeSessionId);
    assert.equal(updates[0]._meta['coder-acp'].harness, harness);
    assert.equal(updates[0].update.content.text, 'saved task');
    await assert.rejects(agent.newSession({ cwd: '/work/project', _meta: { 'coder-acp': { harness: 'unknown' } } }), /Unknown native harness/);
    assert.ok(!f.calls.some(([, path]) => path?.endsWith('/resume')));
    // Never accept a native identity advertised as a different harness.
    f.session.harness = harness === 'codex' ? 'opencode' : 'codex';
    await assert.rejects(agent.loadSession({ sessionId: f.session.sessionId, cwd: '/work/project', mcpServers: [] }), /invalid native session/);
  });

  test(title + ' startup must finish before authenticated mutations become available', async t => {
    let launched = false;
    const broker = await createBroker({ deferPersistence: true, [harness]: {
      async preparePersistence() {}, async close() {},
      async request() { launched = true; return {}; },
    } });
    t.after(() => broker.close());
    const entered = Promise.withResolvers();
    const release = Promise.withResolvers();
    const ready = broker.preparePersistence(async () => { entered.resolve(); await release.promise; });
    await entered.promise;
    const client = createClient(broker);
    await assert.rejects(client.request('POST', '/v1/sessions', { cwd: '/work/project', harness }), error => error.status === 503);
    assert.equal(launched, false);
    release.resolve();
    await ready;
    await client.request('POST', '/v1/sessions', { cwd: '/work/project', harness });
    assert.equal(launched, true);
  });

  test('legacy brokers remain usable and disabled ' + title + ' cannot launch Claude', async t => {
    let launches = 0;
    const broker = await createBroker({ launch: () => { launches++; } });
    t.after(() => broker.close());
    const client = createClient(broker);
    assert.deepEqual((await client.request('GET', '/v1/capabilities')).harnesses.map(h => h.id), ['claude']);
    await assert.rejects(client.request('POST', '/v1/sessions', { cwd: '/work/project', harness }), /not enabled/);
    assert.equal(launches, 0);
    let mutations = 0;
    const agent = createAgent({ async sessionUpdate() {} }, { async request(method) {
      if (method === 'POST') mutations++;
      throw Object.assign(new Error('Unknown route'), { status: 404 });
    } });
    assert.equal((await agent.initialize()).agentInfo.title, 'Claude Code');
    await assert.rejects(agent.newSession({ cwd: '/work/project', _meta: { 'coder-acp': { harness } } }), new RegExp('does not advertise ' + title));
    assert.equal(mutations, 0);
  });
}

test('three harnesses share discovery and lifecycle without routing across services', async t => {
  const codex = nativeService('codex');
  const opencode = nativeService('opencode');
  const broker = await createBroker({ codex: codex.service, opencode: opencode.service, deferPersistence: true });
  t.after(() => broker.close());
  await broker.preparePersistence();
  const client = createClient(broker);
  assert.deepEqual((await client.request('GET', '/v1/capabilities')).harnesses.map(h => h.id), ['claude', 'codex', 'opencode']);
  assert.deepEqual((await client.request('GET', '/v1/sessions')).sessions.map(s => s.sessionId), [codex.session.sessionId, opencode.session.sessionId]);
  const before = codex.calls.length;
  const path = '/v1/sessions/' + encodeURIComponent(opencode.session.sessionId) + '/resume';
  await client.request('POST', path, {});
  assert.equal(codex.calls.length, before);
  assert.deepEqual(opencode.calls.at(-1), ['POST', path, {}]);
  assert.equal(opencode.calls[0][0], 'prepare');
});

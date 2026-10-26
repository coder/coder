import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { setImmediate } from 'node:timers/promises';
import { test } from 'node:test';
import { createAgent } from '../src/acp.mjs';

function deferred() {
  let resolve;
  const promise = new Promise(yes => { resolve = yes; });
  return { promise, resolve };
}

function fixture(t) {
  const sessionId = randomUUID();
  const session = { sessionId, cwd: '/workspace', mode: 'human', runtimeId: randomUUID(),
    archived: true, resumable: true, ended: true, fault: false, cursor: 0 };
  const events = [];
  const updates = [];
  const controller = new AbortController();
  t.after(() => controller.abort());
  const emit = (type, data) => { events.push({ seq: ++session.cursor, type, data }); };
  emit('text', { turnId: 'saved', text: 'saved conversation' });
  const client = { async request(method, path, body, signal) {
    assert.equal(method, 'GET', 'observing restoration must not mutate native sessions');
    if (path.includes('/events?')) {
      await client.beforeEvents?.();
      const after = Number(new URL(path, 'http://broker').searchParams.get('after'));
      if (after === session.cursor && path.endsWith('wait=1')) {
        await new Promise((resolve, reject) => {
          if (signal.aborted) reject(signal.reason);
          else signal.addEventListener('abort', () => reject(signal.reason), { once: true });
        });
      }
      return { cursor: session.cursor, events: events.filter(e => e.seq > after) };
    }
    if (path === '/v1/sessions') return { sessions: [{ ...session }] };
    return { ...session };
  } };
  const agent = createAgent({ signal: controller.signal, async sessionUpdate(value) { updates.push(value.update); } }, client);
  const load = () => agent.loadSession({ sessionId, cwd: session.cwd, mcpServers: [] });
  function resumeSnapshot() {
    emit('session.resuming', { sessionId, mode: 'human' });
    Object.assign(session, { archived: false, ended: false, resumable: false, resumePending: true,
      registrationPending: true, runtimeStartCursor: session.cursor });
    return { ...session };
  }
  return { sessionId, session, events, updates, client, agent, load, emit, resumeSnapshot };
}

test('observing successful startup restoration must not apply a previous refusal as current control', async t => {
  const f = fixture(t);
  await f.load();
  f.resumeSnapshot();
  f.emit('session.resume_failed', { sessionId: f.sessionId, reason: 'missing transcript', retryable: true });
  Object.assign(f.session, { archived: true, ended: true, resumePending: false, registrationPending: false, resumable: true });
  f.resumeSnapshot();
  await f.agent.listSessions();
  await setImmediate();
  const status = f.updates.filter(u => u.sessionUpdate === 'session_info_update').at(-1)._meta['claude-acp-bridge'];
  assert.equal(status.archived, false, 'the native wake succeeded, but the old refusal stopped the new poller');
  assert.equal(status.resumePending, true);
});

test('an archive load overlapping another client resume must remain followable', async t => {
  const f = fixture(t);
  const entered = deferred();
  const release = deferred();
  f.client.beforeEvents = async () => { entered.resolve(); await release.promise; };
  const loading = f.load();
  await entered.promise;
  f.resumeSnapshot();
  Object.assign(f.session, { runtimeId: randomUUID(), resumePending: false, registrationPending: false });
  f.emit('runtime.resumed', { sessionId: f.sessionId, runtimeId: f.session.runtimeId, mode: 'human', lastSeen: 123 });
  f.emit('session.start', { sessionId: f.sessionId, runtimeId: f.session.runtimeId });
  release.resolve();
  await loading;
  await assert.doesNotReject(f.agent.listSessions(), 'the archive replay consumed the new runtime boundary without updating its state');
});

import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { chmod, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { createClient } from '../src/client.mjs';
import { restoreSessions } from '../template/image/startup.mjs';

const secret = 'private-provider-state-must-not-be-reported';
const archive = (harness = 'claude', extra = {}) => ({
  sessionId: harness === 'opencode' ? 'opencode:ses_' + randomBytes(13).toString('hex') : (harness === 'codex' ? 'codex:' : '') + randomUUID(), harness,
  cwd: '/workspace', runtimeId: randomUUID(), archived: true, ended: true,
  resumable: true, resumePending: false, registrationPending: false,
  stale: false, fault: false, mode: 'human', activeTurnId: null, ...extra,
});

async function fixture(t, sessions, handler = () => {}) {
  const state = await mkdtemp('/tmp/opencode/bridge-restoration-');
  const token = randomBytes(32).toString('hex');
  const current = new Map(sessions.map(s => [s.sessionId, { ...s }]));
  const calls = [];
  const reports = [];
  const server = createServer(async (req, res) => {
    try {
      assert.equal(req.headers.authorization, 'Bearer ' + descriptor.token);
      let text = '';
      for await (const chunk of req) text += chunk;
      const body = text ? JSON.parse(text) : undefined;
      const match = req.url.match(/^\/v1\/sessions\/([^/]+)(?:\/(resume|mode))?$/);
      assert.ok(match, 'restoration cannot create sessions, prompt, cancel, or read transcripts');
      const id = decodeURIComponent(match[1]);
      const action = match[2];
      calls.push({ method: req.method, id, action, body });
      const session = current.get(id);
      assert.ok(session);
      if (action === 'resume') {
        assert.equal(req.method, 'POST');
        assert.deepEqual(body, {});
        const files = (await readdir(state)).filter(name => name.startsWith('restore-') && name.endsWith('.json'));
        assert.ok(files.length);
        const guards = await Promise.all(files.map(name => readFile(join(state, name), 'utf8').then(JSON.parse)));
        assert.ok(guards.some(guard => guard.attempted.includes(id)), 'claim must be durable before resume');
      } else if (action === 'mode') {
        assert.equal(req.method, 'POST');
        assert.deepEqual(body, { mode: 'automation' });
        assert.equal(session.registrationPending, false, 'wait for native registration before mode');
        assert.equal(session.resumePending, false);
      } else {
        assert.equal(req.method, 'GET');
        assert.equal(body, undefined);
      }
      const response = await handler({ id, action, session, body, req });
      if (response) {
        res.writeHead(response.status, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(response.body));
        return;
      }
      if (action === 'resume') Object.assign(session, { archived: false, ended: false, runtimeId: randomUUID() });
      if (action === 'mode') session.mode = body.mode;
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(session));
    } catch (error) {
      res.writeHead(500, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: error.message }));
    }
  });
  t.after(async () => {
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
    await rm(state, { recursive: true, force: true });
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const descriptor = { token, url: 'http://127.0.0.1:' + server.address().port };
  const options = { state, instance: 'a'.repeat(64), descriptor, sessions,
    client: createClient(descriptor), report: result => reports.push(result), pollMs: 1 };
  return { state, current, calls, reports, options };
}

test('aggregate native resume latency uses the restoration deadline, not the ordinary HTTP deadline', async t => {
  let now = 0;
  const deadlines = [];
  t.mock.method(AbortSignal, 'timeout', ms => {
    const controller = new AbortController();
    deadlines.push({ at: now + ms, controller });
    return controller.signal;
  });
  for (const elapsed of [30000, 60000]) {
    const session = archive();
    const resuming = Promise.withResolvers();
    const release = Promise.withResolvers();
    const f = await fixture(t, [session], async ({ action }) => {
      if (action === 'resume') { resuming.resolve(); await release.promise; }
    });
    const pending = restoreSessions(f.options);
    await resuming.promise;
    // Several native RPCs can each meet their deadline while taking more than
    // 25 seconds together. Advance only HTTP/restore deadlines, not real time.
    now += elapsed;
    for (const deadline of deadlines) if (deadline.at <= now) deadline.controller.abort();
    release.resolve();
    const { results } = await pending;
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 1);
    if (elapsed === 30000) {
      assert.equal(results[0].status, 'restored');
      assert.equal(f.current.get(session.sessionId).mode, 'automation');
    } else {
      assert.equal(results[0].status, 'failed');
      assert.equal(results[0].reason, 'timeout');
      assert.equal(f.calls.some(c => c.action === 'mode'), false);
    }
  }
});

describe('workspace-only saved session restoration', { concurrency: true, timeout: 10000 }, () => {
  test('restores every eligible Claude, Codex and OpenCode identity sequentially without prompts or cancelling active native work', async t => {
    const sessions = [archive(), archive('codex', { activeTurnId: 'running-native-turn' }), archive('opencode', { sessionId: 'opencode:ses_0123456789abAbCdEfGhIjKlMn' }),
      archive('claude', { archived: false, ended: false }),
      archive('codex', { resumePending: true }), archive('claude', { registrationPending: true }),
      archive('claude', { resumable: false }), archive('claude', { sessionId: secret }),
      archive('opencode', { resumePending: true }), archive('opencode', { registrationPending: true }),
      archive('opencode', { archived: false, ended: false }), archive('opencode', { resumable: false }),
      ...['ses_AbC123', 'opencode:' + randomUUID(), 'opencode:ses_', 'opencode:ses_AbC-123', 'OpenCode:ses_AbC123']
        .map(sessionId => archive('opencode', { sessionId }))];
    const f = await fixture(t, sessions);
    const result = await restoreSessions(f.options);
    assert.deepEqual(result.results, sessions.slice(0, 3).map(s => ({ sessionId: s.sessionId, status: 'restored', mode: 'automation' })));
    assert.deepEqual(f.reports, result.results);
    assert.deepEqual(f.calls.filter(c => c.action === 'resume').map(c => c.id), sessions.slice(0, 3).map(s => s.sessionId));
    assert.deepEqual(f.calls.filter(c => c.action).map(c => c.action), ['resume', 'mode', 'resume', 'mode', 'resume', 'mode']);
    assert.ok(result.sessions.slice(0, 3).every(s => s.mode === 'automation' && !s.archived && !s.ended));
    assert.equal(result.sessions[1].activeTurnId, 'running-native-turn');
    assert.deepEqual(result.sessions.slice(3), sessions.slice(3));
    assert.deepEqual(sessions[0].mode, 'human', 'input snapshots are not mutated');
    const [guardFile] = await readdir(f.state);
    assert.equal((await stat(join(f.state, guardFile))).mode & 0o777, 0o600);
    const raw = await readFile(join(f.state, guardFile), 'utf8');
    assert.ok(!raw.includes(secret) && !raw.includes(f.options.descriptor.token));
  });

  test('restores multiple sessions per harness with original case-preserving identities and no duplicate wake', async t => {
    const sharedUUID = randomUUID();
    const sessions = [archive('claude', { sessionId: sharedUUID }), archive('codex', { sessionId: 'codex:' + sharedUUID }),
      archive('opencode', { sessionId: 'opencode:ses_0123456789abAbCdEfGhIjKlMn' }), archive(), archive('codex'), archive('opencode')];
    const f = await fixture(t, sessions);
    const result = await restoreSessions({ ...f.options, sessions: [...sessions, { ...sessions[2] }] });
    assert.deepEqual(f.calls.filter(c => c.action === 'resume').map(c => c.id), sessions.map(s => s.sessionId));
    assert.equal(new Set(f.calls.filter(c => c.action === 'resume').map(c => c.id)).size, 6);
    assert.deepEqual(result.results.slice(0, 6).map(r => r.status), Array(6).fill('restored'));
    assert.deepEqual(result.results[6], { sessionId: sessions[2].sessionId, status: 'skipped', reason: 'already_attempted' });
    const repeated = await restoreSessions({ ...f.options, sessions });
    assert.deepEqual(repeated.results.map(r => r.reason), Array(6).fill('already_attempted'));
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 6);
  });

  test('rechecks eligibility before waking a session that became live or pending after discovery', async t => {
    const sessions = [archive(), archive('codex'), archive('opencode')];
    const f = await fixture(t, sessions);
    Object.assign(f.current.get(sessions[0].sessionId), { archived: false, ended: false });
    f.current.get(sessions[1].sessionId).resumePending = true;
    const result = await restoreSessions(f.options);
    assert.deepEqual(result.results.map(r => r.status), ['skipped', 'skipped', 'restored']);
    assert.deepEqual(result.results.slice(0, 2).map(r => r.reason), ['no_longer_eligible', 'no_longer_eligible']);
    assert.deepEqual(f.calls.filter(c => c.action === 'resume').map(c => c.id), [sessions[2].sessionId]);
    assert.equal(result.sessions[0].mode, 'human', 'startup cannot change control of a live native session');
  });

  test('waits for registration and native automation acknowledgement before reporting success', async t => {
    const session = archive();
    const registration = Promise.withResolvers();
    const registering = Promise.withResolvers();
    const mode = Promise.withResolvers();
    const changingMode = Promise.withResolvers();
    const f = await fixture(t, [session], async ({ action, session }) => {
      if (action === 'resume') {
        Object.assign(session, { archived: false, ended: false, resumePending: true, registrationPending: true });
        return { status: 200, body: session };
      }
      if (!action && session.registrationPending) {
        registering.resolve();
        await registration.promise;
        Object.assign(session, { runtimeId: randomUUID(), resumePending: false, registrationPending: false });
      }
      if (action === 'mode') { changingMode.resolve(); await mode.promise; }
    });
    let finished = false;
    const restoring = restoreSessions(f.options).finally(() => { finished = true; });
    await registering.promise;
    assert.equal(finished, false);
    assert.equal(f.calls.some(c => c.action === 'mode'), false);
    registration.resolve();
    await changingMode.promise;
    assert.equal(finished, false);
    assert.deepEqual(f.reports, []);
    mode.resolve();
    const result = await restoring;
    assert.equal(result.results[0].status, 'restored');
    assert.equal(result.sessions[0].mode, 'automation');
  });

  test('continues after native refusals and transport ambiguity without leaking provider diagnostics', async t => {
    const sessions = [archive(), archive('codex'), archive('opencode'), archive('opencode')];
    const f = await fixture(t, sessions, ({ id, action, req }) => {
      if (action !== 'resume') return;
      if (id === sessions[0].sessionId) return { status: 409, body: { error: secret } };
      if (id === sessions[1].sessionId) { req.socket.destroy(); return { status: 502, body: {} }; }
      if (id === sessions[2].sessionId) return { status: 502, body: { error: secret } };
    });
    const result = await restoreSessions(f.options);
    assert.deepEqual(result.results.map(r => r.status), ['failed', 'failed', 'failed', 'restored']);
    assert.deepEqual(result.results.slice(0, 3).map(r => r.stage), ['resume', 'resume', 'resume']);
    assert.deepEqual(result.results.slice(0, 3).map(r => r.httpStatus), [409, undefined, 502]);
    assert.ok(!JSON.stringify(f.reports).includes(secret));
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 4);
    const second = await restoreSessions({ ...f.options, sessions: [...f.current.values()] });
    assert.deepEqual(second.results.map(r => r.status), ['skipped', 'skipped', 'skipped']);
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 4, 'refusals and uncertain launches are not automatically retried');
  });

  test('readiness timeout is per session and never retries an ambiguous wake', async t => {
    const sessions = [archive(), archive('codex')];
    const f = await fixture(t, sessions, ({ id, action, session }) => {
      if (id === sessions[0].sessionId && action === 'resume') {
        Object.assign(session, { archived: false, ended: false, resumePending: true, registrationPending: true });
        return { status: 200, body: session };
      }
    });
    const result = await restoreSessions({ ...f.options, timeoutMs: 1000 });
    assert.deepEqual(result.results[0], { sessionId: sessions[0].sessionId, status: 'failed', stage: 'registration', reason: 'timeout' });
    assert.equal(result.results[1].status, 'restored');
    assert.equal(f.calls.filter(c => c.action === 'resume' && c.id === sessions[0].sessionId).length, 1);
    assert.equal(f.calls.filter(c => c.action === 'mode' && c.id === sessions[0].sessionId).length, 0);
  });

  test('only the definite pre-queue human handshake conflict can retry automation mode', async t => {
    const sessions = [archive(), archive('codex'), archive('opencode')];
    let conflicts = 0;
    const f = await fixture(t, sessions, ({ id, action }) => {
      if (action !== 'mode') return;
      if (id === sessions[0].sessionId && conflicts++ === 0) return { status: 409, body: { error: 'Native human control is awaiting acknowledgement; inspect the native terminal' } };
      if (id === sessions[1].sessionId) return { status: 504, body: { error: secret } };
    });
    const result = await restoreSessions(f.options);
    assert.deepEqual(result.results.map(r => r.status), ['restored', 'failed', 'restored']);
    assert.equal(result.results[1].stage, 'automation');
    assert.equal(f.calls.filter(c => c.id === sessions[0].sessionId && c.action === 'mode').length, 2);
    assert.equal(f.calls.filter(c => c.id === sessions[1].sessionId && c.action === 'mode').length, 1);
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 3);
    assert.ok(!JSON.stringify(f.reports).includes(secret));
  });

  test('missing live flags, a replaced runtime or unconfirmed automation never reports success', async t => {
    const sessions = [archive(), archive('codex'), archive('opencode'), archive('opencode')];
    const f = await fixture(t, sessions, ({ id, action, session }) => {
      if (!action && id === sessions[0].sessionId && !session.archived) return { status: 200, body: { sessionId: id, cwd: session.cwd, runtimeId: session.runtimeId, mode: 'automation' } };
      if (action === 'mode' && id === sessions[1].sessionId) session.runtimeId = randomUUID();
      if (action === 'mode' && id === sessions[2].sessionId) return { status: 200, body: {} };
    });
    const result = await restoreSessions(f.options);
    assert.deepEqual(result.results.map(r => r.status), ['failed', 'failed', 'failed', 'restored']);
    assert.deepEqual(result.results.slice(0, 3).map(r => r.stage), ['registration', 'automation', 'automation']);
    assert.equal(f.calls.some(c => c.id === sessions[0].sessionId && c.action === 'mode'), false);
  });

  test('durable attempts survive repeated startup and a new helper client, but not a new container instance', async t => {
    const sessions = [archive(), archive('codex'), archive('opencode')];
    const f = await fixture(t, sessions, ({ action }) => action === 'resume' ? { status: 409, body: { error: secret } } : undefined);
    await restoreSessions(f.options);
    const repeated = await restoreSessions({ ...f.options, client: createClient(f.options.descriptor) });
    assert.deepEqual(repeated.results.map(r => r.reason), ['already_attempted', 'already_attempted', 'already_attempted']);
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 3);
    await restoreSessions({ ...f.options, instance: 'b'.repeat(64) });
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 6);
    assert.equal((await readdir(f.state)).length, 2);
    f.options.descriptor.token = randomBytes(32).toString('hex');
    await restoreSessions({ ...f.options, client: createClient(f.options.descriptor) });
    assert.equal(f.calls.filter(c => c.action === 'resume').length, 9, 'a newly authenticated broker has its own restoration guard');
    assert.equal((await readdir(f.state)).length, 3);
  });

  test('incomplete, mismatched, public or symlinked attempt records fail closed', async t => {
    for (const corrupt of ['invalid', 'incomplete', 'mismatch', 'public', 'symlink']) {
      const sessions = [archive()];
      const f = await fixture(t, sessions, ({ action }) => action === 'resume' ? { status: 409, body: { error: secret } } : undefined);
      await restoreSessions(f.options);
      const [name] = await readdir(f.state);
      const path = join(f.state, name);
      const original = await readFile(path, 'utf8');
      if (corrupt === 'invalid') await writeFile(path, secret);
      if (corrupt === 'incomplete') await writeFile(path, '{}');
      if (corrupt === 'mismatch') await writeFile(path, JSON.stringify({ ...JSON.parse(original), instance: 'b'.repeat(64) }));
      if (corrupt === 'public') await chmod(path, 0o644);
      if (corrupt === 'symlink') {
        const target = join(f.state, 'target');
        await writeFile(target, original, { mode: 0o600 });
        await rm(path);
        await symlink(target, path);
      }
      await assert.rejects(restoreSessions(f.options), error => !error.message.includes(secret));
      assert.equal(f.calls.filter(c => c.action === 'resume').length, 1, corrupt);
    }
  });
});

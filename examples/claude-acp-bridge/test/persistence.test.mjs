import assert from 'node:assert/strict';
import { fork } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { watch } from 'node:fs';
import { chmod, link, mkdir, mkdtemp, open, readFile, readdir, rm, stat, symlink, unlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, test } from 'node:test';
import { createBroker } from '../src/broker.mjs';
import { createClient } from '../src/client.mjs';
import { MCP_BLOCKED_REASON } from '../src/native-discovery.mjs';
import { openSessionStore, sessionRecord, SESSION_LIMITS } from '../src/session-store.mjs';

const secret = 'credential-must-not-appear-in-diagnostics';
const runtime = (overrides = {}) => ({ sessionId: randomUUID(), runtimeId: randomUUID(), cwd: '/workspace', mode: 'automation', activeTurnId: null, events: [], ...overrides });
const event = (seq, text = 'private user text') => ({ seq, type: 'text', data: { text } });
const record = (overrides = {}) => ({ sessionId: randomUUID(), runtimeId: 'native-runtime', cwd: '/workspace', startedAt: 0, cursor: 1, historyStartCursor: 0, events: [event(1)], fault: false, ...overrides });
const envelope = sessions => ({ version: 1, sessions });

async function fixture(t, options = {}) {
  const directory = await mkdtemp('/tmp/opencode/bridge-persistence-');
  const sessionFile = join(directory, 'sessions.json');
  const brokers = new Set();
  t.after(async () => {
    for (const broker of brokers) await broker.close();
    await rm(directory, { recursive: true, force: true });
  });
  async function start(extra = {}) {
    const broker = await createBroker({ sessionFile, ...options, ...extra });
    brokers.add(broker);
    return { broker, client: createClient(broker) };
  }
  async function stop(broker) { brokers.delete(broker); await broker.close(); }
  const disk = async () => JSON.parse(await readFile(sessionFile, 'utf8'));
  return { directory, sessionFile, start, stop, disk };
}

function clock() {
  let time = 0;
  const timers = new Map();
  return {
    now: () => time,
    setTimer(callback, delay) { const id = Symbol(); timers.set(id, { callback, at: time + delay }); return id; },
    clearTimer(id) { timers.delete(id); },
    advance(ms) {
      time += ms;
      for (const [id, timer] of [...timers]) if (timer.at <= time && timers.delete(id)) timer.callback();
    },
  };
}

function assertArchive(s, original) {
  assert.equal(s.sessionId, original.sessionId);
  assert.equal(s.runtimeId, original.runtimeId);
  assert.equal(s.cwd, original.cwd);
  assert.equal(s.archived, true);
  assert.equal(s.ended, true);
  assert.equal(s.mode, 'human');
  assert.equal(s.stale, false);
  assert.equal(s.registrationPending, false);
  assert.equal(s.activeTurnId, null);
}

describe('private session persistence', { concurrency: true, timeout: 20000 }, () => {
  test('acknowledgements are durable, reopen is inert, and only transcript and safe metadata are saved', async t => {
    const f = await fixture(t, { now: () => 123 });
    const { broker, client } = await f.start();
    const state = runtime({ token: secret, providerEnv: { ANTHROPIC_API_KEY: secret }, commands: [secret], launchId: randomUUID() });
    const events = [
      { seq: 1, type: 'turn.start', data: { turnId: 't', text: 'private user prompt' } },
      event(2, 'private assistant answer'),
    ];
    assert.equal((await client.request('POST', '/v1/runtime', { ...state, events })).ack, 2);
    const saved = (await f.disk()).sessions[0];
    assert.equal(saved.startedAt, 123);
    assert.deepEqual(saved.events, events, 'history is present when the acknowledgement arrives, without close');
    assert.equal((await stat(f.sessionFile)).mode & 0o777, 0o600);
    const requestId = randomUUID();
    await client.request('POST', '/v1/sessions/' + state.sessionId + '/prompt', { requestId, text: 'queued payload not a transcript' });
    const raw = await readFile(f.sessionFile, 'utf8');
    for (const value of [broker.token, secret, 'providerEnv', 'commands', 'launchId', requestId, 'queued payload']) assert.ok(!raw.includes(value), value);
    assert.ok(raw.includes('private user prompt'), 'the transcript is private, not redacted user content');
    await f.stop(broker);
    let launches = 0;
    const restored = await f.start({ launch: () => { launches++; }, discover: async () => [{ ...state, startedAt: 999, ended: false }] });
    const path = '/v1/sessions/' + state.sessionId;
    const s = await restored.client.request('GET', path);
    assertArchive(s, state);
    assert.equal(s.startedAt, 123);
    assert.deepEqual((await restored.client.request('GET', '/v1/sessions')).sessions, [s]);
    assert.deepEqual(await restored.client.request('GET', path + '/events?after=0'), { events, cursor: 2 });
    assert.deepEqual(await restored.client.request('GET', path + '/events?after=2'), { events: [], cursor: 2 });
    for (const body of [state, { ...state, runtimeId: randomUUID() }]) await assert.rejects(
      restored.client.request('POST', '/v1/runtime', body), error => error.status === 409 && /Archived.*read-only/.test(error.message));
    for (const [action, body] of [['prompt', { requestId, text: 'queued payload not a transcript' }], ['mode', { mode: 'automation' }], ['mode', { mode: 'human' }], ['cancel', { requestId }]]) {
      await assert.rejects(restored.client.request('POST', path + '/' + action, body), /Archived.*read-only/);
    }
    assert.equal(launches, 0);
    assert.deepEqual(await f.disk(), envelope([saved]));
  });

  test('empty first registration persists, but pure reads and empty or duplicate heartbeats do not rewrite', async t => {
    const timer = clock();
    const f = await fixture(t, timer);
    const { client } = await f.start();
    const state = runtime();
    await client.request('POST', '/v1/runtime', state);
    assert.equal((await f.disk()).sessions[0].cursor, 0);
    await client.request('POST', '/v1/runtime', { ...state, events: [event(1)] });
    const before = await stat(f.sessionFile, { bigint: true });
    const content = await readFile(f.sessionFile);
    timer.advance(20000);
    for (const path of ['/v1/sessions', '/v1/sessions/' + state.sessionId, '/v1/sessions/' + state.sessionId + '/events?after=0']) await client.request('GET', path);
    await client.request('POST', '/v1/runtime', state);
    await client.request('POST', '/v1/runtime', { ...state, events: [event(1)] });
    const after = await stat(f.sessionFile, { bigint: true });
    assert.equal(after.ino, before.ino);
    assert.equal(after.mtimeNs, before.mtimeNs);
    assert.deepEqual(await readFile(f.sessionFile), content);
    assert.equal((await client.request('GET', '/v1/sessions/' + state.sessionId)).lastSeen, timer.now());
    await client.request('POST', '/v1/runtime', { ...state, cwd: '/other' });
    assert.equal((await f.disk()).sessions[0].cwd, '/other');
  });

  test('only retained current-runtime history survives with an explicit partial-history boundary', async t => {
    const f = await fixture(t, { maxEvents: 2, maxJournalBytes: 300, maxTotalJournalBytes: 350 });
    const { broker, client } = await f.start();
    const state = runtime();
    await client.request('POST', '/v1/runtime', { ...state, events: [event(1, 'old runtime')] });
    state.runtimeId = randomUUID();
    await client.request('POST', '/v1/runtime', { ...state, events: [event(1), event(2), event(3)] });
    const saved = (await f.disk()).sessions[0];
    assert.equal(saved.runtimeId, state.runtimeId);
    assert.equal(saved.cursor, 5);
    assert.equal(saved.historyStartCursor, 3);
    assert.deepEqual(saved.events.map(e => e.seq), [4, 5]);
    assert.ok(!JSON.stringify(saved).includes('old runtime'));
    assert.ok(!saved.events.some(e => e.type === 'runtime.replaced'));
    // Global byte eviction can expire another session's entire retained history.
    const other = runtime({ events: [event(1, '😀'.repeat(55))] });
    await client.request('POST', '/v1/runtime', other);
    const records = (await f.disk()).sessions;
    const first = records.find(s => s.sessionId === state.sessionId);
    assert.ok(first.events.length < 2);
    assert.equal(first.historyStartCursor, (first.events[0]?.seq ?? first.cursor + 1) - 1);
    await f.stop(broker);
    const restored = await f.start();
    for (const saved of records) {
      const path = '/v1/sessions/' + saved.sessionId;
      const s = await restored.client.request('GET', path);
      assert.equal(s.historyStartCursor, saved.historyStartCursor);
      assert.deepEqual(await restored.client.request('GET', path + '/events?after=' + s.historyStartCursor), { events: saved.events, cursor: saved.cursor });
    }
  });

  test('pending and ended discovery sessions persist without native configuration or relaunch', async t => {
    const native = { sessionId: randomUUID(), launchId: randomUUID(), cwd: '/workspace', startedAt: 0, ended: false, blockedReason: MCP_BLOCKED_REASON, providerEnv: { key: secret } };
    const other = { ...native, sessionId: randomUUID(), ended: true };
    const f = await fixture(t, { discover: async () => [native, other] });
    const { broker, client } = await f.start();
    assert.equal((await client.request('GET', '/v1/sessions')).sessions.length, 2);
    assert.equal((await f.disk()).sessions.length, 2);
    assert.ok(!(await readFile(f.sessionFile, 'utf8')).includes(secret));
    await f.stop(broker);
    const restored = await f.start({ launch: () => assert.fail('must not relaunch'), discover: async () => [{ ...native, launchId: randomUUID(), cwd: '/different' }] });
    const list = (await restored.client.request('GET', '/v1/sessions')).sessions;
    assert.equal(list.length, 2);
    for (const s of list) {
      assertArchive(s, { ...native, sessionId: s.sessionId });
      assert.equal(s.startedAt, 0);
      assert.equal(s.blockedReason, undefined);
    }
  });

  test('errors and terminal state are durable before an error response and survive journal eviction', async t => {
    const f = await fixture(t, { maxEvents: 1, now: () => 100 });
    const { broker, client } = await f.start();
    const state = runtime();
    await client.request('POST', '/v1/runtime', { ...state, events: [event(1)] });
    await assert.rejects(client.request('POST', '/v1/runtime', { ...state, events: [event(3)] }), /event gap/);
    const saved = (await f.disk()).sessions[0];
    assert.equal(saved.fault, true);
    assert.deepEqual(saved.lastError, { message: 'Runtime event gap; work state is unknown', at: 100 });
    assert.equal(saved.events[0].type, 'overflow');
    await client.request('POST', '/v1/runtime', { ...state, events: [event(2)] });
    await f.stop(broker);
    const restored = await f.start();
    const s = await restored.client.request('GET', '/v1/sessions/' + state.sessionId);
    assert.equal(s.fault, true);
    assert.deepEqual(s.lastError, saved.lastError);
    assert.equal((await f.disk()).sessions[0].events[0].type, 'text');
  });

  test('timer faults flush without another request and close awaits their durability', async t => {
    const timer = clock();
    const f = await fixture(t, timer);
    const { broker, client } = await f.start();
    const state = runtime();
    await client.request('POST', '/v1/runtime', state);
    await client.request('POST', '/v1/sessions/' + state.sessionId + '/prompt', { requestId: randomUUID(), text: 'not delivered' });
    const changed = new Promise((resolve, reject) => {
      const watcher = watch(f.directory, (type, name) => {
        if (type === 'rename' && name === 'sessions.json') { watcher.close(); resolve(); }
      });
      watcher.on('error', reject);
      t.after(() => watcher.close());
    });
    timer.advance(10000);
    await changed;
    assert.equal((await f.disk()).sessions[0].fault, true, 'timer mutation reaches disk without HTTP or close');
    await f.stop(broker);
    const saved = (await f.disk()).sessions[0];
    assert.equal(saved.fault, true);
    assert.match(saved.lastError.message, /acknowledgement timed out/);
    assert.equal(saved.events[0].type, 'overflow');
  });

  test('a command that times out during persistence is not delivered by an older heartbeat response', async t => {
    const timer = clock();
    const f = await fixture(t, timer);
    const { client } = await f.start();
    const state = runtime();
    await client.request('POST', '/v1/runtime', state);
    await client.request('POST', '/v1/sessions/' + state.sessionId + '/prompt', { requestId: randomUUID(), text: 'must not deliver after timeout' });
    let expired = false;
    const watcher = watch(f.directory, (type, name) => {
      if (type === 'rename' && name === 'sessions.json.tmp' && !expired) {
        expired = true;
        timer.advance(10000);
      }
    });
    t.after(() => watcher.close());
    const result = await client.request('POST', '/v1/runtime', { ...state, events: [event(1)] });
    assert.equal(expired, true);
    assert.deepEqual(result, { ack: 1, commands: [] });
    assert.equal((await f.disk()).sessions[0].fault, true);
  });

  test('concurrent acknowledgements serialize durable snapshots without losing sessions', async t => {
    const f = await fixture(t);
    const { client } = await f.start();
    const states = Array.from({ length: 20 }, () => runtime({ events: [event(1)] }));
    await Promise.all(states.map(async state => {
      await client.request('POST', '/v1/runtime', state);
      assert.ok((await f.disk()).sessions.some(s => s.sessionId === state.sessionId && s.cursor === 1));
    }));
    assert.equal((await f.disk()).sessions.length, states.length);
    assert.deepEqual(await readdir(f.directory), ['sessions.json']);
  });

  test('adding a session after restart preserves every earlier session and transcript through another restart', async t => {
    const f = await fixture(t);
    const first = await f.start();
    const states = [runtime({ events: [event(1, 'first transcript')] }), runtime({ events: [event(1, 'second transcript')] })];
    for (const state of states) await first.client.request('POST', '/v1/runtime', state);
    const before = (await f.disk()).sessions;
    await f.stop(first.broker);

    const second = await f.start();
    const added = runtime({ events: [event(1, 'third transcript')] });
    await second.client.request('POST', '/v1/runtime', added);
    const saved = (await f.disk()).sessions;
    assert.equal(saved.length, 3);
    assert.deepEqual(saved.slice(0, 2), before);
    await f.stop(second.broker);

    const third = await f.start();
    const listed = (await third.client.request('GET', '/v1/sessions')).sessions;
    assert.equal(listed.length, 3);
    for (const [index, state] of [...states, added].entries()) {
      assertArchive(listed[index], state);
      assert.equal(listed[index].startedAt, saved[index].startedAt);
      const history = await third.client.request('GET', '/v1/sessions/' + state.sessionId + '/events?after=0');
      assert.deepEqual(history.events, state.events);
    }
    assert.deepEqual((await f.disk()).sessions, saved);
  });

  test('session budget remains bounded across restarts and does not recommend restarting', async t => {
    const f = await fixture(t);
    const store = await openSessionStore(f.sessionFile);
    await store.save(Array.from({ length: 100 }, () => record()));
    const { client } = await f.start({ launch: () => assert.fail('must reject before native launch') });
    await assert.rejects(client.request('POST', '/v1/sessions', { cwd: '/workspace' }), /archive budget exhausted/);
    await assert.rejects(client.request('POST', '/v1/runtime', runtime()), error => /archive budget exhausted/.test(error.message) && !/restart/.test(error.message));
    assert.equal((await client.request('GET', '/v1/sessions')).sessions.length, 100);
    assert.equal((await f.disk()).sessions.length, 100);
  });

  test('write failures are explicit and sanitized, never acknowledge history or overwrite a changed archive', async t => {
    for (const reason of ['changed archive', 'temporary symlink']) await t.test(reason, async t => {
      const f = await fixture(t);
      const { broker, client } = await f.start();
      const state = runtime({ events: [event(1)] });
      await client.request('POST', '/v1/runtime', state);
      const original = await readFile(f.sessionFile, 'utf8');
      const target = join(f.directory, secret);
      if (reason === 'changed archive') await writeFile(f.sessionFile, secret, { mode: 0o600 });
      else {
        await writeFile(target, secret, { mode: 0o600 });
        await symlink(target, f.sessionFile + '.tmp');
      }
      const safeFailure = error => error.status === 503 && /Session persistence/.test(error.message) && !error.message.includes(secret);
      await assert.rejects(client.request('POST', '/v1/runtime', { ...state, events: [event(2)] }), safeFailure);
      await assert.rejects(client.request('POST', '/v1/runtime', { ...state, events: [] }), safeFailure);
      await assert.rejects(f.stop(broker), safeFailure);
      assert.equal(await readFile(f.sessionFile, 'utf8'), reason === 'changed archive' ? secret : original);
      if (reason === 'temporary symlink') assert.equal(await readFile(target, 'utf8'), secret);
    });
  });

  test('invalid schemas, corruption and oversized files are refused without overwriting them', async t => {
    const base = record();
    const invalid = [
      secret, JSON.stringify({ version: 2, sessions: [] }), JSON.stringify({ ...envelope([]), token: secret }),
      ...[
        [{ ...base, commands: [] }], [base, base], [{ ...base, cwd: 'relative' }], [{ ...base, startedAt: -1 }],
        [{ ...base, runtimeId: {} }], [{ ...base, cursor: 0 }], [{ ...base, historyStartCursor: 1 }],
        [{ ...base, lastError: { message: secret, at: 'wrong' } }], [{ ...base, events: [event(1), event(3)], cursor: 3 }],
        Array.from({ length: 101 }, () => record()), [{ ...base, events: [event(1, 'x'.repeat(SESSION_LIMITS.eventBytes + 64))] }],
        [{ ...base, cwd: '/' + 'x'.repeat(32 * 1024) }],
      ].map(sessions => JSON.stringify(envelope(sessions))),
    ];
    const f = await fixture(t);
    for (const raw of invalid) {
      await writeFile(f.sessionFile, raw, { mode: 0o600 });
      await assert.rejects(f.start(), error => /Session persistence/.test(error.message) && !error.message.includes(secret));
      assert.equal(await readFile(f.sessionFile, 'utf8'), raw);
    }
    const handle = await open(f.sessionFile, 'w', 0o600);
    await handle.truncate(SESSION_LIMITS.fileBytes + 1);
    await handle.close();
    await assert.rejects(f.start(), /file budget/);
    assert.equal((await stat(f.sessionFile)).size, SESSION_LIMITS.fileBytes + 1);
  });

  test('event count, per-session and total journal budgets are enforced on disk', async t => {
    const f = await fixture(t);
    const sized = (count, length) => record({ cursor: count, events: Array.from({ length: count }, (_, i) => event(i + 1, 'x'.repeat(length))) });
    for (const sessions of [[sized(10001, 0)], [sized(65, 128 * 1024 - 200)], Array.from({ length: 5 }, () => sized(60, 128 * 1024 - 200))]) {
      await writeFile(f.sessionFile, JSON.stringify(envelope(sessions)), { mode: 0o600 });
      await assert.rejects(f.start(), /Session persistence/);
    }
    await unlink(f.sessionFile);
    const store = await openSessionStore(f.sessionFile);
    // A near-cap snapshot includes JSON framing and metadata in the file budget.
    await store.save(Array.from({ length: 4 }, () => sized(60, 128 * 1024 - 200)));
    assert.ok((await stat(f.sessionFile)).size < SESSION_LIMITS.fileBytes);
    assert.equal((await openSessionStore(f.sessionFile)).records.length, 4);
  });

  test('private owner-only paths reject symlinks, hardlinks and permissive files or directories', async t => {
    for (const kind of ['file symlink', 'parent symlink', 'hardlink', 'file permissions', 'directory permissions']) await t.test(kind, async t => {
      const f = await fixture(t);
      const target = join(f.directory, 'target');
      await writeFile(target, JSON.stringify(envelope([])), { mode: 0o600 });
      let path = f.sessionFile;
      if (kind === 'file symlink') await symlink(target, path);
      if (kind === 'hardlink') await link(target, path);
      if (kind === 'parent symlink') { await symlink(f.directory, join(f.directory, 'redirect')); path = join(f.directory, 'redirect', 'target'); }
      if (kind === 'file permissions') { await writeFile(path, '{}', { mode: 0o644 }); await chmod(path, 0o644); }
      if (kind === 'directory permissions') await chmod(f.directory, 0o755);
      await assert.rejects(openSessionStore(path), /Session persistence/);
      assert.equal(await readFile(target, 'utf8'), JSON.stringify(envelope([])));
    });
  });

  test('deferred brokers cannot serve requests before exclusively owned persistence preparation', async t => {
    const f = await fixture(t, { deferPersistence: true });
    const store = await openSessionStore(f.sessionFile);
    await store.save([record()]);
    const original = await readFile(f.sessionFile, 'utf8');
    await writeFile(f.sessionFile + '.tmp', '{"partial":', { mode: 0o600 });
    const { broker, client } = await f.start();
    await assert.rejects(client.request('GET', '/v1/sessions'), /preparation is not complete/);
    await assert.rejects(client.request('POST', '/v1/runtime', runtime()), /preparation is not complete/);
    await writeFile(join(f.directory, 'descriptor.json'), JSON.stringify(broker), { flag: 'wx', mode: 0o600 });
    await broker.preparePersistence();
    await assert.rejects(stat(f.sessionFile + '.tmp'), error => error.code === 'ENOENT');
    assert.equal(await readFile(f.sessionFile, 'utf8'), original);
    assert.equal((await client.request('GET', '/v1/sessions')).sessions[0].archived, true);
  });

  test('temporary recovery refuses changed primary snapshots and unsafe temporary files', async t => {
    for (const kind of ['changed primary', 'public temporary', 'hardlinked temporary']) await t.test(kind, async t => {
      const f = await fixture(t);
      const store = await openSessionStore(f.sessionFile);
      await store.save([record()]);
      const temporary = f.sessionFile + '.tmp';
      await writeFile(temporary, 'partial', { mode: 0o600 });
      if (kind === 'changed primary') await writeFile(f.sessionFile, 'corrupt primary', { mode: 0o600 });
      if (kind === 'public temporary') await chmod(temporary, 0o644);
      if (kind === 'hardlinked temporary') await link(temporary, join(f.directory, 'other-link'));
      const original = await readFile(f.sessionFile, 'utf8');
      await assert.rejects(store.discardInterruptedTemporary(), /Session persistence/);
      assert.equal(await readFile(temporary, 'utf8'), 'partial');
      assert.equal(await readFile(f.sessionFile, 'utf8'), original);
    });
  });

  test('archive projection excludes previous runtimes and transient work maps', () => {
    const s = record({ cursor: 3, historyStartCursor: 2, events: [event(1), { seq: 2, type: 'runtime.replaced', data: {} }, event(3)] });
    const saved = sessionRecord({ ...s, commands: new Map([['x', secret]]), providerEnv: secret, token: secret, lastError: { message: 'Native tool call failed; inspect the transcript', at: 10, raw: secret } });
    assert.deepEqual(saved.events, [event(3)]);
    assert.equal(saved.historyStartCursor, 2);
    assert.ok(!JSON.stringify(saved).includes(secret));
  });
});

test('CLI restart recovers interrupted temporary writes only under exclusive descriptor ownership', { timeout: 15000 }, async t => {
  const directory = await mkdtemp('/tmp/opencode/bridge-persistence-cli-');
  const stateFile = join(directory, 'bridge.json');
  const cli = fileURLToPath(new URL('../src/cli.mjs', import.meta.url));
  const children = new Set();
  let output = '';
  t.after(async () => {
    for (const child of children) {
      if (child.exitCode !== null || child.signalCode !== null) continue;
      const exited = once(child, 'exit');
      child.kill('SIGKILL');
      await exited;
    }
    await rm(directory, { recursive: true, force: true });
  });
  async function start(expectReady = true) {
    const bin = join(directory, 'bin');
    await mkdir(bin, { recursive: true, mode: 0o700 });
    // Discovery is advisory. No real native Claude command may run in this test.
    await writeFile(join(bin, 'claude'), '#!/bin/sh\nexit 1\n', { mode: 0o700 });
    const child = fork(cli, ['serve', '--state', stateFile], { execArgv: [], env: { ...process.env, HOME: directory, PATH: bin }, stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    children.add(child);
    child.stdout.on('data', chunk => { output += chunk; });
    child.stderr.on('data', chunk => { output += chunk; });
    if (!expectReady) {
      let ready = false;
      child.on('message', () => { ready = true; });
      const [code] = await once(child, 'exit');
      assert.equal(code, 1);
      assert.equal(ready, false, 'a failed preparation must never signal readiness');
      return;
    }
    await new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', () => reject(new Error('broker exited before readiness: ' + output)));
      child.once('message', message => { assert.equal(message.type, 'bridge-ready'); resolve(); });
    });
    return { child, client: createClient(JSON.parse(await readFile(stateFile, 'utf8'))) };
  }
  const live = await start();
  const state = runtime({ events: [event(1)] });
  await live.client.request('POST', '/v1/runtime', state);
  const sessionFile = stateFile + '.sessions.json';
  const temporary = sessionFile + '.tmp';
  const original = await readFile(sessionFile, 'utf8');
  const saved = JSON.parse(original);
  assert.deepEqual(saved.sessions[0].events, state.events);
  // Simulate a killed writer leaving an incomplete, never-acknowledged snapshot.
  const partial = '{"version":1,"sessions":[';
  await writeFile(temporary, partial, { flag: 'wx', mode: 0o600 });
  const descriptor = await readFile(stateFile, 'utf8');
  await start(false);
  assert.equal(await readFile(temporary, 'utf8'), partial, 'a competing serve must not clean up another owner\'s temporary');
  assert.equal(await readFile(stateFile, 'utf8'), descriptor);
  const exited = once(live.child, 'exit');
  live.child.kill('SIGKILL');
  await exited;
  // A stale descriptor is explicitly removed by the test, never by the broker.
  await unlink(stateFile);
  const restored = await start();
  assertArchive((await restored.client.request('GET', '/v1/sessions')).sessions[0], state);
  await assert.rejects(restored.client.request('POST', '/v1/runtime', state), /Archived.*read-only/);
  assert.equal(await readFile(sessionFile, 'utf8'), original, 'recovery leaves the valid primary snapshot unchanged');
  await assert.rejects(stat(temporary), error => error.code === 'ENOENT');
  const next = runtime({ events: [event(1, 'new session after recovery')] });
  await restored.client.request('POST', '/v1/runtime', next);
  assert.equal(JSON.parse(await readFile(sessionFile, 'utf8')).sessions.length, 2, 'new history can be durably written after recovery');
  const stopped = once(restored.child, 'exit');
  restored.child.kill('SIGTERM');
  await stopped;
  await assert.rejects(stat(stateFile), error => error.code === 'ENOENT');

  const retained = await readFile(sessionFile, 'utf8');
  const target = join(directory, 'private-target');
  await writeFile(target, secret, { mode: 0o600 });
  await symlink(target, temporary);
  await start(false);
  assert.equal(await readFile(target, 'utf8'), secret);
  assert.equal(await readFile(sessionFile, 'utf8'), retained);
  await assert.rejects(stat(stateFile), error => error.code === 'ENOENT', 'failed preparation removes only its own descriptor');
  assert.ok(!output.includes(secret));
  assert.match(output, /Session persistence/);

  await unlink(temporary);
  await writeFile(temporary, partial, { flag: 'wx', mode: 0o600 });
  await writeFile(sessionFile, 'corrupt primary', { mode: 0o600 });
  await start(false);
  assert.equal(await readFile(temporary, 'utf8'), partial);
  assert.equal(await readFile(sessionFile, 'utf8'), 'corrupt primary');
});

import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createServer } from 'node:net';
import { createInterface } from 'node:readline';
import { promisify } from 'node:util';
import { EventEmitter, once } from 'node:events';
import { watch } from 'node:fs';
import { chmod, link, lstat, mkdir, mkdtemp, readFile, readlink, rm, symlink, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { describe, test } from 'node:test';
import { agent, methods, ndJsonStream, PROTOCOL_VERSION } from '@agentclientprotocol/sdk';
import { runJob, startDetachedJob } from '../src/job.mjs';

function deferred() {
  let resolve;
  const promise = new Promise(yes => { resolve = yes; });
  return { promise, resolve };
}

async function fixture(t, text = 'literal $(touch /never-created); `false`\n"quotes" \'single\' $TOKEN') {
  const directory = await mkdtemp('/tmp/opencode/bridge-job-test-');
  t.after(() => rm(directory, { recursive: true, force: true }));
  const promptFile = join(directory, 'prompt.txt');
  const jobFile = join(directory, 'job.json');
  await writeFile(promptFile, text, { mode: 0o600 });
  return { directory, text, promptFile, jobFile, cwd: directory, stateFile: join(directory, 'bridge.json') };
}

function fakeACP(t, options = {}) {
  const calls = [];
  const events = new EventEmitter();
  const completed = deferred();
  let server;
  let disconnect;
  let connects = 0;
  const sessionId = 'native-session-123';
  function call(method, params) {
    calls.push({ method, params });
    events.emit('call');
  }
  const app = agent({ name: 'fake-job-agent' });
  app.onRequest(methods.agent.initialize, ({ params }) => {
    call('initialize', params);
    return options.initialize?.(params) ?? { protocolVersion: PROTOCOL_VERSION, agentCapabilities: {} };
  });
  app.onRequest(methods.agent.session.new, async ({ params }) => {
    call('new', params);
    return options.newSession ? options.newSession(params) : {
      sessionId,
      modes: { currentModeId: 'human', availableModes: [{ id: 'human', name: 'Human' }, { id: 'automation', name: 'Automation' }] },
    };
  });
  app.onRequest(methods.agent.session.load, ({ params }) => { call('load', params); return options.loadSession(params); });
  app.onRequest(methods.agent.session.setMode, ({ params }) => { call('mode', params); return {}; });
  app.onRequest(methods.agent.session.prompt, async ({ params }) => {
    call('prompt', params);
    if (options.prompt) return options.prompt(params, server);
    return completed.promise;
  });
  app.onNotification(methods.agent.session.cancel, ({ params }) => call('cancel', params));
  const createConnection = client => {
    connects++;
    const toAgent = new TransformStream();
    const fromAgent = new TransformStream();
    server = app.connect(ndJsonStream(fromAgent.writable, toAgent.readable));
    const connection = client.connect(ndJsonStream(toAgent.writable, fromAgent.readable));
    disconnect = async () => {
      server.close();
      await server.closed;
      await fromAgent.writable.close();
    };
    return {
      connection,
      async dispose() {
        connection.close();
        await toAgent.writable.close().catch(error => { if (error.code !== 'ERR_INVALID_STATE') throw error; });
        await server.closed;
      },
    };
  };
  t.after(() => { completed.resolve({ stopReason: 'end_turn' }); server?.close(); });
  return {
    sessionId, calls, createConnection,
    get connects() { return connects; },
    complete: (stopReason = 'end_turn') => completed.resolve({ stopReason }),
    disconnect: () => disconnect(),
    async called(method) {
      while (!calls.some(call => call.method === method)) await once(events, 'call');
      return calls.find(call => call.method === method).params;
    },
  };
}

const readJob = async file => JSON.parse(await readFile(file, 'utf8'));

describe('noninteractive ACP jobs', { concurrency: true, timeout: 10000 }, () => {
  test('uses SDK framing, literal text, automation mode and durable dispatch readiness before completion', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t);
    const ready = deferred();
    let finished = false;
    const running = runJob({ ...f, createConnection: acp.createConnection, onReady: ready.resolve });
    void running.then(() => { finished = true; });
    const record = await ready.promise;
    assert.equal(record.status, 'prompting');
    assert.equal(record.sessionId, acp.sessionId);
    assert.ok(record.dispatchAttemptedAt);
    assert.equal(finished, false);
    assert.deepEqual(await readJob(f.jobFile), record);
    assert.deepEqual(await acp.called('prompt'), { sessionId: acp.sessionId, prompt: [{ type: 'text', text: f.text }] });
    assert.deepEqual(acp.calls.map(call => call.method), ['initialize', 'new', 'mode', 'prompt']);
    assert.deepEqual(acp.calls[0].params.clientCapabilities, { fs: { readTextFile: false, writeTextFile: false }, terminal: false, auth: { terminal: false } });
    assert.deepEqual(acp.calls[1].params, { cwd: f.cwd, mcpServers: [] });
    assert.deepEqual(acp.calls[2].params, { sessionId: acp.sessionId, modeId: 'automation' });
    acp.complete();
    const final = await running;
    assert.equal(final.status, 'completed');
    assert.equal(final.stopReason, 'end_turn');
    assert.ok(final.completedAt);
    assert.equal(final.jobId, record.jobId);
    assert.deepEqual(await readJob(f.jobFile), final);
    assert.equal((await lstat(f.jobFile)).mode & 0o777, 0o600);
    assert.ok(!JSON.stringify(final).includes(f.text));
    assert.match(final.inputHash, /^[a-f0-9]{64}$/);
    assert.equal(acp.calls.filter(call => call.method === 'cancel').length, 0);
    assert.deepEqual(await runJob({ ...f, createConnection: acp.createConnection }), final);
    assert.equal(acp.connects, 1, 'matching retry does not even connect');
  });

  test('exclusive guard precedes session/new and concurrent invocations cannot replay', async t => {
    const f = await fixture(t);
    const created = deferred();
    const acp = fakeACP(t, { newSession: async () => {
      const guard = await readJob(f.jobFile);
      assert.equal(guard.status, 'starting');
      assert.equal(guard.sessionId, undefined);
      return created.promise;
    } });
    const running = runJob({ ...f, createConnection: acp.createConnection });
    await acp.called('new');
    const retry = await runJob({ ...f, createConnection: acp.createConnection });
    assert.equal(retry.status, 'starting');
    assert.equal(retry.sessionId, undefined);
    assert.equal(acp.connects, 1);
    await writeFile(f.promptFile, 'different prompt');
    await assert.rejects(runJob({ ...f, createConnection: acp.createConnection }), /different input/);
    created.resolve({ sessionId: acp.sessionId });
    await acp.called('prompt');
    acp.complete();
    assert.equal((await running).status, 'completed');
    assert.equal(acp.calls.filter(call => call.method === 'new').length, 1);
  });

  test('harness selection preserves the old hash, accepts opaque native IDs, and never replays a changed harness', async t => {
    for (const harness of [undefined, 'claude', 'codex', 'opencode']) {
      const f = await fixture(t);
      const id = harness === 'opencode' ? 'opencode:ses_test123' : harness === 'codex' ? 'codex:ad72142c-b0ed-474c-9e9e-237345db5784' : 'native-session-123';
      const acp = fakeACP(t, { newSession: () => ({ sessionId: id }), prompt: () => ({ stopReason: 'end_turn' }) });
      const record = await runJob({ ...f, harness, createConnection: acp.createConnection });
      assert.equal(record.harness, harness ?? 'claude');
      assert.equal(record.sessionId, id);
      assert.equal(record.status, 'completed');
      assert.equal(record.inputHash, createHash('sha256').update(JSON.stringify([f.cwd, f.text])).digest('hex'));
      assert.deepEqual(await acp.called('new'), { cwd: f.cwd, mcpServers: [],
        ...(harness && harness !== 'claude' ? { _meta: { 'coder-acp': { harness } } } : {}),
      });
      // Simulate a retained guard written before the optional harness field existed.
      if (harness === undefined) delete record.harness;
      await writeFile(f.jobFile, JSON.stringify(record) + '\n');
      const original = await readFile(f.jobFile, 'utf8');
      for (const run of [runJob, startDetachedJob]) {
        assert.deepEqual(await run({ ...f, harness: harness ?? 'claude', createConnection: acp.createConnection }), record);
        await assert.rejects(run({ ...f, harness: harness === 'codex' ? 'claude' : 'codex', createConnection: acp.createConnection }), /different input|different harness/);
        assert.equal(await readFile(f.jobFile, 'utf8'), original);
      }
      assert.equal(acp.connects, 1);
    }
  });

  test('invalid harness input and retained harness fields fail closed without overwriting or connecting', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t, { prompt: () => ({ stopReason: 'end_turn' }) });
    for (const harness of ['', null, 'unknown', 1]) {
      for (const run of [runJob, startDetachedJob]) {
        await assert.rejects(run({ ...f, harness, createConnection: acp.createConnection }), /harness/);
        await assert.rejects(lstat(f.jobFile), { code: 'ENOENT' });
      }
    }
    assert.equal(acp.connects, 0);
    const record = await runJob({ ...f, createConnection: acp.createConnection });
    for (const harness of ['', null, 'unknown', 1]) {
      const original = JSON.stringify({ ...record, harness }) + '\n';
      await writeFile(f.jobFile, original);
      await assert.rejects(runJob({ ...f, createConnection: acp.createConnection }), /Invalid job file/);
      assert.equal(await readFile(f.jobFile, 'utf8'), original);
    }
    assert.equal(acp.connects, 1);
  });

  test('generic opaque session IDs and literal UTF-8 prompts need no bridge metadata or mode extension', async t => {
    const f = await fixture(t, '\ufeffliteral\r\n雪\t$HOME');
    const id = 'agent/sessions/opaque id:雪';
    const acp = fakeACP(t, {
      newSession: () => ({ sessionId: id, _meta: { secret: f.text } }),
      prompt: ({ sessionId, prompt }) => {
        assert.equal(sessionId, id);
        assert.deepEqual(prompt, [{ type: 'text', text: f.text }]);
        return { stopReason: 'end_turn' };
      },
    });
    const record = await runJob({ ...f, createConnection: acp.createConnection });
    assert.equal(record.sessionId, id);
    assert.equal(record.status, 'completed');
    assert.equal(record._meta, undefined);
    assert.equal(acp.calls.some(call => call.method === 'mode'), false);
  });

  test('blocked native setup retains the session without submitting or replaying the initial task', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t, {
      newSession: () => ({
        sessionId: 'blocked-native-session',
        modes: { currentModeId: 'human', availableModes: [{ id: 'human', name: 'Human' }] },
        _meta: { 'claude-acp-bridge': { registrationPending: true, blockedReason: f.text } },
      }),
    });
    const record = await runJob({ ...f, createConnection: acp.createConnection });
    assert.equal(record.status, 'blocked');
    assert.equal(record.errorCode, 'native_input_required');
    assert.equal(record.sessionId, 'blocked-native-session');
    assert.equal(record.dispatchAttemptedAt, undefined);
    assert.deepEqual(acp.calls.map(call => call.method), ['initialize', 'new']);
    assert.ok(!JSON.stringify(record).includes(f.text));
    assert.deepEqual(await runJob({ ...f, createConnection: acp.createConnection }), record);
    assert.equal(acp.connects, 1);
  });

  for (const mode of ['automation', 'human']) {
    test(`initial registration refreshes the same session and respects ${mode} control`, async t => {
      const f = await fixture(t);
      const acp = fakeACP(t, {
        initialize: () => ({ protocolVersion: PROTOCOL_VERSION, agentCapabilities: { loadSession: true } }),
        newSession: () => ({ sessionId: 'starting-session', _meta: { 'claude-acp-bridge': { registrationPending: true } } }),
        loadSession: params => {
          assert.equal(params.sessionId, 'starting-session');
          return { modes: { currentModeId: mode, availableModes: [{ id: 'human', name: 'Human' }, { id: 'automation', name: 'Automation' }] },
            _meta: { 'claude-acp-bridge': { registrationPending: false } } };
        },
        prompt: () => ({ stopReason: 'end_turn' }),
      });
      const result = await runJob({ ...f, createConnection: acp.createConnection });
      assert.equal(result.sessionId, 'starting-session');
      assert.equal(result.status, mode === 'automation' ? 'completed' : 'blocked');
      assert.deepEqual(acp.calls.map(call => call.method), ['initialize', 'new', 'load', ...(mode === 'automation' ? ['prompt'] : [])]);
    });
  }

  test('permissions are cancelled and agent updates are never retained', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t, { prompt: async ({ sessionId }, server) => {
      await server.client.notify(methods.client.session.update, { sessionId, update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: f.text } } });
      const response = await server.client.request(methods.client.session.requestPermission, {
        sessionId, toolCall: { toolCallId: 'tool-1' }, options: [{ kind: 'allow_always', optionId: 'yes', name: 'Allow' }],
      });
      assert.deepEqual(response, { outcome: { outcome: 'cancelled' } });
      return { stopReason: 'cancelled' };
    } });
    const result = await runJob({ ...f, createConnection: acp.createConnection });
    assert.equal(result.status, 'completed');
    assert.equal(result.stopReason, 'cancelled');
    assert.ok(!(await readFile(f.jobFile, 'utf8')).includes(f.text));
  });

  for (const stage of ['new', 'prompt']) {
    test(`ambiguous ${stage} failure retains identity and never retries or stores remote errors`, async t => {
      const f = await fixture(t);
      const acp = fakeACP(t, stage === 'new' ? { newSession: () => { throw new Error(f.text); } } : { prompt: () => { throw new Error(f.text); } });
      const result = await runJob({ ...f, createConnection: acp.createConnection });
      assert.equal(result.status, 'unknown');
      assert.equal(result.sessionId, stage === 'prompt' ? acp.sessionId : undefined);
      assert.ok(!JSON.stringify(result).includes(f.text));
      assert.deepEqual(await runJob({ ...f, createConnection: acp.createConnection }), result);
      assert.equal(acp.connects, 1);
    });
  }

  test('disconnect becomes unknown without cancellation and retains native identity', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t);
    const running = runJob({ ...f, createConnection: acp.createConnection });
    await acp.called('prompt');
    await acp.disconnect();
    const result = await running;
    assert.equal(result.status, 'unknown');
    assert.equal(result.sessionId, acp.sessionId);
    assert.equal(acp.calls.filter(call => call.method === 'cancel').length, 0);
  });

  test('abort during a pending turn disconnects without cancelling or resubmitting', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t);
    const controller = new AbortController();
    const running = runJob({ ...f, createConnection: acp.createConnection, signal: controller.signal });
    await acp.called('prompt');
    controller.abort(new Error(f.text));
    const result = await running;
    assert.equal(result.status, 'unknown');
    assert.equal(result.sessionId, acp.sessionId);
    assert.equal(acp.calls.filter(call => call.method === 'cancel').length, 0);
  });

  for (const phase of ['startup', 'turn']) {
    test(`${phase} deadline is conservative, bounded and does not cancel native work`, async t => {
      const f = await fixture(t);
      const created = deferred();
      const ready = deferred();
      const acp = fakeACP(t, phase === 'startup' ? { newSession: () => created.promise } : {});
      const timers = new Map();
      const turnDeadline = deferred();
      const running = runJob({ ...f, createConnection: acp.createConnection, onReady: ready.resolve,
        startupTimeoutMs: 1000, turnTimeoutMs: 2000,
        setTimer(callback, ms) { timers.set(ms, callback); if (ms === 2000) turnDeadline.resolve(); return ms; },
        clearTimer(ms) { timers.delete(ms); },
      });
      if (phase === 'startup') await acp.called('new');
      else {
        await ready.promise;
        await acp.called('prompt');
      }
      // Advance the injected deadline without sleeping or waiting for wall time.
      const deadline = phase === 'startup' ? 1000 : 2000;
      if (phase === 'turn') await turnDeadline.promise;
      timers.get(deadline)();
      const result = await running;
      assert.equal(result.status, 'unknown');
      assert.equal(result.sessionId, phase === 'turn' ? acp.sessionId : undefined);
      assert.equal(acp.calls.some(call => call.method === 'cancel'), false);
      assert.deepEqual(await runJob({ ...f, createConnection: acp.createConnection }), result);
      assert.equal(acp.connects, 1);
      created.resolve({ sessionId: acp.sessionId });
    });
  }

  test('an unrelated replacement during a turn is never overwritten', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t);
    const ready = deferred();
    const running = runJob({ ...f, createConnection: acp.createConnection, onReady: ready.resolve });
    await ready.promise;
    await rm(f.jobFile);
    await writeFile(f.jobFile, 'unrelated private state', { mode: 0o600 });
    acp.complete();
    await assert.rejects(running, /incomplete|invalid|changed/);
    assert.equal(await readFile(f.jobFile, 'utf8'), 'unrelated private state');
  });

  test('capability mismatch cannot create a native session', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t, { initialize: () => ({ protocolVersion: 99, agentCapabilities: {} }) });
    const result = await runJob({ ...f, createConnection: acp.createConnection });
    assert.equal(result.status, 'failed');
    assert.equal(acp.calls.some(call => call.method === 'new'), false);
  });

  test('unsupported automation mode retains the created session without prompting', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t, { newSession: () => ({ sessionId: 'native-session-123', modes: { currentModeId: 'human', availableModes: [{ id: 'human', name: 'Human' }] } }) });
    const result = await runJob({ ...f, createConnection: acp.createConnection });
    assert.equal(result.status, 'failed');
    assert.equal(result.sessionId, acp.sessionId);
    assert.equal(acp.calls.some(call => call.method === 'prompt'), false);
  });

  for (const file of ['promptFile', 'jobFile']) {
    for (const kind of ['public', 'symlink', 'hardlink', 'directory']) {
      test(`refuses ${kind} ${file} without overwriting or launching`, async t => {
        const f = await fixture(t);
        const acp = fakeACP(t);
        const path = f[file];
        await rm(path, { force: true });
        if (kind === 'public') { await writeFile(path, 'unrelated'); await chmod(path, 0o644); }
        if (kind === 'directory') await mkdir(path, { mode: 0o700 });
        if (kind === 'symlink' || kind === 'hardlink') {
          const target = join(f.directory, 'target');
          await writeFile(target, 'unrelated', { mode: 0o600 });
          await (kind === 'symlink' ? symlink : link)(target, path);
        }
        await assert.rejects(runJob({ ...f, createConnection: acp.createConnection }), /private|regular|symlink|owner|link/);
        assert.equal(acp.connects, 0);
      });
    }
  }

  test('empty, malformed or unrelated guards fail closed and are not replaced', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t);
    for (const text of ['', '{', JSON.stringify({ secret: f.text })]) {
      await writeFile(f.jobFile, text, { mode: 0o600 });
      await assert.rejects(runJob({ ...f, createConnection: acp.createConnection }), /incomplete|invalid|Invalid/);
      assert.equal(await readFile(f.jobFile, 'utf8'), text);
      assert.equal(acp.connects, 0);
    }
  });

  test('rejects public directories and symlinked path components', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t);
    await chmod(f.directory, 0o755);
    await assert.rejects(runJob({ ...f, createConnection: acp.createConnection }), /private|owner/);
    await chmod(f.directory, 0o700);
    const alias = f.directory + '-alias';
    await symlink(f.directory, alias);
    t.after(() => rm(alias));
    await assert.rejects(runJob({ ...f, promptFile: join(alias, 'prompt.txt'), createConnection: acp.createConnection }), /symlink/);
    assert.equal(acp.connects, 0);
  });
});

async function waitJob(file, predicate) {
  return new Promise((resolve, reject) => {
    let done = false;
    const watcher = watch(dirname(file), () => { void check(); });
    const timer = setTimeout(() => finish(new Error('Job state did not reach expected status')), 5000);
    function finish(error, result) {
      if (done) return;
      done = true;
      watcher.close();
      clearTimeout(timer);
      if (error) reject(error); else resolve(result);
    }
    async function check() {
      try {
        const record = await readJob(file);
        if (predicate(record)) finish(null, record);
      } catch (error) {
        if (error.code !== 'ENOENT' && !(error instanceof SyntaxError)) finish(error);
      }
    }
    void check();
  });
}

async function processFixture(t, agentOptions = {}) {
  const f = await fixture(t);
  const events = new EventEmitter();
  const calls = [];
  const sockets = new Set();
  const server = createServer(socket => {
    sockets.add(socket);
    socket.on('close', () => { sockets.delete(socket); events.emit('change'); });
    createInterface({ input: socket }).on('line', line => {
      calls.push(JSON.parse(line));
      events.emit('change');
    });
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  let workerPid;
  t.after(async () => {
    if (workerPid) {
      try { process.kill(workerPid, 'SIGTERM'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
    }
    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => server.close(resolve));
  });
  const cli = join(f.directory, 'fake-acp.mjs');
  await writeFile(f.stateFile, JSON.stringify({ port: server.address().port, ...agentOptions }), { mode: 0o600 });
  await writeFile(cli, `
import { agent, methods, ndJsonStream, PROTOCOL_VERSION } from ${JSON.stringify(import.meta.resolve('@agentclientprotocol/sdk'))};
import { readFile } from 'node:fs/promises';
import { connect } from 'node:net';
import { createInterface } from 'node:readline';
import { Readable, Writable } from 'node:stream';
const config = JSON.parse(await readFile(process.argv[4], 'utf8'));
const socket = connect(config.port, '127.0.0.1');
const send = (method, params = {}) => socket.write(JSON.stringify({ method, params }) + '\\n');
let complete;
let create;
let mode;
let rpc;
const pending = new Promise(resolve => { complete = resolve; });
const created = new Promise(resolve => { create = resolve; });
const automated = new Promise(resolve => { mode = resolve; });
createInterface({ input: socket }).on('line', line => {
  const command = JSON.parse(line);
  if (command.type === 'complete') complete({ stopReason: 'end_turn' });
  if (command.type === 'create') create();
  if (command.type === 'mode') mode();
  if (command.type === 'disconnect') { rpc.close(); process.stdin.destroy(); }
});
send('online', { pid: process.pid, args: process.argv.slice(2) });
const app = agent({ name: 'fake-detached-job-agent' });
app.onRequest(methods.agent.initialize, ({ params }) => {
  send('initialize', params);
  return { protocolVersion: PROTOCOL_VERSION, agentCapabilities: {} };
});
app.onRequest(methods.agent.session.new, async ({ params }) => {
  send('new', params);
  if (config.pauseNew) await created;
  return { sessionId: config.sessionId ?? 'native-detached-session', ...(config.blocked ? {
    _meta: { 'claude-acp-bridge': { registrationPending: true, blockedReason: process.env.JOB_TEST_SECRET } },
  } : {}), ...(config.pauseMode ? {
    modes: { currentModeId: 'human', availableModes: [{ id: 'human', name: 'Human' }, { id: 'automation', name: 'Automation' }] },
  } : {}) };
});
app.onRequest(methods.agent.session.setMode, async ({ params }) => {
  send('mode', params);
  await automated;
  return {};
});
app.onRequest(methods.agent.session.prompt, async ({ params }) => {
  send('prompt', params);
  console.error(process.env.JOB_TEST_SECRET);
  if (config.invalidNotification) process.stdout.write(JSON.stringify({ jsonrpc: '2.0', method: 'session/update', params: { secret: process.env.JOB_TEST_SECRET } }) + '\\n');
  if (config.unknownResponse) process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id: process.env.JOB_TEST_SECRET, result: {} }) + '\\n');
  await rpc.client.notify(methods.client.session.update, { sessionId: params.sessionId, update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: params.prompt[0].text } } });
  return pending;
});
app.onNotification(methods.agent.session.cancel, ({ params }) => send('cancel', params));
rpc = app.connect(ndJsonStream(Writable.toWeb(process.stdout), Readable.toWeb(process.stdin)));
await rpc.closed;
send('closed');
socket.end();
`, { mode: 0o600 });
  const execute = promisify(execFile);
  const secret = 'never-log-test-token-123';
  const launch = async (options = {}) => {
    // A separate short-lived parent proves worker stdio and IPC cannot keep the
    // starter alive, and losing the starter does not terminate native work.
    const program = `import { startDetachedJob } from ${JSON.stringify(new URL('../src/job.mjs', import.meta.url).href)}; console.log(JSON.stringify(await startDetachedJob(JSON.parse(process.argv[1]))));`;
    const output = await execute(process.execPath, ['--input-type=module', '-e', program, JSON.stringify({ ...f, text: undefined, cli, ...options })], {
      timeout: 10000, env: { ...process.env, JOB_TEST_SECRET: secret },
    });
    const record = JSON.parse(output.stdout);
    workerPid = record.workerPid;
    assert.ok(!(output.stdout + output.stderr).includes(secret));
    assert.ok(!(output.stdout + output.stderr).includes(f.text));
    return record;
  };
  return {
    ...f, cli, secret, calls, launch,
    async until(predicate) {
      while (!predicate()) await once(events, 'change');
    },
    async called(method) {
      while (!calls.some(call => call.method === method)) await once(events, 'change');
      return calls.find(call => call.method === method).params;
    },
    send(type) { for (const socket of sockets) socket.write(JSON.stringify({ type }) + '\n'); },
    get connected() { return sockets.size; },
  };
}

async function runCLI(f, detach) {
  const args = [new URL('../src/cli.mjs', import.meta.url).pathname, 'run',
    '--cwd', f.cwd, '--prompt-file', f.promptFile, '--job-file', f.jobFile, '--state', f.stateFile];
  if (detach) args.push('--detach');
  const result = await promisify(execFile)(process.execPath, args, { timeout: 5000 }).then(
    output => ({ ...output, code: 0 }),
    error => {
      if (!Number.isInteger(error.code) || error.killed) throw error;
      return { code: error.code, stdout: error.stdout, stderr: error.stderr };
    },
  );
  assert.equal(result.stderr, '');
  return { code: result.code, record: JSON.parse(result.stdout) };
}

describe('detached job lifecycle', { concurrency: true, timeout: 15000 }, () => {
  test('detached workers retain the selected harness across IPC and preserve opaque native IDs', async t => {
    for (const harness of ['claude', 'codex', 'opencode']) {
      const id = harness === 'opencode' ? 'opencode:ses_test123' : harness === 'codex' ? 'codex:ad72142c-b0ed-474c-9e9e-237345db5784' : 'native-detached-session';
      const f = await processFixture(t, { sessionId: id });
      const record = await f.launch({ harness });
      assert.equal(record.harness, harness);
      assert.equal(record.sessionId, id);
      assert.deepEqual(await f.called('new'), { cwd: f.cwd, mcpServers: [],
        ...(harness && harness !== 'claude' ? { _meta: { 'coder-acp': { harness } } } : {}),
      });
      assert.deepEqual(await f.launch({ harness }), record);
      await assert.rejects(startDetachedJob({ ...f, harness: harness === 'codex' ? 'claude' : 'codex' }), /different input|different harness/);
      f.send('complete');
      const final = await waitJob(f.jobFile, value => value.status === 'completed');
      assert.equal(final.harness, harness);
      assert.equal(final.sessionId, id);
      await f.until(() => f.connected === 0);
      assert.equal(f.calls.filter(call => call.method === 'new').length, 1);
      assert.equal(f.calls.filter(call => call.method === 'prompt').length, 1);
    }
  });

  test('parent exits after dispatch, worker owns an OS session and private stdio, and observes completion', async t => {
    const f = await processFixture(t);
    const record = await f.launch();
    assert.equal(record.status, 'prompting');
    assert.equal(record.sessionId, 'native-detached-session');
    assert.ok(record.dispatchAttemptedAt);
    assert.ok(record.workerPid);
    const processInfo = await readFile('/proc/' + record.workerPid + '/stat', 'utf8');
    assert.equal(Number(processInfo.slice(processInfo.lastIndexOf(') ') + 2).split(' ')[3]), record.workerPid);
    assert.equal(await readlink('/proc/' + record.workerPid + '/fd/0'), '/dev/null');
    assert.equal(await readlink('/proc/' + record.workerPid + '/fd/1'), f.jobFile + '.log');
    assert.equal(await readlink('/proc/' + record.workerPid + '/fd/2'), f.jobFile + '.log');
    assert.equal((await lstat(f.jobFile + '.log')).mode & 0o777, 0o600);
    assert.deepEqual((await f.called('online')).args, ['acp', '--state', f.stateFile]);
    assert.deepEqual((await f.called('prompt')).prompt, [{ type: 'text', text: f.text }]);
    assert.equal(f.connected, 1, 'observer stays connected after starter exits');
    const duplicate = await f.launch();
    assert.equal(duplicate.jobId, record.jobId);
    assert.equal(duplicate.workerPid, record.workerPid);
    assert.equal(f.calls.filter(call => call.method === 'new').length, 1);
    f.send('complete');
    const final = await waitJob(f.jobFile, value => value.status === 'completed');
    assert.equal(final.sessionId, record.sessionId);
    assert.equal(final.stopReason, 'end_turn');
    await f.until(() => f.connected === 0);
    assert.equal(f.calls.some(call => call.method === 'cancel'), false);
    const log = await readFile(f.jobFile + '.log', 'utf8');
    assert.ok(!log.includes(f.text));
    assert.ok(!log.includes(f.secret));
  });

  test('detached blocked setup exits without claiming dispatch readiness or losing the session', async t => {
    const f = await processFixture(t, { blocked: true });
    const result = await f.launch();
    assert.equal(result.status, 'blocked');
    assert.equal(result.sessionId, 'native-detached-session');
    assert.equal(result.errorCode, 'native_input_required');
    assert.equal(result.dispatchAttemptedAt, undefined);
    await f.until(() => f.connected === 0);
    const retry = await runCLI(f, true);
    assert.equal(retry.code, 1);
    assert.deepEqual(retry.record, result);
    assert.deepEqual(f.calls.map(call => call.method), ['online', 'initialize', 'new', 'closed']);
    assert.ok(!(await readFile(f.jobFile + '.log', 'utf8')).includes(f.secret));
  });

  test('malformed remote notifications cannot leak through SDK diagnostics into the worker log', async t => {
    const f = await processFixture(t, { invalidNotification: true });
    await f.launch();
    await f.called('prompt');
    f.send('complete');
    await waitJob(f.jobFile, record => record.status === 'completed');
    await f.until(() => f.connected === 0);
    assert.ok(!(await readFile(f.jobFile + '.log', 'utf8')).includes(f.secret));
  });

  test('worker transport failure keeps identity unknown and a retry never reconnects', async t => {
    const f = await processFixture(t);
    const record = await f.launch();
    await f.called('prompt');
    f.send('disconnect');
    const final = await waitJob(f.jobFile, value => value.status === 'unknown');
    assert.equal(final.sessionId, record.sessionId);
    assert.deepEqual(await f.launch(), final);
    assert.equal(f.calls.filter(call => call.method === 'new').length, 1);
    assert.equal(f.calls.filter(call => call.method === 'prompt').length, 1);
    assert.equal(f.calls.some(call => call.method === 'cancel'), false);
  });

  test('concurrent detached parents launch at most one observer and one native session', async t => {
    const f = await processFixture(t);
    const results = await Promise.allSettled([f.launch(), f.launch()]);
    const records = results.filter(result => result.status === 'fulfilled').map(result => result.value);
    assert.ok(records.length >= 1);
    for (const result of results.filter(result => result.status === 'rejected')) assert.match(result.reason.stderr, /incomplete|invalid/);
    await f.called('prompt');
    assert.equal(f.calls.filter(call => call.method === 'new').length, 1);
    assert.equal(f.calls.filter(call => call.method === 'prompt').length, 1);
    for (const record of records) assert.equal(record.jobId, records[0].jobId);
    f.send('complete');
    await waitJob(f.jobFile, value => value.status === 'completed');
    await f.until(() => f.connected === 0);
  });

  test('concurrent real CLI retries exit nonzero until their requested milestone without waiting or replaying', async t => {
    const f = await processFixture(t, { pauseNew: true, pauseMode: true });
    const launching = f.launch();
    async function retry(status, codes) {
      const before = await readFile(f.jobFile, 'utf8');
      const record = JSON.parse(before);
      assert.equal(record.status, status);
      const results = await Promise.all([runCLI(f, false), runCLI(f, true)]);
      assert.deepEqual(results.map(result => result.code), codes, status + ' exit codes: foreground, detached');
      for (const result of results) assert.deepEqual(result.record, record);
      assert.equal(await readFile(f.jobFile, 'utf8'), before, 'retries must not mutate the guard');
      assert.equal(f.calls.filter(call => call.method === 'new').length, 1);
    }
    try {
      await f.called('new');
      await retry('starting', [1, 1]);
      assert.equal((await readJob(f.jobFile)).sessionId, undefined);
      f.send('create');
      await f.called('mode');
      await retry('created', [1, 1]);
      assert.equal((await readJob(f.jobFile)).dispatchAttemptedAt, undefined);
      f.send('mode');
      const ready = await launching;
      assert.ok(ready.sessionId);
      assert.ok(ready.dispatchAttemptedAt);
      await f.called('prompt');
      await retry('prompting', [1, 0]);
      f.send('complete');
      await waitJob(f.jobFile, record => record.status === 'completed');
      await f.until(() => f.connected === 0);
      await retry('completed', [0, 0]);
      assert.equal(f.calls.filter(call => call.method === 'prompt').length, 1);
    } finally {
      f.send('create');
      f.send('mode');
      f.send('complete');
      await launching;
      await f.until(() => f.connected === 0);
    }
  });

  test('real CLI success requires completion or durable dispatch, never just a matching guard', async t => {
    const f = await fixture(t);
    const acp = fakeACP(t, { prompt: () => ({ stopReason: 'end_turn' }) });
    const completed = await runJob({ ...f, createConnection: acp.createConnection });
    const { completedAt, stopReason, sessionId, dispatchAttemptedAt, ...guard } = completed;
    for (const [status, dispatched, expected] of [
      ['starting', false, [1, 1]], ['created', false, [1, 1]],
      ['prompting', false, [1, 1]], ['prompting', true, [1, 0]],
      ['completed', false, [0, 1]], ['completed', true, [0, 0]],
      ['failed', true, [1, 1]], ['unknown', true, [1, 1]],
    ]) {
      const record = { ...guard, status,
        ...(status !== 'starting' ? { sessionId } : {}),
        ...(dispatched ? { dispatchAttemptedAt } : {}),
        ...(status === 'completed' ? { completedAt, stopReason } : {}),
      };
      await writeFile(f.jobFile, JSON.stringify(record) + '\n');
      const results = await Promise.all([runCLI(f, false), runCLI(f, true)]);
      assert.deepEqual(results.map(result => result.code), expected, status + ', dispatched=' + dispatched);
      for (const result of results) assert.deepEqual(result.record, record);
      assert.deepEqual(await readJob(f.jobFile), record);
    }
    assert.equal(acp.connects, 1);
  });

  test('an injected parent readiness deadline cleans up only its worker and never replays uncertain creation', async t => {
    const f = await processFixture(t, { pauseNew: true });
    let expire;
    const running = startDetachedJob({ ...f,
      setTimer(callback) { expire = callback; return 1; }, clearTimer() {},
    });
    await f.called('new');
    expire();
    const record = await running;
    assert.equal(record.status, 'unknown');
    assert.equal(record.sessionId, undefined);
    await f.until(() => f.connected === 0);
    assert.equal(f.calls.some(call => call.method === 'cancel'), false);
    assert.deepEqual(await startDetachedJob(f), record);
    assert.equal(f.calls.filter(call => call.method === 'new').length, 1);
  });

  test('an unsolicited response ID is not copied into SDK logs and the job stays unknown', async t => {
    const f = await processFixture(t, { unknownResponse: true });
    await f.launch();
    const record = await waitJob(f.jobFile, value => value.status === 'unknown');
    assert.equal(record.sessionId, 'native-detached-session');
    await f.until(() => f.connected === 0);
    assert.ok(!(await readFile(f.jobFile + '.log', 'utf8')).includes(f.secret));
  });

  test('a failed worker setup is recorded and never replayed', async t => {
    const f = await fixture(t);
    const result = await startDetachedJob({ ...f, cli: join(f.directory, 'missing.mjs') });
    assert.equal(result.status, 'failed');
    assert.equal(result.sessionId, undefined);
    assert.deepEqual(await startDetachedJob({ ...f, cli: join(f.directory, 'missing.mjs') }), result);
    assert.equal((await lstat(f.jobFile + '.log')).mode & 0o777, 0o600);
  });

  for (const kind of ['public', 'private', 'symlink']) {
    test(`does not overwrite an existing ${kind} worker log`, async t => {
      const f = await fixture(t);
      const target = join(f.directory, 'unrelated-log');
      await writeFile(target, 'unrelated', { mode: 0o600 });
      if (kind === 'symlink') await symlink(target, f.jobFile + '.log');
      else await writeFile(f.jobFile + '.log', 'unrelated', { mode: kind === 'public' ? 0o644 : 0o600 });
      const result = await startDetachedJob(f);
      assert.equal(result.status, 'failed');
      assert.equal(result.workerPid, undefined);
      assert.equal(await readFile(f.jobFile + '.log', 'utf8'), 'unrelated');
    });
  }

  test('public CLI validates required paths before creating a job or connecting', async t => {
    const f = await fixture(t);
    const execute = promisify(execFile);
    const cli = new URL('../src/cli.mjs', import.meta.url);
    for (const args of [[], ['--cwd', 'relative'], ['--cwd', f.cwd, '--prompt-file', f.promptFile]]) {
      await assert.rejects(execute(process.execPath, [cli.pathname, 'run', ...args]), error => {
        assert.match(error.stderr, /absolute path/);
        assert.equal(error.stdout, '');
        return true;
      });
    }
    await assert.rejects(lstat(f.jobFile), { code: 'ENOENT' });
  });
});

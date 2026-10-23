import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { EventEmitter, once } from 'node:events';
import { mkdtemp } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { Readable, Writable } from 'node:stream';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import test from 'node:test';
import { client as acpClient, methods, ndJsonStream, PROTOCOL_VERSION } from '@agentclientprotocol/sdk';
import { connect } from '../src/client.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const execute = promisify(execFile);
const cwd = resolve(process.env.BRIDGE_LIVE_CWD || process.cwd());

function spawnCLI(args) {
  return spawn(process.execPath, [resolve(root, 'src/cli.mjs'), ...args], { cwd: root, stdio: ['pipe', 'pipe', 'pipe'] });
}

async function waitFor(check, events, signal) {
  while (!check()) await once(events, 'change', { signal });
}

test('real ACP, broker, native Claude and terminal acceptance', {
  skip: process.env.BRIDGE_LIVE !== '1' && 'Opt in with BRIDGE_LIVE=1; this test makes billed model requests',
  timeout: 240000,
}, async t => {
  const signal = AbortSignal.timeout(220000);
  const directory = await mkdtemp('/tmp/opencode/claude-bridge-live-');
  const stateFile = resolve(directory, 'bridge.json');
  t.diagnostic('Test artifacts: ' + directory);
  const server = spawnCLI(['serve', '--state', stateFile, '--model', process.env.BRIDGE_LIVE_MODEL || 'haiku']);
  const serverExit = once(server, 'exit');
  let stderr = '';
  const readiness = new EventEmitter();
  server.stderr.on('data', chunk => { stderr += chunk; readiness.emit('change'); });
  server.once('exit', () => readiness.emit('change'));
  t.after(async () => { server.kill('SIGTERM'); await serverExit; });
  await waitFor(() => stderr.includes('listening on') || server.exitCode !== null, readiness, signal);
  assert.equal(server.exitCode, null, stderr);
  const http = await connect(stateFile);

  async function attachACP() {
    const child = spawnCLI(['acp', '--state', stateFile]);
    const exited = once(child, 'exit');
    let errors = '';
    child.stderr.on('data', chunk => { errors += chunk; });
    const updates = [];
    const changed = new EventEmitter();
    const connection = acpClient({ name: 'native-acceptance' })
      .onNotification(methods.client.session.update, ({ params }) => { updates.push(params); changed.emit('change'); })
      .onRequest(methods.client.session.requestPermission, () => { throw new Error('No automatic permission approvals'); })
      .connect(ndJsonStream(Writable.toWeb(child.stdin), Readable.toWeb(child.stdout)));
    await connection.agent.request(methods.agent.initialize, { protocolVersion: PROTOCOL_VERSION, clientCapabilities: {} });
    let closed = false;
    async function close() {
      if (closed) return;
      closed = true;
      connection.close();
      child.stdin.end();
      await exited;
      assert.equal(child.exitCode, 0, errors);
    }
    t.after(close);
    return { rpc: connection.agent, updates, changed, close };
  }

  let acp = await attachACP();
  const session = await acp.rpc.request(methods.agent.session.new, { cwd, mcpServers: [] });
  const id = session.sessionId;
  const path = '/v1/sessions/' + id;
  t.after(async () => { await execute('claude', ['stop', id.slice(0, 8)], { timeout: 15000 }); });
  const inspect = () => http.request('GET', path);
  async function native() {
    const { stdout } = await execute('claude', ['agents', '--json', '--all'], { timeout: 15000 });
    return JSON.parse(stdout).find(agent => agent.sessionId === id);
  }
  const before = await native();
  const initial = await inspect();
  assert.ok(before.pid);
  t.diagnostic('Native Claude ' + initial.version.version + ', session ' + id + ', pid ' + before.pid);

  let cursor = 0;
  async function eventUntil(predicate) {
    for (;;) {
      const batch = await http.request('GET', path + '/events?after=' + cursor + '&wait=1', undefined, signal);
      for (const event of batch.events) {
        cursor = event.seq;
        assert.ok(!['overflow', 'runtime.replaced'].includes(event.type), JSON.stringify(event));
        if (predicate(event)) return event;
      }
      cursor = batch.cursor;
    }
  }
  const prompt = text => acp.rpc.request(methods.agent.session.prompt, { sessionId: id, prompt: [{ type: 'text', text }] });
  const mode = modeId => acp.rpc.request(methods.agent.session.setMode, { sessionId: id, modeId });
  const output = () => acp.updates.filter(item => item.update.sessionUpdate === 'agent_message_chunk').map(item => item.update.content.text).join('');

  await t.test('native prompt streams to ACP and completes exactly once', async () => {
    assert.equal((await prompt('Reply with exactly BRIDGE_LIVE_OK. Do not use any tools.')).stopReason, 'end_turn');
    assert.equal(output().trim(), 'BRIDGE_LIVE_OK');
  });

  await t.test('native tool execution produces structured ACP updates', async () => {
    const count = acp.updates.length;
    assert.equal((await prompt('Use the Read tool to read ' + resolve(root, 'package.json') + '. Reply only with its package name. Do not modify anything or use other tools.')).stopReason, 'end_turn');
    const updates = acp.updates.slice(count).map(item => item.update);
    assert.ok(updates.some(update => update.sessionUpdate === 'tool_call'));
    assert.ok(updates.some(update => update.sessionUpdate === 'tool_call_update' && update.status === 'completed'));
  });

  const terminal = spawn('python3', [resolve(root, 'test/terminal.py'), 'claude', 'attach', id.slice(0, 8)], { stdio: ['pipe', 'pipe', 'pipe'] });
  const terminalExit = once(terminal, 'exit');
  t.after(async () => { terminal.kill('SIGTERM'); await terminalExit; });
  const terminalEvents = new EventEmitter();
  let terminalReady = false;
  let terminalErrors = '';
  terminal.stderr.on('data', chunk => { terminalErrors += chunk; });
  createInterface({ input: terminal.stdout }).on('line', line => {
    const value = JSON.parse(line);
    if (value.ready) terminalReady = true;
    terminalEvents.emit('change');
  });
  terminal.once('exit', () => terminalEvents.emit('change'));
  const type = input => terminal.stdin.write(JSON.stringify({ input }) + '\n');

  await t.test('native TUI takes control and submits input in the same live process', async () => {
    await waitFor(() => terminalReady || terminal.exitCode !== null, terminalEvents, signal);
    assert.ok(terminalReady, terminalErrors);
    cursor = (await inspect()).cursor;
    type('/bridge-human\r');
    await eventUntil(event => event.type === 'mode' && event.data.mode === 'human');
    await assert.rejects(prompt('Must not run under human control'), /human control/);
    type('Reply with exactly NATIVE_TUI_OK. Do not use any tools.\r');
    const completed = await eventUntil(event => event.type === 'turn.complete');
    assert.equal(completed.data.answer.trim(), 'NATIVE_TUI_OK');
    assert.equal(completed.data.requestId, undefined);
    assert.equal((await native()).pid, before.pid);
    assert.equal((await inspect()).runtimeId, initial.runtimeId);
    type('/bridge-auto\r');
    await eventUntil(event => event.type === 'mode' && event.data.mode === 'automation');
  });

  await t.test('native detach and ACP disconnect leave a running turn intact', async () => {
    cursor = (await inspect()).cursor;
    const running = prompt('Write exactly 100 numbered lines, each saying background work continues. Do not use tools.');
    // Observe rejection before closing transport, without stopping the native turn.
    const disconnected = assert.rejects(running, /connection closed/);
    const first = await eventUntil(event => event.type === 'text' && Boolean(event.data.requestId));
    type('\u001a');
    await terminalExit;
    assert.equal(terminal.exitCode, 0, terminalErrors);
    assert.equal((await inspect()).activeTurnId, first.data.turnId, 'native turn still running after terminal detach');
    await acp.close();
    await disconnected;
    acp = await attachACP();
    await acp.rpc.request(methods.agent.session.load, { sessionId: id, cwd, mcpServers: [] });
    assert.equal((await inspect()).activeTurnId, first.data.turnId, 'reconnected before native completion');
    const completed = await eventUntil(event => event.type === 'turn.complete' && event.data.requestId === first.data.requestId);
    assert.equal(completed.data.isAborted, false);
    await waitFor(() => output().includes(completed.data.answer), acp.changed, signal);
    assert.equal((await native()).pid, before.pid);
    assert.equal((await inspect()).runtimeId, initial.runtimeId);
  });

  await t.test('ACP cancellation aborts the correlated native turn', async () => {
    await mode('automation');
    cursor = (await inspect()).cursor;
    const running = prompt('Write 1000 numbered lines about counting. Do not use tools.');
    const first = await eventUntil(event => event.type === 'text' && Boolean(event.data.requestId));
    await acp.rpc.notify(methods.agent.session.cancel, { sessionId: id });
    assert.equal((await running).stopReason, 'cancelled');
    const completed = await eventUntil(event => event.type === 'turn.complete' && event.data.requestId === first.data.requestId);
    assert.equal(completed.data.isAborted, true);
    assert.equal((await inspect()).mode, 'human');
  });
});

import assert from 'node:assert/strict';
import { execFile, fork } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { mkdir, mkdtemp, readFile, readlink, rm, symlink, writeFile } from 'node:fs/promises';
import { createServer as createHTTPServer } from 'node:http';
import { createServer as createIPCServer } from 'node:net';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { WebSocketServer } from 'ws';
import { createClient } from '../src/client.mjs';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../src/cli.mjs', import.meta.url));

// A real WebSocket endpoint behind a raw Unix-socket proxy executable. This
// exercises the helper's default probe and the service's async RPC factory.
async function nativeFixture(t, version = '0.164.0') {
  const home = await mkdtemp('/tmp/opencode/bridge-startup-');
  const stateFile = join(home, 'bridge.json');
  const socketFile = join(home, 'codex.sock');
  const target = join(home, 'native.sock');
  const commandsFile = join(home, 'commands.jsonl');
  const bin = join(home, 'bin');
  const requests = [];
  const nativeId = randomUUID();
  const server = createHTTPServer();
  const wss = new WebSocketServer({ server });
  const sockets = new Set();
  let child;
  let exited;
  let log = '';
  const messages = [];
  server.on('connection', socket => { sockets.add(socket); socket.on('close', () => sockets.delete(socket)); });
  wss.on('connection', peer => {
    peer.on('error', () => {});
    peer.on('message', bytes => {
      const message = JSON.parse(bytes.toString());
      requests.push(message);
      if (message.method === 'initialize') {
        peer.send(JSON.stringify({ id: message.id, result: {
          userAgent: 'codex_cli_rs/' + version + ' (Linux)', platformFamily: 'unix', codexHome: join(home, '.codex'),
        } }));
      } else if (message.method === 'thread/start') {
        peer.send(JSON.stringify({ id: message.id, result: { thread: { id: nativeId, cwd: message.params.cwd, status: { type: 'idle' } } } }));
      } else assert.equal(message.method, 'initialized', 'no implicit thread loading or work dispatch');
    });
  });
  t.after(async () => {
    if (child && child.exitCode === null && child.signalCode === null) { child.kill('SIGKILL'); await exited; }
    for (const peer of wss.clients) peer.terminate();
    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => wss.close(resolve));
    await new Promise(resolve => server.close(resolve));
    await rm(home, { recursive: true, force: true });
  });
  server.listen(target);
  await once(server, 'listening');
  await symlink(target, socketFile);
  await mkdir(bin, { mode: 0o700 });
  await writeFile(commandsFile, '', { mode: 0o600 });
  await writeFile(join(bin, 'codex'), `#!${process.execPath}
const args = process.argv.slice(2);
require('node:fs').appendFileSync(${JSON.stringify(commandsFile)}, JSON.stringify(args) + '\\n');
if (args.join(' ') === 'app-server proxy --sock ' + ${JSON.stringify(socketFile)}) {
  const socket = require('node:net').createConnection(args[3]);
  socket.on('error', () => process.exit(1));
  process.stdin.pipe(socket).pipe(process.stdout);
} else if (args[0] === 'resume') {
  console.log(JSON.stringify({ args, cwd: process.cwd() }));
} else {
  console.error('Unexpected native startup');
  process.exitCode = 1;
}
`, { mode: 0o700 });
  const env = { ...process.env, HOME: home, CODEX_HOME: join(home, '.codex'), PATH: bin + ':' + process.env.PATH };
  return {
    home, stateFile, socketFile, target, nativeId, requests, env, messages,
    get log() { return log; },
    async commands() { return (await readFile(commandsFile, 'utf8')).split('\n').filter(Boolean).map(line => JSON.parse(line)); },
    start(args = ['--codex']) {
      child = fork(cli, ['serve', ...args, '--state', stateFile], { env, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
      child.stdout.on('data', chunk => { log += chunk; });
      child.stderr.on('data', chunk => { log += chunk; });
      child.on('message', message => messages.push(message));
      exited = once(child, 'exit');
      return { child, exited };
    },
    async ready() {
      if (messages.some(message => message.type === 'bridge-ready')) return;
      await Promise.race([
        once(child, 'message').then(([message]) => assert.equal(message.type, 'bridge-ready')),
        exited.then(() => { throw new Error('Broker exited before readiness: ' + log); }),
      ]);
    },
  };
}

describe('CLI startup routing', { concurrency: true, timeout: 10000 }, () => {
  for (const capability of [
    { name: 'a legacy broker without capabilities', status: 404, body: { error: 'Unknown route' } },
    { name: 'a broker advertising only Claude', status: 200, body: { harnesses: [{ id: 'claude' }] } },
  ].flatMap(capability => ['codex', 'opencode'].map(harness => ({ ...capability, harness })))) {
    test('start --harness ' + capability.harness + ' refuses ' + capability.name + ' before any mutation', async t => {
      const home = await mkdtemp('/tmp/opencode/bridge-startup-');
      const stateFile = join(home, 'bridge.json');
      const token = randomBytes(32).toString('hex');
      const calls = [];
      const session = { sessionId: randomUUID(), cwd: home, mode: 'human' };
      const server = createHTTPServer((req, res) => {
        calls.push({ method: req.method, path: req.url, authorization: req.headers.authorization });
        req.resume();
        if (req.method === 'GET' && req.url === '/v1/capabilities') {
          res.writeHead(capability.status, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify(capability.body));
        } else if (req.method === 'POST' && req.url === '/v1/sessions') {
          // The legacy broker ignores the harness field and would launch Claude.
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify(session));
        } else {
          res.writeHead(404, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ error: 'Unknown route' }));
        }
      });
      t.after(async () => {
        server.closeAllConnections();
        await new Promise(resolve => server.close(resolve));
        await rm(home, { recursive: true, force: true });
      });
      const listening = once(server, 'listening');
      server.listen(0, '127.0.0.1');
      await listening;
      await writeFile(stateFile, JSON.stringify({ url: 'http://127.0.0.1:' + server.address().port, token }), { mode: 0o600 });
      const args = [cli, 'start', '--state', stateFile, '--cwd', home];

      await assert.rejects(execute(process.execPath, [...args, '--harness', capability.harness], { timeout: 5000 }), error => {
        assert.equal(error.code, 1);
        assert.equal(error.stdout, '');
        assert.match(error.stderr, /Unknown route|does not advertise (?:Codex|OpenCode)/);
        return true;
      });
      assert.deepEqual(calls, [{ method: 'GET', path: '/v1/capabilities', authorization: 'Bearer ' + token }]);

      // A refused optional harness must not break ordinary Claude use of this broker.
      const result = await execute(process.execPath, args, { timeout: 5000 });
      assert.deepEqual(JSON.parse(result.stdout), session);
      assert.deepEqual(calls.slice(1), [{ method: 'POST', path: '/v1/sessions', authorization: 'Bearer ' + token }]);
    });
  }

  for (const version of ['opencode v2.0.29', 'opencode v1.2.3', 'opencode v2.0.28-beta', 'private-native-diagnostic', null]) {
    test('OpenCode attach gates the local CLI before native calls or mode changes: ' + version, async t => {
      const home = await mkdtemp('/tmp/opencode/bridge-attach-version-');
      const stateFile = join(home, 'bridge.json');
      const capture = join(home, 'commands.jsonl');
      const bin = join(home, 'bin');
      const calls = [];
      const nativeSessionId = 'ses_versionGateFixture';
      const session = { sessionId: 'opencode:' + nativeSessionId, nativeSessionId, harness: 'opencode', cwd: home, mode: 'automation' };
      const path = '/v1/sessions/' + encodeURIComponent(session.sessionId);
      const server = createHTTPServer((req, res) => {
        calls.push([req.method, req.url]);
        req.resume();
        if (req.method === 'POST') session.mode = 'human';
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(session));
      });
      t.after(async () => {
        server.closeAllConnections();
        await new Promise(resolve => server.close(resolve));
        await rm(home, { recursive: true, force: true });
      });
      const listening = once(server, 'listening');
      server.listen(0, '127.0.0.1');
      await listening;
      await writeFile(stateFile, JSON.stringify({ url: 'http://127.0.0.1:' + server.address().port, token: randomBytes(32).toString('hex') }), { mode: 0o600 });
      const descriptor = await readFile(stateFile);
      await mkdir(bin, { mode: 0o700 });
      await writeFile(join(bin, 'opencode'), `#!${process.execPath}
require('node:fs').appendFileSync(${JSON.stringify(capture)}, JSON.stringify({ args: process.argv.slice(2), home: process.env.HOME, cwd: process.cwd(), mask: process.umask(), password: process.env.OPENCODE_PASSWORD, database: process.env.OPENCODE_DB }) + '\\n', { mode: 0o600 });
process.stdout.write(${JSON.stringify(version ?? '')});
process.stderr.write('private-native-diagnostic');
process.exitCode = ${version === null ? 1 : 0};
`, { mode: 0o700 });
      const env = { ...process.env, HOME: home, PATH: bin + ':' + process.env.PATH, OPENCODE_DB: '/must-not-touch-database', OPENCODE_PASSWORD: 'must-not-leak-password' };
      await assert.rejects(execute(process.execPath, [cli, 'attach', session.sessionId, '--state', stateFile], { env, cwd: home, timeout: 5000 }), error => {
        assert.equal(error.code, 1);
        assert.equal(error.stdout, '');
        assert.match(error.stderr, version === null ? /CLI version check failed/ : /CLI must be exactly version 2\.0\.28/);
        assert.doesNotMatch(error.stderr, /private-native-diagnostic|must-not-leak-password/);
        return true;
      });
      assert.equal(session.mode, 'automation');
      assert.deepEqual(calls, [['GET', path]]);
      assert.deepEqual(await readFile(stateFile), descriptor);
      const commands = (await readFile(capture, 'utf8')).trim().split('\n').map(JSON.parse);
      assert.equal(commands.length, 1);
      assert.deepEqual(commands[0].args, ['--version']);
      assert.ok(!commands[0].home.startsWith(home));
      assert.equal(commands[0].cwd, commands[0].home);
      assert.equal(commands[0].mask, 0o077);
      assert.equal(commands[0].password, undefined);
      assert.equal(commands[0].database, undefined);
      await assert.rejects(readFile(join(home, 'opencode-server.json')), { code: 'ENOENT' });
    });
  }

  test('explicit socket is shared by the readiness probe, service RPC and native TUI', async t => {
    const f = await nativeFixture(t);
    const { child, exited } = f.start();
    await f.ready();
    const proxy = ['app-server', 'proxy', '--sock', f.socketFile];
    assert.deepEqual(await f.commands(), [proxy]);
    assert.equal(f.requests.filter(message => message.method === 'initialize').length, 1);
    assert.ok(f.requests.every(message => ['initialize', 'initialized'].includes(message.method)));
    await assert.rejects(readFile(join(f.home, 'codex-server.log')), error => error.code === 'ENOENT');
    const client = createClient(JSON.parse(await readFile(f.stateFile, 'utf8')));
    const session = await client.request('POST', '/v1/sessions', { cwd: f.home, harness: 'codex' }, t.signal);
    assert.equal(session.sessionId, 'codex:' + f.nativeId);
    assert.equal(f.requests.filter(message => message.method === 'initialize').length, 2);
    assert.equal(f.requests.filter(message => message.method === 'thread/start').length, 1);
    const attached = await execute(process.execPath, [cli, 'attach', session.sessionId, '--state', 'bridge.json'], { cwd: f.home, env: f.env, timeout: 5000 });
    const resume = ['resume', '--remote', 'unix://' + f.socketFile, f.nativeId];
    assert.deepEqual(JSON.parse(attached.stdout), { args: resume, cwd: f.home });
    assert.deepEqual(await f.commands(), [proxy, proxy, resume]);
    assert.equal(await readlink(f.socketFile), f.target);
    child.kill('SIGTERM');
    assert.deepEqual(await exited, [0, null], f.log);
    assert.ok(f.requests.every(message => ['initialize', 'initialized', 'thread/start'].includes(message.method)), 'close cannot cancel or resume native work');
    assert.equal(await readlink(f.socketFile), f.target, 'broker close preserves the native endpoint');
  });

  test('running native version mismatch fails closed without starting an engine', async t => {
    const f = await nativeFixture(t, '0.165.0');
    const { exited } = f.start();
    assert.deepEqual(await exited, [1, null]);
    assert.match(f.log, /must run exactly Codex 0\.164\.0/);
    assert.deepEqual(f.messages, []);
    assert.deepEqual(await f.commands(), [['app-server', 'proxy', '--sock', f.socketFile]]);
    assert.ok(f.requests.every(message => ['initialize', 'initialized'].includes(message.method)));
    assert.equal(await readlink(f.socketFile), f.target);
    await assert.rejects(readFile(f.stateFile), error => error.code === 'ENOENT');
    await assert.rejects(readFile(join(f.home, 'codex-server.log')), error => error.code === 'ENOENT');
  });

  test('ordinary serve never probes or starts Codex', async t => {
    const f = await nativeFixture(t);
    const { child, exited } = f.start([]);
    await f.ready();
    assert.deepEqual(await f.commands(), []);
    assert.deepEqual(f.requests, []);
    child.kill('SIGTERM');
    assert.deepEqual(await exited, [0, null], f.log);
  });

  test('SIGINT during native server startup rejects requests and removes the descriptor', async t => {
    const home = await mkdtemp('/tmp/opencode/bridge-startup-');
    const stateFile = join(home, 'bridge.json');
    const socketFile = join(home, 'startup.sock');
    const bin = join(home, 'bin');
    const started = Promise.withResolvers();
    const sockets = new Set();
    const server = createIPCServer(socket => {
      sockets.add(socket);
      socket.once('close', () => sockets.delete(socket));
      socket.on('error', started.reject);
      socket.setEncoding('utf8');
      let message = '';
      socket.on('data', chunk => {
        message += chunk;
        if (message === 'server-start\n') started.resolve(socket);
      });
    });
    let child;
    let exited;
    t.after(async () => {
      for (const socket of sockets) socket.destroy();
      if (child && child.exitCode === null && child.signalCode === null) {
        child.kill('SIGKILL');
        await exited;
      }
      await new Promise(resolve => server.close(resolve));
      await rm(home, { recursive: true, force: true });
    });
    const listening = once(server, 'listening');
    server.listen(socketFile);
    await listening;
    await mkdir(bin, { mode: 0o700 });
    await writeFile(join(bin, 'codex'), `#!${process.execPath}
const args = process.argv.slice(2);
if (args.join(' ') === 'app-server proxy --sock ' + ${JSON.stringify(join(home, 'codex.sock'))}) {
  // No endpoint is ready yet. The real RPC handshake must fail, not load a thread.
  process.exitCode = 1;
} else if (args.join(' ') === 'app-server --listen unix://' + ${JSON.stringify(join(home, 'codex.sock'))}) {
  // Signal that the unmanaged child is alive, but never report readiness.
  process.stderr.write('native-secret-diagnostic\\n');
  process.stdout.write('native-private-output\\n');
  const socket = require('node:net').createConnection(process.env.STARTUP_TEST_SOCKET);
  socket.once('connect', () => socket.write('server-start\\n'));
  socket.on('error', error => { console.error(error.message); process.exitCode = 1; });
} else {
  console.error('Unexpected Codex command: ' + args.join(' '));
  process.exitCode = 1;
}
`, { mode: 0o700 });
    const env = {
      ...process.env, HOME: home, CODEX_HOME: join(home, '.codex'),
      PATH: bin + ':' + process.env.PATH, STARTUP_TEST_SOCKET: socketFile,
    };
    child = fork(cli, ['serve', '--codex', '--state', stateFile], { env, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    let log = '';
    const messages = [];
    child.stdout.on('data', chunk => { log += chunk; });
    child.stderr.on('data', chunk => { log += chunk; });
    child.on('message', message => messages.push(message));
    exited = once(child, 'exit');
    const socket = await Promise.race([
      started.promise,
      exited.then(([code, signal]) => { throw new Error('Broker exited before native server startup (' + (signal || code) + '): ' + log); }),
    ]);
    const nativeClosed = once(socket, 'close');
    const descriptor = JSON.parse(await readFile(stateFile, 'utf8'));
    const client = createClient(descriptor);
    for (const [method, path, body] of [
      ['GET', '/v1/capabilities'],
      ['GET', '/v1/sessions'],
      ['POST', '/v1/sessions', { cwd: home, harness: 'codex' }],
      ['POST', '/v1/sessions', { cwd: home, harness: 'claude' }],
    ]) {
      await assert.rejects(client.request(method, path, body, t.signal), error => error.status === 503);
    }
    assert.deepEqual(messages, []);

    assert.equal(child.kill('SIGINT'), true);
    const [code, signal] = await exited;
    assert.equal(signal, null, 'SIGINT must run cleanup instead of terminating the broker directly');
    assert.equal(code, 1, log);
    await nativeClosed;
    await assert.rejects(readFile(stateFile), error => error.code === 'ENOENT');
    assert.deepEqual(messages, [], 'interrupted startup must never announce readiness');
    assert.doesNotMatch(log, /Workspace bridge listening|native-secret-diagnostic|native-private-output/);
    const nativeLog = await readFile(join(home, 'codex-server.log'), 'utf8');
    assert.match(nativeLog, /native-secret-diagnostic/);
    assert.match(nativeLog, /native-private-output/);
  });
});

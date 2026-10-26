import assert from 'node:assert/strict';
import { EventEmitter, once } from 'node:events';
import { createServer } from 'node:http';
import { connect } from 'node:net';
import { PassThrough, Writable } from 'node:stream';
import test from 'node:test';
import { WebSocketServer } from 'ws';
import { createCodexRPC } from '../src/codex-rpc.mjs';

function clock() {
  const timers = new Map();
  let next = 0;
  return {
    timers,
    setTimer(callback) { timers.set(++next, callback); return next; },
    clearTimer(id) { timers.delete(id); },
    expire() { for (const [id, callback] of [...timers]) if (timers.delete(id)) callback(); },
  };
}

// A real WebSocket server behind a fake, raw-byte stdin/stdout proxy. The RPC
// client uses Duplex.from, not the loopback socket, including during HTTP upgrade.
async function fixture(t, options = {}, handshake = 'accept') {
  const events = new EventEmitter();
  const child = new EventEmitter();
  const server = createServer();
  const wss = new WebSocketServer({ noServer: true, perMessageDeflate: false });
  const connections = new Set();
  const f = {
    child, sent: [], textFrames: [], wire: [], notifications: [], requests: [],
    disconnects: [], kills: [], upgrades: [], time: clock(), blocked: false,
    splitIncoming: false, peer: undefined,
    async waitFor(key, count) {
      while (this[key].length < count) await once(events, key);
      return this[key][count - 1];
    },
    receive(message) { this.peer.send(JSON.stringify(message)); },
  };
  function record(key, value) { f[key].push(value); events.emit(key); }
  server.on('connection', socket => {
    connections.add(socket);
    socket.on('close', () => connections.delete(socket));
  });
  server.on('upgrade', (request, socket, head) => {
    record('upgrades', request.headers);
    if (handshake === 'hang') return;
    if (handshake === 'reject') {
      socket.end('HTTP/1.1 403 credential\r\nContent-Length: 10\r\n\r\ncredential');
      return;
    }
    if (handshake === 'redirect') {
      socket.end('HTTP/1.1 302 Found\r\nLocation: ws://127.0.0.1:1/credential\r\nContent-Length: 0\r\n\r\n');
      return;
    }
    if (handshake === 'invalid-accept') {
      socket.end('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: credential\r\n\r\n');
      return;
    }
    wss.handleUpgrade(request, socket, head, peer => {
      f.peer = peer;
      peer.on('error', () => {}); // Malformed peer-message tests can close with an error.
      peer.on('message', (bytes, isBinary) => {
        assert.equal(isBinary, false, 'RPCs are WebSocket text messages');
        f.textFrames.push(bytes.toString());
        record('sent', JSON.parse(bytes.toString()));
      });
    });
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const upstream = connect(server.address().port, '127.0.0.1');
  child.stdin = new Writable({
    write(chunk, _encoding, done) {
      f.wire.push(Buffer.from(chunk));
      if (f.blocked) return; // Hold the actual writable callback, not a fake length.
      upstream.write(chunk, done);
    },
  });
  child.stdout = new PassThrough();
  child.stderr = new PassThrough();
  upstream.on('data', bytes => {
    if (f.splitIncoming) for (const byte of bytes) child.stdout.write(Buffer.from([byte]));
    else child.stdout.write(bytes);
  });
  upstream.on('error', error => child.stdout.destroy(error));
  upstream.on('end', () => child.stdout.end());
  child.kill = signal => { f.kills.push(signal); upstream.destroy(); return true; };
  f.connecting = createCodexRPC({ ...f.time,
    onNotification: e => record('notifications', e),
    onServerRequest: e => record('requests', e),
    onDisconnect: e => record('disconnects', e),
    spawnProcess: (...args) => { f.spawnArgs = args; return child; }, ...options,
  });
  // Register cleanup before awaiting the handshake, including failed handshakes.
  t.after(async () => {
    f.time.expire();
    const rpc = await f.connecting.catch(() => undefined);
    rpc?.close();
    upstream.destroy();
    for (const socket of connections) socket.destroy();
    for (const peer of wss.clients) peer.terminate();
    await new Promise(resolve => wss.close(resolve));
    await new Promise(resolve => server.close(resolve));
  });
  return f;
}

async function openFixture(t, options) {
  const f = await fixture(t, options);
  f.rpc = await f.connecting;
  assert.equal(f.time.timers.size, 0, 'handshake deadline was cleared');
  return f;
}

test('Codex transport upgrades the inherited-home byte proxy, masks frames and routes races', async t => {
  const f = await openFixture(t);
  assert.deepEqual(f.spawnArgs, ['codex', ['app-server', 'proxy'], { stdio: ['pipe', 'pipe', 'pipe'] }]);
  assert.equal(f.upgrades.length, 1);
  assert.equal(f.upgrades[0].host, 'localhost');
  assert.equal(f.upgrades[0].upgrade, 'websocket');
  assert.equal(f.upgrades[0]['sec-websocket-version'], '13');
  assert.equal(Buffer.from(f.upgrades[0]['sec-websocket-key'], 'base64').length, 16);
  assert.equal(f.upgrades[0]['sec-websocket-extensions'], undefined);
  const first = f.rpc.request('thread/read', { threadId: 'one' });
  const second = f.rpc.request('thread/read', { threadId: 'two' });
  await f.waitFor('sent', 2);
  const raw = Buffer.concat(f.wire);
  const frame = raw.subarray(raw.indexOf('\r\n\r\n') + 4);
  assert.equal(frame[0] & 0x0f, 1, 'first frame is text');
  assert.equal(frame[1] & 0x80, 0x80, 'client frames are masked');
  f.receive({ method: 'turn/started', params: { threadId: 'one' } });
  f.receive({ id: f.sent[1].id, result: { value: 2 } });
  f.receive({ id: f.sent[0].id, result: { value: 1 } });
  assert.deepEqual(await Promise.all([first, second]), [{ value: 1 }, { value: 2 }]);
  assert.deepEqual(f.notifications, [{ method: 'turn/started', params: { threadId: 'one' } }]);
  assert.equal(f.time.timers.size, 0);
  f.rpc.notify('initialized');
  assert.deepEqual(await f.waitFor('sent', 3), { method: 'initialized' });
  assert.ok(f.textFrames.every(text => !text.endsWith('\n')), 'one JSON envelope per text message, not JSONL');
});

test('WebSocket fragmentation and split UTF-8 preserve JSON whitespace and wire order', async t => {
  const f = await openFixture(t);
  f.splitIncoming = true;
  const pending = f.rpc.request('initialize', {});
  const { id } = await f.waitFor('sent', 1);
  const text = Buffer.from(JSON.stringify({ method: 'item/agentMessage/delta', params: { delta: '🌲\nhello' } }, null, 2));
  const split = text.indexOf(Buffer.from('🌲')) + 1;
  f.peer.send(text.subarray(0, split), { binary: false, fin: false });
  f.peer.send(text.subarray(split), { binary: false, fin: true });
  f.receive({ method: 'turn/completed', params: {} });
  f.receive({ id, result: {} });
  await pending;
  assert.deepEqual(f.notifications.map(e => e.method), ['item/agentMessage/delta', 'turn/completed']);
  assert.equal(f.notifications[0].params.delta, '🌲\nhello');
});

test('server request with a colliding ID is not a response and close never answers or cancels', async t => {
  const f = await openFixture(t);
  const pending = f.rpc.request('turn/start', {});
  const { id } = await f.waitFor('sent', 1);
  const approval = { id, method: 'item/commandExecution/requestApproval', params: { command: 'secret' } };
  f.receive(approval);
  f.receive({ method: 'serverRequest/resolved', params: { requestId: id } });
  assert.deepEqual(await f.waitFor('requests', 1), approval);
  await f.waitFor('notifications', 1);
  assert.equal(f.sent.length, 1);
  assert.equal(f.time.timers.size, 1);
  f.receive({ id, result: { turn: {} } });
  await pending;
  f.receive({ id: 0, method: 'item/tool/requestUserInput', params: {} });
  await f.waitFor('requests', 2);
  const closed = once(f.peer, 'close');
  f.rpc.close();
  await closed;
  assert.equal(f.sent.length, 1, 'close never answers approval requests or sends interrupts');
  assert.deepEqual(f.kills, ['SIGTERM']);
  f.rpc.close();
  assert.equal(f.disconnects.length, 1);
});

test('unknown IDs and method names cannot settle requests or invoke transport commands', async t => {
  const f = await openFixture(t);
  const pending = f.rpc.request('thread/read', {});
  const { id } = await f.waitFor('sent', 1);
  f.receive({ id: 1, result: {} });
  f.receive({ id: '1', result: {} });
  f.receive({ method: 'turn/started', params: {} });
  await f.waitFor('notifications', 1);
  assert.equal(f.time.timers.size, 1);
  await assert.rejects(f.rpc.request('turn/start/extra', {}), /Unsupported/);
  await assert.rejects(f.rpc.request('__proto__', {}), /Unsupported/);
  assert.throws(() => f.rpc.notify('turn/interrupt'), /Unsupported/);
  f.receive({ id, result: { okay: true } });
  assert.deepEqual(await pending, { okay: true });
});

test('request rejection excludes native diagnostics and recognizes only exact disabled-goal validation', async t => {
  const f = await openFixture(t);
  for (const [method, code, message, reason] of [
    ['turn/start', -32600, 'password=secret', undefined],
    ['thread/goal/get', -32600, 'goals feature is disabled', 'goalsDisabled'],
    ['thread/goal/get', -32603, 'goals feature is disabled', undefined],
  ]) {
    const count = f.sent.length + 1;
    const pending = f.rpc.request(method, {});
    const { id } = await f.waitFor('sent', count);
    f.receive({ id, error: { code, message, data: 'credential' } });
    await assert.rejects(pending, error => error.status === 409 && error.rejected && error.reason === reason &&
      !JSON.stringify(error).includes('credential') && !error.message.includes('secret'));
  }
  assert.equal(f.disconnects.length, 0);
});

test('timeout faults all outstanding RPCs once, without retrying a mutation', async t => {
  const f = await openFixture(t);
  const first = f.rpc.request('turn/start', {});
  const second = f.rpc.request('thread/read', {});
  const rejected = Promise.all([assert.rejects(first, /timed out/), assert.rejects(second, /timed out/)]);
  await f.waitFor('sent', 2);
  f.time.expire();
  await rejected;
  assert.equal(f.sent.length, 2);
  assert.equal(f.time.timers.size, 0);
  assert.equal(f.disconnects.length, 1);
  assert.deepEqual(f.kills, ['SIGTERM']);
  await assert.rejects(f.rpc.request('turn/start', {}), /timed out/);
  assert.equal(f.sent.length, 2);
});

test('pending request and outgoing byte budgets reject or disconnect safely', async t => {
  const f = await openFixture(t, { maxPending: 1, maxFrameBytes: 200 });
  const first = f.rpc.request('thread/read', {});
  await assert.rejects(f.rpc.request('thread/read', {}), /budget/);
  const { id } = await f.waitFor('sent', 1);
  f.receive({ id, result: {} });
  await first;
  await assert.rejects(f.rpc.request('turn/start', { text: 'x'.repeat(201) }), /size limit/);
  assert.equal(f.sent.length, 1);
  assert.equal(f.time.timers.size, 0);
  const g = await openFixture(t, { maxOutputBytes: 1 });
  await assert.rejects(g.rpc.request('initialize', {}), /size limit/);
});

test('backpressure includes outstanding proxy writes and releases budget after draining', async t => {
  const f = await openFixture(t, { maxOutputBytes: 150 });
  for (let count = 1; count <= 3; count++) {
    const pending = f.rpc.request('thread/read', {});
    const { id } = await f.waitFor('sent', count);
    f.receive({ id, result: {} });
    await pending;
  }
  f.blocked = true;
  const first = f.rpc.request('thread/read', {});
  const rejected = assert.rejects(first, /size limit/);
  // Let Duplex.from finish its write callbacks while the child remains blocked.
  f.receive({ method: 'test/barrier', params: {} });
  await f.waitFor('notifications', 1);
  assert.ok(f.child.stdin.writableLength > 0);
  await assert.rejects(f.rpc.request('thread/read', {}), /size limit/);
  await rejected;
  assert.equal(f.sent.length, 3, 'blocked requests are not resent');
  assert.equal(f.time.timers.size, 0);
});

test('ping receives a masked pong without bypassing output backpressure', async t => {
  const f = await openFixture(t, { maxOutputBytes: 150 });
  const pong = once(f.peer, 'pong');
  f.peer.ping('keepalive');
  assert.equal((await pong)[0].toString(), 'keepalive');
  f.blocked = true;
  f.peer.ping('x'.repeat(125));
  f.receive({ method: 'test/barrier', params: {} });
  await f.waitFor('notifications', 1);
  f.peer.ping('x'.repeat(125));
  await f.waitFor('disconnects', 1);
  assert.match(f.disconnects[0].message, /size limit/);
});

for (const [name, deliver] of [
  ['oversized message', f => f.peer.send('x'.repeat(201))],
  ['oversized fragmented message', f => {
    f.peer.send('x'.repeat(150), { fin: false });
    f.peer.send('x'.repeat(51), { fin: false });
  }],
  ['invalid JSON', f => f.peer.send('not json')],
  ['JSONL in one text message', f => f.peer.send('{}\n{}\n')],
  ['binary message', f => f.peer.send(Buffer.from('{}'))],
  ['invalid UTF-8', f => f.peer.send(Buffer.from([0xff]), { binary: false })],
  ['array envelope', f => f.receive([])],
  ['ambiguous response', f => f.receive({ id: f.sent[0].id, result: {}, error: {} })],
  ['response with invalid error', f => f.receive({ id: f.sent[0].id, error: { message: 'secret' } })],
  ['request without params', f => f.receive({ id: 1, method: 'item/tool/call' })],
  ['read error', f => f.child.stdout.emit('error', new Error('credential'))],
  ['write error', f => f.child.stdin.emit('error', new Error('credential'))],
  ['spawn error', f => f.child.emit('error', new Error('credential'))],
  ['stderr error', f => f.child.stderr.emit('error', new Error('credential'))],
  ['process exit', f => f.child.emit('exit', 1)],
  ['peer close', f => f.peer.close(1000, 'credential')],
  ['truncated EOF', f => {
    f.peer.send('{', { fin: false }, () => f.peer.terminate());
  }],
]) test(`Codex RPC ${name} is bounded and sanitized`, async t => {
  const f = await openFixture(t, { maxFrameBytes: 200 });
  const pending = f.rpc.request('initialize', {});
  const rejected = assert.rejects(pending, error => error.status === 502 && !error.rejected && !error.message.includes('credential'));
  await f.waitFor('sent', 1);
  deliver(f);
  await rejected;
  assert.equal(f.time.timers.size, 0);
  assert.equal(f.disconnects.length, 1);
  assert.deepEqual(f.kills, ['SIGTERM']);
});

for (const handshake of ['reject', 'redirect', 'invalid-accept', 'hang']) {
  test(`WebSocket handshake ${handshake} fails without diagnostics, reconnect or TCP fallback`, async t => {
    const f = await fixture(t, {}, handshake);
    const rejected = assert.rejects(f.connecting, error => error.status === 502 && !error.message.includes('credential'));
    await f.waitFor('upgrades', 1);
    if (handshake === 'hang') f.time.expire();
    await rejected;
    assert.equal(f.upgrades.length, 1);
    assert.equal(f.sent.length, 0);
    assert.equal(f.disconnects.length, 1);
    assert.equal(f.time.timers.size, 0);
    assert.deepEqual(f.kills, ['SIGTERM']);
  });
}

test('synchronous spawn failure rejects the factory and never leaks diagnostics', async () => {
  const time = clock();
  await assert.rejects(createCodexRPC({ ...time, spawnProcess() { throw new Error('credential'); } }),
    error => error.status === 502 && !error.message.includes('credential'));
  assert.equal(time.timers.size, 0);
});

test('transport limits cannot be raised above their hard caps', async () => {
  for (const options of [{ timeoutMs: 30001 }, { maxFrameBytes: 1048577 }, { maxOutputBytes: 1048577 },
    { maxPending: 33 }, { timeoutMs: 0 }, { maxFrameBytes: NaN }]) {
    await assert.rejects(createCodexRPC({ ...options, spawnProcess() { assert.fail('must validate before spawning'); } }), /Invalid Codex RPC limit/);
  }
});

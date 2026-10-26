import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { once } from 'node:events';
import test from 'node:test';
import { createOpenCodeClient, isOpenCodeMessageId, isOpenCodeSessionId } from '../src/opencode-rpc.mjs';

function deferred() {
  let resolve;
  const promise = new Promise(r => { resolve = r; });
  return { promise, resolve };
}
async function fixture(t, options = {}) {
  const calls = [];
  const disconnected = deferred();
  const observed = [];
  const events = new Map();
  const sockets = new Set();
  let stream;
  const server = createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    calls.push({ method: request.method, url: request.url, authorization: request.headers.authorization,
      body: chunks.length ? JSON.parse(Buffer.concat(chunks).toString()) : undefined });
    if (request.url === '/api/info') {
      response.setHeader('content-type', 'application/json');
      response.end(JSON.stringify({ version: options.version ?? '2.0.28' }));
    } else if (request.url === '/api/event') {
      stream = response;
      response.setHeader('content-type', options.streamType ?? 'text/event-stream');
      response.flushHeaders();
      if (options.initial !== false) response.write(options.initial ?? 'event: message\r\ndata: {"id":"evt_connected","type":"server.connected","data":{}}\r\n\r\n');
    } else if (options.handler) options.handler(request, response);
    else {
      response.setHeader('content-type', 'application/json');
      response.end('{"data":{"ok":true}}');
    }
  });
  server.on('connection', socket => { sockets.add(socket); socket.on('close', () => sockets.delete(socket)); });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  t.after(async () => {
    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => server.close(resolve));
  });
  const url = 'http://127.0.0.1:' + server.address().port;
  const f = {
    calls, url, disconnected: disconnected.promise, observed,
    write: text => stream.write(text), end: () => stream.end(),
    next(type) { const d = deferred(); events.set(type, d); return d.promise; },
    async connect(overrides = {}) {
      const client = await createOpenCodeClient({ url, password: 'private-password',
        onDisconnect: () => disconnected.resolve(),
        onEvent: event => { observed.push(event); events.get(event.type)?.resolve(event); }, ...overrides });
      t.after(() => client.close());
      return client;
    },
  };
  return f;
}

test('native generated ID validators have exact prefixes, hex timestamps and base62 suffixes', () => {
  assert.ok(isOpenCodeSessionId('ses_abcdef012345ABCDEFGHIJKLMN'));
  assert.ok(isOpenCodeMessageId('msg_abcdef012345ABCDEFGHIJKLMN'));
  for (const value of ['ses_', 'ses_x', 'ses_ABCDEF012345ABCDEFGHIJKLMN', 'ses_abcdef012345ABCDEFGHIJKLM_',
    'ses_abcdef012345ABCDEFGHIJKLMNO', 'opencode:ses_abcdef012345ABCDEFGHIJKLMN', null]) assert.equal(isOpenCodeSessionId(value), false);
});

test('authenticates HTTP and SSE and decodes split UTF8, CRLF, comments and multiline data', async t => {
  const f = await fixture(t);
  const client = await f.connect();
  assert.deepEqual(await client.request('POST', '/api/session', { location: { directory: '/workspace' } }), { data: { ok: true } });
  const next = f.next('session.text.delta');
  const payload = Buffer.from(': comment\r\n\r\nevent: message\r\ndata: {"id":"evt_text","type":"session.text.delta",\r\ndata: "data":{"text":"🌏"}}\r\n\r\n');
  const split = payload.indexOf(Buffer.from('🌏')) + 2;
  f.write(payload.subarray(0, split));
  f.write(payload.subarray(split));
  assert.equal((await next).data.text, '🌏');
  assert.ok(f.calls.every(call => call.authorization === 'Basic ' + Buffer.from('opencode:private-password').toString('base64')));
  assert.deepEqual(f.calls.map(call => call.url), ['/api/info', '/api/event', '/api/session']);
  await client.close();
  assert.equal(f.calls.some(call => call.url.includes('interrupt') || call.method === 'DELETE'), false);
});

for (const version of ['2.0.27', '2.0.280', '2.0.28-beta']) test(`transport rejects version ${version} before opening events or mutations`, async t => {
  const f = await fixture(t, { version });
  await assert.rejects(f.connect(), /exact version/);
  assert.deepEqual(f.calls.map(call => call.url), ['/api/info']);
});

for (const initial of ['event: message\ndata: {"id":"evt_bad","type":"session.text.delta","data":{}}\n\n', 'data: not-json\n\n']) {
  test('malformed initial SSE marker fails closed and redacts payloads', async t => {
    const f = await fixture(t, { initial });
    await assert.rejects(f.connect(), error => !error.message.includes('not-json'));
    await f.disconnected;
  });
}

test('SSE clean EOF is a loss of observation, not silent success or reconnect', async t => {
  const f = await fixture(t);
  const client = await f.connect();
  f.end();
  await f.disconnected;
  await assert.rejects(client.request('POST', '/api/session', {}), /disconnected/);
  assert.equal(f.calls.filter(call => call.url === '/api/event').length, 1);
});

for (const failure of ['effect', 'json', 'oversized', 'marker']) test(`SSE ${failure} fails closed without propagating private data`, async t => {
  const f = await fixture(t);
  const client = await f.connect({ maxEventBytes: 512 });
  if (failure === 'effect') f.write('event: effect/http-api/stream/failure\ndata: [{"secret":"credential"}]\n\n');
  if (failure === 'json') f.write('data: secret-credential\n\n');
  if (failure === 'oversized') f.write('data: ' + 's'.repeat(513));
  if (failure === 'marker') f.write('data: {"id":"evt_second","type":"server.connected","data":{}}\n\n');
  await f.disconnected;
  assert.equal(f.observed.length, 0);
  await assert.rejects(client.request('GET', '/api/session'), error => !error.message.includes('credential'));
});

test('request rejection is sanitized and never retried; 5xx mutations are ambiguous and disconnect', async t => {
  let status = 409;
  const f = await fixture(t, { handler: (_request, response) => {
    response.statusCode = status;
    response.end('{"message":"private token and prompt"}');
  } });
  const client = await f.connect();
  await assert.rejects(client.request('POST', '/api/session', {}), error => error.rejected && !error.message.includes('private'));
  status = 500;
  await assert.rejects(client.request('POST', '/api/session', {}), error => !error.rejected && /unknown/.test(error.message));
  await f.disconnected;
  assert.equal(f.calls.filter(call => call.method === 'POST').length, 2);
});

for (const bad of ['oversized', 'invalid json', 'wrong content type', 'redirect']) test(`HTTP ${bad} fails bounded and does not forward credentials`, async t => {
  const f = await fixture(t, { handler: (_request, response) => {
    if (bad === 'redirect') { response.writeHead(302, { location: 'http://example.invalid/secret' }); response.end(); return; }
    response.setHeader('content-type', bad === 'wrong content type' ? 'text/html' : 'application/json');
    response.end(bad === 'oversized' ? JSON.stringify({ text: 'x'.repeat(1025) }) : bad === 'invalid json' ? 'secret-invalid' : '{}');
  } });
  const client = await f.connect({ maxResponseBytes: 1024 });
  await assert.rejects(client.request('GET', '/api/session'), error => !error.message.includes('secret'));
  await f.disconnected;
  assert.equal(f.calls.length, 3);
});

test('bounded HTTP timeout does not retry a native mutation', async t => {
  const f = await fixture(t, { handler: () => {} });
  const client = await f.connect({ requestTimeoutMs: 50 });
  await assert.rejects(client.request('POST', '/api/session', {}), /unknown/);
  await f.disconnected;
  assert.equal(f.calls.filter(call => call.method === 'POST').length, 1);
});

test('event idle and initial-marker timeouts are bounded', async t => {
  const f = await fixture(t, { initial: false });
  await assert.rejects(f.connect({ requestTimeoutMs: 50 }), /disconnected/);
  const g = await fixture(t);
  await g.connect({ idleTimeoutMs: 50 });
  await g.disconnected;
});

test('closing the client aborts outstanding reads, not native work', async t => {
  const received = deferred();
  const f = await fixture(t, { handler: () => received.resolve() });
  const client = await f.connect();
  const request = client.request('POST', '/api/session', {});
  await received.promise;
  await client.close();
  await assert.rejects(request, /unknown/);
  assert.equal(f.calls.filter(call => call.method !== 'GET').length, 1);
});

for (const url of ['http://example.com:4096', 'https://127.0.0.1:4096', 'http://127.0.0.1:4096/path',
  'http://user:secret@127.0.0.1:4096', 'http://127.0.0.1:4096/?secret', 'http://localhost:4096']) {
  test('unsafe or non-private endpoint is rejected before sending credentials', async () => {
    let fetched = false;
    await assert.rejects(createOpenCodeClient({ url, password: 'secret', fetch: () => { fetched = true; } }), /Invalid private/);
    assert.equal(fetched, false);
  });
}

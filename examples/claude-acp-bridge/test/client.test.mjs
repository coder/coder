import assert from 'node:assert/strict';
import { mkdir, mkdtemp, rm, writeFile, chmod, symlink } from 'node:fs/promises';
import test from 'node:test';
import { connect, createClient } from '../src/client.mjs';

const descriptor = { url: 'http://127.0.0.1:43210', token: 'a'.repeat(64) };

test('descriptor must be private and point to authenticated loopback HTTP', async t => {
  await mkdir('/tmp/opencode', { recursive: true });
  const directory = await mkdtemp('/tmp/opencode/bridge-client-test-');
  t.after(() => rm(directory, { recursive: true }));
  const path = directory + '/bridge.json';
  await writeFile(path, JSON.stringify(descriptor), { mode: 0o600 });
  assert.equal(typeof (await connect(path)).request, 'function');
  await chmod(path, 0o644);
  await assert.rejects(connect(path), /other users/);
  await chmod(path, 0o600);
  for (const change of [{ url: 'https://example.com' }, { url: 'http://localhost:1234' }, { url: 'http://127.0.0.1:43210/path' }, { url: 'http://secret@127.0.0.1:43210' }, { token: '' }]) {
    await writeFile(path, JSON.stringify({ ...descriptor, ...change }));
    await assert.rejects(connect(path), /Invalid loopback/);
  }
});

test('supplied cancellation signals do not disable HTTP deadlines', async t => {
  const deadline = new AbortController();
  const timeouts = [];
  t.mock.method(AbortSignal, 'timeout', ms => { timeouts.push(ms); return deadline.signal; });
  const requests = [];
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    requests.push({ url, ...init });
    return Response.json({ ok: true });
  });
  const external = new AbortController();
  const client = createClient(descriptor);
  await client.request('POST', '/v1/sessions', { cwd: '/workspace' }, external.signal);
  await client.request('GET', '/v1/sessions', undefined, external.signal);
  assert.deepEqual(timeouts, [65000, 25000]);
  assert.equal(requests[0].redirect, 'error');
  assert.equal(requests[0].headers.Authorization, 'Bearer ' + descriptor.token);
  deadline.abort();
  assert.ok(requests.every(request => request.signal.aborted));
});

test('ambiguous mutation failures are never retried', async t => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => { throw new Error('response lost'); });
  await assert.rejects(createClient(descriptor).request('POST', '/v1/sessions', { cwd: '/workspace' }), /Work state is unknown/);
  assert.equal(fetch.mock.callCount(), 1);
});

test('missing, malformed and symlink descriptors fail without exposing private contents', async t => {
  const directory = await mkdtemp('/tmp/opencode/bridge-descriptor-test-');
  t.after(() => rm(directory, { recursive: true }));
  const path = directory + '/bridge.json';
  await assert.rejects(connect(path), /descriptor missing.*Start the broker explicitly/);
  for (const text of ['{"token":"' + descriptor.token + '"', JSON.stringify({ url: descriptor.token, token: descriptor.token }), 'null']) {
    await writeFile(path, text, { mode: 0o600 });
    await assert.rejects(connect(path), error => {
      assert.match(error.message, /Invalid.*descriptor/);
      assert.ok(!error.message.includes(descriptor.token));
      return true;
    });
  }
  await symlink(path, directory + '/link.json');
  await assert.rejects(connect(directory + '/link.json'), /symlinks are not supported/);
  await assert.rejects(connect(directory), /regular file/);
});

test('offline and stale broker diagnostics never expose tokens or retry', async t => {
  const client = createClient(descriptor);
  const fetch = t.mock.method(globalThis, 'fetch', async () => { throw new Error(descriptor.token); });
  await assert.rejects(client.request('GET', '/v1/sessions'), error => {
    assert.match(error.message, /offline.*stale/);
    assert.ok(!error.message.includes(descriptor.token));
    return true;
  });
  for (const status of [401, 403]) {
    fetch.mock.mockImplementation(async () => Response.json({ error: descriptor.token }, { status }));
    await assert.rejects(client.request('GET', '/v1/sessions'), error => {
      assert.equal(error.status, status);
      assert.match(error.message, /stale or authentication/);
      assert.ok(!error.message.includes(descriptor.token));
      return true;
    });
  }
  fetch.mock.mockImplementation(async () => new Response(descriptor.token));
  await assert.rejects(client.request('GET', '/v1/sessions'), /Invalid broker response/);
  fetch.mock.mockImplementation(async () => Response.json({ error: 'failure: ' + descriptor.token }, { status: 500 }));
  await assert.rejects(client.request('GET', '/v1/sessions'), /failure: \[redacted\]/);
  assert.equal(fetch.mock.callCount(), 5);
});

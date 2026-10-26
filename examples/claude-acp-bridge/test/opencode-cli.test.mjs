import assert from 'node:assert/strict';
import { execFile, fork } from 'node:child_process';
import { once } from 'node:events';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import test from 'node:test';
import { connect } from '../src/client.mjs';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../src/cli.mjs', import.meta.url));

test('isolated OpenCode CLI: one native identity, passive Open, Observe and retained archive', {
  skip: process.env.OPENCODE_SERVER_SMOKE !== '1', timeout: 60000,
}, async t => {
  const { shutdownOpenCodeServer } = await import('../src/opencode-server.mjs');
  const { stdout } = await execute('which', ['opencode']);
  const binary = stdout.trim();
  assert.ok(binary.startsWith('/') && !binary.includes("'"));
  const home = await mkdtemp('/tmp/opencode/bridge-opencode-cli-');
  const bin = join(home, 'bin');
  const terminalBin = join(home, 'terminal-bin');
  const mismatchBin = join(home, 'mismatch-bin');
  await mkdir(bin, { mode: 0o700 });
  await mkdir(terminalBin, { mode: 0o700 });
  await mkdir(mismatchBin, { mode: 0o700 });
  const stateFile = join(home, 'bridge/bridge.json');
  const capture = join(home, 'terminal.json');
  const mismatchCapture = join(home, 'mismatch.jsonl');
  await writeFile(join(mismatchBin, 'opencode'), `#!${process.execPath}
require('node:fs').appendFileSync(${JSON.stringify(mismatchCapture)}, JSON.stringify(process.argv.slice(2)) + '\\n', { mode: 0o600 });
process.stdout.write('opencode v2.0.29\\n');
`, { mode: 0o700 });
  await writeFile(join(bin, 'claude'), '#!/bin/sh\nprintf "[]"\n', { mode: 0o700 });
  await writeFile(join(bin, 'capture.cjs'), `const fs = require('node:fs');
fs.writeFileSync(${JSON.stringify(capture)}, JSON.stringify({ args: process.argv.slice(2), home: process.env.HOME, databaseOverride: process.env.OPENCODE_DB, passwordSet: Boolean(process.env.OPENCODE_PASSWORD) }));
`, { mode: 0o600 });
  await writeFile(join(terminalBin, 'opencode'), `#!/bin/sh
case "$1" in
  --server) exec '${process.execPath}' '${join(bin, 'capture.cjs')}' "$@" ;;
  *) exec '${binary}' "$@" ;;
esac
`, { mode: 0o700 });
  const env = { ...process.env, HOME: home, PATH: bin + ':' + process.env.PATH, OPENCODE_DISABLE_MODELS_FETCH: 'true', OPENCODE_DB: '/must-not-touch-shared-database' };
  let child;
  const stop = async () => {
    if (child && child.exitCode === null && child.signalCode === null) {
      const exited = once(child, 'exit');
      child.kill('SIGTERM');
      await exited;
    }
  };
  t.after(async () => {
    await stop();
    await shutdownOpenCodeServer({ stateFile });
    await rm(home, { recursive: true, force: true });
  });
  const start = async () => {
    child = fork(cli, ['serve', '--opencode', '--state', stateFile], { env, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
    let log = '';
    child.stdout.on('data', chunk => { log += chunk; });
    child.stderr.on('data', chunk => { log += chunk; });
    await new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('exit', () => reject(new Error('Fixture broker exited: ' + log)));
      child.once('message', message => { assert.equal(message.type, 'bridge-ready'); resolve(); });
    });
    return connect(stateFile);
  };
  const run = (args, overrides = {}) => execute(process.execPath, [cli, ...args, '--state', stateFile], { env: { ...env, PATH: terminalBin + ':' + env.PATH, ...overrides }, cwd: home, timeout: 15000 });
  let client = await start();
  assert.deepEqual((await client.request('GET', '/v1/capabilities')).harnesses.map(h => h.id), ['claude', 'opencode']);
  const session = JSON.parse((await run(['start', '--harness', 'opencode', '--cwd', home])).stdout);
  assert.equal(session.harness, 'opencode');
  const path = '/v1/sessions/' + encodeURIComponent(session.sessionId);
  await run(['control', session.sessionId, 'automation']);
  assert.equal((await client.request('GET', path)).mode, 'automation');
  const serverDescriptor = await readFile(join(home, 'bridge/opencode-server.json'));
  await assert.rejects(run(['attach', session.sessionId], { PATH: mismatchBin + ':' + env.PATH }), error => {
    assert.match(error.stderr, /CLI must be exactly version 2\.0\.28/);
    return true;
  });
  assert.equal((await client.request('GET', path)).mode, 'automation');
  assert.deepEqual(await readFile(join(home, 'bridge/opencode-server.json')), serverDescriptor);
  assert.deepEqual((await readFile(mismatchCapture, 'utf8')).trim().split('\n').map(JSON.parse), [['--version']]);
  await assert.rejects(readFile(capture), { code: 'ENOENT' });
  await run(['attach', session.sessionId]);
  const terminal = JSON.parse(await readFile(capture, 'utf8'));
  assert.deepEqual(terminal.args.slice(2), ['--session', session.nativeSessionId, home]);
  assert.equal(terminal.args[0], '--server');
  assert.equal(terminal.home, join(home, 'bridge/opencode-home'));
  assert.equal(terminal.databaseOverride, undefined);
  assert.equal(terminal.passwordSet, true);
  assert.equal((await client.request('GET', path)).mode, 'human');
  await run(['attach', session.sessionId]);
  await run(['control', session.sessionId, 'automation']);
  assert.equal((await client.request('GET', path)).mode, 'automation');
  assert.equal((await client.request('GET', '/v1/sessions')).sessions.length, 1);
  const before = await client.request('GET', path + '/events?after=0');
  assert.ok(!before.events.some(event => event.type === 'turn.start'));
  await stop();
  await shutdownOpenCodeServer({ stateFile });
  client = await start();
  assert.equal((await client.request('GET', path)).archived, true);
  await assert.rejects(run(['attach', session.sessionId]), error => /Workspace startup restores saved sessions/.test(error.stderr));
  await client.request('POST', path + '/resume', {});
  await run(['attach', session.sessionId]);
  assert.equal((await client.request('GET', path)).nativeSessionId, session.nativeSessionId);
  const after = await client.request('GET', path + '/events?after=0');
  assert.ok(!after.events.some(event => event.type === 'turn.start'));
});

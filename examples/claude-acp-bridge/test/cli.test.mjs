import assert from 'node:assert/strict';
import { execFile, fork } from 'node:child_process';
import { once } from 'node:events';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { describe, test } from 'node:test';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../src/cli.mjs', import.meta.url));
const secret = 'native-cli-secret-must-not-be-returned';

async function fixture(t, fail = false) {
  const home = await mkdtemp('/tmp/opencode/bridge-cli-');
  const bin = join(home, 'bin');
  await mkdir(bin, { mode: 0o700 });
  const stateFile = join(home, 'bridge/bridge.json');
  const env = { ...process.env, HOME: home, PATH: bin + ':' + process.env.PATH, NATIVE_TEST_FAIL: fail ? '1' : '', NATIVE_TEST_SECRET: secret };
  // This fixture never launches a native session. It emulates only the documented
  // structured CLI/state contract, including native 755 job directory permissions.
  await writeFile(join(bin, 'claude'), `#!${process.execPath}
const fs = require('node:fs');
const path = require('node:path');
const args = process.argv.slice(2);
const root = path.join(process.env.HOME, '.claude');
const jobs = path.join(root, 'jobs');
const catalog = path.join(root, 'agents.json');
if (args[0] === 'agents') {
  process.stdout.write(fs.existsSync(catalog) ? fs.readFileSync(catalog) : '[]');
} else if (process.env.NATIVE_TEST_FAIL || args[0] === 'stop') {
  process.stderr.write(process.env.NATIVE_TEST_SECRET);
  process.exitCode = 1;
} else if (args[0] === '--bg') {
  const sessionId = require('node:crypto').randomUUID();
  const id = sessionId.slice(0, 8);
  fs.mkdirSync(root, { recursive: true, mode: 0o700 });
  fs.mkdirSync(jobs, { mode: 0o755 });
  const directory = path.join(jobs, id);
  fs.mkdirSync(directory, { mode: 0o755 });
  const state = { sessionId, cwd: process.cwd(), state: 'blocked', detail: '3 new MCP servers need approval',
    providerEnv: { ANTHROPIC_API_KEY: process.env.NATIVE_TEST_SECRET },
    respawnFlags: ['--plugin-dir', args[args.indexOf('--plugin-dir') + 1], '--settings', args[args.indexOf('--settings') + 1]] };
  fs.writeFileSync(path.join(directory, 'state.json'), JSON.stringify(state), { mode: 0o600 });
  fs.writeFileSync(catalog, JSON.stringify([{ id, sessionId, cwd: state.cwd, state: state.state, kind: 'background', startedAt: 1792761788871 }]));
  process.stdout.write('Unstructured launch output must not supply identity: ' + process.env.NATIVE_TEST_SECRET);
} else { process.exitCode = 1; }
`, { mode: 0o700 });
  const child = fork(cli, ['serve', '--state', stateFile], { env, execArgv: [], stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  let log = '';
  child.stdout.on('data', chunk => { log += chunk; });
  child.stderr.on('data', chunk => { log += chunk; });
  t.after(async () => {
    if (child.exitCode === null && child.signalCode === null) {
      const exited = once(child, 'exit');
      child.kill('SIGTERM');
      await exited;
    }
    await rm(home, { recursive: true, force: true });
  });
  await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', () => reject(new Error('Fixture broker exited before readiness')));
    child.once('message', message => { assert.equal(message.type, 'bridge-ready'); resolve(); });
  });
  const run = args => execute(process.execPath, [cli, ...args, '--state', stateFile], { env, cwd: home, timeout: 5000 });
  return { home, run, log: () => log };
}

describe('native CLI adapter', { concurrency: true, timeout: 10000 }, () => {
  test('start returns blocked native identity immediately without parsing output or approving permissions', async t => {
    const f = await fixture(t);
    const result = await f.run(['start', '--cwd', f.home]);
    const session = JSON.parse(result.stdout);
    assert.match(session.sessionId, /^[0-9a-f-]{36}$/);
    assert.equal(session.registrationPending, true);
    assert.equal(session.blockedReason, 'Approve project MCP servers in the native terminal.');
    assert.equal(session.mode, 'human');
    assert.equal(session.runtimeId, undefined);
    assert.deepEqual(JSON.parse((await f.run(['list'])).stdout).sessions, [session]);
    assert.deepEqual(JSON.parse((await f.run(['inspect', session.sessionId])).stdout), session);
    await f.run(['control', session.sessionId, 'human']);
    await assert.rejects(f.run(['control', session.sessionId, 'automation']), error => /registration is pending/.test(error.stderr));
    await assert.rejects(f.run(['stop', session.sessionId]), error => /Native Claude stop failed/.test(error.stderr) && !error.stderr.includes(secret));
    assert.ok(!(result.stdout + result.stderr + f.log()).includes(secret));
  });

  test('disabled harness selection cannot create a Claude session', async t => {
    const f = await fixture(t);
    for (const [harness, title] of [['codex', 'Codex'], ['opencode', 'OpenCode']]) {
      await assert.rejects(f.run(['start', '--harness', harness, '--cwd', f.home]), error => error.stderr.includes('does not advertise ' + title));
    }
    await assert.rejects(f.run(['start', '--harness', '', '--cwd', f.home]), error => /--harness/.test(error.stderr));
    assert.deepEqual(JSON.parse((await f.run(['list'])).stdout).sessions, []);
    await assert.rejects(f.run(['list', '--opencode']), error => /only valid with serve/.test(error.stderr));
  });

  test('launch failure never leaks native stderr, stdout or command settings', async t => {
    const f = await fixture(t, true);
    await assert.rejects(f.run(['start', '--cwd', f.home]), error => {
      assert.equal(error.stdout, '');
      assert.match(error.stderr, /Native Claude launch failed/);
      assert.ok(!error.stderr.includes(secret));
      assert.ok(!error.stderr.includes('WORKSPACE_BRIDGE'));
      return true;
    });
    assert.ok(!f.log().includes(secret));
  });
});

import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';
import { stopWorkspaceClaude, stopWorkspaceNative } from '../template/image/shutdown.mjs';

const execute = promisify(execFile);
const domain = 'linux::pid:[1234]';
const secret = 'private-native-output';

async function fixture(t) {
  const home = await mkdtemp('/tmp/opencode/native-shutdown-test-');
  t.after(() => rm(home, { recursive: true, force: true }));
  const root = join(home, '.claude');
  for (const directory of [root, join(root, 'daemon'), join(root, 'sessions'), join(root, 'jobs')]) await mkdir(directory, { mode: 0o700 });
  const put = (path, value) => writeFile(join(root, path), JSON.stringify(value), { mode: 0o600 });
  const supervisor = 2147483600;
  const worker = supervisor + 1;
  const repl = supervisor + 2;
  const roster = { proto: 1, supervisorPid: supervisor, workers: {
    abcdef12: { pid: worker, replPid: repl, cliVersion: '2.1.309' },
  } };
  const session = { pid: repl, pidDomain: domain, version: '2.1.309', kind: 'bg' };
  await put('daemon.lock', { pid: supervisor, version: '2.1.309' });
  await put('daemon/roster.json', roster);
  await put('sessions/' + repl + '.json', session);
  await put('jobs/state.json', { transcript: secret });
  const originals = new Map();
  for (const path of ['daemon.lock', 'daemon/roster.json', 'sessions/' + repl + '.json', 'jobs/state.json']) originals.set(path, await readFile(join(root, path), 'utf8'));
  const unchanged = async () => {
    for (const [path, text] of originals) assert.equal(await readFile(join(root, path), 'utf8'), text);
  };
  const clean = async () => {
    await rm(join(root, 'daemon.lock'), { force: true });
    await put('daemon/roster.json', { ...roster, workers: {} });
    await rm(join(root, 'sessions/' + repl + '.json'), { force: true });
  };
  const live = new Set([supervisor, worker, repl]);
  const probes = [];
  const calls = [];
  let clock = 0;
  const options = {
    env: { HOME: home, CODER_NATIVE_JOB_PROMPT_BASE64: secret, CODER_NATIVE_JOB_CODEX_API_KEY_BASE64: secret },
    identifyDomain: async () => domain,
    now: () => clock,
    wait: async ms => { clock += ms; },
    run: async (...args) => { calls.push(args); },
    probeProcess: pid => {
      probes.push(pid);
      if (!live.has(pid)) throw Object.assign(new Error('gone'), { code: 'ESRCH' });
    },
    timeout: 1000,
  };
  return { home, root, put, clean, live, probes, calls, options, unchanged, roster, session, supervisor, worker, repl };
}

test('shutdown waits past native acknowledgement for every captured PID and native registry cleanup', async t => {
  const f = await fixture(t);
  f.options.timeout = 45000;
  let waits = 0;
  const wait = f.options.wait;
  f.options.wait = async ms => {
    waits++;
    await wait(ms);
    if (waits === 1) {
      // Removing the roster does not prove the captured workers have exited.
      await f.clean();
      f.live.delete(f.supervisor);
      f.live.delete(f.worker);
    } else if (waits === 2) f.live.delete(f.repl);
  };
  await stopWorkspaceClaude(f.options);
  assert.equal(waits, 2);
  assert.equal(f.calls.length, 1);
  const [command, args, options] = f.calls[0];
  assert.equal(command, 'claude');
  assert.deepEqual(args, ['daemon', 'stop', '--any']);
  assert.equal(options.cwd, f.home);
  assert.equal(options.timeout, 4000);
  assert.deepEqual(options.env, { HOME: f.home });
  for (const pid of [f.supervisor, f.worker, f.repl]) assert.ok(f.probes.filter(value => value === pid).length >= 3);
  assert.equal(await readFile(join(f.root, 'jobs/state.json'), 'utf8'), JSON.stringify({ transcript: secret }));
});

test('PID exit alone never repairs or accepts an uncleared native registry', async t => {
  const f = await fixture(t);
  f.live.clear();
  await assert.rejects(stopWorkspaceClaude(f.options), /shutdown timed out/);
  await f.unchanged();
  assert.equal(f.calls.length, 1);
});

test('a surviving supervisor or worker times out even after registry cleanup', async t => {
  for (const survivor of ['supervisor', 'worker', 'repl']) {
    const f = await fixture(t);
    f.live.clear();
    f.live.add(f[survivor]);
    f.options.run = async () => f.clean();
    await assert.rejects(stopWorkspaceClaude(f.options), /shutdown timed out/);
    assert.deepEqual([...f.live], [f[survivor]], 'the hook never kills captured native PIDs');
  }
});

test('cleanup must also cover workers registered while the native stop request is in flight', async t => {
  const f = await fixture(t);
  f.options.run = async () => {
    f.live.clear();
    await f.clean();
    await f.put('daemon/roster.json', { ...f.roster, workers: { extra: { pid: f.worker + 20, cliVersion: '2.1.309' } } });
  };
  await assert.rejects(stopWorkspaceClaude(f.options), /shutdown timed out/);
  assert.ok(f.probes.includes(f.worker + 20));
});

test('foreign PID domains, malformed registries and ambiguous process probes fail without exposing state', async t => {
  for (const scenario of ['foreign', 'invalid-pid', 'invalid-json', 'permission']) {
    const f = await fixture(t);
    if (scenario === 'foreign') await f.put('sessions/' + f.repl + '.json', { ...f.session, pidDomain: 'linux::pid:[9999]' });
    if (scenario === 'invalid-pid') await f.put('daemon.lock', { version: '2.1.309', pid: -1 });
    if (scenario === 'invalid-json') await writeFile(join(f.root, 'daemon/roster.json'), secret);
    if (scenario === 'permission') f.options.probeProcess = () => { throw Object.assign(new Error(secret), { code: 'EPERM' }); };
    const before = await readFile(join(f.root, 'daemon/roster.json'), 'utf8');
    await assert.rejects(stopWorkspaceClaude(f.options), error => {
      assert.match(error.message, /could not be verified/);
      assert.ok(!error.message.includes(secret));
      return true;
    });
    assert.equal(await readFile(join(f.root, 'daemon/roster.json'), 'utf8'), before);
    if (scenario !== 'permission') assert.equal(f.calls.length, 0);
    else await f.unchanged();
  }
});

test('native registry symlinks are refused without following or editing their targets', async t => {
  const f = await fixture(t);
  const path = join(f.root, 'daemon/roster.json');
  const target = join(f.home, 'outside.json');
  const original = await readFile(path, 'utf8');
  await writeFile(target, original);
  await rm(path);
  await symlink(target, path);
  await assert.rejects(stopWorkspaceClaude(f.options), /could not be verified/);
  assert.equal(f.calls.length, 0);
  assert.equal(await readFile(target, 'utf8'), original);
});

test('workspaces without a Claude runtime need no native command and create no state', async t => {
  const f = await fixture(t);
  await rm(f.root, { recursive: true });
  await stopWorkspaceClaude(f.options);
  assert.equal(f.calls.length, 0);
  assert.deepEqual(await readdir(f.home), []);
});

test('native command failure is nonzero, sanitizes diagnostics, and leaves state intact', async t => {
  const f = await fixture(t);
  f.options.run = async () => { throw new Error(secret); };
  await assert.rejects(stopWorkspaceClaude(f.options), error => {
    assert.match(error.message, /could not be verified/);
    assert.ok(!error.message.includes(secret));
    return true;
  });
  await f.unchanged();
  const bin = join(f.home, 'bin');
  await mkdir(bin);
  await writeFile(join(bin, 'claude'), '#!/bin/sh\necho ' + secret + '\nexit 1\n', { mode: 0o700 });
  // No session registry means the actual domain does not need to be mocked.
  await rm(join(f.root, 'sessions/' + f.repl + '.json'));
  await assert.rejects(execute(process.execPath, [new URL('../template/image/shutdown.mjs', import.meta.url).pathname], {
    env: { ...process.env, HOME: f.home, CLAUDE_CONFIG_DIR: '', PATH: bin }, timeout: 10000,
  }), error => {
    assert.equal(error.code, 1);
    assert.equal(error.stdout, '');
    assert.match(error.stderr, /could not be verified/);
    assert.ok(!error.stderr.includes(secret));
    return true;
  });
});

test('workspace shutdown starts both native stops together and waits for both', async () => {
  const calls = [];
  let releaseClaude, releaseOpenCode, bothStarted;
  const claudeDone = new Promise(resolve => { releaseClaude = resolve; });
  const openCodeDone = new Promise(resolve => { releaseOpenCode = resolve; });
  const started = new Promise(resolve => { bothStarted = resolve; });
  const env = { HOME: '/private-home' };
  const signal = new AbortController().signal;
  let finished = false;
  const shutdown = stopWorkspaceNative({ env, root: '/private-root', signal,
    stopClaude: async options => { calls.push(['claude', options]); await claudeDone; },
    stopOpenCode: async options => { calls.push(['opencode', options]); bothStarted(); await openCodeDone; },
  }).then(() => { finished = true; });
  await started;
  assert.deepEqual(calls, [['claude', { env }], ['opencode', { env, root: '/private-root', signal }]]);
  releaseClaude();
  await claudeDone;
  assert.equal(finished, false);
  releaseOpenCode();
  await shutdown;
  assert.equal(finished, true);
});

for (const failing of ['claude', 'opencode']) {
  test('one native stop failure does not skip the other: ' + failing, async () => {
    const calls = [];
    const stop = name => async () => { calls.push(name); if (name === failing) throw new Error(secret); };
    await assert.rejects(stopWorkspaceNative({ stopClaude: stop('claude'), stopOpenCode: stop('opencode') }), error => {
      assert.match(error.message, /shutdown could not be verified/);
      assert.ok(!error.message.includes(secret));
      return true;
    });
    assert.deepEqual(calls.sort(), ['claude', 'opencode']);
  });
}

test('missing OpenCode state never imports the adapter or creates native files', async t => {
  const f = await fixture(t);
  const before = await readdir(f.home);
  await stopWorkspaceNative({ env: f.options.env, root: join(f.home, 'not-installed'), stopClaude: async () => {} });
  assert.deepEqual(await readdir(f.home), before);
  await f.unchanged();
});

test('template hook delegates the dedicated descriptor to the installed lifecycle helper', async t => {
  const f = await fixture(t);
  const root = join(f.home, 'installation');
  const state = join(f.home, '.local/state/coder-acp');
  await mkdir(join(root, 'adapter/src'), { recursive: true, mode: 0o700 });
  await mkdir(state, { recursive: true, mode: 0o700 });
  await writeFile(join(state, 'opencode-server.json'), 'owned native descriptor', { mode: 0o600 });
  await writeFile(join(root, 'adapter/src/opencode-server.mjs'), `
import { writeFile } from 'node:fs/promises';
export async function shutdownOpenCodeServer({ stateFile }) {
  await writeFile(${JSON.stringify(join(f.home, 'called.json'))}, JSON.stringify({ stateFile }), { mode: 0o600 });
}
`);
  await stopWorkspaceNative({ env: f.options.env, root, stopClaude: async () => {} });
  assert.deepEqual(JSON.parse(await readFile(join(f.home, 'called.json'))), { stateFile: join(state, 'bridge.json') });
  assert.equal(await readFile(join(state, 'opencode-server.json'), 'utf8'), 'owned native descriptor');
  await f.unchanged();
});

test('CLI request timeout uses SIGTERM without escalation and does not modify native state', async t => {
  const f = await fixture(t);
  const bin = join(f.home, 'bin');
  await mkdir(bin);
  await writeFile(join(bin, 'claude'), `#!${process.execPath}
import assert from 'node:assert/strict';
assert.deepEqual(process.argv.slice(2), ['daemon', 'stop', '--any']);
assert.equal(process.env.CODER_NATIVE_JOB_PROMPT_BASE64, undefined);
assert.equal(process.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64, undefined);
setInterval(() => {}, 1000);
`, { mode: 0o700 });
  await assert.rejects(stopWorkspaceClaude({ ...f.options, run: undefined, env: { ...f.options.env, PATH: bin }, timeout: 100 }), /shutdown timed out/);
  await f.unchanged();
});

import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { watch } from 'node:fs';
import { chmod, cp, lstat, mkdir, mkdtemp, readFile, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execute = promisify(execFile);
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const secret = 'test-authentication-must-not-be-printed';

async function exists(file) {
  try { await stat(file); return true; }
  catch (error) { if (error.code === 'ENOENT') return false; throw error; }
}

async function stopBroker(file) {
  if (!await exists(file)) return;
  const { pid } = JSON.parse(await readFile(file, 'utf8'));
  if (!pid) return;
  await new Promise((resolve, reject) => {
    const watcher = watch(dirname(file), () => {
      void exists(file).then(present => { if (!present) finish(); }, finish);
    });
    const timer = setTimeout(() => finish(new Error('Test broker did not remove its descriptor')), 5000);
    function finish(error) {
      watcher.close();
      clearTimeout(timer);
      if (error) reject(error);
      else resolve();
    }
    try { process.kill(pid, 'SIGTERM'); }
    catch (error) { finish(error.code === 'ESRCH' ? undefined : error); }
  });
}

async function fixture(t) {
  await mkdir('/tmp/opencode', { recursive: true });
  const directory = await mkdtemp('/tmp/opencode/bridge-install-test-');
  const source = join(directory, "source project's adapter");
  const home = join(directory, "owner's home");
  const bin = join(home, '.local/bin');
  const state = join(home, '.local/state/coder-acp');
  const descriptor = join(state, 'bridge.json');
  const launcher = join(bin, 'coder-acp');
  const commands = join(directory, 'commands');
  t.after(async () => {
    // Every descriptor here belongs to this test's isolated source and home.
    try { await stopBroker(descriptor); }
    finally { await rm(directory, { recursive: true, force: true }); }
  });
  await mkdir(source, { recursive: true });
  await mkdir(commands);
  for (const path of ['install.sh', 'scripts', 'src', 'package.json', 'package-lock.json']) {
    await cp(join(root, path), join(source, path), { recursive: true });
  }
  await symlink(join(root, 'node_modules'), join(source, 'node_modules'));
  await symlink(process.execPath, join(commands, 'node'));
  for (const command of ['claude', 'npm']) {
    await writeFile(join(commands, command), '#!/bin/sh\nprintf called > "$HOME/' + command + '-called"\nexit 99\n', { mode: 0o700 });
  }
  const env = { HOME: home, PATH: commands + ':/usr/bin:/bin', ANTHROPIC_API_KEY: secret };
  const run = (command, args = [], options = {}) => execute(command, args, { env, cwd: directory, timeout: 15000, ...options });
  const install = (...args) => run('/bin/sh', [join(source, 'install.sh'), ...args]);
  return { directory, source, home, bin, state, descriptor, launcher, env, run, install };
}

test('installation safely quotes paths, pins Node, preserves dependencies and does not start anything', async t => {
  const f = await fixture(t);
  const originalLock = await readFile(join(f.source, 'package-lock.json'), 'utf8');
  const result = await f.install();
  assert.match(result.stdout, /Broker not started/);
  assert.ok(!(result.stdout + result.stderr).includes(secret));
  const launcher = await readFile(f.launcher, 'utf8');
  assert.ok(!launcher.includes(secret));
  assert.equal((await stat(f.launcher)).mode & 0o777, 0o700);
  assert.equal((await stat(f.state)).mode & 0o777, 0o700);
  assert.equal((await lstat(join(f.source, 'node_modules'))).isSymbolicLink(), true);
  assert.equal(await readFile(join(f.source, 'package-lock.json'), 'utf8'), originalLock);
  for (const file of [f.descriptor, join(f.home, 'npm-called'), join(f.home, 'claude-called')]) assert.equal(await exists(file), false);
  const help = await f.run(f.launcher, ['--help'], { env: { ...f.env, PATH: '/not/on/path' } });
  assert.match(help.stdout, /Usage: coder-acp/);
  await f.install();
  assert.equal(await readFile(f.launcher, 'utf8'), launcher, 'reinstallation is deterministic');
  await assert.rejects(f.run(f.launcher, ['acp']), error => {
    assert.equal(error.stdout, '');
    assert.match(error.stderr, /descriptor missing.*Start the broker explicitly/);
    return true;
  });
  assert.equal(await exists(f.descriptor), false, 'ACP never starts a broker');
});

test('launcher delegates every command and literal argument without changing cwd', async t => {
  const f = await fixture(t);
  const customBin = join(f.directory, "custom bin's $(not-a-command)");
  const customState = join(f.directory, "custom state's $(not-a-command)");
  await f.install('--bin-dir', customBin, '--state-dir', customState);
  await writeFile(join(f.source, 'src/cli.mjs'), 'console.log(JSON.stringify({ args: process.argv.slice(2), cwd: process.cwd() }));\n');
  for (const args of [
    ['acp'], ['serve'], ['list'], ['attach', "session'; not-a-command"], ['inspect', 'session'],
    ['control', 'session', 'human'], ['stop', 'session'], ['start', '--cwd', "project's directory"],
    ['acp', '--state', '/an explicit override'],
  ]) {
    const result = await f.run(join(customBin, 'coder-acp'), args);
    assert.deepEqual(JSON.parse(result.stdout), { args: ['--state', join(customState, 'bridge.json'), ...args], cwd: f.directory });
  }
});

test('installer refuses unrelated executables, symlinks and unsafe state permissions', async t => {
  const f = await fixture(t);
  await mkdir(f.bin, { recursive: true });
  await writeFile(f.launcher, 'unrelated file\n');
  await assert.rejects(f.install(), /Refusing to replace/);
  assert.equal(await readFile(f.launcher, 'utf8'), 'unrelated file\n');
  await rm(f.launcher);
  await symlink(join(f.source, 'install.sh'), f.launcher);
  await assert.rejects(f.install(), /Refusing to replace/);
  assert.equal((await lstat(f.launcher)).isSymbolicLink(), true);
  await rm(f.launcher);
  await mkdir(f.state, { recursive: true, mode: 0o755 });
  await chmod(f.state, 0o755);
  await assert.rejects(f.install(), /owner-only directory/);
  assert.equal(await exists(f.launcher), false);
  await chmod(f.state, 0o700);
  await f.install();
});

test('installer requires explicit pinned dependencies rather than invoking npm install', async t => {
  const f = await fixture(t);
  await rm(join(f.source, 'node_modules'));
  await assert.rejects(f.install(), /Run npm ci --ignore-scripts/);
  assert.equal(await exists(f.launcher), false);
  assert.equal(await exists(join(f.home, 'npm-called')), false);
});

test('explicit startup detaches one idle broker, keeps logs private and never restarts it for clients', async t => {
  const f = await fixture(t);
  const result = await f.install('--start', '--model', 'test-model-not-used');
  assert.match(result.stdout, /Bridge broker started \(PID \d+\)/);
  const original = await readFile(f.descriptor, 'utf8');
  const { pid, token } = JSON.parse(original);
  const log = join(f.state, 'broker.log');
  for (const file of [f.descriptor, log]) assert.equal((await stat(file)).mode & 0o777, 0o600);
  const proc = await readFile('/proc/' + pid + '/stat', 'utf8');
  assert.equal(Number(proc.slice(proc.lastIndexOf(') ') + 2).split(' ')[3]), pid, 'broker leads its own OS session after installer exits');
  assert.ok(!(result.stdout + result.stderr + await readFile(log, 'utf8')).includes(token));
  assert.ok(!(result.stdout + result.stderr + await readFile(log, 'utf8')).includes(secret));
  assert.deepEqual(JSON.parse((await f.run(f.launcher, ['list'])).stdout), { sessions: [] });
  // Use the installed command's actual NDJSON stream, then close its SSH-like pipe.
  await new Promise((resolve, reject) => {
    const child = execFile(f.launcher, ['acp'], { env: f.env, timeout: 10000 }, (error, stdout, stderr) => {
      if (error) reject(error);
      else {
        try {
          const messages = stdout.trim().split('\n').map(line => JSON.parse(line));
          assert.equal(messages.length, 2);
          assert.equal(messages[0].result.protocolVersion, 1);
          assert.deepEqual(messages[1].result, { sessions: [] });
          assert.equal(stderr, '');
          resolve();
        } catch (error) { reject(error); }
      }
    });
    let buffer = '';
    child.stdout.on('data', chunk => {
      buffer += chunk;
      let newline;
      while ((newline = buffer.indexOf('\n')) !== -1) {
        try {
          const message = JSON.parse(buffer.slice(0, newline));
          buffer = buffer.slice(newline + 1);
          if (message.id === 1) child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'session/list', params: {} }) + '\n');
          if (message.id === 2) child.stdin.end();
        } catch (error) { child.kill(); reject(error); return; }
      }
    });
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: 1, clientCapabilities: {} } }) + '\n');
  });
  assert.equal(await readFile(f.descriptor, 'utf8'), original);
  await assert.rejects(f.run(f.launcher, ['serve', '--detach']), /already exists.*Nothing was restarted/);
  await assert.rejects(f.run(f.launcher, ['serve']), /existing descriptor may be running or stale/);
  assert.equal(await readFile(f.descriptor, 'utf8'), original);
  assert.deepEqual(JSON.parse((await f.run(f.launcher, ['list'])).stdout), { sessions: [] });
  assert.equal(await exists(join(f.home, 'claude-called')), false, 'installation and broker startup make no native launches');
  await stopBroker(f.descriptor);
  assert.equal(await exists(f.descriptor), false);
  // A crashed broker can leave a descriptor. Neither clients nor startup replace it.
  await writeFile(f.descriptor, original, { mode: 0o600 });
  await assert.rejects(f.run(f.launcher, ['acp']), error => {
    assert.equal(error.stdout, '');
    assert.match(error.stderr, /offline.*stale/);
    assert.ok(!error.stderr.includes(token));
    return true;
  });
  await assert.rejects(f.run('/bin/sh', [join(f.source, 'scripts/workspace-startup.sh')]), /already exists/);
  assert.equal(await readFile(f.descriptor, 'utf8'), original);
  await rm(f.descriptor);
});

test('concurrent explicit starts cannot overwrite the winning broker descriptor', async t => {
  const f = await fixture(t);
  await f.install();
  const results = await Promise.allSettled([
    f.run(f.launcher, ['serve', '--detach']),
    f.run(f.launcher, ['serve', '--detach']),
  ]);
  assert.equal(results.filter(result => result.status === 'fulfilled').length, 1);
  assert.equal(results.filter(result => result.status === 'rejected').length, 1);
  assert.deepEqual(JSON.parse((await f.run(f.launcher, ['list'])).stdout), { sessions: [] });
  assert.equal(await exists(join(f.home, 'claude-called')), false);
});

test('detached startup rejects public logs and state-directory symlinks', async t => {
  const f = await fixture(t);
  await f.install();
  const log = join(f.state, 'broker.log');
  await writeFile(log, 'existing log\n', { mode: 0o644 });
  await chmod(log, 0o644);
  await assert.rejects(f.run(f.launcher, ['serve', '--detach']), /log must be an owner-only regular file/);
  assert.equal(await readFile(log, 'utf8'), 'existing log\n');
  assert.equal(await exists(f.descriptor), false);
  await rm(f.state, { recursive: true });
  const target = join(f.directory, 'other-state');
  await mkdir(target, { mode: 0o700 });
  await symlink(target, f.state);
  await assert.rejects(f.run(f.launcher, ['serve', '--detach']), /owner-only directory/);
});

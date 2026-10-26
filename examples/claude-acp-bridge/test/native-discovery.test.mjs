import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { chmod, mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { createNativeDiscovery, MCP_BLOCKED_REASON, NATIVE_BLOCKED_REASON } from '../src/native-discovery.mjs';

const secret = 'provider-secret-must-not-leave-native-state';

async function fixture(t) {
  const directory = await mkdtemp('/tmp/opencode/native-discovery-');
  t.after(() => rm(directory, { recursive: true, force: true }));
  const jobsDirectory = join(directory, 'jobs');
  await mkdir(jobsDirectory, { mode: 0o755 });
  const stateFile = join(directory, 'bridge.json');
  const pluginPath = join(directory, 'plugin');
  const launchId = randomUUID();
  const sessionId = randomUUID();
  const agent = { id: sessionId.slice(0, 8), sessionId, cwd: '/home/coder/project', kind: 'background', startedAt: 1792761788871, state: 'blocked', name: 'workspace-bridge-' + launchId.slice(0, 8) };
  const env = { WORKSPACE_BRIDGE_FILE: stateFile, WORKSPACE_BRIDGE_LAUNCH_ID: launchId, WORKSPACE_BRIDGE_MODE: 'automation' };
  const state = { sessionId, cwd: agent.cwd, state: 'blocked', detail: '3 new MCP servers need approval', needs: secret, providerEnv: { ANTHROPIC_API_KEY: secret }, respawnFlags: ['--plugin-dir', pluginPath, '--settings', JSON.stringify({ env })] };
  const jobDirectory = join(jobsDirectory, agent.id);
  await mkdir(jobDirectory, { mode: 0o755 });
  const file = join(jobDirectory, 'state.json');
  const save = () => writeFile(file, JSON.stringify(state), { mode: 0o600 });
  await save();
  let output;
  let calls = 0;
  const options = { stateFile, pluginPath, jobsDirectory, run: async (command, args, options) => {
    calls++;
    assert.equal(command, 'claude');
    assert.deepEqual(args, ['agents', '--json', '--all']);
    assert.equal(options.timeout, 5000);
    assert.equal(options.maxBuffer, 1024 * 1024);
    return { stdout: output ?? JSON.stringify([agent]), stderr: secret };
  } };
  return { directory, jobsDirectory, jobDirectory, file, stateFile, pluginPath, launchId, agent, state, env, save, options,
    discover: createNativeDiscovery(options), output(value) { output = value; }, get calls() { return calls; } };
}

describe('structured native discovery', { concurrency: true, timeout: 10000 }, () => {
  test('blocked MCP sessions expose only verified identity and a controlled summary', async t => {
    const f = await fixture(t);
    f.agent.providerEnv = { secret };
    f.agent.status = 'idle'; // Explicit MCP setup still needs approval even when the machine is idle.
    f.agent.name = secret; // A bridge-looking name is neither necessary nor sufficient.
    assert.deepEqual(await f.discover(), [{ sessionId: f.agent.sessionId, cwd: f.agent.cwd,
      startedAt: f.agent.startedAt, launchId: f.launchId, ended: false, blockedReason: MCP_BLOCKED_REASON }]);
    assert.ok(!JSON.stringify(await f.discover()).includes(secret));
    f.state.detail = 'Approve MCP ' + secret;
    delete f.agent.status;
    await f.save();
    assert.equal((await f.discover())[0].blockedReason, NATIVE_BLOCKED_REASON);
  });

  test('only structured machine idle suppresses generic blocked summaries', async t => {
    const f = await fixture(t);
    f.state.detail = f.state.needs = 'What would you like to work on?';
    f.state.status = 'idle'; // Only the CLI machine status is authoritative, not arbitrary state fields.
    await f.save();
    for (const status of ['idle', 'busy', 'waiting', undefined, 'unknown', 'Idle', { status: 'idle' }]) {
      f.agent.status = status;
      const [found] = await f.discover();
      assert.equal(found.ended, false);
      assert.equal(found.blockedReason, status === 'idle' ? undefined : NATIVE_BLOCKED_REASON);
    }
  });

  test('starting and unknown states are pending, only explicit native termination is ended', async t => {
    const f = await fixture(t);
    const ended = ['ended', 'stopped', 'exited', 'completed', 'done', 'failed'];
    for (const state of ['starting', 'running', 'unknown', 'cancelled', 'canceled', ...ended]) {
      f.agent.state = f.state.state = state;
      await f.save();
      const [found] = await f.discover();
      assert.equal(found.ended, ended.includes(state), state);
      // Completed CLI records have no pid/status; stale MCP details must not survive termination.
      assert.equal(found.blockedReason, undefined);
      assert.equal(found.startedAt, f.agent.startedAt);
    }
  });

  test('a blocked record without a CLI PID is not classified as ended from task state alone', async t => {
    const f = await fixture(t);
    Object.assign(f.state, { backend: 'daemon', tempo: 'blocked', cliVersion: '2.1.309' });
    delete f.state.detail;
    await f.save();
    assert.equal(Object.hasOwn(f.agent, 'pid'), false);
    assert.equal(Object.hasOwn(f.agent, 'status'), false);
    const [found] = await f.discover();
    assert.equal(found.ended, false);
    assert.equal(found.blockedReason, NATIVE_BLOCKED_REASON);
  });

  test('CLI termination overrides an unreconciled working or blocked state file', async t => {
    const f = await fixture(t);
    f.agent.state = 'failed';
    for (const state of ['working', 'blocked']) {
      f.state.state = state;
      await f.save();
      const [found] = await f.discover();
      assert.equal(found.ended, true);
      assert.equal(found.blockedReason, undefined);
    }
  });

  for (const [name, change] of [
    ['different descriptor', f => { f.env.WORKSPACE_BRIDGE_FILE += '.other'; }],
    ['relative descriptor', f => { f.env.WORKSPACE_BRIDGE_FILE = 'bridge.json'; }],
    ['launch prefix instead of UUID', f => { f.env.WORKSPACE_BRIDGE_LAUNCH_ID = f.launchId.slice(0, 8); }],
    ['missing launch ID', f => { delete f.env.WORKSPACE_BRIDGE_LAUNCH_ID; }],
    ['different plugin', f => { f.state.respawnFlags[1] += '.other'; }],
    ['duplicate settings', f => { f.state.respawnFlags.push('--settings', '{}'); }],
    ['alternate duplicate settings', f => { f.state.respawnFlags.push('--settings={}'); }],
    ['duplicate plugin', f => { f.state.respawnFlags.push('--plugin-dir', f.pluginPath); }],
    ['different session UUID', f => { f.state.sessionId = randomUUID(); }],
    ['different cwd', f => { f.state.cwd = '/different'; }],
    ['non-background session', f => { f.agent.kind = 'terminal'; }],
    ['unsafe job ID', f => { f.agent.id = '../escape'; }],
    ['job ID is not UUID prefix', f => { f.agent.id = randomUUID().slice(0, 8); }],
    ['relative cwd', f => { f.agent.cwd = f.state.cwd = 'project'; }],
    ['invalid start time', f => { f.agent.startedAt = secret; }],
  ]) test('excludes ' + name + ' despite bridge name and cwd', async t => {
    const f = await fixture(t);
    change(f);
    f.state.respawnFlags[3] = JSON.stringify({ env: f.env });
    await f.save();
    assert.deepEqual(await f.discover(), []);
  });

  test('malformed or oversized state is skipped without exposing parser diagnostics', async t => {
    const f = await fixture(t);
    for (const value of [secret, '{"providerEnv":"' + secret, JSON.stringify({ ...f.state, providerEnv: secret.repeat(20000) }), 'null', '[]']) {
      await writeFile(f.file, value);
      assert.deepEqual(await f.discover(), []);
    }
    f.state.respawnFlags[3] = secret;
    await f.save();
    assert.deepEqual(await f.discover(), []);
    await rm(f.file);
    assert.deepEqual(await f.discover(), []);
  });

  for (const target of ['file', 'jobDirectory', 'jobsDirectory', 'directory']) {
    test('rejects unsafe permissions on ' + target, async t => {
      const f = await fixture(t);
      await chmod(f[target], target === 'file' ? 0o644 : target === 'directory' ? 0o755 : 0o777);
      assert.deepEqual(await f.discover(), []);
    });
    if (target === 'directory') continue; // Covered by the config-root symlink test below.
    test('does not follow ' + target + ' symlinks', async t => {
      const f = await fixture(t);
      const alternate = join(f.directory, 'alternate');
      if (target === 'file') await writeFile(alternate, JSON.stringify(f.state), { mode: 0o600 });
      else await mkdir(alternate, { mode: 0o700 });
      await rm(f[target], { recursive: true });
      await symlink(alternate, f[target]);
      assert.deepEqual(await f.discover(), []);
    });
  }

  test('rejects config-root symlinks and missing native directories before invoking CLI', async t => {
    const f = await fixture(t);
    const link = f.directory + '-link';
    t.after(() => rm(link, { force: true }));
    await symlink(f.directory, link);
    assert.deepEqual(await createNativeDiscovery({ ...f.options, jobsDirectory: join(link, 'jobs') })(), []);
    await rm(f.jobsDirectory, { recursive: true });
    assert.deepEqual(await f.discover(), []);
    assert.equal(f.calls, 0);
  });

  test('bounds and sanitizes CLI failures, invalid JSON and oversized lists', async t => {
    const f = await fixture(t);
    const fails = createNativeDiscovery({ ...f.options, run: async () => { throw new Error(secret); } });
    await assert.rejects(fails(), error => /discovery unavailable/.test(error.message) && !String(error).includes(secret));
    for (const value of [secret, '{}', 'null', JSON.stringify(Array(257).fill(f.agent)), secret.repeat(40000)]) {
      f.output(value);
      await assert.rejects(f.discover(), error => /discovery unavailable/.test(error.message) && !String(error).includes(secret));
    }
    f.output(JSON.stringify([null, {}, f.agent, f.agent]));
    assert.equal((await f.discover()).length, 1);
  });
});

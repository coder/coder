import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { chmod, link, mkdir, mkdtemp, readFile, readlink, rm, symlink, truncate, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { join } from 'node:path';
import { describe, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { createNativeResume } from '../src/native-resume.mjs';

const execute = promisify(execFile);
const secret = 'private-native-prompt-provider-secret';
const cli = fileURLToPath(new URL('../src/cli.mjs', import.meta.url));

async function fixture(t) {
  const root = await mkdtemp('/tmp/opencode/native-resume-');
  t.after(() => rm(root, { recursive: true, force: true }));
  const stateFile = join(root, 'bridge.json');
  const pluginPath = join(root, 'plugin');
  const jobsDirectory = join(root, 'jobs');
  const projectsDirectory = join(root, 'projects');
  const sessionId = randomUUID();
  const launchId = randomUUID();
  const cwd = '/home/coder/project';
  const jobDirectory = join(jobsDirectory, sessionId.slice(0, 8));
  const projectDirectory = join(projectsDirectory, '-home-coder-project');
  for (const dir of [jobsDirectory, projectsDirectory, jobDirectory, projectDirectory]) await mkdir(dir, { mode: 0o755 });
  const transcriptFile = join(projectDirectory, sessionId + '.jsonl');
  await writeFile(transcriptFile, JSON.stringify({ type: 'user', sessionId, message: { content: secret } }) + '\n', { mode: 0o600 });
  const state = { sessionId, resumeSessionId: sessionId, daemonShort: sessionId.slice(0, 8), cwd, state: 'done', template: 'bg', cliVersion: '2.1.309',
    intent: secret, linkScanPath: transcriptFile, inFlight: { tasks: 0, queued: 0, kinds: [] },
    respawnFlags: ['--name', 'workspace-bridge-' + launchId.slice(0, 8), '--plugin-dir', pluginPath, '--settings', JSON.stringify({ env: {
      WORKSPACE_BRIDGE_FILE: stateFile, WORKSPACE_BRIDGE_LAUNCH_ID: launchId, WORKSPACE_BRIDGE_MODE: 'automation',
    } })] };
  const file = join(jobDirectory, 'state.json');
  const save = () => writeFile(file, JSON.stringify(state), { mode: 0o600 });
  await save();
  const agent = { id: sessionId.slice(0, 8), sessionId, cwd, kind: 'background', state: 'done', startedAt: 1792761788871 };
  const calls = [];
  const options = { stateFile, pluginPath, jobsDirectory, projectsDirectory, run: async (command, args, options) => {
    calls.push({ command, args, options });
    return { stdout: args[0] === 'agents' ? JSON.stringify([agent]) : secret, stderr: secret };
  } };
  return { root, file, jobDirectory, projectDirectory, transcriptFile, state, agent, sessionId, launchId, cwd, options, calls, save,
    resume: createNativeResume(options), get launches() { return calls.filter(call => call.args[0] === '--bg'); } };
}

describe('native conversation resume', { concurrency: true, timeout: 10000 }, () => {
  test('uses only full UUID resume, preserves options, and discards native output', async t => {
    const f = await fixture(t);
    const saved = await readFile(f.file, 'utf8');
    assert.equal(await f.resume(f.sessionId, f.cwd), undefined);
    assert.equal(f.launches.length, 1);
    assert.deepEqual(f.launches[0], { command: 'claude', args: ['--bg', '--resume', f.sessionId], options: {
      cwd: f.cwd, timeout: 20000, killSignal: 'SIGKILL', maxBuffer: 1024 * 1024, encoding: 'utf8',
    } });
    assert.equal(await readFile(f.file, 'utf8'), saved);
    assert.ok(!JSON.stringify(f.launches).includes(secret));
  });

  test('active native session refuses resume rather than leaving a registration pending', async t => {
    const f = await fixture(t);
    f.agent.state = f.state.state = 'blocked';
    await f.save();
    await rm(f.transcriptFile);
    await assert.rejects(f.resume(f.sessionId, f.cwd), { message: /Native worker is still running/, resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('a done turn with a live native pid refuses resume without cloning', async t => {
    const f = await fixture(t);
    f.agent.pid = 295;
    f.agent.status = 'idle';
    await rm(f.transcriptFile);
    await assert.rejects(f.resume(f.sessionId, f.cwd), { message: /Native worker is still running/, resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('a stopped native conversation resumes under the same UUID', async t => {
    const f = await fixture(t);
    f.agent.state = f.state.state = 'stopped';
    await f.save();
    await f.resume(f.sessionId, f.cwd);
    assert.deepEqual(f.launches.map(call => call.args), [['--bg', '--resume', f.sessionId]]);
  });

  for (const queue of [secret, '', '/bridge-status', null, 0]) test('refuses a persisted queuedPrompt ' + JSON.stringify(queue), async t => {
    const f = await fixture(t);
    f.state.queuedPrompt = queue;
    await f.save();
    await assert.rejects(f.resume(f.sessionId, f.cwd), error => error.resumeRefused === true && /queued work/.test(error.message) && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });

  test('refuses a nonempty native in-flight queue', async t => {
    const f = await fixture(t);
    f.state.inFlight.queued = 1;
    await f.save();
    await assert.rejects(f.resume(f.sessionId, f.cwd), { message: /queued work/, resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const [name, change] of [
    ['missing state', f => rm(f.file)],
    ['different UUID sharing the prefix', f => { f.state.sessionId = f.sessionId.slice(0, 8) + randomUUID().slice(8); return f.save(); }],
    ['different cwd', f => { f.state.cwd = '/other'; return f.save(); }],
    ['different resume identity', f => { f.state.resumeSessionId = randomUUID(); return f.save(); }],
    ['different daemon handle', f => { f.state.daemonShort = '00000000'; return f.save(); }],
    ['session ID taken', f => { f.state.sessionIdTaken = true; return f.save(); }],
    ['different owner plugin', f => { f.state.respawnFlags[3] += '-other'; return f.save(); }],
    ['duplicate settings', f => { f.state.respawnFlags.push('--settings', '{}'); return f.save(); }],
    ['agent initial prompt', f => { f.state.respawnFlags.push('--agent', 'agent'); return f.save(); }],
    ['reply-on-resume', f => { f.state.respawnFlags.push('--reply-on-resume'); return f.save(); }],
    ['exec template', f => { f.state.template = 'exec'; return f.save(); }],
    ['malformed state', f => writeFile(f.file, secret)],
    ['oversized state', f => writeFile(f.file, secret.repeat(20000))],
  ]) test('refuses ' + name + ' without native launch or diagnostic leakage', async t => {
    const f = await fixture(t);
    await change(f);
    await assert.rejects(f.resume(f.sessionId, f.cwd), error => error.resumeRefused === true && /Cannot safely resume/.test(error.message) && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });

  test('requires a full lowercase UUID, never a prefix or option', async t => {
    const f = await fixture(t);
    for (const id of [f.sessionId.slice(0, 8), f.sessionId.toUpperCase(), '--resume', '../escape']) await assert.rejects(f.resume(id, f.cwd), { message: /Cannot safely resume/, resumeRefused: true });
    assert.equal(f.calls.length, 0);
  });

  for (const [name, change] of [
    ['missing transcript', f => rm(f.transcriptFile)],
    ['empty transcript', f => truncate(f.transcriptFile, 0)],
    ['metadata-only transcript', f => writeFile(f.transcriptFile, JSON.stringify({ type: 'summary', sessionId: f.sessionId }) + '\n')],
    ['other conversation', f => writeFile(f.transcriptFile, JSON.stringify({ type: 'user', sessionId: randomUUID(), message: { content: secret } }) + '\n')],
    ['malformed transcript', f => writeFile(f.transcriptFile, secret)],
    ['outside projects', f => { f.state.linkScanPath = join(f.root, f.sessionId + '.jsonl'); return f.save(); }],
    ['other transcript basename', f => { f.state.linkScanPath = f.transcriptFile.replace(f.sessionId, randomUUID()); return f.save(); }],
  ]) test('refuses ' + name + ' instead of falling back to original intent', async t => {
    const f = await fixture(t);
    await change(f);
    await assert.rejects(f.resume(f.sessionId, f.cwd), error => error.resumeRefused === true && /saved conversation/.test(error.message) && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });

  test('uses computed native project path if linkScanPath was not saved', async t => {
    const f = await fixture(t);
    delete f.state.linkScanPath;
    await f.save();
    await f.resume(f.sessionId, f.cwd);
    assert.equal(f.launches.length, 1);
  });

  test('large transcripts need only a bounded prefix proof, not a full read', async t => {
    const f = await fixture(t);
    await truncate(f.transcriptFile, 512 * 1024 * 1024);
    await f.resume(f.sessionId, f.cwd);
    assert.equal(f.launches.length, 1);
  });

  for (const target of ['file', 'jobDirectory', 'projectDirectory', 'transcriptFile']) {
    test('rejects unsafe permissions on ' + target, async t => {
      const f = await fixture(t);
      await chmod(f[target], target.endsWith('Directory') ? 0o777 : 0o644);
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
    test('rejects ' + target + ' symlink', async t => {
      const f = await fixture(t);
      const alternate = join(f.root, 'alternate');
      if (target.endsWith('Directory')) await mkdir(alternate, { mode: 0o700 });
      else await writeFile(alternate, secret, { mode: 0o600 });
      await rm(f[target], { recursive: true });
      await symlink(alternate, f[target]);
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
  }

  test('refuses a queue changed while ownership discovery was running', async t => {
    const f = await fixture(t);
    const resume = createNativeResume({ ...f.options, run: async (...args) => {
      f.state.queuedPrompt = secret;
      await f.save();
      return f.options.run(...args);
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), { message: /Cannot safely resume/, resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('configuration and discovery failures are definite pre-launch refusals', async t => {
    const f = await fixture(t);
    assert.throws(() => createNativeResume({ ...f.options, jobsDirectory: 'relative' }), { resumeRefused: true });
    const resume = createNativeResume({ ...f.options, run: async () => { throw new Error(secret); } });
    await assert.rejects(resume(f.sessionId, f.cwd), error => error.resumeRefused === true && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });

  for (const code of ['ETIMEDOUT', 'UNKNOWN']) test('sanitizes native ' + code + ' without marking uncertain dispatch as refused', async t => {
    const f = await fixture(t);
    const resume = createNativeResume({ ...f.options, run: async (...args) => {
      const output = await f.options.run(...args);
      if (args[1][0] === '--bg') throw Object.assign(new Error(secret), output, { code });
      return output;
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), error => error.resumeRefused === undefined && /may already be running/.test(error.message) && !JSON.stringify(error).includes(secret) && !String(error).includes(secret));
    assert.equal(f.launches.length, 1);
  });
});

async function stoppedFixture(t) {
  const f = await fixture(t);
  Object.assign(f.state, { state: 'blocked', tempo: 'blocked', backend: 'daemon' });
  f.agent.state = 'blocked';
  await f.save();
  const daemonDirectory = join(f.root, 'daemon');
  const sessionsDirectory = join(f.root, 'sessions');
  await mkdir(daemonDirectory, { mode: 0o700 });
  await mkdir(sessionsDirectory, { mode: 0o700 });
  const lockFile = join(f.root, 'daemon.lock');
  const rosterFile = join(daemonDirectory, 'roster.json');
  const registryFile = join(sessionsDirectory, '10003.json');
  const lock = { pid: 10001, version: '2.1.309', procStart: '123', startedAt: 1792761788871 };
  const worker = { pid: 10002, replPid: 10003, sessionId: f.sessionId, cwd: f.cwd, cliVersion: '2.1.309' };
  const roster = { proto: 1, supervisorPid: lock.pid, workers: { [f.sessionId.slice(0, 8)]: worker } };
  const machineId = await readFile('/etc/machine-id', 'utf8').catch(() => '');
  const registry = { pid: worker.replPid, sessionId: f.sessionId, cwd: f.cwd, kind: 'bg', jobId: f.sessionId.slice(0, 8),
    version: '2.1.309', pidDomain: 'linux:' + machineId.trim() + ':' + await readlink('/proc/self/ns/pid') };
  const saveNative = () => Promise.all([
    writeFile(lockFile, JSON.stringify(lock), { mode: 0o644 }),
    writeFile(rosterFile, JSON.stringify(roster), { mode: 0o600 }),
    writeFile(registryFile, JSON.stringify(registry), { mode: 0o644 }),
  ]);
  await saveNative();
  const probes = [];
  const options = { ...f.options, probeProcess: pid => {
    probes.push(pid);
    throw Object.assign(new Error(secret), { code: 'ESRCH' });
  } };
  return { ...f, daemonDirectory, sessionsDirectory, lockFile, rosterFile, registryFile, lock, roster, worker, registry, saveNative, probes,
    options, resume: createNativeResume(options), get launches() { return f.launches; } };
}

describe('blocked native state after workspace restart', { concurrency: true, timeout: 10000, skip: process.platform !== 'linux' }, () => {
  test('proves the supervisor and registry are stopped before resuming the saved UUID without replay', async t => {
    const f = await stoppedFixture(t);
    const files = [f.file, f.lockFile, f.rosterFile, f.registryFile, f.transcriptFile];
    const saved = await Promise.all(files.map(file => readFile(file, 'utf8')));
    await f.resume(f.sessionId, f.cwd);
    assert.deepEqual(f.launches.map(call => call.args), [['--bg', '--resume', f.sessionId]]);
    for (const pid of [f.lock.pid, f.worker.pid, f.registry.pid]) assert.ok(f.probes.includes(pid));
    assert.deepEqual(await Promise.all(files.map(file => readFile(file, 'utf8'))), saved);
    assert.ok(!JSON.stringify(f.launches).includes(secret));
  });

  test('a live supervisor refuses even when agents omits the registry PID', async t => {
    const f = await stoppedFixture(t);
    f.lock.pid = f.roster.supervisorPid = process.pid;
    await f.saveNative();
    const resume = createNativeResume({ ...f.options, probeProcess: undefined });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const source of ['supervisor', 'worker', 'repl', 'registry']) test('refuses a live ' + source + ' PID even without a CLI PID', async t => {
    const f = await stoppedFixture(t);
    if (source === 'registry') {
      f.worker.replPid = 10004;
      await f.saveNative();
    }
    const livePid = { supervisor: f.lock.pid, worker: f.worker.pid, repl: f.worker.replPid, registry: f.registry.pid }[source];
    const resume = createNativeResume({ ...f.options, probeProcess: pid => {
      if (pid === livePid) return; // A reused PID is deliberately indistinguishable from a native worker.
      return f.options.probeProcess(pid);
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const code of ['EPERM', 'EACCES', 'EIO']) test('does not interpret process probe ' + code + ' as absence', async t => {
    const f = await stoppedFixture(t);
    const resume = createNativeResume({ ...f.options, probeProcess: () => { throw Object.assign(new Error(secret), { code }); } });
    await assert.rejects(resume(f.sessionId, f.cwd), error => error.resumeRefused === true && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });

  for (const value of [undefined, null, 0, -1, 1.5, '10001', 2147483648]) test('rejects invalid supervisor PID ' + String(value) + ' without probing it', async t => {
    const f = await stoppedFixture(t);
    f.lock.pid = f.roster.supervisorPid = value;
    await f.saveNative();
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.deepEqual(f.probes, []);
    assert.equal(f.launches.length, 0);
  });

  for (const [name, change] of [
    ['unknown lock version', f => { f.lock.version = '2.1.310'; }],
    ['mismatched supervisor', f => { f.roster.supervisorPid++; }],
    ['unknown roster protocol', f => { f.roster.proto = 2; }],
    ['missing workers', f => { delete f.roster.workers; }],
    ['array workers', f => { f.roster.workers = []; }],
    ['missing target worker', f => { f.roster.workers = {}; }],
    ['null worker', f => { f.roster.workers[f.sessionId.slice(0, 8)] = null; }],
    ['other worker conversation', f => { f.worker.sessionId = randomUUID(); }],
    ['other worker cwd', f => { f.worker.cwd = '/other'; }],
    ['unknown worker version', f => { f.worker.cliVersion = '2.1.310'; }],
    ['missing worker PID', f => { delete f.worker.pid; }],
    ['zero repl PID', f => { f.worker.replPid = 0; }],
    ['PID filename mismatch', f => { f.registry.pid++; }],
    ['missing registry PID', f => { delete f.registry.pid; }],
    ['other registry conversation', f => { f.registry.sessionId = randomUUID(); }],
    ['other registry job', f => { f.registry.jobId = '00000000'; }],
    ['other registry cwd', f => { f.registry.cwd = '/other'; }],
    ['interactive registry', f => { f.registry.kind = 'interactive'; }],
    ['unknown registry version', f => { f.registry.version = '2.1.310'; }],
    ['missing PID domain', f => { delete f.registry.pidDomain; }],
    ['foreign PID domain', f => { f.registry.pidDomain += '-other'; }],
  ]) test('refuses ambiguous native evidence: ' + name, async t => {
    const f = await stoppedFixture(t);
    change(f);
    await f.saveNative();
    await assert.rejects(f.resume(f.sessionId, f.cwd), error => error.resumeRefused === true && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });

  for (const [name, change] of [
    ['unknown state version', f => { f.state.cliVersion = '2.1.310'; }],
    ['missing state version', f => { delete f.state.cliVersion; }],
    ['non-daemon backend', f => { f.state.backend = 'peer'; }],
    ['active tempo', f => { f.state.tempo = 'active'; }],
    ['starting state', f => { f.state.state = f.agent.state = 'starting'; }],
    ['unknown state', f => { f.state.state = f.agent.state = 'unknown'; }],
    ['CLI status without PID', f => { f.agent.status = 'idle'; }],
    ['invalid CLI PID', f => { f.agent.pid = 0; }],
    ['null CLI PID', f => { f.agent.pid = null; }],
    ['live CLI PID', f => { f.agent.pid = 123; }],
  ]) test('keeps conservative state handling: ' + name, async t => {
    const f = await stoppedFixture(t);
    change(f);
    await f.save();
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.deepEqual(f.probes, []);
    assert.equal(f.launches.length, 0);
  });

  test('duplicate CLI rows are not proof of a stopped worker', async t => {
    const f = await stoppedFixture(t);
    const resume = createNativeResume({ ...f.options, run: async (...args) => ({ ...await f.options.run(...args), stdout: JSON.stringify([f.agent, f.agent]) }) });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('duplicate registry rows for the conversation are ambiguous', async t => {
    const f = await stoppedFixture(t);
    await writeFile(join(f.sessionsDirectory, '10004.json'), JSON.stringify({ ...f.registry, pid: 10004 }), { mode: 0o644 });
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const name of ['010003.json', 'pid.json', '2147483648.json']) test('refuses ambiguous registry filename ' + name, async t => {
    const f = await stoppedFixture(t);
    await writeFile(join(f.sessionsDirectory, name), JSON.stringify(f.registry), { mode: 0o644 });
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const target of ['rosterFile', 'daemonDirectory', 'sessionsDirectory']) test('requires private permissions on ' + target, async t => {
    const f = await stoppedFixture(t);
    await chmod(f[target], target.endsWith('Directory') ? 0o755 : 0o644);
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const target of ['lockFile', 'rosterFile', 'registryFile', 'daemonDirectory', 'sessionsDirectory']) {
    test('missing ' + target + ' is not proof of absence', async t => {
      const f = await stoppedFixture(t);
      await rm(f[target], { recursive: true });
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
    test('rejects a symlink at ' + target, async t => {
      const f = await stoppedFixture(t);
      const alternate = join(f.root, 'alternate');
      if (target.endsWith('Directory')) await mkdir(alternate, { mode: 0o700 });
      else await writeFile(alternate, await readFile(f[target]), { mode: 0o600 });
      await rm(f[target], { recursive: true });
      await symlink(alternate, f[target]);
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
    test('rejects writable permissions on ' + target, async t => {
      const f = await stoppedFixture(t);
      await chmod(f[target], target.endsWith('Directory') ? 0o777 : 0o666);
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
  }

  for (const target of ['lockFile', 'rosterFile', 'registryFile']) {
    for (const value of [secret, 'null', '[]', '{}', '10001']) test('refuses malformed ' + target + ': ' + value, async t => {
      const f = await stoppedFixture(t);
      await writeFile(f[target], value);
      await assert.rejects(f.resume(f.sessionId, f.cwd), error => error.resumeRefused === true && !String(error).includes(secret));
      assert.equal(f.launches.length, 0);
    });
    test('bounds reads of ' + target, async t => {
      const f = await stoppedFixture(t);
      await truncate(f[target], 256 * 1024 + 1);
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
    test('refuses hard-linked ' + target, async t => {
      const f = await stoppedFixture(t);
      await link(f[target], join(f.root, 'alias'));
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
    test('refuses FIFO ' + target + ' without hanging', async t => {
      const f = await stoppedFixture(t);
      await rm(f[target]);
      await execute('mkfifo', [f[target]]);
      await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
      assert.equal(f.launches.length, 0);
    });
  }

  test('bounds directory enumeration, including non-JSON entries', async t => {
    const f = await stoppedFixture(t);
    await Promise.all(Array.from({ length: 256 }, (_, i) => writeFile(join(f.sessionsDirectory, i + '.key'), '', { mode: 0o600 })));
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('bounds aggregate evidence size', async t => {
    const f = await stoppedFixture(t);
    for (let i = 0; i < 5; i++) {
      const pid = 20000 + i;
      await writeFile(join(f.sessionsDirectory, pid + '.json'), JSON.stringify({ ...f.registry, pid, sessionId: randomUUID(), padding: 'x'.repeat(230 * 1024) }), { mode: 0o600 });
    }
    await assert.rejects(f.resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  for (const target of ['lockFile', 'rosterFile', 'registryFile']) test('refuses a changed ' + target + ' before launch', async t => {
    const f = await stoppedFixture(t);
    let changed = false;
    const resume = createNativeResume({ ...f.options, probeProcess: async pid => {
      if (!changed) {
        changed = true;
        await writeFile(f[target], await readFile(f[target], 'utf8') + '\n');
      }
      return f.options.probeProcess(pid);
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('refuses a supervisor lock changed during the final process check', async t => {
    const f = await stoppedFixture(t);
    let checks = 0;
    const resume = createNativeResume({ ...f.options, probeProcess: async pid => {
      if (pid === f.lock.pid && ++checks === 2) {
        await writeFile(f.lockFile, JSON.stringify({ ...f.lock, pid: process.pid }));
      }
      return f.options.probeProcess(pid);
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(checks, 2);
    assert.equal(f.launches.length, 0);
  });

  test('refuses a registry entry added during validation', async t => {
    const f = await stoppedFixture(t);
    let changed = false;
    const resume = createNativeResume({ ...f.options, probeProcess: async pid => {
      if (!changed) {
        changed = true;
        await writeFile(join(f.sessionsDirectory, '20000.json'), JSON.stringify({ ...f.registry, pid: 20000 }), { mode: 0o644 });
      }
      return f.options.probeProcess(pid);
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(f.launches.length, 0);
  });

  test('rechecks process absence immediately before launch', async t => {
    const f = await stoppedFixture(t);
    let checks = 0;
    const resume = createNativeResume({ ...f.options, probeProcess: pid => {
      if (pid === f.lock.pid && ++checks === 2) return;
      return f.options.probeProcess(pid);
    } });
    await assert.rejects(resume(f.sessionId, f.cwd), { resumeRefused: true });
    assert.equal(checks, 2);
    assert.equal(f.launches.length, 0);
  });

  for (const [name, change, message] of [
    ['queued prompt', f => { f.state.queuedPrompt = secret; return f.save(); }, /queued work/],
    ['empty queued prompt', f => { f.state.queuedPrompt = ''; return f.save(); }, /queued work/],
    ['in-flight queue', f => { f.state.inFlight.queued = 1; return f.save(); }, /queued work/],
    ['missing transcript', f => rm(f.transcriptFile), /saved conversation/],
    ['metadata-only transcript', f => writeFile(f.transcriptFile, JSON.stringify({ type: 'summary', sessionId: f.sessionId })), /saved conversation/],
    ['reply-on-resume', f => { f.state.respawnFlags.push('--reply-on-resume'); return f.save(); }, /Cannot safely resume/],
    ['routine', f => { f.state.routine = 'routine'; return f.save(); }, /Cannot safely resume/],
    ['other plugin', f => { f.state.respawnFlags[3] += '-other'; return f.save(); }, /Cannot safely resume/],
  ]) test('preserves no-replay checks for stopped blocked sessions: ' + name, async t => {
    const f = await stoppedFixture(t);
    await change(f);
    await assert.rejects(f.resume(f.sessionId, f.cwd), error => error.resumeRefused === true && message.test(error.message) && !String(error).includes(secret));
    assert.equal(f.launches.length, 0);
  });
});

async function attachFixture(t, session, modeError) {
  const root = await mkdtemp('/tmp/opencode/native-resume-cli-');
  const stateFile = join(root, 'bridge.json');
  const bin = join(root, 'bin');
  const argsFile = join(root, 'args.json');
  await mkdir(bin, { mode: 0o700 });
  for (const name of ['claude', 'codex', 'opencode']) {
    await writeFile(join(bin, name), `#!${process.execPath}\nrequire('node:fs').writeFileSync(${JSON.stringify(argsFile)}, JSON.stringify(process.argv.slice(2)));\n`, { mode: 0o700 });
  }
  const requests = [];
  const server = createServer(async (request, response) => {
    let body = '';
    for await (const chunk of request) body += chunk;
    requests.push({ method: request.method, path: request.url, body: body ? JSON.parse(body) : undefined });
    response.setHeader('Content-Type', 'application/json');
    if (modeError && request.method === 'POST' && request.url.endsWith('/mode')) {
      response.statusCode = 409;
      response.end(JSON.stringify({ error: modeError }));
    } else response.end(JSON.stringify(session));
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  await writeFile(stateFile, JSON.stringify({ url: 'http://127.0.0.1:' + server.address().port, token: 'a'.repeat(64) }), { mode: 0o600 });
  t.after(async () => {
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
    await rm(root, { recursive: true, force: true });
  });
  return { requests, args: async () => JSON.parse(await readFile(argsFile, 'utf8')), run: id => execute(process.execPath,
    [cli, 'attach', id, '--state', stateFile], { env: { ...process.env, PATH: bin + ':' + process.env.PATH }, timeout: 5000 }) };
}

describe('CLI terminal opening', { concurrency: true, timeout: 10000 }, () => {
  for (const harness of ['claude', 'codex']) {
    const session = () => {
      const nativeSessionId = randomUUID();
      return { harness, nativeSessionId, sessionId: harness === 'codex' ? 'codex:' + nativeSessionId : nativeSessionId,
        cwd: process.cwd(), archived: false, resumePending: false };
    };
    test(harness + ' attaches to an active session without resuming it, including repeated opens', async t => {
      const active = session();
      const f = await attachFixture(t, active);
      for (let i = 0; i < 2; i++) assert.equal((await f.run(active.sessionId)).stdout, '');
      const path = '/v1/sessions/' + encodeURIComponent(active.sessionId);
      assert.deepEqual(f.requests, Array.from({ length: 2 }, () => [
        { method: 'GET', path, body: undefined },
        { method: 'POST', path: path + '/mode', body: { mode: 'human' } },
      ]).flat());
      const args = await f.args();
      if (harness === 'claude') assert.deepEqual(args, ['attach', active.sessionId.slice(0, 8)]);
      else {
        assert.deepEqual(args.slice(0, 2), ['resume', '--remote']);
        assert.ok(args[2].startsWith('unix:///'));
        assert.equal(args[3], active.nativeSessionId);
      }
    });

    for (const status of [{ archived: true }, { resumePending: true }, { archived: true, resumePending: true }]) {
      test(harness + ' never starts an archived or restoring session from a terminal open: ' + JSON.stringify(status), async t => {
        const stopped = { ...session(), ...status };
        const f = await attachFixture(t, stopped);
        await assert.rejects(f.run(stopped.sessionId), error => /Workspace startup restores saved sessions/.test(error.stderr));
        assert.ok(f.requests.every(request => request.method === 'GET'));
        await assert.rejects(f.args(), { code: 'ENOENT' });
      });
    }

    test(harness + ' does not attach when live control cannot be confirmed', async t => {
      const active = session();
      const f = await attachFixture(t, active, 'Native runtime is unavailable');
      await assert.rejects(f.run(active.sessionId), error => /Native runtime is unavailable/.test(error.stderr));
      assert.equal(f.requests.length, 2);
      assert.ok(!f.requests.some(request => request.path.endsWith('/resume')));
      await assert.rejects(f.args(), { code: 'ENOENT' });
    });
  }

  test('OpenCode terminal opening never restores archived or pending native sessions', async t => {
    for (const status of [{ archived: true }, { resumePending: true }]) {
      const nativeSessionId = 'ses_' + randomUUID().replaceAll('-', '');
      const session = { harness: 'opencode', nativeSessionId, sessionId: 'opencode:' + nativeSessionId, ...status };
      const f = await attachFixture(t, session);
      await assert.rejects(f.run(session.sessionId), error => /Workspace startup restores saved sessions/.test(error.stderr));
      assert.ok(f.requests.every(request => request.method === 'GET'));
      await assert.rejects(f.args(), { code: 'ENOENT' });
    }
  });

  test('never attaches using an arbitrary prefix or mismatched broker identity', async t => {
    const sessionId = randomUUID();
    const f = await attachFixture(t, { sessionId });
    for (const id of [sessionId.slice(0, 8), randomUUID()]) await assert.rejects(f.run(id), error => /exact full session ID/.test(error.stderr));
    assert.ok(f.requests.every(request => request.method === 'GET'));
    await assert.rejects(f.args(), { code: 'ENOENT' });
  });
});

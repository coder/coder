import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import { chmod, mkdir, mkdtemp, readFile, rm, symlink, truncate, writeFile } from 'node:fs/promises';
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

async function attachFixture(t, session, resumeError) {
  const root = await mkdtemp('/tmp/opencode/native-resume-cli-');
  const stateFile = join(root, 'bridge.json');
  const bin = join(root, 'bin');
  const argsFile = join(root, 'args.json');
  await mkdir(bin, { mode: 0o700 });
  await writeFile(join(bin, 'claude'), `#!${process.execPath}\nrequire('node:fs').writeFileSync(${JSON.stringify(argsFile)}, JSON.stringify(process.argv.slice(2)));\n`, { mode: 0o700 });
  const requests = [];
  const server = createServer(async (request, response) => {
    let body = '';
    for await (const chunk of request) body += chunk;
    requests.push({ method: request.method, path: request.url, body: body ? JSON.parse(body) : undefined });
    response.setHeader('Content-Type', 'application/json');
    if (resumeError && request.method === 'POST' && request.url.endsWith('/resume')) {
      response.statusCode = 409;
      response.end(JSON.stringify({ error: resumeError }));
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

describe('CLI Join routing', { concurrency: true, timeout: 10000 }, () => {
  for (const [name, archived, resumePending, action] of [
    ['archived', true, false, 'resume'],
    ['active', false, false, 'mode'],
    ['resume already pending', true, true, 'mode'],
  ]) test(name + ' selects the correct broker action before native attach', async t => {
    const sessionId = randomUUID();
    const f = await attachFixture(t, { sessionId, archived, resumePending });
    const result = await f.run(sessionId);
    assert.equal(result.stdout, '');
    assert.deepEqual(f.requests, [
      { method: 'GET', path: '/v1/sessions/' + sessionId, body: undefined },
      { method: 'POST', path: '/v1/sessions/' + sessionId + '/' + action, body: action === 'resume' ? {} : { mode: 'human' } },
    ]);
    assert.deepEqual(await f.args(), ['attach', sessionId.slice(0, 8)]);
  });

  for (const error of ['Native session has queued work', 'Native saved conversation is missing']) test('does not fall back to native attach after refusal: ' + error, async t => {
    const sessionId = randomUUID();
    const f = await attachFixture(t, { sessionId, archived: true }, error);
    await assert.rejects(f.run(sessionId), result => result.stderr.includes(error));
    assert.equal(f.requests.length, 2);
    await assert.rejects(f.args(), { code: 'ENOENT' });
  });

  test('never attaches using an arbitrary prefix or mismatched broker identity', async t => {
    const sessionId = randomUUID();
    const f = await attachFixture(t, { sessionId });
    for (const id of [sessionId.slice(0, 8), randomUUID()]) await assert.rejects(f.run(id), error => /exact full session ID/.test(error.stderr));
    assert.ok(f.requests.every(request => request.method === 'GET'));
    await assert.rejects(f.args(), { code: 'ENOENT' });
  });
});

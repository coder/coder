import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { packageTemplate } from '../scripts/package-template.mjs';
import { startWorkspaceJob } from '../template/image/startup.mjs';
import { createBroker } from '../src/broker.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const secret = 'test-only-provider-token';

async function fixture(t, { discover } = {}) {
  const directory = await mkdtemp('/tmp/opencode/native-job-template-test-');
  t.after(() => rm(directory, { recursive: true, force: true }));
  const packaged = join(directory, 'template');
  await packageTemplate(packaged);
  const home = join(directory, "owner's home");
  await mkdir(home);
  const authFile = join(directory, 'auth.json');
  await writeFile(authFile, JSON.stringify({ ANTHROPIC_AUTH_TOKEN: secret, ANTHROPIC_BASE_URL: 'https://provider.example.com' }), { mode: 0o600 });
  const prompt = "Read the project. $(touch /not-a-command)\n'quoted'; `literal` 🚀 $HOME ${CODER_AGENT_TOKEN} $UNSET_VARIABLE";
  const env = { HOME: home, CODER_NATIVE_JOB_PROMPT_BASE64: Buffer.from(prompt).toString('base64'), CODER_NATIVE_JOB_TRUST: 'true' };
  const calls = [];
  const brokers = new Set();
  t.after(async () => { for (const broker of brokers) await broker.close(); });
  const run = async (command, args, options) => {
    calls.push({ command, args, options });
    assert.equal(options.env.CODER_NATIVE_JOB_PROMPT_BASE64, undefined);
    assert.ok(!args.some(value => value.includes(secret) || value.includes(prompt)));
    if (args[0] === 'serve') {
      const broker = await createBroker({ discover, launch: () => assert.fail('bootstrap must not launch native sessions') });
      brokers.add(broker);
      await writeFile(join(home, '.local/state/coder-acp/bridge.json'), JSON.stringify({ url: broker.url, token: broker.token, pid: process.pid }), { mode: 0o600, flag: 'wx' });
    }
    if (args[0] === 'run') {
      assert.equal(await readFile(args[args.indexOf('--prompt-file') + 1], 'utf8'), prompt);
      return { stdout: 'job dispatched\n' };
    }
    if (command === 'git') await mkdir(join(home, 'project'));
    return { stdout: '' };
  };
  return { directory, packaged, home, authFile, prompt, env, calls, brokers, run, options: { root: join(packaged, 'image'), home, authFile, env, run } };
}

test('template bundle is self-contained and excludes runtime state, dependencies and hidden directories', async t => {
  const f = await fixture(t);
  const paths = await readdir(f.packaged, { recursive: true });
  assert.ok(paths.includes('image/adapter/src/acp.mjs'));
  assert.ok(paths.includes('image/adapter/plugin/hooks/register.js'));
  assert.ok(paths.includes('image/adapter/plugin-manifest.json'));
  assert.ok(!paths.some(path => /(^|\/)(node_modules|\.runtime|\.claude-plugin|test|tests)(\/|$)/.test(path)));
  let bytes = 0;
  for (const path of paths) {
    const info = await stat(join(f.packaged, path));
    if (info.isFile()) bytes += info.size;
  }
  assert.ok(bytes < 800 * 1024, 'leave room for tar headers within the Coder archive limit');
  await assert.rejects(packageTemplate(f.packaged), { code: 'EEXIST' });
  const template = await readFile(join(f.packaged, 'main.tf'), 'utf8');
  assert.match(template, /form_type\s*= "textarea"/);
  assert.match(template, /dir\s*= "\/home\/coder\/project"/);
  assert.match(template, /CODER_NATIVE_JOB_PROMPT_BASE64\s*= base64encode\(data\.coder_parameter\.prompt\.value\)/);
  assert.match(template, /CODER_NATIVE_JOB_REPO_BASE64\s*= base64encode\(data\.coder_parameter\.repository\.value\)/);
  assert.match(template, /read_only\s*= true/);
  assert.ok(!template.includes(secret));
});

test('repository parameter validation accepts public HTTPS URLs in the browser', async () => {
  const template = await readFile(join(root, 'template/main.tf'), 'utf8');
  const repository = template.split('data "coder_parameter" "repository" {')[1].split('\ndata "coder_parameter"')[0];
  const pattern = repository.match(/regex\s*=\s*("(?:[^"\\]|\\.)*")/)[1];
  const regex = new RegExp(JSON.parse(pattern));
  for (const value of ['', 'https://github.com/coder/coder', 'https://github.com/coder/agentapi.git']) {
    assert.equal(regex.test(value), true, `accept ${JSON.stringify(value)}`);
  }
  for (const value of ['http://github.com/coder/coder', 'git@github.com:coder/coder.git', 'https://', 'https://github.com/coder/my repo', 'https://github.com/\tcoder', 'https://github.com/\ncoder']) {
    assert.equal(regex.test(value), false, `reject ${JSON.stringify(value)}`);
  }
});

test('startup passes literal prompt through a private file, detaches, and configures provider without logging secrets', async t => {
  const f = await fixture(t);
  assert.equal(await startWorkspaceJob(f.options), 'job dispatched\n');
  assert.deepEqual(f.calls.map(call => call.args[0]), [join(f.packaged, 'image/adapter/install.sh'), 'serve', 'run']);
  assert.deepEqual(f.calls[1].args, ['serve', '--detach', '--model', 'haiku']);
  assert.ok(f.calls[2].args.includes('--detach'));
  assert.equal((await stat(join(f.home, '.claude/settings.json'))).mode & 0o777, 0o600);
  const settings = JSON.parse(await readFile(join(f.home, '.claude/settings.json'), 'utf8'));
  assert.deepEqual(settings.env, { ANTHROPIC_AUTH_TOKEN: secret, ANTHROPIC_BASE_URL: 'https://provider.example.com' });
  const config = JSON.parse(await readFile(join(f.home, '.claude.json'), 'utf8'));
  assert.equal(config.projects[join(f.home, 'project')].hasTrustDialogAccepted, true);
  assert.ok(!JSON.stringify(f.calls).includes(secret));
  assert.deepEqual((await readdir(join(f.home, '.local/state/coder-acp'))).sort(), ['bridge.json', 'broker-runtime.json']);
});

test('blocked setup instructs Join with the known session and never claims the prompt was submitted', async t => {
  const f = await fixture(t);
  const sessionId = 'ad72142c-b0ed-474c-9e9e-237345db5784';
  const job = { version: 1, status: 'blocked', sessionId, errorCode: 'native_input_required',
    inputHash: createHash('sha256').update(JSON.stringify([join(f.home, 'project'), f.prompt])).digest('hex') };
  const jobFile = join(f.home, '.local/state/coder-acp/job.json');
  const options = { ...f.options, run: async (command, args, settings) => {
    const result = await f.run(command, args, settings);
    if (args[0] === 'run') {
      await writeFile(jobFile, JSON.stringify(job), { mode: 0o600 });
      throw new Error('Workspace setup command failed.');
    }
    return result;
  } };
  const output = await startWorkspaceJob(options);
  assert.match(output, /Join/);
  assert.match(output, /initial task was not submitted/i);
  assert.ok(output.includes(sessionId));
  assert.ok(!output.includes(secret) && !output.includes(f.prompt));
  assert.deepEqual((await readdir(join(f.home, '.local/state/coder-acp'))).sort(), ['bridge.json', 'broker-runtime.json', 'job.json']);
  const original = { ...job };
  for (const change of [{ inputHash: 'bad' }, { status: 'failed' }, { dispatchAttemptedAt: new Date().toISOString() }]) {
    Object.assign(job, original, change);
    await assert.rejects(startWorkspaceJob(options), /Workspace setup command failed/);
  }
});

test('job metadata explains blocked setup without exposing remote fields', async t => {
  const f = await fixture(t);
  const state = join(f.home, '.local/state/coder-acp');
  await mkdir(state, { recursive: true, mode: 0o700 });
  const sessionId = 'ad72142c-b0ed-474c-9e9e-237345db5784';
  await writeFile(join(state, 'job.json'), JSON.stringify({ status: 'blocked', sessionId, message: secret }), { mode: 0o600 });
  const result = await promisify(execFile)(process.execPath, [join(root, 'template/image/job-status.mjs')], { env: { ...process.env, HOME: f.home } });
  assert.equal(result.stdout, `Initial task not submitted; inspect native sessions (${sessionId})\n`);
  assert.equal(result.stderr, '');
});

test('startup rejects malformed encoded parameters without echoing their contents', async t => {
  const f = await fixture(t);
  for (const value of [secret, '/w==', 'Zg', 'a'.repeat(176001)]) {
    f.env.CODER_NATIVE_JOB_PROMPT_BASE64 = value;
    await assert.rejects(startWorkspaceJob(f.options), error => {
      assert.equal(error.message, 'Invalid encoded job parameter.');
      return true;
    });
  }
  assert.equal(f.calls.length, 0);
});

test('startup provisions API-key helpers and private OAuth files without permission bypasses', async t => {
  for (const type of ['api-key', 'oauth']) {
    const f = await fixture(t);
    const auth = type === 'api-key' ? { ANTHROPIC_API_KEY: secret } : { claudeAiOauth: { accessToken: secret } };
    await writeFile(f.authFile, JSON.stringify(auth));
    await startWorkspaceJob(f.options);
    const file = join(f.home, '.claude', type === 'api-key' ? 'settings.json' : '.credentials.json');
    const contents = JSON.parse(await readFile(file, 'utf8'));
    assert.deepEqual(contents, type === 'api-key' ? { apiKeyHelper: 'node /opt/native-agent/read-auth.mjs' } : auth);
    assert.equal((await stat(file)).mode & 0o777, 0o600);
    assert.ok(!JSON.stringify(f.calls).includes(secret));
    assert.ok(!JSON.stringify(f.calls).includes('skip-permissions'));
  }
});

test('restart delegates to durable job guard without another broker, clone, trust or auth setup', async t => {
  const f = await fixture(t);
  const state = join(f.home, '.local/state/coder-acp');
  await startWorkspaceJob(f.options);
  await writeFile(join(state, 'job.json'), '{}', { mode: 0o600 });
  const descriptor = await readFile(join(state, 'bridge.json'), 'utf8');
  f.calls.length = 0;
  await rm(f.authFile);
  f.env.CODER_NATIVE_JOB_TRUST = 'false';
  await startWorkspaceJob(f.options);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['run']);
  assert.equal(await readFile(join(state, 'bridge.json'), 'utf8'), descriptor);
});

async function retainedBroker(t, brokerOptions) {
  const f = await fixture(t, brokerOptions);
  const instance = 'a'.repeat(64);
  f.options.identifyInstance = async () => instance;
  await startWorkspaceJob(f.options);
  f.state = join(f.home, '.local/state/coder-acp');
  f.jobFile = join(f.state, 'job.json');
  const timestamp = new Date().toISOString();
  f.job = JSON.stringify({ version: 1, jobId: 'bd72142c-b0ed-474c-9e9e-237345db5784',
    sessionId: 'ad72142c-b0ed-474c-9e9e-237345db5784', status: 'blocked', errorCode: 'native_input_required',
    createdAt: timestamp, updatedAt: timestamp,
    inputHash: createHash('sha256').update(JSON.stringify([join(f.home, 'project'), f.prompt])).digest('hex') });
  await writeFile(f.jobFile, f.job, { mode: 0o600 });
  f.calls.length = 0;
  f.stopBroker = async () => {
    for (const broker of f.brokers) { await broker.close(); f.brokers.delete(broker); }
  };
  // Default above Linux PID_MAX_LIMIT; tests may also model namespace PID reuse.
  f.deadDescriptor = async (pid = 2147483647) => {
    await f.stopBroker();
    const path = join(f.state, 'bridge.json');
    const descriptor = JSON.parse(await readFile(path, 'utf8'));
    descriptor.pid = pid;
    await writeFile(path, JSON.stringify(descriptor));
    return readFile(path, 'utf8');
  };
  return f;
}

test('new container recovers its owned broker despite PID reuse and actual CLI guard prevents native launch or prompt replay', async t => {
  const f = await retainedBroker(t);
  const old = await f.deadDescriptor(process.pid);
  const signal = t.mock.method(process, 'kill');
  f.options.identifyInstance = async () => 'b'.repeat(64);
  f.options.run = async (command, args, settings) => {
    const result = await f.run(command, args, settings);
    if (args[0] !== 'run') return result;
    return promisify(execFile)(process.execPath, [join(root, 'src/cli.mjs'), ...args, '--state', join(f.state, 'bridge.json')], settings);
  };
  const output = await startWorkspaceJob(f.options);
  assert.match(output, /old native session was not verified live/);
  assert.match(output, /Nothing was resumed or replayed/);
  assert.doesNotMatch(output, /Select Join/);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['serve', 'run']);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
  assert.notEqual(await readFile(join(f.state, 'bridge.json'), 'utf8'), old);
  assert.equal((await stat(join(f.state, 'broker-runtime.json'))).mode & 0o777, 0o600);
  assert.equal(signal.mock.callCount(), 0, 'never signal the unrelated process reusing the old PID');
  assert.ok((await stat('/proc/' + JSON.parse(old).pid)).isDirectory(), 'replacement process remains alive');
  f.calls.length = 0;
  assert.match(await startWorkspaceJob(f.options), /old native session was not verified live/);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['run']);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
});

test('legacy unknown jobs do not prevent broker recovery or replay work', async t => {
  const f = await retainedBroker(t);
  await f.deadDescriptor();
  await rm(join(f.state, 'broker-runtime.json'));
  const job = JSON.parse(f.job);
  job.status = 'unknown';
  job.errorCode = 'result_unknown';
  delete job.sessionId;
  const retained = JSON.stringify(job);
  await writeFile(f.jobFile, retained);
  f.options.run = async (command, args, settings) => {
    const result = await f.run(command, args, settings);
    if (args[0] !== 'run') return result;
    return promisify(execFile)(process.execPath, [join(root, 'src/cli.mjs'), ...args, '--state', join(f.state, 'bridge.json')], settings);
  };
  assert.match(await startWorkspaceJob(f.options), /broker is available.*unknown outcome/);
  assert.equal(await readFile(f.jobFile, 'utf8'), retained);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['serve', 'run']);
});

test('legacy dead descriptors migrate automatically without replaying the retained job', async t => {
  const f = await retainedBroker(t);
  await f.deadDescriptor();
  await rm(join(f.state, 'broker-runtime.json'));
  await startWorkspaceJob(f.options);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['serve', 'run']);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
  const descriptor = JSON.parse(await readFile(join(f.state, 'bridge.json'), 'utf8'));
  const marker = JSON.parse(await readFile(join(f.state, 'broker-runtime.json'), 'utf8'));
  assert.equal(marker.token, descriptor.token);
  f.calls.length = 0;
  await startWorkspaceJob(f.options);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['run']);
});

test('same-instance failure, ambiguous legacy PID, and rejected authentication or ownership never recover', async t => {
  for (const scenario of ['same-instance', 'same-instance-running-pid', 'legacy', 'authentication', 'unowned']) {
    const f = await retainedBroker(t);
    if (scenario !== 'authentication') await f.deadDescriptor();
    if (!scenario.startsWith('same-instance')) f.options.identifyInstance = async () => 'b'.repeat(64);
    if (scenario === 'legacy') await rm(join(f.state, 'broker-runtime.json'));
    const descriptorFile = join(f.state, 'bridge.json');
    const descriptor = JSON.parse(await readFile(descriptorFile, 'utf8'));
    if (scenario === 'same-instance-running-pid' || scenario === 'legacy') descriptor.pid = process.pid;
    if (scenario === 'authentication' || scenario === 'unowned') descriptor.token = 'c'.repeat(64);
    await writeFile(descriptorFile, JSON.stringify(descriptor));
    const original = await readFile(descriptorFile, 'utf8');
    await assert.rejects(startWorkspaceJob(f.options), /Broker ownership is uncertain/);
    assert.equal(await readFile(descriptorFile, 'utf8'), original);
    assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
    assert.deepEqual(f.calls, []);
  }
});

test('runtime ownership files remain private and symlinks are never followed', async t => {
  for (const name of ['bridge.json', 'broker-runtime.json']) {
    const f = await retainedBroker(t);
    await f.deadDescriptor();
    f.options.identifyInstance = async () => 'b'.repeat(64);
    const path = join(f.state, name);
    const original = await readFile(path, 'utf8');
    const target = join(f.directory, 'outside.json');
    await writeFile(target, original, { mode: 0o600 });
    await rm(path);
    await symlink(target, path);
    await assert.rejects(startWorkspaceJob(f.options), { code: 'ELOOP' });
    assert.equal(await readFile(target, 'utf8'), original);
    assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
    assert.deepEqual(f.calls, []);
  }
});

test('live broker reuse wins even with an older runtime marker and never replaces descriptor', async t => {
  const f = await retainedBroker(t);
  f.options.identifyInstance = async () => 'b'.repeat(64);
  const descriptor = await readFile(join(f.state, 'bridge.json'), 'utf8');
  await startWorkspaceJob(f.options);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['run']);
  assert.equal(await readFile(join(f.state, 'bridge.json'), 'utf8'), descriptor);
});

test('live broker probe allows native discovery to take longer than three seconds without replacing the broker', async t => {
  let discoveries = 0;
  const f = await retainedBroker(t, { discover: async () => {
    discoveries++;
    // Model a slow native discovery response within its supported five-second
    // deadline, rather than delaying startup to hide an ordering race.
    await new Promise(resolve => setTimeout(resolve, 3200));
    return [];
  } });
  const descriptor = await readFile(join(f.state, 'bridge.json'), 'utf8');
  await startWorkspaceJob(f.options);
  assert.equal(discoveries, 1);
  assert.deepEqual(f.calls.map(call => call.args[0]), ['run']);
  assert.equal(await readFile(join(f.state, 'bridge.json'), 'utf8'), descriptor);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
});

test('concurrent startup cannot launch duplicate brokers or dispatch through the held bootstrap lock', async t => {
  const f = await retainedBroker(t);
  await f.deadDescriptor();
  f.options.identifyInstance = async () => 'b'.repeat(64);
  let entered;
  const launching = new Promise(resolve => { entered = resolve; });
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  f.options.run = async (...args) => {
    if (args[1][0] === 'serve') { entered(); await gate; }
    return f.run(...args);
  };
  const first = startWorkspaceJob(f.options);
  await launching;
  try { await assert.rejects(startWorkspaceJob(f.options), /already attempted or in progress/); }
  finally { release(); }
  await first;
  assert.deepEqual(f.calls.map(call => call.args[0]), ['serve', 'run']);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
});

test('failed broker launch is not retried in the same namespace even without a descriptor', async t => {
  const f = await retainedBroker(t);
  await f.deadDescriptor();
  f.options.identifyInstance = async () => 'b'.repeat(64);
  f.options.run = async () => { throw new Error('launch failed'); };
  await assert.rejects(startWorkspaceJob(f.options), /launch failed/);
  await assert.rejects(startWorkspaceJob(f.options), /Broker ownership is uncertain/);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
});

test('startup refuses untrusted projects, embedded repository credentials and stale broker state', async t => {
  const f = await fixture(t);
  f.env.CODER_NATIVE_JOB_TRUST = 'false';
  await assert.rejects(startWorkspaceJob(f.options), /Trust this project/);
  assert.equal(f.calls.length, 0);
  f.env.CODER_NATIVE_JOB_TRUST = 'true';
  f.env.CODER_NATIVE_JOB_REPO_BASE64 = Buffer.from('https://user:password@example.com/repo').toString('base64');
  await assert.rejects(startWorkspaceJob(f.options), /without embedded credentials/);
  f.env.CODER_NATIVE_JOB_REPO_BASE64 = '';
  await writeFile(join(f.home, '.local/state/coder-acp/bridge.json'), '{}');
  await assert.rejects(startWorkspaceJob(f.options), /Broker state already exists/);
  assert.equal(f.calls.length, 0);
});

test('startup treats repository as an argument and rejects unsafe secret files without disclosing their contents', async t => {
  const f = await fixture(t);
  const repo = 'https://example.com/repo?literal=$(not-a-command)&home=$HOME&token=${CODER_AGENT_TOKEN}&missing=$UNSET_VARIABLE';
  f.env.CODER_NATIVE_JOB_REPO_BASE64 = Buffer.from(repo).toString('base64');
  // Coder expands agent environment values before running startup scripts.
  // Neither encoded parameter can reference the bootstrap environment.
  const bootstrap = { HOME: '/different/home', CODER_AGENT_TOKEN: secret };
  for (const name of ['CODER_NATIVE_JOB_PROMPT_BASE64', 'CODER_NATIVE_JOB_REPO_BASE64']) {
    f.env[name] = f.env[name].replace(/\$\{(\w+)\}|\$(\w+)/g, (_, braced, plain) => bootstrap[braced || plain] || '');
  }
  await startWorkspaceJob(f.options);
  assert.deepEqual(f.calls[0].args, ['-c', 'core.hooksPath=/dev/null', '-c', 'protocol.file.allow=never', 'clone', '--depth', '1', '--', repo, join(f.home, 'project')]);
  const g = await fixture(t);
  await rm(g.authFile);
  await symlink(f.authFile, g.authFile);
  await assert.rejects(startWorkspaceJob(g.options), error => {
    assert.match(error.message, /private Claude authentication file/);
    assert.ok(!error.message.includes(secret));
    return true;
  });
  assert.equal(g.calls.length, 0);
});

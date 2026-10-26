import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { packageTemplate } from '../scripts/package-template.mjs';
import { startWorkspaceJob } from '../template/image/startup.mjs';
import { createBroker } from '../src/broker.mjs';
import { createClient } from '../src/client.mjs';
import { createCodexService } from '../src/codex.mjs';
import { createOpenCodeService } from '../src/opencode-service.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const secret = 'test-only-provider-token';

async function fixture(t, brokerOptions = {}) {
  const directory = await mkdtemp('/tmp/opencode/native-job-template-test-');
  const brokers = new Set();
  t.after(async () => {
    try { for (const broker of brokers) await broker.close(); }
    finally { await rm(directory, { recursive: true, force: true }); }
  });
  const packaged = join(directory, 'template');
  await packageTemplate(packaged);
  const home = join(directory, "owner's home");
  await mkdir(home);
  const authFile = join(directory, 'auth.json');
  await writeFile(authFile, JSON.stringify({ ANTHROPIC_AUTH_TOKEN: secret, ANTHROPIC_BASE_URL: 'https://provider.example.com' }), { mode: 0o600 });
  const prompt = "Read the project. $(touch /not-a-command)\n'quoted'; `literal` 🚀 $HOME ${CODER_AGENT_TOKEN} $UNSET_VARIABLE";
  const env = { HOME: home, CODER_NATIVE_JOB_PROMPT_BASE64: Buffer.from(prompt).toString('base64'), CODER_NATIVE_JOB_TRUST: 'true' };
  const calls = [];
  const run = async (command, args, options) => {
    calls.push({ command, args, options });
    assert.equal(options.env.CODER_NATIVE_JOB_PROMPT_BASE64, undefined);
    assert.equal(options.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64, undefined);
    assert.equal(options.env.CODEX_HOME, join(home, '.codex'));
    assert.ok(!args.some(value => value.includes(secret) || value.includes(prompt)));
    if (command === 'opencode') {
      assert.equal(process.umask(), 0o077, 'native process inherits private file permissions');
      const nativeHome = args[0] === '--version' ? options.env.HOME : join(home, '.local/state/coder-acp/opencode-home');
      if (args[0] === '--version') assert.ok(!nativeHome.startsWith(home), 'version probe cannot use persistent state');
      assert.equal(options.env.HOME, nativeHome);
      for (const [name, path] of Object.entries({ HOME: '', XDG_CONFIG_HOME: 'config', XDG_DATA_HOME: 'data', XDG_STATE_HOME: 'state', XDG_CACHE_HOME: 'cache', TMPDIR: 'tmp' })) {
        assert.equal(options.env[name], join(nativeHome, path));
        assert.equal((await stat(options.env[name])).mode & 0o777, 0o700, 'native directories exist and are private before preflight');
      }
      for (const key of ['OPENCODE_DB', 'OPENCODE_SERVER', 'OPENCODE_CONFIG', 'OPENCODE_TEST_HOME', 'OPENCODE_PASSWORD']) {
        assert.equal(options.env[key], undefined, 'preflight cannot inherit shared native overrides');
      }
      assert.equal(options.env.OPENCODE_CONFIG_CONTENT, '{"update":"disable"}');
      assert.equal(options.env.OPENCODE_DISABLE_AUTOUPDATE, 'true');
      if (args[0] === '--version') {
        assert.deepEqual(args, ['--version']);
        assert.equal(options.cwd, nativeHome);
        return { stdout: 'opencode v2.0.28\n' };
      }
      assert.deepEqual(args, ['api', '--standalone', 'get', '/api/model/default']);
      assert.equal(options.cwd, join(home, 'project'));
      return { stdout: JSON.stringify({ location: { directory: options.cwd }, data: { id: 'configured-model', providerID: 'configured-provider', enabled: true } }) };
    }
    assert.equal(options.env.HOME, home);
    for (const [name, path] of Object.entries({ XDG_CONFIG_HOME: '.config', XDG_DATA_HOME: '.local/share', XDG_STATE_HOME: '.local/state', XDG_CACHE_HOME: '.cache' })) {
      assert.equal(options.env[name], join(home, path));
    }
    if (command === 'codex') {
      assert.deepEqual(args.slice(0, 1), ['login']);
      if (args[1] === '--with-api-key') {
        assert.equal(options.input, secret + '\n');
        await writeFile(join(home, '.codex/auth.json'), JSON.stringify({ OPENAI_API_KEY: secret }), { mode: 0o600, flag: 'wx' });
      } else {
        assert.deepEqual(args, ['login', 'status']);
        await stat(join(home, '.codex/auth.json'));
      }
    }
    if (args[0] === 'serve') {
      const broker = await createBroker({ launch: () => assert.fail('bootstrap must not launch native sessions'), ...brokerOptions });
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
  assert.ok(paths.includes('image/shutdown.mjs'));
  assert.ok(!paths.includes('image/job-status.mjs'));
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
  assert.doesNotMatch(template, /key\s*= "native_job"/);
  assert.match(template, /CODER_NATIVE_JOB_PROMPT_BASE64\s*= base64encode\(data\.coder_parameter\.prompt\.value\)/);
  assert.match(template, /CODER_NATIVE_JOB_REPO_BASE64\s*= base64encode\(data\.coder_parameter\.repository\.value\)/);
  assert.match(template, /read_only\s*= true/);
  assert.ok(!template.includes(secret));
  const harness = template.split('data "coder_parameter" "harness" {')[1].split('\ndata "coder_parameter"')[0];
  assert.match(harness, /default\s*= "claude"/);
  assert.match(harness, /mutable\s*= false/);
  assert.match(harness, /value\s*= "codex"/);
  assert.match(harness, /value\s*= "opencode"/);
  assert.doesNotMatch(template, /(?:variable|coder_parameter)"?\s+"opencode_(?:api_key|auth)/);
  const key = template.split('variable "codex_api_key" {')[1].split('\n}')[0];
  assert.ok(!template.includes('data "coder_parameter" "codex_api_key"'), 'ordinary workspace parameters cannot safely carry keys');
  assert.match(key, /sensitive\s*= true/);
  assert.match(key, /default\s*= ""/);
  assert.match(template, /CODER_NATIVE_JOB_HARNESS\s*= data\.coder_parameter\.harness\.value/);
  assert.match(template, /CODER_NATIVE_JOB_CODEX_API_KEY_BASE64\s*= base64encode\(var\.codex_api_key\)/);
  assert.match(template, /CODEX_HOME\s*= "\/home\/coder\/\.codex"/);
  assert.match(template, /for_each\s*= var\.claude_auth_file != "" \? \[var\.claude_auth_file\] : \[\]/);
  const dockerfile = await readFile(join(f.packaged, 'image/Dockerfile'), 'utf8');
  assert.match(dockerfile, /@anthropic-ai\/claude-code@2\.1\.309/);
  assert.match(dockerfile, /@openai\/codex@0\.164\.0/);
  assert.match(dockerfile, /@opencode\/cli@2\.0\.28/);
  assert.match(dockerfile, /test "\$\(opencode --version\)" = "opencode v2\.0\.28"/);
  assert.doesNotMatch(dockerfile, /(?:install|add)[^\n]*opencode-ai@/);
  for (const [name, path] of Object.entries({ HOME: '/home/coder', XDG_CONFIG_HOME: '/home/coder/.config', XDG_DATA_HOME: '/home/coder/.local/share', XDG_STATE_HOME: '/home/coder/.local/state', XDG_CACHE_HOME: '/home/coder/.cache' })) {
    assert.ok(dockerfile.includes(name + '=' + path));
    assert.match(template, new RegExp(name + '\\s*= "' + path.replaceAll('.', '\\.') + '"'));
  }
  assert.match(dockerfile, /COPY startup\.mjs shutdown\.mjs read-auth\.mjs \/opt\/native-agent\//);
  const shutdown = template.split('resource "coder_script" "shutdown" {')[1].split('\n}')[0];
  assert.match(shutdown, /run_on_stop\s*= true/);
  assert.doesNotMatch(shutdown, /run_on_start\s*= true/);
  assert.match(shutdown, /timeout\s*= 60/);
  assert.match(shutdown, /exec node \/opt\/native-agent\/shutdown\.mjs/);
  assert.match(template, /destroy_grace_seconds\s*= 90/);
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
  assert.deepEqual(f.calls[1].args, ['serve', '--detach', '--codex', '--opencode', '--model', 'haiku']);
  assert.ok(f.calls[2].args.includes('--detach'));
  assert.equal((await stat(join(f.home, '.claude/settings.json'))).mode & 0o777, 0o600);
  const settings = JSON.parse(await readFile(join(f.home, '.claude/settings.json'), 'utf8'));
  assert.deepEqual(settings.env, { ANTHROPIC_AUTH_TOKEN: secret, ANTHROPIC_BASE_URL: 'https://provider.example.com' });
  const config = JSON.parse(await readFile(join(f.home, '.claude.json'), 'utf8'));
  assert.equal(config.projects[join(f.home, 'project')].hasTrustDialogAccepted, true);
  assert.ok(!JSON.stringify(f.calls).includes(secret));
  assert.deepEqual((await readdir(join(f.home, '.local/state/coder-acp'))).sort(), ['bridge.json', 'broker-runtime.json']);
});

test('Codex initial jobs require only native Codex auth and keep keys out of child environments and arguments', async t => {
  for (const credentials of ['key', 'existing', 'both']) {
    const existing = credentials !== 'key';
    const f = await fixture(t);
    f.env.CODER_NATIVE_JOB_HARNESS = 'codex';
    if (credentials !== 'existing') f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64 = Buffer.from(secret).toString('base64');
    f.env.CODER_NATIVE_JOB_MODEL = 'haiku';
    await rm(f.authFile);
    if (existing) {
      await mkdir(join(f.home, '.codex'), { mode: 0o700 });
      await writeFile(join(f.home, '.codex/auth.json'), JSON.stringify({ tokens: { access_token: 'retained-native-auth' } }), { mode: 0o600 });
    }
    assert.equal(await startWorkspaceJob(f.options), 'job dispatched\n');
    const login = f.calls.filter(call => call.command === 'codex');
    assert.equal(login.length, 1);
    assert.deepEqual(login[0].args, existing ? ['login', 'status'] : ['login', '--with-api-key']);
    assert.equal(login[0].options.input, existing ? undefined : secret + '\n');
    assert.deepEqual(f.calls.find(call => call.args[0] === 'serve').args, ['serve', '--detach', '--codex', '--opencode']);
    const run = f.calls.find(call => call.args[0] === 'run');
    assert.equal(run.args[run.args.indexOf('--harness') + 1], 'codex');
    assert.ok(!f.calls.some(call => call.args.includes('--model') || call.args.includes('--yolo') || call.args.includes('--dangerously-bypass-approvals-and-sandbox')));
    assert.equal((await stat(join(f.home, '.codex'))).mode & 0o777, 0o700);
    assert.equal((await stat(join(f.home, '.codex/auth.json'))).mode & 0o777, 0o600);
    if (existing) assert.match(await readFile(join(f.home, '.codex/auth.json'), 'utf8'), /retained-native-auth/);
    await assert.rejects(stat(join(f.home, '.claude.json')), { code: 'ENOENT' });
    await assert.rejects(stat(join(f.home, '.claude')), { code: 'ENOENT' });
    assert.ok(!JSON.stringify(f.calls.map(({ command, args, options: { input, ...options } }) => ({ command, args, options }))).includes(secret));
  }
});

test('OpenCode initial jobs use native private configuration without credentials or model overrides', async t => {
  const f = await fixture(t);
  f.env.CODER_NATIVE_JOB_HARNESS = 'opencode';
  f.env.CODER_NATIVE_JOB_MODEL = 'haiku';
  f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64 = Buffer.from(secret).toString('base64');
  f.env.XDG_CONFIG_HOME = '/unrelated-host-config';
  f.env.OPENCODE_DB = '/unrelated-host-database';
  f.env.OPENCODE_SERVER = 'http://127.0.0.1:1234';
  f.env.OPENCODE_CONFIG = '/unrelated-host-config/opencode.json';
  f.env.OPENCODE_CONFIG_CONTENT = '{"model":"unrelated/model"}';
  f.env.OPENCODE_TEST_HOME = '/unrelated-host-home';
  f.env.OPENCODE_PASSWORD = 'unrelated-server-password';
  await rm(f.authFile);
  const nativeHome = join(f.home, '.local/state/coder-acp/opencode-home');
  const configDirectory = join(nativeHome, 'config/opencode');
  await mkdir(configDirectory, { recursive: true, mode: 0o700 });
  const configFile = join(configDirectory, 'opencode.jsonc');
  const config = '{\n  // Native configuration is not rewritten.\n  "providers": { "test": { "settings": { "apiKey": "' + secret + '" } } }\n}\n';
  await writeFile(configFile, config, { mode: 0o600 });
  assert.equal(await startWorkspaceJob(f.options), 'job dispatched\n');
  assert.deepEqual(f.calls.map(call => call.args[0]), ['--version', 'api', join(f.packaged, 'image/adapter/install.sh'), 'serve', 'run']);
  assert.deepEqual(f.calls[1].args, ['api', '--standalone', 'get', '/api/model/default']);
  assert.deepEqual(f.calls[3].args, ['serve', '--detach', '--codex', '--opencode']);
  const run = f.calls[4];
  assert.equal(run.args[run.args.indexOf('--harness') + 1], 'opencode');
  assert.equal(await readFile(configFile, 'utf8'), config);
  for (const path of ['', 'config', 'config/opencode', 'data', 'state', 'cache', 'tmp']) {
    assert.equal((await stat(join(nativeHome, path))).mode & 0o777, 0o700);
  }
  for (const path of ['.config/opencode', '.local/share/opencode', '.local/state/opencode', '.cache/opencode']) {
    await assert.rejects(stat(join(f.home, path)), { code: 'ENOENT' });
  }
  assert.equal(f.env.OPENCODE_DB, '/unrelated-host-database', 'the caller environment is not mutated');
  assert.equal(f.env.HOME, f.home);
  await assert.rejects(stat(join(f.home, '.claude')), { code: 'ENOENT' });
  await assert.rejects(stat(join(f.home, '.codex/auth.json')), { code: 'ENOENT' });
  assert.ok(!f.calls.some(call => call.args.includes('--model') || call.args.includes('--auto')));
  assert.ok(!JSON.stringify(f.calls).includes(secret));
});

test('OpenCode version mismatch refuses standalone preflight before reading the persistent native home', async t => {
  for (const version of ['opencode v2.0.29\n', 'opencode v1.2.3\n', 'opencode v2.0.28-beta\n', secret]) {
    const f = await fixture(t);
    f.env.CODER_NATIVE_JOB_HARNESS = 'opencode';
    const nativeHome = join(f.home, '.local/state/coder-acp/opencode-home');
    await mkdir(nativeHome, { recursive: true, mode: 0o700 });
    await writeFile(join(nativeHome, 'opencode.db'), 'existing native database', { mode: 0o600 });
    await assert.rejects(startWorkspaceJob({ ...f.options, run: async (command, args, options) => {
      await f.run(command, args, options);
      assert.equal(command, 'opencode');
      assert.deepEqual(args, ['--version']);
      assert.ok(!options.env.HOME.startsWith(nativeHome));
      return { stdout: version };
    } }), error => {
      assert.match(error.message, /CLI must be exactly version 2\.0\.28/);
      assert.ok(!error.message.includes(secret));
      return true;
    });
    assert.deepEqual(f.calls.map(call => call.args), [['--version']]);
    await assert.rejects(stat(f.calls[0].options.env.HOME), { code: 'ENOENT' });
    assert.deepEqual(await readdir(nativeHome), ['opencode.db']);
    assert.equal(await readFile(join(nativeHome, 'opencode.db'), 'utf8'), 'existing native database');
    await assert.rejects(stat(join(f.home, '.local/state/coder-acp/job.json')), { code: 'ENOENT' });
    await assert.rejects(stat(join(f.home, '.local/state/coder-acp/bridge.json')), { code: 'ENOENT' });
  }
});

test('OpenCode initial preflight fails safely without an available native model and never mutates authentication', async t => {
  for (const response of [null, {}, { data: null }, { data: { enabled: false, id: 'model', providerID: 'provider' } }, secret]) {
    const f = await fixture(t);
    f.env.CODER_NATIVE_JOB_HARNESS = 'opencode';
    await rm(f.authFile);
    const run = async (command, args, options) => {
      if (command !== 'opencode' || args[0] === '--version') return f.run(command, args, options);
      await f.run(command, args, options);
      if (response === null) throw new Error(secret);
      return { stdout: typeof response === 'string' ? response : JSON.stringify(response) };
    };
    await assert.rejects(startWorkspaceJob({ ...f.options, run }), error => {
      assert.match(error.message, /OpenCode.*native default model/);
      assert.match(error.message, /\/connect and \/models/);
      assert.ok(!error.message.includes(secret));
      return true;
    });
    assert.deepEqual(f.calls.map(call => call.args[0]), ['--version', 'api']);
    await assert.rejects(stat(join(f.home, '.local/state/coder-acp/job.json')), { code: 'ENOENT' });
    await assert.rejects(stat(join(f.home, '.local/state/coder-acp/bridge.json')), { code: 'ENOENT' });
    assert.equal(await startWorkspaceJob(f.options), 'job dispatched\n', 'a preflight refusal did not claim dispatch');
  }
});

test('native login runner uses stdin and suppresses credential-bearing native output', async t => {
  const f = await fixture(t);
  f.env.CODER_NATIVE_JOB_HARNESS = 'codex';
  f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64 = Buffer.from(secret).toString('base64');
  const bin = join(f.directory, 'bin');
  await mkdir(bin, { mode: 0o700 });
  f.env.PATH = bin + ':' + process.env.PATH;
  await writeFile(join(bin, 'codex'), `#!${process.execPath}
import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
assert.deepEqual(process.argv.slice(2), ['login', '--with-api-key']);
assert.equal(process.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64, undefined);
assert.equal(process.env.CODER_NATIVE_JOB_PROMPT_BASE64, undefined);
process.stdin.setEncoding('utf8');
let input = '';
for await (const chunk of process.stdin) input += chunk;
assert.equal(input, ${JSON.stringify(secret + '\n')});
await writeFile(join(process.env.CODEX_HOME, 'auth.json'), JSON.stringify({ OPENAI_API_KEY: input.trim() }), { mode: 0o600, flag: 'wx' });
console.log(input);
console.error(input);
`, { mode: 0o700 });
  // The package intentionally has no node_modules, so launcher installation fails
  // after native login. The setup error must never forward child output.
  await assert.rejects(startWorkspaceJob({ ...f.options, run: undefined }), error => {
    assert.match(error.message, /Workspace setup command failed/);
    assert.ok(!error.message.includes(secret));
    return true;
  });
  assert.equal(JSON.parse(await readFile(join(f.home, '.codex/auth.json'), 'utf8')).OPENAI_API_KEY, secret);
  assert.equal((await stat(join(f.home, '.codex/auth.json'))).mode & 0o777, 0o600);
});

test('Codex authentication is conditional, private, and fails without disclosing provider diagnostics', async t => {
  const f = await fixture(t);
  f.env.CODER_NATIVE_JOB_HARNESS = 'codex';
  await rm(f.authFile);
  await assert.rejects(startWorkspaceJob(f.options), /Codex authentication/);
  assert.ok(!f.calls.some(call => call.args[0] === 'serve' || call.args[0] === 'run'));
  f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64 = Buffer.from(secret).toString('base64');
  await assert.rejects(startWorkspaceJob({ ...f.options, run: async () => { throw new Error(secret); } }), error => {
    assert.match(error.message, /Codex authentication/);
    assert.ok(!error.message.includes(secret));
    return true;
  });
  await symlink(f.authFile, join(f.home, '.codex/auth.json'));
  await assert.rejects(startWorkspaceJob(f.options), /Codex authentication/);
  assert.equal(f.calls.filter(call => call.args.includes('--with-api-key')).length, 0);
});

test('Claude initial jobs still require Claude authentication, ignore Codex keys, and reject invalid harnesses', async t => {
  const f = await fixture(t);
  f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64 = Buffer.from(secret).toString('base64');
  await rm(f.authFile);
  await assert.rejects(startWorkspaceJob(f.options), /Claude authentication/);
  assert.equal(f.calls.length, 0);
  f.env.CODER_NATIVE_JOB_HARNESS = 'other';
  await assert.rejects(startWorkspaceJob(f.options), /harness/);
  assert.equal(f.calls.length, 0);
});

test('blocked setup instructs Open with the known session and never claims the prompt was submitted', async t => {
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
  assert.match(output, /Open/);
  assert.match(output, /initial task was not submitted/i);
  assert.ok(output.includes(sessionId));
  assert.ok(!output.includes(secret) && !output.includes(f.prompt));
  assert.deepEqual((await readdir(join(f.home, '.local/state/coder-acp'))).sort(), ['bridge.json', 'broker-runtime.json', 'job.json']);
  const original = { ...job };
  for (const change of [{ inputHash: 'bad' }, { status: 'failed' }, { sessionId: secret }, { dispatchAttemptedAt: new Date().toISOString() }]) {
    Object.assign(job, original, change);
    await assert.rejects(startWorkspaceJob(options), /Workspace setup command failed/);
  }
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

async function retainedBroker(t, { harness = 'claude', ...brokerOptions } = {}) {
  const f = await fixture(t, brokerOptions);
  if (harness !== 'claude') f.env.CODER_NATIVE_JOB_HARNESS = harness;
  if (harness === 'codex') f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64 = Buffer.from(secret).toString('base64');
  const instance = 'a'.repeat(64);
  f.options.identifyInstance = async () => instance;
  await startWorkspaceJob(f.options);
  f.state = join(f.home, '.local/state/coder-acp');
  f.jobFile = join(f.state, 'job.json');
  const timestamp = new Date().toISOString();
  f.job = JSON.stringify({ version: 1, jobId: 'bd72142c-b0ed-474c-9e9e-237345db5784',
    sessionId: harness === 'opencode' ? 'opencode:ses_0123456789abAbCdEfGhIjKlMn' : (harness === 'codex' ? 'codex:' : '') + 'ad72142c-b0ed-474c-9e9e-237345db5784',
    ...(harness !== 'claude' ? { harness } : {}), status: 'blocked', errorCode: 'native_input_required',
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
  assert.match(output, /initial task was not replayed/);
  assert.doesNotMatch(output, /Select Open/);
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

test('startup restores mixed saved sessions through the real broker, preserves native refusals and the harness replay guard', { timeout: 15000 }, async t => {
  const f = await retainedBroker(t);
  await f.deadDescriptor();
  f.options.identifyInstance = async () => 'b'.repeat(64);
  f.env.CODER_NATIVE_JOB_HARNESS = 'codex';
  const cwd = join(f.home, 'project');
  const claude = [JSON.parse(f.job).sessionId, randomUUID()];
  const codex = [randomUUID(), randomUUID(), randomUUID()];
  const opencode = ['ses_0123456789abAbCdEfGhIjKlMn', 'ses_0123456789abOpQrStUvWxYz01'];
  const record = sessionId => ({ sessionId, cwd, startedAt: 1, runtimeId: 'old-runtime', cursor: 0, historyStartCursor: 0, events: [], fault: false });
  const sessionFile = join(f.state, 'sessions.json');
  const codexFile = join(f.state, 'codex-sessions.json');
  const opencodeFile = join(f.state, 'opencode-sessions.json');
  await writeFile(sessionFile, JSON.stringify({ version: 1, sessions: claude.map(record) }), { mode: 0o600 });
  await writeFile(codexFile, JSON.stringify({ version: 1, sessions: codex.map(id => ({ ...record(id), promptTombstones: [] })) }), { mode: 0o600 });
  await writeFile(opencodeFile, JSON.stringify({ version: 1, sessions: opencode.map(id => ({ ...record(id), promptTombstones: [] })) }), { mode: 0o600 });
  const nativeCalls = [];
  const openCodeCalls = [];
  const resumed = [];
  const reports = [];
  f.options.report = result => reports.push(result);
  const entering = Promise.withResolvers();
  const registration = Promise.withResolvers();
  const stop = new AbortController();
  let pumping;
  let client;
  t.after(async () => { stop.abort(); await pumping; });
  f.options.run = async (command, args, settings) => {
    f.calls.push({ command, args, options: settings });
    if (args[0] === 'serve') {
      const service = await createCodexService({ sessionFile: codexFile, rpcFactory: () => ({
        async request(method, params) {
          nativeCalls.push({ method, params });
          if (method === 'initialize') return { userAgent: 'codex_cli_rs/0.164.0 (Linux)', platformFamily: 'unix', codexHome: join(f.home, '.codex') };
          const id = params.threadId;
          assert.ok(codex.includes(id));
          if (method === 'thread/read') return { thread: { id, cwd, status: { type: id === codex[0] ? 'active' : 'notLoaded' } } };
          if (method === 'thread/queue/list') return { data: id === codex[1] ? [secret] : [], nextCursor: null };
          if (method === 'thread/goal/get') return { goal: { status: 'active' } };
          assert.equal(method, 'thread/resume', 'no native turn start, interrupt, or task replay');
          assert.deepEqual(params, { threadId: codex[0], excludeTurns: true, initialTurnsPage: { limit: 1, itemsView: 'summary', sortDirection: 'desc' } });
          return { thread: { id, cwd, status: { type: 'active' } }, initialTurnsPage: { data: [{ id: 'native-active-turn', status: 'inProgress', items: [
            { id: 'user-item', type: 'userMessage', clientId: 'native-user', content: [{ type: 'text', text: secret, text_elements: [] }] },
          ] }] } };
        },
        notify(method) { assert.equal(method, 'initialized'); },
        close() {},
      }) });
      const openCodeService = await createOpenCodeService({ sessionFile: opencodeFile,
        setTimer: () => ({ unref() {} }), clearTimer() {}, clientFactory: async () => ({
          async request(method, path) {
            openCodeCalls.push({ method, path });
            assert.equal(method, 'GET', 'OpenCode restoration cannot create sessions, prompt, interrupt, or change auth');
            if (path === '/api/info') return { version: '2.0.28' };
            if (path === '/api/session/active') return { data: {} };
            if (path.startsWith('/api/shell?')) return { location: { directory: cwd }, data: [] };
            const match = path.match(/^\/api\/session\/(ses_[A-Za-z0-9]+)(.*)$/);
            assert.ok(match);
            assert.ok(opencode.includes(match[1]), 'native identity is unchanged and case-preserving');
            if (!match[2]) return { data: { id: match[1], location: { directory: cwd }, time: { created: 1 } } };
            if (match[2].startsWith('/message?')) return { data: [], cursor: {} };
            assert.ok(['/inbox', '/permission', '/form'].includes(match[2]));
            return { data: [] };
          },
          close() {},
        }),
      });
      const launchIds = new Map(claude.map(id => [id, randomUUID()]));
      const broker = await createBroker({ sessionFile, codex: service, opencode: openCodeService,
        launch: () => assert.fail('restoration cannot create a new native identity'),
        discover: async () => claude.map(sessionId => ({ sessionId, cwd, launchId: launchIds.get(sessionId), startedAt: 1, ended: true })),
        resume: async (id, project) => {
          resumed.push(id);
          assert.equal(project, cwd);
          if (id === claude[1]) throw Object.assign(new Error(secret), { resumeRefused: true });
          entering.resolve();
          await registration.promise;
          const runtime = { sessionId: id, runtimeId: randomUUID(), launchId: launchIds.get(id), cwd, mode: 'automation', activeTurnId: null, events: [] };
          pumping = (async () => {
            let seq = 0;
            while (!stop.signal.aborted) {
              const automation = runtime.events.some(event => event.type === 'mode' && event.data.mode === 'automation');
              const result = await client.request('POST', '/v1/runtime', runtime, stop.signal);
              if (automation) break;
              runtime.events = [];
              for (const command of result.commands) {
                assert.equal(command.type, 'mode', 'restoration never prompts or cancels');
                runtime.mode = command.mode;
                runtime.events.push({ seq: ++seq, type: 'mode', data: { mode: command.mode } },
                  { seq: ++seq, type: 'command.done', data: { commandId: command.id } });
              }
            }
          })().catch(error => { if (!stop.signal.aborted) throw error; });
        },
      });
      f.brokers.add(broker);
      client = createClient(broker);
      await writeFile(join(f.state, 'bridge.json'), JSON.stringify({ url: broker.url, token: broker.token, pid: process.pid }), { mode: 0o600, flag: 'wx' });
      await broker.preparePersistence();
      return { stdout: '' };
    }
    assert.equal(args[0], 'run');
    const sessions = (await client.request('GET', '/v1/sessions')).sessions;
    for (const id of [claude[0], 'codex:' + codex[0], ...opencode.map(id => 'opencode:' + id)]) {
      const s = sessions.find(s => s.sessionId === id);
      assert.equal(s.mode, 'automation', 'job guard is checked only after runtime and automation readiness');
      assert.equal(s.archived, false);
      assert.equal(s.resumePending, false);
      assert.equal(s.registrationPending, false);
    }
    assert.equal(sessions.find(s => s.sessionId === 'codex:' + codex[0]).activeTurnId, 'native-active-turn');
    return promisify(execFile)(process.execPath, [join(root, 'src/cli.mjs'), ...args, '--state', join(f.state, 'bridge.json')], settings);
  };
  const started = startWorkspaceJob(f.options);
  const rejected = assert.rejects(started, /different input|different harness/);
  await entering.promise;
  assert.ok(!f.calls.some(c => c.args[0] === 'run'));
  try { await assert.rejects(startWorkspaceJob(f.options), /already attempted or in progress/); }
  finally { registration.resolve(); }
  await rejected;
  assert.deepEqual(resumed, claude);
  assert.deepEqual(reports.map(r => r.status), ['restored', 'failed', 'restored', 'failed', 'failed', 'restored', 'restored']);
  assert.deepEqual(openCodeCalls.filter(c => /^\/api\/session\/ses_[A-Za-z0-9]+$/.test(c.path)).map(c => c.path), opencode.map(id => '/api/session/' + id));
  assert.ok(!JSON.stringify(reports).includes(secret));
  assert.equal(nativeCalls.filter(c => c.method === 'thread/resume').length, 1, 'queues and unfinished goals remain refused');
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job, 'changing the selected harness cannot replace its guard or replay the initial task');
  f.env.CODER_NATIVE_JOB_HARNESS = 'claude';
  const output = await startWorkspaceJob(f.options);
  assert.match(output, /ready for automation/);
  assert.doesNotMatch(output, /Select Open|Select Join/);
  assert.deepEqual(resumed, claude, 'repeated startup skips live sessions and previous failed attempts');
  assert.equal(nativeCalls.filter(c => c.method === 'thread/resume').length, 1);
  assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
  assert.deepEqual(f.calls.map(c => c.args[0]), ['serve', 'run', 'run']);
});

for (const harness of ['codex', 'opencode']) {
  test(harness + ' restarts recover the broker without authentication setup, native turns, or replaying the retained opaque session', async t => {
    const f = await retainedBroker(t, { harness });
    const old = await f.deadDescriptor(process.pid);
    f.options.identifyInstance = async () => 'b'.repeat(64);
    delete f.env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64;
    f.env.CODER_NATIVE_JOB_TRUST = 'false';
    await rm(f.authFile);
    if (harness === 'codex') await rm(join(f.home, '.codex/auth.json'));
    f.options.run = async (command, args, settings) => {
      assert.notEqual(command, 'opencode', 'retained jobs never run provider preflight');
      const result = await f.run(command, args, settings);
      if (args[0] !== 'run') return result;
      return promisify(execFile)(process.execPath, [join(root, 'src/cli.mjs'), ...args, '--state', join(f.state, 'bridge.json')], settings);
    };
    const output = await startWorkspaceJob(f.options);
    assert.ok(output.includes(JSON.parse(f.job).sessionId));
    assert.match(output, /old native session was not verified live/);
    assert.match(output, /initial task was not replayed/);
    assert.equal(await readFile(f.jobFile, 'utf8'), f.job);
    assert.notEqual(await readFile(join(f.state, 'bridge.json'), 'utf8'), old);
    assert.deepEqual(f.calls.map(call => call.args[0]), ['serve', 'run']);
    assert.deepEqual(f.calls[0].args, ['serve', '--detach', '--codex', '--opencode']);
  });
}

test('changing harness rejects retained blocked and unknown jobs without overwriting or hiding a CLI failure', async t => {
  const harnesses = ['claude', 'codex', 'opencode'];
  for (const [harness, selected] of harnesses.flatMap(harness => harnesses.filter(other => other !== harness).map(other => [harness, other]))) {
    const f = await retainedBroker(t, { harness });
    f.env.CODER_NATIVE_JOB_HARNESS = selected;
    f.options.run = async (command, args, settings) => {
      await f.run(command, args, settings);
      return promisify(execFile)(process.execPath, [join(root, 'src/cli.mjs'), ...args, '--state', join(f.state, 'bridge.json')], settings);
    };
    for (const status of ['blocked', 'unknown']) {
      const record = JSON.parse(f.job);
      record.status = status;
      record.errorCode = status === 'blocked' ? 'native_input_required' : 'result_unknown';
      const original = JSON.stringify(record);
      await writeFile(f.jobFile, original);
      await assert.rejects(startWorkspaceJob(f.options), /different input|different harness/);
      assert.equal(await readFile(f.jobFile, 'utf8'), original);
    }
    assert.deepEqual(f.calls.map(call => call.args[0]), ['run', 'run']);
  }
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
  let time = Date.now();
  const f = await retainedBroker(t, { now: () => time, discover: async () => {
    discoveries++;
    // Model a slow native discovery response within its supported five-second
    // deadline, rather than delaying startup to hide an ordering race.
    await new Promise(resolve => setTimeout(resolve, 3200));
    return [];
  } });
  const descriptor = await readFile(join(f.state, 'bridge.json'), 'utf8');
  time += 5000;
  await startWorkspaceJob(f.options);
  assert.equal(discoveries, 2, 'both the new and reused broker require an authenticated session refresh');
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

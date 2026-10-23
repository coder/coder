#!/usr/bin/env node
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, mkdtemp, open, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execute = promisify(execFile);
const installedRoot = '/opt/native-agent';

async function exists(path) {
  try { await lstat(path); return true; }
  catch (error) { if (error.code === 'ENOENT') return false; throw error; }
}

async function privateJSON(path) {
  return (await privateRecord(path)).value;
}

async function privateRecord(path) {
  const handle = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) || info.size > 1024 * 1024) {
      throw new Error('Authentication and configuration files must be owner-only regular files.');
    }
    const text = await handle.readFile('utf8');
    let value;
    try { value = JSON.parse(text); } catch { throw new Error('Invalid private runtime JSON. Inspect state before retrying.'); }
    return { info, text, value };
  } finally { await handle.close(); }
}

async function createJSON(path, value) {
  await writeFile(path, JSON.stringify(value) + '\n', { mode: 0o600, flag: 'wx' });
}

function decodeParameter(value = '') {
  if (typeof value !== 'string' || value.length > 176000) throw new Error('Invalid encoded job parameter.');
  const bytes = Buffer.from(value, 'base64');
  if (bytes.toString('base64') !== value) throw new Error('Invalid encoded job parameter.');
  try { return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes); }
  catch { throw new Error('Invalid encoded job parameter.'); }
}

// PID 1 start ticks distinguish Docker restarts on the same host boot. The PID
// namespace inode also distinguishes containers created in the same clock tick.
async function containerInstance() {
  const boot = (await readFile('/proc/sys/kernel/random/boot_id', 'utf8')).trim();
  const init = await readFile('/proc/1/stat', 'utf8');
  const ticks = init.slice(init.lastIndexOf(')') + 2).split(/\s+/)[19];
  const namespace = await stat('/proc/self/ns/pid');
  if (!/^[a-f0-9-]{36}$/.test(boot) || !/^\d+$/.test(ticks)) throw new Error('Cannot identify this container instance.');
  return createHash('sha256').update(JSON.stringify([boot, ticks, namespace.ino])).digest('hex');
}

const inspectBroker = 'Broker ownership is uncertain or this instance already attempted startup. Inspect native sessions and private broker state; nothing was restarted.';

async function sameRecord(path, record) {
  const current = await privateRecord(path);
  if (current.info.dev !== record.info.dev || current.info.ino !== record.info.ino || current.text !== record.text) {
    throw new Error(inspectBroker);
  }
}

async function runtimeMarker(path, value) {
  const temporary = await mkdtemp(path + '-');
  try {
    const file = join(temporary, 'marker');
    const handle = await open(file, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
    try { await handle.writeFile(JSON.stringify(value) + '\n'); await handle.sync(); }
    finally { await handle.close(); }
    await rename(file, path);
    const directory = await open(dirname(path), constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW);
    try { await directory.sync(); } finally { await directory.close(); }
  } finally { await rm(temporary, { recursive: true, force: true }); }
}

// This is template-only policy. Generic clients and serve still never recover
// descriptors. The caller holds the instance lock through dispatch bookkeeping.
async function ensureBroker({ state, instance, initial, launch }) {
  const descriptorFile = join(state, 'bridge.json');
  const markerFile = join(state, 'broker-runtime.json');
  const marker = await exists(markerFile) ? await privateJSON(markerFile) : undefined;
  if (marker && (marker.version !== 1 || !/^[a-f0-9]{64}$/.test(marker.instance))) throw new Error(inspectBroker);
  const descriptor = await exists(descriptorFile) ? await privateRecord(descriptorFile) : undefined;
  if (descriptor) {
    const { url, token, pid } = descriptor.value;
    let endpoint;
    try { endpoint = new URL(url); } catch { throw new Error(inspectBroker); }
    if (endpoint.protocol !== 'http:' || endpoint.hostname !== '127.0.0.1' || !endpoint.port || endpoint.origin !== url ||
        !/^[a-f0-9]{64}$/.test(token) || !Number.isSafeInteger(pid) || pid <= 0) throw new Error(inspectBroker);
    let response;
    try {
      response = await fetch(url + '/v1/sessions', {
        headers: { Authorization: 'Bearer ' + token }, redirect: 'error', signal: AbortSignal.timeout(10000),
      });
    } catch (error) {
      // A timeout, reset, or other transport error is not evidence of a dead broker.
      if (error.cause?.code !== 'ECONNREFUSED') throw new Error(inspectBroker);
    }
    if (response) {
      if (!response.ok) { await response.body?.cancel(); throw new Error(inspectBroker); }
      const result = await response.json().catch(() => undefined);
      if (!Array.isArray(result?.sessions)) throw new Error(inspectBroker);
      await sameRecord(descriptorFile, descriptor);
      await runtimeMarker(markerFile, { version: 1, instance, token });
      return result.sessions;
    }
    if (marker) {
      // A new container cannot contain the old runtime, even if its PID is reused.
      if (marker.instance === instance || marker.token !== token) throw new Error(inspectBroker);
    } else {
      // Older templates did not record container ownership. Migrate only when
      // both the saved listener and process are absent; never kill a reused PID.
      try { process.kill(pid, 0); throw new Error(inspectBroker); }
      catch (error) { if (error.code !== 'ESRCH') throw new Error(inspectBroker); }
    }
    await sameRecord(descriptorFile, descriptor);
    await rm(descriptorFile);
  } else if ((!marker && !initial) || marker?.instance === instance) {
    throw new Error(inspectBroker);
  }
  // Persist the attempt before launching: failure must not trigger another broker
  // in this namespace, even if it failed before publishing its descriptor.
  await runtimeMarker(markerFile, { version: 1, instance });
  await launch();
  const created = await privateJSON(descriptorFile);
  if (typeof created.token !== 'string' || !/^[a-f0-9]{64}$/.test(created.token)) throw new Error(inspectBroker);
  await runtimeMarker(markerFile, { version: 1, instance, token: created.token });
  return [];
}

/** Start one workspace job; retained dispatch state prevents automatic resubmission. */
export async function startWorkspaceJob({
  env = process.env,
  root = installedRoot,
  authFile = '/run/secrets/claude-auth.json',
  identifyInstance = containerInstance,
  run = async (command, args, options) => {
    try { return await execute(command, args, { ...options, timeout: 120000, maxBuffer: 1024 * 1024 }); }
    catch { throw new Error('Workspace setup command failed. Inspect native job state before retrying; nothing was resubmitted.'); }
  },
} = {}) {
  const { privateDirectory } = await import(join(root, 'adapter/src/service.mjs'));
  const home = env.HOME;
  if (!home || !home.startsWith('/')) throw new Error('An absolute HOME is required.');
  const prompt = decodeParameter(env.CODER_NATIVE_JOB_PROMPT_BASE64);
  if (typeof prompt !== 'string' || !prompt.trim() || prompt.length > 32000) throw new Error('Enter a nonempty prompt of at most 32,000 characters.');
  const repository = decodeParameter(env.CODER_NATIVE_JOB_REPO_BASE64);
  if (repository) {
    let url;
    try { url = new URL(repository); } catch { throw new Error('Repository must be a public HTTPS URL.'); }
    if (url.protocol !== 'https:' || url.username || url.password) throw new Error('Repository must be a public HTTPS URL without embedded credentials.');
  }
  const model = env.CODER_NATIVE_JOB_MODEL || 'haiku';
  if (!['haiku', 'sonnet', 'opus'].includes(model)) throw new Error('Choose Haiku, Sonnet, or Opus.');
  const state = join(home, '.local/state/coder-acp');
  await privateDirectory(state);
  const jobFile = join(state, 'job.json');
  const project = join(home, 'project');
  const launcher = join(home, '.local/bin/coder-acp');
  const instance = await identifyInstance();
  if (!/^[a-f0-9]{64}$/.test(instance)) throw new Error('Cannot identify this container instance.');
  // Locks are namespace-specific: a crashed bootstrap blocks retries in its own
  // namespace but cannot block a later Docker instance. Never reap a live lock.
  const lock = join(state, 'startup-' + instance + '.lock');
  try { await mkdir(lock, { mode: 0o700 }); }
  catch (error) { if (error.code === 'EEXIST') throw new Error('Workspace startup already attempted or in progress in this container. Inspect state before retrying.'); throw error; }
  let temporary;
  const commandEnv = { ...env, GIT_TERMINAL_PROMPT: '0' };
  // Native children need provider configuration, not a copy of the job prompt.
  delete commandEnv.CODER_NATIVE_JOB_PROMPT_BASE64;
  const invoke = (command, args, cwd = home) => run(command, args, { cwd, env: commandEnv });
  try {
    temporary = await mkdtemp(join(state, 'prompt-'));
    const promptFile = join(temporary, 'prompt.txt');
    await writeFile(promptFile, prompt, { mode: 0o600, flag: 'wx' });
    const initial = !await exists(jobFile);
    if (!initial) await privateJSON(jobFile);
    if (initial) {
      if (env.CODER_NATIVE_JOB_TRUST !== 'true') throw new Error('Select Trust this project to authorize unattended startup. Tool permissions are not bypassed.');
      // A previous broker can own native work even if launch bookkeeping failed.
      if (await exists(join(state, 'bridge.json'))) throw new Error('Broker state already exists. Inspect native sessions before attempting another launch.');
      const authentication = await privateJSON(authFile).catch(() => { throw new Error('Cannot read the private Claude authentication file. Check the Docker-host path, file format, ownership, and permissions.'); });
      await privateDirectory(join(home, '.claude'));
      if (authentication.claudeAiOauth?.accessToken) {
        if (!await exists(join(home, '.claude/.credentials.json'))) await createJSON(join(home, '.claude/.credentials.json'), authentication);
      } else if (typeof authentication.ANTHROPIC_API_KEY === 'string' && authentication.ANTHROPIC_API_KEY) {
        // The helper reads the read-only mount; no API key enters Terraform state.
        if (!await exists(join(home, '.claude/settings.json'))) {
          await createJSON(join(home, '.claude/settings.json'), { apiKeyHelper: 'node /opt/native-agent/read-auth.mjs' });
        }
      } else if (typeof authentication.ANTHROPIC_AUTH_TOKEN === 'string' && authentication.ANTHROPIC_AUTH_TOKEN && typeof authentication.ANTHROPIC_BASE_URL === 'string') {
        let url;
        try { url = new URL(authentication.ANTHROPIC_BASE_URL); } catch { throw new Error('Invalid provider gateway URL.'); }
        if (url.protocol !== 'https:' || url.username || url.password) throw new Error('Provider gateway URL must use HTTPS without embedded credentials.');
        if (!await exists(join(home, '.claude/settings.json'))) {
          await createJSON(join(home, '.claude/settings.json'), { env: {
            ANTHROPIC_AUTH_TOKEN: authentication.ANTHROPIC_AUTH_TOKEN,
            ANTHROPIC_BASE_URL: authentication.ANTHROPIC_BASE_URL,
          } });
        }
      } else {
        throw new Error('Authentication file must contain Claude OAuth credentials, ANTHROPIC_API_KEY, or an HTTPS provider gateway and bearer token.');
      }
      if (!await exists(project)) {
        if (repository) await invoke('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'protocol.file.allow=never', 'clone', '--depth', '1', '--', repository, project]);
        else await mkdir(project, { mode: 0o700 });
      } else if (repository && !await exists(join(project, '.git'))) {
        throw new Error('Project directory already exists without a Git checkout. Inspect it before retrying.');
      }
      const configFile = join(home, '.claude.json');
      if (!await exists(configFile)) {
        await createJSON(configFile, { hasCompletedOnboarding: true, projects: { [project]: { hasTrustDialogAccepted: true } } });
      } else {
        const config = await privateJSON(configFile);
        if (!config.projects?.[project]?.hasTrustDialogAccepted) throw new Error('Existing Claude configuration has not trusted this project. Open Claude in the project and confirm trust.');
      }
      await invoke('/bin/sh', [join(root, 'adapter/install.sh')]);
    }
    const sessions = await ensureBroker({ state, instance, initial,
      launch: () => invoke(launcher, ['serve', '--detach', '--model', model]),
    });
    // run checks its durable guard before connecting, including after a reboot.
    try {
      const result = await invoke(launcher, ['run', '--detach', '--cwd', project, '--prompt-file', promptFile, '--job-file', jobFile]);
      return result.stdout;
    } catch (error) {
      const job = await privateJSON(jobFile).catch(() => undefined);
      const inputHash = createHash('sha256').update(JSON.stringify([project, prompt])).digest('hex');
      if (!initial && job?.version === 1 && job.inputHash === inputHash && job.status === 'unknown') {
        return 'The broker is available. The retained initial job has an unknown outcome; inspect native history and tool side effects before starting new work. Nothing was resumed or replayed.\n';
      }
      if (job?.version !== 1 || job.status !== 'blocked' || job.errorCode !== 'native_input_required' ||
          job.inputHash !== inputHash || job.dispatchAttemptedAt !== undefined ||
          typeof job.sessionId !== 'string' || !/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(job.sessionId)) throw error;
      if (!initial && !sessions.some(session => session.sessionId === job.sessionId && !session.ended && !session.stale && !session.fault)) {
        return 'Retained blocked job for native session ' + job.sessionId + '. The initial task was not submitted. ' +
          'The broker is available, but the old native session was not verified live. Inspect native sessions before explicitly starting new work. Nothing was resumed or replayed.\n';
      }
      return 'Native session ' + job.sessionId + ' needs attention. Select Join on its session card to continue native setup. ' +
        'The initial task was not submitted. After setup, select Observe and submit your task explicitly. Nothing was replayed.\n';
    }
  } finally {
    if (temporary) await rm(temporary, { recursive: true, force: true });
    await rm(lock, { recursive: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await startWorkspaceJob().then(
    output => process.stdout.write(output),
    error => { console.error(error.message); process.exitCode = 1; },
  );
}

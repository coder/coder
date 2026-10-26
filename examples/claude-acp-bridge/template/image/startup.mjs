#!/usr/bin/env node
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { constants } from 'node:fs';
import { lstat, mkdir, mkdtemp, open, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
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
      return { sessions: result.sessions, descriptor: descriptor.value };
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
  // A newly launched broker also needs authenticated discovery. Its archives are
  // not necessarily empty, and readiness alone does not register native runtimes.
  return ensureBroker({ state, instance, initial, launch });
}

function isSessionID(id) {
  return typeof id === 'string' && (/^(?:codex:)?[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(id) || /^opencode:ses_[A-Za-z0-9]+$/.test(id));
}
const inspectRestoration = 'Session restoration state is uncertain. Inspect private startup state before retrying; no saved task was replayed.';

/**
 * Restore archived bridge sessions under the caller's workspace startup lock.
 * Each wake is durably claimed before dispatch, including failed/unknown wakes.
 * Returns refreshed sessions and safe per-session outcomes, never native output.
 */
export async function restoreSessions({ state, instance, descriptor, sessions, client,
  timeoutMs = 60000, pollMs = 100, wait = delay, report = () => {} }) {
  const tokenHash = createHash('sha256').update(descriptor.token).digest('hex');
  const file = join(state, 'restore-' + createHash('sha256').update(instance + tokenHash).digest('hex') + '.json');
  const saved = await exists(file) ? await privateJSON(file) : { version: 1, instance, tokenHash, attempted: [] };
  if (saved?.version !== 1 || saved.instance !== instance || saved.tokenHash !== tokenHash ||
      !Array.isArray(saved.attempted) || saved.attempted.some(id => !isSessionID(id)) ||
      new Set(saved.attempted).size !== saved.attempted.length) throw new Error(inspectRestoration);
  const attempted = new Set(saved.attempted);
  const results = [];
  const refreshed = [...sessions];
  for (const [index, original] of sessions.entries()) {
    const id = original?.sessionId;
    // Only broker-advertised native identities qualify. Live and pending sessions
    // remain untouched, including work already running under human control.
    if (!isSessionID(id) || original.archived !== true ||
        original.resumable !== true || original.resumePending || original.registrationPending) continue;
    if (attempted.has(id)) {
      const result = { sessionId: id, status: 'skipped', reason: 'already_attempted' };
      results.push(result);
      report(result);
      continue;
    }
    attempted.add(id);
    saved.attempted = [...attempted];
    await runtimeMarker(file, saved);
    const signal = AbortSignal.timeout(timeoutMs);
    const path = '/v1/sessions/' + encodeURIComponent(id);
    let stage = 'inspection';
    try {
      // Earlier restorations can take time. Do not wake a session which became
      // live or pending after the initial list was read.
      const latest = await client.request('GET', path, undefined, signal);
      if (latest?.sessionId !== id || latest.cwd !== original.cwd ||
          ['archived', 'resumable', 'resumePending', 'registrationPending'].some(key => typeof latest[key] !== 'boolean')) throw new Error('Session identity not confirmed');
      refreshed[index] = latest;
      if (!latest.archived || latest.resumable !== true || latest.resumePending || latest.registrationPending) {
        const result = { sessionId: id, status: 'skipped', reason: 'no_longer_eligible' };
        results.push(result);
        report(result);
        continue;
      }
      stage = 'resume';
      await client.request('POST', path + '/resume', {}, signal);
      stage = 'registration';
      let current;
      let runtimeId;
      const ready = async () => {
        current = await client.request('GET', path, undefined, signal);
        if (current?.sessionId !== id || current.cwd !== original.cwd || current.archived !== false || current.ended !== false ||
            current.fault !== false || current.stale !== false || !['human', 'automation'].includes(current.mode) ||
            (runtimeId !== undefined && current.runtimeId !== runtimeId)) {
          throw new Error('Runtime not available');
        }
        refreshed[index] = current;
        return current.resumePending === false && current.registrationPending === false && typeof current.runtimeId === 'string' && current.runtimeId.length > 0;
      };
      while (!await ready()) await wait(pollMs, undefined, { signal });
      runtimeId = current.runtimeId;
      stage = 'automation';
      while (current.mode !== 'automation') {
        try {
          // Enabling automation never cancels an active native turn or submits a
          // prompt. The broker waits for Claude's native mode acknowledgement.
          await client.request('POST', path + '/mode', { mode: 'automation' }, signal);
          break;
        } catch (error) {
          // Resume can register with its saved automation setting before the
          // broker's human-mode handshake completes. Only this pre-queue refusal
          // is safe to retry; ambiguous mode requests are never repeated.
          if (error.status !== 409 || error.message !== 'Native human control is awaiting acknowledgement; inspect the native terminal') throw error;
          await wait(pollMs, undefined, { signal });
          if (!await ready()) throw new Error('Runtime registration changed');
        }
      }
      if (!await ready() || current.mode !== 'automation') throw new Error('Automation not confirmed');
      const result = { sessionId: id, status: 'restored', mode: current.mode };
      results.push(result);
      report(result);
    } catch (error) {
      // Error bodies, lastError, blockedReason and provider output are private.
      const result = { sessionId: id, status: 'failed', stage,
        reason: signal.aborted ? 'timeout' : 'unconfirmed',
        ...(Number.isInteger(error.status) && error.status >= 400 && error.status <= 599 ? { httpStatus: error.status } : {}) };
      results.push(result);
      report(result);
    }
  }
  return { sessions: refreshed, results };
}

function reportRestoration(result) {
  let detail;
  if (result.status === 'restored') detail = 'ready for automation';
  else if (result.status === 'skipped') detail = result.reason === 'already_attempted'
    ? 'already attempted in this broker instance; automatic retry suppressed' : 'no longer eligible; live and pending sessions were not changed';
  else detail = result.stage + ' ' + result.reason + (result.httpStatus ? ' (HTTP ' + result.httpStatus + ')' : '') + '; inspect the native terminal and private broker log; automatic retry suppressed';
  process.stderr.write('Native session ' + result.sessionId + ': ' + detail + '. No task was replayed.\n');
}

/** Start one workspace job; retained dispatch state prevents automatic resubmission. */
export async function startWorkspaceJob({
  env = process.env,
  root = installedRoot,
  authFile = '/run/secrets/claude-auth.json',
  identifyInstance = containerInstance,
  report = reportRestoration,
  run = async (command, args, { input, ...options }) => {
    try {
      const pending = execute(command, args, { ...options, timeout: 120000, maxBuffer: 1024 * 1024 });
      // Native login receives credentials only through stdin, never argv or env.
      pending.child.stdin.on('error', () => {});
      pending.child.stdin.end(input);
      return await pending;
    } catch { throw new Error('Workspace setup command failed. Inspect native job state before retrying; nothing was resubmitted.'); }
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
  const harness = env.CODER_NATIVE_JOB_HARNESS ?? 'claude';
  if (!['claude', 'codex', 'opencode'].includes(harness)) throw new Error('Choose the claude, codex, or opencode harness.');
  const model = harness === 'claude' ? env.CODER_NATIVE_JOB_MODEL || 'haiku' : undefined;
  if (model && !['haiku', 'sonnet', 'opus'].includes(model)) throw new Error('Choose Haiku, Sonnet, or Opus.');
  const state = join(home, '.local/state/coder-acp');
  const codexHome = join(home, '.codex');
  await privateDirectory(state);
  await privateDirectory(codexHome);
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
  const commandEnv = { ...env, CODEX_HOME: codexHome, GIT_TERMINAL_PROMPT: '0',
    XDG_CONFIG_HOME: join(home, '.config'), XDG_DATA_HOME: join(home, '.local/share'),
    XDG_STATE_HOME: join(home, '.local/state'), XDG_CACHE_HOME: join(home, '.cache'),
  };
  // Native children use saved authentication, not the bootstrap key or job prompt.
  delete commandEnv.CODER_NATIVE_JOB_PROMPT_BASE64;
  delete commandEnv.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64;
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
      if (harness === 'codex') {
        try {
          const key = decodeParameter(env.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64);
          if (key && /\s/.test(key)) throw new Error('Invalid key.');
          const nativeAuth = join(codexHome, 'auth.json');
          if (await exists(nativeAuth)) {
            // Never replace existing native auth, including invalid/expired state.
            await privateJSON(nativeAuth);
            await invoke('codex', ['login', 'status']);
          } else if (key) {
            await run('codex', ['login', '--with-api-key'], { cwd: home, env: commandEnv, input: key + '\n' });
            await privateJSON(nativeAuth);
          } else {
            await invoke('codex', ['login', 'status']);
          }
        } catch {
          throw new Error('Codex authentication is unavailable. Supply the sensitive codex_api_key template variable or provision native login in the private persistent CODEX_HOME. Existing credentials are not overwritten; inspect their ownership and permissions.');
        }
      } else if (harness === 'claude') {
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
      }
      if (!await exists(project)) {
        if (repository) await invoke('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'protocol.file.allow=never', 'clone', '--depth', '1', '--', repository, project]);
        else await mkdir(project, { mode: 0o700 });
      } else if (repository && !await exists(join(project, '.git'))) {
        throw new Error('Project directory already exists without a Git checkout. Inspect it before retrying.');
      }
      if (harness === 'claude') {
        const configFile = join(home, '.claude.json');
        if (!await exists(configFile)) {
          await createJSON(configFile, { hasCompletedOnboarding: true, projects: { [project]: { hasTrustDialogAccepted: true } } });
        } else {
          const config = await privateJSON(configFile);
          if (!config.projects?.[project]?.hasTrustDialogAccepted) throw new Error('Existing Claude configuration has not trusted this project. Open Claude in the project and confirm trust.');
        }
      }
      if (harness === 'opencode') {
        const { openCodeEnvironment, verifyOpenCodeVersion } = await import(join(root, 'adapter/src/opencode-server.mjs'));
        await verifyOpenCodeVersion({ env: commandEnv, runVersion: run });
        try {
          const nativeEnv = openCodeEnvironment(join(state, 'bridge.json'), commandEnv);
          for (const key of ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME', 'TMPDIR']) {
            await privateDirectory(nativeEnv[key]);
          }
          // Use the dedicated server's exact home and configuration without
          // discovering a managed service. Native files must remain private.
          let pending;
          const mask = process.umask(0o077);
          try { pending = run('opencode', ['api', '--standalone', 'get', '/api/model/default'], { cwd: project, env: nativeEnv }); }
          finally { process.umask(mask); }
          const { stdout } = await pending;
          const { data } = JSON.parse(stdout);
          if (!data?.enabled || typeof data.id !== 'string' || typeof data.providerID !== 'string') throw new Error('No native default model.');
        } catch {
          // Native model settings may contain credentials. Never print the body.
          throw new Error('OpenCode has no available native default model or its configuration could not be loaded. Provision a provider and model in the private persistent ' + join(state, 'opencode-home/config/opencode/opencode.jsonc') + '. For an existing running session, select Open and use /connect and /models on its dedicated server. Ordinary-home OpenCode configuration is not used. Existing settings are not overwritten. No initial task was submitted.');
        }
      }
      await invoke('/bin/sh', [join(root, 'adapter/install.sh')]);
    }
    const broker = await ensureBroker({ state, instance, initial,
      // Explicitly enable all adapters and their dedicated native servers.
      launch: () => invoke(launcher, ['serve', '--detach', '--codex', '--opencode', ...(model ? ['--model', model] : [])]),
    });
    const { createClient } = await import(join(root, 'adapter/src/client.mjs'));
    const { sessions } = await restoreSessions({ state, instance, ...broker, client: createClient(broker.descriptor), report });
    // run checks its durable guard before connecting, including after a reboot.
    try {
      const result = await invoke(launcher, ['run', '--detach', ...(harness !== 'claude' ? ['--harness', harness] : []), '--cwd', project, '--prompt-file', promptFile, '--job-file', jobFile]);
      return result.stdout;
    } catch (error) {
      const job = await privateJSON(jobFile).catch(() => undefined);
      const inputHash = createHash('sha256').update(JSON.stringify([project, prompt])).digest('hex');
      const sameHarness = (job?.harness === undefined ? 'claude' : job.harness) === harness;
      if (!initial && sameHarness && job?.version === 1 && job.inputHash === inputHash && job.status === 'unknown') {
        return 'The broker is available. The retained initial job has an unknown outcome; inspect native history and tool side effects before starting new work. The initial task was not replayed.\n';
      }
      if (!sameHarness || job?.version !== 1 || job.status !== 'blocked' || job.errorCode !== 'native_input_required' ||
          job.inputHash !== inputHash || job.dispatchAttemptedAt !== undefined ||
          !isSessionID(job.sessionId)) throw error;
      if (!initial && !sessions.some(session => session.sessionId === job.sessionId && !session.archived && !session.resumePending && !session.registrationPending && !session.ended && !session.stale && !session.fault)) {
        return 'Retained blocked job for native session ' + job.sessionId + '. The initial task was not submitted. ' +
          'The broker is available, but the old native session was not verified live. Inspect native sessions before explicitly starting new work. The initial task was not replayed.\n';
      }
      if (!initial && sessions.some(session => session.sessionId === job.sessionId && session.mode === 'automation')) {
        return 'Native session ' + job.sessionId + ' is ready for automation. The retained initial task was not submitted or replayed. Submit new work explicitly.\n';
      }
      return 'Native session ' + job.sessionId + ' needs attention. Select Open on its session card to continue native setup. ' +
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

#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { constants } from 'node:fs';
import { lstat, open, readFile, readdir, readlink } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';

const failed = 'Native Claude shutdown could not be verified. Inspect native sessions before resuming; saved state was not repaired.';
const timedOut = 'Native Claude shutdown timed out. Saved state was not repaired; inspect native sessions before resuming.';
const object = value => value && typeof value === 'object' && !Array.isArray(value);

async function optional(operation) {
  try { return await operation(); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
}

async function record(path) {
  return optional(async () => {
    const handle = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    try {
      const info = await handle.stat();
      // Native lock and session files can be 644 beneath the private config root.
      if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o022) || info.size > 1024 * 1024) throw new Error(failed);
      const value = JSON.parse(await handle.readFile('utf8'));
      if (!object(value)) throw new Error(failed);
      return value;
    } finally { await handle.close(); }
  });
}

async function pidDomain() {
  const machine = (await optional(() => readFile('/etc/machine-id', 'utf8')) ?? '').trim();
  return 'linux:' + machine + ':' + await readlink('/proc/self/ns/pid');
}

function runStop(command, args, { timeout, ...options }) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { ...options, stdio: 'ignore' });
    const timer = setTimeout(() => {
      // Terminate only our CLI request, never a saved native PID. Do not wait
      // indefinitely for a stuck command or escalate to SIGKILL.
      child.kill('SIGTERM');
      child.unref();
      reject(new Error(timedOut));
    }, timeout);
    child.once('error', () => { clearTimeout(timer); reject(new Error(failed)); });
    child.once('exit', code => {
      clearTimeout(timer);
      if (code === 0) resolve();
      else reject(new Error(failed));
    });
  });
}

/** Stop the template's native daemon and verify its asynchronous cleanup. */
export async function stopWorkspaceClaude({
  env = process.env,
  run = runStop,
  probeProcess = pid => process.kill(pid, 0),
  identifyDomain = pidDomain,
  now = () => performance.now(),
  wait = delay,
  timeout = 45000,
} = {}) {
  const deadline = now() + timeout;
  try {
    if (!env.HOME?.startsWith('/') || (env.CLAUDE_CONFIG_DIR && env.CLAUDE_CONFIG_DIR !== join(env.HOME, '.claude'))) throw new Error(failed);
    const root = join(env.HOME, '.claude');
    const domain = await identifyDomain();
    const pids = new Set();
    const capture = pid => {
      if (!Number.isSafeInteger(pid) || pid <= 1 || pid > 2147483647) throw new Error(failed);
      pids.add(pid);
    };
    const snapshot = async () => {
      for (const directory of [root, join(root, 'daemon'), join(root, 'sessions')]) {
        const info = await optional(() => lstat(directory));
        if (info && (!info.isDirectory() || info.uid !== process.getuid() || (info.mode & 0o022))) throw new Error(failed);
      }
      const lock = await record(join(root, 'daemon.lock'));
      if (lock) {
        if (lock.version !== '2.1.309') throw new Error(failed);
        capture(lock.pid);
      }
      const roster = await record(join(root, 'daemon/roster.json'));
      let workers = [];
      if (roster) {
        if (roster.proto !== 1 || !object(roster.workers)) throw new Error(failed);
        capture(roster.supervisorPid);
        workers = Object.values(roster.workers);
        if (workers.length > 256) throw new Error(failed);
        for (const worker of workers) {
          if (!object(worker) || worker.cliVersion !== '2.1.309') throw new Error(failed);
          capture(worker.pid);
          if (Object.hasOwn(worker, 'replPid')) capture(worker.replPid);
        }
      }
      const names = await optional(() => readdir(join(root, 'sessions'))) ?? [];
      if (names.length > 256) throw new Error(failed);
      let sessions = 0;
      for (const name of names.filter(name => name.endsWith('.json'))) {
        const session = await record(join(root, 'sessions', name));
        if (!session) continue; // Native cleanup may remove it during the scan.
        if (session.version !== '2.1.309' || name !== session.pid + '.json' || session.pidDomain !== domain) throw new Error(failed);
        capture(session.pid);
        sessions++;
      }
      return !lock && workers.length === 0 && sessions === 0;
    };
    await snapshot();
    if (pids.size === 0) return;
    const commandEnv = { ...env };
    delete commandEnv.CODER_NATIVE_JOB_PROMPT_BASE64;
    delete commandEnv.CODER_NATIVE_JOB_CODEX_API_KEY_BASE64;
    // 2.1.309 acknowledges before exit. Stay below its 5s worker and 10s
    // supervisor CLI fallback escalation, then only observe native cleanup.
    await run('claude', ['daemon', 'stop', '--any'], { cwd: env.HOME, env: commandEnv, timeout: Math.max(1, Math.min(4000, deadline - now())) });
    while (now() < deadline) {
      const clean = await snapshot();
      let alive = false;
      for (const pid of pids) {
        try { await probeProcess(pid); alive = true; }
        catch (error) { if (error.code !== 'ESRCH') throw error; }
      }
      if (clean && !alive) return;
      await wait(Math.min(200, Math.max(0, deadline - now())));
    }
    throw new Error(timedOut);
  } catch (error) {
    // Native diagnostics and registry contents can contain credentials or prompts.
    throw new Error(error.message === timedOut ? timedOut : failed);
  }
}

async function stopWorkspaceOpenCode({ env, root, signal }) {
  if (!env.HOME?.startsWith('/')) throw new Error('Invalid workspace home.');
  const stateFile = join(env.HOME, '.local/state/coder-acp/bridge.json');
  // Missing native state is a no-op, even if bootstrap never installed the adapter.
  if (!await optional(() => lstat(join(env.HOME, '.local/state/coder-acp/opencode-server.json')))) return;
  const { shutdownOpenCodeServer } = await import(pathToFileURL(join(root, 'adapter/src/opencode-server.mjs')).href);
  await shutdownOpenCodeServer({ stateFile, signal });
}

/** Stop independent native servers within the template's shared shutdown budget. */
export async function stopWorkspaceNative({
  env = process.env, root = '/opt/native-agent', signal,
  stopClaude = stopWorkspaceClaude, stopOpenCode = stopWorkspaceOpenCode,
} = {}) {
  // Claude may take 45s and OpenCode 20s. Both must finish within the 60s hook.
  const [claude, opencode] = await Promise.allSettled([
    Promise.resolve().then(() => stopClaude({ env })),
    Promise.resolve().then(() => stopOpenCode({ env, root, signal })),
  ]);
  const errors = [];
  if (claude.status === 'rejected') errors.push(claude.reason?.message === timedOut ? timedOut : failed);
  if (opencode.status === 'rejected') errors.push('Native OpenCode shutdown could not be verified. Saved state was not repaired.');
  if (errors.length) throw new Error(errors.join(' '));
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await stopWorkspaceNative().then(
    () => console.log('Native Claude and OpenCode shutdown verified.'),
    error => { console.error(error.message); process.exitCode = 1; },
  );
}

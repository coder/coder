import { execFile } from 'node:child_process';
import { constants } from 'node:fs';
import { lstat, open } from 'node:fs/promises';
import { homedir } from 'node:os';
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { promisify } from 'node:util';
import { createNativeDiscovery } from './native-discovery.mjs';

const execute = promisify(execFile);
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const MAX_STATE_BYTES = 256 * 1024;
const MAX_TRANSCRIPT_PROBE_BYTES = 256 * 1024;
const validPath = value => typeof value === 'string' && value.length <= 4096 && isAbsolute(value) && resolve(value) === value && !/[\x00-\x1f\x7f]/.test(value);
const REFUSED = 'Cannot safely resume the saved native session. Inspect claude agents and its saved state in the native terminal; the original task was not rerun.';
const TRANSCRIPT_REFUSED = 'Native saved conversation is missing or cannot be verified. Restore its private transcript before joining; the original task was not rerun.';
const QUEUE_REFUSED = 'Native session has queued work. Inspect and resolve it in the native terminal before joining; nothing was resumed.';
const WORKER_REFUSED = 'Native worker is still running. Inspect claude agents in the native terminal; an archived bridge session cannot safely rejoin the previous runtime. Nothing was resumed.';
const refusal = message => Object.assign(new Error(message), { resumeRefused: true });

async function directory(path, privateRoot = false) {
  const info = await lstat(path);
  if (!info.isDirectory() || info.uid !== process.getuid() || (info.mode & (privateRoot ? 0o077 : 0o022)) !== 0) throw refusal(REFUSED);
}

async function ancestors(path) {
  for (let current = path; ; current = dirname(current)) {
    if (!(await lstat(current)).isDirectory()) throw refusal(REFUSED);
    if (dirname(current) === current) return;
  }
}

async function privateFile(path) {
  const handle = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) !== 0 || info.nlink !== 1) throw refusal(REFUSED);
    return { handle, info };
  } catch (error) {
    await handle.close();
    throw error;
  }
}

async function readState(path) {
  const { handle, info } = await privateFile(path);
  try {
    if (info.size > MAX_STATE_BYTES) throw refusal(REFUSED);
    const buffer = Buffer.alloc(MAX_STATE_BYTES + 1);
    let size = 0;
    while (size < buffer.length) {
      const { bytesRead } = await handle.read(buffer, size, buffer.length - size, null);
      if (!bytesRead) break;
      size += bytesRead;
    }
    if (size > MAX_STATE_BYTES) throw refusal(REFUSED);
    const text = buffer.subarray(0, size).toString('utf8');
    const state = JSON.parse(text);
    if (!state || typeof state !== 'object' || Array.isArray(state)) throw refusal(REFUSED);
    return { state, text };
  } finally { await handle.close(); }
}

async function transcript(state, sessionId, cwd, projectsDirectory) {
  // Claude 2.1.309 probes linkScanPath first, then projects/<encoded-cwd>/<UUID>.jsonl.
  // Long project names use a native hash; without an explicit saved path, fail closed.
  const project = cwd.replace(/[^a-zA-Z0-9]/g, '-');
  const file = state.linkScanPath ?? (project.length <= 200 ? join(projectsDirectory, project, sessionId + '.jsonl') : undefined);
  if (!validPath(file) || basename(file) !== sessionId + '.jsonl') throw refusal(TRANSCRIPT_REFUSED);
  const parts = relative(projectsDirectory, file).split(sep);
  if (parts.length !== 2 || parts[0] === '..' || !parts[0]) throw refusal(TRANSCRIPT_REFUSED);
  await directory(projectsDirectory);
  await directory(dirname(file));
  const { handle, info } = await privateFile(file);
  try {
    if (info.size === 0) throw refusal(TRANSCRIPT_REFUSED);
    // A metadata-only JSONL is not resumable: native Claude would replay its intent.
    // Prove an actual message exists with a bounded prefix read, never a full transcript read.
    const buffer = Buffer.alloc(Math.min(info.size, MAX_TRANSCRIPT_PROBE_BYTES));
    let size = 0;
    while (size < buffer.length) {
      const { bytesRead } = await handle.read(buffer, size, buffer.length - size, size);
      if (!bytesRead) break;
      size += bytesRead;
    }
    const lines = buffer.subarray(0, size).toString('utf8').split('\n');
    if (size < info.size) lines.pop();
    const hasMessage = lines.some(line => {
      // Match the native probe's compact type marker as well as structured identity.
      if (!line.includes('"type":"user"') && !line.includes('"type":"assistant"')) return false;
      try {
        const value = JSON.parse(line);
        return value.sessionId === sessionId && ['user', 'assistant'].includes(value.type) && value.message && typeof value.message === 'object';
      } catch { return false; }
    });
    if (!hasMessage) throw refusal(TRANSCRIPT_REFUSED);
  } finally { await handle.close(); }
}

/** Resume a verified bridge-owned conversation without new flags, prompts, or task replay. */
export function createNativeResume({ stateFile, pluginPath, jobsDirectory = join(homedir(), '.claude', 'jobs'), projectsDirectory = join(dirname(jobsDirectory), 'projects'), run = execute }) {
  if (![stateFile, pluginPath, jobsDirectory, projectsDirectory].every(validPath) || dirname(projectsDirectory) !== dirname(jobsDirectory)) throw refusal('Native resume requires absolute paths under one native config root');
  return async (sessionId, cwd) => {
    if (typeof sessionId !== 'string' || !UUID.test(sessionId) || !validPath(cwd)) throw refusal(REFUSED);
    const jobDirectory = join(jobsDirectory, sessionId.slice(0, 8));
    const statePath = join(jobDirectory, 'state.json');
    let saved;
    try {
      await ancestors(dirname(jobsDirectory));
      await directory(dirname(jobsDirectory), true);
      await directory(jobsDirectory);
      await directory(jobDirectory);
      saved = await readState(statePath);
      // A completed turn can still have a live worker. Native --bg --resume would
      // clone it, so retain the structured pid even though discovery marks it ended.
      let workerPresent = false;
      const discover = createNativeDiscovery({ stateFile, pluginPath, jobsDirectory, run: async (...args) => {
        const output = await run(...args);
        if (typeof output.stdout === 'string' && Buffer.byteLength(output.stdout) <= 1024 * 1024) {
          try {
            const agents = JSON.parse(output.stdout);
            workerPresent = Array.isArray(agents) && agents.some(agent => agent?.sessionId === sessionId && Number.isSafeInteger(agent.pid) && agent.pid > 0);
          } catch { /* Discovery rejects malformed structured output without leaking it. */ }
        }
        return output;
      } });
      // Reuse discovery's strict settings/plugin ownership check.
      const native = (await discover()).find(item => item.sessionId === sessionId && item.cwd === cwd);
      const { state } = saved;
      if (!native || state.sessionId !== sessionId || state.cwd !== cwd ||
          (state.resumeSessionId !== undefined && state.resumeSessionId !== sessionId) ||
          (state.daemonShort !== undefined && state.daemonShort !== sessionId.slice(0, 8)) || state.sessionIdTaken) throw refusal(REFUSED);
      if ((await readState(statePath)).text !== saved.text) throw refusal(REFUSED);
      if (!native.ended || workerPresent) throw refusal(WORKER_REFUSED);
      // Mit()/sie() in 2.1.309 persist pending replies as state.json.queuedPrompt.
      // Even an empty or slash-command queue is refused rather than interpreting it.
      if (Object.hasOwn(state, 'queuedPrompt') || (state.inFlight?.queued ?? 0) !== 0) throw refusal(QUEUE_REFUSED);
      if (state.template !== 'bg' || state.routine || state.respawnFlags.some(flag => /^--(?:reply-on-resume|agent|routine)(?:=|$)/.test(flag))) throw refusal(REFUSED);
      try { await transcript(state, sessionId, cwd, projectsDirectory); }
      catch { throw refusal(TRANSCRIPT_REFUSED); }
      // Do not knowingly dispatch after a native writer changed the checked queue or ownership.
      if ((await readState(statePath)).text !== saved.text) throw refusal(REFUSED);
    } catch (error) {
      if ([QUEUE_REFUSED, TRANSCRIPT_REFUSED, WORKER_REFUSED].includes(error?.message)) throw refusal(error.message);
      throw refusal(REFUSED);
    }
    try {
      // Additional --settings/--plugin-dir flags make native Claude clone an existing job.
      // Ignore all native output: it can contain prompts, settings, or provider diagnostics.
      await run('claude', ['--bg', '--resume', sessionId], { cwd, timeout: 20000, killSignal: 'SIGKILL', maxBuffer: 1024 * 1024, encoding: 'utf8' });
    } catch {
      throw new Error('Native Claude resume did not complete successfully. Inspect claude agents before retrying; the native session may already be running.');
    }
  };
}

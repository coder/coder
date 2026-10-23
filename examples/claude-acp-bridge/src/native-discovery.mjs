import { execFile } from 'node:child_process';
import { constants } from 'node:fs';
import { lstat, open } from 'node:fs/promises';
import { homedir } from 'node:os';
import { dirname, isAbsolute, join } from 'node:path';
import { promisify } from 'node:util';

const execute = promisify(execFile);
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const MAX_STATE_BYTES = 256 * 1024;
const MAX_LIST_BYTES = 1024 * 1024;
const MAX_AGENTS = 256;
const ENDED = new Set(['ended', 'stopped', 'exited', 'completed', 'done', 'failed']);
const object = value => value && typeof value === 'object' && !Array.isArray(value);
const path = value => typeof value === 'string' && value.length <= 4096 && isAbsolute(value) && !/[\x00-\x1f\x7f]/.test(value);

/** Only these summaries, never native detail, needs, settings or providerEnv, leave discovery. */
export const NATIVE_BLOCKED_REASON = 'Native Claude needs input in the native terminal.';
export const MCP_BLOCKED_REASON = 'Approve project MCP servers in the native terminal.';
export const REGISTRATION_PENDING_REASON = 'Native session registration is pending; join the native terminal to complete setup.';

function blockedReason(state, status) {
  if (state.state !== 'blocked') return undefined;
  if (typeof state.detail === 'string' && /^(?:\d+ new )?MCP servers? needs? approval[.!]?$/i.test(state.detail)) return MCP_BLOCKED_REASON;
  // The CLI machine status distinguishes idle greetings from actual waiting; setup approval takes precedence.
  return status === 'idle' ? undefined : NATIVE_BLOCKED_REASON;
}

function ownership(flags, stateFile, pluginPath) {
  if (!Array.isArray(flags) || flags.length > 256 || flags.some(flag => typeof flag !== 'string' || /^--(?:settings|plugin-dir)=/.test(flag))) return;
  // Reject ambiguous duplicate settings or plugin flags instead of guessing CLI precedence.
  const settings = flags.flatMap((flag, index) => flag === '--settings' ? [flags[index + 1]] : []);
  const plugins = flags.flatMap((flag, index) => flag === '--plugin-dir' ? [flags[index + 1]] : []);
  if (settings.length !== 1 || plugins.length !== 1 || plugins[0] !== pluginPath) return;
  let config;
  try { config = JSON.parse(settings[0]); } catch { return; }
  const env = config?.env;
  if (!object(env) || env.WORKSPACE_BRIDGE_FILE !== stateFile || typeof env.WORKSPACE_BRIDGE_LAUNCH_ID !== 'string' || !UUID.test(env.WORKSPACE_BRIDGE_LAUNCH_ID)) return;
  return env.WORKSPACE_BRIDGE_LAUNCH_ID;
}

async function nativeDirectory(directory, privateRoot = false) {
  const info = await lstat(directory);
  // Claude creates jobs directories as 755 beneath its private 700 config root.
  if (!info.isDirectory() || info.uid !== process.getuid() || (info.mode & (privateRoot ? 0o077 : 0o022)) !== 0) throw new Error('Unsafe native state directory');
}

async function readState(file) {
  const handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) !== 0 || info.size > MAX_STATE_BYTES) return;
    // readFile can grow without bound if the file is concurrently extended.
    const buffer = Buffer.alloc(MAX_STATE_BYTES + 1);
    let size = 0;
    while (size < buffer.length) {
      const { bytesRead } = await handle.read(buffer, size, buffer.length - size, null);
      if (!bytesRead) break;
      size += bytesRead;
    }
    if (size > MAX_STATE_BYTES) return;
    return JSON.parse(buffer.subarray(0, size).toString('utf8'));
  } finally { await handle.close(); }
}

/**
 * Discover only bridge-owned native background sessions from bounded structured state.
 * The broker owns caching. Missing state is normal; CLI failures have no raw diagnostics.
 */
export function createNativeDiscovery({ stateFile, pluginPath, jobsDirectory = join(homedir(), '.claude', 'jobs'), run = execute }) {
  if (!path(stateFile) || !path(pluginPath) || !path(jobsDirectory)) throw new Error('Native discovery requires absolute paths');
  return async () => {
    try {
      // A private config root and non-writable descendants prevent another user
      // replacing a checked parent. No native path component may be a symlink.
      await nativeDirectory(dirname(jobsDirectory), true);
      await nativeDirectory(jobsDirectory);
    } catch { return []; }
    let agents;
    try {
      const { stdout } = await run('claude', ['agents', '--json', '--all'], { timeout: 5000, maxBuffer: MAX_LIST_BYTES, encoding: 'utf8' });
      if (typeof stdout !== 'string' || Buffer.byteLength(stdout) > MAX_LIST_BYTES) throw new Error();
      agents = JSON.parse(stdout);
      if (!Array.isArray(agents) || agents.length > MAX_AGENTS) throw new Error();
    } catch {
      throw new Error('Native session discovery unavailable; inspect claude agents in the native terminal.');
    }
    const sessions = [];
    const seen = new Set();
    for (const agent of agents) {
      if (!object(agent) || agent.kind !== 'background' || typeof agent.id !== 'string' || !/^[0-9a-f]{8}$/i.test(agent.id) ||
          typeof agent.sessionId !== 'string' || !UUID.test(agent.sessionId) || agent.id !== agent.sessionId.slice(0, 8) ||
          !path(agent.cwd) || !Number.isSafeInteger(agent.startedAt) || agent.startedAt < 0 || agent.startedAt > 8640000000000000 || seen.has(agent.sessionId)) continue;
      try {
        const directory = join(jobsDirectory, agent.id);
        await nativeDirectory(directory);
        const state = await readState(join(directory, 'state.json'));
        if (!object(state) || state.sessionId !== agent.sessionId || state.cwd !== agent.cwd || typeof state.state !== 'string') continue;
        const launchId = ownership(state.respawnFlags, stateFile, pluginPath);
        if (!launchId) continue;
        seen.add(agent.sessionId);
        // The CLI can report a reaped process as failed before state.json changes.
        const ended = ENDED.has(agent.state) || ENDED.has(state.state);
        sessions.push({ sessionId: agent.sessionId, cwd: agent.cwd, startedAt: agent.startedAt, launchId,
          ended, blockedReason: ended ? undefined : blockedReason(state, agent.status) });
      } catch { /* Missing, unsafe or partially written state is not proof of termination. */ }
    }
    return sessions;
  };
}

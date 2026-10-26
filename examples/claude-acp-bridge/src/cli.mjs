#!/usr/bin/env node
import { execFile, spawn } from 'node:child_process';
import { readFile, unlink, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs, promisify } from 'node:util';
import { createBroker } from './broker.mjs';
import { createNativeDiscovery } from './native-discovery.mjs';
import { createNativeResume } from './native-resume.mjs';
import { connect } from './client.mjs';
import { privateDirectory, startDetached } from './service.mjs';
import { runJob, startDetachedJob } from './job.mjs';
import { HARNESS_INFO, nativeIdentity } from './harness.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const { values, positionals } = parseArgs({
  allowPositionals: true,
  options: { state: { type: 'string', default: resolve(root, '.runtime/bridge.json') }, cwd: { type: 'string' }, model: { type: 'string' }, 'prompt-file': { type: 'string' }, 'job-file': { type: 'string' }, detach: { type: 'boolean' }, codex: { type: 'boolean' }, opencode: { type: 'boolean' }, harness: { type: 'string' }, help: { type: 'boolean' } },
});
const [command, id, ...rest] = positionals;
const stateFile = resolve(values.state);
const execute = promisify(execFile);

async function main() {
  if (values.help || !command) {
    console.log('Usage: coder-acp <serve|acp|start|run|list|inspect ID|control ID human|automation|attach ID|stop ID> [--state FILE] [--cwd DIR] [--model MODEL]\nserve --codex enables Codex 0.164.0 alongside Claude and starts its dedicated native app-server.\nserve --opencode enables OpenCode V2 alongside the other harnesses and starts its dedicated native server.\nstart/run --harness claude|codex|opencode selects the native harness (default claude).\nrun --cwd /abs/project --prompt-file /private/prompt.txt --job-file /private/job.json [--detach]\nrun waits for completion; run --detach waits only for dispatch attempted, not native acceptance, then observes completion in a background worker. Matching job files return immediately without replay. Exit 0 requires completion for run, or recorded dispatch with prompting/completed status for run --detach.\nserve runs in the foreground; serve --detach explicitly starts a background broker with private logs.\nClients never start or restart the broker.');
    return;
  }
  if (values.detach && !['serve', 'run'].includes(command)) throw new Error('--detach is only valid with serve or run');
  if ((values.codex || values.opencode) && command !== 'serve') throw new Error('--codex and --opencode are only valid with serve');
  if (values.harness !== undefined && (!['start', 'run'].includes(command) || !Object.hasOwn(HARNESS_INFO, values.harness))) throw new Error('--harness claude|codex|opencode is only valid with start or run');
  if (command === 'run') {
    if (id || values.model) throw new Error('run accepts --cwd, --prompt-file, --job-file, --state and --detach only');
    const controller = new AbortController();
    const stop = () => controller.abort();
    if (!values.detach) {
      process.once('SIGINT', stop);
      process.once('SIGTERM', stop);
    }
    try {
      const record = await (values.detach ? startDetachedJob : runJob)({
        cwd: values.cwd, promptFile: values['prompt-file'], jobFile: values['job-file'], stateFile, signal: controller.signal,
        ...(values.harness ? { harness: values.harness } : {}),
      });
      console.log(JSON.stringify(record, null, 2));
      const reachedMilestone = values.detach
        ? ['prompting', 'completed'].includes(record.status) && Boolean(record.sessionId && record.dispatchAttemptedAt)
        : record.status === 'completed';
      if (!reachedMilestone) process.exitCode = 1;
    } finally {
      process.off('SIGINT', stop);
      process.off('SIGTERM', stop);
    }
    return;
  }
  if (values['prompt-file'] || values['job-file']) throw new Error('--prompt-file and --job-file are only valid with run');
  if (command === 'serve') {
    if (values.detach) {
      const { pid, logFile } = await startDetached(fileURLToPath(import.meta.url), stateFile, values.model, { codex: values.codex, opencode: values.opencode });
      console.log('Bridge broker started (PID ' + pid + '). Private log: ' + logFile);
      return;
    }
    await privateDirectory(dirname(stateFile));
    const pluginPath = resolve(root, 'plugin');
    let codex;
    let ensureCodexServer;
    if (values.codex) {
      const { createCodexService } = await import('./codex.mjs');
      const native = await import('./codex-server.mjs');
      ensureCodexServer = native.ensureCodexServer;
      codex = await createCodexService({
        sessionFile: stateFile + '.codex-sessions.json',
        rpcFactory: native.createCodexRPCFactory(stateFile),
      });
    }
    let opencode;
    let ensureOpenCodeServer;
    if (values.opencode) {
      const { createOpenCodeService } = await import('./opencode-service.mjs');
      const { createOpenCodeClient } = await import('./opencode-rpc.mjs');
      const native = await import('./opencode-server.mjs');
      ensureOpenCodeServer = native.ensureOpenCodeServer;
      opencode = await createOpenCodeService({
        sessionFile: stateFile + '.opencode-sessions.json',
        clientFactory: async callbacks => createOpenCodeClient({
          ...await native.connectOpenCodeServer({ stateFile }), ...callbacks,
        }),
      });
    }
    const broker = await createBroker({
      codex, opencode,
      sessionFile: stateFile + '.sessions.json',
      deferPersistence: true,
      discover: createNativeDiscovery({ stateFile, pluginPath }),
      resume: createNativeResume({ stateFile, pluginPath }),
      launch: async (cwd, launchId) => {
        // The native supervisor does not inherit arbitrary caller env vars.
        // Explicit settings are carried into the background session instead.
        const env = { WORKSPACE_BRIDGE_FILE: stateFile, WORKSPACE_BRIDGE_LAUNCH_ID: launchId, WORKSPACE_BRIDGE_MODE: 'automation', DISABLE_AUTOUPDATER: '1' };
        const args = ['--bg', '--name', 'workspace-bridge-' + launchId.slice(0, 8), '--plugin-dir', pluginPath, '--settings', JSON.stringify({ env })];
        if (values.model) args.push('--model', values.model);
        args.push('/bridge-status');
        try {
          await execute('claude', args, {
            cwd,
            env: { ...process.env, WORKSPACE_BRIDGE_FILE: stateFile, WORKSPACE_BRIDGE_LAUNCH_ID: launchId, WORKSPACE_BRIDGE_MODE: 'automation' },
            timeout: 30000,
            maxBuffer: 1024 * 1024,
          });
        } catch {
          throw new Error('Native Claude launch failed; inspect claude agents and verify login and workspace trust in the native terminal before retrying.');
        }
      },
    });
    try { await writeFile(stateFile, JSON.stringify({ url: broker.url, token: broker.token, pid: process.pid }) + '\n', { mode: 0o600, flag: 'wx' }); }
    catch (error) { await broker.close(); throw new Error('Cannot create descriptor ' + stateFile + ' (' + error.code + '). An existing descriptor may be running or stale; inspect before removing it. Nothing was restarted.'); }
    const startup = new AbortController();
    let stopping;
    function stop() {
      if (stopping) return stopping;
      startup.abort();
      stopping = (async () => {
        try { await broker.close(); }
        finally {
          let current;
          try { current = JSON.parse(await readFile(stateFile, 'utf8')); }
          catch { /* Leave an unreadable or replaced descriptor for manual inspection. */ }
          if (current?.token === broker.token) await unlink(stateFile);
        }
      })();
      return stopping;
    }
    process.once('SIGINT', () => stop().catch(error => console.error(error.message)));
    process.once('SIGTERM', () => stop().catch(error => console.error(error.message)));
    // Keep authenticated requests blocked until storage and native startup are
    // ready. A competing serve cannot pass the exclusive descriptor claim.
    try {
      await broker.preparePersistence(async () => {
        startup.signal.throwIfAborted();
        if (values.codex) await ensureCodexServer({ stateFile, signal: startup.signal });
        startup.signal.throwIfAborted();
        if (values.opencode) await ensureOpenCodeServer({ stateFile, signal: startup.signal });
        startup.signal.throwIfAborted();
      });
    }
    catch (error) { await stop(); throw error; }
    console.error('Workspace bridge listening on ' + broker.url + '; private descriptor: ' + stateFile);
    if (process.send) process.send({ type: 'bridge-ready' }, () => {});
    return;
  }
  if (!['acp', 'start', 'list', 'inspect', 'control', 'attach', 'stop'].includes(command)) {
    throw new Error('Unknown command; run coder-acp --help');
  }
  const client = await connect(stateFile);
  if (command === 'acp') {
    // Fail before opening ACP when a descriptor refers to an offline broker.
    await client.request('GET', '/v1/sessions');
    const { serveACP } = await import('./acp.mjs');
    await serveACP(client);
  } else if (command === 'start') {
    if (values.harness && values.harness !== 'claude') {
      const capabilities = await client.request('GET', '/v1/capabilities');
      if (!capabilities.harnesses?.some(harness => harness.id === values.harness)) throw new Error('Broker does not advertise ' + HARNESS_INFO[values.harness].title + ' support; no session was created');
    }
    console.log(JSON.stringify(await client.request('POST', '/v1/sessions', { cwd: resolve(values.cwd || process.cwd()), ...(values.harness ? { harness: values.harness } : {}) }), null, 2));
  } else if (command === 'list') {
    console.log(JSON.stringify(await client.request('GET', '/v1/sessions'), null, 2));
  } else {
    if (!id) throw new Error('Expected a full session ID from start or list');
    const session = await client.request('GET', '/v1/sessions/' + encodeURIComponent(id));
    if (command === 'inspect') console.log(JSON.stringify(session, null, 2));
    const path = '/v1/sessions/' + encodeURIComponent(id);
    if (command === 'attach' && (session.archived || session.resumePending)) {
      throw new Error('Native session is stopped or still restoring. Workspace startup restores saved sessions; opening a terminal never restarts them. Inspect the startup logs.');
    }
    if (command === 'control') await client.request('POST', path + '/mode', { mode: rest[0] });
    if (session.harness === 'opencode' && ['attach', 'stop'].includes(command)) {
      const identity = nativeIdentity(id);
      if (identity?.harness !== 'opencode' || identity.nativeSessionId !== session.nativeSessionId || session.sessionId !== id) throw new Error('Expected the exact full OpenCode session ID');
      if (command === 'stop') {
        await client.request('POST', path + '/stop', {});
        return;
      }
      const { connectOpenCodeServer, openCodeEnvironment, verifyOpenCodeVersion } = await import('./opencode-server.mjs');
      await verifyOpenCodeVersion();
      const endpoint = await connectOpenCodeServer({ stateFile });
      await client.request('POST', path + '/mode', { mode: 'human' });
      const { createOpenCodeClient } = await import('./opencode-rpc.mjs');
      const native = await createOpenCodeClient(endpoint);
      try {
        // --session can create a missing native session. Fail closed at preflight;
        // the native CLI has no atomic attach-only option for a later deletion.
        const { data } = await native.request('GET', '/api/session/' + identity.nativeSessionId);
        if (data?.id !== identity.nativeSessionId || data.location?.directory !== session.cwd) throw new Error('OpenCode session identity or directory changed; terminal was not opened');
      } finally { await native.close(); }
      const child = spawn('opencode', ['--server', endpoint.url, '--session', identity.nativeSessionId, session.cwd], {
        cwd: session.cwd, stdio: 'inherit', env: { ...openCodeEnvironment(stateFile), OPENCODE_PASSWORD: endpoint.password },
      });
      await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', code => code === 0 ? resolve() : reject(new Error('OpenCode terminal exited with ' + code))); });
      return;
    }
    if (session.harness === 'codex' && ['attach', 'stop'].includes(command)) {
      const nativeId = session.nativeSessionId;
      if (typeof nativeId !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(nativeId) || id !== 'codex:' + nativeId || session.sessionId !== id) throw new Error('Expected the exact full Codex session ID');
      if (command === 'stop') {
        await client.request('POST', path + '/stop', {});
        return;
      }
      await client.request('POST', path + '/mode', { mode: 'human' });
      // Connect to the same explicit native server, never the managed daemon.
      const { codexSocketPath } = await import('./codex-server.mjs');
      const child = spawn('codex', ['resume', '--remote', 'unix://' + codexSocketPath(stateFile), nativeId], { cwd: session.cwd, stdio: 'inherit' });
      await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', code => code === 0 ? resolve() : reject(new Error('Codex terminal exited with ' + code))); });
      return;
    }
    if (command === 'stop') {
      try { await execute('claude', ['stop', id.slice(0, 8)], { timeout: 30000, maxBuffer: 1024 * 1024 }); }
      catch { throw new Error('Native Claude stop failed; inspect the native terminal before retrying.'); }
    }
    if (command === 'attach') {
      if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(id) || session.sessionId !== id) throw new Error('Attach requires the exact full session ID from list');
      // Taking human control does not restart or interrupt native work.
      await client.request('POST', path + '/mode', { mode: 'human' });
      const child = spawn('claude', ['attach', id.slice(0, 8)], { stdio: 'inherit' });
      await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', code => code === 0 ? resolve() : reject(new Error('claude attach exited with ' + code))); });
    }
  }
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });

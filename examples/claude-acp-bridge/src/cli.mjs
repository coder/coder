#!/usr/bin/env node
import { execFile, spawn } from 'node:child_process';
import { readFile, unlink, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs, promisify } from 'node:util';
import { createBroker } from './broker.mjs';
import { createNativeDiscovery } from './native-discovery.mjs';
import { connect } from './client.mjs';
import { privateDirectory, startDetached } from './service.mjs';
import { runJob, startDetachedJob } from './job.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const { values, positionals } = parseArgs({
  allowPositionals: true,
  options: { state: { type: 'string', default: resolve(root, '.runtime/bridge.json') }, cwd: { type: 'string' }, model: { type: 'string' }, 'prompt-file': { type: 'string' }, 'job-file': { type: 'string' }, detach: { type: 'boolean' }, help: { type: 'boolean' } },
});
const [command, id, ...rest] = positionals;
const stateFile = resolve(values.state);
const execute = promisify(execFile);

async function main() {
  if (values.help || !command) {
    console.log('Usage: coder-acp <serve|acp|start|run|list|inspect ID|control ID human|automation|attach ID|stop ID> [--state FILE] [--cwd DIR] [--model MODEL]\nrun --cwd /abs/project --prompt-file /private/prompt.txt --job-file /private/job.json [--detach]\nrun waits for completion; run --detach waits only for dispatch attempted, not native acceptance, then observes completion in a background worker. Matching job files return immediately without replay. Exit 0 requires completion for run, or recorded dispatch with prompting/completed status for run --detach.\nserve runs in the foreground; serve --detach explicitly starts a background broker with private logs.\nClients never start or restart the broker.');
    return;
  }
  if (values.detach && !['serve', 'run'].includes(command)) throw new Error('--detach is only valid with serve or run');
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
      const { pid, logFile } = await startDetached(fileURLToPath(import.meta.url), stateFile, values.model);
      console.log('Bridge broker started (PID ' + pid + '). Private log: ' + logFile);
      return;
    }
    await privateDirectory(dirname(stateFile));
    const pluginPath = resolve(root, 'plugin');
    const broker = await createBroker({
      sessionFile: stateFile + '.sessions.json',
      deferPersistence: true,
      discover: createNativeDiscovery({ stateFile, pluginPath }),
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
    let stopping = false;
    async function stop() {
      if (stopping) return;
      stopping = true;
      try { await broker.close(); }
      finally {
        let current;
        try { current = JSON.parse(await readFile(stateFile, 'utf8')); }
        catch { /* Leave an unreadable or replaced descriptor for manual inspection. */ }
        if (current?.token === broker.token) await unlink(stateFile);
      }
    }
    // The session path is derived from this descriptor. A competing serve cannot
    // reach recovery because it cannot claim the descriptor with exclusive create.
    try { await broker.preparePersistence(); }
    catch (error) { await stop(); throw error; }
    console.error('Workspace bridge listening on ' + broker.url + '; private descriptor: ' + stateFile);
    process.once('SIGINT', () => stop().catch(error => console.error(error.message)));
    process.once('SIGTERM', () => stop().catch(error => console.error(error.message)));
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
    console.log(JSON.stringify(await client.request('POST', '/v1/sessions', { cwd: resolve(values.cwd || process.cwd()) }), null, 2));
  } else if (command === 'list') {
    console.log(JSON.stringify(await client.request('GET', '/v1/sessions'), null, 2));
  } else {
    if (!id) throw new Error('Expected a full session ID from start or list');
    const session = await client.request('GET', '/v1/sessions/' + encodeURIComponent(id));
    if (command === 'inspect') console.log(JSON.stringify(session, null, 2));
    if (command === 'control') await client.request('POST', '/v1/sessions/' + id + '/mode', { mode: rest[0] });
    if (command === 'stop') {
      try { await execute('claude', ['stop', id.slice(0, 8)], { timeout: 30000, maxBuffer: 1024 * 1024 }); }
      catch { throw new Error('Native Claude stop failed; inspect the native terminal before retrying.'); }
    }
    if (command === 'attach') {
      // Taking control does not interrupt work already running. /bridge-auto
      // explicitly gives automation permission to submit again after detaching.
      await client.request('POST', '/v1/sessions/' + id + '/mode', { mode: 'human' });
      const child = spawn('claude', ['attach', id.slice(0, 8)], { stdio: 'inherit' });
      await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', code => code === 0 ? resolve() : reject(new Error('claude attach exited with ' + code))); });
    }
  }
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });

import { spawn } from 'node:child_process';
import { constants } from 'node:fs';
import { lstat, mkdir, open } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';

/** Require an owner-only runtime directory without changing existing permissions. */
export async function privateDirectory(directory) {
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const info = await lstat(directory);
  if (!info.isDirectory() || info.uid !== process.getuid() || (info.mode & 0o077) !== 0) {
    throw new Error('Bridge state directory must be an owner-only directory (chmod 700): ' + directory);
  }
}

/** Start one broker in a new OS session; return only after its descriptor is ready. */
export async function startDetached(cli, stateFile, model, { codex = false, opencode = false } = {}) {
  await privateDirectory(dirname(stateFile));
  try {
    await lstat(stateFile);
    throw new Error('Bridge descriptor already exists: ' + stateFile + '. Broker may be running or the descriptor may be stale; inspect it before explicitly starting again. Nothing was restarted.');
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  const logFile = resolve(dirname(stateFile), 'broker.log');
  const log = await open(logFile, constants.O_WRONLY | constants.O_APPEND | constants.O_CREAT | constants.O_NOFOLLOW | constants.O_NONBLOCK, 0o600);
  try {
    const info = await log.stat();
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) !== 0) {
      throw new Error('Bridge log must be an owner-only regular file (chmod 600): ' + logFile);
    }
    const args = [cli, 'serve', '--state', stateFile];
    if (model) args.push('--model', model);
    if (codex) args.push('--codex');
    if (opencode) args.push('--opencode');
    const child = spawn(process.execPath, args, { detached: true, stdio: ['ignore', log.fd, log.fd, 'ipc'] });
    try {
      await new Promise((resolve, reject) => {
        const timer = setTimeout(() => finish(new Error('Broker startup timed out; inspect the private log: ' + logFile)), opencode ? 90000 : codex ? 45000 : 10000);
        function finish(error) {
          clearTimeout(timer);
          child.off('error', onError);
          child.off('exit', onExit);
          child.off('message', onMessage);
          if (error) reject(error);
          else resolve();
        }
        function onError() { finish(new Error('Cannot start broker; inspect the private log: ' + logFile)); }
        function onExit(code, signal) { finish(new Error('Broker exited before startup (' + (signal || code) + '); inspect the private log: ' + logFile)); }
        function onMessage(message) { if (message?.type === 'bridge-ready') finish(); }
        child.once('error', onError);
        child.once('exit', onExit);
        child.on('message', onMessage);
      });
    } catch (error) {
      // Kill only this attempted start, never a process named by an old descriptor.
      child.kill('SIGTERM');
      throw error;
    } finally {
      if (child.connected) child.disconnect();
      child.unref();
    }
    return { pid: child.pid, logFile };
  } finally {
    await log.close();
  }
}
